package tool

import (
	"context"
	"testing"
)

func TestToolExecutionIdentityRoundTripRequiresCompleteHostIdentity(t *testing.T) {
	identity := ToolExecutionIdentity{RunID: "run-1", ToolCallID: "call-1", ToolName: "artifact_publish"}
	ctx := ContextWithToolExecutionIdentity(context.Background(), identity)
	if got, ok := ToolExecutionIdentityFromContext(ctx); !ok || got != identity {
		t.Fatalf("tool execution identity did not round trip: got=%+v ok=%v", got, ok)
	}

	incomplete := ContextWithToolExecutionIdentity(context.Background(), ToolExecutionIdentity{RunID: "run-1"})
	if _, ok := ToolExecutionIdentityFromContext(incomplete); ok {
		t.Fatal("incomplete tool execution identity must not be accepted")
	}
}
