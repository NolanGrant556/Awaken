package updateplan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"awaken/src/tool"
)

const ToolName = "update_plan"

type Tool struct{}

type input struct {
	Explanation string          `json:"explanation,omitempty"`
	Plan        []inputPlanItem `json:"plan"`
	BaseVersion uint64          `json:"base_version,omitempty"`
}

type inputPlanItem struct {
	Step       string `json:"step"`
	ActiveForm string `json:"active_form"`
	Status     string `json:"status"`
}

type output struct {
	PlanID         string             `json:"plan_id"`
	Version        uint64             `json:"version"`
	CompletedCount int                `json:"completed_count"`
	TotalCount     int                `json:"total_count"`
	CurrentStep    int                `json:"current_step"`
	AllCompleted   bool               `json:"all_completed"`
	Plan           []tool.RunPlanItem `json:"plan"`
}

func (t *Tool) Name() string { return ToolName }

func (t *Tool) Description() string {
	return "Create or replace the durable run plan of the current complex task as one whole ordered step list. " +
		"Use it only when the task needs 3 or more distinguishable investigation/implementation/verification steps, spans a long time, combines several user requests, or the user asks for a plan; never for simple Q&A, single commands, or small local edits. " +
		"Before starting work on a step mark exactly that step in_progress; update in place as soon as it finishes instead of batch-checking at the end; mark completed only when the implementation, its automation, and required real verification actually pass; when blocked keep the step in_progress and surface the blocker instead of inventing status; explain added, removed, or rewritten steps through explanation; never repeat the whole plan in natural language unless the user asks."
}

func (t *Tool) Parameters() []tool.ToolParameter { return nil }

func (t *Tool) InputSchema() tool.ToolParametersSchema {
	return tool.ToolParametersSchema{
		Type:                 "object",
		Required:             []string{"plan"},
		AdditionalProperties: tool.BoolPtr(false),
		Properties: map[string]tool.ToolParam{
			"explanation": {
				Type:        "string",
				Description: "Optional one-sentence reason for this plan change. Required when a completed step is reverted or removed.",
			},
			"plan": {
				Type:        "array",
				Description: "The complete ordered step list (1-12 items). Order must be completed steps, then the single in_progress step, then pending steps.",
				Items: &tool.ToolParam{
					Type:                 "object",
					Required:             []string{"step", "active_form", "status"},
					AdditionalProperties: tool.BoolPtr(false),
					Properties: map[string]tool.ToolParam{
						"step":        {Type: "string", Description: "Static step goal, 1-160 characters."},
						"active_form": {Type: "string", Description: "Present-tense wording shown while the step runs, 1-160 characters."},
						"status":      {Type: "string", Enum: []any{"pending", "in_progress", "completed"}, Description: "Step status."},
					},
				},
			},
			"base_version": {
				Type:        "integer",
				Description: "Optimistic concurrency guard: omit or 0 for the first update of this run; afterwards the version returned by the previous update_plan call.",
				Minimum:     floatPtr(0),
			},
		},
	}
}

func (t *Tool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name: ToolName, DisplayName: "Update Plan", Description: t.Description(),
		Purpose: "Maintain the durable, user-visible run plan (steps and progress) of the current complex run.",
		WhenToUse: []string{
			"The task needs 3 or more distinguishable investigation, implementation, or verification steps",
			"The user gave several requirements at once, or the task runs long enough that progress must stay visible across edits, builds, and verification",
			"The user explicitly asks for a plan, steps, or a progress list",
		},
		WhenNotToUse: []string{
			"Pure information Q&A, single commands, or small local edits",
			"Tasks owned by a native Subagent child — child progress uses native Subagent facts",
			"Restating the plan in natural language; the plan UI already renders every accepted snapshot",
		},
		SideEffects: []tool.ToolSideEffect{{
			Kind:          tool.SideEffectNone,
			Description:   "只替换当前 run 的耐久计划快照（SemanticJournal journal-first），不修改 workspace 文件或外部系统。",
			Reversibility: tool.ReversibilityNotApplicable,
		}},
		Examples: []tool.ToolExample{
			{
				Scenario: "Start a multi-step change: first step in progress, the rest pending",
				Input: map[string]any{
					"plan": []map[string]any{
						{"step": "调查运行计划相关现状代码", "active_form": "正在调查运行计划相关现状代码", "status": "in_progress"},
						{"step": "实现 Core 快照与工具", "active_form": "正在实现 Core 快照与工具", "status": "pending"},
						{"step": "补齐自动化与真实验收", "active_form": "正在补齐自动化与真实验收", "status": "pending"},
					},
				},
				Outcome: "Establishes plan_id and version 1 with exactly one in_progress step.",
			},
			{
				Scenario: "Finish the first step and start the second, reusing the returned version",
				Input: map[string]any{
					"base_version": 1,
					"plan": []map[string]any{
						{"step": "调查运行计划相关现状代码", "active_form": "正在调查运行计划相关现状代码", "status": "completed"},
						{"step": "实现 Core 快照与工具", "active_form": "正在实现 Core 快照与工具", "status": "in_progress"},
						{"step": "补齐自动化与真实验收", "active_form": "正在补齐自动化与真实验收", "status": "pending"},
					},
				},
				Outcome: "Advances the same plan to version 2.",
			},
			{
				Scenario: "Scope change: remove a completed step with an explanation",
				Input: map[string]any{
					"base_version": 2,
					"explanation":  "用户确认不再需要兼容旧格式，删除对应步骤",
					"plan": []map[string]any{
						{"step": "调查运行计划相关现状代码", "active_form": "正在调查运行计划相关现状代码", "status": "completed"},
						{"step": "实现 Core 快照与工具", "active_form": "正在实现 Core 快照与工具", "status": "in_progress"},
					},
				},
				Outcome: "Accepted because the removal of a completed step is explained.",
			},
		},
		FailureModes: []tool.ToolFailureMode{
			{Code: "plan_invalid_input", Cause: "Schema, three-state, ordering, duplicate, or explanation invariants are violated; the previous snapshot is kept", Retryable: false},
			{Code: "plan_version_conflict", Cause: "base_version is stale; the response carries the latest version and a bounded snapshot to retry against", Retryable: true},
			{Code: "plan_persistence_failed", Cause: "The durable plan_updated journal append/flush failed; memory and UI did not move", Retryable: true},
			{Code: "plan_service_unavailable", Cause: "No run-scoped plan service is bound to this execution context (for example Subagent child runs)", Retryable: false},
		},
		Annotations: tool.ToolAnnotations{
			// ReadOnly follows the local-transform builtin convention (subagent,
			// ask_user, calculator): the plan mutation is host-internal run
			// state, never a workspace or external-system side effect.
			ReadOnly:        true,
			Destructive:     false,
			OpenWorld:       false,
			CapabilityClass: tool.CapabilityLocalTransform,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
			FileBridgeScope: tool.FileBridgeNone,
			NetworkScope:    tool.NetworkScopeNone,
		},
	})
}

func (t *Tool) Execute(ctx context.Context, raw string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, raw, nil)
	if err != nil {
		return "", err
	}
	if outcome.IsError() {
		return "", errors.New(firstNonEmpty(outcome.ErrorMessage, outcome.Content, "update_plan failed"))
	}
	return outcome.Content, nil
}

func (t *Tool) ExecuteOutcome(ctx context.Context, raw string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	var p input
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return tool.FailedOutcome(
			fmt.Sprintf("plan_invalid_input: %v", err),
			"plan_invalid_input", tool.ErrorCategoryValidation, false,
		).WithTiming(started, time.Now().UTC()), nil
	}
	service, ok := tool.RunPlanServiceFromContext(ctx)
	if !ok {
		return tool.FailedOutcome(
			"plan_service_unavailable: no run plan service is bound to this execution context",
			"plan_service_unavailable", tool.ErrorCategoryUnsupported, false,
		).WithTiming(started, time.Now().UTC()), nil
	}
	identity, _ := tool.ToolExecutionIdentityFromContext(ctx)
	update := tool.RunPlanUpdate{
		Explanation: p.Explanation,
		BaseVersion: p.BaseVersion,
		Items:       make([]tool.RunPlanItem, 0, len(p.Plan)),
	}
	for _, item := range p.Plan {
		update.Items = append(update.Items, tool.RunPlanItem{
			Step:       item.Step,
			ActiveForm: item.ActiveForm,
			Status:     tool.RunPlanStatus(item.Status),
		})
	}
	result, err := service.ApplyUpdate(ctx, identity, update)
	if err != nil {
		var planErr *tool.RunPlanError
		if errors.As(err, &planErr) {
			outcome := tool.FailedOutcome(planErr.Error(), planErr.Code, planErrorCategory(planErr), planErr.Retryable)
			if planErr.Code == tool.RunPlanErrorVersionConflict {
				diagnostic := map[string]any{"latest_version": planErr.LatestVersion}
				if planErr.LatestSnapshot != nil {
					diagnostic["latest_snapshot"] = *planErr.LatestSnapshot
				}
				outcome.Diagnostic = diagnostic
			}
			return outcome.WithTiming(started, time.Now().UTC()), nil
		}
		return tool.FailedOutcome(
			err.Error(), "plan_persistence_failed", tool.ErrorCategoryPersistence, true,
		).WithTiming(started, time.Now().UTC()), nil
	}
	encoded, err := json.Marshal(output{
		PlanID:         result.Snapshot.PlanID,
		Version:        result.Snapshot.Version,
		CompletedCount: result.CompletedCount,
		TotalCount:     result.TotalCount,
		CurrentStep:    result.CurrentStep,
		AllCompleted:   result.AllCompleted,
		Plan:           result.Snapshot.Items,
	})
	if err != nil {
		return tool.FailedOutcome(
			err.Error(), "plan_persistence_failed", tool.ErrorCategoryTool, true,
		).WithTiming(started, time.Now().UTC()), nil
	}
	outcome := tool.SucceededOutcome(string(encoded))
	outcome.Summary = fmt.Sprintf("Run plan updated to version %d (%d/%d completed).", result.Snapshot.Version, result.CompletedCount, result.TotalCount)
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

func planErrorCategory(err *tool.RunPlanError) tool.ErrorCategory {
	switch err.Code {
	case tool.RunPlanErrorInvalidInput:
		return tool.ErrorCategoryValidation
	case tool.RunPlanErrorVersionConflict:
		return tool.ErrorCategoryDomain
	case tool.RunPlanErrorPersistenceFailed:
		return tool.ErrorCategoryPersistence
	default:
		return tool.ErrorCategoryTool
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func floatPtr(value float64) *float64 { return &value }

func init() { tool.MustRegisterToolWithMode(&Tool{}, tool.DefinitionValidationStrictBuiltin) }
