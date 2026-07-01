package agentcore

import (
	"strings"

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

func (rb *RequestBuilder) Build(ctx ContextProvider, tools []tool.ToolSchema) LLMRequest {
	if tools == nil {
		tools = rb.getTools()
	}
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

func (rb *RequestBuilder) getTools() []tool.ToolSchema {
	tools := tool.ListTools()
	definitions := make([]tool.ToolSchema, 0, len(tools))

	for _, t := range tools {
		definitions = append(definitions, tool.ToolSchema{
			Type: "function",
			Function: tool.ToolFunctionSchema{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  rb.buildToolParameters(t),
			},
		})
	}
	return definitions
}

func (rb *RequestBuilder) buildToolParameters(t tool.Tool) tool.ToolParametersSchema {
	params := t.Parameters()
	properties := make(map[string]tool.ToolParam, len(params))
	required := make([]string, 0)

	for _, p := range params {
		properties[p.Name] = tool.ToolParam{
			Type:        p.Type,
			Description: p.Description,
		}
		if p.Required {
			required = append(required, p.Name)
		}
	}

	return tool.ToolParametersSchema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
}

func (rb *RequestBuilder) updateRuntimeInfoFromConfig(tools []tool.ToolSchema) {
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

func (rb *RequestBuilder) buildToolSummary(tools []tool.ToolSchema) string {
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

func BuildRequest(config AgentConfig, ctx ContextProvider) LLMRequest {
	builder := NewRequestBuilder(config)
	return builder.Build(ctx, nil)
}
