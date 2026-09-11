package runtime

import (
	"context"
	"errors"
)

var ErrActivityCursorExpired = errors.New("task activity cursor expired; reload the canonical transcript")

func (s *RunService) activityBusLocked(threadID string) *EventBus {
	if s.activityBuses == nil {
		s.activityBuses = make(map[string]*EventBus)
	}
	bus := s.activityBuses[threadID]
	if bus == nil {
		bus = NewActivityBus()
		s.activityBuses[threadID] = bus
	}
	return bus
}

func (s *RunService) SubscribeThreadActivity(threadID string, afterSeq uint64) (Subscription, uint64, error) {
	if _, err := s.GetThreadTranscript(threadID); err != nil {
		return Subscription{}, 0, err
	}
	s.mu.Lock()
	bus := s.activityBusLocked(threadID)
	s.mu.Unlock()
	return bus.SubscribeRetained(afterSeq)
}

// StopThreadAgents cancels every queued or running Subagent assignment in
// the thread work group. It does not cancel the root run or fence later
// child creation; the parent Agent stays running and receives a host
// notice that the user stopped the children. Composer stop-run still uses
// CancelRun.
func (s *RunService) StopThreadAgents(ctx context.Context, threadID string) error {
	s.mu.RLock()
	thread := s.threads[threadID]
	if thread == nil {
		s.mu.RUnlock()
		return ErrThreadNotFound
	}
	manager := thread.workGroup
	s.mu.RUnlock()
	if manager == nil {
		return nil
	}
	return manager.CancelActiveAssignments(ctx, "user_stopped_all")
}
