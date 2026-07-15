package tool

import (
	"context"
	"strings"
)

// ToolExecutionIdentity identifies the exact host-owned invocation. It is
// intentionally not derived from model-provided tool arguments and is shared
// by typed providers that must bind work to the current run and tool call.
type ToolExecutionIdentity struct {
	RunID      string
	ToolCallID string
	ToolName   string
}

type toolExecutionIdentityContextKey struct{}

func ContextWithToolExecutionIdentity(ctx context.Context, identity ToolExecutionIdentity) context.Context {
	return context.WithValue(ctx, toolExecutionIdentityContextKey{}, identity)
}

func ToolExecutionIdentityFromContext(ctx context.Context) (ToolExecutionIdentity, bool) {
	if ctx == nil {
		return ToolExecutionIdentity{}, false
	}
	identity, ok := ctx.Value(toolExecutionIdentityContextKey{}).(ToolExecutionIdentity)
	return identity, ok && validToolExecutionIdentity(identity)
}

func validToolExecutionIdentity(identity ToolExecutionIdentity) bool {
	return strings.TrimSpace(identity.RunID) != "" &&
		strings.TrimSpace(identity.ToolCallID) != "" &&
		strings.TrimSpace(identity.ToolName) != ""
}
