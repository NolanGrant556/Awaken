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

func TestPrepareOpenAIResponsesRequestMarksHistoricalOutputItemsCompleted(t *testing.T) {
	prepared, err := PrepareRequest(Config{
		APIKey: "test", BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
		Model: "doubao-seed-2-1-pro-260628", ProviderID: "doubao", Protocol: ProtocolOpenAIResponses,
	}, Request{Messages: []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "first"},
		{
			Role: convcontext.RoleAssistant, Content: "checking",
			ToolCalls: []convcontext.ToolCall{{
				ID: "call_1", Type: "function",
				Function: convcontext.FunctionCall{Name: "Read", Arguments: `{"file_path":"README.md"}`},
			}},
		},
		{Role: convcontext.RoleTool, ToolCallID: "call_1", Name: "Read", Content: "done"},
		{Role: convcontext.RoleUser, Content: "second"},
	}}, false)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Input []map[string]any `json:"input"`
	}
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Input) != 5 {
		t.Fatalf("expected five Responses input items, got %d: %s", len(body.Input), prepared.Body)
	}
	if body.Input[0]["type"] != "message" || body.Input[0]["role"] != "user" {
		t.Fatalf("user history must be an explicit input message: %#v", body.Input[0])
	}
	if _, exists := body.Input[0]["status"]; exists {
		t.Fatalf("user input message must not invent output status: %#v", body.Input[0])
	}
	if body.Input[1]["type"] != "message" || body.Input[1]["role"] != "assistant" || body.Input[1]["status"] != "completed" {
		t.Fatalf("assistant history must be a completed output message: %#v", body.Input[1])
	}
	if body.Input[2]["type"] != "function_call" || body.Input[2]["status"] != "completed" {
		t.Fatalf("historical function call must be completed: %#v", body.Input[2])
	}
	if body.Input[3]["type"] != "function_call_output" {
		t.Fatalf("tool result must remain a function call output: %#v", body.Input[3])
	}
	if _, exists := body.Input[3]["status"]; exists {
		t.Fatalf("function call output must not invent status: %#v", body.Input[3])
	}
	if body.Input[4]["type"] != "message" || body.Input[4]["role"] != "user" {
		t.Fatalf("current user turn must be an explicit input message: %#v", body.Input[4])
	}
}

func TestPrepareOpenAIResponsesRequestKeepsCompletedHistoryAcrossPurposes(t *testing.T) {
	purposes := []RequestPurpose{
		RequestPurposeAgentTurn,
		RequestPurposeSubagent,
		RequestPurposeScheduledTask,
		RequestPurposeThreadTitle,
		RequestPurposeCompactSummary,
		RequestPurposeMemoryRecall,
		RequestPurposeMemoryMaintenance,
		RequestPurposeConnectionTest,
		RequestPurposeHookPrompt,
		RequestPurposeHookAgent,
	}
	for _, purpose := range purposes {
		t.Run(string(purpose), func(t *testing.T) {
			prepared, err := PrepareRequest(Config{
				APIKey: "test", BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
				Model: "doubao-seed-2-1-pro-260628", ProviderID: "doubao", Protocol: ProtocolOpenAIResponses,
			}, Request{
				Purpose: purpose,
				Messages: []convcontext.Message{
					{Role: convcontext.RoleUser, Content: "first"},
					{Role: convcontext.RoleAssistant, Content: "reply"},
					{Role: convcontext.RoleUser, Content: "second"},
				},
			}, false)
			if err != nil {
				t.Fatal(err)
			}
			var body struct {
				Input []map[string]any `json:"input"`
			}
			if err := json.Unmarshal(prepared.Body, &body); err != nil {
				t.Fatal(err)
			}
			if len(body.Input) != 3 || body.Input[1]["type"] != "message" || body.Input[1]["status"] != "completed" {
				t.Fatalf("purpose %s lost completed assistant history: %s", purpose, prepared.Body)
			}
		})
	}
}

func TestPrepareDeepSeekResponsesAuxiliaryRequestDisablesReasoning(t *testing.T) {
	prepared, err := PrepareRequest(Config{
		APIKey: "test", BaseURL: "https://api.deepseek.com", Model: "deepseek-v4-pro",
		ProviderID: "deepseek", Protocol: ProtocolOpenAIResponses,
	}, Request{Purpose: RequestPurposeCompactSummary, DisableReasoning: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Reasoning *openAIResponsesReasoning `json:"reasoning"`
	}
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.Reasoning == nil || body.Reasoning.Effort != "none" {
		t.Fatalf("DeepSeek compact summary must explicitly disable default reasoning: %s", prepared.Body)
	}
}

func TestPrepareOpenAIResponsesAuxiliaryRequestDoesNotLeakDeepSeekPrivateControl(t *testing.T) {
	tests := []Config{
		{APIKey: "test", BaseURL: "https://api.openai.com/v1", Model: "gpt-test", ProviderID: "openai", Protocol: ProtocolOpenAIResponses},
		{APIKey: "test", BaseURL: "https://proxy.example/v1", Model: "deepseek-v4-pro", ProviderID: "deepseek", Protocol: ProtocolOpenAIResponses},
		{APIKey: "test", BaseURL: "https://api.deepseek.com", Model: "custom-model", ProviderID: "deepseek", Protocol: ProtocolOpenAIResponses},
	}
	for _, cfg := range tests {
		prepared, err := PrepareRequest(cfg, Request{Purpose: RequestPurposeCompactSummary, DisableReasoning: true}, false)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(prepared.Body, &body); err != nil {
			t.Fatal(err)
		}
		if _, exists := body["reasoning"]; exists {
			t.Fatalf("unmatched Responses endpoint inherited DeepSeek control: %s", prepared.Body)
		}
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

func TestStreamOpenAIResponsesCleanEOFBeforeTerminalIsDisconnected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"type":"response.output_text.delta","delta":"partial"}` + "\n\n"))
	}))
	defer server.Close()

	resp, err := StreamInvokeWithObserver(
		context.Background(),
		Config{APIKey: "test", BaseURL: server.URL, Model: "gpt-test", Protocol: ProtocolOpenAIResponses},
		Request{UserPrompt: "hi"},
		StreamObserver{},
	)
	var disconnected *StreamDisconnectedError
	if !errors.As(err, &disconnected) {
		t.Fatalf("clean EOF before response.completed/[DONE] must disconnect, response=%+v err=%v", resp, err)
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
