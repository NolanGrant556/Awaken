package agentcore

import (
	stdcontext "context"
	"encoding/json"
	"fmt"
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
	ToolDecisionAllow                  ToolDecisionAction = "allow"
	ToolDecisionDeny                   ToolDecisionAction = "deny"
	ToolDecisionAbort                  ToolDecisionAction = "abort"
	ToolDecisionTransform              ToolDecisionAction = "transform"
	ToolDecisionNeedsAuth              ToolDecisionAction = "needs_auth"
	ToolDecisionNeedsRuntime           ToolDecisionAction = "needs_runtime"
	ToolDecisionUnsupported            ToolDecisionAction = "unsupported"
	ToolDecisionNeedsMassMessageReview ToolDecisionAction = "needs_mass_message_review"
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
	ToolDecisionSourceMassMessageReview ToolDecisionSource = "mass_message_review"
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
	IsDestructive           bool
	IsOpenWorld             bool
	RequiresUserInteraction bool
	CapabilityClass         tool.CapabilityClass
	TrustedOrigin           tool.TrustedOrigin
	FileBridgeScope         tool.FileBridgeScope
	// RiskTone carries the tool-declared risk tone ("", "safe", "warning",
	// or "danger"). The auto policy uses it to distinguish normal
	// in-workspace mutations from dangerous ones (delete,
	// out-of-writable-roots, sensitive path).
	RiskTone string
	// AffectedPaths carries the workspace-relative (or absolute if outside)
	// paths the tool will touch. Used by the auto policy to re-check
	// workspace boundary and sensitive-path membership for file-chain tools.
	AffectedPaths []string
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

	if request.ToolName == "apply_patch" {
		if decision, handled := decideApplyPatchFileChain(request); handled {
			return decision
		}
	}

	if request.ToolName == "shell" {
		if decision, handled := p.decideShellTransform(ctx, request); handled {
			return decision
		}
	}

	if decision, handled := decideByCapabilityClass(request); handled {
		return decision
	}

	if request.StaticBehavior == tool.PermissionAllow {
		if deniesStaticAllowInHeadless(request) {
			return ToolDecision{
				Action: ToolDecisionDeny,
				Source: ToolDecisionSourceHeadlessDefault,
				Reason: firstNonEmpty(strings.TrimSpace(request.StaticReason), "AUTO policy denies write-capable tool by default"),
			}
		}
		return ToolDecision{
			Action:       ToolDecisionAllow,
			Source:       ToolDecisionSourceToolRule,
			Reason:       firstNonEmpty(strings.TrimSpace(request.StaticReason), "tool allowed execution"),
			UpdatedInput: request.NormalizedInput,
		}
	}

	if request.IsReadOnly && !request.IsOpenWorld && !request.RequiresUserInteraction {
		return ToolDecision{
			Action:       ToolDecisionAllow,
			Source:       ToolDecisionSourceWorkspaceBoundary,
			Reason:       "read-only tool is constrained to the workspace",
			UpdatedInput: request.NormalizedInput,
		}
	}

	// File-chain relaxation (Task 4 of "零审批自动执行策略"):
	// apply_patch is no longer hard-denied in headless mode. With workspace
	// boundary checks (validateMutationPath), sensitive-path guards
	// (SensitivePathMatcher), and trash-backed deletes (TrashAdapter) all
	// enforced inside apply_patch itself, in-workspace mutations can
	// auto-execute without approval. Out-of-writable-roots and sensitive
	// paths are still denied here so the policy fails fast before execution.
	reason := firstNonEmpty(strings.TrimSpace(request.StaticReason), "AUTO policy denied tool execution")
	return ToolDecision{
		Action: ToolDecisionDeny,
		Source: ToolDecisionSourceHeadlessDefault,
		Reason: reason,
	}
}

// DecideExecution is the unified decision method. It mirrors the logic of
// Decide but produces an ExecutionDecision (the unified output that carries
// the ExecutionIntent and a map-typed UpdatedInput). The existing internal
// helpers (decideApplyPatchFileChain, decideShellTransform) are reused:
// their ToolDecision outputs are mapped to ExecutionDecision via
// toolDecisionToExecutionDecision.
//
// Key mapping difference from Decide: a cancelled context (which would
// produce ToolDecisionAbort in the legacy path) surfaces as a deny with
// DecisionSourcePolicyError here, because the unified model has no "abort"
// action — context cancellation is handled separately by the runtime.
//
// The runtime no-review contract is final here: DecideExecution may return
// allow, deny, transform, needs_auth, needs_runtime, unsupported, or
// needs_mass_message_review. It never returns a generic human review action;
// mass-message review is the only dedicated gate and is represented by its
// own action.
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

	if request.ToolName == "apply_patch" {
		if td, handled := decideApplyPatchFileChain(request); handled {
			return toolDecisionToExecutionDecision(td, intent)
		}
	}

	if request.ToolName == "shell" {
		if td, handled := p.decideShellTransform(ctx, request); handled {
			return toolDecisionToExecutionDecision(td, intent)
		}
	}

	if td, handled := decideByCapabilityClass(request); handled {
		return toolDecisionToExecutionDecision(td, intent)
	}

	if request.StaticBehavior == tool.PermissionAllow {
		if deniesStaticAllowInHeadless(request) {
			return profile.DenyDecision(
				string(profile.DecisionSourceHeadlessDefault),
				firstNonEmpty(strings.TrimSpace(request.StaticReason), "AUTO policy denies write-capable tool by default"),
				false,
			)
		}
		decision := profile.AllowDecision(
			string(profile.DecisionSourceToolRule),
			firstNonEmpty(strings.TrimSpace(request.StaticReason), "tool allowed execution"),
		)
		decision.UpdatedIntent = &intent
		return decision
	}

	if request.IsReadOnly && !request.IsOpenWorld && !request.RequiresUserInteraction {
		decision := profile.AllowDecision(
			string(profile.DecisionSourceWorkspaceBoundary),
			"read-only tool is constrained to the workspace",
		)
		decision.UpdatedIntent = &intent
		return decision
	}

	reason := firstNonEmpty(strings.TrimSpace(request.StaticReason), "AUTO policy denied tool execution")
	return profile.DenyDecision(string(profile.DecisionSourceHeadlessDefault), reason, false)
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
	case ToolDecisionNeedsMassMessageReview:
		ed.Action = profile.DecisionNeedsMassMessageReview
		ed.Source = profile.DecisionSourceMassMessageReview
	default:
		ed.Action = profile.DecisionDeny
		ed.Source = profile.DecisionSourcePolicyError
		ed.Reason = firstNonEmpty(strings.TrimSpace(td.Reason), "invalid tool decision action")
	}
	return ed
}

// deniesStaticAllowInHeadless reports whether a tool that returned
// PermissionAllow should still be denied in headless AUTO mode. Read-only
// tools are always allowed. Destructive tools (IsDestructive) are denied
// because they lack the file-chain safety constraints.
//
// apply_patch is handled by decideApplyPatchFileChain, which uses
// workspace boundaries, sensitive-path guards, and trash-backed delete
// semantics before allowing in-workspace mutation.
func deniesStaticAllowInHeadless(request ToolDecisionRequest) bool {
	if strings.HasPrefix(strings.TrimSpace(request.ToolName), "mcp__") {
		return false
	}
	switch request.CapabilityClass {
	case tool.CapabilityConnectorRead,
		tool.CapabilityConnectorWrite,
		tool.CapabilityBrowserRead,
		tool.CapabilityBrowserInteract:
		return false
	}
	if request.IsReadOnly {
		return false
	}
	if request.IsDestructive {
		return true
	}
	return false
}

// decideApplyPatchFileChain applies the file-chain relaxation policy to an
// apply_patch decision request. It returns (decision, true) if the request
// was handled.
//
// Safety invariants that make this relaxation safe (Task 4 of the
// "零审批自动执行策略" spec):
//   - Workspace boundary: apply_patch.validateMutationPath enforces that
//     every mutation path is within workspace_root/writable_roots (with
//     symlink resolution) before any filesystem write.
//   - Sensitive paths: validateMutationPath also rejects .git/, .env,
//     key/certificate files, and agent metadata directories regardless of
//     workspace membership.
//   - Trash: deletes go through TrashAdapter, making them recoverable.
//
// Decision matrix:
//   - PermissionAllow (parse failure fall-through): allow - executor will
//     surface the parse error.
//   - warning RiskTone: allow - normal in-workspace Add/Update.
//   - danger RiskTone:
//   - All paths within writable roots and non-sensitive: allow (delete/move
//     within workspace; trash makes it recoverable).
//   - Any path outside writable roots: deny.
//   - Any sensitive path: deny.
func decideApplyPatchFileChain(request ToolDecisionRequest) (ToolDecision, bool) {
	if strings.TrimSpace(request.ToolName) != "apply_patch" {
		return ToolDecision{}, false
	}

	riskTone := strings.ToLower(strings.TrimSpace(request.RiskTone))
	switch riskTone {
	case "", "safe", "warning":
		// Normal in-workspace mutation (Add/Update). Boundary + sensitive
		// path checks already passed in CheckPermission. Auto-allow.
		return ToolDecision{
			Action:       ToolDecisionAllow,
			Source:       ToolDecisionSourceWorkspaceBoundary,
			Reason:       "apply_patch within workspace boundary (boundary + sensitive-path + trash constraints satisfied)",
			UpdatedInput: request.NormalizedInput,
		}, true
	case "danger":
		// Danger tone means at least one operation is a delete, move, or
		// touches an out-of-writable-roots / sensitive path. Allow
		// deletes/moves within workspace (trash makes them recoverable)
		// but deny out-of-writable-roots and sensitive paths.
		permCtx := tool.NewPermissionContext(request.WorkspaceRoot, request.WritableRoots)
		for _, path := range request.AffectedPaths {
			if err := permCtx.CheckSensitivePath(path); err != nil {
				return ToolDecision{
					Action: ToolDecisionDeny,
					Source: ToolDecisionSourceHeadlessDefault,
					Reason: fmt.Sprintf("apply_patch touches sensitive path: %v", err),
				}, true
			}
			if !permCtx.IsWithinWritableRoots(path) {
				return ToolDecision{
					Action: ToolDecisionDeny,
					Source: ToolDecisionSourceHeadlessDefault,
					Reason: fmt.Sprintf("apply_patch path %q is outside writable roots", path),
				}, true
			}
		}
		// All paths are within writable roots and non-sensitive. The danger
		// tone is from a delete or move within workspace — trash makes
		// these recoverable, so auto-allow.
		return ToolDecision{
			Action:       ToolDecisionAllow,
			Source:       ToolDecisionSourceWorkspaceBoundary,
			Reason:       "apply_patch delete/move within workspace (trash-recoverable)",
			UpdatedInput: request.NormalizedInput,
		}, true
	default:
		return ToolDecision{
			Action: ToolDecisionDeny,
			Source: ToolDecisionSourceHeadlessDefault,
			Reason: "apply_patch has unknown risk tone, denied in headless mode",
		}, true
	}
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
	case tool.CapabilityMassMessageSend:
		return ToolDecision{
			Action: ToolDecisionNeedsMassMessageReview,
			Source: ToolDecisionSourceMassMessageReview,
			Reason: "mass message send requires human review",
		}, true
	case tool.CapabilityConnectorWrite, tool.CapabilityBrowserInteract:
		if request.TrustedOrigin == tool.TrustedOriginOfficialConnector ||
			request.TrustedOrigin == tool.TrustedOriginOfficialHosted ||
			request.TrustedOrigin == tool.TrustedOriginBuiltin {
			return ToolDecision{
				Action:       ToolDecisionAllow,
				Source:       ToolDecisionSourceWorkspaceBoundary,
				Reason:       "trusted connector/browser capability is allowed by lane policy",
				UpdatedInput: request.NormalizedInput,
			}, true
		}
		return ToolDecision{
			Action: ToolDecisionDeny,
			Source: ToolDecisionSourceHeadlessDefault,
			Reason: "connector/browser write capability requires a trusted origin",
		}, true
	case tool.CapabilityExternalPublish, tool.CapabilitySystemOperation:
		if request.TrustedOrigin == tool.TrustedOriginBuiltin && request.ToolName == "shell" {
			return ToolDecision{}, false
		}
		return ToolDecision{
			Action: ToolDecisionDeny,
			Source: ToolDecisionSourceHeadlessDefault,
			Reason: "system/external capabilities require a higher-trust execution profile",
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
	case tool.PermissionNeedsMassMessageReview:
		return ToolDecision{
			Action: ToolDecisionNeedsMassMessageReview,
			Source: ToolDecisionSourceMassMessageReview,
			Reason: firstNonEmpty(strings.TrimSpace(request.StaticReason), "mass message send requires human review"),
		}, true
	default:
		return ToolDecision{}, false
	}
}

func normalizeToolDecision(decision ToolDecision, fallbackReason string) ToolDecision {
	switch decision.Action {
	case ToolDecisionAllow, ToolDecisionDeny, ToolDecisionAbort, ToolDecisionTransform,
		ToolDecisionNeedsAuth, ToolDecisionNeedsRuntime, ToolDecisionUnsupported, ToolDecisionNeedsMassMessageReview:
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
	selectedTool tool.Tool,
	rawInput string,
	normalizedInput string,
	permission tool.PermissionDecision,
	permissionContext tool.PermissionContext,
) ToolDecisionRequest {
	def := selectedTool.Definition()
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
		IsReadOnly:              isReadOnlyTool(selectedTool),
		IsDestructive:           isDestructivePermission(permission),
		IsOpenWorld:             def.Annotations.OpenWorld,
		RequiresUserInteraction: requiresUserInteraction(permission),
		CapabilityClass:         def.Annotations.CapabilityClass,
		TrustedOrigin:           def.Annotations.TrustedOrigin,
		FileBridgeScope:         def.Annotations.FileBridgeScope,
		RiskTone:                permission.Metadata.RiskTone,
		AffectedPaths:           append([]string(nil), permission.Metadata.AffectedPaths...),
	}
}

func isReadOnlyTool(selectedTool tool.Tool) bool {
	readOnlyTool, ok := selectedTool.(tool.ReadOnlyTool)
	return ok && readOnlyTool.IsReadOnly()
}

func isDestructivePermission(permission tool.PermissionDecision) bool {
	request := permission.Metadata
	if strings.EqualFold(strings.TrimSpace(request.RiskTone), "danger") {
		return true
	}
	text := strings.ToLower(permission.Reason + " " + request.Description + " " + request.PatchSummary)
	for _, marker := range []string{"delete", "move", "modify", "mutate", "write", "outside writable", "danger"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func requiresUserInteraction(permission tool.PermissionDecision) bool {
	text := strings.ToLower(permission.Reason + " " + permission.Metadata.Description)
	return strings.Contains(text, "requires user interaction") || strings.Contains(text, "user interaction")
}
