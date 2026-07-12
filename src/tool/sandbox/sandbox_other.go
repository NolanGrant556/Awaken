//go:build !darwin && !windows

package sandbox

import "os/exec"

// SandboxAdapterOther is the fail-closed fallback for platforms without a
// dedicated sandbox adapter. It reports SandboxNone, so the default Shell
// broker permits bounded read-only inspection only.
type SandboxAdapterOther struct{}

func defaultPlatformAdapter() SandboxAdapter {
	return SandboxAdapterOther{}
}

// SupportLevel returns SandboxNone — no isolation is available on this
// platform.
func (SandboxAdapterOther) SupportLevel() SandboxSupport {
	return SandboxNone
}

// ApplySandbox is a no-op; there is no sandbox to apply.
func (SandboxAdapterOther) ApplySandbox(cmd *exec.Cmd, tier string, workspaceRoot string) (SandboxResult, error) {
	return SandboxResult{
		Support: SandboxNone,
		Method:  "none",
		Applied: false,
	}, nil
}
