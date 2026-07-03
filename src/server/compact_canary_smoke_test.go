package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"awaken/src/agentcore"
	"awaken/src/llm"
	"awaken/src/protocol/agui"
	awruntime "awaken/src/runtime"
)

// TestCompactCanaryFactsSurviveThreadRestoreSmoke exercises the real provider
// path with a deliberately small context budget. The first run should compact
// away the long prompt while preserving the canary token in the compact summary.
// The second run reuses the same thread_id from a fresh server instance to
// verify transcript restoration and canary recall after compaction.
func TestCompactCanaryFactsSurviveThreadRestoreSmoke(t *testing.T) {
	workspaceRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(workspaceRoot); err != nil {
		t.Fatalf("failed to chdir to project root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	cfg := llm.LoadConfig()
	if cfg.APIKey == "" {
		t.Skip("LLM_API_KEY not set")
	}
	if cfg.Model == "" {
		t.Skip("LLM_MODEL_ID not set")
	}

	tempWorkspace := t.TempDir()
	canary := "canary-orchid-47"
	firstInput := fmt.Sprintf(
		"Remember the canary token %s. %sReply with ack.",
		canary,
		strings.Repeat("padding ", 2200),
	)

	newRunService := func() *awruntime.RunService {
		return awruntime.NewRunService(
			func(workspace awruntime.WorkspaceConfig, threadID string) *agentcore.Agent {
				agentCfg := agentcore.AgentConfig{
					MaxThoughts: 4,
					EntryPoint:  "awaken-core-smoke",
					ModelConfig: agentcore.ModelConfig{
						APIKey:                cfg.APIKey,
						BaseURL:               cfg.BaseURL,
						Model:                 cfg.Model,
						Temperature:           cfg.Temperature,
						MaxTokens:             cfg.MaxTokens,
						AuthScheme:            cfg.AuthScheme,
						ContextWindow:         40000,
						AutoCompactTokenLimit: 2000,
					},
					Workspace: agentcore.WorkspaceConfig{
						Root:          workspace.WorkspaceRoot,
						WritableRoots: workspace.WritableRoots,
					},
				}
				return agentcore.NewAgentWithThreadID(agentCfg, threadID)
			},
			func(agent *agentcore.Agent) awruntime.Executor {
				return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
					return agent.RunStreamWithEvents(ctx, input, sink)
				})
			},
		)
	}

	postRun := func(baseURL, input, threadID, sessionID, requestID string) awruntime.Run {
		t.Helper()
		body := fmt.Sprintf(
			`{"input":%q,"thread_id":%q,"workspace":{"workspace_root":%q,"writable_roots":[%q]},"client":{"session_id":%q,"request_id":%q,"entrypoint":"smoke"}}`,
			input,
			threadID,
			tempWorkspace,
			tempWorkspace,
			sessionID,
			requestID,
		)

		resp, err := http.Post(baseURL+"/v1/runs", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("POST /v1/runs failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d", resp.StatusCode)
		}

		var run awruntime.Run
		if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
			t.Fatalf("failed to decode create response: %v", err)
		}
		if run.RunID == "" || run.ThreadID == "" {
			t.Fatalf("expected run and thread ids, got %+v", run)
		}
		return run
	}

	collectRunEvents := func(baseURL, runID string) []agui.Event {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/runs/"+runID+"/events", nil)
		if err != nil {
			t.Fatalf("failed to create events request: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET events failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 from events, got %d", resp.StatusCode)
		}
		events, err := collectSSEEvents(resp.Body)
		if err != nil {
			t.Fatalf("failed to collect events: %v", err)
		}
		return events
	}

	assertCompactAndCanary := func(events []agui.Event, label string) {
		t.Helper()
		sawCompactStarted := false
		sawCompactFinished := false
		var assistantText strings.Builder
		for _, ev := range events {
			switch ev.Type {
			case agui.EventAwakenCompactStarted:
				sawCompactStarted = true
			case agui.EventAwakenCompactFinished:
				sawCompactFinished = true
			case agui.EventTextMessageContent:
				assistantText.WriteString(ev.Delta)
			case agui.EventRunError:
				t.Fatalf("%s run failed: %s (code=%s)", label, ev.Message, ev.Code)
			}
		}
		if !sawCompactStarted || !sawCompactFinished {
			t.Fatalf("%s expected compact lifecycle events, got %+v", label, events)
		}
		if strings.TrimSpace(assistantText.String()) == "" {
			t.Fatalf("%s expected assistant text, got empty output", label)
		}
	}

	firstServer := httptest.NewServer(NewRouter(newRunService(), nil))
	firstRun := postRun(firstServer.URL, firstInput, "", "smoke-canary-1", "smoke-req-canary-1")
	firstEvents := collectRunEvents(firstServer.URL, firstRun.RunID)
	assertCompactAndCanary(firstEvents, "first")

	transcriptURL := firstServer.URL + "/v1/threads/" + firstRun.ThreadID + "/transcript"
	deadline := time.Now().Add(10 * time.Second)
	for {
		transcriptResp, err := http.Get(transcriptURL)
		if err != nil {
			t.Fatalf("GET transcript failed: %v", err)
		}
		if transcriptResp.StatusCode == http.StatusOK {
			transcriptResp.Body.Close()
			break
		}
		transcriptResp.Body.Close()
		if time.Now().After(deadline) {
			t.Fatalf("expected 200 from transcript, got %d", transcriptResp.StatusCode)
		}
		time.Sleep(50 * time.Millisecond)
	}

	firstServer.Close()

	secondServer := httptest.NewServer(NewRouter(newRunService(), nil))
	defer secondServer.Close()

	secondInput := "What was the canary token? Reply with the token only."
	secondRun := postRun(secondServer.URL, secondInput, firstRun.ThreadID, "smoke-canary-2", "smoke-req-canary-2")
	if secondRun.ThreadID != firstRun.ThreadID {
		t.Fatalf("expected second run to reuse thread id %q, got %q", firstRun.ThreadID, secondRun.ThreadID)
	}

	secondEvents := collectRunEvents(secondServer.URL, secondRun.RunID)
	assertCompactAndCanary(secondEvents, "second")

	var assistantText strings.Builder
	for _, ev := range secondEvents {
		if ev.Type == agui.EventTextMessageContent {
			assistantText.WriteString(ev.Delta)
		}
	}

	if !strings.Contains(strings.ToLower(assistantText.String()), canary) {
		t.Fatalf("expected second run to recall canary token %q, got %q", canary, assistantText.String())
	}
}
