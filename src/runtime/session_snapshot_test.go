package runtime

import (
	"runtime"
	"testing"
	"time"

	"awaken/src/agentcore"
)

func TestSessionTranscriptSnapshotPublishesTerminalAtomically(t *testing.T) {
	for _, finish := range []struct {
		name    string
		publish func(*Session)
	}{
		{"finished", func(s *Session) { s.CompleteFinished("done") }},
		{"cancelled", func(s *Session) { s.CompleteCancelled("stopped", "") }},
		{"error", func(s *Session) { s.CompleteError("failed", "TEST") }},
		{"agent", func(s *Session) { s.RecordAgentEvent(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType}) }},
		{"semantic", func(s *Session) {
			s.RecordSemanticRecords([]agentcore.SemanticRecord{{Kind: agentcore.SemanticKindRunTerminal, Payload: map[string]any{"status": "finished"}}})
		}},
	} {
		t.Run(finish.name, func(t *testing.T) {
			s := NewSession("run", "thread", "test", nil, nil, nil)
			s.RecordAgentEvent(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			// Block publication after the session writer has acquired its lock.
			s.bus.mu.Lock()
			done := make(chan struct{})
			go func() { finish.publish(s); close(done) }()
			deadline := time.Now().Add(5 * time.Second)
			for s.mu.TryRLock() {
				s.mu.RUnlock()
				if time.Now().After(deadline) {
					s.bus.mu.Unlock()
					t.Fatal("publication never acquired session lock")
				}
				runtime.Gosched()
			}
			captured := make(chan Run, 1)
			go func() {
				run, events, cursor := s.TranscriptSnapshot()
				if len(events) < 2 || cursor != streamCursorForEventSeq(uint64(len(events))) {
					t.Errorf("terminal snapshot did not include published stream: events=%d cursor=%d", len(events), cursor)
				}
				captured <- run
			}()
			s.bus.mu.Unlock()
			<-done
			select {
			case run := <-captured:
				if !isTerminalStatus(run.Status) {
					t.Fatalf("snapshot status=%s", run.Status)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("snapshot deadlocked")
			}
		})
	}
}
