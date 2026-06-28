package agentcore

import (
	"sort"
	"strings"

	"awaken/src/llm"
	"awaken/src/prompt"
	"awaken/src/tool"
)

type RequestBuilder struct {
	config      AgentConfig
	runtimeInfo prompt.RuntimeInfo
}

func NewRequestBuilder(config AgentConfig) *RequestBuilder {
	return &RequestBuilder{
		config:      config,
		runtimeInfo: prompt.NewRuntimeInfo(),
	}
}

func (rb *RequestBuilder) UpdateRuntimeInfo(info prompt.RuntimeInfo) {
	rb.runtimeInfo = info
}

func (rb *RequestBuilder) Build(ctx ContextProvider, tools []tool.ToolDefinition) LLMRequest {
	if tools == nil {
		tools = rb.getTools()
	}
	tools = rb.config.ToolAccessPolicy().FilterDefinitions(tools)
	tools = rb.filterToolsForPermissionMode(tools)
	rb.updateRuntimeInfoFromConfig(tools)

	systemPrompt := rb.buildSystemPrompt()
	messages := ctx.GetMessages()

	return LLMRequest{
		SystemPrompt: systemPrompt,
		Messages:     messages,
		Tools:        tools,
		Model:        rb.config.ModelConfig.Model,
		Temperature:  rb.config.ModelConfig.Temperature,
		MaxTokens:    rb.config.ModelConfig.MaxTokens,
		Cache:        rb.buildRequestCacheMetadata(systemPrompt),
	}
}

func (rb *RequestBuilder) buildRequestCacheMetadata(systemPrompt string) llm.RequestCacheMetadata {
	if !rb.config.PromptCache.SystemPrompt {
		return llm.RequestCacheMetadata{}
	}
	if strings.TrimSpace(systemPrompt) == "" {
		return llm.RequestCacheMetadata{}
	}
	if min := rb.config.PromptCache.MinSystemPromptBytes; min > 0 && len(systemPrompt) < min {
		return llm.RequestCacheMetadata{}
	}
	return llm.RequestCacheMetadata{
		SystemPromptCacheControl: llm.CacheControl{Type: llm.CacheControlTypeEphemeral},
	}
}

func (rb *RequestBuilder) buildSystemPrompt() string {
	var staticContent string
	if rb.config.SystemPrompt != "" {
		staticContent = rb.config.SystemPrompt
	} else {
		staticContent, _ = prompt.LoadDefaultPrompt()
	}
	return prompt.BuildEffectiveSystemPrompt(staticContent, rb.runtimeInfo)
}

func (rb *RequestBuilder) getTools() []tool.ToolDefinition {
	definitions := tool.DefinitionsFromTools(tool.ListTools())
	if rb.config.MCPRegistry != nil {
		definitions = append(definitions, rb.config.MCPRegistry.ToolDefinitions()...)
	}

	sort.Slice(definitions, func(i, j int) bool {
		return definitions[i].Function.Name < definitions[j].Function.Name
	})
	return definitions
}

func (rb *RequestBuilder) updateRuntimeInfoFromConfig(tools []tool.ToolDefinition) {
	rb.runtimeInfo.UpdateCurrentDate()
	if strings.TrimSpace(rb.config.Workspace.Root) != "" {
		rb.runtimeInfo.SetWorkingDir(tool.NewPermissionContext(rb.config.Workspace.Root, rb.config.Workspace.WritableRoots).WorkspaceRootOrCWD())
	} else {
		rb.runtimeInfo.UpdateWorkingDir()
	}
	rb.runtimeInfo.SetModelName(rb.config.ModelConfig.Model)
	rb.runtimeInfo.SetEntryPoint(rb.config.EntryPoint)
	rb.runtimeInfo.SetToolSummary(rb.buildToolSummary(tools))
}

func (rb *RequestBuilder) buildToolSummary(tools []tool.ToolDefinition) string {
	if len(tools) == 0 {
		return "无可用工具"
	}

	summary := ""
	for i, t := range tools {
		if i > 0 {
			summary += ", "
		}
		summary += t.Function.Name
	}
	return summary
}

func (rb *RequestBuilder) filterToolsForPermissionMode(definitions []tool.ToolDefinition) []tool.ToolDefinition {
	if rb.config.PermissionMode() != PermissionModeDeny || len(definitions) == 0 {
		return definitions
	}

	filtered := make([]tool.ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		registeredTool, err := tool.GetTool(definition.Function.Name)
		if err != nil {
			continue
		}
		readOnlyTool, ok := registeredTool.(tool.ReadOnlyTool)
		if ok && readOnlyTool.IsReadOnly() {
			filtered = append(filtered, definition)
		}
	}
	return filtered
}

func BuildRequest(config AgentConfig, ctx ContextProvider) LLMRequest {
	builder := NewRequestBuilder(config)
	return builder.Build(ctx, nil)
}
