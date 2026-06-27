package agentcore

import (
	"awaken/src/context"
	"awaken/src/llm"
	"awaken/src/prompt"
	"awaken/src/tool"
)

type AgentConfig struct {
	SystemPrompt string
	MaxThoughts  int
	ModelConfig  ModelConfig
	EntryPoint   string
	Workspace    WorkspaceConfig
	PromptCache  PromptCacheConfig
}

type WorkspaceConfig struct {
	Root          string
	WritableRoots []string
}

type ModelConfig struct {
	APIKey      string
	BaseURL     string
	Model       string
	Temperature float64
	MaxTokens   int
	AuthScheme  string
}

type PromptCacheConfig struct {
	SystemPrompt         bool
	MinSystemPromptBytes int
}

type LLMRequest struct {
	SystemPrompt string
	Messages     []context.Message
	Tools        []tool.ToolDefinition
	Model        string
	Temperature  float64
	MaxTokens    int
	Cache        llm.RequestCacheMetadata
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
