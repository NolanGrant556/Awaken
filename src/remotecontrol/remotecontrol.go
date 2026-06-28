package remotecontrol

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	MessageTypeControlRequest  = "control_request"
	MessageTypeControlResponse = "control_response"
	MessageTypeUser            = "user"

	ResponseSubtypeSuccess = "success"
	ResponseSubtypeError   = "error"

	RequestSubtypeInitialize           = "initialize"
	RequestSubtypeSetModel             = "set_model"
	RequestSubtypeSetMaxThinkingTokens = "set_max_thinking_tokens"
	RequestSubtypeSetPermissionMode    = "set_permission_mode"
	RequestSubtypeInterrupt            = "interrupt"
)

const outboundOnlyError = "This session is outbound-only. Enable Remote Control locally to allow inbound control."

type ControlRequest struct {
	Type      string                `json:"type"`
	RequestID string                `json:"request_id"`
	Request   ControlRequestPayload `json:"request"`
}

type ControlRequestPayload struct {
	Subtype           string  `json:"subtype"`
	Model             *string `json:"model,omitempty"`
	MaxThinkingTokens *int    `json:"max_thinking_tokens,omitempty"`
	Mode              string  `json:"mode,omitempty"`
}

type ControlResponse struct {
	Type     string              `json:"type"`
	Response ControlResponseBody `json:"response"`
}

type ControlResponseBody struct {
	Subtype   string `json:"subtype"`
	RequestID string `json:"request_id"`
	Error     string `json:"error,omitempty"`
	Response  any    `json:"response,omitempty"`
}

type SDKMessage struct {
	Type string          `json:"type"`
	UUID string          `json:"uuid,omitempty"`
	Raw  json.RawMessage `json:"-"`
}

type InitializeCapabilities struct {
	Commands              []string       `json:"commands"`
	OutputStyle           string         `json:"output_style"`
	AvailableOutputStyles []string       `json:"available_output_styles"`
	Models                []string       `json:"models"`
	Account               map[string]any `json:"account"`
	PID                   int            `json:"pid"`
}

type PermissionModeResult struct {
	OK    bool
	Error string
}

type Options struct {
	OutboundOnly bool
	PID          int

	OnInterrupt            func()
	OnSetModel             func(model *string)
	OnSetMaxThinkingTokens func(maxTokens *int)
	OnSetPermissionMode    func(mode string) PermissionModeResult
}

func HandleRequest(req ControlRequest, opts Options) ControlResponse {
	subtype := req.Request.Subtype
	if opts.OutboundOnly && subtype != RequestSubtypeInitialize {
		return errorResponse(req.RequestID, outboundOnlyError)
	}

	switch subtype {
	case RequestSubtypeInitialize:
		pid := opts.PID
		if pid == 0 {
			pid = os.Getpid()
		}
		return successResponse(req.RequestID, InitializeCapabilities{
			Commands:              []string{},
			OutputStyle:           "normal",
			AvailableOutputStyles: []string{"normal"},
			Models:                []string{},
			Account:               map[string]any{},
			PID:                   pid,
		})
	case RequestSubtypeSetModel:
		if opts.OnSetModel != nil {
			opts.OnSetModel(req.Request.Model)
		}
		return successResponse(req.RequestID, nil)
	case RequestSubtypeSetMaxThinkingTokens:
		if opts.OnSetMaxThinkingTokens != nil {
			opts.OnSetMaxThinkingTokens(req.Request.MaxThinkingTokens)
		}
		return successResponse(req.RequestID, nil)
	case RequestSubtypeSetPermissionMode:
		if opts.OnSetPermissionMode == nil {
			return errorResponse(req.RequestID, "set_permission_mode is not supported in this context (OnSetPermissionMode callback not registered)")
		}
		result := opts.OnSetPermissionMode(req.Request.Mode)
		if result.OK {
			return successResponse(req.RequestID, nil)
		}
		if result.Error == "" {
			result.Error = "set_permission_mode failed"
		}
		return errorResponse(req.RequestID, result.Error)
	case RequestSubtypeInterrupt:
		if opts.OnInterrupt != nil {
			opts.OnInterrupt()
		}
		return successResponse(req.RequestID, nil)
	default:
		return errorResponse(req.RequestID, fmt.Sprintf("Awaken remote control does not handle control_request subtype: %s", subtype))
	}
}

type UUIDSets struct {
	RecentPosted  *BoundedUUIDSet
	RecentInbound *BoundedUUIDSet
}

type IngressHandlers struct {
	RequestOptions    Options
	OnControlRequest  func(ControlRequest, ControlResponse)
	OnControlResponse func(ControlResponse)
	OnUserMessage     func(SDKMessage)
}

type IngressKind string

const (
	IngressInvalid         IngressKind = "invalid"
	IngressIgnored         IngressKind = "ignored"
	IngressControlRequest  IngressKind = "control_request"
	IngressControlResponse IngressKind = "control_response"
	IngressUserMessage     IngressKind = "user_message"
)

type RouteResult struct {
	Kind            IngressKind
	ControlRequest  *ControlRequest
	ControlResponse *ControlResponse
	ResponseToSend  *ControlResponse
	SDKMessage      *SDKMessage
	IgnoredReason   string
	Err             error
}

func RouteIngress(data []byte, sets UUIDSets, handlers IngressHandlers) RouteResult {
	var envelope struct {
		Type      string           `json:"type"`
		UUID      string           `json:"uuid"`
		RequestID string           `json:"request_id"`
		Request   *json.RawMessage `json:"request"`
		Response  *json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return RouteResult{Kind: IngressInvalid, Err: err}
	}

	if envelope.Type == MessageTypeControlResponse && envelope.Response != nil {
		var response ControlResponse
		if err := json.Unmarshal(data, &response); err != nil {
			return RouteResult{Kind: IngressInvalid, Err: err}
		}
		if handlers.OnControlResponse != nil {
			handlers.OnControlResponse(response)
		}
		return RouteResult{Kind: IngressControlResponse, ControlResponse: &response}
	}

	if envelope.Type == MessageTypeControlRequest && envelope.RequestID != "" && envelope.Request != nil {
		var request ControlRequest
		if err := json.Unmarshal(data, &request); err != nil {
			return RouteResult{Kind: IngressInvalid, Err: err}
		}
		response := HandleRequest(request, handlers.RequestOptions)
		if handlers.OnControlRequest != nil {
			handlers.OnControlRequest(request, response)
		}
		return RouteResult{Kind: IngressControlRequest, ControlRequest: &request, ResponseToSend: &response}
	}

	if envelope.Type == "" {
		return RouteResult{Kind: IngressIgnored, IgnoredReason: "missing_type"}
	}

	message := SDKMessage{
		Type: envelope.Type,
		UUID: envelope.UUID,
		Raw:  append(json.RawMessage(nil), data...),
	}
	if message.UUID != "" && sets.RecentPosted != nil && sets.RecentPosted.Has(message.UUID) {
		return RouteResult{Kind: IngressIgnored, IgnoredReason: "posted_echo", SDKMessage: &message}
	}
	if message.UUID != "" && sets.RecentInbound != nil && sets.RecentInbound.Has(message.UUID) {
		return RouteResult{Kind: IngressIgnored, IgnoredReason: "inbound_duplicate", SDKMessage: &message}
	}
	if message.Type != MessageTypeUser {
		return RouteResult{Kind: IngressIgnored, IgnoredReason: "non_user_message", SDKMessage: &message}
	}

	if message.UUID != "" && sets.RecentInbound != nil {
		sets.RecentInbound.Add(message.UUID)
	}
	if handlers.OnUserMessage != nil {
		handlers.OnUserMessage(message)
	}
	return RouteResult{Kind: IngressUserMessage, SDKMessage: &message}
}

type BoundedUUIDSet struct {
	ring     []string
	set      map[string]struct{}
	writeIdx int
}

func NewBoundedUUIDSet(capacity int) *BoundedUUIDSet {
	if capacity < 1 {
		capacity = 1
	}
	return &BoundedUUIDSet{
		ring: make([]string, capacity),
		set:  make(map[string]struct{}, capacity),
	}
}

func (s *BoundedUUIDSet) Add(uuid string) {
	if uuid == "" {
		return
	}
	if _, ok := s.set[uuid]; ok {
		return
	}
	evicted := s.ring[s.writeIdx]
	if evicted != "" {
		delete(s.set, evicted)
	}
	s.ring[s.writeIdx] = uuid
	s.set[uuid] = struct{}{}
	s.writeIdx = (s.writeIdx + 1) % len(s.ring)
}

func (s *BoundedUUIDSet) Has(uuid string) bool {
	_, ok := s.set[uuid]
	return ok
}

func (s *BoundedUUIDSet) Clear() {
	clear(s.set)
	for i := range s.ring {
		s.ring[i] = ""
	}
	s.writeIdx = 0
}

func successResponse(requestID string, payload any) ControlResponse {
	body := ControlResponseBody{
		Subtype:   ResponseSubtypeSuccess,
		RequestID: requestID,
	}
	if payload != nil {
		body.Response = payload
	}
	return ControlResponse{
		Type:     MessageTypeControlResponse,
		Response: body,
	}
}

func errorResponse(requestID string, message string) ControlResponse {
	return ControlResponse{
		Type: MessageTypeControlResponse,
		Response: ControlResponseBody{
			Subtype:   ResponseSubtypeError,
			RequestID: requestID,
			Error:     message,
		},
	}
}
