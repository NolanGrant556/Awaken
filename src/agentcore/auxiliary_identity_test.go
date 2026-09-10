package agentcore

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"awaken/src/llm"
	"awaken/src/subagentruntime"
	"awaken/src/tool/profile"
)

func TestCompactSummaryReportsEmptyFinalShapeWithoutReasoningText(t *testing.T) {
	_, err := finalCompactSummaryText(&llm.Response{Thinking: "private reasoning must not enter diagnostics", StopReason: "tool_use", ToolCalls: []llm.ToolCall{{Name: "Read"}}})
	if err == nil || !strings.Contains(err.Error(), "tool_calls=1") || !strings.Contains(err.Error(), "stop_reason=tool_use") || strings.Contains(err.Error(), "private reasoning") {
		t.Fatalf("incorrect diagnostic: %v", err)
	}
}

func TestChildCompactUsageKeepsIdentityAndLogicalRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "invalid temperature: only 0.6 is allowed for this model", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"saved facts\"}}],\"usage\":{\"prompt_tokens\":50,\"completion_tokens\":3,\"total_tokens\":53}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	recorder := &failingRecorder{}
	e := newRunEventEmitter("thread", nil)
	e.runID, e.recorder = "run", recorder
	e.agentID, e.parentAgentID, e.subagentTaskID, e.attemptID = "child", "parent", "assignment", "attempt"
	var progress []subagentruntime.ProgressUpdate
	e.subagentProgress = func(_ context.Context, update subagentruntime.ProgressUpdate) error {
		progress = append(progress, update)
		return nil
	}
	cfg := llm.Config{APIKey: "fixture", BaseURL: server.URL, Model: "fixture-model", Protocol: llm.ProtocolOpenAIChat}
	for i := 0; i < 2; i++ {
		if _, err := e.invokeCompactSummary(context.Background(), cfg, llm.Request{UserPrompt: "summarize"}); err != nil {
			t.Fatal(err)
		}
	}
	var requests, usages []SemanticRecord
	for _, record := range recorder.records {
		for key, want := range map[string]string{"agent_id": "child", "parent_agent_id": "parent", "subagent_task_id": "assignment", "attempt_id": "attempt"} {
			if record.Payload[key] != want {
				t.Fatalf("%s lost %s: %+v", record.Kind, key, record.Payload)
			}
		}
		switch record.Kind {
		case SemanticKindModelRequest:
			requests = append(requests, record)
		case SemanticKindModelUsage:
			usages = append(usages, record)
		}
	}
	if len(requests) != 3 || len(usages) != 3 || len(progress) != 3 {
		t.Fatalf("requests=%d usages=%d progress=%d", len(requests), len(usages), len(progress))
	}
	first := requests[0].Payload["logical_call_id"]
	if first == "" || first != requests[1].Payload["logical_call_id"] || first == requests[2].Payload["logical_call_id"] {
		t.Fatal("logical calls did not distinguish a retry from a new summary")
	}
	for i, want := range []int{0, 1, 0} {
		if requests[i].Payload["retry_index"] != want || requests[i].Payload["logical_call_id"] != usages[i].Payload["logical_call_id"] || requests[i].Payload["request_id"] != usages[i].Payload["request_id"] {
			t.Fatalf("request/usage mismatch at attempt %d", i)
		}
		if usages[i].Payload["input_estimator_id"] != contextEstimatorID {
			t.Fatal("auxiliary estimate version was not recorded")
		}
	}
}

func TestPromptHookAuthorityHasOwnIdentityUnderChild(t *testing.T) {
	r := NewRunner(AgentConfig{}, nil, nil)
	r.config.SubagentOrigin = &profile.SubagentOrigin{AgentID: "child", ParentAgentID: "root"}
	authority := r.hookModelAuthority("invocation")
	if authority.AgentID != "hook_invocation" || authority.ParentAgentID != "child" || authority.SubagentTaskID != "hook_task_invocation" || authority.AttemptID != "hook_attempt_invocation" || authority.HookInvocationID != "invocation" {
		t.Fatalf("wrong helper authority: %+v", authority)
	}
}
