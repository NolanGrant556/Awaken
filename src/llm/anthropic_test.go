package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ctx "awaken/src/context"
	"awaken/src/tool"
)

func TestInvokeAnthropicTextResponse(t *testing.T) {
	server := newAnthropicServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": "hello"},
			},
			"stop_reason": "end_turn",
		})
	})
	defer server.Close()

	resp, err := Invoke(context.Background(), anthropicConfig(server.URL), Request{UserPrompt: "hi"})
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
	if resp.Content != "hello" {
		t.Fatalf("expected content hello, got %q", resp.Content)
	}
	if len(resp.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls, got %d", len(resp.ToolCalls))
	}
	if !resp.IsFinished {
		t.Fatal("expected response to be finished")
	}
}

func TestInvokeAnthropicToolUseResponse(t *testing.T) {
	server := newAnthropicServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "tool_use",
					"id":   "toolu_123",
					"name": "calculator",
					"input": map[string]interface{}{
						"expression": "1+1",
					},
				},
			},
			"stop_reason": "tool_use",
		})
	})
	defer server.Close()

	resp, err := Invoke(context.Background(), anthropicConfig(server.URL), Request{UserPrompt: "calc"})
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
	if resp.IsFinished {
		t.Fatal("expected tool call response to be unfinished")
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "calculator" {
		t.Fatalf("unexpected tool call name: %s", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].ID != "toolu_123" {
		t.Fatalf("unexpected tool call id: %s", resp.ToolCalls[0].ID)
	}
}

func TestInvokeAnthropicTopLevelSystem(t *testing.T) {
	server := newAnthropicServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			System string `json:"system"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if body.System != "you are a test" {
			t.Fatalf("expected system prompt 'you are a test', got %q", body.System)
		}
		writeJSON(t, w, map[string]interface{}{
			"content":     []map[string]interface{}{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
		})
	})
	defer server.Close()

	_, err := Invoke(context.Background(), anthropicConfig(server.URL), Request{
		SystemPrompt: "you are a test",
		UserPrompt:   "hi",
	})
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
}

func TestInvokeAnthropicToolsSchema(t *testing.T) {
	server := newAnthropicServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Tools []anthropicTool `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if len(body.Tools) != 1 {
			t.Fatalf("expected 1 tool, got %d", len(body.Tools))
		}
		if body.Tools[0].Name != "calculator" {
			t.Fatalf("expected tool name calculator, got %s", body.Tools[0].Name)
		}
		if _, ok := body.Tools[0].InputSchema["properties"]; !ok {
			t.Fatalf("expected input_schema to have properties")
		}
		writeJSON(t, w, map[string]interface{}{
			"content":     []map[string]interface{}{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
		})
	})
	defer server.Close()

	tools := []tool.ToolSchema{
		{
			Type: "function",
			Function: tool.ToolFunctionSchema{
				Name:        "calculator",
				Description: "calc",
				Parameters: tool.ToolParametersSchema{
					Type: "object",
					Properties: map[string]tool.ToolParam{
						"expression": {Type: "string", Description: "expr"},
					},
					Required: []string{"expression"},
				},
			},
		},
	}

	_, err := Invoke(context.Background(), anthropicConfig(server.URL), Request{
		UserPrompt: "calc",
		Tools:      tools,
	})
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
}

func TestInvokeAnthropicSendsConfigMaxTokens(t *testing.T) {
	server := newAnthropicServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			MaxTokens int `json:"max_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if body.MaxTokens != 8192 {
			t.Fatalf("expected max_tokens 8192, got %d", body.MaxTokens)
		}
		writeJSON(t, w, map[string]interface{}{
			"content":     []map[string]interface{}{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
		})
	})
	defer server.Close()

	cfg := anthropicConfig(server.URL)
	cfg.MaxTokens = 8192
	if _, err := Invoke(context.Background(), cfg, Request{UserPrompt: "hi"}); err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
}

func TestInvokeAnthropicRequestDoesNotExposeInternalMetadataNames(t *testing.T) {
	server := newAnthropicServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		assertNoInternalMetadataKeys(t, body)
		writeJSON(t, w, map[string]interface{}{
			"content":     []map[string]interface{}{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
		})
	})
	defer server.Close()

	if _, err := Invoke(context.Background(), anthropicConfig(server.URL), Request{UserPrompt: "hi"}); err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
}

func TestInvokeAnthropicToolCallsConversion(t *testing.T) {
	server := newAnthropicServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []anthropicMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if len(body.Messages) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(body.Messages))
		}

		// Check assistant message with tool_use
		if body.Messages[0].Role != "assistant" {
			t.Fatalf("expected assistant message, got %s", body.Messages[0].Role)
		}
		contentBlocks, ok := body.Messages[0].Content.([]interface{})
		if !ok {
			t.Fatalf("expected content blocks, got %T", body.Messages[0].Content)
		}
		if len(contentBlocks) != 1 {
			t.Fatalf("expected 1 content block, got %d", len(contentBlocks))
		}

		// Check tool_result
		if body.Messages[1].Role != "user" {
			t.Fatalf("expected user message for tool result, got %s", body.Messages[1].Role)
		}

		writeJSON(t, w, map[string]interface{}{
			"content":     []map[string]interface{}{{"type": "text", "text": "done"}},
			"stop_reason": "end_turn",
		})
	})
	defer server.Close()

	_, err := Invoke(context.Background(), anthropicConfig(server.URL), Request{
		Messages: []ctx.Message{
			{
				Role: ctx.RoleAssistant,
				ToolCalls: []ctx.ToolCall{
					{
						ID:   "call_1",
						Type: "function",
						Function: ctx.FunctionCall{
							Name:      "calc",
							Arguments: `{"a":1}`,
						},
					},
				},
			},
			{Role: ctx.RoleTool, ToolCallID: "call_1", Name: "calc", Content: "2"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
}

func TestInvokeAnthropicEmptyTextWithToolUse(t *testing.T) {
	server := newAnthropicServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type":  "tool_use",
					"id":    "toolu_123",
					"name":  "calc",
					"input": map[string]interface{}{"a": 1},
				},
			},
			"stop_reason": "tool_use",
		})
	})
	defer server.Close()

	resp, err := Invoke(context.Background(), anthropicConfig(server.URL), Request{UserPrompt: "calc"})
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
	if resp.Content != "" {
		t.Fatalf("expected empty content, got %q", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(resp.ToolCalls))
	}
}

func TestStreamInvokeAnthropicToolUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/anthropic/v1/messages" {
			t.Fatalf("expected /anthropic/v1/messages, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		events := []string{
			`event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_123","name":"calculator"}}`,
			`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{"}}`,
			`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"expression\":\"1+1\""}}`,
			`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"}"}}`,
			`event: content_block_stop
data: {"type":"content_block_stop","index":0}`,
			`event: message_stop
data: {"type":"message_stop"}`,
		}

		for _, event := range events {
			io.WriteString(w, event+"\n\n")
		}
	}))
	defer server.Close()

	var content string
	var deltas []ToolCallDelta
	resp, err := StreamInvokeWithObserver(context.Background(), anthropicConfig(server.URL), Request{UserPrompt: "calc"}, StreamObserver{
		OnContent: func(c string, isThinking bool) {
			content += c
		},
		OnToolCallDelta: func(delta ToolCallDelta) {
			deltas = append(deltas, delta)
		},
	})
	if err != nil {
		t.Fatalf("StreamInvokeWithObserver returned error: %v", err)
	}
	if resp.IsFinished {
		t.Fatal("expected tool call response to be unfinished")
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "calculator" {
		t.Fatalf("unexpected tool call name: %s", resp.ToolCalls[0].Name)
	}
	if !strings.Contains(resp.ToolCalls[0].Input, "expression") {
		t.Fatalf("expected input to contain expression, got %s", resp.ToolCalls[0].Input)
	}
	if len(deltas) != 3 {
		t.Fatalf("expected three input_json_delta callbacks, got %+v", deltas)
	}
	if deltas[0].ID != "toolu_123" || deltas[0].Name != "calculator" || deltas[0].ArgumentsDelta != "{" {
		t.Fatalf("unexpected first delta: %+v", deltas[0])
	}
	if deltas[1].ArgumentsDelta != `"expression":"1+1"` || deltas[2].ArgumentsDelta != "}" {
		t.Fatalf("unexpected argument deltas: %+v", deltas)
	}
}

func TestStreamInvokeAnthropicThinkingAndText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/anthropic/v1/messages" {
			t.Fatalf("expected /anthropic/v1/messages, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		events := []string{
			`event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`,
			`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me think"}}`,
			`event: content_block_stop
data: {"type":"content_block_stop","index":0}`,
			`event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text"}}`,
			`event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Hello!"}}`,
			`event: content_block_stop
data: {"type":"content_block_stop","index":1}`,
			`event: message_stop
data: {"type":"message_stop"}`,
		}

		for _, event := range events {
			io.WriteString(w, event+"\n\n")
		}
	}))
	defer server.Close()

	var textContent string
	var thinkingContent string
	resp, err := StreamInvoke(context.Background(), anthropicConfig(server.URL), Request{UserPrompt: "hi"}, func(c string, isThinking bool) {
		if isThinking {
			thinkingContent += c
		} else {
			textContent += c
		}
	})
	if err != nil {
		t.Fatalf("StreamInvoke returned error: %v", err)
	}
	if !resp.IsFinished {
		t.Fatal("expected response to be finished")
	}
	if textContent != "Hello!" {
		t.Fatalf("expected text 'Hello!', got %q", textContent)
	}
	if thinkingContent != "Let me think" {
		t.Fatalf("expected thinking 'Let me think', got %q", thinkingContent)
	}
}

func TestStreamInvokeAnthropicMaxOutputStopReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/anthropic/v1/messages" {
			t.Fatalf("expected /anthropic/v1/messages, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		events := []string{
			`event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
			`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`,
			`event: content_block_stop
data: {"type":"content_block_stop","index":0}`,
			`event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"max_tokens"}}`,
			`event: message_stop
data: {"type":"message_stop"}`,
		}

		for _, event := range events {
			io.WriteString(w, event+"\n\n")
		}
	}))
	defer server.Close()

	resp, err := StreamInvoke(context.Background(), anthropicConfig(server.URL), Request{UserPrompt: "hi"}, nil)
	if err != nil {
		t.Fatalf("StreamInvoke returned error: %v", err)
	}
	if resp.Content != "partial" {
		t.Fatalf("expected partial content, got %q", resp.Content)
	}
	if resp.StopReason != "max_tokens" {
		t.Fatalf("expected stop reason max_tokens, got %q", resp.StopReason)
	}
	if !resp.MaxOutputReached {
		t.Fatal("expected max output marker")
	}
}

func TestBuildAnthropicMessagesURL(t *testing.T) {
	tests := []struct {
		baseURL string
		want    string
	}{
		{"https://api.deepseek.com/anthropic", "https://api.deepseek.com/anthropic/v1/messages"},
		{"https://api.anthropic.com", "https://api.anthropic.com/v1/messages"},
		{"https://example.com/v1", "https://example.com/v1/messages"},
		{"https://example.com/v1/", "https://example.com/v1/messages"},
		{"https://example.com/anthropic/", "https://example.com/anthropic/v1/messages"},
	}
	for _, tt := range tests {
		got := buildAnthropicMessagesURL(tt.baseURL)
		if got != tt.want {
			t.Errorf("buildAnthropicMessagesURL(%q) = %q, want %q", tt.baseURL, got, tt.want)
		}
	}
}

func TestApplyAuthHeadersAnthropicDirect(t *testing.T) {
	req, err := http.NewRequest("POST", "http://example.com", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	cfg := Config{APIKey: "secret", BaseURL: "https://api.anthropic.com/v1"}
	applyAuthHeaders(req, cfg, ProtocolAnthropicMessages)
	if req.Header.Get("x-api-key") != "secret" {
		t.Errorf("expected x-api-key secret, got %q", req.Header.Get("x-api-key"))
	}
	if req.Header.Get("anthropic-version") != "2023-06-01" {
		t.Errorf("expected anthropic-version 2023-06-01, got %q", req.Header.Get("anthropic-version"))
	}
	if req.Header.Get("Authorization") != "" {
		t.Errorf("expected no Authorization header, got %q", req.Header.Get("Authorization"))
	}
}

func TestApplyAuthHeadersDeepSeekAnthropic(t *testing.T) {
	req, err := http.NewRequest("POST", "http://example.com", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	cfg := Config{APIKey: "secret", BaseURL: "https://api.deepseek.com/anthropic"}
	applyAuthHeaders(req, cfg, ProtocolAnthropicMessages)
	if req.Header.Get("Authorization") != "Bearer secret" {
		t.Errorf("expected Authorization Bearer secret, got %q", req.Header.Get("Authorization"))
	}
	if req.Header.Get("x-api-key") != "" {
		t.Errorf("expected no x-api-key header, got %q", req.Header.Get("x-api-key"))
	}
}

func TestApplyAuthHeadersExplicitXAPIKey(t *testing.T) {
	req, err := http.NewRequest("POST", "http://example.com", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	cfg := Config{APIKey: "secret", BaseURL: "https://api.openai.com/v1", AuthScheme: "x-api-key"}
	applyAuthHeaders(req, cfg, ProtocolOpenAIChat)
	if req.Header.Get("x-api-key") != "secret" {
		t.Errorf("expected x-api-key secret, got %q", req.Header.Get("x-api-key"))
	}
	if req.Header.Get("anthropic-version") != "2023-06-01" {
		t.Errorf("expected anthropic-version 2023-06-01, got %q", req.Header.Get("anthropic-version"))
	}
	if req.Header.Get("Authorization") != "" {
		t.Errorf("expected no Authorization header, got %q", req.Header.Get("Authorization"))
	}
}

func TestApplyAuthHeadersExplicitBearer(t *testing.T) {
	req, err := http.NewRequest("POST", "http://example.com", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	cfg := Config{APIKey: "secret", BaseURL: "https://api.anthropic.com/v1", AuthScheme: "bearer"}
	applyAuthHeaders(req, cfg, ProtocolAnthropicMessages)
	if req.Header.Get("Authorization") != "Bearer secret" {
		t.Errorf("expected Authorization Bearer secret, got %q", req.Header.Get("Authorization"))
	}
	if req.Header.Get("x-api-key") != "" {
		t.Errorf("expected no x-api-key header, got %q", req.Header.Get("x-api-key"))
	}
}

func TestBuildAnthropicMessagesMergesToolResults(t *testing.T) {
	req := Request{
		Messages: []ctx.Message{
			{
				Role: ctx.RoleAssistant,
				ToolCalls: []ctx.ToolCall{
					{ID: "call_1", Type: "function", Function: ctx.FunctionCall{Name: "calc", Arguments: `{"a":1}`}},
					{ID: "call_2", Type: "function", Function: ctx.FunctionCall{Name: "calc", Arguments: `{"b":2}`}},
				},
			},
			{Role: ctx.RoleTool, ToolCallID: "call_1", Name: "calc", Content: "2"},
			{Role: ctx.RoleTool, ToolCallID: "call_2", Name: "calc", Content: "4"},
		},
	}

	messages, err := buildAnthropicMessages(req)
	if err != nil {
		t.Fatalf("buildAnthropicMessages returned error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Role != "assistant" {
		t.Fatalf("expected assistant message, got %s", messages[0].Role)
	}
	assistantBlocks, ok := messages[0].Content.([]map[string]interface{})
	if !ok || len(assistantBlocks) != 2 {
		t.Fatalf("expected 2 assistant content blocks, got %T %d", messages[0].Content, len(assistantBlocks))
	}
	if messages[1].Role != "user" {
		t.Fatalf("expected user message, got %s", messages[1].Role)
	}
	resultBlocks, ok := messages[1].Content.([]map[string]interface{})
	if !ok || len(resultBlocks) != 2 {
		t.Fatalf("expected 2 tool_result blocks, got %T %d", messages[1].Content, len(resultBlocks))
	}
	if resultBlocks[0]["tool_use_id"] != "call_1" {
		t.Errorf("expected first tool_result call_1, got %v", resultBlocks[0]["tool_use_id"])
	}
	if resultBlocks[1]["tool_use_id"] != "call_2" {
		t.Errorf("expected second tool_result call_2, got %v", resultBlocks[1]["tool_use_id"])
	}
}

func TestBuildAnthropicMessagesInvalidToolInput(t *testing.T) {
	req := Request{
		Messages: []ctx.Message{
			{
				Role: ctx.RoleAssistant,
				ToolCalls: []ctx.ToolCall{
					{ID: "call_1", Type: "function", Function: ctx.FunctionCall{Name: "calc", Arguments: "not-json"}},
				},
			},
			{Role: ctx.RoleTool, ToolCallID: "call_1", Name: "calc", Content: "2"},
		},
	}

	messages, err := buildAnthropicMessages(req)
	if err != nil {
		t.Fatalf("buildAnthropicMessages returned error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	blocks, ok := messages[0].Content.([]map[string]interface{})
	if !ok || len(blocks) != 1 {
		t.Fatalf("expected 1 assistant content block, got %T %d", messages[0].Content, len(blocks))
	}
	input, ok := blocks[0]["input"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected input to be empty object, got %T", blocks[0]["input"])
	}
	if len(input) != 0 {
		t.Fatalf("expected empty object, got %v", input)
	}
}

func TestBuildAnthropicMessagesIncludesImageBlocks(t *testing.T) {
	req := Request{
		Messages: []ctx.Message{
			{
				Role:    ctx.RoleUser,
				Content: "describe this",
				Attachments: []ctx.MessageAttachment{
					{
						ID:         "image-1",
						Kind:       "image",
						MimeType:   "image/png",
						DataBase64: "aGVsbG8=",
					},
				},
			},
		},
	}

	messages, err := buildAnthropicMessages(req)
	if err != nil {
		t.Fatalf("buildAnthropicMessages returned error: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	blocks, ok := messages[0].Content.([]map[string]interface{})
	if !ok || len(blocks) != 2 {
		t.Fatalf("expected 2 multimodal content blocks, got %T %d", messages[0].Content, len(blocks))
	}
	if blocks[0]["type"] != "text" || blocks[0]["text"] != "describe this" {
		t.Fatalf("unexpected text block: %#v", blocks[0])
	}
	if blocks[1]["type"] != "image" {
		t.Fatalf("unexpected image block: %#v", blocks[1])
	}
	source, ok := blocks[1]["source"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected image source map, got %T", blocks[1]["source"])
	}
	if source["type"] != "base64" || source["media_type"] != "image/png" || source["data"] != "aGVsbG8=" {
		t.Fatalf("unexpected image source payload: %#v", source)
	}
}

func TestBuildAnthropicMessagesIncludesFileAttachmentPaths(t *testing.T) {
	req := Request{
		Messages: []ctx.Message{
			{
				Role:    ctx.RoleUser,
				Content: "你看到这个文件吗？",
				Attachments: []ctx.MessageAttachment{
					{
						ID:          "file-1",
						Kind:        "file",
						MimeType:    "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
						Path:        "/Users/nolangrant/Desktop/Awaken 测试文件/QoderWork下载与使用指南.docx",
						DisplayName: "QoderWork下载与使用指南.docx",
					},
				},
			},
		},
	}

	messages, err := buildAnthropicMessages(req)
	if err != nil {
		t.Fatalf("buildAnthropicMessages returned error: %v", err)
	}
	blocks, ok := messages[0].Content.([]map[string]interface{})
	if !ok || len(blocks) != 1 {
		t.Fatalf("expected 1 text content block, got %T %d", messages[0].Content, len(blocks))
	}
	text, ok := blocks[0]["text"].(string)
	if !ok {
		t.Fatalf("expected text block, got %#v", blocks[0])
	}
	if !strings.Contains(text, "Files mentioned by the user:") ||
		!strings.Contains(text, "QoderWork下载与使用指南.docx: /Users/nolangrant/Desktop/Awaken 测试文件/QoderWork下载与使用指南.docx") {
		t.Fatalf("expected file attachment path in user content, got %q", text)
	}
}

func TestInvokeAnthropicRejectsOrphanToolResultBeforeRequest(t *testing.T) {
	called := false
	server := newAnthropicServer(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		writeJSON(t, w, map[string]interface{}{
			"content":     []map[string]interface{}{{"type": "text", "text": "should not be called"}},
			"stop_reason": "end_turn",
		})
	})
	defer server.Close()

	_, err := Invoke(context.Background(), anthropicConfig(server.URL), Request{
		Messages: []ctx.Message{
			{Role: ctx.RoleTool, ToolCallID: "call_orphan", Name: "calc", Content: "2"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid Anthropic tool result pairing") {
		t.Fatalf("expected local pairing error, got %v", err)
	}
	if called {
		t.Fatal("expected invalid request to be rejected before reaching provider")
	}
}

func newAnthropicServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/anthropic/v1/messages" {
			t.Fatalf("expected /anthropic/v1/messages, got %s", r.URL.Path)
		}
		handler(w, r)
	}))
}

func anthropicConfig(baseURL string) Config {
	return Config{
		APIKey:  "test-key",
		BaseURL: baseURL + "/anthropic",
		Model:   "claude-test",
	}
}
