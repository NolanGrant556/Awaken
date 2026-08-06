package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"awaken/src/agentcore"
	"awaken/src/tool"
)

// v1SemanticRecorder persists SemanticRecord facts into a legacy v1 JSONL transcript
// for threads that already have v1 history. It does not rewrite existing lines.
type v1SemanticRecorder struct {
	mu       sync.Mutex
	path     string
	threadID string
	pending  []agentcore.SemanticRecord
}

func newV1SemanticRecorder(path, threadID string) *v1SemanticRecorder {
	return &v1SemanticRecorder{path: path, threadID: threadID}
}

func (r *v1SemanticRecorder) Append(ctx context.Context, record agentcore.SemanticRecord) error {
	return r.AppendBatch(ctx, []agentcore.SemanticRecord{record})
}

func (r *v1SemanticRecorder) AppendBatch(ctx context.Context, records []agentcore.SemanticRecord) error {
	if len(records) == 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	r.pending = append(r.pending, records...)
	if err := r.flushPending(); err != nil {
		return err
	}
	return nil
}

func (r *v1SemanticRecorder) Flush(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.flushPending()
}

func (r *v1SemanticRecorder) Close() error { return nil }

func (r *v1SemanticRecorder) flushPending() error {
	if len(r.pending) == 0 {
		return nil
	}
	out := make([]TranscriptRecord, 0, len(r.pending))
	for _, record := range r.pending {
		converted, ok := semanticRecordToV1Transcript(record)
		if !ok {
			continue
		}
		out = append(out, converted...)
	}
	if len(out) == 0 {
		r.pending = r.pending[:0]
		return nil
	}
	if err := appendTranscriptRecords(r.path, out); err != nil {
		return err
	}
	r.pending = r.pending[:0]
	return nil
}

func semanticRecordToV1Transcript(record agentcore.SemanticRecord) ([]TranscriptRecord, bool) {
	ts := record.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	base := TranscriptRecord{
		SessionID: record.SessionID,
		ThreadID:  firstNonEmpty(record.ThreadID, ""),
		RunID:     record.RunID,
		TurnID:    record.TurnID,
		Timestamp: ts,
	}
	switch record.Kind {
	case agentcore.SemanticKindThread:
		base.RecordType = transcriptRecordTypeThread
		base.Content = stringPayload(record.Payload, "label")
		return []TranscriptRecord{base}, true
	case agentcore.SemanticKindUserMessage:
		base.RecordType = transcriptRecordTypeMessage
		base.Role = "user"
		base.Content = stringPayload(record.Payload, "content")
		base.Attachments = cloneTranscriptAttachments(
			transcriptAttachmentsFromSemanticUserMessage(record.Payload),
		)
		base.ComposerTokens = composerTokensFromSemanticUserMessage(record.Payload)
		base.MessageID = firstNonEmpty(record.RecordID, fmt.Sprintf("msg_%s", record.RecordID))
		return []TranscriptRecord{base}, true
	case agentcore.SemanticKindReasoningSummary:
		content := stringPayload(record.Payload, "content")
		if strings.TrimSpace(content) == "" {
			return nil, false
		}
		event := agentcore.AwakenEvent{
			Type:      agentcore.ReasoningDeltaEventType,
			Timestamp: ts,
			ThreadID:  record.ThreadID,
			RunID:     record.RunID,
			TurnID:    record.TurnID,
			MessageID: firstNonEmpty(stringPayload(record.Payload, "message_id"), record.RecordID),
			Delta:     content,
		}
		return []TranscriptRecord{eventRecord(base, event)}, true
	case agentcore.SemanticKindAssistantMessage:
		base.RecordType = transcriptRecordTypeMessage
		base.Role = "assistant"
		base.Content = stringPayload(record.Payload, "content")
		base.MessageID = firstNonEmpty(stringPayload(record.Payload, "message_id"), record.RecordID)
		return []TranscriptRecord{base}, true
	case agentcore.SemanticKindToolCall:
		event := agentcore.AwakenEvent{
			Type:            agentcore.ToolCallStartedEventType,
			Timestamp:       ts,
			ThreadID:        record.ThreadID,
			RunID:           record.RunID,
			TurnID:          record.TurnID,
			ToolCallID:      stringPayload(record.Payload, "tool_call_id"),
			ToolName:        stringPayload(record.Payload, "tool_name"),
			Arguments:       stringPayload(record.Payload, "arguments"),
			ParentMessageID: record.ParentID,
		}
		args := event
		args.Type = agentcore.ToolCallArgsEventType
		end := event
		end.Type = agentcore.ToolCallFinishedEventType
		return []TranscriptRecord{
			eventRecord(base, event),
			eventRecord(base, args),
			eventRecord(base, end),
		}, true
	case agentcore.SemanticKindToolResult:
		event := agentcore.AwakenEvent{
			Type:       agentcore.ToolResultEventType,
			Timestamp:  ts,
			ThreadID:   record.ThreadID,
			RunID:      record.RunID,
			TurnID:     record.TurnID,
			ToolCallID: stringPayload(record.Payload, "tool_call_id"),
			ToolName:   stringPayload(record.Payload, "tool_name"),
			Content:    stringPayload(record.Payload, "content"),
			Role:       "tool",
		}
		if outcome, ok := record.Payload["outcome"].(map[string]any); ok {
			event.ToolStatus = stringPayload(outcome, "status")
			event.TransportStatus = stringPayload(outcome, "transport_status")
			event.SemanticStatus = stringPayload(outcome, "semantic_status")
			event.ProcessStatus = stringPayload(outcome, "process_status")
			event.ErrorCode = stringPayload(outcome, "error_code")
			event.ErrorCategory = stringPayload(outcome, "error_category")
			event.Retryable = boolPayload(outcome, "retryable")
		} else if boolPayload(record.Payload, "is_error") {
			event.ToolStatus = "failed"
		}
		// Also keep message-layer tool result for conversation restore.
		msg := base
		msg.RecordType = transcriptRecordTypeMessage
		msg.Role = "tool"
		msg.ToolCallID = event.ToolCallID
		msg.ToolName = event.ToolName
		msg.Content = event.Content
		msg.IsError = boolPayload(record.Payload, "is_error") || strings.EqualFold(event.ToolStatus, "failed")
		msg.MessageID = record.RecordID
		return []TranscriptRecord{eventRecord(base, event), msg}, true
	case agentcore.SemanticKindArtifact:
		eventType := agentcore.ArtifactUpdatedEventType
		if boolPayload(record.Payload, "created") {
			eventType = agentcore.ArtifactCreatedEventType
		}
		producer := firstNonEmpty(stringPayload(record.Payload, "producer"), stringPayload(record.Payload, "tool_name"))
		artifactKind := stringPayload(record.Payload, "kind")
		artifactPath := stringPayload(record.Payload, "path")
		event := agentcore.AwakenEvent{
			Type:                    eventType,
			Timestamp:               ts,
			ThreadID:                record.ThreadID,
			RunID:                   record.RunID,
			TurnID:                  record.TurnID,
			ToolCallID:              stringPayload(record.Payload, "tool_call_id"),
			ToolName:                stringPayload(record.Payload, "tool_name"),
			ArtifactContractVersion: tool.ArtifactContractVersionOrLegacy(intPayload(record.Payload, "contract_version")),
			ArtifactRole:            tool.ResolveArtifactRole(tool.ArtifactRole(stringPayload(record.Payload, "role")), producer, artifactKind, artifactPath),
			ArtifactProducer:        producer,
			ArtifactID:              stringPayload(record.Payload, "artifact_id"),
			ArtifactGroupID:         stringPayload(record.Payload, "group_id"),
			ArtifactGroupIndex:      intPayload(record.Payload, "group_index"),
			ArtifactTitle:           stringPayload(record.Payload, "title"),
			ArtifactDescription:     stringPayload(record.Payload, "description"),
			ArtifactKind:            artifactKind,
			ArtifactPath:            artifactPath,
			ArtifactContent:         stringPayload(record.Payload, "content"),
			ArtifactStatus:          agentcore.ArtifactStatus(stringPayload(record.Payload, "status")),
		}
		return []TranscriptRecord{eventRecord(base, event)}, true
	case agentcore.SemanticKindTurnDiff:
		event := agentcore.AwakenEvent{
			Type:      agentcore.TurnDiffEventType,
			Timestamp: ts,
			ThreadID:  record.ThreadID,
			RunID:     record.RunID,
			TurnID:    record.TurnID,
			Diff:      stringPayload(record.Payload, "diff"),
			Truncated: boolPayload(record.Payload, "truncated"),
		}
		return []TranscriptRecord{eventRecord(base, event)}, true
	case agentcore.SemanticKindContextSnapshot:
		encoded, err := json.Marshal(record.Payload)
		if err != nil {
			return nil, false
		}
		var snapshot agentcore.ContextUsageSnapshot
		if json.Unmarshal(encoded, &snapshot) != nil {
			return nil, false
		}
		event := agentcore.AwakenEvent{
			Type: agentcore.ContextSnapshotEventType, Timestamp: ts,
			ThreadID: record.ThreadID, RunID: record.RunID, TurnID: record.TurnID,
			ContextUsageSnapshot: &snapshot,
		}
		return []TranscriptRecord{eventRecord(base, event)}, true
	case agentcore.SemanticKindTurnBoundary:
		event := agentcore.AwakenEvent{
			Type:             agentcore.TurnFinishedEventType,
			Timestamp:        ts,
			ThreadID:         record.ThreadID,
			RunID:            record.RunID,
			TurnID:           record.TurnID,
			TerminalReason:   agentcore.TerminalReason(stringPayload(record.Payload, "terminal_reason")),
			TransitionReason: agentcore.TransitionReason(stringPayload(record.Payload, "transition_reason")),
		}
		return []TranscriptRecord{eventRecord(base, event)}, true
	case agentcore.SemanticKindRunStarted:
		event := agentcore.AwakenEvent{
			Type:      agentcore.RunStartedEventType,
			Timestamp: ts,
			ThreadID:  record.ThreadID,
			RunID:     record.RunID,
		}
		return []TranscriptRecord{eventRecord(base, event)}, true
	case agentcore.SemanticKindRunTerminal:
		status := stringPayload(record.Payload, "status")
		if status == "finished" {
			event := agentcore.AwakenEvent{
				Type:           agentcore.RunFinishedEventType,
				Timestamp:      ts,
				ThreadID:       record.ThreadID,
				RunID:          record.RunID,
				Content:        stringPayload(record.Payload, "result"),
				TerminalReason: agentcore.TerminalReason(stringPayload(record.Payload, "terminal_reason")),
			}
			return []TranscriptRecord{eventRecord(base, event)}, true
		}
		event := agentcore.AwakenEvent{
			Type:           agentcore.RunErrorEventType,
			Timestamp:      ts,
			ThreadID:       record.ThreadID,
			RunID:          record.RunID,
			Error:          firstNonEmpty(stringPayload(record.Payload, "error"), status),
			Code:           stringPayload(record.Payload, "code"),
			Content:        stringPayload(record.Payload, "result"),
			TerminalReason: agentcore.TerminalReason(firstNonEmpty(stringPayload(record.Payload, "terminal_reason"), stringPayload(record.Payload, "reason"))),
		}
		return []TranscriptRecord{eventRecord(base, event)}, true
	default:
		return nil, false
	}
}

func eventRecord(base TranscriptRecord, event agentcore.AwakenEvent) TranscriptRecord {
	base.RecordType = transcriptRecordTypeEvent
	cloned := event
	base.Event = &cloned
	base.MessageID = fmt.Sprintf("evt_%s_%s", event.Type, firstNonEmpty(event.ToolCallID, event.MessageID, fmt.Sprintf("%d", time.Now().UnixNano())))
	base.TerminalReason = event.TerminalReason
	return base
}

func openThreadRecorder(path, threadID string) (agentcore.SemanticRecorder, error) {
	schema, err := DetectJournalSchema(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if schema == 1 {
		return newV1SemanticRecorder(path, threadID), nil
	}
	journal, err := NewExecutionJournal(path, threadID)
	if err != nil {
		return nil, err
	}
	return journal, nil
}
