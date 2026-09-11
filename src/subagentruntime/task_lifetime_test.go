package subagentruntime

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTerminalWaitsForResourceCleanup(t *testing.T) {
	cleaning, release := make(chan struct{}), make(chan struct{})
	var cleaningOnce sync.Once
	m := NewManager(context.Background(), Config{
		Starter: func(context.Context, StartRequest) (string, error) { return "done", nil },
		Retain:  func(TaskSnapshot) func() { return func() { cleaningOnce.Do(func() { close(cleaning) }); <-release } },
	})
	task, err := m.Spawn(context.Background(), SpawnRequest{Description: "cleanup", Prompt: "cleanup"})
	if err != nil {
		t.Fatal(err)
	}
	<-cleaning
	if got, _ := m.Status(task.TaskID); got.Terminal() {
		t.Fatal("terminal published before cleanup")
	}
	if _, err := m.Cancel(context.Background(), task.TaskID, "user"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := m.EndAttempt(context.Background(), "root", ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Status(task.TaskID); got.Status != StatusCancelled {
		t.Fatal(got.Status)
	}
	if m.HasLiveWork("") {
		t.Fatal("cleanup barrier returned with live work")
	}
}

func TestStopCancelsDescendantsAndQueueAndFencesFollowup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	descendantStarted := make(chan struct{})
	parentStarted := make(chan TaskSnapshot, 1)
	var queuedStarts atomic.Int32
	var m *Manager
	m = NewManager(context.Background(), Config{RootContext: ctx, RunID: "run", BindRunner: true, Policy: Policy{MaxActiveAgents: 2}, Starter: func(childCtx context.Context, req StartRequest) (string, error) {
		branch := m.Bind(childCtx, req.Task.AgentID, 1, Config{RunID: "run", Starter: func(grandCtx context.Context, _ StartRequest) (string, error) {
			close(descendantStarted)
			<-grandCtx.Done()
			return "", grandCtx.Err()
		}})
		grandchild, err := branch.Spawn(childCtx, SpawnRequest{Description: "grandchild", Prompt: "work", Background: true})
		if err != nil {
			return "", err
		}
		parentStarted <- grandchild
		<-childCtx.Done()
		return "", childCtx.Err()
	}})
	defer m.Close(context.Background())
	first, err := m.Spawn(ctx, SpawnRequest{Description: "parent", Prompt: "delegate"})
	if err != nil {
		t.Fatal(err)
	}
	grandchild := <-parentStarted
	<-descendantStarted
	// Both slots are occupied, so this child must remain queued.
	m.mu.Lock()
	owner := m.owners["root"]
	owner.config.Starter = func(context.Context, StartRequest) (string, error) { queuedStarts.Add(1); return "bad", nil }
	m.owners["root"] = owner
	m.mu.Unlock()
	queued, err := m.Spawn(ctx, SpawnRequest{Description: "queued", Prompt: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.StopAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Followup(context.Background(), first.TaskID, "late"); err == nil {
		t.Fatal("followup removed stop fence")
	}
	if err := m.EndAttempt(context.Background(), "root", "run"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{first.TaskID, grandchild.TaskID, queued.TaskID} {
		if got, _ := m.Status(id); got.Status != StatusCancelled {
			t.Fatalf("%s: %s", id, got.Status)
		}
	}
	if queuedStarts.Load() != 0 {
		t.Fatal("queued work started after stop")
	}
}

func TestStopDuringSpawnPersistenceWaitsForReservation(t *testing.T) {
	persisting, allowPersist := make(chan struct{}), make(chan struct{})
	var starts atomic.Int32
	m := NewManager(context.Background(), Config{RunID: "run", Starter: func(context.Context, StartRequest) (string, error) { starts.Add(1); return "bad", nil }, Persist: func(_ context.Context, event PersistEvent) error {
		if event.Kind == "subagent_spawned" {
			close(persisting)
			<-allowPersist
		}
		return nil
	}})
	defer m.Close(context.Background())
	spawned := make(chan error, 1)
	go func() {
		_, err := m.Spawn(context.Background(), SpawnRequest{Description: "race", Prompt: "race"})
		spawned <- err
	}()
	<-persisting
	drained := make(chan error, 1)
	go func() { drained <- m.EndAttempt(context.Background(), "root", "run") }()
	select {
	case err := <-drained:
		t.Fatalf("finished with uncommitted spawn: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(allowPersist)
	if err := <-spawned; err != nil {
		t.Fatal(err)
	}
	if err := <-drained; err != nil {
		t.Fatal(err)
	}
	if starts.Load() != 0 || m.HasLiveWork("") {
		t.Fatal("late spawn escaped parent lifetime")
	}
}

func TestLateStopAndCleanupDoNotAffectNewRoot(t *testing.T) {
	cfg := Config{RunID: "old", BindRunner: true, Starter: func(context.Context, StartRequest) (string, error) { return "done", nil }}
	m := NewManager(context.Background(), cfg)
	defer m.Close(context.Background())
	if err := m.EndAttempt(context.Background(), "root", "old"); err != nil {
		t.Fatal(err)
	}
	cfg.RunID = "new"
	started := make(chan struct{})
	cfg.Starter = func(ctx context.Context, _ StartRequest) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	}
	controller := m.Bind(context.Background(), "root", 0, cfg)
	task, err := controller.Spawn(context.Background(), SpawnRequest{Description: "new", Prompt: "new"})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if err := m.StopRun(context.Background(), "old"); err != nil {
		t.Fatal(err)
	}
	if err := m.EndAttempt(context.Background(), "root", "old"); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Status(task.TaskID); got.Status != StatusRunning {
		t.Fatalf("old operation affected new run: %s", got.Status)
	}
}

func TestCancelActiveAssignmentsKeepsCreatorAndNotifiesParent(t *testing.T) {
	started := make(chan struct{})
	var startOnce sync.Once
	m := NewManager(context.Background(), Config{
		BindRunner: true,
		Starter: func(ctx context.Context, _ StartRequest) (string, error) {
			startOnce.Do(func() { close(started) })
			<-ctx.Done()
			return "", ctx.Err()
		},
	})
	defer m.Close(context.Background())
	first, err := m.Spawn(context.Background(), SpawnRequest{Description: "child", Prompt: "work"})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if err := m.CancelActiveAssignments(context.Background(), "user_stopped_all"); err != nil {
		t.Fatal(err)
	}
	notices := m.DrainParentNotices()
	if len(notices) != 1 || !strings.Contains(notices[0].Content, "the user stopped all active Subagent assignments") {
		t.Fatalf("parent notice = %+v", notices)
	}
	if !strings.Contains(notices[0].Content, first.TaskID) {
		t.Fatalf("parent notice omitted task id: %+v", notices[0])
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, _ := m.Status(first.TaskID)
		if got.Status == StatusCancelled {
			if got.ErrorMessage != "the user stopped this Subagent assignment" {
				t.Fatalf("cancel message = %q", got.ErrorMessage)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("child status = %s", got.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if m.stopped {
		t.Fatal("user stop-all must not fence the manager")
	}
	if _, err := m.Followup(context.Background(), first.TaskID, "continue after user stop"); err != nil {
		t.Fatalf("parent must still be able to continue: %v", err)
	}
}
