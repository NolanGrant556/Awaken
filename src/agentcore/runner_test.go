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

func (t runnerTestTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        t.name,
		DisplayName: t.name,
		Source:      tool.SourceBuiltin,
		Description: "test tool",
		Annotations: tool.ToolAnnotations{ReadOnly: true},
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
	if messages[2].Role != convcontext.RoleTool || !toolContentMatches(messages[2].Content, "echo:hello") {
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
	if len(messages) < 3 || !strings.Contains(messages[2].Content, "echo:ping") {
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
	if len(messages) < 3 || !messages[2].IsError || !strings.Contains(messages[2].Content, "boom") {
		t.Fatalf("expected tool failure observation, got %+v", messages)
	}
}

func TestRunnerOpenAIApplyPatchAutoAllowFeedsBack(t *testing.T) {
	root := t.TempDir()
	target := root + "/runner_patch.txt"
	createPatch := "*** Begin Patch\n" +
		"*** Add File: " + target + "\n" +
		"+created by runner\n" +
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
			// Task 4: apply_patch within workspace is now auto-allowed
			// (boundary + sensitive-path + trash constraints). The file
			// must exist and the tool result must be a success.
			if _, err := os.Stat(target); err != nil {
				t.Fatalf("expected AUTO-allowed patch to create file, stat err=%v", err)
			}
			if containsToolResultSubstring(body.Messages, "call_create_patch", "工具执行被 AUTO policy 拒绝") {
				t.Fatalf("expected AUTO allow tool result, got denial in %+v", body.Messages)
			}
			if !containsToolResultSubstring(body.Messages, "call_create_patch", "Success") {
				t.Fatalf("expected AUTO allow tool result to contain Success, got %+v", body.Messages)
			}
			writeRunnerStreamText(t, w, "patch applied")
			return
		}
		t.Fatalf("unexpected call %d", calls)
	})
	defer server.Close()

	var sawAuthRequired bool
	var gotEvents []AwakenEvent
	agent := NewAgent(testAgentConfigWithWorkspace(server.URL, 3, root))
	answer, err := agent.RunStreamWithEvents(context.Background(), "create via apply_patch", EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
		if event.Type == AuthRequiredEventType {
			sawAuthRequired = true
		}
	}))
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if answer != "patch applied" {
		t.Fatalf("expected final answer, got %q", answer)
	}
	if sawAuthRequired {
		t.Fatalf("expected AUTO allow path not to require auth")
	}
	startIndex := firstEventIndex(gotEvents, ToolCallStartedEventType)
	decisionIndex := firstEventIndex(gotEvents, ToolPermissionDecidedEventType)
	resultIndex := firstEventIndex(gotEvents, ToolResultEventType)
	if startIndex < 0 || decisionIndex < 0 || resultIndex < 0 || !(startIndex < decisionIndex && decisionIndex < resultIndex) {
		t.Fatalf("expected ToolPermissionDecided between ToolCallStarted and ToolResult, got start=%d decision=%d result=%d events=%+v",
			startIndex, decisionIndex, resultIndex, eventTypes(gotEvents))
	}
}

func TestRunnerAnthropicApplyPatchAutoAllowFeedsBack(t *testing.T) {
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
		// Task 4: apply_patch within workspace is now auto-allowed.
		// The file must exist after the call.
		if _, err := os.Stat(target); err != nil {
			t.Fatalf("expected AUTO-allowed patch to create file, stat err=%v", err)
		}
		writeRunnerAnthropicTextStream(t, w, "anthropic patch applied")
	}))
	defer server.Close()

	cfg := testAgentConfigWithWorkspace(server.URL+"/anthropic", 3, root)
	var sawAuthRequired bool
	agent := NewAgent(cfg)
	answer, err := agent.RunStreamWithEvents(context.Background(), "create via anthropic apply_patch", EventSinkFunc(func(event AwakenEvent) {
		if event.Type == AuthRequiredEventType {
			sawAuthRequired = true
		}
	}))
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if answer != "anthropic patch applied" {
		t.Fatalf("expected final answer, got %q", answer)
	}
	if sawAuthRequired {
		t.Fatalf("expected AUTO allow path not to require auth")
	}
}

func TestRunnerMaxThoughts(t *testing.T) {
	toolName := "runner_max_tool"
	mustRegisterRunnerTool(t, runnerTestTool{name: toolName})

	server := newRunnerStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeRunnerStreamToolCall(t, w, "call_max", toolName, map[string]string{"value": "again"})
	})
	defer server.Close()

	agent := NewAgent(testAgentConfig(server.URL, 1))
	var gotEvents []AwakenEvent
	_, err := agent.RunStreamWithEvents(context.Background(), "loop", EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))
	if err == nil || !strings.Contains(err.Error(), "达到最大思考次数") {
		t.Fatalf("expected max thoughts error, got %v", err)
	}
	messages := agent.GetConversation().Context.GetMessages()
	if !conversationHasToolResult(messages, "call_max", "echo:again") {
		t.Fatalf("expected final tool result to be retained before max_turns, got %+v", messages)
	}
	turnFinishedIndex := firstEventIndex(gotEvents, TurnFinishedEventType)
	if turnFinishedIndex < 0 || gotEvents[turnFinishedIndex].TerminalReason != TerminalReasonMaxTurns {
		t.Fatalf("expected TurnFinished max_turns, got %+v", gotEvents)
	}
	runErrorIndex := firstEventIndex(gotEvents, RunErrorEventType)
	if runErrorIndex < 0 || gotEvents[runErrorIndex].TerminalReason != TerminalReasonMaxTurns {
		t.Fatalf("expected RunError max_turns, got %+v", gotEvents)
	}
}

func TestRunnerHundredToolLoopStopsAtMaxTurns(t *testing.T) {
	toolName := "runner_hundred_loop_tool"
	mustRegisterRunnerTool(t, runnerTestTool{name: toolName})

	var calls int
	server := newRunnerStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		writeRunnerStreamToolCall(t, w,
			fmt.Sprintf("call_loop_%03d", calls),
			toolName,
			map[string]string{"value": fmt.Sprintf("again-%03d", calls)},
		)
	})
	defer server.Close()

	agent := NewAgent(testAgentConfig(server.URL, 100))
	var gotEvents []AwakenEvent
	_, err := agent.RunStreamWithEvents(context.Background(), "loop", EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))
	if err == nil || !strings.Contains(err.Error(), "达到最大思考次数") {
		t.Fatalf("expected max thoughts error, got %v", err)
	}
	if calls != 100 {
		t.Fatalf("expected 100 LLM calls, got %d", calls)
	}

	messages := agent.GetConversation().Context.GetMessages()
	if err := convcontext.ValidateToolResultPairing(messages); err != nil {
		t.Fatalf("expected valid tool result pairing after 100-turn loop, got %v", err)
	}
	var toolResultCount int
	for _, msg := range messages {
		if msg.Role == convcontext.RoleTool {
			toolResultCount++
		}
	}
	if toolResultCount != 100 {
		t.Fatalf("expected 100 tool results, got %d", toolResultCount)
	}

	var approvalCount, decisionCount int
	for _, event := range gotEvents {
		switch event.Type {
		case AuthRequiredEventType:
			approvalCount++
		case ToolPermissionDecidedEventType:
			decisionCount++
			if event.ToolDecisionAction != string(ToolDecisionAllow) {
				t.Fatalf("expected allow decision in 100-turn loop, got %+v", event)
			}
		}
	}
	if approvalCount != 0 {
		t.Fatalf("expected no approval requests, got %d", approvalCount)
	}
	if decisionCount != 100 {
		t.Fatalf("expected 100 tool permission decisions, got %d", decisionCount)
	}
	if len(gotEvents) > 1400 {
		t.Fatalf("expected event count to remain bounded, got %d", len(gotEvents))
	}
	runErrorIndex := firstEventIndex(gotEvents, RunErrorEventType)
	if runErrorIndex < 0 || gotEvents[runErrorIndex].TerminalReason != TerminalReasonMaxTurns {
		t.Fatalf("expected RunError max_turns, got %+v", gotEvents)
	}
}

func TestRunnerZeroMaxThoughtsUsesDefaultNotUnlimited(t *testing.T) {
	if got := EffectiveMaxAgentTurns(0); got != DefaultMaxAgentTurns {
		t.Fatalf("MaxThoughts=0 must use product default %d, got %d", DefaultMaxAgentTurns, got)
	}
	toolName := "runner_default_limit_tool"
	mustRegisterRunnerTool(t, runnerTestTool{name: toolName})

	var calls int
	server := newRunnerStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls <= 3 {
			writeRunnerStreamToolCall(t, w,
				fmt.Sprintf("call_default_%03d", calls),
				toolName,
				map[string]string{"value": fmt.Sprintf("again-%03d", calls)},
			)
			return
		}
		writeRunnerStreamText(t, w, "default final")
	})
	defer server.Close()

	agent := NewAgent(testAgentConfig(server.URL, 0))
	answer, err := agent.RunStreamWithEvents(context.Background(), "loop with default budget", nil)
	if err != nil {
		t.Fatalf("RunStreamWithEvents returned error: %v", err)
	}
	if answer != "default final" {
		t.Fatalf("expected final answer, got %q", answer)
	}
	if calls != 4 {
		t.Fatalf("expected 4 LLM calls under default budget, got %d", calls)
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
		TurnStartedEventType,
		ContextEstimatedEventType,
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
		ToolPermissionDecidedEventType,
		ToolResultEventType,
		TurnFinishedEventType,
		TurnStartedEventType,
		ContextEstimatedEventType,
		TextMessageStartedEventType,
		TextDeltaEventType,
		TextMessageFinishedEventType,
		TurnFinishedEventType,
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
	if gotEvents[5].Delta != "{" || gotEvents[6].Delta != "\"value\":\"hello\"" || gotEvents[7].Delta != "}" {
		t.Fatalf("unexpected tool args deltas: %q %q %q", gotEvents[5].Delta, gotEvents[6].Delta, gotEvents[7].Delta)
	}
	if gotEvents[9].ToolName != toolName || gotEvents[10].ToolName != toolName || gotEvents[11].ToolName != toolName {
		t.Fatalf("expected tool identity on start/args/end events, got %q %q %q", gotEvents[9].ToolName, gotEvents[10].ToolName, gotEvents[11].ToolName)
	}
	if gotEvents[10].Arguments != `{"value":"hello"}` {
		t.Fatalf("expected complete tool args fallback, got %q", gotEvents[10].Arguments)
	}
	if gotEvents[12].ToolProgressStage != "started" || gotEvents[13].ToolProgressStage != "completed" {
		t.Fatalf("unexpected runtime tool progress events: %+v %+v", gotEvents[12], gotEvents[13])
	}
	if gotEvents[15].Content != "echo:hello" {
		t.Fatalf("expected tool result content echo:hello, got %q", gotEvents[15].Content)
	}
	if gotEvents[15].ToolStatus == "" {
		t.Fatalf("expected tool result to carry toolStatus, got %+v", gotEvents[15])
	}
	if gotEvents[20].Delta != "done" {
		t.Fatalf("expected final text delta done, got %q", gotEvents[20].Delta)
	}
}

func TestRunnerRunStreamWithEventsEmitsTurnState(t *testing.T) {
	toolName := "runner_turn_state_tool"
	mustRegisterRunnerTool(t, runnerTestTool{name: toolName})

	var calls int
	server := newRunnerStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			writeRunnerStreamToolCall(t, w, "call_turn_state", toolName, map[string]string{"value": "hello"})
			return
		}
		writeRunnerStreamText(t, w, "done")
	})
	defer server.Close()

	agent := NewAgent(testAgentConfig(server.URL, 3))
	var gotEvents []AwakenEvent
	answer, err := agent.RunStreamWithEvents(context.Background(), "turn state", EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))
	if err != nil {
		t.Fatalf("RunStreamWithEvents returned error: %v", err)
	}
	if answer != "done" {
		t.Fatalf("expected done, got %q", answer)
	}

	var starts, finishes []AwakenEvent
	for _, event := range gotEvents {
		switch event.Type {
		case TurnStartedEventType:
			starts = append(starts, event)
		case TurnFinishedEventType:
			finishes = append(finishes, event)
		}
	}
	if len(starts) != 2 || len(finishes) != 2 {
		t.Fatalf("expected two turn starts and finishes, got starts=%+v finishes=%+v", starts, finishes)
	}
	if starts[0].TurnCount != 1 || starts[1].TurnCount != 2 {
		t.Fatalf("expected turn count to grow 1,2, got %d,%d", starts[0].TurnCount, starts[1].TurnCount)
	}
	if starts[0].TurnID == "" || starts[1].TurnID == "" || starts[0].TurnID == starts[1].TurnID {
		t.Fatalf("expected stable unique turn ids, got %q and %q", starts[0].TurnID, starts[1].TurnID)
	}
	if starts[0].QueryChainID == "" || starts[0].QueryChainID != starts[1].QueryChainID {
		t.Fatalf("expected shared query chain id, got %q and %q", starts[0].QueryChainID, starts[1].QueryChainID)
	}
	if finishes[0].TransitionReason != TransitionToolResultContinuation {
		t.Fatalf("expected first turn to continue after tool result, got %q", finishes[0].TransitionReason)
	}
	if finishes[1].TerminalReason != TerminalReasonCompleted {
		t.Fatalf("expected final turn terminal reason completed, got %q", finishes[1].TerminalReason)
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
	if !toolContentMatches(messages[2].Content, "echo:hello") {
		t.Fatalf("expected tool result content echo:hello, got %q", messages[2].Content)
	}
	if messages[3].Role != convcontext.RoleAssistant || messages[3].Content != "done" {
		t.Fatalf("expected final assistant message, got %+v", messages[3])
	}
}

func TestRunnerReadOnlyToolOutsideWorkspaceFeedsResultBack(t *testing.T) {
	workspaceRoot := t.TempDir()
	outsideRoot := t.TempDir()
	outsideFile := filepath.Join(outsideRoot, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("outside content"), 0644); err != nil {
		t.Fatalf("failed to write outside file: %v", err)
	}

	var calls int
	server := newRunnerAnthropicStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			writeRunnerAnthropicStreamEvents(t, w, []string{
				`event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_outside","name":"read_file"}}`,
				`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{"}}`,
				fmt.Sprintf(`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":%s}}`, quoteJSONStringForStream(outsideFile)),
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

	var sawToolResult, sawAuthRequired bool
	var toolResultContent string
	for _, ev := range gotEvents {
		switch ev.Type {
		case ToolResultEventType:
			sawToolResult = true
			toolResultContent = ev.Content
		case AuthRequiredEventType:
			sawAuthRequired = true
		}
	}
	if !sawToolResult {
		t.Fatalf("expected ToolResult event, got events: %+v", gotEvents)
	}
	if sawAuthRequired {
		t.Fatalf("unexpected auth requirement for read-only tool")
	}
	if !strings.Contains(toolResultContent, "outside content") {
		t.Fatalf("expected tool result to contain outside file content, got %q", toolResultContent)
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
	if !strings.Contains(messages[2].Content, "outside content") {
		t.Fatalf("expected tool result content to contain outside file content, got %q", messages[2].Content)
	}
}

func quoteJSONStringForStream(value string) string {
	encodedValue, _ := json.Marshal(value)
	encodedPartial, _ := json.Marshal(`"path":` + string(encodedValue))
	return string(encodedPartial)
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
		TurnStartedEventType,
		ContextEstimatedEventType,
		TextMessageStartedEventType,
		TextDeltaEventType,
		TextMessageFinishedEventType,
		TurnFinishedEventType,
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
		case ToolCallStartedEventType, ToolCallArgsEventType, ToolCallFinishedEventType, ToolResultEventType, AuthRequiredEventType:
			t.Fatalf("unexpected event type in plain Q&A: %s", event.Type)
		}
	}
}

func TestRunnerReactiveCompactRetrySuccess(t *testing.T) {
	conv := convcontext.NewConversation(nil)
	runner := NewRunner(AgentConfig{
		MaxThoughts: 3,
		ModelConfig: ModelConfig{
			Model: "test-model",
		},
	}, conv, NewRequestBuilder(AgentConfig{
		SystemPrompt: "{{.RuntimeInfo}}",
		ModelConfig:  ModelConfig{Model: "test-model"},
	}))
	calls := 0
	runner.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("prompt too long")
		}
		return &llm.Response{Content: "done"}, nil
	}

	var gotEvents []AwakenEvent
	conv.UserInput("remember this before compact")
	answer, err := runner.RunStreamWithEvents(context.Background(), EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))
	if err != nil {
		t.Fatalf("RunStreamWithEvents returned error: %v", err)
	}
	if answer != "done" || calls != 2 {
		t.Fatalf("expected retry to succeed after 2 calls, answer=%q calls=%d", answer, calls)
	}
	if firstEventIndex(gotEvents, CompactStartedEventType) < 0 || firstEventIndex(gotEvents, CompactFinishedEventType) < 0 {
		t.Fatalf("expected compact lifecycle events, got %v", eventTypes(gotEvents))
	}
	retryTurnIndex := firstEventIndex(gotEvents, TurnFinishedEventType)
	if retryTurnIndex < 0 || gotEvents[retryTurnIndex].TransitionReason != TransitionReactiveCompactRetry {
		t.Fatalf("expected first turn to finish with reactive compact transition, got %+v", gotEvents)
	}
}

func TestRunnerReactiveCompactSecondPromptTooLongTerminates(t *testing.T) {
	conv := convcontext.NewConversation(nil)
	runner := NewRunner(AgentConfig{
		MaxThoughts: 3,
		ModelConfig: ModelConfig{
			Model: "test-model",
		},
	}, conv, NewRequestBuilder(AgentConfig{
		SystemPrompt: "{{.RuntimeInfo}}",
		ModelConfig:  ModelConfig{Model: "test-model"},
	}))
	calls := 0
	runner.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
		calls++
		return nil, errors.New("maximum context length exceeded")
	}

	var gotEvents []AwakenEvent
	conv.UserInput("remember this before compact")
	_, err := runner.RunStreamWithEvents(context.Background(), EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))
	if err == nil || !strings.Contains(err.Error(), "maximum context length") {
		t.Fatalf("expected prompt too long error, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected exactly one retry after reactive compact, got %d calls", calls)
	}
	runErrorIndex := firstEventIndex(gotEvents, RunErrorEventType)
	if runErrorIndex < 0 {
		t.Fatalf("expected RunError event, got %v", eventTypes(gotEvents))
	}
	if gotEvents[runErrorIndex].TerminalReason != TerminalReasonPromptTooLong {
		t.Fatalf("expected prompt_too_long terminal reason, got %+v", gotEvents[runErrorIndex])
	}
}

func TestRunnerThinkingOnlyResponseIsNotEmpty(t *testing.T) {
	conv := convcontext.NewConversation(nil)
	cfg := AgentConfig{
		MaxThoughts: 1,
		ModelConfig: ModelConfig{
			Model: "test-model",
		},
	}
	runner := NewRunner(cfg, conv, NewRequestBuilder(cfg))
	runner.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
		return &llm.Response{Thinking: "reasoning only", Usage: &llm.Usage{InputTokens: 7}}, nil
	}

	answer, err := runner.RunStreamWithEvents(context.Background(), EventSinkFunc(func(event AwakenEvent) {}))
	if err != nil {
		t.Fatalf("RunStreamWithEvents returned error: %v", err)
	}
	if answer != "" {
		t.Fatalf("expected empty final answer for thinking-only response, got %q", answer)
	}
	if runner.contextGovernor.lastAPIUsageTokens != 7 {
		t.Fatalf("expected runner to retain usage from thinking-only response, got %d", runner.contextGovernor.lastAPIUsageTokens)
	}
}

func TestRunEventEmitterCompactStructuredFields(t *testing.T) {
	var gotEvents []AwakenEvent
	emitter := newRunEventEmitter("thread-1", EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))
	state := ContextWindowState{
		EstimatedTokens:  120,
		CompactTarget:    36,
		WarningThreshold: 100,
	}
	result := CompactResult{
		Mode:                      CompactModeReactive,
		BoundaryID:                "compact_1",
		EstimatedTokens:           48,
		PostEstimateTokens:        48,
		ToolResultsPruned:         1,
		ToolResultsPrunedCount:    1,
		PrunedSafeGroupCount:      2,
		PrunedMessageCount:        5,
		SummaryMaxTokens:          8192,
		SummaryRetryCount:         1,
		SummaryRawChars:           321,
		SummaryParsedSections:     13,
		SummaryMeaningfulSections: 4,
		SummaryQualityIssue:       "semantic compact summary omitted compacted tool findings",
		SummarySourceHadUser:      true,
		SummaryLatestUserPinned:   true,
		Fallback: CompactFallback{
			Used:   true,
			Marker: "[COMPACT_FALLBACK:llm_summary_failed]",
			Reason: "provider_timeout",
		},
	}

	emitter.emitCompactStarted(state, CompactModeReactive)
	emitter.emitCompactFinished(state, result, "gpt-5-mini")
	emitter.emitCompactFailed(state, CompactModeReactive, "compact failed", "provider_timeout")

	if len(gotEvents) != 3 {
		t.Fatalf("expected 3 events, got %d", len(gotEvents))
	}
	started := gotEvents[0]
	if started.Type != CompactStartedEventType ||
		started.CompactMode != CompactModeReactive ||
		started.CompactPreEstimate == nil ||
		*started.CompactPreEstimate != 120 {
		t.Fatalf("unexpected compact started event: %+v", started)
	}

	finished := gotEvents[1]
	if finished.Type != CompactFinishedEventType ||
		finished.CompactMode != CompactModeReactive ||
		finished.CompactBoundaryID != "compact_1" ||
		finished.CompactPreEstimate == nil ||
		*finished.CompactPreEstimate != 120 ||
		finished.CompactPostEstimate == nil ||
		*finished.CompactPostEstimate != 48 ||
		finished.ContextEstimatedTokens != 48 ||
		finished.CompactSummaryModel != "gpt-5-mini" ||
		finished.CompactFallbackUsed == nil ||
		!*finished.CompactFallbackUsed ||
		finished.CompactToolResultsPruned == nil ||
		!*finished.CompactToolResultsPruned ||
		finished.CompactToolResultsPrunedCount != 1 ||
		finished.CompactPrunedSafeGroupCount != 2 ||
		finished.CompactPrunedMessageCount != 5 ||
		finished.CompactSummaryMaxTokens != 8192 ||
		finished.CompactSummaryRetryCount != 1 ||
		finished.CompactSummaryRawChars != 321 ||
		finished.CompactSummaryParsedSections != 13 ||
		finished.CompactSummaryMeaningfulSections != 4 ||
		finished.CompactSummaryQualityIssue != "semantic compact summary omitted compacted tool findings" ||
		finished.CompactSummarySourceHadUser == nil ||
		!*finished.CompactSummarySourceHadUser ||
		finished.CompactSummaryLatestUserPinned == nil ||
		!*finished.CompactSummaryLatestUserPinned ||
		finished.CompactFallbackMarker != "[COMPACT_FALLBACK:llm_summary_failed]" ||
		finished.CompactFailureReason != "provider_timeout" {
		t.Fatalf("unexpected compact finished event: %+v", finished)
	}

	fallbackOnly := CompactResult{
		Mode:               CompactModeReactive,
		BoundaryID:         "compact_2",
		EstimatedTokens:    64,
		PostEstimateTokens: 64,
		Fallback: CompactFallback{
			Used: true,
		},
	}
	emitter.emitCompactFinished(state, fallbackOnly, "gpt-5-mini")
	if len(gotEvents) != 4 {
		t.Fatalf("expected 4 events after fallback-only compact, got %d", len(gotEvents))
	}
	fallbackFinished := gotEvents[3]
	if fallbackFinished.Type != CompactFinishedEventType ||
		fallbackFinished.CompactToolResultsPruned == nil ||
		*fallbackFinished.CompactToolResultsPruned {
		t.Fatalf("expected fallback-only compact not to report tool results pruned: %+v", fallbackFinished)
	}

	failed := gotEvents[2]
	if failed.Type != CompactFailedEventType ||
		failed.CompactMode != CompactModeReactive ||
		failed.CompactFailureReason != "provider_timeout" {
		t.Fatalf("unexpected compact failed event: %+v", failed)
	}
}

func TestRunnerContinuesOnceAfterMaxOutput(t *testing.T) {
	conv := convcontext.NewConversation(nil)
	runner := NewRunner(AgentConfig{
		MaxThoughts: 3,
		ModelConfig: ModelConfig{
			Model: "test-model",
		},
	}, conv, NewRequestBuilder(AgentConfig{
		SystemPrompt: "{{.RuntimeInfo}}",
		ModelConfig:  ModelConfig{Model: "test-model"},
	}))
	calls := 0
	runner.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
		calls++
		if calls == 1 {
			return &llm.Response{
				Content:          "partial ",
				IsFinished:       true,
				StopReason:       "max_tokens",
				MaxOutputReached: true,
			}, nil
		}
		return &llm.Response{Content: "done", IsFinished: true}, nil
	}

	var gotEvents []AwakenEvent
	conv.UserInput("write a long answer")
	answer, err := runner.RunStreamWithEvents(context.Background(), EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))
	if err != nil {
		t.Fatalf("RunStreamWithEvents returned error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected one continuation request, got %d calls", calls)
	}
	if answer != "partial done" {
		t.Fatalf("expected combined answer, got %q", answer)
	}
	firstFinish := firstEventIndex(gotEvents, TurnFinishedEventType)
	if firstFinish < 0 || gotEvents[firstFinish].TransitionReason != TransitionMaxOutputContinuation {
		t.Fatalf("expected first turn to finish with max output continuation, got %+v", gotEvents)
	}
	messages := conv.Context.GetMessages()
	if len(messages) < 3 || !strings.Contains(messages[2].Content, "stop_reason=max_tokens") {
		t.Fatalf("expected continuation prompt to record stop reason, got %+v", messages)
	}
}

func TestRunnerStopsAfterMaxOutputContinuationLimit(t *testing.T) {
	conv := convcontext.NewConversation(nil)
	runner := NewRunner(AgentConfig{
		MaxThoughts: 3,
		ModelConfig: ModelConfig{
			Model: "test-model",
		},
	}, conv, NewRequestBuilder(AgentConfig{
		SystemPrompt: "{{.RuntimeInfo}}",
		ModelConfig:  ModelConfig{Model: "test-model"},
	}))
	calls := 0
	runner.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
		calls++
		return &llm.Response{
			Content:          fmt.Sprintf("partial-%d ", calls),
			IsFinished:       true,
			StopReason:       "max_tokens",
			MaxOutputReached: true,
		}, nil
	}

	var gotEvents []AwakenEvent
	conv.UserInput("write a long answer")
	answer, err := runner.RunStreamWithEvents(context.Background(), EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))
	if err == nil || !strings.Contains(err.Error(), "max output limit") {
		t.Fatalf("expected max output error, answer=%q err=%v", answer, err)
	}
	if calls != maxOutputContinuationAttempts+1 {
		t.Fatalf("expected exactly one continuation attempt, got %d calls", calls)
	}
	runErrorIndex := firstEventIndex(gotEvents, RunErrorEventType)
	if runErrorIndex < 0 {
		t.Fatalf("expected RunError event, got %v", eventTypes(gotEvents))
	}
	if gotEvents[runErrorIndex].TerminalReason != TerminalReasonMaxOutputReached {
		t.Fatalf("expected max_output_reached terminal reason, got %+v", gotEvents[runErrorIndex])
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

func TestRunnerStreamingCancelUsesAbortedStreamingTerminalReason(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conv := convcontext.NewConversation(nil)
	conv.UserInput("stream then cancel")
	runner := NewRunner(AgentConfig{
		MaxThoughts: 2,
		ModelConfig: ModelConfig{
			Model: "test-model",
		},
	}, conv, NewRequestBuilder(AgentConfig{
		SystemPrompt: "{{.RuntimeInfo}}",
		ModelConfig:  ModelConfig{Model: "test-model"},
	}))
	runner.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
		cancel()
		return nil, ctx.Err()
	}

	var gotEvents []AwakenEvent
	_, err := runner.RunStreamWithEvents(ctx, EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled error, got %v", err)
	}
	turnFinishedIndex := firstEventIndex(gotEvents, TurnFinishedEventType)
	if turnFinishedIndex < 0 || gotEvents[turnFinishedIndex].TerminalReason != TerminalReasonAbortedStreaming {
		t.Fatalf("expected TurnFinished aborted_streaming, got %+v", gotEvents)
	}
	runErrorIndex := firstEventIndex(gotEvents, RunErrorEventType)
	if runErrorIndex < 0 || gotEvents[runErrorIndex].TerminalReason != TerminalReasonAbortedStreaming {
		t.Fatalf("expected RunError aborted_streaming, got %+v", gotEvents)
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

func (t cancelOnExecuteTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name: t.name, DisplayName: t.name, Source: tool.SourceBuiltin,
		Description: "cancels the run context on execute",
		Annotations: tool.ToolAnnotations{ReadOnly: true},
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

	messages := conv.Context.GetMessages()
	if err := convcontext.ValidateToolResultPairing(messages); err != nil {
		t.Fatalf("expected valid tool result pairing after cancel, got %v", err)
	}
	turnFinishedIndex := firstEventIndex(gotEvents, TurnFinishedEventType)
	if turnFinishedIndex < 0 || gotEvents[turnFinishedIndex].TerminalReason != TerminalReasonAbortedTools {
		t.Fatalf("expected TurnFinished aborted_tools, got %+v", gotEvents)
	}
	runErrorIndex := firstEventIndex(gotEvents, RunErrorEventType)
	if runErrorIndex < 0 || gotEvents[runErrorIndex].TerminalReason != TerminalReasonAbortedTools {
		t.Fatalf("expected RunError aborted_tools, got %+v", gotEvents)
	}
}

func TestA30ToolCallJournalFailurePreventsExecutionAndPersistsPersistenceTerminal(t *testing.T) {
	toolName := "a30_tool_call_persistence_" + t.Name()
	var executions int
	mustRegisterRunnerTool(t, countingJournalTool{name: toolName, calls: &executions})

	cfg := AgentConfig{MaxThoughts: 2, ModelConfig: ModelConfig{Model: "test-model"}}
	conv := convcontext.NewConversation(nil)
	conv.UserInput("must persist before execution")
	recorder := &failingRecorder{failToolCall: true}
	runner := NewRunner(cfg, conv, NewRequestBuilder(cfg))
	runner.threadID = "thread_a30"
	runner.runID = "run_a30"
	runner.sessionID = "session_a30"
	runner.recorder = recorder
	runner.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
		return &llm.Response{ToolCalls: []llm.ToolCall{{
			ID: "call_a30", Name: toolName, Input: `{"value":"side effect"}`,
		}}}, nil
	}

	var live []AwakenEvent
	_, err := runner.RunStreamWithEvents(context.Background(), EventSinkFunc(func(event AwakenEvent) {
		live = append(live, event)
	}))
	if !errors.Is(err, ErrPersistenceFailed) {
		t.Fatalf("A30 expected ErrPersistenceFailed, got %v", err)
	}
	if executions != 0 {
		t.Fatalf("A30 tool executed before tool_call became durable: %d", executions)
	}
	assertOnlyPersistenceTerminalAfterFailure(t, recorder.records, "run_a30")
	for _, record := range recorder.records {
		if record.Kind == SemanticKindToolCall || record.Kind == SemanticKindToolResult {
			t.Fatalf("A30 failed tool_call append leaked tool facts: %+v", recorder.records)
		}
	}
	assertPersistenceErrorVisibleWithoutSuccess(t, live)
}

func TestA31ToolResultBatchFailureStopsLaterSideEffectsAndPersistsPersistenceTerminal(t *testing.T) {
	toolName := "a31_tool_result_persistence_" + t.Name()
	var executions int
	mustRegisterRunnerTool(t, countingJournalTool{name: toolName, calls: &executions})

	cfg := AgentConfig{MaxThoughts: 2, ModelConfig: ModelConfig{Model: "test-model"}}
	conv := convcontext.NewConversation(nil)
	conv.UserInput("two side effects")
	recorder := &failingRecorder{failToolResult: true}
	runner := NewRunner(cfg, conv, NewRequestBuilder(cfg))
	runner.threadID = "thread_a31"
	runner.runID = "run_a31"
	runner.sessionID = "session_a31"
	runner.recorder = recorder
	runner.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
		return &llm.Response{ToolCalls: []llm.ToolCall{
			{ID: "call_a31_1", Name: toolName, Input: `{"value":"one"}`},
			{ID: "call_a31_2", Name: toolName, Input: `{"value":"two"}`},
		}}, nil
	}

	var live []AwakenEvent
	_, err := runner.RunStreamWithEvents(context.Background(), EventSinkFunc(func(event AwakenEvent) {
		live = append(live, event)
	}))
	if !errors.Is(err, ErrPersistenceFailed) {
		t.Fatalf("A31 expected ErrPersistenceFailed, got %v", err)
	}
	if executions != 1 {
		t.Fatalf("A31 later side effect executed after tool_result persistence failure: %d", executions)
	}
	var calls, results int
	for _, record := range recorder.records {
		switch record.Kind {
		case SemanticKindToolCall:
			calls++
			if record.Payload["tool_call_id"] != "call_a31_1" {
				t.Fatalf("A31 second call became durable/executable: %+v", record)
			}
		case SemanticKindToolResult:
			results++
		}
	}
	if calls != 1 || results != 0 {
		t.Fatalf("A31 expected one durable call and zero uncommitted results, calls=%d results=%d records=%+v", calls, results, recorder.records)
	}
	assertOnlyPersistenceTerminalAfterFailure(t, recorder.records, "run_a31")
	assertPersistenceErrorVisibleWithoutSuccess(t, live)
}

func TestA36CancelAfterDurableToolCallKeepsPairingAndCancelledTerminal(t *testing.T) {
	toolName := "a36_cancel_after_call_" + t.Name()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mustRegisterRunnerTool(t, cancelOnExecuteTool{name: toolName, cancel: cancel})

	cfg := AgentConfig{MaxThoughts: 2, ModelConfig: ModelConfig{Model: "test-model"}}
	conv := convcontext.NewConversation(nil)
	conv.UserInput("cancel after tool call")
	recorder := &failingRecorder{}
	runner := NewRunner(cfg, conv, NewRequestBuilder(cfg))
	runner.threadID = "thread_a36"
	runner.runID = "run_a36"
	runner.sessionID = "session_a36"
	runner.recorder = recorder
	runner.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
		return &llm.Response{ToolCalls: []llm.ToolCall{{
			ID: "call_a36", Name: toolName, Input: `{"value":"cancel"}`,
		}}}, nil
	}

	_, err := runner.RunStreamWithEvents(ctx, EventSinkFunc(func(AwakenEvent) {}))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("A36 expected context cancellation, got %v", err)
	}
	if err := convcontext.ValidateToolResultPairing(conv.Context.GetMessages()); err != nil {
		t.Fatalf("A36 live model history is unpaired: %v", err)
	}

	var callRecord, resultRecord, terminalRecord *SemanticRecord
	for i := range recorder.records {
		record := &recorder.records[i]
		switch record.Kind {
		case SemanticKindToolCall:
			callRecord = record
		case SemanticKindToolResult:
			resultRecord = record
		case SemanticKindRunTerminal:
			terminalRecord = record
		}
	}
	if callRecord == nil || resultRecord == nil || terminalRecord == nil {
		t.Fatalf("A36 durable pairing/terminal incomplete: %+v", recorder.records)
	}
	if callRecord.Payload["tool_call_id"] != resultRecord.Payload["tool_call_id"] ||
		resultRecord.ParentID != "call_a36" {
		t.Fatalf("A36 durable tool call/result IDs do not pair: call=%+v result=%+v", callRecord, resultRecord)
	}
	if terminalRecord.Payload["status"] != "cancelled" ||
		terminalRecord.Payload["code"] != "RUN_CANCELLED" ||
		terminalRecord.Payload["terminal_reason"] != string(TerminalReasonAbortedTools) {
		t.Fatalf("A36 terminal is not a typed cancelled outcome: %+v", terminalRecord.Payload)
	}
	if err := convcontext.ValidateToolResultPairing(toolRecordsToMessagesForTest(recorder.records)); err != nil {
		t.Fatalf("A36 restored durable history is unpaired: %v", err)
	}
}

func assertOnlyPersistenceTerminalAfterFailure(t *testing.T, records []SemanticRecord, runID string) {
	t.Helper()
	var terminals []SemanticRecord
	for _, record := range records {
		if record.Kind == SemanticKindRunTerminal && record.RunID == runID {
			terminals = append(terminals, record)
		}
	}
	if len(terminals) != 1 {
		t.Fatalf("expected exactly one persistence terminal for %s, got %+v", runID, terminals)
	}
	payload := terminals[0].Payload
	if payload["status"] != "error" || payload["code"] != string(TerminalReasonPersistenceError) ||
		payload["terminal_reason"] != string(TerminalReasonPersistenceError) {
		t.Fatalf("unexpected persistence terminal payload: %+v", payload)
	}
}

func assertPersistenceErrorVisibleWithoutSuccess(t *testing.T, events []AwakenEvent) {
	t.Helper()
	var persistenceErrors int
	for _, event := range events {
		if event.Type == RunFinishedEventType {
			t.Fatalf("persistence failure exposed false success: %+v", events)
		}
		if event.Type == RunErrorEventType && event.Code == "PERSISTENCE_ERROR" &&
			event.TerminalReason == TerminalReasonPersistenceError {
			persistenceErrors++
		}
	}
	if persistenceErrors != 1 {
		t.Fatalf("expected one visible persistence_error, got %d: %+v", persistenceErrors, events)
	}
}

func toolRecordsToMessagesForTest(records []SemanticRecord) []convcontext.Message {
	messages := make([]convcontext.Message, 0, len(records))
	for _, record := range records {
		switch record.Kind {
		case SemanticKindToolCall:
			messages = append(messages, convcontext.Message{
				Role: convcontext.RoleAssistant,
				ToolCalls: []convcontext.ToolCall{{
					ID: fmt.Sprint(record.Payload["tool_call_id"]),
					Function: convcontext.FunctionCall{
						Name:      fmt.Sprint(record.Payload["tool_name"]),
						Arguments: fmt.Sprint(record.Payload["arguments"]),
					},
				}},
			})
		case SemanticKindToolResult:
			messages = append(messages, convcontext.Message{
				Role:       convcontext.RoleTool,
				ToolCallID: fmt.Sprint(record.Payload["tool_call_id"]),
				Name:       fmt.Sprint(record.Payload["tool_name"]),
				Content:    fmt.Sprint(record.Payload["content"]),
			})
		}
	}
	return messages
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

func (t failIfExecutedTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name: t.name, DisplayName: t.name, Source: tool.SourceBuiltin,
		Description: "fails the test if executed",
		Annotations: tool.ToolAnnotations{ReadOnly: true},
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
	turnFinishedIndex := firstEventIndex(gotEvents, TurnFinishedEventType)
	if turnFinishedIndex < 0 || gotEvents[turnFinishedIndex].TerminalReason != TerminalReasonAbortedTools {
		t.Fatalf("expected TurnFinished aborted_tools, got %+v", gotEvents)
	}
	runErrorIndex := firstEventIndex(gotEvents, RunErrorEventType)
	if runErrorIndex < 0 || gotEvents[runErrorIndex].TerminalReason != TerminalReasonAbortedTools {
		t.Fatalf("expected RunError aborted_tools, got %+v", gotEvents)
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
	emitter.runErrored(context.Background(), wrapped)

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
	emitter.runErrored(context.Background(), wrapped)

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

func TestRunEventEmitterRunErroredSetsRunCancelledCode(t *testing.T) {
	var gotEvents []AwakenEvent
	emitter := newRunEventEmitter("thread-test", EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))

	emitter.runErrored(context.Background(), context.Canceled, TerminalReasonCancelled)

	if len(gotEvents) != 1 {
		t.Fatalf("expected 1 event, got %d: %+v", len(gotEvents), gotEvents)
	}
	if gotEvents[0].Type != RunErrorEventType {
		t.Fatalf("expected RunError event, got %s", gotEvents[0].Type)
	}
	if gotEvents[0].TerminalReason != TerminalReasonCancelled {
		t.Fatalf("expected TerminalReasonCancelled, got %q", gotEvents[0].TerminalReason)
	}
	if gotEvents[0].Code != "RUN_CANCELLED" {
		t.Fatalf("expected Code RUN_CANCELLED, got %q", gotEvents[0].Code)
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
		if msg.Role == "tool" && msg.ToolCallID == id && toolContentMatches(msg.Content, content) {
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

func conversationHasToolResult(messages []convcontext.Message, id, content string) bool {
	for _, msg := range messages {
		if msg.Role == convcontext.RoleTool && msg.ToolCallID == id && toolContentMatches(msg.Content, content) {
			return true
		}
	}
	return false
}

// toolContentMatches accepts either exact content or ModelPayload JSON that
// embeds the expected content (Phase 1 ToolOutcome wire-through).
func toolContentMatches(actual, expected string) bool {
	if actual == expected {
		return true
	}
	return strings.Contains(actual, expected)
}

func firstEventIndex(events []AwakenEvent, eventType AwakenEventType) int {
	for i, event := range events {
		if event.Type == eventType {
			return i
		}
	}
	return -1
}

func eventTypes(events []AwakenEvent) []AwakenEventType {
	out := make([]AwakenEventType, 0, len(events))
	for _, event := range events {
		out = append(out, event.Type)
	}
	return out
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
