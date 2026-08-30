package agentcore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/hookruntime"
	"awaken/src/llm"
	"awaken/src/skillruntime"
	"awaken/src/skills"
	"awaken/src/tool"
)

type hookCaptureTool struct {
	name  string
	input *string
	calls *int
}

func TestEmptyDispatcherAcceptsActivatedComposerSkillHooks(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "verify")
	if err := os.WriteFile(script, []byte("fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	snapshot := hookruntime.EmptySnapshot()
	agent := NewAgent(AgentConfig{HookAvailabilitySnapshot: &snapshot})
	agent.ConfigureHookDispatcher()
	if agent.config.HookDispatcher == nil || agent.config.HookDispatcher.HasHooks() {
		t.Fatalf("empty run dispatcher not retained: %+v", agent.config.HookDispatcher)
	}
	plan := &skillruntime.SkillActivationPlan{
		SkillID: "review", Source: skills.SourceProject, RootDir: root,
		SkillFilePath: filepath.Join(root, "SKILL.md"),
		HooksRaw: map[string]any{
			"RunEnd": []any{map[string]any{
				"matcher": map[string]any{"toolNames": []any{}},
				"hooks":   []any{map[string]any{"id": "verify", "type": "command", "command": []any{"verify"}}},
			}},
		},
	}
	if err := agent.RegisterActivatedSkillHooks(plan); err != nil {
		t.Fatal(err)
	}
	if !agent.config.HookDispatcher.HasEvent(hookruntime.EventRunEnd) {
		t.Fatal("activated Skill Hook was not registered")
	}
}

func TestHookPreflightDrainsLateAsyncContextForSameThread(t *testing.T) {
	store := hookruntime.NewAsyncCompletionStore()
	store.RecordAsyncHookCompletion("thread_async", hookruntime.AsyncCompletion{
		InvocationID: "hook_async", Event: hookruntime.EventBeforeFinish,
		AdditionalContext: "后台审查发现需要补充鉴权测试", CompletedAt: time.Now().UTC(),
	})
	snapshot := hookruntime.EmptySnapshot()
	snapshot.AsyncSink = store
	agent := NewAgent(AgentConfig{Workspace: WorkspaceConfig{Root: t.TempDir()}, HookAvailabilitySnapshot: &snapshot})
	agent.threadID, agent.runID, agent.sessionID = "thread_async", "run_next", "session_next"
	agent.recorder = &forkCaptureRecorder{}
	agent.ConfigureHookDispatcher()
	if !agent.HookPreflightRequiresPreparedUserMessage() {
		t.Fatal("pending async Hook context did not require preflight")
	}
	if err := agent.RunHookPreflight(t.Context(), EventSinkFunc(func(AwakenEvent) {}), "继续"); err != nil {
		t.Fatal(err)
	}
	if len(agent.config.HookContextBlocks) != 1 || !strings.Contains(agent.config.HookContextBlocks[0].Content, "后台审查发现需要补充鉴权测试") {
		t.Fatalf("context blocks=%#v", agent.config.HookContextBlocks)
	}
	if store.HasAsyncHookCompletions("thread_async") {
		t.Fatal("async Hook context was not drained")
	}
}

func (t hookCaptureTool) Name() string        { return t.name }
func (t hookCaptureTool) Description() string { return "captures hook-updated input" }
func (t hookCaptureTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{{Name: "value", Type: "string", Required: true}}
}
func (t hookCaptureTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{Name: t.name, DisplayName: t.name, Source: tool.SourceBuiltin, Description: t.Description(), Annotations: tool.ToolAnnotations{ReadOnly: true}}
}
func (t hookCaptureTool) Execute(_ context.Context, input string) (string, error) {
	*t.calls++
	*t.input = input
	return "executed", nil
}

func TestHookPreToolUpdateAndPostReceiptOrder(t *testing.T) {
	toolName := "hook_update_" + strings.ReplaceAll(t.Name(), "/", "_")
	var calls int
	var input string
	mustRegisterRunnerTool(t, hookCaptureTool{name: toolName, calls: &calls, input: &input})

	snapshot := hookTestSnapshot(map[hookruntime.Event]string{
		hookruntime.EventPreToolUse:  "update",
		hookruntime.EventPostToolUse: "continue",
	}, toolName)
	cfg := AgentConfig{Workspace: WorkspaceConfig{Root: t.TempDir()}, HookDispatcher: hookruntime.NewDispatcher(snapshot, os.Getenv("PATH"))}
	recorder := &forkCaptureRecorder{}
	events := newRunEventEmitter("thread_hook", EventSinkFunc(func(AwakenEvent) {}))
	events.runID, events.sessionID, events.recorder = "run_hook", "session_hook", recorder
	runner := NewRunner(cfg, convcontext.NewConversation(nil), NewRequestBuilder(cfg))
	runner.threadID, runner.runID, runner.sessionID, runner.recorder = "thread_hook", "run_hook", "session_hook", recorder
	runner.toolRuntime = NewToolRuntimeWithConfig(events.sink, toolRuntimeConfigFromAgentConfig(cfg))

	results := runner.executeToolCalls(t.Context(), []convcontext.ToolCall{{ID: "call_hook", Function: convcontext.FunctionCall{Name: toolName, Arguments: `{"value":"original"}`}}}, events)
	if calls != 1 || input != "updated" || len(results) != 1 || results[0].IsError {
		t.Fatalf("updated execution failed: calls=%d input=%s results=%+v", calls, input, results)
	}
	records := recorder.snapshot()
	preStarted, preCompleted, toolResult, postStarted := -1, -1, -1, -1
	for index, record := range records {
		switch record.Kind {
		case SemanticKindHookInvocationStarted:
			if record.Payload["event"] == hookruntime.EventPreToolUse {
				preStarted = index
			}
			if record.Payload["event"] == hookruntime.EventPostToolUse {
				postStarted = index
			}
		case SemanticKindHookInvocationCompleted:
			if record.Payload["event"] == hookruntime.EventPreToolUse {
				preCompleted = index
				if record.Payload["input_digest"] == "" || record.Payload["updated_input_digest"] == "" {
					t.Fatalf("missing input digests: %+v", record.Payload)
				}
			}
		case SemanticKindToolResult:
			toolResult = index
		}
	}
	if !(preStarted >= 0 && preStarted < preCompleted && preCompleted < toolResult && toolResult < postStarted) {
		t.Fatalf("causal order violated: pre_start=%d pre_done=%d result=%d post_start=%d records=%+v", preStarted, preCompleted, toolResult, postStarted, records)
	}
}

func TestHookPreToolBlockSkipsExecution(t *testing.T) {
	toolName := "hook_block_" + strings.ReplaceAll(t.Name(), "/", "_")
	var calls int
	var input string
	mustRegisterRunnerTool(t, hookCaptureTool{name: toolName, calls: &calls, input: &input})
	snapshot := hookTestSnapshot(map[hookruntime.Event]string{hookruntime.EventPreToolUse: "block"}, toolName)
	cfg := AgentConfig{Workspace: WorkspaceConfig{Root: t.TempDir()}, HookDispatcher: hookruntime.NewDispatcher(snapshot, os.Getenv("PATH"))}
	recorder := &forkCaptureRecorder{}
	events := newRunEventEmitter("thread_hook_block", EventSinkFunc(func(AwakenEvent) {}))
	events.runID, events.recorder = "run_hook_block", recorder
	runner := &Runner{config: cfg, threadID: "thread_hook_block", runID: "run_hook_block", recorder: recorder, toolRuntime: NewToolRuntimeWithConfig(events.sink, toolRuntimeConfigFromAgentConfig(cfg))}
	results := runner.executeToolCalls(t.Context(), []convcontext.ToolCall{{ID: "call_block", Function: convcontext.FunctionCall{Name: toolName, Arguments: `{"value":"original"}`}}}, events)
	if calls != 0 || len(results) != 1 || results[0].Status != ToolExecutionDenied || results[0].Outcome.Operation == nil || results[0].Outcome.Operation.SideEffectState != tool.OutcomeSideEffectNotStarted {
		t.Fatalf("blocked tool leaked execution: calls=%d result=%+v", calls, results)
	}
}

func TestRunEndHookStartsAfterDurableTerminal(t *testing.T) {
	snapshot := hookTestSnapshot(map[hookruntime.Event]string{hookruntime.EventRunEnd: "continue"}, "")
	cfg := AgentConfig{MaxThoughts: 1, Workspace: WorkspaceConfig{Root: t.TempDir()}, ModelConfig: ModelConfig{Model: "test"}, HookDispatcher: hookruntime.NewDispatcher(snapshot, os.Getenv("PATH"))}
	recorder := &forkCaptureRecorder{}
	runner := NewRunner(cfg, convcontext.NewConversation(nil), NewRequestBuilder(cfg))
	runner.threadID, runner.runID, runner.recorder = "thread_end", "run_end", recorder
	runner.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
		return &llm.Response{Content: "done"}, nil
	}
	if _, err := runner.RunStreamWithEvents(t.Context(), EventSinkFunc(func(AwakenEvent) {})); err != nil {
		t.Fatal(err)
	}
	records := recorder.snapshot()
	terminal, hookStart := -1, -1
	for index, record := range records {
		if record.Kind == SemanticKindRunTerminal {
			terminal = index
		}
		if record.Kind == SemanticKindHookInvocationStarted && record.Payload["event"] == hookruntime.EventRunEnd {
			hookStart = index
		}
	}
	if terminal < 0 || hookStart <= terminal {
		t.Fatalf("RunEnd order violated: terminal=%d hook=%d records=%+v", terminal, hookStart, records)
	}
}

func TestBeforeFinishAllowsTwoRetriesThenForcesFinal(t *testing.T) {
	snapshot := hookTestSnapshot(map[hookruntime.Event]string{hookruntime.EventBeforeFinish: "block_finish"}, "")
	cfg := AgentConfig{MaxThoughts: 1, Workspace: WorkspaceConfig{Root: t.TempDir()}, ModelConfig: ModelConfig{Model: "test"}, HookDispatcher: hookruntime.NewDispatcher(snapshot, os.Getenv("PATH"))}
	recorder := &forkCaptureRecorder{}
	runner := NewRunner(cfg, convcontext.NewConversation(nil), NewRequestBuilder(cfg))
	runner.threadID, runner.runID, runner.recorder = "thread_finish", "run_finish", recorder
	calls := 0
	runner.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
		calls++
		return &llm.Response{Content: fmt.Sprintf("candidate-%d", calls)}, nil
	}
	answer, err := runner.RunStreamWithEvents(t.Context(), EventSinkFunc(func(AwakenEvent) {}))
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 || answer != "candidate-3" {
		t.Fatalf("expected third candidate to be forced final, calls=%d answer=%q", calls, answer)
	}
	limitSeen := false
	for _, record := range recorder.snapshot() {
		if record.Kind == SemanticKindHookInvocationCompleted && record.Payload["reason_code"] == "hook_reentry_limit" {
			limitSeen = true
		}
	}
	if !limitSeen {
		t.Fatal("missing durable hook_reentry_limit receipt")
	}
}

func TestBeforeFinishIsSkippedForProviderErrorsAndCancellation(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{{"provider", errors.New("transport failed")}, {"cancel", context.Canceled}} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := hookTestSnapshot(map[hookruntime.Event]string{hookruntime.EventBeforeFinish: "block_finish"}, "")
			cfg := AgentConfig{MaxThoughts: 1, Workspace: WorkspaceConfig{Root: t.TempDir()}, ModelConfig: ModelConfig{Model: "test"}, HookDispatcher: hookruntime.NewDispatcher(snapshot, os.Getenv("PATH"))}
			recorder := &forkCaptureRecorder{}
			runner := NewRunner(cfg, convcontext.NewConversation(nil), NewRequestBuilder(cfg))
			runner.threadID, runner.runID, runner.recorder = "thread_skip", "run_skip", recorder
			runner.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) { return nil, test.err }
			if _, err := runner.RunStreamWithEvents(t.Context(), EventSinkFunc(func(AwakenEvent) {})); err == nil {
				t.Fatal("expected run error")
			}
			for _, record := range recorder.snapshot() {
				if record.Kind == SemanticKindHookInvocationStarted && record.Payload["event"] == hookruntime.EventBeforeFinish {
					t.Fatalf("BeforeFinish ran on %s error", test.name)
				}
			}
		})
	}
}

func TestPhaseTwoScopedHookReceiptsCarrySubagentTaskID(t *testing.T) {
	snapshot := hookTestSnapshot(map[hookruntime.Event]string{hookruntime.EventSubagentStart: "continue"}, "")
	cfg := AgentConfig{Workspace: WorkspaceConfig{Root: t.TempDir()}, HookDispatcher: hookruntime.NewDispatcher(snapshot, os.Getenv("PATH"))}
	recorder := &forkCaptureRecorder{}
	runner := NewRunner(cfg, convcontext.NewConversation(nil), NewRequestBuilder(cfg))
	runner.threadID, runner.runID, runner.recorder = "thread_child_hook", "run_child_hook", recorder
	if _, err := runner.dispatchHookScoped(t.Context(), nil, hookruntime.EventSubagentStart, map[string]any{"taskId": "task-1"}, "task-1", false); err != nil {
		t.Fatal(err)
	}
	for _, record := range recorder.snapshot() {
		if record.Kind == SemanticKindHookInvocationStarted || record.Kind == SemanticKindHookInvocationCompleted {
			if record.Payload["subagent_task_id"] != "task-1" {
				t.Fatalf("Hook receipt escaped child scope: %+v", record.Payload)
			}
		}
	}
}

func TestPreCompactHookCanSupplyInstructionOrBlock(t *testing.T) {
	for _, test := range []struct {
		name            string
		mode            string
		wantInstruction string
		wantError       bool
	}{{"instruction", "compact_context", "preserve migration decisions", false}, {"block", "block_compact", "", true}} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := hookTestSnapshot(map[hookruntime.Event]string{hookruntime.EventPreCompact: test.mode}, "")
			cfg := AgentConfig{Workspace: WorkspaceConfig{Root: t.TempDir()}, HookDispatcher: hookruntime.NewDispatcher(snapshot, os.Getenv("PATH"))}
			runner := NewRunner(cfg, convcontext.NewConversation(nil), NewRequestBuilder(cfg))
			runner.threadID, runner.runID, runner.recorder = "thread_compact", "run_compact", &forkCaptureRecorder{}
			instruction, err := runner.contextGovernor.preCompactHook(t.Context(), nil, CompactModeAuto, ContextWindowState{EstimatedTokens: 100, CompactTarget: 50})
			if (err != nil) != test.wantError || instruction != test.wantInstruction {
				t.Fatalf("instruction=%q err=%v", instruction, err)
			}
		})
	}
}

func hookTestSnapshot(events map[hookruntime.Event]string, toolName string) hookruntime.AvailabilitySnapshot {
	snapshot := hookruntime.EmptySnapshot()
	for event, mode := range events {
		matcher := hookruntime.Matcher{}
		if toolName != "" {
			matcher.ToolNames = []string{toolName}
		}
		hook := hookruntime.CompiledHook{StableID: "user:" + string(event), Event: event, SourceKind: hookruntime.SourceUser, Matcher: matcher, Command: hookruntime.CommandHook{ID: string(event), Type: "command", Command: []string{os.Args[0], "-test.run=TestAgentHookCommandHelper", "--", mode}, CWD: "workspace_root", TimeoutMS: 2000, MaxOutputBytes: 4096, OnFailure: "block"}}
		snapshot.HooksByEvent[event] = []hookruntime.CompiledHook{hook}
	}
	return snapshot
}

func TestAgentHookCommandHelper(t *testing.T) {
	mode := ""
	for index, arg := range os.Args {
		if arg == "--" && index+1 < len(os.Args) {
			mode = os.Args[index+1]
		}
	}
	if mode == "" {
		return
	}
	switch mode {
	case "update":
		fmt.Print(`{"protocolVersion":1,"action":"update_input","updatedInput":{"value":"updated"}}`)
	case "block":
		fmt.Print(`{"protocolVersion":1,"action":"block_tool","reason":"blocked by test hook"}`)
	case "continue":
		fmt.Print(`{"protocolVersion":1,"action":"continue"}`)
	case "block_finish":
		fmt.Print(`{"protocolVersion":1,"action":"block_finish","reason":"revise once more","additionalContext":"address the local review note"}`)
	case "compact_context":
		fmt.Print(`{"protocolVersion":1,"action":"continue","additionalContext":"preserve migration decisions"}`)
	case "block_compact":
		fmt.Print(`{"protocolVersion":1,"action":"block_compact","reason":"local policy"}`)
	default:
		os.Exit(2)
	}
	os.Exit(0)
}
