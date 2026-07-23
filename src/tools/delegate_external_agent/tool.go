package delegateexternalagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"awaken/src/acp"
	"awaken/src/tool"
)

func threadIDFromContext(ctx context.Context) string {
	return tool.ToolThreadIDFromContext(ctx)
}

func runIDFromContext(ctx context.Context) string {
	if id, ok := tool.ToolExecutionIdentityFromContext(ctx); ok {
		return id.RunID
	}
	return ""
}

func toolCallIDFromContext(ctx context.Context) string {
	if id, ok := tool.ToolExecutionIdentityFromContext(ctx); ok {
		return id.ToolCallID
	}
	return ""
}


const ToolName = "delegate_external_agent"

type delegateInput struct {
	Action            string `json:"action"`
	Runner            string `json:"runner,omitempty"`
	Message           string `json:"message,omitempty"`
	Cwd               string `json:"cwd,omitempty"`
	MaxRuntimeSeconds int    `json:"max_runtime_seconds,omitempty"`
}

type DelegateExternalAgentTool struct {
	Manager *acp.Manager
}

var defaultTool = &DelegateExternalAgentTool{}

// SetManager injects the ACP manager into the registered tool.
func SetManager(m *acp.Manager) {
	defaultTool.Manager = m
}

func (t *DelegateExternalAgentTool) Name() string {
	return ToolName
}

func (t *DelegateExternalAgentTool) Description() string {
	return "将任务委派给已配置的 ACP（Agent Client Protocol）外部 Agent（如 Codex、Claude Code、OpenCode、Qwen Code）。支持 list/status/start/message/cancel/close 六种动作。外部 Agent 的文本回复、思考状态、工具调用进度和权限请求会投影到当前 run。"
}

func (t *DelegateExternalAgentTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "action", Description: "list | status | start | message | cancel | close", Type: "string", Required: true},
		{Name: "runner", Description: "runner ID，例如 codex、claude_code、opencode、qwen_code", Type: "string", Required: false},
		{Name: "message", Description: "start/message 时发送给外部 Agent 的任务或后续消息", Type: "string", Required: false},
		{Name: "cwd", Description: "start 时使用的相对或绝对工作目录，默认当前 run workspace root", Type: "string", Required: false},
		{Name: "max_runtime_seconds", Description: "单次 turn 最大运行秒数", Type: "integer", Required: false},
	}
}

func (t *DelegateExternalAgentTool) InputSchema() tool.ToolParametersSchema {
	noExtra := false
	return tool.ToolParametersSchema{
		Type:                 "object",
		AdditionalProperties: &noExtra,
		Required:             []string{"action"},
		Properties: map[string]tool.ToolParam{
			"action":  {Type: "string", Description: "list | status | start | message | cancel | close", Enum: []any{"list", "status", "start", "message", "cancel", "close"}},
			"runner":  {Type: "string", Description: "runner ID"},
			"message": {Type: "string", Description: "prompt text"},
			"cwd":     {Type: "string", Description: "working directory"},
			"max_runtime_seconds": {Type: "integer", Description: "per-turn timeout"},
		},
	}
}

func (t *DelegateExternalAgentTool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:        ToolName,
		DisplayName: "Delegate External Agent",
		Description: t.Description(),
		Purpose:     "把明确任务交给本地已配置的 ACP Agent，并在同一 Awaken thread 中持续对话、观察进度、处理权限和取消。",
		WhenToUse: []string{
			"任务明显超出当前 Agent 能力范围，需要专用工具链或生态时",
			"用户明确要求使用特定外部 Agent（如 Codex）时",
			"需要在外部 Agent 自己的上下文里完成多步工具调用时",
		},
		WhenNotToUse: []string{
			"当前 Awaken 工具已能完成该任务时",
			"任务涉及当前 workspace 外的敏感路径且未获授权时",
			"外部 Agent 未安装或未配置时",
		},
		InputSchema: t.InputSchema(),
		OutputSchema: tool.ToolParam{Type: "object", AdditionalProperties: tool.BoolPtr(false), Properties: map[string]tool.ToolParam{
			"runner_id":        {Type: "string"},
			"session_state":    {Type: "string"},
			"acp_stop_reason":  {Type: "string"},
			"assistant_text":   {Type: "string"},
			"tool_summary":     {Type: "object", AdditionalProperties: tool.BoolPtr(false), Properties: map[string]tool.ToolParam{"completed": {Type: "integer"}, "failed": {Type: "integer"}, "total": {Type: "integer"}}},
			"permission_state": {Type: "string"},
		}},
		Examples: []tool.ToolExample{
			{Scenario: "Start a task with Codex", Input: map[string]any{"action": "start", "runner": "codex", "message": "Refactor the auth module to use OAuth2"}, Outcome: "Starts an ACP session with Codex and returns the turn result summary."},
			{Scenario: "Send a follow-up message", Input: map[string]any{"action": "message", "runner": "codex", "message": "Also update the tests"}, Outcome: "Continues the existing session and returns the updated turn result."},
			{Scenario: "List configured runners", Input: map[string]any{"action": "list"}, Outcome: "Returns the list of enabled ACP runners."},
		},
		FailureModes: []tool.ToolFailureMode{
			{Code: "acp_runner_not_configured", Cause: "The requested runner is not configured or disabled", Retryable: false},
			{Code: "acp_runner_not_ready", Cause: "The runner command is not available on PATH", Retryable: true},
			{Code: "acp_session_conflict", Cause: "A session already exists for this thread and runner", Retryable: false},
			{Code: "acp_transport_closed", Cause: "The ACP transport closed unexpectedly", Retryable: true},
			{Code: "acp_permission_denied", Cause: "The user denied a permission request from the external agent", Retryable: false},
			{Code: "acp_turn_timeout", Cause: "The external agent turn exceeded the configured timeout", Retryable: true},
		},
		SideEffects: []tool.ToolSideEffect{{Kind: tool.SideEffectUpdate, Description: "可能修改 workspace 中的文件，取决于外部 Agent 的行为。", Reversibility: tool.ReversibilityUnknown}},
		Annotations: tool.ToolAnnotations{
			ReadOnly:        false,
			Destructive:     true,
			OpenWorld:       true,
			CapabilityClass: tool.CapabilityExternalPublish,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
		},
	})
}

func init() {
	tool.MustRegisterToolWithMode(defaultTool, tool.DefinitionValidationStrictBuiltin)
}

func (t *DelegateExternalAgentTool) Execute(ctx context.Context, input string) (string, error) {
	var req delegateInput
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	if req.Action == "" {
		return "", fmt.Errorf("action is required")
	}
	if t.Manager == nil {
		return "", fmt.Errorf("acp manager not available")
	}

	switch req.Action {
	case "list":
		return t.listRunners()
	case "status":
		return t.status(ctx, req.Runner)
	case "start":
		return t.start(ctx, req)
	case "message":
		return t.message(ctx, req)
	case "cancel":
		return t.cancel(ctx, req)
	case "close":
		return t.close(ctx, req)
	default:
		return "", fmt.Errorf("unknown action %q", req.Action)
	}
}

func (t *DelegateExternalAgentTool) listRunners() (string, error) {
	runners := t.Manager.Registry().ListEnabled()
	items := make([]map[string]any, 0, len(runners))
	for _, r := range runners {
		items = append(items, map[string]any{
			"id":             r.ID,
			"command":        r.Command,
			"args":           r.Args,
			"permission_mode": string(r.PermissionMode),
		})
	}
	out := map[string]any{"runners": items}
	b, _ := json.Marshal(out)
	return string(b), nil
}

func (t *DelegateExternalAgentTool) status(ctx context.Context, runnerID string) (string, error) {
	threadID := threadIDFromContext(ctx)
	out := map[string]any{"thread_id": threadID}
	if runnerID != "" {
		if conv, ok := t.Manager.GetConversation(threadID, runnerID); ok {
			summary := conv.Summary()
			out["runner_id"] = summary.RunnerID
			out["session_state"] = string(summary.State)
			out["acp_session_id"] = summary.ACPSessionID
			if summary.ActiveTurnID != "" {
				out["active_turn_id"] = summary.ActiveTurnID
			}
			if summary.PendingPermission != nil {
				out["pending_permission"] = summary.PendingPermission
			}
		} else {
			out["session_state"] = "absent"
		}
	} else {
		out["sessions"] = t.Manager.ListSessions()
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

func (t *DelegateExternalAgentTool) start(ctx context.Context, req delegateInput) (string, error) {
	if strings.TrimSpace(req.Runner) == "" {
		return "", fmt.Errorf("runner is required for start")
	}
	if strings.TrimSpace(req.Message) == "" {
		return "", fmt.Errorf("message is required for start")
	}
	threadID := threadIDFromContext(ctx)
	pc := tool.PermissionContextFromContext(ctx)
	cwd, err := pc.ResolveACPCwd(req.Cwd)
	if err != nil {
		return "", fmt.Errorf("%s: %w", acp.ErrCodePermissionDenied, err)
	}
	conv, err := t.Manager.StartSession(ctx, threadID, req.Runner, cwd)
	if err != nil {
		return "", err
	}
	hostRunID := runIDFromContext(ctx)
	toolCallID := toolCallIDFromContext(ctx)
	emitACPEvent(ctx, tool.ACPEvent{
		Type:       tool.ACPEventTypeSessionStarted,
		ThreadID:   threadID,
		ToolCallID: toolCallID,
		Content:    fmt.Sprintf("ACP session started on runner %s", req.Runner),
		Diagnostic: map[string]any{
			"runner_id":      req.Runner,
			"acp_session_id": conv.ACPSessionID,
			"workspace_root": cwd,
		},
	})
	result, err := t.Manager.Message(ctx, threadID, req.Runner, hostRunID, toolCallID, req.Message, acpUpdateHandler(ctx, req.Runner, toolCallID), req.MaxRuntimeSeconds)
	if err != nil {
		return "", err
	}
	return formatResult(req.Runner, string(conv.State()), result), nil
}

func (t *DelegateExternalAgentTool) message(ctx context.Context, req delegateInput) (string, error) {
	if strings.TrimSpace(req.Runner) == "" {
		return "", fmt.Errorf("runner is required for message")
	}
	if strings.TrimSpace(req.Message) == "" {
		return "", fmt.Errorf("message is required for message")
	}
	threadID := threadIDFromContext(ctx)
	hostRunID := runIDFromContext(ctx)
	toolCallID := toolCallIDFromContext(ctx)
	result, err := t.Manager.Message(ctx, threadID, req.Runner, hostRunID, toolCallID, req.Message, acpUpdateHandler(ctx, req.Runner, toolCallID), req.MaxRuntimeSeconds)
	if err != nil {
		return "", err
	}
	return formatResult(req.Runner, "ready", result), nil
}

func (t *DelegateExternalAgentTool) cancel(ctx context.Context, req delegateInput) (string, error) {
	if strings.TrimSpace(req.Runner) == "" {
		return "", fmt.Errorf("runner is required for cancel")
	}
	threadID := threadIDFromContext(ctx)
	if err := t.Manager.Cancel(ctx, threadID, req.Runner); err != nil {
		return "", err
	}
	out := map[string]any{"runner_id": req.Runner, "session_state": "cancelling"}
	b, _ := json.Marshal(out)
	return string(b), nil
}

func (t *DelegateExternalAgentTool) close(ctx context.Context, req delegateInput) (string, error) {
	if strings.TrimSpace(req.Runner) == "" {
		return "", fmt.Errorf("runner is required for close")
	}
	threadID := threadIDFromContext(ctx)
	if err := t.Manager.CloseSession(ctx, threadID, req.Runner); err != nil {
		return "", err
	}
	out := map[string]any{"runner_id": req.Runner, "session_state": "closed"}
	b, _ := json.Marshal(out)
	return string(b), nil
}

func formatResult(runnerID, sessionState string, result acp.TurnResult) string {
	out := map[string]any{
		"runner_id":       runnerID,
		"session_state":   sessionState,
		"acp_stop_reason": result.StopReason,
		"assistant_text":  result.AssistantText,
		"tool_summary": map[string]any{
			"completed": result.ToolSummary.Completed,
			"failed":    result.ToolSummary.Failed,
			"total":     result.ToolSummary.Total,
		},
		"permission_state": "none",
		"truncated":        result.Truncated,
	}
	if result.ErrorCode != "" {
		out["error_code"] = result.ErrorCode
		out["error_message"] = result.ErrorMessage
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func emitACPEvent(ctx context.Context, event tool.ACPEvent) {
	if emit := tool.ACPEventEmitterFromContext(ctx); emit != nil {
		emit(event)
	}
}

func acpUpdateHandler(ctx context.Context, runnerID, hostToolCallID string) acp.UpdateHandler {
	return acp.UpdateHandlerFunc(func(update acp.SessionUpdate) {
		switch update.Kind {
		case acp.SessionUpdateKindToolCall, acp.SessionUpdateKindToolCallUpdate:
			if update.ToolCall == nil {
				return
			}
			emitACPEvent(ctx, tool.ACPEvent{
				Type:       tool.ACPEventTypeToolSnapshot,
				ThreadID:   tool.ToolThreadIDFromContext(ctx),
				ToolCallID: hostToolCallID,
				ToolName:   update.ToolCall.Kind,
				Content:    update.ToolCall.Summary,
				Diagnostic: map[string]any{
					"runner_id":      runnerID,
					"acp_session_id": update.SessionID,
					"tool_call_id":   update.ToolCall.ToolCallID,
					"title":          update.ToolCall.Title,
					"status":         update.ToolCall.Status,
					"locations":      update.ToolCall.Locations,
				},
			})
		}
	})
}
