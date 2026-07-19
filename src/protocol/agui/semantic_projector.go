package agui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"awaken/src/agentcore"
	"awaken/src/tool"
)

// ProjectSemanticRecords converts durable SemanticRecord values into AG-UI events.
// Live SSE and transcript replay must share this projector.
func ProjectSemanticRecords(records []agentcore.SemanticRecord) []Event {
	out := make([]Event, 0, len(records)*2)
	for _, record := range records {
		frames := ProjectSemanticRecord(record)
		out = append(out, frames...)
	}
	return out
}

// ProjectSemanticRecord projects one SemanticRecord into zero or more AG-UI frames.
func ProjectSemanticRecord(record agentcore.SemanticRecord) []Event {
	frames := projectSemanticRecord(record)
	for i := range frames {
		frames[i].Seq = FrameSeq(record.Seq, i)
		frames[i].SemanticSeq = frames[i].Seq
		frames[i].RecordID = record.RecordID
	}
	return frames
}

func projectSemanticRecord(record agentcore.SemanticRecord) []Event {
	base := Event{
		Timestamp: toUnixMillis(record.Timestamp),
		ThreadID:  record.ThreadID,
		RunID:     record.RunID,
		TurnID:    record.TurnID,
		Seq:       record.Seq,
	}
	payload := record.Payload
	if payload == nil {
		payload = map[string]any{}
	}

	switch record.Kind {
	case agentcore.SemanticKindRunStarted:
		base.Type = EventRunStarted
		return []Event{base}
	case agentcore.SemanticKindMemoryRecalled:
		memoryEvent := base
		memoryEvent.Type = EventAwakenMemoryRecalled
		raw, err := json.Marshal(payload["memories"])
		if err == nil {
			var receipts []agentcore.MemoryRecallReceipt
			if json.Unmarshal(raw, &receipts) == nil {
				for _, receipt := range receipts {
					memoryEvent.Memories = append(memoryEvent.Memories, MemoryReceipt{MemoryID: receipt.MemoryID, Scope: receipt.Scope, Type: receipt.Type, Revision: receipt.Revision, Bytes: receipt.Bytes})
				}
			}
		}
		return []Event{memoryEvent}
	case agentcore.SemanticKindMemoryMutated:
		memoryEvent := base
		raw, err := json.Marshal(payload["memory"])
		if err == nil {
			var receipt agentcore.MemoryRecallReceipt
			if json.Unmarshal(raw, &receipt) == nil {
				memoryEvent.Memories = mapMemoryReceipts([]agentcore.MemoryRecallReceipt{receipt})
			}
		}
		memoryEvent.Type = memoryMutationEventType(memoryEvent.Memories)
		return []Event{memoryEvent}
	case agentcore.SemanticKindUserMessage:
		// User prompts are restored via run metadata; no AG-UI frame required.
		return nil
	case agentcore.SemanticKindAssistantMessage:
		messageID := stringFromAny(payload["message_id"])
		if messageID == "" {
			messageID = record.RecordID
		}
		content := stringFromAny(payload["content"])
		if strings.TrimSpace(content) == "" {
			return nil
		}
		start := base
		start.Type = EventTextMessageStart
		start.MessageID = messageID
		start.Role = "assistant"
		delta := base
		delta.Type = EventTextMessageContent
		delta.MessageID = messageID
		delta.Role = "assistant"
		delta.Delta = content
		end := base
		end.Type = EventTextMessageEnd
		end.MessageID = messageID
		end.Role = "assistant"
		return []Event{start, delta, end}
	case agentcore.SemanticKindToolCall:
		toolCallID := stringFromAny(payload["tool_call_id"])
		toolName := stringFromAny(payload["tool_name"])
		args := stringFromAny(payload["arguments"])
		parent := record.ParentID
		start := base
		start.Type = EventToolCallStart
		start.ToolCallID = toolCallID
		start.ToolCallName = toolName
		start.ParentMessageID = parent
		frames := []Event{start}
		if args != "" {
			argsEvent := base
			argsEvent.Type = EventToolCallArgs
			argsEvent.ToolCallID = toolCallID
			argsEvent.ToolCallName = toolName
			argsEvent.Delta = args
			frames = append(frames, argsEvent)
		}
		end := base
		end.Type = EventToolCallEnd
		end.ToolCallID = toolCallID
		end.ToolCallName = toolName
		frames = append(frames, end)
		return frames
	case agentcore.SemanticKindToolResult:
		result := base
		result.Type = EventToolCallResult
		result.ToolCallID = stringFromAny(payload["tool_call_id"])
		result.ToolCallName = stringFromAny(payload["tool_name"])
		result.Content = stringFromAny(payload["content"])
		result.Role = "tool"
		result.TrashedPaths = stringSliceFromAny(payload["trashed_paths"])
		result.AffectedPaths = stringSliceFromAny(payload["affected_paths"])
		result.PatchSummary = stringFromAny(payload["patch_summary"])
		applyOutcomePayload(&result, payload)
		applyExecutionMetadata(&result, payload)
		return []Event{result}
	case agentcore.SemanticKindArtifact:
		artifact := base
		if boolFromAny(payload["created"]) {
			artifact.Type = EventAwakenArtifactCreated
		} else {
			artifact.Type = EventAwakenArtifactUpdated
		}
		artifact.ToolCallID = stringFromAny(payload["tool_call_id"])
		artifact.ToolCallName = stringFromAny(payload["tool_name"])
		artifact.ArtifactContractVersion = tool.ArtifactContractVersionOrLegacy(intFromAny(payload["contract_version"]))
		artifact.ArtifactProducer = firstNonEmpty(stringFromAny(payload["producer"]), artifact.ToolCallName)
		artifact.ArtifactID = stringFromAny(payload["artifact_id"])
		artifact.ArtifactTitle = stringFromAny(payload["title"])
		artifact.ArtifactDescription = stringFromAny(payload["description"])
		artifact.ArtifactKind = stringFromAny(payload["kind"])
		artifact.ArtifactPath = stringFromAny(payload["path"])
		artifact.ArtifactContent = stringFromAny(payload["content"])
		artifact.ArtifactStatus = stringFromAny(payload["status"])
		artifact.ArtifactRole = string(tool.ResolveArtifactRole(
			tool.ArtifactRole(stringFromAny(payload["role"])),
			artifact.ArtifactProducer,
			artifact.ArtifactKind,
			artifact.ArtifactPath,
		))
		return []Event{artifact}
	case agentcore.SemanticKindTurnDiff:
		diff := base
		diff.Type = EventAwakenTurnDiff
		diff.Diff = stringFromAny(payload["diff"])
		diff.Truncated = boolFromAny(payload["truncated"])
		return []Event{diff}
	case agentcore.SemanticKindTurnBoundary:
		boundary := base
		boundary.Type = EventAwakenTurnFinished
		boundary.TerminalReason = stringFromAny(payload["terminal_reason"])
		boundary.TransitionReason = stringFromAny(payload["transition_reason"])
		return []Event{boundary}
	case agentcore.SemanticKindRunTerminal:
		status := stringFromAny(payload["status"])
		switch status {
		case "finished":
			finished := base
			finished.Type = EventRunFinished
			finished.Result = stringFromAny(payload["result"])
			finished.TerminalReason = stringFromAny(payload["terminal_reason"])
			finished.Outcome = &RunOutcome{Type: "success"}
			return []Event{finished}
		case "cancelled":
			cancelled := base
			cancelled.Type = EventRunError
			cancelled.Message = firstNonEmpty(stringFromAny(payload["error"]), "run cancelled")
			cancelled.Code = firstNonEmpty(stringFromAny(payload["code"]), "RUN_CANCELLED")
			cancelled.TerminalReason = stringFromAny(payload["terminal_reason"])
			return []Event{cancelled}
		case "interrupted":
			interrupted := base
			interrupted.Type = EventRunError
			interrupted.Message = firstNonEmpty(stringFromAny(payload["error"]), "run interrupted")
			interrupted.Code = firstNonEmpty(stringFromAny(payload["code"]), "PROCESS_RESTARTED")
			interrupted.TerminalReason = firstNonEmpty(stringFromAny(payload["terminal_reason"]), stringFromAny(payload["reason"]))
			return []Event{interrupted}
		default:
			errored := base
			errored.Type = EventRunError
			errored.Message = firstNonEmpty(stringFromAny(payload["error"]), "run failed")
			errored.Code = stringFromAny(payload["code"])
			errored.TerminalReason = stringFromAny(payload["terminal_reason"])
			return []Event{errored}
		}
	case agentcore.SemanticKindPermissionDecision:
		decision := base
		decision.Type = EventAwakenToolPermissionDecided
		decision.ToolCallID = stringFromAny(payload["tool_call_id"])
		decision.ToolCallName = stringFromAny(payload["tool_name"])
		decision.ToolDecisionAction = stringFromAny(payload["action"])
		decision.ToolDecisionSource = stringFromAny(payload["source"])
		decision.ToolDecisionReason = stringFromAny(payload["reason"])
		decision.StaticPermissionBehavior = stringFromAny(payload["behavior"])
		decision.StaticPermissionReason = stringFromAny(payload["static_reason"])
		decision.AffectedPaths = stringSliceFromAny(payload["affected_paths"])
		decision.PatchSummary = stringFromAny(payload["patch_summary"])
		decision.ProfileName = stringFromAny(payload["profile_name"])
		decision.TransformedFrom = stringFromAny(payload["transformed_from"])
		decision.TransformedTo = stringFromAny(payload["transformed_to"])
		decision.TrashedPaths = stringSliceFromAny(payload["trashed_paths"])
		decision.GuardianViolation = stringFromAny(payload["guardian_violation"])
		return []Event{decision}
	case agentcore.SemanticKindPersistenceWarning:
		warning := base
		warning.Type = EventAwakenUnknown
		warning.RawType = "persistence_warning"
		warning.Message = stringFromAny(payload["error"])
		return []Event{warning}
	case agentcore.SemanticKindRuntimeNotice:
		notice := base
		eventType := agentcore.AwakenEventType(stringFromAny(payload["event_type"]))
		switch eventType {
		case agentcore.RuntimePrepareEventType, agentcore.RuntimePrepareStartedEventType:
			notice.Type = EventAwakenRuntimePrepare
		case agentcore.RuntimePrepareProgressEventType:
			notice.Type = EventAwakenRuntimePrepareProgress
		case agentcore.RuntimeReadyEventType:
			notice.Type = EventAwakenRuntimeReady
		case agentcore.RuntimeRepairFailedEventType:
			notice.Type = EventAwakenRuntimeRepairFailed
		case agentcore.RuntimeMissingEventType:
			notice.Type = EventAwakenRuntimeMissing
		default:
			notice.Type = EventAwakenUnknown
			notice.RawType = string(eventType)
		}
		notice.Message = stringFromAny(payload["content"])
		notice.Progress = stringFromAny(payload["progress"])
		notice.ToolCallID = stringFromAny(payload["tool_call_id"])
		notice.ToolCallName = stringFromAny(payload["tool_name"])
		notice.RuntimeName = stringFromAny(payload["runtime_name"])
		notice.RuntimeVersion = stringFromAny(payload["runtime_version"])
		return []Event{notice}
	case agentcore.SemanticKindRecoveryDecision:
		recovery := base
		if boolFromAny(payload["blocked"]) {
			recovery.Type = EventAwakenToolRetryBlocked
		} else {
			recovery.Type = EventAwakenToolRecoveryRequired
		}
		recovery.ToolCallID = stringFromAny(payload["tool_call_id"])
		recovery.ToolCallName = stringFromAny(payload["tool_name"])
		recovery.Code = stringFromAny(payload["error_code"])
		recovery.ErrorCode = stringFromAny(payload["error_code"])
		recovery.Message = stringFromAny(payload["message"])
		if recovery.Message == "" {
			if boolFromAny(payload["blocked"]) {
				recovery.Message = "tool retry blocked"
			} else {
				recovery.Message = "tool recovery required"
			}
		}
		recovery.RecoveryOptions = stringSliceFromAny(payload["recovery_options"])
		recovery.ToolStatus = stringFromAny(payload["tool_status"])
		recovery.TransportStatus = stringFromAny(payload["transport_status"])
		recovery.SemanticStatus = stringFromAny(payload["semantic_status"])
		recovery.ProcessStatus = stringFromAny(payload["process_status"])
		recovery.ErrorCategory = stringFromAny(payload["error_category"])
		recovery.Retryable = boolFromAny(payload["retryable"])
		applyExecutionMetadata(&recovery, payload)
		return []Event{recovery}
	case agentcore.SemanticKindSkillActivated:
		invoked := skillEventFromPayload(base, payload)
		invoked.Type = EventAwakenSkillInvoked
		frames := []Event{invoked}
		if invoked.SkillStatus == "" || invoked.SkillStatus == "ok" {
			executionContext := invoked
			executionContext.Type = EventAwakenSkillExecutionContext
			frames = append(frames, executionContext)
		} else {
			failed := invoked
			failed.Type = EventAwakenSkillFailedToLoad
			frames = append(frames, failed)
		}
		return frames
	case agentcore.SemanticKindResourceReceipt:
		loaded := skillEventFromPayload(base, payload)
		loaded.Type = EventAwakenSkillContentLoaded
		loaded.SkillResource = stringFromAny(payload["resource"])
		loaded.SkillResourceSource = stringFromAny(payload["source"])
		loaded.SkillResourceSHA256 = stringFromAny(payload["sha256"])
		loaded.SkillResourceSize = int64FromAny(payload["size"])
		loaded.SkillResourceMode = stringFromAny(payload["mode"])
		return []Event{loaded}
	case agentcore.SemanticKindThread,
		agentcore.SemanticKindFileChange:
		return nil
	default:
		unknown := base
		unknown.Type = EventAwakenUnknown
		unknown.RawType = string(record.Kind)
		return []Event{unknown}
	}
}

func applyOutcomePayload(dst *Event, payload map[string]any) {
	outcomeRaw, ok := payload["outcome"]
	if !ok || outcomeRaw == nil {
		if boolFromAny(payload["is_error"]) {
			dst.ToolStatus = "failed"
		}
		return
	}
	raw, err := json.Marshal(outcomeRaw)
	if err != nil {
		return
	}
	var outcome struct {
		Status          string         `json:"status"`
		TransportStatus string         `json:"transport_status"`
		SemanticStatus  string         `json:"semantic_status"`
		ProcessStatus   string         `json:"process_status"`
		ExitCode        *int           `json:"exit_code"`
		TimedOut        bool           `json:"timed_out"`
		ErrorCode       string         `json:"error_code"`
		ErrorCategory   string         `json:"error_category"`
		Retryable       bool           `json:"retryable"`
		RecoveryOptions []string       `json:"recovery_options"`
		DurationMs      int64          `json:"duration_ms"`
		Diagnostic      map[string]any `json:"diagnostic"`
	}
	if err := json.Unmarshal(raw, &outcome); err != nil {
		return
	}
	dst.ToolStatus = outcome.Status
	dst.TransportStatus = outcome.TransportStatus
	dst.SemanticStatus = outcome.SemanticStatus
	dst.ProcessStatus = outcome.ProcessStatus
	dst.ProcessExitCode = outcome.ExitCode
	dst.ProcessTimedOut = outcome.TimedOut
	dst.ErrorCode = outcome.ErrorCode
	dst.ErrorCategory = outcome.ErrorCategory
	dst.Retryable = outcome.Retryable
	dst.RecoveryOptions = append([]string(nil), outcome.RecoveryOptions...)
	dst.DurationMs = outcome.DurationMs
	if len(outcome.Diagnostic) > 0 {
		dst.Diagnostic = make(map[string]any, len(outcome.Diagnostic))
		for key, value := range outcome.Diagnostic {
			dst.Diagnostic[key] = value
		}
	}
	var outcomePayload map[string]any
	if err := json.Unmarshal(raw, &outcomePayload); err == nil {
		applyExecutionMetadata(dst, outcomePayload)
	}
	applyExecutionMetadata(dst, outcome.Diagnostic)
}

func skillEventFromPayload(base Event, payload map[string]any) Event {
	event := base
	event.SkillID = stringFromAny(payload["skill_id"])
	event.SkillVersion = stringFromAny(payload["skill_version"])
	event.SkillSource = firstNonEmpty(stringFromAny(payload["skill_source"]), stringFromAny(payload["source"]))
	event.SkillPath = firstNonEmpty(
		stringFromAny(payload["skill_path"]),
		stringFromAny(payload["skill_file_path"]),
		stringFromAny(payload["path"]),
	)
	event.SkillStatus = firstNonEmpty(stringFromAny(payload["skill_status"]), stringFromAny(payload["compatibility_status"]), stringFromAny(payload["status"]))
	event.SkillEffectiveTools = stringSliceFromAny(payload["effective_tools"])
	event.SkillDiagnostics = diagnosticStringsFromAny(firstPresentValue(payload, "diagnostics", "compatibility_errors"))
	return event
}

func diagnosticStringsFromAny(value any) []string {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var values []any
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				out = append(out, trimmed)
			}
		case map[string]any:
			code := strings.TrimSpace(stringFromAny(typed["code"]))
			message := strings.TrimSpace(stringFromAny(typed["message"]))
			switch {
			case code != "" && message != "":
				out = append(out, code+": "+message)
			case message != "":
				out = append(out, message)
			case code != "":
				out = append(out, code)
			}
		}
	}
	return out
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	default:
		return ""
	}
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	default:
		return false
	}
}

func boolPointerFromAny(value any) *bool {
	valueBool, ok := value.(bool)
	if !ok {
		return nil
	}
	return &valueBool
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case uint:
		return int(typed)
	case uint8:
		return int(typed)
	case uint16:
		return int(typed)
	case uint32:
		return int(typed)
	case uint64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		value, err := typed.Int64()
		if err == nil {
			return int(value)
		}
	default:
		return 0
	}
	return 0
}

func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int8:
		return int64(typed)
	case int16:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case uint:
		return int64(typed)
	case uint8:
		return int64(typed)
	case uint16:
		return int64(typed)
	case uint32:
		return int64(typed)
	case uint64:
		return int64(typed)
	case float32:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		value, err := typed.Int64()
		if err == nil {
			return value
		}
	}
	return 0
}

func stringSliceFromAny(value any) []string {
	var values []any
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		values = typed
	default:
		raw, err := json.Marshal(value)
		if err != nil || json.Unmarshal(raw, &values) != nil {
			return nil
		}
	}
	out := make([]string, 0, len(values))
	for _, item := range values {
		if s := strings.TrimSpace(fmt.Sprint(item)); s != "" && s != "<nil>" {
			out = append(out, s)
		}
	}
	return out
}

// SemanticRecordsToAwakenEvents is used by runtime transcript packaging that still
// wraps agent events for compatibility with existing ThreadTranscriptRun shapes.
func SemanticRecordsToAwakenEvents(records []agentcore.SemanticRecord) []agentcore.AwakenEvent {
	frames := ProjectSemanticRecords(records)
	out := make([]agentcore.AwakenEvent, 0, len(frames))
	for _, frame := range frames {
		out = append(out, awakenEventFromAGUI(frame))
	}
	return out
}

func awakenEventFromAGUI(frame Event) agentcore.AwakenEvent {
	event := agentcore.AwakenEvent{
		Timestamp:               time.UnixMilli(frame.Timestamp).UTC(),
		ThreadID:                frame.ThreadID,
		RunID:                   frame.RunID,
		TurnID:                  frame.TurnID,
		MessageID:               frame.MessageID,
		ParentMessageID:         frame.ParentMessageID,
		Role:                    frame.Role,
		Delta:                   frame.Delta,
		Content:                 firstNonEmpty(frame.Content, frame.Result, frame.Message),
		ToolCallID:              frame.ToolCallID,
		ToolName:                frame.ToolCallName,
		Arguments:               frame.Delta,
		Diff:                    frame.Diff,
		Truncated:               frame.Truncated,
		Error:                   frame.Message,
		Code:                    frame.Code,
		TerminalReason:          agentcore.TerminalReason(frame.TerminalReason),
		ToolStatus:              frame.ToolStatus,
		TransportStatus:         frame.TransportStatus,
		SemanticStatus:          frame.SemanticStatus,
		ProcessStatus:           frame.ProcessStatus,
		ProcessExitCode:         frame.ProcessExitCode,
		ProcessTimedOut:         frame.ProcessTimedOut,
		ErrorCode:               frame.ErrorCode,
		ErrorCategory:           frame.ErrorCategory,
		Retryable:               frame.Retryable,
		RecoveryOptions:         append([]string(nil), frame.RecoveryOptions...),
		DurationMs:              frame.DurationMs,
		ArtifactContractVersion: frame.ArtifactContractVersion,
		ArtifactRole:            tool.ArtifactRole(frame.ArtifactRole),
		ArtifactProducer:        frame.ArtifactProducer,
		ArtifactID:              frame.ArtifactID,
		ArtifactTitle:           frame.ArtifactTitle,
		ArtifactDescription:     frame.ArtifactDescription,
		ArtifactKind:            frame.ArtifactKind,
		ArtifactPath:            frame.ArtifactPath,
		ArtifactContent:         frame.ArtifactContent,
		ArtifactStatus:          agentcore.ArtifactStatus(frame.ArtifactStatus),
	}
	switch frame.Type {
	case EventRunStarted:
		event.Type = agentcore.RunStartedEventType
	case EventRunFinished:
		event.Type = agentcore.RunFinishedEventType
		event.Content = frame.Result
	case EventRunError:
		event.Type = agentcore.RunErrorEventType
	case EventTextMessageStart:
		event.Type = agentcore.TextMessageStartedEventType
	case EventTextMessageContent:
		event.Type = agentcore.TextDeltaEventType
	case EventTextMessageEnd:
		event.Type = agentcore.TextMessageFinishedEventType
	case EventToolCallStart:
		event.Type = agentcore.ToolCallStartedEventType
	case EventToolCallArgs:
		event.Type = agentcore.ToolCallArgsEventType
		event.Arguments = frame.Delta
	case EventToolCallEnd:
		event.Type = agentcore.ToolCallFinishedEventType
	case EventToolCallResult:
		event.Type = agentcore.ToolResultEventType
	case EventAwakenArtifactCreated:
		event.Type = agentcore.ArtifactCreatedEventType
	case EventAwakenArtifactUpdated:
		event.Type = agentcore.ArtifactUpdatedEventType
	case EventAwakenTurnDiff:
		event.Type = agentcore.TurnDiffEventType
	case EventAwakenTurnFinished:
		event.Type = agentcore.TurnFinishedEventType
	case EventAwakenToolPermissionDecided:
		event.Type = agentcore.ToolPermissionDecidedEventType
	default:
		event.Type = agentcore.AwakenEventType(frame.RawType)
	}
	return event
}
