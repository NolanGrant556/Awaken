package feedbackbundle

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestBuildPreservesDescriptionAndFallback(t *testing.T) {
	bundle := Build(Input{InitialDescription: "the submit button hangs"})
	if bundle.Description != "the submit button hangs" {
		t.Fatalf("expected description to be preserved, got %q", bundle.Description)
	}

	fallback := Build(Input{InitialDescription: "   "})
	if fallback.Description != DefaultDescription {
		t.Fatalf("expected fallback description, got %q", fallback.Description)
	}
}

func TestBuildKeepsRecentMessagesInInputOrder(t *testing.T) {
	bundle := Build(Input{
		Messages: []Message{
			{Role: "user", Text: "one"},
			{Role: "assistant", Text: "two"},
			{Role: "user", Text: "three"},
			{Role: "assistant", Text: "four"},
		},
		MaxMessages: 2,
	})

	if len(bundle.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(bundle.Messages))
	}
	if bundle.Messages[0].Text != "three" || bundle.Messages[1].Text != "four" {
		t.Fatalf("expected newest messages in order, got %+v", bundle.Messages)
	}
	if bundle.TotalMessageCount != 4 || bundle.OmittedMessages != 2 {
		t.Fatalf("unexpected message counts: %+v", bundle)
	}
}

func TestBuildKeepsRecentMessagesByTimestampWhenAvailable(t *testing.T) {
	base := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	bundle := Build(Input{
		Messages: []Message{
			{Role: "assistant", Text: "newest", Timestamp: base.Add(2 * time.Minute)},
			{Role: "user", Text: "oldest", Timestamp: base},
			{Role: "assistant", Text: "middle", Timestamp: base.Add(time.Minute)},
		},
		MaxMessages: 2,
	})

	if got := []string{bundle.Messages[0].Text, bundle.Messages[1].Text}; got[0] != "middle" || got[1] != "newest" {
		t.Fatalf("expected timestamp ordered recent messages, got %v", got)
	}
}

func TestBuildSummarizesBackgroundTasksAndOrdersStably(t *testing.T) {
	bundle := Build(Input{
		BackgroundTasks: []BackgroundTask{
			{ID: "task-b", Type: "local_agent", AgentID: "agent-b", Status: "done", MessageCount: 1, LastMessage: "finished"},
			{ID: "task-a", Type: "in_process_teammate", AgentID: "agent-a", Status: "running", MessageCount: 3, LastMessage: "token: abc123"},
		},
	})

	if len(bundle.BackgroundTasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(bundle.BackgroundTasks))
	}
	first := bundle.BackgroundTasks[0]
	if first.ID != "task-a" || first.Type != "in_process_teammate" || first.AgentID != "agent-a" || first.Status != "running" || first.MessageCount != 3 {
		t.Fatalf("unexpected first task summary: %+v", first)
	}
	if strings.Contains(first.LastMessage, "abc123") {
		t.Fatalf("expected task last message to be redacted, got %q", first.LastMessage)
	}
}

func TestRedactRemovesSupportedSecrets(t *testing.T) {
	text := `sk-proj-abcdef Bearer secret-token password=hunter2 token: abc123 api_key="xyz"`
	redacted := Redact(text)

	for _, raw := range []string{"sk-proj-abcdef", "secret-token", "hunter2", "abc123", "xyz"} {
		if strings.Contains(redacted, raw) {
			t.Fatalf("expected %q to be redacted from %q", raw, redacted)
		}
	}
	if !strings.Contains(redacted, "[REDACTED") {
		t.Fatalf("expected redaction markers, got %q", redacted)
	}
}

func TestBuildLimitsTextRunes(t *testing.T) {
	bundle := Build(Input{
		InitialDescription: "0123456789abcdef",
		Messages: []Message{
			{Role: "user", Text: "hello世界abcdef"},
		},
		MaxTextRunes: 8,
	})

	if utf8.RuneCountInString(bundle.Description) > 8 {
		t.Fatalf("description exceeded rune limit: %q", bundle.Description)
	}
	if utf8.RuneCountInString(bundle.Messages[0].Text) > 8 {
		t.Fatalf("message exceeded rune limit: %q", bundle.Messages[0].Text)
	}
}

func TestRenderMarkdownIncludesExpectedSections(t *testing.T) {
	markdown := RenderMarkdown(Build(Input{
		InitialDescription: "bad state",
		Messages:           []Message{{Role: "user", Text: "please fix"}},
		BackgroundTasks:    []BackgroundTask{{ID: "task-1", Type: "local_agent", AgentID: "agent-1", Status: "running", MessageCount: 2, LastMessage: "working"}},
	}))

	for _, section := range []string{"## Description", "## Recent Messages", "## Background Tasks", "## Redaction Notes"} {
		if !strings.Contains(markdown, section) {
			t.Fatalf("expected markdown section %q in:\n%s", section, markdown)
		}
	}
}

func TestBuildDoesNotIncludeRawLongToolOutput(t *testing.T) {
	longOutput := "tool output start " + strings.Repeat("x", DefaultMaxToolTextRunes+100) + " tool output end"
	bundle := Build(Input{
		Messages: []Message{
			{Role: "tool", ToolName: "shell", Text: longOutput},
		},
		MaxTextRunes: DefaultMaxTextRunes,
	})

	if strings.Contains(bundle.Messages[0].Text, "tool output end") {
		t.Fatalf("expected long tool output tail to be omitted, got %q", bundle.Messages[0].Text)
	}
	if utf8.RuneCountInString(bundle.Messages[0].Text) > DefaultMaxToolTextRunes {
		t.Fatalf("tool output exceeded tool cap: %d", utf8.RuneCountInString(bundle.Messages[0].Text))
	}
	if !hasNote(bundle.RedactionNotes, noteToolTruncated) {
		t.Fatalf("expected tool truncation note, got %v", bundle.RedactionNotes)
	}
}

func hasNote(notes []string, target string) bool {
	for _, note := range notes {
		if note == target {
			return true
		}
	}
	return false
}
