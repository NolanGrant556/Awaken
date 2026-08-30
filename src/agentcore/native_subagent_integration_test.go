package agentcore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/subagentruntime"
	"awaken/src/tool"
	_ "awaken/src/tools/read"
	_ "awaken/src/tools/subagent"
	_ "awaken/src/tools/write"
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

func TestRootAutomaticallyReceivesTerminalHandoffAfterWaitTimeout(t *testing.T) {
	releaseChild := make(chan struct{})
	terminalPersisted := make(chan struct{})
	var terminalOnce sync.Once
	manager := subagentruntime.NewManager(context.Background(), subagentruntime.Config{
		Starter: func(_ context.Context, _ subagentruntime.StartRequest) (string, error) {
			<-releaseChild
			return "复核已完成：结论可用", nil
		},
		Persist: func(_ context.Context, event subagentruntime.PersistEvent) error {
			if event.Kind == "subagent_terminal" {
				terminalOnce.Do(func() { close(terminalPersisted) })
			}
			return nil
		},
	})
	config := AgentConfig{
		MaxThoughts:        6,
		SubagentController: manager,
		ToolExposure:       tool.ToolExposurePolicy{Mode: tool.ExposureAll},
	}
	conversation := convcontext.NewConversation(nil)
	conversation.UserInput("请交给子任务复核并整合结果")
	runner := NewRunner(config, conversation, NewRequestBuilder(config))
	runner.threadID = "thread-auto-handoff"
	runner.runID = "run-auto-handoff"

	turn := 0
	runner.streamLLMFunc = func(_ context.Context, _ *runEventEmitter) (*llm.Response, error) {
		turn++
		switch turn {
		case 1:
			return &llm.Response{ToolCalls: []llm.ToolCall{{
				ID: "spawn-review", Name: "subagent",
				Input: `{"action":"spawn","description":"复核结论","prompt":"只读复核结论","agent_type":"review","workspace_mode":"shared_readonly"}`,
			}}}, nil
		case 2:
			tasks := manager.List()
			if len(tasks) != 1 {
				t.Fatalf("spawned tasks = %+v", tasks)
			}
			waitInput, err := json.Marshal(map[string]any{
				"action": "wait", "task_ids": []string{tasks[0].TaskID},
				"return_when": "all", "timeout_ms": 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			return &llm.Response{ToolCalls: []llm.ToolCall{{
				ID: "wait-review", Name: "subagent", Input: string(waitInput),
			}}}, nil
		case 3:
			close(releaseChild)
			select {
			case <-terminalPersisted:
			case <-time.After(time.Second):
				t.Fatal("child terminal state was not persisted")
			}
			// This response was generated from the stale post-timeout context.
			// The runner must retain it as history but must not finalize the run.
			return &llm.Response{Content: "子任务还在执行中"}, nil
		case 4:
			messages := conversation.Context.GetMessages()
			if got := conversation.Context.GetLastUserMessage(); got != "请交给子任务复核并整合结果" {
				t.Fatalf("synthetic handoff replaced user request: %q", got)
			}
			var notifications []convcontext.Message
			for _, message := range messages {
				if message.Origin == convcontext.MessageOriginSynthetic {
					notifications = append(notifications, message)
				}
			}
			if len(notifications) != 1 ||
				!strings.Contains(notifications[0].Content, `"status":"completed"`) ||
				!strings.Contains(notifications[0].Content, "复核已完成：结论可用") {
				t.Fatalf("automatic terminal handoff = %+v", notifications)
			}
			return &llm.Response{Content: "已根据完成的复核结果整合回答"}, nil
		default:
			t.Fatalf("unexpected model turn %d", turn)
			return nil, nil
		}
	}

	answer, err := runner.RunStreamWithEvents(context.Background(), nil)
	if err != nil {
		t.Fatalf("RunStreamWithEvents: %v", err)
	}
	if answer != "已根据完成的复核结果整合回答" || turn != 4 {
		t.Fatalf("answer=%q turns=%d", answer, turn)
	}
	if notifications := manager.DrainTerminalNotifications(); len(notifications) != 0 {
		t.Fatalf("automatic handoff must be one-shot: %+v", notifications)
	}
}

func TestRootDefersFinalAnswerWhileSubagentIsStillRunning(t *testing.T) {
	releaseChild := make(chan struct{})
	terminalPersisted := make(chan struct{})
	var terminalOnce sync.Once
	manager := subagentruntime.NewManager(context.Background(), subagentruntime.Config{
		Starter: func(_ context.Context, _ subagentruntime.StartRequest) (string, error) {
			<-releaseChild
			return "child result", nil
		},
		Persist: func(_ context.Context, event subagentruntime.PersistEvent) error {
			if event.Kind == "subagent_terminal" {
				terminalOnce.Do(func() { close(terminalPersisted) })
			}
			return nil
		},
	})
	config := AgentConfig{
		MaxThoughts:        6,
		SubagentController: manager,
		ToolExposure:       tool.ToolExposurePolicy{Mode: tool.ExposureAll},
	}
	conversation := convcontext.NewConversation(nil)
	conversation.UserInput("delegate and use the result")
	runner := NewRunner(config, conversation, NewRequestBuilder(config))
	runner.threadID = "thread-active-child"
	runner.runID = "run-active-child"

	turn := 0
	runner.streamLLMFunc = func(_ context.Context, _ *runEventEmitter) (*llm.Response, error) {
		turn++
		switch turn {
		case 1:
			return &llm.Response{ToolCalls: []llm.ToolCall{{
				ID: "spawn-review", Name: "subagent",
				Input: `{"action":"spawn","description":"review","prompt":"review","agent_type":"review","workspace_mode":"shared_readonly"}`,
			}}}, nil
		case 2:
			return &llm.Response{Content: "premature answer"}, nil
		case 3:
			var sawActiveNotice bool
			for _, message := range conversation.Context.GetMessages() {
				if message.Origin == convcontext.MessageOriginSynthetic && strings.Contains(message.Content, "Subagents are still active") {
					sawActiveNotice = true
				}
			}
			if !sawActiveNotice {
				t.Fatal("missing host-owned active Subagent wait instruction")
			}
			close(releaseChild)
			select {
			case <-terminalPersisted:
			case <-time.After(time.Second):
				t.Fatal("child terminal state was not persisted")
			}
			return &llm.Response{Content: "waiting for handoff"}, nil
		case 4:
			var sawTerminalHandoff bool
			for _, message := range conversation.Context.GetMessages() {
				if message.Origin == convcontext.MessageOriginSynthetic && strings.Contains(message.Content, "child result") {
					sawTerminalHandoff = true
				}
			}
			if !sawTerminalHandoff {
				t.Fatal("missing terminal Subagent handoff")
			}
			return &llm.Response{Content: "final answer with child result"}, nil
		default:
			t.Fatalf("unexpected model turn %d", turn)
			return nil, nil
		}
	}

	answer, err := runner.RunStreamWithEvents(context.Background(), nil)
	if err != nil {
		t.Fatalf("RunStreamWithEvents: %v", err)
	}
	if answer != "final answer with child result" || turn != 4 {
		t.Fatalf("answer=%q turns=%d", answer, turn)
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

func TestNativeGeneralSharedChildWritesParentWorkspace(t *testing.T) {
	root := t.TempDir()
	config := AgentConfig{
		Workspace: WorkspaceConfig{
			Root: root, ReadableRoots: []string{root}, WritableRoots: []string{root},
		},
		ToolExposure: tool.ToolExposurePolicy{Mode: tool.ExposureAll},
	}
	recorder := &forkCaptureRecorder{}
	runner := NewRunner(config, convcontext.NewConversation(nil), NewRequestBuilder(config))
	runner.threadID = "thread_write"
	runner.runID = "run_write"
	runner.recorder = recorder
	childTurns := 0
	runner.streamLLMFunc = func(_ context.Context, events *runEventEmitter) (*llm.Response, error) {
		if events.subagentTaskID == "" {
			t.Fatal("child identity missing")
		}
		childTurns++
		if childTurns == 1 {
			return &llm.Response{ToolCalls: []llm.ToolCall{{
				ID: "child-write", Name: "Write", Input: `{"file_path":"note.txt","content":"from general"}`,
			}}}, nil
		}
		return &llm.Response{Content: "wrote note.txt"}, nil
	}
	var drained []string
	manager := subagentruntime.NewManager(context.Background(), subagentruntime.Config{
		ParentWritableRoots: []string{root},
		Starter: func(ctx context.Context, request subagentruntime.StartRequest) (string, error) {
			original := request.DrainMessages
			request.DrainMessages = func() []subagentruntime.QueuedMessage {
				messages := original()
				for _, message := range messages {
					drained = append(drained, message.Content)
				}
				return messages
			}
			return runner.startNativeChild(ctx, request, nil)
		},
		Persist: runner.persistSubagentEvent,
	})
	runner.subagentController = manager
	t.Cleanup(func() { _ = manager.Close(context.Background()) })
	events := newRunEventEmitter(runner.threadID, nil)
	events.runID = runner.runID
	events.recorder = recorder
	runner.toolRuntime = NewToolRuntimeWithConfig(nil, toolRuntimeConfigFromAgentConfig(config))
	runner.toolRuntime.runID = runner.runID
	runner.toolRuntime.threadID = runner.threadID

	spawned := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{{
		ID: "call_spawn_write",
		Function: convcontext.FunctionCall{
			Name:      "subagent",
			Arguments: `{"action":"spawn","description":"写说明","prompt":"创建 note.txt","agent_type":"general","workspace_mode":"shared"}`,
		},
	}}, events)
	if len(spawned) != 1 || spawned[0].IsError {
		t.Fatalf("spawn = %+v", spawned)
	}
	var spawnPayload struct {
		Task subagentruntime.TaskSnapshot `json:"task"`
	}
	if err := json.Unmarshal([]byte(spawned[0].Content), &spawnPayload); err != nil {
		t.Fatal(err)
	}
	waitArgs, _ := json.Marshal(map[string]any{
		"action": "wait", "task_ids": []string{spawnPayload.Task.TaskID}, "timeout_ms": 2000,
	})
	waited := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{{
		ID: "call_wait_write", Function: convcontext.FunctionCall{Name: "subagent", Arguments: string(waitArgs)},
	}}, events)
	if len(waited) != 1 || waited[0].IsError {
		t.Fatalf("wait = %+v", waited)
	}
	data, err := os.ReadFile(filepath.Join(root, "note.txt"))
	if err != nil || string(data) != "from general" {
		t.Fatalf("parent file = %q err=%v", data, err)
	}
}

func TestNativeChildDrainsQueuedMessageAtNextTurn(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "auth.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := AgentConfig{
		Workspace:    WorkspaceConfig{Root: root, ReadableRoots: []string{root}, WritableRoots: []string{root}},
		ToolExposure: tool.ToolExposurePolicy{Mode: tool.ExposureAll},
	}
	recorder := &forkCaptureRecorder{}
	runner := NewRunner(config, convcontext.NewConversation(nil), NewRequestBuilder(config))
	runner.threadID = "thread_msg"
	runner.runID = "run_msg"
	runner.recorder = recorder
	started := make(chan struct{})
	release := make(chan struct{})
	childTurns := 0
	runner.streamLLMFunc = func(_ context.Context, events *runEventEmitter) (*llm.Response, error) {
		if events.subagentTaskID == "" {
			return &llm.Response{Content: "root"}, nil
		}
		childTurns++
		if childTurns == 1 {
			close(started)
			<-release
			return &llm.Response{ToolCalls: []llm.ToolCall{{
				ID: "child-read", Name: "Read", Input: `{"file_path":"auth.txt"}`,
			}}}, nil
		}
		return &llm.Response{Content: "processed follow-up"}, nil
	}
	var drained []string
	manager := subagentruntime.NewManager(context.Background(), subagentruntime.Config{
		Starter: func(ctx context.Context, request subagentruntime.StartRequest) (string, error) {
			original := request.DrainMessages
			request.DrainMessages = func() []subagentruntime.QueuedMessage {
				messages := original()
				for _, message := range messages {
					drained = append(drained, message.Content)
				}
				return messages
			}
			return runner.startNativeChild(ctx, request, nil)
		},
		Persist: runner.persistSubagentEvent,
	})
	runner.subagentController = manager
	t.Cleanup(func() { _ = manager.Close(context.Background()) })
	events := newRunEventEmitter(runner.threadID, nil)
	events.runID = runner.runID
	events.recorder = recorder
	runner.toolRuntime = NewToolRuntimeWithConfig(nil, toolRuntimeConfigFromAgentConfig(config))
	runner.toolRuntime.runID = runner.runID
	runner.toolRuntime.threadID = runner.threadID
	spawned := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{{
		ID: "call_spawn_msg",
		Function: convcontext.FunctionCall{
			Name:      "subagent",
			Arguments: `{"action":"spawn","description":"探索","prompt":"先读文件","agent_type":"explore"}`,
		},
	}}, events)
	if len(spawned) != 1 || spawned[0].IsError {
		t.Fatalf("spawn = %+v", spawned)
	}
	var spawnPayload struct {
		Task subagentruntime.TaskSnapshot `json:"task"`
	}
	if err := json.Unmarshal([]byte(spawned[0].Content), &spawnPayload); err != nil {
		t.Fatal(err)
	}
	<-started
	if _, err := manager.Message(context.Background(), spawnPayload.Task.TaskID, "also list risks"); err != nil {
		t.Fatal(err)
	}
	close(release)
	waitArgs, _ := json.Marshal(map[string]any{
		"action": "wait", "task_ids": []string{spawnPayload.Task.TaskID}, "timeout_ms": 2000,
	})
	waited := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{{
		ID: "call_wait_msg", Function: convcontext.FunctionCall{Name: "subagent", Arguments: string(waitArgs)},
	}}, events)
	if len(waited) != 1 || waited[0].IsError {
		t.Fatalf("wait = %+v", waited)
	}
	found := false
	for _, content := range drained {
		if content == "also list risks" {
			found = true
		}
	}
	if !found {
		t.Fatalf("queued message was not drained at child turn boundary: %v", drained)
	}
}

func TestNativeChildInheritsFrozenMemorySnapshot(t *testing.T) {
	root := t.TempDir()
	config := AgentConfig{
		Workspace:    WorkspaceConfig{Root: root, ReadableRoots: []string{root}, WritableRoots: []string{root}},
		ToolExposure: tool.ToolExposurePolicy{Mode: tool.ExposureAll},
	}
	runner := NewRunner(config, convcontext.NewConversation(nil), NewRequestBuilder(config))
	runner.memoryRecallSnapshot = &MemoryRecallSnapshot{
		WorkspaceRoot: root,
		Items: []MemoryRecallSnapshotItem{{
			MemoryID: "mem_auth", Scope: "project", Content: "<memory id=\"mem_auth\">jwt</memory>",
		}},
	}
	var seen string
	runner.streamLLMFunc = func(_ context.Context, events *runEventEmitter) (*llm.Response, error) {
		if events.subagentTaskID == "" {
			return &llm.Response{Content: "root"}, nil
		}
		if runner.reqBuilder != nil {
			// Child runner copies streamLLMFunc but uses its own request builder.
		}
		return &llm.Response{Content: "used inherited memory"}, nil
	}
	manager := subagentruntime.NewManager(context.Background(), subagentruntime.Config{
		Starter: func(ctx context.Context, request subagentruntime.StartRequest) (string, error) {
			content, err := runner.startNativeChild(ctx, request, nil)
			seen = content
			return content, err
		},
	})
	task, err := manager.Spawn(context.Background(), subagentruntime.SpawnRequest{
		Description: "use memory", Prompt: "summarize auth memory", AgentType: subagentruntime.AgentTypeReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Wait(context.Background(), subagentruntime.WaitRequest{TaskIDs: []string{task.TaskID}, Timeout: 2 * time.Second})
	if err != nil || result.Tasks[0].Status != subagentruntime.StatusCompleted || seen != "used inherited memory" {
		t.Fatalf("result=%+v seen=%q err=%v", result, seen, err)
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
