package executors

import (
	"context"

	"awaken/src/tool"
	"awaken/src/tool/profile"
	"awaken/src/tool/sandbox"
)

// ShellExecutor is the executors-package wrapper for shell command tools.
// It is a thin layer that delegates to the tool's own Execute / ExecuteWithObserver,
// which internally calls the existing tools/shell.ShellExecutor (Task 7).
//
// The existing tools/shell.ShellExecutor already:
//   - Classifies the command via ClassifyShellCommand.
//   - Looks up the platform sandbox adapter and its support level.
//   - Fails closed with ErrSandboxUnavailable when an open_world_system
//     command is requested on a platform without SandboxFull support.
//   - Applies sandbox constraints to the exec.Cmd.
//
// This wrapper does NOT duplicate the shell execution logic. Its role is
// to provide a uniform ToolExecutor interface so the Router can route
// shell_command intents through the same executor layer as file_chain and
// mcp_app intents, and to serve as the extension point for executor-level
// enforcement of the ExecutionProfile's SandboxPolicy and EnvPolicy.
type ShellExecutor struct{}

// NewShellExecutor returns a ShellExecutor.
func NewShellExecutor() *ShellExecutor {
	return &ShellExecutor{}
}

// Execute enforces the profile's SandboxPolicy and EnvPolicy before
// delegating to the shell tool's own Execute / ExecuteWithObserver.
//
// SandboxPolicy enforcement:
//   - SandboxRequired: the platform MUST provide SandboxFull isolation.
//     If sandbox.DefaultAdapter().SupportLevel() is anything less than
//     SandboxFull, the call is denied fail-closed rather than running
//     unsandboxed. This is the executor-layer analogue of the
//     open_world_system guard inside tools/shell.ShellExecutor, but
//     keyed off the profile's hard requirement rather than the command
//     tier.
//   - SandboxBestEffort / SandboxNone: execution proceeds. The
//     tools/shell.ShellExecutor still consults the adapter for
//     open_world_system commands.
//
// EnvPolicy enforcement:
//   - The profile's EnvPolicy is injected into the context via
//     profile.WithEnvPolicy. tools/shell.ShellExecutor reads it via
//     profile.EnvPolicyFromContext and constrains cmd.Env accordingly
//     (EnvMinimal → allowlist; EnvInherit → inherit parent).
func (e *ShellExecutor) Execute(
	ctx context.Context,
	intent profile.ExecutionIntent,
	execProfile profile.ExecutionProfile,
	t tool.Tool,
	input string,
	observer tool.ToolObserver,
) (ToolExecutionOutput, error) {
	if execProfile.SandboxPolicy == profile.SandboxRequired {
		if sandbox.DefaultAdapter().SupportLevel() != sandbox.SandboxFull {
			return deniedOutput("sandbox_required", "profile requires sandbox but platform does not provide trusted isolation")
		}
	}
	ctx = profile.WithEnvPolicy(ctx, execProfile.EnvPolicy)
	content, err := executeWithObserver(ctx, t, input, observer)
	if err != nil {
		return ToolExecutionOutput{}, err
	}
	return ToolExecutionOutput{Content: content}, nil
}
