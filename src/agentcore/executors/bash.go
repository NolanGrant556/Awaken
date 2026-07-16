package executors

import (
	"context"

	"awaken/src/tool"
	"awaken/src/tool/profile"
)

// BashExecutor is the neutral executor-layer wrapper for the Bash tool. The
// Bash tool owns current-user process launch, no-rc shell selection, cwd,
// timeout, output, cancellation, and background-task behavior.
type BashExecutor struct{}

func NewBashExecutor() *BashExecutor { return &BashExecutor{} }

// Execute injects only the selected environment policy and delegates to the
// tool. It has no restricted, supervised, staging, or broker-receipt branch.
func (e *BashExecutor) Execute(
	ctx context.Context,
	intent profile.ExecutionIntent,
	execProfile profile.ExecutionProfile,
	t tool.Tool,
	input string,
	observer tool.ToolObserver,
) (ToolExecutionOutput, error) {
	ctx = profile.WithEnvPolicy(ctx, execProfile.EnvPolicy)
	return executeWithObserver(ctx, t, input, observer)
}
