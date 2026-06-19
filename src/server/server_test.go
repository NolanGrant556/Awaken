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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"awaken/src/agentcore"
	"awaken/src/llm"
	"awaken/src/protocol/agui"
	awruntime "awaken/src/runtime"
	_ "awaken/src/tools"
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

func TestRuntimeInfoEndpointDoesNotLeakSecrets(t *testing.T) {
	provider := NewLLMRuntimeInfoProvider(
		"claude-test",
		"https://api.anthropic.com/v1",
		"sk-secret-key-must-not-appear",
		"/workspace/secret",
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
	if secondResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for workspace mismatch, got %d", secondResp.StatusCode)
	}
	var errResp errorResponse
	if err := json.NewDecoder(secondResp.Body).Decode(&errResp); err != nil {
		t.Fatalf("expected JSON error response, got decode error: %v", err)
	}
	if errResp.Error != "THREAD_WORKSPACE_MISMATCH" {
		t.Fatalf("expected THREAD_WORKSPACE_MISMATCH error code, got %q", errResp.Error)
	}
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
			return run
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run %s did not reach terminal status", runID)
	return awruntime.Run{}
}

func TestMapRuntimeCancelledUsesAGUIRunErrorWithCancelCode(t *testing.T) {
	events := mapRuntimeEvent(awruntime.Event{
		Kind:     awruntime.EventKindRunCancelled,
		ThreadID: "thread-1",
		RunID:    "run-1",
		Message:  "run cancelled",
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
		`"artifactKind":"a2ui_surface"`,
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
				return agent.RunStreamWithEvents(ctx, input, nil, sink)
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
				return agent.RunStreamWithEvents(ctx, input, nil, sink)
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
	var sawReadOnlyTool bool
	var assistantText strings.Builder
	var finalResult string
	var toolResultContents []string

	for _, ev := range events {
		switch ev.Type {
		case agui.EventToolCallStart:
			sawToolCallStart = true
			if isReadOnlyTool(ev.ToolCallName) {
				sawReadOnlyTool = true
			}
		case agui.EventToolCallResult:
			sawToolCallResult = true
			toolResultContents = append(toolResultContents, ev.Content)
		case agui.EventTextMessageContent:
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

// TestFileMutationApprovalSmoke 通过真实 LLM 调用验证 apply_patch 文件修改审批流程。
// 需要项目根目录的 .env/config.toml 提供 LLM_API_KEY；未设置时自动跳过。
func TestFileMutationApprovalSmoke(t *testing.T) {
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
				return agent.RunStreamWithEvents(ctx, input, nil, sink)
			})
		},
	)

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	reqBody := fmt.Sprintf(`{
		"input": "把 note.md 中 hello 改成 world",
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
		sawRunStarted                 bool
		sawApplyPatchToolCallStart    bool
		sawApprovalRequested          bool
		sawStatusAwaitingApproval     bool
		sawStatusRunningAfterApproval bool
		sawApplyPatchToolCallResult   bool
		sawRunFinished                bool
		finalOutcome                  string
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
		case agui.EventAwakenApprovalRequested:
			sawApprovalRequested = true
			hasNoteMD := false
			for _, p := range ev.ApprovalAffectedPaths {
				if strings.Contains(p, "note.md") {
					hasNoteMD = true
					break
				}
			}
			if !hasNoteMD {
				t.Fatalf("expected approval affected paths to contain note.md, got %v", ev.ApprovalAffectedPaths)
			}
		case agui.EventRunStatusChanged:
			if ev.Status == "awaiting_approval" {
				sawStatusAwaitingApproval = true
			}
			if sawStatusAwaitingApproval && ev.Status == "running" {
				sawStatusRunningAfterApproval = true
			}
		case agui.EventToolCallResult:
			if ev.ToolCallName == "apply_patch" {
				sawApplyPatchToolCallResult = true
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
	if !sawApprovalRequested {
		t.Fatalf("expected AWAKEN_APPROVAL_REQUESTED event")
	}
	if !sawStatusAwaitingApproval {
		t.Fatalf("expected RUN_STATUS_CHANGED event with awaiting_approval")
	}
	if !sawStatusRunningAfterApproval {
		t.Fatalf("expected RUN_STATUS_CHANGED event with running after approval resolved")
	}
	if !sawApplyPatchToolCallResult {
		t.Fatalf("expected TOOL_CALL_RESULT event for apply_patch")
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
		t.Fatalf("expected note.md to contain 'world', got %q", string(content))
	}
}

// TestFileMutationRejectSmoke 通过真实 LLM 调用验证 apply_patch 文件修改拒绝审批流程。
// 当审批被拒绝时，文件不应被修改，run 应继续运行并最终成功完成。
// 需要项目根目录的 .env/config.toml 提供 LLM_API_KEY；未设置时自动跳过。
func TestFileMutationRejectSmoke(t *testing.T) {
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
				return agent.RunStreamWithEvents(ctx, input, nil, sink)
			})
		},
	)

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	reqBody := fmt.Sprintf(`{
		"input": "把 note.md 中 hello 改成 world",
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
		sawRunStarted               bool
		sawApplyPatchToolCallStart  bool
		sawApprovalRequested        bool
		sawStatusAwaitingApproval   bool
		sawStatusRunningAfterReject bool
		sawRunFinished              bool
		sawApprovalResolvedRejected bool
		rejectedToolResultContent   string
		approvalRequestedCount      int
		approvalResolvedCount       int
		finalOutcome                string
		assistantText               strings.Builder
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
		case agui.EventAwakenApprovalRequested:
			sawApprovalRequested = true
			approvalRequestedCount++
			if ev.ToolCallName == "apply_patch" {
				hasNoteMD := false
				for _, p := range ev.ApprovalAffectedPaths {
					if strings.Contains(p, "note.md") {
						hasNoteMD = true
						break
					}
				}
				if !hasNoteMD {
					t.Fatalf("expected apply_patch approval affected paths to contain note.md, got %v", ev.ApprovalAffectedPaths)
				}
			}
		case agui.EventAwakenApprovalResolved:
			approvalResolvedCount++
			if ev.ApprovalDecision == "rejected" {
				sawApprovalResolvedRejected = true
			} else {
				t.Fatalf("expected approval decision rejected, got %q", ev.ApprovalDecision)
			}
		case agui.EventToolCallResult:
			if ev.ToolCallName == "apply_patch" && strings.Contains(ev.Content, "审批拒绝") {
				rejectedToolResultContent = ev.Content
			}
		case agui.EventRunStatusChanged:
			if ev.Status == "awaiting_approval" {
				sawStatusAwaitingApproval = true
			}
			if sawStatusAwaitingApproval && ev.Status == "running" {
				sawStatusRunningAfterReject = true
			}
		case agui.EventTextMessageContent:
			assistantText.WriteString(ev.Delta)
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
	if !sawApprovalRequested {
		t.Fatalf("expected AWAKEN_APPROVAL_REQUESTED event")
	}
	if !sawStatusAwaitingApproval {
		t.Fatalf("expected RUN_STATUS_CHANGED event with awaiting_approval")
	}
	if !sawStatusRunningAfterReject {
		t.Fatalf("expected RUN_STATUS_CHANGED event with running after rejection resolved")
	}
	if !sawApprovalResolvedRejected {
		t.Fatalf("expected AWAKEN_APPROVAL_RESOLVED event with decision rejected")
	}
	if rejectedToolResultContent == "" {
		t.Fatalf("expected TOOL_CALL_RESULT for apply_patch containing '审批拒绝', got no matching event")
	}
	if !strings.Contains(rejectedToolResultContent, "工具执行已被审批拒绝") {
		t.Fatalf("expected rejected tool result content to contain '工具执行已被审批拒绝', got %q", rejectedToolResultContent)
	}
	if approvalResolvedCount != approvalRequestedCount {
		t.Fatalf("expected approval requested count (%d) to equal resolved count (%d)", approvalRequestedCount, approvalResolvedCount)
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
	if strings.TrimSpace(string(content)) != "hello" {
		t.Fatalf("expected note.md to still contain 'hello' (file should NOT be modified after rejection), got %q", string(content))
	}

	combinedText := strings.ToLower(assistantText.String())
	hasExplanation := strings.Contains(combinedText, "拒绝") ||
		strings.Contains(combinedText, "reject") ||
		strings.Contains(combinedText, "审批") ||
		strings.Contains(combinedText, "approval") ||
		strings.Contains(combinedText, "修改") ||
		strings.Contains(combinedText, "无法") ||
		strings.Contains(combinedText, "不能") ||
		strings.Contains(combinedText, "抱歉")
	if !hasExplanation {
		t.Fatalf("expected assistant text to contain some explanation about the rejection, got %q", assistantText.String())
	}
}

// TestFileMutationArtifactSmoke 通过真实 LLM 调用验证 apply_patch 文件修改审批通过后
// 会发出文件变更 Artifact 事件，且 Artifact 的 kind/path/toolCallName 符合预期。
// 需要项目根目录的 .env/config.toml 提供 LLM_API_KEY；未设置时自动跳过。
func TestFileMutationArtifactSmoke(t *testing.T) {
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
				return agent.RunStreamWithEvents(ctx, input, nil, sink)
			})
		},
	)

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	reqBody := fmt.Sprintf(`{
		"input": "把 note.md 中 hello 改成 world",
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
		sawRunStarted      bool
		sawRunFinished     bool
		finalOutcome       string
		sawArtifactEvent   bool
		artifactKind       string
		artifactPath       string
		artifactToolCall   string
		artifactEventTypes []string
		approvalPaths      []string
	)

	for _, ev := range events {
		t.Logf("event: type=%s tool=%s artifactKind=%s artifactPath=%s outcome=%v",
			ev.Type, ev.ToolCallName, ev.ArtifactKind, ev.ArtifactPath, ev.Outcome)
		switch ev.Type {
		case agui.EventRunStarted:
			sawRunStarted = true
		case agui.EventAwakenApprovalRequested:
			if ev.ToolCallName == "apply_patch" && len(approvalPaths) == 0 {
				approvalPaths = append(approvalPaths, ev.ApprovalAffectedPaths...)
			}
		case agui.EventAwakenArtifactCreated, agui.EventAwakenArtifactUpdated:
			sawArtifactEvent = true
			artifactEventTypes = append(artifactEventTypes, string(ev.Type))
			// 优先记录关联 apply_patch 的 Artifact；apply_patch 修改 note.md 应产生 modified Artifact。
			if ev.ToolCallName == "apply_patch" {
				artifactToolCall = ev.ToolCallName
				artifactKind = ev.ArtifactKind
				artifactPath = ev.ArtifactPath
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
	if !sawArtifactEvent {
		t.Fatalf("expected AWAKEN_ARTIFACT_CREATED or AWAKEN_ARTIFACT_UPDATED event, got event types=%v", artifactEventTypes)
	}
	if artifactToolCall != "apply_patch" {
		t.Fatalf("expected artifact toolCallName to be apply_patch, got %q (artifact events=%v)", artifactToolCall, artifactEventTypes)
	}
	if artifactKind != "modified" {
		t.Fatalf("expected artifact kind to be 'modified', got %q", artifactKind)
	}
	// Tight path assertion: artifact path must be the workspace-relative
	// "note.md", not an absolute path and not merely a substring match.
	if artifactPath != "note.md" {
		t.Fatalf("expected artifact path to be exactly 'note.md', got %q", artifactPath)
	}
	if filepath.IsAbs(artifactPath) {
		t.Fatalf("artifact path should not be absolute, got %q", artifactPath)
	}
	if strings.Contains(artifactPath, "\\") {
		t.Fatalf("artifact path should not contain backslashes, got %q", artifactPath)
	}
	// Cross-check: artifact path must appear in approval affectedPaths so the
	// two口径 stay consistent end-to-end.
	pathInApproval := false
	for _, p := range approvalPaths {
		if p == artifactPath {
			pathInApproval = true
			break
		}
	}
	if !pathInApproval {
		t.Fatalf("artifact path %q not found in approval affectedPaths %v", artifactPath, approvalPaths)
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
		t.Fatalf("expected note.md to contain 'world', got %q", string(content))
	}
}

// TestMultiTurnThreadSmoke 通过真实 LLM 调用验证同一 thread 多轮对话：
// 第一轮创建 a.txt（内容 hello），第二轮在同 thread 中追加 world，
// 验证 agent 复用上下文识别上一轮文件；并验证未知 thread_id 返回 404。
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
	aTxtPath := filepath.Join(tempDir, "a.txt")

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
				return agent.RunStreamWithEvents(ctx, input, nil, sink)
			})
		},
	)

	ts := httptest.NewServer(NewRouter(runService, nil))
	defer ts.Close()

	// ---------- 第一轮：创建 a.txt，内容 hello（不带 thread_id）----------
	firstReqBody := fmt.Sprintf(`{
		"input": "创建一个 a.txt 文件，内容是 hello",
		"workspace": { "workspace_root": %q, "writable_roots": [%q] },
		"client": { "session_id": "smoke-multiturn-1", "request_id": "smoke-req-multiturn-1", "entrypoint": "smoke" }
	}`, tempDir, tempDir)

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

	// 验证 a.txt 已创建且内容为 hello
	content, err := os.ReadFile(aTxtPath)
	if err != nil {
		t.Fatalf("failed to read a.txt after first run: %v", err)
	}
	if strings.TrimSpace(string(content)) != "hello" {
		t.Fatalf("expected a.txt to contain 'hello' after first run, got %q", string(content))
	}
	t.Logf("a.txt after first run: %q", string(content))

	// ---------- 第二轮：同 thread_id，追加 world ----------
	secondReqBody := fmt.Sprintf(`{
		"input": "在 a.txt 末尾追加 world",
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
	for _, ev := range secondEvents {
		t.Logf("second run event: type=%s tool=%s status=%s outcome=%v",
			ev.Type, ev.ToolCallName, ev.Status, ev.Outcome)
		switch ev.Type {
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

	// 验证 a.txt 现在同时包含 hello 和 world（agent 应能从复用的上下文中识别 a.txt）
	content, err = os.ReadFile(aTxtPath)
	if err != nil {
		t.Fatalf("failed to read a.txt after second run: %v", err)
	}
	lowerContent := strings.ToLower(string(content))
	if !strings.Contains(lowerContent, "hello") || !strings.Contains(lowerContent, "world") {
		t.Fatalf("expected a.txt to contain both 'hello' and 'world' after second run, got %q", string(content))
	}
	t.Logf("a.txt after second run: %q", string(content))

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
