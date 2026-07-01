package agentcore

import (
	stdcontext "context"
	"strings"

	"awaken/src/tool"
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
	ToolDecisionAllow ToolDecisionAction = "allow"
	ToolDecisionDeny  ToolDecisionAction = "deny"
	ToolDecisionAbort ToolDecisionAction = "abort"
)

type ToolDecisionSource string

const (
	ToolDecisionSourceToolRule          ToolDecisionSource = "tool_rule"
	ToolDecisionSourceAutoAllowlist     ToolDecisionSource = "auto_allowlist"
	ToolDecisionSourceWorkspaceBoundary ToolDecisionSource = "workspace_boundary"
	ToolDecisionSourceHeadlessDefault   ToolDecisionSource = "headless_default"
	ToolDecisionSourcePolicyError       ToolDecisionSource = "policy_error"
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
	WritableRoots           []string
	IsReadOnly              bool
	IsDestructive           bool
	IsOpenWorld             bool
	RequiresUserInteraction bool
}

type ToolDecision struct {
	Action       ToolDecisionAction
	Source       ToolDecisionSource
	Reason       string
	Retryable    bool
	UpdatedInput string
}

type AutoToolPolicy interface {
	Decide(ctx stdcontext.Context, request ToolDecisionRequest) ToolDecision
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

	if p.Allowlist[strings.TrimSpace(request.ToolName)] {
		return ToolDecision{
			Action:       ToolDecisionAllow,
			Source:       ToolDecisionSourceAutoAllowlist,
			Reason:       "tool is explicitly allowlisted for AUTO execution",
			UpdatedInput: request.NormalizedInput,
		}
	}

	if request.StaticBehavior == tool.PermissionAllow {
		if deniesStaticAllowInHeadless(request) {
			return ToolDecision{
				Action: ToolDecisionDeny,
				Source: ToolDecisionSourceHeadlessDefault,
				Reason: firstNonEmpty(strings.TrimSpace(request.StaticReason), "AUTO headless policy denies write-capable tool by default"),
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

	reason := firstNonEmpty(strings.TrimSpace(request.StaticReason), "AUTO headless policy denies ask permission by default")
	return ToolDecision{
		Action: ToolDecisionDeny,
		Source: ToolDecisionSourceHeadlessDefault,
		Reason: reason,
	}
}

func deniesStaticAllowInHeadless(request ToolDecisionRequest) bool {
	if request.IsReadOnly {
		return false
	}
	if request.IsDestructive {
		return true
	}
	switch strings.TrimSpace(request.ToolName) {
	case "apply_patch":
		return true
	default:
		return false
	}
}

func normalizeToolDecision(decision ToolDecision, fallbackReason string) ToolDecision {
	switch decision.Action {
	case ToolDecisionAllow, ToolDecisionDeny, ToolDecisionAbort:
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
	return ToolDecisionRequest{
		ToolCallID:              callID,
		ToolName:                toolName,
		RawInput:                rawInput,
		NormalizedInput:         normalizedInput,
		StaticBehavior:          permission.Behavior,
		StaticReason:            permission.Reason,
		WorkspaceRoot:           permissionContext.WorkspaceRoot,
		WritableRoots:           append([]string(nil), permissionContext.WritableRoots...),
		IsReadOnly:              isReadOnlyTool(selectedTool),
		IsDestructive:           isDestructivePermission(permission),
		IsOpenWorld:             isOpenWorldTool(toolName),
		RequiresUserInteraction: requiresUserInteraction(permission),
	}
}

func isReadOnlyTool(selectedTool tool.Tool) bool {
	readOnlyTool, ok := selectedTool.(tool.ReadOnlyTool)
	return ok && readOnlyTool.IsReadOnly()
}

func isOpenWorldTool(toolName string) bool {
	switch strings.TrimSpace(toolName) {
	case "shell", "web_search":
		return true
	default:
		return false
	}
}

func isDestructivePermission(permission tool.PermissionDecision) bool {
	request := permission.ApprovalRequest
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
	text := strings.ToLower(permission.Reason + " " + permission.ApprovalRequest.Description)
	return strings.Contains(text, "requires user interaction") || strings.Contains(text, "user interaction")
}
