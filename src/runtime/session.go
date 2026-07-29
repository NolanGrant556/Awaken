package runtime

import (
	"context"
	"strings"
	"sync"
	"time"

	"awaken/src/agentcore"
	"awaken/src/subagentruntime"
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
	EventKindSemantic         EventKind = "semantic"
	EventKindRunError         EventKind = "run_error"
	EventKindRunCancelled     EventKind = "run_cancelled"
	EventKindRunStatusChanged EventKind = "run_status_changed"
)

type Run struct {
	RunID            string               `json:"run_id"`
	ThreadID         string               `json:"thread_id"`
	Input            string               `json:"input,omitempty"`
	InputAttachments []RunInputAttachment `json:"input_attachments,omitempty"`
	ComposerTokens   []ComposerToken      `json:"composer_tokens,omitempty"`
	Status           RunStatus            `json:"status"`
	Result           string               `json:"result,omitempty"`
	Error            string               `json:"error,omitempty"`
	CancelRequested  bool                 `json:"cancel_requested,omitempty"`
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`
	FinishedAt       *time.Time           `json:"finished_at,omitempty"`
	ModelProfileID   string               `json:"model_profile_id,omitempty"`
	ReasoningEffort  string               `json:"reasoning_effort,omitempty"`
}

type Event struct {
	Kind           EventKind
	Seq            uint64 // monotonically increasing sequence number assigned by EventBus
	Timestamp      time.Time
	ThreadID       string
	RunID          string
	Status         string
	AgentEvent     agentcore.AwakenEvent
	SemanticRecord agentcore.SemanticRecord
	Message        string
	Content        string
	Code           string
	TerminalReason agentcore.TerminalReason
}

type Session struct {
	mu                   sync.RWMutex
	run                  Run
	cancel               context.CancelFunc
	bus                  *EventBus
	semanticRecorder     agentcore.SemanticRecorder
	subagents            subagentruntime.Controller
	userQuestions        map[string]*pendingUserQuestion
	externalInteractions map[string]*pendingExternalInteraction
}

func (s *Session) SetSubagentController(controller subagentruntime.Controller) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.subagents = controller
	s.mu.Unlock()
}

func (s *Session) SubagentController() subagentruntime.Controller {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.subagents
}

type pendingUserQuestion struct {
	request agentcore.AwakenEvent
	answer  chan string
}

type pendingExternalInteraction struct {
	request agentcore.AwakenEvent
	answer  chan agentcore.ExternalInteractionResolution
}

func NewSession(runID, threadID, input string, attachments []RunInputAttachment, composerTokens []ComposerToken, cancel context.CancelFunc) *Session {
	now := time.Now().UTC()
	return &Session{
		run: Run{
			RunID:            runID,
			ThreadID:         threadID,
			Input:            strings.TrimSpace(input),
			InputAttachments: cloneRunInputAttachments(attachments),
			ComposerTokens:   cloneComposerTokens(composerTokens),
			Status:           RunStatusRunning,
			CreatedAt:        now,
			UpdatedAt:        now,
		},
		cancel:               cancel,
		bus:                  NewEventBus(),
		userQuestions:        make(map[string]*pendingUserQuestion),
		externalInteractions: make(map[string]*pendingExternalInteraction),
	}
}

func (s *Session) SetModelSelection(profileID, reasoningEffort string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.run.ModelProfileID = strings.TrimSpace(profileID)
	s.run.ReasoningEffort = strings.TrimSpace(reasoningEffort)
	s.mu.Unlock()
}

func (s *Session) SetSemanticRecorder(recorder agentcore.SemanticRecorder) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.semanticRecorder = recorder
	s.mu.Unlock()
}

func (s *Session) semanticRecorderSnapshot() agentcore.SemanticRecorder {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.semanticRecorder
}

func cloneComposerTokens(tokens []ComposerToken) []ComposerToken {
	if len(tokens) == 0 {
		return nil
	}
	return append([]ComposerToken(nil), tokens...)
}

func (s *Session) Snapshot() Run {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot := s.run
	snapshot.InputAttachments = cloneRunInputAttachments(snapshot.InputAttachments)
	snapshot.ComposerTokens = cloneComposerTokens(snapshot.ComposerTokens)
	return snapshot
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
	case agentcore.TextDeltaEventType:
		s.run.Result += event.Delta
	case agentcore.RunFinishedEventType:
		s.run.Status = RunStatusFinished
		if event.Content != "" {
			s.run.Result = event.Content
		}
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

// RecordSemanticRecords publishes already-assigned durable facts into the live
// EventBus. Session state and the stream therefore advance from the same facts
// that restart replay reads from the journal.
func (s *Session) RecordSemanticRecords(records []agentcore.SemanticRecord) {
	for _, record := range records {
		if record.RunID != "" && record.RunID != s.Snapshot().RunID {
			continue
		}
		timestamp := record.Timestamp.UTC()
		if timestamp.IsZero() {
			timestamp = time.Now().UTC()
		}
		record.Timestamp = timestamp

		s.mu.Lock()
		if isTerminalStatus(s.run.Status) {
			s.mu.Unlock()
			return
		}
		if record.ThreadID == "" {
			record.ThreadID = s.run.ThreadID
		}
		if record.RunID == "" {
			record.RunID = s.run.RunID
		}
		s.run.UpdatedAt = timestamp
		terminal := false
		terminalReason := agentcore.TerminalReason("")
		switch record.Kind {
		case agentcore.SemanticKindRunStarted:
			if !s.run.CancelRequested {
				s.run.Status = RunStatusRunning
			}
		case agentcore.SemanticKindRunTerminal:
			status := stringPayload(record.Payload, "status")
			terminalReason = agentcore.TerminalReason(firstNonEmpty(
				stringPayload(record.Payload, "terminal_reason"),
				stringPayload(record.Payload, "reason"),
			))
			switch status {
			case "finished":
				s.run.Status = RunStatusFinished
				s.run.Result = stringPayload(record.Payload, "result")
			case "cancelled":
				s.run.Status = RunStatusCancelled
				s.run.CancelRequested = true
				s.run.Error = firstNonEmpty(stringPayload(record.Payload, "error"), "run cancelled")
				s.run.Result = firstNonEmpty(stringPayload(record.Payload, "result"), s.run.Result)
			default:
				s.run.Status = RunStatusError
				s.run.Error = firstNonEmpty(stringPayload(record.Payload, "error"), "run failed")
			}
			finishedAt := timestamp
			s.run.FinishedAt = &finishedAt
			terminal = true
		}
		wrapped := Event{
			Kind:           EventKindSemantic,
			Timestamp:      timestamp,
			ThreadID:       s.run.ThreadID,
			RunID:          s.run.RunID,
			SemanticRecord: record,
			TerminalReason: terminalReason,
		}
		bus := s.bus
		s.mu.Unlock()

		bus.Append(wrapped, terminal)
	}
}

func (s *Session) StreamCursor() uint64 {
	if s == nil || s.bus == nil {
		return 0
	}
	return s.bus.StreamCursor()
}

func (s *Session) StreamSnapshot() ([]Event, uint64) {
	if s == nil || s.bus == nil {
		return nil, 0
	}
	return s.bus.Snapshot()
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

func (s *Session) AwaitExternalInteraction(ctx context.Context, request agentcore.AwakenEvent) (agentcore.ExternalInteractionResolution, error) {
	timestamp := request.Timestamp.UTC()
	if request.Timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	request.Timestamp = timestamp
	request.Type = agentcore.ExternalInteractionRequestedType
	if strings.TrimSpace(request.InteractionID) == "" {
		request.InteractionID = "external_interaction"
	}

	pending := &pendingExternalInteraction{
		request: request,
		answer:  make(chan agentcore.ExternalInteractionResolution, 1),
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
	s.externalInteractions[request.InteractionID] = pending
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
	case resolution := <-pending.answer:
		return resolution, nil
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.externalInteractions, request.InteractionID)
		s.mu.Unlock()
		return "", ctx.Err()
	}
}

func (s *Session) ResolveExternalInteraction(interactionID string, resolution agentcore.ExternalInteractionResolution) error {
	interactionID = strings.TrimSpace(interactionID)
	if interactionID == "" {
		return ErrExternalInteractionNotFound
	}
	switch resolution {
	case agentcore.ExternalInteractionCompleted,
		agentcore.ExternalInteractionCancelled,
		agentcore.ExternalInteractionExpired,
		agentcore.ExternalInteractionFailed:
	default:
		return ErrExternalInteractionNotFound
	}

	now := time.Now().UTC()
	s.mu.Lock()
	if isTerminalStatus(s.run.Status) {
		s.mu.Unlock()
		return ErrRunTerminal
	}
	pending, ok := s.externalInteractions[interactionID]
	if !ok {
		s.mu.Unlock()
		return ErrExternalInteractionNotFound
	}
	delete(s.externalInteractions, interactionID)
	resolved := pending.request
	resolved.Type = agentcore.ExternalInteractionResolvedType
	resolved.Timestamp = now
	resolved.InteractionResolution = string(resolution)
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
	pending.answer <- resolution
	return nil
}

func (s *Session) CompleteCancelled(message, result string) {
	now := time.Now().UTC()
	if strings.TrimSpace(message) == "" {
		message = "你停止了生成本次回答"
	}

	s.mu.Lock()
	if isTerminalStatus(s.run.Status) {
		s.mu.Unlock()
		return
	}

	if strings.TrimSpace(s.run.Result) != "" {
		result = strings.TrimSpace(s.run.Result)
	} else {
		result = strings.TrimSpace(result)
	}

	s.run.Status = RunStatusCancelled
	s.run.Error = message
	s.run.Result = result
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
		Message:        message,
		Content:        result,
		Code:           "RUN_CANCELLED",
		TerminalReason: agentcore.TerminalReasonCancelled,
	}
	event := Event{
		Kind:           EventKindRunCancelled,
		Timestamp:      now,
		ThreadID:       s.run.ThreadID,
		RunID:          s.run.RunID,
		Message:        message,
		Content:        result,
		Code:           "RUN_CANCELLED",
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
