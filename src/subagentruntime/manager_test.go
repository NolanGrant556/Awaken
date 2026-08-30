package subagentruntime

import (
	"context"
	"errors"
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
	if notifications := manager.DrainTerminalNotifications(); len(notifications) != 0 {
		t.Fatalf("explicit wait must consume automatic terminal handoff: %+v", notifications)
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

func TestManagerUsesHostResolvedModelProfileIdentity(t *testing.T) {
	started := make(chan TaskSnapshot, 1)
	manager := NewManager(context.Background(), Config{
		ModelProfileID: "profile-purpose-subagent",
		Starter: func(_ context.Context, request StartRequest) (string, error) {
			started <- request.Task
			return "done", nil
		},
	})
	task, err := manager.Spawn(context.Background(), SpawnRequest{
		Description: "inspect", Prompt: "inspect", AgentType: "explore",
		WorkspaceMode: "shared_readonly", ModelProfileID: "profile-untrusted-input",
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.ModelProfileID != "profile-purpose-subagent" {
		t.Fatalf("spawn identity=%q", task.ModelProfileID)
	}
	if running := <-started; running.ModelProfileID != "profile-purpose-subagent" {
		t.Fatalf("starter identity=%q", running.ModelProfileID)
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
		"readonly-worktree": {
			request: SpawnRequest{Description: "x", Prompt: "y", AgentType: "explore", WorkspaceMode: "git_worktree", ParentAgentID: "root"},
			code:    ErrorWorkspaceMode,
		},
		"unknown-type": {
			request: SpawnRequest{Description: "x", Prompt: "y", AgentType: "custom", WorkspaceMode: "shared_readonly", ParentAgentID: "root"},
			code:    ErrorInvalidInput,
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

func TestManagerAcceptsPlanAndGeneralShared(t *testing.T) {
	manager := NewManager(context.Background(), Config{
		ParentWritableRoots: []string{"/workspace"},
		Starter:             func(context.Context, StartRequest) (string, error) { return "done", nil },
	})
	plan, err := manager.Spawn(context.Background(), SpawnRequest{
		Description: "plan change", Prompt: "plan the auth rewrite", AgentType: AgentTypePlan,
	})
	if err != nil || plan.AgentType != AgentTypePlan || plan.WorkspaceMode != WorkspaceSharedReadOnly {
		t.Fatalf("plan spawn = %+v err=%v", plan, err)
	}
	general, err := manager.Spawn(context.Background(), SpawnRequest{
		Description: "implement", Prompt: "write the file", AgentType: AgentTypeGeneral,
	})
	if err != nil || general.AgentType != AgentTypeGeneral || general.WorkspaceMode != WorkspaceShared {
		t.Fatalf("general spawn = %+v err=%v", general, err)
	}
	_, err = manager.Spawn(context.Background(), SpawnRequest{
		Description: "second write", Prompt: "also write", AgentType: AgentTypeGeneral, WorkspaceMode: WorkspaceShared,
	})
	if ErrorCode(err) != ErrorBoundaryViolation {
		t.Fatalf("overlapping shared write: %v", err)
	}
}

func TestManagerMessageQueuesBeforeChildDrainAndRejectsTerminal(t *testing.T) {
	started := make(chan StartRequest, 1)
	release := make(chan struct{})
	var events []string
	var mu sync.Mutex
	manager := NewManager(context.Background(), Config{
		Starter: func(_ context.Context, request StartRequest) (string, error) {
			started <- request
			<-release
			return "done", nil
		},
		Persist: func(_ context.Context, event PersistEvent) error {
			mu.Lock()
			kind := event.Kind
			if event.Message != nil {
				kind += ":" + event.Message.Content
			}
			events = append(events, kind)
			mu.Unlock()
			return nil
		},
	})
	task, err := manager.Spawn(context.Background(), SpawnRequest{
		Description: "chat", Prompt: "start", AgentType: AgentTypeExplore,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := <-started
	receipt, err := manager.Message(context.Background(), task.TaskID, "follow up")
	if err != nil || receipt.MessageID == "" {
		t.Fatalf("message: %+v err=%v", receipt, err)
	}
	queued := request.DrainMessages()
	if len(queued) != 1 || queued[0].Content != "follow up" {
		t.Fatalf("drain = %+v", queued)
	}
	if second := request.DrainMessages(); len(second) != 0 {
		t.Fatalf("messages must be one-shot: %+v", second)
	}
	close(release)
	_, _ = manager.Wait(context.Background(), WaitRequest{TaskIDs: []string{task.TaskID}, Timeout: time.Second})
	if _, err := manager.Message(context.Background(), task.TaskID, "too late"); ErrorCode(err) != ErrorNotRunning {
		t.Fatalf("terminal message: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, event := range events {
		if event == "subagent_message_queued:follow up" {
			found = true
		}
	}
	if !found {
		t.Fatalf("message was not persisted before queue: %v", events)
	}
}

func TestManagerMessageIdempotency(t *testing.T) {
	manager := NewManager(context.Background(), Config{
		Starter: func(ctx context.Context, _ StartRequest) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
	})
	task, err := manager.Spawn(context.Background(), SpawnRequest{Description: "id", Prompt: "id", AgentType: AgentTypeReview})
	if err != nil {
		t.Fatal(err)
	}
	first, err := manager.MessageIdempotent(context.Background(), task.TaskID, "same", "key-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.MessageIdempotent(context.Background(), task.TaskID, "same", "key-1")
	if err != nil || second.MessageID != first.MessageID {
		t.Fatalf("idempotent replay = %+v err=%v", second, err)
	}
	if _, err := manager.MessageIdempotent(context.Background(), task.TaskID, "other", "key-1"); ErrorCode(err) != ErrorIdempotency {
		t.Fatalf("conflict: %v", err)
	}
	_ = manager.Close(context.Background())
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

func TestManagerPublishesTerminalNotificationAfterWaitTimeout(t *testing.T) {
	release := make(chan struct{})
	manager := NewManager(context.Background(), Config{
		Starter: func(_ context.Context, _ StartRequest) (string, error) {
			<-release
			return "review complete", nil
		},
	})
	task, err := manager.Spawn(context.Background(), SpawnRequest{
		Description: "review", Prompt: "review", AgentType: "review", WorkspaceMode: "shared_readonly",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	result, err := manager.Wait(context.Background(), WaitRequest{
		TaskIDs: []string{task.TaskID}, ReturnWhen: "all", Timeout: time.Millisecond,
	})
	if err != nil || result.RetrievalStatus != RetrievalTimeout {
		t.Fatalf("timeout result = %+v, err=%v", result, err)
	}
	close(release)

	deadline := time.Now().Add(time.Second)
	var notifications []TaskSnapshot
	for time.Now().Before(deadline) {
		notifications = manager.DrainTerminalNotifications()
		if len(notifications) > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if len(notifications) != 1 || notifications[0].TaskID != task.TaskID || notifications[0].Status != StatusCompleted || notifications[0].ResultPreview != "review complete" {
		t.Fatalf("terminal notifications = %+v", notifications)
	}
	if duplicate := manager.DrainTerminalNotifications(); len(duplicate) != 0 {
		t.Fatalf("terminal notification must be one-shot: %+v", duplicate)
	}
	if err := manager.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
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
	if progress[1].CurrentActivity != "正在整理子任务结果" {
		t.Fatalf("model continuation kept stale tool activity: %+v", progress[1])
	}
}

func TestManagerPersistsProgressAfterProducerContextCancellation(t *testing.T) {
	manager := NewManager(context.Background(), Config{
		Starter: func(ctx context.Context, request StartRequest) (string, error) {
			progressCtx, cancel := context.WithCancel(ctx)
			cancel()
			if err := request.ReportProgress(progressCtx, ProgressUpdate{
				ModelRequestID: "request-cancelled", TotalTokens: 42, HasTotalTokens: true,
			}); err != nil {
				return "", err
			}
			return "done", nil
		},
		Persist: func(ctx context.Context, _ PersistEvent) error {
			return ctx.Err()
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
	if len(result.Tasks) != 1 || result.Tasks[0].Status != StatusCompleted || result.Tasks[0].TotalTokens != 42 {
		t.Fatalf("result=%+v", result)
	}
}

func TestManagerCancelQueuedTaskNeverStartsIt(t *testing.T) {
	release := make(chan struct{})
	started := make(chan string, 2)
	manager := NewManager(context.Background(), Config{
		MaxConcurrent: 1,
		Starter: func(ctx context.Context, request StartRequest) (string, error) {
			started <- request.Task.TaskID
			select {
			case <-release:
				return "done", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		},
	})
	first, err := manager.Spawn(context.Background(), SpawnRequest{Description: "first", Prompt: "first", AgentType: "explore", WorkspaceMode: "shared_readonly"})
	if err != nil {
		t.Fatal(err)
	}
	if got := <-started; got != first.TaskID {
		t.Fatalf("unexpected first starter %q", got)
	}
	second, err := manager.Spawn(context.Background(), SpawnRequest{Description: "second", Prompt: "second", AgentType: "review", WorkspaceMode: "shared_readonly"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Cancel(context.Background(), second.TaskID, "test"); err != nil {
		t.Fatal(err)
	}
	result, err := manager.Wait(context.Background(), WaitRequest{TaskIDs: []string{second.TaskID}, Timeout: time.Second})
	if err != nil || result.Tasks[0].Status != StatusCancelled {
		t.Fatalf("queued cancellation = %+v err=%v", result, err)
	}
	select {
	case taskID := <-started:
		t.Fatalf("cancelled queued task started: %s", taskID)
	default:
	}
	close(release)
	_ = manager.Close(context.Background())
}

func TestManagerCancelGraceInterruptsUncooperativeStarter(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	manager := NewManager(context.Background(), Config{
		CancelGrace: 10 * time.Millisecond,
		Starter: func(context.Context, StartRequest) (string, error) {
			close(started)
			<-release
			return "late", nil
		},
	})
	task, err := manager.Spawn(context.Background(), SpawnRequest{Description: "slow", Prompt: "slow", AgentType: "explore", WorkspaceMode: "shared_readonly"})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if _, err := manager.Cancel(context.Background(), task.TaskID, "test"); err != nil {
		t.Fatal(err)
	}
	result, err := manager.Wait(context.Background(), WaitRequest{TaskIDs: []string{task.TaskID}, Timeout: time.Second})
	if err != nil || result.Tasks[0].Status != StatusInterrupted {
		t.Fatalf("cancel grace result = %+v err=%v", result, err)
	}
	close(release)
	_ = manager.Close(context.Background())
}

func TestManagerTerminalPersistenceFailureDoesNotPublishFalseTerminal(t *testing.T) {
	persistFailure := errors.New("disk unavailable")
	manager := NewManager(context.Background(), Config{
		Starter: func(context.Context, StartRequest) (string, error) { return "done", nil },
		Persist: func(_ context.Context, event PersistEvent) error {
			if event.Kind == "subagent_terminal" {
				return persistFailure
			}
			return nil
		},
	})
	task, err := manager.Spawn(context.Background(), SpawnRequest{Description: "persist", Prompt: "persist", AgentType: "review", WorkspaceMode: "shared_readonly"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	snapshot, err := manager.Status(task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Terminal() {
		t.Fatalf("unpersisted terminal became authoritative: %+v", snapshot)
	}
	if notifications := manager.DrainTerminalNotifications(); len(notifications) != 0 {
		t.Fatalf("unpersisted terminal must not notify root: %+v", notifications)
	}
	if err := manager.Close(context.Background()); err == nil || ErrorCode(err) != ErrorPersistence {
		t.Fatalf("close did not surface terminal persistence failure: %v", err)
	}
}
