package subagentruntime

import (
	"context"
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
	ErrorBoundaryViolation  = "subagent_boundary_violation"
	ErrorPersistence        = "subagent_persistence_failed"
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
	Description      string `json:"description"`
	Prompt           string `json:"prompt"`
	AgentType        string `json:"agent_type"`
	WorkspaceMode    string `json:"workspace_mode"`
	ModelProfileID   string `json:"model_profile_id,omitempty"`
	ParentAgentID    string `json:"-"`
	ParentToolCallID string `json:"-"`
	ThreadID         string `json:"-"`
	RunID            string `json:"-"`
	TurnID           string `json:"-"`
}

type WaitRequest struct {
	TaskIDs    []string
	ReturnWhen string
	Timeout    time.Duration
}

type MessageReceipt struct {
	TaskID    string    `json:"task_id"`
	QueuedAt  time.Time `json:"queued_at"`
	MessageID string    `json:"message_id"`
}

type TaskSnapshot struct {
	TaskID           string     `json:"task_id"`
	AgentID          string     `json:"agent_id"`
	ParentAgentID    string     `json:"parent_agent_id"`
	ParentToolCallID string     `json:"parent_tool_call_id,omitempty"`
	ThreadID         string     `json:"thread_id"`
	RunID            string     `json:"run_id"`
	TurnID           string     `json:"turn_id,omitempty"`
	Description      string     `json:"description"`
	AgentType        string     `json:"agent_type"`
	WorkspaceMode    string     `json:"workspace_mode"`
	ModelProfileID   string     `json:"model_profile_id,omitempty"`
	Status           TaskStatus `json:"status"`
	CurrentActivity  string     `json:"current_activity,omitempty"`
	ToolUseCount     int        `json:"tool_use_count"`
	EstimatedTokens  int        `json:"estimated_tokens,omitempty"`
	ResultPreview    string     `json:"result_preview,omitempty"`
	ErrorCode        string     `json:"error_code,omitempty"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	SideEffectState  string     `json:"side_effect_state,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
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
	Task   TaskSnapshot
	Prompt string
}

type Starter func(context.Context, StartRequest) (string, error)

type PersistEvent struct {
	Kind     string
	Snapshot TaskSnapshot
}

type Persister func(context.Context, PersistEvent) error

type Controller interface {
	Spawn(context.Context, SpawnRequest) (TaskSnapshot, error)
	Status(string) (TaskSnapshot, error)
	List() []TaskSnapshot
	Wait(context.Context, WaitRequest) ([]TaskSnapshot, error)
	Message(context.Context, string, string) (MessageReceipt, error)
	Cancel(context.Context, string, string) (TaskSnapshot, error)
	Close(context.Context) error
}
