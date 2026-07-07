// Package profile defines ExecutionProfile, the unified policy bundle that
// every tool execution lands on. This is the profile layer of Phase 3 (unified
// execution profile) of the "零审批自动执行策略" spec: an ExecutionProfile
// bundles the permission profile, sandbox policy, network
// policy, environment policy, and visibility policy that the runtime should
// apply when executing a tool call.
//
// The 5 built-in profiles cover the spec's standard trust scenarios:
//   - GuardianReadOnly:     guardian/review agents (read-only, sandboxed, restricted net)
//   - WorkspaceReadOnly:    read-only workspace inspection (no net, no writes)
//   - WorkspaceWrite:       DEFAULT for normal agent runs (workspace writes, no net)
//   - OpenWorldRestricted:  agents that need restricted network reach
//   - DangerFullAccess:     explicit high-trust scenarios (full access, no sandbox)
package profile

import "fmt"

// PermissionProfile bounds the kind of filesystem access a tool call may
// perform. It is the profile-level analogue of the per-tool ReadOnly /
// Destructive / OpenWorld annotations: the runtime combines the profile's
// bound with the tool's annotations to make a final decision.
type PermissionProfile string

const (
	// PermissionReadOnly: tools may only read from the workspace.
	PermissionReadOnly PermissionProfile = "read_only"
	// PermissionWorkspaceWrite: tools may read and write inside the
	// workspace root and configured writable roots.
	PermissionWorkspaceWrite PermissionProfile = "workspace_write"
	// PermissionOpenWorld: tools may read/write within the workspace and
	// reach out to network services (subject to NetworkPolicy).
	PermissionOpenWorld PermissionProfile = "open_world"
	// PermissionDangerFullAccess: tools may read, write, and execute
	// arbitrary commands with no boundary enforcement.
	PermissionDangerFullAccess PermissionProfile = "danger_full_access"
)

// SandboxPolicy controls whether tool calls must execute inside a sandbox.
type SandboxPolicy string

const (
	// SandboxNone: no sandbox is enforced. Tools execute directly on the
	// host. Reserved for explicit high-trust profiles.
	SandboxNone SandboxPolicy = "none"
	// SandboxRequired: the runtime refuses to execute a tool call if no
	// sandbox is available. Used by guardian and open-world profiles where
	// isolation is a hard requirement.
	SandboxRequired SandboxPolicy = "required"
	// SandboxBestEffort: the runtime uses a sandbox when available but
	// falls back to direct execution when one is not configured. Used by
	// the workspace profiles so the runtime degrades gracefully on hosts
	// without sandbox support.
	SandboxBestEffort SandboxPolicy = "best_effort"
)

// NetworkPolicy bounds the network reach of a tool call.
type NetworkPolicy string

const (
	// NetworkNone: no network access is permitted.
	NetworkNone NetworkPolicy = "none"
	// NetworkRestricted: network access is permitted but bounded to a
	// known set of services (e.g. a named MCP server, an allowlisted host).
	NetworkRestricted NetworkPolicy = "restricted"
	// NetworkOpen: network access is unrestricted.
	NetworkOpen NetworkPolicy = "open"
)

// EnvPolicy controls the environment variables a tool call can observe.
type EnvPolicy string

const (
	// EnvMinimal: only a minimal allowlist of environment variables is
	// exposed to the tool (PATH, HOME, etc.). Sensitive variables such as
	// API keys are stripped.
	EnvMinimal EnvPolicy = "minimal"
	// EnvInherit: the tool inherits the runtime's full environment. Used
	// by profiles where the tool needs access to project-local env (e.g.
	// language-specific PATHs, workspace tokens).
	EnvInherit EnvPolicy = "inherit"
)

// VisibilityPolicy controls how much of a tool call's I/O is surfaced to the
// user (via the event stream / UI).
type VisibilityPolicy string

const (
	// VisibilityHidden: tool calls are not surfaced to the user. Reserved
	// for internal/system tools whose output would only confuse the user.
	VisibilityHidden VisibilityPolicy = "hidden"
	// VisibilitySummary: only a summary of the tool call (name, status,
	// short result) is surfaced.
	VisibilitySummary VisibilityPolicy = "summary"
	// VisibilityFull: the full tool call (input, output, progress events)
	// is surfaced. This is the default for normal agent runs.
	VisibilityFull VisibilityPolicy = "full"
)

// ExecutionProfile is the unified policy bundle that every tool execution
// lands on. It is set on AgentConfig / ToolRuntimeConfig and carried through
// to the runtime so the decision layer and executors can reason about a
// single shape.
//
// All fields are value types (strings or []string) so a profile can be
// copied and modified without aliasing. Profiles are constructed by the
// built-in constructor functions (GuardianReadOnly, WorkspaceWrite, etc.)
// and should be treated as semi-immutable: callers may swap a field on a
// copy but must not mutate a shared instance.
type ExecutionProfile struct {
	// Name is the human-readable profile name (e.g. "workspace_write").
	Name string
	// PermissionProfile bounds the kind of filesystem access allowed.
	PermissionProfile PermissionProfile
	// SandboxPolicy controls whether sandboxing is required.
	SandboxPolicy SandboxPolicy
	// SandboxPermissions is the set of capabilities granted inside the
	// sandbox (e.g. ["read"], ["read","write"], ["read","write","execute"]).
	SandboxPermissions []string
	// NetworkPolicy bounds network reach.
	NetworkPolicy NetworkPolicy
	// EnvPolicy controls environment variable exposure.
	EnvPolicy EnvPolicy
	// VisibilityPolicy controls how much of the tool call is surfaced.
	VisibilityPolicy VisibilityPolicy
}

// String returns a compact one-line representation suitable for logging.
func (p ExecutionProfile) String() string {
	return fmt.Sprintf(
		"profile(name=%s permission=%s sandbox=%s perms=%v net=%s env=%s visibility=%s)",
		p.Name,
		p.PermissionProfile,
		p.SandboxPolicy,
		p.SandboxPermissions,
		p.NetworkPolicy,
		p.EnvPolicy,
		p.VisibilityPolicy,
	)
}

// GuardianReadOnly is the profile for guardian/review agents. Guardian
// agents must never mutate state and must always run sandboxed with
// restricted network reach (so they can read from external sources for
// review but cannot leak data or mutate the workspace).
func GuardianReadOnly() ExecutionProfile {
	return ExecutionProfile{
		Name:               "guardian_read_only",
		PermissionProfile:  PermissionReadOnly,
		SandboxPolicy:      SandboxRequired,
		SandboxPermissions: []string{"read"},
		NetworkPolicy:      NetworkRestricted,
		EnvPolicy:          EnvMinimal,
		VisibilityPolicy:   VisibilityFull,
	}
}

// WorkspaceReadOnly is the profile for read-only workspace inspection. It
// permits reads from the workspace but no network access and no writes.
// Sandboxing is best-effort because reads are non-destructive and the
// runtime should work on hosts without sandbox support.
func WorkspaceReadOnly() ExecutionProfile {
	return ExecutionProfile{
		Name:               "workspace_read_only",
		PermissionProfile:  PermissionReadOnly,
		SandboxPolicy:      SandboxBestEffort,
		SandboxPermissions: []string{"read"},
		NetworkPolicy:      NetworkNone,
		EnvPolicy:          EnvMinimal,
		VisibilityPolicy:   VisibilityFull,
	}
}

// WorkspaceWrite is the DEFAULT profile for normal agent runs. It permits
// read/write within the workspace and writable roots, with no network
// access. Sandboxing is best-effort and the environment is inherited so
// tools can access project-local env (PATHs, language tokens, etc.).
func WorkspaceWrite() ExecutionProfile {
	return ExecutionProfile{
		Name:               "workspace_write",
		PermissionProfile:  PermissionWorkspaceWrite,
		SandboxPolicy:      SandboxBestEffort,
		SandboxPermissions: []string{"read", "write"},
		NetworkPolicy:      NetworkNone,
		EnvPolicy:          EnvInherit,
		VisibilityPolicy:   VisibilityFull,
	}
}

// OpenWorldRestricted is the profile for agents that need network reach but
// should still be bounded. It allows read/write within the workspace and
// restricted network access, and requires sandboxing.
func OpenWorldRestricted() ExecutionProfile {
	return ExecutionProfile{
		Name:               "open_world_restricted",
		PermissionProfile:  PermissionOpenWorld,
		SandboxPolicy:      SandboxRequired,
		SandboxPermissions: []string{"read", "write"},
		NetworkPolicy:      NetworkRestricted,
		EnvPolicy:          EnvMinimal,
		VisibilityPolicy:   VisibilityFull,
	}
}

// DangerFullAccess is the profile for explicit high-trust scenarios. It
// disables sandboxing, allows read/write/execute, opens network access, and
// inherits the full environment.
//
// This profile must only be used when the user has explicitly opted into a
// high-trust run. The runtime should never default to it.
func DangerFullAccess() ExecutionProfile {
	return ExecutionProfile{
		Name:               "danger_full_access",
		PermissionProfile:  PermissionDangerFullAccess,
		SandboxPolicy:      SandboxNone,
		SandboxPermissions: []string{"read", "write", "execute"},
		NetworkPolicy:      NetworkOpen,
		EnvPolicy:          EnvInherit,
		VisibilityPolicy:   VisibilityFull,
	}
}

// DefaultProfile returns the profile used when no explicit profile is
// configured. It is WorkspaceWrite — the standard profile for normal agent
// runs.
func DefaultProfile() ExecutionProfile {
	return WorkspaceWrite()
}

// ProfileForGuardian returns the profile used for guardian/review sub-agents.
// It is GuardianReadOnly.
func ProfileForGuardian() ExecutionProfile {
	return GuardianReadOnly()
}
