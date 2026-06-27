package delegationevents

import (
	"strings"
	"testing"
)

func TestEventTypeValuesAreDelegateStrings(t *testing.T) {
	for _, eventType := range []EventType{
		TaskSpawned,
		TaskProgress,
		TaskCompleted,
		TaskFailed,
		TaskThinking,
		ToolStarted,
		ToolCompleted,
	} {
		if !strings.HasPrefix(string(eventType), "delegate.") {
			t.Fatalf("expected %q to use delegate.* namespace", eventType)
		}
	}
}

func TestNormalizeLegacyEvents(t *testing.T) {
	tests := map[string]EventType{
		"_thinking":           TaskThinking,
		"reasoning.available": TaskThinking,
		"tool.started":        ToolStarted,
		"tool.completed":      ToolCompleted,
		"subagent_progress":   TaskProgress,
	}

	for input, want := range tests {
		got, ok := Normalize(input)
		if !ok {
			t.Fatalf("expected %q to normalize", input)
		}
		if got != want {
			t.Fatalf("Normalize(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeNewStyleEvents(t *testing.T) {
	tests := []EventType{
		TaskSpawned,
		TaskProgress,
		TaskCompleted,
		TaskFailed,
		TaskThinking,
		ToolStarted,
		ToolCompleted,
	}

	for _, want := range tests {
		got, ok := Normalize(string(want))
		if !ok {
			t.Fatalf("expected %q to normalize", want)
		}
		if got != want {
			t.Fatalf("Normalize(%q) = %q, want %q", want, got, want)
		}
	}
}

func TestNormalizeUnknownEvents(t *testing.T) {
	for _, input := range []string{"", "delegate.unknown", "tool.finished"} {
		got, ok := Normalize(input)
		if ok {
			t.Fatalf("Normalize(%q) ok = true, want false", input)
		}
		if got != Unknown {
			t.Fatalf("Normalize(%q) = %q, want %q", input, got, Unknown)
		}
	}
}

func TestRelayNames(t *testing.T) {
	tests := map[EventType]string{
		TaskSpawned:   "subagent.start",
		TaskProgress:  "subagent_progress",
		TaskCompleted: "subagent.complete",
		TaskFailed:    "subagent.failed",
		TaskThinking:  "subagent.thinking",
		ToolStarted:   "subagent.tool",
		ToolCompleted: "subagent.tool.completed",
		Unknown:       "unknown",
	}

	for eventType, want := range tests {
		if got := eventType.RelayName(); got != want {
			t.Fatalf("%q.RelayName() = %q, want %q", eventType, got, want)
		}
	}
}

func TestToolCompletedIsNoopDisplay(t *testing.T) {
	if !ToolCompleted.NoopDisplay() {
		t.Fatal("expected tool completed to be marked no-op display")
	}
	if ToolStarted.NoopDisplay() {
		t.Fatal("expected tool started to remain displayable")
	}
}

func TestNewProgressEventNormalizesAndPreservesFields(t *testing.T) {
	args := map[string]any{"command": "ls"}
	event, ok := NewProgressEvent("tool.started", "terminal", "ls", args)
	if !ok {
		t.Fatal("expected event to normalize")
	}
	if event.Type != ToolStarted {
		t.Fatalf("event.Type = %q, want %q", event.Type, ToolStarted)
	}
	if event.ToolName != "terminal" {
		t.Fatalf("event.ToolName = %q, want terminal", event.ToolName)
	}
	if event.Preview != "ls" {
		t.Fatalf("event.Preview = %q, want ls", event.Preview)
	}
	if event.Args == nil {
		t.Fatal("expected args to be preserved")
	}
	if event.RelayName() != "subagent.tool" {
		t.Fatalf("event.RelayName() = %q, want subagent.tool", event.RelayName())
	}
}

func TestPreBatchedSubagentProgressIsNotAToolName(t *testing.T) {
	event, ok := NewProgressEvent("subagent_progress", "[1] terminal, file", "", nil)
	if !ok {
		t.Fatal("expected pre-batched progress to normalize")
	}
	if event.Type != TaskProgress {
		t.Fatalf("event.Type = %q, want %q", event.Type, TaskProgress)
	}
	if event.ToolName != "" {
		t.Fatalf("event.ToolName = %q, want empty", event.ToolName)
	}
	if event.Preview != "[1] terminal, file" {
		t.Fatalf("event.Preview = %q, want pre-batched summary", event.Preview)
	}
	if event.RelayName() != "subagent_progress" {
		t.Fatalf("event.RelayName() = %q, want subagent_progress", event.RelayName())
	}
}

func TestUnknownProgressEventDoesNotPanic(t *testing.T) {
	event, ok := NewProgressEvent("some.unknown.event", "terminal", "ls", nil)
	if ok {
		t.Fatal("expected unknown progress event to return ok=false")
	}
	if event.Type != Unknown {
		t.Fatalf("event.Type = %q, want %q", event.Type, Unknown)
	}
}
