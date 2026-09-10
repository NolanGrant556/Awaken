package agentcore

import (
	"awaken/src/llm"
	"awaken/src/subagentruntime"
	stdcontext "context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (e *runEventEmitter) contextEpoch(ctx stdcontext.Context, receipt ContextEpochReceipt) error {
	if e == nil || e.recorder == nil {
		return nil
	}
	payload := map[string]any{
		"epoch_id": receipt.EpochID, "baseline_digest": receipt.BaselineDigest,
		"state": receipt.State, "boundary_reason": receipt.BoundaryReason,
		"creation_boundary_reason": receipt.CreationBoundaryReason,
		"baseline_sequence":        receipt.BaselineSequence,
	}
	persistCtx, cancel := durablePersistenceContext(ctx)
	defer cancel()
	record := e.withAgentIdentity(NewSemanticRecord(SemanticKindContextEpoch, e.threadID, e.runID, e.turnID(), "", e.sessionID, payload))
	if err := e.recorder.Append(persistCtx, record); err != nil {
		return e.notePersistenceFailure("context_epoch_append", err)
	}
	if err := e.recorder.Flush(persistCtx); err != nil {
		return e.notePersistenceFailure("context_epoch_flush", err)
	}
	return nil
}

func (e *runEventEmitter) memoryRecalled(ctx stdcontext.Context, receipts []MemoryRecallReceipt) error {
	if len(receipts) == 0 {
		return nil
	}
	event := AwakenEvent{Type: MemoryRecalledEventType, MemoryReceipts: append([]MemoryRecallReceipt(nil), receipts...)}
	e.fillIDs(&event)
	payload := map[string]any{"count": len(receipts), "memories": receipts}
	if e.recorder != nil {
		return e.persistDurable(ctx, "memory_recalled", NewSemanticRecord(SemanticKindMemoryRecalled, e.threadID, e.runID, e.turnID(), "", e.sessionID, payload), event)
	}
	e.sink.Emit(event)
	return nil
}

func (e *runEventEmitter) modelRequest(ctx stdcontext.Context, snapshot ContextUsageSnapshot, purpose llm.RequestPurpose) error {
	event := AwakenEvent{Type: ContextSnapshotEventType, ContextUsageSnapshot: &snapshot}
	e.fillIDs(&event)
	if e.recorder == nil {
		e.sink.Emit(event)
		return nil
	}
	records := []SemanticRecord{
		e.withAgentIdentity(ContextSnapshotSemanticRecord(e.threadID, e.runID, e.turnID(), e.sessionID, snapshot)),
		e.withAgentIdentity(ModelRequestSemanticRecord(e.threadID, e.runID, e.turnID(), e.sessionID, snapshot, purpose)),
	}
	if e.inboxMessageIDs != nil {
		records[1].Payload["admitted_message_ids"] = e.inboxMessageIDs()
	}
	if e.contextRevision != nil {
		records[1].Payload["context_revision"] = e.contextRevision() - 1
	}
	records[1].Payload["logical_call_id"] = e.logicalModelCallID
	records[1].Payload["transport_attempt_index"] = e.transportAttempt
	persistCtx, cancel := durablePersistenceContext(ctx)
	defer cancel()
	if err := PersistDurableBatch(persistCtx, e.recorder, e.sink, records, event); err != nil {
		return e.notePersistenceFailure("model_request_append", err)
	}
	// The model_request record must be on stable storage before the provider
	// side effect starts.
	if err := e.recorder.Flush(persistCtx); err != nil {
		return e.notePersistenceFailure("model_request_flush", err)
	}
	return nil
}

func (e *runEventEmitter) contextSnapshot(ctx stdcontext.Context, snapshot ContextUsageSnapshot) error {
	event := AwakenEvent{Type: ContextSnapshotEventType, ContextUsageSnapshot: &snapshot}
	e.fillIDs(&event)
	if e.recorder == nil {
		e.sink.Emit(event)
		return nil
	}
	persistCtx, cancel := durablePersistenceContext(ctx)
	defer cancel()
	record := e.withAgentIdentity(ContextSnapshotSemanticRecord(e.threadID, e.runID, e.turnID(), e.sessionID, snapshot))
	if err := PersistDurable(persistCtx, e.recorder, e.sink, record, event); err != nil {
		return e.notePersistenceFailure("context_snapshot_append", err)
	}
	if err := e.recorder.Flush(persistCtx); err != nil {
		return e.notePersistenceFailure("context_snapshot_flush", err)
	}
	return nil
}

func (e *runEventEmitter) modelUsage(ctx stdcontext.Context, usage ModelUsageRecord) error {
	event := AwakenEvent{Type: ModelUsageEventType, ModelUsage: &usage}
	e.fillIDs(&event)
	if e.recorder == nil {
		e.sink.Emit(event)
		return e.reportModelUsage(ctx, usage)
	}
	persistCtx, cancel := durablePersistenceContext(ctx)
	defer cancel()
	record := e.withAgentIdentity(ModelUsageSemanticRecord(e.threadID, e.runID, e.turnID(), e.sessionID, usage))
	if err := PersistDurable(persistCtx, e.recorder, e.sink, record, event); err != nil {
		return e.notePersistenceFailure("model_usage_append", err)
	}
	if err := e.recorder.Flush(persistCtx); err != nil {
		return e.notePersistenceFailure("model_usage_flush", err)
	}
	return e.reportModelUsage(ctx, usage)
}

func (e *runEventEmitter) reportModelUsage(ctx stdcontext.Context, usage ModelUsageRecord) error {
	if e == nil || e.subagentProgress == nil || strings.TrimSpace(usage.RequestID) == "" {
		return nil
	}
	update := subagentruntime.ProgressUpdate{ModelRequestID: usage.RequestID}
	if usage.InputTokens != nil {
		update.InputTokens, update.HasInputTokens = *usage.InputTokens, true
	}
	if usage.OutputTokens != nil {
		update.OutputTokens, update.HasOutputTokens = *usage.OutputTokens, true
	}
	if usage.TotalTokens != nil {
		update.TotalTokens, update.HasTotalTokens = *usage.TotalTokens, true
	}
	if err := e.subagentProgress(ctx, update); err != nil {
		return e.notePersistenceFailure("subagent_model_usage_progress", err)
	}
	return nil
}

func durablePersistenceContext(ctx stdcontext.Context) (stdcontext.Context, stdcontext.CancelFunc) {
	if ctx != nil && ctx.Err() == nil {
		return stdcontext.WithCancel(ctx)
	}
	return stdcontext.WithTimeout(stdcontext.Background(), 2*time.Second)
}

func (e *runEventEmitter) emitContextWindow(state ContextWindowState) {
	e.emit(contextWindowEvent(ContextEstimatedEventType, state, state.Summary()))
	if state.ShouldWarn() {
		e.emit(contextWindowEvent(ContextWarningEventType, state, "context window warning: "+state.Summary()))
	}
}

func (e *runEventEmitter) emitContextBlocked(state ContextWindowState, message string) {
	e.emit(contextWindowEvent(ContextBlockedEventType, state, message))
}

func (e *runEventEmitter) emitCompactStarted(state ContextWindowState, mode CompactMode) {
	event := contextWindowEvent(CompactStartedEventType, state, "compact started: "+string(mode))
	event.CompactMode = mode
	event.CompactPreEstimate = intPointer(state.EstimatedTokens)
	e.emit(event)
}

func (e *runEventEmitter) emitCompactFinished(state ContextWindowState, result CompactResult, summaryModel string) {
	mode := result.Mode
	if mode == "" {
		mode = CompactModeAuto
	}
	event := contextWindowEvent(CompactFinishedEventType, state, fmt.Sprintf(
		"compact finished: %s boundary=%s estimated_tokens=%d",
		mode,
		result.BoundaryID,
		result.EstimatedTokens,
	))
	postEstimate := result.PostEstimateTokens
	if postEstimate <= 0 {
		postEstimate = result.EstimatedTokens
	}
	toolResultsPruned := result.ToolResultsPruned > 0
	event.ContextEstimatedTokens = postEstimate
	event.ContextCompactTarget = state.CompactTarget
	event.CompactMode = mode
	event.CompactBoundaryID = result.BoundaryID
	event.CompactPreEstimate = intPointer(state.EstimatedTokens)
	event.CompactPostEstimate = intPointer(postEstimate)
	event.CompactSummaryModel = summaryModel
	event.CompactFallbackUsed = boolPointer(result.Fallback.Used)
	event.CompactToolResultsPruned = boolPointer(toolResultsPruned)
	event.CompactToolResultsPrunedCount = firstPositiveInt(result.ToolResultsPrunedCount, result.ToolResultsPruned)
	event.CompactPrunedSafeGroupCount = result.PrunedSafeGroupCount
	event.CompactPrunedMessageCount = result.PrunedMessageCount
	event.CompactSummaryMaxTokens = result.SummaryMaxTokens
	event.CompactSummaryRetryCount = result.SummaryRetryCount
	event.CompactSummaryRawChars = result.SummaryRawChars
	event.CompactSummaryParsedSections = result.SummaryParsedSections
	event.CompactSummaryMeaningfulSections = result.SummaryMeaningfulSections
	event.CompactSummaryQualityIssue = result.SummaryQualityIssue
	if result.SummarySourceHadUser || result.SummaryLatestUserPinned {
		event.CompactSummarySourceHadUser = boolPointer(result.SummarySourceHadUser)
		event.CompactSummaryLatestUserPinned = boolPointer(result.SummaryLatestUserPinned)
	}
	event.CompactFallbackMarker = firstNonEmptyCompactValue(result.FallbackMarker, result.Fallback.Marker)
	if result.BudgetReceipt != nil {
		event.ContextCompactTarget = result.BudgetReceipt.CompactTargetTokens
		event.CompactMandatoryFloor = result.BudgetReceipt.MandatoryFloorTokens
		event.CompactLocalProjected = result.BudgetReceipt.LocalProjectedTokens
		event.CompactConservativeProjected = result.BudgetReceipt.ConservativeProjected
		event.CompactContributionTokens = cloneStringIntMap(result.BudgetReceipt.ContributionTokens)
	}
	if result.NativeCompactionAttempted || result.NativeCompactionUsed || result.NativeCompactionReason != "" {
		event.CompactNativeAttempted = boolPointer(result.NativeCompactionAttempted)
		event.CompactNativeUsed = boolPointer(result.NativeCompactionUsed)
		event.CompactNativeReason = result.NativeCompactionReason
	}
	if result.Fallback.Used {
		event.CompactFailureReason = result.Fallback.Reason
	}
	e.emit(event)
}

func (e *runEventEmitter) emitCompactFailed(state ContextWindowState, mode CompactMode, message string, failureReason string, failure error) {
	event := contextWindowEvent(CompactFailedEventType, state, message)
	event.CompactMode = mode
	event.CompactFailureReason = failureReason
	var targetErr *CompactTargetUnachievableError
	if errors.As(failure, &targetErr) && targetErr != nil {
		event.ContextCompactTarget = targetErr.Target
		event.CompactMandatoryFloor = targetErr.MandatoryFloor
		event.CompactConservativeProjected = targetErr.Projected
		event.CompactContributionTokens = cloneStringIntMap(targetErr.CategoryBreakdown)
	}
	e.emit(event)
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func contextWindowEvent(eventType AwakenEventType, state ContextWindowState, content string) AwakenEvent {
	event := AwakenEvent{
		Type:    eventType,
		Content: content,
	}
	applyContextWindowEstimate(&event, state)
	return event
}

func applyContextWindowEstimate(event *AwakenEvent, state ContextWindowState) {
	if event == nil {
		return
	}
	event.ContextModelWindow = state.ModelWindow
	event.ContextOutputReserve = state.OutputReserve
	event.ContextEffectiveWindow = state.EffectiveWindow
	event.ContextEstimatedTokens = state.EstimatedTokens
	event.ContextWarningThreshold = state.WarningThreshold
	event.ContextAutoCompactThreshold = state.AutoCompactThreshold
	event.ContextCompactTarget = state.CompactTarget
	event.ContextEstimatedDeltaTokens = state.EstimatedDeltaTokens
}

func intPointer(value int) *int {
	v := value
	return &v
}

func boolPointer(value bool) *bool {
	v := value
	return &v
}
