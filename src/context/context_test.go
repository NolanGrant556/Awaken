package context

import (
	"strings"
	"testing"
)

func TestContextDoesNotStoreSystemPrompt(t *testing.T) {
	ctx := New()
	if got := ctx.GetMessages(); len(got) != 0 {
		t.Fatalf("expected new context to have no messages, got %d", len(got))
	}

	ctx.AddUserMessage("hello")
	ctx.Clear()
	if got := ctx.GetMessages(); len(got) != 0 {
		t.Fatalf("expected cleared context to have no messages, got %d", len(got))
	}
}

func TestValidateToolResultPairingPassesMatchedResults(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Content: "use tools"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{
			{ID: "call_1", Type: "function", Function: FunctionCall{Name: "lookup", Arguments: `{"value":"a"}`}},
			{ID: "call_2", Type: "function", Function: FunctionCall{Name: "lookup", Arguments: `{"value":"b"}`}},
		}},
		{Role: RoleTool, ToolCallID: "call_2", Name: "lookup", Content: "b"},
		{Role: RoleTool, ToolCallID: "call_1", Name: "lookup", Content: "a"},
		{Role: RoleAssistant, Content: "done"},
	}

	if err := ValidateToolResultPairing(messages); err != nil {
		t.Fatalf("expected valid pairing, got %v", err)
	}
}

func TestValidateToolResultPairingRejectsMissingResult(t *testing.T) {
	messages := []Message{
		{Role: RoleAssistant, ToolCalls: []ToolCall{
			{ID: "call_1", Type: "function", Function: FunctionCall{Name: "lookup", Arguments: `{}`}},
		}},
		{Role: RoleUser, Content: "next"},
	}

	if err := ValidateToolResultPairing(messages); err == nil {
		t.Fatal("expected missing tool result error")
	}
}

func TestValidateToolResultPairingRejectsOrphanResult(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Content: "hello"},
		{Role: RoleTool, ToolCallID: "call_1", Name: "lookup", Content: "result"},
	}

	if err := ValidateToolResultPairing(messages); err == nil {
		t.Fatal("expected orphan tool result error")
	}
}

func TestValidateToolResultPairingRejectsNameMismatch(t *testing.T) {
	messages := []Message{
		{Role: RoleAssistant, ToolCalls: []ToolCall{
			{ID: "call_1", Type: "function", Function: FunctionCall{Name: "lookup", Arguments: `{}`}},
		}},
		{Role: RoleTool, ToolCallID: "call_1", Name: "other", Content: "result"},
	}

	if err := ValidateToolResultPairing(messages); err == nil {
		t.Fatal("expected tool result name mismatch error")
	}
}

func TestCompactMessagesPreservesToolPairing(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Content: strings.Repeat("old ", 20)},
		{Role: RoleAssistant, Content: "", ToolCalls: []ToolCall{
			{ID: "call_1", Type: "function", Function: FunctionCall{Name: "lookup", Arguments: `{"path":"old"}`}},
		}},
		{Role: RoleTool, ToolCallID: "call_1", Name: "lookup", Content: strings.Repeat("tool ", 20)},
		{Role: RoleUser, Content: "keep this user turn"},
		{Role: RoleAssistant, Content: "keep this answer"},
	}

	compacted, result := CompactMessages(messages, 0, 10, 64)
	if !result.Compacted {
		t.Fatal("expected compaction to drop the oldest tool exchange")
	}
	if len(compacted) != 2 {
		t.Fatalf("expected 2 remaining messages, got %d", len(compacted))
	}
	if compacted[0].Role != RoleUser || compacted[0].Content != "keep this user turn" {
		t.Fatalf("unexpected first kept message: %+v", compacted[0])
	}
	if err := ValidateToolResultPairing(compacted); err != nil {
		t.Fatalf("expected compacted messages to remain pair-safe, got %v", err)
	}
}

func TestCompactMessagesKeepsLatestUnitEvenWhenStillOverBudget(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Content: "old"},
		{Role: RoleAssistant, Content: strings.Repeat("latest ", 30)},
	}

	compacted, result := CompactMessages(messages, 0, 1, 8)
	if !result.Compacted {
		t.Fatal("expected compaction to drop the old prefix")
	}
	if len(compacted) != 1 {
		t.Fatalf("expected to keep the latest unit only, got %d messages", len(compacted))
	}
	if compacted[0].Content != strings.Repeat("latest ", 30) {
		t.Fatalf("expected latest message to survive, got %+v", compacted[0])
	}
}
