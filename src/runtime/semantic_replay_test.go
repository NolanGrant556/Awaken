package runtime

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"awaken/src/agentcore"
	"awaken/src/protocol/agui"
)

func TestLiveAndReplaySemanticProjectionDeepEqual(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "thread_live.jsonl")
	journal, err := NewExecutionJournal(path, "thread_live")
	if err != nil {
		t.Fatalf("NewExecutionJournal: %v", err)
	}
	defer journal.Close()

	ts := time.Date(2026, 7, 10, 1, 2, 3, 0, time.UTC)
	records := []agentcore.SemanticRecord{
		{
			SchemaVersion: 2, RecordID: "rec_run", Seq: 1, ThreadID: "thread_live", RunID: "run_1",
			Kind: agentcore.SemanticKindRunStarted, Timestamp: ts, Payload: map[string]any{},
		},
		{
			SchemaVersion: 2, RecordID: "rec_call", Seq: 2, ThreadID: "thread_live", RunID: "run_1", TurnID: "turn_1",
			Kind: agentcore.SemanticKindToolCall, Timestamp: ts.Add(time.Second), Payload: map[string]any{
				"tool_call_id": "call_1",
				"tool_name":    "shell",
				"arguments":    `{"command":"echo hi"}`,
			},
		},
		{
			SchemaVersion: 2, RecordID: "rec_result", Seq: 3, ThreadID: "thread_live", RunID: "run_1", TurnID: "turn_1",
			Kind: agentcore.SemanticKindToolResult, Timestamp: ts.Add(2 * time.Second), Payload: map[string]any{
				"tool_call_id": "call_1",
				"tool_name":    "shell",
				"content":      "hi\n",
				"is_error":     false,
				"outcome": map[string]any{
					"status":           "succeeded",
					"transport_status": "completed",
					"semantic_status":  "success",
					"process_status":   "exited",
					"retryable":        false,
				},
			},
		},
		{
			SchemaVersion: 2, RecordID: "rec_art", Seq: 4, ThreadID: "thread_live", RunID: "run_1", TurnID: "turn_1",
			Kind: agentcore.SemanticKindArtifact, Timestamp: ts.Add(3 * time.Second), Payload: map[string]any{
				"artifact_id":  "art_1",
				"title":        "out.txt",
				"kind":         "file",
				"path":         "out.txt",
				"status":       "ready",
				"tool_call_id": "call_1",
				"created":      true,
			},
		},
		{
			SchemaVersion: 2, RecordID: "rec_term", Seq: 5, ThreadID: "thread_live", RunID: "run_1",
			Kind: agentcore.SemanticKindRunTerminal, Timestamp: ts.Add(4 * time.Second), Payload: map[string]any{
				"status": "finished",
				"result": "done",
			},
		},
	}
	if err := journal.AppendBatch(context.Background(), records); err != nil {
		t.Fatalf("AppendBatch: %v", err)
	}
	if err := journal.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	live := normalizeReplayEvents(agui.ProjectSemanticRecords(records))
	loaded, err := ReadExecutionJournal(path)
	if err != nil {
		t.Fatalf("ReadExecutionJournal: %v", err)
	}
	replay := normalizeReplayEvents(agui.ProjectSemanticRecords(loaded))
	if !reflect.DeepEqual(live, replay) {
		t.Fatalf("live/replay mismatch\nlive=%+v\nreplay=%+v", live, replay)
	}
}

func normalizeReplayEvents(events []agui.Event) []agui.Event {
	out := make([]agui.Event, len(events))
	for i, event := range events {
		event.Timestamp = 0 // ignore receive/wall time
		out[i] = event
	}
	return out
}

func TestV1FixtureRecoversToolCardsViaAdapterAndProjector(t *testing.T) {
	records, err := (LegacyTranscriptAdapter{}).ReadFile("testdata/v1_tool_messages_without_tool_events.jsonl")
	if err != nil {
		t.Fatalf("adapter: %v", err)
	}
	events := agui.ProjectSemanticRecords(records)
	var starts, results int
	for _, event := range events {
		switch event.Type {
		case agui.EventToolCallStart:
			starts++
		case agui.EventToolCallResult:
			results++
		}
	}
	if starts < 3 || results < 3 {
		t.Fatalf("expected tool cards from v1 fixture, starts=%d results=%d", starts, results)
	}
}
