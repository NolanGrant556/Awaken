package agentcore

import (
	"awaken/src/context"
	"awaken/src/mcp"
	"awaken/src/prompt"
	"awaken/src/skills"
	"awaken/src/tool"
)

type AgentConfig struct {
	SystemPrompt string
	MaxThoughts  int
	ModelConfig  ModelConfig
	EntryPoint   string
	Workspace    WorkspaceConfig
	MCPRegistry  mcp.Registry
	Skills       skills.Registry
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

type LLMRequest struct {
	SystemPrompt string
	Messages     []context.Message
	Tools        []tool.ToolDefinition
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
