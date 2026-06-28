package streamgate

import (
	"context"
	"sync"
)

// CapacityWake wakes the current at-capacity waiter and re-arms future waiters.
type CapacityWake struct {
	mu   sync.Mutex
	wake chan struct{}
}

// NewCapacityWake returns an armed capacity wake primitive.
func NewCapacityWake() *CapacityWake {
	return &CapacityWake{wake: make(chan struct{})}
}

// Signal returns a context canceled by either ctx or the next Wake call.
// Call cleanup after the wait is no longer needed to release resources.
func (w *CapacityWake) Signal(ctx context.Context) (signal context.Context, cleanup func()) {
	w.mu.Lock()
	wake := w.wake
	if wake == nil {
		wake = make(chan struct{})
		w.wake = wake
	}
	w.mu.Unlock()

	signal, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-wake:
			cancel()
		case <-signal.Done():
		}
	}()
	return signal, cancel
}

// Wake cancels current signals and arms a fresh signal channel.
func (w *CapacityWake) Wake() {
	w.mu.Lock()
	wake := w.wake
	if wake == nil {
		w.wake = make(chan struct{})
		w.mu.Unlock()
		return
	}
	w.wake = make(chan struct{})
	w.mu.Unlock()

	close(wake)
}
