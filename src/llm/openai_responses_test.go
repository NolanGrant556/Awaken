package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	convcontext "awaken/src/context"
)

func TestPrepareOpenAIResponsesRequestIsStatelessAndProtocolSpecific(t *testing.T) {
	cfg := Config{APIKey: "test", BaseURL: "https://api.openai.com/v1", Model: "gpt-test", ProviderID: "openai", ProfileID: "p1", Protocol: ProtocolOpenAIResponses}
	prepared, err := PrepareRequest(cfg, Request{SystemPrompt: "system", Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "hello"}}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Protocol != ProtocolOpenAIResponses || prepared.Endpoint != "https://api.openai.com/v1/responses" {
		t.Fatalf("unexpected prepared route: %+v", prepared)
	}
	var body map[string]any
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["store"] != false || body["instructions"] != "system" {
		t.Fatalf("unexpected Responses body: %s", prepared.Body)
	}
	if _, ok := body["previous_response_id"]; ok {
		t.Fatalf("Responses request must remain stateless: %s", prepared.Body)
	}
	if body["prompt_cache_key"] == "" {
		t.Fatalf("official OpenAI Responses request should carry an affinity key: %s", prepared.Body)
	}
}

func TestInvokeOpenAIResponsesNormalizesTextToolsAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "resp_1", "status": "completed",
			"output": []any{
				map[string]any{"type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": "think"}}},
				map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": "done"}}},
				map[string]any{"type": "function_call", "call_id": "call_1", "name": "Read", "arguments": `{"file_path":"README.md"}`},
			},
			"usage": map[string]any{"input_tokens": 20, "output_tokens": 8, "total_tokens": 28, "input_tokens_details": map[string]any{"cached_tokens": 12}, "output_tokens_details": map[string]any{"reasoning_tokens": 3}},
		})
	}))
	defer server.Close()
	cfg := Config{APIKey: "test", BaseURL: server.URL, Model: "gpt-test", Protocol: ProtocolOpenAIResponses}
	resp, err := Invoke(context.Background(), cfg, Request{UserPrompt: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "done" || resp.Thinking != "think" || len(resp.ToolCalls) != 1 || resp.ProviderRequestID != "resp_1" {
		t.Fatalf("unexpected response %+v", resp)
	}
	if resp.Usage == nil || resp.Usage.CachedInputTokens == nil || *resp.Usage.CachedInputTokens != 12 || resp.Usage.UncachedInputTokens == nil || *resp.Usage.UncachedInputTokens != 8 {
		t.Fatalf("unexpected normalized usage %+v", resp.Usage)
	}
}

func TestStreamOpenAIResponsesNormalizesEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		lines := []string{
			`data: {"type":"response.reasoning_summary_text.delta","delta":"why"}`,
			`data: {"type":"response.output_text.delta","delta":"ok"}`,
			`data: {"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","call_id":"call_1","name":"Read"}}`,
			`data: {"type":"response.function_call_arguments.delta","output_index":1,"delta":"{\"file_path\":\"README.md\"}"}`,
			`data: {"type":"response.output_item.done","output_index":1,"item":{"type":"function_call","call_id":"call_1","name":"Read","arguments":"{\"file_path\":\"README.md\"}"}}`,
			`data: {"type":"response.completed","response":{"id":"resp_stream","status":"completed","usage":{"input_tokens":4,"output_tokens":2,"total_tokens":6}}}`,
			`data: [DONE]`,
		}
		for _, line := range lines {
			_, _ = w.Write([]byte(line + "\n\n"))
		}
	}))
	defer server.Close()
	var text, thinking string
	var typed []StreamEventType
	resp, err := StreamInvokeWithObserver(context.Background(), Config{APIKey: "test", BaseURL: server.URL, Model: "gpt-test", Protocol: ProtocolOpenAIResponses}, Request{UserPrompt: "hi"}, StreamObserver{
		OnContent: func(chunk string, isThinking bool) {
			if isThinking {
				thinking += chunk
			} else {
				text += chunk
			}
		},
		OnEvent: func(event StreamEvent) { typed = append(typed, event.Type) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "ok" || thinking != "why" || len(resp.ToolCalls) != 1 || resp.ProviderRequestID != "resp_stream" {
		t.Fatalf("unexpected stream response %+v callbacks=%q/%q", resp, text, thinking)
	}
	joined := make([]string, len(typed))
	for i, value := range typed {
		joined[i] = string(value)
	}
	eventNames := strings.Join(joined, ",")
	if !strings.Contains(eventNames, "tool_call_start") || !strings.Contains(eventNames, "tool_call_delta") ||
		!strings.Contains(eventNames, "tool_call_done") || !strings.Contains(eventNames, "usage") {
		t.Fatalf("missing normalized stream events: %v", typed)
	}
}

func TestUnknownProtocolFailsClosedWithoutRequest(t *testing.T) {
	_, err := Invoke(context.Background(), Config{APIKey: "test", BaseURL: "https://api.anthropic.com", Model: "claude", Protocol: Protocol("mystery")}, Request{UserPrompt: "hi"})
	if !errors.Is(err, ErrUnsupportedProtocol) {
		t.Fatalf("expected unsupported protocol, got %v", err)
	}
}

func TestAnthropicOfficialRequestAppliesCacheControl(t *testing.T) {
	cfg := Config{APIKey: "test", BaseURL: "https://api.anthropic.com", Model: "claude-test", ProviderID: "anthropic", ProfileID: "p1", Protocol: ProtocolAnthropicMessages, AuthScheme: "x-api-key"}
	prepared, err := PrepareRequest(cfg, Request{SystemPrompt: "stable", UserPrompt: "hi"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prepared.Body), `"cache_control":{"type":"ephemeral"}`) {
		t.Fatalf("expected Anthropic cache breakpoint: %s", prepared.Body)
	}
}
