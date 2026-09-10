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
	ModelProfileID  string   `json:"model_profile_id,omitempty"`
	ReasoningEffort string   `json:"reasoning_effort,omitempty"`
	ContextMode     string   `json:"context_mode,omitempty"`
	RecentTurns     int      `json:"recent_turns,omitempty"`
	Action          string   `json:"action"`
	TaskID          string   `json:"task_id,omitempty"`
	TaskIDs         []string `json:"task_ids,omitempty"`
	Description     string   `json:"description,omitempty"`
	Prompt          string   `json:"prompt,omitempty"`
	AgentType       string   `json:"agent_type,omitempty"`
	WorkspaceMode   string   `json:"workspace_mode,omitempty"`
	ReturnWhen      string   `json:"return_when,omitempty"`
	TimeoutMS       int      `json:"timeout_ms,omitempty"`
	Reason          string   `json:"reason,omitempty"`
}

type SubagentTool struct{}

var _ tool.OutcomeTool = (*SubagentTool)(nil)

func (t *SubagentTool) Name() string { return ToolName }

func (t *SubagentTool) Description() string {
	return "创建和管理当前会话内有权访问的本地 Subagent。支持 spawn、status、wait、message、followup、interrupt、close、models。" +
		"只读角色 explore/plan/review 使用 shared_readonly；general 默认可写父工作区，也可使用 git_worktree。" +
		"所有子任务归属本次主任务，父任务必须等待并汇总；停止主任务同时停止后代。终态会自动通知主 Agent；message 只在 child 下一模型轮次边界生效，followup 创建新任务并复用同一 Agent 的上下文；interrupt 保留 Agent，close 关闭后续入口。"
}

func (t *SubagentTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "action", Description: "spawn | status | wait | cancel | message | followup | interrupt | close | models", Type: "string", Required: true},
		{Name: "task_id", Description: "status/cancel/message 的子任务 ID", Type: "string"},
		{Name: "task_ids", Description: "wait 的子任务 ID 列表", Type: "array"},
		{Name: "description", Description: "spawn 的简短任务说明", Type: "string"},
		{Name: "prompt", Description: "spawn 或 message 的完整说明", Type: "string"},
		{Name: "agent_type", Description: "host-owned role: explore | plan | review | general, an enabled Plugin Agent identity prefixed plugin__, or a unique enabled Plugin Agent manifest name", Type: "string"},
		{Name: "workspace_mode", Description: "shared_readonly | shared | git_worktree", Type: "string"},
		{Name: "model_profile_id", Description: "可选精确模型配置 ID；通过 models 查询，省略时继承有效配置", Type: "string"},
		{Name: "reasoning_effort", Description: "目标配置支持的思考强度", Type: "string"},
		{Name: "context_mode", Description: "fresh | full | recent；默认 fresh", Type: "string"},
		{Name: "recent_turns", Description: "recent 模式继承的已闭合用户任务轮数（1-100）", Type: "integer"},
		{Name: "return_when", Description: "wait 使用 any | all", Type: "string"},
		{Name: "timeout_ms", Description: "0 查询快照；1-60000 表示宿主持续等待结果，期间不调用模型", Type: "integer"},
	}
}

func (t *SubagentTool) InputSchema() tool.ToolParametersSchema {
	noExtra := false
	return tool.ToolParametersSchema{
		Type: "object", AdditionalProperties: &noExtra,
		Required: []string{"action"},
		Properties: map[string]tool.ToolParam{
			"action":           {Type: "string", Description: "要执行的生命周期动作", Enum: []any{"spawn", "status", "wait", "cancel", "message", "followup", "interrupt", "close", "models"}},
			"model_profile_id": {Type: "string", Description: "通过 models 查询的精确配置 ID；无效时失败，不换供应商"},
			"reasoning_effort": {Type: "string", Description: "目标配置支持的思考强度；省略时随有效配置"},
			"context_mode":     {Type: "string", Enum: []any{"fresh", "full", "recent"}, Description: "fresh 新上下文；full 当前有效父上下文；recent 最近已闭合用户任务轮次"},
			"recent_turns":     {Type: "integer", Description: "recent 模式的轮数，1-100"},
			"task_id":          {Type: "string", Description: "status、cancel 或 message 的目标 task ID"},
			"task_ids":         {Type: "array", Description: "wait 的目标 task ID 列表", Items: &tool.ToolParam{Type: "string"}},
			"description":      {Type: "string", Description: "用户可见的简短任务说明"},
			"prompt":           {Type: "string", Description: "交给 child 的完整任务说明，或追加给运行中 child 的指令"},
			"agent_type":       {Type: "string", Description: "host-owned child 角色；可为内置角色、已启用的 plugin__ identity，或唯一且无冲突的 Plugin Agent manifest 名称"},
			"workspace_mode":   {Type: "string", Description: "child workspace 隔离模式", Enum: []any{"shared_readonly", "shared", "git_worktree"}},
			"return_when":      {Type: "string", Description: "等待任一或全部任务完成", Enum: []any{"any", "all"}},
			"timeout_ms":       {Type: "integer", Description: "0 查询快照；正数开启宿主事件等待（最大 60000），直到结果或取消"},
			"reason":           {Type: "string", Description: "取消原因"},
		},
	}
}

func (t *SubagentTool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name: ToolName, DisplayName: "Subagent", Description: t.Description(),
		Purpose: "把独立探索、规划、复核或实现任务交给隔离上下文中的本地 child Agent，并查询、追加指令、继续或停止自己有权管理的 Agent。",
		WhenToUse: []string{
			"存在可以独立完成的代码搜索、规划、复核或实现任务时",
			"多个子任务可以与主任务并行时",
			"需要独立上下文减少主 Agent 上下文污染时",
		},
		WhenNotToUse: []string{
			"任务很小、直接使用现有工具更快时",
			"需要改文件时不要使用 explore/plan/review；应使用 general",
			"未授权的跨任务访问或后台执行",
		},
		InputSchema: t.InputSchema(),
		OutputSchema: tool.ToolParam{
			Type: "object", AdditionalProperties: tool.BoolPtr(false),
			Properties: map[string]tool.ToolParam{
				"policy":           {Type: "object", AdditionalProperties: tool.BoolPtr(true)},
				"models":           {Type: "array", Items: &tool.ToolParam{Type: "object", AdditionalProperties: tool.BoolPtr(true)}},
				"task":             {Type: "object", AdditionalProperties: tool.BoolPtr(true)},
				"tasks":            {Type: "array", Items: &tool.ToolParam{Type: "object", AdditionalProperties: tool.BoolPtr(true)}},
				"receipt":          {Type: "object", AdditionalProperties: tool.BoolPtr(true)},
				"retrieval_status": {Type: "string", Enum: []any{subagentruntime.RetrievalCompleted, subagentruntime.RetrievalRunning, subagentruntime.RetrievalTimeout}},
			},
		},
		Examples: []tool.ToolExample{
			{Scenario: "创建只读探索任务", Input: map[string]any{"action": "spawn", "description": "定位认证入口", "prompt": "只读检查认证入口和调用链", "agent_type": "explore", "workspace_mode": "shared_readonly"}, Outcome: "返回稳定 task_id，子任务独立运行；默认需父任务等待，父任务必须等待并汇总，子任务不能脱离主任务继续运行。"},
			{Scenario: "创建可写实现任务", Input: map[string]any{"action": "spawn", "description": "补登录校验", "prompt": "在父工作区实现登录校验", "agent_type": "general", "workspace_mode": "shared"}, Outcome: "返回稳定 task_id，child 可在不超过父权限的前提下改文件。"},
			{Scenario: "向运行中的 child 追加指令", Input: map[string]any{"action": "message", "task_id": "subtask_...", "prompt": "同时检查错误处理"}, Outcome: "指令先耐久入队，child 在下一模型轮次边界处理。"},
			{Scenario: "等待一个子任务", Input: map[string]any{"action": "wait", "task_ids": []any{"subtask_..."}, "return_when": "all", "timeout_ms": 60000}, Outcome: "任务终态后返回结构化结果。"},
		},
		FailureModes: []tool.ToolFailureMode{
			{Code: subagentruntime.ErrorInvalidInput, Cause: "action 字段组合、长度或枚举非法", Retryable: false},
			{Code: subagentruntime.ErrorRuntimeUnavailable, Cause: "当前 run 没有 Subagent manager", Retryable: false},
			{Code: "subagent_concurrency_queue_full", Cause: "待执行分配队列已满，应先等待现有任务", Retryable: true},
			{Code: subagentruntime.ErrorLimitReached, Cause: "Agent 深度、消息队列或兼容模式预算已到上限", Retryable: false},
			{Code: subagentruntime.ErrorNotFound, Cause: "task_id 不属于当前 Agent 可管理的范围", Retryable: false},
			{Code: subagentruntime.ErrorNotRunning, Cause: "对终态或取消中的 child 追加消息", Retryable: false},
			{Code: subagentruntime.ErrorWorkspaceMode, Cause: "agent_type 与 workspace_mode 组合不受支持", Retryable: false},
			{Code: subagentruntime.ErrorWorktreeFailed, Cause: "host 创建或检查 git worktree 失败", Retryable: false},
			{Code: subagentruntime.ErrorBoundaryViolation, Cause: "越权 spawn、角色或工作区边界不匹配", Retryable: false},
		},
		SideEffects: []tool.ToolSideEffect{{
			Kind: tool.SideEffectNone, Description: "只管理有权访问的 child 生命周期；文件副作用发生在 child 自己的工具结果中。",
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
	deferAcknowledgement := false
	if policy, ok := controller.(interface{ EventDrivenWaits() bool }); ok {
		deferAcknowledgement = policy.EventDrivenWaits()
	}
	switch request.Action {
	case "models":
		catalog, ok := controller.(interface {
			Models(context.Context) ([]subagentruntime.ModelOption, error)
		})
		if !ok {
			return "", invalid("configured model catalog is unavailable")
		}
		models, err := catalog.Models(ctx)
		result := map[string]any{"models": models}
		if source, ok := controller.(interface{ EffectivePolicy() subagentruntime.Policy }); ok {
			result["policy"] = source.EffectivePolicy()
		}
		return encode(result, err)
	case "spawn":
		snapshot, err := controller.SpawnFromHost(ctx, subagentruntime.SpawnRequest{
			ContextMode: request.ContextMode, RecentTurns: request.RecentTurns, ReasoningEffort: request.ReasoningEffort,
			Description: request.Description, Prompt: request.Prompt,
			AgentType: request.AgentType, WorkspaceMode: request.WorkspaceMode,
			ModelProfileID: request.ModelProfileID, ParentAgentID: identity.AgentID,
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
		if err == nil && snapshot.Terminal() && !deferAcknowledgement {
			controller.AcknowledgeTerminal(snapshot.TaskID)
		}
		return encode(map[string]any{"task": modelTaskResult(snapshot)}, err)
	case "wait":
		eventDriven := false
		if policy, ok := controller.(interface{ EventDrivenWaits() bool }); ok {
			eventDriven = policy.EventDrivenWaits()
		}
		result, err := controller.Wait(ctx, subagentruntime.WaitRequest{
			DeferAcknowledgement: deferAcknowledgement,
			TaskIDs:              request.TaskIDs, ReturnWhen: request.ReturnWhen,
			Timeout:     time.Duration(request.TimeoutMS) * time.Millisecond,
			EventDriven: eventDriven && request.TimeoutMS > 0,
		})
		results := make([]any, 0, len(result.Tasks))
		for _, snapshot := range result.Tasks {
			results = append(results, modelTaskResult(snapshot))
		}
		return encode(map[string]any{"tasks": results, "retrieval_status": result.RetrievalStatus}, err)
	case "cancel", "interrupt":
		if strings.TrimSpace(request.TaskID) == "" {
			return "", invalid("task_id is required for cancel")
		}
		snapshot, err := controller.Cancel(ctx, request.TaskID, request.Reason)
		if err == nil && snapshot.Terminal() && !deferAcknowledgement {
			controller.AcknowledgeTerminal(snapshot.TaskID)
		}
		return encode(map[string]any{"task": snapshot}, err)
	case "followup", "close":
		sessions, ok := controller.(subagentruntime.SessionController)
		if !ok {
			return "", invalid("agent sessions are unavailable")
		}
		if request.TaskID == "" {
			return "", invalid("task_id is required")
		}
		if request.Action == "close" {
			snapshot, err := sessions.CloseAgent(ctx, request.TaskID)
			return encode(map[string]any{"task": snapshot}, err)
		}
		snapshot, err := sessions.Followup(ctx, request.TaskID, request.Prompt)
		return encode(map[string]any{"task": snapshot}, err)
	case "message":
		if strings.TrimSpace(request.TaskID) == "" {
			return "", invalid("task_id is required for message")
		}
		if strings.TrimSpace(request.Prompt) == "" {
			return "", invalid("prompt is required for message")
		}
		receipt, err := controller.Message(ctx, request.TaskID, request.Prompt)
		return encode(map[string]any{"receipt": receipt}, err)
	default:
		return "", invalid("unsupported subagent action")
	}
}

// Full result bodies belong to explicit tool retrieval. Public task snapshots
// and automatic notifications remain bounded and identify this retrieval by ID.
func modelTaskResult(snapshot subagentruntime.TaskSnapshot) any {
	return struct {
		subagentruntime.TaskSnapshot
		ResultContent string `json:"result_content,omitempty"`
	}{snapshot, snapshot.ResultContent}
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
