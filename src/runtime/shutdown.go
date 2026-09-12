package runtime

import (
	"awaken/src/runcontrol"
	"context"
	"errors"
)

var ErrRuntimeShuttingDown = errors.New("runtime is shutting down")

// BeginShutdown fences admission synchronously. Cancellation is non-blocking;
// Drain joins both accepted starts and the trailing writes of launched runs.
func (s *RunService) BeginShutdown() {
	s.mu.Lock()
	if s.shuttingDown {
		s.mu.Unlock()
		return
	}
	s.shuttingDown = true
	sessions := make([]*Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	s.mu.Unlock()
	for _, session := range sessions {
		session.RequestCancelWithSource(runcontrol.Shutdown)
	}
}

func (s *RunService) Drain(ctx context.Context) error {
	done := make(chan struct{})
	go func() { s.startWG.Wait(); s.runWG.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
