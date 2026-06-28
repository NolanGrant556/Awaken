package titlegen

import (
	stdctx "context"
	"errors"
	"strings"
	"testing"

	convcontext "awaken/src/context"
)

func TestBuildPromptFromMessagesTruncatesFirstExchange(t *testing.T) {
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: strings.Repeat("u", 20)},
		{Role: convcontext.RoleAssistant, Content: strings.Repeat("a", 20)},
		{Role: convcontext.RoleUser, Content: "ignore me"},
		{Role: convcontext.RoleAssistant, Content: "ignore me too"},
	}

	prompt, ok := BuildPromptFromMessages(messages, Config{MaxSnippetRunes: 5})
	if !ok {
		t.Fatal("expected first exchange prompt")
	}
	if !strings.Contains(prompt.SystemPrompt, "3-7 words") {
		t.Fatalf("expected title instruction in system prompt: %q", prompt.SystemPrompt)
	}
	if !strings.Contains(prompt.SystemPrompt, "short Chinese title") {
		t.Fatalf("expected Chinese-title instruction in system prompt: %q", prompt.SystemPrompt)
	}
	if !strings.Contains(prompt.UserPrompt, "User: uuuuu...") {
		t.Fatalf("expected truncated user snippet, got %q", prompt.UserPrompt)
	}
	if !strings.Contains(prompt.UserPrompt, "Assistant: aaaaa...") {
		t.Fatalf("expected truncated assistant snippet, got %q", prompt.UserPrompt)
	}
	if strings.Contains(prompt.UserPrompt, "ignore me") {
		t.Fatalf("expected prompt to use only the first exchange, got %q", prompt.UserPrompt)
	}
}

func TestCleanTitleRemovesNoiseAndCompressesWhitespace(t *testing.T) {
	got := CleanTitle(`"Title:   Fix   login   flow. "`)

	if got != "Fix login flow" {
		t.Fatalf("expected cleaned title, got %q", got)
	}
}

func TestCleanTitleLimitsLength(t *testing.T) {
	if got := CleanTitle(strings.Repeat("a", DefaultMaxTitleRunes+10)); runeCount(got) != DefaultMaxTitleRunes {
		t.Fatalf("expected default length %d, got %d", DefaultMaxTitleRunes, runeCount(got))
	}

	if got := CleanTitle("abcdef", 3); got != "abc" {
		t.Fatalf("expected configurable length limit, got %q", got)
	}
}

func TestShouldAutoTitleSkipsExistingTitle(t *testing.T) {
	req := AutoTitleRequest{
		SessionID:     "session-1",
		ExistingTitle: "Already named",
		Messages:      firstExchangeMessages(),
	}

	if ShouldAutoTitle(req) {
		t.Fatal("expected existing title to skip auto title")
	}
}

func TestShouldAutoTitleRequiresSessionAndExchangeText(t *testing.T) {
	if ShouldAutoTitle(AutoTitleRequest{Messages: firstExchangeMessages()}) {
		t.Fatal("expected missing session id to skip auto title")
	}
	if ShouldAutoTitle(AutoTitleRequest{
		SessionID: "session-1",
		Messages: []convcontext.Message{
			{Role: convcontext.RoleUser, Content: "hello"},
		},
	}) {
		t.Fatal("expected missing assistant text to skip auto title")
	}
	if !ShouldAutoTitle(AutoTitleRequest{SessionID: "session-1", Messages: firstExchangeMessages()}) {
		t.Fatal("expected first exchange to allow auto title")
	}
}

func TestMaybeGenerateSkipsExistingTitleWithoutCallingGenerator(t *testing.T) {
	called := false

	result := MaybeGenerate(stdctx.Background(), MaybeGenerateRequest{
		SessionID:     "session-1",
		ExistingTitle: "Already named",
		Messages:      firstExchangeMessages(),
		Generator: GeneratorFunc(func(ctx stdctx.Context, prompt Prompt) (string, error) {
			called = true
			return "New title", nil
		}),
	})

	if !result.Skipped || result.SkipReason != SkipExistingTitle {
		t.Fatalf("expected existing-title skip, got %+v", result)
	}
	if called {
		t.Fatal("expected generator not to be called")
	}
}

func TestMaybeGenerateSkipsLateConversation(t *testing.T) {
	called := false

	result := MaybeGenerate(stdctx.Background(), MaybeGenerateRequest{
		SessionID: "session-1",
		Messages: []convcontext.Message{
			{Role: convcontext.RoleUser, Content: "first"},
			{Role: convcontext.RoleAssistant, Content: "first response"},
			{Role: convcontext.RoleUser, Content: "second"},
			{Role: convcontext.RoleAssistant, Content: "second response"},
			{Role: convcontext.RoleUser, Content: "third"},
		},
		Generator: GeneratorFunc(func(ctx stdctx.Context, prompt Prompt) (string, error) {
			called = true
			return "Late title", nil
		}),
	})

	if !result.Skipped || result.SkipReason != SkipLateConversation {
		t.Fatalf("expected late-conversation skip, got %+v", result)
	}
	if called {
		t.Fatal("expected generator not to be called")
	}
}

func TestMaybeGenerateReportsLLMError(t *testing.T) {
	wantErr := errors.New("provider unavailable")
	var failures []Failure

	result := MaybeGenerate(stdctx.Background(), MaybeGenerateRequest{
		SessionID: "session-1",
		Messages:  firstExchangeMessages(),
		Generator: GeneratorFunc(func(ctx stdctx.Context, prompt Prompt) (string, error) {
			return "", wantErr
		}),
		Metadata: map[string]string{"source": "test"},
		OnFailure: func(failure Failure) {
			failures = append(failures, failure)
		},
	})

	if result.Generated || result.Title != "" {
		t.Fatalf("expected no generated title, got %+v", result)
	}
	if result.Failure == nil || result.Failure.Reason != FailureLLMError || !errors.Is(result.Failure.Err, wantErr) {
		t.Fatalf("expected llm failure result, got %+v", result.Failure)
	}
	if len(failures) != 1 {
		t.Fatalf("expected one failure callback, got %d", len(failures))
	}
	if failures[0].SessionID != "session-1" || failures[0].Metadata["source"] != "test" {
		t.Fatalf("expected failure metadata, got %+v", failures[0])
	}
}

func TestMaybeGenerateSkipsEmptyCleanTitle(t *testing.T) {
	var failures []Failure

	result := MaybeGenerate(stdctx.Background(), MaybeGenerateRequest{
		SessionID: "session-1",
		Messages:  firstExchangeMessages(),
		Generator: GeneratorFunc(func(ctx stdctx.Context, prompt Prompt) (string, error) {
			return `Title: "..."`, nil
		}),
		OnFailure: func(failure Failure) {
			failures = append(failures, failure)
		},
	})

	if result.Generated || result.Title != "" {
		t.Fatalf("expected empty title to be skipped, got %+v", result)
	}
	if result.Failure == nil || result.Failure.Reason != FailureEmptyTitle {
		t.Fatalf("expected empty-title failure, got %+v", result.Failure)
	}
	if len(failures) != 1 || failures[0].RawTitle != `Title: "..."` {
		t.Fatalf("expected empty-title failure callback, got %+v", failures)
	}
}

func TestMaybeGenerateReturnsCleanTitle(t *testing.T) {
	result := MaybeGenerate(stdctx.Background(), MaybeGenerateRequest{
		SessionID: "session-1",
		Messages:  firstExchangeMessages(),
		Generator: GeneratorFunc(func(ctx stdctx.Context, prompt Prompt) (string, error) {
			return "Title: \"Debug login failure.\"", nil
		}),
	})

	if !result.Generated || result.Title != "Debug login failure" {
		t.Fatalf("expected clean generated title, got %+v", result)
	}
}

func firstExchangeMessages() []convcontext.Message {
	return []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "How do I fix the login bug?"},
		{Role: convcontext.RoleAssistant, Content: "Check the auth callback and session cookie."},
	}
}
