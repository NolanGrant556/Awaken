package capacitywake

import (
	"context"
	"sync"
	"time"
)

// Wake coordinates sleeps that should end early when capacity becomes
// available or when the parent context is canceled.
type Wake struct {
	parent context.Context

	mu sync.Mutex
	ch chan struct{}
}

// New creates a wake primitive bound to parent. A nil parent is treated as
// context.Background().
func New(parent context.Context) *Wake {
	if parent == nil {
		parent = context.Background()
	}
	return &Wake{
		parent: parent,
		ch:     make(chan struct{}),
	}
}

// Wake releases current waiters and arms a fresh wake channel for future
// waits. It is safe to call concurrently.
func (w *Wake) Wake() {
	w.mu.Lock()
	close(w.ch)
	w.ch = make(chan struct{})
	w.mu.Unlock()
}

// Wait blocks until d elapses, ctx is canceled, the parent context is canceled,
// or Wake is called. It returns nil for normal timeout and wake events.
func (w *Wake) Wait(ctx context.Context, d time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := w.parent.Err(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}

	wakeCh := w.signal()
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-w.parent.Done():
		return w.parent.Err()
	case <-ctx.Done():
		return ctx.Err()
	case <-wakeCh:
		return nil
	case <-timer.C:
		return nil
	}
}

func (w *Wake) signal() <-chan struct{} {
	w.mu.Lock()
	ch := w.ch
	w.mu.Unlock()
	return ch
}
