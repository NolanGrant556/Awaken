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
	SemanticKindThread                     SemanticKind = "thread"
	SemanticKindRunStarted                 SemanticKind = "run_started"
	SemanticKindUserMessage                SemanticKind = "user_message"
	SemanticKindAssistantMessage           SemanticKind = "assistant_message"
	SemanticKindToolCall                   SemanticKind = "tool_call"
	SemanticKindToolResult                 SemanticKind = "tool_result"
	SemanticKindArtifact                   SemanticKind = "artifact"
	SemanticKindFileChange                 SemanticKind = "file_change"
	SemanticKindTurnDiff                   SemanticKind = "turn_diff"
	SemanticKindPermissionDecision         SemanticKind = "permission_decision"
	SemanticKindSkillActivated             SemanticKind = "skill_activated"
	SemanticKindResourceReceipt            SemanticKind = "resource_receipt"
	SemanticKindMassMessageReviewRequested SemanticKind = "mass_message_review_requested"
	SemanticKindMassMessageReviewResolved  SemanticKind = "mass_message_review_resolved"
	SemanticKindRuntimeNotice              SemanticKind = "runtime_notice"
	SemanticKindRecoveryDecision           SemanticKind = "recovery_decision"
	SemanticKindTurnBoundary               SemanticKind = "turn_boundary"
	SemanticKindRunTerminal                SemanticKind = "run_terminal"
	SemanticKindPersistenceWarning         SemanticKind = "persistence_warning"
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

// SemanticUserMessageAttachment is the durable identity for an input
// attachment. Image bytes remain in the runtime-owned attachment store and
// are addressed by StorageKey; they are deliberately not duplicated into the
// SemanticJournal payload.
type SemanticUserMessageAttachment struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	MimeType    string `json:"mime_type"`
	Path        string `json:"path,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	ByteSize    int    `json:"byte_size,omitempty"`
	StorageKey  string `json:"storage_key,omitempty"`
}

// SemanticComposerToken is the durable renderer-independent form of a
// composer token. Payload stays open-ended so new token providers can preserve
// their identity without teaching Agent Core provider-specific fields.
type SemanticComposerToken struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Label   string         `json:"label"`
	Payload map[string]any `json:"payload,omitempty"`
}

// SemanticRecorder persists durable semantic facts. All methods return error.
// EventSink remains fire-and-forget for live broadcast after successful append.
type SemanticRecorder interface {
	Append(ctx context.Context, record SemanticRecord) error
	AppendBatch(ctx context.Context, records []SemanticRecord) error
	Flush(ctx context.Context) error
}

// AssignedSemanticRecorder exposes the exact records accepted by durable
// storage, including the sequence assigned while holding the journal lock.
// Production live projection must use these returned values rather than the
// caller's pre-append seq=0 copies.
type AssignedSemanticRecorder interface {
	SemanticRecorder
	AppendAssigned(ctx context.Context, record SemanticRecord) (SemanticRecord, error)
	AppendBatchAssigned(ctx context.Context, records []SemanticRecord) ([]SemanticRecord, error)
}

// CanonicalLiveSemanticRecorder marks a recorder that publishes successfully
// appended SemanticRecords to the live runtime stream. PersistDurable uses the
// marker to avoid also emitting its legacy hand-built AwakenEvent projection.
// Legacy v1 recorders intentionally do not implement this contract.
type CanonicalLiveSemanticRecorder interface {
	SemanticRecorder
	BroadcastsCanonicalSemanticRecords() bool
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
