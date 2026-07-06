// Package sandbox provides the SandboxAdapter abstraction that the
// ShellExecutor uses to confine shell commands before they run. Each
// platform ships its own adapter (mac, windows, other); the factory
// selects the right one at build time.
//
// This is Task 7 of the "零审批自动执行策略" spec: shell execution must go
// through a controlled executor layer, and platform-specific sandboxing
// can be plugged in. When a platform lacks a trusted isolation boundary
// for open-world commands, the executor fail-closes rather than letting
// the command run unsandboxed.
package sandbox

import "os/exec"

// SandboxSupport grades how much isolation the platform adapter can
// actually provide. The ShellExecutor uses this to decide whether to
// fail-closed for open_world_system commands.
type SandboxSupport string

const (
	// SandboxFull means a real isolation boundary is available (e.g.
	// sandbox-exec on macOS, AppContainer/Job Object on Windows).
	// open_world_system commands may run under this adapter.
	SandboxFull SandboxSupport = "full"

	// SandboxDegraded means a partial boundary is available — the adapter
	// may apply some constraints but cannot fully confine open-world
	// commands. The executor must fail-closed for open_world_system.
	// inspect_read_only and workspace_mutation commands may still run.
	SandboxDegraded SandboxSupport = "degraded"

	// SandboxNone means no isolation is available on this platform. The
	// executor must fail-closed for open_world_system and runs
	// inspect_read_only / workspace_mutation without any confinement.
	SandboxNone SandboxSupport = "none"
)

// Tier constants used by the SandboxAdapter. These mirror
// shelltool.ShellTier; they are duplicated as strings here to avoid an
// import cycle (tools/shell cannot be imported by tool/sandbox).
const (
	TierInspectReadOnly   = "inspect_read_only"
	TierWorkspaceMutation = "workspace_mutation"
	TierOpenWorldSystem   = "open_world_system"
)

// SandboxResult describes what the adapter actually applied to the command.
// It is primarily intended for observability — the executor logs or surfaces
// the support level so callers can see whether the command ran sandboxed.
type SandboxResult struct {
	// Support is the support level reported by the adapter.
	Support SandboxSupport

	// Method identifies the confinement mechanism used, e.g. "sandbox-exec",
	// "appcontainer", "degraded", "none".
	Method string

	// Applied is true when the adapter really constrained the command
	// (i.e. Support == SandboxFull and ApplySandbox succeeded).
	Applied bool
}

// SandboxAdapter is the interface for confining an *exec.Cmd before it
// runs. The executor calls ApplySandbox after constructing the command
// and before calling cmd.Run().
//
// Implementations must be safe for concurrent use.
type SandboxAdapter interface {
	// SupportLevel reports the platform's sandbox support level. The
	// executor uses this to decide whether to fail-closed for
	// open-world commands before constructing the exec.Cmd.
	SupportLevel() SandboxSupport

	// ApplySandbox configures sandbox constraints on cmd. tier is one of
	// the Tier* constants above. workspaceRoot is the absolute path to the
	// workspace the command runs in; adapters use it to scope filesystem
	// allowances in the sandbox profile. The adapter must not start the
	// command; it only mutates cmd (e.g. by wrapping cmd.Path, prepending
	// to cmd.Args, or setting cmd.Env).
	//
	// For degraded/none adapters this is a no-op that returns a
	// SandboxResult with Applied=false.
	ApplySandbox(cmd *exec.Cmd, tier string, workspaceRoot string) (SandboxResult, error)
}
