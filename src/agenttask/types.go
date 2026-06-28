package agenttask

import "time"

type TaskStatus string

const (
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusStopped   TaskStatus = "stopped"
)

func IsTerminalStatus(status TaskStatus) bool {
	return status == TaskStatusCompleted || status == TaskStatusFailed || status == TaskStatusStopped
}

type TaskMode string

const (
	TaskModeBackground TaskMode = "background"
	TaskModeForeground TaskMode = "foreground"
)

type StopReason string

const (
	StopReasonUser            StopReason = "user"
	StopReasonParentCancelled StopReason = "parent_cancelled"
	StopReasonTimeout         StopReason = "timeout"
	StopReasonRuntimeShutdown StopReason = "runtime_shutdown"
	StopReasonOther           StopReason = "other"
)

type TaskAgent struct {
	Type  string `json:"type"`
	Name  string `json:"name,omitempty"`
	Model string `json:"model,omitempty"`
}

type ProgressCounters struct {
	ToolUseCount     int    `json:"tool_use_count"`
	TokenCount       int    `json:"token_count"`
	InputTokenCount  int    `json:"input_token_count"`
	OutputTokenCount int    `json:"output_token_count"`
	MessageCount     int    `json:"message_count"`
	OutputEventCount int    `json:"output_event_count"`
	LastActivity     string `json:"last_activity,omitempty"`
}

type Task struct {
	ID             string           `json:"id"`
	ParentRunID    string           `json:"parent_run_id,omitempty"`
	Description    string           `json:"description,omitempty"`
	Prompt         string           `json:"prompt,omitempty"`
	Mode           TaskMode         `json:"mode"`
	Agent          TaskAgent        `json:"agent"`
	Workspace      string           `json:"workspace"`
	TranscriptPath string           `json:"transcript_path,omitempty"`
	Status         TaskStatus       `json:"status"`
	Progress       ProgressCounters `json:"progress"`

	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastActivityAt time.Time  `json:"last_activity_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`

	CancelRequested bool       `json:"cancel_requested,omitempty"`
	StopRequested   bool       `json:"stop_requested,omitempty"`
	StopRequestedAt *time.Time `json:"stop_requested_at,omitempty"`
	StopReason      StopReason `json:"stop_reason,omitempty"`
	StopMessage     string     `json:"stop_message,omitempty"`

	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`

	Messages      []TaskMessage         `json:"messages,omitempty"`
	Continuations []ContinuationRequest `json:"continuations,omitempty"`
}

type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleSystem    MessageRole = "system"
)

type TaskMessage struct {
	ID        string      `json:"id"`
	TaskID    string      `json:"task_id"`
	Seq       uint64      `json:"seq"`
	Role      MessageRole `json:"role"`
	Sender    string      `json:"sender,omitempty"`
	Summary   string      `json:"summary,omitempty"`
	Content   string      `json:"content"`
	CreatedAt time.Time   `json:"created_at"`
}

type TaskEventType string

const (
	TaskEventStarted       TaskEventType = "task_started"
	TaskEventOutput        TaskEventType = "output"
	TaskEventToolUse       TaskEventType = "tool_use"
	TaskEventProgress      TaskEventType = "progress"
	TaskEventMessageSent   TaskEventType = "message_sent"
	TaskEventStopRequested TaskEventType = "stop_requested"
	TaskEventCompleted     TaskEventType = "completed"
	TaskEventFailed        TaskEventType = "failed"
)

type TaskEvent struct {
	TaskID    string        `json:"task_id"`
	Seq       uint64        `json:"seq"`
	Type      TaskEventType `json:"type"`
	Timestamp time.Time     `json:"timestamp"`

	MessageID  string      `json:"message_id,omitempty"`
	Role       MessageRole `json:"role,omitempty"`
	Content    string      `json:"content,omitempty"`
	Delta      string      `json:"delta,omitempty"`
	Summary    string      `json:"summary,omitempty"`
	Activity   string      `json:"activity,omitempty"`
	ToolName   string      `json:"tool_name,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`

	TokenDelta       int `json:"token_delta,omitempty"`
	InputTokenDelta  int `json:"input_token_delta,omitempty"`
	OutputTokenDelta int `json:"output_token_delta,omitempty"`
	ToolUseDelta     int `json:"tool_use_delta,omitempty"`
}

type OutputCursor struct {
	AfterSeq     uint64 `json:"after_seq"`
	NextAfterSeq uint64 `json:"next_after_seq"`
	LastSeq      uint64 `json:"last_seq"`
	Limit        int    `json:"limit,omitempty"`
	HasMore      bool   `json:"has_more"`
}

type OutputPage struct {
	TaskID string       `json:"task_id"`
	Events []TaskEvent  `json:"events"`
	Cursor OutputCursor `json:"cursor"`
}

type ContinuationRequest struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id"`
	MessageID   string    `json:"message_id"`
	ParentRunID string    `json:"parent_run_id,omitempty"`
	Sender      string    `json:"sender,omitempty"`
	Summary     string    `json:"summary,omitempty"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"created_at"`
}

type StartOptions struct {
	ID             string
	ParentRunID    string
	Description    string
	Prompt         string
	Mode           TaskMode
	Agent          TaskAgent
	Workspace      string
	TranscriptPath string
}
