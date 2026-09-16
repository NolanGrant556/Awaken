package agentcore

import (
	"awaken/src/context"
	"awaken/src/hookruntime"
	"awaken/src/llm"
	"awaken/src/managedenv"
	"awaken/src/researchruntime"
	"awaken/src/skillruntime"
	"awaken/src/subagentruntime"
	"awaken/src/tool"
	stdcontext "context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const (
	maxTurnDiffChars                     = 50000
	maxOutputContinuationAttempts        = 1
	maxOutputContinuationPromptPrefix    = "请继续上一条回答，从中断处继续，不要重复已经输出的内容。"
	maxReasoningOnlyContinuationAttempts = 1
	reasoningOnlyContinuationPrompt      = "上一轮只返回了内部推理，没有提供可见答复或工具调用。请继续当前任务：未完成就调用下一步工具，已经完成就返回明确的可见结果。"
	// Stay within the 12 Ki-rune model content projection even for ASCII.
	// The UI preview budget is independent of full-result retrieval.
	toolResultOffloadThresholdBytes = 12 * 1024
	maxTransientLLMRetries          = 5
	initialConnectionRetryDelay     = 5 * time.Second
	maxConnectionRetryDelay         = 60 * time.Second
)

var (
	ErrPersistenceFailed      = errors.New("persistence_error")
	ErrContextEpochProjection = errors.New("project context epoch")
)

type Runner struct {
	progressNudge           *progressNudge
	pendingInbox            []string
	inboxErr                error
	incorporateMessages     func([]string)
	config                  AgentConfig
	conversation            *context.Conversation
	reqBuilder              *RequestBuilder
	toolRuntime             *ToolRuntime
	failureSupervisor       *FailureSupervisor
	threadID                string
	runID                   string
	sessionID               string
	recorder                SemanticRecorder
	contextGovernor         *contextGovernor
	streamLLMFunc           func(stdcontext.Context, *runEventEmitter) (*llm.Response, error)
	llmRetrySleep           func(stdcontext.Context, time.Duration) error
	suppressRunLifecycle    bool
	subagentController      subagentruntime.Controller
	subagentProgress        subagentruntime.ProgressReporter
	subagentMessages        func() []subagentruntime.QueuedMessage
	memoryRecallSnapshot    *MemoryRecallSnapshot
	configureEmitter        func(*runEventEmitter)
	runPlanService          tool.RunPlanService
	nativeCompactionInvoker func(stdcontext.Context, ModelCallAuthority, llm.Config, llm.Request) (*llm.Response, error)
	researchPrefetch        []researchruntime.PrefetchedSource
	researchReceipts        []subagentruntime.HostToolReceipt
	researchRevision        int
}

func NewRunner(config AgentConfig, conv *context.Conversation, reqBuilder *RequestBuilder) *Runner {
	if config.ChildSessions == nil {
		config.ChildSessions = NewChildSessions()
	}
	runner := &Runner{
		config:            config,
		conversation:      conv,
		reqBuilder:        reqBuilder,
		toolRuntime:       NewToolRuntimeWithConfig(nil, toolRuntimeConfigFromAgentConfig(config)),
		failureSupervisor: NewFailureSupervisor(config.Workspace.Root, config.MaxThoughts, config.MaxToolExecutions),
	}
	if config.SubagentOrigin != nil {
		runner.failureSupervisor.toolExecutions = config.AgentToolsUsed
	}
	runner.contextGovernor = newContextGovernor(
		NewContextEngine(contextWindowConfigFromModelConfig(config.ModelConfig)),
		func() (LLMRequest, error) { return runner.buildLLMRequest() },
		func(ctx stdcontext.Context, result CompactResult) error {
			if !contextCompactionV2Enabled() || usesLegacyContextCompactionRecorder(runner.recorder) {
				if err := runner.persistContextCompactedV1(ctx, result); err != nil {
					return err
				}
				runner.conversation.Context.SetMessages(stripProviderOpaqueCompactionState(result.Messages))
				runner.compactInvokedSkillContext()
				if runner.config.MemoryRecallReset != nil {
					runner.config.MemoryRecallReset()
				}
				runner.config.RestoredContextBundleV2 = nil
				runner.config.CompactContextBlocks = nil
				runner.config.CompactedMemoryContext = ""
				runner.memoryRecallSnapshot = nil
				return nil
			}
			// Build every post-compact projection before persistence. The V2
			// bundle is then appended and flushed as one durable boundary; only
			// after that succeeds may any in-memory authority be installed.
			result = runner.tryProviderNativeCompaction(ctx, result)
			skillBlocks := compactInvokedSkillContextBlocks(runner.config.SkillContextBlocks)
			bundle, err := runner.buildPostCompactContextBundleV2(result, skillBlocks)
			if err != nil && result.NativeCompactionState != nil {
				// Native opaque output is optional. If its exact prepared-body
				// projection cannot satisfy the same hard 10% gate (or the opaque
				// section is otherwise invalid), retry the draft with the local V2
				// checkpoint only. No boundary has been persisted yet.
				result.NativeCompactionState = nil
				result.NativeCompactionUsed = false
				result.NativeCompactionReason = "native_projection_fallback"
				bundle, err = runner.buildPostCompactContextBundleV2(result, skillBlocks)
			}
			if err != nil {
				return err
			}
			runner.contextGovernor.captureCompactDiagnostics(result, bundle)
			if err := runner.persistContextCompactedV2(ctx, bundle); err != nil {
				return err
			}
			runner.conversation.Context.SetMessages(stripProviderOpaqueCompactionState(result.Messages))
			runner.config.SkillContextBlocks = SkillContextBlocksFromBundleV2(&bundle)
			runner.config.RestoredContextBundleV2 = &bundle
			runner.config.CompactContextBlocks = CompactContextBlocksFromBundleV2(&bundle)
			runner.config.CompactedMemoryContext = CompactedMemoryContextFromBundleV2(&bundle)
			compactedMemoryIDs := CompactedMemoryIDsFromBundleV2(&bundle)
			runner.dropCompactedMemoryBodies(compactedMemoryIDs)
			if runner.config.MemoryRecallMarkCompacted != nil {
				runner.config.MemoryRecallMarkCompacted(compactedMemoryIDs)
			}
			runner.rebuildRequestBuilder()
			return nil
		},
	)
	runner.contextGovernor.projectCompactBudget = runner.projectPostCompactBudget
	runner.contextGovernor.RestoreUsageAnchor(config.RestoredContextWindowUsageAnchor, config.ModelConfig)
	runner.contextGovernor.preCompactHook = func(ctx stdcontext.Context, sink contextWindowEventSink, mode CompactMode, state ContextWindowState) (string, error) {
		events, _ := sink.(*runEventEmitter)
		result, err := runner.dispatchHookScoped(ctx, events, hookruntime.EventPreCompact, map[string]any{
			"mode": mode, "estimatedInputTokens": state.EstimatedTokens,
			"compactTarget": state.CompactTarget,
		}, "", false)
		if err != nil {
			return "", err
		}
		if result.Action == "block_compact" {
			return "", fmt.Errorf("%w: %s", ErrHookBlocked, firstNonEmpty(result.Reason, "PreCompact Hook blocked compaction"))
		}
		return strings.Join(result.AdditionalContext, "\n"), nil
	}
	runner.contextGovernor.postCompactHook = func(ctx stdcontext.Context, sink contextWindowEventSink, result CompactResult) error {
		events, _ := sink.(*runEventEmitter)
		_, err := runner.dispatchHookScoped(ctx, events, hookruntime.EventPostCompact, map[string]any{
			"mode": result.Mode, "compactBoundaryId": result.BoundaryID,
			"preEstimateTokens": result.PreEstimateTokens, "postEstimateTokens": result.PostEstimateTokens,
		}, "", false)
		return err
	}
	return runner
}

func (r *Runner) RunStreamWithEvents(ctx stdcontext.Context, sink EventSink) (answer string, runErr error) {

	r.progressNudge = newProgressNudge(r.config)
	events := newRunEventEmitter(r.threadID, sink)
	events.attemptID = r.config.AttemptID
	events.inboxMessageIDs = func() []string { return append([]string(nil), r.pendingInbox...) }
	events.contextRevision = func() uint64 { return r.config.ContextRevision + 1 }
	events.contextCommitted = func() {
		r.config.ContextRevision++
		if r.incorporateMessages != nil {
			r.incorporateMessages(r.pendingInbox)
		}
		r.pendingInbox = nil
	}
	defer r.syncRestoredContextWindowUsageAnchor()
	defer func() {
		if r.config.SubagentOrigin != nil && r.failureSupervisor != nil {
			r.config.AgentToolsUsed = r.failureSupervisor.toolExecutions
		}
	}()
	if r.configureEmitter != nil {
		r.configureEmitter(events)
	}
	events.suppressRunLifecycle = r.suppressRunLifecycle
	if r.config.SubagentOrigin != nil {
		events.agentID = r.config.SubagentOrigin.AgentID
		events.parentAgentID = r.config.SubagentOrigin.ParentAgentID
		events.subagentTaskID = r.config.SubagentTaskID
	}
	events.subagentProgress = r.subagentProgress
	defer func() {
		if err := events.persistenceFailure(); err != nil {
			answer = ""
			runErr = err
		}
	}()
	if r.runID != "" {
		events.runID = r.runID
	} else {
		r.runID = events.runID
	}
	if manager := managedenv.Default(); manager != nil {
		if origin := r.config.SubagentOrigin; origin != nil {
			if e := manager.Inherit(managedenv.Owner{ThreadID: r.threadID, AgentID: origin.ParentAgentID}, managedenv.Owner{ThreadID: r.threadID, AgentID: origin.AgentID, RunID: r.runID, TaskID: r.config.SubagentTaskID, Kind: "run"}); e != nil {
				slog.Warn("managed environment inheritance unavailable", "error", e)
			}
		}
		owner := managedenv.Owner{ThreadID: r.threadID, AgentID: "root", RunID: r.runID}
		if r.config.SubagentOrigin != nil {
			owner.AgentID = r.config.SubagentOrigin.AgentID
		}
		defer manager.ReleaseOwner(owner)
		// A model may use an exact previously returned interpreter path without
		// asking for the same environment again. Keep its healthy references alive
		// for this run; this does not change PATH or select another implementation.
		for _, binding := range manager.Bindings(owner) {
			if _, err := manager.Binding(ctx, binding.EnvironmentRef, owner); err != nil {
				slog.Warn("managed environment binding unavailable", "error", err)
			}
		}
		if bundle := r.config.RestoredContextBundleV2; bundle != nil {
			for _, ref := range bundle.EnvironmentBindings {
				if _, err := manager.RestoreBinding(ctx, ref.Reference, ref.InstanceID, owner); err != nil {
					slog.Warn("managed environment reference needs preparation", "error", err)
				}
			}
		}
	}
	events.recorder = r.recorder
	events.sessionID = r.sessionID
	r.subagentController = r.config.SubagentController
	if r.subagentController == nil && r.recorder != nil && r.config.SubagentOrigin == nil {
		managerConfig := r.childCreatorConfig(events.sink)
		managerConfig.Persist = r.persistSubagentEvent
		r.subagentController = subagentruntime.NewManager(ctx, managerConfig)
	}
	if r.subagentController != nil {
		if binder, ok := r.subagentController.(interface {
			Bind(stdcontext.Context, string, int, subagentruntime.Config) subagentruntime.Controller
		}); ok {
			creatorID, depth := "root", 0
			if r.config.SubagentOrigin != nil {
				creatorID, depth = r.config.SubagentOrigin.AgentID, r.config.SubagentOrigin.Depth
			}
			managerConfig := r.childCreatorConfig(events.sink)
			r.subagentController = binder.Bind(ctx, creatorID, depth, managerConfig)
		}
		if execution, ok := r.subagentController.(interface {
			EnterExecution(stdcontext.Context) (stdcontext.Context, func(), error)
		}); ok {
			var release func()
			var err error
			ctx, release, err = execution.EnterExecution(ctx)
			if err != nil {
				return "", err
			}
			defer release()
		}
		events.beforeTerminal = func(terminalCtx stdcontext.Context) error {
			// Cancellation is a request; terminal waits for actual child cleanup.
			barrierCtx := stdcontext.Background()
			if scoped, ok := r.subagentController.(interface {
				EndAttempt(stdcontext.Context, string) error
			}); ok {
				if err := scoped.EndAttempt(barrierCtx, r.runID); err != nil {
					return err
				}
				r.retireChildCreator(events.sink)
				return nil
			}
			return r.subagentController.Close(barrierCtx)
		}
		if r.suppressRunLifecycle {
			defer func() {
				if err := events.beforeTerminal(stdcontext.Background()); err != nil && runErr == nil {
					runErr = err
				}
			}()
		}
	}

	r.toolRuntime = NewToolRuntimeWithConfig(events.scopedSink(), toolRuntimeConfigFromAgentConfig(r.config))
	// The run identity is host-owned and becomes final only after the emitter
	// adopts a caller-supplied ID or generates one. Bind that exact identity to
	// the already configured runtime before any model turn can issue a tool call.
	// Child/fork runners enter through this same path with their inherited ID.
	r.toolRuntime.runID = r.runID
	r.toolRuntime.threadID = r.threadID
	{
		// Each Runner owns its plan; records carry the current Agent identity.
		planService := NewRunPlanService(RunPlanServiceConfig{
			Restored:  r.config.RestoredRunPlan,
			ThreadID:  r.threadID,
			RunID:     r.runID,
			SessionID: r.sessionID,
			Persist: func(persistCtx stdcontext.Context, record SemanticRecord) error {
				return r.persistRunPlanRecord(persistCtx, events, events.withAgentIdentity(record))
			},
		})
		r.runPlanService = planService
		r.toolRuntime.runPlanService = planService
		r.config.RunPlanContextProvider = planService.ContextBlock
		r.reqBuilder.UpdateRunPlanContextProvider(planService.ContextBlock)
		defer func() {
			if r.config.SubagentOrigin != nil {
				if snapshot, ok := planService.Snapshot(); ok {
					r.config.RestoredRunPlan = &snapshot
				}
			}
		}()
	}
	if !r.config.HookLifecycleStarted {
		if err := events.runStarted(ctx); err != nil {
			return "", err
		}
		if err := r.persistHookSnapshot(ctx); err != nil {
			return "", err
		}
	}
	events.afterTerminal = r.runEndHookCallback()
	if err := r.prepareResearchTeam(ctx, events); err != nil {
		if ctx.Err() != nil {
			events.runErrored(ctx, ctx.Err(), TerminalReasonCancelled)
			return "", ctx.Err()
		}
		events.runErrored(ctx, err)
		return "", err
	}
	turnState := newTurnState(events.threadID, events.runID, r.effectiveMaxThoughts(), r.config.ModelConfig.Model)
	transition := TransitionInitial
	maxOutputContinuations := 0
	reasoningOnlyContinuations := 0
	var continuedOutput strings.Builder
	maxThoughts := r.effectiveMaxThoughts()
	beforeFinishBlocks := 0

	for i := 0; i < maxThoughts+beforeFinishBlocks; i++ {
		if err := events.persistenceFailure(); err != nil {
			return "", err
		}
		select {
		case <-ctx.Done():
			r.injectSyntheticToolResultsForCancel()
			events.runErrored(ctx, ctx.Err(), TerminalReasonCancelled)
			return "", ctx.Err()
		default:
		}

		r.injectSubagentTerminalNotifications(nil)
		r.injectSubagentQueuedMessages()
		if r.inboxErr != nil {
			return "", r.inboxErr
		}

		if r.failureSupervisor != nil {
			r.failureSupervisor.BeginTurn()
		}
		r.progressNudge.prepare(transition)
		turnState = turnState.nextTurn(r.conversation.Context.GetMessages(), transition)
		events.turnStarted(turnState)

		streamLLM := r.streamLLM
		if r.config.SubagentOrigin != nil {
			if r.config.AgentTurnsUsed >= r.effectiveMaxThoughts() {
				err := maxThoughtsError(r.effectiveMaxThoughts())
				events.runErrored(ctx, err, TerminalReasonMaxTurns)
				return "", err
			}
			r.config.AgentTurnsUsed++
		}
		if r.streamLLMFunc != nil {
			streamLLM = r.streamLLMFunc
		}
		response, err := r.streamLLMWithTransientRetry(ctx, events, streamLLM)
		if err != nil {
			if ctx.Err() != nil {
				r.injectSyntheticToolResultsForCancel()
				events.turnFinished(ctx, TerminalReasonAbortedStreaming, "")
				events.runErrored(ctx, ctx.Err(), TerminalReasonAbortedStreaming)
				return "", ctx.Err()
			}
			if isPromptTooLongError(err) {
				if retried, compactErr := r.compactConversationForRetry(ctx, events); compactErr == nil && retried {
					events.turnFinished(ctx, "", TransitionReactiveCompactRetry)
					events.resetTurn()
					transition = TransitionReactiveCompactRetry
					continue
				}
				runErr := fmt.Errorf("LLM stream call failed: %w", err)
				events.turnFinished(ctx, TerminalReasonPromptTooLong, "")
				events.runErrored(ctx, runErr, TerminalReasonPromptTooLong)
				return "", runErr
			}
			runErr := wrapModelInvocationError(err)
			reason := terminalReasonForError(err)
			events.turnFinished(ctx, reason, "")
			events.runErrored(ctx, runErr, reason)
			return "", runErr
		}
		r.contextGovernor.RecordResponseUsage(lastAPIUsageTokens(response))
		events.finishReasoningIfOpen()
		events.finishTextIfOpen()
		if err := events.persistenceFailure(); err != nil {
			return "", err
		}
		r.progressNudge.visible(response.Content)
		if len(response.ToolCalls) > 0 {
			reasoningOnlyContinuations = 0
			toolCalls := llmToolCallsToContext(response.ToolCalls)
			results := r.executeToolCalls(ctx, toolCalls, events)
			r.progressNudge.observe(results, toolCalls...)
			r.conversation.ToolExchangeWithProviderState(response.Content, toolCalls, toolExecutionResultsToContext(results), response.ProviderOpaqueState)
			if stop, stopErr := r.stopAfterToolExchangeIfNeeded(ctx, events, i); stop {
				return "", stopErr
			}
			events.turnFinished(ctx, "", TransitionToolResultContinuation)
			events.resetTurn()
			transition = TransitionToolResultContinuation
			continue
		}

		// Inputs queued during the final provider call still belong to this
		// active assignment. Preserve the older answer before the new input and
		// cross another request boundary instead of leaving the input idle.
		if !response.MaxOutputReached {
			if messages := r.queuedSubagentMessages(); len(messages) > 0 {
				if response.Content != "" {
					r.conversation.AssistantOutputWithProviderState(response.Content, nil, response.ProviderOpaqueState)
				}
				r.appendSubagentQueuedMessages(messages)
				if r.inboxErr != nil {
					return "", r.inboxErr
				}
				events.turnFinished(ctx, "", TransitionSubagentNotification)
				events.resetTurn()
				transition = TransitionSubagentNotification
				continue
			}
		}

		// A child can finish while the provider is generating this turn. Do not
		// let a response built from the older running snapshot become the root's
		// final answer; retain it as context, append the durable terminal handoff,
		// and give the model one fresh continuation boundary.
		notifications := 0
		if !response.MaxOutputReached {
			notifications = r.injectSubagentTerminalNotifications(response)
		}
		if r.inboxErr != nil {
			return "", r.inboxErr
		}
		if notifications > 0 {
			events.turnFinished(ctx, "", TransitionSubagentNotification)
			events.resetTurn()
			transition = TransitionSubagentNotification
			continue
		}
		// Pure waiting is owned by the Host. An unchanged child set never
		// consumes another root model turn just to ask it to keep waiting.
		if !response.MaxOutputReached {
			waited, waitErr := r.waitForActiveSubagents(ctx, events, response)
			if waitErr != nil {
				events.runErrored(ctx, waitErr, terminalReasonForError(waitErr))
				return "", waitErr
			}
			if waited {
				events.turnFinished(ctx, "", TransitionSubagentNotification)
				events.resetTurn()
				transition = TransitionSubagentNotification
				continue
			}
		}

		if response.MaxOutputReached {
			if response.Content != "" {
				continuedOutput.WriteString(response.Content)
				r.conversation.AssistantOutputWithProviderState(response.Content, nil, response.ProviderOpaqueState)
			}
			if maxOutputContinuations < maxOutputContinuationAttempts {
				maxOutputContinuations++
				r.conversation.UserInput(maxOutputContinuationPrompt(response.StopReason))
				events.turnFinished(ctx, "", TransitionMaxOutputContinuation)
				events.resetTurn()
				transition = TransitionMaxOutputContinuation
				continue
			}
			err := maxOutputReachedError(maxOutputContinuations, response.StopReason)
			events.turnFinished(ctx, TerminalReasonMaxOutputReached, "")
			events.runErrored(ctx, err, TerminalReasonMaxOutputReached)
			return continuedOutput.String(), err
		}

		if handled, finalAnswer := r.handleXMLFallback(ctx, response.Content, events); handled {
			if finalAnswer != "" {
				blocked, hookErr := r.beforeFinish(ctx, events, finalAnswer, &beforeFinishBlocks)
				if hookErr != nil {
					return "", hookErr
				}
				if blocked {
					events.turnFinished(ctx, "", TransitionBeforeFinishHook)
					events.resetTurn()
					transition = TransitionBeforeFinishHook
					continue
				}
				finalAnswer = continuedOutput.String() + finalAnswer
				finalState, snapshotErr := r.persistCurrentContextSnapshot(ctx, events)
				if snapshotErr != nil {
					return "", snapshotErr
				}
				events.emitTurnDiff(ctx, r.config.Workspace.Root)
				events.turnFinished(ctx, TerminalReasonCompleted, "")
				events.runFinished(ctx, finalAnswer, finalState, TerminalReasonCompleted)
				return finalAnswer, nil
			}
			if stop, stopErr := r.stopAfterToolExchangeIfNeeded(ctx, events, i); stop {
				return "", stopErr
			}
			events.turnFinished(ctx, "", TransitionXMLFallbackContinuation)
			events.resetTurn()
			transition = TransitionXMLFallbackContinuation
			continue
		}

		if !responseHasTerminalSignal(response) {
			if strings.TrimSpace(response.Thinking) != "" && reasoningOnlyContinuations < maxReasoningOnlyContinuationAttempts {
				reasoningOnlyContinuations++
				// Preserve provider-native reasoning replay without presenting it as
				// a user-visible answer. The synthetic host instruction gives the
				// same run one bounded chance to produce a tool call or final text.
				r.conversation.AssistantOutputWithProviderState("", nil, response.ProviderOpaqueState)
				r.conversation.SyntheticUserInput(reasoningOnlyContinuationPrompt, fmt.Sprintf("reasoning_only_%s_%d", r.runID, reasoningOnlyContinuations))
				events.turnFinished(ctx, "", TransitionReasoningOnlyContinuation)
				events.resetTurn()
				transition = TransitionReasoningOnlyContinuation
				continue
			}
			events.turnFinished(ctx, TerminalReasonEmptyResponse, "")
			events.runErrored(ctx, llm.ErrEmptyResponse, TerminalReasonEmptyResponse)
			return "", llm.ErrEmptyResponse
		}
		finalContent := continuedOutput.String() + response.Content
		blocked, hookErr := r.beforeFinish(ctx, events, response.Content, &beforeFinishBlocks)
		if hookErr != nil {
			return "", hookErr
		}
		r.conversation.AssistantOutputWithProviderState(response.Content, nil, response.ProviderOpaqueState)
		if blocked {
			events.turnFinished(ctx, "", TransitionBeforeFinishHook)
			events.resetTurn()
			transition = TransitionBeforeFinishHook
			continue
		}
		events.finishTextIfOpen()
		finalState, snapshotErr := r.persistCurrentContextSnapshot(ctx, events)
		if snapshotErr != nil {
			return "", snapshotErr
		}
		events.emitTurnDiff(ctx, r.config.Workspace.Root)
		events.turnFinished(ctx, TerminalReasonCompleted, "")
		events.runFinished(ctx, finalContent, finalState, TerminalReasonCompleted)
		return finalContent, nil
	}

	err := maxThoughtsError(maxThoughts)
	events.runErrored(ctx, err, TerminalReasonMaxTurns)
	return "", err
}

func (r *Runner) persistHookSnapshot(ctx stdcontext.Context) error {
	if r == nil || r.recorder == nil || r.config.HookAvailabilitySnapshot == nil {
		return nil
	}
	snapshot := r.config.HookAvailabilitySnapshot
	sources := make([]map[string]any, 0, len(snapshot.Sources))
	for _, source := range snapshot.Sources {
		sources = append(sources, map[string]any{
			"kind": source.Kind, "status": source.Status, "digest": source.Digest,
			"hook_count": source.HookCount,
		})
	}
	enabled := 0
	for _, hooks := range snapshot.HooksByEvent {
		enabled += len(hooks)
	}
	record := NewSemanticRecord(SemanticKindHookSnapshot, r.threadID, r.runID, "", "", r.sessionID, map[string]any{
		"snapshot_id": snapshot.SnapshotID, "schema_version": snapshot.SchemaVersion,
		"enabled_hook_count": enabled, "diagnostic_count": len(snapshot.Diagnostics), "sources": sources,
	})
	if origin := r.config.SubagentOrigin; origin != nil {
		record.Payload["agent_id"], record.Payload["parent_agent_id"] = origin.AgentID, origin.ParentAgentID
		record.Payload["subagent_task_id"], record.Payload["attempt_id"] = r.config.SubagentTaskID, r.config.AttemptID
	}
	persistCtx, cancel := durablePersistenceContext(ctx)
	defer cancel()
	if err := r.recorder.Append(persistCtx, record); err != nil {
		return fmt.Errorf("persist hook snapshot: %w", err)
	}
	if err := r.recorder.Flush(persistCtx); err != nil {
		return fmt.Errorf("flush hook snapshot: %w", err)
	}
	return nil
}

type subagentTerminalNotification struct {
	Type string                       `json:"type"`
	Task subagentruntime.TaskSnapshot `json:"task"`
}

func minPositive(value, limit int) int {
	if value <= 0 || value > limit {
		return limit
	}
	return value
}

func appendUniqueStrings(values []string, additions ...string) []string {
	seen := make(map[string]bool, len(values)+len(additions))
	result := make([]string, 0, len(values)+len(additions))
	for _, value := range append(append([]string(nil), values...), additions...) {
		if value = strings.TrimSpace(value); value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func isSkillInvocationToolName(name string) bool {
	name = strings.TrimSpace(name)
	return name == skillruntime.CatalogToolName
}

func (r *Runner) stopAfterToolExchangeIfNeeded(ctx stdcontext.Context, events *runEventEmitter, turnIndex int) (bool, error) {
	if events != nil {
		if err := events.persistenceFailure(); err != nil {
			return true, err
		}
	}
	if err := context.ValidateToolResultPairing(r.conversation.Context.GetMessages()); err != nil {
		runErr := fmt.Errorf("invalid tool result pairing after tool exchange: %w", err)
		if events != nil {
			events.turnFinished(ctx, TerminalReasonToolPairingError, "")
			events.runErrored(ctx, runErr, TerminalReasonToolPairingError)
		}
		return true, runErr
	}
	if events != nil && events.pendingTerminalReason != "" {
		reason := events.pendingTerminalReason
		events.pendingTerminalReason = ""
		runErr := fmt.Errorf("tool supervision stopped run: %s", reason)
		events.turnFinished(ctx, reason, "")
		events.runErrored(ctx, runErr, reason)
		return true, runErr
	}
	if ctx.Err() != nil {
		if events != nil {
			events.turnFinished(ctx, TerminalReasonAbortedTools, "")
			events.runErrored(ctx, ctx.Err(), TerminalReasonAbortedTools)
		}
		return true, ctx.Err()
	}
	maxThoughts := r.effectiveMaxThoughts()
	if turnIndex+1 >= maxThoughts {
		err := maxThoughtsError(maxThoughts)
		if events != nil {
			events.turnFinished(ctx, TerminalReasonMaxTurns, "")
			events.runErrored(ctx, err, TerminalReasonMaxTurns)
		}
		return true, err
	}
	return false, nil
}

func firstTerminalReason(reasons []TerminalReason, fallback TerminalReason) TerminalReason {
	for _, reason := range reasons {
		if reason != "" {
			return reason
		}
	}
	return fallback
}

func terminalReasonForError(err error) TerminalReason {
	var disconnected *llm.StreamDisconnectedError
	if errors.As(err, &disconnected) {
		return TerminalReasonModelStreamDisconnected
	}
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrContextEpochProjection):
		return TerminalReasonContextEpochError
	case errors.Is(err, errContextWindowBlocked):
		return TerminalReasonContextBlocked
	case errors.Is(err, stdcontext.Canceled):
		return TerminalReasonCancelled
	case errors.Is(err, llm.ErrEmptyResponse):
		return TerminalReasonEmptyResponse
	case strings.Contains(err.Error(), "invalid tool result pairing"):
		return TerminalReasonToolPairingError
	default:
		return TerminalReasonModelError
	}
}

func wrapModelInvocationError(err error) error {
	if errors.Is(err, ErrContextEpochProjection) {
		return fmt.Errorf("context preparation failed: %w", err)
	}
	return fmt.Errorf("LLM stream call failed: %w", err)
}

func maxOutputContinuationPrompt(stopReason string) string {
	stopReason = strings.TrimSpace(stopReason)
	if stopReason == "" {
		return maxOutputContinuationPromptPrefix
	}
	return fmt.Sprintf("%s 上一轮 stop_reason=%s。", maxOutputContinuationPromptPrefix, stopReason)
}

func maxOutputReachedError(attempts int, stopReason string) error {
	stopReason = strings.TrimSpace(stopReason)
	if stopReason == "" {
		return fmt.Errorf("model reached max output limit after %d continuation attempt(s)", attempts)
	}
	return fmt.Errorf("model reached max output limit after %d continuation attempt(s): stop_reason=%s", attempts, stopReason)
}

func responseHasTerminalSignal(response *llm.Response) bool {
	if response == nil {
		return false
	}
	return strings.TrimSpace(response.Content) != "" || len(response.ToolCalls) > 0
}

func lastAPIUsageTokens(response *llm.Response) int {
	if response == nil || response.Usage == nil {
		return 0
	}
	// Within one run, the next provider request re-admits the previous
	// assistant output. Prefer total usage as a conservative approximation of
	// that continuation input; compatible providers that omit total usage fall
	// back to their reported input usage.
	if response.Usage.TotalTokens != nil && *response.Usage.TotalTokens > 0 {
		return *response.Usage.TotalTokens
	}
	if response.Usage.InputTokens != nil && *response.Usage.InputTokens > 0 {
		return *response.Usage.InputTokens
	}
	return 0
}

func maxThoughtsError(maxThoughts int) error {
	return fmt.Errorf("达到最大思考次数 %d，未能完成任务", maxThoughts)
}

func (r *Runner) effectiveMaxThoughts() int {
	if r == nil {
		return DefaultMaxAgentTurns
	}
	if r.failureSupervisor != nil {
		return r.failureSupervisor.MaxAgentTurns()
	}
	return EffectiveMaxAgentTurns(r.config.MaxThoughts)
}

func syntheticCancelledToolResult(call context.ToolCall) ToolExecutionResult {
	return ToolExecutionResult{
		ToolCallID: call.ID,
		Name:       call.Function.Name,
		Content:    "工具执行已被取消",
		IsError:    true,
		Status:     ToolExecutionCancelled,
		ErrorCode:  "tool_cancelled",
	}
}

func syntheticPersistenceFailureResult(call context.ToolCall, err error) ToolExecutionResult {
	now := time.Now().UTC()
	message := "persistence_error"
	if err != nil {
		message = err.Error()
	}
	outcome := tool.ExecutionOutcome{
		Content:         message,
		Status:          tool.OutcomeFailed,
		TransportStatus: tool.TransportNotStarted,
		SemanticStatus:  tool.SemanticFailure,
		ProcessStatus:   tool.ProcessNotApplicable,
		ErrorCode:       string(TerminalReasonPersistenceError),
		ErrorCategory:   tool.ErrorCategoryPersistence,
		ErrorMessage:    message,
		Retryable:       false,
		RecoveryOptions: []string{"stop_new_side_effects", "wait_for_journal_recovery_or_terminate"},
		StartedAt:       now,
		FinishedAt:      now,
	}
	return ToolExecutionResult{
		ToolCallID: call.ID,
		Name:       call.Function.Name,
		Content:    message,
		IsError:    true,
		Status:     ToolExecutionFailed,
		ErrorCode:  outcome.ErrorCode,
		Outcome:    outcome,
		StartedAt:  now,
		FinishedAt: now,
	}
}

func toolExecutionResultsToContext(results []ToolExecutionResult) []context.ToolResult {
	out := make([]context.ToolResult, 0, len(results))
	for _, result := range results {
		content := result.Content
		isError := result.IsError
		if result.Outcome.Status != "" {
			content = result.Outcome.ModelPayload()
			isError = result.Outcome.IsError()
		}
		out = append(out, context.ToolResult{
			ToolCallID:   result.ToolCallID,
			Name:         result.Name,
			Content:      content,
			IsError:      isError,
			ModelContent: tool.CloneModelContent(result.Outcome.ModelContent),
		})
	}
	return out
}

func llmToolCallsToContext(toolCalls []llm.ToolCall) []context.ToolCall {
	result := make([]context.ToolCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		id := tc.ID
		if id == "" {
			id = context.GenerateToolCallID()
		}
		result = append(result, context.ToolCall{
			ID:   id,
			Type: "function",
			Function: context.FunctionCall{
				Name:      tc.Name,
				Arguments: tc.Input,
			},
		})
	}
	return result
}
