// Server-side allowlist and normalization for client-supplied
// ExecutionProfile values.
//
// POST /v1/runs accepts a workspace.execution_profile field. Without
// server-side validation a client could inject DangerFullAccess (or a
// tampered WorkspaceWrite with elevated fields) and bypass the runtime's
// trust boundary. This file defines the client allowlist and a
// NormalizeClientProfile entry point used by normalizeWorkspace to map
// any client-supplied profile onto a known built-in instance.
package profile

import (
	"log"
	"os"
)

// dangerEnvVar is the environment variable that explicitly opts the
// server into accepting danger_full_access profile requests from
// clients. Only the literal value "true" enables it.
const dangerEnvVar = "AWAKEN_ALLOW_DANGER_PROFILE"

// ClientAllowedProfiles returns the built-in profiles that a client may
// request via POST /v1/runs workspace.execution_profile. Profiles not in
// this list (GuardianReadOnly, DangerFullAccess) are server-only: any
// client request for them is normalized back to DefaultProfile (or, for
// danger_full_access with the env var set, to the built-in DangerFullAccess).
func ClientAllowedProfiles() []ExecutionProfile {
	return []ExecutionProfile{
		WorkspaceReadOnly(),
		WorkspaceWrite(),
		OpenWorldRestricted(),
	}
}

// IsClientAllowed reports whether p exactly matches one of the built-in
// client-allowed profiles by Name plus the six key policy fields
// (ApprovalPolicy, PermissionProfile, SandboxPolicy, NetworkPolicy,
// EnvPolicy, VisibilityPolicy). Tampering with any of those fields
// causes the profile to no longer match.
//
// GuardianReadOnly and DangerFullAccess are never client-allowed
// regardless of field values: they are server-only profiles set by the
// subagent spawner or the danger opt-in path.
func IsClientAllowed(p ExecutionProfile) bool {
	for _, allowed := range ClientAllowedProfiles() {
		if profilesKeyEqual(p, allowed) {
			return true
		}
	}
	return false
}

// NormalizeClientProfile returns the ExecutionProfile the runtime should
// actually use for a client-supplied profile. The returned profile is
// always a built-in profile value, never the caller-supplied struct, so
// clients cannot inject tampered field values into the runtime.
//
// Behavior:
//   - nil -> DefaultProfile()
//   - danger_full_access -> DangerFullAccess() when
//     AWAKEN_ALLOW_DANGER_PROFILE=true, otherwise DefaultProfile()
//     (with a warning).
//   - name matches a client-allowed profile (workspace_write,
//     workspace_read_only, open_world_restricted) -> the built-in
//     version of that profile. Field tampering is silently overwritten
//     and a warning is logged.
//   - any other name (including guardian_read_only and unknown names) ->
//     DefaultProfile() (with a warning).
func NormalizeClientProfile(requested *ExecutionProfile) ExecutionProfile {
	if requested == nil {
		return DefaultProfile()
	}
	if requested.Name == DangerFullAccess().Name {
		if IsDangerFullAccessEnabled() {
			return DangerFullAccess()
		}
		log.Printf("profile: client requested %q but %s is not enabled; falling back to %q",
			requested.Name, dangerEnvVar, DefaultProfile().Name)
		return DefaultProfile()
	}
	for _, allowed := range ClientAllowedProfiles() {
		if requested.Name != allowed.Name {
			continue
		}
		if !profilesKeyEqual(*requested, allowed) {
			log.Printf("profile: client-supplied profile %q had tampered fields; overwriting with built-in version",
				requested.Name)
		}
		return allowed
	}
	log.Printf("profile: client-supplied profile name %q is not in the client allowlist; falling back to %q",
		requested.Name, DefaultProfile().Name)
	return DefaultProfile()
}

// IsDangerFullAccessEnabled reports whether the server has explicitly
// opted into accepting danger_full_access profile requests from clients
// via the AWAKEN_ALLOW_DANGER_PROFILE environment variable. Only the
// literal value "true" enables it; any other value (including empty)
// disables it.
func IsDangerFullAccessEnabled() bool {
	return os.Getenv(dangerEnvVar) == "true"
}

// profilesKeyEqual reports whether two profiles share the same Name and
// the six key policy fields used to detect client tampering
// (ApprovalPolicy, PermissionProfile, SandboxPolicy, NetworkPolicy,
// EnvPolicy, VisibilityPolicy).
//
// SandboxPermissions is intentionally excluded from the comparison: it
// is reset whenever NormalizeClientProfile returns a built-in profile.
// Comparing only the policy fields keeps the allowlist check robust to
// clients that copy a built-in profile and append an extra sandbox
// permission without otherwise escalating trust.
func profilesKeyEqual(a, b ExecutionProfile) bool {
	if a.Name != b.Name {
		return false
	}
	if a.ApprovalPolicy != b.ApprovalPolicy {
		return false
	}
	if a.PermissionProfile != b.PermissionProfile {
		return false
	}
	if a.SandboxPolicy != b.SandboxPolicy {
		return false
	}
	if a.NetworkPolicy != b.NetworkPolicy {
		return false
	}
	if a.EnvPolicy != b.EnvPolicy {
		return false
	}
	if a.VisibilityPolicy != b.VisibilityPolicy {
		return false
	}
	return true
}
