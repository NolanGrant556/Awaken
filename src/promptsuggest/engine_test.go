package promptsuggest

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSuppressReason(t *testing.T) {
	base := Guard{Enabled: true, AssistantTurnCount: MinAssistantTurns}

	tests := []struct {
		name  string
		guard Guard
		want  Reason
	}{
		{
			name:  "disabled suppress",
			guard: withGuard(base, func(g *Guard) { g.Enabled = false }),
			want:  ReasonDisabled,
		},
		{
			name:  "early conversation suppress",
			guard: Guard{Enabled: true, AssistantTurnCount: MinAssistantTurns - 1},
			want:  ReasonEarlyConversation,
		},
		{
			name:  "pending permission suppress",
			guard: withGuard(base, func(g *Guard) { g.PendingPermission = true }),
			want:  ReasonPendingPermission,
		},
		{
			name:  "elicitation active suppress",
			guard: withGuard(base, func(g *Guard) { g.ElicitationActive = true }),
			want:  ReasonElicitationActive,
		},
		{
			name:  "plan mode suppress",
			guard: withGuard(base, func(g *Guard) { g.PlanMode = true }),
			want:  ReasonPlanMode,
		},
		{
			name:  "rate limited suppress",
			guard: withGuard(base, func(g *Guard) { g.RateLimited = true }),
			want:  ReasonRateLimited,
		},
		{
			name:  "last assistant error suppress",
			guard: withGuard(base, func(g *Guard) { g.LastAssistantErrored = true }),
			want:  ReasonLastResponseError,
		},
		{
			name:  "enabled conversation allowed",
			guard: base,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.guard.SuppressReason()
			if got != tt.want {
				t.Fatalf("SuppressReason() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTryGenerateSuppressesWithoutCallingGenerator(t *testing.T) {
	base := allowedInput()

	tests := []struct {
		name  string
		input Input
		want  Reason
	}{
		{
			name:  "early conversation suppress",
			input: withInput(base, func(i *Input) { i.Guard.AssistantTurnCount = MinAssistantTurns - 1 }),
			want:  ReasonEarlyConversation,
		},
		{
			name:  "pending permission suppress",
			input: withInput(base, func(i *Input) { i.Guard.PendingPermission = true }),
			want:  ReasonPendingPermission,
		},
		{
			name:  "plan mode suppress",
			input: withInput(base, func(i *Input) { i.Guard.PlanMode = true }),
			want:  ReasonPlanMode,
		},
		{
			name:  "last assistant error suppress",
			input: withInput(base, func(i *Input) { i.Guard.LastAssistantErrored = true }),
			want:  ReasonLastResponseError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := &stubGenerator{suggestion: Suggestion{Text: "run the tests"}}
			result, err := NewEngine(generator).TryGenerate(context.Background(), tt.input)
			if err != nil {
				t.Fatalf("TryGenerate() error = %v", err)
			}
			if result.Suppressed != tt.want {
				t.Fatalf("TryGenerate().Suppressed = %q, want %q", result.Suppressed, tt.want)
			}
			if generator.calls != 0 {
				t.Fatalf("generator calls = %d, want 0", generator.calls)
			}
		})
	}
}

func TestTryGenerateCallsGenerator(t *testing.T) {
	input := allowedInput()
	generator := &stubGenerator{suggestion: Suggestion{Text: " run the tests "}}

	result, err := NewEngine(generator).TryGenerate(context.Background(), input)
	if err != nil {
		t.Fatalf("TryGenerate() error = %v", err)
	}
	if result.Suppressed != "" {
		t.Fatalf("TryGenerate().Suppressed = %q, want empty", result.Suppressed)
	}
	if result.Suggestion.Text != "run the tests" {
		t.Fatalf("TryGenerate().Suggestion.Text = %q, want %q", result.Suggestion.Text, "run the tests")
	}
	if generator.calls != 1 {
		t.Fatalf("generator calls = %d, want 1", generator.calls)
	}
	if !reflect.DeepEqual(generator.conversation, input.Conversation) {
		t.Fatalf("generator conversation = %#v, want %#v", generator.conversation, input.Conversation)
	}
}

func TestTryGenerateFiltersSuggestion(t *testing.T) {
	tests := []struct {
		name       string
		suggestion Suggestion
		want       Reason
	}{
		{
			name:       "empty suggestion filtered",
			suggestion: Suggestion{Text: "   "},
			want:       ReasonEmpty,
		},
		{
			name:       "too long suggestion filtered",
			suggestion: Suggestion{Text: strings.Repeat("a", MaxSuggestionLength)},
			want:       ReasonTooLong,
		},
		{
			name:       "code block suggestion filtered conservatively",
			suggestion: Suggestion{Text: "```go\nfmt.Println(\"hi\")\n```"},
			want:       ReasonCodeBlock,
		},
		{
			name:       "bare file path suggestion filtered conservatively",
			suggestion: Suggestion{Text: "src/promptsuggest/engine.go"},
			want:       ReasonFilePath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := &stubGenerator{suggestion: tt.suggestion}
			result, err := NewEngine(generator).TryGenerate(context.Background(), allowedInput())
			if err != nil {
				t.Fatalf("TryGenerate() error = %v", err)
			}
			if result.Suppressed != tt.want {
				t.Fatalf("TryGenerate().Suppressed = %q, want %q", result.Suppressed, tt.want)
			}
			if generator.calls != 1 {
				t.Fatalf("generator calls = %d, want 1", generator.calls)
			}
		})
	}
}

func TestFilterReasonKeepsCommandWithPathArgument(t *testing.T) {
	got := FilterReason(Suggestion{Text: "run go test ./..."})
	if got != "" {
		t.Fatalf("FilterReason() = %q, want empty", got)
	}
}

func TestTryGenerateContextCanceledBeforeGenerator(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	generator := &stubGenerator{suggestion: Suggestion{Text: "run the tests"}}

	result, err := NewEngine(generator).TryGenerate(ctx, allowedInput())
	if err != nil {
		t.Fatalf("TryGenerate() error = %v", err)
	}
	if result.Suppressed != ReasonAborted {
		t.Fatalf("TryGenerate().Suppressed = %q, want %q", result.Suppressed, ReasonAborted)
	}
	if generator.calls != 0 {
		t.Fatalf("generator calls = %d, want 0", generator.calls)
	}
}

func TestTryGenerateContextCanceledAfterGenerator(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	generator := &stubGenerator{
		suggestion:   Suggestion{Text: "run the tests"},
		beforeReturn: cancel,
	}

	result, err := NewEngine(generator).TryGenerate(ctx, allowedInput())
	if err != nil {
		t.Fatalf("TryGenerate() error = %v", err)
	}
	if result.Suppressed != ReasonAborted {
		t.Fatalf("TryGenerate().Suppressed = %q, want %q", result.Suppressed, ReasonAborted)
	}
	if result.Suggestion.Text != "" {
		t.Fatalf("TryGenerate().Suggestion.Text = %q, want empty", result.Suggestion.Text)
	}
}

func TestTryGenerateGeneratorError(t *testing.T) {
	wantErr := errors.New("boom")
	generator := &stubGenerator{err: wantErr}

	_, err := NewEngine(generator).TryGenerate(context.Background(), allowedInput())
	if !errors.Is(err, wantErr) {
		t.Fatalf("TryGenerate() error = %v, want wrapping %v", err, wantErr)
	}
}

type stubGenerator struct {
	suggestion   Suggestion
	err          error
	beforeReturn func()
	calls        int
	conversation Conversation
}

func (s *stubGenerator) Generate(_ context.Context, conversation Conversation) (Suggestion, error) {
	s.calls++
	s.conversation = conversation
	if s.beforeReturn != nil {
		s.beforeReturn()
	}
	return s.suggestion, s.err
}

func allowedInput() Input {
	return Input{
		Guard: Guard{
			Enabled:            true,
			AssistantTurnCount: MinAssistantTurns,
		},
		Conversation: Conversation{
			Messages: []Message{
				{Role: "user", Content: "fix the bug"},
				{Role: "assistant", Content: "fixed it"},
			},
		},
	}
}

func withGuard(guard Guard, update func(*Guard)) Guard {
	update(&guard)
	return guard
}

func withInput(input Input, update func(*Input)) Input {
	update(&input)
	return input
}
