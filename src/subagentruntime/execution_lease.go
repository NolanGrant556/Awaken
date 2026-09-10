package subagentruntime

import (
	"context"
	"sync"
)

type executionLeaseKey struct{}
type executionLease struct {
	mu    sync.Mutex
	slots chan struct{}
	held  bool
}

func acquireExecution(ctx context.Context, slots chan struct{}) (context.Context, *executionLease, error) {
	lease := &executionLease{slots: slots}
	if err := lease.resume(ctx); err != nil {
		return ctx, nil, err
	}
	return context.WithValue(ctx, executionLeaseKey{}, lease), lease, nil
}
func (l *executionLease) release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held {
		<-l.slots
		l.held = false
	}
}
func (l *executionLease) resume(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held {
		return nil
	}
	select {
	case l.slots <- struct{}{}:
		l.held = true
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) EnterExecution(ctx context.Context) (context.Context, func(), error) {
	if lease, ok := ctx.Value(executionLeaseKey{}).(*executionLease); ok && lease != nil {
		return ctx, func() {}, nil
	}
	next, lease, err := acquireExecution(ctx, m.slots)
	if err != nil {
		return ctx, nil, err
	}
	return next, lease.release, nil
}
func (s *scopedController) EnterExecution(ctx context.Context) (context.Context, func(), error) {
	return s.manager.EnterExecution(ctx)
}

// SuspendExecution is for Host waits with no in-flight transport. The caller
// must resume before advancing its model context again.
func SuspendExecution(ctx context.Context) func() error {
	lease, ok := ctx.Value(executionLeaseKey{}).(*executionLease)
	if !ok || lease == nil {
		return func() error { return nil }
	}
	lease.release()
	return func() error { return lease.resume(ctx) }
}

func WithoutExecutionLease(ctx context.Context) context.Context {
	return context.WithValue(ctx, executionLeaseKey{}, (*executionLease)(nil))
}
