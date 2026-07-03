package agentcore

import (
	"errors"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
)

func TestEstimateContextEnvelopeTokensIncludesRequestParts(t *testing.T) {
	base := ContextEnvelope{
		SystemBlocks: []ContextBlock{{ID: "system", Content: "system prompt"}},
		Messages: []convcontext.Message{
			{Role: convcontext.RoleUser, Content: "hello world"},
		},
	}
	baseTokens := EstimateContextEnvelopeTokens(base)
	if baseTokens == 0 {
		t.Fatalf("expected non-zero token estimate")
	}

	withToolCall := base
	withToolCall.Messages = []convcontext.Message{
		{
			Role:    convcontext.RoleAssistant,
			Content: "using tool",
			ToolCalls: []convcontext.ToolCall{{
				ID: "call-1",
				Function: convcontext.FunctionCall{
					Name:      "test_tool",
					Arguments: `{"value":"hello"}`,
				},
			}},
		},
	}
	if EstimateContextEnvelopeTokens(withToolCall) <= baseTokens {
		t.Fatalf("expected tool call arguments to increase token estimate")
	}

	withLongToolResult := base
	withLongToolResult.Messages = []convcontext.Message{
		{Role: convcontext.RoleTool, ToolCallID: "call-1", Name: "test_tool", Content: strings.Repeat("x", 800)},
	}
	if EstimateContextEnvelopeTokens(withLongToolResult) < 100 {
		t.Fatalf("expected long tool result to be counted by char estimate")
	}

	withToolSchema := base
	withToolSchema.ToolSchemas = []tool.ToolDefinition{{
		Type: "function",
		Function: tool.ToolFunctionSchema{
			Name:        "test_tool",
			Description: "test schema",
			Parameters: tool.ToolParametersSchema{
				Type:       "object",
				Properties: map[string]tool.ToolParam{"value": {Type: "string", Description: "value"}},
				Required:   []string{"value"},
			},
		},
	}}
	if EstimateContextEnvelopeTokens(withToolSchema) <= baseTokens {
		t.Fatalf("expected tool schema JSON to increase token estimate")
	}

	withAttachment := base
	withAttachment.Attachments = []ContextAttachment{{
		ID:      "att-1",
		Kind:    "text",
		Path:    "notes.md",
		Content: "attached context content",
	}}
	if EstimateContextEnvelopeTokens(withAttachment) <= baseTokens {
		t.Fatalf("expected attachment content to increase token estimate")
	}
}

func TestNewContextWindowStateDefaults(t *testing.T) {
	state := NewContextWindowState(ContextEnvelope{
		Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "hello"}},
	})
	if state.ModelWindow != defaultModelWindow || state.OutputReserve != defaultOutputReserve {
		t.Fatalf("unexpected defaults: %+v", state)
	}
	if state.EffectiveWindow != defaultModelWindow-defaultOutputReserve {
		t.Fatalf("unexpected effective window: %+v", state)
	}
	if state.WarningThreshold == 0 || state.AutoCompactThreshold == 0 || state.CompactTarget == 0 {
		t.Fatalf("expected derived thresholds: %+v", state)
	}
	if state.EstimatedTokens == 0 {
		t.Fatalf("expected estimated tokens")
	}
}

func TestNewContextWindowStateUsesConfiguredWindow(t *testing.T) {
	state := NewContextWindowStateWithConfig(ContextEnvelope{
		Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "hello"}},
	}, ContextWindowConfig{
		ModelWindow:           400000,
		AutoCompactTokenLimit: 300000,
	})
	if state.ModelWindow != 400000 {
		t.Fatalf("expected configured model window, got %+v", state)
	}
	if state.EffectiveWindow != 400000-defaultOutputReserve {
		t.Fatalf("unexpected effective window: %+v", state)
	}
	if state.AutoCompactThreshold != 300000 {
		t.Fatalf("expected configured auto compact threshold, got %+v", state)
	}
}

func TestCompactConversationKeepsPairingAndBoundary(t *testing.T) {
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "please use tool"},
		{
			Role:    convcontext.RoleAssistant,
			Content: "calling",
			ToolCalls: []convcontext.ToolCall{{
				ID: "call-1",
				Function: convcontext.FunctionCall{
					Name:      "test_tool",
					Arguments: `{"value":"hello"}`,
				},
			}},
		},
		{Role: convcontext.RoleTool, ToolCallID: "call-1", Name: "test_tool", Content: "result"},
		{Role: convcontext.RoleUser, Content: "thanks"},
		{Role: convcontext.RoleAssistant, Content: "done"},
	}
	result, err := CompactConversation(messages, ContextEnvelope{Messages: messages}, 20)
	if err != nil {
		t.Fatalf("CompactConversation returned error: %v", err)
	}
	if result.BoundaryID == "" || result.Messages[0].CompactBoundaryID != result.BoundaryID {
		t.Fatalf("expected compact boundary on summary message, got %+v", result)
	}
	if err := convcontext.ValidateToolResultPairing(result.Messages); err != nil {
		t.Fatalf("compact result broke tool pairing: %v", err)
	}

	ctx := convcontext.New()
	ctx.SetMessages(result.Messages)
	modelMessages := ctx.GetMessagesForModel()
	if len(modelMessages) == 0 || modelMessages[0].CompactBoundaryID != result.BoundaryID {
		t.Fatalf("expected GetMessagesForModel to start at compact boundary, got %+v", modelMessages)
	}
}

func TestCompactConversationModelMessagesUseSummaryAndTail(t *testing.T) {
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "ancient context " + strings.Repeat("x", 2000)},
		{Role: convcontext.RoleAssistant, Content: "old answer"},
		{Role: convcontext.RoleUser, Content: "latest tail question"},
	}

	result, err := CompactConversation(messages, ContextEnvelope{Messages: messages}, 1)
	if err != nil {
		t.Fatalf("CompactConversation returned error: %v", err)
	}
	if len(result.Messages) != 2 {
		t.Fatalf("expected compacted messages to contain summary plus one tail message, got %+v", result.Messages)
	}
	if result.Messages[0].CompactBoundaryID != result.BoundaryID {
		t.Fatalf("expected first compacted message to be boundary summary, got %+v", result.Messages[0])
	}
	if result.Messages[1].Content != "latest tail question" {
		t.Fatalf("expected tail message after boundary summary, got %+v", result.Messages)
	}

	ctx := convcontext.New()
	ctx.SetMessages(result.Messages)
	modelMessages := ctx.GetMessagesForModel()
	if len(modelMessages) != 2 {
		t.Fatalf("expected model messages to contain summary plus tail, got %+v", modelMessages)
	}
	if modelMessages[0].CompactBoundaryID == "" || modelMessages[1].CompactBoundaryID != "" {
		t.Fatalf("expected model messages to contain boundary summary followed by raw tail only, got %+v", modelMessages)
	}
}

func TestIsPromptTooLongError(t *testing.T) {
	if !isPromptTooLongError(errors.New("maximum context length exceeded")) {
		t.Fatalf("expected prompt-too-long detection")
	}
	if isPromptTooLongError(errors.New("transport error")) {
		t.Fatalf("did not expect prompt-too-long detection")
	}
}
