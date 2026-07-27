package server

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"awaken/src/agentcore"
	"awaken/src/desktopdata"
	"awaken/src/filestore"
	"awaken/src/llm"
	"awaken/src/mcpclient"
	"awaken/src/protocol/agui"
	awruntime "awaken/src/runtime"
	"awaken/src/skills"
	"awaken/src/tool/trash"
	_ "awaken/src/tools"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type serverSkillTrashAdapter struct {
	dir string
}

func (a serverSkillTrashAdapter) TrashPath(source string) (trash.TrashResult, error) {
	if err := os.MkdirAll(a.dir, 0o700); err != nil {
		return trash.TrashResult{}, err
	}
	destination := filepath.Join(a.dir, filepath.Base(source))
	if err := os.Rename(source, destination); err != nil {
		return trash.TrashResult{}, err
	}
	return trash.TrashResult{OriginalPath: source, TrashedPath: destination, Method: "test_rename"}, nil
}

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
	seenSeq := map[uint64]bool{}
	var restoredText string
	for _, event := range transcript.Runs[1].Events {
		if seenSeq[event.Seq] {
			t.Fatalf("transcript frame seq must be unique, duplicate %d in %+v", event.Seq, transcript.Runs[1].Events)
		}
		seenSeq[event.Seq] = true
		if event.Type == agui.EventTextMessageContent {
			restoredText += event.Delta
		}
	}
	if restoredText != "follow up" {
		t.Fatalf("expected transcript assistant text %q, got %q", "follow up", restoredText)
	}
}

func TestRefreshThreadContextSnapshotEndpointPersistsReplayableV4Detail(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	workspace := t.TempDir()
	runService := awruntime.NewRunServiceWithModelProfiles(
		func(workspace awruntime.WorkspaceConfig, threadID string, profile *awruntime.ModelProfileConfig) *agentcore.Agent {
			cfg := agentcore.ModelConfig{}
			if profile != nil {
				cfg = agentcore.ModelConfig{
					APIKey: profile.APIKey, BaseURL: profile.BaseURL, Model: profile.ModelID,
					Protocol: profile.Protocol, ProviderID: profile.ProviderID, ProfileID: profile.ProfileID,
					CredentialRevision: profile.CredentialRevision, MaxTokens: profile.MaxTokens,
					ContextWindow: profile.ContextWindow,
				}
			}
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				ModelConfig: cfg, Workspace: agentcore.WorkspaceConfig{Root: workspace.WorkspaceRoot},
				ContextEpochStore: awruntime.NewContextEpochStore(),
			}, threadID)
		},
		func(agent *agentcore.Agent) awruntime.Executor {
			return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
				return "done", nil
			})
		},
	)
	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	profileJSON := `{"profile_id":"profile-deepseek","provider_id":"deepseek","model_id":"deepseek-v4-pro","base_url":"https://api.deepseek.com","protocol":"openai_chat","api_key":"secret","credential_revision":1,"context_window":1000000,"max_tokens":16000}`
	createBody := fmt.Sprintf(`{"input":"legacy thread","workspace":{"workspace_root":%q},"model_profile":%s}`, workspace, profileJSON)
	createdResp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(createBody))
	if err != nil {
		t.Fatalf("create seed run: %v", err)
	}
	defer createdResp.Body.Close()
	var run awruntime.Run
	if err := json.NewDecoder(createdResp.Body).Decode(&run); err != nil {
		t.Fatalf("decode seed run: %v", err)
	}
	waitHTTPRunTerminal(t, ts.URL, run.RunID)

	refreshBody := fmt.Sprintf(`{"model_profile":%s}`, profileJSON)
	refreshResp, err := http.Post(ts.URL+"/v1/threads/"+run.ThreadID+"/context-snapshot", "application/json", strings.NewReader(refreshBody))
	if err != nil {
		t.Fatalf("refresh context snapshot: %v", err)
	}
	defer refreshResp.Body.Close()
	if refreshResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(refreshResp.Body)
		t.Fatalf("expected 204 from context refresh, got %d: %s", refreshResp.StatusCode, body)
	}

	transcriptResp, err := http.Get(ts.URL + "/v1/threads/" + run.ThreadID + "/transcript")
	if err != nil {
		t.Fatalf("read refreshed transcript: %v", err)
	}
	defer transcriptResp.Body.Close()
	var transcript threadTranscriptResponse
	if err := json.NewDecoder(transcriptResp.Body).Decode(&transcript); err != nil {
		t.Fatalf("decode refreshed transcript: %v", err)
	}
	found := false
	for _, frame := range transcript.Runs[len(transcript.Runs)-1].Events {
		if frame.Type == agui.EventAwakenContextSnapshot && frame.ContextUsageSnapshot != nil {
			found = frame.ContextUsageSnapshot.SchemaVersion == 4 && len(frame.ContextUsageSnapshot.Contributions) > 0
		}
	}
	if !found {
		t.Fatalf("expected replayable v4 context snapshot, got %+v", transcript.Runs[len(transcript.Runs)-1].Events)
	}
}

func TestGetThreadTranscriptEndpointProjectsDurableSemanticFramesAfterRestart(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	workspace := t.TempDir()
	startExecution := make(chan struct{})
	var durableRunID string
	var durableThreadID string

	firstService := awruntime.NewRunService(
		func(workspace awruntime.WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				Workspace: agentcore.WorkspaceConfig{Root: workspace.WorkspaceRoot},
			}, threadID)
		},
		func(agent *agentcore.Agent) awruntime.Executor {
			return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				<-startExecution
				recorder := agent.SemanticRecorder()
				records := []agentcore.SemanticRecord{
					agentcore.NewSemanticRecord(agentcore.SemanticKindRunStarted, durableThreadID, durableRunID, "turn-1", "", "", nil),
					agentcore.NewSemanticRecord(agentcore.SemanticKindUserMessage, durableThreadID, durableRunID, "turn-1", "", "", map[string]any{
						"content": input,
						"role":    "user",
					}),
					agentcore.NewSemanticRecord(agentcore.SemanticKindAssistantMessage, durableThreadID, durableRunID, "turn-1", "", "", map[string]any{
						"content":    "先读取数据",
						"role":       "assistant",
						"message_id": "msg-durable",
					}),
					agentcore.NewSemanticRecord(agentcore.SemanticKindToolCall, durableThreadID, durableRunID, "turn-1", "msg-durable", "", map[string]any{
						"tool_call_id": "call-durable",
						"tool_name":    "read_file",
						"arguments":    `{"path":"README.md"}`,
					}),
					agentcore.NewSemanticRecord(agentcore.SemanticKindToolResult, durableThreadID, durableRunID, "turn-1", "call-durable", "", map[string]any{
						"tool_call_id": "call-durable",
						"tool_name":    "read_file",
						"content":      "# Awaken",
						"is_error":     false,
					}),
					agentcore.NewSemanticRecord(agentcore.SemanticKindAssistantMessage, durableThreadID, durableRunID, "turn-1", "", "", map[string]any{
						"content":    "历史最终回答",
						"role":       "assistant",
						"message_id": "msg-final",
					}),
					agentcore.NewSemanticRecord(agentcore.SemanticKindRunTerminal, durableThreadID, durableRunID, "turn-1", "", "", map[string]any{
						"status":          "finished",
						"result":          "历史最终回答",
						"terminal_reason": "completed",
					}),
				}
				if err := recorder.AppendBatch(ctx, records); err != nil {
					return "", err
				}
				if err := recorder.Flush(ctx); err != nil {
					return "", err
				}
				return "历史最终回答", nil
			})
		},
	)

	run, err := firstService.StartRun(awruntime.StartRunRequest{
		Input:     "恢复语义历史",
		Workspace: awruntime.WorkspaceConfig{WorkspaceRoot: workspace},
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	durableRunID = run.RunID
	durableThreadID = run.ThreadID
	close(startExecution)
	waitHTTPServiceRunTerminal(t, firstService, run.RunID)
	waitHTTPServiceThreadIdle(t, firstService, run.ThreadID)
	if err := firstService.FlushTranscripts(); err != nil {
		t.Fatalf("FlushTranscripts: %v", err)
	}
	// A new service has no in-memory Session and must load the durable journal.
	restartedService := awruntime.NewRunService(nil, nil)
	ts := httptest.NewServer(NewRouter(restartedService, nil))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/threads/" + run.ThreadID + "/transcript")
	if err != nil {
		t.Fatalf("GET durable transcript: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var transcript threadTranscriptResponse
	if err := json.NewDecoder(resp.Body).Decode(&transcript); err != nil {
		t.Fatalf("decode durable transcript: %v", err)
	}
	if len(transcript.Runs) != 1 {
		t.Fatalf("expected one durable run, got %d", len(transcript.Runs))
	}

	seenSeq := map[uint64]bool{}
	var assistantText string
	var toolArgs string
	var textStarts, textEnds, toolStarts, toolEnds int
	for _, event := range transcript.Runs[0].Events {
		if seenSeq[event.Seq] {
			t.Fatalf("durable transcript frame seq must be unique, duplicate %d", event.Seq)
		}
		seenSeq[event.Seq] = true
		switch event.Type {
		case agui.EventTextMessageStart:
			textStarts++
		case agui.EventTextMessageContent:
			assistantText += event.Delta
		case agui.EventTextMessageEnd:
			textEnds++
		case agui.EventToolCallStart:
			toolStarts++
		case agui.EventToolCallArgs:
			toolArgs += event.Delta
		case agui.EventToolCallEnd:
			toolEnds++
		}
	}
	if assistantText != "先读取数据历史最终回答" {
		t.Fatalf("unexpected durable assistant text %q; events=%+v", assistantText, transcript.Runs[0].Events)
	}
	if toolArgs != `{"path":"README.md"}` {
		t.Fatalf("unexpected durable tool args %q", toolArgs)
	}
	if textStarts != 2 || textEnds != 2 || toolStarts != 1 || toolEnds != 1 {
		t.Fatalf("incomplete durable frame lifecycle: text=%d/%d tool=%d/%d", textStarts, textEnds, toolStarts, toolEnds)
	}
}

func TestDurableLiveAndRestartTranscriptShareCanonicalProjection(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	workspace := t.TempDir()
	service := awruntime.NewRunService(
		func(workspace awruntime.WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				Workspace: agentcore.WorkspaceConfig{Root: workspace.WorkspaceRoot},
			}, threadID)
		},
		func(agent *agentcore.Agent) awruntime.Executor {
			return awruntime.ExecutorFunc(func(ctx context.Context, _ string, _ agentcore.EventSink) (string, error) {
				threadID, runID, sessionID := agent.ExecutionIdentity()
				records := []agentcore.SemanticRecord{
					agentcore.NewSemanticRecord(agentcore.SemanticKindRunStarted, threadID, runID, "turn_1", "", sessionID, nil),
					agentcore.NewSemanticRecord(agentcore.SemanticKindToolCall, threadID, runID, "turn_1", "msg_1", sessionID, map[string]any{
						"tool_call_id": "call_1",
						"tool_name":    "apply_patch",
						"arguments":    `{"patch":"*** Begin Patch"}`,
					}),
					agentcore.NewSemanticRecord(agentcore.SemanticKindRecoveryDecision, threadID, runID, "turn_1", "call_1", sessionID, map[string]any{
						"tool_call_id":      "call_1",
						"tool_name":         "apply_patch",
						"error_code":        "repeated_tool_failure",
						"blocked":           true,
						"recovery_options":  []string{"inspect_current_file", "change_patch_or_stop"},
						"recovery_required": true,
						"message":           "tool retry blocked",
						"retryable":         false,
						"tool_status":       "denied",
						"transport_status":  "not_started",
						"semantic_status":   "failure",
						"process_status":    "not_applicable",
						"error_category":    "policy",
					}),
					agentcore.NewSemanticRecord(agentcore.SemanticKindToolResult, threadID, runID, "turn_1", "call_1", sessionID, map[string]any{
						"tool_call_id":   "call_1",
						"tool_name":      "apply_patch",
						"content":        "retry blocked",
						"is_error":       true,
						"affected_paths": []string{"/tmp/example.txt"},
						"patch_summary":  "no changes applied",
						"outcome": map[string]any{
							"status":           "denied",
							"transport_status": "not_started",
							"semantic_status":  "failure",
							"process_status":   "not_applicable",
							"error_code":       "repeated_tool_failure",
							"error_category":   "policy",
							"retryable":        false,
						},
					}),
					agentcore.NewSemanticRecord(agentcore.SemanticKindArtifact, threadID, runID, "turn_1", "call_1", sessionID, map[string]any{
						"artifact_id":  "artifact_1",
						"title":        "验收产物",
						"description":  "canonical artifact",
						"kind":         "text",
						"path":         "/tmp/example.txt",
						"content":      "artifact body",
						"status":       "ready",
						"tool_call_id": "call_1",
						"tool_name":    "apply_patch",
						"created":      true,
					}),
					agentcore.NewSemanticRecord(agentcore.SemanticKindRunTerminal, threadID, runID, "turn_1", "", sessionID, map[string]any{
						"status":          "finished",
						"result":          "done",
						"terminal_reason": "completed",
					}),
				}
				if err := agent.SemanticRecorder().AppendBatch(ctx, records); err != nil {
					return "", err
				}
				if err := agent.SemanticRecorder().Flush(ctx); err != nil {
					return "", err
				}
				return "done", nil
			})
		},
	)
	ts := httptest.NewServer(NewRouter(service, nil))
	defer ts.Close()

	run, err := service.StartRun(awruntime.StartRunRequest{
		Input:     "canonical durable projection",
		Workspace: awruntime.WorkspaceConfig{WorkspaceRoot: workspace},
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	waitHTTPServiceRunTerminal(t, service, run.RunID)
	waitHTTPServiceThreadIdle(t, service, run.ThreadID)

	live := fetchSSEEvents(t, ts.URL, run.RunID, "")
	activeTranscript := fetchThreadTranscript(t, ts.URL, run.ThreadID)
	if len(activeTranscript.Runs) != 1 || activeTranscript.Runs[0].StreamCursor == 0 {
		t.Fatalf("active transcript missing reliable stream cursor: %+v", activeTranscript.Runs)
	}
	assertCanonicalDurableFramesEqual(t, live, activeTranscript.Runs[0].Events)

	if err := service.FlushTranscripts(); err != nil {
		t.Fatalf("FlushTranscripts: %v", err)
	}
	restarted := awruntime.NewRunService(nil, nil)
	restartedServer := httptest.NewServer(NewRouter(restarted, nil))
	defer restartedServer.Close()
	restartedTranscript := fetchThreadTranscript(t, restartedServer.URL, run.ThreadID)
	if len(restartedTranscript.Runs) != 1 {
		t.Fatalf("restarted transcript run count: %d", len(restartedTranscript.Runs))
	}
	assertCanonicalDurableFramesEqual(t, live, restartedTranscript.Runs[0].Events)

	frames := canonicalDurableFrames(live)
	recoveryIndex, resultIndex := -1, -1
	for i, event := range frames {
		switch event.Type {
		case agui.EventAwakenToolRetryBlocked:
			recoveryIndex = i
			if event.ErrorCode != "repeated_tool_failure" || event.ToolStatus != "denied" ||
				event.TransportStatus != "not_started" || event.SemanticStatus != "failure" ||
				event.ProcessStatus != "not_applicable" || event.ErrorCategory != "policy" ||
				event.Retryable || len(event.RecoveryOptions) != 2 {
				t.Fatalf("blocked recovery fields drifted: %+v", event)
			}
		case agui.EventToolCallResult:
			resultIndex = i
			if event.PatchSummary != "no changes applied" || len(event.AffectedPaths) != 1 {
				t.Fatalf("tool result fields drifted: %+v", event)
			}
		}
	}
	if recoveryIndex < 0 || resultIndex < 0 || recoveryIndex >= resultIndex {
		t.Fatalf("blocked recovery must precede tool result: recovery=%d result=%d frames=%+v", recoveryIndex, resultIndex, frames)
	}
}

func TestRunningTranscriptResumesEphemeralStreamWithoutLossOrDuplicates(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	workspace := t.TempDir()
	ready := make(chan struct{})
	release := make(chan struct{})
	service := awruntime.NewRunService(
		func(workspace awruntime.WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				Workspace: agentcore.WorkspaceConfig{Root: workspace.WorkspaceRoot},
			}, threadID)
		},
		func(agent *agentcore.Agent) awruntime.Executor {
			return awruntime.ExecutorFunc(func(ctx context.Context, _ string, sink agentcore.EventSink) (string, error) {
				threadID, runID, sessionID := agent.ExecutionIdentity()
				if err := agent.SemanticRecorder().Append(ctx, agentcore.NewSemanticRecord(
					agentcore.SemanticKindRunStarted, threadID, runID, "turn_1", "", sessionID, nil,
				)); err != nil {
					return "", err
				}
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.ReasoningStartedEventType, MessageID: "reason_1"})
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.ReasoningDeltaEventType, MessageID: "reason_1", Delta: "before"})
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextMessageStartedEventType, MessageID: "msg_1", Role: "assistant"})
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextDeltaEventType, MessageID: "msg_1", Role: "assistant", Delta: "partial"})
				close(ready)
				<-release
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextDeltaEventType, MessageID: "msg_1", Role: "assistant", Delta: " done"})
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextMessageFinishedEventType, MessageID: "msg_1", Role: "assistant"})
				terminal := agentcore.NewSemanticRecord(agentcore.SemanticKindRunTerminal, threadID, runID, "turn_1", "", sessionID, map[string]any{
					"status":          "finished",
					"result":          "partial done",
					"terminal_reason": "completed",
				})
				if err := agent.SemanticRecorder().Append(ctx, terminal); err != nil {
					return "", err
				}
				if err := agent.SemanticRecorder().Flush(ctx); err != nil {
					return "", err
				}
				return "partial done", nil
			})
		},
	)
	ts := httptest.NewServer(NewRouter(service, nil))
	defer ts.Close()
	run, err := service.StartRun(awruntime.StartRunRequest{
		Input:     "running resume",
		Workspace: awruntime.WorkspaceConfig{WorkspaceRoot: workspace},
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	<-ready

	transcript := fetchThreadTranscript(t, ts.URL, run.ThreadID)
	if len(transcript.Runs) != 1 || transcript.Runs[0].StreamCursor == 0 {
		t.Fatalf("running transcript missing stream cursor: %+v", transcript.Runs)
	}
	entry := transcript.Runs[0]
	if got := joinedTextContent(entry.Events); got != "partial" {
		t.Fatalf("running transcript lost pre-snapshot ephemeral text: %q events=%+v", got, entry.Events)
	}

	currentResp := openSSESubscription(t, ts.URL, run.RunID, "after_cursor", entry.StreamCursor)
	defer currentResp.Body.Close()
	legacyResp := openSSESubscription(t, ts.URL, run.RunID, "after_seq", entry.StreamCursor)
	defer legacyResp.Body.Close()
	close(release)
	currentEvents, err := collectSSEEvents(currentResp.Body)
	if err != nil {
		t.Fatalf("collect current cursor SSE: %v", err)
	}
	legacyEvents, err := collectSSEEvents(legacyResp.Body)
	if err != nil {
		t.Fatalf("collect legacy after_seq SSE: %v", err)
	}
	if got, want := mustJSON(t, currentEvents), mustJSON(t, legacyEvents); got != want {
		t.Fatalf("legacy after_seq diverged from after_cursor\ncurrent=%s\nlegacy=%s", got, want)
	}
	for _, event := range currentEvents {
		if event.StreamCursor <= entry.StreamCursor {
			t.Fatalf("resume replayed an acknowledged frame: cursor=%d watermark=%d event=%+v", event.StreamCursor, entry.StreamCursor, event)
		}
	}

	combined := append(append([]agui.Event(nil), entry.Events...), currentEvents...)
	if got := joinedTextContent(combined); got != "partial done" {
		t.Fatalf("running transcript + SSE text must be exact once, got %q events=%+v", got, combined)
	}
	if countEventType(combined, agui.EventRunStarted) != 1 || countEventType(combined, agui.EventRunFinished) != 1 {
		t.Fatalf("running transcript + SSE duplicated/lost terminal lifecycle: %+v", combined)
	}
	seenCursor := map[uint64]struct{}{}
	lastCursor := uint64(0)
	for _, event := range combined {
		if event.StreamCursor == 0 {
			t.Fatalf("active frame missing stream cursor: %+v", event)
		}
		if _, duplicate := seenCursor[event.StreamCursor]; duplicate {
			t.Fatalf("duplicate stream cursor %d in %+v", event.StreamCursor, combined)
		}
		if event.StreamCursor <= lastCursor {
			t.Fatalf("non-monotonic stream cursor %d after %d", event.StreamCursor, lastCursor)
		}
		seenCursor[event.StreamCursor] = struct{}{}
		lastCursor = event.StreamCursor
	}
	waitHTTPServiceThreadIdle(t, service, run.ThreadID)
	if err := service.FlushTranscripts(); err != nil {
		t.Fatalf("FlushTranscripts: %v", err)
	}
}

func TestA34TranscriptEndpointRecoversCompleteFactsBeforePartialTail(t *testing.T) {
	service, run, path := createCompletedBoundaryRun(t, "a34 endpoint")
	if err := service.FlushTranscripts(); err != nil {
		t.Fatalf("A34 FlushTranscripts: %v", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("A34 open journal: %v", err)
	}
	if _, err := file.WriteString(`{"schema_version":2,"record_id":"partial`); err != nil {
		file.Close()
		t.Fatalf("A34 append partial tail: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("A34 close journal: %v", err)
	}

	restarted := awruntime.NewRunService(nil, nil)
	ts := httptest.NewServer(NewRouter(restarted, nil))
	defer ts.Close()
	transcript := fetchThreadTranscript(t, ts.URL, run.ThreadID)
	if len(transcript.Runs) != 1 {
		t.Fatalf("A34 endpoint lost recovered run: %+v", transcript.Runs)
	}
	if got := joinedTextContent(transcript.Runs[0].Events); got != "boundary answer" {
		t.Fatalf("A34 endpoint lost/duplicated pre-tail assistant content: %q events=%+v", got, transcript.Runs[0].Events)
	}
	if countEventType(transcript.Runs[0].Events, agui.EventRunFinished) != 1 {
		t.Fatalf("A34 endpoint lost terminal before partial tail: %+v", transcript.Runs[0].Events)
	}
}

func TestA35TranscriptEndpointReturnsFailureForMiddleCorruption(t *testing.T) {
	service, run, path := createCompletedBoundaryRun(t, "a35 endpoint")
	if err := service.FlushTranscripts(); err != nil {
		t.Fatalf("A35 FlushTranscripts: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("A35 read journal: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("A35 fixture unexpectedly short: %q", raw)
	}
	lines = append(lines[:len(lines)-1], append([]string{"{not-json"}, lines[len(lines)-1])...)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("A35 inject middle corruption: %v", err)
	}

	restarted := awruntime.NewRunService(nil, nil)
	ts := httptest.NewServer(NewRouter(restarted, nil))
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/v1/threads/" + run.ThreadID + "/transcript")
	if err != nil {
		t.Fatalf("A35 GET transcript: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("A35 endpoint silently skipped middle corruption: status=%d body=%s", resp.StatusCode, body)
	}
}

func TestPersistenceErrorTerminalWinsAcrossLiveActiveAndRestartTranscript(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	workspace := t.TempDir()
	service := awruntime.NewRunService(
		func(workspace awruntime.WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				Workspace: agentcore.WorkspaceConfig{Root: workspace.WorkspaceRoot},
			}, threadID)
		},
		func(agent *agentcore.Agent) awruntime.Executor {
			return awruntime.ExecutorFunc(func(ctx context.Context, _ string, _ agentcore.EventSink) (string, error) {
				threadID, runID, sessionID := agent.ExecutionIdentity()
				records := []agentcore.SemanticRecord{
					agentcore.NewSemanticRecord(agentcore.SemanticKindRunStarted, threadID, runID, "turn_terminal", "", sessionID, nil),
					agentcore.NewSemanticRecord(agentcore.SemanticKindRunTerminal, threadID, runID, "turn_terminal", "", sessionID, map[string]any{
						"status": "finished", "result": "must not surface", "terminal_reason": "completed",
					}),
					agentcore.NewSemanticRecord(agentcore.SemanticKindRunTerminal, threadID, runID, "turn_terminal", "", sessionID, map[string]any{
						"status": "error", "error": "fsync failed", "code": "PERSISTENCE_ERROR", "terminal_reason": "persistence_error",
					}),
				}
				if err := agent.SemanticRecorder().AppendBatch(ctx, records); err != nil {
					return "", err
				}
				if err := agent.SemanticRecorder().Flush(ctx); err != nil {
					return "", err
				}
				return "must not surface", nil
			})
		},
	)
	ts := httptest.NewServer(NewRouter(service, nil))
	defer ts.Close()
	run, err := service.StartRun(awruntime.StartRunRequest{
		Input:     "terminal last wins",
		Workspace: awruntime.WorkspaceConfig{WorkspaceRoot: workspace},
	})
	if err != nil {
		t.Fatalf("start terminal recovery run: %v", err)
	}
	terminal := waitHTTPServiceRunTerminal(t, service, run.RunID)
	if terminal.Status != awruntime.RunStatusError || terminal.Error != "fsync failed" {
		t.Fatalf("live session exposed stale success: %+v", terminal)
	}
	waitHTTPServiceThreadIdle(t, service, run.ThreadID)

	live := fetchSSEEvents(t, ts.URL, run.RunID, "")
	assertOnlyPersistenceRunErrorFrames(t, live)
	active := fetchThreadTranscript(t, ts.URL, run.ThreadID)
	assertOnlyPersistenceRunErrorFrames(t, active.Runs[0].Events)
	if err := service.FlushTranscripts(); err != nil {
		t.Fatalf("flush terminal recovery transcript: %v", err)
	}

	restarted := awruntime.NewRunService(nil, nil)
	restartedServer := httptest.NewServer(NewRouter(restarted, nil))
	defer restartedServer.Close()
	replayed := fetchThreadTranscript(t, restartedServer.URL, run.ThreadID)
	assertOnlyPersistenceRunErrorFrames(t, replayed.Runs[0].Events)
}

func assertOnlyPersistenceRunErrorFrames(t *testing.T, events []agui.Event) {
	t.Helper()
	var finished, persistenceErrors int
	for _, event := range events {
		switch event.Type {
		case agui.EventRunFinished:
			finished++
		case agui.EventRunError:
			if event.Code == "PERSISTENCE_ERROR" && event.TerminalReason == "persistence_error" {
				persistenceErrors++
			}
		}
	}
	if finished != 0 || persistenceErrors != 1 {
		t.Fatalf("terminal last-wins violated: finished=%d persistence_errors=%d events=%+v", finished, persistenceErrors, events)
	}
}

func createCompletedBoundaryRun(t *testing.T, input string) (*awruntime.RunService, awruntime.Run, string) {
	t.Helper()
	t.Setenv("AWAKEN_HOME", t.TempDir())
	workspace := t.TempDir()
	service := awruntime.NewRunService(
		func(workspace awruntime.WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				Workspace: agentcore.WorkspaceConfig{Root: workspace.WorkspaceRoot},
			}, threadID)
		},
		func(agent *agentcore.Agent) awruntime.Executor {
			return awruntime.ExecutorFunc(func(ctx context.Context, input string, _ agentcore.EventSink) (string, error) {
				threadID, runID, sessionID := agent.ExecutionIdentity()
				records := []agentcore.SemanticRecord{
					agentcore.NewSemanticRecord(agentcore.SemanticKindRunStarted, threadID, runID, "turn_boundary", "", sessionID, nil),
					agentcore.NewSemanticRecord(agentcore.SemanticKindUserMessage, threadID, runID, "turn_boundary", "", sessionID, map[string]any{"content": input}),
					agentcore.NewSemanticRecord(agentcore.SemanticKindAssistantMessage, threadID, runID, "turn_boundary", "", sessionID, map[string]any{"content": "boundary answer", "message_id": "msg_boundary"}),
					agentcore.NewSemanticRecord(agentcore.SemanticKindRunTerminal, threadID, runID, "turn_boundary", "", sessionID, map[string]any{"status": "finished", "result": "boundary answer", "terminal_reason": "completed"}),
				}
				if err := agent.SemanticRecorder().AppendBatch(ctx, records); err != nil {
					return "", err
				}
				if err := agent.SemanticRecorder().Flush(ctx); err != nil {
					return "", err
				}
				return "boundary answer", nil
			})
		},
	)
	run, err := service.StartRun(awruntime.StartRunRequest{
		Input:     input,
		Workspace: awruntime.WorkspaceConfig{WorkspaceRoot: workspace},
	})
	if err != nil {
		t.Fatalf("start boundary run: %v", err)
	}
	waitHTTPServiceRunTerminal(t, service, run.RunID)
	waitHTTPServiceThreadIdle(t, service, run.ThreadID)
	generationDir, _, err := filestore.ReadCurrentGeneration(desktopdata.AwakenHomeRoot())
	if err != nil {
		t.Fatalf("resolve FileStore generation: %v", err)
	}
	return service, run, filepath.Join(generationDir, "sessions", run.ThreadID, "journal.jsonl")
}

func fetchThreadTranscript(t *testing.T, baseURL, threadID string) threadTranscriptResponse {
	t.Helper()
	resp, err := http.Get(baseURL + "/v1/threads/" + threadID + "/transcript")
	if err != nil {
		t.Fatalf("GET thread transcript: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET thread transcript status=%d", resp.StatusCode)
	}
	var transcript threadTranscriptResponse
	if err := json.NewDecoder(resp.Body).Decode(&transcript); err != nil {
		t.Fatalf("decode thread transcript: %v", err)
	}
	return transcript
}

func openSSESubscription(t *testing.T, baseURL, runID, cursorName string, cursor uint64) *http.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	url := fmt.Sprintf("%s/v1/runs/%s/events?%s=%d", baseURL, runID, cursorName, cursor)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("create SSE request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open SSE subscription: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("open SSE status=%d", resp.StatusCode)
	}
	return resp
}

func canonicalDurableFrames(events []agui.Event) []agui.Event {
	frames := make([]agui.Event, 0, len(events))
	for _, event := range events {
		if event.RecordID == "" {
			continue
		}
		event.StreamCursor = 0
		if event.SemanticSeq != 0 {
			event.Seq = event.SemanticSeq
		}
		frames = append(frames, event)
	}
	return frames
}

func assertCanonicalDurableFramesEqual(t *testing.T, left, right []agui.Event) {
	t.Helper()
	got := mustJSON(t, canonicalDurableFrames(left))
	want := mustJSON(t, canonicalDurableFrames(right))
	if got != want {
		t.Fatalf("canonical durable projections differ\nleft=%s\nright=%s", got, want)
	}
}

func joinedTextContent(events []agui.Event) string {
	var out strings.Builder
	for _, event := range events {
		if event.Type == agui.EventTextMessageContent {
			out.WriteString(event.Delta)
		}
	}
	return out.String()
}

func countEventType(events []agui.Event, eventType agui.EventType) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal comparison value: %v", err)
	}
	return string(raw)
}

func waitHTTPServiceRunTerminal(t *testing.T, service *awruntime.RunService, runID string) awruntime.Run {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, err := service.GetRun(runID)
		if err != nil {
			t.Fatalf("GetRun: %v", err)
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

func waitHTTPServiceThreadIdle(t *testing.T, service *awruntime.RunService, threadID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, thread := range service.ListThreads() {
			if thread.ThreadID == threadID && thread.ActiveRunID == "" {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("thread %s did not become idle", threadID)
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

func TestMapRuntimeEventProjectsScheduledNotice(t *testing.T) {
	// The live SSE path wraps assigned scheduled_notice records as
	// EventKindSemantic and maps them through the canonical projector, so the
	// wire frame must carry the full reminder payload plus the resume cursor.
	events := mapRuntimeEvent(awruntime.Event{
		Kind:     awruntime.EventKindSemantic,
		Seq:      9,
		ThreadID: "thread-1",
		RunID:    "run-1",
		SemanticRecord: agentcore.SemanticRecord{
			SchemaVersion: 2,
			RecordID:      "rec_scheduled_notice_1",
			Seq:           2,
			ThreadID:      "thread-1",
			RunID:         "run-1",
			Kind:          agentcore.SemanticKindScheduledNotice,
			Timestamp:     time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC),
			Payload: map[string]any{
				"task_id":      "task_1",
				"execution_id": "exec_1",
				"task_name":    "每日站会提醒",
				"content":      "记得填写日报",
			},
		},
	})

	if len(events) != 1 {
		t.Fatalf("expected one mapped event, got %+v", events)
	}
	event := events[0]
	if event.Type != agui.EventAwakenScheduledNotice {
		t.Fatalf("expected AWAKEN_SCHEDULED_NOTICE, got %s", event.Type)
	}
	if event.TaskID != "task_1" || event.ExecutionID != "exec_1" || event.TaskName != "每日站会提醒" || event.NoticeContent != "记得填写日报" {
		t.Fatalf("live mapping lost scheduled notice fields: %+v", event)
	}
	if cursor := frameSeq(9, 0); event.Seq != cursor || event.StreamCursor != cursor {
		t.Fatalf("expected live cursor %d, got seq=%d streamCursor=%d", cursor, event.Seq, event.StreamCursor)
	}
	if event.SemanticSeq != agui.FrameSeq(2, 0) || event.RecordID != "rec_scheduled_notice_1" {
		t.Fatalf("live mapping lost durable identity: %+v", event)
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
		t.Fatalf("expected at least one read-only tool call (Read/Glob/Grep)")
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
	case "Read", "Glob", "Grep":
		return true
	}
	return false
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

	firstEvents, resolveErr := streamEvents(t, ts.URL, firstRun.RunID)
	if resolveErr != nil {
		t.Fatalf("failed to stream events (first run): %v", resolveErr)
	}

	var firstSawRunFinished bool
	var firstFinalOutcome string
	for _, ev := range firstEvents {
		t.Logf("first run event: type=%s tool=%s status=%s outcome=%v",
			ev.Type, ev.ToolCallName, ev.Status, ev.Outcome)
		switch ev.Type {
		case agui.EventToolCallStart:
			t.Fatalf("did not expect tool call on first memory-only run")
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

	secondEvents, resolveErr := streamEvents(t, ts.URL, secondRun.RunID)
	if resolveErr != nil {
		t.Fatalf("failed to stream events (second run): %v", resolveErr)
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

func streamEvents(t *testing.T, baseURL, runID string) ([]agui.Event, error) {
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
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
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
	t.Setenv("AWAKEN_HOME", workspace)
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

func TestMCPUserConfigRoutesWorkWithoutWorkspace(t *testing.T) {
	awakenHome := t.TempDir()
	t.Setenv("AWAKEN_HOME", awakenHome)

	configPath := filepath.Clean(mcpclient.UserConfigPath())
	relConfigPath, err := filepath.Rel(awakenHome, configPath)
	if err != nil || relConfigPath == ".." || strings.HasPrefix(relConfigPath, ".."+string(os.PathSeparator)) {
		t.Fatalf("user MCP config path %q must be inside isolated AWAKEN_HOME %q", configPath, awakenHome)
	}

	registry := mcpclient.NewInProcRegistry()
	if err := registry.Register("workspace-free-echo", echoInProcServerFactory()); err != nil {
		t.Fatalf("register in-process MCP server: %v", err)
	}
	mgr := mcpclient.NewManager(
		mcpclient.WithInProcRegistry(registry),
		mcpclient.WithLogger(testLogger{}),
	)
	const serverName = "workspace_free_user"
	t.Cleanup(func() { _ = mgr.Remove(serverName) })
	router := NewRouterWithWorkspace(nil, nil, mgr, nil, nil, "")

	doRequest := func(method, target string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var reader io.Reader
		if body != nil {
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal %s %s request: %v", method, target, err)
			}
			reader = bytes.NewReader(data)
		}
		req := httptest.NewRequest(method, target, reader)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	initialConfig := mcpclient.McpServerConfig{
		InProc:      &mcpclient.InProcTransportConfig{ServerName: "workspace-free-echo"},
		DisplayName: "Workspace-free initial",
	}
	initialEntry, err := json.Marshal(initialConfig)
	if err != nil {
		t.Fatalf("marshal initial MCP server entry: %v", err)
	}
	postConfigText := fmt.Sprintf(`{"servers":{%q:%s}}`, serverName, initialEntry)
	postRec := doRequest(http.MethodPost, "/v1/mcp/servers", configureMCPServersRequest{ConfigText: postConfigText})
	if postRec.Code != http.StatusOK {
		t.Fatalf("POST user MCP config without workspace: expected 200, got %d: %s", postRec.Code, postRec.Body.String())
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected persisted user MCP config at %q: %v", configPath, err)
	}
	persisted, err := mcpclient.LoadUserConfig()
	if err != nil {
		t.Fatalf("load persisted user MCP config after POST: %v", err)
	}
	if got := persisted.Servers[serverName]; got.InProc == nil || got.InProc.ServerName != "workspace-free-echo" || got.DisplayName != initialConfig.DisplayName {
		t.Fatalf("unexpected persisted user MCP config after POST: %+v", got)
	}
	postSnapshot, err := mgr.SnapshotServer(serverName)
	if err != nil {
		t.Fatalf("expected manager snapshot after POST: %v", err)
	}
	if postSnapshot.Scope != mcpclient.ScopeUser || postSnapshot.Transport != mcpclient.TransportInProc {
		t.Fatalf("unexpected manager snapshot after POST: %+v", postSnapshot)
	}

	getRec := doRequest(http.MethodGet, "/v1/mcp/servers/"+serverName+"/config", nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET user MCP config without workspace: expected 200, got %d: %s", getRec.Code, getRec.Body.String())
	}
	var getResponse mcpServerConfigResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResponse); err != nil {
		t.Fatalf("decode GET user MCP config response: %v", err)
	}
	if getResponse.Name != serverName || getResponse.Scope != string(mcpclient.ScopeUser) || strings.TrimSpace(getResponse.ConfigText) == "" {
		t.Fatalf("unexpected GET user MCP config response: %+v", getResponse)
	}
	var fetchedConfig mcpclient.McpServerConfig
	if err := json.Unmarshal([]byte(getResponse.ConfigText), &fetchedConfig); err != nil {
		t.Fatalf("decode configText from GET response: %v", err)
	}
	if fetchedConfig.DisplayName != initialConfig.DisplayName || fetchedConfig.InProc == nil || fetchedConfig.InProc.ServerName != "workspace-free-echo" {
		t.Fatalf("unexpected configText from GET response: %+v", fetchedConfig)
	}

	updatedConfig := initialConfig
	updatedConfig.DisplayName = "Workspace-free updated"
	updatedEntry, err := json.Marshal(updatedConfig)
	if err != nil {
		t.Fatalf("marshal updated MCP server entry: %v", err)
	}
	patchRec := doRequest(http.MethodPatch, "/v1/mcp/servers/"+serverName+"/config", updateMcpServerConfigRequest{ConfigText: string(updatedEntry)})
	if patchRec.Code != http.StatusOK {
		t.Fatalf("PATCH user MCP config without workspace: expected 200, got %d: %s", patchRec.Code, patchRec.Body.String())
	}
	persisted, err = mcpclient.LoadUserConfig()
	if err != nil {
		t.Fatalf("load persisted user MCP config after PATCH: %v", err)
	}
	if got := persisted.Servers[serverName]; got.DisplayName != updatedConfig.DisplayName {
		t.Fatalf("expected updated config to persist, got %+v", got)
	}
	patchSnapshot, err := mgr.SnapshotServer(serverName)
	if err != nil {
		t.Fatalf("expected manager snapshot after PATCH: %v", err)
	}
	if patchSnapshot.Scope != mcpclient.ScopeUser {
		t.Fatalf("expected user-scope manager snapshot after PATCH, got %+v", patchSnapshot)
	}

	deleteRec := doRequest(http.MethodDelete, "/v1/mcp/servers/"+serverName, nil)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("DELETE user MCP config without workspace: expected 200, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}
	persisted, err = mcpclient.LoadUserConfig()
	if err != nil {
		t.Fatalf("load persisted user MCP config after DELETE: %v", err)
	}
	if _, ok := persisted.Servers[serverName]; ok {
		t.Fatalf("expected DELETE to remove %q from persisted user config", serverName)
	}
	if _, err := mgr.SnapshotServer(serverName); err == nil {
		t.Fatalf("expected DELETE to remove %q from manager snapshots", serverName)
	}
}

func TestMCPProjectOverrideLifecyclePersistsAcrossRestart(t *testing.T) {
	awakenHome := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("AWAKEN_HOME", awakenHome)

	registry := mcpclient.NewInProcRegistry()
	if err := registry.Register("scoped-echo", echoInProcServerFactory()); err != nil {
		t.Fatalf("register in-process MCP server: %v", err)
	}
	const serverName = "shared_scope"
	userConfig := mcpclient.McpServerConfig{
		InProc:      &mcpclient.InProcTransportConfig{ServerName: "scoped-echo"},
		DisplayName: "User fallback",
	}
	projectConfig := mcpclient.McpServerConfig{
		InProc:      &mcpclient.InProcTransportConfig{ServerName: "scoped-echo"},
		DisplayName: "Project override",
	}
	if err := mcpclient.SaveUserConfig(mcpclient.ConfigFile{Servers: map[string]mcpclient.McpServerConfig{serverName: userConfig}}); err != nil {
		t.Fatalf("save user config: %v", err)
	}
	if err := mcpclient.SaveProjectConfig(workspace, mcpclient.ConfigFile{Servers: map[string]mcpclient.McpServerConfig{serverName: projectConfig}}); err != nil {
		t.Fatalf("save project config: %v", err)
	}

	newManager := func() *mcpclient.Manager {
		mgr := mcpclient.NewManager(mcpclient.WithInProcRegistry(registry), mcpclient.WithLogger(testLogger{}))
		userCfg, err := mcpclient.LoadUserConfig()
		if err != nil {
			t.Fatalf("load user config: %v", err)
		}
		projectCfg, err := mcpclient.LoadProjectConfig(workspace)
		if err != nil {
			t.Fatalf("load project config: %v", err)
		}
		mgr.Start(context.Background(), mcpclient.MergeScoped(
			mcpclient.ToScoped(userCfg, mcpclient.ScopeUser),
			mcpclient.ToScoped(projectCfg, mcpclient.ScopeProject),
			nil,
			testLogger{},
		))
		return mgr
	}
	doRequest := func(router http.Handler, method, target string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var reader io.Reader
		if body != nil {
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal %s %s request: %v", method, target, err)
			}
			reader = bytes.NewReader(data)
		}
		req := httptest.NewRequest(method, target, reader)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	mgr := newManager()
	t.Cleanup(func() { mgr.Shutdown(context.Background()) })
	router := NewRouterWithWorkspace(nil, nil, mgr, nil, nil, workspace)
	userConfig.DisplayName = "User updated under override"
	userEntry, err := json.Marshal(userConfig)
	if err != nil {
		t.Fatalf("marshal user update: %v", err)
	}
	postRec := doRequest(router, http.MethodPost, "/v1/mcp/servers", configureMCPServersRequest{ConfigText: fmt.Sprintf(`{"servers":{%q:%s}}`, serverName, userEntry)})
	if postRec.Code != http.StatusOK {
		t.Fatalf("POST shadowed user config: expected 200, got %d: %s", postRec.Code, postRec.Body.String())
	}
	if snapshot, err := mgr.SnapshotServer(serverName); err != nil || snapshot.Scope != mcpclient.ScopeProject {
		t.Fatalf("POST shadowed user config replaced effective project runtime: snapshot=%+v err=%v", snapshot, err)
	}
	getRec := doRequest(router, http.MethodGet, "/v1/mcp/servers/"+serverName+"/config", nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET effective config: expected 200, got %d: %s", getRec.Code, getRec.Body.String())
	}
	var getResponse mcpServerConfigResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResponse); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	if getResponse.Scope != string(mcpclient.ScopeProject) || !strings.Contains(getResponse.ConfigText, "Project override") {
		t.Fatalf("GET did not return project override: %+v", getResponse)
	}

	projectConfig.DisplayName = "Project updated"
	updatedEntry, err := json.Marshal(projectConfig)
	if err != nil {
		t.Fatalf("marshal project update: %v", err)
	}
	patchRec := doRequest(router, http.MethodPatch, "/v1/mcp/servers/"+serverName+"/config", updateMcpServerConfigRequest{ConfigText: string(updatedEntry)})
	if patchRec.Code != http.StatusOK {
		t.Fatalf("PATCH effective project config: expected 200, got %d: %s", patchRec.Code, patchRec.Body.String())
	}
	projectCfg, err := mcpclient.LoadProjectConfig(workspace)
	if err != nil || projectCfg.Servers[serverName].DisplayName != "Project updated" {
		t.Fatalf("project update was not persisted: cfg=%+v err=%v", projectCfg.Servers[serverName], err)
	}
	userCfg, err := mcpclient.LoadUserConfig()
	if err != nil || userCfg.Servers[serverName].DisplayName != "User updated under override" {
		t.Fatalf("project update mutated user fallback: cfg=%+v err=%v", userCfg.Servers[serverName], err)
	}

	disableRec := doRequest(router, http.MethodPost, "/v1/mcp/servers/"+serverName+"/disable", nil)
	if disableRec.Code != http.StatusNoContent {
		t.Fatalf("disable project override: expected 204, got %d: %s", disableRec.Code, disableRec.Body.String())
	}
	projectCfg, err = mcpclient.LoadProjectConfig(workspace)
	if err != nil || projectCfg.Servers[serverName].Enabled == nil || *projectCfg.Servers[serverName].Enabled {
		t.Fatalf("disabled flag was not persisted in project config: cfg=%+v err=%v", projectCfg.Servers[serverName], err)
	}
	if snapshot, err := mgr.SnapshotServer(serverName); err != nil || snapshot.State != mcpclient.StateDisabled {
		t.Fatalf("manager did not enter disabled state: snapshot=%+v err=%v", snapshot, err)
	}

	mgr.Shutdown(context.Background())
	restarted := newManager()
	t.Cleanup(func() { restarted.Shutdown(context.Background()) })
	if snapshot, err := restarted.SnapshotServer(serverName); err != nil || snapshot.State != mcpclient.StateDisabled || snapshot.Scope != mcpclient.ScopeProject {
		t.Fatalf("restart did not restore project disabled state: snapshot=%+v err=%v", snapshot, err)
	}
	restartedRouter := NewRouterWithWorkspace(nil, nil, restarted, nil, nil, workspace)
	enableRec := doRequest(restartedRouter, http.MethodPost, "/v1/mcp/servers/"+serverName+"/enable", nil)
	if enableRec.Code != http.StatusNoContent {
		t.Fatalf("enable project override: expected 204, got %d: %s", enableRec.Code, enableRec.Body.String())
	}
	projectCfg, err = mcpclient.LoadProjectConfig(workspace)
	if err != nil || projectCfg.Servers[serverName].Enabled == nil || !*projectCfg.Servers[serverName].Enabled {
		t.Fatalf("enabled flag was not persisted in project config: cfg=%+v err=%v", projectCfg.Servers[serverName], err)
	}

	deleteRec := doRequest(restartedRouter, http.MethodDelete, "/v1/mcp/servers/"+serverName, nil)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete project override: expected 200, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}
	projectCfg, err = mcpclient.LoadProjectConfig(workspace)
	if err != nil {
		t.Fatalf("load project config after delete: %v", err)
	}
	if _, ok := projectCfg.Servers[serverName]; ok {
		t.Fatalf("project override still exists after delete: %+v", projectCfg.Servers[serverName])
	}
	userCfg, err = mcpclient.LoadUserConfig()
	if err != nil || userCfg.Servers[serverName].DisplayName != "User updated under override" {
		t.Fatalf("user fallback was not preserved: cfg=%+v err=%v", userCfg.Servers[serverName], err)
	}
	if snapshot, err := restarted.SnapshotServer(serverName); err != nil || snapshot.Scope != mcpclient.ScopeUser {
		t.Fatalf("deleting project override did not reveal user runtime: snapshot=%+v err=%v", snapshot, err)
	}
}

func TestMCPConfigEndpointChangeClearsOldCredential(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	disabled := false
	const serverName = "remote"
	oldConfig := mcpclient.McpServerConfig{
		HTTP:    &mcpclient.HTTPTransportConfig{URL: "https://old.example/mcp"},
		Enabled: &disabled,
	}
	if err := mcpclient.SaveUserConfig(mcpclient.ConfigFile{Servers: map[string]mcpclient.McpServerConfig{serverName: oldConfig}}); err != nil {
		t.Fatalf("save user config: %v", err)
	}
	store := mcpclient.NewInMemoryCredentialStore()
	oldIdentity := mcpclient.CredentialIdentity(mcpclient.ScopeUser, serverName, oldConfig)
	if err := store.SaveTokenSource(oldIdentity, mcpclient.StoredTokenSource{ServerURL: oldConfig.HTTP.URL}); err != nil {
		t.Fatalf("save old credential: %v", err)
	}
	mgr := mcpclient.NewManager(mcpclient.WithLogger(testLogger{}))
	mgr.SetCredentialStore(store)
	mgr.Start(context.Background(), []mcpclient.ScopedMcpServerConfig{{Name: serverName, Scope: mcpclient.ScopeUser, Config: oldConfig}})
	t.Cleanup(func() { mgr.Shutdown(context.Background()) })
	router := NewRouterWithWorkspace(nil, nil, mgr, nil, nil, "")
	newConfig := oldConfig
	newConfig.HTTP = &mcpclient.HTTPTransportConfig{URL: "https://new.example/mcp"}
	entry, err := json.Marshal(newConfig)
	if err != nil {
		t.Fatalf("marshal new config: %v", err)
	}
	body, err := json.Marshal(updateMcpServerConfigRequest{ConfigText: string(entry)})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/v1/mcp/servers/"+serverName+"/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH endpoint: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if exists, err := store.HasTokenSource(oldIdentity); err != nil || exists {
		t.Fatalf("old endpoint credential was not cleared: exists=%v err=%v", exists, err)
	}
}

func TestInstallSkillWritesProjectSkillAndRegisters(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
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
	if resp.Skill.Scope != "project" || resp.Skill.RootDir == "" || resp.Skill.SkillFilePath == "" {
		t.Fatalf("expected package paths in response: %+v", resp.Skill)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".awaken", "skills", "daily-followup", "SKILL.md")); err != nil {
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
	if listed[0].RootDir == "" || listed[0].SkillFilePath == "" || listed[0].ProjectPath != workspace {
		t.Fatalf("expected package metadata in list, got %+v", listed[0])
	}
}

func TestDeleteSkillMovesPackageToTrashAndSynchronizesList(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	workspace := t.TempDir()
	registry := skills.NewSkillRegistry(testLogger{})
	trashDir := filepath.Join(t.TempDir(), "Trash")
	router := newRouterInternal(routerConfig{
		skillRegistry:     registry,
		workspaceRoot:     workspace,
		skillTrashAdapter: serverSkillTrashAdapter{dir: trashDir},
	})

	rawSkill := `---
name: removable-skill
description: Remove this skill.
---
Body.`
	body, err := json.Marshal(installSkillRequest{
		FileName: "removable-skill.md",
		Content:  rawSkill,
		Scope:    "project",
	})
	if err != nil {
		t.Fatalf("marshal install request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/skills/install", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("install expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	deleteBody, err := json.Marshal(deleteSkillRequest{
		SourceOrigin: string(skills.SourceProject),
		ProjectRoot:  workspace,
	})
	if err != nil {
		t.Fatalf("marshal delete request: %v", err)
	}
	req = httptest.NewRequest(http.MethodDelete, "/v1/skills/removable-skill", bytes.NewReader(deleteBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var deleted deleteSkillResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &deleted); err != nil {
		t.Fatalf("decode delete response: %v", err)
	}
	if deleted.ID != "removable-skill" || deleted.SourceOrigin != "project" || deleted.TrashedPath == "" {
		t.Fatalf("unexpected delete response: %+v", deleted)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".awaken", "skills", "removable-skill")); !os.IsNotExist(err) {
		t.Fatalf("source package still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(deleted.TrashedPath, "SKILL.md")); err != nil {
		t.Fatalf("trashed package missing: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var listed []skillInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listed) != 0 || registry.Get("removable-skill") != nil {
		t.Fatalf("deleted skill remained in list or registry: listed=%+v registry=%+v", listed, registry.Get("removable-skill"))
	}
}

func TestDeleteBundledSkillFailsClosed(t *testing.T) {
	registry := skills.NewSkillRegistry(testLogger{})
	router := NewRouterWithWorkspace(nil, nil, nil, nil, registry, t.TempDir())
	body, err := json.Marshal(deleteSkillRequest{SourceOrigin: string(skills.SourceBundled)})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodDelete, "/v1/skills/bundled-skill", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "SKILL_DELETE_SOURCE_UNSUPPORTED") {
		t.Fatalf("expected bundled delete error code, got %s", rec.Body.String())
	}
}

func TestInstallSkillZipPreservesPackageResources(t *testing.T) {
	workspace := t.TempDir()
	registry := skills.NewSkillRegistry(testLogger{})
	router := NewRouterWithWorkspace(nil, nil, nil, nil, registry, workspace)

	zipData := makeSkillZip(t, map[string]string{
		"dws/SKILL.md": `---
name: dws
description: DingTalk skill.
---
Read ./references/guide.md and run ./scripts/helper.py.`,
		"dws/references/guide.md": "Guide body",
		"dws/scripts/helper.py":   "print('ok')\n",
	})
	body, err := json.Marshal(installSkillRequest{
		FileName:      "dws.zip",
		ContentBase64: base64.StdEncoding.EncodeToString(zipData),
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/skills/install", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected install 200, got %d: %s", rec.Code, rec.Body.String())
	}

	root := filepath.Join(workspace, ".awaken", "skills", "dws")
	for _, rel := range []string{"SKILL.md", "references/guide.md", "scripts/helper.py"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected package file %s: %v", rel, err)
		}
	}
}

func TestInstallSkillZipRejectsInvalidPackages(t *testing.T) {
	workspace := t.TempDir()
	registry := skills.NewSkillRegistry(testLogger{})
	router := NewRouterWithWorkspace(nil, nil, nil, nil, registry, workspace)

	cases := []struct {
		name string
		data []byte
	}{
		{
			name: "multiple-skill-md",
			data: makeSkillZip(t, map[string]string{
				"a/SKILL.md": "---\nname: a\ndescription: a\n---\n",
				"b/SKILL.md": "---\nname: b\ndescription: b\n---\n",
			}),
		},
		{
			name: "path-traversal",
			data: makeSkillZip(t, map[string]string{
				"../SKILL.md": "---\nname: bad\ndescription: bad\n---\n",
			}),
		},
	}

	for _, tc := range cases {
		body, err := json.Marshal(installSkillRequest{
			FileName:      tc.name + ".zip",
			ContentBase64: base64.StdEncoding.EncodeToString(tc.data),
		})
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/v1/skills/install", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d: %s", tc.name, rec.Code, rec.Body.String())
		}
	}
}

func makeSkillZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		writer, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func writeServerTestSkillPackage(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir skill package: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatalf("write skill package: %v", err)
	}
}

func TestListSkillsReloadsRemovedProjectSkillFromDisk(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	workspace := t.TempDir()
	registry := skills.NewSkillRegistry(testLogger{})
	router := NewRouterWithWorkspace(nil, nil, nil, nil, registry, workspace)

	rawSkill := `---
name: stale-skill
description: Skill that should disappear when removed from disk.
---

Body.`
	body, err := json.Marshal(installSkillRequest{
		FileName: "stale-skill.md",
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
		t.Fatalf("expected install 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if err := os.RemoveAll(filepath.Join(workspace, ".awaken")); err != nil {
		t.Fatalf("remove .awaken: %v", err)
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
	if len(listed) != 0 {
		t.Fatalf("expected removed skill to disappear after reload, got %+v", listed)
	}
}

func TestInstallSkillWritesSelectedProjectRoot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	workspace := t.TempDir()
	selectedProject := t.TempDir()
	registry := skills.NewSkillRegistry(testLogger{})
	router := NewRouterWithWorkspace(nil, nil, nil, nil, registry, workspace)

	rawSkill := `---
name: selected-project-skill
description: Skill installed into a selected project folder.
---

Body.`
	body, err := json.Marshal(installSkillRequest{
		FileName:    "selected-project-skill.md",
		Content:     rawSkill,
		Scope:       "project",
		ProjectRoot: selectedProject,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/skills/install", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected install 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(selectedProject, ".awaken", "skills", "selected-project-skill", "SKILL.md")); err != nil {
		t.Fatalf("expected selected project skill file: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/skills?projectRoot="+url.QueryEscape(selectedProject), nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var listed []skillInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "selected-project-skill" {
		t.Fatalf("expected selected project skill in list, got %+v", listed)
	}
}

func TestListSkillsWithoutWorkspaceStillLoadsUserSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	userSkillDir := filepath.Join(home, ".awaken", "skills", "always-user")
	if err := os.MkdirAll(userSkillDir, 0o700); err != nil {
		t.Fatalf("mkdir user skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userSkillDir, "SKILL.md"), []byte(`---
name: always-user
description: Skill available without a project folder.
---

Body.`), 0o600); err != nil {
		t.Fatalf("write user skill: %v", err)
	}

	registry := skills.NewSkillRegistry(testLogger{})
	invokedAt := time.Date(2026, 7, 12, 1, 2, 3, 0, time.UTC)
	registry.MarkInvoked("always-user", invokedAt)
	router := NewRouterWithWorkspace(nil, nil, nil, nil, registry, "")

	req := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var listed []skillInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "always-user" || listed[0].SourceOrigin != "user" {
		t.Fatalf("expected user skill without workspace, got %+v", listed)
	}
	if listed[0].State != string(skills.StateLoaded) || !listed[0].UserInvocable || !listed[0].ModelInvocable {
		t.Fatalf("expected concrete loaded/invocation states, got %+v", listed[0])
	}
	if listed[0].LastInvokedAt != invokedAt.Format(time.RFC3339Nano) {
		t.Fatalf("lastInvokedAt=%q", listed[0].LastInvokedAt)
	}
}

func TestListSkillsProjectPreviewDoesNotMutateRuntimeRegistry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	workspaceA := t.TempDir()
	workspaceB := t.TempDir()

	writeSkillFile := func(root, id string) {
		dir := filepath.Join(root, ".awaken", "skills", id)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir skills %s: %v", id, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(fmt.Sprintf(`---
name: %s
description: Project skill %s.
---

Body.`, id, id)), 0o600); err != nil {
			t.Fatalf("write skill %s: %v", id, err)
		}
	}
	writeSkillFile(workspaceA, "project-a")
	writeSkillFile(workspaceB, "project-b")

	registry := skills.NewSkillRegistry(testLogger{})
	registry.LoadFromSources(map[skills.SourceOrigin]string{
		skills.SourceProject: filepath.Join(workspaceA, ".awaken", "skills"),
	})
	router := NewRouterWithWorkspace(nil, nil, nil, nil, registry, workspaceA)

	req := httptest.NewRequest(http.MethodGet, "/v1/skills?projectRoot="+url.QueryEscape(workspaceB), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var listed []skillInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "project-b" {
		t.Fatalf("expected project-b preview, got %+v", listed)
	}
	if registry.Get("project-a") == nil {
		t.Fatalf("expected runtime registry to keep project-a after preview")
	}
	if registry.Get("project-b") != nil {
		t.Fatalf("did not expect previewed project-b to mutate runtime registry")
	}
}

func TestInstallSkillWritesUserSkill(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	registry := skills.NewSkillRegistry(testLogger{})
	router := NewRouterWithWorkspace(nil, nil, nil, nil, registry, workspace)

	rawSkill := `---
name: user-skill
description: Skill installed for the current user.
---

Body.`
	body, err := json.Marshal(installSkillRequest{
		FileName: "user-skill.md",
		Content:  rawSkill,
		Scope:    "global",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/skills/install", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected install 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp installSkillResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Skill.ID != "user-skill" || resp.Skill.SourceOrigin != "user" || resp.Skill.Scope != "global" {
		t.Fatalf("unexpected installed skill response: %+v", resp.Skill)
	}
	if _, err := os.Stat(filepath.Join(home, ".awaken", "skills", "user-skill", "SKILL.md")); err != nil {
		t.Fatalf("expected user skill file: %v", err)
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
	if len(listed) != 1 || listed[0].ID != "user-skill" || listed[0].SourceOrigin != "user" {
		t.Fatalf("expected user skill in list, got %+v", listed)
	}
}

func TestValidateComposerTokensAcceptsAvailableProjectSkill(t *testing.T) {
	skillsRoot := t.TempDir()
	writeServerTestSkillPackage(t, skillsRoot, "daily-followup", `---
name: daily-followup
description: Generate follow-up plans.
---
Body.`)
	registry := skills.NewSkillRegistry(testLogger{})
	registry.LoadFromSources(map[skills.SourceOrigin]string{
		skills.SourceProject: skillsRoot,
	})
	handler := runHandlers{skillRegistry: registry}
	skillPath := filepath.Join(skillsRoot, "daily-followup", "SKILL.md")

	err := handler.validateComposerTokens([]awruntime.ComposerToken{
		{
			ID:    "skill:daily-followup:project",
			Type:  "skill",
			Label: "Daily Followup",
			Payload: awruntime.ComposerTokenPayload{
				SkillID:       "daily-followup",
				SourceOrigin:  "project",
				SkillFilePath: skillPath,
				RootDir:       filepath.Dir(skillPath),
			},
		},
	})
	if err != nil {
		t.Fatalf("expected available project skill token to pass, got %v", err)
	}
}

func TestValidateComposerTokensAllowsSkillWithMissingExecutionDependency(t *testing.T) {
	skillsRoot := t.TempDir()
	writeServerTestSkillPackage(t, skillsRoot, "needs-mcp", `---
name: needs-mcp
description: Needs a missing MCP server.
requiredMcpServers:
  - missing-server
---
Body.`)
	registry := skills.NewSkillRegistry(testLogger{})
	registry.LoadFromSources(map[skills.SourceOrigin]string{
		skills.SourceProject: skillsRoot,
	})
	handler := runHandlers{skillRegistry: registry}

	err := handler.validateComposerTokens([]awruntime.ComposerToken{
		{
			ID:    "skill:needs-mcp:project",
			Type:  "skill",
			Label: "needs-mcp",
			Payload: awruntime.ComposerTokenPayload{
				SkillID:      "needs-mcp",
				SourceOrigin: "project",
			},
		},
	})
	if err != nil {
		t.Fatalf("dependency readiness must not disable a structurally valid Skill: %v", err)
	}
	def := registry.Get("needs-mcp")
	if def == nil || def.State != skills.StateLoaded || len(def.DeprecatedWarnings) == 0 {
		t.Fatalf("expected loaded Skill with execution diagnostic, got %+v", def)
	}
}

func TestValidateComposerTokensAllowsLoadedUserInvocableMCPSkill(t *testing.T) {
	registry := skills.NewSkillRegistry(testLogger{})
	registry.RegisterSkill(&skills.SkillDefinition{
		ID: "mcp-calendar", Name: "mcp-calendar", Description: "Calendar prompt",
		Source: skills.SourceMcp, Scope: skills.SourceMcp, State: skills.StateLoaded,
		Context: skills.ContextInline, UserInvocable: true,
		McpBinding: &skills.McpBinding{ServerName: "calendar", PromptName: "plan", Kind: "prompt"},
	})
	handler := runHandlers{skillRegistry: registry}
	err := handler.validateComposerTokens([]awruntime.ComposerToken{{
		ID: "skill:mcp-calendar:mcp", Type: "skill", Label: "MCP Calendar",
		Payload: awruntime.ComposerTokenPayload{SkillID: "mcp-calendar", SourceOrigin: "mcp"},
	}})
	if err != nil {
		t.Fatalf("loaded user-invocable MCP Skill should be accepted: %v", err)
	}
}

func TestValidateComposerTokensRejectsNonUserInvocableSkill(t *testing.T) {
	registry := skills.NewSkillRegistry(testLogger{})
	registry.RegisterSkill(&skills.SkillDefinition{
		ID: "model-only", Name: "model-only", Description: "Model-only workflow",
		Source: skills.SourcePlugin, Scope: skills.SourcePlugin, State: skills.StateLoaded,
		Context: skills.ContextInline, UserInvocable: false, UserInvocableSet: true,
	})
	handler := runHandlers{skillRegistry: registry}
	err := handler.validateComposerTokens([]awruntime.ComposerToken{{
		ID: "skill:model-only:plugin", Type: "skill",
		Payload: awruntime.ComposerTokenPayload{SkillID: "model-only", SourceOrigin: "plugin"},
	}})
	if err == nil || !strings.Contains(err.Error(), "not user-invocable") {
		t.Fatalf("expected user-invocable rejection, got %v", err)
	}
}

func TestValidateComposerTokensRejectsMismatchedSkillPath(t *testing.T) {
	skillsRoot := t.TempDir()
	writeServerTestSkillPackage(t, skillsRoot, "daily-followup", `---
name: daily-followup
description: Generate follow-up plans.
---
Body.`)
	registry := skills.NewSkillRegistry(testLogger{})
	registry.LoadFromSources(map[skills.SourceOrigin]string{
		skills.SourceProject: skillsRoot,
	})
	handler := runHandlers{skillRegistry: registry}

	err := handler.validateComposerTokens([]awruntime.ComposerToken{
		{
			ID:    "skill:daily-followup:project",
			Type:  "skill",
			Label: "Daily Followup",
			Payload: awruntime.ComposerTokenPayload{
				SkillID:       "daily-followup",
				SourceOrigin:  "project",
				SkillFilePath: filepath.Join(t.TempDir(), "SKILL.md"),
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "path changed") {
		t.Fatalf("expected path mismatch rejection, got %v", err)
	}
}

func TestValidateComposerTokensKeepsLegacyNoTokenRequestsCompatible(t *testing.T) {
	handler := runHandlers{}
	if err := handler.validateComposerTokens(nil); err != nil {
		t.Fatalf("expected no-token request to remain compatible, got %v", err)
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
