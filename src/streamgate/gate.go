package streamgate

import "sync"

// Gate queues stream items while an initial flush is active.
type Gate[T any] struct {
	mu      sync.Mutex
	active  bool
	pending []T
}

// NewGate returns an empty gate. The zero value is also ready to use.
func NewGate[T any]() *Gate[T] {
	return &Gate[T]{}
}

// Start marks the gate active. Enqueue will queue items until End, Drop, or
// Deactivate clears the active flag.
func (g *Gate[T]) Start() {
	g.mu.Lock()
	g.active = true
	g.mu.Unlock()
}

// Enqueue appends items when the gate is active and reports whether they were
// queued. If the gate is inactive, callers should send the items directly.
func (g *Gate[T]) Enqueue(items ...T) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.active {
		return false
	}
	g.pending = append(g.pending, items...)
	return true
}

// End clears the active flag and returns queued items in enqueue order.
func (g *Gate[T]) End() []T {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.active = false
	items := append([]T(nil), g.pending...)
	clear(g.pending)
	g.pending = nil
	return items
}

// Drop clears the active flag, discards queued items, and returns their count.
func (g *Gate[T]) Drop() int {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.active = false
	count := len(g.pending)
	clear(g.pending)
	g.pending = nil
	return count
}

// Deactivate clears the active flag without dropping queued items.
func (g *Gate[T]) Deactivate() {
	g.mu.Lock()
	g.active = false
	g.mu.Unlock()
}

// PendingCount returns the number of queued items.
func (g *Gate[T]) PendingCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()

	return len(g.pending)
}

// Active reports whether the gate is currently queueing items.
func (g *Gate[T]) Active() bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.active
}
