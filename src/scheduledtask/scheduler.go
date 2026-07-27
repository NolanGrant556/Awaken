package scheduledtask

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Timer abstracts the scheduler's wait primitive so unit tests never wait on
// the wall clock.
type Timer interface {
	C() <-chan time.Time
	Stop()
}

type realTimer struct{ inner *time.Timer }

// RealTimer is the production timer factory.
func RealTimer(d time.Duration) Timer {
	if d < 0 {
		d = 0
	}
	return &realTimer{inner: time.NewTimer(d)}
}

func (t *realTimer) C() <-chan time.Time { return t.inner.C }
func (t *realTimer) Stop()               { t.inner.Stop() }

// ManualTimer is a test timer fired explicitly by the test.
type ManualTimer struct {
	mu      sync.Mutex
	ch      chan time.Time
	fired   bool
	stopped bool
}

// NewManualTimer returns an unfired manual timer.
func NewManualTimer() *ManualTimer {
	return &ManualTimer{ch: make(chan time.Time, 1)}
}

// C implements Timer.
func (t *ManualTimer) C() <-chan time.Time { return t.ch }

// Stop implements Timer.
func (t *ManualTimer) Stop() {
	t.mu.Lock()
	t.stopped = true
	t.mu.Unlock()
}

// Fire delivers one tick unless the timer already fired or was stopped. It
// reports whether the tick was delivered.
func (t *ManualTimer) Fire(at time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.fired || t.stopped {
		return false
	}
	t.fired = true
	t.ch <- at
	return true
}

// maxConcurrentAgentExecutions is the global agent semaphore (contract §8.4).
// Reminders never occupy it.
const maxConcurrentAgentExecutions = 2

// activeState is the lifecycle of one dispatched execution.
type activeState int

const (
	activeQueued activeState = iota
	activeRunning
	activeCancelled
	activeDone
)

// activeExecution tracks one dispatched execution of a task. All fields are
// protected by the scheduler mutex except cancelCh, which is closed only once.
type activeExecution struct {
	executionID string
	state       activeState
	shutdown    bool // true when Stop requested; worker leaves terminal to recovery
	cancelOnce  sync.Once
	cancelCh    chan struct{}
	execCancel  context.CancelFunc // cancels the per-execution side-effect context
}

func (a *activeExecution) cancel() {
	a.cancelOnce.Do(func() { close(a.cancelCh) })
}

// SchedulerConfig wires the scheduler. NewTimer and Emit are optional; Clock
// defaults to RealClock.
type SchedulerConfig struct {
	Repo      Repository
	Clock     Clock
	Executor  Executor
	Validator Validator
	NewTimer  func(d time.Duration) Timer
	Emit      func(Event)
}

// Scheduler owns the scheduling loop, misfire recovery, per-task re-entry
// protection and the global agent concurrency cap (contract §8). All timer
// computation goes through the injectable clock and timer factory.
type Scheduler struct {
	repo      Repository
	clock     Clock
	executor  Executor
	validator Validator
	newTimer  func(d time.Duration) Timer
	emit      func(Event)

	mu       sync.Mutex
	started  bool
	stopCh   chan struct{}
	loopDone chan struct{}
	nudge    chan struct{}
	active   map[string]*activeExecution // taskID -> active execution
	agentSem chan struct{}
	workers  sync.WaitGroup
	baseCtx  context.Context
	baseStop context.CancelFunc
}

// NewScheduler builds a scheduler; Start performs recovery and arms timers.
func NewScheduler(cfg SchedulerConfig) *Scheduler {
	clock := cfg.Clock
	if clock == nil {
		clock = RealClock{}
	}
	newTimer := cfg.NewTimer
	if newTimer == nil {
		newTimer = RealTimer
	}
	return &Scheduler{
		repo:      cfg.Repo,
		clock:     clock,
		executor:  cfg.Executor,
		validator: cfg.Validator,
		newTimer:  newTimer,
		emit:      cfg.Emit,
		nudge:     make(chan struct{}, 1),
		active:    map[string]*activeExecution{},
		agentSem:  make(chan struct{}, maxConcurrentAgentExecutions),
	}
}

// Start runs the startup recovery of contract §8.1 and launches the
// scheduling loop. It is idempotent. Recovery seals leftover executions,
// re-validates every enabled definition (a single bad definition becomes
// invalid without blocking the others), settles exhausted schedules, applies
// the misfire rules and only then arms the first timer; the caller starts
// HTTP afterwards.
func (s *Scheduler) Start() error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}
	s.started = true
	s.stopCh = make(chan struct{})
	s.loopDone = make(chan struct{})
	s.baseCtx, s.baseStop = context.WithCancel(context.Background())
	s.mu.Unlock()

	if err := s.recover(); err != nil {
		// Roll back the started state so the next Start retries and Stop does
		// not wait on a loop that never launched.
		s.mu.Lock()
		s.started = false
		if s.baseStop != nil {
			s.baseStop()
		}
		s.baseCtx = nil
		s.baseStop = nil
		s.stopCh = nil
		s.loopDone = nil
		s.mu.Unlock()
		return err
	}

	// The loop is tracked through loopDone, not the worker WaitGroup, so
	// waitWorkers only joins execution workers.
	go s.loop()
	return nil
}

// Stop implements the shutdown order of contract §8.1: stop accepting new
// occurrences, mark queued entries cancelled, request cancellation of active
// scheduled runs and return inside the caller's graceful-shutdown budget
// without waiting for terminal records; unsealed executions are recovered as
// interrupted on the next start. It is idempotent.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return
	}
	s.started = false
	stopCh := s.stopCh
	baseStop := s.baseStop
	// Cancel every active execution through the unified cancellation path.
	// shutdown=true tells workers not to write terminal records; recovery will
	// seal leftover queued/running executions as interrupted on the next start.
	running := make([]string, 0)
	for taskID, active := range s.active {
		if active.state == activeDone {
			continue
		}
		if active.state == activeRunning {
			running = append(running, taskID)
		}
		active.state = activeCancelled
		active.shutdown = true
		active.cancel()
		if active.execCancel != nil {
			active.execCancel()
		}
	}
	s.mu.Unlock()

	for _, taskID := range running {
		s.executor.Cancel(taskID)
	}
	if baseStop != nil {
		baseStop()
	}
	if stopCh != nil {
		close(stopCh)
		<-s.loopDone
	}
}

// Reload nudges the loop to recompute the due table after task mutations
// (create/edit/pause/resume/delete). It coalesces and never blocks.
func (s *Scheduler) Reload() {
	select {
	case s.nudge <- struct{}{}:
	default:
	}
}

// IsActive reports whether the task currently has a dispatched (queued or
// running) execution.
func (s *Scheduler) IsActive(taskID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.active[taskID]
	return ok
}

// waitWorkers joins all worker goroutines; tests use it after Stop or after
// releasing blocked executors. Not part of the shutdown contract path.
func (s *Scheduler) waitWorkers() {
	s.workers.Wait()
}

func (s *Scheduler) publish(event Event) {
	if s.emit != nil {
		s.emit(event)
	}
}

// recover performs the §8.1 startup recovery synchronously.
func (s *Scheduler) recover() error {
	tasks, err := s.repo.List()
	if err != nil {
		return err
	}
	now := s.clock.Now().UTC()
	for _, task := range tasks {
		if _, err := s.repo.SealInterrupted(task.TaskID, s.clock); err != nil {
			return err
		}
	}
	for _, task := range tasks {
		if task.Completed || !task.Enabled || task.InvalidReason != "" {
			continue
		}
		if err := s.validator.ValidateTask(task); err != nil {
			if _, markErr := s.repo.Update(task.TaskID, task.Version, func(current Task) (Task, error) {
				current.InvalidReason = err.Error()
				return current, nil
			}); markErr != nil {
				return markErr
			}
		}
	}
	// Completion sweep: a crash can leave a schedule exhausted but unmarked.
	tasks, err = s.repo.List()
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task.Completed || task.InvalidReason != "" {
			continue
		}
		if s.scheduleExhausted(task) {
			if _, err := s.repo.MarkCompleted(task.TaskID); err != nil {
				return err
			}
		}
	}
	// Misfire pass: coalesce missed occurrences per task (§8.3).
	tasks, err = s.repo.List()
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task.Completed || !task.Enabled || task.InvalidReason != "" {
			continue
		}
		decision, err := DecideMisfire(task.Schedule, task.LastProcessedOccurrence, task.CreatedAt, now, s.grace(task))
		if err != nil {
			continue
		}
		switch decision.Action {
		case MisfireRunOnce:
			s.dispatch(task, decision.Occurrence, TriggerMisfire, "")
		case MisfireSkip:
			s.recordSkipped(task, decision.Occurrence, TriggerMisfire, ErrorCodeMisfireGraceExceeded, "occurrence exceeded misfire grace")
			if decision.Complete {
				if _, err := s.repo.MarkCompleted(task.TaskID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// scheduleExhausted reports whether the task's schedule can never produce
// another occurrence: a one-time calendar task whose single occurrence was
// already processed, or a count-bounded repeat that reached its count.
func (s *Scheduler) scheduleExhausted(task Task) bool {
	if task.Schedule.Kind == ScheduleKindCalendar && task.Schedule.RepeatEveryDays <= 0 {
		return task.LastProcessedOccurrence != nil
	}
	if end := task.Schedule.RepeatEnd; end != nil && end.Kind == RepeatEndCount {
		return task.ConsumedOccurrences >= end.Count
	}
	return false
}

func (s *Scheduler) grace(task Task) time.Duration {
	return time.Duration(task.Execution.MisfireGraceSeconds) * time.Second
}

// loop is the single scheduling goroutine: one next-due timer, recomputed on
// every fire or nudge (task counts are small).
func (s *Scheduler) loop() {
	defer close(s.loopDone)
	for {
		next, ok := s.nextDue()
		var timer Timer
		var fire <-chan time.Time
		if ok {
			timer = s.newTimer(next.Sub(s.clock.Now()))
			fire = timer.C()
		}
		select {
		case <-fire:
			timer.Stop()
			s.processDue()
		case <-s.nudge:
			if timer != nil {
				timer.Stop()
			}
		case <-s.stopCh:
			if timer != nil {
				timer.Stop()
			}
			return
		}
	}
}

// nextDue computes the earliest armed instant across schedulable tasks. A
// task with no future occurrence but an unprocessed due one (for example a
// just-created calendar task with a past run_at) is due immediately.
func (s *Scheduler) nextDue() (time.Time, bool) {
	tasks, err := s.repo.List()
	if err != nil {
		return time.Time{}, false
	}
	now := s.clock.Now().UTC()
	var earliest time.Time
	found := false
	for _, task := range tasks {
		if !s.schedulable(task) {
			continue
		}
		if next, ok := NextOccurrence(task.Schedule, now); ok {
			if !found || next.Before(earliest) {
				earliest = next
				found = true
			}
			continue
		}
		decision, err := DecideMisfire(task.Schedule, task.LastProcessedOccurrence, task.CreatedAt, now, s.grace(task))
		if err != nil || decision.Action == MisfireNone {
			continue
		}
		if !found || now.Before(earliest) {
			earliest = now
			found = true
		}
	}
	return earliest, found
}

func (s *Scheduler) schedulable(task Task) bool {
	return task.Enabled && !task.Completed && task.InvalidReason == ""
}

// processDue evaluates every schedulable task at the current time and
// dispatches or skips whatever is due. Occurrence idempotency in the
// repository makes repeated evaluation safe across restarts and timer lag.
func (s *Scheduler) processDue() {
	tasks, err := s.repo.List()
	if err != nil {
		return
	}
	now := s.clock.Now().UTC()
	for _, task := range tasks {
		if !s.schedulable(task) {
			continue
		}
		decision, err := DecideMisfire(task.Schedule, task.LastProcessedOccurrence, task.CreatedAt, now, s.grace(task))
		if err != nil {
			continue
		}
		switch decision.Action {
		case MisfireRunOnce:
			s.dispatch(task, decision.Occurrence, TriggerScheduled, "")
		case MisfireSkip:
			s.recordSkipped(task, decision.Occurrence, TriggerScheduled, ErrorCodeMisfireGraceExceeded, "occurrence exceeded misfire grace")
			if decision.Complete {
				_, _ = s.repo.MarkCompleted(task.TaskID)
			}
		}
	}
}

// dispatch creates the execution record for one occurrence (or one manual
// run) and starts its worker. Occurrence/idempotency dedup and the per-task
// re-entry guard live here; everything else lives in the worker.
func (s *Scheduler) dispatch(task Task, occurrence time.Time, trigger Trigger, idempotencyKey string) Execution {
	manual := trigger == TriggerManual
	if !manual {
		// Already processed (restart / double-fire dedup).
		if task.LastProcessedOccurrence != nil && !occurrence.After(*task.LastProcessedOccurrence) {
			return Execution{}
		}
		if s.scheduleExhausted(task) {
			_, _ = s.repo.MarkCompleted(task.TaskID)
			return Execution{}
		}
	}
	exec, created, err := s.repo.AppendExecution(task.TaskID, Execution{
		Trigger:        trigger,
		ScheduledFor:   occurrence.UTC(),
		TaskVersion:    task.Version,
		Status:         ExecutionStatusQueued,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return Execution{}
	}
	if !created {
		// A pre-existing record (dedup hit). Terminal records are settled;
		// in-flight ones belong to an active worker. Either way there is
		// nothing new to dispatch. Make sure the occurrence is consumed and the
		// task completion state is up to date (e.g. one-time calendar after crash).
		if !manual {
			if err := s.consumeIfNeeded(task, occurrence); err != nil {
				// The existing execution is already in flight or terminal; record a
				// separate persistence-failure terminal for this dispatch attempt.
				s.transitionTerminal(task, exec, ExecutionStatusError, ErrorCodePersistenceFailed, err.Error(), "", "")
				s.publishFinished(task, exec, ExecutionStatusError, "")
				return exec
			}
			s.checkCompletion(task.TaskID)
		}
		return exec
	}

	s.mu.Lock()
	if _, busy := s.active[task.TaskID]; busy {
		s.mu.Unlock()
		if manual {
			// RunNow surfaces a domain error instead of a skipped record.
			s.finalizeQueuedManualConflict(task, exec)
			return exec
		}
		s.transitionTerminal(task, exec, ExecutionStatusSkipped, ErrorCodeTaskAlreadyRunning, "task already has an active execution", "", "")
		if err := s.consumeIfNeeded(task, occurrence); err != nil {
			// Occurrence consumption is atomic and conflicts are now impossible; a
			// failure here is a hard persistence error. The skipped record stays
			// terminal; the next scheduler loop will re-evaluate this occurrence.
			_ = err
		}
		s.checkCompletion(task.TaskID)
		return exec
	}
	active := &activeExecution{executionID: exec.ExecutionID, cancelCh: make(chan struct{})}
	s.active[task.TaskID] = active
	s.mu.Unlock()

	s.workers.Add(1)
	go s.runExecution(active, task, exec, occurrence, manual)
	return exec
}

// finalizeQueuedManualConflict records a manual run that collided with an
// active execution as skipped, mirroring the scheduled-occurrence behaviour;
// RunNow additionally returns ErrTaskAlreadyRunning before this path when the
// collision is visible up front.
func (s *Scheduler) finalizeQueuedManualConflict(task Task, exec Execution) {
	s.transitionTerminal(task, exec, ExecutionStatusSkipped, ErrorCodeTaskAlreadyRunning, "task already has an active execution", "", "")
}

// consumeIfNeeded advances the occurrence cursor exactly once per occurrence.
func (s *Scheduler) consumeIfNeeded(task Task, occurrence time.Time) error {
	current, err := s.repo.Get(task.TaskID)
	if err != nil {
		return err
	}
	if current.LastProcessedOccurrence != nil && !occurrence.After(*current.LastProcessedOccurrence) {
		return nil
	}
	_, err = s.repo.ConsumeOccurrence(task.TaskID, occurrence)
	return err
}

// recordSkipped persists a skipped execution for an occurrence that will
// never run (misfire grace exceeded), consumes the occurrence and publishes
// the finished event. It is idempotent across restarts.
func (s *Scheduler) recordSkipped(task Task, occurrence time.Time, trigger Trigger, code, message string) {
	exec, created, err := s.repo.AppendExecution(task.TaskID, Execution{
		Trigger:      trigger,
		ScheduledFor: occurrence.UTC(),
		TaskVersion:  task.Version,
		Status:       ExecutionStatusQueued,
	})
	if err != nil {
		return
	}
	if created {
		now := s.clock.Now().UTC()
		exec, _, err = s.repo.AppendExecution(task.TaskID, Execution{
			ExecutionID:  exec.ExecutionID,
			Trigger:      trigger,
			ScheduledFor: occurrence.UTC(),
			TaskVersion:  task.Version,
			StartedAt:    now,
			FinishedAt:   &now,
			Status:       ExecutionStatusSkipped,
			ErrorCode:    code,
			ErrorMessage: message,
		})
		if err != nil {
			return
		}
	}
	if err := s.consumeIfNeeded(task, occurrence); err != nil {
		// The skipped record is already terminal; a consumption failure here is
		// best-effort surfaced through the next loop's re-evaluation.
		_ = err
	}
	s.checkCompletion(task.TaskID)
	s.publish(Event{
		Type:        EventExecutionFinished,
		TaskID:      task.TaskID,
		ExecutionID: exec.ExecutionID,
		TaskName:    task.Name,
		Status:      string(ExecutionStatusSkipped),
	})
}

// runExecution is the worker for one dispatched execution: global agent
// semaphore with misfire-grace queueing, occurrence consumption, execution,
// terminal record, thread binding and completion bookkeeping.
func (s *Scheduler) runExecution(active *activeExecution, task Task, exec Execution, occurrence time.Time, manual bool) {
	defer s.workers.Done()
	defer func() {
		s.mu.Lock()
		if s.active[task.TaskID] == active {
			delete(s.active, task.TaskID)
		}
		active.state = activeDone
		s.mu.Unlock()
	}()

	agentTask := task.Action.Kind == ActionKindAgent
	if agentTask {
		if status, code, message := s.acquireAgentSlot(active, task, occurrence); status != "" {
			if status == ExecutionStatusSkipped {
				s.transitionTerminal(task, exec, status, code, message, "", "")
				s.consumeIfNeeded(task, occurrence)
				s.checkCompletion(task.TaskID)
				s.publishFinished(task, exec, ExecutionStatusSkipped, "")
			} else if !s.isShutdown(active) {
				s.transitionTerminal(task, exec, status, code, message, "", "")
				s.publishFinished(task, exec, ExecutionStatusCancelled, "")
			}
			return
		}
		defer func() { <-s.agentSem }()
	}

	if s.handleUserCancel(active, task, exec, "execution cancelled while queued") {
		return
	}

	// Consume the occurrence before executing (§8.4: scheduled/misfire
	// occurrences count whether they succeed, fail or skip; manual never
	// counts).
	if !manual {
		if err := s.consumeIfNeeded(task, occurrence); err != nil {
			s.transitionTerminal(task, exec, ExecutionStatusError, ErrorCodePersistenceFailed, err.Error(), "", "")
			s.publishFinished(task, exec, ExecutionStatusError, "")
			return
		}
	}

	if s.handleUserCancel(active, task, exec, "execution cancelled while queued") {
		return
	}

	now := s.clock.Now().UTC()
	runningExec, _, err := s.repo.AppendExecution(task.TaskID, Execution{
		ExecutionID:  exec.ExecutionID,
		Trigger:      exec.Trigger,
		ScheduledFor: exec.ScheduledFor,
		TaskVersion:  task.Version,
		StartedAt:    now,
		Status:       ExecutionStatusRunning,
	})
	if err != nil {
		return
	}
	// Preserve the real start time on the local execution so the terminal
	// transition does not overwrite it with the finish time.
	exec = runningExec

	s.mu.Lock()
	if active.state == activeCancelled {
		shutdown := active.shutdown
		s.mu.Unlock()
		if !shutdown {
			s.transitionTerminal(task, exec, ExecutionStatusCancelled, ErrorCodeRunCancelled, "execution cancelled while running", "", "")
			s.publishFinished(task, exec, ExecutionStatusCancelled, "")
		}
		return
	}
	active.state = activeRunning
	// Create a per-execution context so cancellation reaches the executor even
	// after the run starts and before registerActive has observed it.
	execCtx, execCancel := context.WithCancel(s.baseCtx)
	active.execCancel = execCancel
	s.mu.Unlock()
	defer execCancel()

	s.publish(Event{
		Type:        EventExecutionStarted,
		TaskID:      task.TaskID,
		ExecutionID: exec.ExecutionID,
		TaskName:    task.Name,
		Status:      string(ExecutionStatusRunning),
	})

	var runID, threadID string
	var execErr *ExecError
	if agentTask {
		runID, threadID, execErr = s.executor.ExecuteAgent(execCtx, task, exec)
	} else {
		threadID, execErr = s.executor.ExecuteReminder(execCtx, task, exec)
	}

	if threadID != "" {
		s.bindThread(task.TaskID, threadID)
	}

	switch {
	case execErr == nil:
		s.transitionTerminal(task, exec, ExecutionStatusSuccess, "", "", runID, threadID)
		s.publishFinished(task, exec, ExecutionStatusSuccess, threadID)
		if !agentTask {
			// Reminder notice: Desktop turns this into a native notification.
			s.publish(Event{
				Type:        EventNotice,
				TaskID:      task.TaskID,
				ExecutionID: exec.ExecutionID,
				ThreadID:    threadID,
				TaskName:    task.Name,
				Status:      string(ExecutionStatusSuccess),
				Content:     task.Action.Content,
			})
		}
	case execErr.Skipped:
		s.transitionTerminal(task, exec, ExecutionStatusSkipped, execErr.Code, execErr.Message, runID, threadID)
		s.publishFinished(task, exec, ExecutionStatusSkipped, threadID)
	case execErr.Cancelled:
		s.transitionTerminal(task, exec, ExecutionStatusCancelled, execErr.Code, execErr.Message, runID, threadID)
		s.publishFinished(task, exec, ExecutionStatusCancelled, threadID)
	default:
		s.transitionTerminal(task, exec, ExecutionStatusError, execErr.Code, execErr.Message, runID, threadID)
		s.publishFinished(task, exec, ExecutionStatusError, threadID)
	}

	if !manual {
		s.checkCompletion(task.TaskID)
	}
}

func (s *Scheduler) checkCancelled(active *activeExecution) bool {
	s.mu.Lock()
	cancelled := active.state == activeCancelled
	s.mu.Unlock()
	return cancelled
}

func (s *Scheduler) isShutdown(active *activeExecution) bool {
	s.mu.Lock()
	shutdown := active.shutdown
	s.mu.Unlock()
	return shutdown
}

// handleUserCancel writes a terminal cancelled record when the user cancelled
// before the execution reached a side-effect stage. Shutdown cancellations are
// left non-terminal so recovery can seal them as interrupted.
func (s *Scheduler) handleUserCancel(active *activeExecution, task Task, exec Execution, message string) bool {
	if !s.checkCancelled(active) {
		return false
	}
	if !s.isShutdown(active) {
		s.transitionTerminal(task, exec, ExecutionStatusCancelled, ErrorCodeRunCancelled, message, "", "")
		s.publishFinished(task, exec, ExecutionStatusCancelled, "")
	}
	return true
}

// acquireAgentSlot takes the global agent semaphore, queueing inside the
// task's misfire grace (contract §4.2). It returns a non-empty terminal
// status when the execution must end before starting: skipped when the grace
// expired, cancelled when the caller cancelled while queued.
func (s *Scheduler) acquireAgentSlot(active *activeExecution, task Task, occurrence time.Time) (ExecutionStatus, string, string) {
	s.mu.Lock()
	cancelled := active.state == activeCancelled
	s.mu.Unlock()
	if cancelled {
		return ExecutionStatusCancelled, ErrorCodeRunCancelled, "execution cancelled while queued"
	}
	select {
	case s.agentSem <- struct{}{}:
		return "", "", ""
	default:
	}
	deadline := occurrence.Add(s.grace(task))
	remaining := deadline.Sub(s.clock.Now())
	if remaining <= 0 {
		return ExecutionStatusSkipped, ErrorCodeAgentConcurrencyExceeded, "agent concurrency exceeded misfire grace"
	}
	timer := s.newTimer(remaining)
	defer timer.Stop()
	select {
	case s.agentSem <- struct{}{}:
		return "", "", ""
	case <-active.cancelCh:
		return ExecutionStatusCancelled, ErrorCodeRunCancelled, "execution cancelled while queued"
	case <-timer.C():
		return ExecutionStatusSkipped, ErrorCodeAgentConcurrencyExceeded, "agent concurrency exceeded misfire grace"
	case <-s.baseCtx.Done():
		return ExecutionStatusCancelled, ErrorCodeRunCancelled, "execution cancelled while queued"
	}
}

// transitionTerminal appends the terminal record of an execution.
func (s *Scheduler) transitionTerminal(task Task, exec Execution, status ExecutionStatus, code, message, runID, threadID string) {
	now := s.clock.Now().UTC()
	startedAt := exec.StartedAt
	if startedAt.IsZero() {
		startedAt = now
	}
	_, _, _ = s.repo.AppendExecution(task.TaskID, Execution{
		ExecutionID:  exec.ExecutionID,
		Trigger:      exec.Trigger,
		ScheduledFor: exec.ScheduledFor,
		TaskVersion:  task.Version,
		StartedAt:    startedAt,
		FinishedAt:   &now,
		Status:       status,
		RunID:        runID,
		ThreadID:     threadID,
		ErrorCode:    code,
		ErrorMessage: message,
	})
}

func (s *Scheduler) publishFinished(task Task, exec Execution, status ExecutionStatus, threadID string) {
	s.publish(Event{
		Type:        EventExecutionFinished,
		TaskID:      task.TaskID,
		ExecutionID: exec.ExecutionID,
		ThreadID:    threadID,
		TaskName:    task.Name,
		Status:      string(status),
	})
}

// bindThread records the task's dedicated thread after its first execution.
// The task may have been edited concurrently, so the CAS retries on the
// freshest version while the task still has no thread.
func (s *Scheduler) bindThread(taskID, threadID string) {
	for attempt := 0; attempt < 4; attempt++ {
		current, err := s.repo.Get(taskID)
		if err != nil || current.ThreadID != "" {
			return
		}
		if _, err := s.repo.BindThread(taskID, current.Version, threadID); err != nil {
			if errors.Is(err, ErrVersionConflict) {
				continue
			}
			return
		}
		return
	}
}

// checkCompletion marks a task completed once its schedule is exhausted.
func (s *Scheduler) checkCompletion(taskID string) {
	current, err := s.repo.Get(taskID)
	if err != nil || current.Completed {
		return
	}
	if s.scheduleExhausted(current) {
		_, _ = s.repo.MarkCompleted(taskID)
		s.Reload()
	}
}

// RunManual dispatches a trigger=manual execution (contract §4.4): allowed
// for paused tasks, never consumes the occurrence count, deduplicated by the
// caller's idempotency key.
func (s *Scheduler) RunManual(task Task, idempotencyKey string) (Execution, error) {
	// Idempotency: a retry with the same key returns the existing execution.
	executions, err := s.repo.ListExecutions(task.TaskID, MaxExecutionLimit)
	if err != nil {
		return Execution{}, err
	}
	for _, existing := range executions {
		if existing.Trigger == TriggerManual && existing.IdempotencyKey == idempotencyKey {
			return existing, nil
		}
	}
	s.mu.Lock()
	_, busy := s.active[task.TaskID]
	s.mu.Unlock()
	if busy {
		return Execution{}, ErrTaskAlreadyRunning
	}
	exec := s.dispatch(task, s.clock.Now().UTC(), TriggerManual, idempotencyKey)
	if exec.ExecutionID == "" {
		return Execution{}, ErrTaskAlreadyRunning
	}
	return exec, nil
}

// CancelExecution cancels the task's currently dispatched execution: queued
// entries are marked cancelled directly, running ones are cancelled through
// the executor (contract §4.4). It reports whether there was anything to
// cancel.
func (s *Scheduler) CancelExecution(taskID string) bool {
	s.mu.Lock()
	active, ok := s.active[taskID]
	if !ok || active.state == activeDone || active.state == activeCancelled {
		s.mu.Unlock()
		return false
	}
	wasRunning := active.state == activeRunning
	active.state = activeCancelled
	active.cancel()
	if active.execCancel != nil {
		active.execCancel()
	}
	s.mu.Unlock()
	if wasRunning {
		s.executor.Cancel(taskID)
	}
	return true
}
