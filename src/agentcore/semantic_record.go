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
	SemanticKindThread                  SemanticKind = "thread"
	SemanticKindRunStarted              SemanticKind = "run_started"
	SemanticKindHookSnapshot            SemanticKind = "hook_snapshot"
	SemanticKindHookInvocationStarted   SemanticKind = "hook_invocation_started"
	SemanticKindHookInvocationCompleted SemanticKind = "hook_invocation_completed"
	SemanticKindUserMessage             SemanticKind = "user_message"
	SemanticKindReasoningSummary        SemanticKind = "reasoning_summary"
	SemanticKindAssistantMessage        SemanticKind = "assistant_message"
	SemanticKindToolCall                SemanticKind = "tool_call"
	SemanticKindToolResult              SemanticKind = "tool_result"
	SemanticKindArtifact                SemanticKind = "artifact"
	SemanticKindFileChange              SemanticKind = "file_change"
	SemanticKindTurnDiff                SemanticKind = "turn_diff"
	SemanticKindPermissionDecision      SemanticKind = "permission_decision"
	SemanticKindSkillActivated          SemanticKind = "skill_activated"
	SemanticKindResourceReceipt         SemanticKind = "resource_receipt"
	SemanticKindRuntimeNotice           SemanticKind = "runtime_notice"
	SemanticKindContextCompacted        SemanticKind = "context_compacted"
	SemanticKindRecoveryDecision        SemanticKind = "recovery_decision"
	SemanticKindTurnBoundary            SemanticKind = "turn_boundary"
	SemanticKindRunTerminal             SemanticKind = "run_terminal"
	SemanticKindPersistenceWarning      SemanticKind = "persistence_warning"
	SemanticKindMemoryRecalled          SemanticKind = "memory_recalled"
	SemanticKindMemoryMutated           SemanticKind = "memory_mutated"
	SemanticKindContextSnapshot         SemanticKind = "context_snapshot"
	SemanticKindModelRequest            SemanticKind = "model_request"
	SemanticKindModelUsage              SemanticKind = "model_usage"
	SemanticKindContextEpoch            SemanticKind = "context_epoch"
	SemanticKindProviderOpaqueState     SemanticKind = "provider_opaque_state"
	SemanticKindSubagentSpawned         SemanticKind = "subagent_spawned"
	SemanticKindSubagentStatusChanged   SemanticKind = "subagent_status_changed"
	SemanticKindSubagentMessageQueued   SemanticKind = "subagent_message_queued"
	SemanticKindSubagentProgress        SemanticKind = "subagent_progress"
	SemanticKindSubagentTerminal        SemanticKind = "subagent_terminal"
	SemanticKindACPSessionStarted       SemanticKind = "acp_session_started"
	SemanticKindACPSessionClosed        SemanticKind = "acp_session_closed"
	SemanticKindACPSessionInterrupted   SemanticKind = "acp_session_interrupted"
	SemanticKindACPTurnStarted          SemanticKind = "acp_turn_started"
	SemanticKindACPTurnFinished         SemanticKind = "acp_turn_finished"
	SemanticKindACPToolSnapshot         SemanticKind = "acp_tool_snapshot"
	SemanticKindACPPermissionRequested  SemanticKind = "acp_permission_requested"
	SemanticKindACPPermissionResolved   SemanticKind = "acp_permission_resolved"
	// SemanticKindScheduledNotice is the host-owned fixed-reminder fact of a
	// scheduled task (contract §9.4). The payload carries only task/execution
	// identity, the task name and the fixed content; it is never a user or
	// assistant message.
	SemanticKindScheduledNotice SemanticKind = "scheduled_notice"

	// SemanticKindPlanUpdated is the durable, sanitized run-plan snapshot fact
	// of one root run (contract: 运行计划与任务进度设计契约 §7.2). The payload
	// carries only the bounded snapshot, derived counters and tool_call_id —
	// never model reasoning or raw unvalidated JSON.
	SemanticKindPlanUpdated SemanticKind = "plan_updated"

	// Computer Use semantic kinds (docs/10_计算机操作能力/)
	SemanticKindComputerSessionOpened            SemanticKind = "computer_session_opened"
	SemanticKindComputerStateChanged             SemanticKind = "computer_state_changed"
	SemanticKindComputerObservationCaptured      SemanticKind = "computer_observation_captured"
	SemanticKindComputerActionPrepared           SemanticKind = "computer_action_prepared"
	SemanticKindComputerReviewRequested          SemanticKind = "computer_review_requested"
	SemanticKindComputerReviewResolved           SemanticKind = "computer_review_resolved"
	SemanticKindComputerActionStarted            SemanticKind = "computer_action_started"
	SemanticKindComputerActionCompleted          SemanticKind = "computer_action_completed"
	SemanticKindComputerControlTransferStarted   SemanticKind = "computer_control_transfer_started"
	SemanticKindComputerControlTransferCompleted SemanticKind = "computer_control_transfer_completed"
	SemanticKindComputerSessionClosed            SemanticKind = "computer_session_closed"
	SemanticKindComputerMediaAccessed            SemanticKind = "computer_media_accessed"
	SemanticKindComputerVerificationCompleted    SemanticKind = "computer_verification_completed"
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
	ID   string `json:"id"`
	Type string `json:"type"`
	// LeadingText preserves the prompt text segment that appears immediately
	// before the token so durable replay keeps the token's inline position.
	LeadingText string         `json:"leadingText,omitempty"`
	Label       string         `json:"label"`
	Payload     map[string]any `json:"payload,omitempty"`
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

// FlushedSemanticRecorder appends one safety-sensitive fact, flushes durable
// storage, and only then publishes the assigned record to live consumers.
// Facts whose UI must never get ahead of durable truth (for example Computer
// review gates and run-plan snapshots) use this optional production contract.
type FlushedSemanticRecorder interface {
	SemanticRecorder
	AppendFlushed(ctx context.Context, record SemanticRecord) (SemanticRecord, error)
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
