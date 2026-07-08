package runtime

import (
	"strings"
	"testing"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
)

func TestTranscriptStoreAppendsMessagesEventsIdempotentlyAndLoads(t *testing.T) {
	setRuntimeUserDataDir(t)
	t.TempDir()
	run := Run{RunID: "run_1", ThreadID: "thread_1"}
	path := transcriptPath(run.ThreadID)
	preEstimate := 120
	postEstimate := 80
	fallbackUsed := true
	toolResultsPruned := false
	sourceHadUser := true
	latestUserPinned := true
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

	if err := appendUserTranscriptRecordAtPath(path, run, "session_1", "hello", nil); err != nil {
		t.Fatalf("appendUserTranscriptRecord returned error: %v", err)
	}
	if err := appendUserTranscriptRecordAtPath(path, run, "session_1", "hello", nil); err != nil {
		t.Fatalf("second appendUserTranscriptRecord returned error: %v", err)
	}
	if err := appendConversationTranscriptAtPath(path, run, messages); err != nil {
		t.Fatalf("appendConversationTranscript returned error: %v", err)
	}
	if err := appendConversationTranscriptAtPath(path, run, messages); err != nil {
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
				Type:                             agentcore.CompactFinishedEventType,
				Content:                          "compact finished: auto boundary=compact_legacy estimated_tokens=80",
				ContextCompactTarget:             33000,
				CompactMode:                      agentcore.CompactModeAuto,
				CompactBoundaryID:                "compact_1",
				CompactPreEstimate:               &preEstimate,
				CompactPostEstimate:              &postEstimate,
				CompactSummaryModel:              "gpt-5-mini",
				CompactFallbackUsed:              &fallbackUsed,
				CompactToolResultsPruned:         &toolResultsPruned,
				CompactToolResultsPrunedCount:    2,
				CompactPrunedSafeGroupCount:      1,
				CompactPrunedMessageCount:        3,
				CompactSummaryMaxTokens:          8192,
				CompactSummaryRetryCount:         1,
				CompactSummaryRawChars:           321,
				CompactSummaryParsedSections:     13,
				CompactSummaryMeaningfulSections: 4,
				CompactSummaryQualityIssue:       "semantic compact summary contained no meaningful section content despite non-empty source history",
				CompactSummarySourceHadUser:      &sourceHadUser,
				CompactSummaryLatestUserPinned:   &latestUserPinned,
				CompactFallbackMarker:            "[COMPACT_FALLBACK:llm_summary_failed]",
			},
		},
		{
			Kind:      EventKindAgent,
			Seq:       3,
			Timestamp: now.Add(2 * time.Millisecond),
			AgentEvent: agentcore.AwakenEvent{
				Type:                        agentcore.RunFinishedEventType,
				Content:                     "done",
				TerminalReason:              agentcore.TerminalReasonCompleted,
				ContextModelWindow:          128000,
				ContextOutputReserve:        16000,
				ContextEffectiveWindow:      112000,
				ContextEstimatedTokens:      96,
				ContextActualTokens:         84,
				ContextWarningThreshold:     95000,
				ContextAutoCompactThreshold: 100000,
				ContextCompactTarget:        33000,
			},
		},
	}
	if err := appendRunEventTranscriptAtPath(path, run, events); err != nil {
		t.Fatalf("appendRunEventTranscript returned error: %v", err)
	}
	if err := appendRunEventTranscriptAtPath(path, run, events); err != nil {
		t.Fatalf("second appendRunEventTranscript returned error: %v", err)
	}

	records, err := readTranscriptRecords(path)
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

	restored, err := loadRestoredTranscriptAtPath(path)
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
	if restored.LastContextWindowState.EstimatedTokens != 96 {
		t.Fatalf("expected restored terminal context estimate to win, got %+v", restored.LastContextWindowState)
	}
	if restored.LastContextWindowState.LastAPIUsageTokens != 84 {
		t.Fatalf("expected restored api usage tokens from terminal event, got %+v", restored.LastContextWindowState)
	}
	if restored.LastContextWindowState.LastCompactBoundaryID != "compact_1" {
		t.Fatalf("expected compact boundary to survive terminal context merge, got %+v", restored.LastContextWindowState)
	}
	if restored.TerminalReason != agentcore.TerminalReasonCompleted ||
		restored.TerminalEvent == nil ||
		restored.TerminalEvent.Type != agentcore.RunFinishedEventType {
		t.Fatalf("expected restored terminal event, got reason=%q event=%+v", restored.TerminalReason, restored.TerminalEvent)
	}

	durableRuns, err := loadDurableTranscriptRunsAtPath(path, run.ThreadID)
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
	gotCompact := durable.Events[1].AgentEvent
	if gotCompact.CompactBoundaryID != "compact_1" ||
		gotCompact.CompactSummaryModel != "gpt-5-mini" ||
		gotCompact.CompactPreEstimate == nil ||
		*gotCompact.CompactPreEstimate != 120 ||
		gotCompact.CompactPostEstimate == nil ||
		*gotCompact.CompactPostEstimate != 80 ||
		gotCompact.CompactFallbackUsed == nil ||
		!*gotCompact.CompactFallbackUsed ||
		gotCompact.CompactToolResultsPruned == nil ||
		*gotCompact.CompactToolResultsPruned ||
		gotCompact.CompactToolResultsPrunedCount != 2 ||
		gotCompact.CompactPrunedSafeGroupCount != 1 ||
		gotCompact.CompactPrunedMessageCount != 3 ||
		gotCompact.CompactSummaryMaxTokens != 8192 ||
		gotCompact.CompactSummaryRetryCount != 1 ||
		gotCompact.CompactSummaryRawChars != 321 ||
		gotCompact.CompactSummaryParsedSections != 13 ||
		gotCompact.CompactSummaryMeaningfulSections != 4 ||
		gotCompact.CompactSummaryQualityIssue != "semantic compact summary contained no meaningful section content despite non-empty source history" ||
		gotCompact.CompactSummarySourceHadUser == nil ||
		!*gotCompact.CompactSummarySourceHadUser ||
		gotCompact.CompactSummaryLatestUserPinned == nil ||
		!*gotCompact.CompactSummaryLatestUserPinned ||
		gotCompact.CompactFallbackMarker != "[COMPACT_FALLBACK:llm_summary_failed]" {
		t.Fatalf("expected durable compact metadata to round-trip, got %+v", gotCompact)
	}
}

func TestTranscriptLoaderRejectsInvalidToolPairing(t *testing.T) {
	setRuntimeUserDataDir(t)
	t.TempDir()
	threadID := "thread_bad"
	path := transcriptPath(threadID)
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
	if err := appendTranscriptRecords(path, []TranscriptRecord{record}); err != nil {
		t.Fatalf("appendTranscriptRecords returned error: %v", err)
	}

	_, err := loadRestoredTranscriptAtPath(path)
	if err == nil || !strings.Contains(err.Error(), "invalid transcript tool result pairing") {
		t.Fatalf("expected invalid pairing error, got %v", err)
	}
}

func TestTranscriptStorePreservesRawHistoryAndRestoresCompactRuntimeView(t *testing.T) {
	setRuntimeUserDataDir(t)
	t.TempDir()
	run := Run{RunID: "run_compact", ThreadID: "thread_compact"}
	path := transcriptPath(run.ThreadID)

	rawMessages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "first"},
		{Role: convcontext.RoleAssistant, Content: "alpha"},
		{Role: convcontext.RoleUser, Content: "second"},
		{Role: convcontext.RoleAssistant, Content: "beta"},
	}
	if err := appendConversationTranscriptAtPath(path, run, rawMessages); err != nil {
		t.Fatalf("appendConversationTranscript raw returned error: %v", err)
	}

	compactedMessages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "summary", CompactBoundaryID: "compact_2"},
		{Role: convcontext.RoleUser, Content: "second"},
		{Role: convcontext.RoleAssistant, Content: "beta"},
	}
	if err := appendConversationTranscriptAtPath(path, run, compactedMessages); err != nil {
		t.Fatalf("appendConversationTranscript compacted returned error: %v", err)
	}
	if err := appendConversationTranscriptAtPath(path, run, compactedMessages); err != nil {
		t.Fatalf("second appendConversationTranscript compacted returned error: %v", err)
	}

	records, err := readTranscriptRecords(path)
	if err != nil {
		t.Fatalf("readTranscriptRecords returned error: %v", err)
	}
	if got := transcriptMessageCount(records); got != 7 {
		t.Fatalf("expected raw history plus compacted runtime view to total 7 messages, got %d", got)
	}

	restored, err := loadRestoredTranscriptAtPath(path)
	if err != nil {
		t.Fatalf("loadRestoredTranscript returned error: %v", err)
	}
	if len(restored.Messages) != 7 {
		t.Fatalf("expected restored raw history to keep 7 messages, got %+v", restored.Messages)
	}

	ctx := convcontext.New()
	ctx.SetMessages(restored.Messages)
	modelMessages := ctx.GetMessagesForModel()
	if len(modelMessages) != len(compactedMessages) {
		t.Fatalf("expected model view to restore compacted runtime slice, got %+v", modelMessages)
	}
	for i := range compactedMessages {
		if modelMessages[i].Role != compactedMessages[i].Role ||
			modelMessages[i].Content != compactedMessages[i].Content ||
			modelMessages[i].CompactBoundaryID != compactedMessages[i].CompactBoundaryID {
			t.Fatalf("expected compacted runtime view at %d, got %+v want %+v", i, modelMessages[i], compactedMessages[i])
		}
	}
}

func TestTranscriptLoaderRecoversLegacyCompactBoundaryAndReactiveMode(t *testing.T) {
	setRuntimeUserDataDir(t)
	t.TempDir()
	threadID := "thread_legacy"
	path := transcriptPath(threadID)
	record := TranscriptRecord{
		RecordType: transcriptRecordTypeEvent,
		ThreadID:   threadID,
		RunID:      "run_legacy",
		MessageID:  transcriptEventID(threadID, "run_legacy", 1),
		Event: &agentcore.AwakenEvent{
			Type:                   agentcore.CompactFailedEventType,
			Content:                "reactive compact failed boundary=legacy_boundary",
			CompactFailureReason:   "provider_timeout",
			CompactPreEstimate:     intPtr(210),
			ContextCompactTarget:   33000,
			ContextEffectiveWindow: 112000,
		},
		Timestamp: time.Now().UTC(),
	}
	if err := appendTranscriptRecords(path, []TranscriptRecord{record}); err != nil {
		t.Fatalf("appendTranscriptRecords returned error: %v", err)
	}

	restored, err := loadRestoredTranscriptAtPath(path)
	if err != nil {
		t.Fatalf("loadRestoredTranscript returned error: %v", err)
	}
	if restored.LastContextWindowState == nil {
		t.Fatalf("expected context window state to restore")
	}
	if restored.LastContextWindowState.LastCompactBoundaryID != "legacy_boundary" {
		t.Fatalf("expected legacy boundary fallback to restore, got %+v", restored.LastContextWindowState)
	}
	if !restored.LastContextWindowState.HasAttemptedReactiveCompact {
		t.Fatalf("expected reactive compact fallback to restore, got %+v", restored.LastContextWindowState)
	}
	if restored.LastContextWindowState.EstimatedTokens != 210 {
		t.Fatalf("expected legacy pre estimate to restore, got %+v", restored.LastContextWindowState)
	}
}

func intPtr(value int) *int {
	return &value
}

func TestTranscriptStorePersistsCancellationTerminalEvent(t *testing.T) {
	setRuntimeUserDataDir(t)
	t.TempDir()
	run := Run{RunID: "run_cancel", ThreadID: "thread_cancel"}
	path := transcriptPath(run.ThreadID)

	events := []Event{
		{
			Kind:      EventKindAgent,
			Seq:       1,
			Timestamp: time.Now().UTC(),
			AgentEvent: agentcore.AwakenEvent{
				Type:           agentcore.RunStartedEventType,
				ThreadID:       run.ThreadID,
				RunID:          run.RunID,
				TerminalReason: agentcore.TerminalReasonCompleted,
			},
		},
		{
			Kind:           EventKindRunCancelled,
			Seq:            2,
			Timestamp:      time.Now().UTC().Add(time.Millisecond),
			ThreadID:       run.ThreadID,
			RunID:          run.RunID,
			Message:        "你停止了生成本次回答",
			Code:           "RUN_CANCELLED",
			TerminalReason: agentcore.TerminalReasonCancelled,
		},
	}
	if err := appendRunEventTranscriptAtPath(path, run, events); err != nil {
		t.Fatalf("appendRunEventTranscript returned error: %v", err)
	}

	durableRuns, err := loadDurableTranscriptRunsAtPath(path, run.ThreadID)
	if err != nil {
		t.Fatalf("loadDurableTranscriptRuns returned error: %v", err)
	}
	durable, ok := durableRuns[run.RunID]
	if !ok {
		t.Fatalf("expected durable run %q, got %+v", run.RunID, durableRuns)
	}
	if durable.Run.Status != RunStatusCancelled {
		t.Fatalf("expected cancelled durable run, got %+v", durable.Run)
	}
	if len(durable.Events) != 1 {
		t.Fatalf("expected 1 durable event, got %+v", durable.Events)
	}
	terminal := durable.Events[0].AgentEvent
	if terminal.Type != agentcore.RunErrorEventType {
		t.Fatalf("expected terminal RunError event, got %+v", terminal)
	}
	if terminal.Code != "RUN_CANCELLED" {
		t.Fatalf("expected RUN_CANCELLED code, got %q", terminal.Code)
	}
	if terminal.Error != "你停止了生成本次回答" {
		t.Fatalf("expected Chinese cancel message, got %q", terminal.Error)
	}
}
