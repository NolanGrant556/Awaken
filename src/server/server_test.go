package server

import (
	"bufio"
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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"awaken/src/agentcore"
	"awaken/src/llm"
	"awaken/src/mcpclient"
	"awaken/src/protocol/agui"
	awruntime "awaken/src/runtime"
	"awaken/src/skills"
	_ "awaken/src/tools"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	NewRouter(nil, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "ok" {
		t.Fatalf("expected ok body, got %q", body)
	}
}

func TestAGUIDemoEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/agui/demo", nil)
	rec := httptest.NewRecorder()

	NewRouter(nil, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("expected event-stream content type, got %q", got)
	}

	body := rec.Body.String()
	for _, eventType := range []string{
		"RUN_STARTED",
		"TEXT_MESSAGE_CONTENT",
		"TOOL_CALL_START",
		"TOOL_CALL_RESULT",
		"RUN_FINISHED",
	} {
		if !strings.Contains(body, eventType) {
			t.Fatalf("expected body to contain %s, got %s", eventType, body)
		}
	}
}

func TestRuntimeInfoEndpoint(t *testing.T) {
	provider := &StaticRuntimeInfoProvider{
		Info: RuntimeInfo{
			RuntimeVersion:   RuntimeVersion,
			APIVersion:       "v1",
			ConfiguredModel:  "gpt-4-test",
			Protocol:         "openai_chat",
			HasAPIKey:        true,
			DefaultWorkspace: "/tmp/awaken-test",
			BaseURLHost:      "api.example.com",
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runtime/info", nil)
	rec := httptest.NewRecorder()

	NewRouter(nil, provider).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("expected json content type, got %q", got)
	}

	var info RuntimeInfo
	if err := json.NewDecoder(rec.Body).Decode(&info); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if info.RuntimeVersion != RuntimeVersion {
		t.Fatalf("expected runtime_version %q, got %q", RuntimeVersion, info.RuntimeVersion)
	}
	if info.APIVersion != "v1" {
		t.Fatalf("expected api_version v1, got %q", info.APIVersion)
	}
	if info.ConfiguredModel != "gpt-4-test" {
		t.Fatalf("expected configured_model gpt-4-test, got %q", info.ConfiguredModel)
	}
	if !info.HasAPIKey {
		t.Fatalf("expected has_api_key true")
	}
	if info.DefaultWorkspace != "/tmp/awaken-test" {
		t.Fatalf("expected default_workspace /tmp/awaken-test, got %q", info.DefaultWorkspace)
	}
	if info.BaseURLHost != "api.example.com" {
		t.Fatalf("expected base_url_host api.example.com, got %q", info.BaseURLHost)
	}

	body := rec.Body.String()
	if strings.Contains(body, "secret-api-key") {
		t.Fatalf("response body must not contain api key")
	}
	if strings.Contains(body, "https://api.example.com/v1") {
		t.Fatalf("response body must not contain full base url")
	}
}

func TestModelProvidersEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/model-providers", nil)
	rec := httptest.NewRecorder()

	NewRouter(nil, &StaticRuntimeInfoProvider{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var providers []struct {
		ID                string `json:"id"`
		DisplayName       string `json:"display_name"`
		ModelCapabilities map[string]struct {
			ContextWindow    int `json:"context_window"`
			MaxOutputTokens  int `json:"max_output_tokens"`
			DefaultMaxTokens int `json:"default_max_tokens"`
		} `json:"model_capabilities"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&providers); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	seen := map[string]bool{}
	for _, provider := range providers {
		seen[provider.ID] = true
		if provider.ID == "doubao" || provider.DisplayName == "Doubao" {
			t.Fatal("doubao must not be exposed as a separate provider")
		}
		if provider.ID == "deepseek" {
			if got := provider.ModelCapabilities["deepseek-v4-pro"].ContextWindow; got != 1000000 {
				t.Fatalf("expected deepseek-v4-pro context window metadata, got %d", got)
			}
			if got := provider.ModelCapabilities["deepseek-v4-pro"].MaxOutputTokens; got != 384000 {
				t.Fatalf("expected deepseek-v4-pro max output metadata, got %d", got)
			}
		}
		if provider.ID == "kimi-cn" {
			if got := provider.ModelCapabilities["kimi-k2.6"].ContextWindow; got != 256*1024 {
				t.Fatalf("expected kimi-k2.6 context window metadata, got %d", got)
			}
			if got := provider.ModelCapabilities["kimi-k2.6"].DefaultMaxTokens; got != 32*1024 {
				t.Fatalf("expected kimi-k2.6 default output metadata, got %d", got)
			}
		}
	}
	for _, id := range []string{"deepseek", "kimi-cn", "kimi-coding-plan", "volcengine"} {
		if !seen[id] {
			t.Fatalf("missing provider %q", id)
		}
	}
}

func TestRuntimeInfoEndpointDoesNotLeakSecrets(t *testing.T) {
	provider := NewLLMRuntimeInfoProvider(
		"claude-test",
		"https://api.anthropic.com/v1",
		"sk-secret-key-must-not-appear",
		"/workspace/secret",
		"anthropic_messages",
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/runtime/info", nil)
	rec := httptest.NewRecorder()

	NewRouter(nil, provider).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if strings.Contains(body, "sk-secret-key-must-not-appear") {
		t.Fatalf("response body leaked api key")
	}
	if strings.Contains(body, "https://api.anthropic.com/v1") {
		t.Fatalf("response body leaked full base url")
	}

	var info RuntimeInfo
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&info); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if info.BaseURLHost != "api.anthropic.com" {
		t.Fatalf("expected base_url_host api.anthropic.com, got %q", info.BaseURLHost)
	}
	if info.Protocol != "anthropic_messages" {
		t.Fatalf("expected protocol anthropic_messages, got %q", info.Protocol)
	}
}

func TestRunsEndpointsCreateReplayAndCancel(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextMessageStartedEventType, MessageID: "msg-1", Role: "assistant"})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextDeltaEventType, MessageID: "msg-1", Role: "assistant", Delta: "hello"})
			close(started)
			select {
			case <-release:
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextMessageFinishedEventType, MessageID: "msg-1"})
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
				return "done", nil
			case <-ctx.Done():
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunErrorEventType, Error: ctx.Err().Error()})
				return "", ctx.Err()
			}
		})
	})

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(`{"input":"hello","workspace":{"workspace_root":"/tmp/awaken-test-workspace"}}`))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if created.RunID == "" || created.ThreadID == "" {
		t.Fatalf("expected run and thread ids, got %+v", created)
	}
	if created.Status != awruntime.RunStatusRunning {
		t.Fatalf("expected running status, got %s", created.Status)
	}

	<-started

	done := make(chan string, 1)
	go func() {
		streamResp, err := http.Get(ts.URL + "/v1/runs/" + created.RunID + "/events")
		if err != nil {
			done <- "request error: " + err.Error()
			return
		}
		defer streamResp.Body.Close()
		body, err := io.ReadAll(streamResp.Body)
		if err != nil {
			done <- "read error: " + err.Error()
			return
		}
		done <- string(body)
	}()

	time.Sleep(50 * time.Millisecond)
	close(release)

	body := <-done
	for _, token := range []string{
		"RUN_STARTED",
		"TEXT_MESSAGE_CONTENT",
		"RUN_FINISHED",
		created.RunID,
		created.ThreadID,
	} {
		if !strings.Contains(body, token) {
			t.Fatalf("expected stream body to contain %q, got %s", token, body)
		}
	}

	cancelResp, err := http.Post(ts.URL+"/v1/runs/"+created.RunID+"/cancel", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /cancel failed: %v", err)
	}
	defer cancelResp.Body.Close()
	if cancelResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from cancel terminal run, got %d", cancelResp.StatusCode)
	}
}

func TestRunEventsUnknownFallbackReplayAfterSeq(t *testing.T) {
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{
				Type:    agentcore.AwakenEventType("FutureEvent"),
				Content: "future payload",
			})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			return "done", nil
		})
	})

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(`{"input":"hello","workspace":{"workspace_root":"/tmp/awaken-test-workspace"}}`))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	waitHTTPRunTerminal(t, ts.URL, created.RunID)

	replayResp, err := http.Get(ts.URL + "/v1/runs/" + created.RunID + "/events?after_seq=16")
	if err != nil {
		t.Fatalf("GET replay events failed: %v", err)
	}
	defer replayResp.Body.Close()
	body, err := io.ReadAll(replayResp.Body)
	if err != nil {
		t.Fatalf("failed to read replay body: %v", err)
	}
	got := string(body)
	if strings.Contains(got, "RUN_STARTED") {
		t.Fatalf("expected after_seq replay to skip RUN_STARTED, got %s", got)
	}
	for _, token := range []string{"AWAKEN_UNKNOWN_EVENT", "FutureEvent", "future payload", "RUN_FINISHED"} {
		if !strings.Contains(got, token) {
			t.Fatalf("expected replay body to contain %q, got %s", token, got)
		}
	}
}

func TestRunsEndpointsValidationAndNotFound(t *testing.T) {
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			return "", nil
		})
	})

	router := NewRouter(runService, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", strings.NewReader(`{"input":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty input, got %d", rec.Code)
	}
	var errResp errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("expected JSON error response, got %q", rec.Body.String())
	}
	if errResp.Error != "INVALID_INPUT" {
		t.Fatalf("expected INVALID_INPUT error, got %q", errResp.Error)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/runs/missing/events", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing events run, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/runs/missing/cancel", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing cancel run, got %d", rec.Code)
	}
	var cancelErrResp errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &cancelErrResp); err != nil {
		t.Fatalf("expected JSON error response for missing cancel, got %q", rec.Body.String())
	}
	if cancelErrResp.Error != "RUN_NOT_FOUND" {
		t.Fatalf("expected RUN_NOT_FOUND error code, got %q", cancelErrResp.Error)
	}
	if strings.TrimSpace(cancelErrResp.Message) == "" {
		t.Fatalf("expected non-empty error message for missing cancel, got %+v", cancelErrResp)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/runs/missing/approvals/approval-1/resolve", strings.NewReader(`{"decision":"approved"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing approval run, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/runs/missing/approvals/approval-1/resolve", strings.NewReader(`{"decision":"later"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid approval decision, got %d", rec.Code)
	}
}

func TestCreateRunEmptyInputReturnsInvalidInput(t *testing.T) {
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			return "", nil
		})
	})

	router := NewRouter(runService, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", strings.NewReader(`{"input":"","workspace":{"workspace_root":"/tmp"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty input, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "INVALID_INPUT") {
		t.Fatalf("expected INVALID_INPUT in response body, got %q", body)
	}

	var created awruntime.Run
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err == nil {
		if created.RunID != "" || created.Status == awruntime.RunStatusRunning {
			t.Fatalf("expected no run to be created, got %+v", created)
		}
	}

	var errResp errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("expected JSON error response, got %q", body)
	}
	if errResp.Error != "INVALID_INPUT" {
		t.Fatalf("expected INVALID_INPUT error, got %q", errResp.Error)
	}
	if errResp.Message != "input is required" {
		t.Fatalf("expected message 'input is required', got %q", errResp.Message)
	}
}

func TestCreateRunRequiresModelProfileForProfileAwareRuntime(t *testing.T) {
	runService := awruntime.NewRunServiceWithModelProfiles(
		func(workspace awruntime.WorkspaceConfig, threadID string, profile *awruntime.ModelProfileConfig) *agentcore.Agent {
			t.Fatalf("agent should not be created without a model profile")
			return nil
		},
		func(agent *agentcore.Agent) awruntime.Executor {
			return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				return "ok", nil
			})
		},
	)

	router := NewRouter(runService, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", strings.NewReader(`{"input":"hello","workspace":{"workspace_root":"/tmp"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing model profile, got %d", rec.Code)
	}
	var errResp errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("expected JSON error response, got %q", rec.Body.String())
	}
	if errResp.Error != "MISSING_MODEL_PROFILE" {
		t.Fatalf("expected MISSING_MODEL_PROFILE error, got %q", errResp.Error)
	}
}

func TestCreateRunWithUnknownThreadIDReturns404(t *testing.T) {
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			return "", nil
		})
	})

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(`{"input":"hello","thread_id":"unknown-thread","workspace":{"workspace_root":"/tmp"}}`))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown thread_id, got %d", resp.StatusCode)
	}

	var errResp errorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("expected JSON error response, got decode error: %v", err)
	}
	if errResp.Error != "THREAD_NOT_FOUND" {
		t.Fatalf("expected THREAD_NOT_FOUND error code, got %q", errResp.Error)
	}
}

func TestCreateRunWithValidThreadIDReusesThread(t *testing.T) {
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			return "done", nil
		})
	})

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	// First run: no thread_id, server allocates one.
	firstResp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(`{"input":"hello","workspace":{"workspace_root":"/tmp"}}`))
	if err != nil {
		t.Fatalf("POST /v1/runs (first) failed: %v", err)
	}
	defer firstResp.Body.Close()
	if firstResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 from first run, got %d", firstResp.StatusCode)
	}
	var firstRun awruntime.Run
	if err := json.NewDecoder(firstResp.Body).Decode(&firstRun); err != nil {
		t.Fatalf("failed to decode first run response: %v", err)
	}
	if firstRun.ThreadID == "" {
		t.Fatalf("expected non-empty thread_id on first run, got %+v", firstRun)
	}
	waitHTTPRunTerminal(t, ts.URL, firstRun.RunID)

	// Second run: reuse the thread_id from the first run with the same workspace.
	secondBody := fmt.Sprintf(`{"input":"follow up","thread_id":%q,"workspace":{"workspace_root":"/tmp"}}`, firstRun.ThreadID)
	secondResp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(secondBody))
	if err != nil {
		t.Fatalf("POST /v1/runs (second) failed: %v", err)
	}
	defer secondResp.Body.Close()
	if secondResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 from second run reusing thread_id, got %d", secondResp.StatusCode)
	}
	var secondRun awruntime.Run
	if err := json.NewDecoder(secondResp.Body).Decode(&secondRun); err != nil {
		t.Fatalf("failed to decode second run response: %v", err)
	}
	if secondRun.ThreadID != firstRun.ThreadID {
		t.Fatalf("expected second run to reuse thread_id %q, got %q", firstRun.ThreadID, secondRun.ThreadID)
	}
	if secondRun.RunID == firstRun.RunID {
		t.Fatalf("expected second run to have a new run_id, got duplicate %q", secondRun.RunID)
	}
}

func TestCreateRunWithWorkspaceMismatchReturns400(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatalf("failed to create child dir: %v", err)
	}
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			return "done", nil
		})
	})

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	firstBody := fmt.Sprintf(`{"input":"hello","workspace":{"workspace_root":%q,"writable_roots":[%q]}}`, root, root)
	firstResp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(firstBody))
	if err != nil {
		t.Fatalf("POST /v1/runs (first) failed: %v", err)
	}
	defer firstResp.Body.Close()
	if firstResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 from first run, got %d", firstResp.StatusCode)
	}
	var firstRun awruntime.Run
	if err := json.NewDecoder(firstResp.Body).Decode(&firstRun); err != nil {
		t.Fatalf("failed to decode first run response: %v", err)
	}
	waitHTTPRunTerminal(t, ts.URL, firstRun.RunID)

	secondBody := fmt.Sprintf(`{"input":"follow up","thread_id":%q,"workspace":{"workspace_root":%q,"writable_roots":[%q]}}`, firstRun.ThreadID, root, child)
	secondResp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(secondBody))
	if err != nil {
		t.Fatalf("POST /v1/runs (second) failed: %v", err)
	}
	defer secondResp.Body.Close()
	if secondResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for task-scope update, got %d", secondResp.StatusCode)
	}
	var secondRun awruntime.Run
	if err := json.NewDecoder(secondResp.Body).Decode(&secondRun); err != nil {
		t.Fatalf("failed to decode second run response: %v", err)
	}
	waitHTTPRunTerminal(t, ts.URL, secondRun.RunID)
}

func TestCreateRunWithActiveThreadReturns409(t *testing.T) {
	started := make(chan struct{})
	var startedOnce sync.Once
	release := make(chan struct{})
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			startedOnce.Do(func() { close(started) })
			<-release
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			return "done", nil
		})
	})

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	firstResp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(`{"input":"hello","workspace":{"workspace_root":"/tmp"}}`))
	if err != nil {
		t.Fatalf("POST /v1/runs (first) failed: %v", err)
	}
	defer firstResp.Body.Close()
	if firstResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 from first run, got %d", firstResp.StatusCode)
	}
	var firstRun awruntime.Run
	if err := json.NewDecoder(firstResp.Body).Decode(&firstRun); err != nil {
		t.Fatalf("failed to decode first run response: %v", err)
	}
	<-started

	secondBody := fmt.Sprintf(`{"input":"follow up","thread_id":%q,"workspace":{"workspace_root":"/tmp"}}`, firstRun.ThreadID)
	secondResp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(secondBody))
	if err != nil {
		t.Fatalf("POST /v1/runs (second) failed: %v", err)
	}
	defer secondResp.Body.Close()
	if secondResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for active thread, got %d", secondResp.StatusCode)
	}
	var errResp errorResponse
	if err := json.NewDecoder(secondResp.Body).Decode(&errResp); err != nil {
		t.Fatalf("expected JSON error response, got decode error: %v", err)
	}
	if errResp.Error != "THREAD_BUSY" {
		t.Fatalf("expected THREAD_BUSY error code, got %q", errResp.Error)
	}
	close(release)
	waitHTTPRunTerminal(t, ts.URL, firstRun.RunID)
}

func TestListThreadsEndpointReturnsRuntimeThreads(t *testing.T) {
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			return "done", nil
		})
	})

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(`{"input":"hello list","workspace":{"workspace_root":"/tmp"}}`))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode run response: %v", err)
	}
	waitHTTPRunTerminal(t, ts.URL, created.RunID)

	listResp, err := http.Get(ts.URL + "/v1/threads")
	if err != nil {
		t.Fatalf("GET /v1/threads failed: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /v1/threads, got %d", listResp.StatusCode)
	}
	var threads []awruntime.ThreadSnapshot
	if err := json.NewDecoder(listResp.Body).Decode(&threads); err != nil {
		t.Fatalf("failed to decode thread list: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected one thread, got %d", len(threads))
	}
	if threads[0].ThreadID != created.ThreadID {
		t.Fatalf("expected thread id %q, got %q", created.ThreadID, threads[0].ThreadID)
	}
	if threads[0].RunID != created.RunID {
		t.Fatalf("expected latest run id %q, got %q", created.RunID, threads[0].RunID)
	}
	if threads[0].Label != "hello list" {
		t.Fatalf("expected label from first input, got %q", threads[0].Label)
	}
}

func TestRenameAndDeleteThreadEndpoints(t *testing.T) {
	workspaceRoot := t.TempDir()
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			return "done", nil
		})
	})

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	createBody := fmt.Sprintf(`{"input":"thread to rename","workspace":{"workspace_root":%q}}`, workspaceRoot)
	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(createBody))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode run response: %v", err)
	}
	waitHTTPRunTerminal(t, ts.URL, created.RunID)

	renameReq, err := http.NewRequest(
		http.MethodPatch,
		ts.URL+"/v1/threads/"+created.ThreadID,
		strings.NewReader(`{"label":"Renamed from API"}`),
	)
	if err != nil {
		t.Fatalf("failed to build rename request: %v", err)
	}
	renameReq.Header.Set("Content-Type", "application/json")
	renameResp, err := http.DefaultClient.Do(renameReq)
	if err != nil {
		t.Fatalf("PATCH /v1/threads/{id} failed: %v", err)
	}
	defer renameResp.Body.Close()
	if renameResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from rename, got %d", renameResp.StatusCode)
	}
	var renamed awruntime.ThreadSnapshot
	if err := json.NewDecoder(renameResp.Body).Decode(&renamed); err != nil {
		t.Fatalf("failed to decode rename response: %v", err)
	}
	if renamed.Label != "Renamed from API" {
		t.Fatalf("expected renamed label, got %q", renamed.Label)
	}

	deleteReq, err := http.NewRequest(http.MethodDelete, ts.URL+"/v1/threads/"+created.ThreadID, nil)
	if err != nil {
		t.Fatalf("failed to build delete request: %v", err)
	}
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("DELETE /v1/threads/{id} failed: %v", err)
	}
	defer deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 from delete, got %d", deleteResp.StatusCode)
	}

	listResp, err := http.Get(ts.URL + "/v1/threads")
	if err != nil {
		t.Fatalf("GET /v1/threads failed: %v", err)
	}
	defer listResp.Body.Close()
	var threads []awruntime.ThreadSnapshot
	if err := json.NewDecoder(listResp.Body).Decode(&threads); err != nil {
		t.Fatalf("failed to decode thread list: %v", err)
	}
	if len(threads) != 0 {
		t.Fatalf("expected no threads after delete, got %+v", threads)
	}
}

func TestWorkspaceRegistryEndpointRestoresHistoryFromRegisteredRoots(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	newExecutor := func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			return "done", nil
		})
	}

	seedService := awruntime.NewRunService(nil, newExecutor)
	run, err := seedService.StartRun(awruntime.StartRunRequest{
		Input:     "history in workspace b",
		Workspace: awruntime.WorkspaceConfig{WorkspaceRoot: rootB},
	})
	if err != nil {
		t.Fatalf("seed StartRun returned error: %v", err)
	}
	waitServiceRunTerminal(t, seedService, run.RunID)

	runService := awruntime.NewRunService(nil, newExecutor)
	if err := runService.SetDefaultWorkspace(awruntime.WorkspaceConfig{WorkspaceRoot: rootA}); err != nil {
		t.Fatalf("SetDefaultWorkspace returned error: %v", err)
	}

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	req, err := http.NewRequest(
		http.MethodPut,
		ts.URL+"/v1/workspaces/registry",
		strings.NewReader(fmt.Sprintf(`{"workspace_roots":[%q]}`, rootB)),
	)
	if err != nil {
		t.Fatalf("failed to build workspace registry request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /v1/workspaces/registry failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 from workspace registry sync, got %d", resp.StatusCode)
	}

	listResp, err := http.Get(ts.URL + "/v1/threads")
	if err != nil {
		t.Fatalf("GET /v1/threads failed: %v", err)
	}
	defer listResp.Body.Close()
	var threads []awruntime.ThreadSnapshot
	if err := json.NewDecoder(listResp.Body).Decode(&threads); err != nil {
		t.Fatalf("failed to decode thread list: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected one thread from registered workspace, got %+v", threads)
	}
	if threads[0].Workspace.WorkspaceRoot != rootB {
		t.Fatalf("expected restored thread workspace root %q, got %+v", rootB, threads[0])
	}
}

func TestGetThreadTranscriptEndpointReturnsMappedRuns(t *testing.T) {
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{
				Type:      agentcore.TextMessageStartedEventType,
				MessageID: "msg-1",
				Role:      "assistant",
			})
			sink.Emit(agentcore.AwakenEvent{
				Type:      agentcore.TextDeltaEventType,
				MessageID: "msg-1",
				Role:      "assistant",
				Delta:     input,
			})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: input})
			return input, nil
		})
	})

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	firstResp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(`{"input":"hello","workspace":{"workspace_root":"/tmp"}}`))
	if err != nil {
		t.Fatalf("POST /v1/runs (first) failed: %v", err)
	}
	defer firstResp.Body.Close()
	var firstRun awruntime.Run
	if err := json.NewDecoder(firstResp.Body).Decode(&firstRun); err != nil {
		t.Fatalf("failed to decode first run: %v", err)
	}
	waitHTTPRunTerminal(t, ts.URL, firstRun.RunID)

	secondBody := fmt.Sprintf(`{"input":"follow up","thread_id":%q,"workspace":{"workspace_root":"/tmp"}}`, firstRun.ThreadID)
	secondResp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(secondBody))
	if err != nil {
		t.Fatalf("POST /v1/runs (second) failed: %v", err)
	}
	defer secondResp.Body.Close()
	var secondRun awruntime.Run
	if err := json.NewDecoder(secondResp.Body).Decode(&secondRun); err != nil {
		t.Fatalf("failed to decode second run: %v", err)
	}
	waitHTTPRunTerminal(t, ts.URL, secondRun.RunID)

	transcriptResp, err := http.Get(ts.URL + "/v1/threads/" + firstRun.ThreadID + "/transcript")
	if err != nil {
		t.Fatalf("GET /v1/threads/{thread_id}/transcript failed: %v", err)
	}
	defer transcriptResp.Body.Close()
	if transcriptResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from transcript endpoint, got %d", transcriptResp.StatusCode)
	}

	var transcript threadTranscriptResponse
	if err := json.NewDecoder(transcriptResp.Body).Decode(&transcript); err != nil {
		t.Fatalf("failed to decode transcript response: %v", err)
	}
	if transcript.Thread.ThreadID != firstRun.ThreadID {
		t.Fatalf("expected thread id %q, got %q", firstRun.ThreadID, transcript.Thread.ThreadID)
	}
	if len(transcript.Runs) != 2 {
		t.Fatalf("expected 2 transcript runs, got %d", len(transcript.Runs))
	}
	if transcript.Runs[1].Run.Input != "follow up" {
		t.Fatalf("expected second run input to be preserved, got %q", transcript.Runs[1].Run.Input)
	}
	if len(transcript.Runs[1].Events) == 0 {
		t.Fatalf("expected mapped events in transcript response")
	}
	if transcript.Runs[1].Events[0].Type != "RUN_STARTED" {
		t.Fatalf("expected transcript event to be mapped as AG-UI frame, got %s", transcript.Runs[1].Events[0].Type)
	}
}

func waitHTTPRunTerminal(t *testing.T, baseURL, runID string) awruntime.Run {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		getResp, err := http.Get(baseURL + "/v1/runs/" + runID)
		if err != nil {
			t.Fatalf("GET /v1/runs/{run_id} failed: %v", err)
		}
		var run awruntime.Run
		err = json.NewDecoder(getResp.Body).Decode(&run)
		getResp.Body.Close()
		if err != nil {
			t.Fatalf("failed to decode run response: %v", err)
		}
		switch run.Status {
		case awruntime.RunStatusFinished, awruntime.RunStatusError, awruntime.RunStatusCancelled:
			if !httpThreadHasActiveRun(t, baseURL, run.ThreadID, run.RunID) {
				return run
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run %s did not reach terminal status", runID)
	return awruntime.Run{}
}

func waitServiceRunTerminal(t *testing.T, service *awruntime.RunService, runID string) awruntime.Run {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, err := service.GetRun(runID)
		if err != nil {
			t.Fatalf("GetRun(%s) failed: %v", runID, err)
		}
		switch run.Status {
		case awruntime.RunStatusFinished, awruntime.RunStatusError, awruntime.RunStatusCancelled:
			return run
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run %s did not reach terminal status", runID)
	return awruntime.Run{}
}

func httpThreadHasActiveRun(t *testing.T, baseURL, threadID, runID string) bool {
	t.Helper()
	if threadID == "" {
		return false
	}
	resp, err := http.Get(baseURL + "/v1/threads")
	if err != nil {
		t.Fatalf("GET /v1/threads failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /v1/threads, got %d", resp.StatusCode)
	}
	var threads []awruntime.ThreadSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&threads); err != nil {
		t.Fatalf("failed to decode thread list: %v", err)
	}
	for _, thread := range threads {
		if thread.ThreadID == threadID {
			return thread.ActiveRunID == runID
		}
	}
	return false
}

func TestMapRuntimeCancelledUsesAGUIRunErrorWithCancelCode(t *testing.T) {
	events := mapRuntimeEvent(awruntime.Event{
		Kind:           awruntime.EventKindRunCancelled,
		ThreadID:       "thread-1",
		RunID:          "run-1",
		Message:        "run cancelled",
		TerminalReason: agentcore.TerminalReasonCancelled,
	})

	if len(events) != 1 {
		t.Fatalf("expected one mapped event, got %d", len(events))
	}
	if events[0].Type != "RUN_ERROR" {
		t.Fatalf("expected cancellation to map to RUN_ERROR, got %s", events[0].Type)
	}
	if events[0].Code != "RUN_CANCELLED" {
		t.Fatalf("expected RUN_CANCELLED code, got %q", events[0].Code)
	}
	if events[0].TerminalReason != "cancelled" {
		t.Fatalf("expected terminal reason cancelled, got %q", events[0].TerminalReason)
	}
}

func TestGetRunReturnsRunSnapshot(t *testing.T) {
	started := make(chan struct{})
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			close(started)
			return "done", nil
		})
	})

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(`{"input":"hello","workspace":{"workspace_root":"/tmp/awaken-test-workspace"}}`))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	<-started

	// Wait for run to reach finished state.
	deadline := time.Now().Add(2 * time.Second)
	var run awruntime.Run
	for time.Now().Before(deadline) {
		getResp, err := http.Get(ts.URL + "/v1/runs/" + created.RunID)
		if err != nil {
			t.Fatalf("GET /v1/runs/{run_id} failed: %v", err)
		}
		_ = json.NewDecoder(getResp.Body).Decode(&run)
		getResp.Body.Close()
		if run.Status == awruntime.RunStatusFinished {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if run.Status != awruntime.RunStatusFinished {
		t.Fatalf("expected finished status, got %s", run.Status)
	}
	if run.RunID != created.RunID {
		t.Fatalf("expected run_id %q, got %q", created.RunID, run.RunID)
	}
	if run.ThreadID != created.ThreadID {
		t.Fatalf("expected thread_id %q, got %q", created.ThreadID, run.ThreadID)
	}
}

func TestGetRunMissingReturns404(t *testing.T) {
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			return "", nil
		})
	})

	router := NewRouter(runService, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/missing", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing run, got %d", rec.Code)
	}

	var errResp errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("expected JSON error response, got %q", rec.Body.String())
	}
	if errResp.Error != "RUN_NOT_FOUND" {
		t.Fatalf("expected RUN_NOT_FOUND error code, got %q", errResp.Error)
	}
}

func TestRunEventsAfterSeqFiltersHistory(t *testing.T) {
	finished := make(chan struct{})
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextMessageStartedEventType, MessageID: "msg-1", Role: "assistant"})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextDeltaEventType, MessageID: "msg-1", Role: "assistant", Delta: "hello"})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextMessageFinishedEventType, MessageID: "msg-1"})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			close(finished)
			return "done", nil
		})
	})

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(`{"input":"hello","workspace":{"workspace_root":"/tmp/awaken-test-workspace"}}`))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	<-finished

	// Wait for the run to reach terminal state so the event bus is closed
	// and the subscription returns history only (no live tail).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		getResp, err := http.Get(ts.URL + "/v1/runs/" + created.RunID)
		if err != nil {
			t.Fatalf("GET /v1/runs/{run_id} failed: %v", err)
		}
		var run awruntime.Run
		_ = json.NewDecoder(getResp.Body).Decode(&run)
		getResp.Body.Close()
		if run.Status == awruntime.RunStatusFinished {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// First, get all events (after_seq=0) to determine the seq values.
	allEvents := fetchSSEEvents(t, ts.URL, created.RunID, "")
	if len(allEvents) == 0 {
		t.Fatalf("expected at least one event with after_seq=0, got none")
	}
	// Verify events have strictly increasing seq values.
	for i, ev := range allEvents {
		if ev.Seq == 0 {
			t.Fatalf("event %d has seq=0, expected non-zero seq", i)
		}
		if i > 0 && ev.Seq <= allEvents[i-1].Seq {
			t.Fatalf("seq not strictly increasing: event %d has seq=%d, previous has seq=%d", i, ev.Seq, allEvents[i-1].Seq)
		}
	}

	// Now subscribe with after_seq=<first frame seq> and verify only frames
	// with seq > firstSeq are returned (one fewer than the full set).
	firstSeq := allEvents[0].Seq
	filteredEvents := fetchSSEEvents(t, ts.URL, created.RunID, strconv.FormatUint(firstSeq, 10))
	for _, ev := range filteredEvents {
		if ev.Seq <= firstSeq {
			t.Fatalf("expected all events to have seq > %d, got seq=%d", firstSeq, ev.Seq)
		}
	}
	if len(filteredEvents) != len(allEvents)-1 {
		t.Fatalf("expected %d events after filtering (one less than %d), got %d", len(allEvents)-1, len(allEvents), len(filteredEvents))
	}
}

func TestSSEFrameIncludesIdLineWithSeq(t *testing.T) {
	finished := make(chan struct{})
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			close(finished)
			return "done", nil
		})
	})

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(`{"input":"hello","workspace":{"workspace_root":"/tmp/awaken-test-workspace"}}`))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	<-finished

	// Wait for terminal state.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		getResp, err := http.Get(ts.URL + "/v1/runs/" + created.RunID)
		if err != nil {
			t.Fatalf("GET /v1/runs/{run_id} failed: %v", err)
		}
		var run awruntime.Run
		_ = json.NewDecoder(getResp.Body).Decode(&run)
		getResp.Body.Close()
		if run.Status == awruntime.RunStatusFinished {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Fetch raw SSE body and verify id: lines are present.
	eventsURL := ts.URL + "/v1/runs/" + created.RunID + "/events"
	streamResp, err := http.Get(eventsURL)
	if err != nil {
		t.Fatalf("GET events failed: %v", err)
	}
	defer streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from events, got %d", streamResp.StatusCode)
	}

	body, err := io.ReadAll(streamResp.Body)
	if err != nil {
		t.Fatalf("failed to read events body: %v", err)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "id: ") {
		t.Fatalf("expected SSE body to contain 'id: ' lines, got:\n%s", bodyStr)
	}

	// Verify each id: line is followed by a data: line.
	scanner := bufio.NewScanner(strings.NewReader(bodyStr))
	var idLines, dataLines int
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "id: ") {
			idLines++
		}
		if strings.HasPrefix(line, "data: ") {
			dataLines++
		}
	}
	if idLines == 0 {
		t.Fatalf("expected at least one id: line, got 0")
	}
	if idLines != dataLines {
		t.Fatalf("expected id: line count (%d) to equal data: line count (%d)", idLines, dataLines)
	}
}

func fetchSSEEvents(t *testing.T, baseURL, runID, afterSeq string) []agui.Event {
	t.Helper()
	eventsURL := baseURL + "/v1/runs/" + runID + "/events"
	if afterSeq != "" {
		eventsURL += "?after_seq=" + afterSeq
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, eventsURL, nil)
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
		t.Fatalf("failed to collect SSE events: %v", err)
	}
	return events
}

// TestRunEventsMultiFrameSeqUnique verifies that a runtime event mapping to
// multiple AG-UI frames (e.g. ReasoningStartedEventType → REASONING_START +
// REASONING_MESSAGE_START) produces frames with distinct, strictly increasing
// seq values.
func TestRunEventsMultiFrameSeqUnique(t *testing.T) {
	finished := make(chan struct{})
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.ReasoningStartedEventType, MessageID: "reasoning-1"})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.ReasoningDeltaEventType, MessageID: "reasoning-1", Delta: "thinking"})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.ReasoningFinishedEventType, MessageID: "reasoning-1"})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			close(finished)
			return "done", nil
		})
	})

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(`{"input":"hello","workspace":{"workspace_root":"/tmp/awaken-test-workspace"}}`))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	<-finished

	// Wait for terminal state.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		getResp, err := http.Get(ts.URL + "/v1/runs/" + created.RunID)
		if err != nil {
			t.Fatalf("GET /v1/runs/{run_id} failed: %v", err)
		}
		var run awruntime.Run
		_ = json.NewDecoder(getResp.Body).Decode(&run)
		getResp.Body.Close()
		if run.Status == awruntime.RunStatusFinished {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	allEvents := fetchSSEEvents(t, ts.URL, created.RunID, "")

	// Find the REASONING_START and REASONING_MESSAGE_START frames — they both
	// originate from the same ReasoningStartedEventType runtime event and must
	// have distinct seqs.
	var reasoningStartSeq, reasoningMessageStartSeq uint64
	foundStart, foundMessageStart := false, false
	for _, ev := range allEvents {
		if ev.Type == agui.EventReasoningStart {
			reasoningStartSeq = ev.Seq
			foundStart = true
		}
		if ev.Type == agui.EventReasoningMessageStart {
			reasoningMessageStartSeq = ev.Seq
			foundMessageStart = true
		}
	}
	if !foundStart || !foundMessageStart {
		t.Fatalf("expected REASONING_START and REASONING_MESSAGE_START frames, got events: %+v", allEvents)
	}
	if reasoningStartSeq == reasoningMessageStartSeq {
		t.Fatalf("expected distinct seqs for REASONING_START (%d) and REASONING_MESSAGE_START (%d)", reasoningStartSeq, reasoningMessageStartSeq)
	}
	if reasoningMessageStartSeq <= reasoningStartSeq {
		t.Fatalf("expected REASONING_MESSAGE_START seq (%d) > REASONING_START seq (%d)", reasoningMessageStartSeq, reasoningStartSeq)
	}
}

// TestRunEventsAfterSeqReconnectMultiFrame verifies that reconnecting with
// after_seq set to the first frame of a multi-frame runtime event does not
// lose the second frame.
func TestRunEventsAfterSeqReconnectMultiFrame(t *testing.T) {
	finished := make(chan struct{})
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.ReasoningStartedEventType, MessageID: "reasoning-1"})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.ReasoningDeltaEventType, MessageID: "reasoning-1", Delta: "thinking"})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.ReasoningFinishedEventType, MessageID: "reasoning-1"})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			close(finished)
			return "done", nil
		})
	})

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(`{"input":"hello","workspace":{"workspace_root":"/tmp/awaken-test-workspace"}}`))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	<-finished

	// Wait for terminal state.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		getResp, err := http.Get(ts.URL + "/v1/runs/" + created.RunID)
		if err != nil {
			t.Fatalf("GET /v1/runs/{run_id} failed: %v", err)
		}
		var run awruntime.Run
		_ = json.NewDecoder(getResp.Body).Decode(&run)
		getResp.Body.Close()
		if run.Status == awruntime.RunStatusFinished {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	allEvents := fetchSSEEvents(t, ts.URL, created.RunID, "")

	// Find the REASONING_START frame seq (first frame of the multi-frame event).
	var reasoningStartSeq uint64
	found := false
	for _, ev := range allEvents {
		if ev.Type == agui.EventReasoningStart {
			reasoningStartSeq = ev.Seq
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected REASONING_START frame, got events: %+v", allEvents)
	}

	// Simulate reconnect: client received REASONING_START (seq=reasoningStartSeq)
	// then disconnected. Reconnect with after_seq=reasoningStartSeq should still
	// deliver REASONING_MESSAGE_START (the second frame of the same runtime event).
	replayedEvents := fetchSSEEvents(t, ts.URL, created.RunID, strconv.FormatUint(reasoningStartSeq, 10))

	hasMessageStart := false
	for _, ev := range replayedEvents {
		if ev.Seq <= reasoningStartSeq {
			t.Fatalf("expected all replayed frames to have seq > %d, got seq=%d (type=%s)", reasoningStartSeq, ev.Seq, ev.Type)
		}
		if ev.Type == agui.EventReasoningMessageStart {
			hasMessageStart = true
		}
	}
	if !hasMessageStart {
		t.Fatalf("expected replayed events to include REASONING_MESSAGE_START after reconnect with after_seq=%d, got: %+v", reasoningStartSeq, replayedEvents)
	}
}

func TestResolveApprovalEndpointPublishesExtensionEvents(t *testing.T) {
	approvalReady := make(chan struct{})
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			awaiter, ok := sink.(agentcore.ApprovalAwaiter)
			if !ok {
				return "", errors.New("expected approval awaiter on runtime sink")
			}
			close(approvalReady)
			decision, err := awaiter.AwaitApproval(ctx, agentcore.ApprovalRequest{
				ID:          "approval-1",
				ToolCallID:  "tool-1",
				ToolName:    "write_file",
				Title:       "Approve write_file",
				Description: "Write README.md",
				RiskTone:    agentcore.ApprovalRiskDanger,
			})
			if err != nil {
				return "", err
			}
			sink.Emit(agentcore.AwakenEvent{
				Type:                agentcore.ArtifactCreatedEventType,
				ToolCallID:          "tool-1",
				ToolName:            "write_file",
				ArtifactID:          "artifact-1",
				ArtifactTitle:       "README.md",
				ArtifactDescription: "Generated file",
				ArtifactKind:        "file",
				ArtifactPath:        "/tmp/README.md",
				ArtifactContent:     "# hello",
				ArtifactStatus:      agentcore.ArtifactStatusReady,
			})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: string(decision)})
			return string(decision), nil
		})
	})

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(`{"input":"hello","workspace":{"workspace_root":"/tmp/awaken-test-workspace"}}`))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	<-approvalReady

	done := make(chan string, 1)
	go func() {
		streamResp, err := http.Get(ts.URL + "/v1/runs/" + created.RunID + "/events")
		if err != nil {
			done <- "request error: " + err.Error()
			return
		}
		defer streamResp.Body.Close()
		body, err := io.ReadAll(streamResp.Body)
		if err != nil {
			done <- "read error: " + err.Error()
			return
		}
		done <- string(body)
	}()

	resolveURL := ts.URL + "/v1/runs/" + created.RunID + "/approvals/approval-1/resolve"
	var resolveResp *http.Response
	deadline := time.Now().Add(2 * time.Second)
	for {
		resolveResp, err = http.Post(resolveURL, "application/json", strings.NewReader(`{"decision":"approved"}`))
		if err != nil {
			t.Fatalf("POST resolve approval failed: %v", err)
		}
		if resolveResp.StatusCode == http.StatusNoContent {
			break
		}
		_ = resolveResp.Body.Close()
		if time.Now().After(deadline) {
			t.Fatalf("approval resolve never succeeded, last status %d", resolveResp.StatusCode)
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = resolveResp.Body.Close()

	body := <-done
	for _, token := range []string{
		"AWAKEN_APPROVAL_REQUESTED",
		"AWAKEN_APPROVAL_RESOLVED",
		"AWAKEN_ARTIFACT_CREATED",
		`"approvalId":"approval-1"`,
		`"artifactId":"artifact-1"`,
		"RUN_FINISHED",
	} {
		if !strings.Contains(body, token) {
			t.Fatalf("expected stream body to contain %q, got %s", token, body)
		}
	}
}

func TestResolveApprovalDuplicateHTTPReturnsNotFound(t *testing.T) {
	approvalReady := make(chan struct{})
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			awaiter, ok := sink.(agentcore.ApprovalAwaiter)
			if !ok {
				return "", errors.New("expected approval awaiter on runtime sink")
			}
			close(approvalReady)
			_, err := awaiter.AwaitApproval(ctx, agentcore.ApprovalRequest{
				ID:          "approval-1",
				ToolCallID:  "tool-1",
				ToolName:    "write_file",
				Title:       "Approve write_file",
				Description: "Write README.md",
				RiskTone:    agentcore.ApprovalRiskDanger,
			})
			if err != nil {
				return "", err
			}
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType})
			return "", nil
		})
	})

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(`{"input":"hello","workspace":{"workspace_root":"/tmp/awaken-test-workspace"}}`))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	<-approvalReady

	resolveURL := ts.URL + "/v1/runs/" + created.RunID + "/approvals/approval-1/resolve"
	firstResp, err := http.Post(resolveURL, "application/json", strings.NewReader(`{"decision":"approved"}`))
	if err != nil {
		t.Fatalf("POST resolve approval failed: %v", err)
	}
	firstResp.Body.Close()
	if firstResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 on first resolve, got %d", firstResp.StatusCode)
	}

	secondResp, err := http.Post(resolveURL, "application/json", strings.NewReader(`{"decision":"approved"}`))
	if err != nil {
		t.Fatalf("POST duplicate resolve approval failed: %v", err)
	}
	defer secondResp.Body.Close()
	if secondResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 on duplicate resolve, got %d", secondResp.StatusCode)
	}

	var errResp errorResponse
	body, _ := io.ReadAll(secondResp.Body)
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("expected JSON error response, got %q", string(body))
	}
	if errResp.Error != "APPROVAL_NOT_FOUND" {
		t.Fatalf("expected APPROVAL_NOT_FOUND error, got %q", errResp.Error)
	}
}

func TestResolveApprovalDuplicateRejectedHTTPReturnsNotFound(t *testing.T) {
	approvalReady := make(chan struct{})
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			awaiter, ok := sink.(agentcore.ApprovalAwaiter)
			if !ok {
				return "", errors.New("expected approval awaiter on runtime sink")
			}
			close(approvalReady)
			_, err := awaiter.AwaitApproval(ctx, agentcore.ApprovalRequest{
				ID:          "approval-1",
				ToolCallID:  "tool-1",
				ToolName:    "write_file",
				Title:       "Approve write_file",
				Description: "Write README.md",
				RiskTone:    agentcore.ApprovalRiskDanger,
			})
			if err != nil {
				return "", err
			}
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType})
			return "", nil
		})
	})

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(`{"input":"hello","workspace":{"workspace_root":"/tmp/awaken-test-workspace"}}`))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	<-approvalReady

	resolveURL := ts.URL + "/v1/runs/" + created.RunID + "/approvals/approval-1/resolve"
	firstResp, err := http.Post(resolveURL, "application/json", strings.NewReader(`{"decision":"rejected"}`))
	if err != nil {
		t.Fatalf("POST resolve approval failed: %v", err)
	}
	firstResp.Body.Close()
	if firstResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 on first reject resolve, got %d", firstResp.StatusCode)
	}

	secondResp, err := http.Post(resolveURL, "application/json", strings.NewReader(`{"decision":"rejected"}`))
	if err != nil {
		t.Fatalf("POST duplicate reject resolve approval failed: %v", err)
	}
	defer secondResp.Body.Close()
	if secondResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 on duplicate reject resolve, got %d", secondResp.StatusCode)
	}

	var errResp errorResponse
	body, _ := io.ReadAll(secondResp.Body)
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("expected JSON error response, got %q", string(body))
	}
	if errResp.Error != "APPROVAL_NOT_FOUND" {
		t.Fatalf("expected APPROVAL_NOT_FOUND error, got %q", errResp.Error)
	}
}

// TestPlainQASmoke 通过真实 LLM 调用验证运行时能否完成一次无工具的纯问答 run。
// 需要项目根目录的 .env/config.toml 提供 LLM_API_KEY；未设置时自动跳过。
func TestPlainQASmoke(t *testing.T) {
	workspaceRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}

	// llm.LoadConfig 从当前工作目录读取 .env/config.toml，而 go test ./src/server
	// 的工作目录是 src/server/，因此先切换到项目根目录再加载配置。
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

	runService := awruntime.NewRunService(
		func(workspace awruntime.WorkspaceConfig, threadID string) *agentcore.Agent {
			agentCfg := agentcore.AgentConfig{
				MaxThoughts: 5,
				EntryPoint:  "awaken-core-smoke",
				ModelConfig: agentcore.ModelConfig{
					APIKey:      cfg.APIKey,
					BaseURL:     cfg.BaseURL,
					Model:       cfg.Model,
					Temperature: cfg.Temperature,
					MaxTokens:   cfg.MaxTokens,
					AuthScheme:  cfg.AuthScheme,
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

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	reqBody := fmt.Sprintf(`{
		"input": "请只回复 pong",
		"workspace": { "workspace_root": %q, "writable_roots": [%q] },
		"client": { "session_id": "smoke-1", "request_id": "smoke-req-1", "entrypoint": "smoke" }
	}`, workspaceRoot, workspaceRoot)

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if created.RunID == "" {
		t.Fatalf("expected run id, got empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	eventsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/runs/"+created.RunID+"/events", nil)
	if err != nil {
		t.Fatalf("failed to create event request: %v", err)
	}

	streamResp, err := http.DefaultClient.Do(eventsReq)
	if err != nil {
		t.Fatalf("GET events failed: %v", err)
	}
	defer streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from events, got %d", streamResp.StatusCode)
	}

	events, err := collectSSEEvents(streamResp.Body)
	if err != nil {
		t.Fatalf("failed to collect events: %v", err)
	}

	var sawRunStarted, sawTextMessageContent, sawRunFinished bool
	var assistantText strings.Builder
	var finalOutcome string
	var finalTerminalReason string

	for _, ev := range events {
		switch ev.Type {
		case agui.EventRunStarted:
			sawRunStarted = true
		case agui.EventTextMessageContent:
			sawTextMessageContent = true
			assistantText.WriteString(ev.Delta)
		case agui.EventRunFinished:
			sawRunFinished = true
			if ev.Outcome != nil {
				finalOutcome = ev.Outcome.Type
			}
			finalTerminalReason = ev.TerminalReason
		case agui.EventToolCallStart:
			t.Fatalf("unexpected TOOL_CALL_START event")
		case agui.EventAwakenApprovalRequested:
			t.Fatalf("unexpected AWAKEN_APPROVAL_REQUESTED event")
		case agui.EventAwakenArtifactCreated:
			t.Fatalf("unexpected AWAKEN_ARTIFACT_CREATED event")
		case agui.EventRunError:
			t.Fatalf("run error: %s (code=%s)", ev.Message, ev.Code)
		}
	}

	if !sawRunStarted {
		t.Fatalf("expected RUN_STARTED event")
	}
	if !sawTextMessageContent {
		t.Fatalf("expected TEXT_MESSAGE_CONTENT event")
	}
	if !sawRunFinished {
		t.Fatalf("expected RUN_FINISHED event")
	}
	if strings.TrimSpace(assistantText.String()) != "pong" {
		t.Fatalf("expected assistant text 'pong', got %q", assistantText.String())
	}
	if finalOutcome != "success" {
		t.Fatalf("expected outcome success, got %q", finalOutcome)
	}
	if finalTerminalReason != "completed" {
		t.Fatalf("expected terminal reason completed, got %q", finalTerminalReason)
	}
}

// TestReadOnlyFileSmoke 通过真实 LLM 调用验证运行时能否完成一次只读文件任务。
// 需要项目根目录的 .env/config.toml 提供 LLM_API_KEY；未设置时自动跳过。
func TestReadOnlyFileSmoke(t *testing.T) {
	workspaceRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}

	// llm.LoadConfig 从当前工作目录读取 .env/config.toml，而 go test ./src/server
	// 的工作目录是 src/server/，因此先切换到项目根目录再加载配置。
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

	runService := awruntime.NewRunService(
		func(workspace awruntime.WorkspaceConfig, threadID string) *agentcore.Agent {
			agentCfg := agentcore.AgentConfig{
				MaxThoughts: 10,
				EntryPoint:  "awaken-core-smoke",
				ModelConfig: agentcore.ModelConfig{
					APIKey:      cfg.APIKey,
					BaseURL:     cfg.BaseURL,
					Model:       cfg.Model,
					Temperature: cfg.Temperature,
					MaxTokens:   cfg.MaxTokens,
					AuthScheme:  cfg.AuthScheme,
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

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	reqBody := fmt.Sprintf(`{
		"input": "找出 README 并总结项目",
		"workspace": { "workspace_root": %q, "writable_roots": [%q] },
		"client": { "session_id": "smoke-2", "request_id": "smoke-req-2", "entrypoint": "smoke" }
	}`, workspaceRoot, workspaceRoot)

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if created.RunID == "" {
		t.Fatalf("expected run id, got empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	eventsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/runs/"+created.RunID+"/events", nil)
	if err != nil {
		t.Fatalf("failed to create event request: %v", err)
	}

	streamResp, err := http.DefaultClient.Do(eventsReq)
	if err != nil {
		t.Fatalf("GET events failed: %v", err)
	}
	defer streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from events, got %d", streamResp.StatusCode)
	}

	events, err := collectSSEEvents(streamResp.Body)
	if err != nil {
		t.Fatalf("failed to collect events: %v", err)
	}

	var sawToolCallStart, sawToolCallResult, sawRunFinished bool
	var sawToolPermissionAllow, sawFinalTextAfterTool bool
	var sawReadOnlyTool bool
	var assistantText strings.Builder
	var finalResult string
	var toolResultContents []string
	toolResultIndex := -1

	for i, ev := range events {
		switch ev.Type {
		case agui.EventToolCallStart:
			sawToolCallStart = true
			if isReadOnlyTool(ev.ToolCallName) {
				sawReadOnlyTool = true
			}
		case agui.EventAwakenToolPermissionDecided:
			if isReadOnlyTool(ev.ToolCallName) {
				if ev.ToolDecisionAction != "allow" {
					t.Fatalf("expected read-only tool decision allow, got %q", ev.ToolDecisionAction)
				}
				sawToolPermissionAllow = true
			}
		case agui.EventToolCallResult:
			sawToolCallResult = true
			if toolResultIndex < 0 {
				toolResultIndex = i
			}
			toolResultContents = append(toolResultContents, ev.Content)
		case agui.EventTextMessageContent:
			if toolResultIndex >= 0 && strings.TrimSpace(ev.Delta) != "" {
				sawFinalTextAfterTool = true
			}
			assistantText.WriteString(ev.Delta)
		case agui.EventRunFinished:
			sawRunFinished = true
			finalResult = ev.Result
		case agui.EventAwakenApprovalRequested:
			t.Fatalf("unexpected AWAKEN_APPROVAL_REQUESTED event")
		case agui.EventAwakenArtifactCreated:
			t.Fatalf("unexpected AWAKEN_ARTIFACT_CREATED event")
		case agui.EventAwakenArtifactUpdated:
			t.Fatalf("unexpected AWAKEN_ARTIFACT_UPDATED event")
		case agui.EventRunError:
			t.Fatalf("run error: %s (code=%s)", ev.Message, ev.Code)
		}
	}

	if !sawToolCallStart {
		t.Fatalf("expected TOOL_CALL_START event")
	}
	if !sawToolCallResult {
		t.Fatalf("expected TOOL_CALL_RESULT event")
	}
	if !sawReadOnlyTool {
		t.Fatalf("expected at least one read-only tool call (read_file/list_files/grep_search)")
	}
	if !sawToolPermissionAllow {
		t.Fatalf("expected AWAKEN_TOOL_PERMISSION_DECIDED allow for read-only tool")
	}
	if !sawFinalTextAfterTool {
		t.Fatalf("expected final assistant text after tool_result")
	}
	if !sawRunFinished {
		t.Fatalf("expected RUN_FINISHED event")
	}

	combinedText := strings.ToLower(strings.TrimSpace(assistantText.String()) + " " + strings.TrimSpace(finalResult))
	readmeInResults := false
	for _, content := range toolResultContents {
		if strings.Contains(strings.ToLower(content), "readme") {
			readmeInResults = true
			break
		}
	}
	summaryMatchesProject := strings.Contains(combinedText, "readme") ||
		strings.Contains(combinedText, "项目") ||
		strings.Contains(combinedText, "project") ||
		strings.Contains(combinedText, "awaken")
	if !readmeInResults && !summaryMatchesProject {
		t.Fatalf("expected tool results to mention README or final summary to mention README/project/Awaken, got tool results=%v final=%q", toolResultContents, combinedText)
	}
}

func isReadOnlyTool(name string) bool {
	switch name {
	case "read_file", "list_files", "grep_search":
		return true
	}
	return false
}

// TestFileMutationAutoAllowSmoke 通过真实 LLM 调用验证 apply_patch 在工作区
// 内非敏感路径下默认 AUTO 允许（Task 4 文件链放松），不弹审批，文件被修改。
// 需要项目根目录的 .env/config.toml 提供 LLM_API_KEY；未设置时自动跳过。
func TestFileMutationAutoAllowSmoke(t *testing.T) {
	workspaceRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}

	// llm.LoadConfig 从当前工作目录读取 .env/config.toml，而 go test ./src/server
	// 的工作目录是 src/server/，因此先切换到项目根目录再加载配置。
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

	tempDir := t.TempDir()
	notePath := filepath.Join(tempDir, "note.md")
	if err := os.WriteFile(notePath, []byte("hello"), 0644); err != nil {
		t.Fatalf("failed to create note.md: %v", err)
	}

	runService := awruntime.NewRunService(
		func(workspace awruntime.WorkspaceConfig, threadID string) *agentcore.Agent {
			agentCfg := agentcore.AgentConfig{
				MaxThoughts: 10,
				EntryPoint:  "awaken-core-smoke",
				ModelConfig: agentcore.ModelConfig{
					APIKey:      cfg.APIKey,
					BaseURL:     cfg.BaseURL,
					Model:       cfg.Model,
					Temperature: cfg.Temperature,
					MaxTokens:   cfg.MaxTokens,
					AuthScheme:  cfg.AuthScheme,
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

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	reqBody := fmt.Sprintf(`{
		"input": "请必须调用 apply_patch 修改当前工作区的 note.md，把唯一内容 hello 改成 world。不要使用 shell。修改后只用一句话确认。",
		"workspace": { "workspace_root": %q, "writable_roots": [%q] },
		"client": { "session_id": "smoke-3", "request_id": "smoke-req-3", "entrypoint": "smoke" }
	}`, tempDir, tempDir)

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if created.RunID == "" {
		t.Fatalf("expected run id, got empty")
	}

	events, resolveErr := streamEventsAndResolveApproval(t, ts.URL, created.RunID)
	if resolveErr != nil {
		t.Fatalf("failed to stream and resolve approval: %v", resolveErr)
	}

	var (
		sawRunStarted                bool
		sawApplyPatchToolCallStart   bool
		sawToolPermissionDecided     bool
		sawApprovalRequested         bool
		sawStatusAwaitingApproval    bool
		sawApplyPatchToolCallResult  bool
		sawFinalTextAfterAllowedTool bool
		sawRunFinished               bool
		toolResultContent            string
		finalOutcome                 string
	)

	for _, ev := range events {
		t.Logf("event: type=%s tool=%s status=%s approval=%s paths=%v outcome=%v",
			ev.Type, ev.ToolCallName, ev.Status, ev.ApprovalID, ev.ApprovalAffectedPaths, ev.Outcome)
		switch ev.Type {
		case agui.EventRunStarted:
			sawRunStarted = true
		case agui.EventToolCallStart:
			if ev.ToolCallName == "apply_patch" {
				sawApplyPatchToolCallStart = true
			}
		case agui.EventAwakenToolPermissionDecided:
			if ev.ToolCallName == "apply_patch" {
				sawToolPermissionDecided = true
				if ev.ToolDecisionAction != "allow" {
					t.Fatalf("expected apply_patch tool decision action allow, got %q", ev.ToolDecisionAction)
				}
				if ev.ToolDecisionSource != "workspace_boundary" {
					t.Fatalf("expected apply_patch tool decision source workspace_boundary, got %q", ev.ToolDecisionSource)
				}
				hasNoteMD := false
				for _, p := range ev.ApprovalAffectedPaths {
					if strings.Contains(p, "note.md") {
						hasNoteMD = true
						break
					}
				}
				if !hasNoteMD {
					t.Fatalf("expected tool decision affected paths to contain note.md, got %v", ev.ApprovalAffectedPaths)
				}
			}
		case agui.EventAwakenApprovalRequested:
			sawApprovalRequested = true
		case agui.EventRunStatusChanged:
			if ev.Status == "awaiting_approval" {
				sawStatusAwaitingApproval = true
			}
		case agui.EventToolCallResult:
			if ev.ToolCallName == "apply_patch" {
				sawApplyPatchToolCallResult = true
				toolResultContent = ev.Content
			}
		case agui.EventTextMessageContent:
			if sawApplyPatchToolCallResult && strings.TrimSpace(ev.Delta) != "" {
				sawFinalTextAfterAllowedTool = true
			}
		case agui.EventRunFinished:
			sawRunFinished = true
			if ev.Outcome != nil {
				finalOutcome = ev.Outcome.Type
			}
		case agui.EventRunError:
			t.Fatalf("run error: %s (code=%s)", ev.Message, ev.Code)
		}
	}

	if !sawRunStarted {
		t.Fatalf("expected RUN_STARTED event")
	}
	if !sawApplyPatchToolCallStart {
		t.Fatalf("expected TOOL_CALL_START event for apply_patch")
	}
	if !sawToolPermissionDecided {
		t.Fatalf("expected AWAKEN_TOOL_PERMISSION_DECIDED event for apply_patch")
	}
	if sawApprovalRequested {
		t.Fatalf("did not expect AWAKEN_APPROVAL_REQUESTED event on AUTO allow path")
	}
	if sawStatusAwaitingApproval {
		t.Fatalf("did not expect RUN_STATUS_CHANGED event with awaiting_approval on AUTO allow path")
	}
	if !sawApplyPatchToolCallResult {
		t.Fatalf("expected TOOL_CALL_RESULT event for apply_patch")
	}
	if strings.Contains(toolResultContent, "AUTO policy 拒绝") {
		t.Fatalf("expected AUTO-allowed tool result, got denial: %q", toolResultContent)
	}
	if !sawFinalTextAfterAllowedTool {
		t.Fatalf("expected final assistant text after allowed tool_result")
	}
	if !sawRunFinished {
		t.Fatalf("expected RUN_FINISHED event")
	}
	if finalOutcome != "success" {
		t.Fatalf("expected outcome success, got %q", finalOutcome)
	}

	content, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("failed to read note.md: %v", err)
	}
	if strings.TrimSpace(string(content)) != "world" {
		t.Fatalf("expected note.md to be 'world' after AUTO allow, got %q", string(content))
	}
}

// TestFileMutationAutoAllowDoesNotEnterApprovalSmoke 通过真实 LLM 调用验证
// apply_patch 默认 AUTO 允许（Task 4）时不会进入审批状态或产生 approval resolved。
// 需要项目根目录的 .env/config.toml 提供 LLM_API_KEY；未设置时自动跳过。
func TestFileMutationAutoAllowDoesNotEnterApprovalSmoke(t *testing.T) {
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

	tempDir := t.TempDir()
	notePath := filepath.Join(tempDir, "note.md")
	if err := os.WriteFile(notePath, []byte("hello"), 0644); err != nil {
		t.Fatalf("failed to create note.md: %v", err)
	}

	runService := awruntime.NewRunService(
		func(workspace awruntime.WorkspaceConfig, threadID string) *agentcore.Agent {
			agentCfg := agentcore.AgentConfig{
				MaxThoughts: 10,
				EntryPoint:  "awaken-core-smoke",
				ModelConfig: agentcore.ModelConfig{
					APIKey:      cfg.APIKey,
					BaseURL:     cfg.BaseURL,
					Model:       cfg.Model,
					Temperature: cfg.Temperature,
					MaxTokens:   cfg.MaxTokens,
					AuthScheme:  cfg.AuthScheme,
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

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	reqBody := fmt.Sprintf(`{
		"input": "请必须调用 apply_patch 修改当前工作区的 note.md，把唯一内容 hello 改成 world。不要使用 shell。修改后只用一句话确认。",
		"workspace": { "workspace_root": %q, "writable_roots": [%q] },
		"client": { "session_id": "smoke-reject-1", "request_id": "smoke-req-reject-1", "entrypoint": "smoke" }
	}`, tempDir, tempDir)

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if created.RunID == "" {
		t.Fatalf("expected run id, got empty")
	}

	events, resolveErr := streamEventsAndResolveApprovalDecision(t, ts.URL, created.RunID, "rejected")
	if resolveErr != nil {
		t.Fatalf("failed to stream and resolve approval: %v", resolveErr)
	}

	var (
		sawRunStarted              bool
		sawApplyPatchToolCallStart bool
		sawToolPermissionDecided   bool
		sawApprovalRequested       bool
		sawStatusAwaitingApproval  bool
		sawRunFinished             bool
		toolResultContent          string
		approvalRequestedCount     int
		approvalResolvedCount      int
		finalOutcome               string
	)

	for _, ev := range events {
		t.Logf("event: type=%s tool=%s status=%s approval=%s paths=%v outcome=%v delta=%q",
			ev.Type, ev.ToolCallName, ev.Status, ev.ApprovalID, ev.ApprovalAffectedPaths, ev.Outcome, ev.Delta)
		switch ev.Type {
		case agui.EventRunStarted:
			sawRunStarted = true
		case agui.EventToolCallStart:
			if ev.ToolCallName == "apply_patch" {
				sawApplyPatchToolCallStart = true
			}
		case agui.EventAwakenToolPermissionDecided:
			if ev.ToolCallName == "apply_patch" {
				sawToolPermissionDecided = true
				if ev.ToolDecisionAction != "allow" {
					t.Fatalf("expected apply_patch tool decision action allow, got %q", ev.ToolDecisionAction)
				}
			}
		case agui.EventAwakenApprovalRequested:
			sawApprovalRequested = true
			approvalRequestedCount++
		case agui.EventAwakenApprovalResolved:
			approvalResolvedCount++
		case agui.EventToolCallResult:
			if ev.ToolCallName == "apply_patch" {
				toolResultContent = ev.Content
			}
		case agui.EventRunStatusChanged:
			if ev.Status == "awaiting_approval" {
				sawStatusAwaitingApproval = true
			}
		case agui.EventRunFinished:
			sawRunFinished = true
			if ev.Outcome != nil {
				finalOutcome = ev.Outcome.Type
			}
		case agui.EventRunError:
			t.Fatalf("run error: %s (code=%s)", ev.Message, ev.Code)
		}
	}

	if !sawRunStarted {
		t.Fatalf("expected RUN_STARTED event")
	}
	if !sawApplyPatchToolCallStart {
		t.Fatalf("expected TOOL_CALL_START event for apply_patch")
	}
	if !sawToolPermissionDecided {
		t.Fatalf("expected AWAKEN_TOOL_PERMISSION_DECIDED event for apply_patch")
	}
	if sawApprovalRequested {
		t.Fatalf("did not expect AWAKEN_APPROVAL_REQUESTED event on AUTO allow path")
	}
	if sawStatusAwaitingApproval {
		t.Fatalf("did not expect RUN_STATUS_CHANGED event with awaiting_approval on AUTO allow path")
	}
	if approvalRequestedCount != 0 {
		t.Fatalf("expected zero approval requests on AUTO allow path, got %d", approvalRequestedCount)
	}
	if approvalResolvedCount != 0 {
		t.Fatalf("expected zero approval resolved events on AUTO allow path, got %d", approvalResolvedCount)
	}
	if strings.Contains(toolResultContent, "AUTO policy 拒绝") {
		t.Fatalf("expected AUTO-allowed tool result, got denial: %q", toolResultContent)
	}
	if !sawRunFinished {
		t.Fatalf("expected RUN_FINISHED event")
	}
	if finalOutcome != "success" {
		t.Fatalf("expected outcome success, got %q", finalOutcome)
	}

	content, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("failed to read note.md: %v", err)
	}
	if strings.TrimSpace(string(content)) != "world" {
		t.Fatalf("expected note.md to be 'world' after AUTO allow, got %q", string(content))
	}
}

// TestFileMutationAutoAllowEmitsArtifactSmoke 通过真实 LLM 调用验证 apply_patch
// 默认 AUTO 允许（Task 4）后会产生文件变更 Artifact。
// 需要项目根目录的 .env/config.toml 提供 LLM_API_KEY；未设置时自动跳过。
func TestFileMutationAutoAllowEmitsArtifactSmoke(t *testing.T) {
	workspaceRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}

	// llm.LoadConfig 从当前工作目录读取 .env/config.toml，而 go test ./src/server
	// 的工作目录是 src/server/，因此先切换到项目根目录再加载配置。
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

	tempDir := t.TempDir()
	notePath := filepath.Join(tempDir, "note.md")
	if err := os.WriteFile(notePath, []byte("hello"), 0644); err != nil {
		t.Fatalf("failed to create note.md: %v", err)
	}

	runService := awruntime.NewRunService(
		func(workspace awruntime.WorkspaceConfig, threadID string) *agentcore.Agent {
			agentCfg := agentcore.AgentConfig{
				MaxThoughts: 10,
				EntryPoint:  "awaken-core-smoke",
				ModelConfig: agentcore.ModelConfig{
					APIKey:      cfg.APIKey,
					BaseURL:     cfg.BaseURL,
					Model:       cfg.Model,
					Temperature: cfg.Temperature,
					MaxTokens:   cfg.MaxTokens,
					AuthScheme:  cfg.AuthScheme,
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

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	reqBody := fmt.Sprintf(`{
		"input": "请必须调用 apply_patch 修改当前工作区的 note.md，把唯一内容 hello 改成 world。不要使用 shell。修改后只用一句话确认。",
		"workspace": { "workspace_root": %q, "writable_roots": [%q] },
		"client": { "session_id": "smoke-artifact-1", "request_id": "smoke-req-artifact-1", "entrypoint": "smoke" }
	}`, tempDir, tempDir)

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if created.RunID == "" {
		t.Fatalf("expected run id, got empty")
	}

	events, resolveErr := streamEventsAndResolveApproval(t, ts.URL, created.RunID)
	if resolveErr != nil {
		t.Fatalf("failed to stream and resolve approval: %v", resolveErr)
	}

	var (
		sawRunStarted            bool
		sawRunFinished           bool
		finalOutcome             string
		sawArtifactEvent         bool
		sawToolPermissionDecided bool
		artifactPath             string
		artifactKind             string
		toolResultContent        string
	)

	for _, ev := range events {
		t.Logf("event: type=%s tool=%s artifactKind=%s artifactPath=%s outcome=%v",
			ev.Type, ev.ToolCallName, ev.ArtifactKind, ev.ArtifactPath, ev.Outcome)
		switch ev.Type {
		case agui.EventRunStarted:
			sawRunStarted = true
		case agui.EventAwakenToolPermissionDecided:
			if ev.ToolCallName == "apply_patch" {
				sawToolPermissionDecided = true
				if ev.ToolDecisionAction != "allow" {
					t.Fatalf("expected apply_patch tool decision action allow, got %q", ev.ToolDecisionAction)
				}
				if ev.ToolDecisionSource != "workspace_boundary" {
					t.Fatalf("expected apply_patch tool decision source workspace_boundary, got %q", ev.ToolDecisionSource)
				}
			}
		case agui.EventAwakenArtifactCreated, agui.EventAwakenArtifactUpdated:
			if ev.ToolCallName == "apply_patch" {
				sawArtifactEvent = true
				artifactPath = ev.ArtifactPath
				artifactKind = ev.ArtifactKind
			}
		case agui.EventToolCallResult:
			if ev.ToolCallName == "apply_patch" {
				toolResultContent = ev.Content
			}
		case agui.EventRunFinished:
			sawRunFinished = true
			if ev.Outcome != nil {
				finalOutcome = ev.Outcome.Type
			}
		case agui.EventRunError:
			t.Fatalf("run error: %s (code=%s)", ev.Message, ev.Code)
		}
	}

	if !sawRunStarted {
		t.Fatalf("expected RUN_STARTED event")
	}
	if !sawToolPermissionDecided {
		t.Fatalf("expected AWAKEN_TOOL_PERMISSION_DECIDED event for apply_patch")
	}
	// Task 4: AUTO allow path must emit a file-change artifact (modified note.md).
	if !sawArtifactEvent || artifactPath != "note.md" || artifactKind != "modified" {
		t.Fatalf("expected modified artifact for note.md on AUTO allow path, got saw=%v kind=%q path=%q", sawArtifactEvent, artifactKind, artifactPath)
	}
	if toolResultContent == "" || strings.Contains(toolResultContent, "AUTO policy 拒绝") || strings.Contains(toolResultContent, "工具执行失败") {
		t.Fatalf("expected successful apply_patch tool result on AUTO allow path, got %q", toolResultContent)
	}
	if !sawRunFinished {
		t.Fatalf("expected RUN_FINISHED event")
	}
	if finalOutcome != "success" {
		t.Fatalf("expected outcome success, got %q", finalOutcome)
	}

	content, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("failed to read note.md: %v", err)
	}
	if strings.TrimSpace(string(content)) != "world" {
		t.Fatalf("expected note.md to be 'world' after AUTO allow, got %q", string(content))
	}
}

// TestFileMutationPatchAndTurnDiffSmoke 通过真实 LLM 调用验证 apply_patch
// 默认 AUTO 允许（Task 4）前可见 patch preview；落盘后 turn diff 与 git diff 一致。
// 需要项目根目录的 .env/config.toml 提供 LLM_API_KEY；未设置时自动跳过。
func TestFileMutationPatchAndTurnDiffSmoke(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}

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

	tempDir := t.TempDir()
	runGitForServerTest(t, tempDir, "init")
	runGitForServerTest(t, tempDir, "config", "user.email", "test@example.com")
	runGitForServerTest(t, tempDir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte(".awaken/\n"), 0644); err != nil {
		t.Fatalf("failed to create .gitignore: %v", err)
	}
	notePath := filepath.Join(tempDir, "note.md")
	if err := os.WriteFile(notePath, []byte("hello\n"), 0644); err != nil {
		t.Fatalf("failed to create note.md: %v", err)
	}
	runGitForServerTest(t, tempDir, "add", ".gitignore", "note.md")
	runGitForServerTest(t, tempDir, "commit", "-m", "initial")

	runService := awruntime.NewRunService(
		func(workspace awruntime.WorkspaceConfig, threadID string) *agentcore.Agent {
			agentCfg := agentcore.AgentConfig{
				MaxThoughts: 10,
				EntryPoint:  "awaken-core-smoke",
				ModelConfig: agentcore.ModelConfig{
					APIKey:      cfg.APIKey,
					BaseURL:     cfg.BaseURL,
					Model:       cfg.Model,
					Temperature: cfg.Temperature,
					MaxTokens:   cfg.MaxTokens,
					AuthScheme:  cfg.AuthScheme,
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

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	reqBody := fmt.Sprintf(`{
		"input": "请必须调用 apply_patch 修改当前工作区的 note.md，把唯一内容 hello 改成 world。不要使用 shell。修改后只用一句话确认。",
		"workspace": { "workspace_root": %q, "writable_roots": [%q] },
		"client": { "session_id": "smoke-patch-diff-1", "request_id": "smoke-req-patch-diff-1", "entrypoint": "smoke" }
	}`, tempDir, tempDir)

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if created.RunID == "" {
		t.Fatalf("expected run id, got empty")
	}

	events, resolveErr := streamEventsAndResolveApproval(t, ts.URL, created.RunID)
	if resolveErr != nil {
		t.Fatalf("failed to stream and resolve approval: %v", resolveErr)
	}

	var (
		patchUpdatedByCallID = map[string]int{}
		decisionByCallID     = map[string]int{}
		approvalRequestIndex = -1
		turnDiff             string
		sawRunFinished       bool
		toolResultContent    string
	)

	for i, ev := range events {
		t.Logf("event: type=%s tool=%s approval=%s decision=%s/%s patchSummary=%q diffLen=%d outcome=%v",
			ev.Type, ev.ToolCallName, ev.ApprovalID, ev.ToolDecisionAction, ev.ToolDecisionSource, ev.PatchSummary, len(ev.Diff), ev.Outcome)
		switch ev.Type {
		case agui.EventAwakenPatchUpdated:
			if ev.ToolCallName == "apply_patch" {
				if _, ok := patchUpdatedByCallID[ev.ToolCallID]; !ok {
					patchUpdatedByCallID[ev.ToolCallID] = i
				}
				if !strings.Contains(ev.Patch, "note.md") || !strings.Contains(ev.Patch, "world") {
					t.Fatalf("expected patch preview to contain note.md and world, got %q", ev.Patch)
				}
				if ev.PatchSummary == "" {
					t.Fatalf("expected non-empty patch summary")
				}
				hasNoteMD := false
				for _, p := range ev.ApprovalAffectedPaths {
					if p == "note.md" {
						hasNoteMD = true
						break
					}
				}
				if !hasNoteMD {
					t.Fatalf("expected patch preview affected paths to contain note.md, got %v", ev.ApprovalAffectedPaths)
				}
			}
		case agui.EventAwakenApprovalRequested:
			if ev.ToolCallName == "apply_patch" && approvalRequestIndex < 0 {
				approvalRequestIndex = i
			}
		case agui.EventAwakenToolPermissionDecided:
			if ev.ToolCallName == "apply_patch" {
				if _, ok := decisionByCallID[ev.ToolCallID]; !ok {
					decisionByCallID[ev.ToolCallID] = i
				}
				// Task 4: apply_patch within workspace is auto-allowed via workspace_boundary.
				if ev.ToolDecisionAction != "allow" {
					t.Fatalf("expected apply_patch tool decision action allow, got %q", ev.ToolDecisionAction)
				}
				if ev.ToolDecisionSource != "workspace_boundary" {
					t.Fatalf("expected apply_patch tool decision source workspace_boundary, got %q", ev.ToolDecisionSource)
				}
			}
		case agui.EventAwakenTurnDiff:
			turnDiff = ev.Diff
		case agui.EventToolCallResult:
			if ev.ToolCallName == "apply_patch" {
				toolResultContent = ev.Content
			}
		case agui.EventRunFinished:
			sawRunFinished = true
		case agui.EventRunError:
			t.Fatalf("run error: %s (code=%s)", ev.Message, ev.Code)
		}
	}

	if len(patchUpdatedByCallID) == 0 {
		t.Fatalf("expected AWAKEN_PATCH_UPDATED event")
	}
	if len(decisionByCallID) == 0 {
		t.Fatalf("expected AWAKEN_TOOL_PERMISSION_DECIDED event")
	}
	if approvalRequestIndex >= 0 {
		t.Fatalf("did not expect AWAKEN_APPROVAL_REQUESTED event on AUTO allow path")
	}
	matchedPreviewAndDecision := false
	for callID, patchIndex := range patchUpdatedByCallID {
		decisionIndex, ok := decisionByCallID[callID]
		if !ok {
			continue
		}
		matchedPreviewAndDecision = true
		if patchIndex > decisionIndex {
			t.Fatalf("expected patch preview before matching tool decision for call %s, got patch index %d decision index %d", callID, patchIndex, decisionIndex)
		}
	}
	if !matchedPreviewAndDecision {
		t.Fatalf("expected an apply_patch call to have both patch preview and tool decision, previews=%v decisions=%v", patchUpdatedByCallID, decisionByCallID)
	}
	if toolResultContent == "" || strings.Contains(toolResultContent, "AUTO policy 拒绝") || strings.Contains(toolResultContent, "工具执行失败") {
		t.Fatalf("expected successful apply_patch tool result on AUTO allow path, got %q", toolResultContent)
	}
	if !sawRunFinished {
		t.Fatalf("expected RUN_FINISHED event")
	}
	waitHTTPRunTerminal(t, ts.URL, created.RunID)

	content, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("failed to read note.md: %v", err)
	}
	if strings.TrimSpace(string(content)) != "world" {
		t.Fatalf("expected note.md to be 'world' after AUTO allow, got %q", string(content))
	}
	wantDiff := runGitOutputForServerTest(t, tempDir, "diff", "--")
	if !strings.Contains(wantDiff, "note.md") || !strings.Contains(wantDiff, "world") {
		t.Fatalf("expected git diff to contain note.md and world after AUTO allow, got:\n%s", wantDiff)
	}
	if turnDiff != wantDiff {
		t.Fatalf("expected turn diff to match git diff after AUTO allow\nwant:\n%s\ngot:\n%s", wantDiff, turnDiff)
	}
}

// TestFileMutationAllowlistAllowSmoke 通过真实 LLM 调用验证显式 allowlist 后
// apply_patch 可在 AUTO 主路径成功写入，并保留 patch preview、artifact 和 turn diff。
// 需要项目根目录的 .env/config.toml 提供 LLM_API_KEY；未设置时自动跳过。
func TestFileMutationAllowlistAllowSmoke(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}

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

	tempDir := t.TempDir()
	runGitForServerTest(t, tempDir, "init")
	runGitForServerTest(t, tempDir, "config", "user.email", "test@example.com")
	runGitForServerTest(t, tempDir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte(".awaken/\n"), 0644); err != nil {
		t.Fatalf("failed to create .gitignore: %v", err)
	}
	notePath := filepath.Join(tempDir, "note.md")
	if err := os.WriteFile(notePath, []byte("hello\n"), 0644); err != nil {
		t.Fatalf("failed to create note.md: %v", err)
	}
	runGitForServerTest(t, tempDir, "add", ".gitignore", "note.md")
	runGitForServerTest(t, tempDir, "commit", "-m", "initial")

	runService := awruntime.NewRunService(
		func(workspace awruntime.WorkspaceConfig, threadID string) *agentcore.Agent {
			agentCfg := agentcore.AgentConfig{
				MaxThoughts: 10,
				EntryPoint:  "awaken-core-smoke",
				ModelConfig: agentcore.ModelConfig{
					APIKey:      cfg.APIKey,
					BaseURL:     cfg.BaseURL,
					Model:       cfg.Model,
					Temperature: cfg.Temperature,
					MaxTokens:   cfg.MaxTokens,
					AuthScheme:  cfg.AuthScheme,
				},
				Workspace: agentcore.WorkspaceConfig{
					Root:          workspace.WorkspaceRoot,
					WritableRoots: workspace.WritableRoots,
				},
				AutoToolPolicy: agentcore.DefaultAutoToolPolicy{
					Allowlist: map[string]bool{"apply_patch": true},
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

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	reqBody := fmt.Sprintf(`{
		"input": "请必须调用 apply_patch 修改当前工作区的 note.md，把唯一内容 hello 改成 world。不要使用 shell。修改后只用一句话确认。",
		"workspace": { "workspace_root": %q, "writable_roots": [%q] },
		"client": { "session_id": "smoke-allowlist-1", "request_id": "smoke-req-allowlist-1", "entrypoint": "smoke" }
	}`, tempDir, tempDir)

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if created.RunID == "" {
		t.Fatalf("expected run id, got empty")
	}

	events, resolveErr := streamEventsAndResolveApproval(t, ts.URL, created.RunID)
	if resolveErr != nil {
		t.Fatalf("failed to stream events: %v", resolveErr)
	}

	var (
		patchUpdatedByCallID = map[string]int{}
		decisionByCallID     = map[string]int{}
		approvalRequestIndex = -1
		toolResultContent    string
		turnDiff             string
		artifactPath         string
		artifactKind         string
		sawArtifactEvent     bool
		sawRunFinished       bool
		finalOutcome         string
	)

	for i, ev := range events {
		t.Logf("event: type=%s tool=%s decision=%s/%s artifact=%s/%s diffLen=%d outcome=%v",
			ev.Type, ev.ToolCallName, ev.ToolDecisionAction, ev.ToolDecisionSource, ev.ArtifactKind, ev.ArtifactPath, len(ev.Diff), ev.Outcome)
		switch ev.Type {
		case agui.EventAwakenPatchUpdated:
			if ev.ToolCallName == "apply_patch" {
				if _, ok := patchUpdatedByCallID[ev.ToolCallID]; !ok {
					patchUpdatedByCallID[ev.ToolCallID] = i
				}
				if !strings.Contains(ev.Patch, "note.md") || !strings.Contains(ev.Patch, "world") {
					t.Fatalf("expected patch preview to contain note.md and world, got %q", ev.Patch)
				}
			}
		case agui.EventAwakenToolPermissionDecided:
			if ev.ToolCallName == "apply_patch" {
				if _, ok := decisionByCallID[ev.ToolCallID]; !ok {
					decisionByCallID[ev.ToolCallID] = i
				}
				if ev.ToolDecisionAction != "allow" {
					t.Fatalf("expected apply_patch tool decision action allow, got %q", ev.ToolDecisionAction)
				}
				if ev.ToolDecisionSource != "auto_allowlist" {
					t.Fatalf("expected apply_patch tool decision source auto_allowlist, got %q", ev.ToolDecisionSource)
				}
			}
		case agui.EventAwakenApprovalRequested:
			if ev.ToolCallName == "apply_patch" && approvalRequestIndex < 0 {
				approvalRequestIndex = i
			}
		case agui.EventToolCallResult:
			if ev.ToolCallName == "apply_patch" {
				toolResultContent = ev.Content
			}
		case agui.EventAwakenArtifactCreated, agui.EventAwakenArtifactUpdated:
			if ev.ToolCallName == "apply_patch" {
				sawArtifactEvent = true
				artifactPath = ev.ArtifactPath
				artifactKind = ev.ArtifactKind
			}
		case agui.EventAwakenTurnDiff:
			turnDiff = ev.Diff
		case agui.EventRunFinished:
			sawRunFinished = true
			if ev.Outcome != nil {
				finalOutcome = ev.Outcome.Type
			}
		case agui.EventRunError:
			t.Fatalf("run error: %s (code=%s)", ev.Message, ev.Code)
		}
	}

	if len(patchUpdatedByCallID) == 0 {
		t.Fatalf("expected AWAKEN_PATCH_UPDATED event")
	}
	if len(decisionByCallID) == 0 {
		t.Fatalf("expected AWAKEN_TOOL_PERMISSION_DECIDED event")
	}
	if approvalRequestIndex >= 0 {
		t.Fatalf("did not expect AWAKEN_APPROVAL_REQUESTED event on allowlist AUTO path")
	}
	matchedPreviewAndDecision := false
	for callID, patchIndex := range patchUpdatedByCallID {
		decisionIndex, ok := decisionByCallID[callID]
		if !ok {
			continue
		}
		matchedPreviewAndDecision = true
		if patchIndex > decisionIndex {
			t.Fatalf("expected patch preview before matching tool decision for call %s, got patch index %d decision index %d", callID, patchIndex, decisionIndex)
		}
	}
	if !matchedPreviewAndDecision {
		t.Fatalf("expected an apply_patch call to have both patch preview and tool decision, previews=%v decisions=%v", patchUpdatedByCallID, decisionByCallID)
	}
	if toolResultContent == "" || strings.Contains(toolResultContent, "AUTO policy 拒绝") || strings.Contains(toolResultContent, "工具执行失败") {
		t.Fatalf("expected successful apply_patch tool result, got %q", toolResultContent)
	}
	if !sawArtifactEvent || artifactPath != "note.md" || artifactKind != "modified" {
		t.Fatalf("expected modified artifact for note.md, got saw=%v kind=%q path=%q", sawArtifactEvent, artifactKind, artifactPath)
	}
	if !sawRunFinished {
		t.Fatalf("expected RUN_FINISHED event")
	}
	if finalOutcome != "success" {
		t.Fatalf("expected outcome success, got %q", finalOutcome)
	}
	waitHTTPRunTerminal(t, ts.URL, created.RunID)

	content, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("failed to read note.md: %v", err)
	}
	if strings.TrimSpace(string(content)) != "world" {
		t.Fatalf("expected note.md to contain world after allowlist write, got %q", string(content))
	}
	wantDiff := runGitOutputForServerTest(t, tempDir, "diff", "--")
	if !strings.Contains(wantDiff, "note.md") || !strings.Contains(wantDiff, "world") {
		t.Fatalf("expected git diff to contain note.md and world, got:\n%s", wantDiff)
	}
	if turnDiff != wantDiff {
		t.Fatalf("expected turn diff to match git diff\nwant:\n%s\ngot:\n%s", wantDiff, turnDiff)
	}
}

// TestMultiTurnThreadSmoke 通过真实 LLM 调用验证同一 thread 多轮对话：
// 第一轮让模型记住短语，第二轮在同 thread 中复述短语；并验证未知 thread_id 返回 404。
// 需要项目根目录的 .env/config.toml 提供 LLM_API_KEY；未设置时自动跳过。
func TestMultiTurnThreadSmoke(t *testing.T) {
	workspaceRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}

	// llm.LoadConfig 从当前工作目录读取 .env/config.toml，而 go test ./src/server
	// 的工作目录是 src/server/，因此先切换到项目根目录再加载配置。
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

	tempDir := t.TempDir()
	memoryToken := "awaken-thread-token-0427"

	runService := awruntime.NewRunService(
		func(workspace awruntime.WorkspaceConfig, threadID string) *agentcore.Agent {
			agentCfg := agentcore.AgentConfig{
				MaxThoughts: 10,
				EntryPoint:  "awaken-core-smoke",
				ModelConfig: agentcore.ModelConfig{
					APIKey:      cfg.APIKey,
					BaseURL:     cfg.BaseURL,
					Model:       cfg.Model,
					Temperature: cfg.Temperature,
					MaxTokens:   cfg.MaxTokens,
					AuthScheme:  cfg.AuthScheme,
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

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	// ---------- 第一轮：记住短语（不带 thread_id）----------
	firstReqBody := fmt.Sprintf(`{
		"input": "请记住这个短语：%s。不要调用任何工具，只回答：已记住。",
		"workspace": { "workspace_root": %q, "writable_roots": [%q] },
		"client": { "session_id": "smoke-multiturn-1", "request_id": "smoke-req-multiturn-1", "entrypoint": "smoke" }
	}`, memoryToken, tempDir, tempDir)

	firstResp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(firstReqBody))
	if err != nil {
		t.Fatalf("POST /v1/runs (first) failed: %v", err)
	}
	defer firstResp.Body.Close()
	if firstResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 from first run, got %d", firstResp.StatusCode)
	}

	var firstRun awruntime.Run
	if err := json.NewDecoder(firstResp.Body).Decode(&firstRun); err != nil {
		t.Fatalf("failed to decode first run response: %v", err)
	}
	if firstRun.RunID == "" {
		t.Fatalf("expected run id on first run, got empty")
	}
	if firstRun.ThreadID == "" {
		t.Fatalf("expected thread id on first run, got empty")
	}
	t.Logf("first run: run_id=%s thread_id=%s", firstRun.RunID, firstRun.ThreadID)

	firstEvents, resolveErr := streamEventsAndResolveApproval(t, ts.URL, firstRun.RunID)
	if resolveErr != nil {
		t.Fatalf("failed to stream and resolve approval (first run): %v", resolveErr)
	}

	var firstSawRunFinished bool
	var firstFinalOutcome string
	for _, ev := range firstEvents {
		t.Logf("first run event: type=%s tool=%s status=%s outcome=%v",
			ev.Type, ev.ToolCallName, ev.Status, ev.Outcome)
		switch ev.Type {
		case agui.EventToolCallStart:
			t.Fatalf("did not expect tool call on first memory-only run")
		case agui.EventAwakenApprovalRequested:
			t.Fatalf("did not expect approval request on first memory-only run")
		case agui.EventRunFinished:
			firstSawRunFinished = true
			if ev.Outcome != nil {
				firstFinalOutcome = ev.Outcome.Type
			}
		case agui.EventRunError:
			t.Fatalf("first run error: %s (code=%s)", ev.Message, ev.Code)
		}
	}
	if !firstSawRunFinished {
		t.Fatalf("expected RUN_FINISHED event on first run")
	}
	if firstFinalOutcome != "success" {
		t.Fatalf("expected first run outcome success, got %q", firstFinalOutcome)
	}
	waitHTTPRunTerminal(t, ts.URL, firstRun.RunID)

	// ---------- 第二轮：同 thread_id，复述上一轮短语 ----------
	secondReqBody := fmt.Sprintf(`{
		"input": "不要调用任何工具。请回答上一轮让我记住的短语，原样输出这个短语。",
		"thread_id": %q,
		"workspace": { "workspace_root": %q, "writable_roots": [%q] },
		"client": { "session_id": "smoke-multiturn-2", "request_id": "smoke-req-multiturn-2", "entrypoint": "smoke" }
	}`, firstRun.ThreadID, tempDir, tempDir)

	secondResp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(secondReqBody))
	if err != nil {
		t.Fatalf("POST /v1/runs (second) failed: %v", err)
	}
	defer secondResp.Body.Close()
	if secondResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 from second run, got %d", secondResp.StatusCode)
	}

	var secondRun awruntime.Run
	if err := json.NewDecoder(secondResp.Body).Decode(&secondRun); err != nil {
		t.Fatalf("failed to decode second run response: %v", err)
	}
	if secondRun.RunID == "" {
		t.Fatalf("expected run id on second run, got empty")
	}
	if secondRun.ThreadID != firstRun.ThreadID {
		t.Fatalf("expected second run to reuse thread_id %q, got %q", firstRun.ThreadID, secondRun.ThreadID)
	}
	if secondRun.RunID == firstRun.RunID {
		t.Fatalf("expected second run to have a new run_id, got duplicate %q", secondRun.RunID)
	}
	t.Logf("second run: run_id=%s thread_id=%s", secondRun.RunID, secondRun.ThreadID)

	secondEvents, resolveErr := streamEventsAndResolveApproval(t, ts.URL, secondRun.RunID)
	if resolveErr != nil {
		t.Fatalf("failed to stream and resolve approval (second run): %v", resolveErr)
	}

	var secondSawRunFinished bool
	var secondFinalOutcome string
	var secondAssistantText strings.Builder
	for _, ev := range secondEvents {
		t.Logf("second run event: type=%s tool=%s status=%s outcome=%v",
			ev.Type, ev.ToolCallName, ev.Status, ev.Outcome)
		switch ev.Type {
		case agui.EventToolCallStart:
			t.Fatalf("did not expect tool call on second memory-only run")
		case agui.EventAwakenApprovalRequested:
			t.Fatalf("did not expect approval request on second memory-only run")
		case agui.EventTextMessageContent:
			secondAssistantText.WriteString(ev.Delta)
		case agui.EventRunFinished:
			secondSawRunFinished = true
			if ev.Outcome != nil {
				secondFinalOutcome = ev.Outcome.Type
			}
		case agui.EventRunError:
			t.Fatalf("second run error: %s (code=%s)", ev.Message, ev.Code)
		}
	}
	if !secondSawRunFinished {
		t.Fatalf("expected RUN_FINISHED event on second run")
	}
	if secondFinalOutcome != "success" {
		t.Fatalf("expected second run outcome success, got %q", secondFinalOutcome)
	}
	if !strings.Contains(secondAssistantText.String(), memoryToken) {
		t.Fatalf("expected second run to recall %q from thread context, got %q", memoryToken, secondAssistantText.String())
	}
	waitHTTPRunTerminal(t, ts.URL, secondRun.RunID)

	// ---------- 未知 thread_id 测试 ----------
	unknownReqBody := fmt.Sprintf(`{
		"input": "hello",
		"thread_id": "nonexistent-thread",
		"workspace": { "workspace_root": %q }
	}`, tempDir)
	unknownResp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(unknownReqBody))
	if err != nil {
		t.Fatalf("POST /v1/runs (unknown thread) failed: %v", err)
	}
	defer unknownResp.Body.Close()
	if unknownResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown thread_id, got %d", unknownResp.StatusCode)
	}
	var errResp errorResponse
	if err := json.NewDecoder(unknownResp.Body).Decode(&errResp); err != nil {
		t.Fatalf("expected JSON error response for unknown thread, got decode error: %v", err)
	}
	if errResp.Error != "THREAD_NOT_FOUND" {
		t.Fatalf("expected THREAD_NOT_FOUND error code, got %q", errResp.Error)
	}
}

func streamEventsAndResolveApproval(t *testing.T, baseURL, runID string) ([]agui.Event, error) {
	return streamEventsAndResolveApprovalDecision(t, baseURL, runID, "approved")
}

func streamEventsAndResolveApprovalDecision(t *testing.T, baseURL, runID, decision string) ([]agui.Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	eventsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/runs/"+runID+"/events", nil)
	if err != nil {
		return nil, err
	}

	streamResp, err := http.DefaultClient.Do(eventsReq)
	if err != nil {
		return nil, err
	}
	defer streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("expected 200 from events, got %d", streamResp.StatusCode)
	}

	var events []agui.Event
	var resolveErr error
	var resolveMu sync.Mutex
	resolvedApprovals := make(map[string]bool)

	scanner := bufio.NewScanner(streamResp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "" {
			continue
		}
		var ev agui.Event
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			return nil, fmt.Errorf("decode event %q: %w", payload, err)
		}
		events = append(events, ev)

		if ev.Type == agui.EventAwakenApprovalRequested {
			resolveMu.Lock()
			alreadyResolved := resolvedApprovals[ev.ApprovalID]
			if !alreadyResolved {
				resolvedApprovals[ev.ApprovalID] = true
			}
			resolveMu.Unlock()
			if !alreadyResolved {
				go func(approvalID string) {
					err := resolveApprovalWithRetryDecision(baseURL, runID, approvalID, decision)
					resolveMu.Lock()
					if resolveErr == nil && err != nil {
						resolveErr = err
					}
					resolveMu.Unlock()
				}(ev.ApprovalID)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if resolveErr != nil {
		return nil, resolveErr
	}
	return events, nil
}

func resolveApprovalWithRetry(baseURL, runID, approvalID string) error {
	return resolveApprovalWithRetryDecision(baseURL, runID, approvalID, "approved")
}

func resolveApprovalWithRetryDecision(baseURL, runID, approvalID, decision string) error {
	resolveURL := baseURL + "/v1/runs/" + runID + "/approvals/" + approvalID + "/resolve"
	body := fmt.Sprintf(`{"decision":"%s"}`, decision)
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Post(resolveURL, "application/json", strings.NewReader(body))
		if err != nil {
			return fmt.Errorf("POST resolve approval failed: %w", err)
		}
		status := resp.StatusCode
		_ = resp.Body.Close()
		if status == http.StatusNoContent {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("approval resolve never succeeded, last status %d", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func collectSSEEvents(r io.Reader) ([]agui.Event, error) {
	var events []agui.Event
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "" {
			continue
		}
		var ev agui.Event
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			return nil, fmt.Errorf("decode event %q: %w", payload, err)
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func runGitForServerTest(t *testing.T, root string, args ...string) {
	t.Helper()
	_ = runGitOutputForServerTest(t, root, args...)
}

func runGitOutputForServerTest(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
	return string(output)
}

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found")
		}
		dir = parent
	}
}

// TestModelStatusSmoke 通过真实 LLM 调用验证 /v1/runtime/info 返回当前 configured model
// 和 protocol，且不泄漏 API key；同时发起一次真实 run 验证运行时可用。
// 需要项目根目录的 .env/config.toml 提供 LLM_API_KEY；未设置时自动跳过。
func TestModelStatusSmoke(t *testing.T) {
	workspaceRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}

	// llm.LoadConfig 从当前工作目录读取 .env/config.toml，而 go test ./src/server
	// 的工作目录是 src/server/，因此先切换到项目根目录再加载配置。
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

	provider := NewLLMRuntimeInfoProvider(cfg.Model, cfg.BaseURL, cfg.APIKey, workspaceRoot, string(llm.ResolveProtocol(cfg)))

	runService := awruntime.NewRunService(
		func(workspace awruntime.WorkspaceConfig, threadID string) *agentcore.Agent {
			agentCfg := agentcore.AgentConfig{
				MaxThoughts: 5,
				EntryPoint:  "awaken-core-smoke",
				ModelConfig: agentcore.ModelConfig{
					APIKey:      cfg.APIKey,
					BaseURL:     cfg.BaseURL,
					Model:       cfg.Model,
					Temperature: cfg.Temperature,
					MaxTokens:   cfg.MaxTokens,
					AuthScheme:  cfg.AuthScheme,
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

	ts := httptest.NewServer(NewRouter(runService, provider))
	defer ts.Close()

	// 1. 验证 /v1/runtime/info 返回正确的 model 和 protocol，且不泄漏 secret。
	infoResp, err := http.Get(ts.URL + "/v1/runtime/info")
	if err != nil {
		t.Fatalf("GET /v1/runtime/info failed: %v", err)
	}
	defer infoResp.Body.Close()
	if infoResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from runtime info, got %d", infoResp.StatusCode)
	}

	infoBody, err := io.ReadAll(infoResp.Body)
	if err != nil {
		t.Fatalf("failed to read runtime info body: %v", err)
	}
	if strings.Contains(string(infoBody), cfg.APIKey) {
		t.Fatalf("runtime info response must not contain api key")
	}

	var info RuntimeInfo
	if err := json.Unmarshal(infoBody, &info); err != nil {
		t.Fatalf("failed to decode runtime info: %v", err)
	}
	if info.ConfiguredModel == "" {
		t.Fatalf("expected non-empty configured_model")
	}
	if info.ConfiguredModel != cfg.Model {
		t.Fatalf("expected configured_model %q, got %q", cfg.Model, info.ConfiguredModel)
	}
	if info.Protocol != "openai_chat" && info.Protocol != "anthropic_messages" {
		t.Fatalf("expected protocol openai_chat or anthropic_messages, got %q", info.Protocol)
	}
	if !info.HasAPIKey {
		t.Fatalf("expected has_api_key true")
	}

	// 2. 发起一次真实 run，验证运行时可用。
	reqBody := fmt.Sprintf(`{
		"input": "请只回复 pong",
		"workspace": { "workspace_root": %q, "writable_roots": [%q] },
		"client": { "session_id": "smoke-status-1", "request_id": "smoke-req-status-1", "entrypoint": "smoke" }
	}`, workspaceRoot, workspaceRoot)

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if created.RunID == "" {
		t.Fatalf("expected run id, got empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	eventsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/runs/"+created.RunID+"/events", nil)
	if err != nil {
		t.Fatalf("failed to create event request: %v", err)
	}

	streamResp, err := http.DefaultClient.Do(eventsReq)
	if err != nil {
		t.Fatalf("GET events failed: %v", err)
	}
	defer streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from events, got %d", streamResp.StatusCode)
	}

	events, err := collectSSEEvents(streamResp.Body)
	if err != nil {
		t.Fatalf("failed to collect events: %v", err)
	}

	var sawRunFinished bool
	for _, ev := range events {
		switch ev.Type {
		case agui.EventRunFinished:
			sawRunFinished = true
		case agui.EventRunError:
			t.Fatalf("unexpected run error: %s (code=%s)", ev.Message, ev.Code)
		}
	}
	if !sawRunFinished {
		t.Fatalf("expected RUN_FINISHED event")
	}
}

// TestRunWithoutAPIKeyEmitsProviderAuthMissing 验证缺少 API key 时 RunError 事件
// code=PROVIDER_AUTH_MISSING。此测试不依赖真实 LLM，始终运行。
func TestRunWithoutAPIKeyEmitsProviderAuthMissing(t *testing.T) {
	runService := awruntime.NewRunService(
		func(workspace awruntime.WorkspaceConfig, threadID string) *agentcore.Agent {
			agentCfg := agentcore.AgentConfig{
				MaxThoughts: 5,
				EntryPoint:  "awaken-core-smoke",
				ModelConfig: agentcore.ModelConfig{
					APIKey:  "",
					BaseURL: "https://api.anthropic.com/v1",
					Model:   "test-model",
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

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	workspaceRoot := t.TempDir()
	reqBody := fmt.Sprintf(`{
		"input": "hello",
		"workspace": { "workspace_root": %q, "writable_roots": [%q] },
		"client": { "session_id": "smoke-nokey-1", "request_id": "smoke-req-nokey-1", "entrypoint": "smoke" }
	}`, workspaceRoot, workspaceRoot)

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if created.RunID == "" {
		t.Fatalf("expected run id, got empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	eventsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/runs/"+created.RunID+"/events", nil)
	if err != nil {
		t.Fatalf("failed to create event request: %v", err)
	}

	streamResp, err := http.DefaultClient.Do(eventsReq)
	if err != nil {
		t.Fatalf("GET events failed: %v", err)
	}
	defer streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from events, got %d", streamResp.StatusCode)
	}

	events, err := collectSSEEvents(streamResp.Body)
	if err != nil {
		t.Fatalf("failed to collect events: %v", err)
	}

	var sawRunError, sawRunFinished bool
	var errorCode, errorMessage string
	for _, ev := range events {
		switch ev.Type {
		case agui.EventRunError:
			sawRunError = true
			errorCode = ev.Code
			errorMessage = ev.Message
		case agui.EventRunFinished:
			sawRunFinished = true
		}
	}
	if !sawRunError {
		t.Fatalf("expected RUN_ERROR event, got events: %v", events)
	}
	if errorCode != "PROVIDER_AUTH_MISSING" {
		t.Fatalf("expected error code PROVIDER_AUTH_MISSING, got %q", errorCode)
	}
	if strings.Contains(errorMessage, "sk-") {
		t.Fatalf("error message must not contain api key fragment: %q", errorMessage)
	}
	if sawRunFinished {
		t.Fatalf("did not expect RUN_FINISHED event")
	}
}

// TestRunWithBadBaseURLEmitsProviderRequestFailed 验证 provider 请求失败时
// RunError 事件 code=PROVIDER_REQUEST_FAILED。此测试不依赖真实 LLM，始终运行。
func TestRunWithBadBaseURLEmitsProviderRequestFailed(t *testing.T) {
	runService := awruntime.NewRunService(
		func(workspace awruntime.WorkspaceConfig, threadID string) *agentcore.Agent {
			agentCfg := agentcore.AgentConfig{
				MaxThoughts: 5,
				EntryPoint:  "awaken-core-smoke",
				ModelConfig: agentcore.ModelConfig{
					APIKey:  "dummy-key-not-real",
					BaseURL: "http://127.0.0.1:1/invalid",
					Model:   "test-model",
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

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	workspaceRoot := t.TempDir()
	reqBody := fmt.Sprintf(`{
		"input": "hello",
		"workspace": { "workspace_root": %q, "writable_roots": [%q] },
		"client": { "session_id": "smoke-badurl-1", "request_id": "smoke-req-badurl-1", "entrypoint": "smoke" }
	}`, workspaceRoot, workspaceRoot)

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if created.RunID == "" {
		t.Fatalf("expected run id, got empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	eventsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/runs/"+created.RunID+"/events", nil)
	if err != nil {
		t.Fatalf("failed to create event request: %v", err)
	}

	streamResp, err := http.DefaultClient.Do(eventsReq)
	if err != nil {
		t.Fatalf("GET events failed: %v", err)
	}
	defer streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from events, got %d", streamResp.StatusCode)
	}

	events, err := collectSSEEvents(streamResp.Body)
	if err != nil {
		t.Fatalf("failed to collect events: %v", err)
	}

	var sawRunError, sawRunFinished bool
	var errorCode, errorMessage string
	for _, ev := range events {
		switch ev.Type {
		case agui.EventRunError:
			sawRunError = true
			errorCode = ev.Code
			errorMessage = ev.Message
		case agui.EventRunFinished:
			sawRunFinished = true
		}
	}
	if !sawRunError {
		t.Fatalf("expected RUN_ERROR event, got events: %v", events)
	}
	if errorCode != "PROVIDER_REQUEST_FAILED" {
		t.Fatalf("expected error code PROVIDER_REQUEST_FAILED, got %q", errorCode)
	}
	if strings.Contains(errorMessage, "dummy-key-not-real") {
		t.Fatalf("error message must not contain api key: %q", errorMessage)
	}
	if strings.Contains(errorMessage, "127.0.0.1:1") {
		t.Fatalf("error message must not contain full URL host:port: %q", errorMessage)
	}
	if strings.Contains(errorMessage, "/invalid") {
		t.Fatalf("error message must not contain full URL path: %q", errorMessage)
	}
	if sawRunFinished {
		t.Fatalf("did not expect RUN_FINISHED event")
	}
}

// TestRunWithProviderBodySecretEmitsSanitizedMessage 验证 provider 返回非 2xx 且 body
// 含敏感信息时，RunError 事件 message 不泄漏 body 内容，仅保留 status code。
func TestRunWithProviderBodySecretEmitsSanitizedMessage(t *testing.T) {
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"sk-test-secret is invalid","detail":"internal path /etc/secrets"}`))
	}))
	defer providerServer.Close()

	runService := awruntime.NewRunService(
		func(workspace awruntime.WorkspaceConfig, threadID string) *agentcore.Agent {
			agentCfg := agentcore.AgentConfig{
				MaxThoughts: 5,
				EntryPoint:  "awaken-core-smoke",
				ModelConfig: agentcore.ModelConfig{
					APIKey:  "dummy-key-not-real",
					BaseURL: providerServer.URL,
					Model:   "test-model",
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

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	workspaceRoot := t.TempDir()
	reqBody := fmt.Sprintf(`{
		"input": "hello",
		"workspace": { "workspace_root": %q, "writable_roots": [%q] },
		"client": { "session_id": "smoke-body-1", "request_id": "smoke-req-body-1", "entrypoint": "smoke" }
	}`, workspaceRoot, workspaceRoot)

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if created.RunID == "" {
		t.Fatalf("expected run id, got empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	eventsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/runs/"+created.RunID+"/events", nil)
	if err != nil {
		t.Fatalf("failed to create event request: %v", err)
	}

	streamResp, err := http.DefaultClient.Do(eventsReq)
	if err != nil {
		t.Fatalf("GET events failed: %v", err)
	}
	defer streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from events, got %d", streamResp.StatusCode)
	}

	events, err := collectSSEEvents(streamResp.Body)
	if err != nil {
		t.Fatalf("failed to collect events: %v", err)
	}

	var sawRunError bool
	var errorCode, errorMessage string
	for _, ev := range events {
		switch ev.Type {
		case agui.EventRunError:
			sawRunError = true
			errorCode = ev.Code
			errorMessage = ev.Message
		}
	}
	if !sawRunError {
		t.Fatalf("expected RUN_ERROR event, got events: %v", events)
	}
	if errorCode != "PROVIDER_REQUEST_FAILED" {
		t.Fatalf("expected error code PROVIDER_REQUEST_FAILED, got %q", errorCode)
	}
	if strings.Contains(errorMessage, "sk-test-secret") {
		t.Fatalf("error message must not contain response body secret: %q", errorMessage)
	}
	if strings.Contains(errorMessage, "/etc/secrets") {
		t.Fatalf("error message must not contain response body detail: %q", errorMessage)
	}
	if !strings.Contains(errorMessage, "status 502") {
		t.Fatalf("error message should contain status code, got %q", errorMessage)
	}
}

// TestMCPToolCallSmoke 通过真实 LLM 调用验证 agent 能通过 MCP runtime 调用一个
// in-proc echo server 暴露的工具。需要 LLM_API_KEY；未设置时自动跳过。
//
// 流程：注册一个 in-proc MCP server（echo_smoke）暴露 echo 工具 → 启动 manager →
// 通过 NewRouterWithMCP 注入 → POST /v1/runs 让 agent 调用 mcp__echo_smoke__echo
// → 校验 TOOL_CALL_START + TOOL_CALL_RESULT + RUN_FINISHED 事件。
func TestMCPToolCallSmoke(t *testing.T) {
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

	// 注册一个 in-proc MCP server，暴露一个 echo 工具。
	registry := mcpclient.NewInProcRegistry()
	echoFactory := mcpclient.InProcServerFactory(func() *mcp.Server {
		srv := mcp.NewServer(&mcp.Implementation{Name: "echo-smoke", Version: "v1"}, nil)
		mcp.AddTool(srv, &mcp.Tool{
			Name:        "echo",
			Description: "Echoes back the provided message. Use this when the user asks you to echo something. Pass the message verbatim.",
			Annotations: &mcp.ToolAnnotations{
				ReadOnlyHint:    true,
				DestructiveHint: boolPtr(false),
				OpenWorldHint:   boolPtr(false),
			},
		}, func(ctx context.Context, req *mcp.CallToolRequest, input map[string]string) (*mcp.CallToolResult, map[string]string, error) {
			msg := input["message"]
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "echo:" + msg}},
			}, input, nil
		})
		return srv
	})
	if err := registry.Register("echo_smoke", echoFactory); err != nil {
		t.Fatalf("register in-proc: %v", err)
	}

	manager := mcpclient.NewManager(
		mcpclient.WithInProcRegistry(registry),
		mcpclient.WithLogger(mcpclient.StdLogger{}),
	)
	// 在启动 manager 前先反注册可能残留的同名 MCP 工具，避免上一轮测试污染。
	mcpclient.UnregisterMcpToolsForTest("echo_smoke")
	t.Cleanup(func() {
		mcpclient.UnregisterMcpToolsForTest("echo_smoke")
		manager.Shutdown(context.Background())
	})

	startCtx, startCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer startCancel()
	manager.Start(startCtx, []mcpclient.ScopedMcpServerConfig{
		{Name: "echo_smoke", Scope: mcpclient.ScopeProject, Config: mcpclient.McpServerConfig{
			InProc: &mcpclient.InProcTransportConfig{ServerName: "echo_smoke"},
		}},
	})
	// 等待 server 进入 connected 状态。
	if err := mcpclient.WaitForServerStateForTest(manager, "echo_smoke", mcpclient.StateConnected, 5*time.Second); err != nil {
		snap, _ := manager.SnapshotServer("echo_smoke")
		t.Fatalf("echo_smoke never connected: %v (state=%s reason=%q)", err, snap.State, snap.ErrorReason)
	}

	runService := awruntime.NewRunService(
		func(workspace awruntime.WorkspaceConfig, threadID string) *agentcore.Agent {
			agentCfg := agentcore.AgentConfig{
				MaxThoughts: 5,
				EntryPoint:  "awaken-core-smoke",
				ModelConfig: agentcore.ModelConfig{
					APIKey:      cfg.APIKey,
					BaseURL:     cfg.BaseURL,
					Model:       cfg.Model,
					Temperature: cfg.Temperature,
					MaxTokens:   cfg.MaxTokens,
					AuthScheme:  cfg.AuthScheme,
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

	ts := httptest.NewServer(NewRouterWithMCP(runService, nil, manager))
	defer ts.Close()

	reqBody := fmt.Sprintf(`{
		"input": "请调用 echo 工具，参数 message 为 hello-mcp。不要回复其他内容。",
		"workspace": { "workspace_root": %q, "writable_roots": [%q] },
		"client": { "session_id": "smoke-mcp-1", "request_id": "smoke-req-mcp-1", "entrypoint": "smoke" }
	}`, workspaceRoot, workspaceRoot)

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /v1/runs failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created awruntime.Run
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if created.RunID == "" {
		t.Fatalf("expected run id, got empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	eventsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/runs/"+created.RunID+"/events", nil)
	if err != nil {
		t.Fatalf("failed to create event request: %v", err)
	}

	streamResp, err := http.DefaultClient.Do(eventsReq)
	if err != nil {
		t.Fatalf("GET events failed: %v", err)
	}
	defer streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from events, got %d", streamResp.StatusCode)
	}

	events, err := collectSSEEvents(streamResp.Body)
	if err != nil {
		t.Fatalf("failed to collect events: %v", err)
	}

	var sawToolCallStart, sawToolCallResult, sawRunFinished bool
	var sawMcpEchoTool bool
	var toolResultContents []string
	var finalOutcome string

	for _, ev := range events {
		switch ev.Type {
		case agui.EventToolCallStart:
			sawToolCallStart = true
			if ev.ToolCallName == "mcp__echo_smoke__echo" {
				sawMcpEchoTool = true
			}
		case agui.EventToolCallResult:
			sawToolCallResult = true
			toolResultContents = append(toolResultContents, ev.Content)
		case agui.EventRunFinished:
			sawRunFinished = true
			if ev.Outcome != nil {
				finalOutcome = ev.Outcome.Type
			}
		case agui.EventRunError:
			t.Fatalf("run error: %s (code=%s)", ev.Message, ev.Code)
		}
	}

	if !sawToolCallStart {
		t.Fatalf("expected TOOL_CALL_START event")
	}
	if !sawMcpEchoTool {
		t.Fatalf("expected agent to call mcp__echo_smoke__echo")
	}
	if !sawToolCallResult {
		t.Fatalf("expected TOOL_CALL_RESULT event")
	}
	if !sawRunFinished {
		t.Fatalf("expected RUN_FINISHED event")
	}
	if finalOutcome != "success" {
		t.Fatalf("expected outcome success, got %q", finalOutcome)
	}
	echoedInResults := false
	for _, content := range toolResultContents {
		if strings.Contains(content, "echo:hello-mcp") {
			echoedInResults = true
			break
		}
	}
	if !echoedInResults {
		t.Fatalf("expected tool results to contain 'echo:hello-mcp', got %v", toolResultContents)
	}
}

// TestMCPListToolsWithoutManager verifies GET /v1/mcp/tools returns builtin
// tools even when no MCP manager is configured (the endpoint is registered
// unconditionally).
func TestMCPListToolsWithoutManager(t *testing.T) {
	router := NewRouter(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/mcp/tools", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var tools []mcpToolInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &tools); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected at least one builtin tool")
	}
	for _, tool := range tools {
		if tool.Source != "builtin" {
			t.Errorf("expected builtin source for tool %q, got %s", tool.Name, tool.Source)
		}
	}
}

func TestMCPConfigureServersAcceptsShorthandAndPersists(t *testing.T) {
	workspace := t.TempDir()
	// Redirect user-level MCP config into the temporary workspace so tests
	// do not write to the real home directory.
	t.Setenv("HOME", workspace)
	t.Setenv("USERPROFILE", workspace)
	mgr := mcpclient.NewManager(mcpclient.WithLogger(testLogger{}))
	router := NewRouterWithWorkspace(nil, nil, mgr, nil, nil, workspace)

	configText := `{"mcpServers":{"example-server":{"command":"awaken-missing-mcp-test-command","args":["--version"]}}}`
	body, err := json.Marshal(configureMCPServersRequest{ConfigText: configText})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/mcp/servers", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp configureMCPServersResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.ServerNames) != 1 || resp.ServerNames[0] != "example_server" {
		t.Fatalf("expected normalized example_server, got %#v", resp.ServerNames)
	}
	userCfg, err := mcpclient.LoadUserConfig()
	if err != nil {
		t.Fatalf("load user config: %v", err)
	}
	stored := userCfg.Servers["example_server"]
	if stored.Stdio == nil || stored.Stdio.Command != "awaken-missing-mcp-test-command" {
		t.Fatalf("expected persisted stdio config, got %+v", stored)
	}
	if _, err := mgr.SnapshotServer("example_server"); err != nil {
		t.Fatalf("expected manager snapshot for configured server: %v", err)
	}
}

func TestInstallSkillWritesProjectSkillAndRegisters(t *testing.T) {
	workspace := t.TempDir()
	registry := skills.NewSkillRegistry(testLogger{})
	router := NewRouterWithWorkspace(nil, nil, nil, nil, registry, workspace)

	rawSkill := `---
name: daily-followup
description: Generate a daily follow-up plan.
whenToUse: Use when a user asks for daily customer follow-up.
context: inline
---

Create a short follow-up checklist.`
	body, err := json.Marshal(installSkillRequest{
		FileName: "SKILL.md",
		Content:  rawSkill,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/skills/install", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp installSkillResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Skill.ID != "daily-followup" || resp.Skill.SourceOrigin != "project" {
		t.Fatalf("unexpected installed skill response: %+v", resp.Skill)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".awaken", "skills", "daily-followup.md")); err != nil {
		t.Fatalf("expected installed skill file: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var listed []skillInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "daily-followup" {
		t.Fatalf("expected installed skill in list, got %+v", listed)
	}
}

// TestMCPAuthFetcherNotInstalledReturns409 verifies POST /v1/mcp/servers/{name}/auth
// returns 409 Conflict when the manager has no auth fetcher installed.
func TestMCPAuthFetcherNotInstalledReturns409(t *testing.T) {
	registry := mcpclient.NewInProcRegistry()
	if err := registry.Register("echo", func() *mcp.Server {
		server := mcp.NewServer(&mcp.Implementation{Name: "echo", Version: "v1"}, nil)
		return server
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	mgr := mcpclient.NewManager(
		mcpclient.WithInProcRegistry(registry),
		mcpclient.WithLogger(testLogger{}),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr.Start(ctx, []mcpclient.ScopedMcpServerConfig{
		{Name: "echo", Scope: mcpclient.ScopeProject, Config: mcpclient.McpServerConfig{
			InProc: &mcpclient.InProcTransportConfig{ServerName: "echo"},
		}},
	})
	router := NewRouterWithMCP(nil, nil, mgr)

	req := httptest.NewRequest(http.MethodPost, "/v1/mcp/servers/echo/auth", strings.NewReader(`{"redirectUrl":"http://localhost:0/callback"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 AUTH_FETCHER_REQUIRED, got %d: %s", rec.Code, rec.Body.String())
	}
	var errResp errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("expected JSON error response: %v", err)
	}
	if errResp.Error != "AUTH_FETCHER_REQUIRED" {
		t.Errorf("expected AUTH_FETCHER_REQUIRED error code, got %q", errResp.Error)
	}
}

// TestMCPEnableIllegalStateReturnsConflict verifies POST /v1/mcp/servers/{name}/enable
// returns 409 Conflict when the server is not in a state that allows enable.
func TestMCPEnableIllegalStateReturnsConflict(t *testing.T) {
	registry := mcpclient.NewInProcRegistry()
	if err := registry.Register("echo", echoInProcServerFactory()); err != nil {
		t.Fatalf("register: %v", err)
	}
	mgr := mcpclient.NewManager(
		mcpclient.WithInProcRegistry(registry),
		mcpclient.WithLogger(testLogger{}),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr.Start(ctx, []mcpclient.ScopedMcpServerConfig{
		{Name: "echo", Scope: mcpclient.ScopeProject, Config: mcpclient.McpServerConfig{
			InProc: &mcpclient.InProcTransportConfig{ServerName: "echo"},
		}},
	})
	// Wait for the server to connect.
	deadline := time.Now().Add(3 * time.Second)
	var connected bool
	for time.Now().Before(deadline) {
		snap, err := mgr.SnapshotServer("echo")
		if err == nil && snap.State == mcpclient.StateConnected {
			connected = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !connected {
		t.Fatal("echo server never connected")
	}
	router := NewRouterWithMCP(nil, nil, mgr)

	req := httptest.NewRequest(http.MethodPost, "/v1/mcp/servers/echo/enable", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 ILLEGAL_STATE, got %d: %s", rec.Code, rec.Body.String())
	}
	var errResp errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("expected JSON error response: %v", err)
	}
	if errResp.Error != "ILLEGAL_STATE" {
		t.Errorf("expected ILLEGAL_STATE error code, got %q", errResp.Error)
	}
}

// testLogger implements mcpclient.Logger for tests.
type testLogger struct{}

func (testLogger) Debugf(string, ...any) {}
func (testLogger) Warnf(string, ...any)  {}
func (testLogger) Errorf(string, ...any) {}

// echoInProcServerFactory returns an InProcServerFactory for an echo server with one tool.
func echoInProcServerFactory() mcpclient.InProcServerFactory {
	return func() *mcp.Server {
		server := mcp.NewServer(&mcp.Implementation{Name: "echo", Version: "v1.0.0"}, nil)
		mcp.AddTool(server, &mcp.Tool{
			Name:        "echo",
			Description: "echoes input",
			Annotations: &mcp.ToolAnnotations{
				ReadOnlyHint:    true,
				DestructiveHint: boolPtr(false),
				OpenWorldHint:   boolPtr(false),
			},
		}, func(ctx context.Context, req *mcp.CallToolRequest, input map[string]string) (*mcp.CallToolResult, map[string]string, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: input["message"]}},
			}, input, nil
		})
		return server
	}
}

func boolPtr(b bool) *bool { return &b }
