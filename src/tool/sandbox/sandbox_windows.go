//go:build windows

package sandbox

import (
	"fmt"
	"os/exec"
)

// SandboxAdapterWindows is the Windows sandbox adapter.
//
// DEGRADATION STRATEGY (per spec "平台现实约束"):
// Real Windows shell sandboxing requires either cgo bindings to the
// AppContainer / Job Object APIs, or launching commands through a
// PowerShell wrapper that enforces a restricted runspace. Both are out of
// scope for the first phase. Until one lands, this adapter reports
// SandboxDegraded:
//
//   - the default Shell broker permits bounded read-only inspection only;
//   - workspace mutation cannot obtain RequiresAppliedSandbox authorization;
//   - open_world_system is also rejected by ApplySandbox as defense-in-depth.
//
// ESCALATION PATH: GeneratePowerShellWrapper (in sandbox_windows_logic.go,
// no build tag) generates the PowerShell command string that a real
// AppContainer/Job Object implementation would invoke inside a restricted
// process. When cgo bindings or a PowerShell runspace wrapper land:
//   - Change SupportLevel to return SandboxFull.
//   - Have ApplySandbox wrap cmd.Path/Args with the generated profile
//     (e.g. launch powershell.exe -NoProfile -Command <wrapper> inside an
//     AppContainer with a restricted capability SID).
//   - Return SandboxResult{Applied: true, Method: "appcontainer"} on success.
type SandboxAdapterWindows struct{}

func defaultPlatformAdapter() SandboxAdapter {
	return SandboxAdapterWindows{}
}

// SupportLevel returns SandboxDegraded. See the type doc for the
// escalation path to SandboxFull.
func (SandboxAdapterWindows) SupportLevel() SandboxSupport {
	return SandboxDegraded
}

// ApplySandbox configures sandbox constraints on cmd.
//
// For inspect_read_only, local_cli, and workspace_mutation this reports a
// visible no-op. That is sufficient only for a restricted read-only broker;
// RequiresAppliedSandbox authorizations reject Applied=false. The generated
// PowerShell wrapper is not a security boundary and is intentionally unused.
//
// For open_world_system this returns an error (fail-closed). The
// The default broker already refuses open_world_system; this guard is
// defense-in-depth for direct adapter callers.
func (SandboxAdapterWindows) ApplySandbox(cmd *exec.Cmd, tier string, workspaceRoot string) (SandboxResult, error) {
	base := SandboxResult{
		Support: SandboxDegraded,
		Method:  "degraded",
		Applied: false,
	}
	switch tier {
	case TierInspectReadOnly, TierLocalCLI, TierWorkspaceMutation:
		return base, nil
	case TierOpenWorldSystem:
		return base, fmt.Errorf(
			"open_world_system commands require SandboxFull support; " +
				"Windows adapter is degraded (integrate AppContainer/Job Object to enable)",
		)
	default:
		return base, fmt.Errorf("unknown sandbox tier %q", tier)
	}
}
