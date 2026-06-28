package streamgate

import (
	"context"
	"testing"
	"time"
)

func TestCapacityWakeWakesCurrentSleeperAndRearms(t *testing.T) {
	wake := NewCapacityWake()

	first, cleanupFirst := wake.Signal(context.Background())
	assertNotDone(t, first, "new signal")

	wake.Wake()
	assertDone(t, first, "first signal after Wake")
	cleanupFirst()

	second, cleanupSecond := wake.Signal(context.Background())
	defer cleanupSecond()
	assertNotDone(t, second, "rearmed signal")

	wake.Wake()
	assertDone(t, second, "second signal after Wake")
}

func TestCapacityWakeOuterContextCancellation(t *testing.T) {
	wake := NewCapacityWake()
	outer, cancelOuter := context.WithCancel(context.Background())
	signal, cleanup := wake.Signal(outer)
	defer cleanup()

	cancelOuter()
	assertDone(t, signal, "signal after outer cancellation")
}

func assertDone(t *testing.T, ctx context.Context, label string) {
	t.Helper()

	select {
	case <-ctx.Done():
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for %s to finish", label)
	}
}

func assertNotDone(t *testing.T, ctx context.Context, label string) {
	t.Helper()

	select {
	case <-ctx.Done():
		t.Fatalf("expected %s to remain active", label)
	default:
	}
}
