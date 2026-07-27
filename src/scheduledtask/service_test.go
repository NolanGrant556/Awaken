package scheduledtask

import (
	"errors"
	"testing"
)

type serviceHarness struct {
	*schedHarness
	service *Service
}

func newServiceHarness(t *testing.T) *serviceHarness {
	t.Helper()
	base := newSchedHarness(t)
	service := NewService(base.repo, Validator{Clock: base.clock}, base.exec)
	service.SetEventListener(base.events.emit)
	base.sched = NewScheduler(SchedulerConfig{
		Repo:      base.repo,
		Clock:     base.clock,
		Executor:  base.exec,
		Validator: Validator{Clock: base.clock},
		NewTimer:  base.timers.newTimer,
		Emit:      service.PublishEvent,
	})
	service.SetScheduler(base.sched)
	if err := base.sched.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return &serviceHarness{schedHarness: base, service: service}
}

func TestServiceCreateValidatesAndPublishes(t *testing.T) {
	h := newServiceHarness(t)

	invalid := agentCronTask("", "* * * * *", 300)
	if _, err := h.service.Create(invalid); !errors.Is(err, ErrValidationFailed) {
		t.Fatalf("Create invalid = %v, want %v", err, ErrValidationFailed)
	}

	created, err := h.service.Create(agentCronTask("daily report", "* * * * *", 300))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.TaskID == "" || created.Version != 1 {
		t.Fatalf("created task = %+v", created)
	}
	waitForCond(t, "task_changed event", func() bool { return h.events.count(EventTaskChanged) == 1 })
}

func TestServicePatchRenameSyncsThreadTitle(t *testing.T) {
	h := newServiceHarness(t)
	created, err := h.service.Create(agentCronTask("old name", "* * * * *", 300))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := h.repo.BindThread(created.TaskID, created.Version, "thread-1"); err != nil {
		t.Fatalf("BindThread: %v", err)
	}
	fresh, _ := h.repo.Get(created.TaskID)

	// A rename follows onto the dedicated thread.
	updated, err := h.service.Patch(created.TaskID, fresh.Version, func(task Task) (Task, error) {
		task.Name = "new name"
		return task, nil
	})
	if err != nil {
		t.Fatalf("Patch rename: %v", err)
	}
	syncs := h.exec.titleSyncSnapshot()
	if len(syncs) != 1 || syncs[0].threadID != "thread-1" || syncs[0].taskName != "new name" {
		t.Fatalf("title syncs = %+v, want one for thread-1/new name", syncs)
	}

	// An edit without a rename never touches the thread title.
	if _, err := h.service.Patch(created.TaskID, updated.Version, func(task Task) (Task, error) {
		task.Action.Content = "a different prompt"
		return task, nil
	}); err != nil {
		t.Fatalf("Patch content: %v", err)
	}
	if got := len(h.exec.titleSyncSnapshot()); got != 1 {
		t.Fatalf("title syncs after content edit = %d, want 1", got)
	}
}

func TestServicePatchVersionConflict(t *testing.T) {
	h := newServiceHarness(t)
	created, err := h.service.Create(agentCronTask("conflict", "* * * * *", 300))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := h.service.Patch(created.TaskID, created.Version+9, func(task Task) (Task, error) {
		task.Name = "other"
		return task, nil
	}); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("Patch stale version = %v, want %v", err, ErrVersionConflict)
	}
}

func TestServicePatchClearsInvalidReason(t *testing.T) {
	h := newServiceHarness(t)
	created, err := h.service.Create(agentCronTask("fixme", "* * * * *", 300))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	marked, err := h.repo.Update(created.TaskID, created.Version, func(task Task) (Task, error) {
		task.InvalidReason = "scheduledtask: invalid schedule: bad cron"
		return task, nil
	})
	if err != nil {
		t.Fatalf("mark invalid: %v", err)
	}
	updated, err := h.service.Patch(created.TaskID, marked.Version, func(task Task) (Task, error) {
		task.Schedule.Cron = "0 9 * * *"
		return task, nil
	})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if updated.InvalidReason != "" {
		t.Fatalf("successful edit must clear invalid_reason, got %q", updated.InvalidReason)
	}
}

func TestServiceDelete(t *testing.T) {
	h := newServiceHarness(t)
	created, err := h.service.Create(agentCronTask("disposable", "* * * * *", 300))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := h.service.Delete(created.TaskID, created.Version, "svc-del-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := h.repo.Get(created.TaskID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("Get after delete = %v, want %v", err, ErrTaskNotFound)
	}
	var deleted *Event
	for _, event := range h.events.snapshot() {
		if event.Type == EventTaskChanged && event.Status == "deleted" {
			copy := event
			deleted = &copy
		}
	}
	if deleted == nil || deleted.TaskID != created.TaskID {
		t.Fatalf("no deleted event for %s: %+v", created.TaskID, h.events.snapshot())
	}
}

func TestServiceDeleteActiveRejected(t *testing.T) {
	h := newServiceHarness(t)
	h.exec.setBlock(true)
	created, err := h.service.Create(agentCronTask("busy-delete", "* * * * *", 300))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := h.service.RunNow(created.TaskID, "delete-key"); err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	waitForCond(t, "execution running", func() bool { return h.exec.agentCallCount() == 1 })

	if err := h.service.Delete(created.TaskID, created.Version, "svc-del-active"); !errors.Is(err, ErrTaskAlreadyRunning) {
		t.Fatalf("Delete active = %v, want %v", err, ErrTaskAlreadyRunning)
	}
	h.exec.release()
	waitForCond(t, "execution settled", func() bool {
		execs, _ := h.repo.ListExecutions(created.TaskID, MaxExecutionLimit)
		return findExecution(execs, ExecutionStatusSuccess) != nil
	})
	waitForCond(t, "execution inactive", func() bool { return !h.service.IsRunning(created.TaskID) })
	// Version may have advanced due to pause/resume or thread binding; fetch
	// the current version before deleting.
	current, err := h.repo.Get(created.TaskID)
	if err != nil {
		t.Fatalf("Get after settle: %v", err)
	}
	if err := h.service.Delete(created.TaskID, current.Version, "svc-del-settle"); err != nil {
		t.Fatalf("Delete after settle: %v", err)
	}
}

func TestServiceRunNowIdempotencyAndPause(t *testing.T) {
	h := newServiceHarness(t)
	created, err := h.service.Create(agentCronTask("manual", "* * * * *", 300))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := h.service.Pause(created.TaskID, created.Version); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	// Manual runs work on paused tasks and retry-safe by idempotency key.
	first, err := h.service.RunNow(created.TaskID, "key-1")
	if err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	waitForCond(t, "manual success", func() bool {
		execs, _ := h.repo.ListExecutions(created.TaskID, MaxExecutionLimit)
		return findExecution(execs, ExecutionStatusSuccess) != nil
	})
	waitForCond(t, "execution inactive", func() bool { return !h.service.IsRunning(created.TaskID) })
	second, err := h.service.RunNow(created.TaskID, "key-1")
	if err != nil {
		t.Fatalf("RunNow retry: %v", err)
	}
	if first.ExecutionID == "" || first.ExecutionID != second.ExecutionID {
		t.Fatalf("idempotent retry = %q then %q", first.ExecutionID, second.ExecutionID)
	}
	if got := h.exec.agentCallCount(); got != 1 {
		t.Fatalf("agent calls = %d, want 1", got)
	}
	fresh, _ := h.repo.Get(created.TaskID)
	if fresh.ConsumedOccurrences != 0 {
		t.Fatalf("consumed = %d, want 0 (manual never counts)", fresh.ConsumedOccurrences)
	}
}

func TestServiceRunNowInvalidTask(t *testing.T) {
	h := newServiceHarness(t)
	created, err := h.service.Create(agentCronTask("invalid-now", "* * * * *", 300))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := h.repo.Update(created.TaskID, created.Version, func(task Task) (Task, error) {
		task.InvalidReason = "scheduledtask: invalid schedule: bad cron"
		return task, nil
	}); err != nil {
		t.Fatalf("mark invalid: %v", err)
	}
	if _, err := h.service.RunNow(created.TaskID, "key"); !errors.Is(err, ErrValidationFailed) {
		t.Fatalf("RunNow invalid = %v, want %v", err, ErrValidationFailed)
	}
}

func TestServiceCancelExecution(t *testing.T) {
	h := newServiceHarness(t)
	h.exec.setBlock(true)
	created, err := h.service.Create(agentCronTask("cancellable", "* * * * *", 300))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := h.service.CancelExecution(created.TaskID); !errors.Is(err, ErrExecutionNotFound) {
		t.Fatalf("CancelExecution idle = %v, want %v", err, ErrExecutionNotFound)
	}

	if _, err := h.service.RunNow(created.TaskID, "cancel-key"); err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	waitForCond(t, "execution running", func() bool { return h.exec.agentCallCount() == 1 })
	if err := h.service.CancelExecution(created.TaskID); err != nil {
		t.Fatalf("CancelExecution: %v", err)
	}
	cancels := h.exec.cancelSnapshot()
	if len(cancels) != 1 || cancels[0] != created.TaskID {
		t.Fatalf("executor cancels = %v, want [%s]", cancels, created.TaskID)
	}
	// The cancelled executor call settles the record as cancelled.
	h.exec.release()
	waitForCond(t, "execution cancelled", func() bool {
		execs, _ := h.repo.ListExecutions(created.TaskID, MaxExecutionLimit)
		return findExecution(execs, ExecutionStatusCancelled) != nil
	})
}

func TestServicePauseResumePublishes(t *testing.T) {
	h := newServiceHarness(t)
	created, err := h.service.Create(agentCronTask("toggle", "* * * * *", 300))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	paused, err := h.service.Pause(created.TaskID, created.Version)
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if paused.Enabled {
		t.Fatalf("task still enabled after Pause")
	}
	if _, err := h.service.Resume(created.TaskID, paused.Version); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	// create + pause + resume
	if got := h.events.count(EventTaskChanged); got != 3 {
		t.Fatalf("task_changed events = %d, want 3", got)
	}
}

func TestServiceRunNowUsesLiveClockOccurrence(t *testing.T) {
	h := newServiceHarness(t)
	created, err := h.service.Create(reminderCronTask("remind-now", "* * * * *", 300))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	exec, err := h.service.RunNow(created.TaskID, "clock-key")
	if err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	if !exec.ScheduledFor.Equal(h.clock.Now()) {
		t.Fatalf("manual scheduled_for = %v, want clock now %v", exec.ScheduledFor, h.clock.Now())
	}
	waitForCond(t, "reminder done", func() bool { return h.events.count(EventNotice) == 1 })
}
