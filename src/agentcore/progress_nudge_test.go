package agentcore

import (
	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/tool"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestProgressNudgeNaturalBoundaryAndCooldown(t *testing.T) {
	now := time.Unix(100, 0)
	p := newProgressNudge(AgentConfig{ProgressNudgePolicy: &ProgressNudgePolicy{Enabled: true, Now: func() time.Time { return now }}})
	calls := func(offset int) []ToolExecutionResult {
		var out []ToolExecutionResult
		for i := 0; i < 8; i++ {
			out = append(out, ToolExecutionResult{ToolCallID: fmt.Sprint(offset + i), Name: "read_file", Outcome: tool.ExecutionOutcome{TransportStatus: tool.TransportCompleted}})
		}
		return out
	}
	p.observe(calls(0))
	p.observe(calls(0))
	p.prepare(TransitionToolResultContinuation)
	if p.notice != "" || p.calls != 8 {
		t.Fatal("threshold or duplicate count wrong")
	}
	now = now.Add(time.Minute)
	p.prepare(TransitionReason("waiting"))
	if p.notice != "" {
		t.Fatal("non tool continuation nudged")
	}
	p.prepare(TransitionToolResultContinuation)
	if p.notice == "" {
		t.Fatal("missing nudge")
	}
	req := LLMRequest{Messages: []convcontext.Message{{Role: convcontext.RoleTool, Content: "original result"}}}
	p.project(&req)
	p.project(&req)
	if strings.Count(req.Messages[0].ProviderProjectionText, progressNudgeText) != 1 {
		t.Fatal("retry duplicated notice")
	}
	encoded, _ := json.Marshal(req.Messages)
	if strings.Contains(string(encoded), "awaken_runtime_notice") {
		t.Fatal("notice persisted")
	}
	if req.Messages[0].Content != "original result" {
		t.Fatal("rewrote tool content")
	}
	p.observe(calls(8))
	p.prepare(TransitionToolResultContinuation)
	if p.notice != "" {
		t.Fatal("cooldown ignored")
	}
	now = now.Add(time.Minute)
	p.visible("confirmed progress")
	p.prepare(TransitionToolResultContinuation)
	if p.notice != "" {
		t.Fatal("visible message did not reset")
	}
	p.observe(calls(16))
	now = now.Add(time.Minute)
	p.prepare(TransitionToolResultContinuation)
	if p.notice == "" {
		t.Fatal("subsequent nudge missing")
	}
}

func TestProgressNudgeExcludesUnexecutedPollingAndChild(t *testing.T) {
	p := newProgressNudge(AgentConfig{})
	p.observe([]ToolExecutionResult{{ToolCallID: "validation", Name: "read_file"}, {ToolCallID: "wait", Name: "subagent_wait", Outcome: tool.ExecutionOutcome{TransportStatus: tool.TransportCompleted}}, {ToolCallID: "poll", Name: "task_status", Outcome: tool.ExecutionOutcome{TransportStatus: tool.TransportCompleted}}})
	if p.calls != 0 {
		t.Fatal("counted non-work")
	}
	if newProgressNudge(AgentConfig{SubagentTaskID: "child"}).policy.Enabled {
		t.Fatal("child enabled")
	}
}

func TestProgressNudgeRunnerUsesExistingContinuation(t *testing.T) {
	name := "progress_nudge_read"
	mustRegisterRunnerTool(t, runnerTestTool{name: name})
	t.Cleanup(func() { tool.UnregisterTool(name) })
	now := time.Unix(500, 0)
	cfg := testAgentConfig("http://unused.invalid", 0)
	cfg.ProgressNudgePolicy = &ProgressNudgePolicy{Enabled: true, QuietWindow: time.Minute, ToolThreshold: 8, Now: func() time.Time { return now }}
	r := acceptanceBudgetRunner(cfg)
	calls := 0
	r.streamLLMFunc = func(_ context.Context, _ *runEventEmitter) (*llm.Response, error) {
		calls++
		req, err := r.buildLLMRequest()
		if err != nil {
			t.Fatal(err)
		}
		projected := false
		for _, msg := range req.Messages {
			projected = projected || strings.Contains(msg.ProviderProjectionText, progressNudgeText)
		}
		if projected != (calls == 9) {
			t.Fatalf("request %d projected=%v", calls, projected)
		}
		if calls == 9 {
			return &llm.Response{Content: "已核查八项，任务完成。"}, nil
		}
		now = now.Add(8 * time.Second)
		return &llm.Response{ToolCalls: []llm.ToolCall{{ID: fmt.Sprint(calls), Name: name, Input: `{"value":"fixture"}`}}}, nil
	}
	if _, err := r.RunStreamWithEvents(context.Background(), EventSinkFunc(func(AwakenEvent) {})); err != nil {
		t.Fatal(err)
	}
	if calls != 9 {
		t.Fatalf("added model requests: %d", calls)
	}
	for _, msg := range r.conversation.Context.GetMessages() {
		if msg.ProviderProjectionText != "" || strings.Contains(msg.Content, "awaken_runtime_notice") {
			t.Fatal("persisted host reminder")
		}
	}
}

func TestProgressNudgeExcludesMixedSubagentPollingActions(t *testing.T) {
	p := newProgressNudge(AgentConfig{})
	var results []ToolExecutionResult
	var calls []convcontext.ToolCall
	for _, action := range []string{"spawn", "status", "wait", "models"} {
		results = append(results, ToolExecutionResult{ToolCallID: action, Name: "subagent", Outcome: tool.ExecutionOutcome{TransportStatus: tool.TransportCompleted}})
		calls = append(calls, convcontext.ToolCall{ID: action, Function: convcontext.FunctionCall{Name: "subagent", Arguments: `{"action":"` + action + `"}`}})
	}
	p.observe(results, calls...)
	if p.calls != 1 {
		t.Fatalf("counted subagent polling: %d", p.calls)
	}
}
