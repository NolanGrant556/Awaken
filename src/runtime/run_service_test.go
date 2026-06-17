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
	service := NewRunService(func() Executor {
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

	run, err := service.StartRun("hello")
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

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
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
	service := NewRunService(func() Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			close(started)
			<-ctx.Done()
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunErrorEventType, Error: ctx.Err().Error()})
			return "", ctx.Err()
		})
	})

	run, err := service.StartRun("cancel me")
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
	service := NewRunService(func() Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			return "done without explicit terminal event", nil
		})
	})

	run, err := service.StartRun("finish me")
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
	service := NewRunService(func() Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			close(started)
			<-ctx.Done()
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "too late"})
			return "too late", nil
		})
	})

	run, err := service.StartRun("cancel me")
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

func TestRunServiceValidatesInputAndMissingRun(t *testing.T) {
	service := NewRunService(func() Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			return "", errors.New("should not run")
		})
	})

	if _, err := service.StartRun("   "); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
	if _, err := service.Subscribe("missing"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound from Subscribe, got %v", err)
	}
	if _, err := service.CancelRun("missing"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound from CancelRun, got %v", err)
	}
}
