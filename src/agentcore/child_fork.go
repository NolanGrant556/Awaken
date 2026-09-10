package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"

	convcontext "awaken/src/context"
	"awaken/src/subagentruntime"
	"awaken/src/tool"
)

// ChildContextSeed is a durable, neutral semantic snapshot, not an executor.
// The source watermark grants bounded history access; it grants no ownership
// of the parent's pending tools, usage, inbox, gates, or mutable services.
type ChildContextSeed struct {
	ModelRoutes       map[string]subagentruntime.ModelBinding `json:"model_routes,omitempty"`
	Version           int                                     `json:"version"`
	Mode              string                                  `json:"mode"`
	OriginAgentID     string                                  `json:"origin_agent_id"`
	SourceSequenceEnd uint64                                  `json:"source_sequence_end"`
	Ancestors         []tool.HistoryRange                     `json:"ancestors,omitempty"`
	Messages          []convcontext.Message                   `json:"messages,omitempty"`
	Bundle            *PostCompactContextBundleV2             `json:"bundle,omitempty"`
	Digest            string                                  `json:"digest"`
}

const childAssignmentBoundaryInstructions = `
<awaken_child_assignment_contract>
You are a separate child Agent. Earlier messages may be inherited from your parent as background evidence and constraints. They do not assign the parent's unfinished work or delegation steps to you.
The LAST <awaken_assignment> block whose agent_id matches your host-provided Agent ID is your current assignment. Execute only that assignment and subsequent corrections addressed to you. Keep applicable user constraints from inherited history, but do not repeat parent spawn/wait instructions unless your own assignment explicitly asks for delegation.
Adjacent user messages may be combined by a provider; the assignment boundary still applies. Inherited assistant/tool messages describe another Agent's past actions, not actions you have performed.
</awaken_child_assignment_contract>`

func childAssignmentContent(prompt, agentID string) string {
	return fmt.Sprintf("<awaken_assignment agent_id=%q>\n%s\n</awaken_assignment>", agentID, prompt)
}

func projectInheritedUserMessages(messages []convcontext.Message, agentID string) []convcontext.Message {
	for i := range messages {
		message := &messages[i]
		if message.Role != convcontext.RoleUser || message.OriginAgentID == "" || message.OriginAgentID == agentID {
			continue
		}
		// Preserve the original in the durable neutral seed. This request-only
		// view keeps inherited assignment delimiters from masquerading as the
		// child's live assignment when adapters merge adjacent user messages.
		message.Content = fmt.Sprintf("<awaken_inherited_user_context source_agent=%q>\n%s\n</awaken_inherited_user_context>", message.OriginAgentID, html.EscapeString(message.Content))
	}
	return messages
}

func (r *Runner) prepareChildSpawn(sink EventSink) func(context.Context, subagentruntime.SpawnRequest) (subagentruntime.SpawnPreparation, error) {
	return func(ctx context.Context, request subagentruntime.SpawnRequest) (subagentruntime.SpawnPreparation, error) {
		roleID := request.AgentType
		if request.RoleSnapshot != nil {
			roleID = "__frozen_role__"
			if request.ModelProfileID == "" && request.RoleSnapshot.ModelHint != "" {
				request.ModelProfileID, roleID = request.RoleSnapshot.ModelHint, "__role_model_hint__"
			}
		}
		if request.Host != nil && request.Host.Skill != nil {
			request.ModelProfileID, request.ReasoningEffort = request.Host.Skill.Model, request.Host.Skill.Effort
			if request.ModelProfileID != "" {
				roleID = "__skill_model_hint__"
			}
		}
		model, err := resolveChildModel(ctx, r.config, request.ModelProfileID, request.ReasoningEffort, roleID)
		if err != nil {
			return subagentruntime.SpawnPreparation{}, err
		}
		config := r.config
		config.SubagentModelConfig = model
		seed, err := r.captureChildContext(request.ContextMode, request.RecentTurns)
		if err != nil {
			return subagentruntime.SpawnPreparation{}, err
		}
		seed.ModelRoutes = make(map[string]subagentruntime.ModelBinding)
		for purpose, route := range map[string]ModelConfig{"compact_summary": config.CompactModelConfig, "hook_prompt": config.HookPromptModelConfig, "hook_agent": config.HookAgentModelConfig} {
			if route.ProfileID != "" {
				seed.ModelRoutes[purpose] = subagentruntime.ModelBinding{ProfileID: route.ProfileID, ReasoningEffort: route.ReasoningEffort, ModelID: route.Model, Source: route.BindingSource, Revision: route.ConfigSnapshotRevision}
			}
		}
		seed.Digest = ""
		unsigned, _ := json.Marshal(seed)
		seed.Digest = digestText(string(unsigned))
		encoded, err := json.Marshal(seed)
		if err != nil {
			return subagentruntime.SpawnPreparation{}, err
		}
		return subagentruntime.SpawnPreparation{
			Starter: r.scopedChildStarterWithConfig(sink, config), ContextSeed: encoded,
			Model: subagentruntime.ModelBinding{ProfileID: model.ProfileID, ModelID: model.Model,
				ReasoningEffort: model.ReasoningEffort, Source: model.BindingSource, Revision: model.ConfigSnapshotRevision},
		}, nil
	}
}

func resolveChildModel(ctx context.Context, parent AgentConfig, profileID, effort, roleID string) (ModelConfig, error) {
	model := parent.ModelConfig
	model.BindingSource = "inherited"
	if parent.SubagentModelConfig.Model != "" {
		model = parent.SubagentModelConfig
		if model.BindingSource == "" {
			model.BindingSource = "host_route"
		}
	}
	source := "assignment_pin"
	if roleID == "__role_model_hint__" {
		source = "role_pin"
	}
	if roleID == "__skill_model_hint__" {
		source = "skill_pin"
	}
	if profileID == "" {
		if role, ok := subagentruntime.LookupRole(roleID); ok {
			profileID = strings.TrimSpace(role.ModelHint)
			source = "role_pin"
		}
	}
	if profileID == "" && effort == "" {
		return model, nil
	}
	if parent.ResolveModelBinding == nil {
		return ModelConfig{}, fmt.Errorf("model binding resolver is unavailable")
	}
	if profileID == "" {
		profileID = model.ProfileID
	}
	// Role/Skill hints may name a unique configured model. Never synthesize a
	// provider by replacing only Model on an unrelated profile.
	if (source == "role_pin" || source == "skill_pin") && parent.ListModelBindings != nil {
		options, err := parent.ListModelBindings(ctx)
		if err != nil {
			return ModelConfig{}, err
		}
		matched := ""
		for _, option := range options {
			if option.ProfileID == profileID {
				matched = profileID
				break
			}
		}
		if matched == "" {
			for _, option := range options {
				if option.ModelID == profileID || option.Name == profileID {
					if matched != "" {
						return ModelConfig{}, fmt.Errorf("model hint %q is ambiguous; select a configured profile_id", profileID)
					}
					matched = option.ProfileID
				}
			}
		}
		if matched == "" {
			return ModelConfig{}, fmt.Errorf("model hint %q has no configured profile", profileID)
		}
		profileID = matched
	}
	if effort == "" && profileID == parent.ModelConfig.ProfileID {
		effort = parent.ModelConfig.ReasoningEffort
	}
	resolved, err := parent.ResolveModelBinding(ctx, profileID, effort)
	if err != nil {
		return ModelConfig{}, err
	}
	resolved.BindingSource = source
	return resolved, nil
}

func (r *Runner) captureChildContext(mode string, turns int) (ChildContextSeed, error) {
	if mode == "" {
		mode = "fresh"
	}
	seed := ChildContextSeed{Version: 1, Mode: mode, OriginAgentID: "root"}
	if r.config.SubagentOrigin != nil {
		seed.OriginAgentID = r.config.SubagentOrigin.AgentID
	}
	if reader, ok := r.recorder.(SemanticSequenceReader); ok {
		seed.SourceSequenceEnd = reader.LastAssignedSequence()
	}
	if mode != "fresh" {
		seed.Ancestors = append([]tool.HistoryRange(nil), r.config.HistoryAncestors...)
		seed.Ancestors = append(seed.Ancestors, tool.HistoryRange{AgentID: seed.OriginAgentID, ThroughSeq: seed.SourceSequenceEnd})
		messages, err := closedForkMessages(r.conversation.Context.GetMessages())
		if err != nil {
			return seed, err
		}
		if mode == "recent" {
			if turns < 1 {
				return seed, fmt.Errorf("recent fork requires a positive turn count")
			}
			groups := compactUserTurnGroups(messages)
			closed := make([][]convcontext.Message, 0, len(groups))
			for _, group := range groups {
				if len(group) > 1 && group[len(group)-1].Role == convcontext.RoleAssistant && len(group[len(group)-1].ToolCalls) == 0 {
					closed = append(closed, group)
				}
			}
			retained := make([]convcontext.Message, 0)
			// Keep existing user constraints even when their tool traces are
			// outside the selected recent tasks. Budgeting still covers them.
			for _, group := range closed[:max(0, len(closed)-turns)] {
				retained = append(retained, group[0])
			}
			for _, group := range closed[max(0, len(closed)-turns):] {
				retained = append(retained, group...)
			}
			// The current user message may carry constraints for the fork even
			// though its unfinished tool traces cannot be inherited.
			if len(groups) > 0 {
				last := groups[len(groups)-1]
				if len(last) == 1 || last[len(last)-1].Role != convcontext.RoleAssistant || len(last[len(last)-1].ToolCalls) != 0 {
					retained = append(retained, last[0])
				}
			}
			for _, message := range messages {
				if message.Origin == convcontext.MessageOriginCompactSummary || message.CompactBoundaryID != "" {
					retained = append([]convcontext.Message{message}, retained...)
				}
			}
			messages = retained
		}
		for i := range messages {
			if messages[i].OriginAgentID == "" {
				messages[i].OriginAgentID = seed.OriginAgentID
			}
		}
		seed.Messages = messages
		if r.config.RestoredContextBundleV2 != nil {
			body, err := json.Marshal(r.config.RestoredContextBundleV2)
			if err != nil {
				return seed, err
			}
			var bundle PostCompactContextBundleV2
			if err := json.Unmarshal(body, &bundle); err != nil {
				return seed, err
			}
			bundle.ProviderOpaqueState = NotMaterializedSection[ProviderOpaqueCompactionRef]()
			bundle.PendingSubagents = nil
			bundle.ActiveGateRefs = nil
			bundle.RunPlanRef = UnavailableSnapshotRef("new_agent_plan")
			if err := bundle.Seal(); err != nil {
				return seed, err
			}
			seed.Bundle = &bundle
		}
	}
	seed.Digest = ""
	digest, err := digestCanonicalValue(seed)
	seed.Digest = digest
	return seed, err
}

func closedForkMessages(messages []convcontext.Message) ([]convcontext.Message, error) {
	messages = stripProviderOpaqueCompactionState(messages)
	cut := len(messages)
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == convcontext.RoleAssistant && len(messages[i].ToolCalls) > 0 {
			pending := map[string]bool{}
			for _, call := range messages[i].ToolCalls {
				pending[call.ID] = true
			}
			for _, message := range messages[i+1:] {
				if message.Role == convcontext.RoleTool {
					delete(pending, message.ToolCallID)
				}
			}
			if len(pending) > 0 {
				cut = i
			}
		}
	}
	messages = messages[:cut]
	if err := convcontext.ValidateToolResultPairing(messages); err != nil {
		return nil, fmt.Errorf("fork boundary: %w", err)
	}
	return messages, nil
}

func installChildContextSeed(raw json.RawMessage, config *AgentConfig, conversation *convcontext.Conversation) error {
	if len(raw) == 0 {
		return nil
	}
	var seed ChildContextSeed
	if err := json.Unmarshal(raw, &seed); err != nil {
		return err
	}
	digest := seed.Digest
	seed.Digest = ""
	expected, err := digestCanonicalValue(seed)
	if err != nil || seed.Version != 1 || digest != expected {
		return fmt.Errorf("invalid child context seed")
	}
	if err := convcontext.ValidateToolResultPairing(seed.Messages); err != nil {
		return err
	}
	config.HistoryAncestors = append([]tool.HistoryRange(nil), seed.Ancestors...)
	conversation.Context.SetMessages(stripProviderOpaqueCompactionState(seed.Messages))
	if seed.Bundle != nil {
		if err := seed.Bundle.Validate(); err != nil {
			return err
		}
		config.RestoredContextBundleV2 = seed.Bundle
		config.CompactContextBlocks = CompactContextBlocksFromBundleV2(seed.Bundle)
		config.CompactedMemoryContext = CompactedMemoryContextFromBundleV2(seed.Bundle)
		config.SkillContextBlocks = SkillContextBlocksFromBundleV2(seed.Bundle)
	}
	return nil
}
