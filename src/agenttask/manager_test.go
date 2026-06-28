package agenttask

import (
	"errors"
	"testing"
	"time"
)

func TestManagerStartListReadOutputCursor(t *testing.T) {
	manager := newTestManager()

	task, err := manager.Start(StartOptions{
		ID:             "task-1",
		ParentRunID:    "run-1",
		Description:    "inspect repo",
		Prompt:         "Find the relevant package.",
		Mode:           TaskModeBackground,
		Agent:          TaskAgent{Type: "general-purpose", Name: "scout", Model: "sonnet"},
		Workspace:      "/tmp/awaken",
		TranscriptPath: "/tmp/awaken/task-1.jsonl",
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if task.Status != TaskStatusRunning {
		t.Fatalf("expected running status, got %s", task.Status)
	}
	if task.ParentRunID != "run-1" || task.Agent.Type != "general-purpose" || task.Agent.Name != "scout" || task.Agent.Model != "sonnet" {
		t.Fatalf("task lost identity fields: %+v", task)
	}

	if _, err := manager.RegisterEvent("task-1", TaskEvent{
		Type:    TaskEventOutput,
		Content: "reading files",
	}); err != nil {
		t.Fatalf("RegisterEvent output returned error: %v", err)
	}
	if _, err := manager.RegisterEvent("task-1", TaskEvent{
		Type:             TaskEventToolUse,
		ToolName:         "Read",
		Activity:         "Reading src/runtime/session.go",
		InputTokenDelta:  5,
		OutputTokenDelta: 7,
	}); err != nil {
		t.Fatalf("RegisterEvent tool use returned error: %v", err)
	}

	list := manager.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 task, got %d", len(list))
	}
	if list[0].Progress.ToolUseCount != 1 || list[0].Progress.TokenCount != 12 {
		t.Fatalf("unexpected progress counters: %+v", list[0].Progress)
	}
	if list[0].Progress.LastActivity != "Reading src/runtime/session.go" {
		t.Fatalf("unexpected last activity: %q", list[0].Progress.LastActivity)
	}

	firstPage, err := manager.ReadOutput("task-1", 0, 2)
	if err != nil {
		t.Fatalf("ReadOutput returned error: %v", err)
	}
	if len(firstPage.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(firstPage.Events))
	}
	if firstPage.Events[0].Seq != 1 || firstPage.Events[0].Type != TaskEventStarted {
		t.Fatalf("expected start event at seq 1, got %+v", firstPage.Events[0])
	}
	if firstPage.Events[1].Seq != 2 || firstPage.Events[1].Content != "reading files" {
		t.Fatalf("expected output event at seq 2, got %+v", firstPage.Events[1])
	}
	if !firstPage.Cursor.HasMore || firstPage.Cursor.NextAfterSeq != 2 {
		t.Fatalf("unexpected first cursor: %+v", firstPage.Cursor)
	}

	if _, err := manager.RegisterEvent("task-1", TaskEvent{Content: "done reading"}); err != nil {
		t.Fatalf("RegisterEvent after cursor returned error: %v", err)
	}

	replayFirstPage, err := manager.ReadOutput("task-1", 0, 2)
	if err != nil {
		t.Fatalf("ReadOutput replay returned error: %v", err)
	}
	if replayFirstPage.Cursor.NextAfterSeq != firstPage.Cursor.NextAfterSeq {
		t.Fatalf("expected stable cursor, got first=%+v replay=%+v", firstPage.Cursor, replayFirstPage.Cursor)
	}
	if replayFirstPage.Events[0].Seq != firstPage.Events[0].Seq || replayFirstPage.Events[1].Seq != firstPage.Events[1].Seq {
		t.Fatalf("ReadOutput mutated its internal offset: first=%+v replay=%+v", firstPage.Events, replayFirstPage.Events)
	}

	secondPage, err := manager.ReadOutput("task-1", firstPage.Cursor.NextAfterSeq, 10)
	if err != nil {
		t.Fatalf("ReadOutput second page returned error: %v", err)
	}
	if len(secondPage.Events) != 2 {
		t.Fatalf("expected 2 events after cursor, got %d", len(secondPage.Events))
	}
	if secondPage.Events[0].Seq != 3 || secondPage.Events[1].Seq != 4 {
		t.Fatalf("unexpected second page events: %+v", secondPage.Events)
	}
	if secondPage.Cursor.HasMore || secondPage.Cursor.NextAfterSeq != 4 || secondPage.Cursor.LastSeq != 4 {
		t.Fatalf("unexpected second cursor: %+v", secondPage.Cursor)
	}

	futurePage, err := manager.ReadOutput("task-1", 99, 10)
	if err != nil {
		t.Fatalf("ReadOutput future cursor returned error: %v", err)
	}
	if len(futurePage.Events) != 0 || futurePage.Cursor.NextAfterSeq != 99 || futurePage.Cursor.LastSeq != 4 {
		t.Fatalf("unexpected future cursor page: %+v", futurePage)
	}
}

func TestManagerSendMessageQueuesContinuation(t *testing.T) {
	manager := newTestManager()
	startTask(t, manager, "task-message", TaskModeBackground)

	continuation, err := manager.SendMessage("task-message", ContinuationRequest{
		Sender:  "lead",
		Summary: "add tests",
		Content: "Please add the missing tests before finishing.",
	})
	if err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}
	if continuation.ID != "cont-1" || continuation.MessageID != "msg-1" || continuation.TaskID != "task-message" {
		t.Fatalf("unexpected continuation identity: %+v", continuation)
	}
	if continuation.ParentRunID != "run-parent" {
		t.Fatalf("expected parent run id on continuation, got %q", continuation.ParentRunID)
	}

	task, err := manager.Get("task-message")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if len(task.Messages) != 1 || len(task.Continuations) != 1 {
		t.Fatalf("expected queued message and continuation, got messages=%d continuations=%d", len(task.Messages), len(task.Continuations))
	}
	if task.Messages[0].Content != "Please add the missing tests before finishing." {
		t.Fatalf("unexpected message content: %+v", task.Messages[0])
	}
	if task.Progress.MessageCount != 1 || task.Progress.LastActivity != "message_sent" {
		t.Fatalf("unexpected progress after send: %+v", task.Progress)
	}

	task.Messages[0].Content = "mutated by caller"
	again, err := manager.Get("task-message")
	if err != nil {
		t.Fatalf("Get after mutation returned error: %v", err)
	}
	if again.Messages[0].Content == "mutated by caller" {
		t.Fatalf("Get returned mutable internal message slice")
	}

	page, err := manager.ReadOutput("task-message", 0, 0)
	if err != nil {
		t.Fatalf("ReadOutput returned error: %v", err)
	}
	if got := page.Events[len(page.Events)-1]; got.Type != TaskEventMessageSent || got.MessageID != "msg-1" {
		t.Fatalf("expected message sent event, got %+v", got)
	}
}

func TestManagerStopCompleteFailTransitions(t *testing.T) {
	manager := newTestManager()
	startTask(t, manager, "task-stop", TaskModeBackground)

	stopped, err := manager.RequestStop("task-stop", StopReasonUser, "user requested stop")
	if err != nil {
		t.Fatalf("RequestStop returned error: %v", err)
	}
	if stopped.Status != TaskStatusStopped || !stopped.StopRequested || !stopped.CancelRequested {
		t.Fatalf("expected stopped task with stop flags, got %+v", stopped)
	}
	if stopped.StopReason != StopReasonUser || stopped.StopMessage != "user requested stop" || stopped.FinishedAt == nil || stopped.StopRequestedAt == nil {
		t.Fatalf("unexpected stop metadata: %+v", stopped)
	}
	if _, err := manager.SendMessage("task-stop", ContinuationRequest{Content: "keep going"}); !errors.Is(err, ErrTaskTerminal) {
		t.Fatalf("expected terminal send error, got %v", err)
	}
	if _, err := manager.Complete("task-stop", "late result"); !errors.Is(err, ErrTaskTerminal) {
		t.Fatalf("expected terminal complete error, got %v", err)
	}
	if _, err := manager.RegisterEvent("task-stop", TaskEvent{Content: "late output"}); !errors.Is(err, ErrTaskTerminal) {
		t.Fatalf("expected terminal register error, got %v", err)
	}

	startTask(t, manager, "task-complete", TaskModeBackground)
	completed, err := manager.Complete("task-complete", "final answer")
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if completed.Status != TaskStatusCompleted || completed.Result != "final answer" || completed.FinishedAt == nil {
		t.Fatalf("unexpected completed task: %+v", completed)
	}
	if _, err := manager.Fail("task-complete", "too late"); !errors.Is(err, ErrTaskTerminal) {
		t.Fatalf("expected terminal fail error, got %v", err)
	}

	startTask(t, manager, "task-fail", TaskModeBackground)
	failed, err := manager.Fail("task-fail", "boom")
	if err != nil {
		t.Fatalf("Fail returned error: %v", err)
	}
	if failed.Status != TaskStatusFailed || failed.Error != "boom" || failed.FinishedAt == nil {
		t.Fatalf("unexpected failed task: %+v", failed)
	}
}

func TestManagerInvalidTransitionsAndValidation(t *testing.T) {
	manager := newTestManager()

	if _, err := manager.Start(StartOptions{
		ID:        "bad/id",
		Agent:     TaskAgent{Type: "general-purpose"},
		Workspace: "/tmp/awaken",
	}); !errors.Is(err, ErrInvalidTaskID) {
		t.Fatalf("expected invalid task id, got %v", err)
	}

	if _, err := manager.Start(StartOptions{
		ID:        "task-missing-agent",
		Workspace: "/tmp/awaken",
	}); !errors.Is(err, ErrInvalidAgent) {
		t.Fatalf("expected invalid agent, got %v", err)
	}

	if _, err := manager.Start(StartOptions{
		ID:        "task-relative-workspace",
		Agent:     TaskAgent{Type: "general-purpose"},
		Workspace: "relative/path",
	}); !errors.Is(err, ErrInvalidWorkspace) {
		t.Fatalf("expected invalid workspace, got %v", err)
	}

	if _, err := manager.Start(StartOptions{
		ID:        "task-bad-mode",
		Mode:      TaskMode("sideways"),
		Agent:     TaskAgent{Type: "general-purpose"},
		Workspace: "/tmp/awaken",
	}); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("expected invalid mode, got %v", err)
	}

	startTask(t, manager, "task-duplicate", TaskModeBackground)
	if _, err := manager.Start(StartOptions{
		ID:        "task-duplicate",
		Agent:     TaskAgent{Type: "general-purpose"},
		Workspace: "/tmp/awaken",
	}); !errors.Is(err, ErrTaskExists) {
		t.Fatalf("expected duplicate task error, got %v", err)
	}

	if _, err := manager.ReadOutput("task-duplicate", 0, -1); !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("expected invalid limit error, got %v", err)
	}
	if _, err := manager.Complete("task-missing", "result"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("expected task not found error, got %v", err)
	}
	if _, err := manager.SendMessage("task-duplicate", ContinuationRequest{TaskID: "other-task", Content: "hello"}); !errors.Is(err, ErrInvalidTaskID) {
		t.Fatalf("expected mismatched continuation task id error, got %v", err)
	}
	if _, err := manager.SendMessage("task-duplicate", ContinuationRequest{Content: ""}); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("expected invalid message error, got %v", err)
	}
	if _, err := manager.RequestStop("task-duplicate", StopReason("unknown"), "stop"); !errors.Is(err, ErrInvalidStopReason) {
		t.Fatalf("expected invalid stop reason error, got %v", err)
	}
}

func TestManagerModes(t *testing.T) {
	manager := newTestManager()

	foreground := startTask(t, manager, "task-foreground", TaskModeForeground)
	if foreground.Mode != TaskModeForeground {
		t.Fatalf("expected foreground mode, got %s", foreground.Mode)
	}

	background, err := manager.Start(StartOptions{
		ID:        "task-default-background",
		Agent:     TaskAgent{Type: "general-purpose"},
		Workspace: "/tmp/awaken",
	})
	if err != nil {
		t.Fatalf("Start default mode returned error: %v", err)
	}
	if background.Mode != TaskModeBackground {
		t.Fatalf("expected default background mode, got %s", background.Mode)
	}
}

func TestManagerStartAutoIDSkipsExistingGeneratedIDs(t *testing.T) {
	manager := newTestManager()

	startTask(t, manager, "agenttask-1", TaskModeBackground)

	task, err := manager.Start(StartOptions{
		Agent:     TaskAgent{Type: "general-purpose"},
		Workspace: "/tmp/awaken",
	})
	if err != nil {
		t.Fatalf("Start auto id returned error: %v", err)
	}
	if task.ID != "agenttask-2" {
		t.Fatalf("expected auto-generated id to skip existing generated name, got %q", task.ID)
	}
}

func startTask(t *testing.T, manager *Manager, id string, mode TaskMode) Task {
	t.Helper()

	task, err := manager.Start(StartOptions{
		ID:          id,
		ParentRunID: "run-parent",
		Description: "test task",
		Prompt:      "do work",
		Mode:        mode,
		Agent:       TaskAgent{Type: "general-purpose", Name: "worker", Model: "sonnet"},
		Workspace:   "/tmp/awaken",
	})
	if err != nil {
		t.Fatalf("Start(%s) returned error: %v", id, err)
	}
	return task
}

func newTestManager() *Manager {
	manager := NewManager()
	base := time.Unix(1_700_000_000, 0).UTC()
	tick := 0
	manager.now = func() time.Time {
		tick++
		return base.Add(time.Duration(tick) * time.Second)
	}
	return manager
}
