package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/tool"
)

// LegacyTranscriptAdapter projects v1 dual message+event JSONL into SemanticRecord.
// It does not invent missing Artifacts or new ToolOutcome fields.
type LegacyTranscriptAdapter struct{}

func (LegacyTranscriptAdapter) ReadFile(path string) ([]agentcore.SemanticRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return (LegacyTranscriptAdapter{}).ProjectJSONL(string(data))
}

func (LegacyTranscriptAdapter) ProjectJSONL(content string) ([]agentcore.SemanticRecord, error) {
	lines := strings.Split(content, "\n")
	var v1Records []TranscriptRecord
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		var record TranscriptRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			isLast := true
			for _, later := range lines[i+1:] {
				if strings.TrimSpace(later) != "" {
					isLast = false
					break
				}
			}
			if isLast {
				break
			}
			return nil, fmt.Errorf("%w at line %d: %v", ErrJournalMiddleCorruption, i+1, err)
		}
		v1Records = append(v1Records, record)
	}
	return projectV1TranscriptRecords(v1Records), nil
}

func projectV1TranscriptRecords(records []TranscriptRecord) []agentcore.SemanticRecord {
	out := make([]agentcore.SemanticRecord, 0, len(records)*2)
	var seq uint64 = 1
	appendRec := func(kind agentcore.SemanticKind, base TranscriptRecord, parentID string, payload map[string]any) {
		ts := base.Timestamp
		if ts.IsZero() {
			ts = time.Now().UTC()
		}
		recordID := base.MessageID
		if recordID == "" {
			recordID = agentcore.NewSemanticRecordID(string(kind))
		} else {
			recordID = fmt.Sprintf("v1_%s_%s", kind, recordID)
		}
		out = append(out, agentcore.SemanticRecord{
			SchemaVersion: agentcore.SemanticSchemaVersion,
			RecordID:      recordID,
			Seq:           seq,
			SessionID:     base.SessionID,
			ThreadID:      base.ThreadID,
			RunID:         base.RunID,
			TurnID:        base.TurnID,
			ParentID:      parentID,
			Kind:          kind,
			Timestamp:     ts.UTC(),
			Payload:       payload,
		})
		seq++
	}

	for _, record := range records {
		switch record.RecordType {
		case transcriptRecordTypeThread:
			appendRec(agentcore.SemanticKindThread, record, "", map[string]any{
				"label": trimThreadLabel(record.Content),
			})
		case transcriptRecordTypeMessage:
			switch record.Role {
			case string(convcontext.RoleUser):
				payload := map[string]any{
					"content": record.Content,
					"role":    record.Role,
				}
				if len(record.Attachments) > 0 {
					payload["attachment_count"] = len(record.Attachments)
					payload["attachments"] = cloneTranscriptAttachments(record.Attachments)
				}
				if len(record.ComposerTokens) > 0 {
					payload["composer_tokens"] = cloneComposerTokens(record.ComposerTokens)
				}
				appendRec(agentcore.SemanticKindUserMessage, record, record.ParentMessageID, payload)
			case string(convcontext.RoleAssistant):
				payload := map[string]any{
					"content":    record.Content,
					"role":       record.Role,
					"message_id": record.MessageID,
				}
				appendRec(agentcore.SemanticKindAssistantMessage, record, record.ParentMessageID, payload)
				for _, call := range record.ToolCalls {
					toolPayload := map[string]any{
						"tool_call_id": call.ID,
						"tool_name":    call.Function.Name,
						"arguments":    call.Function.Arguments,
					}
					appendRec(agentcore.SemanticKindToolCall, record, record.MessageID, toolPayload)
				}
			case string(convcontext.RoleTool):
				payload := map[string]any{
					"tool_call_id": record.ToolCallID,
					"tool_name":    record.ToolName,
					"content":      record.Content,
					"is_error":     record.IsError,
				}
				// Preserve IsError only; do not invent missing outcome fields.
				appendRec(agentcore.SemanticKindToolResult, record, record.ParentMessageID, payload)
			}
		case transcriptRecordTypeEvent:
			if record.Event == nil {
				continue
			}
			projectV1Event(&out, &seq, record)
		}
	}
	return out
}

func projectV1Event(out *[]agentcore.SemanticRecord, seq *uint64, record TranscriptRecord) {
	event := *record.Event
	ts := record.Timestamp
	if ts.IsZero() {
		ts = event.Timestamp
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	base := agentcore.SemanticRecord{
		SchemaVersion: agentcore.SemanticSchemaVersion,
		SessionID:     record.SessionID,
		ThreadID:      firstNonEmpty(record.ThreadID, event.ThreadID),
		RunID:         firstNonEmpty(record.RunID, event.RunID),
		TurnID:        firstNonEmpty(record.TurnID, event.TurnID),
		ParentID:      record.ParentMessageID,
		Timestamp:     ts.UTC(),
	}
	switch event.Type {
	case agentcore.RunStartedEventType:
		appendProjected(out, seq, base, agentcore.SemanticKindRunStarted, map[string]any{})
	case agentcore.RunFinishedEventType:
		appendProjected(out, seq, base, agentcore.SemanticKindRunTerminal, map[string]any{
			"status":          "finished",
			"result":          event.Content,
			"terminal_reason": string(firstNonEmptyTerminal(record.TerminalReason, event.TerminalReason)),
		})
	case agentcore.RunErrorEventType:
		status := "error"
		if event.Code == "RUN_CANCELLED" {
			status = "cancelled"
		}
		appendProjected(out, seq, base, agentcore.SemanticKindRunTerminal, map[string]any{
			"status":          status,
			"error":           event.Error,
			"code":            event.Code,
			"result":          event.Content,
			"terminal_reason": string(firstNonEmptyTerminal(record.TerminalReason, event.TerminalReason)),
		})
	case agentcore.TurnFinishedEventType:
		appendProjected(out, seq, base, agentcore.SemanticKindTurnBoundary, map[string]any{
			"terminal_reason":   string(event.TerminalReason),
			"transition_reason": string(event.TransitionReason),
		})
	case agentcore.TextMessageStartedEventType, agentcore.TextDeltaEventType, agentcore.TextMessageFinishedEventType:
		// Text streaming is ephemeral in v2; final assistant_message comes from message records.
		// Keep a compact assistant_message only when message layer lacked content (handled elsewhere).
	case agentcore.TurnDiffEventType:
		appendProjected(out, seq, base, agentcore.SemanticKindTurnDiff, map[string]any{
			"diff":      event.Diff,
			"truncated": event.Truncated,
		})
	default:
		// Other recoverable events stay as runtime_notice for audit continuity.
		if isRecoverableTranscriptEvent(event.Type) {
			appendProjected(out, seq, base, agentcore.SemanticKindRuntimeNotice, map[string]any{
				"event_type": string(event.Type),
				"content":    event.Content,
			})
		}
	}
}

func appendProjected(out *[]agentcore.SemanticRecord, seq *uint64, base agentcore.SemanticRecord, kind agentcore.SemanticKind, payload map[string]any) {
	base.Kind = kind
	base.RecordID = agentcore.NewSemanticRecordID(string(kind))
	base.Seq = *seq
	base.Payload = payload
	*out = append(*out, base)
	*seq++
}

func firstNonEmptyTerminal(values ...agentcore.TerminalReason) agentcore.TerminalReason {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// SyntheticInterruptedToolResult builds the orphan-run recovery outcome.
func SyntheticInterruptedToolResult() tool.ExecutionOutcome {
	now := time.Now().UTC()
	return tool.ExecutionOutcome{
		Content:         "tool execution interrupted by process restart; outcome unknown",
		Status:          tool.OutcomeInterrupted,
		TransportStatus: tool.TransportUnknown,
		SemanticStatus:  tool.SemanticUnknown,
		ProcessStatus:   tool.ProcessUnknown,
		ErrorCode:       "process_restarted_outcome_unknown",
		ErrorCategory:   tool.ErrorCategoryPersistence,
		Retryable:       false,
		Diagnostic: map[string]any{
			"side_effect_state": "unknown",
		},
		StartedAt:  now,
		FinishedAt: now,
		DurationMs: 0,
	}
}
