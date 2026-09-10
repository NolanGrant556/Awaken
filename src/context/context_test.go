package context

import (
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestTokenEstimateDoesNotTreatCJKAsLatinCharacters(t *testing.T) {
	if got := EstimateTokens(strings.Repeat("天地玄黄宇宙洪荒", 1000)); got < 8000 {
		t.Fatalf("large CJK text was underestimated as Latin: %d", got)
	}
	if got := EstimateTokens(strings.Repeat("abcd", 1000)); got != 1000 {
		t.Fatalf("Latin estimate changed: %d", got)
	}
}

func TestTokenEstimateKeepsMixedScriptAndWordBounds(t *testing.T) {
	for text, want := range map[string]int{
		"ABCD中文":        3,
		"漢字かなカナ한글abcd":  9,
		"🙂abcd":         1,
		"one two three": 4,
		"   ":           0,
	} {
		if got := EstimateTokens(text); got != want {
			t.Errorf("EstimateTokens(%q) = %d; want %d", text, got, want)
		}
	}
}

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

func TestSyntheticUserMessageDoesNotReplaceLastUserInput(t *testing.T) {
	ctx := New()
	ctx.AddUserMessage("real user request")
	ctx.AddAssistantMessage("waiting", nil)
	ctx.AddSyntheticUserMessage("host child completion", "subagent_terminal_task-1")

	if got := ctx.GetLastUserMessage(); got != "real user request" {
		t.Fatalf("last user message = %q", got)
	}
	messages := ctx.GetMessages()
	if len(messages) != 3 || messages[2].Origin != MessageOriginSynthetic || messages[2].MessageID != "subagent_terminal_task-1" {
		t.Fatalf("synthetic message metadata = %+v", messages)
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
	if metadataOnly >= MediaPartTokenEstimate {
		t.Fatalf("cleared stub must not pay the media session price, got %d", metadataOnly)
	}
	part.DataBase64 = "aW1hZ2U="
	if withPayload := EstimateModelContentTokens(part); withPayload < metadataOnly+MediaPartTokenEstimate {
		t.Fatalf("expected admitted media payload estimate above metadata-only estimate: metadata=%d payload=%d", metadataOnly, withPayload)
	}
}

func TestEstimateModelContentTokensCountsDurableMediaWithoutPayload(t *testing.T) {
	stub := tool.ModelContentPart{Type: "image", MimeType: "image/png", DisplayName: "chart.png"}
	stubTokens := EstimateModelContentTokens(stub)

	withKey := stub
	withKey.StorageKey = "thread/tool-results/call/part-001.png"
	if got := EstimateModelContentTokens(withKey); got < stubTokens+MediaPartTokenEstimate {
		t.Fatalf("storage_key-only image must still cost %d, got %d", MediaPartTokenEstimate, got)
	}

	withSize := stub
	withSize.ByteSize = 4096
	if got := EstimateModelContentTokens(withSize); got < stubTokens+MediaPartTokenEstimate {
		t.Fatalf("ByteSize-only image must still cost %d, got %d", MediaPartTokenEstimate, got)
	}

	filePart := tool.ModelContentPart{Type: "file", MimeType: "application/pdf", DisplayName: "report.pdf", StorageKey: "thread/tool-results/call/part-001.pdf"}
	if got := EstimateModelContentTokens(filePart); got < MediaPartTokenEstimate {
		t.Fatalf("native PDF with storage_key must cost %d, got %d", MediaPartTokenEstimate, got)
	}
}

func TestEstimateMessageAttachmentTokensCountsStorageKeyWithoutPayload(t *testing.T) {
	cleared := MessageAttachment{Kind: "image", MimeType: "image/png", DisplayName: "a.png"}
	if got := EstimateMessageAttachmentTokens(cleared); got >= MediaPartTokenEstimate {
		t.Fatalf("cleared image stub must not pay the media session price, got %d", got)
	}
	durable := MessageAttachment{
		Kind: "image", MimeType: "image/png", DisplayName: "a.png",
		StorageKey: "sessions/t/attachments/a.png", ByteSize: 12,
	}
	if got := EstimateMessageAttachmentTokens(durable); got < MediaPartTokenEstimate {
		t.Fatalf("durable image attachment must cost %d, got %d", MediaPartTokenEstimate, got)
	}
}
