package runtime

import (
	"strings"
	"testing"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
)

func TestTranscriptStoreAppendsMessagesEventsIdempotentlyAndLoads(t *testing.T) {
	workspace := WorkspaceConfig{WorkspaceRoot: t.TempDir()}
	run := Run{RunID: "run_1", ThreadID: "thread_1"}
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "hello"},
		{
			Role:    convcontext.RoleAssistant,
			Content: "I will read the file.",
			ToolCalls: []convcontext.ToolCall{{
				ID:   "tool_1",
				Type: "function",
				Function: convcontext.FunctionCall{
					Name:      "file_read",
					Arguments: `{"path":"README.md"}`,
				},
			}},
		},
		{Role: convcontext.RoleTool, ToolCallID: "tool_1", Name: "file_read", Content: "# README"},
		{Role: convcontext.RoleAssistant, Content: "done", CompactBoundaryID: "compact_1"},
	}

	if err := appendUserTranscriptRecord(workspace, run, "session_1", "hello"); err != nil {
		t.Fatalf("appendUserTranscriptRecord returned error: %v", err)
	}
	if err := appendUserTranscriptRecord(workspace, run, "session_1", "hello"); err != nil {
		t.Fatalf("second appendUserTranscriptRecord returned error: %v", err)
	}
	if err := appendConversationTranscript(workspace, run, messages); err != nil {
		t.Fatalf("appendConversationTranscript returned error: %v", err)
	}
	if err := appendConversationTranscript(workspace, run, messages); err != nil {
		t.Fatalf("second appendConversationTranscript returned error: %v", err)
	}

	now := time.Now().UTC()
	events := []Event{
		{
			Kind:      EventKindAgent,
			Seq:       1,
			Timestamp: now,
			AgentEvent: agentcore.AwakenEvent{
				Type:                        agentcore.ContextEstimatedEventType,
				ContextModelWindow:          128000,
				ContextOutputReserve:        16000,
				ContextEffectiveWindow:      112000,
				ContextEstimatedTokens:      120,
				ContextWarningThreshold:     95000,
				ContextAutoCompactThreshold: 100000,
				ContextCompactTarget:        33000,
				ContextEstimatedDeltaTokens: 4,
			},
		},
		{
			Kind:      EventKindAgent,
			Seq:       2,
			Timestamp: now.Add(time.Millisecond),
			AgentEvent: agentcore.AwakenEvent{
				Type:                   agentcore.CompactFinishedEventType,
				Content:                "compact finished: auto boundary=compact_1 estimated_tokens=80",
				ContextEstimatedTokens: 80,
				ContextCompactTarget:   33000,
			},
		},
		{
			Kind:      EventKindAgent,
			Seq:       3,
			Timestamp: now.Add(2 * time.Millisecond),
			AgentEvent: agentcore.AwakenEvent{
				Type:           agentcore.RunFinishedEventType,
				Content:        "done",
				TerminalReason: agentcore.TerminalReasonCompleted,
			},
		},
	}
	if err := appendRunEventTranscript(workspace, run, events); err != nil {
		t.Fatalf("appendRunEventTranscript returned error: %v", err)
	}
	if err := appendRunEventTranscript(workspace, run, events); err != nil {
		t.Fatalf("second appendRunEventTranscript returned error: %v", err)
	}

	records, err := readTranscriptRecords(transcriptPath(workspace.WorkspaceRoot, run.ThreadID))
	if err != nil {
		t.Fatalf("readTranscriptRecords returned error: %v", err)
	}
	if got := transcriptMessageCount(records); got != len(messages) {
		t.Fatalf("expected %d message records, got %d: %+v", len(messages), got, records)
	}
	var eventCount int
	for _, record := range records {
		if record.RecordType == transcriptRecordTypeEvent {
			eventCount++
		}
	}
	if eventCount != len(events) {
		t.Fatalf("expected %d event records, got %d: %+v", len(events), eventCount, records)
	}
	if records[0].SessionID != "session_1" || records[0].ParentMessageID != "" {
		t.Fatalf("unexpected first transcript record: %+v", records[0])
	}
	if records[1].ParentMessageID != records[0].MessageID || len(records[1].ToolCalls) != 1 {
		t.Fatalf("unexpected assistant transcript record: %+v", records[1])
	}
	if records[2].ToolCallID != "tool_1" || records[2].ToolName != "file_read" {
		t.Fatalf("unexpected tool transcript record: %+v", records[2])
	}

	restored, err := loadRestoredTranscript(workspace, run.ThreadID)
	if err != nil {
		t.Fatalf("loadRestoredTranscript returned error: %v", err)
	}
	if len(restored.Messages) != len(messages) {
		t.Fatalf("expected %d restored messages, got %+v", len(messages), restored.Messages)
	}
	if restored.Messages[3].CompactBoundaryID != "compact_1" {
		t.Fatalf("expected compact boundary to restore, got %+v", restored.Messages[3])
	}
	if restored.LastContextWindowState == nil || restored.LastContextWindowState.LastCompactBoundaryID != "compact_1" {
		t.Fatalf("expected restored context window compact boundary, got %+v", restored.LastContextWindowState)
	}
	if restored.TerminalReason != agentcore.TerminalReasonCompleted ||
		restored.TerminalEvent == nil ||
		restored.TerminalEvent.Type != agentcore.RunFinishedEventType {
		t.Fatalf("expected restored terminal event, got reason=%q event=%+v", restored.TerminalReason, restored.TerminalEvent)
	}

	durableRuns, err := loadDurableTranscriptRuns(workspace, run.ThreadID)
	if err != nil {
		t.Fatalf("loadDurableTranscriptRuns returned error: %v", err)
	}
	durable := durableRuns[run.RunID]
	if durable.Run.Status != RunStatusFinished || durable.Run.Result != "done" {
		t.Fatalf("expected finished durable run, got %+v", durable.Run)
	}
	if len(durable.Events) != len(events) {
		t.Fatalf("expected %d durable events, got %+v", len(events), durable.Events)
	}
}

func TestTranscriptLoaderRejectsInvalidToolPairing(t *testing.T) {
	workspace := WorkspaceConfig{WorkspaceRoot: t.TempDir()}
	threadID := "thread_bad"
	record := transcriptRecordForMessage(threadID, "run_bad", "", 0, convcontext.Message{
		Role: convcontext.RoleAssistant,
		ToolCalls: []convcontext.ToolCall{{
			ID:   "tool_missing",
			Type: "function",
			Function: convcontext.FunctionCall{
				Name:      "file_read",
				Arguments: "{}",
			},
		}},
	}, time.Now().UTC())
	if err := appendTranscriptRecords(transcriptPath(workspace.WorkspaceRoot, threadID), []TranscriptRecord{record}); err != nil {
		t.Fatalf("appendTranscriptRecords returned error: %v", err)
	}

	_, err := loadRestoredTranscript(workspace, threadID)
	if err == nil || !strings.Contains(err.Error(), "invalid transcript tool result pairing") {
		t.Fatalf("expected invalid pairing error, got %v", err)
	}
}
