package agentcore

import (
	stdcontext "context"
	"encoding/json"
	"strings"

	"awaken/src/tool"
	"awaken/src/tool/profile"
)

type ToolExecutionStatus string

const (
	ToolExecutionSucceeded ToolExecutionStatus = "succeeded"
	ToolExecutionDenied    ToolExecutionStatus = "denied"
	ToolExecutionCancelled ToolExecutionStatus = "cancelled"
	ToolExecutionFailed    ToolExecutionStatus = "failed"
)

type ToolDecisionAction string

const (
	ToolDecisionAllow        ToolDecisionAction = "allow"
	ToolDecisionDeny         ToolDecisionAction = "deny"
	ToolDecisionAbort        ToolDecisionAction = "abort"
	ToolDecisionTransform    ToolDecisionAction = "transform"
	ToolDecisionNeedsAuth    ToolDecisionAction = "needs_auth"
	ToolDecisionNeedsRuntime ToolDecisionAction = "needs_runtime"
	ToolDecisionUnsupported  ToolDecisionAction = "unsupported"
)

type ToolDecisionSource string

const (
	ToolDecisionSourceToolRule          ToolDecisionSource = "tool_rule"
	ToolDecisionSourceAutoAllowlist     ToolDecisionSource = "auto_allowlist"
	ToolDecisionSourceWorkspaceBoundary ToolDecisionSource = "workspace_boundary"
	ToolDecisionSourceHeadlessDefault   ToolDecisionSource = "headless_default"
	ToolDecisionSourcePolicyError       ToolDecisionSource = "policy_error"
	ToolDecisionSourceTransform         ToolDecisionSource = "transform"
	ToolDecisionSourceAuthRequired      ToolDecisionSource = "auth_required"
	ToolDecisionSourceRuntimeRequired   ToolDecisionSource = "runtime_required"
	ToolDecisionSourceHook              ToolDecisionSource = "hook"
)

type ToolDecisionRequest struct {
	RunID                   string
	ThreadID                string
	TurnID                  string
	ToolCallID              string
	ToolName                string
	RawInput                string
	NormalizedInput         string
	StaticBehavior          tool.PermissionBehavior
	StaticReason            string
	WorkspaceRoot           string
	ReadableRoots           []string
	WritableRoots           []string
	OutputRoots             []string
	IsReadOnly              bool
	IsOpenWorld             bool
	RequiresUserInteraction bool
	CapabilityClass         tool.CapabilityClass
	TrustedOrigin           tool.TrustedOrigin
	FileBridgeScope         tool.FileBridgeScope
}

type ToolDecision struct {
	Action          ToolDecisionAction
	Source          ToolDecisionSource
	Reason          string
	Retryable       bool
	UpdatedInput    string
	UpdatedToolName string
}

type AutoToolPolicy interface {
	Decide(ctx stdcontext.Context, request ToolDecisionRequest) ToolDecision
	// DecideExecution is the unified decision method that produces an
	// ExecutionDecision from an ExecutionIntent plus the legacy
	// ToolDecisionRequest. New code should call this instead of Decide;
	// Decide is retained for backward compatibility.
	DecideExecution(ctx stdcontext.Context, intent profile.ExecutionIntent, request ToolDecisionRequest) profile.ExecutionDecision
}

type DefaultAutoToolPolicy struct {
	Allowlist map[string]bool
}

func (p DefaultAutoToolPolicy) Decide(ctx stdcontext.Context, request ToolDecisionRequest) ToolDecision {
	if err := ctx.Err(); err != nil {
		return ToolDecision{
			Action: ToolDecisionAbort,
			Source: ToolDecisionSourcePolicyError,
			Reason: err.Error(),
		}
	}

	if request.StaticBehavior == tool.PermissionDeny {
		return ToolDecision{
			Action: ToolDecisionDeny,
			Source: ToolDecisionSourceToolRule,
			Reason: firstNonEmpty(strings.TrimSpace(request.StaticReason), "tool denied execution"),
		}
	}
	if decision, handled := decideStructuredPermissionBehavior(request); handled {
		return decision
	}

	if p.Allowlist[strings.TrimSpace(request.ToolName)] {
		return ToolDecision{
			Action:       ToolDecisionAllow,
			Source:       ToolDecisionSourceAutoAllowlist,
			Reason:       "tool is explicitly allowlisted for AUTO execution",
			UpdatedInput: request.NormalizedInput,
		}
	}

	if request.ToolName == "Bash" && request.StaticBehavior == tool.PermissionAllow {
		return ToolDecision{
			Action:       ToolDecisionAllow,
			Source:       ToolDecisionSourceToolRule,
			Reason:       firstNonEmpty(strings.TrimSpace(request.StaticReason), "local Bash command allowed"),
			UpdatedInput: request.NormalizedInput,
		}
	}

	if request.StaticBehavior == tool.PermissionAllow {
		return ToolDecision{
			Action:       ToolDecisionAllow,
			Source:       ToolDecisionSourceToolRule,
			Reason:       firstNonEmpty(strings.TrimSpace(request.StaticReason), "tool allowed execution"),
			UpdatedInput: request.NormalizedInput,
		}
	}
	if decision, handled := decideByCapabilityClass(request); handled {
		return decision
	}

	if request.IsReadOnly && !request.IsOpenWorld && !request.RequiresUserInteraction {
		return ToolDecision{
			Action:       ToolDecisionAllow,
			Source:       ToolDecisionSourceWorkspaceBoundary,
			Reason:       "read-only tool is constrained to the workspace",
			UpdatedInput: request.NormalizedInput,
		}
	}

	return ToolDecision{
		Action:       ToolDecisionAllow,
		Source:       ToolDecisionSourceHeadlessDefault,
		Reason:       firstNonEmpty(strings.TrimSpace(request.StaticReason), "ordinary tool auto-allowed by default policy"),
		UpdatedInput: request.NormalizedInput,
	}
}

// DecideExecution is the unified decision method. It mirrors the logic of
// Decide but produces an ExecutionDecision (the unified output that carries
// the ExecutionIntent and a map-typed UpdatedInput). Existing internal helpers
// return legacy ToolDecision outputs where useful, and those are mapped to
// ExecutionDecision via toolDecisionToExecutionDecision.
//
// Key mapping difference from Decide: a cancelled context (which would
// produce ToolDecisionAbort in the legacy path) surfaces as a deny with
// DecisionSourcePolicyError here, because the unified model has no "abort"
// action — context cancellation is handled separately by the runtime.
//
// The runtime no-review contract is final here: DecideExecution may return
// allow, deny, transform, needs_auth, needs_runtime, or unsupported. It never
// returns a generic or message-specific human review action.
func (p DefaultAutoToolPolicy) DecideExecution(ctx stdcontext.Context, intent profile.ExecutionIntent, request ToolDecisionRequest) profile.ExecutionDecision {
	if err := ctx.Err(); err != nil {
		return profile.DenyDecision(string(profile.DecisionSourcePolicyError), err.Error(), false)
	}

	if request.StaticBehavior == tool.PermissionDeny {
		return profile.DenyDecision(
			string(profile.DecisionSourceToolRule),
			firstNonEmpty(strings.TrimSpace(request.StaticReason), "tool denied execution"),
			false,
		)
	}
	if td, handled := decideStructuredPermissionBehavior(request); handled {
		return toolDecisionToExecutionDecision(td, intent)
	}

	if p.Allowlist[strings.TrimSpace(request.ToolName)] {
		decision := profile.AllowDecision(
			string(profile.DecisionSourceAutoAllowlist),
			"tool is explicitly allowlisted for AUTO execution",
		)
		decision.UpdatedIntent = &intent
		return decision
	}

	if request.ToolName == "Bash" && request.StaticBehavior == tool.PermissionAllow {
		decision := profile.AllowDecision(
			string(profile.DecisionSourceToolRule),
			firstNonEmpty(strings.TrimSpace(request.StaticReason), "local Bash command allowed"),
		)
		decision.UpdatedIntent = &intent
		return decision
	}

	if request.StaticBehavior == tool.PermissionAllow {
		decision := profile.AllowDecision(
			string(profile.DecisionSourceToolRule),
			firstNonEmpty(strings.TrimSpace(request.StaticReason), "tool allowed execution"),
		)
		decision.UpdatedIntent = &intent
		return decision
	}
	if td, handled := decideByCapabilityClass(request); handled {
		return toolDecisionToExecutionDecision(td, intent)
	}

	if request.IsReadOnly && !request.IsOpenWorld && !request.RequiresUserInteraction {
		decision := profile.AllowDecision(
			string(profile.DecisionSourceWorkspaceBoundary),
			"read-only tool is constrained to the workspace",
		)
		decision.UpdatedIntent = &intent
		return decision
	}

	decision := profile.AllowDecision(
		string(profile.DecisionSourceHeadlessDefault),
		firstNonEmpty(strings.TrimSpace(request.StaticReason), "ordinary tool auto-allowed by default policy"),
	)
	decision.UpdatedIntent = &intent
	return decision
}

// toolDecisionToExecutionDecision maps a legacy ToolDecision to the unified
// ExecutionDecision, attaching the given intent as UpdatedIntent. For
// transform decisions, the JSON-string UpdatedInput is parsed into a
// map[string]any so the ExecutionDecision carries structured input.
func toolDecisionToExecutionDecision(td ToolDecision, intent profile.ExecutionIntent) profile.ExecutionDecision {
	ed := profile.ExecutionDecision{
		Source:        profile.ExecutionDecisionSource(td.Source),
		Reason:        td.Reason,
		Retryable:     td.Retryable,
		UpdatedIntent: &intent,
	}
	switch td.Action {
	case ToolDecisionAllow:
		ed.Action = profile.DecisionAllow
	case ToolDecisionDeny:
		ed.Action = profile.DecisionDeny
	case ToolDecisionTransform:
		ed.Action = profile.DecisionTransform
		ed.UpdatedToolName = td.UpdatedToolName
		if strings.TrimSpace(td.UpdatedInput) != "" {
			var inputMap map[string]any
			if err := json.Unmarshal([]byte(td.UpdatedInput), &inputMap); err == nil {
				ed.UpdatedInput = inputMap
			}
		}
	case ToolDecisionAbort:
		// abort has no equivalent in the unified model; map to deny with
		// policy_error so the runtime surfaces it as a denial.
		ed.Action = profile.DecisionDeny
		ed.Source = profile.DecisionSourcePolicyError
	case ToolDecisionNeedsAuth:
		ed.Action = profile.DecisionNeedsAuth
		ed.Source = profile.DecisionSourceAuthRequired
	case ToolDecisionNeedsRuntime:
		ed.Action = profile.DecisionNeedsRuntime
		ed.Source = profile.DecisionSourceRuntimeRequired
	case ToolDecisionUnsupported:
		ed.Action = profile.DecisionUnsupported
	default:
		ed.Action = profile.DecisionDeny
		ed.Source = profile.DecisionSourcePolicyError
		ed.Reason = firstNonEmpty(strings.TrimSpace(td.Reason), "invalid tool decision action")
	}
	return ed
}

func decideByCapabilityClass(request ToolDecisionRequest) (ToolDecision, bool) {
	switch request.CapabilityClass {
	case tool.CapabilityArtifactRead, tool.CapabilityArtifactWrite, tool.CapabilityConnectorRead, tool.CapabilityBrowserRead:
		return ToolDecision{
			Action:       ToolDecisionAllow,
			Source:       ToolDecisionSourceWorkspaceBoundary,
			Reason:       "capability class is auto-allowed for zero-approval execution",
			UpdatedInput: request.NormalizedInput,
		}, true
	case tool.CapabilityLocalTransform:
		return ToolDecision{
			Action:       ToolDecisionAllow,
			Source:       ToolDecisionSourceWorkspaceBoundary,
			Reason:       "local transform runs in the restricted local-compute lane",
			UpdatedInput: request.NormalizedInput,
		}, true
	case tool.CapabilityConnectorWrite, tool.CapabilityBrowserInteract,
		tool.CapabilityExternalPublish, tool.CapabilitySystemOperation:
		return ToolDecision{
			Action:       ToolDecisionAllow,
			Source:       ToolDecisionSourceHeadlessDefault,
			Reason:       "ordinary capability is auto-allowed; executor hard boundaries remain authoritative",
			UpdatedInput: request.NormalizedInput,
		}, true
	}
	return ToolDecision{}, false
}

func decideStructuredPermissionBehavior(request ToolDecisionRequest) (ToolDecision, bool) {
	switch request.StaticBehavior {
	case tool.PermissionNeedsAuth:
		return ToolDecision{
			Action:    ToolDecisionNeedsAuth,
			Source:    ToolDecisionSourceAuthRequired,
			Reason:    firstNonEmpty(strings.TrimSpace(request.StaticReason), "tool requires authentication"),
			Retryable: true,
		}, true
	case tool.PermissionNeedsRuntime:
		return ToolDecision{
			Action:    ToolDecisionNeedsRuntime,
			Source:    ToolDecisionSourceRuntimeRequired,
			Reason:    firstNonEmpty(strings.TrimSpace(request.StaticReason), "tool requires local runtime"),
			Retryable: true,
		}, true
	case tool.PermissionUnsupported:
		return ToolDecision{
			Action: ToolDecisionUnsupported,
			Source: ToolDecisionSourcePolicyError,
			Reason: firstNonEmpty(strings.TrimSpace(request.StaticReason), "tool capability is unsupported"),
		}, true
	default:
		return ToolDecision{}, false
	}
}

func normalizeToolDecision(decision ToolDecision, fallbackReason string) ToolDecision {
	switch decision.Action {
	case ToolDecisionAllow, ToolDecisionDeny, ToolDecisionAbort, ToolDecisionTransform,
		ToolDecisionNeedsAuth, ToolDecisionNeedsRuntime, ToolDecisionUnsupported:
	default:
		decision.Action = ToolDecisionDeny
		decision.Source = ToolDecisionSourcePolicyError
		decision.Reason = firstNonEmpty(strings.TrimSpace(decision.Reason), "auto tool policy returned an invalid decision")
	}
	if decision.Source == "" {
		decision.Source = ToolDecisionSourcePolicyError
	}
	decision.Reason = firstNonEmpty(strings.TrimSpace(decision.Reason), fallbackReason)
	return decision
}

func inferToolDecisionRequest(
	callID string,
	toolName string,
	definition tool.ToolDefinition,
	rawInput string,
	normalizedInput string,
	permission tool.PermissionDecision,
	permissionContext tool.PermissionContext,
) ToolDecisionRequest {
	return ToolDecisionRequest{
		ToolCallID:              callID,
		ToolName:                toolName,
		RawInput:                rawInput,
		NormalizedInput:         normalizedInput,
		StaticBehavior:          permission.Behavior,
		StaticReason:            permission.Reason,
		WorkspaceRoot:           permissionContext.WorkspaceRoot,
		ReadableRoots:           append([]string(nil), permissionContext.ReadableRoots...),
		WritableRoots:           append([]string(nil), permissionContext.WritableRoots...),
		OutputRoots:             append([]string(nil), permissionContext.OutputRoots...),
		IsReadOnly:              definition.Annotations.ReadOnly,
		IsOpenWorld:             definition.Annotations.OpenWorld,
		RequiresUserInteraction: permission.Behavior == tool.PermissionNeedsAuth,
		CapabilityClass:         definition.Annotations.CapabilityClass,
		TrustedOrigin:           definition.Annotations.TrustedOrigin,
		FileBridgeScope:         definition.Annotations.FileBridgeScope,
	}
}

func isReadOnlyTool(selectedTool tool.Tool) bool {
	readOnlyTool, ok := selectedTool.(tool.ReadOnlyTool)
	return ok && readOnlyTool.IsReadOnly()
}
