package computer

import (
	"testing"
)

func TestSessionStateTransitions(t *testing.T) {
	bind, _ := NewBinding("run-1", "thread-1", "agent-1", "", DefaultPolicy())
	sess, err := NewSession(bind, CaptureWindow, TargetRef{AppName: "Test"})
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	if sess.State != SessionOpening {
		t.Fatalf("expected opening, got %s", sess.State)
	}

	cases := []struct {
		from, to  SessionState
		wantError bool
	}{
		{SessionOpening, SessionReady, false},
		{SessionReady, SessionQuiescing, false},
		{SessionQuiescing, SessionTakenOver, false},
		{SessionQuiescing, SessionPaused, false},
		{SessionPaused, SessionResuming, false},
		{SessionTakenOver, SessionResuming, false},
		{SessionResuming, SessionReady, false},
		{SessionReady, SessionClosing, false},
		{SessionClosing, SessionClosed, false},
		{SessionClosed, SessionReady, true},
		{SessionReady, SessionTakenOver, true},
	}

	for _, tc := range cases {
		sess.State = tc.from
		err := sess.Transition(tc.to)
		if tc.wantError {
			if err == nil {
				t.Errorf("transition %s -> %s should fail", tc.from, tc.to)
			}
			continue
		}
		if err != nil {
			t.Errorf("transition %s -> %s failed: %v", tc.from, tc.to, err)
			continue
		}
		if sess.State != tc.to {
			t.Errorf("transition %s -> %s ended in %s", tc.from, tc.to, sess.State)
		}
	}
}

func TestSessionTerminalBlocksNewActions(t *testing.T) {
	bind, _ := NewBinding("run-1", "thread-1", "agent-1", "", DefaultPolicy())
	sess, _ := NewSession(bind, CaptureWindow, TargetRef{AppName: "Test"})
	_ = sess.MarkReady()
	_ = sess.MarkClosed("test")
	if AllowsNewActions(sess.State) {
		t.Fatalf("closed session should not allow new actions")
	}
}
