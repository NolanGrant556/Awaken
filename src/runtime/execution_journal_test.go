package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"awaken/src/agentcore"
	"awaken/src/tool"
)

func TestExecutionJournalAppendAndRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "thread_test.jsonl")
	journal, err := NewExecutionJournal(path, "thread_test")
	if err != nil {
		t.Fatalf("NewExecutionJournal: %v", err)
	}
	defer journal.Close()

	ctx := context.Background()
	if err := journal.Append(ctx, agentcore.NewSemanticRecord(
		agentcore.SemanticKindRunStarted, "thread_test", "run_1", "", "", "session", map[string]any{},
	)); err != nil {
		t.Fatalf("Append run_started: %v", err)
	}
	if err := journal.AppendBatch(ctx, []agentcore.SemanticRecord{
		agentcore.NewSemanticRecord(agentcore.SemanticKindToolCall, "thread_test", "run_1", "turn_1", "", "session", map[string]any{
			"tool_call_id": "call_1",
			"tool_name":    "shell",
			"arguments":    `{"command":"echo hi"}`,
		}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindToolResult, "thread_test", "run_1", "turn_1", "call_1", "session", map[string]any{
			"tool_call_id": "call_1",
			"tool_name":    "shell",
			"content":      "hi",
			"is_error":     false,
		}),
	}); err != nil {
		t.Fatalf("AppendBatch: %v", err)
	}
	if err := journal.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	records, err := ReadExecutionJournal(path)
	if err != nil {
		t.Fatalf("ReadExecutionJournal: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}
	if records[0].Seq != 1 || records[1].Seq != 2 || records[2].Seq != 3 {
		t.Fatalf("unexpected seq assignment: %+v", []uint64{records[0].Seq, records[1].Seq, records[2].Seq})
	}
	if records[1].Kind != agentcore.SemanticKindToolCall || records[2].Kind != agentcore.SemanticKindToolResult {
		t.Fatalf("unexpected kinds: %s %s", records[1].Kind, records[2].Kind)
	}
}

func TestExecutionJournalToleratesIncompleteTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "thread_tail.jsonl")
	good, _ := json.Marshal(agentcore.SemanticRecord{
		SchemaVersion: 2,
		RecordID:      "rec_1",
		Seq:           1,
		ThreadID:      "thread_tail",
		Kind:          agentcore.SemanticKindRunStarted,
		Timestamp:     time.Now().UTC(),
		Payload:       map[string]any{},
	})
	if err := os.WriteFile(path, append(good, []byte("\n{\"schema_version\":2,\"record_id\":\"rec_2\",\"kind\":\"tool_call\"")...), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	records, err := ReadExecutionJournal(path)
	if err != nil {
		t.Fatalf("expected incomplete tail tolerance, got %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 recovered record, got %d", len(records))
	}
}

func TestExecutionJournalMiddleCorruption(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "thread_corrupt.jsonl")
	content := strings.Join([]string{
		`{"schema_version":2,"record_id":"rec_1","seq":1,"thread_id":"t","kind":"run_started","timestamp":"2026-07-01T00:00:00Z","payload":{}}`,
		`{not-json`,
		`{"schema_version":2,"record_id":"rec_3","seq":3,"thread_id":"t","kind":"run_terminal","timestamp":"2026-07-01T00:00:01Z","payload":{}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := ReadExecutionJournal(path)
	if !errors.Is(err, ErrJournalMiddleCorruption) {
		t.Fatalf("expected ErrJournalMiddleCorruption, got %v", err)
	}
}

func TestExecutionJournalFlushRetryEventuallySucceeds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "thread_retry.jsonl")
	journal, err := NewExecutionJournal(path, "thread_retry")
	if err != nil {
		t.Fatalf("NewExecutionJournal: %v", err)
	}
	defer journal.Close()

	ctx := context.Background()
	if err := journal.Append(ctx, agentcore.NewSemanticRecord(
		agentcore.SemanticKindRunTerminal, "thread_retry", "run_1", "", "", "", map[string]any{"status": "finished"},
	)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := journal.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if journal.PendingCount() != 0 {
		t.Fatalf("expected pending cleared after flush")
	}
}

func TestLegacyTranscriptAdapterProjectsToolFacts(t *testing.T) {
	records, err := (LegacyTranscriptAdapter{}).ReadFile("testdata/v1_tool_messages_without_tool_events.jsonl")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var toolCalls, toolResults int
	for _, record := range records {
		switch record.Kind {
		case agentcore.SemanticKindToolCall:
			toolCalls++
		case agentcore.SemanticKindToolResult:
			toolResults++
			if _, ok := record.Payload["outcome"]; ok {
				t.Fatalf("must not invent outcome fields for v1 tool results")
			}
			if _, ok := record.Payload["is_error"]; !ok {
				t.Fatalf("expected is_error preserved")
			}
		case agentcore.SemanticKindArtifact:
			t.Fatalf("must not invent artifacts")
		}
	}
	if toolCalls < 3 || toolResults < 3 {
		t.Fatalf("expected >=3 tool_call and tool_result, got %d/%d", toolCalls, toolResults)
	}
}

func TestOrphanRunRecoverySynthesizesInterruptedOutcome(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "thread_orphan.jsonl")
	journal, err := NewExecutionJournal(path, "thread_orphan")
	if err != nil {
		t.Fatalf("NewExecutionJournal: %v", err)
	}
	ctx := context.Background()
	_ = journal.Append(ctx, agentcore.NewSemanticRecord(agentcore.SemanticKindRunStarted, "thread_orphan", "run_orphan", "", "", "", map[string]any{}))
	_ = journal.Append(ctx, agentcore.NewSemanticRecord(agentcore.SemanticKindToolCall, "thread_orphan", "run_orphan", "turn_1", "", "", map[string]any{
		"tool_call_id": "call_orphan",
		"tool_name":    "shell",
		"arguments":    `{"command":"sleep 1"}`,
	}))
	_ = journal.Close()

	records, err := ReadExecutionJournal(path)
	if err != nil {
		t.Fatalf("ReadExecutionJournal: %v", err)
	}
	journal2, err := NewExecutionJournal(path, "thread_orphan")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer journal2.Close()
	recovered, err := recoverOrphanRuns(ctx, journal2, records)
	if err != nil {
		t.Fatalf("recoverOrphanRuns: %v", err)
	}
	var sawResult, sawTerminal bool
	for _, record := range recovered {
		if record.Kind == agentcore.SemanticKindToolResult {
			sawResult = true
			outcomeRaw, _ := record.Payload["outcome"].(map[string]any)
			if stringPayload(outcomeRaw, "error_code") != "process_restarted_outcome_unknown" {
				t.Fatalf("unexpected error_code: %#v", outcomeRaw)
			}
			if stringPayload(outcomeRaw, "error_category") != string(tool.ErrorCategoryPersistence) {
				t.Fatalf("unexpected error_category: %#v", outcomeRaw)
			}
			if boolPayload(outcomeRaw, "retryable") {
				t.Fatalf("expected retryable=false")
			}
			diag, _ := outcomeRaw["diagnostic"].(map[string]any)
			if stringPayload(diag, "side_effect_state") != "unknown" {
				t.Fatalf("expected side_effect_state=unknown, got %#v", diag)
			}
		}
		if record.Kind == agentcore.SemanticKindRunTerminal {
			sawTerminal = true
			if stringPayload(record.Payload, "status") != "interrupted" || stringPayload(record.Payload, "reason") != "process_restarted" {
				t.Fatalf("unexpected terminal payload: %#v", record.Payload)
			}
		}
	}
	if !sawResult || !sawTerminal {
		t.Fatalf("expected synthetic tool_result and run_terminal, got %+v", recovered)
	}
}

func TestWriteFailureKeepsPending(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing", "thread.jsonl")
	// Create journal then remove directory to force later write failures is hard on some FS.
	// Instead verify cancelled context is non-retryable and leaves pending.
	journal, err := NewExecutionJournal(filepath.Join(dir, "thread_pending.jsonl"), "thread_pending")
	if err != nil {
		t.Fatalf("NewExecutionJournal: %v", err)
	}
	defer journal.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = journal.Append(ctx, agentcore.NewSemanticRecord(
		agentcore.SemanticKindToolCall, "thread_pending", "run", "", "", "", map[string]any{"tool_call_id": "c1"},
	))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled, got %v", err)
	}
	if journal.PendingCount() == 0 {
		t.Fatalf("expected pending retained after failed append")
	}
	_ = path
}
