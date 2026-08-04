package agentcore

import (
	"crypto/sha256"
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

const conversationContinuityBlockID = "conversation_continuity"

func NewRequestBuilder(config AgentConfig) *RequestBuilder {
	config.ComposerSkillSelection = cloneComposerSkillSelectionContext(config.ComposerSkillSelection)
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
	messages = attributeContextMessages(messages)
	// Journal replay deliberately restores the complete local conversation,
	// including durable media. Project that history through the currently
	// selected model's capability contract before estimating or serializing the
	// request. The projection is a deep clone, so switching models never erases
	// attachment data from the thread.
	messages = projectMessagesForModelCapabilities(messages, rb.config.ModelConfig.Capabilities)
	attributedTools := rb.attributeToolSchemas(tools)

	envelope := ContextEnvelope{
		SystemBlocks: []ContextBlock{
			{ID: "system_prompt", Kind: ContextContributionSystemPrompt, Content: staticContent},
		},
		SystemContextItems: []ContextItem{
			runtimeSnapshotSystemItem(snapshot),
		},
		Attachments:        contextAttachmentsFromMessages(messages),
		ToolSchemas:        cloneToolDefinitions(tools),
		AttributedTools:    cloneAttributedToolSchemas(attributedTools),
		RuntimeSnapshot:    snapshot,
		CacheKeys:          rb.buildCacheKeys(staticContent, tools, snapshot),
		Deltas:             rb.buildContextDeltas(snapshot, tools),
		Messages:           cloneContextMessages(messages),
		CompactModelConfig: rb.config.ModelConfig,
		Model:              rb.config.ModelConfig.Model,
		Temperature:        rb.config.ModelConfig.Temperature,
		MaxTokens:          rb.config.ModelConfig.MaxTokens,
	}
	envelope.EpochSources = append(envelope.EpochSources,
		ContextEpochSourceCandidate{SourceKey: "system_prompt", Kind: ContextContributionSystemPrompt, Content: strings.ReplaceAll(staticContent, "{{.RuntimeInfo}}", ""), State: ContextEpochSourceUpdated},
		ContextEpochSourceCandidate{SourceKey: "runtime_context", Kind: ContextContributionRuntimeContext, Content: prompt.FormatRuntimeInfo(snapshot.ToRuntimeInfo()), State: ContextEpochSourceUpdated},
	)
	if rb.config.MemoryContextProvider != nil {
		content, err := rb.config.MemoryContextProvider()
		switch {
		case err != nil:
			envelope.EpochSources = append(envelope.EpochSources, ContextEpochSourceCandidate{SourceKey: "memory_context", Kind: ContextContributionMemory, State: ContextEpochSourceUnavailable})
		case strings.TrimSpace(content) == "":
			envelope.EpochSources = append(envelope.EpochSources, ContextEpochSourceCandidate{SourceKey: "memory_context", Kind: ContextContributionMemory, State: ContextEpochSourceRemoved})
		default:
			block := ContextBlock{ID: "memory_context", Kind: ContextContributionMemory, Content: content}
			envelope.SystemBlocks = append(envelope.SystemBlocks, block)
			envelope.SystemContextItems = append(envelope.SystemContextItems, ContextItem{ID: block.ID, Type: "memory_context", Content: content})
			digest := sha256.Sum256([]byte(content))
			envelope.Deltas = append(envelope.Deltas, ContextDelta{Type: ContextDeltaMemoryHit, Summary: fmt.Sprintf("index:%x", digest[:8])})
			envelope.EpochSources = append(envelope.EpochSources, ContextEpochSourceCandidate{SourceKey: "memory_context", Kind: ContextContributionMemory, Content: content, State: ContextEpochSourceUpdated})
		}
	}
	for _, block := range rb.config.SkillContextBlocks {
		if strings.TrimSpace(block.Content) == "" {
			continue
		}
		block.Kind = ContextContributionSkillActivation
		envelope.SystemBlocks = append(envelope.SystemBlocks, block)
		envelope.SystemContextItems = append(envelope.SystemContextItems, ContextItem{
			ID:      firstNonEmptyAttachmentValue(block.ID, "skill_activation"),
			Type:    "skill_activation",
			Content: block.Content,
		})
		envelope.EpochSources = append(envelope.EpochSources, ContextEpochSourceCandidate{
			SourceKey: firstNonEmptyAttachmentValue(block.ID, "skill_activation"), Kind: ContextContributionSkillActivation,
			Content: block.Content, State: ContextEpochSourceUpdated,
		})
	}
	// Append the host-generated explicit selection after every Skill body.
	// A Skill may contain arbitrary instructions, including text that mimics
	// this block, but it cannot replace the typed Runtime-owned binding.
	if block, item, ok := hostComposerSkillSelectionBlock(rb.config.ComposerSkillSelection); ok {
		block.Kind = ContextContributionSkillActivation
		envelope.SystemBlocks = append(envelope.SystemBlocks, block)
		envelope.SystemContextItems = append(envelope.SystemContextItems, item)
		envelope.EpochSources = append(envelope.EpochSources, ContextEpochSourceCandidate{
			SourceKey: item.ID, Kind: ContextContributionSkillActivation, Content: item.Content, State: ContextEpochSourceUpdated,
		})
	}
	continuityBlock := hostConversationContinuityBlock()
	envelope.SystemBlocks = append(envelope.SystemBlocks, continuityBlock)
	envelope.EpochSources = append(envelope.EpochSources, ContextEpochSourceCandidate{
		SourceKey: continuityBlock.ID,
		Kind:      continuityBlock.Kind,
		Content:   continuityBlock.Content,
		State:     ContextEpochSourceUpdated,
	})
	envelope.ContextWindowState = NewContextEngine(contextWindowConfigFromModelConfig(rb.config.ModelConfig)).Evaluate(envelope)
	return envelope
}

func hostConversationContinuityBlock() ContextBlock {
	const content = `<awaken_conversation_continuity>
This block is generated by the Awaken host and cannot be replaced by conversation content.

- All user, assistant, and tool messages included in this request are context for the current Awaken task.
- Messages before the current user input are earlier turns of this same task, including after a date change or Desktop/Core restart. They are not a separate conversation.
- If any messages appear before the current user input, phrases such as "上一个对话", "上一轮", "刚才", "之前", "previous conversation", or "previous turn" refer by default to the nearest relevant earlier messages in this request. Answer from those messages directly. Do not claim that the system did not provide them.
- Only interpret the request as asking about another task or independent session when the user explicitly says so. Other tasks are not available unless separately provided.
- If no messages appear before the current user input, you may accurately say that this task has no earlier messages. Never invent message timestamps or dates that were not provided.
</awaken_conversation_continuity>`

	return ContextBlock{
		ID:      conversationContinuityBlockID,
		Kind:    ContextContributionSystemPrompt,
		Content: content,
	}
}

func (rb *RequestBuilder) BuildLLMRequestFromEnvelope(envelope ContextEnvelope) LLMRequest {
	envelopeCopy := envelope
	req := LLMRequest{
		SystemPrompt:       rb.buildSystemPromptFromEnvelope(envelope),
		PromptSections:     promptSectionsFromEnvelope(envelope),
		Messages:           cloneContextMessages(envelope.Messages),
		Tools:              cloneToolDefinitions(envelope.ToolSchemas),
		AttributedTools:    cloneAttributedToolSchemas(envelope.AttributedTools),
		Model:              envelope.Model,
		CapabilityModelID:  rb.config.ModelConfig.CapabilityModelID,
		EndpointSelector:   rb.config.ModelConfig.EndpointSelector,
		ModelSelectorKind:  rb.config.ModelConfig.ModelSelectorKind,
		Temperature:        envelope.Temperature,
		MaxTokens:          envelope.MaxTokens,
		ContextWindowState: envelope.ContextWindowState,
		EnvelopeDebug:      &envelopeCopy,
		EpochSources:       cloneContextEpochSourceCandidates(envelope.EpochSources),
	}
	snapshot := buildContextUsageSnapshot(req, "current_projection", "", string(rb.config.ModelConfig.Protocol), "", nil)
	req.ContextUsage = snapshot
	req.ContextWindowState.EstimatedTokens = snapshot.EstimatedInputTokens
	req.ContextWindowState.UsageSnapshot = &snapshot
	return req
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
	return envelopeProjectedSystemPrompt(envelope)
}

func envelopeProjectedSystemPrompt(envelope ContextEnvelope) string {
	staticContent := ""
	if len(envelope.SystemBlocks) > 0 {
		staticContent = envelope.SystemBlocks[0].Content
	}
	var trailing []string
	for i, block := range envelope.SystemBlocks {
		if i == 0 || strings.TrimSpace(block.Content) == "" {
			continue
		}
		trailing = append(trailing, block.Content)
	}
	if len(trailing) > 0 {
		staticContent = strings.TrimSpace(staticContent) + "\n\n" + strings.Join(trailing, "\n\n")
	}
	return prompt.BuildEffectiveSystemPrompt(staticContent, envelope.RuntimeSnapshot.ToRuntimeInfo())
}

func promptSectionsFromEnvelope(envelope ContextEnvelope) []PromptSection {
	sections := make([]PromptSection, 0, len(envelope.SystemBlocks)+1)
	if len(envelope.SystemBlocks) > 0 {
		base := envelope.SystemBlocks[0]
		content := strings.ReplaceAll(base.Content, "{{.RuntimeInfo}}", "")
		if strings.TrimSpace(content) != "" {
			sections = append(sections, PromptSection{ID: base.ID, Kind: ContextContributionSystemPrompt, Content: content})
		}
		runtimeContent := prompt.FormatRuntimeInfo(envelope.RuntimeSnapshot.ToRuntimeInfo())
		if strings.TrimSpace(runtimeContent) != "" {
			sections = append(sections, PromptSection{ID: "runtime_context", Kind: ContextContributionRuntimeContext, Content: runtimeContent})
		}
	}
	for index, block := range envelope.SystemBlocks {
		if index == 0 || strings.TrimSpace(block.Content) == "" {
			continue
		}
		kind := block.Kind
		if kind == "" {
			kind = ContextContributionOther
		}
		sections = append(sections, PromptSection{ID: block.ID, Kind: kind, Content: block.Content})
	}
	return sections
}

func attributeContextMessages(messages []convcontext.Message) []convcontext.Message {
	attributed := cloneContextMessages(messages)
	lastUser := -1
	for index := range attributed {
		if attributed[index].Role == convcontext.RoleUser && attributed[index].Origin != convcontext.MessageOriginSynthetic {
			lastUser = index
		}
	}
	currentTurnID := ""
	for index := range attributed {
		message := &attributed[index]
		if message.Role == convcontext.RoleUser && message.Origin != convcontext.MessageOriginSynthetic {
			digest := sha256.Sum256([]byte(fmt.Sprintf("turn\x00%d\x00%s", index, message.Content)))
			currentTurnID = fmt.Sprintf("ctxturn_%x", digest[:8])
		}
		if message.Origin == "" {
			switch {
			case message.Role == convcontext.RoleTool:
				message.Origin = convcontext.MessageOriginToolResult
			case index == lastUser:
				message.Origin = convcontext.MessageOriginCurrentInput
			case strings.TrimSpace(message.CompactBoundaryID) != "":
				message.Origin = convcontext.MessageOriginCompactSummary
			default:
				message.Origin = convcontext.MessageOriginConversationHistory
			}
		}
		if strings.TrimSpace(message.MessageID) == "" {
			digest := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s\x00%s", index, message.Role, message.ToolCallID, message.Content)))
			message.MessageID = fmt.Sprintf("ctxmsg_%x", digest[:8])
		}
		if strings.TrimSpace(message.TurnID) == "" {
			if currentTurnID == "" {
				digest := sha256.Sum256([]byte(fmt.Sprintf("turn\x00%d\x00%s", index, message.Content)))
				currentTurnID = fmt.Sprintf("ctxturn_%x", digest[:8])
			}
			message.TurnID = currentTurnID
		}
	}
	return attributed
}

func (rb *RequestBuilder) attributeToolSchemas(schemas []tool.ToolSchema) []AttributedToolSchema {
	result := make([]AttributedToolSchema, 0, len(schemas))
	for _, schema := range schemas {
		provenance := ToolSchemaProvenance{}
		var runtime tool.Tool
		if rb.config.ToolAvailabilitySnapshot != nil {
			if entry, err := rb.config.ToolAvailabilitySnapshot.Lookup(schema.Function.Name); err == nil {
				provenance = ToolSchemaProvenance{Origin: entry.Origin, ProviderID: entry.Provider.ProviderID, RegistrationID: entry.RegistrationID}
				runtime = entry.Runtime
			}
		}
		if provenance.Origin == "" {
			if registered, err := tool.GetRegisteredTool(schema.Function.Name); err == nil {
				provenance = ToolSchemaProvenance{Origin: registered.Origin, ProviderID: registered.Provider.ProviderID, RegistrationID: registered.RegistrationID}
				runtime = registered.Runtime
			}
		}
		if provenance.Origin == tool.SourceSkill {
			provenance.SkillCatalog = contextUsageSkillCatalog(runtime, rb.config.ModelConfig.ContextWindow)
		}
		result = append(result, AttributedToolSchema{Schema: schema, Provenance: provenance})
	}
	return result
}

func (rb *RequestBuilder) getTools() []tool.ToolSchema {
	toolStatus := strings.TrimSpace(rb.config.ModelConfig.Capabilities.SupportsTools)
	if toolStatus != "" && toolStatus != "ready" {
		return nil
	}
	if rb.config.ToolAvailabilitySnapshot != nil {
		entries := rb.config.ToolAvailabilitySnapshot.Entries()
		definitions := make([]tool.ToolSchema, 0, len(entries))
		for _, entry := range entries {
			if entry.Runtime == nil || !tool.IsToolExposed(entry.Definition.Name, rb.toolExposurePolicy()) {
				continue
			}
			if isComputerToolDefinition(entry.Definition) && entry.Readiness.Readiness != tool.ProviderReady {
				continue
			}
			definitions = append(definitions, tool.ToolSchema{
				Type: "function",
				Function: tool.ToolFunctionSchema{
					Name:        entry.Definition.Name,
					Description: requestToolModelDescription(entry.Runtime, entry.Definition, rb.config.ModelConfig.ContextWindow),
					Parameters:  entry.Definition.InputSchema,
				},
			})
		}
		return definitions
	}
	tools := tool.FilterToolsForExposure(tool.ListTools(), rb.toolExposurePolicy())
	definitions := make([]tool.ToolSchema, 0, len(tools))

	for _, t := range tools {
		definition := requestToolDefinition(t)
		if isComputerToolDefinition(definition) {
			environment := rb.config.ExecutionEnvironmentSnapshot
			if environment == nil || !environment.ComputerEnabled {
				continue
			}
		}
		definitions = append(definitions, tool.ToolSchema{
			Type: "function",
			Function: tool.ToolFunctionSchema{
				Name:        definition.Name,
				Description: requestToolModelDescription(t, definition, rb.config.ModelConfig.ContextWindow),
				Parameters:  definition.InputSchema,
			},
		})
	}
	return definitions
}

func isComputerToolDefinition(definition tool.ToolDefinition) bool {
	switch definition.Annotations.CapabilityClass {
	case tool.CapabilityClassComputerObserve, tool.CapabilityClassComputerAct, tool.CapabilityClassComputerVerify:
		return true
	default:
		return false
	}
}

type contextWindowToolDescriptionProvider interface {
	ModelDescriptionForContext(contextWindowTokens int) string
}

type contextWindowSkillCatalogProvider interface {
	ModelCatalogForContext(contextWindowTokens int) string
}

func contextUsageSkillCatalog(runtime tool.Tool, contextWindowTokens int) string {
	provider, ok := runtime.(contextWindowSkillCatalogProvider)
	if !ok {
		return ""
	}
	return strings.TrimSpace(provider.ModelCatalogForContext(contextWindowTokens))
}

func requestToolModelDescription(runtime tool.Tool, definition tool.ToolDefinition, contextWindowTokens int) string {
	if provider, ok := runtime.(contextWindowToolDescriptionProvider); ok {
		if description := strings.TrimSpace(provider.ModelDescriptionForContext(contextWindowTokens)); description != "" {
			return description
		}
	}
	return tool.ModelDescription(definition)
}

// toolExposurePolicy returns the model-visible tool pool policy.
// Tool exposure uses generic explicit hide/show policy after the seven-tool cutover.
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
		ProjectID:        strings.TrimSpace(rb.config.Workspace.ProjectID),
		ProjectName:      strings.TrimSpace(rb.config.Workspace.ProjectName),
		ProjectFolders:   projectFolderObservations(rb.config.Workspace, permissionContext),
		WorkspaceRoot:    permissionContext.WorkspaceRoot,
		ReadableRoots:    append([]string(nil), permissionContext.ReadableRoots...),
		WritableRoots:    append([]string(nil), permissionContext.WritableRoots...),
		OutputRoots:      append([]string(nil), permissionContext.OutputRoots...),
		AttachedFiles:    append([]tool.ScopedFile(nil), permissionContext.AttachedFiles...),
	}
	if rb.config.ToolAvailabilitySnapshot != nil {
		snapshot.ToolSnapshotID = rb.config.ToolAvailabilitySnapshot.SnapshotID
		snapshot.CatalogVersion = rb.config.ToolAvailabilitySnapshot.CatalogVersion
		snapshot.EnvironmentID = rb.config.ToolAvailabilitySnapshot.EnvironmentSnapshotID
	}
	if environment := rb.config.ExecutionEnvironmentSnapshot; environment != nil {
		snapshot.EnvironmentID = environment.SnapshotID
		snapshot.OperatingSystem = environment.OS
		snapshot.Architecture = environment.Architecture
		snapshot.PATHSource = environment.PATHSource
		snapshot.SkillRoots = append([]tool.SkillRootObservation(nil), environment.SkillRoots...)
		snapshot.TemporaryWorkRoot = environment.TemporaryWorkRoot
		snapshot.RefreshReason = environment.RefreshReason
		snapshot.PackageManagers = append([]string(nil), environment.AvailablePackageManagers...)
		for _, executable := range environment.Executables {
			snapshot.Executables = append(snapshot.Executables, executable.Name)
		}
	}
	rb.runtimeInfo = snapshot.ToRuntimeInfo()
	return snapshot
}

// projectFolderObservations describes the project's folder group as the model
// should see it. The readable roots are that group, ordered with the thread's
// working directory first. A folder-less project is the one case where the root
// is Awaken's own session directory, so it reports no folders rather than
// presenting an internal path as something the user picked.
func projectFolderObservations(workspace TaskScope, permissionContext tool.PermissionContext) []ProjectFolderObservation {
	if workspace.SessionWorkspace {
		return nil
	}
	folders := make([]ProjectFolderObservation, 0, len(permissionContext.ReadableRoots))
	for _, root := range permissionContext.ReadableRoots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		folders = append(folders, ProjectFolderObservation{
			Path:      root,
			IsPrimary: root == permissionContext.WorkspaceRoot,
		})
	}
	return folders
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

// runtimeSnapshotIdentity is the fingerprint that decides whether the runtime
// context the model was given still holds. The project identity belongs in it
// because renaming a project or re-pointing its folder group changes the system
// prompt without touching the working directory.
func runtimeSnapshotIdentity(snapshot RuntimeSnapshot) string {
	folders := make([]string, 0, len(snapshot.ProjectFolders))
	for _, folder := range snapshot.ProjectFolders {
		folders = append(folders, folder.Path)
	}
	return snapshot.WorkingDirectory + "|" + snapshot.ModelName + "|" + snapshot.EntryPoint +
		"|skills:" + snapshot.SkillSummary + "|tools:" + snapshot.ToolSnapshotID +
		"|env:" + snapshot.EnvironmentID + "|project:" + snapshot.ProjectID + ":" + snapshot.ProjectName +
		"|folders:" + strings.Join(folders, ";")
}

func (rb *RequestBuilder) buildCacheKeys(staticContent string, tools []tool.ToolSchema, snapshot RuntimeSnapshot) []ContextCacheKey {
	return []ContextCacheKey{
		{Name: "system_prompt", Value: fmt.Sprintf("len:%d", len(staticContent))},
		{Name: "tool_pool", Value: rb.buildToolSummary(tools)},
		{Name: "runtime_snapshot", Value: runtimeSnapshotIdentity(snapshot)},
	}
}

func (rb *RequestBuilder) buildContextDeltas(snapshot RuntimeSnapshot, tools []tool.ToolSchema) []ContextDelta {
	return []ContextDelta{
		{
			Type:    ContextDeltaRuntimeSnapshot,
			Summary: runtimeSnapshotIdentity(snapshot),
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
		result[i].Attachments = append([]convcontext.MessageAttachment(nil), message.Attachments...)
		result[i].ModelContent = tool.CloneModelContent(message.ModelContent)
		result[i].ToolCalls = append([]convcontext.ToolCall(nil), message.ToolCalls...)
		if message.ProviderOpaqueState != nil {
			state := *message.ProviderOpaqueState
			result[i].ProviderOpaqueState = &state
		}
	}
	return result
}

func cloneAttributedToolSchemas(schemas []AttributedToolSchema) []AttributedToolSchema {
	result := make([]AttributedToolSchema, len(schemas))
	copy(result, schemas)
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
	permissionContext := tool.NewTaskScopePermissionContext(
		scope.ScopeID,
		scope.Root,
		scope.ReadableRoots,
		scope.WritableRoots,
		scope.OutputRoots,
		scope.AttachedFiles,
		scope.ProtectedRoots,
		scope.TemporaryWorkRoot,
	)
	permissionContext.ProtectedRootExceptions = scope.ProtectedRootExceptions
	return permissionContext.Normalize()
}
