package subagentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type TaskStatus string

const (
	StatusQueued      TaskStatus = "queued"
	StatusRunning     TaskStatus = "running"
	StatusCompleted   TaskStatus = "completed"
	StatusFailed      TaskStatus = "failed"
	StatusCancelling  TaskStatus = "cancelling"
	StatusCancelled   TaskStatus = "cancelled"
	StatusInterrupted TaskStatus = "interrupted"
)

const (
	ErrorInvalidInput       = "subagent_invalid_input"
	ErrorRuntimeUnavailable = "subagent_runtime_unavailable"
	ErrorLimitReached       = "subagent_limit_reached"
	ErrorNotFound           = "subagent_not_found"
	ErrorNotRunning         = "subagent_not_running"
	ErrorWaitTimeout        = "subagent_wait_timeout"
	ErrorCancelled          = "subagent_cancelled"
	ErrorInterrupted        = "subagent_interrupted"
	ErrorWorkspaceMode      = "subagent_workspace_mode_unsupported"
	ErrorWorktreeFailed     = "subagent_worktree_failed"
	ErrorBoundaryViolation  = "subagent_boundary_violation"
	ErrorPersistence        = "subagent_persistence_failed"
	ErrorIdempotency        = "subagent_idempotency_conflict"
)

const (
	AgentTypeExplore = "explore"
	AgentTypePlan    = "plan"
	AgentTypeReview  = "review"
	AgentTypeGeneral = "general"
)

const (
	WorkspaceSharedReadOnly = "shared_readonly"
	WorkspaceShared         = "shared"
	WorkspaceGitWorktree    = "git_worktree"
)

type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) == "" {
		return e.Code
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func ErrorCode(err error) string {
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return ""
}

type SpawnRequest struct {
	RoleSnapshot *Role `json:"-"`
	// Legacy input only; new assignments always use the parent execution lifetime.
	Background       bool   `json:"background,omitempty"`
	ContinuesFrom    string `json:"-"`
	ContextMode      string `json:"context_mode,omitempty"`
	RecentTurns      int    `json:"recent_turns,omitempty"`
	ReasoningEffort  string `json:"reasoning_effort,omitempty"`
	reuseAgentID     string
	Description      string            `json:"description"`
	Prompt           string            `json:"prompt"`
	AgentType        string            `json:"agent_type"`
	WorkspaceMode    string            `json:"workspace_mode"`
	ModelProfileID   string            `json:"model_profile_id,omitempty"`
	ParentAgentID    string            `json:"-"`
	ParentToolCallID string            `json:"-"`
	ThreadID         string            `json:"-"`
	RunID            string            `json:"-"`
	TurnID           string            `json:"-"`
	Host             *HostSpawnOptions `json:"-"`
}

type WaitRequest struct {
	// DeferAcknowledgement lets the Runner acknowledge only after its result
	// and delivery receipt have committed together to the journal.
	DeferAcknowledgement bool
	// EventDriven keeps Host waits suspended until a result or cancellation.
	EventDriven bool
	TaskIDs     []string
	ReturnWhen  string
	Timeout     time.Duration
}

const (
	RetrievalCompleted = "completed"
	RetrievalRunning   = "running"
	RetrievalTimeout   = "timeout"
)

// WaitResult keeps an ordinary wait timeout in the success channel. A timeout
// reports the latest task snapshots and does not imply that any child failed or
// was cancelled.
type WaitResult struct {
	Tasks           []TaskSnapshot `json:"tasks"`
	RetrievalStatus string         `json:"retrieval_status"`
}

type MessageReceipt struct {
	TaskID    string    `json:"task_id"`
	QueuedAt  time.Time `json:"queued_at"`
	MessageID string    `json:"message_id"`
}

type TaskSnapshot struct {
	ScopeDigest   string `json:"scope_digest,omitempty"`
	RoleDigest    string `json:"role_digest,omitempty"`
	Workflow      string `json:"workflow,omitempty"`
	Background    bool   `json:"background,omitempty"`
	ContinuesFrom string `json:"continues_from,omitempty"`
	ContextMode   string `json:"context_mode,omitempty"`
	RecentTurns   int    `json:"recent_turns,omitempty"`
	AttemptID     string `json:"attempt_id,omitempty"`
	AgentClosed   bool   `json:"agent_closed,omitempty"`
	// ResultContent is the full Host result. Preview never serves as typed JSON authority.
	ResultContent          string           `json:"-"`
	ResultDigest           string           `json:"result_digest,omitempty"`
	ModelID                string           `json:"model_id,omitempty"`
	ReasoningEffort        string           `json:"reasoning_effort,omitempty"`
	ModelBindingSource     string           `json:"model_binding_source,omitempty"`
	ConfigSnapshotRevision string           `json:"config_snapshot_revision,omitempty"`
	TaskID                 string           `json:"task_id"`
	AgentID                string           `json:"agent_id"`
	ParentAgentID          string           `json:"parent_agent_id"`
	ParentToolCallID       string           `json:"parent_tool_call_id,omitempty"`
	ThreadID               string           `json:"thread_id"`
	RunID                  string           `json:"run_id"`
	TurnID                 string           `json:"turn_id,omitempty"`
	Description            string           `json:"description"`
	AgentType              string           `json:"agent_type"`
	WorkspaceMode          string           `json:"workspace_mode"`
	ModelProfileID         string           `json:"model_profile_id,omitempty"`
	Status                 TaskStatus       `json:"status"`
	CurrentActivity        string           `json:"current_activity,omitempty"`
	ToolUseCount           int              `json:"tool_use_count"`
	EstimatedTokens        int              `json:"estimated_tokens,omitempty"`
	InputTokens            int              `json:"input_tokens,omitempty"`
	OutputTokens           int              `json:"output_tokens,omitempty"`
	TotalTokens            int              `json:"total_tokens,omitempty"`
	ResultPreview          string           `json:"result_preview,omitempty"`
	ErrorCode              string           `json:"error_code,omitempty"`
	ErrorMessage           string           `json:"error_message,omitempty"`
	SideEffectState        string           `json:"side_effect_state,omitempty"`
	Worktree               *WorktreeReceipt `json:"worktree,omitempty"`
	CreatedAt              time.Time        `json:"created_at"`
	StartedAt              *time.Time       `json:"started_at,omitempty"`
	FinishedAt             *time.Time       `json:"finished_at,omitempty"`
	// HostToolReceipts are host-only, successful read receipts used to ground
	// Deep Research evidence. They are intentionally excluded from the public
	// subagent snapshot and are persisted by the research journal instead.
	HostToolReceipts []HostToolReceipt `json:"-"`
}

type HostToolReceipt struct {
	AgentID    string
	TaskID     string
	AttemptID  string
	ToolCallID string
	ToolName   string
	Details    map[string]any
}

// WorktreeReceipt is the host-owned isolation record for a git_worktree child.
// Host never auto-merges these changes back into the parent workspace.
type WorktreeReceipt struct {
	Path        string   `json:"path,omitempty"`
	Branch      string   `json:"branch,omitempty"`
	BaseCommit  string   `json:"base_commit,omitempty"`
	HeadCommit  string   `json:"head_commit,omitempty"`
	Dirty       bool     `json:"dirty,omitempty"`
	NeedsReview bool     `json:"needs_review,omitempty"`
	Cleaned     bool     `json:"cleaned,omitempty"`
	Changed     []string `json:"changed_paths,omitempty"`
}

type QueuedMessage struct {
	ID       string    `json:"message_id"`
	Content  string    `json:"content"`
	QueuedAt time.Time `json:"queued_at"`
}

func (s TaskSnapshot) Terminal() bool {
	switch s.Status {
	case StatusCompleted, StatusFailed, StatusCancelled, StatusInterrupted:
		return true
	default:
		return false
	}
}

type StartRequest struct {
	RoleSnapshot        *Role
	IncorporateMessages func([]string)
	ContextSeed         json.RawMessage
	Task                TaskSnapshot
	Prompt              string
	Host                *HostSpawnOptions
	ReportProgress      ProgressReporter
	DrainMessages       func() []QueuedMessage
	AttachWorktree      func(*WorktreeReceipt)
}

// HostSpawnOptions are host-only child constraints. They never enter the
// model-visible subagent tool schema.
type HostSpawnOptions struct {
	Skill             *HostSkillContext
	ToolAllowlist     []string
	ExtraHidden       []string
	MaxThoughts       int
	MaxToolExecutions int
	SkipMemory        bool
	SystemPrompt      string
}

type HostSkillContext struct {
	ID     string
	Path   string
	Body   string
	Model  string
	Effort string
}

// ProgressUpdate is emitted only after the corresponding child tool_call or
// model_usage record is durable. IDs let the manager deduplicate retries and
// replay-like duplicate notifications without inflating cumulative totals.
type ProgressUpdate struct {
	Activity        string
	ToolCallID      string
	ToolName        string
	ModelRequestID  string
	InputTokens     int
	OutputTokens    int
	TotalTokens     int
	HasInputTokens  bool
	HasOutputTokens bool
	HasTotalTokens  bool
	HostToolReceipt *HostToolReceipt
}

type ProgressReporter func(context.Context, ProgressUpdate) error

type Starter func(context.Context, StartRequest) (string, error)

type PersistEvent struct {
	Prompt      string
	ContextSeed json.RawMessage
	Kind        string
	Snapshot    TaskSnapshot
	Message     *QueuedMessage
}

// SpawnPreparation is created by the Host at the creator's safe boundary.
// ContextSeed is provider-neutral and durable. Credentials live only inside
// the immutable Starter closure and never in this public model binding.
type SpawnPreparation struct {
	Starter     Starter
	ContextSeed json.RawMessage
	Model       ModelBinding
}

type ModelBinding struct {
	ProfileID       string `json:"profile_id"`
	ModelID         string `json:"model_id"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	Source          string `json:"source"`
	Revision        string `json:"revision,omitempty"`
}

type ModelOption struct {
	ProfileID string   `json:"profile_id"`
	Name      string   `json:"name"`
	ModelID   string   `json:"model_id"`
	Efforts   []string `json:"efforts,omitempty"`
}

type Persister func(context.Context, PersistEvent) error

type Controller interface {
	Spawn(context.Context, SpawnRequest) (TaskSnapshot, error)
	SpawnFromHost(context.Context, SpawnRequest) (TaskSnapshot, error)
	Status(string) (TaskSnapshot, error)
	List() []TaskSnapshot
	Wait(context.Context, WaitRequest) (WaitResult, error)
	DrainTerminalNotifications() []TaskSnapshot
	AcknowledgeTerminal(string)
	Message(context.Context, string, string) (MessageReceipt, error)
	Cancel(context.Context, string, string) (TaskSnapshot, error)
	Close(context.Context) error
}
