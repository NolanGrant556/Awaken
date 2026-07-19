package context

import (
	"strings"
	"testing"

	"awaken/src/tool"
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

func TestEstimateTotalTokensTreatsImageAttachmentsAsFixedRoughCost(t *testing.T) {
	ctx := New()
	ctx.AddUserMessageWithAttachments("look", []MessageAttachment{{
		ID:          "image-1",
		Kind:        "image",
		MimeType:    "image/png",
		DisplayName: "image.png",
		DataBase64:  strings.Repeat("A", 400000),
	}})

	got := ctx.EstimateTotalTokens()
	if got > 2500 {
		t.Fatalf("expected image attachment estimate to stay bounded, got %d", got)
	}
}

func TestEstimateAttachmentPayloadTokensKeepsImageEstimateSeparateFromBase64Length(t *testing.T) {
	imageTokens := EstimateAttachmentPayloadTokens("image", strings.Repeat("A", 400000))
	textTokens := EstimateAttachmentPayloadTokens("text", strings.Repeat("A", 400000))

	if imageTokens != roughImageAttachmentTokenEstimate {
		t.Fatalf("expected fixed image attachment estimate %d, got %d", roughImageAttachmentTokenEstimate, imageTokens)
	}
	if textTokens <= imageTokens {
		t.Fatalf("expected text payload estimate to exceed image rough estimate, got text=%d image=%d", textTokens, imageTokens)
	}
}

func TestEstimateMediaTokensExcludeProviderOmittedPayloads(t *testing.T) {
	if got := EstimateAttachmentPayloadTokens("image", ""); got != 0 {
		t.Fatalf("expected omitted image payload to contribute zero media tokens, got %d", got)
	}
	part := tool.ModelContentPart{Type: "image", MimeType: "image/png", DisplayName: "chart.png"}
	metadataOnly := EstimateModelContentTokens(part)
	part.DataBase64 = "aW1hZ2U="
	if withPayload := EstimateModelContentTokens(part); withPayload <= metadataOnly {
		t.Fatalf("expected admitted media payload estimate above metadata-only estimate: metadata=%d payload=%d", metadataOnly, withPayload)
	}
}
