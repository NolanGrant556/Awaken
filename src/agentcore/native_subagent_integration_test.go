package agentcore

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/subagentruntime"
	"awaken/src/tool"
	_ "awaken/src/tools/subagent"
)

func TestNativeSubagentToolRunsReadOnlyChildAndReturnsHandoff(t *testing.T) {
	root := t.TempDir()
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
	runner.streamLLMFunc = func(_ context.Context, events *runEventEmitter) (*llm.Response, error) {
		if events.agentID == "" || events.subagentTaskID == "" {
			t.Fatalf("native child identity missing: agent=%q task=%q", events.agentID, events.subagentTaskID)
		}
		request, err := runnerForEmitterRequest(events)
		if err != nil {
			t.Fatal(err)
		}
		_ = request
		return &llm.Response{Content: "认证入口位于 auth/service.go"}, nil
	}
	manager := subagentruntime.NewManager(context.Background(), subagentruntime.Config{
		MaxConcurrent: 1,
		Starter: func(ctx context.Context, request subagentruntime.StartRequest) (string, error) {
			return runner.startNativeChild(ctx, request, nil)
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
		waitPayload.Tasks[0].ResultPreview != "认证入口位于 auth/service.go" {
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
