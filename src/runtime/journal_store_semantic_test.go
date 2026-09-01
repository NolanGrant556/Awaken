package runtime

import (
	"encoding/json"
	"testing"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/tool"
)

func TestMessagesFromSemanticRecordsRestoresCanonicalOutcomeModelPayload(t *testing.T) {
	exitCode := 7
	now := time.Now().UTC()
	outcome := tool.ExecutionOutcome{
		Content:         "command exited with status 7",
		Status:          tool.OutcomeFailed,
		TransportStatus: tool.TransportCompleted,
		SemanticStatus:  tool.SemanticFailure,
		ProcessStatus:   tool.ProcessExited,
		ExitCode:        &exitCode,
		ErrorCode:       tool.ErrorCodeProcessExitNonzero,
		ErrorCategory:   tool.ErrorCategoryProcess,
		ErrorMessage:    "exit status 7",
		Retryable:       true,
		RecoveryOptions: []string{"inspect_exit_code_and_stderr", "change_command_or_stop"},
		Diagnostic:      map[string]any{"stderr": "boom"},
		StartedAt:       now,
		FinishedAt:      now,
	}
	record := agentcore.NewSemanticRecord(
		agentcore.SemanticKindToolResult,
		"thread_1",
		"run_1",
		"turn_1",
		"call_1",
		"",
		map[string]any{
			"tool_call_id": "call_1",
			"tool_name":    "shell",
			"content":      "UI display: command failed",
			"is_error":     false, // stale compatibility bit must not win
			"outcome":      outcomeToAnyMap(outcome),
		},
	)
	messages := messagesFromSemanticRecords([]agentcore.SemanticRecord{record})
	if len(messages) != 1 || messages[0].Role != convcontext.RoleTool {
		t.Fatalf("unexpected restored messages: %+v", messages)
	}
	msg := messages[0]
	if msg.Content == "UI display: command failed" {
		t.Fatal("model history received UI display text instead of Outcome.ModelPayload")
	}
	if !msg.IsError {
		t.Fatal("canonical outcome error state did not override stale compatibility bit")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(msg.Content), &payload); err != nil {
		t.Fatalf("restored model payload is not JSON: %v\n%s", err, msg.Content)
	}
	if payload["status"] != string(tool.OutcomeFailed) ||
		payload["error_code"] != tool.ErrorCodeProcessExitNonzero ||
		payload["exit_code"] != float64(7) ||
		payload["retryable"] != true {
		t.Fatalf("restored model payload lost outcome facts: %#v", payload)
	}
	diagnostic, _ := payload["diagnostic"].(map[string]any)
	if diagnostic["stderr"] != "boom" {
		t.Fatalf("restored model payload lost diagnostic: %#v", payload)
	}
}

func TestMessagesFromSemanticRecordsKeepsLegacyDisplayFallback(t *testing.T) {
	record := agentcore.NewSemanticRecord(
		agentcore.SemanticKindToolResult,
		"thread_1",
		"run_1",
		"turn_1",
		"call_legacy",
		"",
		map[string]any{
			"tool_call_id": "call_legacy",
			"tool_name":    "legacy",
			"content":      "legacy result",
			"is_error":     true,
		},
	)
	messages := messagesFromSemanticRecords([]agentcore.SemanticRecord{record})
	if len(messages) != 1 || messages[0].Content != "legacy result" || !messages[0].IsError {
		t.Fatalf("legacy fallback changed: %+v", messages)
	}
}

func TestSkillInvocationHistoryRestoresLatestTimestampWithoutLeakingForkContext(t *testing.T) {
	first := time.Date(2026, 7, 11, 1, 0, 0, 0, time.UTC)
	latest := first.Add(time.Hour)
	records := []agentcore.SemanticRecord{
		{Kind: agentcore.SemanticKindSkillActivated, Timestamp: first, Payload: map[string]any{
			"skill_id": "inline", "content": "inline body", "context": "inline", "invoked_at": first.Format(time.RFC3339Nano),
		}},
		{Kind: agentcore.SemanticKindSkillActivated, Timestamp: latest, Payload: map[string]any{
			"skill_id": "inline", "content": "new body", "context": "inline", "invoked_at": latest.Format(time.RFC3339Nano),
		}},
		{Kind: agentcore.SemanticKindSkillActivated, Timestamp: latest, Payload: map[string]any{
			"skill_id": "forked", "content": "child-only body", "context": "fork", "invoked_at": latest.Format(time.RFC3339Nano),
		}},
	}
	restored := restoredTranscriptFromSemantic(records)
	if got := restored.SkillInvokedAt["inline"]; !got.Equal(latest) {
		t.Fatalf("latest inline invocation=%s", got)
	}
	if got := restored.SkillInvokedAt["forked"]; !got.Equal(latest) {
		t.Fatalf("fork invocation timestamp missing=%s", got)
	}
	if len(restored.SkillContextBlocks) != 1 || restored.SkillContextBlocks[0].SkillID != "inline" || restored.SkillContextBlocks[0].Content != "new body" {
		t.Fatalf("fork context leaked or inline latest lost: %+v", restored.SkillContextBlocks)
	}
}

func TestContextCompactedV2RestoresSameMessagesAndBoundedSkillProjection(t *testing.T) {
	invokedAt := time.Date(2026, 8, 31, 1, 2, 3, 0, time.UTC)
	skillRef := agentcore.SkillContextRef{
		ID: "skill_activation_inline", SkillID: "inline", SourcePath: "/skills/inline/SKILL.md",
		InvokedAt: invokedAt, Compacted: true, ProjectedContent: "bounded skill body",
	}
	if err := skillRef.Seal(); err != nil {
		t.Fatalf("seal Skill ref: %v", err)
	}
	bundle := phase1RuntimeTestBundle(t, "compact_v2_restore", []agentcore.SemanticCompactedMessage{{
		Role: string(convcontext.RoleUser), Content: "V2 summary", CompactBoundaryID: "compact_v2_restore",
	}}, []agentcore.SkillContextRef{skillRef})
	records := []agentcore.SemanticRecord{
		{Kind: agentcore.SemanticKindSkillActivated, Timestamp: invokedAt.Add(-time.Hour), Payload: map[string]any{
			"skill_id": "inline", "content": "unbounded original body", "context": "inline",
		}},
		{Kind: agentcore.SemanticKindContextCompactedV2, Timestamp: invokedAt, Payload: map[string]any{"bundle_v2": bundle}},
	}
	restored := restoredTranscriptFromSemantic(records)
	if restored.LatestContextBundleV2 == nil || restored.LatestContextBundleV2.BundleDigest != bundle.BundleDigest {
		t.Fatalf("latest V2 bundle was not restored: %+v", restored.LatestContextBundleV2)
	}
	if len(restored.Messages) != 1 || restored.Messages[0].Content != "V2 summary" || restored.Messages[0].CompactBoundaryID != bundle.BoundaryID {
		t.Fatalf("V2 conversation projection mismatch: %+v", restored.Messages)
	}
	if len(restored.SkillContextBlocks) != 1 || restored.SkillContextBlocks[0].Content != "bounded skill body" || !restored.SkillContextBlocks[0].Compacted {
		t.Fatalf("V2 Skill projection mismatch: %+v", restored.SkillContextBlocks)
	}
}

func TestContextCompactedV2ThenV1DoesNotRestoreStaleV2Authorities(t *testing.T) {
	skillRef := agentcore.SkillContextRef{
		ID: "skill_activation_inline", SkillID: "inline", SourcePath: "/skills/inline/SKILL.md",
		InvokedAt: time.Now().UTC(), Compacted: true, ProjectedContent: "bounded V2 skill ref",
	}
	if err := skillRef.Seal(); err != nil {
		t.Fatalf("seal Skill ref: %v", err)
	}
	bundle := phase1RuntimeTestBundle(t, "compact_v2_before_rollback", []agentcore.SemanticCompactedMessage{{
		Role: string(convcontext.RoleUser), Content: "V2 summary", CompactBoundaryID: "compact_v2_before_rollback",
	}}, []agentcore.SkillContextRef{skillRef})
	v1Boundary := "compact_v1_after_rollback"
	records := []agentcore.SemanticRecord{
		{Kind: agentcore.SemanticKindSkillActivated, Payload: map[string]any{
			"skill_id": "inline", "content": "original activation body", "context": "inline",
		}},
		{Kind: agentcore.SemanticKindContextCompactedV2, Payload: map[string]any{"bundle_v2": bundle}},
		{Kind: agentcore.SemanticKindContextCompacted, Payload: map[string]any{
			"compact_boundary_id": v1Boundary,
			"messages":            []agentcore.SemanticCompactedMessage{{Role: string(convcontext.RoleUser), Content: "latest V1 summary", CompactBoundaryID: v1Boundary}},
		}},
	}
	restored := restoredTranscriptFromSemantic(records)
	if restored.LatestContextBundleV2 != nil || len(restored.CompactContextBlocks) != 0 {
		t.Fatalf("stale V2 authority survived a later V1 boundary: %+v", restored.LatestContextBundleV2)
	}
	if len(restored.Messages) != 1 || restored.Messages[0].Content != "latest V1 summary" {
		t.Fatalf("latest V1 conversation was not restored: %+v", restored.Messages)
	}
	if len(restored.SkillContextBlocks) != 1 || restored.SkillContextBlocks[0].Content != "original activation body" || restored.SkillContextBlocks[0].Compacted {
		t.Fatalf("V1 rollback did not restore legacy Skill activation authority: %+v", restored.SkillContextBlocks)
	}
}

func TestContextCompactedV1ThenV2RestoresLatestV2Authority(t *testing.T) {
	v1Boundary := "compact_v1_before_v2"
	bundle := phase1RuntimeTestBundle(t, "compact_v2_latest", []agentcore.SemanticCompactedMessage{{
		Role: string(convcontext.RoleUser), Content: "latest V2 summary", CompactBoundaryID: "compact_v2_latest",
	}}, nil)
	records := []agentcore.SemanticRecord{
		{Kind: agentcore.SemanticKindContextCompacted, Payload: map[string]any{
			"compact_boundary_id": v1Boundary,
			"messages":            []agentcore.SemanticCompactedMessage{{Role: string(convcontext.RoleUser), Content: "old V1 summary", CompactBoundaryID: v1Boundary}},
		}},
		{Kind: agentcore.SemanticKindContextCompactedV2, Payload: map[string]any{"bundle_v2": bundle}},
	}
	restored := restoredTranscriptFromSemantic(records)
	if restored.LatestContextBundleV2 == nil || restored.LatestContextBundleV2.BoundaryID != bundle.BoundaryID {
		t.Fatalf("latest V2 authority was not restored: %+v", restored.LatestContextBundleV2)
	}
	if len(restored.Messages) != 1 || restored.Messages[0].Content != "latest V2 summary" {
		t.Fatalf("latest V2 conversation was not restored: %+v", restored.Messages)
	}
}

func TestContextCompactedV2DigestMismatchKeepsLastKnownGoodConversation(t *testing.T) {
	bundle := phase1RuntimeTestBundle(t, "compact_v2_corrupt", []agentcore.SemanticCompactedMessage{{
		Role: string(convcontext.RoleUser), Content: "V2 summary", CompactBoundaryID: "compact_v2_corrupt",
	}}, nil)
	bundle.BundleDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	records := []agentcore.SemanticRecord{
		agentcore.NewSemanticRecord(agentcore.SemanticKindUserMessage, "thread_1", "run_1", "turn_1", "", "", map[string]any{"content": "last known good"}),
		{Kind: agentcore.SemanticKindContextCompactedV2, Payload: map[string]any{"bundle_v2": bundle}},
	}
	messages := messagesFromSemanticRecords(records)
	if len(messages) != 1 || messages[0].Content != "last known good" || messages[0].CompactBoundaryID != "" {
		t.Fatalf("corrupt V2 bundle erased last known-good conversation: %+v", messages)
	}
}

func TestLatestContextWindowUsageAnchorFromSemanticRecordsFallsBackToInput(t *testing.T) {
	sentEstimate := 74_000
	inputTokens := 96_000
	record := agentcore.ModelUsageSemanticRecord("thread-1", "run-1", "turn-1", "session-1", agentcore.ModelUsageRecord{
		SchemaVersion: 1, RequestID: "req-1", Purpose: "agent_turn",
		ProviderID: "minimax", ProfileID: "profile-a", CredentialRevision: 3,
		ModelID: "ep-1", CapabilityModelID: "MiniMax-M1", EndpointSelector: "ep-1",
		ModelSelectorKind: "endpoint", ProtocolFamily: "openai_chat", ReasoningPolicy: "high",
		Status: agentcore.ModelUsageSucceeded, UsageComplete: true,
		SentEstimatedInputTokens: &sentEstimate,
		InputTokens:              &inputTokens,
	})

	anchor := latestContextWindowUsageAnchorFromSemanticRecords([]agentcore.SemanticRecord{record})
	if anchor == nil || anchor.UsageTokens != inputTokens || anchor.SentEstimatedInputTokens != sentEstimate {
		t.Fatalf("expected input-token fallback anchor, got %+v", anchor)
	}
}

func TestLatestContextWindowUsageAnchorFromSemanticRecordsStopsAtCompactBoundary(t *testing.T) {
	sentEstimate := 74_000
	totalTokens := 101_000
	usage := agentcore.ModelUsageSemanticRecord("thread-1", "run-1", "turn-1", "session-1", agentcore.ModelUsageRecord{
		SchemaVersion: 1, RequestID: "req-1", Purpose: "agent_turn",
		ProviderID: "minimax", ProfileID: "profile-a", CredentialRevision: 3,
		ModelID: "ep-1", CapabilityModelID: "MiniMax-M1", EndpointSelector: "ep-1",
		ModelSelectorKind: "endpoint", ProtocolFamily: "openai_chat", ReasoningPolicy: "high",
		Status: agentcore.ModelUsageSucceeded, UsageComplete: true,
		SentEstimatedInputTokens: &sentEstimate,
		TotalTokens:              &totalTokens,
	})
	records := []agentcore.SemanticRecord{
		usage,
		{Kind: agentcore.SemanticKindContextCompactedV2, Payload: map[string]any{"bundle_v2": phase1RuntimeTestBundle(t, "compact-after-usage", []agentcore.SemanticCompactedMessage{{
			Role: string(convcontext.RoleUser), Content: "summary", CompactBoundaryID: "compact-after-usage",
		}}, nil)}},
	}
	if anchor := latestContextWindowUsageAnchorFromSemanticRecords(records); anchor != nil {
		t.Fatalf("compact boundary must clear usage anchor, got %+v", anchor)
	}
}

func TestLatestContextWindowUsageAnchorFromSemanticRecordsStopsAtHardEpochBoundaryAndMissingFields(t *testing.T) {
	sentEstimate := 74_000
	totalTokens := 101_000
	older := agentcore.ModelUsageSemanticRecord("thread-1", "run-1", "turn-1", "session-1", agentcore.ModelUsageRecord{
		SchemaVersion: 1, RequestID: "req-old", Purpose: "agent_turn",
		ProviderID: "minimax", ProfileID: "profile-a", CredentialRevision: 3,
		ModelID: "ep-1", CapabilityModelID: "MiniMax-M1", EndpointSelector: "ep-1",
		ModelSelectorKind: "endpoint", ProtocolFamily: "openai_chat", ReasoningPolicy: "high",
		Status: agentcore.ModelUsageSucceeded, UsageComplete: true,
		SentEstimatedInputTokens: &sentEstimate,
		TotalTokens:              &totalTokens,
	})
	missingSent := agentcore.ModelUsageSemanticRecord("thread-1", "run-2", "turn-2", "session-2", agentcore.ModelUsageRecord{
		SchemaVersion: 1, RequestID: "req-new", Purpose: "agent_turn",
		ProviderID: "minimax", ProfileID: "profile-a", CredentialRevision: 3,
		ModelID: "ep-1", CapabilityModelID: "MiniMax-M1", EndpointSelector: "ep-1",
		ModelSelectorKind: "endpoint", ProtocolFamily: "openai_chat", ReasoningPolicy: "high",
		Status: agentcore.ModelUsageSucceeded, UsageComplete: true,
		TotalTokens: &totalTokens,
	})
	if anchor := latestContextWindowUsageAnchorFromSemanticRecords([]agentcore.SemanticRecord{older, missingSent}); anchor != nil {
		t.Fatalf("missing sent estimate must not reuse an older anchor, got %+v", anchor)
	}

	epochBoundary := agentcore.ContextEpochSemanticRecord("thread-1", "run-2", "turn-2", "session-2", agentcore.ContextEpochReceipt{
		EpochID: "epoch-2", BaselineDigest: "digest", State: "rotated", BoundaryReason: "model_changed", CreationBoundaryReason: "model_changed", BaselineSequence: 2,
	})
	if anchor := latestContextWindowUsageAnchorFromSemanticRecords([]agentcore.SemanticRecord{older, epochBoundary}); anchor != nil {
		t.Fatalf("hard epoch boundary must prevent older usage reuse, got %+v", anchor)
	}
}

func phase1RuntimeTestBundle(t *testing.T, boundaryID string, messages []agentcore.SemanticCompactedMessage, skills []agentcore.SkillContextRef) agentcore.PostCompactContextBundleV2 {
	t.Helper()
	bundle := agentcore.PostCompactContextBundleV2{
		SchemaVersion: agentcore.PostCompactContextBundleSchemaVersion,
		BoundaryID:    boundaryID, Generation: 1, Mode: string(agentcore.CompactModeAuto),
		ConversationProjection: agentcore.CompactConversationProjectionV2{Format: agentcore.CompactProjectionV1Messages, Messages: messages},
		TaskCheckpoint:         agentcore.NotMaterializedSection[agentcore.TaskCheckpointV2](),
		SemanticSummary:        agentcore.NotMaterializedSection[string](),
		RecentTurns:            agentcore.NotMaterializedSection[[]agentcore.CompactTurnV2](),
		InstructionSnapshotRef: agentcore.UnavailableSnapshotRef("instruction_snapshot_unavailable"),
		MemorySnapshot:         agentcore.UnavailableMemorySnapshotRef("memory_snapshot_unavailable"),
		RunPlanRef:             agentcore.UnavailableSnapshotRef("run_plan_unavailable"),
		SkillRefs:              skills,
		ToolSnapshotRef:        agentcore.UnavailableSnapshotRef("tool_snapshot_unavailable"),
		HookSnapshotRef:        agentcore.UnavailableSnapshotRef("hook_snapshot_unavailable"),
		ActiveGateRefs:         []agentcore.GateRef{}, PendingSubagents: []agentcore.SubagentHandoffRef{}, ArtifactRefs: []agentcore.ArtifactRef{},
		TranscriptLocator:   agentcore.NotMaterializedSection[agentcore.TranscriptLocator](),
		ProviderOpaqueState: agentcore.NotMaterializedSection[agentcore.ProviderOpaqueCompactionRef](),
		BudgetReceipt:       agentcore.NotMaterializedSection[agentcore.CompactBudgetReceipt](),
		CoverageReceipt:     agentcore.NotMaterializedSection[agentcore.CompactCoverageReceipt](),
	}
	if err := bundle.Seal(); err != nil {
		t.Fatalf("seal test bundle: %v", err)
	}
	return bundle
}

func TestMessagesFromSemanticRecordsRestoresToolMediaMetadataWithoutHydrating(t *testing.T) {
	outcome := tool.ExecutionOutcome{
		Content: "page image",
		Status:  tool.OutcomeSucceeded,
		ModelContent: []tool.ModelContentPart{{
			Type: "image", MimeType: "image/png", DisplayName: "chart.png",
			StorageKey: "thread_1/tool-results/call_1/part-001.png", ByteSize: 12, DataBase64: "",
		}},
	}
	record := agentcore.NewSemanticRecord(
		agentcore.SemanticKindToolResult,
		"thread_1",
		"run_1",
		"turn_1",
		"call_1",
		"",
		map[string]any{
			"tool_call_id": "call_1",
			"tool_name":    "Read",
			"content":      "page image",
			"outcome":      outcomeToAnyMap(outcome),
		},
	)
	messages := messagesFromSemanticRecords([]agentcore.SemanticRecord{record})
	if len(messages) != 1 || len(messages[0].ModelContent) != 1 {
		t.Fatalf("unexpected restored messages: %+v", messages)
	}
	part := messages[0].ModelContent[0]
	if part.StorageKey != "thread_1/tool-results/call_1/part-001.png" || part.ByteSize != 12 {
		t.Fatalf("replay lost durable media metadata: %+v", part)
	}
	if part.DataBase64 != "" {
		t.Fatalf("replay hydrated media bytes: %q", part.DataBase64)
	}
}
