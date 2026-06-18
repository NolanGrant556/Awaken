package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"awaken/src/agentcore"
)

func TestRunServiceReplaysHistoryAndStreamsLiveEvents(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	service := NewRunService(func(workspace WorkspaceConfig) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextMessageStartedEventType, MessageID: "msg-1", Role: "assistant"})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextDeltaEventType, MessageID: "msg-1", Role: "assistant", Delta: "hello"})
			close(started)
			<-release
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextMessageFinishedEventType, MessageID: "msg-1"})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			return "done", nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	<-started

	sub, err := service.Subscribe(run.RunID)
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}

	if len(sub.History) != 3 {
		t.Fatalf("expected 3 replayed events, got %d", len(sub.History))
	}
	if sub.History[0].Kind != EventKindAgent || sub.History[0].AgentEvent.Type != agentcore.RunStartedEventType {
		t.Fatalf("unexpected first replay event: %+v", sub.History[0])
	}

	close(release)

	var live []Event
	for event := range sub.Events {
		live = append(live, event)
	}

	if len(live) != 2 {
		t.Fatalf("expected 2 live events, got %d", len(live))
	}
	if live[1].AgentEvent.Type != agentcore.RunFinishedEventType {
		t.Fatalf("expected final RunFinished event, got %+v", live[1])
	}

	finishedDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(finishedDeadline) {
		snapshot, err := service.CancelRun(run.RunID)
		if err != nil {
			t.Fatalf("CancelRun returned error while waiting for terminal state: %v", err)
		}
		if snapshot.Status == RunStatusFinished {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run did not reach finished state")
}

func TestRunServiceCancelSuppressesRunErrorAndEmitsCancelled(t *testing.T) {
	started := make(chan struct{})
	service := NewRunService(func(workspace WorkspaceConfig) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			close(started)
			<-ctx.Done()
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunErrorEventType, Error: ctx.Err().Error()})
			return "", ctx.Err()
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "cancel me", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	<-started

	sub, err := service.Subscribe(run.RunID)
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}

	snapshot, err := service.CancelRun(run.RunID)
	if err != nil {
		t.Fatalf("CancelRun returned error: %v", err)
	}
	if snapshot.Status != RunStatusCancelling {
		t.Fatalf("expected cancelling status, got %s", snapshot.Status)
	}

	var got []Event
	got = append(got, sub.History...)
	if sub.Events != nil {
		for event := range sub.Events {
			got = append(got, event)
		}
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 events total, got %d", len(got))
	}
	if got[1].Kind != EventKindRunCancelled {
		t.Fatalf("expected terminal cancelled event, got %+v", got[1])
	}

	final, err := service.CancelRun(run.RunID)
	if err != nil {
		t.Fatalf("CancelRun on terminal run returned error: %v", err)
	}
	if final.Status != RunStatusCancelled {
		t.Fatalf("expected cancelled status, got %s", final.Status)
	}
}

func TestRunServiceCompletesWhenExecutorReturnsWithoutTerminalEvent(t *testing.T) {
	service := NewRunService(func(workspace WorkspaceConfig) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			return "done without explicit terminal event", nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "finish me", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	sub, err := service.Subscribe(run.RunID)
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}

	var got []Event
	got = append(got, sub.History...)
	for event := range sub.Events {
		got = append(got, event)
	}

	if len(got) != 2 {
		t.Fatalf("expected RunStarted plus fallback RunFinished, got %d", len(got))
	}
	if got[1].AgentEvent.Type != agentcore.RunFinishedEventType {
		t.Fatalf("expected fallback RunFinished event, got %+v", got[1])
	}

	final, err := service.CancelRun(run.RunID)
	if err != nil {
		t.Fatalf("CancelRun on terminal run returned error: %v", err)
	}
	if final.Status != RunStatusFinished {
		t.Fatalf("expected finished status, got %s", final.Status)
	}
}

func TestRunServiceCancelWinsOverLateSuccessfulReturn(t *testing.T) {
	started := make(chan struct{})
	service := NewRunService(func(workspace WorkspaceConfig) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			close(started)
			<-ctx.Done()
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "too late"})
			return "too late", nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "cancel me", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	<-started

	sub, err := service.Subscribe(run.RunID)
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}

	if _, err := service.CancelRun(run.RunID); err != nil {
		t.Fatalf("CancelRun returned error: %v", err)
	}

	var got []Event
	got = append(got, sub.History...)
	for event := range sub.Events {
		got = append(got, event)
	}

	if len(got) != 2 {
		t.Fatalf("expected RunStarted plus RunCancelled, got %d", len(got))
	}
	if got[1].Kind != EventKindRunCancelled {
		t.Fatalf("expected cancellation to win over late success, got %+v", got[1])
	}

	final, err := service.CancelRun(run.RunID)
	if err != nil {
		t.Fatalf("CancelRun on terminal run returned error: %v", err)
	}
	if final.Status != RunStatusCancelled {
		t.Fatalf("expected cancelled status, got %s", final.Status)
	}
}

func TestRunServiceResolveApprovalUnblocksExecutorAndReplaysEvents(t *testing.T) {
	approvalReady := make(chan struct{})
	service := NewRunService(func(workspace WorkspaceConfig) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			awaiter, ok := sink.(agentcore.ApprovalAwaiter)
			if !ok {
				return "", errors.New("expected approval awaiter on runtime sink")
			}
			close(approvalReady)
			decision, err := awaiter.AwaitApproval(ctx, agentcore.ApprovalRequest{
				ID:          "approval-1",
				ToolCallID:  "tool-1",
				ToolName:    "write_file",
				Title:       "Approve write_file",
				Description: "Write README.md",
				RiskTone:    agentcore.ApprovalRiskDanger,
			})
			if err != nil {
				return "", err
			}
			sink.Emit(agentcore.AwakenEvent{
				Type:            agentcore.ArtifactCreatedEventType,
				ToolCallID:      "tool-1",
				ToolName:        "write_file",
				ArtifactID:      "artifact-1",
				ArtifactTitle:   "README.md",
				ArtifactPath:    "/tmp/README.md",
				ArtifactStatus:  agentcore.ArtifactStatusReady,
				ArtifactContent: "# hello",
			})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: string(decision)})
			return string(decision), nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "needs approval", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	<-approvalReady

	var sub Subscription
	deadline := time.Now().Add(2 * time.Second)
	for {
		sub, err = service.Subscribe(run.RunID)
		if err != nil {
			t.Fatalf("Subscribe returned error: %v", err)
		}
		if len(sub.History) >= 3 {
			break
		}
		if sub.Close != nil {
			sub.Close()
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for approval surface event, got %+v", sub.History)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if len(sub.History) != 3 {
		t.Fatalf("expected RunStarted + ApprovalRequested + A2UI surface in history, got %+v", sub.History)
	}
	if sub.History[1].AgentEvent.Type != agentcore.ApprovalRequestedEventType {
		t.Fatalf("expected approval request in history, got %+v", sub.History[1])
	}
	if sub.History[2].AgentEvent.Type != agentcore.ArtifactCreatedEventType ||
		sub.History[2].AgentEvent.ArtifactKind != "a2ui_surface" {
		t.Fatalf("expected A2UI surface artifact in history, got %+v", sub.History[2])
	}

	if err := service.ResolveApproval(run.RunID, "approval-1", agentcore.ApprovalDecisionApproved); err != nil {
		t.Fatalf("ResolveApproval returned error: %v", err)
	}

	var got []Event
	got = append(got, sub.History...)
	for event := range sub.Events {
		got = append(got, event)
	}

	if len(got) != 6 {
		t.Fatalf("expected 6 total events, got %+v", got)
	}
	if got[3].AgentEvent.Type != agentcore.ApprovalResolvedEventType {
		t.Fatalf("expected approval resolved event, got %+v", got[3])
	}
	if got[4].AgentEvent.Type != agentcore.ArtifactCreatedEventType {
		t.Fatalf("expected artifact event after approval, got %+v", got[4])
	}
	if got[5].AgentEvent.Type != agentcore.RunFinishedEventType {
		t.Fatalf("expected terminal RunFinished event, got %+v", got[5])
	}
}

func TestRunServiceValidatesInputAndMissingRun(t *testing.T) {
	service := NewRunService(func(workspace WorkspaceConfig) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			return "", errors.New("should not run")
		})
	})

	if _, err := service.StartRun(StartRunRequest{Input: "   ", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
	if _, err := service.Subscribe("missing"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound from Subscribe, got %v", err)
	}
	if _, err := service.CancelRun("missing"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound from CancelRun, got %v", err)
	}
	if err := service.ResolveApproval("missing", "approval-1", agentcore.ApprovalDecisionApproved); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound from ResolveApproval, got %v", err)
	}
}

func TestStartRunValidWorkspaceStartsRun(t *testing.T) {
	root := t.TempDir()
	service := NewRunService(func(workspace WorkspaceConfig) Executor {
		if workspace.WorkspaceRoot != root {
			t.Fatalf("expected workspace root %q, got %q", root, workspace.WorkspaceRoot)
		}
		if len(workspace.WritableRoots) != 1 || workspace.WritableRoots[0] != root {
			t.Fatalf("expected writable roots [%q], got %v", root, workspace.WritableRoots)
		}
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			return "done", nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: root}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if run.Status != RunStatusRunning {
		t.Fatalf("expected running status, got %s", run.Status)
	}
}

func TestStartRunEmptyWorkspaceReturnsInvalidInput(t *testing.T) {
	service := NewRunService(func(workspace WorkspaceConfig) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			return "done", nil
		})
	})

	if _, err := service.StartRun(StartRunRequest{Input: "hello"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for empty workspace, got %v", err)
	}
}

func TestStartRunNonAbsoluteWorkspaceReturnsInvalidInput(t *testing.T) {
	service := NewRunService(func(workspace WorkspaceConfig) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			return "done", nil
		})
	})

	if _, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: "relative/path"}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for non-absolute workspace, got %v", err)
	}
}

func TestStartRunOmittedWritableRootsDefaultsToWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	service := NewRunService(func(workspace WorkspaceConfig) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			return "done", nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: root}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if run.RunID == "" {
		t.Fatalf("expected run to be created")
	}
}
