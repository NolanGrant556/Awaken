package sessioningress

import (
	"context"
	"sync"
)

// AppendGate serializes appends per session while allowing different sessions
// to append independently.
type AppendGate struct {
	mu       sync.Mutex
	sessions map[string]*sessionQueue
}

type sessionQueue struct {
	mu   sync.Mutex
	tail <-chan struct{}
}

func NewAppendGate() *AppendGate {
	return &AppendGate{
		sessions: make(map[string]*sessionQueue),
	}
}

func (g *AppendGate) Do(ctx context.Context, sessionID string, fn func(context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}

	queue := g.queue(sessionID)
	return queue.do(ctx, fn)
}

func (g *AppendGate) queue(sessionID string) *sessionQueue {
	g.mu.Lock()
	defer g.mu.Unlock()

	queue := g.sessions[sessionID]
	if queue == nil {
		queue = &sessionQueue{}
		g.sessions[sessionID] = queue
	}
	return queue
}

func (q *sessionQueue) do(ctx context.Context, fn func(context.Context) error) error {
	done := make(chan struct{})

	q.mu.Lock()
	prev := q.tail
	q.tail = done
	q.mu.Unlock()

	if prev != nil {
		select {
		case <-prev:
		case <-ctx.Done():
			go func() {
				<-prev
				close(done)
			}()
			return ctx.Err()
		}
	}

	defer close(done)
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(ctx)
}
