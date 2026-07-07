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
//   - inspect_read_only and workspace_mutation commands may run. Their
//     safety relies on the file-chain boundary and the permission layer's
//     sensitive-path / writable-roots checks, which are platform-independent.
//   - open_world_system commands are denied two ways:
//     1. The ShellExecutor fail-closes before constructing the exec.Cmd
//     because SupportLevel() != SandboxFull (executor.go line ~112).
//     2. ApplySandbox also returns an error for open_world_system as
//     defense-in-depth, in case a future caller bypasses the executor's
//     pre-check.
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
// For inspect_read_only and workspace_mutation this is currently a no-op:
// the command runs without an additional isolation boundary, and safety
// depends on the permission layer's path checks. The SandboxResult reports
// the degraded status so callers can observe it. GeneratePowerShellWrapper
// is available for a future SandboxFull implementation to wrap cmd with a
// PowerShell profile; it is intentionally NOT invoked here because the
// degraded adapter does not provide a real process boundary.
//
// For open_world_system this returns an error (fail-closed). The
// ShellExecutor already refuses open_world_system commands when
// SupportLevel() != SandboxFull; this guard is defense-in-depth for any
// caller that constructs an exec.Cmd directly and invokes ApplySandbox.
func (SandboxAdapterWindows) ApplySandbox(cmd *exec.Cmd, tier string, workspaceRoot string) (SandboxResult, error) {
	base := SandboxResult{
		Support: SandboxDegraded,
		Method:  "degraded",
		Applied: false,
	}
	switch tier {
	case TierInspectReadOnly, TierWorkspaceMutation:
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
