package scheduledtask

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// --- shared test harness -------------------------------------------------

// timerCapture records every timer the scheduler/workers create so tests can
// fire the scheduler loop explicitly; no test ever waits on the wall clock
// for scheduling behaviour. maxDur optionally excludes long timers (worker
// misfire-grace waits) from firing: latest only returns timers armed with
// d <= maxDur, which keeps grace timers test-controlled.
type timerCapture struct {
	mu     sync.Mutex
	timers []*ManualTimer
	durs   []time.Duration
	maxDur time.Duration
}

func (c *timerCapture) newTimer(d time.Duration) Timer {
	timer := NewManualTimer()
	c.mu.Lock()
	c.timers = append(c.timers, timer)
	c.durs = append(c.durs, d)
	c.mu.Unlock()
	return timer
}

func (c *timerCapture) latest() *ManualTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.timers) - 1; i >= 0; i-- {
		if c.maxDur > 0 && c.durs[i] > c.maxDur {
			continue
		}
		return c.timers[i]
	}
	return nil
}

// fireUntil fires the newest live scheduler timer at the clock's current
// time until cond holds. Repeated firing is safe: every processDue pass is
// idempotent (occurrence dedup), and a tick lost to a concurrent re-arm is
// retried on the next iteration.
func (c *timerCapture) fireUntil(t *testing.T, clock *FakeClock, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		if timer := c.latest(); timer != nil {
			timer.Fire(clock.Now())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// fireFor keeps firing the newest live timer for d; negative-path tests use
// it to prove nothing happens no matter how often the loop wakes.
func (c *timerCapture) fireFor(clock *FakeClock, d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if timer := c.latest(); timer != nil {
			timer.Fire(clock.Now())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// eventCapture collects scheduler/service events.
type eventCapture struct {
	mu     sync.Mutex
	events []Event
}

func (c *eventCapture) emit(event Event) {
	c.mu.Lock()
	c.events = append(c.events, event)
	c.mu.Unlock()
}

func (c *eventCapture) snapshot() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Event(nil), c.events...)
}

func (c *eventCapture) count(ofType EventType) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, event := range c.events {
		if event.Type == ofType {
			n++
		}
	}
	return n
}

type titleSyncCall struct {
	threadID string
	taskName string
}

// schedFakeExecutor is the test Executor. Agent calls optionally block until
// released (or the scheduler base context is cancelled); Cancel records the
// propagation and marks the task so a released call returns a cancelled
// outcome, mirroring the production executor contract.
type schedFakeExecutor struct {
	mu            sync.Mutex
	block         bool
	releaseCh     chan struct{}
	releaseOnce   sync.Once
	enteredCh     chan string
	agentTasks    []string
	agentExecs    []Execution
	reminderTasks []string
	cancels       []string
	cancelled     map[string]bool
	titleSyncs    []titleSyncCall
	agentErr      *ExecError
	reminderErr   *ExecError
	onStarted     func(task Task, exec Execution, runID, threadID string)
}

func newSchedFakeExecutor() *schedFakeExecutor {
	return &schedFakeExecutor{
		releaseCh: make(chan struct{}),
		enteredCh: make(chan string, 64),
		cancelled: map[string]bool{},
	}
}

func (e *schedFakeExecutor) setBlock(block bool) {
	e.mu.Lock()
	e.block = block
	e.mu.Unlock()
}

func (e *schedFakeExecutor) release() {
	e.releaseOnce.Do(func() { close(e.releaseCh) })
}

func (e *schedFakeExecutor) ExecuteAgent(ctx context.Context, task Task, exec Execution) (string, string, *ExecError) {
	e.mu.Lock()
	e.agentTasks = append(e.agentTasks, task.TaskID)
	e.agentExecs = append(e.agentExecs, exec)
	block := e.block
	agentErr := e.agentErr
	onStarted := e.onStarted
	e.mu.Unlock()
	if agentErr != nil {
		return "", "", agentErr
	}
	runID := "run-" + exec.ExecutionID
	threadID := "thread-" + task.TaskID
	if onStarted != nil {
		onStarted(task, exec, runID, threadID)
	}
	if block {
		e.enteredCh <- task.TaskID
		select {
		case <-e.releaseCh:
		case <-ctx.Done():
			return "", "", &ExecError{Code: ErrorCodeRunCancelled, Message: "run cancelled", Cancelled: true}
		}
	}
	e.mu.Lock()
	wasCancelled := e.cancelled[task.TaskID]
	e.mu.Unlock()
	if wasCancelled {
		return "", "", &ExecError{Code: ErrorCodeRunCancelled, Message: "run cancelled", Cancelled: true}
	}
	return runID, threadID, nil
}

func (e *schedFakeExecutor) ExecuteReminder(_ context.Context, task Task, exec Execution) (string, *ExecError) {
	e.mu.Lock()
	e.reminderTasks = append(e.reminderTasks, task.TaskID)
	reminderErr := e.reminderErr
	onStarted := e.onStarted
	e.mu.Unlock()
	if reminderErr != nil {
		return "", reminderErr
	}
	threadID := "thread-" + task.TaskID
	if onStarted != nil {
		onStarted(task, exec, "", threadID)
	}
	return threadID, nil
}

func (e *schedFakeExecutor) SetExecutionStartedHandler(
	handler func(task Task, exec Execution, runID, threadID string),
) {
	e.mu.Lock()
	e.onStarted = handler
	e.mu.Unlock()
}

func (e *schedFakeExecutor) Cancel(taskID string) {
	e.mu.Lock()
	e.cancels = append(e.cancels, taskID)
	e.cancelled[taskID] = true
	e.mu.Unlock()
}

// UpdateScheduledThreadTitle implements the title-sync capability the
// service detects through a type assertion.
func (e *schedFakeExecutor) UpdateScheduledThreadTitle(threadID, taskName string) error {
	e.mu.Lock()
	e.titleSyncs = append(e.titleSyncs, titleSyncCall{threadID: threadID, taskName: taskName})
	e.mu.Unlock()
	return nil
}

func (e *schedFakeExecutor) agentCallCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.agentTasks)
}

func (e *schedFakeExecutor) reminderCallCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.reminderTasks)
}

func (e *schedFakeExecutor) cancelSnapshot() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.cancels...)
}

func (e *schedFakeExecutor) titleSyncSnapshot() []titleSyncCall {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]titleSyncCall(nil), e.titleSyncs...)
}

func (e *schedFakeExecutor) lastAgentExec() Execution {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.agentExecs[len(e.agentExecs)-1]
}

// waitForCond polls cond until it holds or the test times out.
func waitForCond(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitExecutions(t *testing.T, repo Repository, taskID string, n int) []Execution {
	t.Helper()
	var executions []Execution
	waitForCond(t, "execution records", func() bool {
		var err error
		executions, err = repo.ListExecutions(taskID, MaxExecutionLimit)
		return err == nil && len(executions) >= n
	})
	return executions
}

func findExecution(executions []Execution, status ExecutionStatus) *Execution {
	for i := range executions {
		if executions[i].Status == status {
			return &executions[i]
		}
	}
	return nil
}

type schedHarness struct {
	repo   *FileRepository
	clock  *FakeClock
	exec   *schedFakeExecutor
	timers *timerCapture
	events *eventCapture
	sched  *Scheduler
}

func newSchedHarnessAt(t *testing.T, genDir string, clock *FakeClock) *schedHarness {
	t.Helper()
	repo, err := OpenFileRepository(genDir, clock)
	if err != nil {
		t.Fatalf("OpenFileRepository: %v", err)
	}
	h := &schedHarness{
		repo:   repo,
		clock:  clock,
		exec:   newSchedFakeExecutor(),
		timers: &timerCapture{},
		events: &eventCapture{},
	}
	h.sched = NewScheduler(SchedulerConfig{
		Repo:      repo,
		Clock:     clock,
		Executor:  h.exec,
		Validator: Validator{Clock: clock},
		NewTimer:  h.timers.newTimer,
		Emit:      h.events.emit,
	})
	t.Cleanup(func() {
		h.sched.Stop()
		h.exec.release()
		h.sched.waitWorkers()
		_ = repo.Close()
	})
	return h
}

func newSchedHarness(t *testing.T) *schedHarness {
	t.Helper()
	clock := NewFakeClock(time.Date(2026, 7, 26, 10, 0, 30, 0, time.UTC))
	return newSchedHarnessAt(t, t.TempDir(), clock)
}

func (h *schedHarness) mustCreate(t *testing.T, task Task) Task {
	t.Helper()
	created, err := h.repo.Create(task)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return created
}

func (h *schedHarness) mustStart(t *testing.T) {
	t.Helper()
	if err := h.sched.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

func agentCronTask(name, cron string, graceSeconds int) Task {
	return Task{
		Name:      name,
		Enabled:   true,
		Schedule:  Schedule{Kind: ScheduleKindCron, Cron: cron, Timezone: "UTC"},
		Action:    Action{Kind: ActionKindAgent, Content: "do the scheduled thing"},
		Execution: ExecutionOptions{TimeoutSeconds: 600, MisfireGraceSeconds: graceSeconds},
	}
}

func reminderCronTask(name, cron string, graceSeconds int) Task {
	task := agentCronTask(name, cron, graceSeconds)
	task.Action = Action{Kind: ActionKindReminder, Content: "stand up and stretch"}
	return task
}

// --- tests ---------------------------------------------------------------

func TestSchedulerStartStopIdempotent(t *testing.T) {
	h := newSchedHarness(t)
	h.mustStart(t)
	if err := h.sched.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	h.sched.Stop()
	h.sched.Stop()
}

func TestStartupMarksInvalidDefinition(t *testing.T) {
	h := newSchedHarness(t)
	good := h.mustCreate(t, agentCronTask("good", "* * * * *", 300))
	bad := h.mustCreate(t, agentCronTask("bad", "not a cron", 300))

	h.mustStart(t)

	stored, err := h.repo.Get(bad.TaskID)
	if err != nil {
		t.Fatalf("Get bad task: %v", err)
	}
	if stored.InvalidReason == "" {
		t.Fatalf("bad definition not marked invalid")
	}
	if stored.State(false) != TaskStateInvalid {
		t.Fatalf("State() = %q, want %q", stored.State(false), TaskStateInvalid)
	}

	// The invalid definition never blocks a healthy one.
	h.clock.Set(time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC))
	h.timers.fireUntil(t, h.clock, "good task executed", func() bool { return h.exec.agentCallCount() == 1 })
	stored, _ = h.repo.Get(good.TaskID)
	if stored.InvalidReason != "" {
		t.Fatalf("healthy task unexpectedly invalid: %q", stored.InvalidReason)
	}
}

func TestPauseSkipsAndResumeReschedules(t *testing.T) {
	h := newSchedHarness(t)
	task := h.mustCreate(t, agentCronTask("pausable", "* * * * *", 300))
	h.mustStart(t)

	h.clock.Set(time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC))
	h.timers.fireUntil(t, h.clock, "first execution", func() bool { return h.exec.agentCallCount() == 1 })

	fresh, err := h.repo.Get(task.TaskID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, err := h.repo.SetPaused(task.TaskID, fresh.Version); err != nil {
		t.Fatalf("SetPaused: %v", err)
	}

	h.clock.Set(time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC))
	h.timers.fireFor(h.clock, 300*time.Millisecond)
	if got := h.exec.agentCallCount(); got != 1 {
		t.Fatalf("paused task executed: agent calls = %d, want 1", got)
	}

	fresh, _ = h.repo.Get(task.TaskID)
	if _, err := h.repo.SetResumed(task.TaskID, fresh.Version); err != nil {
		t.Fatalf("SetResumed: %v", err)
	}
	h.sched.Reload()

	h.clock.Set(time.Date(2026, 7, 26, 13, 0, 0, 0, time.UTC))
	h.timers.fireUntil(t, h.clock, "execution after resume", func() bool { return h.exec.agentCallCount() == 2 })
}

func TestOccurrenceIdempotentAcrossRestart(t *testing.T) {
	genDir := t.TempDir()
	clock := NewFakeClock(time.Date(2026, 7, 26, 10, 0, 30, 0, time.UTC))
	h := newSchedHarnessAt(t, genDir, clock)
	task := h.mustCreate(t, agentCronTask("restartable", "* * * * *", 300))
	h.mustStart(t)

	clock.Set(time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC))
	h.timers.fireUntil(t, clock, "first-run execution", func() bool { return h.exec.agentCallCount() == 1 })
	waitForCond(t, "occurrence consumed", func() bool {
		fresh, err := h.repo.Get(task.TaskID)
		return err == nil && fresh.LastProcessedOccurrence != nil
	})
	h.sched.Stop()
	h.sched.waitWorkers()
	if err := h.repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Same directory, same clock: a restart must not re-run the occurrence.
	h2 := newSchedHarnessAt(t, genDir, clock)
	h2.mustStart(t)
	h2.timers.fireFor(clock, 300*time.Millisecond)
	if got := h2.exec.agentCallCount(); got != 0 {
		t.Fatalf("occurrence re-executed after restart: agent calls = %d, want 0", got)
	}
	executions, err := h2.repo.ListExecutions(task.TaskID, MaxExecutionLimit)
	if err != nil {
		t.Fatalf("ListExecutions: %v", err)
	}
	if len(executions) != 1 {
		t.Fatalf("executions = %d, want 1", len(executions))
	}
}

func TestReentryGuardSkipsSecondOccurrence(t *testing.T) {
	h := newSchedHarness(t)
	h.exec.setBlock(true)
	task := h.mustCreate(t, agentCronTask("reentry", "* * * * *", 300))
	h.mustStart(t)

	h.clock.Set(time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC))
	h.timers.fireUntil(t, h.clock, "first execution started", func() bool { return h.exec.agentCallCount() == 1 })

	h.clock.Set(time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC))
	h.timers.fireUntil(t, h.clock, "second occurrence settled", func() bool {
		fresh, err := h.repo.Get(task.TaskID)
		return err == nil && fresh.ConsumedOccurrences == 2
	})

	h.exec.release()
	waitForCond(t, "first execution success", func() bool {
		execs, _ := h.repo.ListExecutions(task.TaskID, MaxExecutionLimit)
		return findExecution(execs, ExecutionStatusSuccess) != nil
	})
	executions, _ := h.repo.ListExecutions(task.TaskID, MaxExecutionLimit)
	success := findExecution(executions, ExecutionStatusSuccess)
	skipped := findExecution(executions, ExecutionStatusSkipped)
	if success == nil || skipped == nil {
		t.Fatalf("want one success and one skipped, got %+v", executions)
	}
	if skipped.ErrorCode != ErrorCodeTaskAlreadyRunning {
		t.Fatalf("skipped code = %q, want %q", skipped.ErrorCode, ErrorCodeTaskAlreadyRunning)
	}
	if got := h.exec.agentCallCount(); got != 1 {
		t.Fatalf("agent calls = %d, want 1 (re-entry must not double-run)", got)
	}
}

func TestGlobalConcurrencyCapQueuesThird(t *testing.T) {
	h := newSchedHarness(t)
	// Loop timers here are <= 60s; worker grace waits are 7200s and
	// must only expire under test control, never via fireUntil.
	h.timers.maxDur = 90 * time.Minute
	h.exec.setBlock(true)
	for _, name := range []string{"cap-a", "cap-b", "cap-c"} {
		h.mustCreate(t, agentCronTask(name, "* * * * *", 7200))
	}
	h.mustStart(t)

	h.clock.Set(time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC))
	h.timers.fireUntil(t, h.clock, "two executions running", func() bool { return h.exec.agentCallCount() == 2 })
	// The third execution waits on the global semaphore inside its grace.
	time.Sleep(150 * time.Millisecond)
	if got := h.exec.agentCallCount(); got != 2 {
		t.Fatalf("agent calls = %d, want 2 while semaphore is full", got)
	}

	h.exec.release()
	h.sched.waitWorkers()
	if got := h.exec.agentCallCount(); got != 3 {
		t.Fatalf("agent calls = %d, want 3 after release", got)
	}
}

func TestGlobalConcurrencyCapSkipsWithoutGrace(t *testing.T) {
	h := newSchedHarness(t)
	// Loop timers here are <= 60s; worker grace waits are 7200s and
	// must only expire under test control, never via fireUntil.
	h.timers.maxDur = 90 * time.Minute
	h.exec.setBlock(true)
	tasks := make([]Task, 0, 3)
	for _, name := range []string{"skip-a", "skip-b", "skip-c"} {
		tasks = append(tasks, h.mustCreate(t, agentCronTask(name, "* * * * *", 0)))
	}
	h.mustStart(t)

	h.clock.Set(time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC))
	h.timers.fireUntil(t, h.clock, "two executions running", func() bool { return h.exec.agentCallCount() == 2 })

	// Exactly one task ends skipped with AGENT_CONCURRENCY_EXCEEDED.
	waitForCond(t, "third execution skipped", func() bool {
		for _, task := range tasks {
			executions, err := h.repo.ListExecutions(task.TaskID, MaxExecutionLimit)
			if err == nil && findExecution(executions, ExecutionStatusSkipped) != nil {
				return true
			}
		}
		return false
	})
	skippedTotal := 0
	for _, task := range tasks {
		executions, _ := h.repo.ListExecutions(task.TaskID, MaxExecutionLimit)
		if skipped := findExecution(executions, ExecutionStatusSkipped); skipped != nil {
			skippedTotal++
			if skipped.ErrorCode != ErrorCodeAgentConcurrencyExceeded {
				t.Fatalf("skipped code = %q, want %q", skipped.ErrorCode, ErrorCodeAgentConcurrencyExceeded)
			}
		}
	}
	if skippedTotal != 1 {
		t.Fatalf("skipped executions = %d, want 1", skippedTotal)
	}
	h.exec.release()
	h.sched.waitWorkers()
	if got := h.exec.agentCallCount(); got != 2 {
		t.Fatalf("agent calls = %d, want 2", got)
	}
}

func TestMisfireCoalescedOnceAtStartup(t *testing.T) {
	h := newSchedHarness(t) // clock pinned at 10:00:30
	task := h.mustCreate(t, agentCronTask("hourly", "0 * * * *", 7200))

	// Core was down across 11:00, 12:00 and 13:00; only the newest miss runs.
	h.clock.Set(time.Date(2026, 7, 26, 13, 30, 0, 0, time.UTC))
	h.mustStart(t)

	waitForCond(t, "misfire execution", func() bool { return h.exec.agentCallCount() == 1 })
	executions := waitExecutions(t, h.repo, task.TaskID, 1)
	if len(executions) != 1 {
		t.Fatalf("executions = %d, want 1 coalesced misfire", len(executions))
	}
	exec := executions[0]
	if exec.Trigger != TriggerMisfire {
		t.Fatalf("trigger = %q, want %q", exec.Trigger, TriggerMisfire)
	}
	wantOccurrence := time.Date(2026, 7, 26, 13, 0, 0, 0, time.UTC)
	if !exec.ScheduledFor.Equal(wantOccurrence) {
		t.Fatalf("scheduled_for = %v, want newest miss %v", exec.ScheduledFor, wantOccurrence)
	}
	waitForCond(t, "misfire success", func() bool {
		execs, _ := h.repo.ListExecutions(task.TaskID, MaxExecutionLimit)
		return findExecution(execs, ExecutionStatusSuccess) != nil
	})
	if got := h.exec.agentCallCount(); got != 1 {
		t.Fatalf("agent calls = %d, want 1 (misses coalesced)", got)
	}
}

func TestMisfireBeyondGraceSkipped(t *testing.T) {
	h := newSchedHarness(t)
	task := h.mustCreate(t, agentCronTask("hourly", "0 * * * *", 300))

	h.clock.Set(time.Date(2026, 7, 26, 13, 30, 0, 0, time.UTC))
	h.mustStart(t)

	executions := waitExecutions(t, h.repo, task.TaskID, 1)
	skipped := findExecution(executions, ExecutionStatusSkipped)
	if skipped == nil {
		t.Fatalf("want a skipped execution, got %+v", executions)
	}
	if skipped.ErrorCode != ErrorCodeMisfireGraceExceeded {
		t.Fatalf("skipped code = %q, want %q", skipped.ErrorCode, ErrorCodeMisfireGraceExceeded)
	}
	if skipped.Trigger != TriggerMisfire {
		t.Fatalf("trigger = %q, want %q", skipped.Trigger, TriggerMisfire)
	}
	if got := h.exec.agentCallCount(); got != 0 {
		t.Fatalf("agent calls = %d, want 0", got)
	}
	// The cron task continues: it is skipped, not completed.
	fresh, _ := h.repo.Get(task.TaskID)
	if fresh.Completed {
		t.Fatalf("repeating task must not complete on a skipped misfire")
	}
	if fresh.ConsumedOccurrences != 1 {
		t.Fatalf("consumed = %d, want 1", fresh.ConsumedOccurrences)
	}
}

func TestOneTimeCalendarCompletesAfterSuccess(t *testing.T) {
	h := newSchedHarness(t)
	runAt := time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC)
	task := h.mustCreate(t, Task{
		Name:    "once",
		Enabled: true,
		Schedule: Schedule{
			Kind:     ScheduleKindCalendar,
			Timezone: "UTC",
			RunAt:    &runAt,
		},
		Action:    Action{Kind: ActionKindAgent, Content: "one shot"},
		Execution: ExecutionOptions{TimeoutSeconds: 600, MisfireGraceSeconds: 300},
	})
	h.mustStart(t)

	h.clock.Set(runAt)
	h.timers.fireUntil(t, h.clock, "one-time execution", func() bool { return h.exec.agentCallCount() == 1 })
	waitForCond(t, "task completed", func() bool {
		fresh, err := h.repo.Get(task.TaskID)
		return err == nil && fresh.Completed
	})
	fresh, _ := h.repo.Get(task.TaskID)
	if fresh.State(false) != TaskStateCompleted {
		t.Fatalf("State() = %q, want %q", fresh.State(false), TaskStateCompleted)
	}
}

func TestOneTimeCalendarExpiredSkippedAndCompleted(t *testing.T) {
	h := newSchedHarness(t) // created at 10:00:30
	runAt := time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC)
	task := h.mustCreate(t, Task{
		Name:    "once-expired",
		Enabled: true,
		Schedule: Schedule{
			Kind:     ScheduleKindCalendar,
			Timezone: "UTC",
			RunAt:    &runAt,
		},
		Action:    Action{Kind: ActionKindAgent, Content: "one shot"},
		Execution: ExecutionOptions{TimeoutSeconds: 600, MisfireGraceSeconds: 300},
	})

	// Core restarts an hour after the single occurrence: outside grace.
	h.clock.Set(time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC))
	h.mustStart(t)

	executions := waitExecutions(t, h.repo, task.TaskID, 1)
	skipped := findExecution(executions, ExecutionStatusSkipped)
	if skipped == nil || skipped.ErrorCode != ErrorCodeMisfireGraceExceeded {
		t.Fatalf("want skipped/%s, got %+v", ErrorCodeMisfireGraceExceeded, executions)
	}
	fresh, _ := h.repo.Get(task.TaskID)
	if !fresh.Completed {
		t.Fatalf("expired one-time task must complete")
	}
	if got := h.exec.agentCallCount(); got != 0 {
		t.Fatalf("agent calls = %d, want 0", got)
	}
}

func TestCountBoundScheduleAndManualRun(t *testing.T) {
	h := newSchedHarness(t) // 2026-07-26 10:00:30 UTC
	runAt := time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC)
	task := h.mustCreate(t, Task{
		Name:    "twice",
		Enabled: true,
		Schedule: Schedule{
			Kind:            ScheduleKindCalendar,
			Timezone:        "UTC",
			RunAt:           &runAt,
			RepeatEveryDays: 1,
			RepeatEnd:       &RepeatEnd{Kind: RepeatEndCount, Count: 2},
		},
		Action:    Action{Kind: ActionKindAgent, Content: "twice only"},
		Execution: ExecutionOptions{TimeoutSeconds: 600, MisfireGraceSeconds: 3600},
	})
	h.mustStart(t)

	h.clock.Set(runAt)
	h.timers.fireUntil(t, h.clock, "first occurrence", func() bool { return h.exec.agentCallCount() == 1 })
	// Let the first worker settle fully so the second occurrence is not
	// skipped by the re-entry guard.
	waitForCond(t, "first occurrence settled", func() bool {
		execs, _ := h.repo.ListExecutions(task.TaskID, MaxExecutionLimit)
		return findExecution(execs, ExecutionStatusSuccess) != nil && !h.sched.IsActive(task.TaskID)
	})

	h.clock.Set(runAt.AddDate(0, 0, 1))
	h.timers.fireUntil(t, h.clock, "second occurrence", func() bool { return h.exec.agentCallCount() == 2 })
	waitForCond(t, "task completed at count", func() bool {
		fresh, err := h.repo.Get(task.TaskID)
		return err == nil && fresh.Completed
	})
	waitForCond(t, "second occurrence inactive", func() bool { return !h.sched.IsActive(task.TaskID) })
	fresh, _ := h.repo.Get(task.TaskID)
	if fresh.ConsumedOccurrences != 2 {
		t.Fatalf("consumed = %d, want 2", fresh.ConsumedOccurrences)
	}

	// Manual runs are allowed on a completed task and never consume.
	if _, err := h.sched.RunManual(fresh, "manual-key-1"); err != nil {
		t.Fatalf("RunManual: %v", err)
	}
	waitForCond(t, "manual execution", func() bool { return h.exec.agentCallCount() == 3 })
	waitForCond(t, "manual execution settled", func() bool {
		execs, _ := h.repo.ListExecutions(task.TaskID, MaxExecutionLimit)
		return findExecution(execs, ExecutionStatusSuccess) != nil && len(execs) == 3
	})
	fresh, _ = h.repo.Get(task.TaskID)
	if fresh.ConsumedOccurrences != 2 {
		t.Fatalf("consumed after manual = %d, want 2 (manual never counts)", fresh.ConsumedOccurrences)
	}
}

func TestStopCancelsQueuedAndRunning(t *testing.T) {
	h := newSchedHarness(t)
	// Loop timers here are <= 60s; worker grace waits are 7200s and
	// must only expire under test control, never via fireUntil.
	h.timers.maxDur = 90 * time.Minute
	h.exec.setBlock(true)
	tasks := make([]Task, 0, 3)
	for _, name := range []string{"stop-a", "stop-b", "stop-c"} {
		tasks = append(tasks, h.mustCreate(t, agentCronTask(name, "* * * * *", 7200)))
	}
	h.mustStart(t)

	h.clock.Set(time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC))
	h.timers.fireUntil(t, h.clock, "two executions running", func() bool { return h.exec.agentCallCount() == 2 })
	// All three executions must be dispatched before the stop.
	waitForCond(t, "third execution dispatched", func() bool {
		for _, task := range tasks {
			if !h.sched.IsActive(task.TaskID) {
				return false
			}
		}
		return true
	})

	h.sched.Stop()
	h.sched.waitWorkers()

	// The two running tasks got a best-effort executor cancel.
	cancels := h.exec.cancelSnapshot()
	if len(cancels) != 2 {
		t.Fatalf("executor cancels = %v, want 2 running tasks", cancels)
	}
	// The two running tasks settle as cancelled (executor cancel). The queued
	// task is left non-terminal: recovery will seal it as interrupted on the
	// next start.
	cancelledTotal, queuedTotal := 0, 0
	for _, task := range tasks {
		executions, err := h.repo.ListExecutions(task.TaskID, MaxExecutionLimit)
		if err != nil {
			t.Fatalf("ListExecutions: %v", err)
		}
		if findExecution(executions, ExecutionStatusCancelled) != nil {
			cancelledTotal++
		}
		if findExecution(executions, ExecutionStatusQueued) != nil {
			queuedTotal++
		}
	}
	if cancelledTotal != 2 || queuedTotal != 1 {
		t.Fatalf("cancelled=%d queued=%d, want 2 cancelled and 1 queued (sealed on recovery)", cancelledTotal, queuedTotal)
	}
}

func TestStartupSealsInterruptedExecutions(t *testing.T) {
	genDir := t.TempDir()
	clock := NewFakeClock(time.Date(2026, 7, 26, 10, 0, 30, 0, time.UTC))
	repo, err := OpenFileRepository(genDir, clock)
	if err != nil {
		t.Fatalf("OpenFileRepository: %v", err)
	}
	task, err := repo.Create(agentCronTask("crashy", "* * * * *", 300))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	queued, created, err := repo.AppendExecution(task.TaskID, Execution{
		Trigger:      TriggerScheduled,
		ScheduledFor: clock.Now(),
		Status:       ExecutionStatusQueued,
	})
	if err != nil || !created {
		t.Fatalf("AppendExecution queued: created=%v err=%v", created, err)
	}
	if _, _, err := repo.AppendExecution(task.TaskID, Execution{
		ExecutionID:  queued.ExecutionID,
		Trigger:      TriggerScheduled,
		ScheduledFor: queued.ScheduledFor,
		Status:       ExecutionStatusRunning,
		StartedAt:    clock.Now(),
	}); err != nil {
		t.Fatalf("AppendExecution running: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	h := newSchedHarnessAt(t, genDir, clock)
	h.mustStart(t)

	executions := waitExecutions(t, h.repo, task.TaskID, 1)
	interrupted := findExecution(executions, ExecutionStatusInterrupted)
	if interrupted == nil {
		t.Fatalf("want interrupted execution after restart, got %+v", executions)
	}
	if interrupted.ErrorCode != "INTERRUPTED" {
		t.Fatalf("interrupted code = %q, want INTERRUPTED", interrupted.ErrorCode)
	}
	if got := h.exec.agentCallCount(); got != 0 {
		t.Fatalf("agent calls = %d, want 0", got)
	}
}

// TestStartupMisfireDoesNotDuplicateSealedQueuedExecution verifies the crash
// window where a queued scheduled execution was written but the occurrence
// was not yet consumed. Recovery seals the queued record and then evaluates
// misfires; the same scheduled_for must not create a second execution.
func TestStartupMisfireDoesNotDuplicateSealedQueuedExecution(t *testing.T) {
	genDir := t.TempDir()
	clock := NewFakeClock(time.Date(2026, 7, 26, 10, 0, 30, 0, time.UTC))
	repo, err := OpenFileRepository(genDir, clock)
	if err != nil {
		t.Fatalf("OpenFileRepository: %v", err)
	}
	task, err := repo.Create(agentCronTask("dedup-misfire", "* * * * *", 300))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	scheduledFor := clock.Now()
	queued, created, err := repo.AppendExecution(task.TaskID, Execution{
		Trigger:      TriggerScheduled,
		ScheduledFor: scheduledFor,
		Status:       ExecutionStatusQueued,
	})
	if err != nil || !created {
		t.Fatalf("AppendExecution queued: created=%v err=%v", created, err)
	}
	// Occurrence is intentionally NOT consumed before the crash.
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	h := newSchedHarnessAt(t, genDir, clock)
	h.mustStart(t)

	executions, err := h.repo.ListExecutions(task.TaskID, MaxExecutionLimit)
	if err != nil {
		t.Fatalf("ListExecutions: %v", err)
	}
	if len(executions) != 1 {
		t.Fatalf("want exactly 1 execution after recovery, got %d: %+v", len(executions), executions)
	}
	if executions[0].ExecutionID != queued.ExecutionID {
		t.Fatalf("execution ID changed: got %s want %s", executions[0].ExecutionID, queued.ExecutionID)
	}
	if executions[0].Status != ExecutionStatusInterrupted {
		t.Fatalf("want interrupted, got %s", executions[0].Status)
	}
	// No side effects: the misfire did not start an Agent run.
	if got := h.exec.agentCallCount(); got != 0 {
		t.Fatalf("agent calls = %d, want 0", got)
	}
}

// TestStartupDedupOneTimeCalendarMarksCompleted verifies that when a
// one-time calendar task has a queued execution sealed by recovery and the
// occurrence is then consumed through the dedup path, the task becomes
// completed instead of remaining enabled with no future occurrence.
func TestStartupDedupOneTimeCalendarMarksCompleted(t *testing.T) {
	genDir := t.TempDir()
	clock := NewFakeClock(time.Date(2026, 7, 26, 10, 0, 30, 0, time.UTC))
	repo, err := OpenFileRepository(genDir, clock)
	if err != nil {
		t.Fatalf("OpenFileRepository: %v", err)
	}
	runAt := clock.Now().Add(time.Minute)
	task, err := repo.Create(Task{
		Name:    "once",
		Enabled: true,
		Schedule: Schedule{
			Kind:     ScheduleKindCalendar,
			Timezone: "UTC",
			RunAt:    &runAt,
		},
		Action:    Action{Kind: ActionKindAgent, Content: "one-time"},
		Execution: ExecutionOptions{MisfireGraceSeconds: 300, TimeoutSeconds: 600},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, created, err := repo.AppendExecution(task.TaskID, Execution{
		Trigger:      TriggerScheduled,
		ScheduledFor: runAt,
		TaskVersion:  task.Version,
		Status:       ExecutionStatusQueued,
	}); err != nil || !created {
		t.Fatalf("AppendExecution: created=%v err=%v", created, err)
	}
	// Occurrence intentionally not consumed.
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	clock.Set(runAt.Add(30 * time.Second))
	h := newSchedHarnessAt(t, genDir, clock)
	h.mustStart(t)

	waitForCond(t, "task completed", func() bool {
		current, err := h.repo.Get(task.TaskID)
		if err != nil {
			t.Logf("Get error: %v", err)
			return false
		}
		execs, _ := h.repo.ListExecutions(task.TaskID, MaxExecutionLimit)
		t.Logf("task state: completed=%v invalid=%q execs=%+v", current.Completed, current.InvalidReason, execs)
		return current.Completed
	})
	if got := h.exec.agentCallCount(); got != 0 {
		t.Fatalf("agent calls = %d, want 0", got)
	}
}

func TestRunManualOnPausedTask(t *testing.T) {
	h := newSchedHarness(t)
	task := h.mustCreate(t, agentCronTask("manual", "* * * * *", 300))
	h.mustStart(t)

	fresh, _ := h.repo.Get(task.TaskID)
	if _, err := h.repo.SetPaused(task.TaskID, fresh.Version); err != nil {
		t.Fatalf("SetPaused: %v", err)
	}
	fresh, _ = h.repo.Get(task.TaskID)
	exec, err := h.sched.RunManual(fresh, "manual-paused-1")
	if err != nil {
		t.Fatalf("RunManual on paused task: %v", err)
	}
	if exec.Trigger != TriggerManual {
		t.Fatalf("trigger = %q, want %q", exec.Trigger, TriggerManual)
	}
	waitForCond(t, "manual execution finished", func() bool {
		execs, _ := h.repo.ListExecutions(task.TaskID, MaxExecutionLimit)
		return findExecution(execs, ExecutionStatusSuccess) != nil
	})
	fresh, _ = h.repo.Get(task.TaskID)
	if fresh.ConsumedOccurrences != 0 {
		t.Fatalf("consumed = %d, want 0 (manual never counts)", fresh.ConsumedOccurrences)
	}
}

func TestSchedulerPublishesEvents(t *testing.T) {
	h := newSchedHarness(t)
	task := h.mustCreate(t, reminderCronTask("remind", "* * * * *", 300))
	h.mustStart(t)

	h.clock.Set(time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC))
	h.timers.fireUntil(t, h.clock, "reminder executed", func() bool { return h.exec.reminderCallCount() == 1 })
	waitForCond(t, "notice event", func() bool { return h.events.count(EventNotice) == 1 })

	var notice *Event
	for _, event := range h.events.snapshot() {
		if event.Type == EventNotice {
			copy := event
			notice = &copy
		}
	}
	if notice == nil {
		t.Fatalf("no notice event published")
	}
	if notice.TaskID != task.TaskID || notice.Content != "stand up and stretch" {
		t.Fatalf("notice event = %+v", notice)
	}
	if h.events.count(EventExecutionStarted) != 1 || h.events.count(EventExecutionFinished) != 1 {
		t.Fatalf("events = %+v", h.events.snapshot())
	}
}

func TestSchedulerPublishesThreadIdentityBeforeAgentRunFinishes(t *testing.T) {
	h := newSchedHarness(t)
	h.exec.setBlock(true)
	taskDraft := agentCronTask("sidebar-visible", "* * * * *", 300)
	taskDraft.ProjectID = "project-visible"
	task := h.mustCreate(t, taskDraft)
	h.mustStart(t)

	h.clock.Set(time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC))
	h.timers.fireUntil(t, h.clock, "agent entered executor", func() bool {
		return h.exec.agentCallCount() == 1
	})
	waitForCond(t, "started event with thread identity", func() bool {
		for _, event := range h.events.snapshot() {
			if event.Type == EventExecutionStarted && event.TaskID == task.TaskID {
				return event.ProjectID == task.ProjectID && event.RunID != "" && event.ThreadID == "thread-"+task.TaskID
			}
		}
		return false
	})

	fresh, err := h.repo.Get(task.TaskID)
	if err != nil {
		t.Fatalf("Get task: %v", err)
	}
	if fresh.ThreadID != "thread-"+task.TaskID {
		t.Fatalf("thread binding before finish = %q", fresh.ThreadID)
	}
	if h.events.count(EventExecutionFinished) != 0 {
		t.Fatalf("execution finished before the blocked agent was released")
	}

	h.exec.release()
	waitForCond(t, "agent execution finished", func() bool {
		return h.events.count(EventExecutionFinished) == 1
	})
}

func TestRunManualIdempotencyKey(t *testing.T) {
	h := newSchedHarness(t)
	task := h.mustCreate(t, agentCronTask("idem", "* * * * *", 300))
	h.mustStart(t)

	first, err := h.sched.RunManual(task, "same-key")
	if err != nil {
		t.Fatalf("first RunManual: %v", err)
	}
	second, err := h.sched.RunManual(task, "same-key")
	if err != nil {
		t.Fatalf("second RunManual: %v", err)
	}
	if first.ExecutionID == "" || first.ExecutionID != second.ExecutionID {
		t.Fatalf("idempotent retry returned %q then %q", first.ExecutionID, second.ExecutionID)
	}
	waitForCond(t, "manual execution finished", func() bool {
		execs, _ := h.repo.ListExecutions(task.TaskID, MaxExecutionLimit)
		return findExecution(execs, ExecutionStatusSuccess) != nil
	})
	if got := h.exec.agentCallCount(); got != 1 {
		t.Fatalf("agent calls = %d, want 1", got)
	}
}

func TestCancelExecutionQueued(t *testing.T) {
	h := newSchedHarness(t)
	// Loop timers here are <= 60s; worker grace waits are 7200s and
	// must only expire under test control, never via fireUntil.
	h.timers.maxDur = 90 * time.Minute
	h.exec.setBlock(true)
	tasks := make([]Task, 0, 3)
	for _, name := range []string{"holder-a", "holder-b", "waiter"} {
		tasks = append(tasks, h.mustCreate(t, agentCronTask(name, "* * * * *", 7200)))
	}
	h.mustStart(t)

	h.clock.Set(time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC))
	h.timers.fireUntil(t, h.clock, "semaphore full", func() bool { return h.exec.agentCallCount() == 2 })

	// Which task ended up queued behind the semaphore is nondeterministic;
	// find it as the one whose executor call never started.
	entered := map[string]bool{<-h.exec.enteredCh: true, <-h.exec.enteredCh: true}
	var waiter Task
	found := false
	for _, task := range tasks {
		if !entered[task.TaskID] {
			waiter, found = task, true
		}
	}
	if !found {
		t.Fatalf("no queued task behind the semaphore")
	}
	waitForCond(t, "waiter queued behind the semaphore", func() bool { return h.sched.IsActive(waiter.TaskID) })

	if !h.sched.CancelExecution(waiter.TaskID) {
		t.Fatalf("CancelExecution reported nothing to cancel")
	}
	waitForCond(t, "queued execution cancelled", func() bool {
		execs, _ := h.repo.ListExecutions(waiter.TaskID, MaxExecutionLimit)
		return findExecution(execs, ExecutionStatusCancelled) != nil
	})
	h.exec.release()
	h.sched.waitWorkers()
	if got := h.exec.agentCallCount(); got != 2 {
		t.Fatalf("agent calls = %d, want 2 (cancelled queued execution never ran)", got)
	}
}

// TestCancelExecutionAfterRunningRecord verifies that cancelling after the
// running record is written still produces a terminal cancelled record rather
// than leaving a stale running execution for recovery.
func TestCancelExecutionAfterRunningRecord(t *testing.T) {
	h := newSchedHarness(t)
	h.exec.setBlock(true)
	task := h.mustCreate(t, agentCronTask("cancel-after-running", "* * * * *", 300))
	h.mustStart(t)

	h.clock.Set(time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC))
	h.timers.fireUntil(t, h.clock, "execution running", func() bool {
		execs, err := h.repo.ListExecutions(task.TaskID, MaxExecutionLimit)
		if err != nil {
			return false
		}
		return findExecution(execs, ExecutionStatusRunning) != nil
	})

	if !h.sched.CancelExecution(task.TaskID) {
		t.Fatalf("CancelExecution reported nothing to cancel")
	}
	waitForCond(t, "execution cancelled", func() bool {
		execs, _ := h.repo.ListExecutions(task.TaskID, MaxExecutionLimit)
		return findExecution(execs, ExecutionStatusCancelled) != nil
	})

	h.exec.release()
	h.sched.waitWorkers()
	execs, _ := h.repo.ListExecutions(task.TaskID, MaxExecutionLimit)
	if len(execs) != 1 {
		t.Fatalf("want 1 execution, got %d: %+v", len(execs), execs)
	}
	if execs[0].Status != ExecutionStatusCancelled {
		t.Fatalf("want cancelled, got %s", execs[0].Status)
	}
}

func TestRunManualBusyReturnsError(t *testing.T) {
	h := newSchedHarness(t)
	h.exec.setBlock(true)
	task := h.mustCreate(t, agentCronTask("busy", "* * * * *", 300))
	h.mustStart(t)

	h.clock.Set(time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC))
	h.timers.fireUntil(t, h.clock, "execution running", func() bool { return h.exec.agentCallCount() == 1 })

	fresh, _ := h.repo.Get(task.TaskID)
	if _, err := h.sched.RunManual(fresh, "busy-key"); !errors.Is(err, ErrTaskAlreadyRunning) {
		t.Fatalf("RunManual on busy task = %v, want %v", err, ErrTaskAlreadyRunning)
	}
	h.exec.release()
	h.sched.waitWorkers()
}
