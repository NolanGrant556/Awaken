package mcpelicitation

import "time"

type Action string

const (
	ActionOpen         Action = "open"
	ActionApprove      Action = "approve"
	ActionConfirm      Action = "confirm"
	ActionDecline      Action = "decline"
	ActionTimeout      Action = "timeout"
	ActionHookResolved Action = "hook-resolved"
	ActionCancel       Action = "cancel"
)

type Source string

const (
	SourceUser   Source = "user"
	SourceHook   Source = "hook"
	SourceSystem Source = "system"
)

type State string

const (
	StateQueued       State = "queued"
	StateOpened       State = "opened"
	StateApproved     State = "approved"
	StateConfirmed    State = "confirmed"
	StateDeclined     State = "declined"
	StateTimedOut     State = "timed_out"
	StateCanceled     State = "canceled"
	StateHookResolved State = "hook_resolved"
)

type Key struct {
	ServerName    string `json:"serverName"`
	ToolName      string `json:"toolName"`
	ElicitationID string `json:"elicitationId"`
}

type Request struct {
	ServerName    string            `json:"serverName"`
	ToolName      string            `json:"toolName"`
	ElicitationID string            `json:"elicitationId"`
	Message       string            `json:"message,omitempty"`
	URL           string            `json:"url"`
	RedactedURL   string            `json:"redactedUrl"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	CreatedAt     time.Time         `json:"createdAt"`
}

func (r Request) Key() Key {
	return Key{
		ServerName:    r.ServerName,
		ToolName:      r.ToolName,
		ElicitationID: r.ElicitationID,
	}
}

func (r Request) SafeURL() string {
	if r.RedactedURL != "" {
		return r.RedactedURL
	}
	return RedactURL(r.URL)
}

type Response struct {
	Action  Action    `json:"action"`
	Source  Source    `json:"source,omitempty"`
	Message string    `json:"message,omitempty"`
	At      time.Time `json:"at"`
}

type ItemSnapshot struct {
	ServerName    string    `json:"serverName"`
	ToolName      string    `json:"toolName"`
	ElicitationID string    `json:"elicitationId"`
	Message       string    `json:"message,omitempty"`
	RedactedURL   string    `json:"redactedUrl"`
	State         State     `json:"state"`
	LastAction    Action    `json:"lastAction,omitempty"`
	Response      *Response `json:"response,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func (s ItemSnapshot) Key() Key {
	return Key{
		ServerName:    s.ServerName,
		ToolName:      s.ToolName,
		ElicitationID: s.ElicitationID,
	}
}

type Snapshot struct {
	Items        []ItemSnapshot `json:"items"`
	PendingCount int            `json:"pendingCount"`
	UpdatedAt    time.Time      `json:"updatedAt"`
}

func actionState(action Action) State {
	switch action {
	case ActionOpen:
		return StateOpened
	case ActionApprove:
		return StateApproved
	case ActionConfirm:
		return StateConfirmed
	case ActionDecline:
		return StateDeclined
	case ActionTimeout:
		return StateTimedOut
	case ActionHookResolved:
		return StateHookResolved
	case ActionCancel:
		return StateCanceled
	default:
		return StateQueued
	}
}

func actionAllowsRetry(action Action) bool {
	return action == ActionApprove || action == ActionConfirm || action == ActionHookResolved
}

func statePending(state State) bool {
	return state == StateQueued || state == StateOpened
}
