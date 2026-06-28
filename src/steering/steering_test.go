package steering

import "testing"

func TestAddIgnoresEmpty(t *testing.T) {
	manager := NewManager()

	if manager.Add(" \n\t ") {
		t.Fatal("expected empty steer to be ignored")
	}
	if got := manager.Leftover(); got != "" {
		t.Fatalf("expected no leftover, got %q", got)
	}
}

func TestAddMergesMultipleSteers(t *testing.T) {
	manager := NewManager()

	if !manager.Add(" first ") {
		t.Fatal("expected first steer to be accepted")
	}
	if !manager.Add("second") {
		t.Fatal("expected second steer to be accepted")
	}

	if got := manager.Drain(); got != "first\nsecond" {
		t.Fatalf("unexpected merged steer: %q", got)
	}
}

func TestApplyToRecentToolResultUsesLastRecentTool(t *testing.T) {
	manager := NewManager()
	manager.Add("focus on the failure")
	messages := []Message{
		{Role: "user", Content: "run tests"},
		{Role: "tool", Content: "older result"},
		{Role: "assistant", Content: "calling tools"},
		{Role: "tool", Content: "first recent result"},
		{Role: "tool", Content: "last recent result"},
	}

	updated, report := manager.ApplyToRecentToolResult(messages, 2)

	if !report.Applied || report.TargetIndex != 4 || report.Restored {
		t.Fatalf("unexpected report: %+v", report)
	}
	want := "last recent result\n\nUser guidance: focus on the failure"
	if got := updated[4].Content; got != want {
		t.Fatalf("unexpected target content: %#v", got)
	}
	if got := updated[3].Content; got != "first recent result" {
		t.Fatalf("unexpected earlier recent content: %#v", got)
	}
	if got := messages[4].Content; got != "last recent result" {
		t.Fatalf("original messages were mutated: %#v", got)
	}
}

func TestApplyWithoutToolRestoresPending(t *testing.T) {
	manager := NewManager()
	manager.Add("do not lose this")
	messages := []Message{{Role: "assistant", Content: "no tool here"}}

	updated, report := manager.ApplyToRecentToolResult(messages, 1)

	if report.Applied || !report.Restored || report.Drained != "do not lose this" {
		t.Fatalf("unexpected report: %+v", report)
	}
	if got := updated[0].Content; got != "no tool here" {
		t.Fatalf("unexpected message mutation: %#v", got)
	}
	if got := manager.Leftover(); got != "do not lose this" {
		t.Fatalf("expected steer to be restored, got %q", got)
	}
}

func TestApplyAppendsTextBlockToBlockContent(t *testing.T) {
	manager := NewManager()
	manager.Add("inspect stderr")
	messages := []Message{
		{Role: "tool", Content: []ContentBlock{{Type: "text", Text: "stdout"}}},
	}

	updated, report := manager.ApplyToRecentToolResult(messages, 1)

	if !report.Applied {
		t.Fatalf("expected steer to apply, report: %+v", report)
	}
	blocks, ok := updated[0].Content.([]ContentBlock)
	if !ok {
		t.Fatalf("expected content blocks, got %T", updated[0].Content)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected two blocks, got %d", len(blocks))
	}
	if blocks[1] != (ContentBlock{Type: "text", Text: "User guidance: inspect stderr"}) {
		t.Fatalf("unexpected appended block: %+v", blocks[1])
	}
	originalBlocks := messages[0].Content.([]ContentBlock)
	if len(originalBlocks) != 1 {
		t.Fatalf("original blocks were mutated: %+v", originalBlocks)
	}
}

func TestPreAPIApplyToLastToolResultScansAllMessages(t *testing.T) {
	manager := NewManager()
	manager.Add("answer with the tool result")
	messages := []Message{
		{Role: "tool", Content: "build passed"},
		{Role: "assistant", Content: "thinking"},
	}

	updated, report := manager.ApplyToLastToolResult(messages)

	if !report.Applied || report.TargetIndex != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
	want := "build passed\n\nUser guidance: answer with the tool result"
	if got := updated[0].Content; got != want {
		t.Fatalf("unexpected content: %#v", got)
	}
}

func TestInterruptClearsPending(t *testing.T) {
	manager := NewManager()
	manager.Add("will be cancelled")

	manager.ClearOnInterrupt()

	if got := manager.Leftover(); got != "" {
		t.Fatalf("expected interrupt to clear pending steer, got %q", got)
	}
}

func TestLeftoverDrainsPending(t *testing.T) {
	manager := NewManager()
	manager.Add("next user turn")

	if got := manager.Leftover(); got != "next user turn" {
		t.Fatalf("unexpected leftover: %q", got)
	}
	if got := manager.Leftover(); got != "" {
		t.Fatalf("expected leftover to drain once, got %q", got)
	}
}
