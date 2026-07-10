package executors

import (
	"context"

	"awaken/src/tool"
	"awaken/src/tool/profile"
)

// McpExecutor wraps execution of MCP / connector tools. In Awaken's zero-
// approval work mode, "configured and connected" is the trust grant: once an
// MCP server is available, its tools should remain callable without a second
// metadata gate. The executor therefore only enforces profile-level read-only
// constraints; metadata stays useful for routing and UI but is not a hard stop.
type McpExecutor struct{}

// NewMcpExecutor returns an McpExecutor.
func NewMcpExecutor() *McpExecutor {
	return &McpExecutor{}
}

// Execute enforces only profile-level read-only constraints and otherwise
// delegates to the tool. needs_auth remains handled in the adapter's
// CheckPermission path.
func (e *McpExecutor) Execute(
	ctx context.Context,
	intent profile.ExecutionIntent,
	execProfile profile.ExecutionProfile,
	t tool.Tool,
	input string,
	observer tool.ToolObserver,
) (ToolExecutionOutput, error) {
	if execProfile.PermissionProfile == profile.PermissionReadOnly && !t.Definition().Annotations.ReadOnly {
		return deniedOutput("read_only_violation", "profile is read_only but MCP tool requests write-capable behavior")
	}
	return executeWithObserver(ctx, t, input, observer)
}
