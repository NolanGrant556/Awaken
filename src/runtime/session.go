package runtime

import (
	"context"
	"fmt"
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
	mu                         sync.RWMutex
	run                        Run
	cancel                     context.CancelFunc
	bus                        *EventBus
	semanticRecorder           agentcore.SemanticRecorder
	massMessageReviews         map[string]*pendingMassMessageReview
	resolvedMassMessageReviews map[string]agentcore.ReviewDecision
	userQuestions              map[string]*pendingUserQuestion
}

type pendingMassMessageReview struct {
	request   agentcore.AwakenEvent
	decision  chan agentcore.ReviewDecision
	resolveMu sync.Mutex
}

// MassMessageReviewDigests identifies the exact canonical send envelope the
// user saw. Resolve callers must echo all three values from the request event.
type MassMessageReviewDigests struct {
	EnvelopeDigest          string
	BodyDigest              string
	RecipientIdentityDigest string
}

type pendingUserQuestion struct {
	request agentcore.AwakenEvent
	answer  chan string
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
		cancel:                     cancel,
		bus:                        NewEventBus(),
		massMessageReviews:         make(map[string]*pendingMassMessageReview),
		resolvedMassMessageReviews: make(map[string]agentcore.ReviewDecision),
		userQuestions:              make(map[string]*pendingUserQuestion),
	}
}

func (s *Session) SetSemanticRecorder(recorder agentcore.SemanticRecorder) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.semanticRecorder = recorder
	s.mu.Unlock()
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
	if !validMassMessageReviewDigests(massMessageReviewDigestsFromEvent(request)) {
		return "", ErrMassMessageReviewDigestMissing
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
	if _, exists := s.massMessageReviews[request.ReviewID]; exists {
		s.mu.Unlock()
		return "", ErrMassMessageReviewAlreadyPending
	}
	if _, resolved := s.resolvedMassMessageReviews[request.ReviewID]; resolved {
		s.mu.Unlock()
		return "", ErrMassMessageReviewAlreadyResolved
	}
	request.ThreadID = s.run.ThreadID
	request.RunID = s.run.RunID
	pending.request = request
	s.run.UpdatedAt = timestamp
	s.massMessageReviews[request.ReviewID] = pending
	recorder := s.semanticRecorder
	wrapped := Event{
		Kind:       EventKindAgent,
		Timestamp:  timestamp,
		ThreadID:   s.run.ThreadID,
		RunID:      s.run.RunID,
		AgentEvent: request,
	}
	bus := s.bus
	s.mu.Unlock()

	if recorder != nil {
		record := agentcore.NewSemanticRecord(
			agentcore.SemanticKindMassMessageReviewRequested,
			request.ThreadID,
			request.RunID,
			request.TurnID,
			request.ToolCallID,
			"",
			massMessageReviewPayload(request),
		)
		if err := recorder.Append(ctx, record); err != nil {
			s.mu.Lock()
			delete(s.massMessageReviews, request.ReviewID)
			s.mu.Unlock()
			return "", fmt.Errorf("persist mass-message review request: %w", err)
		}
		if _, canonical := recorder.(agentcore.CanonicalLiveSemanticRecorder); !canonical {
			bus.Append(wrapped, false)
		}
	} else {
		bus.Append(wrapped, false)
	}

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

func (s *Session) ResolveMassMessageReview(reviewID string, decision agentcore.ReviewDecision, digests MassMessageReviewDigests) error {
	reviewID = strings.TrimSpace(reviewID)
	if reviewID == "" {
		return ErrMassMessageReviewNotFound
	}
	if decision != agentcore.ReviewDecisionApproved && decision != agentcore.ReviewDecisionRejected {
		return ErrInvalidReviewDecision
	}
	digests = normalizeMassMessageReviewDigests(digests)
	if !validMassMessageReviewDigests(digests) {
		return ErrMassMessageReviewDigestMissing
	}

	now := time.Now().UTC()
	s.mu.Lock()
	if isTerminalStatus(s.run.Status) {
		s.mu.Unlock()
		return ErrRunTerminal
	}
	pending, ok := s.massMessageReviews[reviewID]
	if !ok {
		if _, resolved := s.resolvedMassMessageReviews[reviewID]; resolved {
			s.mu.Unlock()
			return ErrMassMessageReviewAlreadyResolved
		}
		s.mu.Unlock()
		return ErrMassMessageReviewNotFound
	}
	if massMessageReviewDigestsFromEvent(pending.request) != digests {
		s.mu.Unlock()
		return ErrMassMessageReviewDigestMismatch
	}
	s.mu.Unlock()

	// Persisting a resolution can re-enter Session through a live recorder, so
	// it cannot happen while s.mu is held. Serialize per-review resolution and
	// re-check state after taking that lock: exactly one caller may persist and
	// release the waiter; every later resolution is rejected as a replay.
	pending.resolveMu.Lock()
	defer pending.resolveMu.Unlock()
	s.mu.Lock()
	if isTerminalStatus(s.run.Status) {
		s.mu.Unlock()
		return ErrRunTerminal
	}
	current, stillPending := s.massMessageReviews[reviewID]
	if !stillPending || current != pending {
		if _, resolved := s.resolvedMassMessageReviews[reviewID]; resolved {
			s.mu.Unlock()
			return ErrMassMessageReviewAlreadyResolved
		}
		s.mu.Unlock()
		return ErrMassMessageReviewNotFound
	}
	if massMessageReviewDigestsFromEvent(pending.request) != digests {
		s.mu.Unlock()
		return ErrMassMessageReviewDigestMismatch
	}
	resolved := pending.request
	resolved.Type = agentcore.MassMessageReviewResolvedType
	resolved.Timestamp = now
	resolved.ReviewDecision = decision
	resolved.ThreadID = s.run.ThreadID
	resolved.RunID = s.run.RunID
	s.run.UpdatedAt = now
	recorder := s.semanticRecorder
	wrapped := Event{
		Kind:       EventKindAgent,
		Timestamp:  now,
		ThreadID:   s.run.ThreadID,
		RunID:      s.run.RunID,
		AgentEvent: resolved,
	}
	s.mu.Unlock()

	if recorder != nil {
		record := agentcore.NewSemanticRecord(
			agentcore.SemanticKindMassMessageReviewResolved,
			resolved.ThreadID,
			resolved.RunID,
			resolved.TurnID,
			resolved.ToolCallID,
			"",
			massMessageReviewPayload(resolved),
		)
		if err := recorder.Append(context.Background(), record); err != nil {
			return fmt.Errorf("persist mass-message review resolution: %w", err)
		}
	}
	s.mu.Lock()
	delete(s.massMessageReviews, reviewID)
	s.resolvedMassMessageReviews[reviewID] = decision
	bus := s.bus
	s.mu.Unlock()
	if recorder == nil {
		bus.Append(wrapped, false)
	} else if _, canonical := recorder.(agentcore.CanonicalLiveSemanticRecorder); !canonical {
		bus.Append(wrapped, false)
	}
	pending.decision <- decision
	return nil
}

func massMessageReviewPayload(event agentcore.AwakenEvent) map[string]any {
	payload := map[string]any{
		"review_id":                 event.ReviewID,
		"decision":                  string(event.ReviewDecision),
		"review_decision":           string(event.ReviewDecision),
		"tool_call_id":              event.ToolCallID,
		"tool_name":                 event.ToolName,
		"channel":                   event.MassMessageChannel,
		"recipient_scope":           event.MassMessageRecipientScope,
		"recipient_count":           event.MassMessageRecipientCount,
		"message_count":             event.MassMessageMessageCount,
		"expands_recipients":        event.MassMessageExpandsRecipients,
		"body_preview":              event.MassMessageBodyPreview,
		"template_preview":          event.MassMessageTemplatePreview,
		"attachments":               append([]string(nil), event.MassMessageAttachments...),
		"sending_account":           event.MassMessageSendingAccount,
		"source_tool":               event.MassMessageSourceTool,
		"envelope_digest":           event.ReviewEnvelopeDigest,
		"body_digest":               event.ReviewBodyDigest,
		"recipient_identity_digest": event.ReviewRecipientIdentityDigest,
	}
	return payload
}

func massMessageReviewDigestsFromEvent(event agentcore.AwakenEvent) MassMessageReviewDigests {
	return normalizeMassMessageReviewDigests(MassMessageReviewDigests{
		EnvelopeDigest:          event.ReviewEnvelopeDigest,
		BodyDigest:              event.ReviewBodyDigest,
		RecipientIdentityDigest: event.ReviewRecipientIdentityDigest,
	})
}

func normalizeMassMessageReviewDigests(digests MassMessageReviewDigests) MassMessageReviewDigests {
	digests.EnvelopeDigest = strings.TrimSpace(digests.EnvelopeDigest)
	digests.BodyDigest = strings.TrimSpace(digests.BodyDigest)
	digests.RecipientIdentityDigest = strings.TrimSpace(digests.RecipientIdentityDigest)
	return digests
}

func validMassMessageReviewDigests(digests MassMessageReviewDigests) bool {
	digests = normalizeMassMessageReviewDigests(digests)
	return digests.EnvelopeDigest != "" && digests.BodyDigest != "" && digests.RecipientIdentityDigest != ""
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
