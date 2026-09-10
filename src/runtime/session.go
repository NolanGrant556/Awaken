package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"awaken/src/agentcore"
	"awaken/src/computer"
	"awaken/src/instructions"
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
	RunID               string                `json:"run_id"`
	ThreadID            string                `json:"thread_id"`
	Input               string                `json:"input,omitempty"`
	InputAttachments    []RunInputAttachment  `json:"input_attachments,omitempty"`
	ComposerTokens      []ComposerToken       `json:"composer_tokens,omitempty"`
	FileComments        []FileLineComment     `json:"file_comments,omitempty"`
	Status              RunStatus             `json:"status"`
	Result              string                `json:"result,omitempty"`
	Error               string                `json:"error,omitempty"`
	CancelRequested     bool                  `json:"cancel_requested,omitempty"`
	CreatedAt           time.Time             `json:"created_at"`
	UpdatedAt           time.Time             `json:"updated_at"`
	FinishedAt          *time.Time            `json:"finished_at,omitempty"`
	ModelProfileID      string                `json:"model_profile_id,omitempty"`
	ReasoningEffort     string                `json:"reasoning_effort,omitempty"`
	HookSnapshotID      string                `json:"hook_snapshot_id,omitempty"`
	InstructionSnapshot *instructions.Summary `json:"instruction_snapshot,omitempty"`
	PluginGeneration    string                `json:"plugin_generation,omitempty"`
}

func (s *Session) SetHookSnapshotID(snapshotID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.run.HookSnapshotID = strings.TrimSpace(snapshotID)
	s.mu.Unlock()
}

func (s *Session) SetInstructionSnapshot(snapshot instructions.Summary) {
	if s == nil {
		return
	}
	s.mu.Lock()
	copy := snapshot
	s.run.InstructionSnapshot = &copy
	s.mu.Unlock()
}

type Event struct {
	SourceSeq      uint64
	Activity       bool
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

// appendEventLocked preserves the original run frame identity while adding
// a separate thread activity cursor. Closing a root stream does not close the
// activity stream used by its explicitly retained children.
func (s *Session) appendEventLocked(event Event, terminal bool) {
	event.SourceSeq = s.bus.Append(event, terminal)
	if event.SourceSeq == 0 {
		agentID := event.AgentEvent.AgentID
		if event.Kind == EventKindSemantic {
			agentID, _ = event.SemanticRecord.Payload["agent_id"].(string)
		}
		if agentID != "" && agentID != "root" {
			event.SourceSeq = s.bus.RetainAfterClose(event)
		}
	}
	event.Activity = true
	if s.activity != nil {
		s.activity.Append(event, false)
	}
}

type Session struct {
	childComputers       map[string]*computer.Runtime
	computerReviewOwners map[string]*computer.Runtime
	mu                   sync.RWMutex
	run                  Run
	cancel               context.CancelFunc
	bus                  *EventBus
	activity             *EventBus
	semanticRecorder     agentcore.SemanticRecorder
	subagents            subagentruntime.Controller
	userQuestions        map[string]*pendingUserQuestion
	externalInteractions map[string]*pendingExternalInteraction
	computerRuntime      *computer.Runtime
	pluginLeaseRelease   func()
	pluginLeaseOnce      sync.Once
	pluginLeaseRequested bool
	asyncHookRetains     int
}

func (s *Session) SetPluginRuntimeLease(generation string, release func()) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.run.PluginGeneration = strings.TrimSpace(generation)
	s.pluginLeaseRelease = release
	s.mu.Unlock()
}

func (s *Session) ReleasePluginRuntimeLease() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.pluginLeaseRequested = true
	release := s.pluginLeaseRelease
	ready := s.asyncHookRetains == 0
	s.mu.Unlock()
	if ready {
		s.releasePluginRuntimeLease(release)
	}
}

// RetainAsyncHook keeps the run's immutable Plugin generation alive while a
// background Hook still executes from that package. The returned release is
// idempotent and may complete after the run itself reached terminal state.
func (s *Session) RetainAsyncHook() func() {
	if s == nil {
		return func() {}
	}
	s.mu.Lock()
	s.asyncHookRetains++
	s.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			if s.asyncHookRetains > 0 {
				s.asyncHookRetains--
			}
			ready := s.pluginLeaseRequested && s.asyncHookRetains == 0
			release := s.pluginLeaseRelease
			s.mu.Unlock()
			if ready {
				s.releasePluginRuntimeLease(release)
			}
		})
	}
}

func (s *Session) releasePluginRuntimeLease(release func()) {
	s.pluginLeaseOnce.Do(func() {
		if release != nil {
			release()
		}
	})
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

func (s *Session) SetComputerRuntime(runtime *computer.Runtime) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.computerRuntime = runtime
	s.mu.Unlock()
}

func (s *Session) ComputerRuntime() *computer.Runtime {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.computerRuntime
}

type pendingUserQuestion struct {
	ctx        context.Context
	request    agentcore.AwakenEvent
	resolution chan agentcore.UserQuestionResolution
}

func (s *Session) SetActivityBus(bus *EventBus) {
	s.mu.Lock()
	s.activity = bus
	s.mu.Unlock()
}

// CloseRootComputer ends only the root execution's desktop ownership. Child
// runtimes own their leases independently; retaining shared Plugin resources
// must never prevent a waiting child from acquiring the root's released lease.
func (s *Session) CloseRootComputer() {
	if runtime := s.ComputerRuntime(); runtime != nil {
		_ = runtime.Terminate(context.Background(), "work_ended")
	}
}

type pendingExternalInteraction struct {
	ctx     context.Context
	request agentcore.AwakenEvent
	answer  chan agentcore.ExternalInteractionAnswer
}

func NewSession(runID, threadID, input string, attachments []RunInputAttachment, composerTokens []ComposerToken, cancel context.CancelFunc) *Session {
	return NewSessionWithFileComments(runID, threadID, input, attachments, composerTokens, nil, cancel)
}

func NewSessionWithFileComments(runID, threadID, input string, attachments []RunInputAttachment, composerTokens []ComposerToken, fileComments []FileLineComment, cancel context.CancelFunc) *Session {
	now := time.Now().UTC()
	return &Session{
		run: Run{
			RunID:            runID,
			ThreadID:         threadID,
			Input:            strings.TrimSpace(input),
			InputAttachments: cloneRunInputAttachments(attachments),
			ComposerTokens:   cloneComposerTokens(composerTokens),
			FileComments:     cloneFileLineComments(fileComments),
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
	return s.snapshotLocked()
}

func (s *Session) snapshotLocked() Run {
	snapshot := s.run
	snapshot.InputAttachments = cloneRunInputAttachments(snapshot.InputAttachments)
	snapshot.ComposerTokens = cloneComposerTokens(snapshot.ComposerTokens)
	snapshot.FileComments = cloneFileLineComments(snapshot.FileComments)
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
	if statusChanged {
		s.appendEventLocked(statusEvent, false)
	}
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
	if event.AgentID != "" && event.AgentID != "root" {
		event.ThreadID, event.RunID = s.run.ThreadID, s.run.RunID
		wrapped := Event{Kind: EventKindAgent, Timestamp: timestamp, ThreadID: s.run.ThreadID, RunID: s.run.RunID, AgentEvent: event}
		s.appendEventLocked(wrapped, false)
		s.mu.Unlock()
		return
	}
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
	s.appendEventLocked(wrapped, terminal)
	s.mu.Unlock()
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
		if id := stringPayload(record.Payload, "agent_id"); id != "" && id != "root" {
			wrapped := Event{Kind: EventKindSemantic, Timestamp: timestamp, ThreadID: s.run.ThreadID, RunID: s.run.RunID, SemanticRecord: record}
			s.appendEventLocked(wrapped, false)
			s.mu.Unlock()
			continue
		}
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
		s.appendEventLocked(wrapped, terminal)
		s.mu.Unlock()
	}
}

func (s *Session) StreamCursor() uint64 {
	if s == nil || s.bus == nil {
		return 0
	}
	return s.bus.StreamCursor()
}

// TranscriptSnapshot captures run state and the corresponding published stream
// under the same lock. Publication always locks Session before EventBus; Append
// is non-blocking and never calls back into Session.
func (s *Session) TranscriptSnapshot() (Run, []Event, uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	events, cursor := s.bus.Snapshot()
	// A finished root bus no longer retains late child gates. Reconstruct
	// current child gates from their live owners so transcript reload (including
	// activity-buffer overflow) cannot lose a still-answerable question.
	filtered := events[:0]
	var nextSeq uint64
	for _, event := range events {
		nextSeq = max(nextSeq, event.Seq)
		if event.Kind == EventKindAgent && event.AgentEvent.AgentID != "" && event.AgentEvent.AgentID != "root" {
			switch event.AgentEvent.Type {
			case agentcore.UserQuestionRequestedType, agentcore.UserQuestionResolvedType, agentcore.ExternalInteractionRequestedType, agentcore.ExternalInteractionResolvedType:
				continue
			}
		}
		filtered = append(filtered, event)
	}
	var pending []agentcore.AwakenEvent
	for _, interaction := range s.externalInteractions {
		if interaction.ctx.Err() == nil && interaction.request.AgentID != "" && interaction.request.AgentID != "root" {
			pending = append(pending, interaction.request)
		}
	}
	sort.Slice(pending, func(i, j int) bool {
		if pending[i].Timestamp.Equal(pending[j].Timestamp) {
			return pending[i].QuestionID+pending[i].InteractionID < pending[j].QuestionID+pending[j].InteractionID
		}
		return pending[i].Timestamp.Before(pending[j].Timestamp)
	})
	for _, request := range pending {
		nextSeq++
		filtered = append(filtered, Event{Kind: EventKindAgent, Seq: nextSeq, ThreadID: s.run.ThreadID, RunID: s.run.RunID, Timestamp: request.Timestamp, AgentEvent: request})
	}
	return s.snapshotLocked(), filtered, cursor
}

func (s *Session) StreamSnapshot() ([]Event, uint64) {
	if s == nil || s.bus == nil {
		return nil, 0
	}
	return s.bus.Snapshot()
}

var ErrChildUserQuestionUnsupported = errors.New("child questions must be returned to the parent Agent")

func (s *Session) AwaitUserQuestion(ctx context.Context, request agentcore.AwakenEvent) (agentcore.UserQuestionResolution, error) {
	if (strings.TrimSpace(request.AgentID) != "" && strings.TrimSpace(request.AgentID) != "root") || strings.TrimSpace(request.SubagentTaskID) != "" {
		return agentcore.UserQuestionResolution{}, ErrChildUserQuestionUnsupported
	}

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
		ctx:        ctx,
		request:    request,
		resolution: make(chan agentcore.UserQuestionResolution, 1),
	}

	s.mu.Lock()
	if ctx.Err() != nil || isTerminalStatus(s.run.Status) {
		s.mu.Unlock()
		return agentcore.UserQuestionResolution{}, ErrRunTerminal
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
	s.appendEventLocked(wrapped, false)
	s.mu.Unlock()

	select {
	case resolution := <-pending.resolution:
		return resolution, nil
	case <-ctx.Done():
		s.mu.Lock()
		if s.userQuestions[request.QuestionID] == pending {
			delete(s.userQuestions, request.QuestionID)
			resolved := request
			resolved.Type, resolved.QuestionResolution = agentcore.UserQuestionResolvedType, "cancelled"
			s.appendEventLocked(Event{Kind: EventKindAgent, ThreadID: s.run.ThreadID, RunID: s.run.RunID, Timestamp: time.Now().UTC(), AgentEvent: resolved}, false)
		}
		s.mu.Unlock()
		return agentcore.UserQuestionResolution{}, ctx.Err()
	}
}

func (s *Session) ResolveUserQuestion(questionID string, answer string) error {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return ErrUserQuestionNotFound
	}
	return s.resolveUserQuestion(questionID, agentcore.UserQuestionResolution{
		Status: agentcore.UserQuestionAnswered,
		Answer: answer,
	})
}

func (s *Session) CancelUserQuestion(questionID string) error {
	return s.resolveUserQuestion(questionID, agentcore.UserQuestionResolution{
		Status: agentcore.UserQuestionCancelled,
	})
}

func (s *Session) resolveUserQuestion(questionID string, resolution agentcore.UserQuestionResolution) error {
	questionID = strings.TrimSpace(questionID)
	if questionID == "" {
		return ErrUserQuestionNotFound
	}
	if resolution.Status != agentcore.UserQuestionAnswered && resolution.Status != agentcore.UserQuestionCancelled {
		return ErrUserQuestionNotFound
	}
	resolution.Answer = strings.TrimSpace(resolution.Answer)
	if resolution.Status == agentcore.UserQuestionAnswered && resolution.Answer == "" {
		return ErrUserQuestionNotFound
	}
	if resolution.Status == agentcore.UserQuestionCancelled {
		resolution.Answer = ""
	}

	now := time.Now().UTC()
	s.mu.Lock()
	pending, ok := s.userQuestions[questionID]
	if !ok {
		s.mu.Unlock()
		return ErrUserQuestionNotFound
	}
	if pending.ctx.Err() != nil || isTerminalStatus(s.run.Status) {
		delete(s.userQuestions, questionID)
		s.mu.Unlock()
		return ErrRunTerminal
	}
	delete(s.userQuestions, questionID)
	resolved := pending.request
	resolved.Type = agentcore.UserQuestionResolvedType
	resolved.Timestamp = now
	resolved.QuestionAnswer = resolution.Answer
	resolved.QuestionResolution = string(resolution.Status)
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
	s.appendEventLocked(wrapped, false)
	s.mu.Unlock()
	pending.resolution <- resolution
	return nil
}

func (s *Session) AwaitExternalInteraction(ctx context.Context, request agentcore.AwakenEvent) (agentcore.ExternalInteractionResolution, error) {
	answer, err := s.awaitExternalInteractionAnswer(ctx, request)
	return answer.Resolution, err
}

func (s *Session) AwaitExternalInteractionForm(ctx context.Context, request agentcore.AwakenEvent) (agentcore.ExternalInteractionAnswer, error) {
	return s.awaitExternalInteractionAnswer(ctx, request)
}

func (s *Session) awaitExternalInteractionAnswer(ctx context.Context, request agentcore.AwakenEvent) (agentcore.ExternalInteractionAnswer, error) {
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
		ctx:     ctx,
		request: request,
		answer:  make(chan agentcore.ExternalInteractionAnswer, 1),
	}

	s.mu.Lock()
	if ctx.Err() != nil || (isTerminalStatus(s.run.Status) && (request.AgentID == "" || request.AgentID == "root")) {
		s.mu.Unlock()
		return agentcore.ExternalInteractionAnswer{}, ErrRunTerminal
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
	s.appendEventLocked(wrapped, false)
	s.mu.Unlock()

	select {
	case resolution := <-pending.answer:
		return resolution, nil
	case <-ctx.Done():
		s.mu.Lock()
		if s.externalInteractions[request.InteractionID] == pending {
			delete(s.externalInteractions, request.InteractionID)
			resolved := request
			resolved.Type, resolved.InteractionResolution = agentcore.ExternalInteractionResolvedType, string(agentcore.ExternalInteractionCancelled)
			s.appendEventLocked(Event{Kind: EventKindAgent, ThreadID: s.run.ThreadID, RunID: s.run.RunID, Timestamp: time.Now().UTC(), AgentEvent: resolved}, false)
		}
		s.mu.Unlock()
		return agentcore.ExternalInteractionAnswer{}, ctx.Err()
	}
}

func (s *Session) ResolveExternalInteraction(interactionID string, resolution agentcore.ExternalInteractionResolution) error {
	return s.ResolveExternalInteractionWithContent(interactionID, resolution, nil)
}

func (s *Session) ResolveExternalInteractionWithContent(interactionID string, resolution agentcore.ExternalInteractionResolution, content map[string]any) error {
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
	pending, ok := s.externalInteractions[interactionID]
	if !ok {
		s.mu.Unlock()
		return ErrExternalInteractionNotFound
	}
	if pending.ctx.Err() != nil || (isTerminalStatus(s.run.Status) && (pending.request.AgentID == "" || pending.request.AgentID == "root")) {
		delete(s.externalInteractions, interactionID)
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
	s.appendEventLocked(wrapped, false)
	s.mu.Unlock()
	pending.answer <- agentcore.ExternalInteractionAnswer{Resolution: resolution, Content: cloneInteractionContent(content)}
	return nil
}

func cloneInteractionContent(content map[string]any) map[string]any {
	if content == nil {
		return nil
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return nil
	}
	var cloned map[string]any
	if json.Unmarshal(raw, &cloned) != nil {
		return nil
	}
	return cloned
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
	s.bus.Append(statusEvent, false)
	s.bus.Append(event, true)
	s.mu.Unlock()
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
	s.bus.Append(wrapped, true)
	s.mu.Unlock()
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
	s.bus.Append(event, true)
	s.mu.Unlock()
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
