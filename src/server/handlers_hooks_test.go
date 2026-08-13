package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"awaken/src/agentcore"
	"awaken/src/hookruntime"
	awruntime "awaken/src/runtime"
)

func TestHookStatusTrustAndRevokeRoutes(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	service := awruntime.NewRunService(func(workspace awruntime.WorkspaceConfig, threadID string) *agentcore.Agent {
		return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
	}, func(*agentcore.Agent) awruntime.Executor {
		return awruntime.ExecutorFunc(func(_ context.Context, _ string, _ agentcore.EventSink) (string, error) { return "ok", nil })
	})
	store, err := hookruntime.NewStore(os.Getenv("AWAKEN_HOME"))
	if err != nil {
		t.Fatal(err)
	}
	service.SetHookStore(store)
	projectRoot := t.TempDir()
	project, err := service.ProjectService().CreateProject([]awruntime.ProjectFolder{{Path: projectRoot, IsPrimary: true}}, "hooks")
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(projectRoot, ".awaken", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	config := `{"version":1,"enabled":true,"hooks":{"RunStart":[{"matcher":{"toolNames":[]},"hooks":[{"id":"hello","type":"command","command":["/usr/bin/true"],"cwd":"project_root","timeoutMs":1000,"maxOutputBytes":1024,"onFailure":"continue"}]}]}}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	router := NewRouter(service, nil)

	statusRecorder := httptest.NewRecorder()
	router.ServeHTTP(statusRecorder, httptest.NewRequest(http.MethodGet, "/v1/hooks?project_id="+project.ProjectID, nil))
	if statusRecorder.Code != http.StatusOK {
		t.Fatalf("status = %d %s", statusRecorder.Code, statusRecorder.Body.String())
	}
	var status hookStatusResponse
	if err := json.Unmarshal(statusRecorder.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Sources) != 1 || status.Sources[0].Status != hookruntime.StatusUntrusted {
		t.Fatalf("status = %+v", status)
	}
	if len(status.Sources[0].Hooks) != 1 || status.Sources[0].Hooks[0].ID != "hello" || status.Sources[0].Hooks[0].Event != hookruntime.EventRunStart {
		t.Fatalf("review details = %+v", status.Sources[0].Hooks)
	}
	if got := status.Sources[0].Hooks[0].Command; len(got) != 1 || got[0] != "/usr/bin/true" {
		t.Fatalf("review command = %#v", got)
	}
	var raw map[string]any
	if err := json.Unmarshal(statusRecorder.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, exposed := raw["hooksByEvent"]; exposed {
		t.Fatal("status API exposed compiled hook commands")
	}

	requestBody, _ := json.Marshal(map[string]string{"projectId": project.ProjectID, "sourcePath": status.Sources[0].Path, "digest": status.Sources[0].Digest})
	trustRecorder := httptest.NewRecorder()
	router.ServeHTTP(trustRecorder, httptest.NewRequest(http.MethodPost, "/v1/hooks/trust", bytes.NewReader(requestBody)))
	if trustRecorder.Code != http.StatusOK {
		t.Fatalf("trust = %d %s", trustRecorder.Code, trustRecorder.Body.String())
	}
	if err := json.Unmarshal(trustRecorder.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Sources[0].Status != hookruntime.StatusEnabled {
		t.Fatalf("trusted = %+v", status.Sources[0])
	}
	if status.EnabledHookCount != 1 {
		t.Fatalf("enabled hook count = %d", status.EnabledHookCount)
	}

	revokeRecorder := httptest.NewRecorder()
	router.ServeHTTP(revokeRecorder, httptest.NewRequest(http.MethodPost, "/v1/hooks/revoke", bytes.NewReader(requestBody)))
	if revokeRecorder.Code != http.StatusOK {
		t.Fatalf("revoke = %d %s", revokeRecorder.Code, revokeRecorder.Body.String())
	}
	if err := json.Unmarshal(revokeRecorder.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Sources[0].Status != hookruntime.StatusUntrusted {
		t.Fatalf("revoked = %+v", status.Sources[0])
	}
}

func TestHookStatusWithoutSourcesReturnsJSONArray(t *testing.T) {
	service := awruntime.NewRunService(nil, nil)
	store, err := hookruntime.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service.SetHookStore(store)
	router := NewRouter(service, nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/hooks", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d %s", recorder.Code, recorder.Body.String())
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["sources"]) != "[]" {
		t.Fatalf("sources = %s", raw["sources"])
	}
}
