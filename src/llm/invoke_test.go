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

func TestInvokeOpenAIProviderUsage(t *testing.T) {
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
		t.Fatal("expected usage metadata")
	}
	if resp.Usage.PromptTokens != 11 || resp.Usage.CompletionTokens != 7 || resp.Usage.TotalTokens != 18 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
	if resp.Usage.Estimated {
		t.Fatalf("provider usage should not be marked estimated: %+v", resp.Usage)
	}
	if resp.Usage.Protocol != string(ProtocolOpenAIChat) || resp.Usage.Model != "test-model" {
		t.Fatalf("expected protocol/model metadata, got %+v", resp.Usage)
	}
	if resp.Usage.Provider != "127.0.0.1" {
		t.Fatalf("expected provider from test server host, got %+v", resp.Usage)
	}
}

func TestInvokeEstimatesUsageWhenProviderOmitsUsage(t *testing.T) {
	server := newChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "hello"}},
			},
		})
	})
	defer server.Close()

	resp, err := Invoke(context.Background(), testConfig(server.URL), Request{
		SystemPrompt: "system prompt",
		UserPrompt:   "hello world",
	})
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
	if resp.Usage == nil {
		t.Fatal("expected estimated usage")
	}
	if !resp.Usage.Estimated {
		t.Fatalf("expected estimated usage, got %+v", resp.Usage)
	}
	if resp.Usage.InputBytes != len("system prompt")+len("hello world") {
		t.Fatalf("unexpected input bytes: %+v", resp.Usage)
	}
	if resp.Usage.PromptTokens == 0 || resp.Usage.CompletionTokens != 0 {
		t.Fatalf("unexpected estimated tokens: %+v", resp.Usage)
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

func TestInvokeOpenAIIgnoresRequestCacheMetadata(t *testing.T) {
	server := newChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if _, ok := body["cache"]; ok {
			t.Fatalf("OpenAI request must not serialize request cache metadata: %s", body["cache"])
		}
		messagesJSON := string(body["messages"])
		if strings.Contains(messagesJSON, "cache_control") {
			t.Fatalf("OpenAI messages must not include Anthropic cache_control: %s", messagesJSON)
		}
		writeJSON(t, w, map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "ok"}},
			},
		})
	})
	defer server.Close()

	_, err := Invoke(context.Background(), testConfig(server.URL), Request{
		SystemPrompt: "you are a test",
		UserPrompt:   "hi",
		Cache: RequestCacheMetadata{
			SystemPromptCacheControl: CacheControl{Type: CacheControlTypeEphemeral},
			CacheKey:                 "request-cache-key",
			Hints:                    []string{"stable-system"},
		},
	})
	if err != nil {
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

func TestInvokeOpenAINonOKDoesNotLeakHeaderLikeSecrets(t *testing.T) {
	secret := "abcdefghijklmnopqrstuvwxyz123456"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Authorization: Bearer ` + secret + `","x-api-key":"provider-key-value"}`))
	}))
	defer server.Close()

	_, err := Invoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"})
	if !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("expected ErrRequestFailed, got %v", err)
	}
	msg := err.Error()
	for _, leaked := range []string{secret, "provider-key-value", "Authorization", "x-api-key"} {
		if strings.Contains(msg, leaked) {
			t.Fatalf("error message must not contain provider body detail %q: %q", leaked, msg)
		}
	}
	if !strings.Contains(msg, "status 401") {
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
