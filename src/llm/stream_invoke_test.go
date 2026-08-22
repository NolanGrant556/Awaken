package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamInvokeOpenAITextDeltas(t *testing.T) {
	server := newStreamServer(t, "/chat/completions", []string{
		`data: {"choices":[{"delta":{"content":"Hel"}}]}`,
		`data: {"choices":[{"delta":{"content":"lo"}}]}`,
		`data: [DONE]`,
	})
	defer server.Close()

	var chunks []string
	resp, err := StreamInvoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"}, func(content string, isThinking bool) {
		if isThinking {
			t.Fatalf("unexpected thinking callback: %q", content)
		}
		chunks = append(chunks, content)
	})
	if err != nil {
		t.Fatalf("StreamInvoke returned error: %v", err)
	}
	if resp.Content != "Hello" {
		t.Fatalf("expected Hello, got %q", resp.Content)
	}
	if !resp.IsFinished {
		t.Fatal("expected text-only stream to be finished")
	}
	if len(chunks) != 2 || chunks[0] != "Hel" || chunks[1] != "lo" {
		t.Fatalf("expected two streamed chunks, got %+v", chunks)
	}
}

func TestStreamInvokeOpenAICleanEOFBeforeTerminalIsDisconnected(t *testing.T) {
	server := newStreamServer(t, "/chat/completions", []string{
		`data: {"choices":[{"delta":{"content":"partial"}}]}`,
	})
	defer server.Close()

	resp, err := StreamInvoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"}, nil)
	var disconnected *StreamDisconnectedError
	if !errors.As(err, &disconnected) {
		t.Fatalf("clean EOF before a terminal marker must disconnect, response=%+v err=%v", resp, err)
	}
}

func TestStreamInvokeOpenAIFinishReasonCompletesWithoutDoneSentinel(t *testing.T) {
	server := newStreamServer(t, "/chat/completions", []string{
		`data: {"choices":[{"delta":{"content":"complete"},"finish_reason":"stop"}]}`,
	})
	defer server.Close()

	resp, err := StreamInvoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"}, nil)
	if err != nil || resp == nil || resp.Content != "complete" {
		t.Fatalf("finish_reason is an explicit terminal signal, response=%+v err=%v", resp, err)
	}
}

func TestStreamInvokeOpenAIReasoningContentDeltas(t *testing.T) {
	server := newStreamServer(t, "/chat/completions", []string{
		`data: {"choices":[{"delta":{"reasoning_content":"Let "}}]}`,
		`data: {"choices":[{"delta":{"reasoning_content":"me think"}}]}`,
		`data: {"choices":[{"delta":{"content":"done"}}]}`,
		`data: [DONE]`,
	})
	defer server.Close()

	var thinking string
	var text string
	resp, err := StreamInvoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"}, func(content string, isThinking bool) {
		if isThinking {
			thinking += content
			return
		}
		text += content
	})
	if err != nil {
		t.Fatalf("StreamInvoke returned error: %v", err)
	}
	if thinking != "Let me think" {
		t.Fatalf("expected thinking deltas, got %q", thinking)
	}
	if text != "done" || resp.Content != "done" {
		t.Fatalf("expected text done, got callback=%q response=%q", text, resp.Content)
	}
	if resp.Thinking != "Let me think" {
		t.Fatalf("expected response thinking, got %q", resp.Thinking)
	}
}

func TestStreamInvokeOpenAIReasoningOnlyDoesNotReturnEmptyResponse(t *testing.T) {
	server := newStreamServer(t, "/chat/completions", []string{
		`data: {"choices":[{"delta":{"reasoning_content":"step 1"}}]}`,
		`data: {"choices":[{"delta":{"reasoning_content":" -> step 2"}}]}`,
		`data: [DONE]`,
	})
	defer server.Close()

	var thinking string
	resp, err := StreamInvoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"}, func(content string, isThinking bool) {
		if isThinking {
			thinking += content
		}
	})
	if err != nil {
		t.Fatalf("StreamInvoke returned error: %v", err)
	}
	if resp.Content != "" {
		t.Fatalf("expected empty content, got %q", resp.Content)
	}
	if thinking != "step 1 -> step 2" || resp.Thinking != thinking {
		t.Fatalf("expected reasoning-only response, callback=%q response=%q", thinking, resp.Thinking)
	}
	if !resp.IsFinished {
		t.Fatal("expected reasoning-only stream to be finished")
	}
}

func TestStreamInvokeMiniMaxReasoningDetailsSnapshotsBecomeUnifiedDeltas(t *testing.T) {
	server := newStreamServer(t, "/chat/completions", []string{
		`data: {"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.text","text":"step"}]}}]}`,
		`data: {"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.text","text":"step one"}]}}]}`,
		`data: {"choices":[{"delta":{"content":"answer"}}]}`,
		`data: {"choices":[{"delta":{"content":"answer done"}}]}`,
		`data: [DONE]`,
	})
	defer server.Close()

	cfg := testConfig(server.URL)
	cfg.ProviderID = "minimax"
	var thinking, answer string
	resp, err := StreamInvoke(context.Background(), cfg, Request{UserPrompt: "hi"}, func(content string, isThinking bool) {
		if isThinking {
			thinking += content
		} else {
			answer += content
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if thinking != "step one" || resp.Thinking != "step one" {
		t.Fatalf("reasoning snapshot duplicated or lost: callback=%q response=%q", thinking, resp.Thinking)
	}
	if answer != "answer done" || resp.Content != "answer done" {
		t.Fatalf("content snapshot duplicated or lost: callback=%q response=%q", answer, resp.Content)
	}
}

func TestStreamInvokeLeadingThinkTagsSplitAcrossChunks(t *testing.T) {
	server := newStreamServer(t, "/chat/completions", []string{
		`data: {"choices":[{"delta":{"content":"<thi"}}]}`,
		`data: {"choices":[{"delta":{"content":"nk>private "}}]}`,
		`data: {"choices":[{"delta":{"content":"reasoning</thi"}}]}`,
		`data: {"choices":[{"delta":{"content":"nk>\nfinal"}}]}`,
		`data: [DONE]`,
	})
	defer server.Close()

	var thinking, answer string
	resp, err := StreamInvoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"}, func(content string, isThinking bool) {
		if isThinking {
			thinking += content
		} else {
			answer += content
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if thinking != "private reasoning" || resp.Thinking != thinking {
		t.Fatalf("inline thinking was not normalized: callback=%q response=%q", thinking, resp.Thinking)
	}
	if answer != "final" || resp.Content != "final" || strings.Contains(resp.Content, "<think>") {
		t.Fatalf("inline thinking leaked into answer: callback=%q response=%q", answer, resp.Content)
	}
}

func TestPrepareMiniMaxChatRequestsSeparatedReasoning(t *testing.T) {
	cfg := Config{
		APIKey: "test", BaseURL: "https://api.minimaxi.com/v1", Model: "MiniMax-M3",
		Protocol: ProtocolOpenAIChat, ProviderID: "minimax", ProfileID: "profile-minimax",
	}
	prepared, err := PrepareRequest(cfg, Request{UserPrompt: "hi"}, true)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["reasoning_split"] != true {
		t.Fatalf("MiniMax request missing reasoning_split: %s", prepared.Body)
	}
	if !prepared.Cache.ReasoningReplay {
		t.Fatal("MiniMax separated reasoning must be retained for exact-contract continuation")
	}
}

func TestStreamInvokeOpenAIMaxOutputStopReason(t *testing.T) {
	server := newStreamServer(t, "/chat/completions", []string{
		`data: {"choices":[{"delta":{"content":"partial"}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"length"}]}`,
		`data: [DONE]`,
	})
	defer server.Close()

	resp, err := StreamInvoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"}, nil)
	if err != nil {
		t.Fatalf("StreamInvoke returned error: %v", err)
	}
	if resp.Content != "partial" {
		t.Fatalf("expected partial content, got %q", resp.Content)
	}
	if resp.StopReason != "length" {
		t.Fatalf("expected stop reason length, got %q", resp.StopReason)
	}
	if !resp.MaxOutputReached {
		t.Fatal("expected max output marker")
	}
}

func TestStreamInvokeOpenAIUsageChunk(t *testing.T) {
	server := newStreamServer(t, "/chat/completions", []string{
		`data: {"choices":[{"delta":{"content":"partial"}}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13}}`,
		`data: [DONE]`,
	})
	defer server.Close()

	resp, err := StreamInvoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"}, nil)
	if err != nil {
		t.Fatalf("StreamInvoke returned error: %v", err)
	}
	if resp.Usage == nil {
		t.Fatal("expected usage to be populated")
	}
	if tokenValue(resp.Usage.InputTokens) != 9 || tokenValue(resp.Usage.OutputTokens) != 4 || tokenValue(resp.Usage.TotalTokens) != 13 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
}

func TestStreamingHTTPClientDoesNotUseTotalTimeout(t *testing.T) {
	if streamHTTPClient.Timeout != 0 {
		t.Fatalf("expected streaming HTTP client to avoid total timeout, got %s", streamHTTPClient.Timeout)
	}
	if httpClient.Timeout != defaultProviderRequestTimeout {
		t.Fatalf("expected non-streaming HTTP client timeout %s, got %s", defaultProviderRequestTimeout, httpClient.Timeout)
	}
}

func TestStreamIdleTimeoutForRequestOverride(t *testing.T) {
	t.Setenv("AWAKEN_LLM_STREAM_IDLE_TIMEOUT_MS", "25")
	if got := streamIdleTimeoutForRequest(Request{}); got != 25*time.Millisecond {
		t.Fatalf("expected env stream idle timeout 25ms, got %s", got)
	}
	if got := streamIdleTimeoutForRequest(Request{StreamIdleTimeout: 5 * time.Minute}); got != 5*time.Minute {
		t.Fatalf("expected request stream idle timeout 5m, got %s", got)
	}
}

func TestStreamInvokeOpenAIDisableReasoningWithThinkingTypeDisabledProviders(t *testing.T) {
	cases := []struct {
		name       string
		providerID string
		baseURL    string
		model      string
	}{
		{name: "deepseek", providerID: "deepseek", baseURL: "https://api.deepseek.com", model: "deepseek-v4-pro"},
		{name: "kimi cn", providerID: "kimi-cn", baseURL: "https://api.moonshot.cn/v1", model: "kimi-k2.6"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chatReq := openaiChatRequest{Model: tc.model}
			applyOpenAIReasoningControls(&chatReq, Config{ProviderID: tc.providerID, BaseURL: tc.baseURL, Protocol: ProtocolOpenAIChat, Model: tc.model}, Request{DisableReasoning: true})
			if chatReq.Thinking == nil {
				t.Fatalf("expected %s to use thinking.type disabled", tc.name)
			}
			if chatReq.Thinking.Type != "disabled" {
				t.Fatalf("expected thinking.type disabled, got %+v", chatReq.Thinking)
			}
		})
	}
}

func TestStreamInvokeOpenAIKimiK27AliasSuppressesDisableReasoning(t *testing.T) {
	requests := make([]map[string]json.RawMessage, 0, 2)
	server := newStreamInspectServer(t, "/chat/completions", func(r *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		requests = append(requests, body)
	})
	defer server.Close()

	cfg := testConfig(server.URL)
	cfg.Model = "kimi-for-coding"
	if _, err := StreamInvoke(context.Background(), cfg, Request{
		UserPrompt:       "compact",
		DisableReasoning: true,
	}, nil); err != nil {
		t.Fatalf("StreamInvoke returned error: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected one request, got %d", len(requests))
	}
	if _, ok := requests[0]["thinking"]; ok {
		t.Fatalf("Kimi K2.7 alias must not include thinking.disabled: %+v", requests[0])
	}

	if _, err := StreamInvoke(context.Background(), cfg, Request{UserPrompt: "normal"}, nil); err != nil {
		t.Fatalf("StreamInvoke returned error: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("expected two requests, got %d", len(requests))
	}
	if _, ok := requests[1]["thinking"]; ok {
		t.Fatalf("normal request must not include compact-only thinking controls: %+v", requests[1])
	}
}

func TestStreamInvokeOpenAICompactSummaryAppliesProviderTemperaturePolicy(t *testing.T) {
	cases := []struct {
		name             string
		model            string
		wantTemperature  float64
		wantThinkingType string
		wantNoThinking   bool
	}{
		{name: "kimi coding standard always thinking", model: "kimi-for-coding", wantTemperature: 1.0, wantNoThinking: true},
		{name: "kimi coding highspeed always thinking", model: "kimi-for-coding-highspeed", wantTemperature: 1.0, wantNoThinking: true},
		{name: "kimi k2.6 instant", model: "kimi-k2.6", wantTemperature: 0.6, wantNoThinking: true},
		{name: "kimi k2.7 always thinking", model: "kimi-k2.7-code", wantTemperature: 1.0, wantNoThinking: true},
		{name: "kimi k3", model: "k3", wantTemperature: 1.0, wantNoThinking: true},
		{name: "kimi k3 256k", model: "k3-256k", wantTemperature: 1.0, wantNoThinking: true},
		{name: "kimi k3 cn", model: "kimi-k3", wantTemperature: 1.0, wantNoThinking: true},
		{name: "deepseek deterministic compact", model: "deepseek-v4-pro", wantTemperature: 0.0, wantNoThinking: true},
		{name: "ark deterministic compact", model: "ark-code-latest", wantTemperature: 0.1, wantNoThinking: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]json.RawMessage
			server := newStreamInspectServer(t, "/chat/completions", func(r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("failed to decode request: %v", err)
				}
			})
			defer server.Close()

			cfg := testConfig(server.URL)
			cfg.Model = tc.model
			_, err := StreamInvoke(context.Background(), cfg, Request{
				UserPrompt:       "compact",
				DisableReasoning: true,
				Purpose:          RequestPurposeCompactSummary,
			}, nil)
			if err != nil {
				t.Fatalf("StreamInvoke returned error: %v", err)
			}

			gotTemperature := decodeRequiredFloat(t, body, "temperature")
			if gotTemperature != tc.wantTemperature {
				t.Fatalf("expected temperature %.1f, got %v in body %+v", tc.wantTemperature, gotTemperature, body)
			}
			if tc.wantNoThinking {
				if _, ok := body["thinking"]; ok {
					t.Fatalf("expected no thinking field for %s, got %+v", tc.model, body)
				}
				return
			}
			var thinking struct {
				Type string `json:"type"`
			}
			if raw := body["thinking"]; len(raw) == 0 {
				t.Fatalf("expected thinking field for %s, got %+v", tc.model, body)
			} else if err := json.Unmarshal(raw, &thinking); err != nil {
				t.Fatalf("failed to decode thinking field: %v", err)
			}
			if thinking.Type != tc.wantThinkingType {
				t.Fatalf("expected thinking.type %s, got %+v", tc.wantThinkingType, thinking)
			}
		})
	}
}

func TestStreamInvokeOpenAIKimiK27DisableReasoningIsSuppressed(t *testing.T) {
	models := []string{
		"kimi-k2.7-code",
		"kimi-k2.7-code-highspeed",
		"kimi-for-coding",
		"kimi-for-coding-highspeed",
	}
	for _, model := range models {
		chatReq := openaiChatRequest{Model: model}
		applyOpenAIReasoningControls(&chatReq, Config{ProviderID: "kimi-coding-plan", BaseURL: "https://api.kimi.com/coding/v1", Protocol: ProtocolOpenAIChat, Model: model}, Request{DisableReasoning: true})
		if chatReq.Thinking != nil {
			t.Fatalf("Kimi K2.7 Code model %q rejects thinking.disabled, got %+v", model, chatReq.Thinking)
		}
	}
}

func TestResolveTemperatureOmitsUnknownAndPreservesExplicitValues(t *testing.T) {
	if got := resolveTemperature(Config{}, Request{OmitTemperature: true}); got != nil {
		t.Fatalf("expected omitted temperature, got %v", *got)
	}
	if got := resolveTemperature(Config{Temperature: 1}, Request{TemperatureOverride: float64Pointer(0)}); got == nil || *got != 0 {
		t.Fatalf("expected explicit zero temperature, got %v", got)
	}
	if got := resolveTemperature(Config{}, Request{}); got != nil {
		t.Fatalf("expected unspecified temperature to be omitted, got %v", *got)
	}
	if got := resolveTemperature(Config{Temperature: 0.4}, Request{}); got == nil || *got != 0.4 {
		t.Fatalf("expected configured temperature 0.4, got %v", got)
	}
	if got := resolveTemperature(Config{}, Request{Temperature: 0.2}); got == nil || *got != 0.2 {
		t.Fatalf("expected request temperature 0.2, got %v", got)
	}
}

func TestRequiredTemperatureFromError(t *testing.T) {
	temp, ok := RequiredTemperatureFromError(errors.New("LLM request failed: status 400: invalid temperature: only 0.6 is allowed for this model"))
	if !ok || temp != 0.6 {
		t.Fatalf("expected required temperature 0.6, got %v ok=%v", temp, ok)
	}
}

func TestStreamInvokeOpenAIIdleTimeoutForHungBody(t *testing.T) {
	t.Setenv("AWAKEN_LLM_STREAM_IDLE_TIMEOUT_MS", "25")
	server := newIdleStallStreamServer(t, "/chat/completions")
	defer server.Close()

	start := time.Now()
	_, err := StreamInvoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"}, nil)
	if err == nil {
		t.Fatal("expected idle timeout error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("expected idle timeout quickly, took %s", elapsed)
	}
	if !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("expected ErrRequestFailed wrapper, got %v", err)
	}
	if !strings.Contains(err.Error(), "stream idle timeout after 25ms") {
		t.Fatalf("expected stream idle timeout detail, got %v", err)
	}
}

func TestStreamInvokeOpenAIToolCallDeltas(t *testing.T) {
	server := newStreamServer(t, "/chat/completions", []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"value\""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"abc\"}"}}]}}]}`,
		`data: [DONE]`,
	})
	defer server.Close()

	resp, err := StreamInvoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "tool"}, nil)
	if err != nil {
		t.Fatalf("StreamInvoke returned error: %v", err)
	}
	if resp.IsFinished {
		t.Fatal("expected tool call stream to be unfinished")
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].ID != "call_1" || resp.ToolCalls[0].Name != "lookup" || resp.ToolCalls[0].Input != `{"value":"abc"}` {
		t.Fatalf("unexpected tool call: %+v", resp.ToolCalls[0])
	}
}

func TestStreamInvokeWithObserverOpenAIToolCallArgumentDeltas(t *testing.T) {
	server := newStreamServer(t, "/chat/completions", []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"value\""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"abc\"}"}}]}}]}`,
		`data: [DONE]`,
	})
	defer server.Close()

	var deltas []ToolCallDelta
	resp, err := StreamInvokeWithObserver(context.Background(), testConfig(server.URL), Request{UserPrompt: "tool"}, StreamObserver{
		OnToolCallDelta: func(delta ToolCallDelta) {
			deltas = append(deltas, delta)
		},
	})
	if err != nil {
		t.Fatalf("StreamInvokeWithObserver returned error: %v", err)
	}
	if len(deltas) != 2 {
		t.Fatalf("expected two argument deltas, got %+v", deltas)
	}
	if deltas[0].Index != 0 || deltas[0].ID != "call_1" || deltas[0].Name != "lookup" || deltas[0].ArgumentsDelta != `{"value"` {
		t.Fatalf("unexpected first delta: %+v", deltas[0])
	}
	if deltas[1].Index != 0 || deltas[1].ID != "call_1" || deltas[1].Name != "lookup" || deltas[1].ArgumentsDelta != `:"abc"}` {
		t.Fatalf("unexpected second delta: %+v", deltas[1])
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Input != `{"value":"abc"}` {
		t.Fatalf("unexpected response tool calls: %+v", resp.ToolCalls)
	}
}

func TestStreamInvokeOpenAIToolCallFallbackID(t *testing.T) {
	server := newStreamServer(t, "/chat/completions", []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"type":"function","function":{"name":"lookup","arguments":"{}"}}]}}]}`,
		`data: [DONE]`,
	})
	defer server.Close()

	resp, err := StreamInvoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "tool"}, nil)
	if err != nil {
		t.Fatalf("StreamInvoke returned error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].ID != "tool_index_0" {
		t.Fatalf("expected fallback tool id tool_index_0, got %q", resp.ToolCalls[0].ID)
	}
}

func TestStreamInvokeAnthropicThinkingAndTextDeltas(t *testing.T) {
	server := newStreamServer(t, "/anthropic/v1/messages", []string{
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"hmm"}}`,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"done"}}`,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
	})
	defer server.Close()

	var thinking string
	var text string
	resp, err := StreamInvoke(context.Background(), anthropicConfig(server.URL), Request{UserPrompt: "hi"}, func(content string, isThinking bool) {
		if isThinking {
			thinking += content
			return
		}
		text += content
	})
	if err != nil {
		t.Fatalf("StreamInvoke returned error: %v", err)
	}
	if thinking != "hmm" {
		t.Fatalf("expected thinking hmm, got %q", thinking)
	}
	if text != "done" || resp.Content != "done" {
		t.Fatalf("expected text done, got callback=%q response=%q", text, resp.Content)
	}
}

func TestStreamInvokeAnthropicIdleTimeoutForHungBody(t *testing.T) {
	t.Setenv("AWAKEN_LLM_STREAM_IDLE_TIMEOUT_MS", "25")
	server := newIdleStallStreamServer(t, "/anthropic/v1/messages")
	defer server.Close()

	_, err := StreamInvoke(context.Background(), anthropicConfig(server.URL), Request{UserPrompt: "hi"}, nil)
	if err == nil {
		t.Fatal("expected idle timeout error")
	}
	if !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("expected ErrRequestFailed wrapper, got %v", err)
	}
	if !strings.Contains(err.Error(), "stream idle timeout after 25ms") {
		t.Fatalf("expected stream idle timeout detail, got %v", err)
	}
}

func TestStreamInvokeOpenAITextDeltasNoSpace(t *testing.T) {
	server := newStreamServer(t, "/chat/completions", []string{
		`data:{"choices":[{"delta":{"content":"Hel"}}]}`,
		`data:{"choices":[{"delta":{"content":"lo"}}]}`,
		`data:[DONE]`,
	})
	defer server.Close()

	var chunks []string
	resp, err := StreamInvoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"}, func(content string, isThinking bool) {
		if isThinking {
			t.Fatalf("unexpected thinking callback: %q", content)
		}
		chunks = append(chunks, content)
	})
	if err != nil {
		t.Fatalf("StreamInvoke returned error: %v", err)
	}
	if resp.Content != "Hello" {
		t.Fatalf("expected Hello, got %q", resp.Content)
	}
	if len(chunks) != 2 || chunks[0] != "Hel" || chunks[1] != "lo" {
		t.Fatalf("expected two streamed chunks, got %+v", chunks)
	}
}

func TestStreamInvokeOpenAIToolCallDeltasNoSpace(t *testing.T) {
	server := newStreamServer(t, "/chat/completions", []string{
		`data:{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"value\""}}]}}]}`,
		`data:{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"abc\"}"}}]}}]}`,
		`data:[DONE]`,
	})
	defer server.Close()

	resp, err := StreamInvoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "tool"}, nil)
	if err != nil {
		t.Fatalf("StreamInvoke returned error: %v", err)
	}
	if resp.IsFinished {
		t.Fatal("expected tool call stream to be unfinished")
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].ID != "call_1" || resp.ToolCalls[0].Name != "lookup" || resp.ToolCalls[0].Input != `{"value":"abc"}` {
		t.Fatalf("unexpected tool call: %+v", resp.ToolCalls[0])
	}
}

func newStreamServer(t *testing.T, expectedPath string, lines []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != expectedPath {
			t.Fatalf("expected %s, got %s", expectedPath, r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, line := range lines {
			if _, err := w.Write([]byte(line + "\n\n")); err != nil {
				t.Fatalf("failed to write stream line: %v", err)
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}))
}

func newStreamInspectServer(t *testing.T, expectedPath string, inspect func(*http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != expectedPath {
			t.Fatalf("expected %s, got %s", expectedPath, r.URL.Path)
		}
		if inspect != nil {
			inspect(r)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"ok"}}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
}

func newIdleStallStreamServer(t *testing.T, expectedPath string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != expectedPath {
			t.Fatalf("expected %s, got %s", expectedPath, r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
}

func decodeRequiredFloat(t *testing.T, body map[string]json.RawMessage, key string) float64 {
	t.Helper()
	raw, ok := body[key]
	if !ok {
		t.Fatalf("expected %q in body %+v", key, body)
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("failed to decode %q as float: %v", key, err)
	}
	return value
}
