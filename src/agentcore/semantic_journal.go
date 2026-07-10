package agentcore

import (
	"context"
	"encoding/json"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/tool"
)

// PersistDurable appends a durable semantic record then broadcasts the live event.
// Ephemeral events should call sink.Emit directly.
func PersistDurable(
	ctx context.Context,
	recorder SemanticRecorder,
	sink EventSink,
	record SemanticRecord,
	liveEvents ...AwakenEvent,
) error {
	if recorder == nil {
		return ErrPersistenceUnavailable
	}
	if err := recorder.Append(ctx, record); err != nil {
		return err
	}
	emitLive(sink, liveEvents...)
	return nil
}

// PersistDurableBatch appends durable records then broadcasts live events.
func PersistDurableBatch(
	ctx context.Context,
	recorder SemanticRecorder,
	sink EventSink,
	records []SemanticRecord,
	liveEvents ...AwakenEvent,
) error {
	if recorder == nil {
		return ErrPersistenceUnavailable
	}
	if err := recorder.AppendBatch(ctx, records); err != nil {
		return err
	}
	emitLive(sink, liveEvents...)
	return nil
}

func emitLive(sink EventSink, events ...AwakenEvent) {
	sink = normalizeEventSink(sink)
	for _, event := range events {
		if event.Timestamp.IsZero() {
			event.Timestamp = time.Now().UTC()
		}
		sink.Emit(event)
	}
}

func NewSemanticRecord(kind SemanticKind, threadID, runID, turnID, parentID, sessionID string, payload map[string]any) SemanticRecord {
	if payload == nil {
		payload = map[string]any{}
	}
	return SemanticRecord{
		SchemaVersion: SemanticSchemaVersion,
		RecordID:      NewSemanticRecordID(string(kind)),
		SessionID:     sessionID,
		ThreadID:      threadID,
		RunID:         runID,
		TurnID:        turnID,
		ParentID:      parentID,
		Kind:          kind,
		Timestamp:     time.Now().UTC(),
		Payload:       payload,
	}
}

func ToolCallSemanticRecord(threadID, runID, turnID, sessionID, parentID string, call convcontext.ToolCall) SemanticRecord {
	return NewSemanticRecord(SemanticKindToolCall, threadID, runID, turnID, parentID, sessionID, map[string]any{
		"tool_call_id": call.ID,
		"tool_name":    call.Function.Name,
		"arguments":    call.Function.Arguments,
	})
}

func ToolResultSemanticRecord(threadID, runID, turnID, sessionID string, result ToolExecutionResult) SemanticRecord {
	payload := map[string]any{
		"tool_call_id": result.ToolCallID,
		"tool_name":    result.Name,
		"content":      result.Content,
		"is_error":     result.IsError,
	}
	if result.Outcome.Status != "" {
		payload["outcome"] = outcomeToMap(result.Outcome)
	}
	if len(result.TrashedPaths) > 0 {
		payload["trashed_paths"] = append([]string(nil), result.TrashedPaths...)
	}
	return NewSemanticRecord(SemanticKindToolResult, threadID, runID, turnID, result.ToolCallID, sessionID, payload)
}

func ArtifactSemanticRecord(threadID, runID, turnID, sessionID string, event AwakenEvent) SemanticRecord {
	return NewSemanticRecord(SemanticKindArtifact, threadID, runID, turnID, event.ToolCallID, sessionID, map[string]any{
		"artifact_id":          event.ArtifactID,
		"title":                event.ArtifactTitle,
		"description":          event.ArtifactDescription,
		"kind":                 event.ArtifactKind,
		"path":                 event.ArtifactPath,
		"content":              event.ArtifactContent,
		"status":               string(event.ArtifactStatus),
		"tool_call_id":         event.ToolCallID,
		"tool_name":            event.ToolName,
		"created":              event.Type == ArtifactCreatedEventType,
	})
}

func outcomeToMap(outcome tool.ExecutionOutcome) map[string]any {
	raw, err := json.Marshal(outcome)
	if err != nil {
		return map[string]any{
			"status":     string(outcome.Status),
			"error_code": outcome.ErrorCode,
			"content":    outcome.Content,
		}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{
			"status":     string(outcome.Status),
			"error_code": outcome.ErrorCode,
			"content":    outcome.Content,
		}
	}
	return out
}

// artifactBufferingSink captures Artifact* events during tool execution so they
// can be persisted in the same AppendBatch as tool_result.
type artifactBufferingSink struct {
	inner     EventSink
	artifacts []AwakenEvent
}

func (s *artifactBufferingSink) Emit(event AwakenEvent) {
	switch event.Type {
	case ArtifactCreatedEventType, ArtifactUpdatedEventType:
		s.artifacts = append(s.artifacts, event)
		return
	default:
		s.inner.Emit(event)
	}
}

func (s *artifactBufferingSink) AwaitUserQuestion(ctx context.Context, request AwakenEvent) (string, error) {
	if awaiter, ok := s.inner.(UserQuestionAwaiter); ok {
		return awaiter.AwaitUserQuestion(ctx, request)
	}
	return "", context.Canceled
}

func (s *artifactBufferingSink) AwaitMassMessageReview(ctx context.Context, request AwakenEvent) (ReviewDecision, error) {
	if awaiter, ok := s.inner.(MassMessageReviewAwaiter); ok {
		return awaiter.AwaitMassMessageReview(ctx, request)
	}
	return "", context.Canceled
}
