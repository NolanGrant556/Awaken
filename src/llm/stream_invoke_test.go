package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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
