package delegateexternalagent

import (
	"context"
	"testing"

	"awaken/src/acp"
	"awaken/src/tool"
)

func TestACPUpdateHandlerEmitsToolSnapshot(t *testing.T) {
	var emitted []tool.ACPEvent
	ctx := tool.ContextWithACPEventEmitter(context.Background(), func(e tool.ACPEvent) {
		emitted = append(emitted, e)
	})
	ctx = tool.ContextWithToolThreadID(ctx, "thread-1")

	handler := acpUpdateHandler(ctx, "codex", "host-tool-1")
	handler.OnUpdate(acp.SessionUpdate{
		SessionID: "sess-1",
		Kind:      acp.SessionUpdateKindToolCall,
		ToolCall: &acp.ToolCallSnapshot{
			ToolCallID: "acp-tool-1",
			Title:      "Edit file",
			Kind:       "edit",
			Status:     "completed",
			Locations:  []string{"/tmp/note.md"},
			Summary:    "Updated note.md",
		},
	})

	if len(emitted) != 1 {
		t.Fatalf("expected 1 event, got %d", len(emitted))
	}
	got := emitted[0]
	if got.Type != tool.ACPEventTypeToolSnapshot {
		t.Fatalf("expected tool snapshot event, got %s", got.Type)
	}
	if got.ToolCallID != "host-tool-1" || got.ToolName != "edit" {
		t.Fatalf("unexpected tool snapshot payload: %+v", got)
	}
	if got.Diagnostic["runner_id"] != "codex" || got.Diagnostic["acp_session_id"] != "sess-1" {
		t.Fatalf("unexpected diagnostic: %+v", got.Diagnostic)
	}
}

func TestACPUpdateHandlerIgnoresNonToolUpdates(t *testing.T) {
	var emitted []tool.ACPEvent
	ctx := tool.ContextWithACPEventEmitter(context.Background(), func(e tool.ACPEvent) {
		emitted = append(emitted, e)
	})
	ctx = tool.ContextWithToolThreadID(ctx, "thread-1")

	handler := acpUpdateHandler(ctx, "codex", "host-tool-1")
	handler.OnUpdate(acp.SessionUpdate{Kind: acp.SessionUpdateKindAgentText, TextDelta: "hello"})
	handler.OnUpdate(acp.SessionUpdate{Kind: acp.SessionUpdateKindAgentThought, TextDelta: "thinking"})

	if len(emitted) != 0 {
		t.Fatalf("expected no events for non-tool updates, got %+v", emitted)
	}
}
