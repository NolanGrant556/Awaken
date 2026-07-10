package runtime

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
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

func TestA32V1AdapterPreservesCompleteToolArgsResultsAndDesktopCardSemantics(t *testing.T) {
	records, err := (LegacyTranscriptAdapter{}).ReadFile("testdata/v1_tool_messages_without_tool_events.jsonl")
	if err != nil {
		t.Fatalf("A32 adapter: %v", err)
	}
	events := agui.ProjectSemanticRecords(records)

	type card struct {
		start, args, end, result int
		arguments                string
		content                  string
		role                     string
		status                   string
	}
	cards := map[string]*card{
		"call_apply_1": {start: -1, args: -1, end: -1, result: -1},
		"call_apply_2": {start: -1, args: -1, end: -1, result: -1},
		"call_shell_1": {start: -1, args: -1, end: -1, result: -1},
	}
	artifacts := 0
	for index, event := range events {
		if event.Type == agui.EventAwakenArtifactCreated || event.Type == agui.EventAwakenArtifactUpdated {
			artifacts++
		}
		card := cards[event.ToolCallID]
		if card == nil {
			continue
		}
		switch event.Type {
		case agui.EventToolCallStart:
			card.start = index
		case agui.EventToolCallArgs:
			card.args = index
			card.arguments = event.Delta
		case agui.EventToolCallEnd:
			card.end = index
		case agui.EventToolCallResult:
			card.result = index
			card.content = event.Content
			card.role = event.Role
			card.status = event.ToolStatus
		}
	}
	if artifacts != 0 {
		t.Fatalf("A32 adapter invented %d artifact cards", artifacts)
	}

	patchArgs := `{"patch":"*** Begin Patch\n*** Update File: notes/sample.txt\n@@\n-alpha\n+beta\n*** End Patch"}`
	patchResult := "failed to find expected lines in notes/sample.txt:\nalpha"
	for _, callID := range []string{"call_apply_1", "call_apply_2"} {
		card := cards[callID]
		if card.start < 0 || card.args <= card.start || card.end <= card.args || card.result <= card.end {
			t.Fatalf("A32 incomplete/out-of-order Desktop tool card %s: %+v", callID, card)
		}
		if card.arguments != patchArgs || card.content != patchResult || card.role != "tool" || card.status != "failed" {
			t.Fatalf("A32 patch card lost exact args/result semantics %s: %+v", callID, card)
		}
	}

	shell := cards["call_shell_1"]
	if shell.start < 0 || shell.args <= shell.start || shell.end <= shell.args || shell.result <= shell.end {
		t.Fatalf("A32 incomplete/out-of-order shell card: %+v", shell)
	}
	if shell.arguments != `{"command":"exit 1"}` ||
		!strings.Contains(shell.content, `"exit_code": 1`) ||
		!strings.Contains(shell.content, `"timed_out": false`) || shell.role != "tool" {
		t.Fatalf("A32 shell card lost full legacy payload: %+v", shell)
	}
	if shell.status != "" {
		t.Fatalf("A32 adapter must preserve legacy is_error=false instead of inventing typed failure, got %q", shell.status)
	}
}
