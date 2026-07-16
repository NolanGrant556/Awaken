package executors

import (
	"context"

	"awaken/src/tool"
	"awaken/src/tool/profile"
)

// ArtifactExecutor is the top-level lane executor for typed file/artifact work.
type ArtifactExecutor struct {
	fileOperation ToolExecutor
}

func NewArtifactExecutor() *ArtifactExecutor {
	return &ArtifactExecutor{
		fileOperation: NewFileOperationExecutor(),
	}
}

func (e *ArtifactExecutor) Execute(
	ctx context.Context,
	intent profile.ExecutionIntent,
	execProfile profile.ExecutionProfile,
	t tool.Tool,
	input string,
	observer tool.ToolObserver,
) (ToolExecutionOutput, error) {
	return e.fileOperation.Execute(ctx, intent, execProfile, t, input, observer)
}
