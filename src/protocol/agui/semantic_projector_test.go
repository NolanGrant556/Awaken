package agui

import (
	"testing"
	"time"

	"awaken/src/agentcore"
	"awaken/src/tool"
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

func TestProjectSemanticRecordSuppressesLocalContextAuthorities(t *testing.T) {
	for _, kind := range []agentcore.SemanticKind{
		agentcore.SemanticKindContextEpoch,
		agentcore.SemanticKindProviderOpaqueState,
	} {
		record := agentcore.NewSemanticRecord(kind, "thread-1", "run-1", "turn-1", "", "", map[string]any{
			"content": "must-not-enter-wire",
		})
		if frames := projectSemanticRecord(record); len(frames) != 0 {
			t.Fatalf("local authority %s projected to wire: %+v", kind, frames)
		}
	}
}

func TestArtifactV2SemanticRoundTrip(t *testing.T) {
	event := agentcore.AwakenEvent{
		Type:                    agentcore.ArtifactCreatedEventType,
		ToolCallID:              "call_publish",
		ToolName:                "artifact_publish",
		ArtifactContractVersion: tool.ArtifactContractVersion,
		ArtifactRole:            tool.ArtifactRoleDeliverable,
		ArtifactProducer:        "artifact_publish",
		ArtifactID:              "artifact_publish:report.pdf",
		ArtifactTitle:           "Final report",
		ArtifactDescription:     "Published report.pdf",
		ArtifactKind:            "pdf",
		ArtifactPath:            "report.pdf",
		ArtifactStatus:          agentcore.ArtifactStatusReady,
	}
	record := agentcore.ArtifactSemanticRecord("thread", "run", "turn", "session", event)
	if record.Payload["contract_version"] != tool.ArtifactContractVersion || record.Payload["role"] != "deliverable" || record.Payload["producer"] != "artifact_publish" {
		t.Fatalf("semantic record lost v2 metadata: %+v", record.Payload)
	}

	frames := ProjectSemanticRecord(record)
	if len(frames) != 1 {
		t.Fatalf("expected one artifact frame, got %+v", frames)
	}
	frame := frames[0]
	if frame.ArtifactContractVersion != tool.ArtifactContractVersion || frame.ArtifactRole != "deliverable" || frame.ArtifactProducer != "artifact_publish" {
		t.Fatalf("projector lost v2 metadata: %+v", frame)
	}

	replayed := SemanticRecordsToAwakenEvents([]agentcore.SemanticRecord{record})
	if len(replayed) != 1 {
		t.Fatalf("expected one replay event, got %+v", replayed)
	}
	if replayed[0].ArtifactContractVersion != tool.ArtifactContractVersion || replayed[0].ArtifactRole != tool.ArtifactRoleDeliverable || replayed[0].ArtifactProducer != "artifact_publish" {
		t.Fatalf("replay lost v2 metadata: %+v", replayed[0])
	}
}

func TestProjectLegacyArtifactInfersRoleWithoutRewritingHistory(t *testing.T) {
	record := agentcore.SemanticRecord{
		SchemaVersion: 2,
		RecordID:      "legacy_artifact",
		Seq:           1,
		ThreadID:      "thread",
		RunID:         "run",
		Kind:          agentcore.SemanticKindArtifact,
		Timestamp:     time.Now().UTC(),
		Payload: map[string]any{
			"artifact_id":  "presentation_create:deck_preview/deck.pdf",
			"title":        "deck.pdf",
			"kind":         "pdf",
			"path":         "deck_preview/deck.pdf",
			"status":       "ready",
			"tool_call_id": "call_legacy",
			"tool_name":    "presentation_create",
			"created":      true,
		},
	}

	frames := ProjectSemanticRecord(record)
	if len(frames) != 1 {
		t.Fatalf("expected one frame, got %+v", frames)
	}
	if frames[0].ArtifactContractVersion != tool.ArtifactLegacyContractVersion || frames[0].ArtifactRole != "preview" || frames[0].ArtifactProducer != "presentation_create" {
		t.Fatalf("legacy inference mismatch: %+v", frames[0])
	}
	if _, exists := record.Payload["role"]; exists {
		t.Fatalf("projector must not rewrite legacy payload: %+v", record.Payload)
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

func TestProjectSemanticSkillLifecycle(t *testing.T) {
	ts := time.Date(2026, 7, 12, 8, 0, 0, 0, time.UTC)
	records := []agentcore.SemanticRecord{
		{
			SchemaVersion: 2,
			RecordID:      "skill-invoked",
			Seq:           3,
			ThreadID:      "thread-skill",
			RunID:         "run-skill",
			Kind:          agentcore.SemanticKindSkillActivated,
			Timestamp:     ts,
			Payload: map[string]any{
				"skill_id":             "presentations",
				"skill_version":        "2",
				"compatibility_status": "ok",
				"effective_tools":      []string{"presentation_create", "shell"},
			},
		},
		{
			SchemaVersion: 2,
			RecordID:      "skill-resource",
			Seq:           4,
			ThreadID:      "thread-skill",
			RunID:         "run-skill",
			Kind:          agentcore.SemanticKindResourceReceipt,
			Timestamp:     ts,
			Payload: map[string]any{
				"skill_id":      "presentations",
				"skill_version": "2",
				"resource":      "references/layout.md",
				"source":        "package",
				"sha256":        "abc123",
				"size":          int64(42),
				"mode":          "text",
			},
		},
	}

	frames := ProjectSemanticRecords(records)
	if len(frames) != 3 {
		t.Fatalf("expected invoked, execution-context and content-loaded frames, got %+v", frames)
	}
	if frames[0].Type != EventAwakenSkillInvoked || frames[1].Type != EventAwakenSkillExecutionContext || frames[2].Type != EventAwakenSkillContentLoaded {
		t.Fatalf("unexpected skill lifecycle order: %+v", frames)
	}
	if frames[0].SkillID != "presentations" || len(frames[1].SkillEffectiveTools) != 2 {
		t.Fatalf("skill activation metadata lost: %+v", frames)
	}
	if frames[2].SkillResource != "references/layout.md" || frames[2].SkillResourceSHA256 != "abc123" || frames[2].SkillResourceSize != 42 {
		t.Fatalf("skill resource metadata lost: %+v", frames[2])
	}
}

func TestProjectSemanticFailedSkillLifecycle(t *testing.T) {
	record := agentcore.SemanticRecord{
		SchemaVersion: 2,
		RecordID:      "skill-failed",
		Seq:           7,
		ThreadID:      "thread-skill",
		RunID:         "run-skill",
		Kind:          agentcore.SemanticKindSkillActivated,
		Timestamp:     time.Now().UTC(),
		Payload: map[string]any{
			"skill_id":             "broken-skill",
			"compatibility_status": "failed",
			"compatibility_errors": []map[string]any{{"code": "parse_failed", "message": "frontmatter is invalid"}},
		},
	}

	frames := ProjectSemanticRecord(record)
	if len(frames) != 2 || frames[0].Type != EventAwakenSkillInvoked || frames[1].Type != EventAwakenSkillFailedToLoad {
		t.Fatalf("unexpected failed skill lifecycle: %+v", frames)
	}
	if len(frames[1].SkillDiagnostics) != 1 || frames[1].SkillDiagnostics[0] != "parse_failed: frontmatter is invalid" {
		t.Fatalf("failed skill diagnostics lost: %+v", frames[1])
	}
}

func TestProjectSemanticToolExecutionMetadata(t *testing.T) {
	record := agentcore.SemanticRecord{
		SchemaVersion: 2,
		RecordID:      "tool-result-with-provider",
		Seq:           8,
		ThreadID:      "thread-tool",
		RunID:         "run-tool",
		Kind:          agentcore.SemanticKindToolResult,
		Timestamp:     time.Now().UTC(),
		Payload: map[string]any{
			"tool_call_id": "call-1",
			"tool_name":    "presentation_render",
			"content":      "rendered",
			"outcome": map[string]any{
				"status":                  "succeeded",
				"environment_snapshot_id": "env-1",
				"tool_snapshot_id":        "tools-1",
				"provider_id":             "builtin.presentation_render",
				"capabilities":            []string{"presentation.render", "artifact.preview"},
				"provider_readiness":      "ready",
				"dependency":              map[string]any{"name": "libreoffice", "kind": "executable"},
				"dependency_observations": []map[string]any{{"name": "libreoffice", "kind": "executable"}},
				"operation":               map[string]any{"side_effect_state": "applied"},
			},
		},
	}

	frames := ProjectSemanticRecord(record)
	if len(frames) != 1 {
		t.Fatalf("expected one tool result frame, got %+v", frames)
	}
	frame := frames[0]
	if frame.EnvironmentSnapshotID != "env-1" || frame.ToolAvailabilitySnapshotID != "tools-1" || frame.ProviderID != "builtin.presentation_render" || frame.Capability != "presentation.render" || len(frame.Capabilities) != 2 || frame.Readiness != "ready" || frame.SideEffectState != "applied" {
		t.Fatalf("execution metadata lost: %+v", frame)
	}
	if frame.Dependency["name"] != "libreoffice" || len(frame.Dependencies) != 1 {
		t.Fatalf("dependency metadata lost: %+v", frame)
	}
}

func TestProjectSemanticContextSnapshotAndModelUsageForLiveReplay(t *testing.T) {
	now := time.Now().UTC()
	snapshot := agentcore.ContextUsageSnapshot{
		SchemaVersion: 4, SnapshotID: "ctx_1", EstimatorID: "awaken_leaf_v4", Basis: "sent_request",
		RequestID: "req_1", ModelID: "model", ProtocolFamily: "openai_chat", RequestShapeDigest: "digest",
		ModelWindowTokens: 128000, OutputReserveTokens: 16000, EffectiveInputWindowTokens: 112000,
		EstimatedInputTokens: 42, WarningThresholdTokens: 95000, AutoCompactThresholdTokens: 100000,
		CompactTargetTokens: 33000, GeneratedAt: now,
		Contributions: []agentcore.ContextUsageContribution{{Kind: agentcore.ContextContributionConversation, EstimatedTokens: 42, SourceCount: 1}},
	}
	usage := agentcore.ModelUsageRecord{
		SchemaVersion: 1, RequestID: "req_1", Purpose: "agent_turn", ModelID: "model",
		ProtocolFamily: "openai_chat", InputTokens: intPtr(40), OutputTokens: intPtr(2), TotalTokens: intPtr(42),
		UsageComplete: true, RetryIndex: 2, AuxiliaryFallback: true, CalculatedCostUSD: floatPtr(0.125), Currency: "USD",
		PricingSnapshot: &agentcore.PricingSnapshot{SnapshotID: "price_1", ProviderID: "deepseek", ModelID: "model", Currency: "USD", UnitTokens: 1_000_000, EffectiveDate: "2026-07-19", SourceURL: "https://example.test/pricing"},
		LatencyMS:       25, Status: agentcore.ModelUsageSucceeded, CompletedAt: now,
	}
	records := []agentcore.SemanticRecord{
		agentcore.ContextSnapshotSemanticRecord("thread", "run", "turn", "session", snapshot),
		agentcore.ModelRequestSemanticRecord("thread", "run", "turn", "session", snapshot),
		agentcore.ModelUsageSemanticRecord("thread", "run", "turn", "session", usage),
	}
	for index := range records {
		records[index].Seq = uint64(index + 1)
		records[index].Timestamp = now
	}
	frames := ProjectSemanticRecords(records)
	if len(frames) != 2 || frames[0].Type != EventAwakenContextSnapshot || frames[1].Type != EventAwakenModelUsage {
		t.Fatalf("unexpected context/usage projection: %+v", frames)
	}
	if frames[0].ContextUsageSnapshot == nil || frames[0].ContextUsageSnapshot.EstimatedInputTokens != 42 || len(frames[0].ContextUsageSnapshot.Contributions) != 1 {
		t.Fatalf("context breakdown was lost: %+v", frames[0])
	}
	if frames[1].ModelUsage == nil || frames[1].ModelUsage.RequestID != "req_1" || frames[1].ModelUsage.InputTokens == nil || *frames[1].ModelUsage.InputTokens != 40 {
		t.Fatalf("model usage was lost: %+v", frames[1])
	}
	if frames[1].ModelUsage.RetryIndex != 2 || !frames[1].ModelUsage.AuxiliaryFallback || frames[1].ModelUsage.CalculatedCostUSD == nil || frames[1].ModelUsage.PricingSnapshot == nil {
		t.Fatalf("pricing/retry fields were lost: %+v", frames[1].ModelUsage)
	}

	legacy := ProjectSemanticRecord(agentcore.SemanticRecord{
		SchemaVersion: 2, RecordID: "legacy", Seq: 9, ThreadID: "thread", RunID: "run",
		Kind: agentcore.SemanticKindRunTerminal, Timestamp: now, Payload: map[string]any{"status": "finished"},
	})
	if len(legacy) != 1 || legacy[0].ContextUsageSnapshot != nil {
		t.Fatalf("legacy record must not fabricate category details: %+v", legacy)
	}
}

func intPtr(value int) *int           { return &value }
func floatPtr(value float64) *float64 { return &value }
