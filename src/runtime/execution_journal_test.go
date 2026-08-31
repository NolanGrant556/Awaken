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
	convcontext "awaken/src/context"
	"awaken/src/filestore"
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

func TestExecutionJournalAppendFlushedKeepsOldFileAndSequenceOnAtomicWriteFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thread_transactional_compact.jsonl")
	journal, err := NewExecutionJournal(path, "thread_transactional_compact")
	if err != nil {
		t.Fatalf("NewExecutionJournal: %v", err)
	}
	defer journal.Close()
	if err := journal.Append(context.Background(), agentcore.NewSemanticRecord(
		agentcore.SemanticKindUserMessage, "thread_transactional_compact", "run_1", "turn_1", "", "", map[string]any{"content": "known good"},
	)); err != nil {
		t.Fatalf("append baseline: %v", err)
	}
	if err := journal.Flush(context.Background()); err != nil {
		t.Fatalf("flush baseline: %v", err)
	}
	beforeSeq := journal.NextSeq()
	journal.atomicWriteHook = func(string, []byte) error { return errors.New("injected atomic write failure") }
	_, err = journal.AppendFlushed(context.Background(), agentcore.NewSemanticRecord(
		agentcore.SemanticKindContextCompactedV2, "thread_transactional_compact", "run_1", "", "", "", map[string]any{"bundle_v2": map[string]any{"invalid": "not relevant"}},
	))
	if err == nil {
		t.Fatal("expected transactional atomic write failure")
	}
	journal.atomicWriteHook = nil
	if journal.NextSeq() != beforeSeq || journal.PendingCount() != 0 {
		t.Fatalf("transaction state was not restored: next=%d pending=%d", journal.NextSeq(), journal.PendingCount())
	}
	records, readErr := ReadExecutionJournal(path)
	if readErr != nil {
		t.Fatalf("read rolled-back journal: %v", readErr)
	}
	if len(records) != 1 || records[0].Kind != agentcore.SemanticKindUserMessage {
		t.Fatalf("failed compact leaked into durable journal: %+v", records)
	}
	if err := journal.Append(context.Background(), agentcore.NewSemanticRecord(
		agentcore.SemanticKindAssistantMessage, "thread_transactional_compact", "run_1", "turn_1", "", "", map[string]any{"content": "continued"},
	)); err != nil {
		t.Fatalf("append after rollback: %v", err)
	}
	if err := journal.Flush(context.Background()); err != nil {
		t.Fatalf("flush after rollback: %v", err)
	}
	records, readErr = ReadExecutionJournal(path)
	if readErr != nil || len(records) != 2 || records[1].Kind != agentcore.SemanticKindAssistantMessage {
		t.Fatalf("journal did not continue from rolled-back sequence: records=%+v err=%v", records, readErr)
	}
}

func TestExecutionJournalAppendFlushedTreatsPostRenameErrorAsCommitted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thread_post_rename_compact.jsonl")
	journal, err := NewExecutionJournal(path, "thread_post_rename_compact")
	if err != nil {
		t.Fatalf("NewExecutionJournal: %v", err)
	}
	defer journal.Close()
	journal.atomicWriteHook = func(path string, data []byte) error {
		if err := filestore.WriteFileAtomic(path, data); err != nil {
			return err
		}
		return errors.New("injected error after durable rename")
	}
	assigned, err := journal.AppendFlushed(context.Background(), agentcore.NewSemanticRecord(
		agentcore.SemanticKindContextCompactedV2, "thread_post_rename_compact", "run", "", "", "", map[string]any{"bundle_v2": "test"},
	))
	if err != nil || assigned.Seq != 1 {
		t.Fatalf("durably renamed record was reported as failed: assigned=%+v err=%v", assigned, err)
	}
	journal.atomicWriteHook = nil
	records, readErr := ReadExecutionJournal(path)
	if readErr != nil || len(records) != 1 || records[0].RecordID != assigned.RecordID {
		t.Fatalf("committed boundary was not restart-readable exactly once: records=%+v err=%v", records, readErr)
	}
	if appendErr := journal.Append(context.Background(), agentcore.NewSemanticRecord(
		agentcore.SemanticKindUserMessage, "thread_post_rename_compact", "run", "", "", "", map[string]any{"content": "continue"},
	)); appendErr != nil {
		t.Fatalf("journal could not continue after committed boundary: %v", appendErr)
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
	_ = journal.Append(ctx, agentcore.NewSemanticRecord(agentcore.SemanticKindModelRequest, "thread_orphan", "run_orphan", "turn_1", "", "", map[string]any{
		"request_id": "request_orphan", "purpose": "agent_turn", "model_id": "test-model", "protocol_family": "openai_chat",
	}))
	_ = journal.Append(ctx, agentcore.NewSemanticRecord(agentcore.SemanticKindHookInvocationStarted, "thread_orphan", "run_orphan", "turn_1", "", "", map[string]any{
		"invocation_id": "hookinv_orphan", "hook_id": "user:orphan", "event": "PreToolUse", "source_kind": "user", "status": "started",
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
	var sawResult, sawUsage, sawHookCompletion, sawTerminal bool
	for _, record := range recovered {
		if record.Seq == 0 {
			t.Fatalf("recovered record %s returned seq=0: %+v", record.Kind, record)
		}
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
		if record.Kind == agentcore.SemanticKindModelUsage {
			sawUsage = true
			if stringPayload(record.Payload, "request_id") != "request_orphan" ||
				stringPayload(record.Payload, "status") != "interrupted" ||
				stringPayload(record.Payload, "error_code") != "usage_unknown" {
				t.Fatalf("unexpected recovered model usage: %#v", record.Payload)
			}
			if _, exists := record.Payload["input_tokens"]; exists {
				t.Fatalf("recovery must not invent provider usage: %#v", record.Payload)
			}
		}
		if record.Kind == agentcore.SemanticKindHookInvocationCompleted {
			sawHookCompletion = true
			if stringPayload(record.Payload, "invocation_id") != "hookinv_orphan" ||
				stringPayload(record.Payload, "status") != "interrupted" ||
				stringPayload(record.Payload, "process_state") != "unknown" {
				t.Fatalf("unexpected recovered Hook receipt: %#v", record.Payload)
			}
		}
	}
	if !sawResult || !sawUsage || !sawHookCompletion || !sawTerminal {
		t.Fatalf("expected synthetic tool_result, model_usage, Hook completion, and run_terminal, got %+v", recovered)
	}
	durable, err := ReadExecutionJournal(path)
	if err != nil {
		t.Fatalf("read durable recovery: %v", err)
	}
	if len(durable) != len(recovered) {
		t.Fatalf("returned recovery and durable journal differ: %d != %d", len(recovered), len(durable))
	}
	for i := range durable {
		if durable[i].Seq != recovered[i].Seq {
			t.Fatalf("returned seq differs from durable seq at %d: %d != %d", i, recovered[i].Seq, durable[i].Seq)
		}
	}
}

func TestOrphanRecoveryPairsOpenCallEvenWhenPersistenceTerminalExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thread_persistence_terminal.jsonl")
	journal, err := NewExecutionJournal(path, "thread_persistence_terminal")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	records := []agentcore.SemanticRecord{
		agentcore.NewSemanticRecord(agentcore.SemanticKindRunStarted, "thread_persistence_terminal", "run_1", "turn_1", "", "", nil),
		agentcore.NewSemanticRecord(agentcore.SemanticKindToolCall, "thread_persistence_terminal", "run_1", "turn_1", "", "", map[string]any{
			"tool_call_id": "call_open",
			"tool_name":    "text_edit",
			"arguments":    `{"path":"note.txt","old_string":"a","new_string":"b"}`,
		}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindRunTerminal, "thread_persistence_terminal", "run_1", "turn_1", "", "", map[string]any{
			"status":          "error",
			"terminal_reason": "persistence_error",
		}),
	}
	if err := journal.AppendBatch(ctx, records); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	durable, err := ReadExecutionJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := NewExecutionJournal(path, "thread_persistence_terminal")
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := recoverOrphanRuns(ctx, reopened, durable)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	var results, terminals int
	for _, record := range recovered {
		switch record.Kind {
		case agentcore.SemanticKindToolResult:
			results++
		case agentcore.SemanticKindRunTerminal:
			terminals++
		}
	}
	if results != 1 || terminals != 1 {
		t.Fatalf("expected one synthetic result and original terminal, got results=%d terminals=%d: %+v", results, terminals, recovered)
	}
	if err := convcontext.ValidateToolResultPairing(messagesFromSemanticRecords(recovered)); err != nil {
		t.Fatalf("recovered model history remains unpaired: %v", err)
	}
	run := durableRunsFromSemanticRecords("thread_persistence_terminal", recovered)["run_1"]
	if len(run.SemanticRecords) == 0 || run.SemanticRecords[len(run.SemanticRecords)-1].Kind != agentcore.SemanticKindRunTerminal {
		t.Fatalf("terminal must project after recovered tool result: %+v", run.SemanticRecords)
	}
}

func TestOrphanRecoveryClosesRunThatCrashedBeforeRunStarted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thread_pre_runner.jsonl")
	journal, err := NewExecutionJournal(path, "thread_pre_runner")
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Append(context.Background(), agentcore.NewSemanticRecord(
		agentcore.SemanticKindUserMessage,
		"thread_pre_runner",
		"run_pre_runner",
		"",
		"",
		"",
		map[string]any{"content": "durable before runner"},
	)); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	records, err := ReadExecutionJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := NewExecutionJournal(path, "thread_pre_runner")
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := recoverOrphanRuns(context.Background(), reopened, records)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if len(recovered) != 2 || recovered[1].Kind != agentcore.SemanticKindRunTerminal ||
		stringPayload(recovered[1].Payload, "status") != "interrupted" {
		t.Fatalf("pre-runner crash remained a running ghost: %+v", recovered)
	}
}

func TestOrphanSubagentIsSealedInterruptedWithoutRerun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thread_subagent_orphan.jsonl")
	journal, err := NewExecutionJournal(path, "thread_subagent_orphan")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_ = journal.Append(ctx, agentcore.NewSemanticRecord(agentcore.SemanticKindRunStarted, "thread_subagent_orphan", "run_sa", "", "", "", map[string]any{}))
	_ = journal.Append(ctx, agentcore.NewSemanticRecord(agentcore.SemanticKindSubagentSpawned, "thread_subagent_orphan", "run_sa", "turn_1", "call_1", "", map[string]any{
		"task_id": "subtask_orphan", "agent_id": "subagent_orphan", "status": "running", "agent_type": "explore",
	}))
	_ = journal.Append(ctx, agentcore.NewSemanticRecord(agentcore.SemanticKindRunTerminal, "thread_subagent_orphan", "run_sa", "", "", "", map[string]any{
		"status": "completed",
	}))
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	records, err := ReadExecutionJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := NewExecutionJournal(path, "thread_subagent_orphan")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	recovered, err := recoverOrphanRuns(ctx, reopened, records)
	if err != nil {
		t.Fatal(err)
	}
	var terminals int
	for _, record := range recovered {
		if record.Kind == agentcore.SemanticKindSubagentTerminal {
			terminals++
			if stringPayload(record.Payload, "task_id") != "subtask_orphan" ||
				stringPayload(record.Payload, "status") != "interrupted" ||
				stringPayload(record.Payload, "error_code") != "subagent_interrupted" {
				t.Fatalf("orphan terminal = %#v", record.Payload)
			}
		}
	}
	if terminals != 1 {
		t.Fatalf("expected one interrupted subagent terminal, recovered=%+v", recovered)
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

func TestExecutionJournalRealClosedFileWriteFailureKeepsAssignedPendingRecord(t *testing.T) {
	dir := t.TempDir()
	journal, err := NewExecutionJournal(filepath.Join(dir, "thread_real_failure.jsonl"), "thread_real_failure")
	if err != nil {
		t.Fatalf("NewExecutionJournal: %v", err)
	}
	defer journal.Close()

	// Close the actual descriptor without marking the journal closed. This
	// exercises the production os.File write/retry path rather than a fake
	// recorder or a pre-cancelled context.
	if err := journal.file.Close(); err != nil {
		t.Fatalf("close underlying descriptor: %v", err)
	}
	err = journal.Append(context.Background(), agentcore.NewSemanticRecord(
		agentcore.SemanticKindToolResult,
		"thread_real_failure",
		"run_1",
		"turn_1",
		"call_1",
		"",
		map[string]any{"tool_call_id": "call_1"},
	))
	if err == nil {
		t.Fatal("expected real closed-file write failure")
	}
	if journal.PendingCount() != 1 {
		t.Fatalf("failed durable write must retain one pending record, got %d", journal.PendingCount())
	}
	if journal.NextSeq() != 2 {
		t.Fatalf("failed durable write must retain assigned sequence, next=%d", journal.NextSeq())
	}
}
