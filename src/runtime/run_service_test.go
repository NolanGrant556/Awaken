package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"awaken/src/agentcore"
)

func TestRunServiceReplaysHistoryAndStreamsLiveEvents(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
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

	sub, err := service.Subscribe(run.RunID, 0)
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
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
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

	sub, err := service.Subscribe(run.RunID, 0)
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

	if len(got) != 4 {
		t.Fatalf("expected 4 events total, got %d", len(got))
	}
	if got[0].AgentEvent.Type != agentcore.RunStartedEventType {
		t.Fatalf("expected RunStarted event, got %+v", got[0])
	}
	if got[1].Kind != EventKindRunStatusChanged || got[1].Status != string(RunStatusCancelling) {
		t.Fatalf("expected RUN_STATUS_CHANGED(cancelling), got %+v", got[1])
	}
	if got[2].Kind != EventKindRunStatusChanged || got[2].Status != string(RunStatusCancelled) {
		t.Fatalf("expected RUN_STATUS_CHANGED(cancelled), got %+v", got[2])
	}
	if got[3].Kind != EventKindRunCancelled {
		t.Fatalf("expected terminal cancelled event, got %+v", got[3])
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
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			return "done without explicit terminal event", nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "finish me", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	sub, err := service.Subscribe(run.RunID, 0)
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
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
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

	sub, err := service.Subscribe(run.RunID, 0)
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

	if len(got) != 4 {
		t.Fatalf("expected RunStarted plus RunCancelled, got %d", len(got))
	}
	if got[0].AgentEvent.Type != agentcore.RunStartedEventType {
		t.Fatalf("expected RunStarted event, got %+v", got[0])
	}
	if got[1].Kind != EventKindRunStatusChanged || got[1].Status != string(RunStatusCancelling) {
		t.Fatalf("expected RUN_STATUS_CHANGED(cancelling), got %+v", got[1])
	}
	if got[2].Kind != EventKindRunStatusChanged || got[2].Status != string(RunStatusCancelled) {
		t.Fatalf("expected RUN_STATUS_CHANGED(cancelled), got %+v", got[2])
	}
	if got[3].Kind != EventKindRunCancelled {
		t.Fatalf("expected cancellation to win over late success, got %+v", got[3])
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
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
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
		sub, err = service.Subscribe(run.RunID, 0)
		if err != nil {
			t.Fatalf("Subscribe returned error: %v", err)
		}
		if len(sub.History) >= 4 {
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

	if len(sub.History) != 4 {
		t.Fatalf("expected RunStarted + RunStatusChanged + ApprovalRequested + A2UI surface in history, got %+v", sub.History)
	}
	if sub.History[1].Kind != EventKindRunStatusChanged || sub.History[1].Status != string(RunStatusAwaitingApproval) {
		t.Fatalf("expected awaiting_approval status change in history, got %+v", sub.History[1])
	}
	if sub.History[2].AgentEvent.Type != agentcore.ApprovalRequestedEventType {
		t.Fatalf("expected approval request in history, got %+v", sub.History[2])
	}
	if sub.History[3].AgentEvent.Type != agentcore.ArtifactCreatedEventType ||
		sub.History[3].AgentEvent.ArtifactKind != "a2ui_surface" {
		t.Fatalf("expected A2UI surface artifact in history, got %+v", sub.History[3])
	}

	if err := service.ResolveApproval(run.RunID, "approval-1", agentcore.ApprovalDecisionApproved); err != nil {
		t.Fatalf("ResolveApproval returned error: %v", err)
	}

	var got []Event
	got = append(got, sub.History...)
	for event := range sub.Events {
		got = append(got, event)
	}

	if len(got) != 8 {
		t.Fatalf("expected 8 total events, got %+v", got)
	}
	if got[4].Kind != EventKindRunStatusChanged || got[4].Status != string(RunStatusRunning) {
		t.Fatalf("expected running status change after resolve, got %+v", got[4])
	}
	if got[5].AgentEvent.Type != agentcore.ApprovalResolvedEventType {
		t.Fatalf("expected approval resolved event, got %+v", got[5])
	}
	if got[6].AgentEvent.Type != agentcore.ArtifactCreatedEventType {
		t.Fatalf("expected artifact event after approval, got %+v", got[6])
	}
	if got[7].AgentEvent.Type != agentcore.RunFinishedEventType {
		t.Fatalf("expected terminal RunFinished event, got %+v", got[7])
	}
}

func TestResolveApprovalDuplicateReturnsNotFound(t *testing.T) {
	approvalReady := make(chan struct{})
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
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
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: string(decision)})
			return string(decision), nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "needs approval", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	<-approvalReady

	if err := service.ResolveApproval(run.RunID, "approval-1", agentcore.ApprovalDecisionApproved); err != nil {
		t.Fatalf("first ResolveApproval returned error: %v", err)
	}

	if err := service.ResolveApproval(run.RunID, "approval-1", agentcore.ApprovalDecisionApproved); !errors.Is(err, ErrApprovalNotFound) {
		t.Fatalf("expected ErrApprovalNotFound on duplicate resolve, got %v", err)
	}
}

func TestResolveApprovalUnknownIDReturnsNotFound(t *testing.T) {
	approvalReady := make(chan struct{})
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			awaiter, ok := sink.(agentcore.ApprovalAwaiter)
			if !ok {
				return "", errors.New("expected approval awaiter on runtime sink")
			}
			close(approvalReady)
			_, err := awaiter.AwaitApproval(ctx, agentcore.ApprovalRequest{
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
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType})
			return "", nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "needs approval", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	<-approvalReady

	if err := service.ResolveApproval(run.RunID, "unknown-approval", agentcore.ApprovalDecisionApproved); !errors.Is(err, ErrApprovalNotFound) {
		t.Fatalf("expected ErrApprovalNotFound for unknown approval_id, got %v", err)
	}
}

func TestResolveApprovalRejectedRestoresRunningAndEmitsResolved(t *testing.T) {
	approvalReady := make(chan struct{})
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
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
		sub, err = service.Subscribe(run.RunID, 0)
		if err != nil {
			t.Fatalf("Subscribe returned error: %v", err)
		}
		// Wait until the approval request event is present in history. The session
		// also emits an a2ui surface artifact right after, so the history length
		// may be 3 or 4; do not assert on a fixed index.
		hasApprovalRequest := false
		for _, ev := range sub.History {
			if ev.AgentEvent.Type == agentcore.ApprovalRequestedEventType {
				hasApprovalRequest = true
				break
			}
		}
		if hasApprovalRequest {
			break
		}
		if sub.Close != nil {
			sub.Close()
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for approval request event, got %+v", sub.History)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Verify the pre-resolve history contains the awaiting_approval status change.
	foundAwaiting := false
	for _, ev := range sub.History {
		if ev.Kind == EventKindRunStatusChanged && ev.Status == string(RunStatusAwaitingApproval) {
			foundAwaiting = true
			break
		}
	}
	if !foundAwaiting {
		t.Fatalf("expected awaiting_approval status change in pre-resolve history, got %+v", sub.History)
	}

	// Record the history length before resolve so we can search only post-resolve events.
	preResolveLen := len(sub.History)

	if err := service.ResolveApproval(run.RunID, "approval-1", agentcore.ApprovalDecisionRejected); err != nil {
		t.Fatalf("ResolveApproval with rejected returned error: %v", err)
	}

	var got []Event
	got = append(got, sub.History...)
	for event := range sub.Events {
		got = append(got, event)
	}

	if len(got) <= preResolveLen {
		t.Fatalf("expected events after resolve, got none (pre=%d, total=%d)", preResolveLen, len(got))
	}

	// Search post-resolve events for RunStatusChanged(running) followed by
	// ApprovalResolved(decision=rejected). Do not rely on fixed indices because
	// the a2ui surface artifact may or may not have been appended before resolve.
	var (
		sawRunningAfterResolve bool
		sawResolvedRejected    bool
		runningIdx             = -1
		resolvedIdx            = -1
	)
	for i := preResolveLen; i < len(got); i++ {
		ev := got[i]
		if !sawRunningAfterResolve &&
			ev.Kind == EventKindRunStatusChanged &&
			ev.Status == string(RunStatusRunning) {
			sawRunningAfterResolve = true
			runningIdx = i
		}
		if ev.AgentEvent.Type == agentcore.ApprovalResolvedEventType {
			resolvedIdx = i
			if ev.AgentEvent.ApprovalDecision == agentcore.ApprovalDecisionRejected {
				sawResolvedRejected = true
			} else {
				t.Fatalf("expected approval decision rejected, got %q", ev.AgentEvent.ApprovalDecision)
			}
		}
	}

	if !sawRunningAfterResolve {
		t.Fatalf("expected RunStatusChanged(running) after reject resolve, got %+v", got[preResolveLen:])
	}
	if !sawResolvedRejected {
		t.Fatalf("expected ApprovalResolved with decision rejected after resolve, got %+v", got[preResolveLen:])
	}
	if runningIdx >= 0 && resolvedIdx >= 0 && runningIdx > resolvedIdx {
		t.Fatalf("expected RunStatusChanged(running) before ApprovalResolved, got running at %d, resolved at %d", runningIdx, resolvedIdx)
	}
}

func TestResolveApprovalDuplicateRejectedReturnsNotFound(t *testing.T) {
	approvalReady := make(chan struct{})
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
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
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: string(decision)})
			return string(decision), nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "needs approval", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	<-approvalReady

	if err := service.ResolveApproval(run.RunID, "approval-1", agentcore.ApprovalDecisionRejected); err != nil {
		t.Fatalf("first ResolveApproval with rejected returned error: %v", err)
	}

	if err := service.ResolveApproval(run.RunID, "approval-1", agentcore.ApprovalDecisionRejected); !errors.Is(err, ErrApprovalNotFound) {
		t.Fatalf("expected ErrApprovalNotFound on duplicate reject resolve, got %v", err)
	}
}

func TestRunServiceValidatesInputAndMissingRun(t *testing.T) {
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			return "", errors.New("should not run")
		})
	})

	if _, err := service.StartRun(StartRunRequest{Input: "   ", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
	if _, err := service.Subscribe("missing", 0); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound from Subscribe, got %v", err)
	}
	if _, err := service.CancelRun("missing"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound from CancelRun, got %v", err)
	}
	if err := service.ResolveApproval("missing", "approval-1", agentcore.ApprovalDecisionApproved); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound from ResolveApproval, got %v", err)
	}
}

func TestRunServicePlainInputPassesExecutorUnchanged(t *testing.T) {
	received := make(chan string, 1)
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			received <- input
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			return "done", nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "  hello  ", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	if run.Input != "hello" {
		t.Fatalf("expected run input to keep existing trimmed behavior, got %q", run.Input)
	}
	if got := <-received; got != "hello" {
		t.Fatalf("expected executor to receive trimmed plain input, got %q", got)
	}
}

func TestRunServiceAbsolutePathPromptPassesExecutorUnchanged(t *testing.T) {
	received := make(chan string, 1)
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			received <- input
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			return "done", nil
		})
	})

	for _, input := range []string{"/tmp foo", "/var log"} {
		run, err := service.StartRun(StartRunRequest{Input: input, Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
		if err != nil {
			t.Fatalf("StartRun(%q) returned error: %v", input, err)
		}
		if run.Input != input {
			t.Fatalf("expected run input %q, got %q", input, run.Input)
		}
		if got := <-received; got != input {
			t.Fatalf("expected executor to receive %q, got %q", input, got)
		}
	}
}

func TestRunServiceSlashCommandExpandsExecutorInputAndPreservesRunInput(t *testing.T) {
	received := make(chan string, 1)
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			received <- input
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			return "done", nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "/plan add slash commands", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	if run.Input != "/plan add slash commands" {
		t.Fatalf("expected run input to preserve user slash input, got %q", run.Input)
	}

	got := <-received
	for _, want := range []string{
		"Awaken slash command instruction",
		"Slash command:\n/plan",
		"Original user input:\n/plan add slash commands",
		"Command arguments:\nadd slash commands",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected executor input to contain %q, got:\n%s", want, got)
		}
	}
}

func TestRunServiceRejectsInvalidSlashCommands(t *testing.T) {
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			return "should not run", nil
		})
	})

	if _, err := service.StartRun(StartRunRequest{Input: "/", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}}); !errors.Is(err, ErrInvalidSlashCommand) {
		t.Fatalf("expected ErrInvalidSlashCommand for slash only, got %v", err)
	}
	if _, err := service.StartRun(StartRunRequest{Input: "/unknown details", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}}); !errors.Is(err, ErrInvalidSlashCommand) {
		t.Fatalf("expected ErrInvalidSlashCommand for unknown command, got %v", err)
	}
}

func TestStartRunValidWorkspaceStartsRun(t *testing.T) {
	root := t.TempDir()
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			if workspace.WorkspaceRoot != root {
				t.Fatalf("expected workspace root %q, got %q", root, workspace.WorkspaceRoot)
			}
			if len(workspace.WritableRoots) != 1 || workspace.WritableRoots[0] != root {
				t.Fatalf("expected writable roots [%q], got %v", root, workspace.WritableRoots)
			}
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				return "done", nil
			})
		},
	)

	run, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: root}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if run.Status != RunStatusRunning {
		t.Fatalf("expected running status, got %s", run.Status)
	}
}

func TestStartRunEmptyWorkspaceReturnsInvalidInput(t *testing.T) {
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			return "done", nil
		})
	})

	if _, err := service.StartRun(StartRunRequest{Input: "hello"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for empty workspace, got %v", err)
	}
}

func TestStartRunNonAbsoluteWorkspaceReturnsInvalidInput(t *testing.T) {
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
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
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
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

// TestRunServiceGetRunReturnsSnapshot verifies that GetRun returns the current
// run snapshot and that GetRun on a missing run returns ErrRunNotFound.
func TestRunServiceGetRunReturnsSnapshot(t *testing.T) {
	started := make(chan struct{})
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			close(started)
			<-ctx.Done()
			return "", ctx.Err()
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	<-started

	got, err := service.GetRun(run.RunID)
	if err != nil {
		t.Fatalf("GetRun returned error: %v", err)
	}
	if got.RunID != run.RunID {
		t.Fatalf("expected run id %q, got %q", run.RunID, got.RunID)
	}
	if got.Status != RunStatusRunning {
		t.Fatalf("expected running status, got %s", got.Status)
	}

	if _, err := service.CancelRun(run.RunID); err != nil {
		t.Fatalf("CancelRun returned error: %v", err)
	}
}

func TestRunServiceGetRunMissingReturnsNotFound(t *testing.T) {
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			return "done", nil
		})
	})

	if _, err := service.GetRun("missing"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound from GetRun, got %v", err)
	}
}

// TestRunServiceSubscribeAfterSeqFiltersHistory verifies that Subscribe with an
// afterSeq parameter only replays events whose Seq is strictly greater than
// afterSeq, and that live events continue to stream.
func TestRunServiceSubscribeAfterSeqFiltersHistory(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
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

	// First subscription captures the first 3 events and records the last seq.
	full, err := service.Subscribe(run.RunID, 0)
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}
	if len(full.History) != 3 {
		t.Fatalf("expected 3 replayed events, got %d", len(full.History))
	}
	lastSeq := full.History[len(full.History)-1].Seq
	if lastSeq != 3 {
		t.Fatalf("expected last seq 3, got %d", lastSeq)
	}
	if full.Close != nil {
		full.Close()
	}

	// Subscribing with afterSeq=lastSeq should return an empty history but
	// still receive the 2 live events emitted after release.
	incremental, err := service.Subscribe(run.RunID, lastSeq)
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}
	if len(incremental.History) != 0 {
		t.Fatalf("expected 0 replayed events after seq=%d, got %d", lastSeq, len(incremental.History))
	}

	close(release)

	var live []Event
	for event := range incremental.Events {
		live = append(live, event)
	}
	if len(live) != 2 {
		t.Fatalf("expected 2 live events, got %d", len(live))
	}
	// Live events must have seq strictly greater than lastSeq.
	for i, event := range live {
		expected := lastSeq + uint64(i+1)
		if event.Seq != expected {
			t.Fatalf("live event %d: expected seq %d, got %d", i, expected, event.Seq)
		}
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

// TestRunServiceSubscribeAfterSeqZeroReplaysAllHistory verifies that
// Subscribe(runID, 0) replays the full history (all events have seq > 0).
func TestRunServiceSubscribeAfterSeqZeroReplaysAllHistory(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextDeltaEventType, MessageID: "msg-1", Role: "assistant", Delta: "hi"})
			close(started)
			<-release
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			return "done", nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	<-started

	sub, err := service.Subscribe(run.RunID, 0)
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}

	// History should contain the 2 events emitted before release.
	if len(sub.History) != 2 {
		t.Fatalf("expected 2 replayed events, got %d", len(sub.History))
	}
	for i, event := range sub.History {
		expected := uint64(i + 1)
		if event.Seq != expected {
			t.Fatalf("replay event %d: expected seq %d, got %d", i, expected, event.Seq)
		}
	}

	close(release)

	// Live channel should receive the terminal RunFinished event (seq=3).
	var live []Event
	for event := range sub.Events {
		live = append(live, event)
	}
	if len(live) != 1 {
		t.Fatalf("expected 1 live event, got %d", len(live))
	}
	if live[0].Seq != 3 {
		t.Fatalf("expected live event seq 3, got %d", live[0].Seq)
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

// TestStartRunWithoutThreadIDCreatesNewThread verifies that StartRun without a
// thread_id creates a new thread and stores it in the threads map.
func TestStartRunWithoutThreadIDCreatesNewThread(t *testing.T) {
	root := t.TempDir()

	var agentCreateCount int
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			agentCreateCount++
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				Workspace: agentcore.WorkspaceConfig{
					Root:          workspace.WorkspaceRoot,
					WritableRoots: workspace.WritableRoots,
				},
			}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				return "done", nil
			})
		},
	)

	run, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: root}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if run.ThreadID == "" {
		t.Fatalf("expected non-empty thread_id, got empty")
	}
	if agentCreateCount != 1 {
		t.Fatalf("expected newAgent to be called once, got %d", agentCreateCount)
	}
	waitForRunTerminal(t, service, run.RunID)

	// Verify the thread is stored and accessible by starting a second run with the thread_id.
	secondRun, err := service.StartRun(StartRunRequest{
		Input:     "world",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
		ThreadID:  run.ThreadID,
	})
	if err != nil {
		t.Fatalf("second StartRun with thread_id returned error: %v", err)
	}
	if secondRun.ThreadID != run.ThreadID {
		t.Fatalf("expected second run to reuse thread_id %q, got %q", run.ThreadID, secondRun.ThreadID)
	}
	waitForRunTerminal(t, service, secondRun.RunID)
	// Agent should not be created again for the second run.
	if agentCreateCount != 1 {
		t.Fatalf("expected newAgent to still be called once after reuse, got %d", agentCreateCount)
	}
}

// TestStartRunWithValidThreadIDReusesAgent verifies that StartRun with a valid
// thread_id reuses the same Agent instance across runs.
func TestStartRunWithValidThreadIDReusesAgent(t *testing.T) {
	root := t.TempDir()

	var (
		mu               sync.Mutex
		agentCreateCount int
		createdAgent     *agentcore.Agent
		executorAgents   []*agentcore.Agent
	)

	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			mu.Lock()
			defer mu.Unlock()
			agentCreateCount++
			createdAgent = agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				Workspace: agentcore.WorkspaceConfig{
					Root:          workspace.WorkspaceRoot,
					WritableRoots: workspace.WritableRoots,
				},
			}, threadID)
			return createdAgent
		},
		func(agent *agentcore.Agent) Executor {
			mu.Lock()
			executorAgents = append(executorAgents, agent)
			mu.Unlock()
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				return "done", nil
			})
		},
	)

	// First run: creates a new thread and Agent.
	firstRun, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: root}})
	if err != nil {
		t.Fatalf("first StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, firstRun.RunID)

	// Second run: reuses the same thread and Agent.
	secondRun, err := service.StartRun(StartRunRequest{
		Input:     "world",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
		ThreadID:  firstRun.ThreadID,
	})
	if err != nil {
		t.Fatalf("second StartRun with thread_id returned error: %v", err)
	}
	waitForRunTerminal(t, service, secondRun.RunID)

	mu.Lock()
	defer mu.Unlock()
	if agentCreateCount != 1 {
		t.Fatalf("expected newAgent to be called once, got %d", agentCreateCount)
	}
	if len(executorAgents) != 2 {
		t.Fatalf("expected 2 executor agent captures, got %d", len(executorAgents))
	}
	if executorAgents[0] != executorAgents[1] {
		t.Fatalf("expected both runs to receive the same Agent instance")
	}
	if executorAgents[0] != createdAgent {
		t.Fatalf("expected executor to receive the agent created by newAgent")
	}
}

// TestStartRunWithUnknownThreadIDReturnsErrThreadNotFound verifies that
// StartRun with an unknown thread_id returns ErrThreadNotFound.
func TestStartRunWithUnknownThreadIDReturnsErrThreadNotFound(t *testing.T) {
	root := t.TempDir()
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				return "done", nil
			})
		},
	)

	_, err := service.StartRun(StartRunRequest{
		Input:     "hello",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
		ThreadID:  "thread_unknown_123",
	})
	if !errors.Is(err, ErrThreadNotFound) {
		t.Fatalf("expected ErrThreadNotFound, got %v", err)
	}
}

// TestStartRunWithMismatchedWorkspaceReturnsErrThreadWorkspaceMismatch
// verifies that StartRun with a thread_id from a different workspace returns
// ErrThreadWorkspaceMismatch.
func TestStartRunWithMismatchedWorkspaceReturnsErrThreadWorkspaceMismatch(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()

	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				Workspace: agentcore.WorkspaceConfig{
					Root:          workspace.WorkspaceRoot,
					WritableRoots: workspace.WritableRoots,
				},
			}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				return "done", nil
			})
		},
	)

	// First run: creates a thread with workspace A.
	firstRun, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: rootA}})
	if err != nil {
		t.Fatalf("first StartRun returned error: %v", err)
	}

	// Second run: tries to reuse the thread with workspace B.
	_, err = service.StartRun(StartRunRequest{
		Input:     "world",
		Workspace: WorkspaceConfig{WorkspaceRoot: rootB},
		ThreadID:  firstRun.ThreadID,
	})
	if !errors.Is(err, ErrThreadWorkspaceMismatch) {
		t.Fatalf("expected ErrThreadWorkspaceMismatch, got %v", err)
	}
}

func TestStartRunPassesRuntimeThreadIDToNewAgent(t *testing.T) {
	root := t.TempDir()
	var capturedThreadID string
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			capturedThreadID = threadID
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				return "done", nil
			})
		},
	)

	run, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: root}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if capturedThreadID != run.ThreadID {
		t.Fatalf("expected newAgent to receive runtime thread_id %q, got %q", run.ThreadID, capturedThreadID)
	}
}

func TestStartRunWithActiveThreadReturnsErrThreadBusy(t *testing.T) {
	root := t.TempDir()
	started := make(chan struct{})
	var startedOnce sync.Once
	release := make(chan struct{})
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				startedOnce.Do(func() { close(started) })
				<-release
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
				return "done", nil
			})
		},
	)

	firstRun, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: root}})
	if err != nil {
		t.Fatalf("first StartRun returned error: %v", err)
	}
	<-started

	_, err = service.StartRun(StartRunRequest{
		Input:     "second",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
		ThreadID:  firstRun.ThreadID,
	})
	if !errors.Is(err, ErrThreadBusy) {
		t.Fatalf("expected ErrThreadBusy, got %v", err)
	}

	close(release)
	waitForRunTerminal(t, service, firstRun.RunID)
	thirdRun, err := service.StartRun(StartRunRequest{
		Input:     "third",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
		ThreadID:  firstRun.ThreadID,
	})
	if err != nil {
		t.Fatalf("expected thread reuse after first run completed, got %v", err)
	}
	if thirdRun.ThreadID != firstRun.ThreadID {
		t.Fatalf("expected third run to reuse thread %q, got %q", firstRun.ThreadID, thirdRun.ThreadID)
	}
}

func TestStartRunWithSameRootDifferentWritableRootsReturnsWorkspaceMismatch(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatalf("failed to create child dir: %v", err)
	}
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				return "done", nil
			})
		},
	)

	firstRun, err := service.StartRun(StartRunRequest{
		Input:     "hello",
		Workspace: WorkspaceConfig{WorkspaceRoot: root, WritableRoots: []string{root}},
	})
	if err != nil {
		t.Fatalf("first StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, firstRun.RunID)

	_, err = service.StartRun(StartRunRequest{
		Input:     "world",
		Workspace: WorkspaceConfig{WorkspaceRoot: root, WritableRoots: []string{child}},
		ThreadID:  firstRun.ThreadID,
	})
	if !errors.Is(err, ErrThreadWorkspaceMismatch) {
		t.Fatalf("expected ErrThreadWorkspaceMismatch, got %v", err)
	}
}

func TestListThreadsReturnsRuntimeThreads(t *testing.T) {
	root := t.TempDir()
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				return "done", nil
			})
		},
	)

	run, err := service.StartRun(StartRunRequest{Input: "hello thread", Workspace: WorkspaceConfig{WorkspaceRoot: root}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, run.RunID)

	threads := service.ListThreads()
	if len(threads) != 1 {
		t.Fatalf("expected one thread, got %d", len(threads))
	}
	if threads[0].ThreadID != run.ThreadID {
		t.Fatalf("expected thread id %q, got %q", run.ThreadID, threads[0].ThreadID)
	}
	if threads[0].RunID != run.RunID {
		t.Fatalf("expected latest run id %q, got %q", run.RunID, threads[0].RunID)
	}
	if threads[0].ActiveRunID != "" {
		t.Fatalf("expected inactive terminal thread, got active_run_id %q", threads[0].ActiveRunID)
	}
	if threads[0].Label != "hello thread" {
		t.Fatalf("expected thread label from first input, got %q", threads[0].Label)
	}
}

func TestGetThreadTranscriptReturnsOrderedRunsAndInputs(t *testing.T) {
	root := t.TempDir()
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{
				Type:      agentcore.TextMessageStartedEventType,
				MessageID: "msg-1",
				Role:      "assistant",
			})
			sink.Emit(agentcore.AwakenEvent{
				Type:      agentcore.TextDeltaEventType,
				MessageID: "msg-1",
				Role:      "assistant",
				Delta:     input,
			})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: input})
			return input, nil
		})
	})

	firstRun, err := service.StartRun(StartRunRequest{
		Input:     "hello",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("first StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, firstRun.RunID)

	secondRun, err := service.StartRun(StartRunRequest{
		Input:     "follow up",
		ThreadID:  firstRun.ThreadID,
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("second StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, secondRun.RunID)

	transcript, err := service.GetThreadTranscript(firstRun.ThreadID)
	if err != nil {
		t.Fatalf("GetThreadTranscript returned error: %v", err)
	}
	if transcript.Thread.ThreadID != firstRun.ThreadID {
		t.Fatalf("expected thread id %q, got %q", firstRun.ThreadID, transcript.Thread.ThreadID)
	}
	if len(transcript.Runs) != 2 {
		t.Fatalf("expected 2 transcript runs, got %d", len(transcript.Runs))
	}
	if transcript.Runs[0].Run.RunID != firstRun.RunID || transcript.Runs[0].Run.Input != "hello" {
		t.Fatalf("unexpected first transcript run: %+v", transcript.Runs[0].Run)
	}
	if transcript.Runs[1].Run.RunID != secondRun.RunID || transcript.Runs[1].Run.Input != "follow up" {
		t.Fatalf("unexpected second transcript run: %+v", transcript.Runs[1].Run)
	}
	if len(transcript.Runs[1].Events) == 0 {
		t.Fatalf("expected transcript history for second run")
	}
}

func waitForRunTerminal(t *testing.T, service *RunService, runID string) Run {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, err := service.GetRun(runID)
		if err != nil {
			t.Fatalf("GetRun returned error: %v", err)
		}
		if isTerminalStatus(run.Status) {
			return run
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run %s did not reach terminal status", runID)
	return Run{}
}
