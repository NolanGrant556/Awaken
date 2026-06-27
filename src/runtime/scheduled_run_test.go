package runtime

import (
	"context"
	"sync"
	"testing"
	"time"

	"awaken/src/agentcore"
)

func TestRunServiceScheduledRunsCreateListCancel(t *testing.T) {
	now := time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC)
	clock := newFakeScheduleClock(now)
	executorCalled := make(chan struct{})
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			close(executorCalled)
			return "should not run", nil
		})
	})
	service.scheduleClock = clock

	root := t.TempDir()
	runAt := now.Add(time.Hour)
	scheduled, err := service.CreateScheduledRun(CreateScheduledRunRequest{
		Input:     "  remind me  ",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
		RunAt:     runAt,
	})
	if err != nil {
		t.Fatalf("CreateScheduledRun returned error: %v", err)
	}
	if scheduled.ID == "" {
		t.Fatalf("expected scheduled run id")
	}
	if scheduled.Input != "remind me" {
		t.Fatalf("expected trimmed input, got %q", scheduled.Input)
	}
	if scheduled.Workspace.WorkspaceRoot != root {
		t.Fatalf("expected workspace root %q, got %q", root, scheduled.Workspace.WorkspaceRoot)
	}
	if scheduled.RunAt != runAt {
		t.Fatalf("expected run_at %s, got %s", runAt, scheduled.RunAt)
	}
	if scheduled.Status != ScheduledRunStatusScheduled {
		t.Fatalf("expected scheduled status, got %s", scheduled.Status)
	}
	if scheduled.CreatedAt != now || scheduled.UpdatedAt != now {
		t.Fatalf("expected created/updated at fake now, got %+v", scheduled)
	}

	listed := service.ListScheduledRuns()
	if len(listed) != 1 {
		t.Fatalf("expected one scheduled run, got %d", len(listed))
	}
	if listed[0].ID != scheduled.ID {
		t.Fatalf("expected list id %q, got %q", scheduled.ID, listed[0].ID)
	}

	clock.Advance(30 * time.Minute)
	cancelled, err := service.CancelScheduledRun(scheduled.ID)
	if err != nil {
		t.Fatalf("CancelScheduledRun returned error: %v", err)
	}
	if cancelled.Status != ScheduledRunStatusCancelled {
		t.Fatalf("expected cancelled status, got %s", cancelled.Status)
	}
	if cancelled.UpdatedAt != now.Add(30*time.Minute) {
		t.Fatalf("expected updated_at to move on cancel, got %s", cancelled.UpdatedAt)
	}

	clock.Advance(2 * time.Hour)
	select {
	case <-executorCalled:
		t.Fatalf("cancelled scheduled run should not trigger executor")
	default:
	}
}

func TestRunServiceScheduledRunDueTriggerStartsRun(t *testing.T) {
	now := time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC)
	clock := newFakeScheduleClock(now)
	started := make(chan string, 1)
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			started <- input
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			return "done", nil
		})
	})
	service.scheduleClock = clock

	root := t.TempDir()
	scheduled, err := service.CreateScheduledRun(CreateScheduledRunRequest{
		Input:     "run later",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
		RunAt:     now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateScheduledRun returned error: %v", err)
	}

	clock.Advance(time.Hour)

	listed := service.ListScheduledRuns()
	if len(listed) != 1 {
		t.Fatalf("expected one scheduled run, got %d", len(listed))
	}
	triggered := listed[0]
	if triggered.ID != scheduled.ID {
		t.Fatalf("expected scheduled id %q, got %q", scheduled.ID, triggered.ID)
	}
	if triggered.Status != ScheduledRunStatusTriggered {
		t.Fatalf("expected triggered status, got %s", triggered.Status)
	}
	if triggered.RunID == "" {
		t.Fatalf("expected triggered run_id")
	}
	if triggered.UpdatedAt != now.Add(time.Hour) {
		t.Fatalf("expected updated_at at trigger time, got %s", triggered.UpdatedAt)
	}

	run, err := service.GetRun(triggered.RunID)
	if err != nil {
		t.Fatalf("GetRun for triggered run returned error: %v", err)
	}
	if run.Input != "run later" {
		t.Fatalf("expected ordinary run input to match scheduled input, got %q", run.Input)
	}

	select {
	case input := <-started:
		if input != "run later" {
			t.Fatalf("expected executor input %q, got %q", "run later", input)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for scheduled run executor")
	}
}

type fakeScheduleClock struct {
	mu     sync.Mutex
	now    time.Time
	nextID int
	timers map[int]*fakeScheduledTimer
}

type fakeScheduledTimer struct {
	clock   *fakeScheduleClock
	id      int
	due     time.Time
	stopped bool
	fn      func()
}

func newFakeScheduleClock(now time.Time) *fakeScheduleClock {
	return &fakeScheduleClock{
		now:    now,
		timers: make(map[int]*fakeScheduledTimer),
	}
}

func (c *fakeScheduleClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeScheduleClock) AfterFunc(d time.Duration, fn func()) scheduledTimer {
	if d < 0 {
		d = 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	timer := &fakeScheduledTimer{
		clock: c,
		id:    c.nextID,
		due:   c.now.Add(d),
		fn:    fn,
	}
	c.timers[timer.id] = timer
	return timer
}

func (c *fakeScheduleClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	for {
		callbacks := c.popDueLocked()
		if len(callbacks) == 0 {
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
		for _, callback := range callbacks {
			callback()
		}
		c.mu.Lock()
	}
}

func (c *fakeScheduleClock) popDueLocked() []func() {
	callbacks := make([]func(), 0)
	for id, timer := range c.timers {
		if timer.stopped || timer.due.After(c.now) {
			continue
		}
		timer.stopped = true
		delete(c.timers, id)
		callbacks = append(callbacks, timer.fn)
	}
	return callbacks
}

func (t *fakeScheduledTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	if t.stopped {
		return false
	}
	t.stopped = true
	delete(t.clock.timers, t.id)
	return true
}
