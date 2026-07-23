package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"awaken/src/acp"
	"awaken/src/agentcore"
	awruntime "awaken/src/runtime"
)

func TestControlACPRunLifecycle(t *testing.T) {
	userDataDir := t.TempDir()
	id, err := awruntime.GenerateRuntimeIdentity(userDataDir, "headless", "http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}

	projectService := &fakeACPProjectService{projects: make(map[string]awruntime.Project)}
	acpSessionManager := awruntime.NewACPSessionManager(nil, projectService)
	control := &ControlConfig{
		Identity:          id,
		ACPSessionManager: acpSessionManager,
	}

	workspaceRoot := t.TempDir()
	release := make(chan struct{})
	started := make(chan struct{})
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextDeltaEventType, MessageID: "msg-1", Role: "assistant", Delta: "hello"})
			close(started)
			select {
			case <-release:
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
				return "done", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		})
	})

	router := NewRouterWithControl(runService, NewLLMRuntimeInfoProvider("", "", "", "", ""), nil, nil, nil, nil, workspaceRoot, nil, control)

	// Create session.
	sessionID := "session_1"
	createBody, _ := json.Marshal(map[string]any{
		"acp_session_id": sessionID,
		"cwd":            workspaceRoot,
		"visibility":     "visible",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/control/acp/sessions", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+id.ControlToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create session: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var createResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	threadID := createResp["thread_id"].(string)

	// Create run.
	runBody, _ := json.Marshal(map[string]any{
		"thread_id": threadID,
		"input":     "hello",
	})
	req = httptest.NewRequest(http.MethodPost, "/v1/control/acp/sessions/"+sessionID+"/runs", bytes.NewReader(runBody))
	req.Header.Set("Authorization", "Bearer "+id.ControlToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create run: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var run awruntime.Run
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	if run.RunID == "" {
		t.Fatalf("expected run id, got: %+v", run)
	}
	// The reserved session thread is created lazily; the actual run thread may
	// differ from the reserved thread ID.
	threadID = run.ThreadID

	// Set current run route is idempotent.
	req = httptest.NewRequest(http.MethodPost, "/v1/control/acp/sessions/"+sessionID+"/current-run/"+run.RunID, nil)
	req.Header.Set("Authorization", "Bearer "+id.ControlToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("set current run: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Stream events.
	<-started
	done := make(chan string, 1)
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/v1/control/acp/sessions/"+sessionID+"/runs/"+run.RunID+"/events", nil)
		req.Header.Set("Authorization", "Bearer "+id.ControlToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		body, _ := io.ReadAll(rec.Body)
		done <- string(body)
	}()

	time.Sleep(50 * time.Millisecond)
	close(release)

	body := <-done
	for _, token := range []string{
		"RUN_STARTED",
		"TEXT_MESSAGE_CONTENT",
		"RUN_FINISHED",
		run.RunID,
		threadID,
	} {
		if !strings.Contains(body, token) {
			t.Fatalf("expected event stream to contain %q, got %s", token, body)
		}
	}

	// Cancel the now-terminal run still returns the run snapshot.
	req = httptest.NewRequest(http.MethodPost, "/v1/control/acp/sessions/"+sessionID+"/runs/"+run.RunID+"/cancel", nil)
	req.Header.Set("Authorization", "Bearer "+id.ControlToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel run: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Clear current run.
	req = httptest.NewRequest(http.MethodDelete, "/v1/control/acp/sessions/"+sessionID+"/current-run", nil)
	req.Header.Set("Authorization", "Bearer "+id.ControlToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear current run: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestControlACPRunRequiresToken(t *testing.T) {
	userDataDir := t.TempDir()
	id, _ := awruntime.GenerateRuntimeIdentity(userDataDir, "headless", "http://127.0.0.1:1")
	control := &ControlConfig{
		Identity:          id,
		ACPSessionManager: awruntime.NewACPSessionManager(nil, nil),
	}
	router := NewRouterWithControl(awruntime.NewRunService(nil, nil), NewLLMRuntimeInfoProvider("", "", "", "", ""), nil, nil, nil, nil, "", nil, control)

	req := httptest.NewRequest(http.MethodPost, "/v1/control/acp/sessions/session_1/runs", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestControlACPResolvePermission(t *testing.T) {
	userDataDir := t.TempDir()
	id, err := awruntime.GenerateRuntimeIdentity(userDataDir, "headless", "http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}

	projectService := &fakeACPProjectService{projects: make(map[string]awruntime.Project)}
	acpSessionManager := awruntime.NewACPSessionManager(nil, projectService)
	control := &ControlConfig{
		Identity:          id,
		ACPSessionManager: acpSessionManager,
	}

	workspaceRoot := t.TempDir()
	runService := awruntime.NewRunService(nil, func(agent *agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			return "done", nil
		})
	})

	registry := acp.NewRegistry(t.TempDir())
	manager := acp.NewManager(registry, func(ctx context.Context, snapshot acp.RunnerSnapshot, cwd string) (acp.Client, error) {
		return nil, errors.New("not needed")
	})
	awaiter := &fakePermissionAwaiter{
		pending: map[string]acp.PendingPermission{
			"ix_1": {
				InteractionID:    "ix_1",
				RunID:            "run_1",
				AllowedOptionIDs: []string{"allow-once"},
			},
		},
	}
	manager.SetPermissionAwaiter(awaiter)

	router := NewRouterWithControl(runService, NewLLMRuntimeInfoProvider("", "", "", "", ""), nil, nil, nil, manager, workspaceRoot, nil, control)

	// Create session and run so current-run matches.
	sessionID := "session_1"
	createBody, _ := json.Marshal(map[string]any{
		"acp_session_id": sessionID,
		"cwd":            workspaceRoot,
		"visibility":     "visible",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/control/acp/sessions", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+id.ControlToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create session: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var createResp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &createResp)
	threadID := createResp["thread_id"].(string)

	// Create a run and capture its ID.
	runBody, _ := json.Marshal(map[string]any{
		"thread_id": threadID,
		"input":     "hello",
	})
	req = httptest.NewRequest(http.MethodPost, "/v1/control/acp/sessions/"+sessionID+"/runs", bytes.NewReader(runBody))
	req.Header.Set("Authorization", "Bearer "+id.ControlToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create run: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var run awruntime.Run
	_ = json.Unmarshal(rec.Body.Bytes(), &run)

	// Update the awaiter with the real run ID.
	awaiter.pending["ix_1"] = acp.PendingPermission{
		InteractionID:    "ix_1",
		RunID:            run.RunID,
		AllowedOptionIDs: []string{"allow-once"},
	}

	resolveBody, _ := json.Marshal(map[string]any{
		"option_id": "allow-once",
		"cancelled": false,
	})
	req = httptest.NewRequest(http.MethodPost, "/v1/control/acp/sessions/"+sessionID+"/runs/"+run.RunID+"/permissions/ix_1/resolve", bytes.NewReader(resolveBody))
	req.Header.Set("Authorization", "Bearer "+id.ControlToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("resolve permission: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

type fakePermissionAwaiter struct {
	pending map[string]acp.PendingPermission
}

func (f *fakePermissionAwaiter) AwaitPermission(ctx context.Context, req acp.PermissionRequest) (acp.PermissionResolution, error) {
	return acp.PermissionResolution{}, nil
}

func (f *fakePermissionAwaiter) Resolve(interactionID string, res acp.PermissionResolution) error {
	return nil
}

func (f *fakePermissionAwaiter) Pending(interactionID string) (acp.PendingPermission, bool) {
	p, ok := f.pending[interactionID]
	return p, ok
}
