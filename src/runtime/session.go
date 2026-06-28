package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"awaken/src/agentcore"
	"awaken/src/protocol/a2ui"
)

type RunStatus string

const (
	RunStatusRunning          RunStatus = "running"
	RunStatusAwaitingApproval RunStatus = "awaiting_approval"
	RunStatusCancelling       RunStatus = "cancelling"
	RunStatusFinished         RunStatus = "finished"
	RunStatusError            RunStatus = "error"
	RunStatusCancelled        RunStatus = "cancelled"
)

type EventKind string

const (
	EventKindAgent            EventKind = "agent"
	EventKindRunError         EventKind = "run_error"
	EventKindRunCancelled     EventKind = "run_cancelled"
	EventKindRunStatusChanged EventKind = "run_status_changed"
)

type Run struct {
	RunID           string     `json:"run_id"`
	ThreadID        string     `json:"thread_id"`
	Input           string     `json:"input,omitempty"`
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
	Seq        uint64 // monotonically increasing sequence number assigned by EventBus
	Timestamp  time.Time
	ThreadID   string
	RunID      string
	Status     string
	AgentEvent agentcore.AwakenEvent
	Message    string
	Code       string
}

type Session struct {
	mu             sync.RWMutex
	run            Run
	cancel         context.CancelFunc
	bus            *EventBus
	approvals      map[string]*pendingApproval
	approvalPolicy agentcore.ApprovalPolicy
	after          func(time.Duration) <-chan time.Time
	observer       sessionObserver
}

type SessionConfig struct {
	ApprovalPolicy agentcore.ApprovalPolicy
	After          func(time.Duration) <-chan time.Time
	Observer       sessionObserver
}

type sessionObserver interface {
	OnRunUpdated(run Run)
	OnEvent(event Event)
}

func NewSession(runID, threadID, input string, cancel context.CancelFunc) *Session {
	return NewSessionWithConfig(runID, threadID, input, cancel, SessionConfig{})
}

func NewSessionWithConfig(runID, threadID, input string, cancel context.CancelFunc, config SessionConfig) *Session {
	now := time.Now().UTC()
	config = normalizeSessionConfig(config)
	return &Session{
		run: Run{
			RunID:     runID,
			ThreadID:  threadID,
			Input:     strings.TrimSpace(input),
			Status:    RunStatusRunning,
			CreatedAt: now,
			UpdatedAt: now,
		},
		cancel:         cancel,
		bus:            NewEventBus(),
		approvals:      make(map[string]*pendingApproval),
		approvalPolicy: config.ApprovalPolicy,
		after:          config.After,
		observer:       config.Observer,
	}
}

func NewSessionFromSnapshot(run Run, events []Event) *Session {
	return &Session{
		run:       run,
		bus:       NewEventBusWithHistory(events, isTerminalStatus(run.Status)),
		approvals: make(map[string]*pendingApproval),
		after:     time.After,
	}
}

func (s *Session) SetObserver(observer sessionObserver) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observer = observer
}

type pendingApproval struct {
	request  agentcore.ApprovalRequest
	decision chan agentcore.ApprovalDecision
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
	if s.run.Status == RunStatusRunning || s.run.Status == RunStatusAwaitingApproval {
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
	observer := s.observer
	s.mu.Unlock()

	if statusChanged {
		appended := bus.Append(statusEvent, false)
		notifySessionObserver(observer, snapshot, &appended)
	} else {
		notifySessionObserver(observer, snapshot, nil)
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
	snapshot := s.run
	observer := s.observer
	s.mu.Unlock()

	appended := bus.Append(wrapped, terminal)
	notifySessionObserver(observer, snapshot, &appended)
}

func (s *Session) AwaitApproval(ctx context.Context, request agentcore.ApprovalRequest) (agentcore.ApprovalDecision, error) {
	request = normalizeApprovalRequest(request)
	pending := &pendingApproval{
		request:  request,
		decision: make(chan agentcore.ApprovalDecision, 1),
	}

	now := time.Now().UTC()
	s.mu.Lock()
	if isTerminalStatus(s.run.Status) || s.run.Status == RunStatusCancelling {
		s.mu.Unlock()
		return "", context.Canceled
	}
	s.approvals[request.ID] = pending
	s.run.Status = RunStatusAwaitingApproval
	s.run.UpdatedAt = now
	statusEvent := Event{
		Kind:      EventKindRunStatusChanged,
		Timestamp: now,
		ThreadID:  s.run.ThreadID,
		RunID:     s.run.RunID,
		Status:    string(RunStatusAwaitingApproval),
	}
	bus := s.bus
	snapshot := s.run
	observer := s.observer
	s.mu.Unlock()

	appended := bus.Append(statusEvent, false)
	notifySessionObserver(observer, snapshot, &appended)

	s.RecordAgentEvent(agentcore.AwakenEvent{
		Type:                  agentcore.ApprovalRequestedEventType,
		ApprovalID:            request.ID,
		ToolCallID:            request.ToolCallID,
		ToolName:              request.ToolName,
		ApprovalTitle:         request.Title,
		ApprovalDescription:   request.Description,
		ApprovalRiskTone:      request.RiskTone,
		ApprovalAffectedPaths: request.AffectedPaths,
		ApprovalPatchSummary:  request.PatchSummary,
	})
	s.RecordAgentEvent(approvalSurfaceArtifactEvent(request))

	timeoutCh := s.approvalTimeout()
	for {
		select {
		case decision := <-pending.decision:
			return decision, nil
		case <-timeoutCh:
			resolved, err := s.resolveApproval("approval_timeout", request.ID, agentcore.ApprovalDecisionRejected)
			if err == nil && resolved {
				return agentcore.ApprovalDecisionRejected, nil
			}
			timeoutCh = nil
		case <-ctx.Done():
			s.mu.Lock()
			delete(s.approvals, request.ID)
			s.mu.Unlock()
			return "", ctx.Err()
		}
	}
}

func approvalSurfaceArtifactEvent(request agentcore.ApprovalRequest) agentcore.AwakenEvent {
	content, err := a2ui.ApprovalSurfaceJSON(a2ui.ApprovalSurfaceRequest{
		ApprovalID:    request.ID,
		Title:         request.Title,
		Description:   request.Description,
		RiskTone:      string(request.RiskTone),
		ToolName:      request.ToolName,
		ToolCallID:    request.ToolCallID,
		AffectedPaths: request.AffectedPaths,
		PatchSummary:  request.PatchSummary,
	})
	if err != nil {
		content = ""
	}

	return agentcore.AwakenEvent{
		Type:                agentcore.ArtifactCreatedEventType,
		ApprovalID:          request.ID,
		ToolCallID:          request.ToolCallID,
		ToolName:            request.ToolName,
		ArtifactID:          fmt.Sprintf("%s-a2ui-surface", request.ID),
		ArtifactTitle:       "Generated approval surface",
		ArtifactDescription: "Phase 5 read-only A2UI evaluation payload generated from the approval request.",
		ArtifactKind:        a2ui.ArtifactKind,
		ArtifactContent:     content,
		ArtifactStatus:      agentcore.ArtifactStatusReady,
	}
}

func (s *Session) ResolveApproval(approvalID string, decision agentcore.ApprovalDecision) error {
	_, err := s.resolveApproval("", approvalID, decision)
	return err
}

func (s *Session) resolveApproval(code string, approvalID string, decision agentcore.ApprovalDecision) (bool, error) {
	approvalID = strings.TrimSpace(approvalID)
	if approvalID == "" {
		return false, ErrApprovalNotFound
	}

	now := time.Now().UTC()
	s.mu.Lock()
	pending, ok := s.approvals[approvalID]
	if !ok {
		s.mu.Unlock()
		return false, ErrApprovalNotFound
	}

	switch {
	case isTerminalStatus(s.run.Status):
		s.mu.Unlock()
		return false, ErrRunTerminal
	case s.run.Status == RunStatusCancelling:
		s.mu.Unlock()
		return false, ErrRunCancelling
	}

	delete(s.approvals, approvalID)
	request := pending.request

	s.run.Status = RunStatusRunning
	s.run.UpdatedAt = now
	statusEvent := Event{
		Kind:      EventKindRunStatusChanged,
		Timestamp: now,
		ThreadID:  s.run.ThreadID,
		RunID:     s.run.RunID,
		Status:    string(RunStatusRunning),
	}
	bus := s.bus
	snapshot := s.run
	observer := s.observer
	s.mu.Unlock()

	appended := bus.Append(statusEvent, false)
	notifySessionObserver(observer, snapshot, &appended)

	s.RecordAgentEvent(agentcore.AwakenEvent{
		Type:                agentcore.ApprovalResolvedEventType,
		ApprovalID:          approvalID,
		ToolCallID:          request.ToolCallID,
		ToolName:            request.ToolName,
		ApprovalTitle:       request.Title,
		ApprovalDescription: request.Description,
		ApprovalRiskTone:    request.RiskTone,
		ApprovalDecision:    decision,
		Code:                code,
	})

	pending.decision <- decision
	close(pending.decision)
	return true, nil
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
		Kind:      EventKindRunStatusChanged,
		Timestamp: now,
		ThreadID:  s.run.ThreadID,
		RunID:     s.run.RunID,
		Status:    string(RunStatusCancelled),
	}
	event := Event{
		Kind:      EventKindRunCancelled,
		Timestamp: now,
		ThreadID:  s.run.ThreadID,
		RunID:     s.run.RunID,
		Message:   message,
	}
	bus := s.bus
	snapshot := s.run
	observer := s.observer
	s.mu.Unlock()

	appendedStatus := bus.Append(statusEvent, false)
	notifySessionObserver(observer, snapshot, &appendedStatus)
	appendedEvent := bus.Append(event, true)
	notifySessionObserver(observer, snapshot, &appendedEvent)
}

func (s *Session) approvalTimeout() <-chan time.Time {
	timeout := s.approvalPolicy.Timeout()
	if timeout <= 0 {
		return nil
	}
	return s.after(timeout)
}

func normalizeSessionConfig(config SessionConfig) SessionConfig {
	config.ApprovalPolicy = config.ApprovalPolicy.Normalize()
	if config.After == nil {
		config.After = time.After
	}
	return config
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
	snapshot := s.run
	observer := s.observer
	s.mu.Unlock()

	appended := bus.Append(wrapped, true)
	notifySessionObserver(observer, snapshot, &appended)
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
	snapshot := s.run
	observer := s.observer
	s.mu.Unlock()

	appended := bus.Append(event, true)
	notifySessionObserver(observer, snapshot, &appended)
}

func normalizeApprovalRequest(request agentcore.ApprovalRequest) agentcore.ApprovalRequest {
	request.ID = strings.TrimSpace(request.ID)
	request.ToolCallID = strings.TrimSpace(request.ToolCallID)
	request.ToolName = strings.TrimSpace(request.ToolName)
	request.Title = strings.TrimSpace(request.Title)
	request.Description = strings.TrimSpace(request.Description)
	request.PatchSummary = strings.TrimSpace(request.PatchSummary)
	request.AffectedPaths = trimStrings(request.AffectedPaths)

	if request.ID == "" {
		request.ID = newRuntimeID("approval")
	}
	if request.Title == "" {
		if request.ToolName != "" {
			request.Title = fmt.Sprintf("Approve %s", request.ToolName)
		} else {
			request.Title = "Approval required"
		}
	}
	if request.RiskTone == "" {
		request.RiskTone = agentcore.ApprovalRiskWarning
	}

	return request
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

func trimStrings(values []string) []string {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			trimmed = append(trimmed, strings.TrimSpace(value))
		}
	}
	return trimmed
}

func notifySessionObserver(observer sessionObserver, run Run, event *Event) {
	if observer == nil {
		return
	}
	observer.OnRunUpdated(run)
	if event != nil {
		observer.OnEvent(*event)
	}
}
