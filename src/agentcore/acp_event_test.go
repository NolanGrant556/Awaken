package agentcore

import (
	"testing"

	"awaken/src/tool"
)

func TestMapACPEvent(t *testing.T) {
	cases := []struct {
		input    tool.ACPEventType
		expected AwakenEventType
	}{
		{tool.ACPEventTypeSessionStarted, ACPSessionStartedEventType},
		{tool.ACPEventTypeSessionClosed, ACPSessionClosedEventType},
		{tool.ACPEventTypeSessionInterrupted, ACPSessionInterruptedEventType},
		{tool.ACPEventTypeTurnStarted, ACPTurnStartedEventType},
		{tool.ACPEventTypeTurnFinished, ACPTurnFinishedEventType},
		{tool.ACPEventTypeToolSnapshot, ACPToolSnapshotEventType},
		{tool.ACPEventTypePermissionRequested, ACPPermissionRequestedEventType},
		{tool.ACPEventTypePermissionResolved, ACPPermissionResolvedEventType},
	}
	for _, tc := range cases {
		event := MapACPEvent(tool.ACPEvent{
			Type:          tc.input,
			ThreadID:      "thread-1",
			RunID:         "run-1",
			ToolCallID:    "tool-1",
			InteractionID: "perm-1",
			ToolName:      "edit",
			Content:       "content",
			Status:        "status",
			Diagnostic:    map[string]any{"key": "value"},
		})
		if event.Type != tc.expected {
			t.Fatalf("expected %s, got %s", tc.expected, event.Type)
		}
		if event.ThreadID != "thread-1" || event.RunID != "run-1" || event.ToolCallID != "tool-1" {
			t.Fatalf("metadata not preserved: %+v", event)
		}
		if event.Diagnostic["key"] != "value" {
			t.Fatalf("diagnostic not preserved: %+v", event.Diagnostic)
		}
		if event.Diagnostic["status"] != "status" {
			t.Fatalf("status not copied to diagnostic: %+v", event.Diagnostic)
		}
	}
}

func TestMapACPEventUnknownType(t *testing.T) {
	event := MapACPEvent(tool.ACPEvent{Type: tool.ACPEventType("custom")})
	if event.Type != AwakenEventType("custom") {
		t.Fatalf("expected unknown type to pass through, got %s", event.Type)
	}
}
