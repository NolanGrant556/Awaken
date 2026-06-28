package transcriptrepair

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestEnsureToolResultPairingInsertsSyntheticForMissingResult(t *testing.T) {
	messages := []Message{
		assistant(toolUse("call_1")),
	}

	repaired, report, err := EnsureToolResultPairing(messages, Options{})
	if err != nil {
		t.Fatalf("EnsureToolResultPairing() error = %v", err)
	}

	if len(repaired) != 2 {
		t.Fatalf("len(repaired) = %d, want 2", len(repaired))
	}
	if repaired[1].Role != RoleUser {
		t.Fatalf("inserted role = %q, want %q", repaired[1].Role, RoleUser)
	}
	got := repaired[1].Content[0]
	if got.Type != BlockToolResult || got.ToolUseID != "call_1" || !got.IsError {
		t.Fatalf("synthetic block = %#v", got)
	}
	if got.Content != SyntheticToolResultPlaceholder {
		t.Fatalf("synthetic content = %q", got.Content)
	}
	if report.InsertedSynthetic != 1 {
		t.Fatalf("InsertedSynthetic = %d, want 1", report.InsertedSynthetic)
	}
}

func TestEnsureToolResultPairingRemovesOrphanResultAtBeginning(t *testing.T) {
	messages := []Message{
		user(toolResult("missing")),
		assistant(text("ok")),
	}

	repaired, report, err := EnsureToolResultPairing(messages, Options{})
	if err != nil {
		t.Fatalf("EnsureToolResultPairing() error = %v", err)
	}

	if len(repaired) != 2 {
		t.Fatalf("len(repaired) = %d, want 2", len(repaired))
	}
	if got := repaired[0].Content; len(got) != 1 || got[0].Type != BlockText || got[0].Text != OrphanToolResultPlaceholder {
		t.Fatalf("beginning orphan repaired to %#v", got)
	}
	if report.RemovedOrphan != 1 {
		t.Fatalf("RemovedOrphan = %d, want 1", report.RemovedOrphan)
	}
}

func TestEnsureToolResultPairingRemovesMiddleOrphanAndInsertsMissingResult(t *testing.T) {
	messages := []Message{
		assistant(toolUse("call_1")),
		user(toolResult("other"), text("keep me")),
	}

	repaired, report, err := EnsureToolResultPairing(messages, Options{})
	if err != nil {
		t.Fatalf("EnsureToolResultPairing() error = %v", err)
	}

	got := repaired[1].Content
	if len(got) != 2 {
		t.Fatalf("len(user content) = %d, want 2: %#v", len(got), got)
	}
	if got[0].Type != BlockToolResult || got[0].ToolUseID != "call_1" || !got[0].IsError {
		t.Fatalf("missing result synthetic = %#v", got[0])
	}
	if got[1].Type != BlockText || got[1].Text != "keep me" {
		t.Fatalf("text block = %#v", got[1])
	}
	if report.InsertedSynthetic != 1 || report.RemovedOrphan != 1 {
		t.Fatalf("report = %#v, want one synthetic and one orphan removal", report)
	}
}

func TestEnsureToolResultPairingRemovesDuplicateToolUse(t *testing.T) {
	messages := []Message{
		assistant(toolUse("call_1"), toolUse("call_1")),
		user(toolResult("call_1")),
	}

	repaired, report, err := EnsureToolResultPairing(messages, Options{})
	if err != nil {
		t.Fatalf("EnsureToolResultPairing() error = %v", err)
	}

	got := repaired[0].Content
	if len(got) != 1 || got[0].Type != BlockToolUse || got[0].ID != "call_1" {
		t.Fatalf("assistant content = %#v", got)
	}
	if report.RemovedDuplicateToolUse != 1 {
		t.Fatalf("RemovedDuplicateToolUse = %d, want 1", report.RemovedDuplicateToolUse)
	}
	if report.InsertedSynthetic != 0 {
		t.Fatalf("InsertedSynthetic = %d, want 0", report.InsertedSynthetic)
	}
}

func TestEnsureToolResultPairingRemovesDuplicateToolResult(t *testing.T) {
	messages := []Message{
		assistant(toolUse("call_1")),
		user(toolResult("call_1"), toolResult("call_1")),
	}

	repaired, report, err := EnsureToolResultPairing(messages, Options{})
	if err != nil {
		t.Fatalf("EnsureToolResultPairing() error = %v", err)
	}

	got := repaired[1].Content
	if len(got) != 1 || got[0].Type != BlockToolResult || got[0].ToolUseID != "call_1" {
		t.Fatalf("user content = %#v", got)
	}
	if report.RemovedDuplicateToolResult != 1 {
		t.Fatalf("RemovedDuplicateToolResult = %d, want 1", report.RemovedDuplicateToolResult)
	}
}

func TestEnsureToolResultPairingKeepsPlaceholderWhenStrippingEmptiesUser(t *testing.T) {
	messages := []Message{
		assistant(text("before")),
		user(toolResult("orphan")),
		assistant(text("after")),
	}

	repaired, report, err := EnsureToolResultPairing(messages, Options{})
	if err != nil {
		t.Fatalf("EnsureToolResultPairing() error = %v", err)
	}

	if len(repaired) != 3 {
		t.Fatalf("len(repaired) = %d, want 3", len(repaired))
	}
	got := repaired[1].Content
	if len(got) != 1 || got[0].Type != BlockText || got[0].Text != OrphanToolResultPlaceholder {
		t.Fatalf("placeholder content = %#v", got)
	}
	if report.RemovedOrphan != 1 {
		t.Fatalf("RemovedOrphan = %d, want 1", report.RemovedOrphan)
	}
}

func TestEnsureToolResultPairingStrictModeReturnsErrorWithoutRepairing(t *testing.T) {
	messages := []Message{
		assistant(toolUse("call_1")),
	}

	repaired, report, err := EnsureToolResultPairing(messages, Options{Strict: true})
	if err == nil {
		t.Fatal("EnsureToolResultPairing() error = nil, want strict error")
	}
	var strictErr *StrictPairingError
	if !errors.As(err, &strictErr) {
		t.Fatalf("error type = %T, want *StrictPairingError", err)
	}
	if !strings.Contains(err.Error(), "strict mode") {
		t.Fatalf("error = %q, want strict mode detail", err.Error())
	}
	if !reflect.DeepEqual(repaired, messages) {
		t.Fatalf("strict repaired = %#v, want original %#v", repaired, messages)
	}
	if !report.Strict || report.InsertedSynthetic != 1 {
		t.Fatalf("report = %#v, want strict report with one synthetic candidate", report)
	}
}

func TestEnsureToolResultPairingLeavesValidTranscriptEquivalent(t *testing.T) {
	messages := []Message{
		user(text("hello")),
		assistant(text("running"), toolUse("call_1")),
		user(toolResult("call_1"), text("done")),
		assistant(text("final")),
	}

	repaired, report, err := EnsureToolResultPairing(messages, Options{})
	if err != nil {
		t.Fatalf("EnsureToolResultPairing() error = %v", err)
	}

	if !reflect.DeepEqual(repaired, messages) {
		t.Fatalf("repaired = %#v, want equivalent original %#v", repaired, messages)
	}
	if report.HasRepairs() {
		t.Fatalf("report.HasRepairs() = true, report = %#v", report)
	}
}

func user(blocks ...ContentBlock) Message {
	return Message{Role: RoleUser, Content: blocks}
}

func assistant(blocks ...ContentBlock) Message {
	return Message{Role: RoleAssistant, Content: blocks}
}

func text(value string) ContentBlock {
	return ContentBlock{Type: BlockText, Text: value}
}

func toolUse(id string) ContentBlock {
	return ContentBlock{Type: BlockToolUse, ID: id, Name: "tool"}
}

func toolResult(id string) ContentBlock {
	return ContentBlock{Type: BlockToolResult, ToolUseID: id, Content: "result"}
}
