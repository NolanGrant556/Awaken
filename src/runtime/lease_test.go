package runtime

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLeaseCreateAndHeartbeat(t *testing.T) {
	lm := NewLeaseManager(nil)
	lease, err := lm.CreateLease("bridge", "bridge-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if lease.LeaseID == "" {
		t.Fatal("missing lease_id")
	}
	if lease.OwnerKind != "bridge" || lease.OwnerID != "bridge-1" {
		t.Fatalf("unexpected owner: %+v", lease)
	}

	time.Sleep(100 * time.Millisecond)
	renewed, err := lm.Heartbeat(lease.LeaseID)
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if renewed.LeaseID != lease.LeaseID {
		t.Fatal("lease_id changed after heartbeat")
	}
	if renewed.ExpiresAt.Before(lease.ExpiresAt) {
		t.Fatal("expires_at moved backwards")
	}
}

func TestLeaseDuplicateOwnerReturnsSameLease(t *testing.T) {
	lm := NewLeaseManager(nil)
	lease1, err := lm.CreateLease("bridge", "bridge-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	lease2, err := lm.CreateLease("bridge", "bridge-1")
	if err != nil {
		t.Fatalf("create duplicate: %v", err)
	}
	if lease1.LeaseID != lease2.LeaseID {
		t.Fatal("duplicate owner got different lease")
	}
}

func TestLeaseRelease(t *testing.T) {
	lm := NewLeaseManager(nil)
	lease, _ := lm.CreateLease("bridge", "bridge-1")
	if err := lm.ReleaseLease(lease.LeaseID); err != nil {
		t.Fatalf("release: %v", err)
	}
	if lm.LeaseCount() != 0 {
		t.Fatalf("expected 0 leases, got %d", lm.LeaseCount())
	}
	if err := lm.ReleaseLease(lease.LeaseID); !IsLeaseNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestLeaseHeartbeatUnknown(t *testing.T) {
	lm := NewLeaseManager(nil)
	if _, err := lm.Heartbeat("lease_does_not_exist"); !IsLeaseNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestLeaseExpiry(t *testing.T) {
	lm := NewLeaseManager(nil)
	lease, _ := lm.CreateLease("bridge", "bridge-1")
	// Force expiry by manipulating the stored lease.
	lm.mu.Lock()
	lm.leases[lease.LeaseID].ExpiresAt = time.Now().UTC().Add(-time.Second)
	lm.mu.Unlock()

	if _, err := lm.Heartbeat(lease.LeaseID); !IsLeaseExpired(err) {
		t.Fatalf("expected expired, got %v", err)
	}
	if lm.LeaseCount() != 0 {
		t.Fatalf("expected 0 leases after expiry, got %d", lm.LeaseCount())
	}
}

func TestLeaseIdleCallback(t *testing.T) {
	called := make(chan struct{}, 1)
	lm := NewLeaseManagerWithInterval(func() {
		called <- struct{}{}
	}, 100*time.Millisecond, WithIdleBudget(100*time.Millisecond))
	lease, _ := lm.CreateLease("bridge", "bridge-1")
	lm.mu.Lock()
	lm.leases[lease.LeaseID].ExpiresAt = time.Now().UTC().Add(-time.Second)
	lm.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go lm.Run(ctx)

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("idle callback was not called")
	}
}

func TestLeaseIdleCallbackBlockedByActiveRun(t *testing.T) {
	called := make(chan struct{}, 1)
	activeRuns := 1
	lm := NewLeaseManagerWithInterval(func() {
		called <- struct{}{}
	}, 100*time.Millisecond, WithActiveRunCountFunc(func() int { return activeRuns }), WithIdleBudget(100*time.Millisecond))
	lease, _ := lm.CreateLease("bridge", "bridge-1")
	lm.mu.Lock()
	lm.leases[lease.LeaseID].ExpiresAt = time.Now().UTC().Add(-time.Second)
	lm.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go lm.Run(ctx)

	select {
	case <-called:
		t.Fatal("idle callback called while active runs exist")
	case <-time.After(500 * time.Millisecond):
	}

	activeRuns = 0
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("idle callback was not called after active runs completed")
	}
}

func TestLeaseIdleBudgetContinuous(t *testing.T) {
	called := make(chan struct{}, 1)
	lm := NewLeaseManagerWithInterval(func() {
		called <- struct{}{}
	}, 50*time.Millisecond, WithIdleBudget(150*time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go lm.Run(ctx)

	lease, _ := lm.CreateLease("bridge", "bridge-1")
	lm.mu.Lock()
	lm.leases[lease.LeaseID].ExpiresAt = time.Now().UTC().Add(-time.Second)
	lm.mu.Unlock()

	// Before the continuous idle budget elapses, no callback.
	select {
	case <-called:
		t.Fatal("idle callback called without continuous idle")
	case <-time.After(80 * time.Millisecond):
	}

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("idle callback was not called after continuous idle budget")
	}
}

func IsLeaseNotFound(err error) bool {
	return errors.Is(err, ErrLeaseNotFound)
}

func IsLeaseExpired(err error) bool {
	return errors.Is(err, ErrLeaseExpired)
}
