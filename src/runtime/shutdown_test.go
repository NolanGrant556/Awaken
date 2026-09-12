package runtime

import (
	"awaken/src/agentcore"
	"awaken/src/runcontrol"
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func waitQuestion(t *testing.T, s *Session) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		count := len(s.userQuestions)
		s.mu.Unlock()
		if count == 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("question not registered")
}

func TestStopQuestionRejectsLateAnswerAndRecordsWithdrawal(t *testing.T) {
	ctx, cancel := runcontrol.WithCancel(context.Background())
	defer cancel()
	s := NewSession("r", "t", "", nil, nil, cancel)
	s.stopContext = ctx
	done := make(chan error, 1)
	go func() { _, err := s.AwaitUserQuestion(ctx, agentcore.AwakenEvent{QuestionID: "q"}); done <- err }()
	waitQuestion(t, s)
	s.RequestCancelWithSource(runcontrol.Shutdown)
	if err := s.ResolveUserQuestion("q", "late"); err == nil {
		t.Fatal("accepted after Stop")
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error: %v", err)
	}
	count := 0
	for _, e := range s.History(0) {
		if e.AgentEvent.Type == agentcore.UserQuestionResolvedType {
			count++
			if e.AgentEvent.QuestionCloseReason != "run_terminated" {
				t.Fatalf("wrong reason: %+v", e.AgentEvent)
			}
		}
	}
	if count != 1 {
		t.Fatalf("resolved %d times", count)
	}
	s.RequestCancelWithSource(runcontrol.User)
	s.CompleteCancelled("", "")
	if s.Snapshot().StopSource != "shutdown" || s.Snapshot().Status != RunStatusCancelled {
		t.Fatal("lost source")
	}
}

func TestQuestionAnswerStopRaceCompletesOnce(t *testing.T) {
	for i := 0; i < 50; i++ {
		ctx, cancel := runcontrol.WithCancel(context.Background())
		s := NewSession("r", "t", "", nil, nil, cancel)
		s.stopContext = ctx
		done := make(chan struct{})
		go func() { defer close(done); _, _ = s.AwaitUserQuestion(ctx, agentcore.AwakenEvent{QuestionID: "q"}) }()
		waitQuestion(t, s)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _ = s.ResolveUserQuestion("q", "yes") }()
		go func() { defer wg.Done(); s.RequestCancel() }()
		wg.Wait()
		<-done
		count := 0
		for _, e := range s.History(0) {
			if e.AgentEvent.Type == agentcore.UserQuestionResolvedType {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("resolved %d times", count)
		}
		cancel()
	}
}

func TestShutdownFencesStartsAndDrainsTrailingWork(t *testing.T) {
	ctx, cancel := runcontrol.WithCancel(context.Background())
	defer cancel()
	s := NewSession("r", "t", "", nil, nil, cancel)
	s.stopContext = ctx
	service := &RunService{sessions: map[string]*Session{"r": s}}
	service.runWG.Add(1)
	service.BeginShutdown()
	service.BeginShutdown()
	if _, err := service.StartRun(StartRunRequest{}); !errors.Is(err, ErrRuntimeShuttingDown) {
		t.Fatalf("admission open: %v", err)
	}
	if ctx.Err() == nil || s.Snapshot().StopSource != "shutdown" {
		t.Fatal("did not stop root")
	}
	deadline, end := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer end()
	if err := service.Drain(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("false drain: %v", err)
	}
	if s.Snapshot().Status != RunStatusCancelling {
		t.Fatal("fabricated terminal")
	}
	service.runWG.Done()
	if err := service.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStopSourceDurableAndLegacyProjection(t *testing.T) {
	for _, source := range []string{"user", "parent", "shutdown", "timeout", "unknown", ""} {
		record := agentcore.SemanticRecord{RunID: "r", ThreadID: "t", Timestamp: time.Now(), Kind: agentcore.SemanticKindRunTerminal, Payload: map[string]any{"status": "cancelled", "terminal_reason": "aborted_tools", "stop_source": source}}
		s := NewSession("r", "t", "", nil, nil, func() {})
		s.RecordSemanticRecords([]agentcore.SemanticRecord{record})
		restored := durableRunsFromSemanticRecords("t", []agentcore.SemanticRecord{record})["r"]
		if restored.Run.StopSource != source || s.Snapshot().StopSource != source || restored.Run.TerminalReason != "aborted_tools" {
			t.Fatalf("projection mismatch: %+v %+v", restored.Run, s.Snapshot())
		}
		event, ok := recoverableAgentEventForTranscript(Event{Kind: EventKindRunCancelled, StopSource: source, TerminalReason: agentcore.TerminalReasonAbortedTools})
		if !ok || event.StopSource != source {
			t.Fatal("v1 fallback lost source")
		}
	}
}

func TestFailedStreamResetDoesNotAccumulateSnapshotText(t *testing.T) {
	s := NewSession("r", "t", "", nil, nil, func() {})
	s.RecordAgentEvent(agentcore.AwakenEvent{Type: agentcore.TextMessageStartedEventType})
	s.RecordAgentEvent(agentcore.AwakenEvent{Type: agentcore.TextDeltaEventType, Delta: "committed progress"})
	s.RecordAgentEvent(agentcore.AwakenEvent{Type: agentcore.TextMessageFinishedEventType})
	for i := 0; i < 3; i++ {
		s.RecordAgentEvent(agentcore.AwakenEvent{Type: agentcore.TextMessageStartedEventType})
		s.RecordAgentEvent(agentcore.AwakenEvent{Type: agentcore.TextDeltaEventType, Delta: "discarded attempt"})
		s.RecordAgentEvent(agentcore.AwakenEvent{Type: agentcore.ProviderReconnectEventType, ProviderReconnect: &agentcore.ProviderReconnectStatus{ResetInProgress: true}})
	}
	if s.Snapshot().Result != "committed progress" {
		t.Fatalf("dirty result: %q", s.Snapshot().Result)
	}
}
