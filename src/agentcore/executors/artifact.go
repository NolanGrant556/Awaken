package executors

import (
	"context"

	"awaken/src/tool"
	"awaken/src/tool/profile"
)

// ArtifactExecutor is the top-level lane executor for file/artifact work. It
// keeps file-chain tools behind a product-facing artifact capability instead of
// exposing them directly as the top-level routing concept.
type ArtifactExecutor struct {
	fileChain ToolExecutor
}

func NewArtifactExecutor() *ArtifactExecutor {
	return &ArtifactExecutor{
		fileChain: NewFileChainExecutor(),
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
	return e.fileChain.Execute(ctx, intent, execProfile, t, input, observer)
}
