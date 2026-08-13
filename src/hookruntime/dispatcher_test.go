package hookruntime

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

type captureHookRecorder struct {
	started   []InputEnvelope
	completed []Outcome
}

func (r *captureHookRecorder) InvocationStarted(_ context.Context, input InputEnvelope, _ CompiledHook) error {
	r.started = append(r.started, input)
	return nil
}
func (r *captureHookRecorder) InvocationCompleted(_ context.Context, outcome Outcome) error {
	r.completed = append(r.completed, outcome)
	return nil
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
	default:
		os.Exit(2)
	}
}
