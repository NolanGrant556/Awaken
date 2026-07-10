package agui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"awaken/src/agentcore"
)

// ProjectSemanticRecords converts durable SemanticRecord values into AG-UI events.
// Live SSE and transcript replay must share this projector.
func ProjectSemanticRecords(records []agentcore.SemanticRecord) []Event {
	out := make([]Event, 0, len(records)*2)
	for _, record := range records {
		out = append(out, ProjectSemanticRecord(record)...)
	}
	return out
}

// ProjectSemanticRecord projects one SemanticRecord into zero or more AG-UI frames.
func ProjectSemanticRecord(record agentcore.SemanticRecord) []Event {
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
		applyOutcomePayload(&result, payload)
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
		artifact.ArtifactID = stringFromAny(payload["artifact_id"])
		artifact.ArtifactTitle = stringFromAny(payload["title"])
		artifact.ArtifactDescription = stringFromAny(payload["description"])
		artifact.ArtifactKind = stringFromAny(payload["kind"])
		artifact.ArtifactPath = stringFromAny(payload["path"])
		artifact.ArtifactContent = stringFromAny(payload["content"])
		artifact.ArtifactStatus = stringFromAny(payload["status"])
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
		return []Event{decision}
	case agentcore.SemanticKindPersistenceWarning:
		warning := base
		warning.Type = EventAwakenUnknown
		warning.RawType = "persistence_warning"
		warning.Message = stringFromAny(payload["error"])
		return []Event{warning}
	case agentcore.SemanticKindRuntimeNotice:
		notice := base
		notice.Type = EventAwakenUnknown
		notice.RawType = stringFromAny(payload["event_type"])
		notice.Message = stringFromAny(payload["content"])
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
		if opts, ok := payload["recovery_options"].([]any); ok {
			for _, opt := range opts {
				if s := strings.TrimSpace(fmt.Sprint(opt)); s != "" && s != "<nil>" {
					recovery.RecoveryOptions = append(recovery.RecoveryOptions, s)
				}
			}
		}
		return []Event{recovery}
	case agentcore.SemanticKindThread,
		agentcore.SemanticKindFileChange,
		agentcore.SemanticKindSkillActivated,
		agentcore.SemanticKindResourceReceipt:
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
		Status          string   `json:"status"`
		TransportStatus string   `json:"transport_status"`
		SemanticStatus  string   `json:"semantic_status"`
		ProcessStatus   string   `json:"process_status"`
		ExitCode        *int     `json:"exit_code"`
		TimedOut        bool     `json:"timed_out"`
		ErrorCode       string   `json:"error_code"`
		ErrorCategory   string   `json:"error_category"`
		Retryable       bool     `json:"retryable"`
		RecoveryOptions []string `json:"recovery_options"`
		DurationMs      int64    `json:"duration_ms"`
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
		Timestamp:       time.UnixMilli(frame.Timestamp).UTC(),
		ThreadID:        frame.ThreadID,
		RunID:           frame.RunID,
		TurnID:          frame.TurnID,
		MessageID:       frame.MessageID,
		ParentMessageID: frame.ParentMessageID,
		Role:            frame.Role,
		Delta:           frame.Delta,
		Content:         firstNonEmpty(frame.Content, frame.Result, frame.Message),
		ToolCallID:      frame.ToolCallID,
		ToolName:        frame.ToolCallName,
		Arguments:       frame.Delta,
		Diff:            frame.Diff,
		Truncated:       frame.Truncated,
		Error:           frame.Message,
		Code:            frame.Code,
		TerminalReason:  agentcore.TerminalReason(frame.TerminalReason),
		ToolStatus:      frame.ToolStatus,
		TransportStatus: frame.TransportStatus,
		SemanticStatus:  frame.SemanticStatus,
		ProcessStatus:   frame.ProcessStatus,
		ProcessExitCode: frame.ProcessExitCode,
		ProcessTimedOut: frame.ProcessTimedOut,
		ErrorCode:       frame.ErrorCode,
		ErrorCategory:   frame.ErrorCategory,
		Retryable:       frame.Retryable,
		RecoveryOptions: append([]string(nil), frame.RecoveryOptions...),
		DurationMs:      frame.DurationMs,
		ArtifactID:      frame.ArtifactID,
		ArtifactTitle:   frame.ArtifactTitle,
		ArtifactDescription: frame.ArtifactDescription,
		ArtifactKind:    frame.ArtifactKind,
		ArtifactPath:    frame.ArtifactPath,
		ArtifactContent: frame.ArtifactContent,
		ArtifactStatus:  agentcore.ArtifactStatus(frame.ArtifactStatus),
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
