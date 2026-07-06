package executors

import (
	"context"

	"awaken/src/tool"
	"awaken/src/tool/profile"
)

// ConnectorExecutor is the top-level lane executor for official connectors and
// MCP-backed capabilities.
type ConnectorExecutor struct {
	mcp ToolExecutor
}

func NewConnectorExecutor() *ConnectorExecutor {
	return &ConnectorExecutor{
		mcp: NewMcpExecutor(),
	}
}

func (e *ConnectorExecutor) Execute(
	ctx context.Context,
	intent profile.ExecutionIntent,
	execProfile profile.ExecutionProfile,
	t tool.Tool,
	input string,
	observer tool.ToolObserver,
) (ToolExecutionOutput, error) {
	return e.mcp.Execute(ctx, intent, execProfile, t, input, observer)
}
