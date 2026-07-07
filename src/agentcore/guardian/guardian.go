// Package guardian enforces the fixed read-only constraints applied to
// guardian/review sub-agents. This is Task 13 of the "零审批自动执行策略"
// spec: guardian agents are forced to read-only — they cannot write, cannot
// trigger open execution chains, cannot escalate.
//
// The enforcement is absolute: it runs BEFORE the normal auto-tool-policy
// decision flow so no policy override can let a guardian mutate state. This
// complements any executor-level enforcement (Task 11) as defense in depth —
// the executor enforces at execution time, this package enforces at decision
// time.
package guardian

import (
	"fmt"
	"strings"

	"awaken/src/agentcore/subagent"
	"awaken/src/tool/profile"
)

// GuardianViolationError is returned by EnforceGuardianConstraints when a
// guardian agent's tool call violates the fixed read-only constraints.
type GuardianViolationError struct {
	// Reason is a human-readable description of why the call was rejected.
	Reason string
	// ToolName is the name of the tool that was rejected.
	ToolName string
}

// Error implements the error interface.
func (e *GuardianViolationError) Error() string {
	tool := strings.TrimSpace(e.ToolName)
	if tool == "" {
		tool = "<unknown>"
	}
	reason := strings.TrimSpace(e.Reason)
	if reason == "" {
		reason = "guardian read-only constraint violated"
	}
	return fmt.Sprintf("guardian violation on tool %q: %s", tool, reason)
}

// IsGuardianProfile reports whether the given ExecutionProfile is the
// guardian read-only profile. The check is structural: it returns true only
// when permission is read_only, sandbox is required, and network is restricted.
func IsGuardianProfile(execProfile profile.ExecutionProfile) bool {
	return execProfile.PermissionProfile == profile.PermissionReadOnly &&
		execProfile.SandboxPolicy == profile.SandboxRequired &&
		execProfile.NetworkPolicy == profile.NetworkRestricted
}

// EnforceGuardianConstraints checks that a guardian agent's tool call is
// compliant with the fixed read-only constraints. It returns nil if the call
// is allowed, or a *GuardianViolationError describing the violation.
//
// Enforcement only applies when the profile is a guardian profile (see
// IsGuardianProfile). For any other profile, the function returns nil
// immediately — non-guardian profiles are governed by the normal auto tool
// policy.
//
// The checks are (each returns immediately on violation):
//   - The tool must be read-only. The ReadOnly annotation is inferred from
//     RequestedAction: read/inspect are read-only; all other actions are not.
//   - The tool must not be OpenWorld (inferred from NetworkScope == open).
//   - The tool must not be Destructive (inferred from delete/move actions).
//   - The RequestedAction must not be write/delete/move/execute.
//   - The NetworkScope must not be open.
//
// Some checks overlap (a destructive action is also a non-read-only action);
// the redundancy is intentional defense in depth. The first matching
// violation is returned.
func EnforceGuardianConstraints(intent profile.ExecutionIntent, execProfile profile.ExecutionProfile) error {
	if !IsGuardianProfile(execProfile) {
		return nil
	}

	toolName := strings.TrimSpace(intent.ToolName)

	// Check 1: the tool must be read-only. The ReadOnly annotation is
	// inferred from RequestedAction: read and inspect are read-only; all
	// other actions (write, delete, move, execute) are not.
	if intent.RequestedAction != profile.ActionRead && intent.RequestedAction != profile.ActionInspect {
		return &GuardianViolationError{
			ToolName: toolName,
			Reason: fmt.Sprintf(
				"guardian profile requires a read-only tool; tool %q has action %q which is not read-only",
				toolName, intent.RequestedAction,
			),
		}
	}

	// Check 2: the tool must not be OpenWorld. The OpenWorld annotation is
	// inferred from NetworkScope == open.
	if intent.NetworkScope == profile.NetworkScopeOpen {
		return &GuardianViolationError{
			ToolName: toolName,
			Reason: fmt.Sprintf(
				"guardian profile forbids open-world tools; tool %q has network scope %q",
				toolName, intent.NetworkScope,
			),
		}
	}

	// Check 3: the tool must not be Destructive. Destructive actions are
	// delete and move (they mutate the filesystem in a way that is not
	// recoverable without trash).
	if intent.RequestedAction == profile.ActionDelete || intent.RequestedAction == profile.ActionMove {
		return &GuardianViolationError{
			ToolName: toolName,
			Reason: fmt.Sprintf(
				"guardian profile forbids destructive tools; tool %q has destructive action %q",
				toolName, intent.RequestedAction,
			),
		}
	}

	// Check 4: the RequestedAction must not be write/delete/move/execute.
	// This is the spec's explicit action deny-list. It overlaps with Check 1
	// (non-read-only) but is kept separate for clarity.
	switch intent.RequestedAction {
	case profile.ActionWrite, profile.ActionDelete, profile.ActionMove, profile.ActionExecute:
		return &GuardianViolationError{
			ToolName: toolName,
			Reason: fmt.Sprintf(
				"guardian profile forbids action %q on tool %q",
				intent.RequestedAction, toolName,
			),
		}
	}

	// Check 5: the NetworkScope must not be open. This overlaps with Check 2
	// (OpenWorld) but is kept separate so the error message is specific to
	// the network dimension.
	if intent.NetworkScope == profile.NetworkScopeOpen {
		return &GuardianViolationError{
			ToolName: toolName,
			Reason: fmt.Sprintf(
				"guardian profile forbids open network scope on tool %q",
				toolName,
			),
		}
	}

	return nil
}

// ResolveGuardianBoundary takes a parent boundary and returns a NEW boundary
// with the guardian profile, ignoring the parent's higher-privilege profile.
// The workspace roots are inherited (a guardian can read the workspace), but
// the profile is forced to GuardianReadOnly() so the guardian cannot mutate
// state, reach open network, or escalate.
//
// The returned boundary is a fresh copy: mutating it does not affect the
// parent boundary. Unlike subagent.ResolveSubagentBoundary (which inherits
// the parent's profile and validates no escalation), this function ALWAYS
// forces the guardian profile — a guardian sub-agent's permissions are
// absolute, not inherited.
func ResolveGuardianBoundary(parent subagent.ParentBoundary) subagent.ParentBoundary {
	guardianProfile := profile.GuardianReadOnly()
	return subagent.ParentBoundary{
		Profile:       &guardianProfile,
		WorkspaceRoot: parent.WorkspaceRoot,
		WritableRoots: append([]string(nil), parent.WritableRoots...),
	}
}
