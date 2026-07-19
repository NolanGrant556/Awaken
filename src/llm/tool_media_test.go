package llm

import (
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
)

func toolMediaRequest() Request {
	return Request{Messages: []convcontext.Message{
		{Role: convcontext.RoleAssistant, ToolCalls: []convcontext.ToolCall{
			{ID: "call_image", Type: "function", Function: convcontext.FunctionCall{Name: "Read", Arguments: `{"file_path":"image.png"}`}},
			{ID: "call_pdf", Type: "function", Function: convcontext.FunctionCall{Name: "Read", Arguments: `{"file_path":"report.pdf"}`}},
		}},
		{Role: convcontext.RoleTool, ToolCallID: "call_image", Name: "Read", Content: "image result", ModelContent: []tool.ModelContentPart{{Type: "image", MimeType: "image/png", DisplayName: "image.png", DataBase64: "aW1hZ2U="}}},
		{Role: convcontext.RoleTool, ToolCallID: "call_pdf", Name: "Read", Content: "pdf result", ModelContent: []tool.ModelContentPart{{Type: "file", MimeType: "application/pdf", DisplayName: "report.pdf", DataBase64: "cGRm"}}},
	}}
}

func TestAnthropicToolResultCarriesImageAndPDFInCompositeContent(t *testing.T) {
	messages, err := buildAnthropicMessages(toolMediaRequest())
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages=%+v", messages)
	}
	results, ok := messages[1].Content.([]map[string]interface{})
	if !ok || len(results) != 2 {
		t.Fatalf("results=%#v", messages[1].Content)
	}
	imageContent, ok := results[0]["content"].([]map[string]interface{})
	if !ok || len(imageContent) != 2 || imageContent[1]["type"] != "image" {
		t.Fatalf("image content=%#v", results[0]["content"])
	}
	pdfContent, ok := results[1]["content"].([]map[string]interface{})
	if !ok || len(pdfContent) != 2 || pdfContent[1]["type"] != "document" {
		t.Fatalf("pdf content=%#v", results[1]["content"])
	}
}

func TestOpenAIToolResultsPrecedeOneAssociatedMediaMessage(t *testing.T) {
	messages := buildOpenAIMessages(Config{}, toolMediaRequest(), PreparedPromptCache{})
	if len(messages) != 4 {
		t.Fatalf("messages=%+v", messages)
	}
	if messages[1].Role != "tool" || messages[2].Role != "tool" || messages[3].Role != "user" {
		t.Fatalf("order=%+v", messages)
	}
	blocks, ok := messages[3].Content.([]map[string]interface{})
	if !ok || len(blocks) != 3 || blocks[1]["type"] != "image_url" || blocks[2]["type"] != "file" {
		t.Fatalf("blocks=%#v", messages[3].Content)
	}
	textBlock := blocks[0]
	if textBlock["text"] != "Multimodal content returned by tool_call_id(s): call_image, call_pdf" {
		t.Fatalf("association=%#v", textBlock)
	}
}
