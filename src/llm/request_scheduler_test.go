package llm

import (
	"context"
	"testing"
	"time"
)

func TestRequestSchedulerAccountIsolationCancellationAndCleanup(t *testing.T) {
	s := NewRequestScheduler(2, 8)
	s.SetAccountLimit(1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	releaseA, err := s.AcquireScoped(ctx, RequestPurposeAgentTurn, "account-a", "task-a")
	if err != nil {
		t.Fatal(err)
	}
	waitCtx, cancelWait := context.WithCancel(ctx)
	waited := make(chan error, 1)
	go func() {
		release, err := s.AcquireScoped(waitCtx, RequestPurposeSubagent, "account-a", "task-a")
		if release != nil {
			release()
		}
		waited <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		queued := len(s.queue)
		s.mu.Unlock()
		if queued == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("request did not queue")
		}
		time.Sleep(time.Millisecond)
	}
	releaseB, err := s.AcquireScoped(ctx, RequestPurposeAgentTurn, "account-b", "task-b")
	if err != nil {
		t.Fatal("independent account was blocked", err)
	}
	cancelWait()
	if err := <-waited; err != context.Canceled {
		t.Fatalf("cancelled wait: %v", err)
	}
	releaseA()
	releaseB()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != 0 || len(s.queue) != 0 || len(s.accounts) != 0 || len(s.groupActive) != 0 || len(s.lastServed) != 0 {
		t.Fatal("scheduler retained completed work")
	}
}

func TestRequestSchedulerPriorityHasBoundedOrdinaryWait(t *testing.T) {
	s := NewRequestScheduler(1, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	hold, err := s.Acquire(ctx, RequestPurposeAgentTurn)
	if err != nil {
		t.Fatal(err)
	}
	ordinary := &requestWaiter{ready: make(chan struct{}), group: "ordinary"}
	s.mu.Lock()
	for i := 0; i < 5; i++ {
		s.queue = append(s.queue, &requestWaiter{ready: make(chan struct{}), priority: true, group: "compact"})
	}
	s.queue = append(s.queue, ordinary)
	s.mu.Unlock()
	hold()
	s.mu.Lock()
	for i := 0; i < 3; i++ {
		if ordinary.granted {
			break
		}
		s.active--
		s.accounts[""]--
		s.groupActive["compact"]--
		s.admitLocked()
	}
	granted := ordinary.granted
	s.mu.Unlock()
	if !granted {
		t.Fatal("ordinary request starved behind compact requests")
	}
}
