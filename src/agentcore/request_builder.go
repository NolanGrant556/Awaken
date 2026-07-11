package agentcore

import (
	"fmt"
	"strings"

	convcontext "awaken/src/context"
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
	envelope := rb.BuildContextEnvelope(ctx, tools)
	return rb.BuildLLMRequestFromEnvelope(envelope)
}

func (rb *RequestBuilder) BuildContextEnvelope(ctx ContextProvider, tools []tool.ToolSchema) ContextEnvelope {
	if tools == nil {
		tools = rb.getTools()
	}

	staticContent := rb.buildStaticSystemPrompt()
	snapshot := rb.buildRuntimeSnapshot(tools)
	messages := ctx.GetMessages()
	if modelProvider, ok := ctx.(interface {
		GetMessagesForModel() []convcontext.Message
	}); ok {
		messages = modelProvider.GetMessagesForModel()
	}

	envelope := ContextEnvelope{
		SystemBlocks: []ContextBlock{
			{ID: "system_prompt", Content: staticContent},
		},
		SystemContextItems: []ContextItem{
			runtimeSnapshotSystemItem(snapshot),
		},
		Attachments:        contextAttachmentsFromMessages(messages),
		ToolSchemas:        cloneToolDefinitions(tools),
		RuntimeSnapshot:    snapshot,
		CacheKeys:          rb.buildCacheKeys(staticContent, tools, snapshot),
		Deltas:             rb.buildContextDeltas(snapshot, tools),
		Messages:           cloneContextMessages(messages),
		CompactModelConfig: rb.config.ModelConfig,
		Model:              rb.config.ModelConfig.Model,
		Temperature:        rb.config.ModelConfig.Temperature,
		MaxTokens:          rb.config.ModelConfig.MaxTokens,
	}
	for _, block := range rb.config.SkillContextBlocks {
		if strings.TrimSpace(block.Content) == "" {
			continue
		}
		envelope.SystemBlocks = append(envelope.SystemBlocks, block)
		envelope.SystemContextItems = append(envelope.SystemContextItems, ContextItem{
			ID:      firstNonEmptyAttachmentValue(block.ID, "skill_activation"),
			Type:    "skill_activation",
			Content: block.Content,
		})
	}
	envelope.ContextWindowState = NewContextEngine(contextWindowConfigFromModelConfig(rb.config.ModelConfig)).Evaluate(envelope)
	return envelope
}

func (rb *RequestBuilder) BuildLLMRequestFromEnvelope(envelope ContextEnvelope) LLMRequest {
	envelopeCopy := envelope
	return LLMRequest{
		SystemPrompt:       rb.buildSystemPromptFromEnvelope(envelope),
		Messages:           cloneContextMessages(envelope.Messages),
		Tools:              cloneToolDefinitions(envelope.ToolSchemas),
		Model:              envelope.Model,
		Temperature:        envelope.Temperature,
		MaxTokens:          envelope.MaxTokens,
		ContextWindowState: envelope.ContextWindowState,
		EnvelopeDebug:      &envelopeCopy,
	}
}

func (rb *RequestBuilder) buildStaticSystemPrompt() string {
	var staticContent string
	if rb.config.SystemPrompt != "" {
		staticContent = rb.config.SystemPrompt
	} else {
		staticContent, _ = prompt.LoadDefaultPrompt()
	}
	return staticContent
}

func (rb *RequestBuilder) buildSystemPromptFromEnvelope(envelope ContextEnvelope) string {
	staticContent := ""
	if len(envelope.SystemBlocks) > 0 {
		staticContent = envelope.SystemBlocks[0].Content
	}
	var skillParts []string
	for i, block := range envelope.SystemBlocks {
		if i == 0 {
			continue
		}
		if strings.TrimSpace(block.Content) == "" {
			continue
		}
		skillParts = append(skillParts, block.Content)
	}
	if len(skillParts) > 0 {
		staticContent = strings.TrimSpace(staticContent) + "\n\n" + strings.Join(skillParts, "\n\n")
	}
	return prompt.BuildEffectiveSystemPrompt(staticContent, envelope.RuntimeSnapshot.ToRuntimeInfo())
}

func (rb *RequestBuilder) getTools() []tool.ToolSchema {
	tools := tool.FilterToolsForExposure(tool.ListTools(), rb.toolExposurePolicy())
	if rb.config.ResolvedToolPool != nil {
		allowed := make(map[string]bool, len(rb.config.ResolvedToolPool))
		for _, name := range rb.config.ResolvedToolPool {
			name = strings.TrimSpace(name)
			if name != "" {
				allowed[name] = true
			}
		}
		filtered := make([]tool.Tool, 0, len(tools))
		for _, t := range tools {
			if allowed[t.Name()] {
				filtered = append(filtered, t)
			}
		}
		tools = filtered
	}
	definitions := make([]tool.ToolSchema, 0, len(tools))

	for _, t := range tools {
		definition := requestToolDefinition(t)
		definitions = append(definitions, tool.ToolSchema{
			Type: "function",
			Function: tool.ToolFunctionSchema{
				Name:        definition.Name,
				Description: tool.ModelDescription(definition),
				Parameters:  definition.InputSchema,
			},
		})
	}
	return definitions
}

// toolExposurePolicy returns the model-visible tool pool policy.
// Ordinary providers hide raw apply_patch; compat/grammar-capable modes keep it.
func (rb *RequestBuilder) toolExposurePolicy() tool.ToolExposurePolicy {
	if rb.config.ToolExposure.Mode != "" || len(rb.config.ToolExposure.ExtraVisible) > 0 || len(rb.config.ToolExposure.ExtraHidden) > 0 {
		return rb.config.ToolExposure
	}
	return tool.DefaultToolExposurePolicy()
}

func contextAttachmentsFromMessages(messages []convcontext.Message) []ContextAttachment {
	attachments := make([]ContextAttachment, 0)
	for _, message := range messages {
		for _, attachment := range message.Attachments {
			attachments = append(attachments, ContextAttachment{
				ID:   attachment.ID,
				Kind: attachment.Kind,
				Path: firstNonEmptyAttachmentValue(
					strings.TrimSpace(attachment.Path),
					strings.TrimSpace(attachment.DisplayName),
					strings.TrimSpace(attachment.StorageKey),
				),
				Content: attachment.DataBase64,
			})
		}
	}
	return attachments
}

func firstNonEmptyAttachmentValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (rb *RequestBuilder) buildToolParameters(t tool.Tool) tool.ToolParametersSchema {
	return requestToolDefinition(t).InputSchema
}

func requestToolDefinition(t tool.Tool) tool.ToolDefinition {
	if registered, err := tool.GetRegisteredTool(t.Name()); err == nil {
		return registered.Definition
	}
	return tool.ResolveDefinition(t)
}

func (rb *RequestBuilder) updateRuntimeInfoFromConfig(tools []tool.ToolSchema) {
	rb.runtimeInfo.UpdateCurrentDate()
	if strings.TrimSpace(rb.config.Workspace.Root) != "" {
		rb.runtimeInfo.SetWorkingDir(permissionContextFromTaskScope(rb.config.Workspace).WorkspaceRootOrCWD())
	} else {
		rb.runtimeInfo.UpdateWorkingDir()
	}
	rb.runtimeInfo.SetModelName(rb.config.ModelConfig.Model)
	rb.runtimeInfo.SetEntryPoint(rb.config.EntryPoint)
	rb.runtimeInfo.SetToolSummary(rb.buildToolSummary(tools))
	rb.runtimeInfo.SetSkillSummary(strings.TrimSpace(rb.config.SkillSummary))
}

func (rb *RequestBuilder) buildRuntimeSnapshot(tools []tool.ToolSchema) RuntimeSnapshot {
	rb.updateRuntimeInfoFromConfig(tools)
	permissionContext := permissionContextFromTaskScope(rb.config.Workspace)
	snapshot := RuntimeSnapshot{
		CurrentDate:      rb.runtimeInfo.CurrentDate,
		WorkingDirectory: rb.runtimeInfo.WorkingDir,
		ModelName:        rb.runtimeInfo.ModelName,
		EntryPoint:       rb.runtimeInfo.EntryPoint,
		ToolSummary:      rb.runtimeInfo.ToolSummary,
		SkillSummary:     rb.runtimeInfo.SkillSummary,
		ScopeID:          permissionContext.ScopeID,
		WorkspaceRoot:    permissionContext.WorkspaceRoot,
		ReadableRoots:    append([]string(nil), permissionContext.ReadableRoots...),
		WritableRoots:    append([]string(nil), permissionContext.WritableRoots...),
		OutputRoots:      append([]string(nil), permissionContext.OutputRoots...),
		AttachedFiles:    append([]tool.ScopedFile(nil), permissionContext.AttachedFiles...),
	}
	rb.runtimeInfo = snapshot.ToRuntimeInfo()
	return snapshot
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

func (rb *RequestBuilder) buildCacheKeys(staticContent string, tools []tool.ToolSchema, snapshot RuntimeSnapshot) []ContextCacheKey {
	return []ContextCacheKey{
		{Name: "system_prompt", Value: fmt.Sprintf("len:%d", len(staticContent))},
		{Name: "tool_pool", Value: rb.buildToolSummary(tools)},
		{Name: "runtime_snapshot", Value: snapshot.WorkingDirectory + "|" + snapshot.ModelName + "|" + snapshot.EntryPoint + "|skills:" + snapshot.SkillSummary},
	}
}

func (rb *RequestBuilder) buildContextDeltas(snapshot RuntimeSnapshot, tools []tool.ToolSchema) []ContextDelta {
	return []ContextDelta{
		{
			Type:    ContextDeltaRuntimeSnapshot,
			Summary: snapshot.WorkingDirectory + "|" + snapshot.ModelName + "|" + snapshot.EntryPoint + "|skills:" + snapshot.SkillSummary,
		},
		{
			Type:    ContextDeltaToolPool,
			Summary: rb.buildToolSummary(tools),
		},
	}
}

func cloneContextMessages(messages []convcontext.Message) []convcontext.Message {
	result := make([]convcontext.Message, len(messages))
	for i, message := range messages {
		result[i] = message
		result[i].ToolCalls = append([]convcontext.ToolCall(nil), message.ToolCalls...)
	}
	return result
}

func cloneToolDefinitions(tools []tool.ToolSchema) []tool.ToolSchema {
	result := make([]tool.ToolSchema, len(tools))
	copy(result, tools)
	return result
}

func BuildRequest(config AgentConfig, ctx ContextProvider) LLMRequest {
	builder := NewRequestBuilder(config)
	return builder.Build(ctx, nil)
}

func permissionContextFromTaskScope(scope WorkspaceConfig) tool.PermissionContext {
	return tool.NewTaskScopePermissionContext(
		scope.ScopeID,
		scope.Root,
		scope.ReadableRoots,
		scope.WritableRoots,
		scope.OutputRoots,
		scope.AttachedFiles,
		scope.ProtectedRoots,
		scope.TemporaryWorkRoot,
	)
}
