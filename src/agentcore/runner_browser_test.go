package agentcore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"awaken/src/browserhost"
	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/tool/profile"
)

func TestRunnerBrowserOwnerCleanupOnSuccessAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name, agent, task string
		cancelled         bool
	}{
		{"root_success", "root", "", false},
		{"child_success", "child-A", "assignment-A", false},
		{"child_cancelled", "child-B", "assignment-B", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() { _ = browserhost.Set(nil) })
			received := make(chan browserhost.ActionRequest, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request browserhost.ActionRequest
				_ = json.NewDecoder(r.Body).Decode(&request)
				received <- request
				_ = json.NewEncoder(w).Encode(browserhost.Result{OK: true})
			}))
			defer server.Close()
			if err := browserhost.Set(&browserhost.Registration{BaseURL: server.URL, Token: "test"}); err != nil {
				t.Fatal(err)
			}
			cfg := AgentConfig{MaxThoughts: 2, ModelConfig: ModelConfig{Model: "test-model"}, SubagentTaskID: tc.task}
			if tc.agent != "root" {
				cfg.SubagentOrigin = &profile.SubagentOrigin{AgentID: tc.agent, ParentAgentID: "root"}
			}
			conv := convcontext.NewConversation(nil)
			conv.UserInput("finish")
			runner := NewRunner(cfg, conv, NewRequestBuilder(cfg))
			runner.threadID, runner.runID = "thread-owned", "run-owned"
			runner.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
				return &llm.Response{Content: "done"}, nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			_, _ = runner.RunStreamWithEvents(ctx, EventSinkFunc(func(AwakenEvent) {}))
			select {
			case request := <-received:
				if request.Action != "end_owner" || request.ThreadID != "thread-owned" || request.RunID != "run-owned" || request.AgentID != tc.agent || request.TaskID != tc.task {
					t.Fatalf("wrong cleanup: %+v", request)
				}
			default:
				t.Fatal("runner did not release browser ownership")
			}
		})
	}
}
