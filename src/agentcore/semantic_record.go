package agentcore

import (
	"context"
	"errors"
	"strings"
	"time"
)

const SemanticSchemaVersion = 2

// SemanticKind is the stable kind set for SemanticJournal v2.
type SemanticKind string

const (
	SemanticKindThread             SemanticKind = "thread"
	SemanticKindRunStarted         SemanticKind = "run_started"
	SemanticKindUserMessage        SemanticKind = "user_message"
	SemanticKindAssistantMessage   SemanticKind = "assistant_message"
	SemanticKindToolCall           SemanticKind = "tool_call"
	SemanticKindToolResult         SemanticKind = "tool_result"
	SemanticKindArtifact           SemanticKind = "artifact"
	SemanticKindFileChange         SemanticKind = "file_change"
	SemanticKindTurnDiff           SemanticKind = "turn_diff"
	SemanticKindPermissionDecision SemanticKind = "permission_decision"
	SemanticKindSkillActivated     SemanticKind = "skill_activated"
	SemanticKindResourceReceipt    SemanticKind = "resource_receipt"
	SemanticKindRuntimeNotice      SemanticKind = "runtime_notice"
	SemanticKindRecoveryDecision   SemanticKind = "recovery_decision"
	SemanticKindTurnBoundary       SemanticKind = "turn_boundary"
	SemanticKindRunTerminal        SemanticKind = "run_terminal"
	SemanticKindPersistenceWarning SemanticKind = "persistence_warning"
)

// SemanticRecord is the canonical durable execution-journal record (schema_version=2).
type SemanticRecord struct {
	SchemaVersion int            `json:"schema_version"`
	RecordID      string         `json:"record_id"`
	Seq           uint64         `json:"seq"`
	SessionID     string         `json:"session_id,omitempty"`
	ThreadID      string         `json:"thread_id"`
	RunID         string         `json:"run_id,omitempty"`
	TurnID        string         `json:"turn_id,omitempty"`
	ParentID      string         `json:"parent_id,omitempty"`
	Kind          SemanticKind   `json:"kind"`
	Timestamp     time.Time      `json:"timestamp"`
	Payload       map[string]any `json:"payload,omitempty"`
}

// SemanticRecorder persists durable semantic facts. All methods return error.
// EventSink remains fire-and-forget for live broadcast after successful append.
type SemanticRecorder interface {
	Append(ctx context.Context, record SemanticRecord) error
	AppendBatch(ctx context.Context, records []SemanticRecord) error
	Flush(ctx context.Context) error
}

// NoopSemanticRecorder is for pure unit tests without side effects only.
// Formal StartRun must not use this; missing recorder returns persistence_unavailable.
type NoopSemanticRecorder struct{}

func (NoopSemanticRecorder) Append(context.Context, SemanticRecord) error { return nil }

func (NoopSemanticRecorder) AppendBatch(context.Context, []SemanticRecord) error { return nil }

func (NoopSemanticRecorder) Flush(context.Context) error { return nil }

// ErrPersistenceUnavailable is returned when a formal run lacks a SemanticRecorder.
var ErrPersistenceUnavailable = errors.New("persistence_unavailable")

// NewSemanticRecordID returns a unique record_id for a semantic journal entry.
func NewSemanticRecordID(kind string) string {
	prefix := "rec"
	if strings.TrimSpace(kind) != "" {
		prefix = "rec_" + kind
	}
	return newAwakenID(prefix)
}
