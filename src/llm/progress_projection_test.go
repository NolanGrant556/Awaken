package llm

import (
	convcontext "awaken/src/context"
	"encoding/json"
	"strings"
	"testing"
)

func TestToolHostNoticeLoweringAllProtocols(t *testing.T) {
	const notice = "<awaken_runtime_notice>简短汇报</awaken_runtime_notice>"
	request := Request{SystemPrompt: "stable prefix", Messages: []convcontext.Message{
		{Role: convcontext.RoleAssistant, ToolCalls: []convcontext.ToolCall{{ID: "call", Type: "function", Function: convcontext.FunctionCall{Name: "Read", Arguments: `{}`}}}},
		{Role: convcontext.RoleTool, Name: "Read", ToolCallID: "call", Content: "original", ProviderProjectionText: notice},
	}}
	anthropic, err := buildAnthropicMessages(request)
	if err != nil {
		t.Fatal(err)
	}
	responses, err := buildOpenAIResponsesInput(request)
	if err != nil {
		t.Fatal(err)
	}
	chat := buildOpenAIMessages(Config{}, request, PreparedPromptCache{})
	for name, value := range map[string]any{"anthropic": anthropic, "responses": responses, "chat": chat} {
		data, _ := json.Marshal(value)
		var decoded any
		_ = json.Unmarshal(data, &decoded)
		text := string(data)
		// JSON escapes angle brackets; match the host marker instead.
		if strings.Count(text, "awaken_runtime_notice") != 2 || !strings.Contains(text, "original") {
			t.Fatalf("%s lost notice: %s", name, text)
		}
	}
	if request.SystemPrompt != "stable prefix" || request.Messages[1].Content != "original" {
		t.Fatal("mutated canonical input")
	}
}
