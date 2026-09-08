package agentcore

import (
	"awaken/src/agentcore/executors"
	"awaken/src/context"
	"awaken/src/hookruntime"
	"awaken/src/skillruntime"
	"awaken/src/skills"
	"awaken/src/subagentruntime"
	"awaken/src/tool"
	stdcontext "context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (r *Runner) injectSyntheticToolResultsForCancel() {
	messages := r.conversation.Context.GetMessages()

	pendingToolCalls := map[string]string{}
	for _, msg := range messages {
		if msg.Role == context.RoleAssistant {
			for _, tc := range msg.ToolCalls {
				if tc.ID != "" {
					pendingToolCalls[tc.ID] = tc.Function.Name
				}
			}
		}
		if msg.Role == context.RoleTool {
			delete(pendingToolCalls, msg.ToolCallID)
		}
	}

	if len(pendingToolCalls) == 0 {
		return
	}

	injected := map[string]bool{}
	for _, msg := range messages {
		if msg.Role != context.RoleAssistant {
			continue
		}
		for _, tc := range msg.ToolCalls {
			if tc.ID == "" || injected[tc.ID] {
				continue
			}
			if _, orphan := pendingToolCalls[tc.ID]; !orphan {
				continue
			}
			r.conversation.Context.AddToolResultWithError(tc.ID, tc.Function.Name, "工具执行已被取消")
			injected[tc.ID] = true
		}
	}
}

func toolRuntimeConfigFromAgentConfig(config AgentConfig) ToolRuntimeConfig {
	permissionContext := permissionContextFromTaskScope(config.Workspace)
	permissionContext.MediaArtifacts = append(
		[]tool.ScopedMediaArtifact(nil),
		config.MediaArtifactReferences...,
	)
	cfg := ToolRuntimeConfig{
		PermissionContext:            permissionContext.Normalize(),
		AutoToolPolicy:               config.AutoToolPolicy,
		Profile:                      config.ExecutionProfile,
		SubagentBoundary:             config.SubagentBoundary,
		SubagentOrigin:               config.SubagentOrigin,
		SubagentTaskID:               config.SubagentTaskID,
		ModelProfileID:               config.ModelConfig.ProfileID,
		ToolAvailabilitySnapshot:     config.ToolAvailabilitySnapshot,
		ExecutionEnvironmentSnapshot: config.ExecutionEnvironmentSnapshot,
		ModelCapabilities:            config.ModelConfig.Capabilities,
		ToolSessionState:             config.ToolSessionState,
		ModelContentStore:            config.ModelContentStore,
		ArchivedHistoryService:       config.ArchivedHistoryService,
	}
	if config.ComputerRuntime != nil {
		cfg.ComputerExecutor = executors.NewComputerExecutor(config.ComputerRuntime)
	}
	return cfg
}

func (r *Runner) handleXMLFallback(ctx stdcontext.Context, response string, events *runEventEmitter) (bool, string) {
	parsed, err := ParseAction(response)
	if err != nil {
		return false, ""
	}

	if parsed.ToolName == "final" || parsed.ToolName == "总结" {
		r.conversation.AssistantOutput(parsed.Input, nil)
		if events != nil && !events.textWasStartedThisTurn() && parsed.Input != "" {
			events.emitStandaloneText(parsed.Input)
		}
		return true, parsed.Input
	}

	toolCall := context.ToolCall{
		ID:   context.GenerateToolCallID(),
		Type: "function",
		Function: context.FunctionCall{
			Name:      parsed.ToolName,
			Arguments: argumentsForXMLFallback(parsed.ToolName, parsed.Input),
		},
	}
	results := r.executeToolCalls(ctx, []context.ToolCall{toolCall}, events)
	r.conversation.ToolExchange(response, []context.ToolCall{toolCall}, toolExecutionResultsToContext(results))
	return true, ""
}

func (r *Runner) executeToolCalls(ctx stdcontext.Context, toolCalls []context.ToolCall, events *runEventEmitter) []ToolExecutionResult {
	results := make([]ToolExecutionResult, 0, len(toolCalls))
	runCtx, cancel := stdcontext.WithCancel(ctx)
	defer cancel()
	var terminateReason TerminalReason
	for _, call := range toolCalls {
		if events != nil {
			if err := events.persistenceFailure(); err != nil {
				results = append(results, syntheticPersistenceFailureResult(call, err))
				terminateReason = TerminalReasonPersistenceError
				cancel()
				continue
			}
		}
		if runCtx.Err() != nil {
			results = append(results, syntheticCancelledToolResult(call))
			continue
		}
		if terminateReason != "" {
			results = append(results, syntheticCancelledToolResult(call))
			continue
		}

		before := BeforeCallDecision{Action: CallExecute}
		if r.failureSupervisor != nil {
			before = r.failureSupervisor.BeforeCall(call)
		}
		if before.Action != CallExecute {
			result := SyntheticBlockedResult(call, before)
			if events != nil {
				if err := events.emitToolCall(runCtx, call); err != nil {
					cancel()
					results = append(results, syntheticPersistenceFailureResult(call, err))
					terminateReason = TerminalReasonPersistenceError
					continue
				}
				// Persist and canonically project the blocked recovery decision before
				// its synthetic result, matching causal live/replay order.
				events.emitRecoveryDecision(call, before.RecoveryOptions, before.ErrorCode, true)
				if err := events.emitToolResultBatch(runCtx, result, nil, nil); err != nil {
					cancel()
					terminateReason = TerminalReasonPersistenceError
				}
			}
			results = append(results, result)
			if before.Action == CallStopRun {
				terminateReason = before.TerminalReason
				if terminateReason == "" {
					terminateReason = TerminalReasonRepeatedToolFailure
				}
				cancel()
			}
			continue
		}

		if events != nil {
			if err := events.emitToolCall(runCtx, call); err != nil {
				cancel()
				results = append(results, syntheticPersistenceFailureResult(call, err))
				terminateReason = TerminalReasonPersistenceError
				continue
			}
		}
		hookCall := call
		preSubject := hookToolSubject(hookCall, nil)
		preHook, hookErr := r.dispatchHook(runCtx, events, hookruntime.EventPreToolUse, preSubject)
		if hookErr != nil {
			cancel()
			results = append(results, syntheticPersistenceFailureResult(call, hookErr))
			terminateReason = TerminalReasonPersistenceError
			continue
		}
		if preHook.Action == "update_input" && len(preHook.UpdatedInput) > 0 {
			hookCall.Function.Arguments = string(preHook.UpdatedInput)
		}
		if preHook.Action == "block_tool" {
			result := syntheticHookBlockedToolResult(hookCall, preHook.Reason)
			results = append(results, result)
			if events != nil {
				if err := events.emitToolResultBatch(runCtx, result, nil, nil); err != nil {
					cancel()
					terminateReason = TerminalReasonPersistenceError
				}
			}
			continue
		}
		// Re-check after durable tool_call append; never execute tools after journal failure.
		if runCtx.Err() != nil {
			results = append(results, syntheticCancelledToolResult(call))
			continue
		}

		if result, rejected := r.admitToolCall(hookCall, results); rejected {
			results = append(results, result)
			if events != nil {
				if err := events.emitToolResultBatch(runCtx, result, nil, nil); err != nil {
					cancel()
					terminateReason = TerminalReasonPersistenceError
				}
			}
			continue
		}

		if r.failureSupervisor != nil {
			r.failureSupervisor.NoteRealExecution(before.CallSignature)
		}

		var buffer *artifactBufferingSink
		if events != nil {
			buffer = &artifactBufferingSink{inner: events.scopedSink()}
			r.toolRuntime.sink = buffer
		}
		executionCtx := runCtx
		agentID := "root"
		if r.config.SubagentOrigin != nil && strings.TrimSpace(r.config.SubagentOrigin.AgentID) != "" {
			agentID = strings.TrimSpace(r.config.SubagentOrigin.AgentID)
		}
		identity := tool.ToolExecutionIdentity{
			RunID:          r.runID,
			AgentID:        agentID,
			SubagentTaskID: strings.TrimSpace(r.config.SubagentTaskID),
			ModelProfileID: strings.TrimSpace(r.config.ModelConfig.ProfileID),
			ToolCallID:     call.ID,
			ToolName:       call.Function.Name,
		}
		if events != nil {
			identity.TurnID = events.turnID()
		}
		executionCtx = tool.ContextWithToolExecutionIdentity(executionCtx, identity)
		if r.subagentController != nil && r.config.SubagentOrigin == nil {
			executionCtx = subagentruntime.WithController(executionCtx, r.subagentController)
		}
		var activationPlan *skillruntime.SkillActivationPlan
		if isSkillInvocationToolName(call.Function.Name) {
			executionCtx = skillruntime.WithActivationScope(
				runCtx,
				r.currentImplicitParentPool(),
				func(observerCtx stdcontext.Context, plan *skillruntime.SkillActivationPlan) error {
					activationPlan = plan.Clone()
					return r.persistImplicitSkillActivation(observerCtx, events, call, activationPlan)
				},
			)
			executionCtx = skillruntime.WithChildAgentStarter(executionCtx, skillruntime.ChildAgentStarterFunc(
				func(childCtx stdcontext.Context, request skillruntime.SkillForkRequest) (skillruntime.SkillForkResult, error) {
					return r.startSkillChild(childCtx, request, events.sink)
				},
			))
		}
		var deferredToolsLoadedByFind []string
		if call.Function.Name == tool.ToolFindToolName {
			if r.reqBuilder != nil {
				executionCtx = tool.ContextWithDeferredToolSearchSpace(executionCtx,
					r.reqBuilder.toolDisclosureProjection().SearchSpace)
			}
			executionCtx = tool.ContextWithDeferredToolActivationObserver(executionCtx,
				func(observerCtx stdcontext.Context, names []string) error {
					newly := r.newlyActivatedDeferredToolNames(names)
					if len(newly) == 0 {
						return nil
					}
					if err := r.persistDeferredToolActivation(observerCtx, events, call, newly, "tool_find"); err != nil {
						return err
					}
					deferredToolsLoadedByFind = append(deferredToolsLoadedByFind, newly...)
					return nil
				})
		}
		result := r.toolRuntime.Execute(executionCtx, hookCall)
		if offloadErr := r.offloadLargeToolResult(runCtx, &result); offloadErr != nil {
			cancel()
			result = syntheticPersistenceFailureResult(call, fmt.Errorf("tool result offload: %w", offloadErr))
			terminateReason = TerminalReasonPersistenceError
		}
		if events != nil {
			r.toolRuntime.sink = events.scopedSink()
			if persistErr := events.persistenceFailure(); persistErr != nil {
				cancel()
				results = append(results, syntheticPersistenceFailureResult(call, persistErr))
				terminateReason = TerminalReasonPersistenceError
				continue
			}
		}

		var recoveryRequired bool
		var recoveryOptions []string
		var recoveryErrorCode string
		if r.failureSupervisor != nil {
			after := r.failureSupervisor.AfterOutcome(call, result.Outcome)
			if len(after.RecoveryOptions) > 0 || after.RecoveryRequired {
				result.Outcome = ApplyRecoveryToOutcome(result.Outcome, after.RecoveryOptions, after.RecoveryRequired)
				result.IsError = result.Outcome.IsError()
			}
			if after.RecoveryRequired {
				recoveryRequired = true
				recoveryOptions = append([]string(nil), after.RecoveryOptions...)
				recoveryErrorCode = result.Outcome.ErrorCode
			}

		}

		r.applyImplicitSkillActivation(call.Function.Name, result, activationPlan)
		r.applyDeferredToolActivation(runCtx, events, call, deferredToolsLoadedByFind)
		results = append(results, result)
		if events != nil {
			artifacts := []AwakenEvent{}
			if buffer != nil {
				artifacts = buffer.artifacts
			}
			if toolResultAppliedMutation(result) &&
				(len(result.AffectedPaths) > 0 || len(result.TrashedPaths) > 0 || len(artifacts) > 0) {
				events.markMutation()
			}
			var semanticFacts []AwakenEvent
			if buffer != nil {
				semanticFacts = buffer.semanticFacts
			}
			if err := events.emitToolResultBatch(runCtx, result, artifacts, semanticFacts); err != nil {
				cancel()
				terminateReason = TerminalReasonPersistenceError
			} else if recoveryRequired {
				// A real execution result is the cause; publish it durably before the
				// recovery directive so live and replay never ask for recovery before
				// showing the failure that requires it.
				events.emitRecoveryDecision(call, recoveryOptions, recoveryErrorCode, false)
			}
		}
		if terminateReason == "" {
			postEvent := hookruntime.EventPostToolUse
			if result.IsError || result.Outcome.IsError() {
				postEvent = hookruntime.EventPostToolUseFailure
			}
			if _, hookErr := r.dispatchHook(runCtx, events, postEvent, hookToolSubject(hookCall, &result)); hookErr != nil {
				cancel()
				terminateReason = TerminalReasonPersistenceError
			}
		}
	}
	if terminateReason != "" && events != nil {
		events.setPendingTerminalReason(terminateReason)
	}
	return results
}

func (r *Runner) offloadLargeToolResult(ctx stdcontext.Context, result *ToolExecutionResult) error {
	if r == nil || result == nil || r.config.ToolResultOffloadStore == nil {
		return nil
	}
	content := result.Content
	if result.Outcome.Status != "" && len(result.Outcome.Content) > len(content) {
		content = result.Outcome.Content
	}
	if len([]byte(content)) <= toolResultOffloadThresholdBytes {
		return nil
	}
	identity := result.executionIdentity
	if strings.TrimSpace(identity.ToolCallID) == "" {
		identity = tool.ToolExecutionIdentity{
			RunID:          r.runID,
			AgentID:        "root",
			SubagentTaskID: strings.TrimSpace(r.config.SubagentTaskID),
			ModelProfileID: strings.TrimSpace(r.config.ModelConfig.ProfileID),
			ToolCallID:     result.ToolCallID,
			ToolName:       result.Name,
		}
		if r.config.SubagentOrigin != nil && strings.TrimSpace(r.config.SubagentOrigin.AgentID) != "" {
			identity.AgentID = strings.TrimSpace(r.config.SubagentOrigin.AgentID)
		}
	}
	receipt, err := r.config.ToolResultOffloadStore.OffloadToolResult(ctx, r.threadID, identity, content)
	if err != nil {
		return err
	}
	projection := fmt.Sprintf(
		"[TOOL_RESULT_OFFLOADED]\nstorage_key=%s\nsha256=%s\nbytes=%d\npreview=%s",
		receipt.StorageKey, receipt.Digest, receipt.ByteSize, receipt.Preview,
	)
	result.Content = projection
	result.Truncated = true
	if result.Outcome.Status != "" {
		result.Outcome.Content = projection
		result.Outcome.Truncated = true
		result.Outcome.Details = map[string]any{
			"result_storage_key": receipt.StorageKey,
			"result_sha256":      receipt.Digest,
			"result_bytes":       receipt.ByteSize,
		}
	}
	return nil
}

func (r *Runner) hydrateOffloadedToolResults(ctx stdcontext.Context, messages []context.Message) error {
	if r == nil || r.config.ToolResultOffloadStore == nil {
		return nil
	}
	for i := range messages {
		if messages[i].Role != context.RoleTool {
			continue
		}
		marker := messages[i].Content
		var envelope map[string]any
		if _, ok := parseToolResultOffloadReceipt(marker); !ok {
			if json.Unmarshal([]byte(marker), &envelope) != nil {
				continue
			}
			content, _ := envelope["content"].(string)
			marker = content
		}
		receipt, ok := parseToolResultOffloadReceipt(marker)
		if !ok {
			continue
		}
		content, err := r.config.ToolResultOffloadStore.HydrateOffloadedToolResult(ctx, r.threadID, receipt)
		if err != nil {
			return fmt.Errorf("hydrate offloaded tool result %q: %w", messages[i].ToolCallID, err)
		}
		if envelope == nil {
			messages[i].Content = content
			continue
		}
		envelope["content"] = content
		envelope["truncated"] = false
		hydrated, err := json.Marshal(envelope)
		if err != nil {
			return fmt.Errorf("encode hydrated tool result %q: %w", messages[i].ToolCallID, err)
		}
		messages[i].Content = string(hydrated)
	}
	return nil
}

func parseToolResultOffloadReceipt(content string) (ToolResultOffloadReceipt, bool) {
	if !strings.HasPrefix(strings.TrimSpace(content), "[TOOL_RESULT_OFFLOADED]") {
		return ToolResultOffloadReceipt{}, false
	}
	var receipt ToolResultOffloadReceipt
	for _, line := range strings.Split(content, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found {
			continue
		}
		switch key {
		case "storage_key":
			receipt.StorageKey = strings.TrimSpace(value)
		case "sha256":
			receipt.Digest = strings.TrimSpace(value)
		case "bytes":
			_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d", &receipt.ByteSize)
		}
	}
	return receipt, strings.TrimSpace(receipt.StorageKey) != ""
}

func (r *Runner) currentImplicitParentPool() []string {
	if r == nil {
		return nil
	}
	if r.reqBuilder == nil {
		return nil
	}
	schemas := r.reqBuilder.getTools()
	names := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		name := strings.TrimSpace(schema.Function.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func (r *Runner) persistImplicitSkillActivation(
	ctx stdcontext.Context,
	events *runEventEmitter,
	call context.ToolCall,
	plan *skillruntime.SkillActivationPlan,
) error {
	if r == nil || plan == nil {
		return nil
	}
	turnID := ""
	threadID := r.threadID
	runID := r.runID
	if events != nil {
		turnID = events.turnID()
		threadID = events.threadID
		runID = events.runID
	}
	payload := plan.DurablePayload()
	payload["skill_path"] = skillActivationSourcePath(plan)
	payload["content"] = plan.MainBody
	payload["invoked_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	if r.config.SubagentOrigin != nil {
		payload["agent_id"] = r.config.SubagentOrigin.AgentID
	}
	records := []SemanticRecord{NewSemanticRecord(
		SemanticKindSkillActivated,
		threadID,
		runID,
		turnID,
		call.ID,
		r.sessionID,
		payload,
	)}
	for _, receipt := range plan.ResourceReceipts {
		records = append(records, NewSemanticRecord(
			SemanticKindResourceReceipt,
			threadID,
			runID,
			turnID,
			call.ID,
			r.sessionID,
			receipt.DurablePayload(),
		))
	}
	if events != nil && events.recorder != nil {
		return events.persistDurableBatch(ctx, "skill_activation", records)
	}
	if r.recorder != nil {
		persistCtx, cancel := durablePersistenceContext(ctx)
		defer cancel()
		if err := r.recorder.AppendBatch(persistCtx, records); err != nil {
			return fmt.Errorf("%w: phase=skill_activation: %v", ErrPersistenceFailed, err)
		}
	}
	return nil
}

func (r *Runner) applyImplicitSkillActivation(
	toolName string,
	result ToolExecutionResult,
	plan *skillruntime.SkillActivationPlan,
) {
	if r == nil || result.IsError || !isSkillInvocationToolName(toolName) ||
		plan == nil || plan.CompatibilityStatus != skillruntime.CompatibilityOK {
		return
	}
	if plan.Context == skills.ContextFork {
		// A forked Skill body belongs exclusively to its child conversation. The
		// parent receives only the child result through the Skill tool result.
		return
	}
	if body := strings.TrimSpace(plan.MainBody); body != "" {
		block := ContextBlock{
			ID:         "skill_activation_" + plan.SkillID,
			SkillID:    plan.SkillID,
			SourcePath: skillActivationSourcePath(plan),
			InvokedAt:  time.Now().UTC(),
			Content:    body,
		}
		// Re-activating the same Skill within one run must replace its
		// existing block rather than append a duplicate. Restart recovery
		// (skillContextBlocksFromSemanticRecords) already dedupes by
		// SkillID and keeps the latest; live runs must match that
		// latest-wins semantics instead of diverging from it.
		replaced := false
		for i, existing := range r.config.SkillContextBlocks {
			if existing.SkillID == plan.SkillID {
				r.config.SkillContextBlocks[i] = block
				replaced = true
				break
			}
		}
		if !replaced {
			r.config.SkillContextBlocks = append(r.config.SkillContextBlocks, block)
		}
	}
	r.rebuildRequestBuilder()
}

// newlyActivatedDeferredToolNames filters candidate names down to deferrable
// tools (MCP-origin or the builtin auxiliary deferral group) present in the
// pinned run snapshot, visible under the run's exposure policy, and not
// activated yet. The per-group enablement gate stays with the callers.
func (r *Runner) newlyActivatedDeferredToolNames(names []string) []string {
	if r == nil || r.config.ToolAvailabilitySnapshot == nil {
		return nil
	}
	existing := make(map[string]bool, len(r.config.ActivatedDeferredTools))
	for _, name := range r.config.ActivatedDeferredTools {
		existing[name] = true
	}
	exposure := r.config.ToolExposure
	if exposure.Mode == "" && len(exposure.ExtraVisible) == 0 && len(exposure.ExtraHidden) == 0 {
		exposure = tool.DefaultToolExposurePolicy()
	}
	var disclosure toolDisclosureProjection
	if r.reqBuilder != nil {
		disclosure = r.reqBuilder.toolDisclosureProjection()
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || existing[name] || seen[name] {
			continue
		}
		entry, err := r.config.ToolAvailabilitySnapshot.Lookup(name)
		if err != nil {
			continue
		}
		switch {
		case tool.IsMCPToolEntry(entry):
			if !disclosure.MCPEnabled {
				continue
			}
		case tool.IsDeferrableBuiltinEntry(entry):
			if !disclosure.BuiltinEnabled {
				continue
			}
		default:
			continue
		}
		if !tool.IsToolExposed(name, exposure) {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// persistDeferredToolActivation appends the durable activation fact before
// the in-memory tool pool changes; restore replays it into
// ActivatedDeferredTools.
func (r *Runner) persistDeferredToolActivation(
	ctx stdcontext.Context,
	events *runEventEmitter,
	call context.ToolCall,
	names []string,
	source string,
) error {
	if r == nil || len(names) == 0 {
		return nil
	}
	turnID := ""
	threadID := r.threadID
	runID := r.runID
	if events != nil {
		turnID = events.turnID()
		threadID = events.threadID
		runID = events.runID
	}
	payload := map[string]any{
		"tools":        append([]string(nil), names...),
		"source":       source,
		"activated_at": time.Now().UTC().Format(time.RFC3339Nano),
	}
	if r.config.SubagentOrigin != nil {
		payload["agent_id"] = r.config.SubagentOrigin.AgentID
	}
	records := []SemanticRecord{NewSemanticRecord(
		SemanticKindDeferredToolsActivated,
		threadID,
		runID,
		turnID,
		call.ID,
		r.sessionID,
		payload,
	)}
	if events != nil && events.recorder != nil {
		return events.persistDurableBatch(ctx, "deferred_tool_activation", records)
	}
	if r.recorder != nil {
		persistCtx, cancel := durablePersistenceContext(ctx)
		defer cancel()
		if err := r.recorder.AppendBatch(persistCtx, records); err != nil {
			return fmt.Errorf("%w: phase=deferred_tool_activation: %v", ErrPersistenceFailed, err)
		}
	}
	return nil
}

// applyDeferredToolActivation merges freshly loaded deferred tools into the
// run config so the next request projects their full definitions. A direct
// call to a still-deferred tool auto-loads it (lenient semantics): the call
// already executed through the pinned snapshot, so withholding the schema
// would only force retry churn.
func (r *Runner) applyDeferredToolActivation(
	ctx stdcontext.Context,
	events *runEventEmitter,
	call context.ToolCall,
	loadedByFind []string,
) {
	if r == nil {
		return
	}
	names := loadedByFind
	if call.Function.Name != tool.ToolFindToolName && len(names) == 0 {
		newly := r.newlyActivatedDeferredToolNames([]string{call.Function.Name})
		if len(newly) > 0 {
			if err := r.persistDeferredToolActivation(ctx, events, call, newly, "direct_call"); err == nil {
				names = newly
			}
		}
	}
	if len(names) == 0 {
		return
	}
	existing := make(map[string]bool, len(r.config.ActivatedDeferredTools))
	for _, name := range r.config.ActivatedDeferredTools {
		existing[name] = true
	}
	changed := false
	for _, name := range names {
		if name == "" || existing[name] {
			continue
		}
		r.config.ActivatedDeferredTools = append(r.config.ActivatedDeferredTools, name)
		existing[name] = true
		changed = true
	}
	if changed {
		r.rebuildRequestBuilder()
	}
}
