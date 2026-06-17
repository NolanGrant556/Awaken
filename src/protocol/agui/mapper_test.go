package agui

import (
	"testing"
	"time"

	"awaken/src/agentcore"
)

func TestMapEventReasoningLifecycle(t *testing.T) {
	ts := time.Unix(0, 0).UTC()
	events := MapEvent(agentcore.AwakenEvent{
		Type:      agentcore.ReasoningStartedEventType,
		Timestamp: ts,
		ThreadID:  "thread-1",
		RunID:     "run-1",
		MessageID: "reasoning-1",
	})

	if len(events) != 2 {
		t.Fatalf("expected 2 AG-UI events, got %d", len(events))
	}
	if events[0].Type != EventReasoningStart || events[1].Type != EventReasoningMessageStart {
		t.Fatalf("unexpected event types: %+v", events)
	}
	if events[1].Role != "reasoning" {
		t.Fatalf("expected reasoning role, got %q", events[1].Role)
	}
	if events[0].Timestamp != 0 {
		t.Fatalf("expected unix epoch millis, got %d", events[0].Timestamp)
	}
}

func TestMapEventToolResult(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:       agentcore.ToolResultEventType,
		ThreadID:   "thread-1",
		RunID:      "run-1",
		MessageID:  "toolmsg-1",
		ToolCallID: "tool-1",
		Content:    "echo:hello",
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventToolCallResult {
		t.Fatalf("expected TOOL_CALL_RESULT, got %s", events[0].Type)
	}
	if events[0].Role != "tool" {
		t.Fatalf("expected tool role, got %q", events[0].Role)
	}
	if events[0].Content != "echo:hello" {
		t.Fatalf("unexpected content %q", events[0].Content)
	}
}
