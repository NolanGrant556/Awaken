package convrecovery

const (
	MessageTypeAssistant  = "assistant"
	MessageTypeAttachment = "attachment"
	MessageTypeProgress   = "progress"
	MessageTypeSystem     = "system"
	MessageTypeUser       = "user"

	BlockTypeText             = "text"
	BlockTypeThinking         = "thinking"
	BlockTypeRedactedThinking = "redacted_thinking"
	BlockTypeToolResult       = "tool_result"
	BlockTypeToolUse          = "tool_use"

	TurnInterruptionNone              TurnInterruptionState = "none"
	TurnInterruptionInterruptedPrompt TurnInterruptionState = "interrupted_prompt"
	TurnInterruptionInterruptedTurn   TurnInterruptionState = "interrupted_turn"

	ContinuationPrompt  = "Continue from where you left off."
	NoResponseRequested = "No response requested."
)

type TurnInterruptionState string

type Message struct {
	Type             string
	ID               string
	ContentBlocks    []Block
	PermissionMode   string
	IsMeta           bool
	IsCompactSummary bool
	IsAPIError       bool

	AttachmentType     string
	AttachmentPath     string
	AttachmentFilename string
	DisplayPath        string
}

type Block struct {
	Type      string
	Text      string
	ID        string
	Name      string
	ToolUseID string
	IsError   bool
}

type Options struct {
	CWD                  string
	ValidPermissionModes []string
	TerminalToolNames    []string
}

type Result struct {
	Messages              []Message
	TurnInterruptionState TurnInterruptionState
	ContinuationAdded     bool
	SentinelAdded         bool
}
