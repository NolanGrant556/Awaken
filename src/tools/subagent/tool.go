package subagenttool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"awaken/src/subagentruntime"
	"awaken/src/tool"
)

const ToolName = "subagent"

type input struct {
	Action        string   `json:"action"`
	TaskID        string   `json:"task_id,omitempty"`
	TaskIDs       []string `json:"task_ids,omitempty"`
	Description   string   `json:"description,omitempty"`
	Prompt        string   `json:"prompt,omitempty"`
	AgentType     string   `json:"agent_type,omitempty"`
	WorkspaceMode string   `json:"workspace_mode,omitempty"`
	ReturnWhen    string   `json:"return_when,omitempty"`
	TimeoutMS     int      `json:"timeout_ms,omitempty"`
	Reason        string   `json:"reason,omitempty"`
}

type SubagentTool struct{}

var _ tool.OutcomeTool = (*SubagentTool)(nil)

func (t *SubagentTool) Name() string { return ToolName }

func (t *SubagentTool) Description() string {
	return "创建和管理当前 run 内的本地只读 Subagent。支持 spawn、status、wait、cancel；Subagent 拥有独立上下文，终态会自动通知主 Agent，status/wait 可用于显式查询。"
}

func (t *SubagentTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "action", Description: "spawn | status | wait | cancel", Type: "string", Required: true},
		{Name: "task_id", Description: "status/cancel 的子任务 ID", Type: "string"},
		{Name: "task_ids", Description: "wait 的子任务 ID 列表", Type: "array"},
		{Name: "description", Description: "spawn 的简短任务说明", Type: "string"},
		{Name: "prompt", Description: "spawn 的完整任务说明", Type: "string"},
		{Name: "agent_type", Description: "explore | review", Type: "string"},
		{Name: "workspace_mode", Description: "当前只支持 shared_readonly", Type: "string"},
		{Name: "return_when", Description: "wait 使用 any | all", Type: "string"},
		{Name: "timeout_ms", Description: "wait 超时，0-60000", Type: "integer"},
	}
}

func (t *SubagentTool) InputSchema() tool.ToolParametersSchema {
	noExtra := false
	return tool.ToolParametersSchema{
		Type: "object", AdditionalProperties: &noExtra,
		Required: []string{"action"},
		Properties: map[string]tool.ToolParam{
			"action":         {Type: "string", Description: "要执行的生命周期动作", Enum: []any{"spawn", "status", "wait", "cancel"}},
			"task_id":        {Type: "string", Description: "status 或 cancel 的目标 task ID"},
			"task_ids":       {Type: "array", Description: "wait 的目标 task ID 列表", Items: &tool.ToolParam{Type: "string"}},
			"description":    {Type: "string", Description: "用户可见的简短任务说明"},
			"prompt":         {Type: "string", Description: "交给 child 的完整任务说明"},
			"agent_type":     {Type: "string", Description: "host-owned child 角色", Enum: []any{"explore", "review"}},
			"workspace_mode": {Type: "string", Description: "child workspace 隔离模式", Enum: []any{"shared_readonly"}},
			"return_when":    {Type: "string", Description: "等待任一或全部任务完成", Enum: []any{"any", "all"}},
			"timeout_ms":     {Type: "integer", Description: "wait 超时毫秒数，最大 60000"},
			"reason":         {Type: "string", Description: "取消原因"},
		},
	}
}

func (t *SubagentTool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name: ToolName, DisplayName: "Subagent", Description: t.Description(),
		Purpose: "把独立的只读探索或复核任务交给隔离上下文中的本地 child Agent，并在当前 run 内等待或取消。",
		WhenToUse: []string{
			"存在可以独立完成的代码搜索、事实梳理或复核任务时",
			"多个只读子任务可以与主任务并行时",
			"需要独立上下文减少主 Agent 上下文污染时",
		},
		WhenNotToUse: []string{
			"任务需要直接修改文件时；当前版本只允许 shared_readonly",
			"任务很小、直接使用现有工具更快时",
			"需要 child 向用户提问、递归创建 Subagent 或脱离当前 run 运行时",
		},
		InputSchema: t.InputSchema(),
		OutputSchema: tool.ToolParam{
			Type: "object", AdditionalProperties: tool.BoolPtr(false),
			Properties: map[string]tool.ToolParam{
				"task":             {Type: "object", AdditionalProperties: tool.BoolPtr(true)},
				"tasks":            {Type: "array", Items: &tool.ToolParam{Type: "object", AdditionalProperties: tool.BoolPtr(true)}},
				"retrieval_status": {Type: "string", Enum: []any{subagentruntime.RetrievalCompleted, subagentruntime.RetrievalRunning, subagentruntime.RetrievalTimeout}},
			},
		},
		Examples: []tool.ToolExample{
			{Scenario: "创建只读探索任务", Input: map[string]any{"action": "spawn", "description": "定位认证入口", "prompt": "只读检查认证入口和调用链", "agent_type": "explore", "workspace_mode": "shared_readonly"}, Outcome: "返回稳定 task_id，任务在当前 run 后台执行。"},
			{Scenario: "等待一个子任务", Input: map[string]any{"action": "wait", "task_ids": []any{"subtask_..."}, "return_when": "all", "timeout_ms": 60000}, Outcome: "任务终态后返回结构化结果。"},
		},
		FailureModes: []tool.ToolFailureMode{
			{Code: subagentruntime.ErrorInvalidInput, Cause: "action 字段组合、长度或枚举非法", Retryable: false},
			{Code: subagentruntime.ErrorRuntimeUnavailable, Cause: "当前 run 没有 Subagent manager", Retryable: false},
			{Code: subagentruntime.ErrorLimitReached, Cause: "当前 run 已达到子任务总量限制", Retryable: false},
			{Code: subagentruntime.ErrorNotFound, Cause: "task_id 不属于当前 run", Retryable: false},
			{Code: subagentruntime.ErrorWorkspaceMode, Cause: "当前版本不支持请求的 workspace mode", Retryable: false},
		},
		SideEffects: []tool.ToolSideEffect{{
			Kind: tool.SideEffectNone, Description: "只管理当前 run 内的只读 child 生命周期，不直接修改 workspace。",
			Reversibility: tool.ReversibilityNotApplicable,
		}},
		Annotations: tool.ToolAnnotations{
			ReadOnly: true, Destructive: false, OpenWorld: false,
			CapabilityClass: tool.CapabilityLocalTransform,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
			FileBridgeScope: tool.FileBridgeNone,
			NetworkScope:    tool.NetworkScopeNone,
		},
	})
}

func (t *SubagentTool) IsReadOnly() bool { return true }

func (t *SubagentTool) Execute(ctx context.Context, raw string) (string, error) {
	controller, ok := subagentruntime.ControllerFromContext(ctx)
	if !ok {
		return "", &subagentruntime.Error{Code: subagentruntime.ErrorRuntimeUnavailable, Message: "current run has no subagent controller"}
	}
	var request input
	if err := json.Unmarshal([]byte(raw), &request); err != nil {
		return "", &subagentruntime.Error{Code: subagentruntime.ErrorInvalidInput, Message: err.Error()}
	}
	request.Action = strings.ToLower(strings.TrimSpace(request.Action))
	identity, _ := tool.ToolExecutionIdentityFromContext(ctx)
	switch request.Action {
	case "spawn":
		modelProfileID := strings.TrimSpace(identity.ModelProfileID)
		snapshot, err := controller.Spawn(ctx, subagentruntime.SpawnRequest{
			Description: request.Description, Prompt: request.Prompt,
			AgentType: request.AgentType, WorkspaceMode: request.WorkspaceMode,
			ModelProfileID: modelProfileID, ParentAgentID: "root",
			ParentToolCallID: identity.ToolCallID,
			ThreadID:         tool.ToolThreadIDFromContext(ctx), RunID: identity.RunID,
			TurnID: identity.TurnID,
		})
		return encode(map[string]any{"task": snapshot}, err)
	case "status":
		if strings.TrimSpace(request.TaskID) == "" {
			return "", invalid("task_id is required for status")
		}
		snapshot, err := controller.Status(request.TaskID)
		if err == nil && snapshot.Terminal() {
			controller.AcknowledgeTerminal(snapshot.TaskID)
		}
		return encode(map[string]any{"task": snapshot}, err)
	case "wait":
		result, err := controller.Wait(ctx, subagentruntime.WaitRequest{
			TaskIDs: request.TaskIDs, ReturnWhen: request.ReturnWhen,
			Timeout: time.Duration(request.TimeoutMS) * time.Millisecond,
		})
		return encode(map[string]any{"tasks": result.Tasks, "retrieval_status": result.RetrievalStatus}, err)
	case "cancel":
		if strings.TrimSpace(request.TaskID) == "" {
			return "", invalid("task_id is required for cancel")
		}
		snapshot, err := controller.Cancel(ctx, request.TaskID, request.Reason)
		if err == nil && snapshot.Terminal() {
			controller.AcknowledgeTerminal(snapshot.TaskID)
		}
		return encode(map[string]any{"task": snapshot}, err)
	default:
		return "", invalid("action must be spawn, status, wait, or cancel")
	}
}

func (t *SubagentTool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	content, err := t.Execute(ctx, input)
	if err != nil {
		code := subagentruntime.ErrorCode(err)
		if code == "" {
			code = "subagent_failed"
		}
		outcome := tool.FailedOutcome(err.Error(), code, tool.ErrorCategoryDomain, false)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	outcome := tool.SucceededOutcome(content)
	_ = json.Unmarshal([]byte(content), &outcome.Details)
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

func encode(value map[string]any, err error) (string, error) {
	if err != nil {
		return "", err
	}
	raw, marshalErr := json.Marshal(value)
	if marshalErr != nil {
		return "", fmt.Errorf("encode subagent result: %w", marshalErr)
	}
	return string(raw), nil
}

func invalid(message string) error {
	return &subagentruntime.Error{Code: subagentruntime.ErrorInvalidInput, Message: message}
}

func init() {
	tool.MustRegisterToolWithMode(&SubagentTool{}, tool.DefinitionValidationStrictBuiltin)
}
