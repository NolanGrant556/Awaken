package subagentruntime

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestManagerSpawnWaitAndDurableOrdering(t *testing.T) {
	var mu sync.Mutex
	var events []string
	manager := NewManager(context.Background(), Config{
		MaxConcurrent: 1,
		Starter: func(context.Context, StartRequest) (string, error) {
			mu.Lock()
			events = append(events, "starter")
			mu.Unlock()
			return "done", nil
		},
		Persist: func(_ context.Context, event PersistEvent) error {
			mu.Lock()
			events = append(events, event.Kind+":"+string(event.Snapshot.Status))
			mu.Unlock()
			return nil
		},
	})
	task, err := manager.Spawn(context.Background(), SpawnRequest{
		Description: "inspect auth", Prompt: "read the auth flow",
		AgentType: "explore", WorkspaceMode: "shared_readonly",
		ParentAgentID: "root", ThreadID: "thread", RunID: "run",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	result, err := manager.Wait(context.Background(), WaitRequest{
		TaskIDs: []string{task.TaskID}, ReturnWhen: "all", Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if len(result) != 1 || result[0].Status != StatusCompleted || result[0].ResultPreview != "done" {
		t.Fatalf("unexpected result: %+v", result)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{
		"subagent_spawned:queued",
		"subagent_status_changed:running",
		"starter",
		"subagent_terminal:completed",
	}
	if len(events) != len(want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("events = %v, want %v", events, want)
		}
	}
}

func TestManagerRejectsNestedAndWritableSpawn(t *testing.T) {
	manager := NewManager(context.Background(), Config{
		Starter: func(context.Context, StartRequest) (string, error) { return "", nil },
	})
	for name, testCase := range map[string]struct {
		request SpawnRequest
		code    string
	}{
		"nested": {
			request: SpawnRequest{Description: "x", Prompt: "y", AgentType: "explore", WorkspaceMode: "shared_readonly", ParentAgentID: "child"},
			code:    ErrorBoundaryViolation,
		},
		"writable": {
			request: SpawnRequest{Description: "x", Prompt: "y", AgentType: "explore", WorkspaceMode: "git_worktree", ParentAgentID: "root"},
			code:    ErrorWorkspaceMode,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := manager.Spawn(context.Background(), testCase.request)
			if ErrorCode(err) != testCase.code {
				t.Fatalf("error = %v, code = %q, want %q", err, ErrorCode(err), testCase.code)
			}
		})
	}
}

func TestManagerCancelDoesNotCancelSibling(t *testing.T) {
	started := make(chan string, 2)
	manager := NewManager(context.Background(), Config{
		MaxConcurrent: 2,
		Starter: func(ctx context.Context, request StartRequest) (string, error) {
			started <- request.Task.TaskID
			<-ctx.Done()
			return "", ctx.Err()
		},
	})
	first, _ := manager.Spawn(context.Background(), SpawnRequest{Description: "one", Prompt: "one", AgentType: "explore", WorkspaceMode: "shared_readonly"})
	second, _ := manager.Spawn(context.Background(), SpawnRequest{Description: "two", Prompt: "two", AgentType: "review", WorkspaceMode: "shared_readonly"})
	<-started
	<-started
	if _, err := manager.Cancel(context.Background(), first.TaskID, "test"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	waited, err := manager.Wait(context.Background(), WaitRequest{TaskIDs: []string{first.TaskID}, Timeout: time.Second})
	if err != nil || waited[0].Status != StatusCancelled {
		t.Fatalf("cancelled task = %+v, err=%v", waited, err)
	}
	sibling, err := manager.Status(second.TaskID)
	if err != nil || sibling.Status != StatusRunning {
		t.Fatalf("sibling = %+v, err=%v", sibling, err)
	}
	_ = manager.Close(context.Background())
}

func TestManagerWaitAnyOnlyDeliversNewCompletions(t *testing.T) {
	releaseFirst := make(chan struct{})
	releaseSecond := make(chan struct{})
	manager := NewManager(context.Background(), Config{
		MaxConcurrent: 2,
		Starter: func(_ context.Context, request StartRequest) (string, error) {
			if request.Task.Description == "first" {
				<-releaseFirst
			} else {
				<-releaseSecond
			}
			return request.Task.Description, nil
		},
	})
	first, err := manager.Spawn(context.Background(), SpawnRequest{
		Description: "first", Prompt: "first", AgentType: "explore", WorkspaceMode: "shared_readonly",
	})
	if err != nil {
		t.Fatalf("spawn first: %v", err)
	}
	second, err := manager.Spawn(context.Background(), SpawnRequest{
		Description: "second", Prompt: "second", AgentType: "review", WorkspaceMode: "shared_readonly",
	})
	if err != nil {
		t.Fatalf("spawn second: %v", err)
	}

	close(releaseFirst)
	completed, err := manager.Wait(context.Background(), WaitRequest{
		TaskIDs: []string{first.TaskID, second.TaskID}, ReturnWhen: "any", Timeout: time.Second,
	})
	if err != nil || len(completed) != 1 || completed[0].TaskID != first.TaskID {
		t.Fatalf("first completion = %+v, err=%v", completed, err)
	}
	current, err := manager.Wait(context.Background(), WaitRequest{
		TaskIDs: []string{first.TaskID, second.TaskID}, ReturnWhen: "any",
	})
	if err != nil || len(current) != 2 {
		t.Fatalf("already-delivered completion should not report ready: %+v, err=%v", current, err)
	}

	close(releaseSecond)
	completed, err = manager.Wait(context.Background(), WaitRequest{
		TaskIDs: []string{first.TaskID, second.TaskID}, ReturnWhen: "any", Timeout: time.Second,
	})
	if err != nil || len(completed) != 1 || completed[0].TaskID != second.TaskID {
		t.Fatalf("second completion = %+v, err=%v", completed, err)
	}
}
