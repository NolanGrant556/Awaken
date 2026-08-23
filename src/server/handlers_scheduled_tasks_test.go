package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"awaken/src/scheduledtask"
)

// stubScheduledTaskExecutor is the test Executor: immediate success by
// default, blocking when release is non-nil (Cancel unblocks it as
// cancelled, finish as success), mirroring the RunServiceExecutor contract.
type stubScheduledTaskExecutor struct {
	mu        sync.Mutex
	release   chan struct{}
	cancelled bool
}

func (e *stubScheduledTaskExecutor) ExecuteAgent(ctx context.Context, _ scheduledtask.Task, _ scheduledtask.Execution) (string, string, *scheduledtask.ExecError) {
	if err := e.wait(ctx); err != nil {
		return "", "", err
	}
	return "run-stub", "thread-stub", nil
}

func (e *stubScheduledTaskExecutor) ExecuteReminder(ctx context.Context, _ scheduledtask.Task, _ scheduledtask.Execution) (string, *scheduledtask.ExecError) {
	if err := e.wait(ctx); err != nil {
		return "", err
	}
	return "thread-stub", nil
}

func (e *stubScheduledTaskExecutor) wait(ctx context.Context) *scheduledtask.ExecError {
	e.mu.Lock()
	release := e.release
	e.mu.Unlock()
	if release == nil {
		return nil
	}
	select {
	case <-release:
		e.mu.Lock()
		cancelled := e.cancelled
		e.mu.Unlock()
		if cancelled {
			return &scheduledtask.ExecError{Code: scheduledtask.ErrorCodeRunCancelled, Message: "execution cancelled", Cancelled: true}
		}
		return nil
	case <-ctx.Done():
		return &scheduledtask.ExecError{Code: scheduledtask.ErrorCodeRunCancelled, Message: "execution cancelled", Cancelled: true}
	}
}

func (e *stubScheduledTaskExecutor) Cancel(string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cancelled = true
	if e.release != nil {
		close(e.release)
		e.release = nil
	}
}

type scheduledTaskFixture struct {
	service  *scheduledtask.Service
	router   http.Handler
	executor *stubScheduledTaskExecutor
	repo     *scheduledtask.FileRepository
}

func newScheduledTaskFixture(t *testing.T) *scheduledTaskFixture {
	t.Helper()
	repo, err := scheduledtask.OpenFileRepository(t.TempDir(), scheduledtask.RealClock{})
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	executor := &stubScheduledTaskExecutor{}
	validator := scheduledtask.Validator{}
	service := scheduledtask.NewService(repo, validator, executor)
	scheduler := scheduledtask.NewScheduler(scheduledtask.SchedulerConfig{
		Repo:      repo,
		Executor:  executor,
		Validator: validator,
		Emit:      service.PublishEvent,
	})
	service.SetScheduler(scheduler)
	if err := scheduler.Start(); err != nil {
		t.Fatalf("start scheduler: %v", err)
	}
	t.Cleanup(func() {
		scheduler.Stop()
		if err := service.Close(); err != nil {
			t.Fatalf("close service: %v", err)
		}
	})
	return &scheduledTaskFixture{
		service:  service,
		router:   NewRouterWithScheduledTasks(nil, nil, nil, nil, nil, nil, "", nil, nil, service, nil),
		executor: executor,
		repo:     repo,
	}
}

func scheduledTaskRequestBody(content string) map[string]any {
	return map[string]any{
		"name":       "测试任务",
		"enabled":    true,
		"project_id": "project-1",
		"schedule": map[string]any{
			"kind":     "cron",
			"cron":     "0 9 * * *",
			"timezone": "Asia/Shanghai",
		},
		"action": map[string]any{
			"kind":    "reminder",
			"content": content,
		},
		"execution": map[string]any{
			"profile_id":            "",
			"timeout_seconds":       600,
			"misfire_grace_seconds": 300,
		},
	}
}

func doJSON(t *testing.T, router http.Handler, method, path string, body any, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(payload)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
	return decoded
}

func createScheduledTask(t *testing.T, router http.Handler, body map[string]any) map[string]any {
	t.Helper()
	rec := doJSON(t, router, http.MethodPost, "/v1/scheduled-tasks", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d body %s", rec.Code, rec.Body.String())
	}
	return decodeBody(t, rec)
}

func waitForTerminalExecution(t *testing.T, f *scheduledTaskFixture, taskID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		executions, err := f.service.ListExecutions(taskID, 1)
		if err == nil && len(executions) > 0 && executions[0].Status.Terminal() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("execution of %s did not reach a terminal state", taskID)
}

func TestScheduledTasksUnavailableWithoutService(t *testing.T) {
	router := NewRouterWithScheduledTasks(nil, nil, nil, nil, nil, nil, "", nil, nil, nil, nil)
	for _, path := range []string{
		"/v1/scheduled-tasks",
		"/v1/scheduled-tasks/events",
		"/v1/scheduled-tasks/sched_x",
		"/v1/scheduled-tasks/sched_x/executions",
	} {
		rec := doJSON(t, router, http.MethodGet, path, nil)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("GET %s: expected 503, got %d", path, rec.Code)
		}
		body := decodeBody(t, rec)
		if body["error"] != "SCHEDULED_TASKS_UNAVAILABLE" {
			t.Fatalf("GET %s: unexpected error body %v", path, body)
		}
	}
}

func TestScheduledTaskCreateListGetWireShape(t *testing.T) {
	f := newScheduledTaskFixture(t)

	created := createScheduledTask(t, f.router, scheduledTaskRequestBody("喝水提醒"))

	// snake_case wire keys with the materialized projections.
	for _, key := range []string{"task_id", "version", "name", "enabled", "project_id", "schedule", "action", "execution", "state", "next_run_at", "last_execution", "created_at", "updated_at"} {
		if _, ok := created[key]; !ok {
			t.Fatalf("create response missing key %q: %v", key, created)
		}
	}
	for _, key := range []string{"taskId", "nextRunAt", "lastExecution", "createdAt"} {
		if _, ok := created[key]; ok {
			t.Fatalf("create response must not carry camelCase key %q", key)
		}
	}
	if !strings.HasPrefix(created["task_id"].(string), "sched_") {
		t.Fatalf("unexpected task id %v", created["task_id"])
	}
	if created["state"] != "enabled" {
		t.Fatalf("expected state enabled, got %v", created["state"])
	}
	if created["next_run_at"] == nil {
		t.Fatalf("enabled cron task must project next_run_at")
	}
	if created["last_execution"] != nil {
		t.Fatalf("fresh task must have null last_execution, got %v", created["last_execution"])
	}
	taskID := created["task_id"].(string)

	// List wraps the same item shape.
	rec := doJSON(t, f.router, http.MethodGet, "/v1/scheduled-tasks", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rec.Code)
	}
	listed := decodeBody(t, rec)
	tasks, ok := listed["tasks"].([]any)
	if !ok || len(tasks) != 1 {
		t.Fatalf("expected one task in list, got %v", listed)
	}
	if tasks[0].(map[string]any)["task_id"] != taskID {
		t.Fatalf("list item task id mismatch: %v", tasks[0])
	}

	// Get returns the single item shape.
	rec = doJSON(t, f.router, http.MethodGet, "/v1/scheduled-tasks/"+taskID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", rec.Code)
	}
	if decodeBody(t, rec)["task_id"] != taskID {
		t.Fatalf("get returned wrong task")
	}

	// A paused task has no next run.
	pausedBody := scheduledTaskRequestBody("暂停任务")
	pausedBody["enabled"] = false
	paused := createScheduledTask(t, f.router, pausedBody)
	if paused["state"] != "paused" {
		t.Fatalf("expected state paused, got %v", paused["state"])
	}
	if value, present := paused["next_run_at"]; !present || value != nil {
		t.Fatalf("paused task must project next_run_at null, got %v (present=%v)", value, present)
	}

	// Unknown task → 404 TASK_NOT_FOUND.
	rec = doJSON(t, f.router, http.MethodGet, "/v1/scheduled-tasks/sched_missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get missing: expected 404, got %d", rec.Code)
	}
	if decodeBody(t, rec)["error"] != "TASK_NOT_FOUND" {
		t.Fatalf("get missing: unexpected body %s", rec.Body.String())
	}
}

func TestScheduledTaskCreatePreservesAgentComposerTokens(t *testing.T) {
	f := newScheduledTaskFixture(t)
	body := scheduledTaskRequestBody("")
	body["action"] = map[string]any{
		"kind":    "agent",
		"content": "生成日报",
		"composer_tokens": []any{map[string]any{
			"id":          "skill:daily-report:project",
			"type":        "skill",
			"leadingText": "请使用 ",
			"label":       "daily-report",
			"payload": map[string]any{
				"skillId":      "daily-report",
				"sourceOrigin": "project",
			},
		}},
	}

	created := createScheduledTask(t, f.router, body)
	action, ok := created["action"].(map[string]any)
	if !ok {
		t.Fatalf("action response = %#v", created["action"])
	}
	tokens, ok := action["composer_tokens"].([]any)
	if !ok || len(tokens) != 1 {
		t.Fatalf("composer_tokens response = %#v", action["composer_tokens"])
	}
	token := tokens[0].(map[string]any)
	payload := token["payload"].(map[string]any)
	if token["id"] != "skill:daily-report:project" ||
		token["leadingText"] != "请使用 " ||
		payload["skillId"] != "daily-report" ||
		payload["sourceOrigin"] != "project" {
		t.Fatalf("composer token identity changed: %#v", token)
	}
}

func TestScheduledTaskCreateValidationAndIdentity(t *testing.T) {
	f := newScheduledTaskFixture(t)

	// Field validation failure → 422 with a field-level message that never
	// carries the task content.
	body := scheduledTaskRequestBody("SECRET-CONTENT-MARKER")
	body["name"] = ""
	rec := doJSON(t, f.router, http.MethodPost, "/v1/scheduled-tasks", body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid create: expected 422, got %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid name") {
		t.Fatalf("422 message must name the field, got %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "SECRET-CONTENT-MARKER") {
		t.Fatalf("error body must not contain task content: %s", rec.Body.String())
	}
	if decodeBody(t, rec)["error"] != "VALIDATION_FAILED" {
		t.Fatalf("unexpected error code in %s", rec.Body.String())
	}

	// Client-supplied identity/timestamps are rejected as unknown fields.
	body = scheduledTaskRequestBody("x")
	body["task_id"] = "sched_forged"
	rec = doJSON(t, f.router, http.MethodPost, "/v1/scheduled-tasks", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("forged identity: expected 400, got %d", rec.Code)
	}

	// An invalid timezone is a field error, not a 500.
	body = scheduledTaskRequestBody("x")
	body["schedule"].(map[string]any)["timezone"] = "Mars/Olympus"
	rec = doJSON(t, f.router, http.MethodPost, "/v1/scheduled-tasks", body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad timezone: expected 422, got %d body %s", rec.Code, rec.Body.String())
	}
}

func TestScheduledTaskPatchVersionCAS(t *testing.T) {
	f := newScheduledTaskFixture(t)
	created := createScheduledTask(t, f.router, scheduledTaskRequestBody("原始"))
	taskID := created["task_id"].(string)
	version := int64(created["version"].(float64))

	// Missing expected_version → 400.
	rec := doJSON(t, f.router, http.MethodPatch, "/v1/scheduled-tasks/"+taskID, map[string]any{"name": "改名"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("patch without expected_version: expected 400, got %d", rec.Code)
	}

	// Stale version → 409 VERSION_CONFLICT.
	rec = doJSON(t, f.router, http.MethodPatch, "/v1/scheduled-tasks/"+taskID, map[string]any{
		"expected_version": version + 5,
		"name":             "改名",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("stale patch: expected 409, got %d body %s", rec.Code, rec.Body.String())
	}
	if decodeBody(t, rec)["error"] != "VERSION_CONFLICT" {
		t.Fatalf("stale patch: unexpected body %s", rec.Body.String())
	}

	// Valid patch → 200 with the updated wire item.
	rec = doJSON(t, f.router, http.MethodPatch, "/v1/scheduled-tasks/"+taskID, map[string]any{
		"expected_version": version,
		"name":             "改名后",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: expected 200, got %d body %s", rec.Code, rec.Body.String())
	}
	patched := decodeBody(t, rec)
	if patched["name"] != "改名后" || int64(patched["version"].(float64)) != version+1 {
		t.Fatalf("patch did not apply: %v", patched)
	}

	// Patch of an unknown task → 404.
	rec = doJSON(t, f.router, http.MethodPatch, "/v1/scheduled-tasks/sched_missing", map[string]any{
		"expected_version": 1,
		"name":             "x",
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("patch missing: expected 404, got %d", rec.Code)
	}
}

func TestScheduledTaskPauseResume(t *testing.T) {
	f := newScheduledTaskFixture(t)
	created := createScheduledTask(t, f.router, scheduledTaskRequestBody("启停"))
	taskID := created["task_id"].(string)
	version := int64(created["version"].(float64))

	rec := doJSON(t, f.router, http.MethodPost, "/v1/scheduled-tasks/"+taskID+"/pause", map[string]any{
		"expected_version": version,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("pause: expected 200, got %d body %s", rec.Code, rec.Body.String())
	}
	paused := decodeBody(t, rec)
	if paused["enabled"] != false || paused["state"] != "paused" {
		t.Fatalf("pause did not apply: %v", paused)
	}

	rec = doJSON(t, f.router, http.MethodPost, "/v1/scheduled-tasks/"+taskID+"/resume", map[string]any{
		"expected_version": version, // stale after pause
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("resume with stale version: expected 409, got %d", rec.Code)
	}

	rec = doJSON(t, f.router, http.MethodPost, "/v1/scheduled-tasks/"+taskID+"/resume", map[string]any{
		"expected_version": version + 1,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("resume: expected 200, got %d body %s", rec.Code, rec.Body.String())
	}
	if decodeBody(t, rec)["enabled"] != true {
		t.Fatalf("resume did not apply: %s", rec.Body.String())
	}
}

func TestScheduledTaskDelete(t *testing.T) {
	f := newScheduledTaskFixture(t)
	created := createScheduledTask(t, f.router, scheduledTaskRequestBody("待删除"))
	taskID := created["task_id"].(string)
	version := int64(created["version"].(float64))

	// expected_version rides the query string.
	rec := doJSON(t, f.router, http.MethodDelete, "/v1/scheduled-tasks/"+taskID, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("delete without expected_version: expected 400, got %d", rec.Code)
	}

	rec = doJSON(t, f.router, http.MethodDelete, fmt.Sprintf("/v1/scheduled-tasks/%s?expected_version=%d", taskID, version+9), nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete with stale version: expected 409, got %d body %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, f.router, http.MethodDelete,
		fmt.Sprintf("/v1/scheduled-tasks/%s?expected_version=%d", taskID, version),
		nil, "Idempotency-Key", "delete-key-1")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d body %s", rec.Code, rec.Body.String())
	}

	// A replayed delete with the same idempotency key replays the 204.
	rec = doJSON(t, f.router, http.MethodDelete,
		fmt.Sprintf("/v1/scheduled-tasks/%s?expected_version=%d", taskID, version),
		nil, "Idempotency-Key", "delete-key-1")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("replayed delete: expected 204, got %d", rec.Code)
	}

	// A delete without the idempotency key after the task is gone returns 404.
	rec = doJSON(t, f.router, http.MethodDelete,
		fmt.Sprintf("/v1/scheduled-tasks/%s?expected_version=%d", taskID, version),
		nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unkeyed replay delete: expected 404, got %d", rec.Code)
	}

	// Tombstoned tasks disappear from the list.
	rec = doJSON(t, f.router, http.MethodGet, "/v1/scheduled-tasks", nil)
	if tasks := decodeBody(t, rec)["tasks"].([]any); len(tasks) != 0 {
		t.Fatalf("deleted task still listed: %v", tasks)
	}
}

func TestScheduledTaskRunNowAndLastExecution(t *testing.T) {
	f := newScheduledTaskFixture(t)
	created := createScheduledTask(t, f.router, scheduledTaskRequestBody("立即执行"))
	taskID := created["task_id"].(string)

	// idempotency_key is required.
	rec := doJSON(t, f.router, http.MethodPost, "/v1/scheduled-tasks/"+taskID+"/run", map[string]any{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("run without key: expected 400, got %d", rec.Code)
	}

	rec = doJSON(t, f.router, http.MethodPost, "/v1/scheduled-tasks/"+taskID+"/run", map[string]any{
		"idempotency_key": "run-key-1",
	})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("run: expected 202, got %d body %s", rec.Code, rec.Body.String())
	}
	execution := decodeBody(t, rec)
	for _, key := range []string{"execution_id", "task_id", "task_version", "trigger", "scheduled_for", "status"} {
		if _, ok := execution[key]; !ok {
			t.Fatalf("execution wire missing key %q: %v", key, execution)
		}
	}
	if execution["trigger"] != "manual" || execution["task_id"] != taskID {
		t.Fatalf("unexpected execution wire: %v", execution)
	}
	executionID := execution["execution_id"].(string)

	// A retry with the same key is idempotent even while the original worker is
	// active; a distinct key is the case that conflicts with 409.
	rec = doJSON(t, f.router, http.MethodPost, "/v1/scheduled-tasks/"+taskID+"/run", map[string]any{
		"idempotency_key": "run-key-1",
	})
	if rec.Code != http.StatusAccepted || decodeBody(t, rec)["execution_id"] != executionID {
		t.Fatalf("active idempotent retry did not replay execution: status=%d body=%s", rec.Code, rec.Body.String())
	}

	waitForTerminalExecution(t, f, taskID)

	// A retry with the same key after completion returns the same execution,
	// never a second one.
	rec = doJSON(t, f.router, http.MethodPost, "/v1/scheduled-tasks/"+taskID+"/run", map[string]any{
		"idempotency_key": "run-key-1",
	})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("idempotent retry: expected 202, got %d body %s", rec.Code, rec.Body.String())
	}
	if decodeBody(t, rec)["execution_id"] != executionID {
		t.Fatalf("idempotent retry created a new execution: %s", rec.Body.String())
	}

	// The list/get projection now carries last_execution.
	rec = doJSON(t, f.router, http.MethodGet, "/v1/scheduled-tasks/"+taskID, nil)
	last := decodeBody(t, rec)["last_execution"].(map[string]any)
	if last["execution_id"] != executionID || last["status"] != "success" {
		t.Fatalf("unexpected last_execution projection: %v", last)
	}
	if _, ok := last["started_at"]; !ok {
		t.Fatalf("last_execution must carry started_at: %v", last)
	}

	// Run of an unknown task → 404.
	rec = doJSON(t, f.router, http.MethodPost, "/v1/scheduled-tasks/sched_missing/run", map[string]any{
		"idempotency_key": "k",
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("run missing: expected 404, got %d", rec.Code)
	}
}

func TestScheduledTaskRunConflictAndCancel(t *testing.T) {
	f := newScheduledTaskFixture(t)
	f.executor.release = make(chan struct{})
	created := createScheduledTask(t, f.router, scheduledTaskRequestBody("阻塞执行"))
	taskID := created["task_id"].(string)

	rec := doJSON(t, f.router, http.MethodPost, "/v1/scheduled-tasks/"+taskID+"/run", map[string]any{
		"idempotency_key": "run-key-block",
	})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("run: expected 202, got %d body %s", rec.Code, rec.Body.String())
	}

	// The executor blocks, so the task stays active: a second run conflicts.
	deadline := time.Now().Add(2 * time.Second)
	for !f.service.IsRunning(taskID) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	rec = doJSON(t, f.router, http.MethodPost, "/v1/scheduled-tasks/"+taskID+"/run", map[string]any{
		"idempotency_key": "run-key-conflict",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("conflicting run: expected 409, got %d body %s", rec.Code, rec.Body.String())
	}
	if decodeBody(t, rec)["error"] != "TASK_ALREADY_RUNNING" {
		t.Fatalf("conflicting run: unexpected body %s", rec.Body.String())
	}

	// Cancel only reaches the executor once the execution is running (a
	// queued reminder has no cancellation hook), so wait for the running
	// record before cancelling.
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		records, err := f.service.ListExecutions(taskID, 1)
		if err == nil && len(records) > 0 && records[0].Status == scheduledtask.ExecutionStatusRunning {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Cancel the active execution → 204 and a cancelled terminal record.
	rec = doJSON(t, f.router, http.MethodPost, "/v1/scheduled-tasks/"+taskID+"/cancel", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("cancel: expected 204, got %d body %s", rec.Code, rec.Body.String())
	}
	waitForTerminalExecution(t, f, taskID)
	executions, err := f.service.ListExecutions(taskID, 1)
	if err != nil || len(executions) == 0 {
		t.Fatalf("list executions after cancel: %v", err)
	}
	if executions[0].Status != scheduledtask.ExecutionStatusCancelled {
		t.Fatalf("expected cancelled execution, got %s", executions[0].Status)
	}

	// Nothing active → 404 EXECUTION_NOT_FOUND.
	rec = doJSON(t, f.router, http.MethodPost, "/v1/scheduled-tasks/"+taskID+"/cancel", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cancel without active execution: expected 404, got %d", rec.Code)
	}
	if decodeBody(t, rec)["error"] != "EXECUTION_NOT_FOUND" {
		t.Fatalf("cancel without active execution: unexpected body %s", rec.Body.String())
	}
}

func TestScheduledTaskExecutionsHistory(t *testing.T) {
	f := newScheduledTaskFixture(t)
	created := createScheduledTask(t, f.router, scheduledTaskRequestBody("历史"))
	taskID := created["task_id"].(string)

	// Seed 55 execution records directly; the HTTP surface must cap at 50.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 55; i++ {
		_, _, err := f.repo.AppendExecution(taskID, scheduledtask.Execution{
			Trigger:      scheduledtask.TriggerScheduled,
			ScheduledFor: base.Add(time.Duration(i) * time.Hour),
			TaskVersion:  1,
			StartedAt:    base.Add(time.Duration(i) * time.Hour),
			Status:       scheduledtask.ExecutionStatusSuccess,
		})
		if err != nil {
			t.Fatalf("append execution %d: %v", i, err)
		}
	}

	rec := doJSON(t, f.router, http.MethodGet, "/v1/scheduled-tasks/"+taskID+"/executions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("executions: expected 200, got %d", rec.Code)
	}
	body := decodeBody(t, rec)
	executions, ok := body["executions"].([]any)
	if !ok {
		t.Fatalf("executions key missing: %v", body)
	}
	if len(executions) != 50 {
		t.Fatalf("expected the 50-record cap, got %d", len(executions))
	}
	// Newest first by scheduled_for.
	first := executions[0].(map[string]any)
	lastItem := executions[len(executions)-1].(map[string]any)
	if first["scheduled_for"].(string) <= lastItem["scheduled_for"].(string) {
		t.Fatalf("executions must be newest-first: first=%v last=%v", first["scheduled_for"], lastItem["scheduled_for"])
	}
	if first["status"] != "success" {
		t.Fatalf("unexpected execution wire: %v", first)
	}
	for _, key := range []string{"execution_id", "task_id", "task_version", "trigger", "scheduled_for", "started_at", "status"} {
		if _, ok := first[key]; !ok {
			t.Fatalf("execution wire missing key %q: %v", key, first)
		}
	}

	rec = doJSON(t, f.router, http.MethodGet, "/v1/scheduled-tasks/sched_missing/executions", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("executions of missing task: expected 404, got %d", rec.Code)
	}
}

func TestScheduledTaskInternalError(t *testing.T) {
	f := newScheduledTaskFixture(t)
	// A closed repository surfaces as a plain error → 500 INTERNAL_ERROR.
	if err := f.repo.Close(); err != nil {
		t.Fatalf("close repo: %v", err)
	}
	rec := doJSON(t, f.router, http.MethodPost, "/v1/scheduled-tasks", scheduledTaskRequestBody("x"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("closed store: expected 500, got %d body %s", rec.Code, rec.Body.String())
	}
	if decodeBody(t, rec)["error"] != "INTERNAL_ERROR" {
		t.Fatalf("closed store: unexpected body %s", rec.Body.String())
	}
}

func TestScheduledTaskEventsStream(t *testing.T) {
	f := newScheduledTaskFixture(t)
	server := httptest.NewServer(f.router)
	t.Cleanup(server.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/v1/scheduled-tasks/events", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open event stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("events: expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("events: unexpected content type %q", ct)
	}

	frames := make(chan string, 4)
	go func() {
		reader := bufio.NewReader(resp.Body)
		var frame strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if line == "\n" || line == "\r\n" {
				frames <- frame.String()
				frame.Reset()
				continue
			}
			frame.WriteString(line)
		}
	}()

	f.service.PublishEvent(scheduledtask.Event{
		Type:        scheduledtask.EventNotice,
		TaskID:      "sched_1",
		ProjectID:   "project-1",
		ExecutionID: "exec_1",
		RunID:       "run-1",
		ThreadID:    "thread-1",
		TaskName:    "喝水",
		Status:      "success",
		Content:     "该喝水了",
	})

	select {
	case frame := <-frames:
		if !strings.Contains(frame, "event: scheduled-task") {
			t.Fatalf("frame missing event label: %q", frame)
		}
		dataLine := ""
		for _, line := range strings.Split(frame, "\n") {
			if strings.HasPrefix(line, "data:") {
				dataLine = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(dataLine), &payload); err != nil {
			t.Fatalf("decode frame data %q: %v", dataLine, err)
		}
		if payload["type"] != "notice" || payload["task_id"] != "sched_1" || payload["project_id"] != "project-1" || payload["run_id"] != "run-1" || payload["content"] != "该喝水了" || payload["task_name"] != "喝水" {
			t.Fatalf("unexpected notice payload: %v", payload)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for the notice frame")
	}

	// A non-notice event carries no content key at all.
	f.service.PublishEvent(scheduledtask.Event{
		Type:   scheduledtask.EventTaskChanged,
		TaskID: "sched_1",
		Status: "enabled",
	})
	select {
	case frame := <-frames:
		if strings.Contains(frame, "content") {
			t.Fatalf("task_changed must not carry content: %q", frame)
		}
		if !strings.Contains(frame, "task_changed") {
			t.Fatalf("unexpected frame: %q", frame)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for the task_changed frame")
	}
}
