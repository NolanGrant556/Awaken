package subagentruntime

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestConcurrentSharedSpawn(t *testing.T) {
	reached := make(chan struct{}, 2)
	release := make(chan struct{})
	var m *Manager
	m = NewManager(context.Background(), Config{Starter: func(ctx context.Context, _ StartRequest) (string, error) { <-ctx.Done(); return "", ctx.Err() }, Persist: func(_ context.Context, e PersistEvent) error {
		if e.Kind == "subagent_spawned" {
			reached <- struct{}{}
			<-release
		}
		return nil
	}})
	defer m.Close(context.Background())
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, e := m.Spawn(context.Background(), SpawnRequest{Description: "audit", Prompt: "audit", AgentType: AgentTypeGeneral, WorkspaceMode: WorkspaceShared})
			errs <- e
		}()
	}
	<-reached
	<-reached
	close(release)
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("shared workspace must allow independent file work: %v", err)
		}
	}

}
func TestWorktreeSnapshotRace(t *testing.T) {
	started := make(chan StartRequest, 1)
	release := make(chan struct{})
	m := NewManager(context.Background(), Config{Starter: func(ctx context.Context, r StartRequest) (string, error) { started <- r; <-release; return "", nil }})
	_, err := m.Spawn(context.Background(), SpawnRequest{Description: "audit", Prompt: "audit", AgentType: AgentTypeGeneral, WorkspaceMode: WorkspaceGitWorktree})
	if err != nil {
		t.Fatal(err)
	}
	r := <-started
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			r.AttachWorktree(&WorktreeReceipt{})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			m.List()
		}
	}()
	wg.Wait()
	close(release)
	m.Close(context.Background())
}
func TestMessageIdempotencyIncludesTask(t *testing.T) {
	m := NewManager(context.Background(), Config{Starter: func(ctx context.Context, _ StartRequest) (string, error) { <-ctx.Done(); return "", ctx.Err() }})
	defer m.Close(context.Background())
	req := SpawnRequest{Description: "audit", Prompt: "audit", AgentType: AgentTypeExplore, WorkspaceMode: WorkspaceSharedReadOnly}
	a, _ := m.Spawn(context.Background(), req)
	b, _ := m.Spawn(context.Background(), req)
	first, _ := m.MessageIdempotent(context.Background(), a.TaskID, "hello", "retry-key")
	second, err := m.MessageIdempotent(context.Background(), b.TaskID, "hello", "retry-key")
	if err == nil && second.TaskID == first.TaskID {
		t.Fatalf("message to B reported successful with A's receipt: %+v", second)
	}
}
func TestMessageConcurrentRetry(t *testing.T) {
	reached := make(chan struct{}, 1)
	release := make(chan struct{})
	var once sync.Once
	m := NewManager(context.Background(), Config{Starter: func(ctx context.Context, _ StartRequest) (string, error) { <-ctx.Done(); return "", ctx.Err() }, Persist: func(_ context.Context, e PersistEvent) error {
		if e.Kind == "subagent_message_queued" {
			once.Do(func() { reached <- struct{}{}; <-release })
		}
		return nil
	}})
	defer m.Close(context.Background())
	a, _ := m.Spawn(context.Background(), SpawnRequest{Description: "audit", Prompt: "audit", AgentType: AgentTypeExplore})
	receipts := make(chan MessageReceipt, 2)
	go func() { r, _ := m.MessageIdempotent(context.Background(), a.TaskID, "hello", "same"); receipts <- r }()
	<-reached
	go func() { r, _ := m.MessageIdempotent(context.Background(), a.TaskID, "hello", "same"); receipts <- r }()
	time.Sleep(30 * time.Millisecond)
	close(release)
	r1, r2 := <-receipts, <-receipts
	if r1.MessageID != r2.MessageID {
		t.Fatalf("one idempotency key queued twice: %s != %s", r1.MessageID, r2.MessageID)
	}
}

func TestReturnedWorktreeSnapshotDoesNotMutateManager(t *testing.T) {
	m := NewManager(context.Background(), Config{Starter: func(ctx context.Context, _ StartRequest) (string, error) { <-ctx.Done(); return "", ctx.Err() }})
	defer m.Close(context.Background())
	task, err := m.Spawn(context.Background(), SpawnRequest{Description: "snapshot", Prompt: "snapshot", AgentType: AgentTypeExplore})
	if err != nil {
		t.Fatal(err)
	}
	m.attachWorktree(task.TaskID, &WorktreeReceipt{Path: "original", Changed: []string{"original"}})
	snapshot := m.List()[0]
	snapshot.Worktree.Path = "changed"
	snapshot.Worktree.Changed[0] = "changed"
	current, err := m.Status(task.TaskID)
	if err != nil || current.Worktree.Path != "original" || current.Worktree.Changed[0] != "original" {
		t.Fatalf("snapshot leaked: %+v %v", current, err)
	}
}

func TestSpawnAfterCloseRejected(t *testing.T) {
	m := NewManager(context.Background(), Config{Starter: func(context.Context, StartRequest) (string, error) { return "", nil }})
	if err := m.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Spawn(context.Background(), SpawnRequest{Description: "late", Prompt: "late", AgentType: AgentTypeExplore}); err == nil {
		t.Fatal("closed manager accepted work")
	}
}
