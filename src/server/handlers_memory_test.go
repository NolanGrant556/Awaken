package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"awaken/src/memory"
	"awaken/src/tool"
	memorytool "awaken/src/tools/memory"
)

func TestMemoryHTTPListsDetailsAndDeletesWithoutEditRoutes(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("AWAKEN_HOME", home)
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(workspace, []string{workspace}))
	ctx = tool.ContextWithToolThreadID(ctx, "thread_agent")
	ctx = tool.ContextWithToolExecutionIdentity(ctx, tool.ToolExecutionIdentity{RunID: "run_agent", ToolCallID: "call_agent", ToolName: "memory"})
	createdJSON, err := (&memorytool.Tool{}).Execute(ctx, `{"action":"create","scope":"project","type":"project","name":"Release policy","description":"Release requires a signed checklist","content":"Run the signed release checklist."}`)
	if err != nil {
		t.Fatal(err)
	}
	store, _ := memory.NewStore(workspace)
	records, err := store.Search("Release", nil, nil, 10)
	if err != nil || len(records) != 1 {
		t.Fatalf("created=%s records=%+v err=%v", createdJSON, records, err)
	}

	router := NewRouterWithWorkspace(nil, nil, nil, nil, nil, workspace)
	listed := httptest.NewRecorder()
	router.ServeHTTP(listed, httptest.NewRequest(http.MethodGet, "/v1/memories?scope=project", nil))
	if listed.Code != http.StatusOK || strings.Contains(listed.Body.String(), "Run the signed") {
		t.Fatalf("list leaked body or failed: %d %s", listed.Code, listed.Body.String())
	}
	detail := httptest.NewRecorder()
	router.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, "/v1/memories/"+records[0].MemoryID, nil))
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "Run the signed") {
		t.Fatalf("detail status=%d body=%s", detail.Code, detail.Body.String())
	}
	deleted := httptest.NewRecorder()
	router.ServeHTTP(deleted, httptest.NewRequest(http.MethodDelete, "/v1/memories/"+records[0].MemoryID, strings.NewReader(`{"expected_revision":1}`)))
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/v1/memories/"+records[0].MemoryID, nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("deleted detail status=%d body=%s", missing.Code, missing.Body.String())
	}
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, "/v1/memories", strings.NewReader(`{}`)))
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s mutation status=%d", method, response.Code)
		}
	}
}

func TestMemoryDreamHTTPUsesMainProfileAndCompletesEmptyScope(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	router := NewRouter(nil, nil)
	started := httptest.NewRecorder()
	router.ServeHTTP(started, httptest.NewRequest(http.MethodPost, "/v1/memory/dream", strings.NewReader(`{"scopes":["user"],"model_profile":{"model_id":"main-model"}}`)))
	if started.Code != http.StatusAccepted {
		t.Fatalf("start status=%d body=%s", started.Code, started.Body.String())
	}
	var task memory.DreamTask
	if err := json.Unmarshal(started.Body.Bytes(), &task); err != nil || task.TaskID == "" {
		t.Fatalf("task=%+v err=%v", task, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/memory/dream/"+task.TaskID, nil))
		if err := json.Unmarshal(response.Body.Bytes(), &task); err != nil {
			t.Fatal(err)
		}
		if task.Status == memory.DreamCompleted {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("dream did not complete: %+v", task)
}

func TestMemorySettingsHTTPKeepsFourSwitchesWithRevisionCAS(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	router := NewRouter(nil, nil)
	initial := httptest.NewRecorder()
	router.ServeHTTP(initial, httptest.NewRequest(http.MethodGet, "/v1/memory/settings", nil))
	if initial.Code != http.StatusOK || !strings.Contains(initial.Body.String(), `"automatic_recall":true`) || !strings.Contains(initial.Body.String(), `"auto_dream":true`) {
		t.Fatalf("initial status=%d body=%s", initial.Code, initial.Body.String())
	}

	updated := httptest.NewRecorder()
	router.ServeHTTP(updated, httptest.NewRequest(http.MethodPatch, "/v1/memory/settings", strings.NewReader(`{"expected_revision":0,"auto_dream":true,"proactive_memory":false}`)))
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"revision":1`) || !strings.Contains(updated.Body.String(), `"auto_dream":true`) || !strings.Contains(updated.Body.String(), `"proactive_memory":false`) {
		t.Fatalf("updated status=%d body=%s", updated.Code, updated.Body.String())
	}

	disabled := httptest.NewRecorder()
	router.ServeHTTP(disabled, httptest.NewRequest(http.MethodPatch, "/v1/memory/settings", strings.NewReader(`{"expected_revision":1,"enabled":false}`)))
	if disabled.Code != http.StatusOK || !strings.Contains(disabled.Body.String(), `"revision":2`) || !strings.Contains(disabled.Body.String(), `"enabled":false`) || !strings.Contains(disabled.Body.String(), `"automatic_recall":false`) || !strings.Contains(disabled.Body.String(), `"proactive_memory":false`) || !strings.Contains(disabled.Body.String(), `"auto_dream":false`) {
		t.Fatalf("disabled status=%d body=%s", disabled.Code, disabled.Body.String())
	}

	reenabled := httptest.NewRecorder()
	router.ServeHTTP(reenabled, httptest.NewRequest(http.MethodPatch, "/v1/memory/settings", strings.NewReader(`{"expected_revision":2,"enabled":true}`)))
	if reenabled.Code != http.StatusOK || !strings.Contains(reenabled.Body.String(), `"revision":3`) || !strings.Contains(reenabled.Body.String(), `"enabled":true`) || !strings.Contains(reenabled.Body.String(), `"automatic_recall":true`) || !strings.Contains(reenabled.Body.String(), `"proactive_memory":true`) || !strings.Contains(reenabled.Body.String(), `"auto_dream":true`) {
		t.Fatalf("reenabled status=%d body=%s", reenabled.Code, reenabled.Body.String())
	}

	conflict := httptest.NewRecorder()
	router.ServeHTTP(conflict, httptest.NewRequest(http.MethodPatch, "/v1/memory/settings", strings.NewReader(`{"expected_revision":2,"enabled":true}`)))
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflict status=%d body=%s", conflict.Code, conflict.Body.String())
	}
}

func TestMemoryHTTPEmptyListIsJSONArray(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	router := NewRouter(nil, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/memories?scope=user", nil))
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "[]" {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
