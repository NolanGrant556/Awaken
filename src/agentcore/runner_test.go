package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/tool"
	_ "awaken/src/tools/apply_patch"
	_ "awaken/src/tools/file_read"
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
	server := newRunnerStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body chatRequestSnapshot
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		mu.Lock()
		requests = append(requests, body)
		callNumber := len(requests)
		mu.Unlock()

		if callNumber == 1 {
			writeRunnerStreamToolCall(t, w, "call_native", toolName, map[string]string{"value": "hello"})
			return
		}

		if !containsToolResult(body.Messages, "call_native", "echo:hello") {
			t.Fatalf("expected second request to contain tool result, got %+v", body.Messages)
		}
		writeRunnerStreamText(t, w, "final answer")
	})
	defer server.Close()

	agent := NewAgent(testAgentConfig(server.URL, 3))
	answer, err := agent.RunStreamWithEvents(context.Background(), "use native tool", nil)
	if err != nil {
		t.Fatalf("RunStreamWithEvents returned error: %v", err)
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
	server := newRunnerStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			writeRunnerStreamText(t, w, "<action><name>"+toolName+"</name><input>ping</input></action>")
			return
		}
		writeRunnerStreamText(t, w, "xml final")
	})
	defer server.Close()

	agent := NewAgent(testAgentConfig(server.URL, 3))
	answer, err := agent.RunStreamWithEvents(context.Background(), "use xml tool", nil)
	if err != nil {
		t.Fatalf("RunStreamWithEvents returned error: %v", err)
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
	server := newRunnerStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			writeRunnerStreamToolCall(t, w, "call_fail", toolName, map[string]string{"value": "hello"})
			return
		}
		writeRunnerStreamText(t, w, "handled")
	})
	defer server.Close()

	agent := NewAgent(testAgentConfig(server.URL, 3))
	if _, err := agent.RunStreamWithEvents(context.Background(), "fail tool", nil); err != nil {
		t.Fatalf("RunStreamWithEvents returned error: %v", err)
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
	server := newRunnerStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			writeRunnerStreamToolCall(t, w, "call_create_patch", "apply_patch", map[string]string{"patch": createPatch})
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
			writeRunnerStreamToolCall(t, w, "call_delete_patch", "apply_patch", map[string]string{"patch": deletePatch})
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
			writeRunnerStreamText(t, w, "patch complete")
			return
		}
		t.Fatalf("unexpected call %d", calls)
	})
	defer server.Close()

	answer, err := runAgentWithApprovalSink(context.Background(), testAgentConfigWithWorkspace(server.URL, 5, root), "create then delete via apply_patch", testApprovalAwaiter{
		decision: ApprovalDecisionApproved,
	})
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
			writeRunnerAnthropicToolUseStream(t, w, "toolu_patch", "apply_patch", map[string]string{
				"patch": patch,
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
		writeRunnerAnthropicTextStream(t, w, "anthropic patch complete")
	}))
	defer server.Close()

	cfg := testAgentConfigWithWorkspace(server.URL+"/anthropic", 3, root)
	answer, err := runAgentWithApprovalSink(context.Background(), cfg, "create via anthropic apply_patch", testApprovalAwaiter{
		decision: ApprovalDecisionApproved,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if answer != "anthropic patch complete" {
		t.Fatalf("expected final answer, got %q", answer)
	}
}

func runAgentWithApprovalSink(ctx context.Context, cfg AgentConfig, userInput string, sink EventSink) (string, error) {
	conv := convcontext.NewConversation(nil)
	conv.UserInput(userInput)
	runner := NewRunner(cfg, conv, NewRequestBuilder(cfg))
	return runner.RunStreamWithEvents(ctx, sink)
}

func TestRunnerMaxThoughts(t *testing.T) {
	toolName := "runner_max_tool"
	mustRegisterRunnerTool(t, runnerTestTool{name: toolName})

	server := newRunnerStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeRunnerStreamToolCall(t, w, "call_max", toolName, map[string]string{"value": "again"})
	})
	defer server.Close()

	agent := NewAgent(testAgentConfig(server.URL, 1))
	_, err := agent.RunStreamWithEvents(context.Background(), "loop", nil)
	if err == nil || !strings.Contains(err.Error(), "达到最大思考次数") {
		t.Fatalf("expected max thoughts error, got %v", err)
	}
}

func TestRunnerRunStreamWithEventsEmitsTextDeltas(t *testing.T) {
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
	answer, err := agent.RunStreamWithEvents(context.Background(), "stream", EventSinkFunc(func(event AwakenEvent) {
		if event.Type == TextDeltaEventType {
			chunks = append(chunks, event.Delta)
		}
	}))
	if err != nil {
		t.Fatalf("RunStreamWithEvents returned error: %v", err)
	}
	if answer != "Hello" {
		t.Fatalf("expected Hello, got %q", answer)
	}
	if len(chunks) != 2 || chunks[0] != "Hel" || chunks[1] != "lo" {
		t.Fatalf("expected streamed chunks, got %+v", chunks)
	}
}

func TestRunnerRunStreamWithEventsNativeToolCallLoop(t *testing.T) {
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
	var toolResultSeen bool
	var text string
	answer, err := agent.RunStreamWithEvents(context.Background(), "stream tool", EventSinkFunc(func(event AwakenEvent) {
		if event.Type == ToolResultEventType && strings.Contains(event.Content, "echo:hello") {
			toolResultSeen = true
		}
		if event.Type == TextDeltaEventType {
			text += event.Delta
		}
	}))
	if err != nil {
		t.Fatalf("RunStreamWithEvents returned error: %v", err)
	}
	if answer != "done" {
		t.Fatalf("expected done, got %q", answer)
	}
	if !toolResultSeen || !strings.Contains(text, "done") {
		t.Fatalf("expected tool result and final output, got toolResultSeen=%v text=%q", toolResultSeen, text)
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
	answer, err := agent.RunStreamWithEvents(context.Background(), "stream tool with events", EventSinkFunc(func(event AwakenEvent) {
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
		ToolCallArgsDeltaEventType,
		ToolCallArgsDeltaEventType,
		ToolCallArgsDeltaEventType,
		ReasoningFinishedEventType,
		ToolCallStartedEventType,
		ToolCallArgsEventType,
		ToolCallFinishedEventType,
		ToolProgressEventType,
		ToolProgressEventType,
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
	if gotEvents[3].Delta != "{" || gotEvents[4].Delta != "\"value\":\"hello\"" || gotEvents[5].Delta != "}" {
		t.Fatalf("unexpected tool args deltas: %q %q %q", gotEvents[3].Delta, gotEvents[4].Delta, gotEvents[5].Delta)
	}
	if gotEvents[7].ToolName != toolName || gotEvents[8].ToolName != toolName || gotEvents[9].ToolName != toolName {
		t.Fatalf("expected tool identity on start/args/end events, got %q %q %q", gotEvents[7].ToolName, gotEvents[8].ToolName, gotEvents[9].ToolName)
	}
	if gotEvents[8].Arguments != `{"value":"hello"}` {
		t.Fatalf("expected complete tool args fallback, got %q", gotEvents[8].Arguments)
	}
	if gotEvents[10].Progress != "Executing tool" || gotEvents[11].Progress != "Tool execution complete" {
		t.Fatalf("unexpected tool progress events: %q %q", gotEvents[10].Progress, gotEvents[11].Progress)
	}
	if gotEvents[12].Content != "echo:hello" {
		t.Fatalf("expected tool result content echo:hello, got %q", gotEvents[12].Content)
	}
	if gotEvents[14].Delta != "done" {
		t.Fatalf("expected final text delta done, got %q", gotEvents[14].Delta)
	}
}

func TestRunnerRunStreamWithEventsFeedsToolResultsBack(t *testing.T) {
	toolName := "runner_event_feedback_tool"
	mustRegisterRunnerTool(t, runnerTestTool{name: toolName})

	var calls int
	server := newRunnerAnthropicStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			writeRunnerAnthropicStreamEvents(t, w, []string{
				`event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_feedback","name":"` + toolName + `"}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{"}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"value\":\"hello\""}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"}"}}`,
				`event: content_block_stop
data: {"type":"content_block_stop","index":0}`,
				`event: message_stop
data: {"type":"message_stop"}`,
			})
			return
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
	conv := convcontext.NewConversation(nil)
	conv.UserInput("use tool")
	runner := NewRunner(cfg, conv, NewRequestBuilder(cfg))
	answer, err := runner.RunStreamWithEvents(context.Background(), nil)
	if err != nil {
		t.Fatalf("RunStreamWithEvents returned error: %v", err)
	}
	if answer != "done" {
		t.Fatalf("expected done, got %q", answer)
	}

	messages := conv.Context.GetMessages()
	if len(messages) != 4 {
		t.Fatalf("expected 4 context messages, got %d: %+v", len(messages), messages)
	}
	if messages[0].Role != convcontext.RoleUser {
		t.Fatalf("expected first message to be user, got %+v", messages[0])
	}
	if messages[1].Role != convcontext.RoleAssistant || len(messages[1].ToolCalls) != 1 {
		t.Fatalf("expected assistant message with tool_calls, got %+v", messages[1])
	}
	if messages[1].ToolCalls[0].ID != "toolu_feedback" {
		t.Fatalf("expected tool_call id toolu_feedback, got %q", messages[1].ToolCalls[0].ID)
	}
	if messages[2].Role != convcontext.RoleTool || messages[2].ToolCallID != "toolu_feedback" {
		t.Fatalf("expected tool result message with matching tool_call_id, got %+v", messages[2])
	}
	if messages[2].Content != "echo:hello" {
		t.Fatalf("expected tool result content echo:hello, got %q", messages[2].Content)
	}
	if messages[3].Role != convcontext.RoleAssistant || messages[3].Content != "done" {
		t.Fatalf("expected final assistant message, got %+v", messages[3])
	}
}

func TestRunnerReadOnlyToolOutsideWorkspaceFeedsErrorBack(t *testing.T) {
	workspaceRoot := t.TempDir()

	var calls int
	server := newRunnerAnthropicStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			writeRunnerAnthropicStreamEvents(t, w, []string{
				`event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_outside","name":"read_file"}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{"}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"path\":\"/etc/hosts\""}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"}"}}`,
				`event: content_block_stop
data: {"type":"content_block_stop","index":0}`,
				`event: message_stop
data: {"type":"message_stop"}`,
			})
			return
		}

		writeRunnerAnthropicStreamEvents(t, w, []string{
			`event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
			`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"handled"}}`,
			`event: content_block_stop
data: {"type":"content_block_stop","index":0}`,
			`event: message_stop
data: {"type":"message_stop"}`,
		})
	})
	defer server.Close()

	cfg := testAgentConfig(server.URL+"/anthropic", 3)
	cfg.Workspace.Root = workspaceRoot
	cfg.Workspace.WritableRoots = []string{workspaceRoot}

	var gotEvents []AwakenEvent
	agent := NewAgent(cfg)
	answer, err := agent.RunStreamWithEvents(context.Background(), "read outside workspace", EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))
	if err != nil {
		t.Fatalf("RunStreamWithEvents returned error: %v", err)
	}
	if answer != "handled" {
		t.Fatalf("expected handled, got %q", answer)
	}

	var sawToolResult, sawApproval bool
	var toolResultContent string
	for _, ev := range gotEvents {
		switch ev.Type {
		case ToolResultEventType:
			sawToolResult = true
			toolResultContent = ev.Content
		case ApprovalRequestedEventType:
			sawApproval = true
		}
	}
	if !sawToolResult {
		t.Fatalf("expected ToolResult event, got events: %+v", gotEvents)
	}
	if sawApproval {
		t.Fatalf("unexpected approval request for read-only tool")
	}
	if !strings.Contains(toolResultContent, "outside workspace") && !strings.Contains(toolResultContent, "permission") {
		t.Fatalf("expected tool result to contain workspace denial, got %q", toolResultContent)
	}

	messages := agent.GetConversation().Context.GetMessages()
	if len(messages) != 4 {
		t.Fatalf("expected 4 messages, got %d: %+v", len(messages), messages)
	}
	if messages[1].Role != convcontext.RoleAssistant || len(messages[1].ToolCalls) != 1 {
		t.Fatalf("expected assistant message with tool call, got %+v", messages[1])
	}
	if messages[2].Role != convcontext.RoleTool || messages[2].ToolCallID != "toolu_outside" {
		t.Fatalf("expected tool result message, got %+v", messages[2])
	}
	if !strings.Contains(messages[2].Content, "outside workspace") {
		t.Fatalf("expected tool result content to contain denial, got %q", messages[2].Content)
	}
}

func TestRunnerRunStreamWithEventsPlainQA(t *testing.T) {
	server := newRunnerStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeRunnerStreamLines(t, w, []string{
			`data: {"choices":[{"delta":{"content":"pong"}}]}`,
			`data: [DONE]`,
		})
	})
	defer server.Close()

	cfg := testAgentConfig(server.URL, 2)
	var gotEvents []AwakenEvent
	agent := NewAgent(cfg)
	answer, err := agent.RunStreamWithEvents(context.Background(), "ping", EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))
	if err != nil {
		t.Fatalf("RunStreamWithEvents returned error: %v", err)
	}
	if answer != "pong" {
		t.Fatalf("expected pong, got %q", answer)
	}

	wantTypes := []AwakenEventType{
		RunStartedEventType,
		TextMessageStartedEventType,
		TextDeltaEventType,
		TextMessageFinishedEventType,
		RunFinishedEventType,
	}
	var gotTypes []AwakenEventType
	for _, event := range gotEvents {
		gotTypes = append(gotTypes, event.Type)
	}
	if len(gotTypes) != len(wantTypes) {
		t.Fatalf("unexpected event count: want %d got %d (%v)", len(wantTypes), len(gotTypes), gotTypes)
	}
	for i, want := range wantTypes {
		if gotTypes[i] != want {
			t.Fatalf("unexpected event type at %d: want %s got %s (%v)", i, want, gotTypes[i], gotTypes)
		}
	}

	for _, event := range gotEvents {
		switch event.Type {
		case ToolCallStartedEventType, ToolCallArgsEventType, ToolCallFinishedEventType, ToolResultEventType, ApprovalRequestedEventType:
			t.Fatalf("unexpected event type in plain Q&A: %s", event.Type)
		}
	}
}

func TestRunEventEmitterEmitsTurnDiffAfterMutation(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}

	root := t.TempDir()
	runGitInRepo(t, root, "init")
	runGitInRepo(t, root, "config", "user.email", "test@example.com")
	runGitInRepo(t, root, "config", "user.name", "Test User")

	target := filepath.Join(root, "note.md")
	if err := os.WriteFile(target, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}
	runGitInRepo(t, root, "add", "note.md")
	runGitInRepo(t, root, "commit", "-m", "initial")

	if err := os.WriteFile(target, []byte("world\n"), 0o644); err != nil {
		t.Fatalf("write changed file: %v", err)
	}
	wantDiff := runGitOutputInRepo(t, root, "diff", "--")

	var events []AwakenEvent
	emitter := newRunEventEmitter("thread-test", EventSinkFunc(func(event AwakenEvent) {
		events = append(events, event)
	}))
	emitter.emitTurnDiff(context.Background(), root)
	if len(events) != 0 {
		t.Fatalf("expected no turn diff before mutation marker, got %+v", events)
	}

	emitter.markMutation()
	emitter.emitTurnDiff(context.Background(), root)
	if len(events) != 1 {
		t.Fatalf("expected one turn diff event, got %+v", events)
	}
	if events[0].Type != TurnDiffEventType {
		t.Fatalf("expected TurnDiff event, got %s", events[0].Type)
	}
	if events[0].Diff != wantDiff {
		t.Fatalf("expected turn diff to match git diff\nwant:\n%s\ngot:\n%s", wantDiff, events[0].Diff)
	}
	if events[0].Truncated {
		t.Fatalf("expected small diff not to be truncated")
	}
}

func TestRunEventEmitterEmitsTurnDiffForUntrackedAddedFile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}

	root := t.TempDir()
	runGitInRepo(t, root, "init")
	runGitInRepo(t, root, "config", "user.email", "test@example.com")
	runGitInRepo(t, root, "config", "user.name", "Test User")

	tracked := filepath.Join(root, "tracked.md")
	if err := os.WriteFile(tracked, []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	runGitInRepo(t, root, "add", "tracked.md")
	runGitInRepo(t, root, "commit", "-m", "initial")

	added := filepath.Join(root, "added.md")
	if err := os.WriteFile(added, []byte("created\n"), 0o644); err != nil {
		t.Fatalf("write added file: %v", err)
	}

	var events []AwakenEvent
	emitter := newRunEventEmitter("thread-test", EventSinkFunc(func(event AwakenEvent) {
		events = append(events, event)
	}))
	emitter.markMutation()
	emitter.emitTurnDiff(context.Background(), root)

	if len(events) != 1 {
		t.Fatalf("expected one turn diff event, got %+v", events)
	}
	if events[0].Type != TurnDiffEventType {
		t.Fatalf("expected TurnDiff event, got %s", events[0].Type)
	}
	if !strings.Contains(events[0].Diff, "added.md") || !strings.Contains(events[0].Diff, "+created") {
		t.Fatalf("expected turn diff to include untracked added file, got:\n%s", events[0].Diff)
	}
}

func TestRunnerPropagatesBearerAuthByDefault(t *testing.T) {
	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		writeRunnerStreamText(t, w, "ok")
	}))
	defer server.Close()

	cfg := testAgentConfig(server.URL, 2)
	agent := NewAgent(cfg)
	if _, err := agent.RunStreamWithEvents(context.Background(), "hi", nil); err != nil {
		t.Fatalf("RunStreamWithEvents returned error: %v", err)
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
		writeRunnerStreamText(t, w, "ok")
	}))
	defer server.Close()

	cfg := testAgentConfig(server.URL, 2)
	cfg.ModelConfig.AuthScheme = "x-api-key"
	agent := NewAgent(cfg)
	if _, err := agent.RunStreamWithEvents(context.Background(), "hi", nil); err != nil {
		t.Fatalf("RunStreamWithEvents returned error: %v", err)
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
	if _, err := agent.RunStreamWithEvents(context.Background(), "hi", nil); err != nil {
		t.Fatalf("RunStreamWithEvents returned error: %v", err)
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

	_, err := runner.RunStreamWithEvents(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "invalid tool result pairing") {
		t.Fatalf("expected pairing error, got %v", err)
	}
	if called {
		t.Fatal("expected invalid pairing to be rejected before LLM request")
	}
}

func runGitInRepo(t *testing.T, root string, args ...string) {
	t.Helper()
	_ = runGitOutputInRepo(t, root, args...)
}

func runGitOutputInRepo(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
	return string(output)
}

func TestRunnerCancelInjectsSyntheticToolResults(t *testing.T) {
	server := newRunnerAnthropicStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("expected no LLM call when context is already cancelled")
	})
	defer server.Close()

	conv := convcontext.NewConversation(nil)
	conv.UserInput("use tool")
	conv.AssistantOutput("thinking", []convcontext.ToolCall{
		{ID: "call_orphan_1", Type: "function", Function: convcontext.FunctionCall{Name: "orphan_tool_a", Arguments: `{}`}},
		{ID: "call_orphan_2", Type: "function", Function: convcontext.FunctionCall{Name: "orphan_tool_b", Arguments: `{}`}},
	})

	cfg := testAgentConfig(server.URL+"/anthropic", 3)
	runner := NewRunner(cfg, conv, NewRequestBuilder(cfg))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runner.RunStreamWithEvents(ctx, nil)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled error, got %v", err)
	}

	messages := conv.Context.GetMessages()
	if err := convcontext.ValidateToolResultPairing(messages); err != nil {
		t.Fatalf("expected valid tool result pairing after cancel injection, got %v", err)
	}

	var syntheticResults []convcontext.Message
	for _, msg := range messages {
		if msg.Role == convcontext.RoleTool && msg.IsError {
			syntheticResults = append(syntheticResults, msg)
		}
	}
	if len(syntheticResults) != 2 {
		t.Fatalf("expected 2 synthetic error tool results, got %d: %+v", len(syntheticResults), syntheticResults)
	}

	seenIDs := map[string]bool{}
	for _, msg := range syntheticResults {
		if msg.Content != "工具执行已被取消" {
			t.Fatalf("expected synthetic result content '工具执行已被取消', got %q", msg.Content)
		}
		seenIDs[msg.ToolCallID] = true
	}
	if !seenIDs["call_orphan_1"] || !seenIDs["call_orphan_2"] {
		t.Fatalf("expected synthetic results for call_orphan_1 and call_orphan_2, got %+v", seenIDs)
	}
}

type cancelOnExecuteTool struct {
	name   string
	cancel context.CancelFunc
}

func (t cancelOnExecuteTool) Name() string {
	return t.name
}

func (t cancelOnExecuteTool) Description() string {
	return "cancels the run context on execute"
}

func (t cancelOnExecuteTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "value", Description: "value", Type: "string", Required: true},
	}
}

func (t cancelOnExecuteTool) Execute(ctx context.Context, input string) (string, error) {
	t.cancel()
	return "cancelled", nil
}

func TestRunnerCancelBetweenTurnsExitsEarly(t *testing.T) {
	toolName := "runner_cancel_between_turns"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mustRegisterRunnerTool(t, cancelOnExecuteTool{name: toolName, cancel: cancel})

	var calls int
	server := newRunnerAnthropicStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			writeRunnerAnthropicStreamEvents(t, w, []string{
				`event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_cancel","name":"` + toolName + `"}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{"}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"value\":\"hello\""}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"}"}}`,
				`event: content_block_stop
data: {"type":"content_block_stop","index":0}`,
				`event: message_stop
data: {"type":"message_stop"}`,
			})
			return
		}
		t.Fatalf("expected no second LLM call after cancel between turns, got call %d", calls)
	})
	defer server.Close()

	cfg := testAgentConfig(server.URL+"/anthropic", 3)
	conv := convcontext.NewConversation(nil)
	conv.UserInput("use cancel tool")
	runner := NewRunner(cfg, conv, NewRequestBuilder(cfg))

	_, err := runner.RunStreamWithEvents(ctx, nil)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 LLM call, got %d", calls)
	}

	messages := conv.Context.GetMessages()
	if err := convcontext.ValidateToolResultPairing(messages); err != nil {
		t.Fatalf("expected valid tool result pairing after cancel, got %v", err)
	}
}

type failIfExecutedTool struct {
	name string
}

func (t failIfExecutedTool) Name() string {
	return t.name
}

func (t failIfExecutedTool) Description() string {
	return "fails the test if executed"
}

func (t failIfExecutedTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "value", Description: "value", Type: "string", Required: true},
	}
}

func (t failIfExecutedTool) Execute(ctx context.Context, input string) (string, error) {
	panic("failIfExecutedTool should never be executed when run is cancelled")
}

func TestRunnerCancelStopsSubsequentToolsInBatch(t *testing.T) {
	cancelToolName := "runner_cancel_in_batch"
	failToolName := "runner_must_not_execute_in_batch"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mustRegisterRunnerTool(t, cancelOnExecuteTool{name: cancelToolName, cancel: cancel})
	mustRegisterRunnerTool(t, failIfExecutedTool{name: failToolName})

	var calls int
	server := newRunnerAnthropicStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			writeRunnerAnthropicStreamEvents(t, w, []string{
				`event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_cancel_1","name":"` + cancelToolName + `"}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{"}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"value\":\"hello\""}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"}"}}`,
				`event: content_block_stop
data: {"type":"content_block_stop","index":0}`,
				`event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_cancel_2","name":"` + failToolName + `"}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{"}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"value\":\"world\""}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"}"}}`,
				`event: content_block_stop
data: {"type":"content_block_stop","index":1}`,
				`event: message_stop
data: {"type":"message_stop"}`,
			})
			return
		}
		t.Fatalf("expected no second LLM call after cancel in batch, got call %d", calls)
	})
	defer server.Close()

	cfg := testAgentConfig(server.URL+"/anthropic", 3)
	conv := convcontext.NewConversation(nil)
	conv.UserInput("use cancel then fail tool")
	runner := NewRunner(cfg, conv, NewRequestBuilder(cfg))

	var gotEvents []AwakenEvent
	_, err := runner.RunStreamWithEvents(ctx, EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 LLM call, got %d", calls)
	}

	var toolCallStarts []string
	for _, ev := range gotEvents {
		if ev.Type == ToolCallStartedEventType {
			toolCallStarts = append(toolCallStarts, ev.ToolCallID)
		}
	}
	if len(toolCallStarts) != 1 || toolCallStarts[0] != "toolu_cancel_1" {
		t.Fatalf("expected exactly 1 TOOL_CALL_START for toolu_cancel_1, got %+v", toolCallStarts)
	}

	messages := conv.Context.GetMessages()
	if err := convcontext.ValidateToolResultPairing(messages); err != nil {
		t.Fatalf("expected valid tool result pairing after batch cancel, got %v", err)
	}

	var cancel2Result *convcontext.Message
	for i := range messages {
		msg := messages[i]
		if msg.Role == convcontext.RoleTool && msg.ToolCallID == "toolu_cancel_2" {
			cancel2Result = &messages[i]
			break
		}
	}
	if cancel2Result == nil {
		t.Fatalf("expected synthetic tool result for toolu_cancel_2, got messages: %+v", messages)
	}
	if !cancel2Result.IsError {
		t.Fatalf("expected synthetic result to be error, got %+v", cancel2Result)
	}
	if cancel2Result.Content != "工具执行已被取消" {
		t.Fatalf("expected synthetic result content '工具执行已被取消', got %q", cancel2Result.Content)
	}
}

func TestRunnerXMLFallbackEmitsSingleToolCallEventSet(t *testing.T) {
	toolName := "runner_xml_evt_single"
	mustRegisterRunnerTool(t, runnerTestTool{name: toolName})

	var calls int
	server := newRunnerAnthropicStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			writeRunnerAnthropicStreamEvents(t, w, []string{
				`event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"<action><name>` + toolName + `</name><input>hello</input></action>"}}`,
				`event: content_block_stop
data: {"type":"content_block_stop","index":0}`,
				`event: message_stop
data: {"type":"message_stop"}`,
			})
			return
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
	conv := convcontext.NewConversation(nil)
	conv.UserInput("use xml tool")
	runner := NewRunner(cfg, conv, NewRequestBuilder(cfg))

	var gotEvents []AwakenEvent
	_, err := runner.RunStreamWithEvents(context.Background(), EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))
	if err != nil {
		t.Fatalf("RunStreamWithEvents returned error: %v", err)
	}

	var toolCallStarts, toolCallArgs, toolCallFinished int
	for _, ev := range gotEvents {
		switch ev.Type {
		case ToolCallStartedEventType:
			toolCallStarts++
		case ToolCallArgsEventType:
			toolCallArgs++
		case ToolCallFinishedEventType:
			toolCallFinished++
		}
	}
	if toolCallStarts != 1 || toolCallArgs != 1 || toolCallFinished != 1 {
		t.Fatalf("expected exactly 1 of each tool-call event, got starts=%d args=%d finished=%d",
			toolCallStarts, toolCallArgs, toolCallFinished)
	}
}

func TestRunnerXMLFallbackCancelBeforeExecutionEmitsNoToolCallStart(t *testing.T) {
	toolName := "runner_xml_cancel_before"
	mustRegisterRunnerTool(t, runnerTestTool{name: toolName})

	cfg := testAgentConfig("http://unused", 3)
	conv := convcontext.NewConversation(nil)
	conv.UserInput("use xml tool")
	runner := NewRunner(cfg, conv, NewRequestBuilder(cfg))

	var gotEvents []AwakenEvent
	events := newRunEventEmitter("thread-test", EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	xmlAction := "<action><name>" + toolName + "</name><input>hello</input></action>"
	handled, _ := runner.handleXMLFallback(ctx, xmlAction, events)
	if !handled {
		t.Fatalf("expected XML fallback to be handled")
	}

	for _, ev := range gotEvents {
		if ev.Type == ToolCallStartedEventType {
			t.Fatalf("expected no ToolCallStarted event when ctx is cancelled, got %+v", ev)
		}
	}

	messages := conv.Context.GetMessages()
	if err := convcontext.ValidateToolResultPairing(messages); err != nil {
		t.Fatalf("expected valid tool result pairing after cancel, got %v", err)
	}

	var syntheticResult *convcontext.Message
	for i := range messages {
		if messages[i].Role == convcontext.RoleTool && messages[i].IsError {
			syntheticResult = &messages[i]
			break
		}
	}
	if syntheticResult == nil {
		t.Fatalf("expected synthetic error tool result, got messages: %+v", messages)
	}
	if syntheticResult.Content != "工具执行已被取消" {
		t.Fatalf("expected synthetic result content '工具执行已被取消', got %q", syntheticResult.Content)
	}
}

func TestRunnerRunStreamWithEventsMissingAPIKeyEmitsAuthErrorCode(t *testing.T) {
	server := newRunnerStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("expected no LLM call when API key is missing")
	})
	defer server.Close()

	cfg := testAgentConfig(server.URL, 2)
	cfg.ModelConfig.APIKey = ""
	var gotEvents []AwakenEvent
	agent := NewAgent(cfg)
	_, err := agent.RunStreamWithEvents(context.Background(), "ping", EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))
	if err == nil {
		t.Fatalf("expected error when API key is missing")
	}

	var runErrorEvent *AwakenEvent
	for i := range gotEvents {
		if gotEvents[i].Type == RunErrorEventType {
			runErrorEvent = &gotEvents[i]
			break
		}
	}
	if runErrorEvent == nil {
		t.Fatalf("expected RunError event, got events: %+v", gotEvents)
	}
	if runErrorEvent.Code != "PROVIDER_AUTH_MISSING" {
		t.Fatalf("expected Code PROVIDER_AUTH_MISSING, got %q", runErrorEvent.Code)
	}
}

func TestRunEventEmitterRunErroredSetsRequestFailedCode(t *testing.T) {
	var gotEvents []AwakenEvent
	emitter := newRunEventEmitter("thread-test", EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))

	wrapped := fmt.Errorf("%w: status 500", llm.ErrRequestFailed)
	emitter.runErrored(wrapped)

	if len(gotEvents) != 1 {
		t.Fatalf("expected 1 event, got %d: %+v", len(gotEvents), gotEvents)
	}
	if gotEvents[0].Type != RunErrorEventType {
		t.Fatalf("expected RunError event, got %s", gotEvents[0].Type)
	}
	if gotEvents[0].Code != "PROVIDER_REQUEST_FAILED" {
		t.Fatalf("expected Code PROVIDER_REQUEST_FAILED, got %q", gotEvents[0].Code)
	}
	if !strings.Contains(gotEvents[0].Error, llm.ErrRequestFailed.Error()) {
		t.Fatalf("expected error text to contain %q, got %q", llm.ErrRequestFailed.Error(), gotEvents[0].Error)
	}
}

func TestRunEventEmitterRunErroredTransportErrorDoesNotLeakURL(t *testing.T) {
	var gotEvents []AwakenEvent
	emitter := newRunEventEmitter("thread-test", EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))

	// Simulate a sanitized transport error as produced by llm.transportError.
	wrapped := fmt.Errorf("LLM stream call failed: %w", fmt.Errorf("%w: transport error", llm.ErrRequestFailed))
	emitter.runErrored(wrapped)

	if len(gotEvents) != 1 {
		t.Fatalf("expected 1 event, got %d: %+v", len(gotEvents), gotEvents)
	}
	if gotEvents[0].Type != RunErrorEventType {
		t.Fatalf("expected RunError event, got %s", gotEvents[0].Type)
	}
	if gotEvents[0].Code != "PROVIDER_REQUEST_FAILED" {
		t.Fatalf("expected Code PROVIDER_REQUEST_FAILED, got %q", gotEvents[0].Code)
	}
	msg := gotEvents[0].Error
	if strings.Contains(msg, "127.0.0.1") {
		t.Fatalf("error message must not contain URL host: %q", msg)
	}
	if strings.Contains(msg, "/invalid") {
		t.Fatalf("error message must not contain URL path: %q", msg)
	}
	if !strings.Contains(msg, "transport error") {
		t.Fatalf("error message should contain transport error, got %q", msg)
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

func testAgentConfigWithWorkspace(baseURL string, maxThoughts int, root string) AgentConfig {
	cfg := testAgentConfig(baseURL, maxThoughts)
	if root != "" {
		cfg.Workspace.Root = root
		cfg.Workspace.WritableRoots = []string{root}
	}
	return cfg
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

func writeRunnerStreamText(t *testing.T, w http.ResponseWriter, text string) {
	t.Helper()
	writeRunnerStreamEvent(t, w, map[string]interface{}{
		"choices": []map[string]interface{}{
			{"delta": map[string]interface{}{"content": text}},
		},
	})
	writeRunnerStreamLines(t, w, []string{"data: [DONE]"})
}

func writeRunnerStreamToolCall(t *testing.T, w http.ResponseWriter, id, name string, arguments interface{}) {
	t.Helper()
	argumentData, err := json.Marshal(arguments)
	if err != nil {
		t.Fatalf("failed to marshal tool arguments: %v", err)
	}
	writeRunnerStreamEvent(t, w, map[string]interface{}{
		"choices": []map[string]interface{}{
			{
				"delta": map[string]interface{}{
					"tool_calls": []map[string]interface{}{
						{
							"index": 0,
							"id":    id,
							"type":  "function",
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
	writeRunnerStreamLines(t, w, []string{"data: [DONE]"})
}

func writeRunnerStreamEvent(t *testing.T, w http.ResponseWriter, value interface{}) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("failed to marshal stream event: %v", err)
	}
	writeRunnerStreamLines(t, w, []string{"data: " + string(data)})
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

func writeRunnerAnthropicToolUseStream(t *testing.T, w http.ResponseWriter, id, name string, input interface{}) {
	t.Helper()
	inputJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal anthropic tool input: %v", err)
	}
	writeRunnerAnthropicStreamEvent(t, w, "content_block_start", map[string]interface{}{
		"type":  "content_block_start",
		"index": 0,
		"content_block": map[string]interface{}{
			"type": "tool_use",
			"id":   id,
			"name": name,
		},
	})
	writeRunnerAnthropicStreamEvent(t, w, "content_block_delta", map[string]interface{}{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]interface{}{
			"type":         "input_json_delta",
			"partial_json": string(inputJSON),
		},
	})
	writeRunnerAnthropicStreamEvent(t, w, "content_block_stop", map[string]interface{}{
		"type":  "content_block_stop",
		"index": 0,
	})
	writeRunnerAnthropicStreamEvent(t, w, "message_stop", map[string]interface{}{
		"type": "message_stop",
	})
}

func writeRunnerAnthropicTextStream(t *testing.T, w http.ResponseWriter, text string) {
	t.Helper()
	writeRunnerAnthropicStreamEvent(t, w, "content_block_start", map[string]interface{}{
		"type":  "content_block_start",
		"index": 0,
		"content_block": map[string]interface{}{
			"type": "text",
		},
	})
	writeRunnerAnthropicStreamEvent(t, w, "content_block_delta", map[string]interface{}{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]interface{}{
			"type": "text_delta",
			"text": text,
		},
	})
	writeRunnerAnthropicStreamEvent(t, w, "content_block_stop", map[string]interface{}{
		"type":  "content_block_stop",
		"index": 0,
	})
	writeRunnerAnthropicStreamEvent(t, w, "message_stop", map[string]interface{}{
		"type": "message_stop",
	})
}

func TestBuildLLMRequestCompactsBeforeSending(t *testing.T) {
	conv := convcontext.NewConversation(nil)
	conv.UserInput(strings.Repeat("old ", 24))
	conv.ToolExchange("", []convcontext.ToolCall{
		{ID: "call_old", Type: "function", Function: convcontext.FunctionCall{Name: "lookup", Arguments: `{"path":"old"}`}},
	}, []convcontext.ToolResult{
		{ToolCallID: "call_old", Name: "lookup", Content: strings.Repeat("result ", 24)},
	})
	conv.UserInput("keep latest question")

	cfg := AgentConfig{
		ModelConfig: ModelConfig{
			ContextWindow:         64,
			MaxTokens:             16,
			AutoCompactTokenLimit: 12,
		},
	}

	runner := NewRunner(cfg, conv, NewRequestBuilder(cfg))
	req, err := runner.buildLLMRequest()
	if err != nil {
		t.Fatalf("buildLLMRequest returned error: %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected only the latest user turn to remain, got %+v", req.Messages)
	}
	if req.Messages[0].Role != convcontext.RoleUser || req.Messages[0].Content != "keep latest question" {
		t.Fatalf("unexpected compacted message tail: %+v", req.Messages)
	}
	if err := convcontext.ValidateToolResultPairing(req.Messages); err != nil {
		t.Fatalf("expected compacted request to remain pair-safe, got %v", err)
	}
	if got := conv.Context.GetMessages(); len(got) != 1 || got[0].Content != "keep latest question" {
		t.Fatalf("expected in-memory context to be compacted too, got %+v", got)
	}
}

func writeRunnerAnthropicStreamEvent(t *testing.T, w http.ResponseWriter, name string, value interface{}) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("failed to marshal anthropic stream event: %v", err)
	}
	writeRunnerAnthropicStreamEvents(t, w, []string{
		"event: " + name + "\n" + "data: " + string(data),
	})
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
