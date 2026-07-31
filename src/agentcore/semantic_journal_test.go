package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/skillruntime"
	"awaken/src/skills"
	"awaken/src/tool"
)

type failingRecorder struct {
	failToolCall   bool
	failToolResult bool
	failSkill      bool
	failUser       bool
	failFlush      bool
	flushes        int
	records        []SemanticRecord
	batches        [][]SemanticKind
}

func (r *failingRecorder) Append(ctx context.Context, record SemanticRecord) error {
	return r.AppendBatch(ctx, []SemanticRecord{record})
}

func (r *failingRecorder) AppendBatch(_ context.Context, records []SemanticRecord) error {
	for _, record := range records {
		if r.failUser && record.Kind == SemanticKindUserMessage {
			return errors.New("disk full while writing user_message")
		}
		if r.failToolCall && record.Kind == SemanticKindToolCall {
			return errors.New("disk full")
		}
		if r.failToolResult && record.Kind == SemanticKindToolResult {
			return errors.New("disk full while writing tool_result")
		}
		if r.failSkill && record.Kind == SemanticKindSkillActivated {
			return errors.New("disk full while writing skill activation")
		}
	}
	kinds := make([]SemanticKind, 0, len(records))
	for _, record := range records {
		kinds = append(kinds, record.Kind)
	}
	r.batches = append(r.batches, kinds)
	r.records = append(r.records, records...)
	return nil
}

func TestUserMessagePersistenceFailureIsVisibleAndDoesNotAdvanceConversation(t *testing.T) {
	recorder := &failingRecorder{failUser: true}
	agent := NewAgentWithThreadID(AgentConfig{}, "thread_user_persist")
	agent.SetRunIdentity("run_user_persist", "session_user_persist")
	agent.SetSemanticRecorder(recorder)
	var events []AwakenEvent

	_, err := agent.RunStreamWithEvents(context.Background(), "must be durable", EventSinkFunc(func(event AwakenEvent) {
		events = append(events, event)
	}))
	if !errors.Is(err, ErrPersistenceFailed) {
		t.Fatalf("expected typed persistence failure, got %v", err)
	}
	if messages := agent.GetConversation().Context.GetMessages(); len(messages) != 0 {
		t.Fatalf("unpersisted user message leaked into conversation: %+v", messages)
	}
	if len(events) != 1 || events[0].Type != RunErrorEventType ||
		events[0].Code != "PERSISTENCE_ERROR" || events[0].TerminalReason != TerminalReasonPersistenceError {
		t.Fatalf("missing user-visible persistence event: %+v", events)
	}
}

func TestUserMessagePersistsStructuredAttachmentsAndComposerTokens(t *testing.T) {
	server := newRunnerStreamServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeRunnerStreamText(t, w, "done")
	})
	defer server.Close()

	recorder := &failingRecorder{}
	agent := NewAgentWithThreadID(testAgentConfig(server.URL, 1), "thread_structured_input")
	agent.SetRunIdentity("run_structured_input", "session_structured_input")
	agent.SetSemanticRecorder(recorder)
	agent.SetPendingUserAttachments([]convcontext.MessageAttachment{
		{
			ID:          "image-1",
			Kind:        "image",
			MimeType:    "image/png",
			DataBase64:  "must-not-enter-journal",
			DisplayName: "clipboard.png",
			Width:       120,
			Height:      80,
			ByteSize:    42,
			StorageKey:  "thread_structured_input/image-1.png",
		},
		{
			ID:          "file-1",
			Kind:        "file",
			MimeType:    "text/plain",
			Path:        "/tmp/acceptance.txt",
			DisplayName: "acceptance.txt",
			ByteSize:    17,
		},
	})
	agent.SetPendingComposerTokens([]SemanticComposerToken{{
		ID:          "skill:docs:user",
		Type:        "skill",
		LeadingText: "先阅读附件，然后使用 ",
		Label:       "docs",
		Payload: map[string]any{
			"skillId":      "docs",
			"sourceOrigin": "user",
		},
	}})

	if _, err := agent.RunStreamWithEvents(context.Background(), "inspect inputs", nil); err != nil {
		t.Fatalf("RunStreamWithEvents: %v", err)
	}

	var userRecord *SemanticRecord
	for i := range recorder.records {
		if recorder.records[i].Kind == SemanticKindUserMessage {
			userRecord = &recorder.records[i]
			break
		}
	}
	if userRecord == nil {
		t.Fatal("missing durable user_message")
	}
	encoded, err := json.Marshal(userRecord.Payload)
	if err != nil {
		t.Fatalf("marshal user_message payload: %v", err)
	}
	if strings.Contains(string(encoded), "must-not-enter-journal") || strings.Contains(string(encoded), "data_base64") {
		t.Fatalf("journal duplicated inline image bytes: %s", encoded)
	}
	var payload struct {
		AttachmentCount int                             `json:"attachment_count"`
		Attachments     []SemanticUserMessageAttachment `json:"attachments"`
		ComposerTokens  []SemanticComposerToken         `json:"composer_tokens"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode user_message payload: %v", err)
	}
	if payload.AttachmentCount != 2 || len(payload.Attachments) != 2 {
		t.Fatalf("attachment identity/order missing: %+v", payload)
	}
	if payload.Attachments[0].ID != "image-1" || payload.Attachments[0].StorageKey != "thread_structured_input/image-1.png" ||
		payload.Attachments[1].ID != "file-1" || payload.Attachments[1].Path != "/tmp/acceptance.txt" {
		t.Fatalf("unexpected durable attachments: %+v", payload.Attachments)
	}
	if len(payload.ComposerTokens) != 1 || payload.ComposerTokens[0].ID != "skill:docs:user" ||
		payload.ComposerTokens[0].Payload["skillId"] != "docs" {
		t.Fatalf("composer token identity missing: %+v", payload.ComposerTokens)
	}
	if payload.ComposerTokens[0].LeadingText != "先阅读附件，然后使用 " {
		t.Fatalf("composer token leadingText missing from durable payload: %+v", payload.ComposerTokens)
	}
}

func TestPreparedUserMessageSeparatesModelAndDisplayInput(t *testing.T) {
	server := newRunnerStreamServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeRunnerStreamText(t, w, "done")
	})
	defer server.Close()

	recorder := &failingRecorder{}
	agent := NewAgentWithThreadID(testAgentConfig(server.URL, 1), "thread_hidden_capability")
	agent.SetRunIdentity("run_hidden_capability", "session_hidden_capability")
	agent.SetSemanticRecorder(recorder)
	agent.SetPendingComposerTokens([]SemanticComposerToken{{
		ID:    "capability:computer-use",
		Type:  "capability",
		Label: "Computer Use",
		Payload: map[string]any{
			"capabilityId": "computer-use",
		},
	}})

	const modelInput = "hidden computer instruction\n打开计算器"
	const displayInput = "打开计算器"
	if err := agent.PrepareUserMessageWithDisplayInput(
		context.Background(),
		modelInput,
		displayInput,
		nil,
	); err != nil {
		t.Fatalf("PrepareUserMessageWithDisplayInput: %v", err)
	}
	if _, err := agent.RunStreamWithEvents(context.Background(), modelInput, nil); err != nil {
		t.Fatalf("RunStreamWithEvents: %v", err)
	}

	for _, record := range recorder.records {
		if record.Kind != SemanticKindUserMessage {
			continue
		}
		if got := record.Payload["content"]; got != displayInput {
			t.Fatalf("durable user_message exposed model input: %#v", got)
		}
		tokens, ok := record.Payload["composer_tokens"].([]SemanticComposerToken)
		if !ok || len(tokens) != 1 || tokens[0].Payload["capabilityId"] != "computer-use" {
			t.Fatalf("capability identity missing from durable user_message: %#v", record.Payload)
		}
		return
	}
	t.Fatal("missing durable user_message")
}

func TestToolResultBatchPersistsAppliedFileChangeAtomically(t *testing.T) {
	recorder := &failingRecorder{}
	events := newRunEventEmitter("thread_file_change", EventSinkFunc(func(AwakenEvent) {}))
	events.runID = "run_file_change"
	events.recorder = recorder
	result := ToolExecutionResult{
		ToolCallID:    "call_file_change",
		Name:          "text_edit",
		Status:        ToolExecutionSucceeded,
		AffectedPaths: []string{"notes.txt"},
		PatchSummary:  "replace one occurrence",
		Outcome: tool.ExecutionOutcome{
			Status: tool.OutcomeSucceeded,
		},
	}

	if err := events.emitToolResultBatch(context.Background(), result, nil, nil); err != nil {
		t.Fatalf("emitToolResultBatch: %v", err)
	}
	if len(recorder.batches) != 1 || len(recorder.batches[0]) != 2 ||
		recorder.batches[0][0] != SemanticKindToolResult || recorder.batches[0][1] != SemanticKindFileChange {
		t.Fatalf("tool_result and file_change must share one batch: %+v", recorder.batches)
	}
	change := recorder.records[1]
	if change.ParentID != result.ToolCallID || change.Payload["path"] != "notes.txt" ||
		change.Payload["status"] != "applied" {
		t.Fatalf("unexpected file_change record: %+v", change)
	}
}

func TestBufferedPermissionAndRuntimeFactsPersistWithToolResult(t *testing.T) {
	recorder := &failingRecorder{}
	var live []AwakenEvent
	buffer := &artifactBufferingSink{inner: EventSinkFunc(func(event AwakenEvent) {
		live = append(live, event)
	})}
	buffer.Emit(AwakenEvent{
		Type:                     ToolPermissionDecidedEventType,
		ToolCallID:               "call_runtime",
		ToolName:                 "presentation_create",
		ToolDecisionAction:       "allow",
		ToolDecisionSource:       "static_policy",
		ToolDecisionReason:       "workspace write allowed",
		StaticPermissionBehavior: "allow",
	})
	buffer.Emit(AwakenEvent{
		Type:           RuntimeReadyEventType,
		ToolCallID:     "call_runtime",
		ToolName:       "presentation_create",
		Content:        "runtime ready",
		RuntimeName:    "primary-runtime",
		RuntimeVersion: "0.2.0",
	})
	if len(live) != 0 {
		t.Fatalf("durable permission/runtime facts must not publish before journal append: %+v", live)
	}

	events := newRunEventEmitter("thread_runtime", EventSinkFunc(func(event AwakenEvent) {
		live = append(live, event)
	}))
	events.runID = "run_runtime"
	events.recorder = recorder
	result := ToolExecutionResult{
		ToolCallID: "call_runtime",
		Name:       "presentation_create",
		Status:     ToolExecutionSucceeded,
		Outcome:    tool.ExecutionOutcome{Status: tool.OutcomeSucceeded},
	}
	if err := events.emitToolResultBatch(context.Background(), result, nil, buffer.semanticFacts); err != nil {
		t.Fatalf("emitToolResultBatch: %v", err)
	}
	if len(live) != 3 || live[0].Type != ToolPermissionDecidedEventType ||
		live[1].Type != RuntimeReadyEventType || live[2].Type != ToolResultEventType {
		t.Fatalf("semantic facts must publish once after successful append in record order: %+v", live)
	}
	want := []SemanticKind{SemanticKindPermissionDecision, SemanticKindRuntimeNotice, SemanticKindToolResult}
	if len(recorder.batches) != 1 || len(recorder.batches[0]) != len(want) {
		t.Fatalf("semantic facts must share one tool result batch: %+v", recorder.batches)
	}
	for i, kind := range want {
		if recorder.batches[0][i] != kind {
			t.Fatalf("batch[%d]=%s want %s: %+v", i, recorder.batches[0][i], kind, recorder.batches)
		}
	}
}

func TestBufferedPermissionAndRuntimeFactsStayInvisibleWhenBatchAppendFails(t *testing.T) {
	recorder := &failingRecorder{failToolResult: true}
	buffer := &artifactBufferingSink{inner: EventSinkFunc(func(AwakenEvent) {
		t.Fatal("buffer published a durable fact before journal append")
	})}
	buffer.Emit(AwakenEvent{Type: ToolPermissionDecidedEventType, ToolCallID: "call_fail", ToolName: "shell"})
	buffer.Emit(AwakenEvent{Type: RuntimeReadyEventType, ToolCallID: "call_fail", ToolName: "shell"})

	var live []AwakenEvent
	events := newRunEventEmitter("thread_fail", EventSinkFunc(func(event AwakenEvent) {
		live = append(live, event)
	}))
	events.runID = "run_fail"
	events.recorder = recorder
	err := events.emitToolResultBatch(context.Background(), ToolExecutionResult{
		ToolCallID: "call_fail",
		Name:       "shell",
		Outcome:    tool.ExecutionOutcome{Status: tool.OutcomeFailed},
	}, nil, buffer.semanticFacts)
	if err == nil {
		t.Fatal("expected atomic tool-result batch append failure")
	}
	for _, event := range live {
		if event.Type == ToolPermissionDecidedEventType || event.Type == RuntimeReadyEventType || event.Type == ToolResultEventType {
			t.Fatalf("uncommitted durable fact became live: %+v", live)
		}
	}
}

func TestImplicitSkillActivationPersistsBeforeExecutionWithoutNarrowingPool(t *testing.T) {
	recorder := &failingRecorder{}
	allowedName := "implicit_skill_allowed"
	skillID := "journal-implicit"
	skillName := skillruntime.CatalogToolName
	mustRegisterRunnerTool(t, countingJournalTool{name: allowedName, calls: new(int)})
	t.Cleanup(func() { tool.UnregisterTool(allowedName) })

	root := t.TempDir()
	skillRoot := filepath.Join(root, skillID)
	if err := os.MkdirAll(skillRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	definition := "---\nname: " + skillID + "\ndescription: durable implicit skill\ntoolPolicy:\n  allow: [" + allowedName + "]\n---\nActivated body\n"
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(definition), 0o644); err != nil {
		t.Fatal(err)
	}
	registry := skills.NewSkillRegistry(nil)
	registry.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	executed := false
	skillTool := &skillruntime.CatalogTool{
		Registry:  registry,
		Activator: skillruntime.NewActivator(registry, nil),
		Executor: skillruntime.SkillExecutorFunc(func(_ context.Context, _ string, body string, allowed []string) (string, error) {
			executed = true
			if !semanticKindsContain(recorder.records, SemanticKindSkillActivated) {
				t.Fatal("skill executor ran before activation was durable")
			}
			if strings.Join(allowed, ",") != allowedName {
				t.Fatalf("executor should receive non-enforcing author hints only: %v", allowed)
			}
			if !strings.Contains(body, "Activated body") {
				t.Fatalf("missing activated body: %q", body)
			}
			return "activated", nil
		}),
	}
	mustRegisterRunnerTool(t, skillTool)
	t.Cleanup(func() { tool.UnregisterTool(skillName) })

	config := AgentConfig{}
	events := newRunEventEmitter("thread_skill", EventSinkFunc(func(AwakenEvent) {}))
	events.runID = "run_skill"
	events.recorder = recorder
	runner := &Runner{
		config:            config,
		recorder:          recorder,
		threadID:          "thread_skill",
		runID:             "run_skill",
		failureSupervisor: NewFailureSupervisor(t.TempDir(), 8, 32),
		toolRuntime:       NewToolRuntimeWithConfig(events.sink, ToolRuntimeConfig{}),
	}
	results := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{{
		ID: "call_skill",
		Function: convcontext.FunctionCall{
			Name:      skillName,
			Arguments: `{"skill":"journal-implicit"}`,
		},
	}}, events)

	if len(results) != 1 || results[0].IsError || !executed {
		t.Fatalf("implicit skill did not execute successfully: executed=%v results=%+v", executed, results)
	}
	if len(runner.config.SkillContextBlocks) != 1 || !strings.Contains(runner.config.SkillContextBlocks[0].Content, "Activated body") {
		t.Fatalf("activation context not installed: %+v", runner.config.SkillContextBlocks)
	}
	activationIndex := semanticKindIndex(recorder.records, SemanticKindSkillActivated)
	resultIndex := semanticKindIndex(recorder.records, SemanticKindToolResult)
	if activationIndex < 0 || resultIndex < 0 || activationIndex >= resultIndex {
		t.Fatalf("activation must precede tool result: %+v", recorder.records)
	}
}

func TestImplicitSkillActivationPersistenceFailurePreventsExecutor(t *testing.T) {
	recorder := &failingRecorder{failSkill: true}
	skillID := "journal-persist-failure"
	skillName := skillruntime.CatalogToolName
	root := t.TempDir()
	skillRoot := filepath.Join(root, skillID)
	if err := os.MkdirAll(skillRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte("---\nname: "+skillID+"\ndescription: persistence gate\n---\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry := skills.NewSkillRegistry(nil)
	registry.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	executed := false
	mustRegisterRunnerTool(t, &skillruntime.CatalogTool{
		Registry:  registry,
		Activator: skillruntime.NewActivator(registry, nil),
		Executor: skillruntime.SkillExecutorFunc(func(context.Context, string, string, []string) (string, error) {
			executed = true
			return "must not run", nil
		}),
	})
	t.Cleanup(func() { tool.UnregisterTool(skillName) })

	config := AgentConfig{}
	events := newRunEventEmitter("thread_skill_fail", EventSinkFunc(func(AwakenEvent) {}))
	events.runID = "run_skill_fail"
	events.recorder = recorder
	runner := &Runner{
		config:      config,
		recorder:    recorder,
		threadID:    "thread_skill_fail",
		runID:       "run_skill_fail",
		toolRuntime: NewToolRuntimeWithConfig(events.sink, ToolRuntimeConfig{}),
	}
	results := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{{
		ID:       "call_skill_fail",
		Function: convcontext.FunctionCall{Name: skillName, Arguments: `{"skill":"journal-persist-failure"}`},
	}}, events)

	if executed {
		t.Fatal("skill executor ran before activation persistence succeeded")
	}
	if len(results) != 1 || results[0].ErrorCode != string(TerminalReasonPersistenceError) {
		t.Fatalf("expected typed persistence failure result: %+v", results)
	}
	if !errors.Is(events.persistenceFailure(), ErrPersistenceFailed) {
		t.Fatalf("expected emitter persistence failure, got %v", events.persistenceFailure())
	}
	if semanticKindsContain(recorder.records, SemanticKindToolResult) {
		t.Fatalf("tool result must not be written after activation persistence failure: %+v", recorder.records)
	}
}

func semanticKindsContain(records []SemanticRecord, kind SemanticKind) bool {
	return semanticKindIndex(records, kind) >= 0
}

func semanticKindIndex(records []SemanticRecord, kind SemanticKind) int {
	for index, record := range records {
		if record.Kind == kind {
			return index
		}
	}
	return -1
}

func (r *failingRecorder) Flush(context.Context) error {
	r.flushes++
	if r.failFlush {
		return errors.New("fsync failed")
	}
	return nil
}

func TestContextEpochJournalRecordContainsOnlyRedactedReceipt(t *testing.T) {
	recorder := &failingRecorder{}
	emitter := newRunEventEmitter("thread-epoch", EventSinkFunc(func(AwakenEvent) {}))
	emitter.recorder = recorder
	if err := emitter.contextEpoch(context.Background(), ContextEpochReceipt{
		EpochID: "epoch-1", BaselineDigest: "digest-1", State: "updated",
		BoundaryReason: "append_only_host_delta", BaselineSequence: 2,
	}); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(recorder.records)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "prompt body") || strings.Contains(string(encoded), "workspace path") {
		t.Fatalf("context epoch receipt leaked sidecar content: %s", encoded)
	}
	if len(recorder.records) != 1 || recorder.records[0].Kind != SemanticKindContextEpoch {
		t.Fatalf("unexpected epoch records: %+v", recorder.records)
	}
}

func TestModelRequestIsFlushedBeforeProviderAndUsageReusesRequestID(t *testing.T) {
	recorder := &failingRecorder{}
	server := newRunnerStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		if recorder.flushes == 0 {
			t.Fatal("provider request started before model_request fsync")
		}
		if semanticKindIndex(recorder.records, SemanticKindModelRequest) < 0 {
			t.Fatal("provider request started before model_request append")
		}
		writeRunnerStreamLines(t, w, []string{
			`data: {"choices":[{"delta":{"content":"done"}}],"usage":{"prompt_tokens":11,"completion_tokens":2,"total_tokens":13}}`,
			`data: [DONE]`,
		})
	})
	defer server.Close()

	agent := NewAgentWithThreadID(testAgentConfig(server.URL, 2), "thread_model_usage")
	agent.SetRunIdentity("run_model_usage", "session_model_usage")
	agent.SetSemanticRecorder(recorder)
	if _, err := agent.RunStreamWithEvents(context.Background(), "hello", EventSinkFunc(func(AwakenEvent) {})); err != nil {
		t.Fatalf("run: %v", err)
	}
	requestIndex := semanticKindIndex(recorder.records, SemanticKindModelRequest)
	usageIndex := semanticKindIndex(recorder.records, SemanticKindModelUsage)
	if requestIndex < 0 || usageIndex <= requestIndex {
		t.Fatalf("model request/usage order is invalid: %+v", recorder.records)
	}
	requestID := strings.TrimSpace(fmt.Sprint(recorder.records[requestIndex].Payload["request_id"]))
	usageID := strings.TrimSpace(fmt.Sprint(recorder.records[usageIndex].Payload["request_id"]))
	if requestID == "" || requestID != usageID || recorder.records[usageIndex].ParentID != requestID {
		t.Fatalf("usage did not reuse request identity: request=%q usage=%q parent=%q", requestID, usageID, recorder.records[usageIndex].ParentID)
	}
	if semanticKindIndex(recorder.records, SemanticKindContextSnapshot) < 0 {
		t.Fatal("context snapshots were not persisted")
	}
}

func TestContextCompactionIsFlushedBeforeConversationChanges(t *testing.T) {
	original := []convcontext.Message{{Role: convcontext.RoleUser, Content: "old history"}}
	compacted := []convcontext.Message{{
		Role:              convcontext.RoleUser,
		Content:           "durable summary",
		CompactBoundaryID: "compact_durable",
	}}
	conversation := convcontext.NewConversation(nil)
	conversation.Context.SetMessages(original)
	runner := NewRunner(AgentConfig{}, conversation, NewRequestBuilder(AgentConfig{}))
	runner.threadID = "thread_compact"
	runner.runID = "run_compact"
	runner.sessionID = "session_compact"
	recorder := &failingRecorder{}
	runner.recorder = recorder

	if err := runner.persistContextCompacted(context.Background(), CompactResult{
		Mode:               CompactModeAuto,
		BoundaryID:         "compact_durable",
		Messages:           compacted,
		PreEstimateTokens:  90000,
		PostEstimateTokens: 12000,
	}); err != nil {
		t.Fatalf("persistContextCompacted: %v", err)
	}
	if recorder.flushes != 1 || len(recorder.records) != 1 || recorder.records[0].Kind != SemanticKindContextCompacted {
		t.Fatalf("compaction was not appended and flushed exactly once: records=%+v flushes=%d", recorder.records, recorder.flushes)
	}

	failing := &failingRecorder{failFlush: true}
	runner.recorder = failing
	if err := runner.contextGovernor.applyCompactResult(context.Background(), CompactResult{
		BoundaryID: "compact_unconfirmed",
		Messages:   compacted,
	}); err == nil {
		t.Fatal("expected compaction flush failure")
	}
	got := conversation.Context.GetMessages()
	if len(got) != 1 || got[0].Content != "old history" || got[0].CompactBoundaryID != "" {
		t.Fatalf("unconfirmed compaction replaced in-memory truth: %+v", got)
	}
}

func TestEmitToolCallPersistFailureBlocksBeforeExecution(t *testing.T) {
	recorder := &failingRecorder{failToolCall: true}
	events := newRunEventEmitter("thread_test", EventSinkFunc(func(AwakenEvent) {}))
	events.recorder = recorder
	events.runID = "run_test"

	err := events.emitToolCall(context.Background(), convcontext.ToolCall{
		ID: "call_1",
		Function: convcontext.FunctionCall{
			Name:      "shell",
			Arguments: `{"command":"echo hi"}`,
		},
	})
	if err == nil {
		t.Fatalf("expected persist failure")
	}
}

func TestPersistDurableRequiresRecorder(t *testing.T) {
	err := PersistDurable(context.Background(), nil, EventSinkFunc(func(AwakenEvent) {}), NewSemanticRecord(
		SemanticKindRunStarted, "t", "r", "", "", "", map[string]any{},
	))
	if !errors.Is(err, ErrPersistenceUnavailable) {
		t.Fatalf("expected ErrPersistenceUnavailable, got %v", err)
	}
}

func TestExecuteToolCallsSkipsExecutionAfterPersistFailure(t *testing.T) {
	recorder := &failingRecorder{failToolCall: true}
	progress := 0
	runner := &Runner{
		recorder: recorder,
		threadID: "thread_test",
		runID:    "run_test",
		toolRuntime: NewToolRuntimeWithConfig(EventSinkFunc(func(event AwakenEvent) {
			progress++
		}), ToolRuntimeConfig{}),
	}
	events := newRunEventEmitter("thread_test", EventSinkFunc(func(AwakenEvent) {}))
	events.recorder = recorder
	events.runID = "run_test"

	results := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{{
		ID: "call_1",
		Function: convcontext.FunctionCall{
			Name:      "shell",
			Arguments: `{"command":"echo hi"}`,
		},
	}}, events)
	if len(results) != 1 {
		t.Fatalf("expected 1 synthetic result, got %d", len(results))
	}
	if results[0].Status != ToolExecutionFailed || results[0].ErrorCode != string(TerminalReasonPersistenceError) {
		t.Fatalf("expected typed persistence failure result, got %+v", results[0])
	}
	if progress != 0 {
		t.Fatalf("expected no tool runtime emissions after persist failure, got %d", progress)
	}
}

type countingJournalTool struct {
	name  string
	calls *int
}

func (t countingJournalTool) Name() string        { return t.name }
func (t countingJournalTool) Description() string { return "counts real executions" }
func (t countingJournalTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{{Name: "value", Description: "value", Type: "string", Required: true}}
}
func (t countingJournalTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name: t.name, DisplayName: t.name, Source: tool.SourceBuiltin,
		Description: t.Description(), Annotations: tool.ToolAnnotations{ReadOnly: true},
	}
}
func (t countingJournalTool) Execute(context.Context, string) (string, error) {
	*t.calls++
	return "executed", nil
}

func TestToolResultPersistenceFailureStopsLaterExecutionsAndEmitsPersistenceError(t *testing.T) {
	toolName := "journal_side_effect_" + t.Name()
	var executions int
	mustRegisterRunnerTool(t, countingJournalTool{name: toolName, calls: &executions})
	recorder := &failingRecorder{failToolResult: true}
	var gotEvents []AwakenEvent
	events := newRunEventEmitter("thread_test", EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))
	events.recorder = recorder
	events.runID = "run_test"
	runner := &Runner{
		recorder:          recorder,
		threadID:          "thread_test",
		runID:             "run_test",
		failureSupervisor: NewFailureSupervisor(t.TempDir(), 8, 32),
		toolRuntime:       NewToolRuntimeWithConfig(events.sink, ToolRuntimeConfig{}),
	}
	results := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{
		{ID: "call_1", Function: convcontext.FunctionCall{Name: toolName, Arguments: `{"value":"one"}`}},
		{ID: "call_2", Function: convcontext.FunctionCall{Name: toolName, Arguments: `{"value":"two"}`}},
	}, events)
	if executions != 1 {
		t.Fatalf("journal failure must stop later real executions, got %d", executions)
	}
	if len(results) != 2 || results[1].ErrorCode != string(TerminalReasonPersistenceError) {
		t.Fatalf("expected second call to be synthetic persistence failure: %+v", results)
	}
	if !errors.Is(events.persistenceFailure(), ErrPersistenceFailed) {
		t.Fatalf("missing typed persistence failure: %v", events.persistenceFailure())
	}
	found := false
	for _, event := range gotEvents {
		if event.Type == RunErrorEventType && event.TerminalReason == TerminalReasonPersistenceError && event.Code == "PERSISTENCE_ERROR" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing user-visible persistence_error event: %+v", gotEvents)
	}
}

func TestTerminalFlushFailureCannotReportSuccessfulRun(t *testing.T) {
	recorder := &failingRecorder{failFlush: true}
	var gotEvents []AwakenEvent
	events := newRunEventEmitter("thread_flush", EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))
	events.recorder = recorder
	events.runID = "run_flush"
	events.runFinished(context.Background(), "done", nil, TerminalReasonCompleted)
	if !errors.Is(events.persistenceFailure(), ErrPersistenceFailed) ||
		!strings.Contains(events.persistenceFailure().Error(), "run_terminal_finished_flush") {
		t.Fatalf("flush failure was not promoted: %v", events.persistenceFailure())
	}
	for _, event := range gotEvents {
		if event.Type == RunFinishedEventType {
			t.Fatalf("flush failure must not broadcast successful run terminal: %+v", gotEvents)
		}
	}
}

func TestToolDisplayContentAndModelPayloadRemainSeparate(t *testing.T) {
	outcome := tool.FailedOutcome("human-readable failure", "typed_failure", tool.ErrorCategoryValidation, false)
	result := ToolExecutionResult{
		ToolCallID: "call_1",
		Name:       "typed_tool",
		Content:    "human-readable failure",
		IsError:    true,
		Status:     ToolExecutionFailed,
		ErrorCode:  "typed_failure",
		Outcome:    outcome,
	}
	var got AwakenEvent
	events := newRunEventEmitter("thread_display", EventSinkFunc(func(event AwakenEvent) {
		if event.Type == ToolResultEventType {
			got = event
		}
	}))
	if err := events.emitToolResultBatch(context.Background(), result, nil, nil); err != nil {
		t.Fatalf("emitToolResultBatch: %v", err)
	}
	if got.Content != "human-readable failure" {
		t.Fatalf("UI event received model JSON instead of display content: %q", got.Content)
	}
	model := toolExecutionResultsToContext([]ToolExecutionResult{result})
	if len(model) != 1 || !strings.Contains(model[0].Content, `"error_code":"typed_failure"`) {
		t.Fatalf("model context did not receive structured payload: %+v", model)
	}
}
