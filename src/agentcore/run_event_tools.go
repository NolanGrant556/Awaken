package agentcore

import (
	"awaken/src/context"
	"awaken/src/llm"
	"awaken/src/subagentruntime"
	stdcontext "context"
	"encoding/json"
	"fmt"
	"strings"
)

func (e *runEventEmitter) onToolCallArgsDelta(delta llm.ToolCallDelta) {
	if delta.ArgumentsDelta == "" {
		return
	}
	e.toolStreamStarted = true
	e.ensureAssistantMessageID()
	// Some OpenAI-compatible providers stream assistant text and tool arguments
	// in the same response. Close the text boundary as soon as tool arguments
	// begin so the durable assistant snapshot remains before the tool item while
	// Write/Edit preparation can still be projected live during generation.
	if e.textOpen {
		e.finishTextIfOpen()
	}
	e.emitToolCallArgsDelta(delta)
}

func (e *runEventEmitter) emitToolCallArgsDelta(delta llm.ToolCallDelta) {
	toolCallID := delta.ID
	if toolCallID == "" {
		toolCallID = fmt.Sprintf("tool_index_%d", delta.Index)
	}
	event := AwakenEvent{
		Type:       ToolCallArgsDeltaEventType,
		ToolCallID: toolCallID,
		ToolName:   delta.Name,
		Delta:      delta.ArgumentsDelta,
	}
	if progress, ok := e.toolArgumentProgress.consume(delta); ok {
		event.ToolPreparationKind = progress.Kind
		event.ToolPreparationPath = progress.Path
		event.ToolPreparationAddedLines = progress.AddedLines
		event.ToolPreparationDeletedLines = progress.DeletedLines
	}
	e.emit(event)
}

func (e *runEventEmitter) emitToolCall(ctx stdcontext.Context, call context.ToolCall) error {
	e.toolStreamStarted = true
	parentMessageID := e.ensureAssistantMessageID()
	started := AwakenEvent{
		Type:            ToolCallStartedEventType,
		MessageID:       parentMessageID,
		ParentMessageID: parentMessageID,
		ToolCallID:      call.ID,
		ToolName:        call.Function.Name,
	}
	var argsEvent *AwakenEvent
	if call.Function.Arguments != "" {
		ev := AwakenEvent{
			Type:       ToolCallArgsEventType,
			ToolCallID: call.ID,
			ToolName:   call.Function.Name,
			Arguments:  call.Function.Arguments,
		}
		argsEvent = &ev
	}
	finished := AwakenEvent{
		Type:       ToolCallFinishedEventType,
		ToolCallID: call.ID,
		ToolName:   call.Function.Name,
	}
	e.fillIDs(&started)
	e.fillIDs(&finished)
	live := []AwakenEvent{started}
	if argsEvent != nil {
		e.fillIDs(argsEvent)
		live = append(live, *argsEvent)
	}
	live = append(live, finished)

	if e.recorder != nil {
		record := ToolCallSemanticRecord(e.threadID, e.runID, e.turnID(), e.sessionID, parentMessageID, call)
		if err := e.persistDurableBatch(ctx, "tool_call", []SemanticRecord{record}, live...); err != nil {
			return err
		}
		return e.reportToolCall(ctx, call)
	}
	emitLive(e.sink, live...)
	return e.reportToolCall(ctx, call)
}

func (e *runEventEmitter) reportToolCall(ctx stdcontext.Context, call context.ToolCall) error {
	if e == nil || e.subagentProgress == nil {
		return nil
	}
	if err := e.subagentProgress(ctx, subagentruntime.ProgressUpdate{
		ToolCallID: call.ID, ToolName: call.Function.Name,
	}); err != nil {
		return e.notePersistenceFailure("subagent_tool_progress", err)
	}
	return nil
}

func (e *runEventEmitter) emitToolResultBatch(
	ctx stdcontext.Context,
	result ToolExecutionResult,
	artifacts []AwakenEvent,
	semanticFacts []AwakenEvent,
) error {
	event := AwakenEvent{
		Type:         ToolResultEventType,
		MessageID:    newAwakenID("toolmsg"),
		ToolCallID:   result.ToolCallID,
		ToolName:     result.Name,
		Content:      result.Content,
		Role:         "tool",
		TrashedPaths: result.TrashedPaths,
	}
	applyToolOutcomeFields(&event, result)
	e.fillIDs(&event)

	records := make([]SemanticRecord, 0, len(semanticFacts)+len(artifacts)+2)
	for _, fact := range semanticFacts {
		switch fact.Type {
		case ToolPermissionDecidedEventType:
			records = append(records, PermissionDecisionSemanticRecord(e.threadID, e.runID, e.turnID(), e.sessionID, fact))
		case RuntimePrepareEventType,
			RuntimePrepareStartedEventType,
			RuntimePrepareProgressEventType,
			RuntimeReadyEventType,
			RuntimeRepairFailedEventType,
			RuntimeMissingEventType:
			records = append(records, RuntimeNoticeSemanticRecord(e.threadID, e.runID, e.turnID(), e.sessionID, fact))
		case ACPSessionStartedEventType, ACPSessionClosedEventType, ACPSessionInterruptedEventType,
			ACPTurnStartedEventType, ACPTurnFinishedEventType,
			ACPToolSnapshotEventType,
			ACPPermissionRequestedEventType, ACPPermissionResolvedEventType:
			records = append(records, ACPSemanticRecord(e.threadID, e.runID, e.turnID(), e.sessionID, fact))
		}
	}
	records = append(records, ToolResultSemanticRecord(e.threadID, e.runID, e.turnID(), e.sessionID, result))
	memoryReceipt, hasMemoryReceipt := memoryMutationReceipt(result)
	if hasMemoryReceipt {
		records = append(records, NewSemanticRecord(SemanticKindMemoryMutated, e.threadID, e.runID, e.turnID(), result.ToolCallID, e.sessionID, map[string]any{"memory": memoryReceipt}))
	}
	records = append(records, fileChangeSemanticRecords(e.threadID, e.runID, e.turnID(), e.sessionID, result)...)
	live := make([]AwakenEvent, 0, len(semanticFacts)+len(artifacts)+1)
	for _, fact := range semanticFacts {
		e.fillIDs(&fact)
		// ACP events are emitted live during tool execution by
		// artifactBufferingSink so that gates and progress appear immediately.
		// Only durable persistence remains for the final batch.
		if !isACPEventType(fact.Type) {
			live = append(live, fact)
		}
	}
	live = append(live, event)
	if hasMemoryReceipt {
		memoryEvent := AwakenEvent{Type: MemoryMutatedEventType, ToolCallID: result.ToolCallID, ToolName: result.Name, MemoryReceipts: []MemoryRecallReceipt{memoryReceipt}}
		e.fillIDs(&memoryEvent)
		live = append(live, memoryEvent)
	}
	for _, artifact := range artifacts {
		e.fillIDs(&artifact)
		records = append(records, ArtifactSemanticRecord(e.threadID, e.runID, e.turnID(), e.sessionID, artifact))
		live = append(live, artifact)
	}
	if e.recorder != nil {
		if err := e.persistDurableBatch(ctx, "tool_result", records, live...); err != nil {
			return err
		}
		return e.reportToolResult(ctx, result)
	}
	emitLive(e.sink, live...)
	return e.reportToolResult(ctx, result)
}

func (e *runEventEmitter) reportToolResult(ctx stdcontext.Context, result ToolExecutionResult) error {
	if e == nil || e.subagentProgress == nil || result.IsError {
		return nil
	}
	if result.Name != "WebFetch" && result.Name != "Read" {
		return nil
	}
	receipt := &subagentruntime.HostToolReceipt{
		ToolCallID: result.ToolCallID,
		ToolName:   result.Name,
		Details:    researchReceiptDetails(result.Name, result.Outcome.Details),
	}
	if err := e.subagentProgress(ctx, subagentruntime.ProgressUpdate{HostToolReceipt: receipt}); err != nil {
		return e.notePersistenceFailure("subagent_tool_receipt", err)
	}
	return nil
}

func researchReceiptDetails(toolName string, details map[string]any) map[string]any {
	keys := []string(nil)
	switch toolName {
	case "WebFetch":
		keys = []string{"requested_url", "final_url", "title", "fetched_at", "content_sha256", "http_status", "content_type", "truncated"}
	case "Read":
		keys = []string{"file_path", "sha256", "mime_type", "size", "mtime"}
	}
	filtered := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, ok := details[key]; ok {
			filtered[key] = value
		}
	}
	return filtered
}

func isACPEventType(t AwakenEventType) bool {
	switch t {
	case ACPSessionStartedEventType, ACPSessionClosedEventType, ACPSessionInterruptedEventType,
		ACPTurnStartedEventType, ACPTurnFinishedEventType,
		ACPToolSnapshotEventType,
		ACPPermissionRequestedEventType, ACPPermissionResolvedEventType:
		return true
	}
	return false
}

func memoryMutationReceipt(result ToolExecutionResult) (MemoryRecallReceipt, bool) {
	if result.Name != "memory" {
		return MemoryRecallReceipt{}, false
	}
	var arguments map[string]any
	if json.Unmarshal([]byte(result.SemanticArguments), &arguments) != nil {
		return MemoryRecallReceipt{}, false
	}
	action := strings.ToLower(memoryPayloadString(arguments, "action"))
	if action != "create" && action != "update" && action != "supersede" && action != "delete" {
		return MemoryRecallReceipt{}, false
	}
	receipt := MemoryRecallReceipt{
		MemoryID:  memoryPayloadString(arguments, "memory_id"),
		Scope:     memoryPayloadString(arguments, "scope"),
		Type:      memoryPayloadString(arguments, "type"),
		Action:    action,
		Status:    "rejected",
		ErrorCode: firstNonEmpty(result.ErrorCode, result.Outcome.ErrorCode),
	}
	if result.IsError || result.Outcome.IsError() {
		return receipt, true
	}
	var payload map[string]any
	if json.Unmarshal([]byte(result.Content), &payload) != nil {
		return MemoryRecallReceipt{}, false
	}
	receipt.MemoryID = memoryPayloadString(payload, "memory_id")
	receipt.Scope = memoryPayloadString(payload, "scope")
	receipt.Type = memoryPayloadString(payload, "type")
	receipt.Action = firstNonEmpty(memoryPayloadString(payload, "action"), action)
	receipt.Status = memoryPayloadString(payload, "status")
	if revision, ok := payload["revision"].(float64); ok {
		receipt.Revision = int(revision)
	}
	if receipt.Action == "delete" {
		receipt.Status = "deleted"
	}
	if receipt.MemoryID == "" || receipt.Action == "" {
		return MemoryRecallReceipt{}, false
	}
	return receipt, true
}

func memoryPayloadString(payload map[string]any, key string) string {
	value, exists := payload[key]
	if !exists || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func fileChangeSemanticRecords(threadID, runID, turnID, sessionID string, result ToolExecutionResult) []SemanticRecord {
	if !toolResultAppliedMutation(result) || len(result.AffectedPaths) == 0 {
		return nil
	}
	operation := "update"
	switch result.Name {
	case "Write":
		for _, change := range result.Outcome.Changes {
			if change.Action == "created" {
				operation = "create"
				break
			}
		}
	case "trash_file":
		operation = "delete"
	}
	seen := make(map[string]struct{}, len(result.AffectedPaths))
	records := make([]SemanticRecord, 0, len(result.AffectedPaths))
	for _, path := range result.AffectedPaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, duplicate := seen[path]; duplicate {
			continue
		}
		seen[path] = struct{}{}
		records = append(records, NewSemanticRecord(
			SemanticKindFileChange,
			threadID,
			runID,
			turnID,
			result.ToolCallID,
			sessionID,
			map[string]any{
				"tool_call_id":  result.ToolCallID,
				"tool_name":     result.Name,
				"path":          path,
				"operation":     operation,
				"status":        "applied",
				"patch_summary": strings.TrimSpace(result.PatchSummary),
			},
		))
	}
	return records
}

// applyToolOutcomeFields copies canonical ToolOutcome fields onto an event.
// Prefers result.Outcome when present; otherwise maps legacy Status/IsError.
func applyToolOutcomeFields(event *AwakenEvent, result ToolExecutionResult) {
	if result.Outcome.Status != "" {
		event.ToolStatus = string(result.Outcome.Status)
		event.TransportStatus = string(result.Outcome.TransportStatus)
		event.SemanticStatus = string(result.Outcome.SemanticStatus)
		event.ProcessStatus = string(result.Outcome.ProcessStatus)
		event.ProcessExitCode = result.Outcome.ExitCode
		event.ProcessTimedOut = result.Outcome.TimedOut
		event.ErrorCode = result.Outcome.ErrorCode
		event.ErrorCategory = string(result.Outcome.ErrorCategory)
		event.Retryable = result.Outcome.Retryable
		event.RecoveryOptions = append([]string(nil), result.Outcome.RecoveryOptions...)
		event.DurationMs = result.Outcome.DurationMs
		if len(result.Outcome.Diagnostic) > 0 {
			event.Diagnostic = make(map[string]any, len(result.Outcome.Diagnostic))
			for key, value := range result.Outcome.Diagnostic {
				event.Diagnostic[key] = value
			}
		}
		if event.DurationMs <= 0 {
			event.DurationMs = result.DurationMs
		}
		if event.ErrorCode == "" {
			event.ErrorCode = result.ErrorCode
		}
		return
	}

	status := string(result.Status)
	if status == "" {
		if result.IsError {
			status = string(ToolExecutionFailed)
		} else {
			status = string(ToolExecutionSucceeded)
		}
	}
	event.ToolStatus = status
	event.ErrorCode = result.ErrorCode
	event.DurationMs = result.DurationMs
	event.Retryable = false
}
