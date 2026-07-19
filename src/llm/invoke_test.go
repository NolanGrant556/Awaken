package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ctx "awaken/src/context"
)

func TestInvokeOpenAITextResponse(t *testing.T) {
	server := newChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "hello"}},
			},
		})
	})
	defer server.Close()

	resp, err := Invoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"})
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

func TestInvokeOpenAIToolCallsWithoutContent(t *testing.T) {
	server := newChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"tool_calls": []map[string]interface{}{
							{
								"id":   "call_1",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "lookup",
									"arguments": `{"value":"abc"}`,
								},
							},
						},
					},
				},
			},
		})
	})
	defer server.Close()

	resp, err := Invoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "use tool"})
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
	if resp.IsFinished {
		t.Fatal("expected tool call response to be unfinished")
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "lookup" || resp.ToolCalls[0].Input != `{"value":"abc"}` {
		t.Fatalf("unexpected tool call: %+v", resp.ToolCalls[0])
	}
}

func TestInvokeOpenAIReasoningOnlyResponse(t *testing.T) {
	server := newChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"reasoning_content": "Let me think"}},
			},
		})
	})
	defer server.Close()

	resp, err := Invoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"})
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
	if resp.Content != "" {
		t.Fatalf("expected empty content, got %q", resp.Content)
	}
	if resp.Thinking != "Let me think" {
		t.Fatalf("expected thinking content, got %q", resp.Thinking)
	}
	if !resp.IsFinished {
		t.Fatal("expected reasoning-only response to be finished")
	}
}

func TestInvokeOpenAIUsageResponse(t *testing.T) {
	server := newChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "hello"}},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     11,
				"completion_tokens": 7,
				"total_tokens":      18,
			},
		})
	})
	defer server.Close()

	resp, err := Invoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"})
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
	if resp.Usage == nil {
		t.Fatal("expected usage to be populated")
	}
	if tokenValue(resp.Usage.InputTokens) != 11 || tokenValue(resp.Usage.OutputTokens) != 7 || tokenValue(resp.Usage.TotalTokens) != 18 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
}

func tokenValue(value *int) int {
	if value == nil {
		return -1
	}
	return *value
}

func TestInvokeSendsToolCallMessages(t *testing.T) {
	server := newChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []openaiChatMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if len(body.Messages) != 3 {
			t.Fatalf("expected 3 messages, got %d", len(body.Messages))
		}
		if body.Messages[1].Role != "assistant" || len(body.Messages[1].ToolCalls) != 1 {
			t.Fatalf("expected assistant tool call message, got %+v", body.Messages[1])
		}
		if body.Messages[2].Role != "tool" || body.Messages[2].ToolCallID != "call_1" || body.Messages[2].Name != "lookup" {
			t.Fatalf("expected tool result message, got %+v", body.Messages[2])
		}
		writeJSON(t, w, map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "done"}},
			},
		})
	})
	defer server.Close()

	_, err := Invoke(context.Background(), testConfig(server.URL), Request{
		Messages: []ctx.Message{
			{Role: ctx.RoleUser, Content: "hi"},
			{
				Role: ctx.RoleAssistant,
				ToolCalls: []ctx.ToolCall{
					{
						ID:   "call_1",
						Type: "function",
						Function: ctx.FunctionCall{
							Name:      "lookup",
							Arguments: `{"value":"abc"}`,
						},
					},
				},
			},
			{Role: ctx.RoleTool, ToolCallID: "call_1", Name: "lookup", Content: "result"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
}

func TestInvokeOpenAISendsMultimodalUserBlocks(t *testing.T) {
	server := newChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []openaiChatMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if len(body.Messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(body.Messages))
		}
		blocks, ok := body.Messages[0].Content.([]interface{})
		if !ok {
			t.Fatalf("expected multimodal content blocks, got %T", body.Messages[0].Content)
		}
		if len(blocks) != 2 {
			t.Fatalf("expected 2 multimodal blocks, got %d", len(blocks))
		}
		textBlock, ok := blocks[0].(map[string]interface{})
		if !ok || textBlock["type"] != "text" || textBlock["text"] != "describe this" {
			t.Fatalf("unexpected text block: %#v", blocks[0])
		}
		imageBlock, ok := blocks[1].(map[string]interface{})
		if !ok || imageBlock["type"] != "image_url" {
			t.Fatalf("unexpected image block: %#v", blocks[1])
		}
		imageURL, ok := imageBlock["image_url"].(map[string]interface{})
		if !ok || imageURL["url"] != "data:image/png;base64,aGVsbG8=" {
			t.Fatalf("unexpected image_url payload: %#v", imageBlock["image_url"])
		}
		writeJSON(t, w, map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "done"}},
			},
		})
	})
	defer server.Close()

	_, err := Invoke(context.Background(), testConfig(server.URL), Request{
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
	})
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
}

func TestOpenAIContentForMessageIncludesFileAttachmentPaths(t *testing.T) {
	content := openAIContentForMessage(ctx.Message{
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
	})

	blocks, ok := content.([]map[string]interface{})
	if !ok || len(blocks) != 1 {
		t.Fatalf("expected one text content block, got %T %#v", content, content)
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

func TestOpenAIToolMessageContentWrapsIsError(t *testing.T) {
	content := openAIContentForMessage(ctx.Message{
		Role:    ctx.RoleTool,
		Content: "boom",
		IsError: true,
	})
	text, ok := content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", content)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("expected JSON payload, got %q: %v", text, err)
	}
	if payload["is_error"] != true || payload["status"] != "failed" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestOpenAIToolMessageContentPreservesModelPayload(t *testing.T) {
	raw := `{"content":"ok","status":"succeeded","transport_status":"completed","semantic_status":"success","process_status":"exited","retryable":false,"duration_ms":1,"truncated":false}`
	content := openAIContentForMessage(ctx.Message{
		Role:    ctx.RoleTool,
		Content: raw,
		IsError: false,
	})
	if content != raw {
		t.Fatalf("expected ModelPayload preserved, got %q", content)
	}
}

func TestInvokeOpenAISendsConfigMaxTokens(t *testing.T) {
	server := newChatServer(t, func(w http.ResponseWriter, r *http.Request) {
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
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "ok"}},
			},
		})
	})
	defer server.Close()

	cfg := testConfig(server.URL)
	cfg.MaxTokens = 8192
	if _, err := Invoke(context.Background(), cfg, Request{UserPrompt: "hi"}); err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
}

func TestInvokeOpenAIRequestDoesNotExposeInternalMetadataNames(t *testing.T) {
	server := newChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		assertNoInternalMetadataKeys(t, body)
		writeJSON(t, w, map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "ok"}},
			},
		})
	})
	defer server.Close()

	if _, err := Invoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"}); err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
}

func TestInvokeOpenAIEmptyChoices(t *testing.T) {
	server := newChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]interface{}{"choices": []interface{}{}})
	})
	defer server.Close()

	_, err := Invoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"})
	if !errors.Is(err, ErrEmptyResponse) {
		t.Fatalf("expected ErrEmptyResponse, got %v", err)
	}
}

func TestInvokeOpenAINonOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadGateway)
	}))
	defer server.Close()

	_, err := Invoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"})
	if !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("expected ErrRequestFailed, got %v", err)
	}
}

func TestInvokeOpenAINonOKIncludesSanitizedProviderDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Your request exceeded model token limit: 262144","api_key":"sk-secretvalue"}}`))
	}))
	defer server.Close()

	_, err := Invoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"})
	if !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("expected ErrRequestFailed, got %v", err)
	}
	text := err.Error()
	if !strings.Contains(text, "status 400") || !strings.Contains(text, "Your request exceeded model token limit: 262144") {
		t.Fatalf("expected provider detail in error, got %q", text)
	}
	if strings.Contains(text, "sk-secretvalue") {
		t.Fatalf("expected provider detail to be redacted, got %q", text)
	}
}

func TestRedactProviderErrorSecretsCoversBearerAndAKTokens(t *testing.T) {
	detail := `Authorization: Bearer ak-secretvalue123456 x-api-key=ak-anothersecret123456 token=sk-secretvalue123456`

	redacted := redactProviderErrorSecrets(detail)

	if strings.Contains(redacted, "ak-secretvalue123456") || strings.Contains(redacted, "ak-anothersecret123456") || strings.Contains(redacted, "sk-secretvalue123456") {
		t.Fatalf("expected tokens to be redacted, got %q", redacted)
	}
	if !strings.Contains(redacted, "Authorization: Bearer [redacted]") {
		t.Fatalf("expected bearer auth marker to remain recognizable, got %q", redacted)
	}
	if !strings.Contains(redacted, "x-api-key=[redacted]") {
		t.Fatalf("expected x-api-key marker to remain recognizable, got %q", redacted)
	}
}

func TestRequestFailedErrorTruncatesProviderDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(strings.Repeat("x", maxProviderErrorBodyBytes+128)))
	}))
	defer server.Close()

	_, err := Invoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"})
	if !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("expected ErrRequestFailed, got %v", err)
	}
	if got := len(err.Error()); got > maxProviderErrorBodyBytes+128 {
		t.Fatalf("expected truncated provider detail, got length %d", got)
	}
	if !strings.HasSuffix(err.Error(), "...") {
		t.Fatalf("expected truncation marker, got %q", err.Error())
	}
}

func newChatServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("expected /chat/completions, got %s", r.URL.Path)
		}
		handler(w, r)
	}))
}

func testConfig(baseURL string) Config {
	return Config{
		APIKey:  "test-key",
		BaseURL: baseURL,
		Model:   "test-model",
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("failed to encode response: %v", err)
	}
}

func assertNoInternalMetadataKeys(t *testing.T, body map[string]json.RawMessage) {
	t.Helper()
	for _, key := range []string{
		"ContextWindowState",
		"contextWindowState",
		"context_window_state",
		"EnvelopeDebug",
		"envelopeDebug",
		"envelope_debug",
	} {
		if _, ok := body[key]; ok {
			t.Fatalf("provider request exposed internal metadata key %q", key)
		}
	}
}

func TestInvokeOpenAINonOKDoesNotLeakResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"sk-test-secret is invalid","detail":"internal path /etc/secrets"}`))
	}))
	defer server.Close()

	_, err := Invoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"})
	if !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("expected ErrRequestFailed, got %v", err)
	}
	msg := err.Error()
	if strings.Contains(msg, "sk-test-secret") {
		t.Fatalf("error message must not contain response body secret: %q", msg)
	}
	if strings.Contains(msg, "/etc/secrets") {
		t.Fatalf("error message must not contain response body detail: %q", msg)
	}
	if !strings.Contains(msg, "status 502") {
		t.Fatalf("error message should contain status code, got %q", msg)
	}
}

func TestInvokeOpenAITransportErrorDoesNotLeakURL(t *testing.T) {
	cfg := testConfig("http://127.0.0.1:1/invalid")
	_, err := Invoke(context.Background(), cfg, Request{UserPrompt: "hi"})
	if !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("expected ErrRequestFailed, got %v", err)
	}
	msg := err.Error()
	if strings.Contains(msg, "127.0.0.1:1") {
		t.Fatalf("error message must not contain full URL host:port: %q", msg)
	}
	if strings.Contains(msg, "/invalid") {
		t.Fatalf("error message must not contain full URL path: %q", msg)
	}
	if !strings.Contains(msg, "transport error") {
		t.Fatalf("error message should contain transport error, got %q", msg)
	}
}

func TestInvokeAnthropicNonOKDoesNotLeakResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"sk-test-secret anthropic detail"}`))
	}))
	defer server.Close()

	cfg := testConfig(server.URL)
	cfg.Protocol = ProtocolAnthropicMessages
	_, err := Invoke(context.Background(), cfg, Request{UserPrompt: "hi"})
	if !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("expected ErrRequestFailed, got %v", err)
	}
	msg := err.Error()
	if strings.Contains(msg, "sk-test-secret") {
		t.Fatalf("error message must not contain response body secret: %q", msg)
	}
	if !strings.Contains(msg, "status 503") {
		t.Fatalf("error message should contain status code, got %q", msg)
	}
}

func TestInvokeAnthropicTransportErrorDoesNotLeakURL(t *testing.T) {
	cfg := testConfig("http://127.0.0.1:1/invalid")
	cfg.Protocol = ProtocolAnthropicMessages
	_, err := Invoke(context.Background(), cfg, Request{UserPrompt: "hi"})
	if !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("expected ErrRequestFailed, got %v", err)
	}
	msg := err.Error()
	if strings.Contains(msg, "127.0.0.1:1") {
		t.Fatalf("error message must not contain full URL host:port: %q", msg)
	}
	if strings.Contains(msg, "/invalid") {
		t.Fatalf("error message must not contain full URL path: %q", msg)
	}
	if !strings.Contains(msg, "transport error") {
		t.Fatalf("error message should contain transport error, got %q", msg)
	}
}

func TestInvokeOpenAIMalformedBaseURLDoesNotLeakURL(t *testing.T) {
	// A control character in the URL forces http.NewRequestWithContext to fail
	// without attempting a network call.
	cfg := testConfig("http://example.com/v1\x00invalid")
	_, err := Invoke(context.Background(), cfg, Request{UserPrompt: "hi"})
	if !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("expected ErrRequestFailed, got %v", err)
	}
	msg := err.Error()
	if strings.Contains(msg, "example.com") {
		t.Fatalf("error message must not contain URL host: %q", msg)
	}
	if strings.Contains(msg, "invalid") {
		t.Fatalf("error message must not contain URL path: %q", msg)
	}
	if !strings.Contains(msg, "request build error") {
		t.Fatalf("error message should contain request build error, got %q", msg)
	}
}

func TestInvokeAnthropicMalformedBaseURLDoesNotLeakURL(t *testing.T) {
	cfg := testConfig("http://example.com/v1\x00invalid")
	cfg.Protocol = ProtocolAnthropicMessages
	_, err := Invoke(context.Background(), cfg, Request{UserPrompt: "hi"})
	if !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("expected ErrRequestFailed, got %v", err)
	}
	msg := err.Error()
	if strings.Contains(msg, "example.com") {
		t.Fatalf("error message must not contain URL host: %q", msg)
	}
	if strings.Contains(msg, "invalid") {
		t.Fatalf("error message must not contain URL path: %q", msg)
	}
	if !strings.Contains(msg, "request build error") {
		t.Fatalf("error message should contain request build error, got %q", msg)
	}
}
