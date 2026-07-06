// Package subagent implements boundary inheritance and no-escalation
// enforcement for ordinary sub-agents. When a sub-agent is spawned, it
// inherits the parent agent's workspace_root, writable_roots, and
// ExecutionProfile; it cannot exceed the parent's permissions.
//
// This is Task 12 of the "零审批自动执行策略" spec: ordinary subagents
// inherit the parent agent's workspace_root, writable_roots,
// permission_profile, sandbox_policy, network_policy — and CANNOT
// escalate privileges.
package subagent

import (
	"errors"
	"fmt"

	"awaken/src/tool/profile"
)

// ParentBoundary captures the parent agent's inherited boundary. It is the
// shape a sub-agent inherits: the same workspace_root, writable_roots, and
// ExecutionProfile the parent was running with.
//
// All fields are copied by value when inherited so a sub-agent cannot mutate
// the parent's boundary by aliasing.
type ParentBoundary struct {
	// WorkspaceRoot is the absolute path of the workspace the sub-agent
	// operates within. It MUST match the parent's workspace root.
	WorkspaceRoot string
	// WritableRoots is the set of paths the sub-agent may write to. It
	// MUST match the parent's writable roots.
	WritableRoots []string
	// Profile is the ExecutionProfile the sub-agent runs under. It MUST
	// not escalate the parent's profile.
	Profile *profile.ExecutionProfile
	// Depth records the nesting level of the parent agent that produced
	// this boundary. The root agent has depth 0; a sub-agent spawned
	// directly from the root inherits depth 0 (its own depth, tracked
	// separately on SubagentOrigin via BuildSubagentOrigin, is 1).
	// Guardian's ResolveGuardianBoundary copies this verbatim so the
	// guardian's boundary depth matches the parent's intended depth.
	Depth int
}

// InheritBoundary returns a copy of the parent boundary for the sub-agent.
// The sub-agent gets the SAME workspace_root, writable_roots, profile, and
// depth as the parent. The returned boundary is a deep copy: mutating its
// slices or the pointed-to profile does not affect the parent.
func InheritBoundary(parent ParentBoundary) ParentBoundary {
	child := ParentBoundary{
		WorkspaceRoot: parent.WorkspaceRoot,
		WritableRoots: append([]string(nil), parent.WritableRoots...),
		Depth:         parent.Depth,
	}
	if parent.Profile != nil {
		copied := *parent.Profile
		// SandboxPermissions is a []string; copy it so the sub-agent's
		// profile cannot be mutated to affect the parent.
		if len(parent.Profile.SandboxPermissions) > 0 {
			copied.SandboxPermissions = append([]string(nil), parent.Profile.SandboxPermissions...)
		}
		child.Profile = &copied
	}
	return child
}

// privilegeRank ranks PermissionProfile values from least to most
// privileged. A higher rank means more permissive. Child rank MUST be <=
// parent rank; any other relationship is an escalation.
//
// Unknown profiles return -1 so they are always rejected: ValidateNoEscalation
// treats a -1 child rank as an error (cannot validate an unknown profile).
func privilegeRank(p profile.PermissionProfile) int {
	switch p {
	case profile.PermissionReadOnly:
		return 0
	case profile.PermissionWorkspaceWrite:
		return 1
	case profile.PermissionOpenWorld:
		return 2
	case profile.PermissionDangerFullAccess:
		return 3
	}
	return -1
}

// sandboxRank ranks SandboxPolicy values from least to most permissive. A
// higher rank means the sandbox constraint is looser. Child rank MUST be <=
// parent rank; any other relationship is an escalation.
//
// Unknown policies return -1 so they are always rejected, mirroring
// privilegeRank's handling of unknown PermissionProfile values.
func sandboxRank(p profile.SandboxPolicy) int {
	switch p {
	case profile.SandboxRequired:
		return 0
	case profile.SandboxBestEffort:
		return 1
	case profile.SandboxNone:
		return 2
	}
	return -1
}

// networkRank ranks NetworkPolicy values from least to most permissive.
// Higher rank means more network reach. Child rank MUST be <= parent rank.
//
// Unknown policies return -1 so they are always rejected.
func networkRank(p profile.NetworkPolicy) int {
	switch p {
	case profile.NetworkNone:
		return 0
	case profile.NetworkRestricted:
		return 1
	case profile.NetworkOpen:
		return 2
	}
	return -1
}

// envRank ranks EnvPolicy values from least to most permissive. Higher rank
// means more environment variables are exposed to the tool. Child rank MUST
// be <= parent rank.
//
// Unknown policies return -1 so they are always rejected.
func envRank(p profile.EnvPolicy) int {
	switch p {
	case profile.EnvMinimal:
		return 0
	case profile.EnvInherit:
		return 1
	}
	return -1
}

// approvalRank ranks ApprovalPolicy values from least to most permissive.
// Higher rank means the runtime asks for approval more often, which is
// MORE permissive from the agent's perspective (more chances to bypass
// auto-deny). Child rank MUST be <= parent rank.
//
// Unknown policies return -1 so they are always rejected.
func approvalRank(p profile.ApprovalPolicy) int {
	switch p {
	case profile.ApprovalNever:
		return 0
	case profile.ApprovalOnDeny:
		return 1
	case profile.ApprovalAlways:
		return 2
	}
	return -1
}

// ValidateNoEscalation checks that the child profile does NOT exceed the
// parent's permissions. Escalation is detected by comparing all
// security-related fields (PermissionProfile, SandboxPolicy, NetworkPolicy,
// EnvPolicy, SandboxPermissions, ApprovalPolicy): for each field, the
// child's permissiveness MUST be less than or equal to the parent's.
//
// Escalation rules (from the spec):
//   - PermissionProfile: read_only → can stay read_only. Cannot escalate to
//     workspace_write, open_world, or danger_full_access. workspace_write →
//     can stay workspace_write or downgrade to read_only. Cannot escalate to
//     open_world or danger_full_access. open_world → can stay open_world or
//     downgrade. Cannot escalate to danger_full_access. danger_full_access →
//     can be anything (already max).
//   - SandboxPolicy: required → can stay required or downgrade. Cannot
//     relax to best_effort or none. best_effort → can stay best_effort or
//     downgrade to required. Cannot relax to none. none → can be anything.
//   - NetworkPolicy: none → can stay none. Cannot escalate to restricted or
//     open. restricted → can stay restricted or downgrade to none. Cannot
//     escalate to open. open → can be anything.
//   - EnvPolicy: minimal → can stay minimal. Cannot escalate to inherit.
//     inherit → can be anything.
//   - ApprovalPolicy: never → can stay never. Cannot escalate to on_deny or
//     always. on_deny → can stay on_deny or downgrade to never. Cannot
//     escalate to always. always → can be anything.
//   - SandboxPermissions: the child's set MUST be a subset of the parent's.
//     The child cannot hold a capability the parent does not have.
//
// Both profiles must be non-nil.
func ValidateNoEscalation(childProfile, parentProfile *profile.ExecutionProfile) error {
	if childProfile == nil {
		return errors.New("subagent escalation check failed: child profile is nil")
	}
	if parentProfile == nil {
		return errors.New("subagent escalation check failed: parent profile is nil")
	}
	childRank := privilegeRank(childProfile.PermissionProfile)
	parentRank := privilegeRank(parentProfile.PermissionProfile)
	if childRank < 0 {
		return fmt.Errorf(
			"subagent escalation check failed: unknown child permission profile %q",
			childProfile.PermissionProfile,
		)
	}
	if parentRank < 0 {
		return fmt.Errorf(
			"subagent escalation check failed: unknown parent permission profile %q",
			parentProfile.PermissionProfile,
		)
	}
	if childRank > parentRank {
		return fmt.Errorf(
			"subagent profile escalation denied: child permission %q (rank %d) exceeds parent permission %q (rank %d)",
			childProfile.PermissionProfile, childRank,
			parentProfile.PermissionProfile, parentRank,
		)
	}

	// SandboxPolicy: child must not be more permissive than parent.
	childSandbox := sandboxRank(childProfile.SandboxPolicy)
	parentSandbox := sandboxRank(parentProfile.SandboxPolicy)
	if childSandbox < 0 {
		return fmt.Errorf(
			"subagent escalation check failed: unknown child sandbox policy %q",
			childProfile.SandboxPolicy,
		)
	}
	if parentSandbox < 0 {
		return fmt.Errorf(
			"subagent escalation check failed: unknown parent sandbox policy %q",
			parentProfile.SandboxPolicy,
		)
	}
	if childSandbox > parentSandbox {
		return fmt.Errorf(
			"subagent profile escalation denied: child sandbox %q (rank %d) exceeds parent sandbox %q (rank %d)",
			childProfile.SandboxPolicy, childSandbox,
			parentProfile.SandboxPolicy, parentSandbox,
		)
	}

	// NetworkPolicy: child must not have more network reach than parent.
	childNet := networkRank(childProfile.NetworkPolicy)
	parentNet := networkRank(parentProfile.NetworkPolicy)
	if childNet < 0 {
		return fmt.Errorf(
			"subagent escalation check failed: unknown child network policy %q",
			childProfile.NetworkPolicy,
		)
	}
	if parentNet < 0 {
		return fmt.Errorf(
			"subagent escalation check failed: unknown parent network policy %q",
			parentProfile.NetworkPolicy,
		)
	}
	if childNet > parentNet {
		return fmt.Errorf(
			"subagent profile escalation denied: child network %q (rank %d) exceeds parent network %q (rank %d)",
			childProfile.NetworkPolicy, childNet,
			parentProfile.NetworkPolicy, parentNet,
		)
	}

	// EnvPolicy: child must not expose more environment than parent.
	childEnv := envRank(childProfile.EnvPolicy)
	parentEnv := envRank(parentProfile.EnvPolicy)
	if childEnv < 0 {
		return fmt.Errorf(
			"subagent escalation check failed: unknown child env policy %q",
			childProfile.EnvPolicy,
		)
	}
	if parentEnv < 0 {
		return fmt.Errorf(
			"subagent escalation check failed: unknown parent env policy %q",
			parentProfile.EnvPolicy,
		)
	}
	if childEnv > parentEnv {
		return fmt.Errorf(
			"subagent profile escalation denied: child env %q (rank %d) exceeds parent env %q (rank %d)",
			childProfile.EnvPolicy, childEnv,
			parentProfile.EnvPolicy, parentEnv,
		)
	}

	// ApprovalPolicy: child must not ask for approval more often than parent.
	childApproval := approvalRank(childProfile.ApprovalPolicy)
	parentApproval := approvalRank(parentProfile.ApprovalPolicy)
	if childApproval < 0 {
		return fmt.Errorf(
			"subagent escalation check failed: unknown child approval policy %q",
			childProfile.ApprovalPolicy,
		)
	}
	if parentApproval < 0 {
		return fmt.Errorf(
			"subagent escalation check failed: unknown parent approval policy %q",
			parentProfile.ApprovalPolicy,
		)
	}
	if childApproval > parentApproval {
		return fmt.Errorf(
			"subagent profile escalation denied: child approval %q (rank %d) exceeds parent approval %q (rank %d)",
			childProfile.ApprovalPolicy, childApproval,
			parentProfile.ApprovalPolicy, parentApproval,
		)
	}

	// SandboxPermissions: child's capability set must be a subset of parent's.
	parentPerms := make(map[string]struct{}, len(parentProfile.SandboxPermissions))
	for _, p := range parentProfile.SandboxPermissions {
		parentPerms[p] = struct{}{}
	}
	for _, perm := range childProfile.SandboxPermissions {
		if _, ok := parentPerms[perm]; !ok {
			return fmt.Errorf(
				"subagent profile escalation denied: child sandbox permission %q is not in parent permissions %v",
				perm, parentProfile.SandboxPermissions,
			)
		}
	}

	return nil
}

// ResolveSubagentBoundary inherits the parent boundary, validates no
// escalation, and returns the effective boundary for the sub-agent.
//
//   - If requestedProfile is nil, the sub-agent inherits the parent's
//     profile verbatim (whatever it is, including nil).
//   - If requestedProfile is non-nil, it must not escalate the parent's
//     profile. If the parent's profile is nil, the function returns an
//     error because escalation cannot be validated against an unknown
//     parent.
//
// The returned boundary is a deep copy: the sub-agent's profile, sandbox
// permissions, and writable roots are independent of the parent's.
func ResolveSubagentBoundary(parent ParentBoundary, requestedProfile *profile.ExecutionProfile) (ParentBoundary, error) {
	child := InheritBoundary(parent)
	if requestedProfile == nil {
		return child, nil
	}
	if parent.Profile == nil {
		return ParentBoundary{}, errors.New(
			"subagent boundary resolution failed: parent profile is nil; cannot validate subagent escalation",
		)
	}
	if err := ValidateNoEscalation(requestedProfile, parent.Profile); err != nil {
		return ParentBoundary{}, err
	}
	copied := *requestedProfile
	if len(requestedProfile.SandboxPermissions) > 0 {
		copied.SandboxPermissions = append([]string(nil), requestedProfile.SandboxPermissions...)
	}
	child.Profile = &copied
	return child, nil
}

// BuildSubagentOrigin builds the SubagentOrigin for a sub-agent, incrementing
// depth. If parentOrigin is nil (the spawning agent is the root agent, which
// has no origin of its own), the sub-agent's depth starts at 1.
//
// The agentID is the new sub-agent's ID. The parentAgentID is the ID of the
// agent that is spawning it. The isGuardian flag marks guardian/review
// sub-agents (which always use the GuardianReadOnly profile regardless of
// inheritance — see Task 13).
func BuildSubagentOrigin(parentOrigin *profile.SubagentOrigin, agentID, parentAgentID string, isGuardian bool) *profile.SubagentOrigin {
	depth := 1
	if parentOrigin != nil {
		depth = parentOrigin.Depth + 1
	}
	return &profile.SubagentOrigin{
		AgentID:       agentID,
		ParentAgentID: parentAgentID,
		IsGuardian:    isGuardian,
		Depth:         depth,
	}
}
