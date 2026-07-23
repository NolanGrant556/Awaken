package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	awruntime "awaken/src/runtime"
)

type fakeACPProjectService struct {
	projects map[string]awruntime.Project
}

func (f *fakeACPProjectService) UpsertProject(workspaceRoot, displayName string) (awruntime.Project, error) {
	root := filepath.Clean(workspaceRoot)
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
	}
	if p, ok := f.projects[root]; ok {
		return p, nil
	}
	p := awruntime.Project{
		ProjectID:     "project_" + root,
		WorkspaceRoot: root,
		DisplayName:   displayName,
		State:         awruntime.ProjectStateActive,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
		Version:       1,
	}
	f.projects[root] = p
	return p, nil
}

func TestControlACPSessionCreateAndClose(t *testing.T) {
	userDataDir := t.TempDir()
	id, err := awruntime.GenerateRuntimeIdentity(userDataDir, "desktop", "http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}

	projectService := &fakeACPProjectService{projects: make(map[string]awruntime.Project)}
	acpSessionManager := awruntime.NewACPSessionManager(nil, projectService)
	control := &ControlConfig{
		Identity:          id,
		ACPSessionManager: acpSessionManager,
	}

	router := NewRouterWithControl(nil, NewLLMRuntimeInfoProvider("", "", "", "", ""), nil, nil, nil, nil, "", nil, control)

	// Create session.
	cwd := t.TempDir()
	createBody, _ := json.Marshal(map[string]any{
		"acp_session_id": "session_1",
		"cwd":            cwd,
		"visibility":     "visible",
		"client_name":    "test-client",
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
	if createResp["acp_session_id"] != "session_1" {
		t.Fatalf("unexpected session id: %v", createResp["acp_session_id"])
	}

	// Get session.
	req = httptest.NewRequest(http.MethodGet, "/v1/control/acp/sessions/session_1", nil)
	req.Header.Set("Authorization", "Bearer "+id.ControlToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get session: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Close session.
	req = httptest.NewRequest(http.MethodPost, "/v1/control/acp/sessions/session_1/close", nil)
	req.Header.Set("Authorization", "Bearer "+id.ControlToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("close session: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestControlACPSessionRequiresToken(t *testing.T) {
	userDataDir := t.TempDir()
	id, _ := awruntime.GenerateRuntimeIdentity(userDataDir, "desktop", "http://127.0.0.1:1")
	control := &ControlConfig{Identity: id, ACPSessionManager: awruntime.NewACPSessionManager(nil, nil)}
	router := NewRouterWithControl(nil, NewLLMRuntimeInfoProvider("", "", "", "", ""), nil, nil, nil, nil, "", nil, control)

	req := httptest.NewRequest(http.MethodGet, "/v1/control/acp/sessions/session_1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
