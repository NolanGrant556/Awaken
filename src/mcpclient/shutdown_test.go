package mcpclient

import (
	"context"
	"errors"
	"testing"
	"time"
)

type slowCloseSession struct {
	*blockingRouteSession
	releaseClose chan struct{}
	manager      *Manager
}

func (s *slowCloseSession) Close() error {
	s.manager.mu.RLock()
	s.manager.mu.RUnlock()
	<-s.releaseClose
	return s.blockingRouteSession.Close()
}

func TestShutdownDeadlineAndJoinWithoutGlobalLock(t *testing.T) {
	mgr := NewManager(WithLogger(noopLogger{}))
	session := &slowCloseSession{blockingRouteSession: newBlockingRouteSession(), releaseClose: make(chan struct{}), manager: mgr}
	server := &managedServer{name: "slow", state: StateConnected, session: session}
	mgr.servers["slow"] = server
	op, finish, err := mgr.beginOperation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release, err := mgr.acquireCall(op, server)
	if err != nil {
		t.Fatal(err)
	}
	queued := make(chan error, 1)
	go func() { _, err := mgr.acquireCall(op, server); queued <- err }()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := mgr.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown=%v", err)
	}
	if err := <-queued; err == nil {
		t.Fatal("queued call started")
	}
	if _, _, err := mgr.beginOperation(context.Background()); err == nil {
		t.Fatal("new operation admitted")
	}
	release()
	finish()
	close(session.releaseClose)
	join, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if err := mgr.Shutdown(join); err != nil {
		t.Fatal(err)
	}
}

func TestShutdownJoinsConcurrentDisabledSessionWithoutLockDeadlock(t *testing.T) {
	mgr := NewManager(WithLogger(noopLogger{}))
	session := &slowCloseSession{blockingRouteSession: newBlockingRouteSession(), releaseClose: make(chan struct{}), manager: mgr}
	mgr.servers["slow"] = &managedServer{name: "slow", state: StateConnected, session: session}
	disabled := make(chan error, 1)
	go func() { disabled <- mgr.Disable("slow") }()
	deadline := time.Now().Add(time.Second)
	for {
		mgr.mu.RLock()
		detached := mgr.servers["slow"].session == nil
		mgr.mu.RUnlock()
		if detached {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("did not detach")
		}
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := mgr.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown=%v", err)
	}
	close(session.releaseClose)
	if err := <-disabled; err != nil {
		t.Fatal(err)
	}
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Enable(context.Background(), "slow"); !errors.Is(err, ErrTransportClosed) {
		t.Fatal("reopened after shutdown")
	}
}
