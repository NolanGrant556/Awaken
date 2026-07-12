package capabilitydiagnose

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"awaken/src/tool"
)

type Tool struct{}

var _ tool.OutcomeTool = (*Tool)(nil)

type input struct {
	Capability tool.Capability `json:"capability"`
}

func (*Tool) Name() string { return "capability_diagnose" }

func (*Tool) Description() string {
	return "诊断当前 run snapshot 中某项 capability 的 provider 候选、readiness 与依赖状态；不刷新目录、不安装依赖、不执行候选工具"
}

func (*Tool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{{
		Name: "capability", Description: "provider-neutral capability，例如 presentation.render 或 message.send", Type: "string", Required: true,
	}}
}

func (t *Tool) Definition() tool.ToolDefinition {
	candidateSchema := tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
		"capability":       {Type: "string"},
		"provider_id":      {Type: "string"},
		"tool_name":        {Type: "string"},
		"registration_id":  {Type: "string"},
		"source":           {Type: "string"},
		"readiness":        {Type: "string"},
		"readiness_reason": {Type: "string"},
		"preference":       {Type: "integer"},
	}, AdditionalProperties: tool.BoolPtr(true)}
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:         t.Name(),
		DisplayName:  "Capability Diagnose",
		ProviderID:   tool.ProviderID("builtin:capability_diagnose"),
		Capabilities: []tool.Capability{tool.CapabilityToolDiagnose},
		Description:  t.Description(),
		Purpose:      "解释 run 已固定工具目录中某个 capability 当前可由哪些 provider 承担，以及为何需要认证、运行时或修复依赖。",
		WhenToUse: []string{
			"执行能力前需要比较 provider readiness 或依赖时",
			"候选工具报告 needs_auth、needs_runtime 或 unavailable 时",
		},
		WhenNotToUse: []string{
			"不要用它安装依赖、刷新 provider 或执行实际任务",
			"不要用它绕过当前 run snapshot 选择进程全局 Registry 中的新工具",
		},
		OutputSchema: tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
			"capability":              {Type: "string"},
			"snapshot_id":             {Type: "string"},
			"environment_snapshot_id": {Type: "string"},
			"ready_candidates":        {Type: "integer"},
			"candidates":              {Type: "array", Items: &candidateSchema},
		}, AdditionalProperties: tool.BoolPtr(false)},
		SideEffects: []tool.ToolSideEffect{{
			Kind: tool.SideEffectNone, Description: "只读取 context 中固定的 run snapshot。", Reversibility: tool.ReversibilityNotApplicable,
		}},
		Examples: []tool.ToolExample{{
			Scenario: "诊断演示文稿渲染 provider", Input: map[string]any{"capability": "presentation.render"}, Outcome: "返回固定 snapshot 中的候选、readiness 和依赖摘要",
		}},
		FailureModes: []tool.ToolFailureMode{
			{Code: "capability_required", Cause: "capability 为空", Retryable: false, Recovery: []string{"提供 capability vocabulary 值"}},
			{Code: "run_availability_snapshot_missing", Cause: "调用 context 未固定 run availability snapshot", Retryable: true, Recovery: []string{"由 run builder 注入 snapshot 后重试"}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly: true, Destructive: false, OpenWorld: false,
			CapabilityClass: tool.CapabilityLocalTransform, TrustedOrigin: tool.TrustedOriginBuiltin,
			FileBridgeScope: tool.FileBridgeNone, NetworkScope: tool.NetworkScopeNone,
		},
	})
}

func (*Tool) IsReadOnly() bool { return true }

func (t *Tool) Execute(ctx context.Context, raw string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, raw, nil)
	if err != nil {
		return outcome.Content, err
	}
	if outcome.IsError() {
		return outcome.Content, tool.ToolError{
			Code: outcome.ErrorCode, Category: outcome.ErrorCategory, Message: outcome.ErrorMessage,
			Retryable: outcome.Retryable, RecoveryOptions: outcome.RecoveryOptions, Diagnostic: outcome.Diagnostic,
		}
	}
	return outcome.Content, nil
}

func (*Tool) ExecuteOutcome(ctx context.Context, raw string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	var params input
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		outcome := tool.FailedOutcome(fmt.Sprintf("invalid capability_diagnose input: %v", err), tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false)
		outcome.TransportStatus = tool.TransportNotStarted
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	params.Capability = tool.Capability(strings.TrimSpace(string(params.Capability)))
	if params.Capability == "" {
		outcome := tool.FailedOutcome("capability_required: capability is required", "capability_required", tool.ErrorCategoryValidation, false)
		outcome.TransportStatus = tool.TransportNotStarted
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	snapshot, _, ok := tool.RunAvailabilityFromContext(ctx)
	if !ok {
		outcome := tool.FailedOutcome("run_availability_snapshot_missing: run availability snapshot is required", "run_availability_snapshot_missing", tool.ErrorCategoryRuntime, false)
		outcome.Status = tool.OutcomeUnsupported
		outcome.TransportStatus = tool.TransportNotStarted
		outcome.ProcessStatus = tool.ProcessNotApplicable
		outcome.RecoveryOptions = []string{"inject_run_availability_snapshot"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	diagnostic := snapshot.ResolveCapability(params.Capability)
	result, err := json.Marshal(diagnostic)
	if err != nil {
		return tool.ExecutionOutcome{}, fmt.Errorf("encode capability diagnostic: %w", err)
	}
	outcome := tool.SucceededOutcome(string(result)).WithTiming(started, time.Now().UTC())
	outcome.Summary = fmt.Sprintf("Found %d provider candidate(s) for %s; %d ready.", len(diagnostic.Candidates), params.Capability, diagnostic.ReadyCandidates)
	outcome.Details = map[string]any{
		"capability": diagnostic.Capability, "snapshot_id": diagnostic.SnapshotID,
		"environment_snapshot_id": diagnostic.EnvironmentID, "ready_candidates": diagnostic.ReadyCandidates,
		"candidates": diagnostic.Candidates,
	}
	outcome.Diagnostic = map[string]any{
		"capability": diagnostic.Capability, "tool_snapshot_id": diagnostic.SnapshotID,
		"environment_snapshot_id": diagnostic.EnvironmentID,
	}
	return outcome, nil
}

func init() {
	tool.MustRegisterToolWithMode(&Tool{}, tool.DefinitionValidationStrictBuiltin)
}
