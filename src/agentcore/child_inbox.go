package agentcore

import (
	"context"
	"encoding/json"
	"fmt"

	convcontext "awaken/src/context"
	"awaken/src/subagentruntime"
)

func (r *Runner) retrievedSubagentTerminals(result ToolExecutionResult) []string {
	if result.Name != "subagent" || result.IsError || r.subagentController == nil {
		return nil
	}
	var payload struct {
		Task  subagentruntime.TaskSnapshot   `json:"task"`
		Tasks []subagentruntime.TaskSnapshot `json:"tasks"`
	}
	if json.Unmarshal([]byte(result.Content), &payload) != nil {
		return nil
	}
	var ids []string
	for _, snapshot := range append(payload.Tasks, payload.Task) {
		if snapshot.TaskID == "" || !snapshot.Terminal() {
			continue
		}
		actual, err := r.subagentController.Status(snapshot.TaskID)
		if err == nil && actual.Terminal() {
			ids = append(ids, snapshot.TaskID)
		}
	}
	return ids
}

func restoreChildModelRoutes(ctx context.Context, raw json.RawMessage, config *AgentConfig) error {
	if len(raw) == 0 {
		return nil
	}
	var seed ChildContextSeed
	if err := json.Unmarshal(raw, &seed); err != nil {
		return err
	}
	for purpose, binding := range seed.ModelRoutes {
		route := ModelConfig{ProfileID: binding.ProfileID, ReasoningEffort: binding.ReasoningEffort, BindingSource: binding.Source}
		switch purpose {
		case "compact_summary":
			config.CompactModelConfig = route
		case "hook_prompt":
			config.HookPromptModelConfig = route
		case "hook_agent":
			config.HookAgentModelConfig = route
		}
	}
	return revalidateChildAuxiliaryModels(ctx, config)
}

func revalidateChildAuxiliaryModels(ctx context.Context, config *AgentConfig) error {
	for _, route := range []*ModelConfig{&config.CompactModelConfig, &config.HookPromptModelConfig, &config.HookAgentModelConfig} {
		if route.ProfileID == "" {
			continue
		}
		if config.ResolveModelBinding == nil {
			return fmt.Errorf("saved auxiliary model resolver is unavailable")
		}
		resolved, err := config.ResolveModelBinding(ctx, route.ProfileID, route.ReasoningEffort)
		if err != nil {
			return err
		}
		resolved.BindingSource = route.BindingSource
		*route = resolved
	}
	return nil
}

func (r *Runner) appendContextMessage(content, id string) error {
	for _, message := range r.conversation.Context.GetMessages() {
		if message.MessageID == id {
			return nil
		}
	}
	if r.recorder != nil {
		payload := map[string]any{"event_type": "agent_context_message", "message_id": id, "content": content}
		if r.config.SubagentOrigin != nil {
			payload["agent_id"], payload["parent_agent_id"], payload["subagent_task_id"] = r.config.SubagentOrigin.AgentID, r.config.SubagentOrigin.ParentAgentID, r.config.SubagentTaskID
			payload["attempt_id"] = r.config.AttemptID
		}
		record := NewSemanticRecord(SemanticKindRuntimeNotice, r.threadID, r.runID, "", "", r.sessionID, payload)
		ctx, cancel := durablePersistenceContext(context.Background())
		defer cancel()
		if err := r.recorder.Append(ctx, record); err != nil {
			return err
		}
		if err := r.recorder.Flush(ctx); err != nil {
			return err
		}
	}
	r.conversation.SyntheticUserInput(content, id)
	return nil
}

func restoreChildContext(state *ChildRestoreState, config *AgentConfig, conversation *convcontext.Conversation) error {
	if state == nil {
		return fmt.Errorf("saved Agent context is unavailable")
	}
	if err := installChildContextSeed(state.ContextSeed, config, conversation); err != nil {
		return err
	}
	if state.Bundle != nil {
		if err := state.Bundle.Validate(); err != nil {
			return err
		}
		config.RestoredContextBundleV2 = state.Bundle
		config.CompactContextBlocks = CompactContextBlocksFromBundleV2(state.Bundle)
		config.CompactedMemoryContext = CompactedMemoryContextFromBundleV2(state.Bundle)
		conversation.Context.SetMessages(state.Messages)
	} else {
		conversation.Context.SetMessages(append(conversation.Context.GetMessages(), cloneContextMessages(state.Messages)...))
	}
	if err := convcontext.ValidateToolResultPairing(conversation.Context.GetMessages()); err != nil {
		return fmt.Errorf("saved Agent tool state requires recovery: %w", err)
	}
	config.ContextRevision = state.ContextRevision
	config.RestoredRunPlan = state.RunPlan
	config.AgentTurnsUsed, config.AgentToolsUsed = state.ModelTurns, state.ToolExecutions
	config.RestoredContextWindowUsageAnchor = CloneContextWindowUsageAnchor(state.UsageAnchor)
	if len(state.SkillBlocks) > 0 {
		config.SkillContextBlocks = append([]ContextBlock(nil), state.SkillBlocks...)
	}
	config.ActivatedDeferredTools = appendUniqueStrings(config.ActivatedDeferredTools, state.ActivatedTools...)
	return nil
}
