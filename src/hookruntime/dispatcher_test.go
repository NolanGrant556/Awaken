package hookruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type captureHookRecorder struct {
	mu              sync.Mutex
	started         []InputEnvelope
	completed       []Outcome
	completedSignal chan struct{}
}

func (r *captureHookRecorder) InvocationStarted(_ context.Context, input InputEnvelope, _ CompiledHook) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.started = append(r.started, input)
	return nil
}
func (r *captureHookRecorder) InvocationCompleted(_ context.Context, outcome Outcome) error {
	r.mu.Lock()
	r.completed = append(r.completed, outcome)
	signal := r.completedSignal
	r.mu.Unlock()
	if signal != nil {
		select {
		case signal <- struct{}{}:
		default:
		}
	}
	return nil
}

func (r *captureHookRecorder) latestCompleted() (Outcome, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.completed) == 0 {
		return Outcome{}, false
	}
	return r.completed[len(r.completed)-1], true
}

func TestDispatcherExecutesCommandProtocolAndMatchesTool(t *testing.T) {
	hook := testCompiledHook(EventPreToolUse, "success")
	hook.Matcher.ToolNames = []string{"Write"}
	dispatcher := NewDispatcher(AvailabilitySnapshot{HooksByEvent: map[Event][]CompiledHook{EventPreToolUse: {hook}}}, os.Getenv("PATH"))
	recorder := &captureHookRecorder{}
	result, err := dispatcher.Dispatch(context.Background(), EventPreToolUse, map[string]any{"toolName": "Write", "input": map[string]any{"path": "x"}}, InvocationIdentity{ThreadID: "t", RunID: "r", WorkspaceRoot: t.TempDir()}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	if len(recorder.started) != 1 || len(recorder.completed) != 1 {
		t.Fatalf("records=%+v %+v", recorder.started, recorder.completed)
	}
	if result.Action != "update_input" || string(result.UpdatedInput) != "{\"path\":\"updated\"}" {
		t.Fatalf("result=%+v", result)
	}
	if recorder.completed[0].Status != "success" {
		t.Fatalf("outcome=%+v", recorder.completed[0])
	}

	notMatched, err := dispatcher.Dispatch(context.Background(), EventPreToolUse, map[string]any{"toolName": "Read"}, InvocationIdentity{RunID: "r", WorkspaceRoot: t.TempDir()}, recorder)
	if err != nil || len(notMatched.Outcomes) != 0 {
		t.Fatalf("not matched=%+v err=%v", notMatched, err)
	}
}

func TestDispatcherTimeoutAndBlockFailure(t *testing.T) {
	hook := testCompiledHook(EventPreToolUse, "sleep")
	hook.Command.TimeoutMS = 100
	hook.Command.OnFailure = "block"
	dispatcher := NewDispatcher(AvailabilitySnapshot{HooksByEvent: map[Event][]CompiledHook{EventPreToolUse: {hook}}}, os.Getenv("PATH"))
	recorder := &captureHookRecorder{}
	result, err := dispatcher.Dispatch(context.Background(), EventPreToolUse, map[string]any{"toolName": "Write"}, InvocationIdentity{RunID: "r", WorkspaceRoot: t.TempDir()}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != "block_tool" || len(result.Outcomes) != 1 || result.Outcomes[0].Status != "timed_out" {
		t.Fatalf("result=%+v", result)
	}
}

func TestDispatcherPersistenceFailurePreventsSpawn(t *testing.T) {
	recorder := failingHookRecorder{}
	hook := testCompiledHook(EventRunStart, "success")
	dispatcher := NewDispatcher(AvailabilitySnapshot{HooksByEvent: map[Event][]CompiledHook{EventRunStart: {hook}}}, os.Getenv("PATH"))
	_, err := dispatcher.Dispatch(context.Background(), EventRunStart, nil, InvocationIdentity{RunID: "r", WorkspaceRoot: t.TempDir()}, recorder)
	if err == nil {
		t.Fatal("expected persistence failure")
	}
}

func TestParseResponseAcceptsPhaseTwoAndThreeBlockingActionsOnlyForOwningEvent(t *testing.T) {
	tests := []struct {
		event  Event
		action string
	}{
		{EventPreCompact, "block_compact"},
		{EventSubagentStart, "block_subagent"},
		{EventBeforeFinish, "block_finish"},
	}
	for _, test := range tests {
		data := []byte(fmt.Sprintf(`{"protocolVersion":1,"action":%q}`, test.action))
		if _, err := parseResponse(data, test.event); err != nil {
			t.Fatalf("%s rejected for %s: %v", test.action, test.event, err)
		}
		if _, err := parseResponse(data, EventRunEnd); err == nil {
			t.Fatalf("%s accepted for wrong event", test.action)
		}
	}
}

func TestVendorProtocolAdaptersAndMCPToolExecutor(t *testing.T) {
	hook := CompiledHook{SourceEvent: "PreToolUse", Protocol: "claude_code"}
	input, err := commandInput(InputEnvelope{RunID: "run", Workspace: InvocationWorkspace{CWD: "/workspace"}, Subject: map[string]any{"toolName": "Bash", "input": map[string]any{"command": "pwd"}}}, hook)
	if err != nil || !strings.Contains(string(input), `"hook_event_name":"PreToolUse"`) || !strings.Contains(string(input), `"tool_name":"Bash"`) {
		t.Fatalf("input=%s err=%v", input, err)
	}
	response, err := parseCommandResponse([]byte(`{"hookSpecificOutput":{"permissionDecision":"deny"},"systemMessage":"blocked"}`), EventPreToolUse, "claude_code")
	if err != nil || response.Action != "block_tool" || response.AdditionalContext != "blocked" {
		t.Fatalf("response=%#v err=%v", response, err)
	}
	cursor, err := parseCommandResponse([]byte(`{"followup_message":"continue learning"}`), EventBeforeFinish, "cursor")
	if err != nil || cursor.Action != "block_finish" || cursor.AdditionalContext != "continue learning" {
		t.Fatalf("cursor=%#v err=%v", cursor, err)
	}
	async, err := parseCommandResponse([]byte("{\"async\":true,\"asyncTimeout\":180000}\n{\"metrics\":{\"sdk_bootstrap\":3,\"sdk_bootstrap_ms\":121671}}"), EventRunStart, "claude_code")
	if err != nil || async.Action != "continue" {
		t.Fatalf("Claude async declaration was not adapted: response=%#v err=%v", async, err)
	}
	contextOnly, err := parseCommandResponse([]byte(`{"hookSpecificOutput":{"additionalContext":"ready"}}`), EventRunStart, "claude_code")
	if err != nil || contextOnly.AdditionalContext != "ready" {
		t.Fatalf("Claude hook additional context was not adapted: response=%#v err=%v", contextOnly, err)
	}

	mcpHook := CompiledHook{
		StableID: "plugin:demo:hook", Event: EventRunStart, SourceKind: SourcePlugin,
		PluginName: "demo", PluginDigest: "digest", LogicalID: "hook", RevisionID: "revision",
		Command: CommandHook{Type: "mcp_tool", MCPServer: "local", MCPTool: "echo", Arguments: json.RawMessage(`{"message":"hi"}`), TimeoutMS: 1000, MaxOutputBytes: 1024, OnFailure: "continue"},
	}
	dispatcher := NewDispatcher(AvailabilitySnapshot{HooksByEvent: map[Event][]CompiledHook{EventRunStart: {mcpHook}}}, "")
	dispatcher.SetMCPToolExecutor(MCPToolExecutorFunc(func(_ context.Context, got CompiledHook, arguments map[string]any) (string, error) {
		if got.PluginDigest != "digest" || arguments["message"] != "hi" {
			t.Fatalf("hook=%#v args=%#v", got, arguments)
		}
		return "mcp result", nil
	}))
	recorder := &captureHookRecorder{}
	result, err := dispatcher.Dispatch(context.Background(), EventRunStart, nil, InvocationIdentity{RunID: "run", WorkspaceRoot: t.TempDir()}, recorder)
	if err != nil || len(result.Outcomes) != 1 || result.Outcomes[0].Status != "success" || result.AdditionalContext[0] != "mcp result" || result.Outcomes[0].PluginName != "demo" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestMinimalEnvironmentPreventsPythonBytecodeWrites(t *testing.T) {
	environment := minimalEnvironment("/bin", InputEnvelope{Event: EventUserPromptSubmit}, CompiledHook{})
	for _, entry := range environment {
		if entry == "PYTHONDONTWRITEBYTECODE=1" {
			return
		}
	}
	t.Fatalf("PYTHONDONTWRITEBYTECODE is missing: %#v", environment)
}

type testAsyncLifecycle struct{ retained atomic.Int32 }

func (l *testAsyncLifecycle) RetainAsyncHook() func() {
	l.retained.Add(1)
	var once sync.Once
	return func() { once.Do(func() { l.retained.Add(-1) }) }
}

func TestDispatcherClaudeAsyncDeclarationReturnsBeforeProcessCompletion(t *testing.T) {
	hook := testCompiledHook(EventRunStart, "claude_async")
	hook.Protocol = "claude_code"
	hook.Matcher = Matcher{}
	store := NewAsyncCompletionStore()
	lifecycle := &testAsyncLifecycle{}
	dispatcher := NewDispatcher(AvailabilitySnapshot{
		HooksByEvent: map[Event][]CompiledHook{EventRunStart: {hook}},
		AsyncSink:    store, AsyncLifecycle: lifecycle,
	}, os.Getenv("PATH"))
	recorder := &captureHookRecorder{completedSignal: make(chan struct{}, 1)}
	started := time.Now()
	result, err := dispatcher.Dispatch(context.Background(), EventRunStart, nil, InvocationIdentity{ThreadID: "thread", RunID: "run", WorkspaceRoot: t.TempDir()}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed >= 200*time.Millisecond {
		t.Fatalf("async Hook blocked dispatch for %s", elapsed)
	}
	if len(result.Outcomes) != 1 || result.Outcomes[0].Status != "scheduled" || lifecycle.retained.Load() != 1 {
		t.Fatalf("result=%#v retained=%d", result, lifecycle.retained.Load())
	}
	select {
	case <-recorder.completedSignal:
	case <-time.After(6 * time.Second):
		t.Fatal("async Hook did not complete")
	}
	outcome, ok := recorder.latestCompleted()
	if !ok || outcome.Status != "success" || outcome.DurationMS < 200 || lifecycle.retained.Load() != 0 {
		t.Fatalf("outcome=%#v retained=%d", outcome, lifecycle.retained.Load())
	}
	if store.HasAsyncHookCompletions("thread") {
		t.Fatal("metrics-only async Hook created model context")
	}
}

func TestDispatcherAsyncRewakeQueuesFeedbackWithoutRetroactiveBlock(t *testing.T) {
	hook := testCompiledHook(EventBeforeFinish, "claude_rewake")
	hook.Protocol = "claude_code"
	hook.Matcher = Matcher{}
	hook.Command.Async = true
	hook.Command.AsyncRewake = true
	hook.Command.RewakeMessage = "后台检查完成"
	store := NewAsyncCompletionStore()
	dispatcher := NewDispatcher(AvailabilitySnapshot{
		HooksByEvent: map[Event][]CompiledHook{EventBeforeFinish: {hook}},
		AsyncSink:    store,
	}, os.Getenv("PATH"))
	recorder := &captureHookRecorder{completedSignal: make(chan struct{}, 1)}
	result, err := dispatcher.Dispatch(context.Background(), EventBeforeFinish, nil, InvocationIdentity{ThreadID: "thread", RunID: "run", WorkspaceRoot: t.TempDir()}, recorder)
	if err != nil || len(result.Outcomes) != 1 || result.Outcomes[0].Status != "scheduled" || result.Action != "" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	select {
	case <-recorder.completedSignal:
	case <-time.After(2 * time.Second):
		t.Fatal("asyncRewake Hook did not complete")
	}
	outcome, ok := recorder.latestCompleted()
	if !ok || outcome.Status != "success" || outcome.Action != "continue" || outcome.ExitCode == nil || *outcome.ExitCode != 2 {
		t.Fatalf("outcome=%#v", outcome)
	}
	completions := store.DrainAsyncHookCompletions("thread")
	if len(completions) != 1 || !strings.Contains(completions[0].AdditionalContext, "后台检查完成") || !strings.Contains(completions[0].AdditionalContext, "修复安全问题") {
		t.Fatalf("completions=%#v", completions)
	}
}

func TestMatcherRequiresVendorInputPattern(t *testing.T) {
	hook := testCompiledHook(EventPostToolUse, "success")
	hook.Matcher = Matcher{ToolNames: []string{"Bash"}, InputPattern: "git commit*"}
	if !matches(hook, map[string]any{"toolName": "Bash", "input": map[string]any{"command": "git commit -m test"}}) {
		t.Fatal("matching Bash command was rejected")
	}
	if matches(hook, map[string]any{"toolName": "Bash", "input": map[string]any{"command": "git status"}}) {
		t.Fatal("unrelated Bash command matched")
	}
}

type failingHookRecorder struct{}

func (failingHookRecorder) InvocationStarted(context.Context, InputEnvelope, CompiledHook) error {
	return fmt.Errorf("disk full")
}
func (failingHookRecorder) InvocationCompleted(context.Context, Outcome) error { return nil }

func testCompiledHook(event Event, mode string) CompiledHook {
	return CompiledHook{StableID: "user:test", Event: event, SourceKind: SourceUser, Matcher: Matcher{ToolNames: []string{"Write"}}, Command: CommandHook{ID: "test", Type: "command", Command: []string{os.Args[0], "-test.run=TestHookCommandHelper", "--", mode}, CWD: "workspace_root", TimeoutMS: 2000, MaxOutputBytes: 4096, OnFailure: "continue"}}
}

func TestHookCommandHelper(t *testing.T) {
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
	case "success":
		fmt.Print(`{"protocolVersion":1,"action":"update_input","updatedInput":{"path":"updated"}}`)
		os.Exit(0)
	case "sleep":
		time.Sleep(5 * time.Second)
	case "claude_async":
		fmt.Println(`{"async":true,"asyncTimeout":4000}`)
		time.Sleep(250 * time.Millisecond)
		fmt.Println(`{"metrics":{"ready":true}}`)
		os.Exit(0)
	case "claude_rewake":
		time.Sleep(100 * time.Millisecond)
		fmt.Println(`{"decision":"block","reason":"请修复安全问题"}`)
		os.Exit(2)
	default:
		os.Exit(2)
	}
}
