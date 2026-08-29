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

func TestProjectLifecycleRESTRenamesByStableIDWithoutIdempotencyKey(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	service := awruntime.NewRunService(nil, nil)
	router := NewRouter(service, nil)

	create := projectJSONRequest(t, router, http.MethodPost, "/v1/projects", map[string]any{
		"display_name": "Before",
	}, "")
	if create.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d: %s", create.Code, create.Body.String())
	}
	var project awruntime.Project
	if err := json.Unmarshal(create.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}

	rename := projectJSONRequest(t, router, http.MethodPatch, "/v1/projects/"+project.ProjectID, map[string]any{
		"display_name": "  After  ",
	}, "")
	if rename.Code != http.StatusOK {
		t.Fatalf("expected rename without idempotency key to return 200, got %d: %s", rename.Code, rename.Body.String())
	}
	var renamed awruntime.Project
	if err := json.Unmarshal(rename.Body.Bytes(), &renamed); err != nil {
		t.Fatal(err)
	}
	if renamed.ProjectID != project.ProjectID || renamed.DisplayName != "After" || renamed.WorkspaceRoot != "" || len(renamed.Folders) != 0 {
		t.Fatalf("unexpected renamed project: before=%+v after=%+v", project, renamed)
	}

	blank := projectJSONRequest(t, router, http.MethodPatch, "/v1/projects/"+project.ProjectID, map[string]any{
		"display_name": " \t ",
	}, "")
	assertProjectRESTError(t, blank, http.StatusBadRequest, "PROJECT_NAME_INVALID")

	missing := projectJSONRequest(t, router, http.MethodPatch, "/v1/projects/project_missing", map[string]any{
		"display_name": "Exists",
	}, "")
	assertProjectRESTError(t, missing, http.StatusNotFound, "PROJECT_NOT_FOUND")

	trashRoot := t.TempDir()
	createPending := projectJSONRequest(t, router, http.MethodPost, "/v1/projects", map[string]any{
		"workspace_root": trashRoot,
		"display_name":   "Pending",
	}, "")
	if createPending.Code != http.StatusCreated {
		t.Fatalf("expected pending project create 201, got %d: %s", createPending.Code, createPending.Body.String())
	}
	var pending awruntime.Project
	if err := json.Unmarshal(createPending.Body.Bytes(), &pending); err != nil {
		t.Fatal(err)
	}
	prepare := projectJSONRequest(t, router, http.MethodPost, "/v1/projects/"+pending.ProjectID+"/folders/trash", map[string]any{
		"path": trashRoot,
	}, "rename-pending-rest-1")
	if prepare.Code != http.StatusAccepted {
		t.Fatalf("expected trash prepare 202, got %d: %s", prepare.Code, prepare.Body.String())
	}
	conflict := projectJSONRequest(t, router, http.MethodPatch, "/v1/projects/"+pending.ProjectID, map[string]any{
		"display_name": "Must not change",
	}, "")
	assertProjectRESTError(t, conflict, http.StatusConflict, "PROJECT_OPERATION_CONFLICT")
}

func TestProjectLifecycleRESTReordersEveryActiveProject(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	service := awruntime.NewRunService(nil, nil)
	router := NewRouter(service, nil)

	projects := make([]awruntime.Project, 0, 3)
	for _, displayName := range []string{"first", "second", "third"} {
		create := projectJSONRequest(t, router, http.MethodPost, "/v1/projects", map[string]any{
			"workspace_root": t.TempDir(),
			"display_name":   displayName,
		}, "")
		if create.Code != http.StatusCreated {
			t.Fatalf("create %s: %d %s", displayName, create.Code, create.Body.String())
		}
		var project awruntime.Project
		if err := json.Unmarshal(create.Body.Bytes(), &project); err != nil {
			t.Fatal(err)
		}
		projects = append(projects, project)
	}

	wanted := []string{projects[0].ProjectID, projects[2].ProjectID, projects[1].ProjectID}
	reorder := projectJSONRequest(t, router, http.MethodPut, "/v1/projects/order", map[string]any{
		"project_ids": wanted,
	}, "")
	if reorder.Code != http.StatusOK {
		t.Fatalf("expected reorder 200, got %d: %s", reorder.Code, reorder.Body.String())
	}
	var ordered []awruntime.Project
	if err := json.Unmarshal(reorder.Body.Bytes(), &ordered); err != nil {
		t.Fatal(err)
	}
	for index, projectID := range wanted {
		if ordered[index].ProjectID != projectID {
			t.Fatalf("unexpected order at %d: got %s want %s", index, ordered[index].ProjectID, projectID)
		}
	}

	invalid := projectJSONRequest(t, router, http.MethodPut, "/v1/projects/order", map[string]any{
		"project_ids": wanted[:2],
	}, "")
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("expected incomplete order 400, got %d: %s", invalid.Code, invalid.Body.String())
	}
	var apiError errorResponse
	if err := json.Unmarshal(invalid.Body.Bytes(), &apiError); err != nil {
		t.Fatal(err)
	}
	if apiError.Error != "PROJECT_ORDER_INVALID" {
		t.Fatalf("unexpected reorder error: %+v", apiError)
	}
}

func TestProjectFoldersRESTCreateAttachSwitchPrimaryAndDetach(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	service := awruntime.NewRunService(nil, nil)
	router := NewRouter(service, nil)
	first, second, third := t.TempDir(), t.TempDir(), t.TempDir()

	create := projectJSONRequest(t, router, http.MethodPost, "/v1/projects", map[string]any{
		"folders": []map[string]any{
			{"path": first},
			{"path": second, "primary": true},
		},
		"display_name": "multi folder",
	}, "")
	if create.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", create.Code, create.Body.String())
	}
	var project awruntime.Project
	if err := json.Unmarshal(create.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	if len(project.Folders) != 2 || project.WorkspaceRoot != second {
		t.Fatalf("unexpected created project: %+v", project)
	}

	attach := projectJSONRequest(t, router, http.MethodPost, "/v1/projects/"+project.ProjectID+"/folders", map[string]any{
		"path": third,
	}, "")
	if attach.Code != http.StatusOK {
		t.Fatalf("expected attach 200, got %d: %s", attach.Code, attach.Body.String())
	}
	if err := json.Unmarshal(attach.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	if len(project.Folders) != 3 {
		t.Fatalf("attach did not add the folder: %+v", project.Folders)
	}

	primary := projectJSONRequest(t, router, http.MethodPut, "/v1/projects/"+project.ProjectID+"/primary-folder", map[string]any{
		"path": third,
	}, "")
	if primary.Code != http.StatusOK {
		t.Fatalf("expected primary switch 200, got %d: %s", primary.Code, primary.Body.String())
	}
	if err := json.Unmarshal(primary.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	if project.WorkspaceRoot != third {
		t.Fatalf("primary switch did not take effect: %+v", project)
	}

	detach := projectJSONRequest(t, router, http.MethodDelete, "/v1/projects/"+project.ProjectID+"/folders", map[string]any{
		"path": first,
	}, "")
	if detach.Code != http.StatusOK {
		t.Fatalf("expected detach 200, got %d: %s", detach.Code, detach.Body.String())
	}
	if err := json.Unmarshal(detach.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	if len(project.Folders) != 2 || project.HasFolder(first) {
		t.Fatalf("detach did not unbind the folder: %+v", project.Folders)
	}

	// A folder is not owned exclusively: creating a second project on a folder
	// the first one still holds succeeds, and both keep it.
	shared := projectJSONRequest(t, router, http.MethodPost, "/v1/projects", map[string]any{
		"folders":      []map[string]any{{"path": third}},
		"display_name": "shares the folder",
	}, "")
	if shared.Code != http.StatusCreated {
		t.Fatalf("expected shared folder 201, got %d: %s", shared.Code, shared.Body.String())
	}
	var sharedProject awruntime.Project
	if err := json.Unmarshal(shared.Body.Bytes(), &sharedProject); err != nil {
		t.Fatal(err)
	}
	if !sharedProject.HasFolder(third) {
		t.Fatalf("the shared folder must join the new project: %+v", sharedProject.Folders)
	}
	if sharedProject.ProjectID == project.ProjectID {
		t.Fatalf("sharing a folder must mint a new project, got %q", sharedProject.ProjectID)
	}
	listed := projectJSONRequest(t, router, http.MethodGet, "/v1/projects", nil, "")
	var projects []awruntime.Project
	if err := json.Unmarshal(listed.Body.Bytes(), &projects); err != nil {
		t.Fatal(err)
	}
	holders := 0
	for _, candidate := range projects {
		if candidate.HasFolder(third) {
			holders++
		}
	}
	if holders != 2 {
		t.Fatalf("both projects must hold the shared folder, got %d holders in %+v", holders, projects)
	}
}

// Creating a project needs nothing but a name, so the sidebar can mint one
// before the user has a folder to bind to it.
func TestProjectFoldersRESTCreatesAProjectFromANameAlone(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	service := awruntime.NewRunService(nil, nil)
	router := NewRouter(service, nil)

	create := projectJSONRequest(t, router, http.MethodPost, "/v1/projects", map[string]any{
		"display_name": "folderless",
	}, "")
	if create.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", create.Code, create.Body.String())
	}
	var project awruntime.Project
	if err := json.Unmarshal(create.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	if len(project.Folders) != 0 {
		t.Fatalf("no folder may be invented for the project, got %+v", project.Folders)
	}
	if project.WorkspaceRoot != "" {
		t.Fatalf("a folderless project must not advertise a root: %+v", project)
	}
	if project.DisplayName != "folderless" || project.ProjectID == "" {
		t.Fatalf("the project must keep its identity: %+v", project)
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

	prepare := projectJSONRequest(t, router, http.MethodPost, "/v1/projects/"+project.ProjectID+"/folders/trash", map[string]any{
		"path": workspaceRoot,
	}, "trash-rest-1")
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

func assertProjectRESTError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("expected %d %s, got %d: %s", status, code, response.Code, response.Body.String())
	}
	var apiError errorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &apiError); err != nil {
		t.Fatal(err)
	}
	if apiError.Error != code {
		t.Fatalf("expected error %s, got %+v", code, apiError)
	}
}
