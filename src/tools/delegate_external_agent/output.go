package delegateexternalagent

import (
	"encoding/json"
	"time"

	"awaken/src/acp"
	"awaken/src/tool"
)

const (
	delegateActionList    = "list"
	delegateActionStatus  = "status"
	delegateActionStart   = "start"
	delegateActionMessage = "message"
	delegateActionCancel  = "cancel"
	delegateActionClose   = "close"
)

type runnerOutput struct {
	ID             string   `json:"id"`
	Command        string   `json:"command"`
	Args           []string `json:"args"`
	PermissionMode string   `json:"permission_mode"`
}

type listOutput struct {
	Action  string         `json:"action"`
	Runners []runnerOutput `json:"runners"`
}

type permissionOptionOutput struct {
	OptionID    string `json:"option_id"`
	Name        string `json:"name"`
	Kind        string `json:"kind,omitempty"`
	Description string `json:"description,omitempty"`
}

type pendingPermissionOutput struct {
	InteractionID string                   `json:"interaction_id"`
	ToolCallID    string                   `json:"tool_call_id,omitempty"`
	Title         string                   `json:"title,omitempty"`
	Kind          string                   `json:"kind,omitempty"`
	Paths         []string                 `json:"paths,omitempty"`
	Command       string                   `json:"command,omitempty"`
	Options       []permissionOptionOutput `json:"options,omitempty"`
	ExpiresAt     time.Time                `json:"expires_at"`
}

type sessionOutput struct {
	ThreadID          string                   `json:"thread_id"`
	RunnerID          string                   `json:"runner_id"`
	ACPSessionID      string                   `json:"acp_session_id,omitempty"`
	SessionState      string                   `json:"session_state"`
	WorkspaceRoot     string                   `json:"workspace_root,omitempty"`
	ActiveTurnID      string                   `json:"active_turn_id,omitempty"`
	PendingPermission *pendingPermissionOutput `json:"pending_permission,omitempty"`
	LastErrorCode     string                   `json:"last_error_code,omitempty"`
	LastErrorMessage  string                   `json:"last_error_message,omitempty"`
	StartedAt         time.Time                `json:"started_at"`
	UpdatedAt         time.Time                `json:"updated_at"`
}

type statusSessionsOutput struct {
	Action   string          `json:"action"`
	ThreadID string          `json:"thread_id"`
	Sessions []sessionOutput `json:"sessions"`
}

type statusRunnerOutput struct {
	Action            string                   `json:"action"`
	ThreadID          string                   `json:"thread_id"`
	RunnerID          string                   `json:"runner_id"`
	ACPSessionID      string                   `json:"acp_session_id,omitempty"`
	SessionState      string                   `json:"session_state"`
	ActiveTurnID      string                   `json:"active_turn_id,omitempty"`
	PendingPermission *pendingPermissionOutput `json:"pending_permission,omitempty"`
	LastErrorCode     string                   `json:"last_error_code,omitempty"`
	LastErrorMessage  string                   `json:"last_error_message,omitempty"`
}

type toolSummaryOutput struct {
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Total     int `json:"total"`
}

type turnOutput struct {
	Action          string            `json:"action"`
	RunnerID        string            `json:"runner_id"`
	SessionState    string            `json:"session_state"`
	ACPStopReason   string            `json:"acp_stop_reason"`
	AssistantText   string            `json:"assistant_text"`
	ToolSummary     toolSummaryOutput `json:"tool_summary"`
	PermissionState string            `json:"permission_state"`
	Truncated       bool              `json:"truncated"`
	ErrorCode       string            `json:"error_code,omitempty"`
	ErrorMessage    string            `json:"error_message,omitempty"`
}

type sessionActionOutput struct {
	Action       string `json:"action"`
	RunnerID     string `json:"runner_id"`
	SessionState string `json:"session_state"`
}

func marshalDelegateOutput(output any) (string, error) {
	data, err := json.Marshal(output)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func runnerOutputFromConfig(config acp.RunnerSnapshot) runnerOutput {
	args := make([]string, len(config.Args))
	copy(args, config.Args)
	return runnerOutput{
		ID:             config.ID,
		Command:        config.Command,
		Args:           args,
		PermissionMode: string(config.PermissionMode),
	}
}

func pendingPermissionOutputFromRequest(request *acp.PermissionRequest) *pendingPermissionOutput {
	if request == nil {
		return nil
	}
	options := make([]permissionOptionOutput, len(request.Options))
	for index, option := range request.Options {
		options[index] = permissionOptionOutput{
			OptionID:    option.OptionID,
			Name:        option.Name,
			Kind:        option.Kind,
			Description: option.Description,
		}
	}
	paths := make([]string, len(request.Paths))
	copy(paths, request.Paths)
	return &pendingPermissionOutput{
		InteractionID: request.InteractionID,
		ToolCallID:    request.ToolCallID,
		Title:         request.Title,
		Kind:          request.Kind,
		Paths:         paths,
		Command:       request.Command,
		Options:       options,
		ExpiresAt:     request.ExpiresAt,
	}
}

func sessionOutputFromSummary(summary acp.SessionSummary) sessionOutput {
	return sessionOutput{
		ThreadID:          summary.ThreadID,
		RunnerID:          summary.RunnerID,
		ACPSessionID:      summary.ACPSessionID,
		SessionState:      string(summary.State),
		WorkspaceRoot:     summary.WorkspaceRoot,
		ActiveTurnID:      summary.ActiveTurnID,
		PendingPermission: pendingPermissionOutputFromRequest(summary.PendingPermission),
		LastErrorCode:     summary.LastErrorCode,
		LastErrorMessage:  summary.LastErrorMessage,
		StartedAt:         summary.StartedAt,
		UpdatedAt:         summary.UpdatedAt,
	}
}

func statusRunnerOutputFromSummary(action, threadID, runnerID string, summary acp.SessionSummary) statusRunnerOutput {
	return statusRunnerOutput{
		Action:            action,
		ThreadID:          threadID,
		RunnerID:          runnerID,
		ACPSessionID:      summary.ACPSessionID,
		SessionState:      string(summary.State),
		ActiveTurnID:      summary.ActiveTurnID,
		PendingPermission: pendingPermissionOutputFromRequest(summary.PendingPermission),
		LastErrorCode:     summary.LastErrorCode,
		LastErrorMessage:  summary.LastErrorMessage,
	}
}

func turnOutputFromResult(action, runnerID, sessionState string, result acp.TurnResult) turnOutput {
	return turnOutput{
		Action:        action,
		RunnerID:      runnerID,
		SessionState:  sessionState,
		ACPStopReason: result.StopReason,
		AssistantText: result.AssistantText,
		ToolSummary: toolSummaryOutput{
			Completed: result.ToolSummary.Completed,
			Failed:    result.ToolSummary.Failed,
			Total:     result.ToolSummary.Total,
		},
		PermissionState: "none",
		Truncated:       result.Truncated,
		ErrorCode:       result.ErrorCode,
		ErrorMessage:    result.ErrorMessage,
	}
}

func delegateOutputSchema() tool.ToolParam {
	return tool.ToolParam{
		Type: "object",
		OneOf: []tool.ToolParam{
			listOutputSchema(),
			statusSessionsOutputSchema(),
			statusRunnerOutputSchema(),
			turnOutputSchema(delegateActionStart),
			turnOutputSchema(delegateActionMessage),
			sessionActionOutputSchema(delegateActionCancel),
			sessionActionOutputSchema(delegateActionClose),
		},
	}
}

func listOutputSchema() tool.ToolParam {
	return strictObject([]string{"action", "runners"}, map[string]tool.ToolParam{
		"action": actionSchema(delegateActionList),
		"runners": {Type: "array", Items: &tool.ToolParam{
			Type: "object", AdditionalProperties: tool.BoolPtr(false),
			Required: []string{"id", "command", "args", "permission_mode"},
			Properties: map[string]tool.ToolParam{
				"id":              {Type: "string"},
				"command":         {Type: "string"},
				"args":            {Type: "array", Items: &tool.ToolParam{Type: "string"}},
				"permission_mode": {Type: "string"},
			},
		}},
	})
}

func statusSessionsOutputSchema() tool.ToolParam {
	return strictObject([]string{"action", "thread_id", "sessions"}, map[string]tool.ToolParam{
		"action":    actionSchema(delegateActionStatus),
		"thread_id": {Type: "string"},
		"sessions":  {Type: "array", Items: sessionOutputSchema()},
	})
}

func statusRunnerOutputSchema() tool.ToolParam {
	properties := map[string]tool.ToolParam{
		"action":             actionSchema(delegateActionStatus),
		"thread_id":          {Type: "string"},
		"runner_id":          {Type: "string"},
		"acp_session_id":     {Type: "string"},
		"session_state":      {Type: "string"},
		"active_turn_id":     {Type: "string"},
		"pending_permission": pendingPermissionOutputSchema(),
		"last_error_code":    {Type: "string"},
		"last_error_message": {Type: "string"},
	}
	return strictObject([]string{"action", "thread_id", "runner_id", "session_state"}, properties)
}

func sessionOutputSchema() *tool.ToolParam {
	properties := map[string]tool.ToolParam{
		"thread_id":          {Type: "string"},
		"runner_id":          {Type: "string"},
		"acp_session_id":     {Type: "string"},
		"session_state":      {Type: "string"},
		"workspace_root":     {Type: "string"},
		"active_turn_id":     {Type: "string"},
		"pending_permission": pendingPermissionOutputSchema(),
		"last_error_code":    {Type: "string"},
		"last_error_message": {Type: "string"},
		"started_at":         {Type: "string"},
		"updated_at":         {Type: "string"},
	}
	schema := strictObject([]string{"thread_id", "runner_id", "session_state", "started_at", "updated_at"}, properties)
	return &schema
}

func pendingPermissionOutputSchema() tool.ToolParam {
	return strictObject([]string{"interaction_id", "expires_at"}, map[string]tool.ToolParam{
		"interaction_id": {Type: "string"},
		"tool_call_id":   {Type: "string"},
		"title":          {Type: "string"},
		"kind":           {Type: "string"},
		"paths":          {Type: "array", Items: &tool.ToolParam{Type: "string"}},
		"command":        {Type: "string"},
		"options": {Type: "array", Items: &tool.ToolParam{
			Type: "object", AdditionalProperties: tool.BoolPtr(false),
			Required: []string{"option_id", "name"},
			Properties: map[string]tool.ToolParam{
				"option_id":   {Type: "string"},
				"name":        {Type: "string"},
				"kind":        {Type: "string"},
				"description": {Type: "string"},
			},
		}},
		"expires_at": {Type: "string"},
	})
}

func turnOutputSchema(action string) tool.ToolParam {
	return strictObject([]string{
		"action", "runner_id", "session_state", "acp_stop_reason", "assistant_text",
		"tool_summary", "permission_state", "truncated",
	}, map[string]tool.ToolParam{
		"action":           actionSchema(action),
		"runner_id":        {Type: "string"},
		"session_state":    {Type: "string"},
		"acp_stop_reason":  {Type: "string"},
		"assistant_text":   {Type: "string"},
		"tool_summary":     {Type: "object", AdditionalProperties: tool.BoolPtr(false), Required: []string{"completed", "failed", "total"}, Properties: map[string]tool.ToolParam{"completed": {Type: "integer"}, "failed": {Type: "integer"}, "total": {Type: "integer"}}},
		"permission_state": {Type: "string"},
		"truncated":        {Type: "boolean"},
		"error_code":       {Type: "string"},
		"error_message":    {Type: "string"},
	})
}

func sessionActionOutputSchema(action string) tool.ToolParam {
	return strictObject([]string{"action", "runner_id", "session_state"}, map[string]tool.ToolParam{
		"action":        actionSchema(action),
		"runner_id":     {Type: "string"},
		"session_state": {Type: "string"},
	})
}

func strictObject(required []string, properties map[string]tool.ToolParam) tool.ToolParam {
	return tool.ToolParam{
		Type:                 "object",
		Required:             required,
		Properties:           properties,
		AdditionalProperties: tool.BoolPtr(false),
	}
}

func actionSchema(action string) tool.ToolParam {
	return tool.ToolParam{Type: "string", Enum: []any{action}}
}
