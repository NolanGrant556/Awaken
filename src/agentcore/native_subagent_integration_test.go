package agentcore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/subagentruntime"
	"awaken/src/tool"
	_ "awaken/src/tools/read"
	_ "awaken/src/tools/subagent"
)

func TestNativeSubagentToolRunsReadOnlyChildAndReturnsHandoff(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "auth.txt"), []byte("auth/service.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := AgentConfig{
		Workspace: WorkspaceConfig{
			Root: root, ReadableRoots: []string{root}, WritableRoots: []string{root},
		},
		ToolExposure: tool.ToolExposurePolicy{Mode: tool.ExposureAll},
	}
	recorder := &forkCaptureRecorder{}
	runner := NewRunner(config, convcontext.NewConversation(nil), NewRequestBuilder(config))
	runner.threadID = "thread_subagent"
	runner.runID = "run_subagent"
	runner.sessionID = "session_subagent"
	runner.recorder = recorder
	var childEventsMu sync.Mutex
	var childEvents []AwakenEvent
	childSink := EventSinkFunc(func(event AwakenEvent) {
		childEventsMu.Lock()
		childEvents = append(childEvents, event)
		childEventsMu.Unlock()
	})
	childTurns := 0
	runner.streamLLMFunc = func(_ context.Context, events *runEventEmitter) (*llm.Response, error) {
		if events.agentID == "" || events.subagentTaskID == "" {
			t.Fatalf("native child identity missing: agent=%q task=%q", events.agentID, events.subagentTaskID)
		}
		request, err := runnerForEmitterRequest(events)
		if err != nil {
			t.Fatal(err)
		}
		_ = request
		childTurns++
		if childTurns == 1 {
			return &llm.Response{ToolCalls: []llm.ToolCall{{
				ID: "child-read", Name: "Read", Input: `{"file_path":"auth.txt"}`,
			}}}, nil
		}
		return &llm.Response{Content: "认证入口位于 auth/service.go"}, nil
	}
	manager := subagentruntime.NewManager(context.Background(), subagentruntime.Config{
		MaxConcurrent: 1,
		Starter: func(ctx context.Context, request subagentruntime.StartRequest) (string, error) {
			return runner.startNativeChild(ctx, request, childSink)
		},
		Persist: runner.persistSubagentEvent,
	})
	runner.subagentController = manager
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = manager.Close(closeCtx)
	})

	events := newRunEventEmitter(runner.threadID, nil)
	events.runID = runner.runID
	events.sessionID = runner.sessionID
	events.recorder = recorder
	runner.toolRuntime = NewToolRuntimeWithConfig(nil, toolRuntimeConfigFromAgentConfig(config))
	runner.toolRuntime.runID = runner.runID
	runner.toolRuntime.threadID = runner.threadID

	spawned := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{{
		ID: "call_spawn",
		Function: convcontext.FunctionCall{
			Name:      "subagent",
			Arguments: `{"action":"spawn","description":"定位认证入口","prompt":"只读检查认证入口","agent_type":"explore","workspace_mode":"shared_readonly"}`,
		},
	}}, events)
	if len(spawned) != 1 || spawned[0].IsError {
		t.Fatalf("spawn result = %+v", spawned)
	}
	var spawnPayload struct {
		Task subagentruntime.TaskSnapshot `json:"task"`
	}
	if err := json.Unmarshal([]byte(spawned[0].Content), &spawnPayload); err != nil {
		t.Fatalf("decode spawn result: %v", err)
	}

	waitArgs, _ := json.Marshal(map[string]any{
		"action": "wait", "task_ids": []string{spawnPayload.Task.TaskID},
		"return_when": "all", "timeout_ms": 1000,
	})
	waited := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{{
		ID:       "call_wait",
		Function: convcontext.FunctionCall{Name: "subagent", Arguments: string(waitArgs)},
	}}, events)
	if len(waited) != 1 || waited[0].IsError {
		t.Fatalf("wait result = %+v", waited)
	}
	var waitPayload struct {
		Tasks []subagentruntime.TaskSnapshot `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(waited[0].Content), &waitPayload); err != nil {
		t.Fatalf("decode wait result: %v", err)
	}
	if len(waitPayload.Tasks) != 1 || waitPayload.Tasks[0].Status != subagentruntime.StatusCompleted ||
		waitPayload.Tasks[0].ResultPreview != "认证入口位于 auth/service.go" || waitPayload.Tasks[0].ToolUseCount != 1 {
		t.Fatalf("wait payload = %+v", waitPayload)
	}

	var spawnedRecord, terminalRecord bool
	for _, record := range recorder.snapshot() {
		switch record.Kind {
		case SemanticKindSubagentSpawned:
			spawnedRecord = true
		case SemanticKindSubagentTerminal:
			terminalRecord = true
		}
	}
	if !spawnedRecord || !terminalRecord {
		t.Fatalf("missing durable lifecycle records: %+v", recorder.snapshot())
	}

	childEventsMu.Lock()
	capturedChildEvents := append([]AwakenEvent(nil), childEvents...)
	childEventsMu.Unlock()
	var sawToolProgress bool
	for _, event := range capturedChildEvents {
		switch event.Type {
		case ToolCallStartedEventType,
			ToolCallArgsEventType,
			ToolCallFinishedEventType,
			ToolProgressEventType,
			CommandOutputEventType,
			PatchUpdatedEventType,
			ToolPermissionDecidedEventType,
			ToolResultEventType:
			if event.Type == ToolProgressEventType {
				sawToolProgress = true
			}
			if event.SubagentTaskID != spawnPayload.Task.TaskID ||
				event.AgentID != spawnPayload.Task.AgentID {
				t.Fatalf("child tool event leaked without task identity: %+v", event)
			}
		}
	}
	if !sawToolProgress {
		t.Fatalf("expected child tool progress event, got %+v", capturedChildEvents)
	}
}

func TestRunScopedEventSinkStampsImmediateChildToolEvents(t *testing.T) {
	var captured AwakenEvent
	emitter := newRunEventEmitter("thread_child", EventSinkFunc(func(event AwakenEvent) {
		captured = event
	}))
	emitter.runID = "run_child"
	emitter.agentID = "agent_child"
	emitter.parentAgentID = "agent_root"
	emitter.subagentTaskID = "task_child"
	emitter.turnState = newTurnState(emitter.threadID, emitter.runID, 3, "test-model").nextTurn(nil, TransitionInitial)

	emitter.scopedSink().Emit(AwakenEvent{
		Type:          CommandOutputEventType,
		ToolCallID:    "tool_child",
		ToolName:      "run_command",
		CommandStream: "stdout",
		Delta:         "child output",
	})

	if captured.ThreadID != emitter.threadID ||
		captured.RunID != emitter.runID ||
		captured.TurnID != emitter.turnID() ||
		captured.AgentID != emitter.agentID ||
		captured.ParentAgentID != emitter.parentAgentID ||
		captured.SubagentTaskID != emitter.subagentTaskID {
		t.Fatalf("immediate child tool event missing scoped identity: %+v", captured)
	}
}

func TestSubagentProgressReportsOnlyAfterChildFactsAreDurable(t *testing.T) {
	recorder := &forkCaptureRecorder{}
	emitter := newRunEventEmitter("thread-subagent-progress", nil)
	emitter.runID = "run-subagent-progress"
	emitter.sessionID = "session-subagent-progress"
	emitter.recorder = recorder
	var updates []subagentruntime.ProgressUpdate
	emitter.subagentProgress = func(_ context.Context, update subagentruntime.ProgressUpdate) error {
		records := recorder.snapshot()
		if len(records) == 0 {
			t.Fatal("progress arrived before its semantic fact")
		}
		if update.ToolCallID != "" && records[len(records)-1].Kind != SemanticKindToolCall {
			t.Fatalf("tool progress preceded durable tool_call: %+v", records)
		}
		if update.ModelRequestID != "" && records[len(records)-1].Kind != SemanticKindModelUsage {
			t.Fatalf("usage progress preceded durable model_usage: %+v", records)
		}
		updates = append(updates, update)
		return nil
	}
	call := convcontext.ToolCall{ID: "call-read", Function: convcontext.FunctionCall{Name: "Read", Arguments: `{"file_path":"README.md"}`}}
	if err := emitter.emitToolCall(context.Background(), call); err != nil {
		t.Fatalf("emit tool call: %v", err)
	}
	input, output, total := 120, 30, 150
	if err := emitter.modelUsage(context.Background(), ModelUsageRecord{
		RequestID: "request-1", InputTokens: &input, OutputTokens: &output, TotalTokens: &total,
	}); err != nil {
		t.Fatalf("emit model usage: %v", err)
	}
	if len(updates) != 2 || updates[0].ToolCallID != "call-read" || updates[1].ModelRequestID != "request-1" || updates[1].TotalTokens != 150 || !updates[1].HasTotalTokens {
		t.Fatalf("progress updates = %+v", updates)
	}
}

// The integration only needs to prove that child event identity was prepared
// before the provider callback. Keeping this helper fallible makes accidental
// nil emitters fail with a useful message.
func runnerForEmitterRequest(events *runEventEmitter) (string, error) {
	if events == nil {
		return "", context.Canceled
	}
	return events.subagentTaskID, nil
}
