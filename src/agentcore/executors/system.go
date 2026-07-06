package executors

import (
	"context"

	"awaken/src/tool"
	"awaken/src/tool/profile"
)

// SystemExecutor wraps the broad system-operation lane. Today that lane is the
// shell surface; keeping it behind a lane executor makes later policy changes
// easier without re-exposing shell as the product-level category.
type SystemExecutor struct {
	shell ToolExecutor
}

func NewSystemExecutor() *SystemExecutor {
	return &SystemExecutor{
		shell: NewShellExecutor(),
	}
}

func (e *SystemExecutor) Execute(
	ctx context.Context,
	intent profile.ExecutionIntent,
	execProfile profile.ExecutionProfile,
	t tool.Tool,
	input string,
	observer tool.ToolObserver,
) (ToolExecutionOutput, error) {
	return e.shell.Execute(ctx, intent, execProfile, t, input, observer)
}
