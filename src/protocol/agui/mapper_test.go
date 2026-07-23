package agui

import (
	"testing"
	"time"

	"awaken/src/agentcore"
	"awaken/src/tool"
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

func TestMapEventCarriesTurnMetadata(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:                   agentcore.RunFinishedEventType,
		ThreadID:               "thread-1",
		RunID:                  "run-1",
		TurnID:                 "turn-1",
		QueryChainID:           "chain-1",
		Depth:                  2,
		TurnCount:              2,
		Content:                "done",
		TerminalReason:         agentcore.TerminalReasonCompleted,
		TransitionReason:       agentcore.TransitionToolResultContinuation,
		ContextModelWindow:     128000,
		ContextEstimatedTokens: 6400,
		ContextActualTokens:    6123,
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	got := events[0]
	if got.TurnID != "turn-1" || got.QueryChainID != "chain-1" || got.Depth != 2 || got.TurnCount != 2 {
		t.Fatalf("expected turn metadata to be preserved, got %+v", got)
	}
	if got.TerminalReason != "completed" || got.TransitionReason != "tool_result_continuation" {
		t.Fatalf("expected reason metadata to be preserved, got %+v", got)
	}
	if got.ContextModelWindow != 128000 || got.ContextEstimatedTokens != 6400 || got.ContextActualTokens != 6123 {
		t.Fatalf("expected context metadata to be preserved, got %+v", got)
	}
}

func TestMapEventTurnLifecycle(t *testing.T) {
	started := MapEvent(agentcore.AwakenEvent{
		Type:             agentcore.TurnStartedEventType,
		TurnID:           "turn-1",
		QueryChainID:     "chain-1",
		Depth:            1,
		TurnCount:        1,
		TransitionReason: agentcore.TransitionInitial,
	})
	finished := MapEvent(agentcore.AwakenEvent{
		Type:           agentcore.TurnFinishedEventType,
		TurnID:         "turn-1",
		QueryChainID:   "chain-1",
		Depth:          1,
		TurnCount:      1,
		TerminalReason: agentcore.TerminalReasonCompleted,
	})

	if len(started) != 1 || started[0].Type != EventAwakenTurnStarted {
		t.Fatalf("expected AWAKEN_TURN_STARTED, got %+v", started)
	}
	if len(finished) != 1 || finished[0].Type != EventAwakenTurnFinished {
		t.Fatalf("expected AWAKEN_TURN_FINISHED, got %+v", finished)
	}
	if finished[0].TerminalReason != "completed" {
		t.Fatalf("expected terminal reason completed, got %+v", finished[0])
	}
}

func TestMapEventToolResult(t *testing.T) {
	exitCode := 7
	events := MapEvent(agentcore.AwakenEvent{
		Type:            agentcore.ToolResultEventType,
		ThreadID:        "thread-1",
		RunID:           "run-1",
		MessageID:       "toolmsg-1",
		ToolCallID:      "tool-1",
		Content:         "echo:hello",
		ToolStatus:      "failed",
		TransportStatus: "completed",
		SemanticStatus:  "failure",
		ProcessStatus:   "exited",
		ProcessExitCode: &exitCode,
		ProcessTimedOut: false,
		ErrorCode:       "process_exit_nonzero",
		ErrorCategory:   "process",
		Retryable:       false,
		RecoveryOptions: []string{"retry"},
		DurationMs:      42,
		Diagnostic: map[string]any{
			"reason":                  "no_match",
			"environment_snapshot_id": "env-1",
			"tool_snapshot_id":        "tools-1",
			"provider_id":             tool.ProviderID("builtin:bash"),
			"capabilities":            []tool.Capability{"process.execute"},
			"provider_readiness":      tool.ProviderReadiness("ready"),
			"dependency_observations": []map[string]any{{"name": "zsh", "kind": "executable"}},
		},
		ToolProgressStage:      "completed",
		ToolProgressMessage:    "done",
		ToolProgressPercentage: 100,
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
	if events[0].ToolCallName != "" {
		t.Fatalf("expected empty tool name when not provided, got %q", events[0].ToolCallName)
	}
	if events[0].Content != "echo:hello" {
		t.Fatalf("unexpected content %q", events[0].Content)
	}
	if events[0].ToolStatus != "failed" || events[0].ProcessExitCode == nil || *events[0].ProcessExitCode != 7 {
		t.Fatalf("unexpected outcome fields: %+v", events[0])
	}
	if events[0].ToolProgressStage != "completed" || events[0].ToolProgressPercentage != 100 {
		t.Fatalf("unexpected progress fields: %+v", events[0])
	}
	if events[0].Diagnostic["reason"] != "no_match" {
		t.Fatalf("unexpected diagnostic: %+v", events[0].Diagnostic)
	}
	if events[0].EnvironmentSnapshotID != "env-1" || events[0].ToolAvailabilitySnapshotID != "tools-1" || events[0].ProviderID != "builtin:bash" || events[0].Capability != "process.execute" || len(events[0].Capabilities) != 1 || events[0].Readiness != "ready" || len(events[0].Dependencies) != 1 {
		t.Fatalf("unexpected execution metadata: %+v", events[0])
	}
}

func TestMapEventSkillLifecycle(t *testing.T) {
	invoked := MapEvent(agentcore.AwakenEvent{
		Type:         agentcore.SkillActivatedEventType,
		SkillID:      "presentations",
		SkillVersion: "2",
		Diagnostic: map[string]any{
			"source":          "project",
			"skill_path":      "/workspace/.awaken/skills/presentations/SKILL.md",
			"status":          "ok",
			"effective_tools": []string{"Bash", "Read", "artifact_publish"},
		},
	})
	if len(invoked) != 1 || invoked[0].Type != EventAwakenSkillInvoked {
		t.Fatalf("unexpected Skill activation projection: %+v", invoked)
	}
	if invoked[0].SkillSource != "project" || len(invoked[0].SkillEffectiveTools) != 3 {
		t.Fatalf("Skill activation metadata lost: %+v", invoked[0])
	}

	loaded := MapEvent(agentcore.AwakenEvent{
		Type:           agentcore.ResourceReceiptEventType,
		SkillID:        "presentations",
		SkillVersion:   "2",
		ResourcePath:   "references/layout.md",
		ResourceSHA256: "abc123",
		ResourceSize:   42,
		ResourceMode:   "text",
		ResourceSource: "package",
	})
	if len(loaded) != 1 || loaded[0].Type != EventAwakenSkillContentLoaded || loaded[0].SkillResource != "references/layout.md" {
		t.Fatalf("unexpected Skill resource projection: %+v", loaded)
	}
}

func TestMapEventToolCallArgsDelta(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:       agentcore.ToolCallArgsDeltaEventType,
		ThreadID:   "thread-1",
		RunID:      "run-1",
		ToolCallID: "tool-1",
		ToolName:   "Read",
		Delta:      `{"path"`,
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventToolCallArgsDelta {
		t.Fatalf("expected TOOL_CALL_ARGS_DELTA, got %s", events[0].Type)
	}
	if events[0].ToolCallID != "tool-1" || events[0].ToolCallName != "Read" || events[0].Delta != `{"path"` {
		t.Fatalf("unexpected args delta payload: %+v", events[0])
	}
}

func TestMapEventToolCallArgs(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:       agentcore.ToolCallArgsEventType,
		ThreadID:   "thread-1",
		RunID:      "run-1",
		ToolCallID: "tool-1",
		ToolName:   "Read",
		Arguments:  `{"path":"README.md"}`,
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventToolCallArgs {
		t.Fatalf("expected TOOL_CALL_ARGS, got %s", events[0].Type)
	}
	if events[0].ToolCallID != "tool-1" || events[0].ToolCallName != "Read" || events[0].Delta != `{"path":"README.md"}` {
		t.Fatalf("unexpected args payload: %+v", events[0])
	}
}

func TestMapEventToolProgress(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:                   agentcore.ToolProgressEventType,
		ThreadID:               "thread-1",
		RunID:                  "run-1",
		ToolCallID:             "tool-1",
		ToolName:               "Read",
		Progress:               "Executing tool",
		ToolProgressStage:      "started",
		ToolProgressMessage:    "Executing tool",
		ToolProgressPercentage: 0,
		ToolStatus:             "succeeded",
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventToolProgress {
		t.Fatalf("expected TOOL_PROGRESS, got %s", events[0].Type)
	}
	if events[0].ToolCallID != "tool-1" || events[0].ToolCallName != "Read" {
		t.Fatalf("unexpected tool identity: %+v", events[0])
	}
	if events[0].Progress != "Executing tool" || events[0].Message != "Executing tool" {
		t.Fatalf("unexpected progress payload: %+v", events[0])
	}
	if events[0].ToolProgressStage != "started" || events[0].ToolStatus != "succeeded" {
		t.Fatalf("unexpected structured progress fields: %+v", events[0])
	}
}

func TestMapEventToolPermissionDecided(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:                     agentcore.ToolPermissionDecidedEventType,
		ThreadID:                 "thread-1",
		RunID:                    "run-1",
		ToolCallID:               "tool-1",
		ToolName:                 "Edit",
		Content:                  "Edit was denied by the file boundary",
		StaticPermissionBehavior: "deny",
		StaticPermissionReason:   "path escapes writable roots",
		ToolDecisionAction:       "deny",
		ToolDecisionSource:       "headless_default",
		ToolDecisionReason:       "path escapes writable roots",
		AffectedPaths:            []string{"note.md"},
		PatchSummary:             "新增 1 个文件",
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	got := events[0]
	if got.Type != EventAwakenToolPermissionDecided {
		t.Fatalf("expected AWAKEN_TOOL_PERMISSION_DECIDED, got %s", got.Type)
	}
	if got.ToolDecisionAction != "deny" || got.ToolDecisionSource != "headless_default" {
		t.Fatalf("unexpected decision fields: %+v", got)
	}
	if len(got.AffectedPaths) != 1 || got.AffectedPaths[0] != "note.md" {
		t.Fatalf("unexpected affected paths: %+v", got.AffectedPaths)
	}
}

func TestMapEventCommandOutput(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:          agentcore.CommandOutputEventType,
		ThreadID:      "thread-1",
		RunID:         "run-1",
		ToolCallID:    "tool-1",
		ToolName:      "Bash",
		CommandStream: "stdout",
		Delta:         "hello",
		Truncated:     true,
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventAwakenCommandOutput {
		t.Fatalf("expected AWAKEN_COMMAND_OUTPUT, got %s", events[0].Type)
	}
	if events[0].ToolCallID != "tool-1" || events[0].ToolCallName != "Bash" || events[0].CommandStream != "stdout" || events[0].Delta != "hello" || !events[0].Truncated {
		t.Fatalf("unexpected command output payload: %+v", events[0])
	}
}

func TestMapEventPatchUpdated(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:          agentcore.PatchUpdatedEventType,
		ThreadID:      "thread-1",
		RunID:         "run-1",
		ToolCallID:    "tool-1",
		ToolName:      "Edit",
		Patch:         "--- a/note.md\n+++ b/note.md",
		PatchSummary:  "新增 1 个文件",
		AffectedPaths: []string{"note.md"},
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventAwakenPatchUpdated {
		t.Fatalf("expected AWAKEN_PATCH_UPDATED, got %s", events[0].Type)
	}
	if events[0].ToolCallID != "tool-1" || events[0].ToolCallName != "Edit" || events[0].PatchSummary != "新增 1 个文件" {
		t.Fatalf("unexpected patch identity: %+v", events[0])
	}
	if events[0].Patch == "" || len(events[0].AffectedPaths) != 1 || events[0].AffectedPaths[0] != "note.md" {
		t.Fatalf("unexpected patch payload: %+v", events[0])
	}
}

func TestMapEventTurnDiff(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:      agentcore.TurnDiffEventType,
		ThreadID:  "thread-1",
		RunID:     "run-1",
		Diff:      "diff --git a/note.md b/note.md",
		Truncated: true,
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventAwakenTurnDiff {
		t.Fatalf("expected AWAKEN_TURN_DIFF, got %s", events[0].Type)
	}
	if events[0].Diff != "diff --git a/note.md b/note.md" || !events[0].Truncated {
		t.Fatalf("unexpected diff payload: %+v", events[0])
	}
}

func TestMapEventContextEstimated(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:                        agentcore.ContextEstimatedEventType,
		ThreadID:                    "thread-1",
		RunID:                       "run-1",
		Content:                     "context tokens estimated=10",
		ContextModelWindow:          128000,
		ContextOutputReserve:        16000,
		ContextEffectiveWindow:      112000,
		ContextEstimatedTokens:      10,
		ContextWarningThreshold:     95200,
		ContextAutoCompactThreshold: 100800,
		ContextCompactTarget:        33600,
		ContextEstimatedDeltaTokens: 3,
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	got := events[0]
	if got.Type != EventAwakenContextEstimated {
		t.Fatalf("expected AWAKEN_CONTEXT_ESTIMATED, got %s", got.Type)
	}
	if got.ContextEstimatedTokens != 10 ||
		got.ContextModelWindow != 128000 ||
		got.ContextOutputReserve != 16000 ||
		got.ContextEffectiveWindow != 112000 ||
		got.ContextWarningThreshold != 95200 ||
		got.ContextAutoCompactThreshold != 100800 ||
		got.ContextCompactTarget != 33600 ||
		got.ContextEstimatedDeltaTokens != 3 {
		t.Fatalf("unexpected context window payload: %+v", got)
	}
}

func TestMapEventCompactMetadataOptionalFields(t *testing.T) {
	preEstimate := 120
	postEstimate := 48
	fallbackUsed := false
	toolResultsPruned := true
	sourceHadUser := true
	latestUserPinned := true

	events := MapEvent(agentcore.AwakenEvent{
		Type:                             agentcore.CompactFailedEventType,
		ThreadID:                         "thread-1",
		RunID:                            "run-1",
		Content:                          "compact failed",
		CompactMode:                      agentcore.CompactModeReactive,
		CompactBoundaryID:                "compact_1",
		CompactPreEstimate:               &preEstimate,
		CompactPostEstimate:              &postEstimate,
		CompactSummaryModel:              "gpt-5-mini",
		CompactFallbackUsed:              &fallbackUsed,
		CompactToolResultsPruned:         &toolResultsPruned,
		CompactToolResultsPrunedCount:    3,
		CompactPrunedSafeGroupCount:      2,
		CompactPrunedMessageCount:        7,
		CompactSummaryMaxTokens:          8192,
		CompactSummaryRetryCount:         1,
		CompactSummaryRawChars:           321,
		CompactSummaryParsedSections:     13,
		CompactSummaryMeaningfulSections: 4,
		CompactSummaryQualityIssue:       "semantic compact summary omitted compacted tool findings",
		CompactSummarySourceHadUser:      &sourceHadUser,
		CompactSummaryLatestUserPinned:   &latestUserPinned,
		CompactFallbackMarker:            "[COMPACT_FALLBACK:llm_summary_failed]",
		CompactFailureReason:             "provider_timeout",
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	got := events[0]
	if got.Type != EventAwakenCompactFailed {
		t.Fatalf("expected AWAKEN_COMPACT_FAILED, got %s", got.Type)
	}
	if got.CompactMode != "reactive" ||
		got.ContextCompactMode != "reactive" ||
		got.CompactBoundaryID != "compact_1" ||
		got.ContextCompactBoundaryID != "compact_1" ||
		got.CompactSummaryModel != "gpt-5-mini" ||
		got.CompactToolResultsPrunedCount != 3 ||
		got.CompactPrunedSafeGroupCount != 2 ||
		got.CompactPrunedMessageCount != 7 ||
		got.CompactSummaryMaxTokens != 8192 ||
		got.CompactSummaryRetryCount != 1 ||
		got.CompactSummaryRawChars != 321 ||
		got.CompactSummaryParsedSections != 13 ||
		got.CompactSummaryMeaningfulSections != 4 ||
		got.CompactSummaryQualityIssue != "semantic compact summary omitted compacted tool findings" ||
		got.CompactSummarySourceHadUser == nil ||
		!*got.CompactSummarySourceHadUser ||
		got.CompactSummaryLatestUserPinned == nil ||
		!*got.CompactSummaryLatestUserPinned ||
		got.CompactFallbackMarker != "[COMPACT_FALLBACK:llm_summary_failed]" ||
		got.CompactFailureReason != "provider_timeout" {
		t.Fatalf("unexpected compact string metadata: %+v", got)
	}
	if got.CompactPreEstimate == nil ||
		*got.CompactPreEstimate != 120 ||
		got.ContextCompactPreEstimate == nil ||
		*got.ContextCompactPreEstimate != 120 ||
		got.CompactPostEstimate == nil ||
		*got.CompactPostEstimate != 48 ||
		got.ContextCompactPostEstimate == nil ||
		*got.ContextCompactPostEstimate != 48 {
		t.Fatalf("unexpected compact estimate metadata: %+v", got)
	}
	if got.CompactFallbackUsed == nil ||
		*got.CompactFallbackUsed ||
		got.CompactToolResultsPruned == nil ||
		!*got.CompactToolResultsPruned {
		t.Fatalf("unexpected compact boolean metadata: %+v", got)
	}
}

func TestMapEventAuthRequired(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:         agentcore.AuthRequiredEventType,
		ThreadID:     "thread-1",
		RunID:        "run-1",
		ToolCallID:   "tool-1",
		ToolName:     "outlook_send",
		Content:      "outlook account is not connected",
		AuthProvider: "outlook",
		AuthURL:      "https://login.example.test",
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventAwakenAuthRequired {
		t.Fatalf("expected AWAKEN_AUTH_REQUIRED, got %s", events[0].Type)
	}
	if events[0].AuthProvider != "outlook" || events[0].AuthURL == "" {
		t.Fatalf("unexpected auth payload: %+v", events[0])
	}
}

func TestMapEventArtifactUpdated(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:                    agentcore.ArtifactUpdatedEventType,
		ThreadID:                "thread-1",
		RunID:                   "run-1",
		ToolCallID:              "tool-1",
		ToolName:                "write_file",
		ArtifactContractVersion: tool.ArtifactContractVersion,
		ArtifactRole:            tool.ArtifactRoleDeliverable,
		ArtifactProducer:        "artifact_publish",
		ArtifactID:              "artifact-1",
		ArtifactTitle:           "README.md",
		ArtifactDescription:     "Updated project readme",
		ArtifactKind:            "file",
		ArtifactPath:            "/tmp/README.md",
		ArtifactContent:         "# hello",
		ArtifactStatus:          agentcore.ArtifactStatusReady,
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventAwakenArtifactUpdated {
		t.Fatalf("expected AWAKEN_ARTIFACT_UPDATED, got %s", events[0].Type)
	}
	if events[0].ArtifactPath != "/tmp/README.md" || events[0].ArtifactStatus != "ready" {
		t.Fatalf("unexpected artifact payload: %+v", events[0])
	}
	if events[0].ArtifactContractVersion != tool.ArtifactContractVersion || events[0].ArtifactRole != "deliverable" || events[0].ArtifactProducer != "artifact_publish" {
		t.Fatalf("unexpected artifact v2 payload: %+v", events[0])
	}
}

func TestMapEventACPSessionLifecycle(t *testing.T) {
	cases := []struct {
		agentcoreType agentcore.AwakenEventType
		expectedType  EventType
		expectedStatus string
	}{
		{agentcore.ACPSessionStartedEventType, EventAwakenACPSessionChanged, "started"},
		{agentcore.ACPSessionClosedEventType, EventAwakenACPSessionChanged, "closed"},
		{agentcore.ACPSessionInterruptedEventType, EventAwakenACPSessionChanged, "interrupted"},
	}
	for _, tc := range cases {
		events := MapEvent(agentcore.AwakenEvent{
			Type:       tc.agentcoreType,
			ThreadID:   "thread-1",
			RunID:      "run-1",
			ToolCallID: "tool-1",
			Content:    "acp session event",
			Diagnostic: map[string]any{"runner_id": "codex", "acp_session_id": "sess-1"},
		})
		if len(events) != 1 {
			t.Fatalf("expected 1 event for %s, got %d", tc.agentcoreType, len(events))
		}
		got := events[0]
		if got.Type != tc.expectedType {
			t.Fatalf("expected %s, got %s", tc.expectedType, got.Type)
		}
		if got.Status != tc.expectedStatus {
			t.Fatalf("expected status %q, got %q", tc.expectedStatus, got.Status)
		}
		if got.ToolCallID != "tool-1" || got.Diagnostic["runner_id"] != "codex" {
			t.Fatalf("unexpected payload: %+v", got)
		}
	}
}

func TestMapEventACPTurnLifecycle(t *testing.T) {
	started := MapEvent(agentcore.AwakenEvent{
		Type:       agentcore.ACPTurnStartedEventType,
		ToolCallID: "tool-1",
		Content:    "turn started",
		Diagnostic: map[string]any{"runner_id": "codex", "turn_id": "turn-1"},
	})
	if len(started) != 1 || started[0].Type != EventAwakenACPTurnChanged || started[0].Status != "started" {
		t.Fatalf("unexpected turn started event: %+v", started)
	}

	finished := MapEvent(agentcore.AwakenEvent{
		Type:       agentcore.ACPTurnFinishedEventType,
		ToolCallID: "tool-1",
		Content:    "turn result text",
		Diagnostic: map[string]any{"runner_id": "codex", "turn_id": "turn-1", "stop_reason": "tool_use"},
	})
	if len(finished) != 1 || finished[0].Type != EventAwakenACPTurnChanged || finished[0].Status != "finished" {
		t.Fatalf("unexpected turn finished event: %+v", finished)
	}
	if finished[0].Content != "turn result text" {
		t.Fatalf("expected content to be preserved, got %q", finished[0].Content)
	}
}

func TestMapEventACPToolSnapshot(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:       agentcore.ACPToolSnapshotEventType,
		ToolCallID: "host-tool-1",
		ToolName:   "edit",
		Content:    "Updated note.md",
		Diagnostic: map[string]any{"runner_id": "codex", "tool_call_id": "acp-tool-1", "status": "completed"},
	})
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	got := events[0]
	if got.Type != EventAwakenACPToolChanged {
		t.Fatalf("expected AWAKEN_ACP_TOOL_CHANGED, got %s", got.Type)
	}
	if got.ToolCallID != "host-tool-1" || got.ToolCallName != "edit" || got.Content != "Updated note.md" {
		t.Fatalf("unexpected tool snapshot payload: %+v", got)
	}
}

func TestMapEventACPPermissionLifecycle(t *testing.T) {
	requested := MapEvent(agentcore.AwakenEvent{
		Type:          agentcore.ACPPermissionRequestedEventType,
		ToolCallID:    "tool-1",
		InteractionID: "perm-1",
		Content:       "Allow editing note.md?",
		Diagnostic:    map[string]any{"runner_id": "codex", "kind": "file_edit"},
	})
	if len(requested) != 1 || requested[0].Type != EventAwakenACPPermissionRequested {
		t.Fatalf("unexpected permission requested event: %+v", requested)
	}
	if requested[0].InteractionID != "perm-1" || requested[0].Message != "Allow editing note.md?" {
		t.Fatalf("unexpected permission requested payload: %+v", requested[0])
	}

	resolved := MapEvent(agentcore.AwakenEvent{
		Type:          agentcore.ACPPermissionResolvedEventType,
		ToolCallID:    "tool-1",
		InteractionID: "perm-1",
		Content:       "allow",
		Diagnostic:    map[string]any{"runner_id": "codex", "cancelled": false},
	})
	if len(resolved) != 1 || resolved[0].Type != EventAwakenACPPermissionResolved {
		t.Fatalf("unexpected permission resolved event: %+v", resolved)
	}
	if resolved[0].Status != "allow" || resolved[0].InteractionID != "perm-1" {
		t.Fatalf("unexpected permission resolved payload: %+v", resolved[0])
	}
}

func TestMapEventUnknownFallsBackWithRawPayload(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:      agentcore.AwakenEventType("FutureEvent"),
		ThreadID:  "thread-1",
		RunID:     "run-1",
		Content:   "future payload",
		ToolName:  "future_tool",
		TurnID:    "turn-1",
		TurnCount: 3,
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	got := events[0]
	if got.Type != EventAwakenUnknown {
		t.Fatalf("expected AWAKEN_UNKNOWN_EVENT, got %s", got.Type)
	}
	if got.RawType != "FutureEvent" || got.Message != "future payload" {
		t.Fatalf("expected raw type and message to be preserved, got %+v", got)
	}
	raw, ok := got.RawEvent.(map[string]any)
	if !ok {
		t.Fatalf("expected raw event map, got %T", got.RawEvent)
	}
	if raw["Type"] != "FutureEvent" || raw["ToolName"] != "future_tool" {
		t.Fatalf("expected raw payload to include original fields, got %+v", raw)
	}
}
