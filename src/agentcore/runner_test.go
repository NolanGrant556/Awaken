package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
)

type runnerTestTool struct {
	name string
	fail bool
}

func (t runnerTestTool) Name() string {
	return t.name
}

func (t runnerTestTool) Description() string {
	return "test tool"
}

func (t runnerTestTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "value", Description: "value", Type: "string", Required: true},
	}
}

func (t runnerTestTool) Execute(ctx context.Context, input string) (string, error) {
	if t.fail {
		return "", errors.New("boom")
	}
	return "echo:" + input, nil
}

func TestRunnerNativeToolCallLoop(t *testing.T) {
	toolName := "runner_echo_native"
	tool.RegisterTool(runnerTestTool{name: toolName})

	var mu sync.Mutex
	requests := make([]chatRequestSnapshot, 0)
	server := newRunnerServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body chatRequestSnapshot
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		mu.Lock()
		requests = append(requests, body)
		callNumber := len(requests)
		mu.Unlock()

		if callNumber == 1 {
			writeRunnerJSON(t, w, map[string]interface{}{
				"choices": []map[string]interface{}{
					{
						"message": map[string]interface{}{
							"tool_calls": []map[string]interface{}{
								{
									"id":   "call_native",
									"type": "function",
									"function": map[string]interface{}{
										"name":      toolName,
										"arguments": `{"value":"hello"}`,
									},
								},
							},
						},
					},
				},
			})
			return
		}

		if !containsToolResult(body.Messages, "call_native", "echo:hello") {
			t.Fatalf("expected second request to contain tool result, got %+v", body.Messages)
		}
		writeRunnerJSON(t, w, map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "final answer"}},
			},
		})
	})
	defer server.Close()

	agent := NewAgent(testAgentConfig(server.URL, 3))
	answer, err := agent.Run(context.Background(), "use native tool")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if answer != "final answer" {
		t.Fatalf("expected final answer, got %q", answer)
	}

	messages := agent.GetConversation().Context.GetMessages()
	if len(messages) != 4 {
		t.Fatalf("expected 4 context messages, got %d: %+v", len(messages), messages)
	}
	if len(messages[1].ToolCalls) != 1 {
		t.Fatalf("expected assistant tool call in context, got %+v", messages[1])
	}
	if messages[2].Role != convcontext.RoleTool || messages[2].Content != "echo:hello" {
		t.Fatalf("expected tool result message, got %+v", messages[2])
	}
}

func TestRunnerXMLFallbackLoop(t *testing.T) {
	toolName := "runner_echo_xml"
	tool.RegisterTool(runnerTestTool{name: toolName})

	var calls int
	server := newRunnerServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			writeRunnerJSON(t, w, map[string]interface{}{
				"choices": []map[string]interface{}{
					{"message": map[string]interface{}{"content": "<action><name>" + toolName + "</name><input>ping</input></action>"}},
				},
			})
			return
		}
		writeRunnerJSON(t, w, map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "xml final"}},
			},
		})
	})
	defer server.Close()

	agent := NewAgent(testAgentConfig(server.URL, 3))
	answer, err := agent.Run(context.Background(), "use xml tool")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if answer != "xml final" {
		t.Fatalf("expected xml final, got %q", answer)
	}

	messages := agent.GetConversation().Context.GetMessages()
	if len(messages) < 3 || len(messages[1].ToolCalls) != 1 || messages[2].Content != "echo:ping" {
		t.Fatalf("expected XML tool call and result in context, got %+v", messages)
	}
}

func TestRunnerToolErrorObservation(t *testing.T) {
	toolName := "runner_fail_tool"
	tool.RegisterTool(runnerTestTool{name: toolName, fail: true})

	var calls int
	server := newRunnerServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			writeRunnerJSON(t, w, map[string]interface{}{
				"choices": []map[string]interface{}{
					{
						"message": map[string]interface{}{
							"tool_calls": []map[string]interface{}{
								{
									"id":   "call_fail",
									"type": "function",
									"function": map[string]interface{}{
										"name":      toolName,
										"arguments": `{"value":"hello"}`,
									},
								},
							},
						},
					},
				},
			})
			return
		}
		writeRunnerJSON(t, w, map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "handled"}},
			},
		})
	})
	defer server.Close()

	agent := NewAgent(testAgentConfig(server.URL, 3))
	if _, err := agent.Run(context.Background(), "fail tool"); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	messages := agent.GetConversation().Context.GetMessages()
	if len(messages) < 3 || !strings.Contains(messages[2].Content, "工具执行失败") {
		t.Fatalf("expected tool failure observation, got %+v", messages)
	}
}

func TestRunnerMaxThoughts(t *testing.T) {
	toolName := "runner_max_tool"
	tool.RegisterTool(runnerTestTool{name: toolName})

	server := newRunnerServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeRunnerJSON(t, w, map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"tool_calls": []map[string]interface{}{
							{
								"id":   "call_max",
								"type": "function",
								"function": map[string]interface{}{
									"name":      toolName,
									"arguments": `{"value":"again"}`,
								},
							},
						},
					},
				},
			},
		})
	})
	defer server.Close()

	agent := NewAgent(testAgentConfig(server.URL, 1))
	_, err := agent.Run(context.Background(), "loop")
	if err == nil || !strings.Contains(err.Error(), "达到最大思考次数") {
		t.Fatalf("expected max thoughts error, got %v", err)
	}
}

func TestRunnerRunStreamEmitsTextDeltas(t *testing.T) {
	server := newRunnerStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeRunnerStreamLines(t, w, []string{
			`data: {"choices":[{"delta":{"content":"Hel"}}]}`,
			`data: {"choices":[{"delta":{"content":"lo"}}]}`,
			`data: [DONE]`,
		})
	})
	defer server.Close()

	agent := NewAgent(testAgentConfig(server.URL, 2))
	var chunks []string
	answer, err := agent.RunStream(context.Background(), "stream", func(content string, isThinking bool) {
		if isThinking {
			t.Fatalf("unexpected thinking callback: %q", content)
		}
		chunks = append(chunks, content)
	})
	if err != nil {
		t.Fatalf("RunStream returned error: %v", err)
	}
	if answer != "Hello" {
		t.Fatalf("expected Hello, got %q", answer)
	}
	if len(chunks) != 2 || chunks[0] != "Hel" || chunks[1] != "lo" {
		t.Fatalf("expected streamed chunks, got %+v", chunks)
	}
}

func TestRunnerRunStreamNativeToolCallLoop(t *testing.T) {
	toolName := "runner_stream_tool"
	tool.RegisterTool(runnerTestTool{name: toolName})

	var calls int
	server := newRunnerStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			writeRunnerStreamLines(t, w, []string{
				`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_stream","type":"function","function":{"name":"` + toolName + `","arguments":"{\"value\""}}]}}]}`,
				`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"hello\"}"}}]}}]}`,
				`data: [DONE]`,
			})
			return
		}
		writeRunnerStreamLines(t, w, []string{
			`data: {"choices":[{"delta":{"content":"done"}}]}`,
			`data: [DONE]`,
		})
	})
	defer server.Close()

	agent := NewAgent(testAgentConfig(server.URL, 3))
	var output string
	answer, err := agent.RunStream(context.Background(), "stream tool", func(content string, isThinking bool) {
		if !isThinking {
			output += content
		}
	})
	if err != nil {
		t.Fatalf("RunStream returned error: %v", err)
	}
	if answer != "done" {
		t.Fatalf("expected done, got %q", answer)
	}
	if !strings.Contains(output, "echo:hello") || !strings.Contains(output, "done") {
		t.Fatalf("expected tool result and final output, got %q", output)
	}
}

type chatRequestSnapshot struct {
	Messages []struct {
		Role       string `json:"role"`
		Content    string `json:"content"`
		ToolCallID string `json:"tool_call_id"`
		Name       string `json:"name"`
	} `json:"messages"`
}

func containsToolResult(messages []struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id"`
	Name       string `json:"name"`
}, id, content string) bool {
	for _, msg := range messages {
		if msg.Role == "tool" && msg.ToolCallID == id && msg.Content == content {
			return true
		}
	}
	return false
}

func testAgentConfig(baseURL string, maxThoughts int) AgentConfig {
	return AgentConfig{
		MaxThoughts: maxThoughts,
		EntryPoint:  "test",
		ModelConfig: ModelConfig{
			APIKey:  "test-key",
			BaseURL: baseURL,
			Model:   "test-model",
		},
	}
}

func newRunnerServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("expected /chat/completions, got %s", r.URL.Path)
		}
		handler(w, r)
	}))
}

func newRunnerStreamServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("expected /chat/completions, got %s", r.URL.Path)
		}
		handler(w, r)
	}))
}

func writeRunnerJSON(t *testing.T, w http.ResponseWriter, value interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("failed to encode response: %v", err)
	}
}

func writeRunnerStreamLines(t *testing.T, w http.ResponseWriter, lines []string) {
	t.Helper()
	w.Header().Set("Content-Type", "text/event-stream")
	for _, line := range lines {
		if _, err := w.Write([]byte(line + "\n\n")); err != nil {
			t.Fatalf("failed to write stream line: %v", err)
		}
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}
}
