package runtime

import (
	"context"
	"testing"
	"time"

	"awaken/src/agentcore"
)

func TestActivitySnapshotNoRunsReturnsZeros(t *testing.T) {
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			return "done", nil
		})
	})

	snapshot := service.ActivitySnapshot()
	if snapshot.Active != 0 || snapshot.Cancelling != 0 {
		t.Fatalf("expected zero activity, got %+v", snapshot)
	}
}

// Scheduled and ACP runs go through the same RunService.StartRun, so counting
// via threads/sessions covers them without a dedicated fixture here.
func TestActivitySnapshotCountsRunningAndCancellingRuns(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			close(started)
			// Ignore ctx so the session stays in cancelling state until release;
			// cancellation still wins over the late success (see
			// TestRunServiceCancelWinsOverLateSuccessfulReturn).
			<-release
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "too late"})
			return "too late", nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	<-started

	snapshot := service.ActivitySnapshot()
	if snapshot.Active != 1 || snapshot.Cancelling != 0 {
		t.Fatalf("expected active=1 cancelling=0 for running run, got %+v", snapshot)
	}

	cancelled, err := service.CancelRun(run.RunID)
	if err != nil {
		t.Fatalf("CancelRun returned error: %v", err)
	}
	if cancelled.Status != RunStatusCancelling {
		t.Fatalf("expected cancelling status, got %s", cancelled.Status)
	}

	snapshot = service.ActivitySnapshot()
	if snapshot.Active != 1 || snapshot.Cancelling != 1 {
		t.Fatalf("expected active=1 cancelling=1 for cancelling run, got %+v", snapshot)
	}

	close(release)
	waitForActivity(t, service, RunActivitySnapshot{})
}

func TestActivitySnapshotReturnsToZeroAfterRunFinishes(t *testing.T) {
	release := make(chan struct{})
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			<-release
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			return "done", nil
		})
	})

	if _, err := service.StartRun(StartRunRequest{Input: "finish me", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}}); err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	waitForActivity(t, service, RunActivitySnapshot{Active: 1})

	close(release)
	waitForActivity(t, service, RunActivitySnapshot{})
}

func waitForActivity(t *testing.T, service *RunService, want RunActivitySnapshot) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := service.ActivitySnapshot()
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("activity snapshot did not settle to %+v, last got %+v", want, got)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
