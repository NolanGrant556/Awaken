package scheduledtask

import (
	"errors"
	"fmt"
	"sync"
)

// EventType classifies scheduled-task live events. The stream is the source
// slice C exposes over HTTP/SSE; events are fire-and-forget and never
// persisted.
type EventType string

const (
	EventExecutionStarted  EventType = "execution_started"
	EventExecutionFinished EventType = "execution_finished"
	EventNotice            EventType = "notice"
	EventTaskChanged       EventType = "task_changed"
)

// Event is one live scheduled-task fact. Only notice events carry Content
// (the fixed reminder body, for the Desktop native notification); every
// other event leaves it empty.
type Event struct {
	Type        EventType
	TaskID      string
	ProjectID   string
	ExecutionID string
	RunID       string
	ThreadID    string
	TaskName    string
	Status      string
	Content     string
}

// Service is the scheduled-task application service: validated CRUD,
// pause/resume, manual runs, cancellation and the live event stream. The
// scheduler drives timed executions; the service nudges it after every
// mutation.
type Service struct {
	repo      Repository
	validator Validator
	executor  Executor

	mu          sync.Mutex
	scheduler   *Scheduler
	listener    func(Event)
	eventOutbox []Event
	closed      bool
}

// maxEventOutbox caps the best-effort buffer for events published before the
// first live-event listener connects (e.g. startup recovery misfire reminders).
// Older events are dropped once the cap is reached.
const maxEventOutbox = 256

// NewService builds the service. The executor is used for interface-level
// capabilities (title sync); timed and manual executions go through the
// scheduler installed with SetScheduler.
func NewService(repo Repository, validator Validator, executor Executor) *Service {
	return &Service{repo: repo, validator: validator, executor: executor}
}

// SetScheduler installs the scheduler the service nudges on mutations and
// delegates RunNow/CancelExecution to.
func (s *Service) SetScheduler(scheduler *Scheduler) {
	s.mu.Lock()
	s.scheduler = scheduler
	s.mu.Unlock()
}

func (s *Service) schedulerOrNil() *Scheduler {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scheduler
}

// SetEventListener installs the live-event sink (slice C's SSE source).
// Events are fire-and-forget; a nil listener drops them. Any events buffered
// before the first listener is installed are replayed to that listener.
func (s *Service) SetEventListener(listener func(Event)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listener = listener
	if listener == nil {
		return
	}
	pending := s.eventOutbox
	s.eventOutbox = nil
	for _, event := range pending {
		listener(event)
	}
}

// PublishEvent emits one live event. The scheduler wires its Emit here.
// Before the first listener connects, events are buffered so startup recovery
// reminders are not silently lost.
func (s *Service) PublishEvent(event Event) {
	s.mu.Lock()
	listener := s.listener
	if listener == nil {
		s.eventOutbox = append(s.eventOutbox, event)
		if len(s.eventOutbox) > maxEventOutbox {
			s.eventOutbox = s.eventOutbox[len(s.eventOutbox)-maxEventOutbox:]
		}
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	listener(event)
}

// Create validates and persists a new task (contract §6.3: Core is the only
// validation owner; client-supplied identity and timestamps are discarded by
// the repository).
func (s *Service) Create(task Task) (Task, error) {
	task.InvalidReason = ""
	if err := s.validator.ValidateTask(task); err != nil {
		return Task{}, err
	}
	created, err := s.repo.Create(task)
	if err != nil {
		return Task{}, err
	}
	s.nudge()
	s.publishTaskChanged(created)
	return created, nil
}

// Get returns one task.
func (s *Service) Get(taskID string) (Task, error) {
	return s.repo.Get(taskID)
}

// List returns all live tasks.
func (s *Service) List() ([]Task, error) {
	return s.repo.List()
}

// ListExecutions returns the newest execution history records (capped at 50
// by the repository).
func (s *Service) ListExecutions(taskID string, limit int) ([]Execution, error) {
	return s.repo.ListExecutions(taskID, limit)
}

// Patch applies a validated edit under version CAS. A successful edit clears
// a previous invalid marker and only affects future triggers; running
// executions keep their start-time snapshot. A rename also follows onto the
// dedicated thread unless the user renamed it manually.
func (s *Service) Patch(taskID string, expectedVersion int64, mutate func(Task) (Task, error)) (Task, error) {
	current, err := s.repo.Get(taskID)
	if err != nil {
		return Task{}, err
	}
	candidate, err := mutate(current)
	if err != nil {
		return Task{}, err
	}
	candidate.InvalidReason = ""
	if err := s.validator.ValidateTask(candidate); err != nil {
		return Task{}, err
	}
	updated, err := s.repo.Update(taskID, expectedVersion, func(Task) (Task, error) {
		return candidate, nil
	})
	if err != nil {
		return Task{}, err
	}
	if updated.Name != current.Name && updated.ThreadID != "" {
		s.syncThreadTitle(updated.ThreadID, updated.Name)
	}
	s.nudge()
	s.publishTaskChanged(updated)
	return updated, nil
}

// syncThreadTitle follows a task rename onto the dedicated thread when the
// executor supports it; a user-manual thread title is never overwritten.
func (s *Service) syncThreadTitle(threadID, taskName string) {
	updater, ok := s.executor.(interface {
		UpdateScheduledThreadTitle(threadID, taskName string) error
	})
	if !ok {
		return
	}
	_ = updater.UpdateScheduledThreadTitle(threadID, taskName)
}

// Delete tombstones a task under version CAS. A task with an active
// execution cannot be deleted (contract §4.4): cancel it or wait for it to
// finish. The dedicated thread and its artifacts are never deleted with the
// task. An idempotency key lets retries replay a previously successful
// delete as success, even after the task becomes invisible to Get.
func (s *Service) Delete(taskID string, expectedVersion int64, idempotencyKey string) error {
	if scheduler := s.schedulerOrNil(); scheduler != nil && scheduler.IsActive(taskID) {
		return ErrTaskAlreadyRunning
	}
	task, err := s.repo.Get(taskID)
	if err != nil {
		if !errors.Is(err, ErrTaskNotFound) {
			return err
		}
		// Task is already tombstoned; a matching idempotency receipt may still
		// replay the previous success.
		task = Task{}
	}
	if err := s.repo.Delete(taskID, expectedVersion, idempotencyKey); err != nil {
		return err
	}
	s.nudge()
	s.PublishEvent(Event{
		Type:      EventTaskChanged,
		TaskID:    taskID,
		ProjectID: task.ProjectID,
		TaskName:  task.Name,
		Status:    "deleted",
	})
	return nil
}

// Pause disables future triggers under version CAS. An active execution is
// not interrupted.
func (s *Service) Pause(taskID string, expectedVersion int64) (Task, error) {
	updated, err := s.repo.SetPaused(taskID, expectedVersion)
	if err != nil {
		return Task{}, err
	}
	s.nudge()
	s.publishTaskChanged(updated)
	return updated, nil
}

// Resume re-enables a task; the scheduler recomputes the next run.
func (s *Service) Resume(taskID string, expectedVersion int64) (Task, error) {
	updated, err := s.repo.SetResumed(taskID, expectedVersion)
	if err != nil {
		return Task{}, err
	}
	s.nudge()
	s.publishTaskChanged(updated)
	return updated, nil
}

// RunNow dispatches a manual execution (contract §4.4): allowed for paused
// tasks, never consumes the occurrence count, deduplicated by the
// idempotency key, and rejected while the task has an active execution.
func (s *Service) RunNow(taskID, idempotencyKey string) (Execution, error) {
	task, err := s.repo.Get(taskID)
	if err != nil {
		return Execution{}, err
	}
	if task.InvalidReason != "" {
		return Execution{}, fmt.Errorf("%w: task is invalid", ErrValidationFailed)
	}
	scheduler := s.schedulerOrNil()
	if scheduler == nil {
		return Execution{}, errors.New("scheduledtask: scheduler is not running")
	}
	// Scheduler.RunManual resolves the caller's idempotency key before checking
	// the active-run guard. A lost 202 may therefore be retried even while the
	// original worker is still unwinding; only a distinct key conflicts.
	return scheduler.RunManual(task, idempotencyKey)
}

// CancelExecution cancels the task's current queued or running execution.
func (s *Service) CancelExecution(taskID string) error {
	scheduler := s.schedulerOrNil()
	if scheduler == nil || !scheduler.CancelExecution(taskID) {
		return ErrExecutionNotFound
	}
	return nil
}

// IsRunning reports whether the task has an active execution.
func (s *Service) IsRunning(taskID string) bool {
	scheduler := s.schedulerOrNil()
	return scheduler != nil && scheduler.IsActive(taskID)
}

// Close releases the repository (journals and locks). The scheduler must be
// stopped first.
func (s *Service) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	return s.repo.Close()
}

func (s *Service) nudge() {
	if scheduler := s.schedulerOrNil(); scheduler != nil {
		scheduler.Reload()
	}
}

func (s *Service) publishTaskChanged(task Task) {
	s.PublishEvent(Event{
		Type:      EventTaskChanged,
		TaskID:    task.TaskID,
		ProjectID: task.ProjectID,
		ThreadID:  task.ThreadID,
		TaskName:  task.Name,
		Status:    string(task.State(s.IsRunning(task.TaskID))),
	})
}
