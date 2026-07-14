package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	awruntime "awaken/src/runtime"
)

func TestProjectLifecycleRESTEmptyCollectionsEncodeAsArrays(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	service := awruntime.NewRunService(nil, nil)
	router := NewRouter(service, nil)

	for _, route := range []string{"/v1/projects", "/v1/project-operations"} {
		response := projectJSONRequest(t, router, http.MethodGet, route, nil, "")
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s: expected 200, got %d: %s", route, response.Code, response.Body.String())
		}
		var values []json.RawMessage
		if err := json.Unmarshal(response.Body.Bytes(), &values); err != nil {
			t.Fatalf("GET %s must return a JSON array, got %q: %v", route, response.Body.String(), err)
		}
		if values == nil || len(values) != 0 {
			t.Fatalf("GET %s: expected [], got %s", route, response.Body.String())
		}
	}
}

func TestProjectLifecycleRESTCreateDetachAndIdempotency(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	service := awruntime.NewRunService(nil, nil)
	router := NewRouter(service, nil)
	workspaceRoot := t.TempDir()

	create := projectJSONRequest(t, router, http.MethodPost, "/v1/projects", map[string]any{
		"workspace_root": workspaceRoot,
		"display_name":   "REST project",
	}, "")
	if create.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", create.Code, create.Body.String())
	}
	var project awruntime.Project
	if err := json.Unmarshal(create.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	if project.ProjectID == "" || project.WorkspaceRoot != workspaceRoot {
		t.Fatalf("unexpected project response: %+v", project)
	}

	missingKey := projectJSONRequest(t, router, http.MethodDelete, "/v1/projects/"+project.ProjectID, nil, "")
	if missingKey.Code != http.StatusBadRequest {
		t.Fatalf("expected missing idempotency key 400, got %d", missingKey.Code)
	}
	var apiError errorResponse
	if err := json.Unmarshal(missingKey.Body.Bytes(), &apiError); err != nil {
		t.Fatal(err)
	}
	if apiError.Error != "IDEMPOTENCY_KEY_REQUIRED" {
		t.Fatalf("unexpected error: %+v", apiError)
	}

	first := projectJSONRequest(t, router, http.MethodDelete, "/v1/projects/"+project.ProjectID, nil, "detach-rest-1")
	second := projectJSONRequest(t, router, http.MethodDelete, "/v1/projects/"+project.ProjectID, nil, "detach-rest-1")
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("detach failed: first=%d %s second=%d %s", first.Code, first.Body.String(), second.Code, second.Body.String())
	}
	var firstOperation, secondOperation awruntime.ProjectOperation
	_ = json.Unmarshal(first.Body.Bytes(), &firstOperation)
	_ = json.Unmarshal(second.Body.Bytes(), &secondOperation)
	if firstOperation.OperationID == "" || firstOperation.OperationID != secondOperation.OperationID {
		t.Fatalf("expected idempotent operation response: %+v %+v", firstOperation, secondOperation)
	}
}

func TestProjectLifecycleRESTTrashPrepareClaimFail(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	service := awruntime.NewRunService(nil, nil)
	router := NewRouter(service, nil)
	workspaceRoot := t.TempDir()
	create := projectJSONRequest(t, router, http.MethodPost, "/v1/projects", map[string]any{"workspace_root": workspaceRoot}, "")
	var project awruntime.Project
	if err := json.Unmarshal(create.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}

	prepare := projectJSONRequest(t, router, http.MethodPost, "/v1/projects/"+project.ProjectID+"/trash", nil, "trash-rest-1")
	if prepare.Code != http.StatusAccepted {
		t.Fatalf("expected 202 prepare, got %d: %s", prepare.Code, prepare.Body.String())
	}
	var operation awruntime.ProjectOperation
	if err := json.Unmarshal(prepare.Body.Bytes(), &operation); err != nil {
		t.Fatal(err)
	}
	if operation.Action != awruntime.ProjectOperationTrashAndPurge || operation.Phase != "waiting_for_runs" {
		t.Fatalf("unexpected prepare response: %+v", operation)
	}

	claim := projectJSONRequest(t, router, http.MethodPost, "/v1/project-operations/"+operation.OperationID+"/claim-trash", map[string]any{
		"worker_id":     "desktop-main",
		"lease_seconds": 30,
	}, "")
	if claim.Code != http.StatusOK {
		t.Fatalf("expected claim 200, got %d: %s", claim.Code, claim.Body.String())
	}
	var claimed awruntime.ProjectOperation
	if err := json.Unmarshal(claim.Body.Bytes(), &claimed); err != nil {
		t.Fatal(err)
	}
	if claimed.AttemptID == "" || claimed.Phase != "trash_claimed" {
		t.Fatalf("claim must return a durable attempt token: %+v", claimed)
	}
	missingAttempt := projectJSONRequest(t, router, http.MethodPost, "/v1/project-operations/"+operation.OperationID+"/attest-trash", nil, "")
	if missingAttempt.Code != http.StatusConflict {
		t.Fatalf("expected missing attempt to be rejected, got %d: %s", missingAttempt.Code, missingAttempt.Body.String())
	}
	attemptBody := map[string]any{"attempt_id": claimed.AttemptID}
	attest := projectJSONRequest(t, router, http.MethodPost, "/v1/project-operations/"+operation.OperationID+"/attest-trash", attemptBody, "")
	if attest.Code != http.StatusOK {
		t.Fatalf("expected path attestation 200, got %d: %s", attest.Code, attest.Body.String())
	}
	stage := projectJSONRequest(t, router, http.MethodPost, "/v1/project-operations/"+operation.OperationID+"/stage-trash", attemptBody, "")
	if stage.Code != http.StatusOK {
		t.Fatalf("expected atomic trash staging 200, got %d: %s", stage.Code, stage.Body.String())
	}
	var staged awruntime.ProjectOperation
	if err := json.Unmarshal(stage.Body.Bytes(), &staged); err != nil {
		t.Fatal(err)
	}
	if staged.Phase != "trash_staged" || staged.TrashStagingPath == "" {
		t.Fatalf("expected durable trash staging path: %+v", staged)
	}
	begin := projectJSONRequest(t, router, http.MethodPost, "/v1/project-operations/"+operation.OperationID+"/begin-trash", attemptBody, "")
	if begin.Code != http.StatusOK {
		t.Fatalf("expected durable trash invocation 200, got %d: %s", begin.Code, begin.Body.String())
	}

	fail := projectJSONRequest(t, router, http.MethodPost, "/v1/project-operations/"+operation.OperationID+"/fail-trash", map[string]any{
		"attempt_id": claimed.AttemptID,
		"code":       "TRASH_FAILED",
		"message":    "injected failure",
	}, "")
	if fail.Code != http.StatusOK {
		t.Fatalf("expected fail callback 200, got %d: %s", fail.Code, fail.Body.String())
	}
	var failed awruntime.ProjectOperation
	if err := json.Unmarshal(fail.Body.Bytes(), &failed); err != nil {
		t.Fatal(err)
	}
	if failed.Status != awruntime.ProjectOperationFailed || failed.ErrorCode != "TRASH_FAILED" {
		t.Fatalf("unexpected failed operation: %+v", failed)
	}
}

func projectJSONRequest(t *testing.T, router http.Handler, method, path string, body any, idempotencyKey string) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}
