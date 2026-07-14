package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/protocol/agui"
	"awaken/src/tool"
)

func TestA34PartialJSONLTailIsIgnoredThroughThreadStoreWithoutLosingPriorFacts(t *testing.T) {
	store, snapshot, path, records := prepareSemanticBoundaryFixture(t, "a34_partial_tail")
	writeSemanticBoundaryJSONL(t, path, records, []byte(`{"schema_version":2,"record_id":"partial`))

	transcript, err := store.GetTranscript(snapshot.ThreadID)
	if err != nil {
		t.Fatalf("A34 ThreadStore must tolerate only an incomplete final line: %v", err)
	}
	assertA34RecoveredTranscript(t, transcript, records)

	service := NewRunService(nil, nil)
	service.threadStore = store
	fromService, err := service.GetThreadTranscript(snapshot.ThreadID)
	if err != nil {
		t.Fatalf("A34 RunService propagation lost tail tolerance: %v", err)
	}
	assertA34RecoveredTranscript(t, fromService, records)
}

func TestA35MiddleJSONLCorruptionPropagatesThroughThreadStoreAndRunService(t *testing.T) {
	store, snapshot, path, records := prepareSemanticBoundaryFixture(t, "a35_middle_corruption")
	lines := make([][]byte, 0, len(records)+1)
	for index, record := range records {
		raw, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("marshal fixture record: %v", err)
		}
		lines = append(lines, raw)
		if index == 1 {
			lines = append(lines, []byte(`{not-json`))
		}
	}
	if err := os.WriteFile(path, append([]byte(strings.Join(byteLinesToStrings(lines), "\n")), '\n'), 0o644); err != nil {
		t.Fatalf("write middle-corrupt fixture: %v", err)
	}

	if _, err := store.GetTranscript(snapshot.ThreadID); !errors.Is(err, ErrJournalMiddleCorruption) {
		t.Fatalf("A35 ThreadStore swallowed/reclassified middle corruption: %v", err)
	}
	service := NewRunService(nil, nil)
	service.threadStore = store
	if _, err := service.GetThreadTranscript(snapshot.ThreadID); !errors.Is(err, ErrJournalMiddleCorruption) {
		t.Fatalf("A35 RunService swallowed/reclassified middle corruption: %v", err)
	}
}

func TestA37OrphanRecoveryIsInterruptedPairedOutcomeUnknownAndNeverReexecutes(t *testing.T) {
	setRuntimeUserDataDir(t)
	workspace := t.TempDir()
	var sideEffectExecutions atomic.Int32
	firstService := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				Workspace: agentcore.WorkspaceConfig{Root: workspace.WorkspaceRoot},
			}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, _ agentcore.EventSink) (string, error) {
				threadID, runID, sessionID := agent.ExecutionIdentity()
				records := []agentcore.SemanticRecord{
					agentcore.NewSemanticRecord(agentcore.SemanticKindRunStarted, threadID, runID, "turn_a37", "", sessionID, nil),
					agentcore.NewSemanticRecord(agentcore.SemanticKindUserMessage, threadID, runID, "turn_a37", "", sessionID, map[string]any{"content": input}),
					agentcore.NewSemanticRecord(agentcore.SemanticKindToolCall, threadID, runID, "turn_a37", "msg_a37", sessionID, map[string]any{
						"tool_call_id": "call_a37",
						"tool_name":    "external_side_effect",
						"arguments":    `{"operation":"send_once"}`,
					}),
				}
				if err := agent.SemanticRecorder().AppendBatch(ctx, records); err != nil {
					return "", err
				}
				if err := agent.SemanticRecorder().Flush(ctx); err != nil {
					return "", err
				}
				// Simulate a side effect that occurred immediately before process death.
				sideEffectExecutions.Add(1)
				return "in-memory only", nil
			})
		},
	)
	run, err := firstService.StartRun(StartRunRequest{
		Input:     "execute once then crash",
		Workspace: WorkspaceConfig{WorkspaceRoot: workspace},
	})
	if err != nil {
		t.Fatalf("A37 StartRun: %v", err)
	}
	waitForRunTerminal(t, firstService, run.RunID)
	if got := sideEffectExecutions.Load(); got != 1 {
		t.Fatalf("A37 fixture did not simulate exactly one execution: %d", got)
	}

	restarted := NewRunService(nil, func(*agentcore.Agent) Executor {
		return ExecutorFunc(func(context.Context, string, agentcore.EventSink) (string, error) {
			sideEffectExecutions.Add(1)
			return "must not execute during recovery", nil
		})
	})
	transcript, err := restarted.GetThreadTranscript(run.ThreadID)
	if err != nil {
		t.Fatalf("A37 recovery: %v", err)
	}
	if got := sideEffectExecutions.Load(); got != 1 {
		t.Fatalf("A37 recovery re-executed an unknown side effect: %d", got)
	}
	if len(transcript.Runs) != 1 {
		t.Fatalf("A37 expected one recovered run: %+v", transcript.Runs)
	}
	recovered := transcript.Runs[0]
	if recovered.Run.Status != RunStatusError || recovered.Run.Error != "process restarted" {
		t.Fatalf("A37 orphan not surfaced as interrupted error: %+v", recovered.Run)
	}

	var resultRecord, terminalRecord *agentcore.SemanticRecord
	var resultCount, terminalCount int
	for index := range recovered.SemanticRecords {
		record := &recovered.SemanticRecords[index]
		switch record.Kind {
		case agentcore.SemanticKindToolResult:
			resultRecord = record
			resultCount++
		case agentcore.SemanticKindRunTerminal:
			terminalRecord = record
			terminalCount++
		}
	}
	if resultCount != 1 || terminalCount != 1 || resultRecord == nil || terminalRecord == nil {
		t.Fatalf("A37 recovery facts are not exactly-once: results=%d terminals=%d records=%+v", resultCount, terminalCount, recovered.SemanticRecords)
	}
	outcome, ok := resultRecord.Payload["outcome"].(map[string]any)
	if !ok || stringPayload(outcome, "status") != string(tool.OutcomeInterrupted) ||
		stringPayload(outcome, "error_code") != "process_restarted_outcome_unknown" ||
		stringPayload(outcome, "error_category") != string(tool.ErrorCategoryPersistence) ||
		boolPayload(outcome, "retryable") {
		t.Fatalf("A37 synthetic outcome is not typed/non-retryable: %#v", outcome)
	}
	diagnostic, _ := outcome["diagnostic"].(map[string]any)
	if stringPayload(diagnostic, "side_effect_state") != "unknown" {
		t.Fatalf("A37 side_effect_state must remain unknown: %#v", diagnostic)
	}
	if stringPayload(terminalRecord.Payload, "status") != "interrupted" ||
		stringPayload(terminalRecord.Payload, "reason") != "process_restarted" {
		t.Fatalf("A37 terminal payload drifted: %#v", terminalRecord.Payload)
	}
	if err := convcontext.ValidateToolResultPairing(messagesFromSemanticRecords(recovered.SemanticRecords)); err != nil {
		t.Fatalf("A37 recovered model history is unpaired: %v", err)
	}

	frames := agui.ProjectSemanticRecords(recovered.SemanticRecords)
	var resultFrame, terminalFrame *agui.Event
	for index := range frames {
		frame := &frames[index]
		switch frame.Type {
		case agui.EventToolCallResult:
			resultFrame = frame
		case agui.EventRunError:
			terminalFrame = frame
		}
	}
	if resultFrame == nil || resultFrame.ToolStatus != string(tool.OutcomeInterrupted) ||
		resultFrame.ErrorCode != "process_restarted_outcome_unknown" || resultFrame.Retryable {
		t.Fatalf("A37 Desktop tool result projection drifted: %+v", resultFrame)
	}
	if terminalFrame == nil || terminalFrame.Code != "PROCESS_RESTARTED" || terminalFrame.TerminalReason != "process_restarted" {
		t.Fatalf("A37 Desktop interrupted terminal projection drifted: %+v", terminalFrame)
	}

	again, err := restarted.GetThreadTranscript(run.ThreadID)
	if err != nil {
		t.Fatalf("A37 idempotent recovery read: %v", err)
	}
	if got := sideEffectExecutions.Load(); got != 1 {
		t.Fatalf("A37 repeated history read re-executed side effect: %d", got)
	}
	var againResults int
	for _, record := range again.Runs[0].SemanticRecords {
		if record.Kind == agentcore.SemanticKindToolResult {
			againResults++
		}
	}
	if againResults != 1 {
		t.Fatalf("A37 repeated recovery duplicated synthetic result: %d", againResults)
	}
}

func prepareSemanticBoundaryFixture(t *testing.T, suffix string) (ThreadRepository, ThreadSnapshot, string, []agentcore.SemanticRecord) {
	t.Helper()
	t.Setenv(awakenHomeEnv, t.TempDir())
	store, ok := newThreadStore().(ThreadRepository)
	if !ok {
		t.Fatal("expected ThreadRepository store")
	}

	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	threadID := "thread_" + suffix
	runID := "run_" + suffix
	snapshot := ThreadSnapshot{
		ThreadID: threadID,
		Label:    suffix,
		RunID:    runID,
		Workspace: WorkspaceConfig{
			WorkspaceRoot: t.TempDir(),
		},
		CreatedAt: now,
		UpdatedAt: now.Add(4 * time.Second),
	}
	if err := store.CreateThread(snapshot, "", suffix); err != nil {
		t.Fatalf("create fixture thread metadata: %v", err)
	}
	path := store.ThreadTranscriptPath(threadID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture transcript dir: %v", err)
	}
	records := []agentcore.SemanticRecord{
		{SchemaVersion: 2, RecordID: "rec_run", Seq: 1, ThreadID: threadID, RunID: runID, Kind: agentcore.SemanticKindRunStarted, Timestamp: now, Payload: map[string]any{}},
		{SchemaVersion: 2, RecordID: "rec_user", Seq: 2, ThreadID: threadID, RunID: runID, Kind: agentcore.SemanticKindUserMessage, Timestamp: now.Add(time.Second), Payload: map[string]any{"content": "keep user"}},
		{SchemaVersion: 2, RecordID: "rec_answer", Seq: 3, ThreadID: threadID, RunID: runID, Kind: agentcore.SemanticKindAssistantMessage, Timestamp: now.Add(2 * time.Second), Payload: map[string]any{"content": "keep assistant", "message_id": "msg_keep"}},
		{SchemaVersion: 2, RecordID: "rec_terminal", Seq: 4, ThreadID: threadID, RunID: runID, Kind: agentcore.SemanticKindRunTerminal, Timestamp: now.Add(3 * time.Second), Payload: map[string]any{"status": "finished", "result": "keep assistant", "terminal_reason": "completed"}},
	}
	return store, snapshot, path, records
}

func writeSemanticBoundaryJSONL(t *testing.T, path string, records []agentcore.SemanticRecord, tail []byte) {
	t.Helper()
	var content []byte
	for _, record := range records {
		raw, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("marshal fixture record: %v", err)
		}
		content = append(content, raw...)
		content = append(content, '\n')
	}
	content = append(content, tail...)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write semantic fixture: %v", err)
	}
}

func assertA34RecoveredTranscript(t *testing.T, transcript ThreadTranscript, source []agentcore.SemanticRecord) {
	t.Helper()
	if len(transcript.Runs) != 1 {
		t.Fatalf("A34 expected one recovered run, got %+v", transcript.Runs)
	}
	run := transcript.Runs[0]
	if len(run.SemanticRecords) != len(source) {
		t.Fatalf("A34 partial tail changed prior fact count: got=%d want=%d records=%+v", len(run.SemanticRecords), len(source), run.SemanticRecords)
	}
	for index := range source {
		if run.SemanticRecords[index].RecordID != source[index].RecordID || run.SemanticRecords[index].Seq != source[index].Seq {
			t.Fatalf("A34 prior fact changed at %d: got=%+v want=%+v", index, run.SemanticRecords[index], source[index])
		}
	}
	if run.Run.Status != RunStatusFinished || run.Run.Result != "keep assistant" {
		t.Fatalf("A34 terminal/result not restored: %+v", run.Run)
	}
}

func byteLinesToStrings(lines [][]byte) []string {
	result := make([]string, len(lines))
	for index, line := range lines {
		result[index] = string(line)
	}
	return result
}
