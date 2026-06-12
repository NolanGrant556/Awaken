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
	if len(chunks) != 2 || chunks[0] != "Hel" || chunks[1] != "lo" {
		t.Fatalf("expected two streamed chunks, got %+v", chunks)
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
	resp, err := StreamInvoke(context.Background(), testConfig(server.URL+"/anthropic"), Request{UserPrompt: "hi"}, func(content string, isThinking bool) {
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
