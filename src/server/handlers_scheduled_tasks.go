package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"awaken/src/scheduledtask"
)

// handlers_scheduled_tasks.go owns the /v1/scheduled-tasks HTTP surface
// (contract §10): typed CRUD under version CAS, manual run/cancel, execution
// history and the live event SSE stream. Scheduling authority stays in
// src/scheduledtask; this file only maps transport.

// scheduledTaskHandlers serves the scheduled-task routes. The service is
// optional: nil means the runtime started without a store (AWAKEN_HOME
// unavailable) and every route answers 503, mirroring the skills surface.
const maxEventOutbox = 256

type scheduledTaskHandlers struct {
	service *scheduledtask.Service
	now     func() time.Time

	// The service exposes a single event-listener slot; the SSE hub owns it
	// and fans events out to every connected stream. Events that arrive while
	// no subscriber is connected are buffered so startup-recovery reminders are
	// not silently lost before Desktop establishes its SSE connection.
	hubMu  sync.Mutex
	hub    map[chan scheduledtask.Event]struct{}
	outbox []scheduledtask.Event
}

func newScheduledTaskHandlers(service *scheduledtask.Service) *scheduledTaskHandlers {
	h := &scheduledTaskHandlers{
		service: service,
		now:     func() time.Time { return time.Now().UTC() },
		hub:     map[chan scheduledtask.Event]struct{}{},
	}
	if service != nil {
		service.SetEventListener(h.publishEvent)
	}
	return h
}

func (h *scheduledTaskHandlers) publishEvent(event scheduledtask.Event) {
	h.hubMu.Lock()
	defer h.hubMu.Unlock()
	if len(h.hub) == 0 {
		h.outbox = append(h.outbox, event)
		if len(h.outbox) > maxEventOutbox {
			h.outbox = h.outbox[len(h.outbox)-maxEventOutbox:]
		}
		return
	}
	for ch := range h.hub {
		select {
		case ch <- event:
		default:
			// Events are fire-and-forget; a stalled consumer loses them rather
			// than blocking the scheduler.
		}
	}
}

func (h *scheduledTaskHandlers) subscribe() chan scheduledtask.Event {
	ch := make(chan scheduledtask.Event, 64)
	h.hubMu.Lock()
	h.hub[ch] = struct{}{}
	pending := h.outbox
	h.outbox = nil
	h.hubMu.Unlock()
	for _, event := range pending {
		select {
		case ch <- event:
		default:
		}
	}
	return ch
}

func (h *scheduledTaskHandlers) unsubscribe(ch chan scheduledtask.Event) {
	h.hubMu.Lock()
	delete(h.hub, ch)
	h.hubMu.Unlock()
}

// Wire shapes. Field names are the snake_case transport contract; the Go
// json tags of src/scheduledtask types are the authority for the embedded
// schedule/action/execution branches.

type scheduledTaskLastExecutionWire struct {
	ExecutionID string     `json:"execution_id"`
	Status      string     `json:"status"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

// scheduledTaskWire is the list/get/mutation response item: the persisted
// task fields plus the materialized state, the projected next run and the
// newest execution summary. Internal bookkeeping (consumed_occurrences,
// last_processed_occurrence, completed) never leaves Core; completed is
// folded into state.
type scheduledTaskWire struct {
	TaskID         string                          `json:"task_id"`
	Version        int64                           `json:"version"`
	Name           string                          `json:"name"`
	Enabled        bool                            `json:"enabled"`
	ProjectID      string                          `json:"project_id"`
	ThreadID       string                          `json:"thread_id,omitempty"`
	Schedule       scheduledtask.Schedule          `json:"schedule"`
	Action         scheduledtask.Action            `json:"action"`
	Execution      scheduledtask.ExecutionOptions  `json:"execution"`
	TemplateOrigin *scheduledtask.TemplateOrigin   `json:"template_origin,omitempty"`
	State          string                          `json:"state"`
	InvalidReason  string                          `json:"invalid_reason,omitempty"`
	NextRunAt      *time.Time                      `json:"next_run_at"`
	LastExecution  *scheduledTaskLastExecutionWire `json:"last_execution"`
	CreatedAt      time.Time                       `json:"created_at"`
	UpdatedAt      time.Time                       `json:"updated_at"`
}

type scheduledTaskEventWire struct {
	Type        string `json:"type"`
	TaskID      string `json:"task_id"`
	ExecutionID string `json:"execution_id,omitempty"`
	ThreadID    string `json:"thread_id,omitempty"`
	TaskName    string `json:"task_name,omitempty"`
	Status      string `json:"status,omitempty"`
	Content     string `json:"content,omitempty"`
}

type scheduledTaskCreateRequest struct {
	Name           string                         `json:"name"`
	Enabled        bool                           `json:"enabled"`
	ProjectID      string                         `json:"project_id"`
	Schedule       scheduledtask.Schedule         `json:"schedule"`
	Action         scheduledtask.Action           `json:"action"`
	Execution      scheduledtask.ExecutionOptions `json:"execution"`
	TemplateOrigin *scheduledtask.TemplateOrigin  `json:"template_origin"`
}

type scheduledTaskPatchRequest struct {
	ExpectedVersion *int64                          `json:"expected_version"`
	Name            *string                         `json:"name"`
	Enabled         *bool                           `json:"enabled"`
	ProjectID       *string                         `json:"project_id"`
	Schedule        *scheduledtask.Schedule         `json:"schedule"`
	Action          *scheduledtask.Action           `json:"action"`
	Execution       *scheduledtask.ExecutionOptions `json:"execution"`
	// TemplateOrigin replaces the origin when present; there is no clearing
	// through patch (a task keeps the template it was created from).
	TemplateOrigin *scheduledtask.TemplateOrigin `json:"template_origin"`
}

type scheduledTaskVersionRequest struct {
	ExpectedVersion *int64 `json:"expected_version"`
}

type scheduledTaskRunRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
}

func (h *scheduledTaskHandlers) taskWire(task scheduledtask.Task) scheduledTaskWire {
	wire := scheduledTaskWire{
		TaskID:         task.TaskID,
		Version:        task.Version,
		Name:           task.Name,
		Enabled:        task.Enabled,
		ProjectID:      task.ProjectID,
		ThreadID:       task.ThreadID,
		Schedule:       task.Schedule,
		Action:         task.Action,
		Execution:      task.Execution,
		TemplateOrigin: task.TemplateOrigin,
		State:          string(task.State(h.service.IsRunning(task.TaskID))),
		InvalidReason:  task.InvalidReason,
		NextRunAt:      scheduledtask.NextRunAt(task, h.now()),
		CreatedAt:      task.CreatedAt,
		UpdatedAt:      task.UpdatedAt,
	}
	if last, err := h.service.LastExecution(task.TaskID); err == nil && last != nil {
		wire.LastExecution = &scheduledTaskLastExecutionWire{
			ExecutionID: last.ExecutionID,
			Status:      string(last.Status),
			FinishedAt:  last.FinishedAt,
		}
		if !last.StartedAt.IsZero() {
			startedAt := last.StartedAt
			wire.LastExecution.StartedAt = &startedAt
		}
	}
	return wire
}

// writeServiceError maps scheduled-task domain errors onto the transport
// codes of contract §10. Validation messages carry only the field name and a
// static issue label; task content never appears in any error body.
func writeScheduledTaskError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, scheduledtask.ErrTaskNotFound):
		writeErrorJSON(w, http.StatusNotFound, "TASK_NOT_FOUND", "scheduled task not found")
	case errors.Is(err, scheduledtask.ErrExecutionNotFound):
		writeErrorJSON(w, http.StatusNotFound, "EXECUTION_NOT_FOUND", "no active execution")
	case errors.Is(err, scheduledtask.ErrVersionConflict):
		writeErrorJSON(w, http.StatusConflict, "VERSION_CONFLICT", "task was modified; reload and retry")
	case errors.Is(err, scheduledtask.ErrTaskAlreadyRunning):
		writeErrorJSON(w, http.StatusConflict, "TASK_ALREADY_RUNNING", "task already has an active execution")
	case errors.Is(err, scheduledtask.ErrInvalidRequest):
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
	case errors.Is(err, scheduledtask.ErrValidationFailed):
		var validationErr *scheduledtask.ValidationError
		if errors.As(err, &validationErr) {
			writeErrorJSON(w, http.StatusUnprocessableEntity, "VALIDATION_FAILED",
				fmt.Sprintf("invalid %s: %s", validationErr.Field, validationErr.Issue))
			return
		}
		writeErrorJSON(w, http.StatusUnprocessableEntity, "VALIDATION_FAILED", "task definition is invalid")
	default:
		writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "scheduled task request failed")
	}
}

func (h *scheduledTaskHandlers) requireService(w http.ResponseWriter) bool {
	if h.service == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "SCHEDULED_TASKS_UNAVAILABLE", "scheduled tasks are unavailable")
		return false
	}
	return true
}

func (h *scheduledTaskHandlers) handleList(w http.ResponseWriter, r *http.Request) {
	if !h.requireService(w) {
		return
	}
	tasks, err := h.service.List()
	if err != nil {
		writeScheduledTaskError(w, err)
		return
	}
	items := make([]scheduledTaskWire, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, h.taskWire(task))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": items})
}

func (h *scheduledTaskHandlers) handleCreate(w http.ResponseWriter, r *http.Request) {
	if !h.requireService(w) {
		return
	}
	var req scheduledTaskCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	// Client-supplied identity, version, thread binding and timestamps are
	// discarded by the repository; only the definition fields are accepted.
	task, err := h.service.Create(scheduledtask.Task{
		Name:           req.Name,
		Enabled:        req.Enabled,
		ProjectID:      req.ProjectID,
		Schedule:       req.Schedule,
		Action:         req.Action,
		Execution:      req.Execution,
		TemplateOrigin: req.TemplateOrigin,
	})
	if err != nil {
		writeScheduledTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, h.taskWire(task))
}

func (h *scheduledTaskHandlers) handleGet(w http.ResponseWriter, r *http.Request) {
	if !h.requireService(w) {
		return
	}
	task, err := h.service.Get(r.PathValue("task_id"))
	if err != nil {
		writeScheduledTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.taskWire(task))
}

func (h *scheduledTaskHandlers) handlePatch(w http.ResponseWriter, r *http.Request) {
	if !h.requireService(w) {
		return
	}
	var req scheduledTaskPatchRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.ExpectedVersion == nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "expected_version is required")
		return
	}
	task, err := h.service.Patch(r.PathValue("task_id"), *req.ExpectedVersion, func(current scheduledtask.Task) (scheduledtask.Task, error) {
		if req.Name != nil {
			current.Name = *req.Name
		}
		if req.Enabled != nil {
			current.Enabled = *req.Enabled
		}
		if req.ProjectID != nil {
			current.ProjectID = *req.ProjectID
		}
		if req.Schedule != nil {
			current.Schedule = *req.Schedule
		}
		if req.Action != nil {
			current.Action = *req.Action
		}
		if req.Execution != nil {
			current.Execution = *req.Execution
		}
		if req.TemplateOrigin != nil {
			origin := *req.TemplateOrigin
			current.TemplateOrigin = &origin
		}
		return current, nil
	})
	if err != nil {
		writeScheduledTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.taskWire(task))
}

func (h *scheduledTaskHandlers) handleDelete(w http.ResponseWriter, r *http.Request) {
	if !h.requireService(w) {
		return
	}
	taskID := r.PathValue("task_id")
	// expected_version rides the query string (DELETE bodies are unreliable
	// through some proxies); the idempotency key rides the Idempotency-Key
	// header and is persisted as a delete receipt so a lost 204 response can
	// be replayed as another 204.
	expectedVersion, err := parseExpectedVersion(r.URL.Query().Get("expected_version"))
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if err := h.service.Delete(taskID, expectedVersion, idempotencyKey); err != nil {
		writeScheduledTaskError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseExpectedVersion(raw string) (int64, error) {
	if raw == "" {
		return 0, errors.New("expected_version is required")
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, errors.New("expected_version must be an integer")
	}
	return value, nil
}

func (h *scheduledTaskHandlers) handleSetEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	if !h.requireService(w) {
		return
	}
	var req scheduledTaskVersionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.ExpectedVersion == nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "expected_version is required")
		return
	}
	taskID := r.PathValue("task_id")
	var (
		task scheduledtask.Task
		err  error
	)
	if enabled {
		task, err = h.service.Resume(taskID, *req.ExpectedVersion)
	} else {
		task, err = h.service.Pause(taskID, *req.ExpectedVersion)
	}
	if err != nil {
		writeScheduledTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.taskWire(task))
}

func (h *scheduledTaskHandlers) handleRun(w http.ResponseWriter, r *http.Request) {
	if !h.requireService(w) {
		return
	}
	var req scheduledTaskRunRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.IdempotencyKey == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "idempotency_key is required")
		return
	}
	execution, err := h.service.RunNow(r.PathValue("task_id"), req.IdempotencyKey)
	if err != nil {
		writeScheduledTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, execution)
}

func (h *scheduledTaskHandlers) handleCancel(w http.ResponseWriter, r *http.Request) {
	if !h.requireService(w) {
		return
	}
	taskID := r.PathValue("task_id")
	if _, err := h.service.Get(taskID); err != nil {
		writeScheduledTaskError(w, err)
		return
	}
	if err := h.service.CancelExecution(taskID); err != nil {
		writeScheduledTaskError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *scheduledTaskHandlers) handleListExecutions(w http.ResponseWriter, r *http.Request) {
	if !h.requireService(w) {
		return
	}
	executions, err := h.service.ListExecutions(r.PathValue("task_id"), scheduledtask.MaxExecutionLimit)
	if err != nil {
		writeScheduledTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"executions": executions})
}

// scheduledTaskSSEEventName is the single SSE event label of the stream; the
// payload type field carries the semantic classification.
const scheduledTaskSSEEventName = "scheduled-task"

func (h *scheduledTaskHandlers) handleEvents(w http.ResponseWriter, r *http.Request) {
	if !h.requireService(w) {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	headers := w.Header()
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Connection", "keep-alive")
	headers.Set("X-Accel-Buffering", "no")
	// Commit the headers so the client knows the subscription is established
	// before the first event arrives (same contract as handleRunEvents).
	flusher.Flush()

	events := h.subscribe()
	defer h.unsubscribe(events)

	write := func(event scheduledtask.Event) error {
		payload, err := json.Marshal(scheduledTaskEventWire{
			Type:        string(event.Type),
			TaskID:      event.TaskID,
			ExecutionID: event.ExecutionID,
			ThreadID:    event.ThreadID,
			TaskName:    event.TaskName,
			Status:      event.Status,
			Content:     event.Content,
		})
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", scheduledTaskSSEEventName, payload); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-events:
			if err := write(event); err != nil {
				return
			}
		}
	}
}
