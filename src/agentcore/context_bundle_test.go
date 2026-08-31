package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/subagentruntime"
	"awaken/src/tool"
)

func TestPostCompactContextBundleV2RoundTripAndDigestFailClosed(t *testing.T) {
	runner := NewRunner(AgentConfig{}, convcontext.NewConversation(nil), NewRequestBuilder(AgentConfig{}))
	bundle, err := runner.buildPostCompactContextBundleV2(CompactResult{
		Mode: CompactModeAuto, BoundaryID: "compact_v2_roundtrip",
		Messages: []convcontext.Message{{
			Role: convcontext.RoleUser, Content: "durable summary", CompactBoundaryID: "compact_v2_roundtrip",
		}},
	}, nil)
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	var raw any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatalf("unmarshal raw bundle: %v", err)
	}
	restored, err := DecodePostCompactContextBundleV2(raw)
	if err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	if restored.BundleDigest != bundle.BundleDigest || restored.TaskCheckpoint.State != MaterializationMaterialized || restored.CoverageReceipt.State != MaterializationMaterialized {
		t.Fatalf("round-trip changed bundle: before=%+v after=%+v", bundle, restored)
	}

	restored.ConversationProjection.Messages[0].Content = "tampered"
	if err := restored.Validate(); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("tampered bundle did not fail closed: %v", err)
	}
}

func TestPostCompactContextBundleV2InstallsAtomicallyWithoutResettingMemoryOrConsumingSubagents(t *testing.T) {
	original := []convcontext.Message{{Role: convcontext.RoleUser, Content: "old history"}}
	conversation := convcontext.NewConversation(nil)
	conversation.Context.SetMessages(original)
	memoryResets := 0
	markedCompactedMemory := []string{}
	config := AgentConfig{
		MemoryRecallReset:         func() { memoryResets++ },
		MemoryRecallMarkCompacted: func(ids []string) { markedCompactedMemory = append(markedCompactedMemory, ids...) },
		SkillContextBlocks: []ContextBlock{{
			ID: "skill_1", SkillID: "skill_1", SourcePath: "/skills/skill_1/SKILL.md",
			InvokedAt: time.Now().UTC(), Content: strings.Repeat("x", 25_000),
		}},
	}
	runner := NewRunner(config, conversation, NewRequestBuilder(config))
	runner.threadID = "thread_bundle_install"
	runner.runID = "run_bundle_install"
	runner.sessionID = "session_bundle_install"
	runner.recorder = &failingRecorder{}
	runner.memoryRecallSnapshot = &MemoryRecallSnapshot{WorkspaceRoot: "/workspace", Items: []MemoryRecallSnapshotItem{{
		MemoryID: "memory_install", Scope: "project", Type: "project", Revision: 1, Bytes: 32, Content: "body must not survive in the live snapshot",
	}}}
	controller := &bundleTestController{tasks: []subagentruntime.TaskSnapshot{{
		TaskID: "task_1", AgentID: "agent_1", Status: subagentruntime.StatusCompleted,
		Description: "research", ResultPreview: "done",
	}}}
	runner.subagentController = controller

	result := CompactResult{
		Mode: CompactModeAuto, BoundaryID: "compact_bundle_install",
		Messages: []convcontext.Message{{
			Role: convcontext.RoleUser, Content: "summary", CompactBoundaryID: "compact_bundle_install",
		}},
	}
	if err := runner.contextGovernor.applyCompactResult(context.Background(), result); err != nil {
		t.Fatalf("apply compact result: %v", err)
	}
	if memoryResets != 0 {
		t.Fatalf("successful V2 compact reset Memory %d times", memoryResets)
	}
	if controller.drainCalls != 0 {
		t.Fatalf("V2 compact consumed terminal handoffs %d times", controller.drainCalls)
	}
	if runner.config.RestoredContextBundleV2 == nil || len(runner.config.RestoredContextBundleV2.PendingSubagents) != 1 {
		t.Fatalf("installed bundle lost Subagent refs: %+v", runner.config.RestoredContextBundleV2)
	}
	if runner.config.RestoredContextBundleV2.MemorySnapshot.State != ReferenceAvailable || len(runner.config.RestoredContextBundleV2.MemorySnapshot.Items) != 1 {
		t.Fatalf("installed bundle lost compacted Memory refs: %+v", runner.config.RestoredContextBundleV2.MemorySnapshot)
	}
	if runner.memoryRecallSnapshot != nil || len(markedCompactedMemory) != 1 || markedCompactedMemory[0] != "memory_install" {
		t.Fatalf("compacted Memory body remained live or identity was not marked: snapshot=%+v marked=%v", runner.memoryRecallSnapshot, markedCompactedMemory)
	}
	if len(runner.config.SkillContextBlocks) != 1 || !runner.config.SkillContextBlocks[0].Compacted {
		t.Fatalf("bounded Skill projection was not installed: %+v", runner.config.SkillContextBlocks)
	}
	if got := conversation.Context.GetMessages(); len(got) != 1 || got[0].CompactBoundaryID != result.BoundaryID {
		t.Fatalf("conversation projection was not installed: %+v", got)
	}
}

func TestTaskCheckpointV2RollsForwardUserCorrectionAcrossRepeatedCompaction(t *testing.T) {
	firstConversation := convcontext.NewConversation(nil)
	firstConversation.Context.SetMessages([]convcontext.Message{{
		Role: convcontext.RoleUser, Content: "我意思是压缩后完整上下文必须剩余10%，不要丢失任务。", MessageID: "msg_correction",
	}})
	firstRunner := NewRunner(AgentConfig{}, firstConversation, NewRequestBuilder(AgentConfig{}))
	first, err := firstRunner.buildPostCompactContextBundleV2(CompactResult{
		Mode: CompactModeAuto, BoundaryID: "compact_generation_1",
		Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "summary one", CompactBoundaryID: "compact_generation_1"}},
	}, nil)
	if err != nil {
		t.Fatalf("first bundle: %v", err)
	}
	if first.TaskCheckpoint.Value == nil || len(first.TaskCheckpoint.Value.UserCorrections) != 1 || len(first.TaskCheckpoint.Value.Constraints) != 1 {
		t.Fatalf("first checkpoint did not capture correction/constraint: %+v", first.TaskCheckpoint.Value)
	}

	current := first
	for generation := 2; generation <= 5; generation++ {
		config := AgentConfig{RestoredContextBundleV2: &current}
		conversation := convcontext.NewConversation(nil)
		conversation.Context.SetMessages([]convcontext.Message{
			{Role: convcontext.RoleUser, Content: "prior compact summary", CompactBoundaryID: current.BoundaryID, Origin: convcontext.MessageOriginCompactSummary},
			{Role: convcontext.RoleUser, Content: "继续按这个约束开发。", MessageID: "msg_continue"},
		})
		runner := NewRunner(config, conversation, NewRequestBuilder(config))
		boundaryID := fmt.Sprintf("compact_generation_%d", generation)
		next, buildErr := runner.buildPostCompactContextBundleV2(CompactResult{
			Mode: CompactModeAuto, BoundaryID: boundaryID,
			Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "rolling summary", CompactBoundaryID: boundaryID}},
		}, nil)
		if buildErr != nil {
			t.Fatalf("generation %d bundle: %v", generation, buildErr)
		}
		if next.Generation != generation || next.ParentBoundaryID != current.BoundaryID {
			t.Fatalf("generation %d rolling identity mismatch: %+v", generation, next)
		}
		if generation == 3 || generation == 5 {
			checkpoint := next.TaskCheckpoint.Value
			if checkpoint == nil || len(checkpoint.UserCorrections) != 1 || !strings.Contains(checkpoint.UserCorrections[0].NormalizedValue, "10%") {
				t.Fatalf("generation %d lost correction: %+v", generation, checkpoint)
			}
		}
		current = next
	}
	checkpoint := current.TaskCheckpoint.Value
	if checkpoint == nil || len(checkpoint.UserCorrections) != 1 || !strings.Contains(checkpoint.UserCorrections[0].NormalizedValue, "10%") {
		t.Fatalf("prior correction was lost after repeated compact: %+v", checkpoint)
	}
	blocks := CompactContextBlocksFromBundleV2(&current)
	if len(blocks) != 1 || strings.Contains(blocks[0].Content, "msg_correction") || !strings.Contains(blocks[0].Content, "10%") ||
		!strings.Contains(blocks[0].Content, `"user_corrections":["$ref:pending_user_asks[0]"]`) {
		t.Fatalf("checkpoint was not rendered into provider-neutral context: %+v", blocks)
	}
}

func TestPostCompactBundleCarriesPriorMemoryRefsWithoutReinliningBodies(t *testing.T) {
	conversation := convcontext.NewConversation(nil)
	conversation.Context.SetMessages([]convcontext.Message{{Role: convcontext.RoleUser, Content: "remember the task", MessageID: "msg_memory"}})
	firstRunner := NewRunner(AgentConfig{}, conversation, NewRequestBuilder(AgentConfig{}))
	firstRunner.memoryRecallSnapshot = &MemoryRecallSnapshot{WorkspaceRoot: "/workspace", Items: []MemoryRecallSnapshotItem{{
		MemoryID: "memory_old", Scope: "project", Type: "project", Revision: 2, Bytes: 100, Content: "old body must become a ref",
	}}}
	first, err := firstRunner.buildPostCompactContextBundleV2(CompactResult{
		Mode: CompactModeAuto, BoundaryID: "compact_memory_1",
		Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "summary", CompactBoundaryID: "compact_memory_1"}},
	}, nil)
	if err != nil {
		t.Fatalf("first bundle: %v", err)
	}
	secondConversation := convcontext.NewConversation(nil)
	secondConversation.Context.SetMessages([]convcontext.Message{{Role: convcontext.RoleUser, Content: "summary", CompactBoundaryID: first.BoundaryID}})
	secondRunner := NewRunner(AgentConfig{RestoredContextBundleV2: &first}, secondConversation, NewRequestBuilder(AgentConfig{}))
	second, err := secondRunner.buildPostCompactContextBundleV2(CompactResult{
		Mode: CompactModeAuto, BoundaryID: "compact_memory_2",
		Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "summary two", CompactBoundaryID: "compact_memory_2"}},
	}, nil)
	if err != nil {
		t.Fatalf("second bundle: %v", err)
	}
	if second.MemorySnapshot.State != ReferenceAvailable || len(second.MemorySnapshot.Items) != 1 || second.MemorySnapshot.Items[0].MemoryID != "memory_old" {
		t.Fatalf("repeated compact lost prior identity-only Memory ref: %+v", second.MemorySnapshot)
	}
	if strings.Contains(CompactedMemoryContextFromBundleV2(&second), "old body must become a ref") {
		t.Fatal("compacted Memory context re-inlined the old body")
	}
}

func TestPostCompactContextBundleV2EnforcesExactHardTenPercentWhenModelLimitIsKnown(t *testing.T) {
	config := AgentConfig{
		SystemPrompt: "{{.RuntimeInfo}}",
		ModelConfig:  ModelConfig{Model: "test-model", ContextWindow: 256_000},
	}
	conversation := convcontext.NewConversation(nil)
	conversation.Context.SetMessages([]convcontext.Message{{Role: convcontext.RoleUser, Content: "keep this task", MessageID: "msg_hard_target"}})
	runner := NewRunner(config, conversation, NewRequestBuilder(config))
	bundle, err := runner.buildPostCompactContextBundleV2(CompactResult{
		Mode: CompactModeAuto, BoundaryID: "compact_hard_target",
		Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "summary", CompactBoundaryID: "compact_hard_target"}},
	}, nil)
	if err != nil {
		t.Fatalf("build hard-target bundle: %v", err)
	}
	if bundle.BudgetReceipt.State != MaterializationMaterialized || bundle.BudgetReceipt.Value == nil {
		t.Fatalf("known model limit did not produce budget receipt: %+v", bundle.BudgetReceipt)
	}
	receipt := bundle.BudgetReceipt.Value
	if receipt.CompactTargetTokens != int(float64(receipt.EffectiveInputTokens)*0.10) || receipt.ConservativeProjected > receipt.CompactTargetTokens {
		t.Fatalf("hard 10%% postcondition failed: %+v", receipt)
	}

	unreachableConfig := config
	unreachableConfig.ModelConfig.ContextWindow = 32_000
	unreachableRunner := NewRunner(unreachableConfig, conversation, NewRequestBuilder(unreachableConfig))
	_, err = unreachableRunner.buildPostCompactContextBundleV2(CompactResult{
		Mode: CompactModeAuto, BoundaryID: "compact_unreachable",
		Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "summary", CompactBoundaryID: "compact_unreachable"}},
	}, nil)
	if err == nil || !errors.Is(err, ErrCompactTargetUnachievable) || !strings.Contains(err.Error(), CompactTargetUnachievableCode) {
		t.Fatalf("mandatory floor did not return typed hard-target error: %v", err)
	}
	var targetErr *CompactTargetUnachievableError
	if !errors.As(err, &targetErr) || len(targetErr.CategoryBreakdown) == 0 || !strings.Contains(err.Error(), "category_breakdown=") {
		t.Fatalf("hard-target error omitted category breakdown: %#v", err)
	}
}

func TestPostCompactContextBundleV2MaterializesNativeCompactionAndDropsItOnIdentityChange(t *testing.T) {
	config := nativeCompactionTestAgentConfig(1_050_000)
	conversation := convcontext.NewConversation(nil)
	conversation.Context.SetMessages([]convcontext.Message{{Role: convcontext.RoleUser, Content: "keep current task"}})
	runner := NewRunner(config, conversation, NewRequestBuilder(config))
	state := nativeCompactionTestState("opaque-small")
	bundle, err := runner.buildPostCompactContextBundleV2(CompactResult{
		Mode: CompactModeAuto, BoundaryID: "compact_native_bundle",
		Messages:              []convcontext.Message{{Role: convcontext.RoleUser, Content: "local checkpoint summary", CompactBoundaryID: "compact_native_bundle"}},
		NativeCompactionState: state, NativeCompactionAttempted: true, NativeCompactionUsed: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.ProviderOpaqueState.State != MaterializationMaterialized || bundle.ProviderOpaqueState.Value == nil {
		t.Fatalf("native state not materialized: %+v", bundle.ProviderOpaqueState)
	}
	restored, err := DecodePostCompactContextBundleV2(mustJSONValue(t, bundle))
	if err != nil {
		t.Fatal(err)
	}
	replayed := nativeCompactionStateFromBundle(config.ModelConfig, restored)
	if replayed == nil || replayed.Digest != state.Digest || len(replayed.Output) != 2 {
		t.Fatalf("native state did not survive restart: %+v", replayed)
	}
	changed := config.ModelConfig
	changed.CredentialRevision++
	if nativeCompactionStateFromBundle(changed, restored) != nil {
		t.Fatal("credential revision change replayed incompatible native state")
	}
	changed = config.ModelConfig
	changed.Model = "gpt-5.6-terra"
	if nativeCompactionStateFromBundle(changed, restored) != nil {
		t.Fatal("model change replayed incompatible native state")
	}
}

func TestProviderNativeCompactionSuccessAndFailureCommitOnlyOneLocalBoundary(t *testing.T) {
	for _, test := range []struct {
		name       string
		invoke     func(context.Context, ModelCallAuthority, llm.Config, llm.Request) (*llm.Response, error)
		wantNative bool
	}{
		{
			name: "native success",
			invoke: func(_ context.Context, _ ModelCallAuthority, _ llm.Config, request llm.Request) (*llm.Response, error) {
				if !request.NativeCompaction || request.Purpose != llm.RequestPurposeNativeCompaction {
					t.Fatalf("unexpected native request: %+v", request)
				}
				return &llm.Response{NativeCompactionState: nativeCompactionTestState("opaque-success")}, nil
			},
			wantNative: true,
		},
		{
			name: "native provider failure",
			invoke: func(_ context.Context, _ ModelCallAuthority, _ llm.Config, _ llm.Request) (*llm.Response, error) {
				return nil, errors.New("provider unavailable")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := nativeCompactionTestAgentConfig(1_050_000)
			conversation := convcontext.NewConversation(nil)
			conversation.Context.SetMessages([]convcontext.Message{{Role: convcontext.RoleUser, Content: "old task history"}})
			runner := NewRunner(config, conversation, NewRequestBuilder(config))
			runner.threadID, runner.runID, runner.sessionID = "thread_native", "run_native", "session_native"
			recorder := &failingRecorder{}
			runner.recorder = recorder
			runner.nativeCompactionInvoker = test.invoke
			result := CompactResult{
				Mode: CompactModeAuto, BoundaryID: "compact_native_once",
				Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "local checkpoint summary", CompactBoundaryID: "compact_native_once"}},
			}
			if err := runner.contextGovernor.applyCompactResult(context.Background(), result); err != nil {
				t.Fatal(err)
			}
			boundaries := 0
			for _, record := range recorder.records {
				if record.Kind == SemanticKindContextCompactedV2 {
					boundaries++
				}
			}
			if boundaries != 1 {
				t.Fatalf("native path committed %d compact boundaries: %+v", boundaries, recorder.records)
			}
			materialized := runner.config.RestoredContextBundleV2 != nil && runner.config.RestoredContextBundleV2.ProviderOpaqueState.State == MaterializationMaterialized
			if materialized != test.wantNative {
				t.Fatalf("native materialization mismatch: got=%v bundle=%+v", materialized, runner.config.RestoredContextBundleV2)
			}
		})
	}
}

func TestOversizedNativeCompactionOutputFallsBackToLocalHardTenPercent(t *testing.T) {
	config := nativeCompactionTestAgentConfig(200_000)
	conversation := convcontext.NewConversation(nil)
	conversation.Context.SetMessages([]convcontext.Message{{Role: convcontext.RoleUser, Content: "old task history"}})
	runner := NewRunner(config, conversation, NewRequestBuilder(config))
	runner.threadID, runner.runID, runner.sessionID = "thread_native_large", "run_native_large", "session_native_large"
	runner.recorder = &failingRecorder{}
	runner.nativeCompactionInvoker = func(_ context.Context, _ ModelCallAuthority, _ llm.Config, _ llm.Request) (*llm.Response, error) {
		return &llm.Response{NativeCompactionState: nativeCompactionTestState(strings.Repeat("x", 160_000))}, nil
	}
	result := CompactResult{
		Mode: CompactModeAuto, BoundaryID: "compact_native_large",
		Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "local checkpoint summary", CompactBoundaryID: "compact_native_large"}},
	}
	if err := runner.contextGovernor.applyCompactResult(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	bundle := runner.config.RestoredContextBundleV2
	if bundle == nil || bundle.ProviderOpaqueState.State != MaterializationNotMaterialized {
		t.Fatalf("oversized native state should fall back locally: %+v", bundle)
	}
	if bundle.BudgetReceipt.Value == nil || bundle.BudgetReceipt.Value.ConservativeProjected > bundle.BudgetReceipt.Value.CompactTargetTokens {
		t.Fatalf("local fallback violated hard target: %+v", bundle.BudgetReceipt)
	}
}

func nativeCompactionTestAgentConfig(contextWindow int) AgentConfig {
	return AgentConfig{
		SystemPrompt: "{{.RuntimeInfo}}",
		ModelConfig: ModelConfig{
			APIKey: "test", BaseURL: "https://api.openai.com/v1", Model: "gpt-5.6-sol",
			ProviderID: "openai", ProfileID: "profile-openai", CredentialRevision: 4,
			Protocol: string(llm.ProtocolOpenAIResponses), ContextWindow: contextWindow,
		},
	}
}

func nativeCompactionTestState(encryptedContent string) *llm.NativeCompactionState {
	output := []json.RawMessage{
		json.RawMessage(`{"type":"message","role":"user","content":[{"type":"input_text","text":"old user"}]}`),
		json.RawMessage(fmt.Sprintf(`{"type":"compaction","encrypted_content":%q}`, encryptedContent)),
	}
	return &llm.NativeCompactionState{
		ProviderID: "openai", ProfileID: "profile-openai", CredentialRevision: 4,
		ModelID: "gpt-5.6-sol", ProtocolFamily: llm.ProtocolOpenAIResponses,
		EndpointContractID: llm.EndpointContractOpenAIResponses,
		Output:             output, Digest: rawMessagesDigest(output),
	}
}

func mustJSONValue(t *testing.T, value any) any {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result any
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestPostCompactBudgetUsesPreparedBodyAcrossProtocols(t *testing.T) {
	for _, protocol := range []string{"openai_chat", "openai_responses", "anthropic_messages"} {
		t.Run(protocol, func(t *testing.T) {
			config := AgentConfig{
				SystemPrompt: "system {{.RuntimeInfo}}",
				ModelConfig: ModelConfig{
					Model: "test-model", Protocol: protocol, ContextWindow: 512_000, MaxTokens: 32_000,
				},
			}
			conversation := convcontext.NewConversation(nil)
			conversation.Context.SetMessages([]convcontext.Message{{Role: convcontext.RoleUser, Content: "keep the task", MessageID: "msg_protocol"}})
			runner := NewRunner(config, conversation, NewRequestBuilder(config))
			bundle, err := runner.buildPostCompactContextBundleV2(CompactResult{
				Mode: CompactModeAuto, BoundaryID: "compact_" + protocol,
				Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "summary", CompactBoundaryID: "compact_" + protocol}},
			}, nil)
			if err != nil {
				t.Fatalf("build protocol bundle: %v", err)
			}
			receipt := bundle.BudgetReceipt.Value
			if receipt == nil || receipt.LocalProjectedTokens <= 0 || receipt.ConservativeProjected > receipt.CompactTargetTokens {
				t.Fatalf("prepared protocol body did not pass the hard gate: %+v", receipt)
			}
		})
	}
}

func TestTaskCheckpointV2CoversExecutionFactsAndInjectsRecentTurns(t *testing.T) {
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "请继续执行已经记录的修复。", MessageID: "msg_task"},
		{Role: convcontext.RoleAssistant, ToolCalls: []convcontext.ToolCall{{ID: "call_1", Function: convcontext.FunctionCall{Name: "bash", Arguments: `{}`}}}},
		{Role: convcontext.RoleTool, ToolCallID: "call_1", Name: "bash", Content: "COMPACT_TARGET_UNACHIEVABLE: failed", IsError: true, MessageID: "msg_error"},
		{Role: convcontext.RoleAssistant, Content: "需要调整预算后继续。"},
	}
	conversation := convcontext.NewConversation(nil)
	conversation.Context.SetMessages(messages)
	runner := NewRunner(AgentConfig{}, conversation, NewRequestBuilder(AgentConfig{}))
	runner.threadID = "thread_execution_facts"
	runner.recorder = &bundleRangeRecorder{sourceRecords: []SemanticRecord{
		{Seq: 1, RecordID: "rec_user", Kind: SemanticKindUserMessage, Payload: map[string]any{"content": "请继续执行已经记录的修复。"}},
		{Seq: 2, RecordID: "rec_tool_call", Kind: SemanticKindToolCall, Payload: map[string]any{
			"tool_name": "bash", "arguments": `{"command":"go test ./src/a","path":"/Users/nolangrant/Awaken/src/a.go"}`,
		}},
		{Seq: 3, RecordID: "rec_tool_result", Kind: SemanticKindToolResult, Payload: map[string]any{
			"tool_name": "bash", "tool_call_id": "call_1", "content": "COMPACT_TARGET_UNACHIEVABLE: failed", "is_error": true,
		}},
		{Seq: 4, RecordID: "rec_tool_call_repeat", Kind: SemanticKindToolCall, Payload: map[string]any{
			"tool_name": "bash", "arguments": `{"command":"go test ./src/a"}`,
		}},
		{Seq: 5, RecordID: "rec_tool_call_sed", Kind: SemanticKindToolCall, Payload: map[string]any{
			"tool_name": "bash", "arguments": `{"command":"sed -n '1p' /Users/nolangrant/Awaken/src/a.go"}`,
		}},
		{Seq: 6, RecordID: "rec_child_private", Kind: SemanticKindToolCall, Payload: map[string]any{
			"agent_id": "child_agent", "tool_name": "bash", "arguments": `{"command":"cat /private/child-only-secret"}`,
		}},
	}}
	bundle, err := runner.buildPostCompactContextBundleV2(CompactResult{
		Mode: CompactModeAuto, BoundaryID: "compact_execution_facts",
		Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "summary", CompactBoundaryID: "compact_execution_facts"}},
	}, nil)
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	checkpoint := bundle.TaskCheckpoint.Value
	if checkpoint == nil || len(checkpoint.RelevantFiles) == 0 || len(checkpoint.Commands) == 0 || len(checkpoint.Errors) == 0 || len(checkpoint.ToolFindings) == 0 {
		t.Fatalf("execution facts were not extracted: %+v", checkpoint)
	}
	if checkpoint.RelevantFiles[0].NormalizedValue != "/Users/nolangrant/Awaken/src/a.go" || checkpoint.RelevantFiles[0].SequenceStart != 2 || checkpoint.Commands[0].SequenceStart != 2 {
		t.Fatalf("Journal tool arguments lost exact path or sequence provenance: files=%+v commands=%+v", checkpoint.RelevantFiles, checkpoint.Commands)
	}
	if checkpoint.RelevantFiles[0].Priority != "supporting" || checkpoint.Commands[0].Priority != "supporting" || checkpoint.ToolFindings[0].Priority != "supporting" {
		t.Fatalf("retrievable execution details must not inflate critical/important coverage: files=%+v commands=%+v findings=%+v", checkpoint.RelevantFiles, checkpoint.Commands, checkpoint.ToolFindings)
	}
	repeatedCommandFacts := 0
	seenCommandFactIDs := map[string]bool{}
	foundSed := false
	for _, fact := range checkpoint.Commands {
		if fact.NormalizedValue == "go test ./src/a" {
			repeatedCommandFacts++
			seenCommandFactIDs[fact.FactID] = true
		}
		if strings.HasPrefix(fact.NormalizedValue, "sed -n") {
			foundSed = true
		}
		if strings.Contains(fact.NormalizedValue, "child-only-secret") {
			t.Fatalf("root checkpoint included a child Agent fact: %+v", fact)
		}
	}
	if repeatedCommandFacts != 2 || len(seenCommandFactIDs) != 2 || !foundSed {
		t.Fatalf("repeated commands lost provenance or shell command capture was incomplete: %+v", checkpoint.Commands)
	}
	coverage := bundle.CoverageReceipt.Value
	if coverage == nil || coverage.CriticalFactTotal == 0 || coverage.CriticalFactCovered != coverage.CriticalFactTotal || len(coverage.CriticalFactIDs) == 0 || len(coverage.MissingFactIDs) != 0 {
		t.Fatalf("coverage did not bind source fact IDs: %+v", coverage)
	}
	blocks := CompactContextBlocksFromBundleV2(&bundle)
	if len(blocks) != 1 || !strings.Contains(blocks[0].Content, "awaken_recent_turns_v2") || !strings.Contains(blocks[0].Content, "COMPACT_TARGET_UNACHIEVABLE") {
		t.Fatalf("recent complete turn was not provider-visible: %+v", blocks)
	}
}

func TestTaskCheckpointV2BoundsHistoricalCriticalFactsWithoutCoverageFailure(t *testing.T) {
	records := make([]SemanticRecord, 0, maxCheckpointFactsPerCategory+6)
	for index := 0; index < maxCheckpointFactsPerCategory+6; index++ {
		records = append(records, SemanticRecord{
			Seq:      uint64(index + 1),
			RecordID: fmt.Sprintf("rec_user_%02d", index),
			Kind:     SemanticKindUserMessage,
			Payload:  map[string]any{"content": fmt.Sprintf("historical request %02d", index)},
		})
	}
	conversation := convcontext.NewConversation(nil)
	conversation.Context.SetMessages([]convcontext.Message{{
		Role: convcontext.RoleUser, Content: "historical request 69", MessageID: "msg_latest",
	}})
	runner := NewRunner(AgentConfig{}, conversation, NewRequestBuilder(AgentConfig{}))
	runner.threadID = "thread_bounded_critical_facts"
	runner.recorder = &bundleRangeRecorder{sourceRecords: records}

	bundle, err := runner.buildPostCompactContextBundleV2(CompactResult{
		Mode: CompactModeAuto, BoundaryID: "compact_bounded_critical_facts",
		Messages: []convcontext.Message{{
			Role: convcontext.RoleUser, Content: "bounded summary", CompactBoundaryID: "compact_bounded_critical_facts",
		}},
	}, nil)
	if err != nil {
		t.Fatalf("bounded historical critical facts must not invalidate coverage: %v", err)
	}
	checkpoint := bundle.TaskCheckpoint.Value
	coverage := bundle.CoverageReceipt.Value
	if checkpoint == nil || len(checkpoint.PendingUserAsks) != maxCheckpointFactsPerCategory ||
		checkpoint.PendingUserAsks[0].NormalizedValue != "historical request 06" {
		t.Fatalf("critical fact rolling window mismatch: %+v", checkpoint)
	}
	if coverage == nil || coverage.CriticalFactTotal != maxCheckpointFactsPerCategory ||
		coverage.CriticalFactCovered != maxCheckpointFactsPerCategory || len(coverage.MissingFactIDs) != 0 {
		t.Fatalf("coverage must bind the retained source-fact window: %+v", coverage)
	}
	if bundle.TranscriptLocator.Value == nil || bundle.TranscriptLocator.Value.SequenceFrom != 1 ||
		bundle.TranscriptLocator.Value.SequenceTo != int64(len(records)) {
		t.Fatalf("evicted historical facts lost their Journal locator: %+v", bundle.TranscriptLocator)
	}
}

func TestPostCompactBudgetDefersOversizedRecentTurnsToJournal(t *testing.T) {
	parts := make([]tool.ModelContentPart, 0, 20)
	for index := 0; index < 20; index++ {
		parts = append(parts, tool.ModelContentPart{
			Type: "image", MimeType: "image/png", DisplayName: "large.pdf", Page: index + 1,
			StorageKey: fmt.Sprintf("thread/tool-results/call/part-%03d.png", index+1), ByteSize: 160000,
		})
	}
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "读取大 PDF 后继续当前任务", MessageID: "msg_pdf_task"},
		{Role: convcontext.RoleAssistant, ToolCalls: []convcontext.ToolCall{{ID: "call_pdf", Function: convcontext.FunctionCall{Name: "Read", Arguments: `{"file_path":"large.pdf","pages":"1-20"}`}}}},
		{Role: convcontext.RoleTool, ToolCallID: "call_pdf", Name: "Read", Content: strings.Repeat("important extracted content ", 4000), ModelContent: parts},
		{Role: convcontext.RoleAssistant, Content: strings.Repeat("PDF findings ", 20000)},
	}
	config := AgentConfig{SystemPrompt: "{{.RuntimeInfo}}", ModelConfig: ModelConfig{
		Model: "kimi-for-coding", Protocol: "anthropic_messages", ContextWindow: 262144, MaxTokens: 32768,
	}}
	conversation := convcontext.NewConversation(nil)
	conversation.Context.SetMessages(messages)
	runner := NewRunner(config, conversation, NewRequestBuilder(config))
	runner.threadID = "thread_recent_turn_budget"
	runner.recorder = &bundleRangeRecorder{sourceRecords: []SemanticRecord{
		{Seq: 1, RecordID: "rec_user", Kind: SemanticKindUserMessage, Payload: map[string]any{"content": "读取大 PDF 后继续当前任务"}},
	}}
	bundle, err := runner.buildPostCompactContextBundleV2(CompactResult{
		Mode: CompactModeAuto, BoundaryID: "compact_recent_turn_budget",
		Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "bounded semantic summary", CompactBoundaryID: "compact_recent_turn_budget"}},
	}, nil)
	if err != nil {
		t.Fatalf("oversized retrievable RecentTurns must not make the hard target unreachable: %v", err)
	}
	if bundle.RecentTurns.Value == nil || len(*bundle.RecentTurns.Value) != 0 {
		t.Fatalf("oversized RecentTurns were not deferred to the Journal: %+v", bundle.RecentTurns)
	}
	if bundle.TranscriptLocator.Value == nil || bundle.BudgetReceipt.Value == nil ||
		bundle.BudgetReceipt.Value.ConservativeProjected > bundle.BudgetReceipt.Value.CompactTargetTokens {
		t.Fatalf("deferred RecentTurns lost locator or hard-target receipt: locator=%+v receipt=%+v", bundle.TranscriptLocator, bundle.BudgetReceipt)
	}
}

func TestCompactTurnsExcludeInProgressCurrentTask(t *testing.T) {
	turns := compactTurnsFromMessages([]convcontext.Message{
		{Role: convcontext.RoleUser, Content: "closed task"},
		{Role: convcontext.RoleAssistant, Content: "closed answer"},
		{Role: convcontext.RoleUser, Content: "active task"},
		{Role: convcontext.RoleAssistant, ToolCalls: []convcontext.ToolCall{{ID: "call_active", Function: convcontext.FunctionCall{Name: "Read"}}}},
		{Role: convcontext.RoleTool, ToolCallID: "call_active", Name: "Read", Content: "still running"},
	})
	if len(turns) != 1 || !turns[0].Closed || turns[0].Messages[0].Content != "closed task" {
		t.Fatalf("RecentTurns must contain only completed tasks: %+v", turns)
	}
}

func TestCompactCheckpointProviderProjectionKeepsSemanticsWithoutHostMetadata(t *testing.T) {
	fact := newCheckpointFactAt("tool_finding", "关键工具结论", "important", true, []string{"rec_secret_host_metadata"}, 10, 11)
	bundle := PostCompactContextBundleV2{
		BoundaryID: "compact_projection",
		TaskCheckpoint: MaterializedSection[TaskCheckpointV2]{
			State: MaterializationMaterialized,
			Value: &TaskCheckpointV2{
				ActiveTask: "保留 canary 玄铁灯塔", ToolFindings: []CheckpointFact{fact},
			},
			Digest: "checkpoint_digest_not_for_provider",
		},
	}
	blocks := CompactContextBlocksFromBundleV2(&bundle)
	if len(blocks) != 1 || !strings.Contains(blocks[0].Content, "保留 canary 玄铁灯塔") ||
		strings.Contains(blocks[0].Content, "关键工具结论") || !strings.Contains(blocks[0].Content, `"tool_findings":1`) {
		t.Fatalf("provider checkpoint projection lost semantic continuity: %+v", blocks)
	}
	for _, forbidden := range []string{"rec_secret_host_metadata", fact.Digest, `"source_record_ids"`, `"sequence_start"`} {
		if strings.Contains(blocks[0].Content, forbidden) {
			t.Fatalf("host-only checkpoint metadata leaked into provider projection: %q in %s", forbidden, blocks[0].Content)
		}
	}
}

func TestCompactCheckpointProviderProjectionBoundsLongValuesWithHeadAndTail(t *testing.T) {
	value := "HEAD-CANARY " + strings.Repeat("中间详情", 200) + " TAIL-BREAKPOINT"
	projection := taskCheckpointProviderProjection(TaskCheckpointV2{
		ActiveTask: value,
		PendingUserAsks: []CheckpointFact{
			newCheckpointFact("pending_user_ask", value, "critical", true, []string{"rec_long"}),
		},
	})
	if !strings.Contains(projection.ActiveTask, "HEAD-CANARY") || !strings.Contains(projection.ActiveTask, "TAIL-BREAKPOINT") ||
		len([]rune(projection.ActiveTask)) > 240 {
		t.Fatalf("provider value did not retain bounded head/tail: %q", projection.ActiveTask)
	}
	if len(projection.PendingUserAsks) != 1 || projection.PendingUserAsks[0] != "$ref:active_task" {
		t.Fatalf("bounded duplicate did not remain a semantic ref: %+v", projection.PendingUserAsks)
	}
}

func TestDeferPostCompactSkillProjectionKeepsReloadIdentity(t *testing.T) {
	ref := SkillContextRef{
		ID: "skill_block_documents", SkillID: "documents", SourcePath: "/skills/documents/SKILL.md",
		Compacted: true, ProjectedContent: "Active Skill reference after context compaction.",
	}
	if err := ref.Seal(); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := MaterializeSection(TaskCheckpointV2{
		ActiveTask: "continue", SkillRefs: []FactRef{{FactID: ref.SkillID, Digest: ref.Digest}},
	})
	if err != nil {
		t.Fatal(err)
	}
	bundle := PostCompactContextBundleV2{SkillRefs: []SkillContextRef{ref}, TaskCheckpoint: checkpoint}
	oldDigest := ref.Digest
	if err := deferPostCompactSkillProjection(&bundle); err != nil {
		t.Fatal(err)
	}
	if len(bundle.SkillRefs) != 1 || bundle.SkillRefs[0].ProjectedContent != "" || bundle.SkillRefs[0].Digest == oldDigest ||
		bundle.SkillRefs[0].SkillID != "documents" || bundle.SkillRefs[0].SourcePath != "/skills/documents/SKILL.md" {
		t.Fatalf("deferred Skill ref lost reload identity: %+v", bundle.SkillRefs)
	}
	if bundle.TaskCheckpoint.Value == nil || len(bundle.TaskCheckpoint.Value.SkillRefs) != 1 ||
		bundle.TaskCheckpoint.Value.SkillRefs[0].Digest != bundle.SkillRefs[0].Digest {
		t.Fatalf("checkpoint Skill ref did not track deferred identity: %+v", bundle.TaskCheckpoint)
	}
	if blocks := SkillContextBlocksFromBundleV2(&bundle); len(blocks) != 1 || blocks[0].Content != "" {
		t.Fatalf("deferred Skill projection was still provider-visible: %+v", blocks)
	}
}

func TestHardTargetUsesBoundedProjectionForHighVolumeDurableFacts(t *testing.T) {
	records := []SemanticRecord{{
		Seq: 1, RecordID: "rec_user", Kind: SemanticKindUserMessage,
		Payload: map[string]any{"content": "保留 canary 并继续批量读取文件"},
	}}
	for index := 0; index < 24; index++ {
		callSeq := uint64(index*2 + 2)
		path := fmt.Sprintf("/workspace/reports/report-%02d.docx", index)
		records = append(records,
			SemanticRecord{Seq: callSeq, RecordID: fmt.Sprintf("rec_call_%02d", index), Kind: SemanticKindToolCall, Payload: map[string]any{
				"tool_name": "Read", "arguments": fmt.Sprintf(`{"file_path":%q}`, path),
			}},
			SemanticRecord{Seq: callSeq + 1, RecordID: fmt.Sprintf("rec_result_%02d", index), Kind: SemanticKindToolResult, Payload: map[string]any{
				"tool_name": "Read", "tool_call_id": fmt.Sprintf("call_%02d", index),
				"content": fmt.Sprintf("report-%02d finding %s", index, strings.Repeat("detail ", 160)), "is_error": false,
			}},
		)
	}
	config := AgentConfig{SystemPrompt: "{{.RuntimeInfo}}", ModelConfig: ModelConfig{
		Model: "kimi-for-coding", Protocol: "anthropic_messages", ContextWindow: 160000,
	}}
	conversation := convcontext.NewConversation(nil)
	conversation.Context.SetMessages([]convcontext.Message{
		{Role: convcontext.RoleUser, Content: "保留 canary 并继续批量读取文件", MessageID: "msg_fact_volume"},
		{Role: convcontext.RoleAssistant, Content: "批量读取仍在继续"},
	})
	runner := NewRunner(config, conversation, NewRequestBuilder(config))
	runner.threadID = "thread_fact_volume"
	runner.recorder = &bundleRangeRecorder{sourceRecords: records}
	bundle, err := runner.buildPostCompactContextBundleV2(CompactResult{
		Mode: CompactModeAuto, BoundaryID: "compact_fact_volume",
		Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "bounded semantic summary", CompactBoundaryID: "compact_fact_volume"}},
	}, nil)
	if err != nil {
		t.Fatalf("host metadata made a reachable provider projection exceed the hard target: %v", err)
	}
	if bundle.TaskCheckpoint.Value == nil || len(bundle.TaskCheckpoint.Value.ToolFindings) != 48 ||
		bundle.BudgetReceipt.Value == nil || bundle.BudgetReceipt.Value.ConservativeProjected > bundle.BudgetReceipt.Value.CompactTargetTokens {
		t.Fatalf("durable facts or hard-target receipt were not preserved: checkpoint=%+v receipt=%+v", bundle.TaskCheckpoint.Value, bundle.BudgetReceipt.Value)
	}
	blocks := CompactContextBlocksFromBundleV2(&bundle)
	if len(blocks) != 1 || strings.Contains(blocks[0].Content, "rec_result_00") ||
		!strings.Contains(blocks[0].Content, `"tool_findings":48`) {
		t.Fatalf("provider projection did not defer host-only high-volume metadata: %+v", blocks)
	}
}

func TestCheckpointGlobInventoryRemainsRetrievableWithoutPromotingEveryPath(t *testing.T) {
	paths := make([]string, 0, 120)
	for index := 0; index < 120; index++ {
		paths = append(paths, fmt.Sprintf("/workspace/reports/report-%03d.pdf", index))
	}
	facts := checkpointFactsForMessage(convcontext.Message{
		Role: convcontext.RoleTool, Name: "Glob", ToolCallID: "call_glob",
		Content: strings.Join(paths, "\n"),
	}, []string{"rec_glob"}, 10)
	relevant := 0
	toolFindings := 0
	for _, fact := range facts {
		switch fact.Kind {
		case "relevant_file":
			relevant++
		case "tool_finding":
			toolFindings++
		}
	}
	if relevant != 0 || toolFindings != 1 {
		t.Fatalf("Glob inventory polluted the active relevant-file set: relevant=%d findings=%d facts=%+v", relevant, toolFindings, facts)
	}
	readFacts := checkpointFactsForMessage(convcontext.Message{
		Role: convcontext.RoleTool, Name: "Read", ToolCallID: "call_read", Content: paths[0],
	}, []string{"rec_read"}, 11)
	foundReadPath := false
	for _, fact := range readFacts {
		if fact.Kind == "relevant_file" && fact.NormalizedValue == paths[0] {
			foundReadPath = true
		}
	}
	if !foundReadPath {
		t.Fatalf("explicit Read result path was not retained: %+v", readFacts)
	}
}

func TestCheckpointPathsExcludeInternalToolResultStorageKeys(t *testing.T) {
	paths := checkpointPaths(`{"file_path":"/workspace/report.pdf","storage_key":"thread_123/tool-results/root/run/call/part-002.png"}`)
	foundBusinessPath := false
	for _, path := range paths {
		if strings.Contains(path, "/tool-results/") {
			t.Fatalf("internal model-content storage key polluted relevant files: %+v", paths)
		}
		if path == "/workspace/report.pdf" {
			foundBusinessPath = true
		}
	}
	if !foundBusinessPath {
		t.Fatalf("business file path was lost while filtering storage keys: %+v", paths)
	}
}

func TestCheckpointToolErrorsUseCanonicalOutcomeInsteadOfJSONKeyHeuristics(t *testing.T) {
	success := checkpointFactsForMessage(convcontext.Message{
		Role: convcontext.RoleTool, Name: "Bash", ToolCallID: "call_success",
		Content: `{"is_error":false,"status":"succeeded","error_message":""}`,
	}, []string{"rec_success"}, 1)
	for _, fact := range success {
		if fact.Kind == "error" || fact.Kind == "blocker" {
			t.Fatalf("successful canonical tool outcome became a critical failure: %+v", success)
		}
	}
	failed := checkpointFactsForMessage(convcontext.Message{
		Role: convcontext.RoleTool, Name: "Bash", ToolCallID: "call_failed", IsError: true,
		Content: "command failed with exit 2",
	}, []string{"rec_failed"}, 2)
	hasError := false
	hasBlocker := false
	for _, fact := range failed {
		hasError = hasError || fact.Kind == "error"
		hasBlocker = hasBlocker || fact.Kind == "blocker"
	}
	if !hasError || !hasBlocker {
		t.Fatalf("failed canonical tool outcome lost critical failure facts: %+v", failed)
	}
}

func TestContextCompactionV2FeatureFlagFallsBackToReadableV1Record(t *testing.T) {
	t.Setenv("AWAKEN_CONTEXT_COMPACTION_V2", "false")
	conversation := convcontext.NewConversation(nil)
	conversation.Context.SetMessages([]convcontext.Message{{Role: convcontext.RoleUser, Content: "old"}})
	resets := 0
	config := AgentConfig{MemoryRecallReset: func() { resets++ }}
	runner := NewRunner(config, conversation, NewRequestBuilder(config))
	recorder := &failingRecorder{}
	runner.recorder = recorder
	result := CompactResult{Mode: CompactModeAuto, BoundaryID: "compact_v1_fallback", Messages: []convcontext.Message{{
		Role: convcontext.RoleUser, Content: "v1 summary", CompactBoundaryID: "compact_v1_fallback",
	}}}
	if err := runner.contextGovernor.applyCompactResult(context.Background(), result); err != nil {
		t.Fatalf("apply v1 fallback: %v", err)
	}
	if len(recorder.records) != 1 || recorder.records[0].Kind != SemanticKindContextCompacted || resets != 1 {
		t.Fatalf("feature-flag fallback mismatch: records=%+v resets=%d", recorder.records, resets)
	}
}

func TestContextCompactionV2KeepsLegacyRecorderOnV1Boundary(t *testing.T) {
	conversation := convcontext.NewConversation(nil)
	conversation.Context.SetMessages([]convcontext.Message{{Role: convcontext.RoleUser, Content: "old"}})
	config := AgentConfig{}
	runner := NewRunner(config, conversation, NewRequestBuilder(config))
	recorder := &legacyBundleTestRecorder{}
	runner.recorder = recorder
	result := CompactResult{Mode: CompactModeAuto, BoundaryID: "compact_legacy_recorder", Messages: []convcontext.Message{{
		Role: convcontext.RoleUser, Content: "legacy summary", CompactBoundaryID: "compact_legacy_recorder",
	}}}
	if err := runner.contextGovernor.applyCompactResult(context.Background(), result); err != nil {
		t.Fatalf("apply legacy recorder compact: %v", err)
	}
	if len(recorder.records) != 1 || recorder.records[0].Kind != SemanticKindContextCompacted {
		t.Fatalf("legacy recorder received unsupported V2 record: %+v", recorder.records)
	}
	if runner.config.RestoredContextBundleV2 != nil {
		t.Fatalf("legacy recorder must not install a V2-only live projection: %+v", runner.config.RestoredContextBundleV2)
	}
}

type legacyBundleTestRecorder struct {
	failingRecorder
}

func (*legacyBundleTestRecorder) UsesLegacyContextCompactionRecord() bool { return true }

type bundleRangeRecorder struct {
	failingRecorder
	sourceRecords []SemanticRecord
}

func (recorder *bundleRangeRecorder) LastAssignedSequence() uint64 {
	if len(recorder.sourceRecords) == 0 {
		return 0
	}
	return recorder.sourceRecords[len(recorder.sourceRecords)-1].Seq
}

func (recorder *bundleRangeRecorder) SemanticRecords(fromSeq, toSeq uint64) ([]SemanticRecord, error) {
	result := make([]SemanticRecord, 0)
	for _, record := range recorder.sourceRecords {
		if record.Seq >= fromSeq && record.Seq <= toSeq {
			result = append(result, record)
		}
	}
	return result, nil
}

type bundleTestController struct {
	tasks      []subagentruntime.TaskSnapshot
	drainCalls int
}

func (*bundleTestController) Spawn(context.Context, subagentruntime.SpawnRequest) (subagentruntime.TaskSnapshot, error) {
	return subagentruntime.TaskSnapshot{}, nil
}
func (*bundleTestController) SpawnFromHost(context.Context, subagentruntime.SpawnRequest) (subagentruntime.TaskSnapshot, error) {
	return subagentruntime.TaskSnapshot{}, nil
}
func (controller *bundleTestController) Status(id string) (subagentruntime.TaskSnapshot, error) {
	for _, task := range controller.tasks {
		if task.TaskID == id {
			return task, nil
		}
	}
	return subagentruntime.TaskSnapshot{}, nil
}
func (controller *bundleTestController) List() []subagentruntime.TaskSnapshot {
	return append([]subagentruntime.TaskSnapshot(nil), controller.tasks...)
}
func (*bundleTestController) Wait(context.Context, subagentruntime.WaitRequest) (subagentruntime.WaitResult, error) {
	return subagentruntime.WaitResult{}, nil
}
func (controller *bundleTestController) DrainTerminalNotifications() []subagentruntime.TaskSnapshot {
	controller.drainCalls++
	return append([]subagentruntime.TaskSnapshot(nil), controller.tasks...)
}
func (*bundleTestController) AcknowledgeTerminal(string) {}
func (*bundleTestController) Message(context.Context, string, string) (subagentruntime.MessageReceipt, error) {
	return subagentruntime.MessageReceipt{}, nil
}
func (*bundleTestController) Cancel(context.Context, string, string) (subagentruntime.TaskSnapshot, error) {
	return subagentruntime.TaskSnapshot{}, nil
}
func (*bundleTestController) Close(context.Context) error { return nil }
