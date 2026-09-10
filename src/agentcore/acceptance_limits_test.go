package agentcore

import (
	"context"
	"fmt"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/tool"
	"awaken/src/tool/profile"
)

func acceptanceBudgetRunner(cfg AgentConfig) *Runner {
	conversation := convcontext.NewConversation(nil)
	conversation.UserInput("Execute the isolated budget fixture.")
	return NewRunner(cfg, conversation, NewRequestBuilder(cfg))
}

func TestAcceptanceChildDefault256TurnsAndFollowupBudget(t *testing.T) {
	name := "acceptance_turn_limit_read"
	mustRegisterRunnerTool(t, runnerTestTool{name: name})
	t.Cleanup(func() { tool.UnregisterTool(name) })
	cfg := testAgentConfig("http://unused.invalid", 0)
	cfg.SubagentOrigin = &profile.SubagentOrigin{AgentID: "budget-child", ParentAgentID: "root", Depth: 1}
	runner := acceptanceBudgetRunner(cfg)
	calls := 0
	runner.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
		calls++
		return &llm.Response{ToolCalls: []llm.ToolCall{{ID: fmt.Sprintf("turn-%03d", calls), Name: name, Input: fmt.Sprintf(`{"value":"read-%d"}`, calls)}}}, nil
	}
	_, err := runner.RunStreamWithEvents(context.Background(), EventSinkFunc(func(AwakenEvent) {}))
	if err == nil || !strings.Contains(err.Error(), "达到最大思考次数") || calls != 256 {
		t.Fatalf("default turn boundary: calls=%d err=%v", calls, err)
	}
	if runner.config.AgentTurnsUsed != 256 || runner.config.AgentToolsUsed != 256 {
		t.Fatalf("budget receipt: turns=%d tools=%d", runner.config.AgentTurnsUsed, runner.config.AgentToolsUsed)
	}
	if err := convcontext.ValidateToolResultPairing(runner.conversation.Context.GetMessages()); err != nil {
		t.Fatal(err)
	}
	continued := acceptanceBudgetRunner(runner.config)
	continued.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
		t.Error("exhausted Agent called the model again")
		return &llm.Response{Content: "unexpected"}, nil
	}
	if _, err := continued.RunStreamWithEvents(context.Background(), EventSinkFunc(func(AwakenEvent) {})); err == nil {
		t.Fatal("followup reset the model budget")
	}
}

func TestAcceptanceChildDefault1024ToolsAndFollowupBudget(t *testing.T) {
	name := "acceptance_tool_limit_read"
	mustRegisterRunnerTool(t, runnerTestTool{name: name})
	t.Cleanup(func() { tool.UnregisterTool(name) })
	cfg := testAgentConfig("http://unused.invalid", 0)
	cfg.SubagentOrigin = &profile.SubagentOrigin{AgentID: "tool-budget-child", ParentAgentID: "root", Depth: 1}
	runner := acceptanceBudgetRunner(cfg)
	calls := 0
	runner.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
		calls++
		batch := make([]llm.ToolCall, 129)
		for i := range batch {
			n := (calls-1)*129 + i
			batch[i] = llm.ToolCall{ID: fmt.Sprintf("tool-%04d", n), Name: name, Input: fmt.Sprintf(`{"value":"item-%d"}`, n)}
		}
		return &llm.Response{ToolCalls: batch}, nil
	}
	_, err := runner.RunStreamWithEvents(context.Background(), EventSinkFunc(func(AwakenEvent) {}))
	if err == nil || calls != 8 || runner.config.AgentToolsUsed != 1024 {
		t.Fatalf("tool boundary: models=%d real_tools=%d err=%v", calls, runner.config.AgentToolsUsed, err)
	}
	if err := convcontext.ValidateToolResultPairing(runner.conversation.Context.GetMessages()); err != nil {
		t.Fatal(err)
	}
	continued := acceptanceBudgetRunner(runner.config)
	continued.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
		return &llm.Response{ToolCalls: []llm.ToolCall{{ID: "tool-after-followup", Name: name, Input: `{"value":"must-not-execute"}`}}}, nil
	}
	_, err = continued.RunStreamWithEvents(context.Background(), EventSinkFunc(func(AwakenEvent) {}))
	if err == nil || continued.config.AgentToolsUsed != 1024 {
		t.Fatalf("followup reset tool budget: tools=%d err=%v", continued.config.AgentToolsUsed, err)
	}
}
