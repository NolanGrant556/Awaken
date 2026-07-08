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
	EventKindAgent            EventKind = "agent"
	EventKindRunError         EventKind = "run_error"
	EventKindRunCancelled     EventKind = "run_cancelled"
	EventKindRunStatusChanged EventKind = "run_status_changed"
)

type Run struct {
	RunID            string               `json:"run_id"`
	ThreadID         string               `json:"thread_id"`
	Input            string               `json:"input,omitempty"`
	InputAttachments []RunInputAttachment `json:"input_attachments,omitempty"`
	Status           RunStatus            `json:"status"`
	Result           string               `json:"result,omitempty"`
	Error            string               `json:"error,omitempty"`
	CancelRequested  bool                 `json:"cancel_requested,omitempty"`
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`
	FinishedAt       *time.Time           `json:"finished_at,omitempty"`
}

type Event struct {
	Kind           EventKind
	Seq            uint64 // monotonically increasing sequence number assigned by EventBus
	Timestamp      time.Time
	ThreadID       string
	RunID          string
	Status         string
	AgentEvent     agentcore.AwakenEvent
	Message        string
	Code           string
	TerminalReason agentcore.TerminalReason
}

type Session struct {
	mu                 sync.RWMutex
	run                Run
	cancel             context.CancelFunc
	bus                *EventBus
	massMessageReviews map[string]*pendingMassMessageReview
	userQuestions      map[string]*pendingUserQuestion
}

type pendingMassMessageReview struct {
	request  agentcore.AwakenEvent
	decision chan agentcore.ReviewDecision
}

type pendingUserQuestion struct {
	request agentcore.AwakenEvent
	answer  chan string
}

func NewSession(runID, threadID, input string, attachments []RunInputAttachment, cancel context.CancelFunc) *Session {
	now := time.Now().UTC()
	return &Session{
		run: Run{
			RunID:            runID,
			ThreadID:         threadID,
			Input:            strings.TrimSpace(input),
			InputAttachments: cloneRunInputAttachments(attachments),
			Status:           RunStatusRunning,
			CreatedAt:        now,
			UpdatedAt:        now,
		},
		cancel:             cancel,
		bus:                NewEventBus(),
		massMessageReviews: make(map[string]*pendingMassMessageReview),
		userQuestions:      make(map[string]*pendingUserQuestion),
	}
}

func (s *Session) Snapshot() Run {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.run
}

func (s *Session) Subscribe(afterSeq uint64) Subscription {
	sub := s.bus.Subscribe(afterSeq)
	sub.History = append([]Event(nil), sub.History...)
	return sub
}

func (s *Session) History(afterSeq uint64) []Event {
	sub := s.Subscribe(afterSeq)
	if sub.Close != nil {
		sub.Close()
	}
	return sub.History
}

func (s *Session) CancelRequested() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.run.CancelRequested
}

func (s *Session) RequestCancel() Run {
	now := time.Now().UTC()
	s.mu.Lock()
	if isTerminalStatus(s.run.Status) {
		snapshot := s.run
		s.mu.Unlock()
		return snapshot
	}

	s.run.CancelRequested = true
	statusChanged := false
	if s.run.Status == RunStatusRunning {
		s.run.Status = RunStatusCancelling
		statusChanged = true
	}
	s.run.UpdatedAt = now
	var statusEvent Event
	if statusChanged {
		statusEvent = Event{
			Kind:      EventKindRunStatusChanged,
			Timestamp: now,
			ThreadID:  s.run.ThreadID,
			RunID:     s.run.RunID,
			Status:    string(RunStatusCancelling),
		}
	}
	cancel := s.cancel
	snapshot := s.run
	bus := s.bus
	s.mu.Unlock()

	if statusChanged {
		bus.Append(statusEvent, false)
	}

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

func (s *Session) AwaitMassMessageReview(ctx context.Context, request agentcore.AwakenEvent) (agentcore.ReviewDecision, error) {
	timestamp := request.Timestamp.UTC()
	if request.Timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	request.Timestamp = timestamp
	request.Type = agentcore.MassMessageReviewRequestedType
	if strings.TrimSpace(request.ReviewID) == "" {
		request.ReviewID = "mass_review"
	}

	pending := &pendingMassMessageReview{
		request:  request,
		decision: make(chan agentcore.ReviewDecision, 1),
	}

	s.mu.Lock()
	if isTerminalStatus(s.run.Status) {
		s.mu.Unlock()
		return "", ErrRunTerminal
	}
	request.ThreadID = s.run.ThreadID
	request.RunID = s.run.RunID
	pending.request = request
	s.run.UpdatedAt = timestamp
	s.massMessageReviews[request.ReviewID] = pending
	wrapped := Event{
		Kind:       EventKindAgent,
		Timestamp:  timestamp,
		ThreadID:   s.run.ThreadID,
		RunID:      s.run.RunID,
		AgentEvent: request,
	}
	bus := s.bus
	s.mu.Unlock()

	bus.Append(wrapped, false)

	select {
	case decision := <-pending.decision:
		return decision, nil
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.massMessageReviews, request.ReviewID)
		s.mu.Unlock()
		return "", ctx.Err()
	}
}

func (s *Session) ResolveMassMessageReview(reviewID string, decision agentcore.ReviewDecision) error {
	reviewID = strings.TrimSpace(reviewID)
	if reviewID == "" {
		return ErrMassMessageReviewNotFound
	}
	if decision != agentcore.ReviewDecisionApproved && decision != agentcore.ReviewDecisionRejected {
		return ErrInvalidReviewDecision
	}

	now := time.Now().UTC()
	s.mu.Lock()
	if isTerminalStatus(s.run.Status) {
		s.mu.Unlock()
		return ErrRunTerminal
	}
	pending, ok := s.massMessageReviews[reviewID]
	if !ok {
		s.mu.Unlock()
		return ErrMassMessageReviewNotFound
	}
	delete(s.massMessageReviews, reviewID)
	resolved := pending.request
	resolved.Type = agentcore.MassMessageReviewResolvedType
	resolved.Timestamp = now
	resolved.ReviewDecision = decision
	resolved.ThreadID = s.run.ThreadID
	resolved.RunID = s.run.RunID
	s.run.UpdatedAt = now
	wrapped := Event{
		Kind:       EventKindAgent,
		Timestamp:  now,
		ThreadID:   s.run.ThreadID,
		RunID:      s.run.RunID,
		AgentEvent: resolved,
	}
	bus := s.bus
	s.mu.Unlock()

	bus.Append(wrapped, false)
	pending.decision <- decision
	return nil
}

func (s *Session) AwaitUserQuestion(ctx context.Context, request agentcore.AwakenEvent) (string, error) {
	timestamp := request.Timestamp.UTC()
	if request.Timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	request.Timestamp = timestamp
	request.Type = agentcore.UserQuestionRequestedType
	if strings.TrimSpace(request.QuestionID) == "" {
		request.QuestionID = "user_question"
	}

	pending := &pendingUserQuestion{
		request: request,
		answer:  make(chan string, 1),
	}

	s.mu.Lock()
	if isTerminalStatus(s.run.Status) {
		s.mu.Unlock()
		return "", ErrRunTerminal
	}
	request.ThreadID = s.run.ThreadID
	request.RunID = s.run.RunID
	pending.request = request
	s.run.UpdatedAt = timestamp
	s.userQuestions[request.QuestionID] = pending
	wrapped := Event{
		Kind:       EventKindAgent,
		Timestamp:  timestamp,
		ThreadID:   s.run.ThreadID,
		RunID:      s.run.RunID,
		AgentEvent: request,
	}
	bus := s.bus
	s.mu.Unlock()

	bus.Append(wrapped, false)

	select {
	case answer := <-pending.answer:
		return answer, nil
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.userQuestions, request.QuestionID)
		s.mu.Unlock()
		return "", ctx.Err()
	}
}

func (s *Session) ResolveUserQuestion(questionID string, answer string) error {
	questionID = strings.TrimSpace(questionID)
	if questionID == "" {
		return ErrUserQuestionNotFound
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return ErrUserQuestionNotFound
	}

	now := time.Now().UTC()
	s.mu.Lock()
	if isTerminalStatus(s.run.Status) {
		s.mu.Unlock()
		return ErrRunTerminal
	}
	pending, ok := s.userQuestions[questionID]
	if !ok {
		s.mu.Unlock()
		return ErrUserQuestionNotFound
	}
	delete(s.userQuestions, questionID)
	resolved := pending.request
	resolved.Type = agentcore.UserQuestionResolvedType
	resolved.Timestamp = now
	resolved.QuestionAnswer = answer
	resolved.ThreadID = s.run.ThreadID
	resolved.RunID = s.run.RunID
	s.run.UpdatedAt = now
	wrapped := Event{
		Kind:       EventKindAgent,
		Timestamp:  now,
		ThreadID:   s.run.ThreadID,
		RunID:      s.run.RunID,
		AgentEvent: resolved,
	}
	bus := s.bus
	s.mu.Unlock()

	bus.Append(wrapped, false)
	pending.answer <- answer
	return nil
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
	statusEvent := Event{
		Kind:           EventKindRunStatusChanged,
		Timestamp:      now,
		ThreadID:       s.run.ThreadID,
		RunID:          s.run.RunID,
		Status:         string(RunStatusCancelled),
		TerminalReason: agentcore.TerminalReasonCancelled,
	}
	event := Event{
		Kind:           EventKindRunCancelled,
		Timestamp:      now,
		ThreadID:       s.run.ThreadID,
		RunID:          s.run.RunID,
		Message:        message,
		TerminalReason: agentcore.TerminalReasonCancelled,
	}
	bus := s.bus
	s.mu.Unlock()

	bus.Append(statusEvent, false)
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
		Type:           agentcore.RunFinishedEventType,
		Timestamp:      now,
		ThreadID:       s.run.ThreadID,
		RunID:          s.run.RunID,
		Content:        result,
		TerminalReason: agentcore.TerminalReasonCompleted,
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
		Kind:           EventKindRunError,
		Timestamp:      now,
		ThreadID:       s.run.ThreadID,
		RunID:          s.run.RunID,
		Message:        message,
		Code:           code,
		TerminalReason: agentcore.TerminalReasonModelError,
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
