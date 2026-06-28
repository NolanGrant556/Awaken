package remotesdk

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type PermissionDecision string

const (
	PermissionAllow PermissionDecision = "allow"
	PermissionDeny  PermissionDecision = "deny"
)

type SDKControlRequest struct {
	Type      string                      `json:"type"`
	RequestID string                      `json:"request_id"`
	Request   SDKControlPermissionRequest `json:"request"`
}

type SDKControlPermissionRequest struct {
	Subtype               string           `json:"subtype"`
	ToolName              string           `json:"tool_name"`
	Input                 map[string]any   `json:"input"`
	PermissionSuggestions []map[string]any `json:"permission_suggestions,omitempty"`
	BlockedPath           string           `json:"blocked_path,omitempty"`
	DecisionReason        string           `json:"decision_reason,omitempty"`
	Title                 string           `json:"title,omitempty"`
	DisplayName           string           `json:"display_name,omitempty"`
	ToolUseID             string           `json:"tool_use_id"`
	AgentID               string           `json:"agent_id,omitempty"`
	Description           string           `json:"description,omitempty"`
}

type PermissionRequestRecord struct {
	RequestID             string
	ToolUseID             string
	ToolName              string
	Input                 map[string]any
	PermissionSuggestions []map[string]any
	BlockedPath           string
	DecisionReason        string
	Title                 string
	DisplayName           string
	AgentID               string
	Description           string
	Timestamp             time.Time
}

type PermissionResponseRecord struct {
	RequestID    string
	ToolUseID    string
	ToolName     string
	Decision     PermissionDecision
	UpdatedInput map[string]any
	Message      string
	Timestamp    time.Time
}

type SDKControlResponse struct {
	Type     string                 `json:"type"`
	Response SDKControlResponseBody `json:"response"`
}

type SDKControlResponseBody struct {
	Subtype   string                       `json:"subtype"`
	RequestID string                       `json:"request_id"`
	Response  SDKPermissionResponsePayload `json:"response,omitempty"`
	Error     string                       `json:"error,omitempty"`
}

type SDKPermissionResponsePayload struct {
	Behavior     PermissionDecision `json:"behavior"`
	UpdatedInput map[string]any     `json:"updatedInput,omitempty"`
	Message      string             `json:"message,omitempty"`
}

func NewPermissionRequestRecord(requestID string, request SDKControlPermissionRequest, opts ConvertOptions) (PermissionRequestRecord, error) {
	record := PermissionRequestRecord{
		RequestID:             strings.TrimSpace(requestID),
		ToolUseID:             strings.TrimSpace(request.ToolUseID),
		ToolName:              strings.TrimSpace(request.ToolName),
		Input:                 cloneMap(request.Input),
		PermissionSuggestions: cloneMapSlice(request.PermissionSuggestions),
		BlockedPath:           strings.TrimSpace(request.BlockedPath),
		DecisionReason:        strings.TrimSpace(request.DecisionReason),
		Title:                 strings.TrimSpace(request.Title),
		DisplayName:           strings.TrimSpace(request.DisplayName),
		AgentID:               strings.TrimSpace(request.AgentID),
		Description:           strings.TrimSpace(request.Description),
		Timestamp:             fallbackTimestamp(opts),
	}
	if record.Input == nil {
		record.Input = map[string]any{}
	}
	if record.Title == "" {
		record.Title = fmt.Sprintf("Approve %s", firstNonEmpty(record.DisplayName, record.ToolName))
	}
	if record.Description == "" {
		record.Description = permissionRequestDescription(record)
	}
	if err := record.Validate(); err != nil {
		return PermissionRequestRecord{}, err
	}
	return record, nil
}

func PermissionRequestRecordFromControl(request SDKControlRequest, opts ConvertOptions) (PermissionRequestRecord, error) {
	if request.Type != "" && request.Type != "control_request" {
		return PermissionRequestRecord{}, fmt.Errorf("unsupported control request type %q", request.Type)
	}
	if request.Request.Subtype != "" && request.Request.Subtype != "can_use_tool" {
		return PermissionRequestRecord{}, fmt.Errorf("unsupported control request subtype %q", request.Request.Subtype)
	}
	return NewPermissionRequestRecord(request.RequestID, request.Request, opts)
}

func (r PermissionRequestRecord) Validate() error {
	switch {
	case strings.TrimSpace(r.RequestID) == "":
		return errors.New("permission request_id is required")
	case strings.TrimSpace(r.ToolUseID) == "":
		return errors.New("permission tool_use_id is required")
	case strings.TrimSpace(r.ToolName) == "":
		return errors.New("permission tool_name is required")
	case r.Timestamp.IsZero():
		return errors.New("permission timestamp is required")
	default:
		return nil
	}
}

func (r PermissionRequestRecord) Event() AwakenRemoteEvent {
	return AwakenRemoteEvent{
		Type:              EventPermissionRequest,
		Severity:          SeverityWarning,
		Timestamp:         r.Timestamp,
		ToolUseID:         r.ToolUseID,
		ToolName:          r.ToolName,
		Content:           r.Description,
		PermissionRequest: &r,
	}
}

func NewPermissionResponseRecord(request PermissionRequestRecord, decision PermissionDecision, updatedInput map[string]any, message string, opts ConvertOptions) (PermissionResponseRecord, error) {
	record := PermissionResponseRecord{
		RequestID:    strings.TrimSpace(request.RequestID),
		ToolUseID:    strings.TrimSpace(request.ToolUseID),
		ToolName:     strings.TrimSpace(request.ToolName),
		Decision:     decision,
		UpdatedInput: cloneMap(updatedInput),
		Message:      strings.TrimSpace(message),
		Timestamp:    fallbackTimestamp(opts),
	}
	if record.Decision == PermissionAllow && record.UpdatedInput == nil {
		record.UpdatedInput = map[string]any{}
	}
	if err := record.Validate(); err != nil {
		return PermissionResponseRecord{}, err
	}
	return record, nil
}

func (r PermissionResponseRecord) Validate() error {
	switch {
	case strings.TrimSpace(r.RequestID) == "":
		return errors.New("permission response request_id is required")
	case r.Decision != PermissionAllow && r.Decision != PermissionDeny:
		return fmt.Errorf("unsupported permission decision %q", r.Decision)
	case r.Decision == PermissionDeny && strings.TrimSpace(r.Message) == "":
		return errors.New("permission denial message is required")
	case r.Timestamp.IsZero():
		return errors.New("permission response timestamp is required")
	default:
		return nil
	}
}

func (r PermissionResponseRecord) Event() AwakenRemoteEvent {
	severity := SeverityInfo
	if r.Decision == PermissionDeny {
		severity = SeverityWarning
	}
	return AwakenRemoteEvent{
		Type:               EventPermissionResult,
		Severity:           severity,
		Timestamp:          r.Timestamp,
		ToolUseID:          r.ToolUseID,
		ToolName:           r.ToolName,
		Content:            firstNonEmpty(r.Message, string(r.Decision)),
		PermissionResponse: &r,
	}
}

func (r PermissionResponseRecord) SDKControlResponse() SDKControlResponse {
	payload := SDKPermissionResponsePayload{
		Behavior: r.Decision,
	}
	if r.Decision == PermissionAllow {
		payload.UpdatedInput = cloneMap(r.UpdatedInput)
	} else {
		payload.Message = r.Message
	}
	return SDKControlResponse{
		Type: "control_response",
		Response: SDKControlResponseBody{
			Subtype:   "success",
			RequestID: r.RequestID,
			Response:  payload,
		},
	}
}

func UnsupportedControlResponse(requestID string, subtype string) SDKControlResponse {
	return SDKControlResponse{
		Type: "control_response",
		Response: SDKControlResponseBody{
			Subtype:   "error",
			RequestID: strings.TrimSpace(requestID),
			Error:     fmt.Sprintf("Unsupported control request subtype: %s", strings.TrimSpace(subtype)),
		},
	}
}

func permissionRequestDescription(record PermissionRequestRecord) string {
	switch {
	case record.DecisionReason != "" && record.BlockedPath != "":
		return fmt.Sprintf("%s\n\nBlocked path: %s", record.DecisionReason, record.BlockedPath)
	case record.DecisionReason != "":
		return record.DecisionReason
	case record.BlockedPath != "":
		return fmt.Sprintf("Tool request touches blocked path: %s", record.BlockedPath)
	default:
		return fmt.Sprintf("%s requests permission to run.", firstNonEmpty(record.DisplayName, record.ToolName))
	}
}

func cloneMapSlice(in []map[string]any) []map[string]any {
	if in == nil {
		return nil
	}
	out := make([]map[string]any, 0, len(in))
	for _, value := range in {
		out = append(out, cloneMap(value))
	}
	return out
}
