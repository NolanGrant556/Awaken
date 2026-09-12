package agentcore

import (
	"awaken/src/context"
	"awaken/src/llm"
	"awaken/src/runcontrol"
	"awaken/src/subagentruntime"
	"awaken/src/tool"
	stdcontext "context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type runEventEmitter struct {
	logicalModelCallID    string
	transportAttempt      int
	attemptID             string
	inboxMessageIDs       func() []string
	contextRevision       func() uint64
	contextCommitted      func()
	sink                  EventSink
	recorder              SemanticRecorder
	sessionID             string
	threadID              string
	runID                 string
	turnState             TurnState
	currentAssistantID    string
	textMessageID         string
	reasoningMessageID    string
	textOpen              bool
	reasoningOpen         bool
	textStartedThisTurn   bool
	toolStreamStarted     bool
	preferNonStreaming    bool
	mutationOccurred      bool
	reasoningText         strings.Builder
	assistantText         strings.Builder
	toolArgumentProgress  toolArgumentProgressTracker
	pendingStopSource     string
	pendingTerminalReason TerminalReason
	persistenceErr        error
	suppressRunLifecycle  bool
	agentID               string
	parentAgentID         string
	subagentTaskID        string
	hookInvocationID      string
	subagentProgress      subagentruntime.ProgressReporter
	beforeTerminal        func(stdcontext.Context) error
	afterTerminal         func(stdcontext.Context, string, TerminalReason, int)
}

func newRunEventEmitter(threadID string, sink EventSink) *runEventEmitter {
	if strings.TrimSpace(threadID) == "" {
		threadID = newAwakenID("thread")
	}
	return &runEventEmitter{
		sink:     normalizeEventSink(sink),
		threadID: threadID,
		runID:    newAwakenID("run"),
	}
}

// runScopedEventSink is the only sink ToolRuntime may use inside a Runner.
// ToolRuntime emits progress, command output and other ephemeral frames directly;
// stamping them here keeps those frames in the same root/child identity scope as
// the durable tool_call and tool_result records that surround them.
type runScopedEventSink struct {
	emitter *runEventEmitter
}

func (s runScopedEventSink) Emit(event AwakenEvent) {
	if s.emitter == nil {
		return
	}
	s.emitter.emit(event)
}

func (s runScopedEventSink) AwaitUserQuestion(ctx stdcontext.Context, request AwakenEvent) (UserQuestionResolution, error) {
	if s.emitter == nil {
		return UserQuestionResolution{}, stdcontext.Canceled
	}
	s.emitter.fillIDs(&request)
	if awaiter, ok := s.emitter.sink.(UserQuestionAwaiter); ok {
		resume := subagentruntime.SuspendExecution(ctx)
		answer, err := awaiter.AwaitUserQuestion(ctx, request)
		if resumeErr := resume(); err == nil {
			err = resumeErr
		}
		return answer, err
	}
	return UserQuestionResolution{}, stdcontext.Canceled
}

func (s runScopedEventSink) AwaitExternalInteraction(ctx stdcontext.Context, request AwakenEvent) (ExternalInteractionResolution, error) {
	if s.emitter == nil {
		return "", stdcontext.Canceled
	}
	s.emitter.fillIDs(&request)
	if awaiter, ok := s.emitter.sink.(ExternalInteractionAwaiter); ok {
		resume := subagentruntime.SuspendExecution(ctx)
		answer, err := awaiter.AwaitExternalInteraction(ctx, request)
		if resumeErr := resume(); err == nil {
			err = resumeErr
		}
		return answer, err
	}
	return "", stdcontext.Canceled
}

func (s runScopedEventSink) AwaitExternalInteractionForm(ctx stdcontext.Context, request AwakenEvent) (ExternalInteractionAnswer, error) {
	if s.emitter == nil {
		return ExternalInteractionAnswer{}, stdcontext.Canceled
	}
	s.emitter.fillIDs(&request)
	if awaiter, ok := s.emitter.sink.(ExternalInteractionFormAwaiter); ok {
		resume := subagentruntime.SuspendExecution(ctx)
		answer, err := awaiter.AwaitExternalInteractionForm(ctx, request)
		if resumeErr := resume(); err == nil {
			err = resumeErr
		}
		return answer, err
	}
	return ExternalInteractionAnswer{}, stdcontext.Canceled
}

func (e *runEventEmitter) scopedSink() EventSink {
	if e == nil {
		return noopEventSink{}
	}
	return runScopedEventSink{emitter: e}
}

func (e *runEventEmitter) turnID() string {
	if e.turnState.TurnID == "" {
		return ""
	}
	return string(e.turnState.TurnID)
}

func (e *runEventEmitter) persistenceFailure() error {
	if e == nil {
		return nil
	}
	return e.persistenceErr
}

func (e *runEventEmitter) notePersistenceFailure(phase string, cause error) error {
	if e == nil || cause == nil {
		return cause
	}
	if e.persistenceErr != nil {
		return e.persistenceErr
	}
	phase = strings.TrimSpace(phase)
	if phase == "" {
		phase = "unknown"
	}
	e.persistenceErr = fmt.Errorf("%w: phase=%s: %v", ErrPersistenceFailed, phase, cause)
	e.pendingTerminalReason = TerminalReasonPersistenceError

	// If storage recovered after the original exhausted append/flush, close
	// the journal with an explicit persistence_error terminal. Failure of
	// this recovery write is retained in the returned error and never ignored.
	canonicalTerminalPublished := false
	if e.recorder != nil {
		recoveryCtx, cancel := stdcontext.WithTimeout(stdcontext.Background(), 2*time.Second)
		terminal := NewSemanticRecord(
			SemanticKindRunTerminal,
			e.threadID,
			e.runID,
			e.turnID(),
			"",
			e.sessionID,
			map[string]any{
				"status":          "error",
				"error":           e.persistenceErr.Error(),
				"code":            string(TerminalReasonPersistenceError),
				"terminal_reason": string(TerminalReasonPersistenceError),
				"failed_phase":    phase,
				"stop_source":     e.pendingStopSource,
			},
		)
		recoveryErr := e.recorder.Append(recoveryCtx, terminal)
		if recoveryErr == nil {
			recoveryErr = e.recorder.Flush(recoveryCtx)
		}
		cancel()
		if recoveryErr != nil {
			e.persistenceErr = fmt.Errorf("%w; persistence terminal failed: %v", e.persistenceErr, recoveryErr)
		} else if canonical, ok := e.recorder.(CanonicalLiveSemanticRecorder); ok && canonical.BroadcastsCanonicalSemanticRecords() {
			canonicalTerminalPublished = true
		}
	}

	event := AwakenEvent{
		Type:           RunErrorEventType,
		Error:          e.persistenceErr.Error(),
		Code:           "PERSISTENCE_ERROR",
		StopSource:     e.pendingStopSource,
		TerminalReason: TerminalReasonPersistenceError,
	}
	e.fillIDs(&event)
	if !canonicalTerminalPublished {
		e.sink.Emit(event)
	}
	return e.persistenceErr
}

func (e *runEventEmitter) persistDurable(ctx stdcontext.Context, phase string, record SemanticRecord, liveEvents ...AwakenEvent) error {
	if err := e.persistenceFailure(); err != nil {
		return err
	}
	record = e.withAgentIdentity(record)
	persistCtx, cancel := durablePersistenceContext(ctx)
	defer cancel()
	if err := PersistDurable(persistCtx, e.recorder, e.sink, record, liveEvents...); err != nil {
		return e.notePersistenceFailure(phase, err)
	}
	return nil
}

func (e *runEventEmitter) persistDurableBatch(ctx stdcontext.Context, phase string, records []SemanticRecord, liveEvents ...AwakenEvent) error {
	if err := e.persistenceFailure(); err != nil {
		return err
	}
	for index := range records {
		records[index] = e.withAgentIdentity(records[index])
	}
	persistCtx, cancel := durablePersistenceContext(ctx)
	defer cancel()
	if err := PersistDurableBatch(persistCtx, e.recorder, e.sink, records, liveEvents...); err != nil {
		return e.notePersistenceFailure(phase, err)
	}
	return nil
}

func (e *runEventEmitter) withAgentIdentity(record SemanticRecord) SemanticRecord {
	if e == nil || (strings.TrimSpace(e.agentID) == "" && strings.TrimSpace(e.parentAgentID) == "" && strings.TrimSpace(e.subagentTaskID) == "" && strings.TrimSpace(e.hookInvocationID) == "") {
		return record
	}
	copyPayload := make(map[string]any, len(record.Payload)+3)
	for key, value := range record.Payload {
		copyPayload[key] = value
	}
	copyPayload["agent_id"] = e.agentID
	if e.attemptID != "" {
		copyPayload["attempt_id"] = e.attemptID
	}
	if strings.TrimSpace(e.parentAgentID) != "" {
		copyPayload["parent_agent_id"] = e.parentAgentID
	}
	if strings.TrimSpace(e.subagentTaskID) != "" {
		copyPayload["subagent_task_id"] = e.subagentTaskID
	}
	if strings.TrimSpace(e.hookInvocationID) != "" {
		copyPayload["hook_invocation_id"] = e.hookInvocationID
	}
	record.Payload = copyPayload
	return record
}

func (e *runEventEmitter) persistTerminal(ctx stdcontext.Context, phase string, record SemanticRecord, event AwakenEvent) error {
	if err := e.persistenceFailure(); err != nil {
		return err
	}
	persistCtx, cancel := durablePersistenceContext(ctx)
	defer cancel()
	if err := e.recorder.Append(persistCtx, record); err != nil {
		return e.notePersistenceFailure(phase+"_append", err)
	}
	if err := e.recorder.Flush(persistCtx); err != nil {
		return e.notePersistenceFailure(phase+"_flush", err)
	}
	if canonical, ok := e.recorder.(CanonicalLiveSemanticRecorder); ok && canonical.BroadcastsCanonicalSemanticRecords() {
		return nil
	}
	// A successful run terminal becomes live only after append + fsync. This
	// prevents the UI from observing RunFinished when the durable terminal
	// was never confirmed.
	e.sink.Emit(event)
	return nil
}

func (e *runEventEmitter) fillIDs(event *AwakenEvent) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.ThreadID == "" {
		event.ThreadID = e.threadID
	}
	if event.RunID == "" {
		event.RunID = e.runID
	}
	if event.AgentID == "" {
		event.AgentID = e.agentID
	}
	if event.ParentAgentID == "" {
		event.ParentAgentID = e.parentAgentID
	}
	if event.SubagentTaskID == "" {
		event.SubagentTaskID = e.subagentTaskID
	}
	if e.turnState.TurnID != "" {
		if event.TurnID == "" {
			event.TurnID = string(e.turnState.TurnID)
		}
		if event.QueryChainID == "" {
			event.QueryChainID = string(e.turnState.QueryChainID)
		}
		if event.Depth == 0 {
			event.Depth = int(e.turnState.Depth)
		}
		if event.TurnCount == 0 {
			event.TurnCount = e.turnState.TurnCount
		}
		if event.TransitionReason == "" {
			event.TransitionReason = e.turnState.TransitionReason
		}
	}
}

func (e *runEventEmitter) emit(event AwakenEvent) {
	e.fillIDs(&event)
	e.sink.Emit(event)
}

func (e *runEventEmitter) runStarted(ctx stdcontext.Context) error {
	if e.suppressRunLifecycle {
		return nil
	}
	event := AwakenEvent{Type: RunStartedEventType}
	e.fillIDs(&event)
	if e.recorder != nil {
		record := NewSemanticRecord(SemanticKindRunStarted, e.threadID, e.runID, "", "", e.sessionID, map[string]any{})
		if err := e.persistDurable(ctx, "run_started", record, event); err != nil {
			return err
		}
		return nil
	}
	e.sink.Emit(event)
	return nil
}

func (e *runEventEmitter) turnStarted(state TurnState) {
	e.turnState = state
	e.emit(AwakenEvent{
		Type:             TurnStartedEventType,
		TransitionReason: state.TransitionReason,
	})
}

func (e *runEventEmitter) turnFinished(ctx stdcontext.Context, reason TerminalReason, transition TransitionReason) {
	if transition == "" {
		transition = e.turnState.TransitionReason
	}
	e.turnState.TerminalReason = reason
	e.turnState.TransitionReason = transition
	event := AwakenEvent{
		Type:             TurnFinishedEventType,
		TerminalReason:   reason,
		TransitionReason: transition,
	}
	e.fillIDs(&event)
	commitsContext := ctx.Err() == nil && (reason == TerminalReasonCompleted || (reason == "" && transition != ""))
	payload := map[string]any{"terminal_reason": string(reason), "transition_reason": string(transition)}
	if commitsContext {
		if e.inboxMessageIDs != nil {
			payload["incorporated_message_ids"] = e.inboxMessageIDs()
		}
		if e.contextRevision != nil {
			payload["context_revision"] = e.contextRevision()
		}
	}
	if e.recorder != nil {
		if err := e.persistDurable(ctx, "turn_boundary", NewSemanticRecord(
			SemanticKindTurnBoundary,
			e.threadID,
			e.runID,
			e.turnID(),
			"",
			e.sessionID,
			payload,
		), event); err != nil {
			return
		}
		if commitsContext {
			persistCtx, cancel := durablePersistenceContext(ctx)
			defer cancel()
			if err := e.recorder.Flush(persistCtx); err != nil {
				e.notePersistenceFailure("context_commit", err)
				return
			}
			if e.contextCommitted != nil {
				e.contextCommitted()
			}
		}
		return
	}
	e.sink.Emit(event)
	if commitsContext && e.contextCommitted != nil {
		e.contextCommitted()
	}
}

func (e *runEventEmitter) runFinished(ctx stdcontext.Context, content string, state *ContextWindowState, reasons ...TerminalReason) {
	if ctx.Err() != nil {
		e.runErrored(ctx, ctx.Err(), TerminalReasonCancelled)
		return
	}
	if e.suppressRunLifecycle {
		return
	}
	if err := e.runTerminalBarrier(ctx); err != nil {
		e.publishSubagentBarrierFailure(ctx, err)
		return
	}
	reason := firstTerminalReason(reasons, TerminalReasonCompleted)
	event := AwakenEvent{
		Type:           RunFinishedEventType,
		Content:        content,
		TerminalReason: reason,
	}
	if state != nil {
		applyContextWindowEstimate(&event, *state)
	}
	e.fillIDs(&event)
	if e.recorder != nil {
		if err := e.persistTerminal(ctx, "run_terminal_finished", NewSemanticRecord(
			SemanticKindRunTerminal,
			e.threadID,
			e.runID,
			e.turnID(),
			"",
			e.sessionID,
			map[string]any{
				"status":          "finished",
				"result":          content,
				"terminal_reason": string(reason),
			},
		), event); err != nil {
			return
		}
		if e.afterTerminal != nil {
			e.afterTerminal(ctx, "finished", reason, len([]byte(content)))
		}
		return
	}
	e.sink.Emit(event)
	if e.afterTerminal != nil {
		e.afterTerminal(ctx, "finished", reason, len([]byte(content)))
	}
}

func (e *runEventEmitter) runErrored(ctx stdcontext.Context, err error, reasons ...TerminalReason) {
	if ctx.Err() != nil {
		e.pendingStopSource = string(runcontrol.FromContext(ctx))
	}
	if err == nil {
		return
	}
	if e.suppressRunLifecycle {
		return
	}
	if barrierErr := e.runTerminalBarrier(ctx); barrierErr != nil {
		e.publishSubagentBarrierFailure(ctx, barrierErr)
		return
	}
	event := AwakenEvent{
		Type:           RunErrorEventType,
		Error:          err.Error(),
		TerminalReason: firstTerminalReason(reasons, terminalReasonForError(err)),
	}
	if ctx.Err() != nil || errors.Is(err, stdcontext.Canceled) {
		event.StopSource = string(runcontrol.FromContext(ctx))
	}
	if event.TerminalReason == TerminalReasonCancelled || errors.Is(err, stdcontext.Canceled) {
		event.Code = "RUN_CANCELLED"
	}
	switch {
	case errors.Is(err, errContextWindowBlocked):
		event.Code = "CONTEXT_BLOCKED"
	case errors.Is(err, ErrContextEpochProjection):
		event.Code = "CONTEXT_EPOCH_ERROR"
	case errors.Is(err, llm.ErrMissingAPIKey):
		event.Code = "PROVIDER_AUTH_MISSING"
	case errors.Is(err, llm.ErrRequestFailed):
		event.Code = "PROVIDER_REQUEST_FAILED"
	}
	e.fillIDs(&event)
	status := "error"
	if event.Code == "RUN_CANCELLED" {
		status = "cancelled"
	}
	if e.recorder != nil {
		if persistErr := e.persistTerminal(ctx, "run_terminal_error", NewSemanticRecord(
			SemanticKindRunTerminal,
			e.threadID,
			e.runID,
			e.turnID(),
			"",
			e.sessionID,
			map[string]any{
				"status":          status,
				"error":           event.Error,
				"code":            event.Code,
				"terminal_reason": string(event.TerminalReason),
				"stop_source":     event.StopSource,
			},
		), event); persistErr != nil {
			return
		}
		if e.afterTerminal != nil {
			e.afterTerminal(ctx, status, event.TerminalReason, 0)
		}
		return
	}
	e.sink.Emit(event)
	if e.afterTerminal != nil {
		e.afterTerminal(ctx, status, event.TerminalReason, 0)
	}
}

func (e *runEventEmitter) runTerminalBarrier(ctx stdcontext.Context) error {
	if e == nil || e.beforeTerminal == nil {
		return nil
	}
	barrier := e.beforeTerminal
	e.beforeTerminal = nil
	return barrier(ctx)
}

func (e *runEventEmitter) publishSubagentBarrierFailure(ctx stdcontext.Context, cause error) {
	if e == nil || cause == nil {
		return
	}
	err := fmt.Errorf("subagent terminal barrier failed: %w", cause)
	event := AwakenEvent{
		Type:           RunErrorEventType,
		Error:          err.Error(),
		Code:           "SUBAGENT_TERMINAL_BARRIER_FAILED",
		StopSource:     e.pendingStopSource,
		TerminalReason: TerminalReasonAbortedTools,
	}
	e.fillIDs(&event)
	if e.recorder != nil {
		if persistErr := e.persistTerminal(ctx, "run_terminal_subagent_barrier", NewSemanticRecord(
			SemanticKindRunTerminal,
			e.threadID,
			e.runID,
			e.turnID(),
			"",
			e.sessionID,
			map[string]any{
				"status":          "error",
				"error":           event.Error,
				"code":            event.Code,
				"terminal_reason": string(event.TerminalReason),
				"stop_source":     event.StopSource,
			},
		), event); persistErr != nil {
			e.notePersistenceFailure("run_terminal_subagent_barrier", persistErr)
			return
		}
		e.persistenceErr = err
		return
	}
	e.persistenceErr = err
	e.sink.Emit(event)
}

func toolResultAppliedMutation(result ToolExecutionResult) bool {
	if result.Outcome.Status != "" {
		return result.Outcome.Status == tool.OutcomeSucceeded
	}
	return result.Status == ToolExecutionSucceeded && !result.IsError
}

func (e *runEventEmitter) markMutation() {
	e.mutationOccurred = true
}

func (e *runEventEmitter) setPendingTerminalReason(reason TerminalReason) {
	if e == nil || reason == "" {
		return
	}
	e.pendingTerminalReason = reason
}

func (e *runEventEmitter) emitRecoveryDecision(call context.ToolCall, options []string, errorCode string, blocked bool) {
	if e == nil {
		return
	}
	msg := "tool recovery required"
	if blocked {
		msg = "tool retry blocked"
	}
	payload := map[string]any{
		"tool_call_id":      call.ID,
		"tool_name":         call.Function.Name,
		"error_code":        errorCode,
		"blocked":           blocked,
		"recovery_options":  append([]string(nil), options...),
		"recovery_required": true,
		"message":           msg,
		"retryable":         !blocked,
	}
	if blocked {
		payload["tool_status"] = string(tool.OutcomeDenied)
		payload["transport_status"] = string(tool.TransportNotStarted)
		payload["semantic_status"] = string(tool.SemanticFailure)
		payload["process_status"] = string(tool.ProcessNotApplicable)
		payload["error_category"] = string(tool.ErrorCategoryPolicy)
	}
	eventType := ToolRecoveryRequiredEventType
	if blocked {
		eventType = ToolRetryBlockedEventType
	}
	event := AwakenEvent{
		Type:            eventType,
		ToolCallID:      call.ID,
		ToolName:        call.Function.Name,
		Content:         msg,
		Code:            errorCode,
		ErrorCode:       errorCode,
		RecoveryOptions: append([]string(nil), options...),
		Retryable:       !blocked,
	}
	if blocked {
		event.ToolStatus = string(tool.OutcomeDenied)
		event.TransportStatus = string(tool.TransportNotStarted)
		event.SemanticStatus = string(tool.SemanticFailure)
		event.ProcessStatus = string(tool.ProcessNotApplicable)
		event.ErrorCategory = string(tool.ErrorCategoryPolicy)
	}
	e.fillIDs(&event)
	if e.recorder == nil {
		e.sink.Emit(event)
		return
	}
	if err := e.persistDurable(stdcontext.Background(), "recovery_decision", NewSemanticRecord(
		SemanticKindRecoveryDecision,
		e.threadID,
		e.runID,
		e.turnID(),
		call.ID,
		e.sessionID,
		payload,
	), event); err != nil {
		return
	}
}
