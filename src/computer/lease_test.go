package computer

import (
	"testing"
	"time"
)

func TestLeaseFenceMonotonic(t *testing.T) {
	lm := NewLeaseManager()
	bind, _ := NewBinding("run-1", "thread-1", "agent-1", "", DefaultPolicy())
	sess, _ := NewSession(bind, CaptureWindow, TargetRef{AppName: "Test"})

	lease1, err := lm.Acquire(bind.ResourceKey, bind, sess.SessionID, "ctrl-1", time.Minute)
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	if lease1.FenceToken != 1 {
		t.Fatalf("first fence should be 1, got %d", lease1.FenceToken)
	}

	lease2, err := lm.Acquire(bind.ResourceKey, bind, sess.SessionID, "ctrl-1", time.Minute)
	if err != nil {
		t.Fatalf("reacquire lease: %v", err)
	}
	if lease2.FenceToken <= lease1.FenceToken {
		t.Fatalf("fence should increase: %d -> %d", lease1.FenceToken, lease2.FenceToken)
	}

	if err := lm.ValidateFence(bind.ResourceKey, lease1.FenceToken); err == nil {
		t.Fatalf("stale fence should be rejected")
	}
	if err := lm.ValidateFence(bind.ResourceKey, lease2.FenceToken); err != nil {
		t.Fatalf("current fence should be accepted: %v", err)
	}
}

func TestLeaseBlocksOtherBinding(t *testing.T) {
	lm := NewLeaseManager()
	bind1, _ := NewBinding("run-1", "thread-1", "agent-1", "", DefaultPolicy())
	bind2, _ := NewBinding("run-2", "thread-2", "agent-2", "", DefaultPolicy())
	sess1, _ := NewSession(bind1, CaptureWindow, TargetRef{AppName: "Test"})

	_, err := lm.Acquire(bind1.ResourceKey, bind1, sess1.SessionID, "ctrl-1", time.Minute)
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}

	sess2, _ := NewSession(bind2, CaptureWindow, TargetRef{AppName: "Test"})
	_, err = lm.Acquire(bind2.ResourceKey, bind2, sess2.SessionID, "ctrl-2", time.Minute)
	if err == nil {
		t.Fatalf("second binding should be blocked from the same resource")
	}
}
