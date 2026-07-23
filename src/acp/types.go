package acp

import (
	"context"
	"time"
)

// PermissionMode controls how ACP permission requests from an external agent
// are handled by Awaken.
type PermissionMode string

const (
	PermissionModeAsk  PermissionMode = "ask"
	PermissionModeDeny PermissionMode = "deny"
)

// RunnerConfig is the persisted configuration for an ACP runner.
type RunnerConfig struct {
	ID                    string            `json:"id"`
	Enabled               bool              `json:"enabled"`
	Command               string            `json:"command"`
	Args                  []string          `json:"args,omitempty"`
	Env                   map[string]string `json:"env,omitempty"`
	PermissionMode        PermissionMode    `json:"permission_mode"`
	StartupTimeoutSeconds int               `json:"startup_timeout_seconds"`
	TurnTimeoutSeconds    int               `json:"turn_timeout_seconds"`
	MaxFrameBytes         int               `json:"max_frame_bytes"`
}

// RunnerSnapshot is the immutable view of a runner used for one session.
// File changes do not affect an active session.
type RunnerSnapshot struct {
	ID                    string
	Command               string
	Args                  []string
	Env                   map[string]string
	PermissionMode        PermissionMode
	StartupTimeoutSeconds int
	TurnTimeoutSeconds    int
	MaxFrameBytes         int
}

// ConversationKey identifies a live ACP session within Core memory.
type ConversationKey struct {
	ThreadID string
	RunnerID string
}

// ConversationState is the high-level lifecycle of an ACP conversation.
type ConversationState string

const (
	ConversationStateAbsent            ConversationState = "absent"
	ConversationStateStarting          ConversationState = "starting"
	ConversationStateReady             ConversationState = "ready"
	ConversationStatePrompting         ConversationState = "prompting"
	ConversationStateWaitingPermission ConversationState = "waiting_permission"
	ConversationStateCancelling        ConversationState = "cancelling"
	ConversationStateClosing           ConversationState = "closing"
	ConversationStateClosed            ConversationState = "closed"
	ConversationStateFailed            ConversationState = "failed"
)

// TurnState holds the active prompt turn for a conversation.
type TurnState struct {
	TurnID       string
	HostRunID    string
	ToolCallID   string
	StartedAt    time.Time
	ExpiresAt    time.Time
	TextBuffer   string
	ToolSnapshot map[string]ToolCallSnapshot
}

// ToolCallSnapshot is the accumulated, sanitized view of an external agent tool call.
type ToolCallSnapshot struct {
	ToolCallID string
	Title      string
	Kind       string
	Status     string
	Locations  []string
	Summary    string
}

// PermissionRequest is the sanitized request exposed to the UI and journal.
type PermissionRequest struct {
	InteractionID string
	ThreadID      string
	RunID         string
	RunnerID      string
	ACPSessionID  string
	ToolCallID    string
	Title         string
	Kind          string
	Paths         []string
	Command       string
	Options       []PermissionOption
	ExpiresAt     time.Time
}

// PermissionOption is a single user-facing choice for an ACP permission request.
type PermissionOption struct {
	OptionID    string
	Name        string
	Kind        string
	Description string
}

// PermissionResolution is the user's decision on a pending permission request.
type PermissionResolution struct {
	InteractionID string
	OptionID      string
	Cancelled     bool
}

// TurnResult is the terminal result of an ACP prompt turn.
type TurnResult struct {
	StopReason      string
	AssistantText   string
	ToolSummary     ToolSummary
	Truncated       bool
	SideEffectState string
	ErrorCode       string
	ErrorMessage    string
}

// ToolSummary aggregates external tool call outcomes for one turn.
type ToolSummary struct {
	Completed int
	Failed    int
	Total     int
}

// AgentCapabilitiesSnapshot captures the subset of ACP agent capabilities that
// Awaken cares about.
type AgentCapabilitiesSnapshot struct {
	ProtocolVersion         int
	SupportsTextPrompt      bool
	SupportsImagePrompt     bool
	SupportsAudioPrompt     bool
	SupportsEmbeddedContext bool
}

// SessionUpdate is the internal representation of an ACP session/update notification.
type SessionUpdate struct {
	SessionID string
	Kind      SessionUpdateKind
	TextDelta string
	ToolCall  *ToolCallSnapshot
	Plan      []PlanEntry
}

// SessionUpdateKind classifies the update.
type SessionUpdateKind string

const (
	SessionUpdateKindAgentText      SessionUpdateKind = "agent_text"
	SessionUpdateKindAgentThought   SessionUpdateKind = "agent_thought"
	SessionUpdateKindToolCall       SessionUpdateKind = "tool_call"
	SessionUpdateKindToolCallUpdate SessionUpdateKind = "tool_call_update"
	SessionUpdateKindPlan           SessionUpdateKind = "plan"
	SessionUpdateKindUnknown        SessionUpdateKind = "unknown"
)

// PlanEntry is a sanitized plan item.
type PlanEntry struct {
	ID     string
	Title  string
	Status string
}

// UpdateHandler receives decoded, sanitized session updates.
type UpdateHandler interface {
	OnUpdate(update SessionUpdate)
}

// UpdateHandlerFunc adapts a function to the UpdateHandler interface.
type UpdateHandlerFunc func(update SessionUpdate)

// OnUpdate implements UpdateHandler.
func (f UpdateHandlerFunc) OnUpdate(update SessionUpdate) {
	if f != nil {
		f(update)
	}
}

// PermissionHandler receives permission requests and must eventually resolve them.
type PermissionHandler interface {
	RequestPermission(ctx context.Context, req PermissionRequest) (PermissionResolution, error)
}

// Client is the protocol-level ACP client. Implementations live in src/acp/protocol.
type Client interface {
	Initialize(ctx context.Context) (AgentCapabilitiesSnapshot, error)
	NewSession(ctx context.Context, cwd string) (string, error)
	Prompt(ctx context.Context, sessionID, turnID, text string, handler UpdateHandler, permission PermissionHandler) (TurnResult, error)
	Cancel(ctx context.Context, sessionID string) error
	CloseSession(ctx context.Context, sessionID string) error
	Close() error
}

// SessionSummary is the safe, non-secret view of a live conversation.
type SessionSummary struct {
	ThreadID          string
	RunnerID          string
	ACPSessionID      string
	State             ConversationState
	WorkspaceRoot     string
	ActiveTurnID      string
	PendingPermission *PermissionRequest
	StartedAt         time.Time
	UpdatedAt         time.Time
}
