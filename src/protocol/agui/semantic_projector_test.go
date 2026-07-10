package agui

import (
	"testing"
	"time"

	"awaken/src/agentcore"
)

func TestProjectSemanticRecordsToolOrder(t *testing.T) {
	ts := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	records := []agentcore.SemanticRecord{
		{SchemaVersion: 2, RecordID: "r1", Seq: 1, ThreadID: "t", RunID: "run", Kind: agentcore.SemanticKindRunStarted, Timestamp: ts},
		{SchemaVersion: 2, RecordID: "r2", Seq: 2, ThreadID: "t", RunID: "run", Kind: agentcore.SemanticKindToolCall, Timestamp: ts, Payload: map[string]any{
			"tool_call_id": "call_1",
			"tool_name":    "shell",
			"arguments":    `{"command":"echo hi"}`,
		}},
		{SchemaVersion: 2, RecordID: "r3", Seq: 3, ThreadID: "t", RunID: "run", Kind: agentcore.SemanticKindToolResult, Timestamp: ts, Payload: map[string]any{
			"tool_call_id": "call_1",
			"tool_name":    "shell",
			"content":      "hi",
			"is_error":     false,
			"outcome": map[string]any{
				"status":           "succeeded",
				"transport_status": "completed",
				"semantic_status":  "neutral",
				"process_status":   "exited",
				"diagnostic":       map[string]any{"reason": "no_match"},
			},
		}},
		{SchemaVersion: 2, RecordID: "r4", Seq: 4, ThreadID: "t", RunID: "run", Kind: agentcore.SemanticKindArtifact, Timestamp: ts, Payload: map[string]any{
			"artifact_id":  "art_1",
			"title":        "note",
			"kind":         "file",
			"path":         "note.txt",
			"status":       "ready",
			"tool_call_id": "call_1",
			"created":      true,
		}},
		{SchemaVersion: 2, RecordID: "r5", Seq: 5, ThreadID: "t", RunID: "run", Kind: agentcore.SemanticKindRunTerminal, Timestamp: ts, Payload: map[string]any{
			"status": "finished",
			"result": "done",
		}},
	}

	events := ProjectSemanticRecords(records)
	var types []EventType
	for _, event := range events {
		types = append(types, event.Type)
	}
	wantPrefix := []EventType{
		EventRunStarted,
		EventToolCallStart,
		EventToolCallArgs,
		EventToolCallEnd,
		EventToolCallResult,
		EventAwakenArtifactCreated,
		EventRunFinished,
	}
	if len(types) < len(wantPrefix) {
		t.Fatalf("expected at least %d events, got %v", len(wantPrefix), types)
	}
	for i, want := range wantPrefix {
		if types[i] != want {
			t.Fatalf("event[%d]=%s want %s (all=%v)", i, types[i], want, types)
		}
	}
	if events[4].ToolStatus != "succeeded" {
		t.Fatalf("expected tool result status succeeded, got %q", events[4].ToolStatus)
	}
	if events[4].Diagnostic["reason"] != "no_match" {
		t.Fatalf("expected diagnostic reason no_match, got %+v", events[4].Diagnostic)
	}
}

func TestProjectSemanticRecordsIdempotent(t *testing.T) {
	records := []agentcore.SemanticRecord{{
		SchemaVersion: 2,
		RecordID:      "r1",
		Seq:           1,
		ThreadID:      "t",
		RunID:         "run",
		Kind:          agentcore.SemanticKindAssistantMessage,
		Timestamp:     time.Now().UTC(),
		Payload:       map[string]any{"content": "hello", "message_id": "msg_1"},
	}}
	first := ProjectSemanticRecords(records)
	second := ProjectSemanticRecords(records)
	if len(first) != len(second) {
		t.Fatalf("idempotent length mismatch: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Type != second[i].Type || first[i].Delta != second[i].Delta || first[i].MessageID != second[i].MessageID || first[i].Seq != second[i].Seq {
			t.Fatalf("idempotent mismatch at %d: %+v vs %+v", i, first[i], second[i])
		}
	}
	if len(first) != 3 {
		t.Fatalf("expected assistant message to project three frames, got %d", len(first))
	}
	seen := map[uint64]bool{}
	for _, event := range first {
		if seen[event.Seq] {
			t.Fatalf("projected frame seq must be unique, duplicate %d in %+v", event.Seq, first)
		}
		seen[event.Seq] = true
	}
}

func TestProjectSemanticRuntimeNoticePreservesRuntimeFrame(t *testing.T) {
	record := agentcore.SemanticRecord{
		SchemaVersion: 2,
		RecordID:      "runtime-ready",
		Seq:           9,
		ThreadID:      "thread-runtime",
		RunID:         "run-runtime",
		Kind:          agentcore.SemanticKindRuntimeNotice,
		Timestamp:     time.Now().UTC(),
		Payload: map[string]any{
			"event_type":      string(agentcore.RuntimeReadyEventType),
			"tool_call_id":    "call-runtime",
			"tool_name":       "presentation_create",
			"content":         "runtime ready",
			"runtime_name":    "primary-runtime",
			"runtime_version": "0.2.0",
		},
	}
	events := ProjectSemanticRecords([]agentcore.SemanticRecord{record})
	if len(events) != 1 || events[0].Type != EventAwakenRuntimeReady {
		t.Fatalf("unexpected runtime replay frame: %+v", events)
	}
	if events[0].ToolCallID != "call-runtime" || events[0].RuntimeVersion != "0.2.0" || events[0].Message != "runtime ready" {
		t.Fatalf("runtime replay fields lost: %+v", events[0])
	}
}
