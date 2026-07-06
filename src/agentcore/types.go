package agentcore

import (
	"awaken/src/context"
	"awaken/src/prompt"
	"awaken/src/tool"
)

type AgentConfig struct {
	SystemPrompt string
	// MaxThoughts <= 0 means the runner has no turn-count limit.
	MaxThoughts       int
	ModelConfig       ModelConfig
	EntryPoint        string
	Workspace         WorkspaceConfig
	AutoToolPolicy    AutoToolPolicy
	// UseApprovalForAsk routes PermissionAsk decisions through the approval
	// UI (via EventSink/ApprovalAwaiter) instead of the headless AUTO policy.
	// This should be true when the agent is backed by an interactive client
	// such as Awaken Desktop.
	UseApprovalForAsk bool
}

type WorkspaceConfig struct {
	Root          string
	WritableRoots []string
}

type ModelConfig struct {
	APIKey                string
	BaseURL               string
	Model                 string
	Protocol              string
	Temperature           float64
	MaxTokens             int
	AuthScheme            string
	ContextWindow         int
	AutoCompactTokenLimit int
}

type LLMRequest struct {
	SystemPrompt       string
	Messages           []context.Message
	Tools              []tool.ToolSchema
	Model              string
	Temperature        float64
	MaxTokens          int
	ContextWindowState ContextWindowState
	EnvelopeDebug      *ContextEnvelope
}

type RuntimeInfoProvider interface {
	GetRuntimeInfo() prompt.RuntimeInfo
}

type ContextProvider interface {
	GetMessages() []context.Message
	AddUserMessage(content string)
	AddAssistantMessage(content string, toolCalls []context.ToolCall)
	AddToolResult(toolCallID, name, content string)
	Clear()
}
