package scheduledtask

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"awaken/src/agentcore"
	"awaken/src/modelprofile"
	"awaken/src/runtime"
)

// Execution error codes recorded on execution-history records. They are
// stable wire values; error messages next to them stay short code-level
// phrases and never carry prompts, reminder content, API keys, tokens,
// QR codes or raw stdout/stderr (contract §9.3).
const (
	ErrorCodeTaskAlreadyRunning           = "TASK_ALREADY_RUNNING"
	ErrorCodeTaskThreadBusy               = "TASK_THREAD_BUSY"
	ErrorCodeAgentConcurrencyExceeded     = "AGENT_CONCURRENCY_EXCEEDED"
	ErrorCodeMisfireGraceExceeded         = "MISFIRE_GRACE_EXCEEDED"
	ErrorCodeProjectUnavailable           = "PROJECT_UNAVAILABLE"
	ErrorCodeModelProfileUnavailable      = "MODEL_PROFILE_UNAVAILABLE"
	ErrorCodeExecutionTimeout             = "EXECUTION_TIMEOUT"
	ErrorCodeScheduledInteractionRequired = "SCHEDULED_INTERACTION_REQUIRED"
	ErrorCodeRunFailed                    = "RUN_FAILED"
	ErrorCodeRunCancelled                 = "RUN_CANCELLED"
	ErrorCodeRunStartFailed               = "RUN_START_FAILED"
	ErrorCodePersistenceFailed            = "PERSISTENCE_FAILED"
)

// ExecError is the sanitized outcome of a failed execution. Skipped marks
// outcomes recorded as skipped (thread busy, concurrency/misfire overflow);
// Cancelled marks outcomes recorded as cancelled; everything else is an
// error.
type ExecError struct {
	Code      string
	Message   string
	Skipped   bool
	Cancelled bool
}

func (e *ExecError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Code + ": " + e.Message
	}
	return e.Code
}

// Executor runs one task execution to its terminal state. Implementations
// block until the run finishes, times out or is cancelled. The scheduler and
// service depend only on this interface, so tests use fakes.
type Executor interface {
	// ExecuteAgent runs the task prompt through RunService on the task's
	// dedicated thread and blocks until the run is terminal.
	ExecuteAgent(ctx context.Context, task Task, exec Execution) (runID, threadID string, execErr *ExecError)
	// ExecuteReminder publishes the fixed reminder notice without any model
	// call.
	ExecuteReminder(ctx context.Context, task Task, exec Execution) (threadID string, execErr *ExecError)
	// Cancel asks the executor to cancel the task's active execution. It is
	// a no-op when the task has none.
	Cancel(taskID string)
}

// ExecutionStartReporter lets the production executor report the durable
// run/thread identity immediately after RunService creates it, while the
// blocking ExecuteAgent call continues waiting for terminal. The scheduler
// remains the only owner that persists and publishes that identity.
type ExecutionStartReporter interface {
	SetExecutionStartedHandler(handler func(task Task, exec Execution, runID, threadID string))
}

// RunServiceExecutor is the production Executor backed by RunService. The
// resolved model profile (including the API key) only ever lives in memory
// inside a single call; nothing here persists it.
type RunServiceExecutor struct {
	RunService *runtime.RunService
	Profiles   *modelprofile.UserStore
	// NewTimer bounds the execution wait; tests inject manual timers so no
	// test ever waits on the wall clock. Nil uses real timers.
	NewTimer func(d time.Duration) Timer

	mu        sync.Mutex
	active    map[string]string // taskID -> runID
	onStarted func(task Task, exec Execution, runID, threadID string)
}

// SetExecutionStartedHandler implements ExecutionStartReporter.
func (e *RunServiceExecutor) SetExecutionStartedHandler(handler func(task Task, exec Execution, runID, threadID string)) {
	e.mu.Lock()
	e.onStarted = handler
	e.mu.Unlock()
}

func (e *RunServiceExecutor) reportExecutionStarted(task Task, exec Execution, runID, threadID string) {
	e.mu.Lock()
	handler := e.onStarted
	e.mu.Unlock()
	if handler != nil {
		handler(task, exec, runID, threadID)
	}
}

// NewRunServiceExecutor builds the production executor.
func NewRunServiceExecutor(runs *runtime.RunService, profiles *modelprofile.UserStore) *RunServiceExecutor {
	return &RunServiceExecutor{
		RunService: runs,
		Profiles:   profiles,
		active:     map[string]string{},
	}
}

func (e *RunServiceExecutor) timer(d time.Duration) Timer {
	if e.NewTimer != nil {
		return e.NewTimer(d)
	}
	return RealTimer(d)
}

func (e *RunServiceExecutor) registerActive(taskID, runID string) {
	e.mu.Lock()
	e.active[taskID] = runID
	e.mu.Unlock()
}

func (e *RunServiceExecutor) unregisterActive(taskID string) {
	e.mu.Lock()
	delete(e.active, taskID)
	e.mu.Unlock()
}

// Cancel implements Executor.Cancel.
func (e *RunServiceExecutor) Cancel(taskID string) {
	e.mu.Lock()
	runID := e.active[taskID]
	e.mu.Unlock()
	if runID == "" || e.RunService == nil {
		return
	}
	_, _ = e.RunService.CancelRun(runID)
}

// UpdateScheduledThreadTitle follows a task rename onto the dedicated thread
// (contract §4.5); a manually renamed thread title is never overwritten.
func (e *RunServiceExecutor) UpdateScheduledThreadTitle(threadID, taskName string) error {
	return e.RunService.UpdateScheduledThreadTitle(threadID, taskName)
}

// resolveWorkspace re-reads the project at trigger time (contract §9.1): the
// current primary folder becomes the working directory and all current
// folders the readable/writable/output roots. A folder-less project runs in
// its session workspace. Removed projects, deletion in progress and missing
// paths fail closed; no stale root snapshot is ever used.
func (e *RunServiceExecutor) resolveWorkspace(projectID string) (runtime.WorkspaceConfig, *ExecError) {
	projectService := e.RunService.ProjectService()
	if projectService == nil {
		return runtime.WorkspaceConfig{}, &ExecError{Code: ErrorCodeProjectUnavailable, Message: "project service unavailable"}
	}
	projects, err := projectService.ListProjects(false)
	if err != nil {
		return runtime.WorkspaceConfig{}, &ExecError{Code: ErrorCodeProjectUnavailable, Message: "project state unreadable"}
	}
	for _, project := range projects {
		if project.ProjectID != projectID {
			continue
		}
		if project.State != runtime.ProjectStateActive {
			return runtime.WorkspaceConfig{}, &ExecError{Code: ErrorCodeProjectUnavailable, Message: "project is not active"}
		}
		roots := project.AllRoots()
		if len(roots) == 0 {
			// Folder-less project: normalizeWorkspace resolves the Awaken
			// session workspace from the project identity.
			return runtime.WorkspaceConfig{
				ProjectID:   project.ProjectID,
				ProjectName: project.DisplayName,
			}, nil
		}
		for _, root := range roots {
			if info, err := os.Stat(root); err != nil || !info.IsDir() {
				return runtime.WorkspaceConfig{}, &ExecError{Code: ErrorCodeProjectUnavailable, Message: "project path is unavailable"}
			}
		}
		return runtime.WorkspaceConfig{
			WorkspaceRoot: roots[0],
			ReadableRoots: roots,
			WritableRoots: roots,
			OutputRoots:   roots,
			ProjectID:     project.ProjectID,
			ProjectName:   project.DisplayName,
		}, nil
	}
	return runtime.WorkspaceConfig{}, &ExecError{Code: ErrorCodeProjectUnavailable, Message: "project is not active"}
}

// resolveProfile re-resolves the model profile at trigger time (contract
// §9.2): an empty ID picks the current default ready profile, an explicit ID
// must still be ready. There is no fallback and no silent provider switch.
func (e *RunServiceExecutor) resolveProfile(profileID string) (*runtime.ModelProfileConfig, *ExecError) {
	if e.Profiles == nil {
		return nil, &ExecError{Code: ErrorCodeModelProfileUnavailable, Message: "model profile store unavailable"}
	}
	resolved, err := e.Profiles.Resolve(profileID)
	if err != nil {
		return nil, &ExecError{Code: ErrorCodeModelProfileUnavailable, Message: "model profile is not ready"}
	}
	return runtime.ModelProfileConfigFromResolved(resolved), nil
}

// ExecuteAgent implements Executor.ExecuteAgent.
func (e *RunServiceExecutor) ExecuteAgent(ctx context.Context, task Task, exec Execution) (string, string, *ExecError) {
	workspace, execErr := e.resolveWorkspace(task.ProjectID)
	if execErr != nil {
		return "", "", execErr
	}
	profile, execErr := e.resolveProfile(task.Execution.ProfileID)
	if execErr != nil {
		return "", "", execErr
	}
	request := runtime.ScheduledRunRequest{
		TaskID:       task.TaskID,
		ExecutionID:  exec.ExecutionID,
		TaskName:     task.Name,
		Prompt:       task.Action.Content,
		ThreadID:     task.ThreadID,
		Workspace:    workspace,
		ModelProfile: profile,
	}
	run, err := e.RunService.StartScheduledRun(request)
	if errors.Is(err, runtime.ErrThreadNotFound) && strings.TrimSpace(task.ThreadID) != "" {
		// A user may explicitly delete a task's dedicated thread while keeping
		// the task. Recreate it once instead of leaving the durable task bound
		// to an ID that can never run again. The scheduler CAS-rebinds the new
		// ID only if the task still points at the stale one.
		request.ThreadID = ""
		run, err = e.RunService.StartScheduledRun(request)
	}
	if err != nil {
		return "", "", mapStartError(err)
	}
	e.registerActive(task.TaskID, run.RunID)
	e.reportExecutionStarted(task, exec, run.RunID, run.ThreadID)
	defer e.unregisterActive(task.TaskID)
	// Cancellation can arrive after the run starts but before registerActive
	// has observed it. Propagate ctx cancellation directly to the run so the
	// executor's Cancel fallback is not the only path.
	stopWatching := make(chan struct{})
	defer close(stopWatching)
	go func() {
		select {
		case <-ctx.Done():
			_, _ = e.RunService.CancelRun(run.RunID)
		case <-stopWatching:
		}
	}()
	return run.RunID, run.ThreadID, e.awaitTerminal(ctx, task, run)
}

// mapStartError converts StartScheduledRun failures into sanitized execution
// outcomes. Thread busy is a skip, not an error: nothing is written to any
// other thread (contract §4.5).
func mapStartError(err error) *ExecError {
	switch {
	case errors.Is(err, runtime.ErrThreadBusy):
		return &ExecError{Code: ErrorCodeTaskThreadBusy, Message: "dedicated thread is busy", Skipped: true}
	case errors.Is(err, runtime.ErrProjectOperationConflict):
		return &ExecError{Code: ErrorCodeProjectUnavailable, Message: "project operation in progress"}
	default:
		return &ExecError{Code: ErrorCodeRunStartFailed, Message: "scheduled run could not start"}
	}
}

// ExecuteReminder implements Executor.ExecuteReminder. It never calls a
// model and never occupies the global agent semaphore.
func (e *RunServiceExecutor) ExecuteReminder(ctx context.Context, task Task, exec Execution) (string, *ExecError) {
	workspace, execErr := e.resolveWorkspace(task.ProjectID)
	if execErr != nil {
		return "", execErr
	}
	publishCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	request := runtime.ScheduledNoticeRequest{
		TaskID:      task.TaskID,
		ExecutionID: exec.ExecutionID,
		TaskName:    task.Name,
		Content:     task.Action.Content,
		ThreadID:    task.ThreadID,
		Workspace:   workspace,
	}
	threadID, runID, err := e.RunService.PublishScheduledNotice(publishCtx, request)
	if errors.Is(err, runtime.ErrThreadNotFound) && strings.TrimSpace(task.ThreadID) != "" {
		request.ThreadID = ""
		threadID, runID, err = e.RunService.PublishScheduledNotice(publishCtx, request)
	}
	if err != nil {
		return "", mapStartError(err)
	}
	e.reportExecutionStarted(task, exec, runID, threadID)
	return threadID, nil
}

// interactionGate reports the interaction gate an event represents, or "".
// Unattended scheduled runs fail fast on the first ask_user, external
// authorization, ACP permission or auth gate (contract §9.3); only the gate
// type is recorded.
func interactionGate(event runtime.Event) string {
	if event.Kind == runtime.EventKindAgent {
		switch event.AgentEvent.Type {
		case agentcore.UserQuestionRequestedType:
			return "user_question"
		case agentcore.ExternalInteractionRequestedType:
			return "external_interaction"
		case agentcore.ACPPermissionRequestedEventType:
			return "acp_permission"
		case agentcore.AuthRequiredEventType:
			return "auth_required"
		}
	}
	if event.Kind == runtime.EventKindSemantic &&
		event.SemanticRecord.Kind == agentcore.SemanticKindACPPermissionRequested {
		return "acp_permission"
	}
	return ""
}

// awaitTerminal subscribes to the run and blocks until it reaches a terminal
// state, the task timeout fires, or the caller context is cancelled.
func (e *RunServiceExecutor) awaitTerminal(ctx context.Context, task Task, run runtime.Run) *ExecError {
	subscription, err := e.RunService.Subscribe(run.RunID, 0)
	if err != nil {
		return &ExecError{Code: ErrorCodeRunFailed, Message: "run stream unavailable"}
	}
	defer func() {
		if subscription.Close != nil {
			subscription.Close()
		}
	}()

	timeoutSeconds := task.Execution.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 600
	}
	timer := e.timer(time.Duration(timeoutSeconds) * time.Second)
	defer timer.Stop()

	events := subscription.Events
	for {
		// History is re-checked every iteration: events appended between
		// StartScheduledRun and Subscribe must not slip past the gate watch.
		for _, event := range subscription.History {
			if gate := interactionGate(event); gate != "" {
				e.RunService.CancelRun(run.RunID)
				e.drainToTerminal(events)
				return &ExecError{Code: ErrorCodeScheduledInteractionRequired, Message: "interaction required: " + gate}
			}
		}
		subscription.History = nil
		select {
		case event, ok := <-events:
			if !ok {
				return e.terminalOutcome(run.RunID)
			}
			if gate := interactionGate(event); gate != "" {
				e.RunService.CancelRun(run.RunID)
				e.drainToTerminal(events)
				return &ExecError{Code: ErrorCodeScheduledInteractionRequired, Message: "interaction required: " + gate}
			}
		case <-timer.C():
			e.RunService.CancelRun(run.RunID)
			e.drainToTerminal(events)
			return &ExecError{Code: ErrorCodeExecutionTimeout, Message: "execution timed out"}
		case <-ctx.Done():
			e.RunService.CancelRun(run.RunID)
			e.drainToTerminal(events)
			return &ExecError{Code: ErrorCodeRunCancelled, Message: "execution cancelled", Cancelled: true}
		}
	}
}

// drainToTerminal waits for the run bus to close after a cancellation so the
// execution only returns once the run has exactly one terminal state.
func (e *RunServiceExecutor) drainToTerminal(events <-chan runtime.Event) {
	for range events {
	}
}

// terminalOutcome maps the durable run status onto an execution outcome.
func (e *RunServiceExecutor) terminalOutcome(runID string) *ExecError {
	run, err := e.RunService.GetRun(runID)
	if err != nil {
		return &ExecError{Code: ErrorCodeRunFailed, Message: "run outcome unavailable"}
	}
	switch run.Status {
	case runtime.RunStatusFinished:
		return nil
	case runtime.RunStatusCancelled:
		return &ExecError{Code: ErrorCodeRunCancelled, Message: "run cancelled", Cancelled: true}
	default:
		return &ExecError{Code: ErrorCodeRunFailed, Message: "agent run failed"}
	}
}
