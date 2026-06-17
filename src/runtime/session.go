package runtime

import (
	"context"
	"strings"
	"sync"
	"time"

	"awaken/src/agentcore"
)

type RunStatus string

const (
	RunStatusRunning    RunStatus = "running"
	RunStatusCancelling RunStatus = "cancelling"
	RunStatusFinished   RunStatus = "finished"
	RunStatusError      RunStatus = "error"
	RunStatusCancelled  RunStatus = "cancelled"
)

type EventKind string

const (
	EventKindAgent        EventKind = "agent"
	EventKindRunError     EventKind = "run_error"
	EventKindRunCancelled EventKind = "run_cancelled"
)

type Run struct {
	RunID           string     `json:"run_id"`
	ThreadID        string     `json:"thread_id"`
	Status          RunStatus  `json:"status"`
	Result          string     `json:"result,omitempty"`
	Error           string     `json:"error,omitempty"`
	CancelRequested bool       `json:"cancel_requested,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
}

type Event struct {
	Kind       EventKind
	Timestamp  time.Time
	ThreadID   string
	RunID      string
	AgentEvent agentcore.AwakenEvent
	Message    string
	Code       string
}

type Session struct {
	mu     sync.RWMutex
	run    Run
	cancel context.CancelFunc
	bus    *EventBus
}

func NewSession(runID, threadID string, cancel context.CancelFunc) *Session {
	now := time.Now().UTC()
	return &Session{
		run: Run{
			RunID:     runID,
			ThreadID:  threadID,
			Status:    RunStatusRunning,
			CreatedAt: now,
			UpdatedAt: now,
		},
		cancel: cancel,
		bus:    NewEventBus(),
	}
}

func (s *Session) Snapshot() Run {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.run
}

func (s *Session) Subscribe() Subscription {
	sub := s.bus.Subscribe()
	sub.History = append([]Event(nil), sub.History...)
	return sub
}

func (s *Session) CancelRequested() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.run.CancelRequested
}

func (s *Session) RequestCancel() Run {
	s.mu.Lock()
	if isTerminalStatus(s.run.Status) {
		snapshot := s.run
		s.mu.Unlock()
		return snapshot
	}

	s.run.CancelRequested = true
	if s.run.Status == RunStatusRunning {
		s.run.Status = RunStatusCancelling
	}
	s.run.UpdatedAt = time.Now().UTC()
	cancel := s.cancel
	snapshot := s.run
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	return snapshot
}

func (s *Session) RecordAgentEvent(event agentcore.AwakenEvent) {
	timestamp := event.Timestamp.UTC()
	if event.Timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	event.Timestamp = timestamp

	s.mu.Lock()
	if isTerminalStatus(s.run.Status) {
		s.mu.Unlock()
		return
	}

	event.ThreadID = s.run.ThreadID
	event.RunID = s.run.RunID
	s.run.UpdatedAt = timestamp

	terminal := false
	switch event.Type {
	case agentcore.RunStartedEventType:
		if !s.run.CancelRequested {
			s.run.Status = RunStatusRunning
		}
	case agentcore.RunFinishedEventType:
		s.run.Status = RunStatusFinished
		s.run.Result = event.Content
		finishedAt := timestamp
		s.run.FinishedAt = &finishedAt
		terminal = true
	case agentcore.RunErrorEventType:
		s.run.Status = RunStatusError
		s.run.Error = firstNonEmpty(strings.TrimSpace(event.Error), "run failed")
		finishedAt := timestamp
		s.run.FinishedAt = &finishedAt
		terminal = true
	}

	wrapped := Event{
		Kind:       EventKindAgent,
		Timestamp:  timestamp,
		ThreadID:   s.run.ThreadID,
		RunID:      s.run.RunID,
		AgentEvent: event,
	}
	bus := s.bus
	s.mu.Unlock()

	bus.Append(wrapped, terminal)
}

func (s *Session) CompleteCancelled(message string) {
	now := time.Now().UTC()
	if strings.TrimSpace(message) == "" {
		message = "run cancelled"
	}

	s.mu.Lock()
	if isTerminalStatus(s.run.Status) {
		s.mu.Unlock()
		return
	}

	s.run.Status = RunStatusCancelled
	s.run.Error = message
	s.run.CancelRequested = true
	s.run.UpdatedAt = now
	finishedAt := now
	s.run.FinishedAt = &finishedAt
	event := Event{
		Kind:      EventKindRunCancelled,
		Timestamp: now,
		ThreadID:  s.run.ThreadID,
		RunID:     s.run.RunID,
		Message:   message,
	}
	bus := s.bus
	s.mu.Unlock()

	bus.Append(event, true)
}

func (s *Session) CompleteFinished(result string) {
	now := time.Now().UTC()

	s.mu.Lock()
	if isTerminalStatus(s.run.Status) {
		s.mu.Unlock()
		return
	}

	s.run.Status = RunStatusFinished
	s.run.Result = result
	s.run.UpdatedAt = now
	finishedAt := now
	s.run.FinishedAt = &finishedAt
	event := agentcore.AwakenEvent{
		Type:      agentcore.RunFinishedEventType,
		Timestamp: now,
		ThreadID:  s.run.ThreadID,
		RunID:     s.run.RunID,
		Content:   result,
	}
	wrapped := Event{
		Kind:       EventKindAgent,
		Timestamp:  now,
		ThreadID:   s.run.ThreadID,
		RunID:      s.run.RunID,
		AgentEvent: event,
	}
	bus := s.bus
	s.mu.Unlock()

	bus.Append(wrapped, true)
}

func (s *Session) CompleteError(message, code string) {
	now := time.Now().UTC()
	if strings.TrimSpace(message) == "" {
		message = "run failed"
	}

	s.mu.Lock()
	if isTerminalStatus(s.run.Status) {
		s.mu.Unlock()
		return
	}

	s.run.Status = RunStatusError
	s.run.Error = message
	s.run.UpdatedAt = now
	finishedAt := now
	s.run.FinishedAt = &finishedAt
	event := Event{
		Kind:      EventKindRunError,
		Timestamp: now,
		ThreadID:  s.run.ThreadID,
		RunID:     s.run.RunID,
		Message:   message,
		Code:      code,
	}
	bus := s.bus
	s.mu.Unlock()

	bus.Append(event, true)
}

func isTerminalStatus(status RunStatus) bool {
	switch status {
	case RunStatusFinished, RunStatusError, RunStatusCancelled:
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
