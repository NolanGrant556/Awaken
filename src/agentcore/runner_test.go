package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
	_ "awaken/src/tools/apply_patch"
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
	mustRegisterRunnerTool(t, runnerTestTool{name: toolName})

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
	mustRegisterRunnerTool(t, runnerTestTool{name: toolName})

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
	mustRegisterRunnerTool(t, runnerTestTool{name: toolName, fail: true})

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

func TestRunnerOpenAIApplyPatchCreateAndDelete(t *testing.T) {
	root := t.TempDir()
	target := root + "/runner_patch.txt"
	createPatch := "*** Begin Patch\n" +
		"*** Add File: " + target + "\n" +
		"+created by runner\n" +
		"*** End Patch"
	deletePatch := "*** Begin Patch\n" +
		"*** Delete File: " + target + "\n" +
		"*** End Patch"

	var calls int
	server := newRunnerServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			writeRunnerToolCallJSON(t, w, "call_create_patch", "apply_patch", map[string]string{"patch": createPatch})
			return
		}
		if calls == 2 {
			var body chatRequestSnapshot
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode request: %v", err)
			}
			if !containsToolResultSubstring(body.Messages, "call_create_patch", "Success. Updated the following files") {
				t.Fatalf("expected create patch result, got %+v", body.Messages)
			}
			if content, err := os.ReadFile(target); err != nil || string(content) != "created by runner\n" {
				t.Fatalf("expected created file, content=%q err=%v", string(content), err)
			}
			writeRunnerToolCallJSON(t, w, "call_delete_patch", "apply_patch", map[string]string{"patch": deletePatch})
			return
		}
		if calls == 3 {
			var body chatRequestSnapshot
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode request: %v", err)
			}
			if !containsToolResultSubstring(body.Messages, "call_delete_patch", "Success. Updated the following files") {
				t.Fatalf("expected delete patch result, got %+v", body.Messages)
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatalf("expected deleted file, stat err=%v", err)
			}
			writeRunnerJSON(t, w, map[string]interface{}{
				"choices": []map[string]interface{}{
					{"message": map[string]interface{}{"content": "patch complete"}},
				},
			})
			return
		}
		t.Fatalf("unexpected call %d", calls)
	})
	defer server.Close()

	agent := NewAgent(testAgentConfig(server.URL, 5))
	answer, err := agent.Run(context.Background(), "create then delete via apply_patch")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if answer != "patch complete" {
		t.Fatalf("expected final answer, got %q", answer)
	}
}

func TestRunnerAnthropicApplyPatchToolUse(t *testing.T) {
	root := t.TempDir()
	target := root + "/anthropic_patch.txt"
	patch := "*** Begin Patch\n" +
		"*** Add File: " + target + "\n" +
		"+created by anthropic\n" +
		"*** End Patch"

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/anthropic/v1/messages" {
			t.Fatalf("expected /anthropic/v1/messages, got %s", r.URL.Path)
		}
		calls++
		if calls == 1 {
			writeRunnerJSON(t, w, map[string]interface{}{
				"content": []map[string]interface{}{
					{
						"type":  "tool_use",
						"id":    "toolu_patch",
						"name":  "apply_patch",
						"input": map[string]string{"patch": patch},
					},
				},
				"stop_reason": "tool_use",
			})
			return
		}

		var body struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode anthropic request: %v", err)
		}
		if !anthropicRequestContainsToolResult(body.Messages, "toolu_patch") {
			t.Fatalf("expected anthropic tool_result, got %+v", body.Messages)
		}
		if content, err := os.ReadFile(target); err != nil || string(content) != "created by anthropic\n" {
			t.Fatalf("expected created file, content=%q err=%v", string(content), err)
		}
		writeRunnerJSON(t, w, map[string]interface{}{
			"content":     []map[string]interface{}{{"type": "text", "text": "anthropic patch complete"}},
			"stop_reason": "end_turn",
		})
	}))
	defer server.Close()

	cfg := testAgentConfig(server.URL+"/anthropic", 3)
	agent := NewAgent(cfg)
	answer, err := agent.Run(context.Background(), "create via anthropic apply_patch")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if answer != "anthropic patch complete" {
		t.Fatalf("expected final answer, got %q", answer)
	}
}

func TestRunnerMaxThoughts(t *testing.T) {
	toolName := "runner_max_tool"
	mustRegisterRunnerTool(t, runnerTestTool{name: toolName})

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
	mustRegisterRunnerTool(t, runnerTestTool{name: toolName})

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

func TestRunnerRunStreamWithEventsEmitsLifecycleSequence(t *testing.T) {
	toolName := "runner_event_tool"
	mustRegisterRunnerTool(t, runnerTestTool{name: toolName})

	var calls int
	server := newRunnerAnthropicStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			writeRunnerAnthropicStreamEvents(t, w, []string{
				`event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me think"}}`,
				`event: content_block_stop
data: {"type":"content_block_stop","index":0}`,
				`event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_evt","name":"` + toolName + `"}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{"}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"value\":\"hello\""}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"}"}}`,
				`event: content_block_stop
data: {"type":"content_block_stop","index":1}`,
				`event: message_stop
data: {"type":"message_stop"}`,
			})
			return
		}

		var body struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode anthropic request: %v", err)
		}
		if !anthropicRequestContainsToolResult(body.Messages, "toolu_evt") {
			t.Fatalf("expected tool result in second request, got %+v", body.Messages)
		}

		writeRunnerAnthropicStreamEvents(t, w, []string{
			`event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
			`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"done"}}`,
			`event: content_block_stop
data: {"type":"content_block_stop","index":0}`,
			`event: message_stop
data: {"type":"message_stop"}`,
		})
	})
	defer server.Close()

	cfg := testAgentConfig(server.URL+"/anthropic", 3)
	var gotEvents []AwakenEvent
	agent := NewAgent(cfg)
	answer, err := agent.RunStreamWithEvents(context.Background(), "stream tool with events", nil, EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))
	if err != nil {
		t.Fatalf("RunStreamWithEvents returned error: %v", err)
	}
	if answer != "done" {
		t.Fatalf("expected done, got %q", answer)
	}

	var gotTypes []AwakenEventType
	for _, event := range gotEvents {
		gotTypes = append(gotTypes, event.Type)
	}

	wantTypes := []AwakenEventType{
		RunStartedEventType,
		ReasoningStartedEventType,
		ReasoningDeltaEventType,
		ReasoningFinishedEventType,
		ToolCallStartedEventType,
		ToolCallArgsEventType,
		ToolCallFinishedEventType,
		ToolResultEventType,
		TextMessageStartedEventType,
		TextDeltaEventType,
		TextMessageFinishedEventType,
		RunFinishedEventType,
	}
	if len(gotTypes) != len(wantTypes) {
		t.Fatalf("unexpected event count: want %d got %d (%v)", len(wantTypes), len(gotTypes), gotTypes)
	}
	for i, want := range wantTypes {
		if gotTypes[i] != want {
			t.Fatalf("unexpected event type at %d: want %s got %s (%v)", i, want, gotTypes[i], gotTypes)
		}
	}
	if gotEvents[7].Content != "echo:hello" {
		t.Fatalf("expected tool result content echo:hello, got %q", gotEvents[7].Content)
	}
	if gotEvents[9].Delta != "done" {
		t.Fatalf("expected final text delta done, got %q", gotEvents[9].Delta)
	}
}

func TestRunnerPropagatesBearerAuthByDefault(t *testing.T) {
	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		writeRunnerJSON(t, w, map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "ok"}},
			},
		})
	}))
	defer server.Close()

	cfg := testAgentConfig(server.URL, 2)
	agent := NewAgent(cfg)
	if _, err := agent.Run(context.Background(), "hi"); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if got := capturedHeaders.Get("Authorization"); got != "Bearer test-key" {
		t.Fatalf("expected Authorization 'Bearer test-key', got %q", got)
	}
	if got := capturedHeaders.Get("x-api-key"); got != "" {
		t.Fatalf("expected no x-api-key header, got %q", got)
	}
}

func TestRunnerPropagatesXAPIKeyAuthScheme(t *testing.T) {
	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		writeRunnerJSON(t, w, map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "ok"}},
			},
		})
	}))
	defer server.Close()

	cfg := testAgentConfig(server.URL, 2)
	cfg.ModelConfig.AuthScheme = "x-api-key"
	agent := NewAgent(cfg)
	if _, err := agent.Run(context.Background(), "hi"); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if got := capturedHeaders.Get("Authorization"); got != "" {
		t.Fatalf("expected no Authorization header, got %q", got)
	}
	if got := capturedHeaders.Get("x-api-key"); got != "test-key" {
		t.Fatalf("expected x-api-key 'test-key', got %q", got)
	}
	if got := capturedHeaders.Get("anthropic-version"); got != "2023-06-01" {
		t.Fatalf("expected anthropic-version '2023-06-01', got %q", got)
	}
}

func TestRunnerPropagatesAuthSchemeInStream(t *testing.T) {
	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		writeRunnerStreamLines(t, w, []string{
			`data: {"choices":[{"delta":{"content":"ok"}}]}`,
			`data: [DONE]`,
		})
	}))
	defer server.Close()

	cfg := testAgentConfig(server.URL, 2)
	cfg.ModelConfig.AuthScheme = "x-api-key"
	agent := NewAgent(cfg)
	if _, err := agent.RunStream(context.Background(), "hi", nil); err != nil {
		t.Fatalf("RunStream returned error: %v", err)
	}

	if got := capturedHeaders.Get("x-api-key"); got != "test-key" {
		t.Fatalf("expected x-api-key 'test-key' in stream, got %q", got)
	}
}

func TestRunnerRejectsInvalidToolResultPairingBeforeLLM(t *testing.T) {
	var called bool
	server := newRunnerServer(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		writeRunnerJSON(t, w, map[string]interface{}{"choices": []interface{}{}})
	})
	defer server.Close()

	conv := convcontext.NewConversation(nil)
	conv.AssistantOutput("", []convcontext.ToolCall{
		{ID: "call_missing", Type: "function", Function: convcontext.FunctionCall{Name: "lookup", Arguments: `{}`}},
	})
	runner := NewRunner(testAgentConfig(server.URL, 1), conv, NewRequestBuilder(testAgentConfig(server.URL, 1)))

	_, err := runner.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid tool result pairing") {
		t.Fatalf("expected pairing error, got %v", err)
	}
	if called {
		t.Fatal("expected invalid pairing to be rejected before LLM request")
	}
}

func mustRegisterRunnerTool(t *testing.T, candidate tool.Tool) {
	t.Helper()
	if err := tool.RegisterTool(candidate); err != nil {
		t.Fatalf("failed to register test tool %s: %v", candidate.Name(), err)
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

type toolResultIn struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
}

func anthropicRequestContainsToolResult(messages []struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}, id string) bool {
	for _, msg := range messages {
		if msg.Role != "user" {
			continue
		}
		var blocks []toolResultIn
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			continue
		}
		for _, block := range blocks {
			if block.Type == "tool_result" && block.ToolUseID == id {
				return true
			}
		}
	}
	return false
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

func containsToolResultSubstring(messages []struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id"`
	Name       string `json:"name"`
}, id, content string) bool {
	for _, msg := range messages {
		if msg.Role == "tool" && msg.ToolCallID == id && strings.Contains(msg.Content, content) {
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

func newRunnerAnthropicStreamServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/anthropic/v1/messages" {
			t.Fatalf("expected /anthropic/v1/messages, got %s", r.URL.Path)
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

func writeRunnerToolCallJSON(t *testing.T, w http.ResponseWriter, id, name string, arguments interface{}) {
	t.Helper()
	argumentData, err := json.Marshal(arguments)
	if err != nil {
		t.Fatalf("failed to marshal tool arguments: %v", err)
	}
	writeRunnerJSON(t, w, map[string]interface{}{
		"choices": []map[string]interface{}{
			{
				"message": map[string]interface{}{
					"tool_calls": []map[string]interface{}{
						{
							"id":   id,
							"type": "function",
							"function": map[string]interface{}{
								"name":      name,
								"arguments": string(argumentData),
							},
						},
					},
				},
			},
		},
	})
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

func writeRunnerAnthropicStreamEvents(t *testing.T, w http.ResponseWriter, events []string) {
	t.Helper()
	w.Header().Set("Content-Type", "text/event-stream")
	for _, event := range events {
		if _, err := io.WriteString(w, event+"\n\n"); err != nil {
			t.Fatalf("failed to write anthropic stream event: %v", err)
		}
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}
}
