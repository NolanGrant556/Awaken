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
	if result.RetrievalStatus != RetrievalCompleted || len(result.Tasks) != 1 || result.Tasks[0].Status != StatusCompleted || result.Tasks[0].ResultPreview != "done" {
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
	if err != nil || waited.Tasks[0].Status != StatusCancelled {
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
	if err != nil || completed.RetrievalStatus != RetrievalCompleted || len(completed.Tasks) != 1 || completed.Tasks[0].TaskID != first.TaskID {
		t.Fatalf("first completion = %+v, err=%v", completed, err)
	}
	current, err := manager.Wait(context.Background(), WaitRequest{
		TaskIDs: []string{first.TaskID, second.TaskID}, ReturnWhen: "any",
	})
	if err != nil || current.RetrievalStatus != RetrievalRunning || len(current.Tasks) != 2 {
		t.Fatalf("already-delivered completion should not report ready: %+v, err=%v", current, err)
	}

	close(releaseSecond)
	completed, err = manager.Wait(context.Background(), WaitRequest{
		TaskIDs: []string{first.TaskID, second.TaskID}, ReturnWhen: "any", Timeout: time.Second,
	})
	if err != nil || completed.RetrievalStatus != RetrievalCompleted || len(completed.Tasks) != 1 || completed.Tasks[0].TaskID != second.TaskID {
		t.Fatalf("second completion = %+v, err=%v", completed, err)
	}
}

func TestManagerWaitTimeoutIsNeutralAndKeepsCurrentSnapshots(t *testing.T) {
	release := make(chan struct{})
	manager := NewManager(context.Background(), Config{
		Starter: func(_ context.Context, _ StartRequest) (string, error) {
			<-release
			return "done", nil
		},
	})
	t.Cleanup(func() {
		close(release)
		_ = manager.Close(context.Background())
	})
	task, err := manager.Spawn(context.Background(), SpawnRequest{
		Description: "slow", Prompt: "slow", AgentType: "explore", WorkspaceMode: "shared_readonly",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	result, err := manager.Wait(context.Background(), WaitRequest{
		TaskIDs: []string{task.TaskID}, ReturnWhen: "all", Timeout: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("timeout must stay in success channel: %v", err)
	}
	if result.RetrievalStatus != RetrievalTimeout || len(result.Tasks) != 1 || result.Tasks[0].Terminal() {
		t.Fatalf("neutral timeout result = %+v", result)
	}
}

func TestManagerPersistsDeduplicatedToolAndTokenProgress(t *testing.T) {
	var mu sync.Mutex
	var progress []TaskSnapshot
	manager := NewManager(context.Background(), Config{
		Starter: func(ctx context.Context, request StartRequest) (string, error) {
			updates := []ProgressUpdate{
				{ToolCallID: "call-1", ToolName: "Read"},
				{ToolCallID: "call-1", ToolName: "Read"},
				{ModelRequestID: "request-1", InputTokens: 120, OutputTokens: 30, TotalTokens: 150, HasInputTokens: true, HasOutputTokens: true, HasTotalTokens: true},
				{ModelRequestID: "request-1", InputTokens: 120, OutputTokens: 30, TotalTokens: 150, HasInputTokens: true, HasOutputTokens: true, HasTotalTokens: true},
			}
			for _, update := range updates {
				if err := request.ReportProgress(ctx, update); err != nil {
					return "", err
				}
			}
			return "done", nil
		},
		Persist: func(_ context.Context, event PersistEvent) error {
			if event.Kind == "subagent_progress" {
				mu.Lock()
				progress = append(progress, event.Snapshot)
				mu.Unlock()
			}
			return nil
		},
	})
	task, err := manager.Spawn(context.Background(), SpawnRequest{
		Description: "stats", Prompt: "stats", AgentType: "review", WorkspaceMode: "shared_readonly",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	result, err := manager.Wait(context.Background(), WaitRequest{TaskIDs: []string{task.TaskID}, Timeout: time.Second})
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	snapshot := result.Tasks[0]
	if snapshot.ToolUseCount != 1 || snapshot.InputTokens != 120 || snapshot.OutputTokens != 30 || snapshot.TotalTokens != 150 || snapshot.EstimatedTokens != 150 {
		t.Fatalf("terminal stats = %+v", snapshot)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(progress) != 2 || progress[0].ToolUseCount != 1 || progress[1].TotalTokens != 150 {
		t.Fatalf("progress records = %+v", progress)
	}
}
