package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"awaken/src/agentcore"
	awruntime "awaken/src/runtime"
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
	runService := awruntime.NewRunService(func() awruntime.Executor {
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

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(`{"input":"hello"}`))
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
	runService := awruntime.NewRunService(func() awruntime.Executor {
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

func TestResolveApprovalEndpointPublishesExtensionEvents(t *testing.T) {
	approvalReady := make(chan struct{})
	runService := awruntime.NewRunService(func() awruntime.Executor {
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

	resp, err := http.Post(ts.URL+"/v1/runs", "application/json", strings.NewReader(`{"input":"hello"}`))
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
