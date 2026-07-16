package executors

import (
	"context"

	"awaken/src/tool"
	"awaken/src/tool/profile"
)

// SystemExecutor wraps the broad system-operation lane. Today that lane is the
// Bash surface; keeping it behind a lane executor preserves the product-level
// system-operation category.
type SystemExecutor struct {
	bash ToolExecutor
}

func NewSystemExecutor() *SystemExecutor {
	return &SystemExecutor{
		bash: NewBashExecutor(),
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
	return e.bash.Execute(ctx, intent, execProfile, t, input, observer)
}
