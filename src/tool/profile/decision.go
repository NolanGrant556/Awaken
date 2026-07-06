// Package profile defines ExecutionDecision, the unified decision output of
// the auto tool policy. This is the decision layer of Phase 3 (unified
// execution profile) of the "零审批自动执行策略" spec: every auto policy
// evaluation produces an ExecutionDecision, which the ToolRuntime consumes
// to allow, deny, or transform the execution intent.
package profile

import "fmt"

// ExecutionDecisionAction is the set of actions the unified decision layer
// can produce. Unlike the legacy ToolDecisionAction, there is no "abort":
// context cancellation is handled separately by the runtime, not as a
// decision action. A cancelled context surfaces as a deny with
// DecisionSourcePolicyError.
type ExecutionDecisionAction string

const (
	DecisionAllow     ExecutionDecisionAction = "allow"
	DecisionDeny      ExecutionDecisionAction = "deny"
	DecisionTransform ExecutionDecisionAction = "transform"
)

// ExecutionDecisionSource records which layer of the policy produced a
// decision. The string values match the legacy ToolDecisionSource so that
// the two types can be converted without loss.
type ExecutionDecisionSource string

const (
	DecisionSourceToolRule          ExecutionDecisionSource = "tool_rule"
	DecisionSourceAutoAllowlist     ExecutionDecisionSource = "auto_allowlist"
	DecisionSourceWorkspaceBoundary ExecutionDecisionSource = "workspace_boundary"
	DecisionSourceHeadlessDefault   ExecutionDecisionSource = "headless_default"
	DecisionSourcePolicyError       ExecutionDecisionSource = "policy_error"
	DecisionSourceTransform         ExecutionDecisionSource = "transform"
	DecisionSourceSensitivePath     ExecutionDecisionSource = "sensitive_path"
)

// ExecutionDecision is the unified decision output of the auto tool policy.
// It supersedes ToolDecision: every ToolRuntime execution produces one
// ExecutionDecision, which the runtime consumes to allow, deny, or transform
// the execution intent.
type ExecutionDecision struct {
	// Action is the decision: allow, deny, or transform.
	Action ExecutionDecisionAction
	// Source records which policy layer produced the decision.
	Source ExecutionDecisionSource
	// Reason is a human-readable explanation suitable for logging and audit.
	Reason string
	// Retryable indicates whether a denied call may succeed if retried
	// (e.g. after a transient policy error).
	Retryable bool
	// UpdatedIntent is the rewritten ExecutionIntent when Action == transform.
	// For allow/deny it carries the original intent (for logging).
	UpdatedIntent *ExecutionIntent
	// UpdatedToolName is the tool to execute after transform (e.g.
	// "apply_patch" when a shell echo-write is rewritten).
	UpdatedToolName string
	// UpdatedInput is the input map for the transformed tool.
	UpdatedInput map[string]any
}

// String returns a compact one-line representation suitable for logging.
func (d ExecutionDecision) String() string {
	if d.UpdatedToolName != "" {
		return fmt.Sprintf("decision(action=%s source=%s tool=%s reason=%s)",
			d.Action, d.Source, d.UpdatedToolName, d.Reason)
	}
	return fmt.Sprintf("decision(action=%s source=%s reason=%s)",
		d.Action, d.Source, d.Reason)
}

// AllowDecision constructs an allow decision from raw string source and
// reason. The source should be one of the DecisionSource* constants.
func AllowDecision(source, reason string) ExecutionDecision {
	return ExecutionDecision{
		Action: DecisionAllow,
		Source: ExecutionDecisionSource(source),
		Reason: reason,
	}
}

// DenyDecision constructs a deny decision from raw string source and reason.
// The source should be one of the DecisionSource* constants.
func DenyDecision(source, reason string, retryable bool) ExecutionDecision {
	return ExecutionDecision{
		Action:    DecisionDeny,
		Source:    ExecutionDecisionSource(source),
		Reason:    reason,
		Retryable: retryable,
	}
}

// TransformDecision constructs a transform decision: the runtime should
// execute UpdatedToolName with UpdatedInput instead of the original call.
// The source is always DecisionSourceTransform.
func TransformDecision(reason string, updatedToolName string, updatedInput map[string]any) ExecutionDecision {
	return ExecutionDecision{
		Action:          DecisionTransform,
		Source:          DecisionSourceTransform,
		Reason:          reason,
		UpdatedToolName: updatedToolName,
		UpdatedInput:    updatedInput,
	}
}
