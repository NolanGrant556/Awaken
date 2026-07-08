package executors

import (
	"context"

	"awaken/src/tool"
	"awaken/src/tool/profile"
)

// ShellExecutor is the executors-package wrapper for shell command tools.
// It is a thin layer that delegates to the tool's own Execute / ExecuteWithObserver,
// which internally calls the existing tools/shell.ShellExecutor (Task 7).
//
// The existing tools/shell.ShellExecutor already:
//   - Classifies the command via ClassifyShellCommand.
//   - Looks up the platform sandbox adapter support level for observability.
//   - Enforces the local-computer hard gates: delete via Trash and
//     mass-message review.
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

// Execute injects the profile EnvPolicy before delegating to the shell tool's
// own Execute / ExecuteWithObserver. Shell is a local-computer surface and no
// longer fails closed on sandbox availability; the shell tool's policy keeps
// the product hard gates (Trash-only deletes and mass-message review).
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
	ctx = profile.WithEnvPolicy(ctx, execProfile.EnvPolicy)
	content, err := executeWithObserver(ctx, t, input, observer)
	if err != nil {
		return ToolExecutionOutput{}, err
	}
	return ToolExecutionOutput{Content: content}, nil
}
