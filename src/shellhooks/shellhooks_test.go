package shellhooks

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeRunner struct {
	mu       sync.Mutex
	requests []RunRequest
	run      func(context.Context, RunRequest) (RunOutput, error)
}

func (f *fakeRunner) Run(ctx context.Context, request RunRequest) (RunOutput, error) {
	f.mu.Lock()
	f.requests = append(f.requests, request)
	f.mu.Unlock()
	if f.run != nil {
		return f.run(ctx, request)
	}
	return RunOutput{}, nil
}

func (f *fakeRunner) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func (f *fakeRunner) lastRequest() RunRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[len(f.requests)-1]
}

func TestMatcherSemantics(t *testing.T) {
	tests := []struct {
		name    string
		matcher string
		query   string
		want    bool
	}{
		{name: "empty matches all", matcher: "", query: "shell", want: true},
		{name: "star matches all", matcher: "*", query: "shell", want: true},
		{name: "exact match", matcher: "shell", query: "shell", want: true},
		{name: "pipe exact match", matcher: "read|write", query: "write", want: true},
		{name: "pipe exact miss", matcher: "read|write", query: "shell", want: false},
		{name: "regex match", matcher: `^file_.*`, query: "file_read", want: true},
		{name: "invalid regex fails closed", matcher: `[`, query: "shell", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchesMatcher(tt.matcher, tt.query); got != tt.want {
				t.Fatalf("MatchesMatcher(%q, %q) = %v, want %v", tt.matcher, tt.query, got, tt.want)
			}
		})
	}

	spec := Spec{Event: EventFileChanged, Matcher: "main.go", Argv: []string{"hook"}}
	if !MatchesInput(spec, Input{FilePath: "/tmp/project/main.go"}) {
		t.Fatal("file_changed matcher should use the basename")
	}
}

func TestExecutorUsesSplitArgvWithoutShell(t *testing.T) {
	runner := &fakeRunner{}
	executor := NewExecutor(Options{Runner: runner})

	result := executor.Run(context.Background(), Spec{
		Event:   EventSessionStart,
		Command: `hook "two words"`,
	}, Input{})

	if result.Status != StatusSuccess {
		t.Fatalf("status = %s, want %s; error=%s", result.Status, StatusSuccess, result.Error)
	}
	got := runner.lastRequest().Argv
	want := []string{"hook", "two words"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv = %#v, want %#v", got, want)
	}
}

func TestExecutorReportsTimeoutStatus(t *testing.T) {
	runner := &fakeRunner{
		run: func(ctx context.Context, request RunRequest) (RunOutput, error) {
			started := time.Now()
			<-ctx.Done()
			return RunOutput{Duration: time.Since(started)}, ctx.Err()
		},
	}
	executor := NewExecutor(Options{Runner: runner})

	result := executor.Run(context.Background(), Spec{
		Event:   EventSessionStart,
		Argv:    []string{"hook"},
		Timeout: time.Millisecond,
	}, Input{})

	if result.Status != StatusTimeout {
		t.Fatalf("status = %s, want %s", result.Status, StatusTimeout)
	}
	if !result.TimedOut {
		t.Fatal("TimedOut = false, want true")
	}
}

func TestBlockDecisionParsing(t *testing.T) {
	runner := &fakeRunner{
		run: func(context.Context, RunRequest) (RunOutput, error) {
			return RunOutput{Stdout: `{"decision":"block","reason":"dangerous command"}`}, nil
		},
	}
	executor := NewExecutor(Options{Runner: runner})

	result := executor.Run(context.Background(), Spec{
		Event:   EventPreToolCall,
		Matcher: "shell",
		Argv:    []string{"hook"},
	}, Input{ToolName: "shell"})

	if result.Status != StatusBlocked {
		t.Fatalf("status = %s, want %s", result.Status, StatusBlocked)
	}
	if result.Decision.Type != DecisionBlock {
		t.Fatalf("decision = %s, want %s", result.Decision.Type, DecisionBlock)
	}
	if result.Decision.Reason != "dangerous command" {
		t.Fatalf("reason = %q", result.Decision.Reason)
	}
}

func TestTransformResultParsing(t *testing.T) {
	parsed := ParseOutput(`{"transform_result":{"stdout":"fixed","exit_code":0}}`)

	if parsed.Decision.Type != DecisionTransformResult {
		t.Fatalf("decision = %s, want %s", parsed.Decision.Type, DecisionTransformResult)
	}
	transformed, ok := parsed.Decision.TransformResult.(map[string]any)
	if !ok {
		t.Fatalf("transform_result type = %T, want map[string]any", parsed.Decision.TransformResult)
	}
	if transformed["stdout"] != "fixed" {
		t.Fatalf("transformed stdout = %v, want fixed", transformed["stdout"])
	}
}

func TestNonJSONOutputIsInert(t *testing.T) {
	runner := &fakeRunner{
		run: func(context.Context, RunRequest) (RunOutput, error) {
			return RunOutput{Stdout: "normal progress\nnot a directive"}, nil
		},
	}
	executor := NewExecutor(Options{Runner: runner})

	result := executor.Run(context.Background(), Spec{
		Event: EventPostToolCall,
		Argv:  []string{"hook"},
	}, Input{ToolName: "shell"})

	if result.Status != StatusSuccess {
		t.Fatalf("status = %s, want %s", result.Status, StatusSuccess)
	}
	if result.Decision.Type != DecisionNone {
		t.Fatalf("decision = %s, want none", result.Decision.Type)
	}
	if result.InertOutput != "normal progress\nnot a directive" {
		t.Fatalf("inert output = %q", result.InertOutput)
	}
}

func TestAllowlistGating(t *testing.T) {
	runner := &fakeRunner{}
	store := NewMemoryApprovalStore()
	spec := Spec{Event: EventSessionStart, Argv: []string{"hook"}}
	executor := NewExecutor(Options{
		Runner:          runner,
		ApprovalStore:   store,
		RequireApproval: true,
	})

	result := executor.Run(context.Background(), spec, Input{})
	if result.Status != StatusSkipped {
		t.Fatalf("status = %s, want %s", result.Status, StatusSkipped)
	}
	if result.ApprovalState != ApprovalUnknown {
		t.Fatalf("approval = %s, want %s", result.ApprovalState, ApprovalUnknown)
	}
	if runner.requestCount() != 0 {
		t.Fatalf("runner called %d times, want 0", runner.requestCount())
	}

	store.Set(spec, ApprovalApproved)
	result = executor.Run(context.Background(), spec, Input{})
	if result.Status != StatusSuccess {
		t.Fatalf("status after approval = %s, want %s", result.Status, StatusSuccess)
	}
	if runner.requestCount() != 1 {
		t.Fatalf("runner called %d times, want 1", runner.requestCount())
	}
}

func TestRegistryDedupesByEventMatcherCommand(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	spec := Spec{Event: EventPreToolCall, Matcher: "shell", Command: `hook --mode safe`}

	registered, err := registry.Register(spec)
	if err != nil {
		t.Fatal(err)
	}
	if !registered {
		t.Fatal("first registration should be new")
	}
	registered, err = registry.Register(spec)
	if err != nil {
		t.Fatal(err)
	}
	if registered {
		t.Fatal("second registration should be a duplicate")
	}

	plan := registry.Plan([]Spec{
		spec,
		{Event: EventPreToolCall, Matcher: "file", Command: `hook --mode safe`},
	})
	if len(plan.Duplicates) != 1 {
		t.Fatalf("duplicates = %d, want 1", len(plan.Duplicates))
	}
	if len(plan.New) != 1 {
		t.Fatalf("new = %d, want 1", len(plan.New))
	}
}

func TestRedactionCoversOutputAndErrors(t *testing.T) {
	secret := "sk-proj-abcdef1234567890"
	runner := &fakeRunner{
		run: func(context.Context, RunRequest) (RunOutput, error) {
			return RunOutput{
				Stdout: "token=" + secret,
				Stderr: "password=hunter2",
			}, errors.New("failed with secret=" + secret)
		},
	}
	executor := NewExecutor(Options{Runner: runner})

	result := executor.Run(context.Background(), Spec{
		Event: EventSessionStart,
		Argv:  []string{"hook"},
	}, Input{})

	for name, value := range map[string]string{
		"stdout": result.Stdout,
		"stderr": result.Stderr,
		"error":  result.Error,
		"inert":  result.InertOutput,
	} {
		if strings.Contains(value, secret) || strings.Contains(value, "hunter2") {
			t.Fatalf("%s leaked secret: %q", name, value)
		}
	}
	if !strings.Contains(result.Stdout, redactedMarker) {
		t.Fatalf("stdout = %q, want redaction marker", result.Stdout)
	}
	if !strings.Contains(result.Stderr, redactedMarker) {
		t.Fatalf("stderr = %q, want redaction marker", result.Stderr)
	}
}
