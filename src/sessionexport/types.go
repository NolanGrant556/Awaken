package sessionexport

import "time"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type PartType string

const (
	PartText       PartType = "text"
	PartToolCall   PartType = "tool_call"
	PartToolResult PartType = "tool_result"
)

type ToolEventKind string

const (
	ToolEventCall   ToolEventKind = "tool_call"
	ToolEventResult ToolEventKind = "tool_result"
)

type Message struct {
	ID        string     `json:"id,omitempty"`
	Role      Role       `json:"role"`
	Timestamp *time.Time `json:"timestamp,omitempty"`
	Parts     []Part     `json:"parts,omitempty"`
}

type Part struct {
	Type      PartType   `json:"type"`
	Text      string     `json:"text,omitempty"`
	ToolEvent *ToolEvent `json:"tool_event,omitempty"`
}

type ToolEvent struct {
	Kind    ToolEventKind `json:"kind"`
	ID      string        `json:"id,omitempty"`
	Name    string        `json:"name,omitempty"`
	Input   string        `json:"input,omitempty"`
	Output  string        `json:"output,omitempty"`
	Summary string        `json:"summary,omitempty"`
	Status  string        `json:"status,omitempty"`
	Error   string        `json:"error,omitempty"`
}

func TextPart(text string) Part {
	return Part{Type: PartText, Text: text}
}

func ToolCallPart(event ToolEvent) Part {
	event.Kind = ToolEventCall
	return Part{Type: PartToolCall, ToolEvent: &event}
}

func ToolResultPart(event ToolEvent) Part {
	event.Kind = ToolEventResult
	return Part{Type: PartToolResult, ToolEvent: &event}
}
