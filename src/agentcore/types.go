package agentcore

import (
	"awaken/src/context"
	"awaken/src/prompt"
)

type AgentConfig struct {
	SystemPrompt string
	MaxThoughts  int
	ModelConfig  ModelConfig
	EntryPoint   string
}

type ModelConfig struct {
	APIKey      string
	BaseURL     string
	Model       string
	Temperature float64
	MaxTokens   int
}

type LLMRequest struct {
	SystemPrompt string
	Messages     []context.Message
	Tools        []context.ToolDefinition
	Model        string
	Temperature  float64
	MaxTokens    int
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
