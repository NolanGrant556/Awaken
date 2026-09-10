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

func (s *RunService) StopThreadAgents(ctx context.Context, threadID string) error {
	s.mu.RLock()
	thread := s.threads[threadID]
	if thread == nil {
		s.mu.RUnlock()
		return ErrThreadNotFound
	}
	manager := thread.workGroup
	session := s.sessions[thread.activeRunID]
	s.mu.RUnlock()
	if session != nil {
		session.RequestCancel()
	}
	if manager != nil {
		return manager.StopAll(ctx)
	}
	return nil
}
