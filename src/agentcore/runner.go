package agentcore

import (
	"bytes"
	stdcontext "context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"awaken/src/context"
	"awaken/src/internal/outputlimit"
	"awaken/src/llm"
)

const (
	maxTurnDiffChars                  = 50000
	maxOutputContinuationAttempts     = 1
	maxOutputContinuationPromptPrefix = "请继续上一条回答，从中断处继续，不要重复已经输出的内容。"
)

type Runner struct {
	config          AgentConfig
	conversation    *context.Conversation
	reqBuilder      *RequestBuilder
	toolRuntime     *ToolRuntime
	threadID        string
	contextGovernor *contextGovernor
	streamLLMFunc   func(stdcontext.Context, *runEventEmitter) (*llm.Response, error)
}

func NewRunner(config AgentConfig, conv *context.Conversation, reqBuilder *RequestBuilder) *Runner {
	runner := &Runner{
		config:       config,
		conversation: conv,
		reqBuilder:   reqBuilder,
		toolRuntime:  NewToolRuntimeWithConfig(nil, toolRuntimeConfigFromAgentConfig(config)),
	}
	runner.contextGovernor = newContextGovernor(
		NewContextEngine(contextWindowConfigFromModelConfig(config.ModelConfig)),
		runner.buildLLMRequest,
		func(result CompactResult) {
			runner.conversation.Context.SetMessages(result.Messages)
		},
	)
	return runner
}

func (r *Runner) RunStreamWithEvents(ctx stdcontext.Context, sink EventSink) (string, error) {
	events := newRunEventEmitter(r.threadID, sink)
	r.toolRuntime = NewToolRuntimeWithConfig(sink, toolRuntimeConfigFromAgentConfig(r.config))
	events.runStarted()
	turnState := newTurnState(events.threadID, events.runID, r.config.MaxThoughts, r.config.ModelConfig.Model)
	transition := TransitionInitial
	maxOutputContinuations := 0
	var continuedOutput strings.Builder

	for i := 0; !hasMaxThoughtsLimit(r.config.MaxThoughts) || i < r.config.MaxThoughts; i++ {
		select {
		case <-ctx.Done():
			r.injectSyntheticToolResultsForCancel()
			events.runErrored(ctx.Err(), TerminalReasonCancelled)
			return "", ctx.Err()
		default:
		}

		turnState = turnState.nextTurn(r.conversation.Context.GetMessages(), transition)
		events.turnStarted(turnState)

		streamLLM := r.streamLLM
		if r.streamLLMFunc != nil {
			streamLLM = r.streamLLMFunc
		}
		response, err := streamLLM(ctx, events)
		if err != nil {
			if ctx.Err() != nil {
				r.injectSyntheticToolResultsForCancel()
				events.turnFinished(TerminalReasonAbortedStreaming, "")
				events.runErrored(ctx.Err(), TerminalReasonAbortedStreaming)
				return "", ctx.Err()
			}
			if isPromptTooLongError(err) {
				if retried, compactErr := r.compactConversationForRetry(ctx, events); compactErr == nil && retried {
					events.turnFinished("", TransitionReactiveCompactRetry)
					events.resetTurn()
					transition = TransitionReactiveCompactRetry
					continue
				}
				runErr := fmt.Errorf("LLM stream call failed: %w", err)
				events.turnFinished(TerminalReasonPromptTooLong, "")
				events.runErrored(runErr, TerminalReasonPromptTooLong)
				return "", runErr
			}
			runErr := fmt.Errorf("LLM stream call failed: %w", err)
			reason := terminalReasonForError(err)
			events.turnFinished(reason, "")
			events.runErrored(runErr, reason)
			return "", runErr
		}
		r.contextGovernor.RecordResponseUsage(lastAPIUsageTokens(response))

		events.finishReasoningIfOpen()
		events.finishTextIfOpen()

		if len(response.ToolCalls) > 0 {
			toolCalls := llmToolCallsToContext(response.ToolCalls)
			results := r.executeToolCalls(ctx, toolCalls, events)
			r.conversation.ToolExchange(response.Content, toolCalls, toolExecutionResultsToContext(results))
			if stop, stopErr := r.stopAfterToolExchangeIfNeeded(ctx, events, i); stop {
				return "", stopErr
			}
			events.turnFinished("", TransitionToolResultContinuation)
			events.resetTurn()
			transition = TransitionToolResultContinuation
			continue
		}

		if response.MaxOutputReached {
			if response.Content != "" {
				continuedOutput.WriteString(response.Content)
				r.conversation.AssistantOutput(response.Content, nil)
			}
			if maxOutputContinuations < maxOutputContinuationAttempts {
				maxOutputContinuations++
				r.conversation.UserInput(maxOutputContinuationPrompt(response.StopReason))
				events.turnFinished("", TransitionMaxOutputContinuation)
				events.resetTurn()
				transition = TransitionMaxOutputContinuation
				continue
			}
			err := maxOutputReachedError(maxOutputContinuations, response.StopReason)
			events.turnFinished(TerminalReasonMaxOutputReached, "")
			events.runErrored(err, TerminalReasonMaxOutputReached)
			return continuedOutput.String(), err
		}

		if handled, finalAnswer := r.handleXMLFallback(ctx, response.Content, events); handled {
			if finalAnswer != "" {
				finalAnswer = continuedOutput.String() + finalAnswer
				finalState := r.latestContextWindowState()
				events.emitTurnDiff(ctx, r.config.Workspace.Root)
				events.turnFinished(TerminalReasonCompleted, "")
				events.runFinished(finalAnswer, finalState, TerminalReasonCompleted)
				return finalAnswer, nil
			}
			if stop, stopErr := r.stopAfterToolExchangeIfNeeded(ctx, events, i); stop {
				return "", stopErr
			}
			events.turnFinished("", TransitionXMLFallbackContinuation)
			events.resetTurn()
			transition = TransitionXMLFallbackContinuation
			continue
		}

		if !responseHasSignal(response) {
			events.turnFinished(TerminalReasonEmptyResponse, "")
			events.runErrored(llm.ErrEmptyResponse, TerminalReasonEmptyResponse)
			return "", llm.ErrEmptyResponse
		}
		finalContent := continuedOutput.String() + response.Content
		r.conversation.AssistantOutput(response.Content, nil)
		finalState := r.latestContextWindowState()
		events.finishTextIfOpen()
		events.emitTurnDiff(ctx, r.config.Workspace.Root)
		events.turnFinished(TerminalReasonCompleted, "")
		events.runFinished(finalContent, finalState, TerminalReasonCompleted)
		return finalContent, nil
	}

	err := maxThoughtsError(r.config.MaxThoughts)
	events.runErrored(err, TerminalReasonMaxTurns)
	return "", err
}

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
	return ToolRuntimeConfig{
		PermissionContext: permissionContextFromTaskScope(config.Workspace),
		AutoToolPolicy:    config.AutoToolPolicy,
		Profile:           config.ExecutionProfile,
		SubagentBoundary:  config.SubagentBoundary,
		SubagentOrigin:    config.SubagentOrigin,
	}
}

func (r *Runner) buildLLMRequest() (LLMRequest, error) {
	llmReq := r.reqBuilder.Build(r.conversation.Context, nil)
	if err := context.ValidateToolResultPairing(llmReq.Messages); err != nil {
		return LLMRequest{}, fmt.Errorf("invalid tool result pairing: %w", err)
	}
	return llmReq, nil
}

func (r *Runner) latestContextWindowState() *ContextWindowState {
	llmReq, err := r.buildLLMRequest()
	if err != nil {
		return nil
	}
	evaluated := r.contextGovernor.evaluateRequest(llmReq)
	state := evaluated.ContextWindowState
	return &state
}

func (r *Runner) streamLLM(ctx stdcontext.Context, events *runEventEmitter) (*llm.Response, error) {
	llmReq, err := r.prepareLLMRequest(ctx, events)
	if err != nil {
		return nil, err
	}
	observer := llm.StreamObserver{}
	if events != nil {
		observer.OnContent = llm.StreamCallback(events.onLLMDelta)
		observer.OnToolCallDelta = events.onToolCallArgsDelta
	}
	return llm.StreamInvokeWithObserver(ctx, llm.Config{
		APIKey:      r.config.ModelConfig.APIKey,
		BaseURL:     r.config.ModelConfig.BaseURL,
		Model:       llmReq.Model,
		Protocol:    llm.Protocol(r.config.ModelConfig.Protocol),
		Temperature: llmReq.Temperature,
		MaxTokens:   llmReq.MaxTokens,
		AuthScheme:  r.config.ModelConfig.AuthScheme,
	}, llm.Request{
		SystemPrompt: llmReq.SystemPrompt,
		Messages:     llmReq.Messages,
		Tools:        llmReq.Tools,
	}, observer)
}

func (r *Runner) prepareLLMRequest(ctx stdcontext.Context, events *runEventEmitter) (LLMRequest, error) {
	llmReq, err := r.buildLLMRequest()
	if err != nil {
		return LLMRequest{}, err
	}
	if events != nil {
		return r.contextGovernor.PrepareRequest(ctx, llmReq, events)
	}
	return r.contextGovernor.PrepareRequest(ctx, llmReq, nil)
}

func (r *Runner) compactConversationForRetry(ctx stdcontext.Context, events *runEventEmitter) (bool, error) {
	return r.contextGovernor.TryReactiveCompact(ctx, events)
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
	for _, call := range toolCalls {
		if ctx.Err() != nil {
			results = append(results, syntheticCancelledToolResult(call))
			continue
		}
		if events != nil {
			events.emitToolCall(call)
		}
		result := r.toolRuntime.Execute(ctx, call)
		results = append(results, result)
		if events != nil && call.Function.Name == "apply_patch" && !result.IsError {
			events.markMutation()
		}
		if events != nil {
			events.emitToolResult(result)
		}
	}
	return results
}

func (r *Runner) stopAfterToolExchangeIfNeeded(ctx stdcontext.Context, events *runEventEmitter, turnIndex int) (bool, error) {
	if err := context.ValidateToolResultPairing(r.conversation.Context.GetMessages()); err != nil {
		runErr := fmt.Errorf("invalid tool result pairing after tool exchange: %w", err)
		if events != nil {
			events.turnFinished(TerminalReasonToolPairingError, "")
			events.runErrored(runErr, TerminalReasonToolPairingError)
		}
		return true, runErr
	}
	if ctx.Err() != nil {
		if events != nil {
			events.turnFinished(TerminalReasonAbortedTools, "")
			events.runErrored(ctx.Err(), TerminalReasonAbortedTools)
		}
		return true, ctx.Err()
	}
	if hasMaxThoughtsLimit(r.config.MaxThoughts) && turnIndex+1 >= r.config.MaxThoughts {
		err := maxThoughtsError(r.config.MaxThoughts)
		if events != nil {
			events.turnFinished(TerminalReasonMaxTurns, "")
			events.runErrored(err, TerminalReasonMaxTurns)
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
	switch {
	case err == nil:
		return ""
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

func responseHasSignal(response *llm.Response) bool {
	if response == nil {
		return false
	}
	return strings.TrimSpace(response.Content) != "" || strings.TrimSpace(response.Thinking) != "" || len(response.ToolCalls) > 0
}

func lastAPIUsageTokens(response *llm.Response) int {
	if response == nil || response.Usage == nil {
		return 0
	}
	if response.Usage.TotalTokens > 0 {
		return response.Usage.TotalTokens
	}
	if response.Usage.InputTokens > 0 {
		return response.Usage.InputTokens
	}
	return 0
}

func maxThoughtsError(maxThoughts int) error {
	return fmt.Errorf("达到最大思考次数 %d，未能完成任务", maxThoughts)
}

func hasMaxThoughtsLimit(maxThoughts int) bool {
	return maxThoughts > 0
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
			ToolCallID: result.ToolCallID,
			Name:       result.Name,
			Content:    content,
			IsError:    isError,
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

type runEventEmitter struct {
	sink                EventSink
	threadID            string
	runID               string
	turnState           TurnState
	currentAssistantID  string
	textMessageID       string
	reasoningMessageID  string
	textOpen            bool
	reasoningOpen       bool
	textStartedThisTurn bool
	mutationOccurred    bool
}

func newRunEventEmitter(threadID string, sink EventSink) *runEventEmitter {
	if strings.TrimSpace(threadID) == "" {
		threadID = newAwakenID("thread")
	}
	return &runEventEmitter{
		sink:     normalizeEventSink(sink),
		threadID: threadID,
		runID:    newAwakenID("run"),
	}
}

func (e *runEventEmitter) emit(event AwakenEvent) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.ThreadID == "" {
		event.ThreadID = e.threadID
	}
	if event.RunID == "" {
		event.RunID = e.runID
	}
	if e.turnState.TurnID != "" {
		if event.TurnID == "" {
			event.TurnID = string(e.turnState.TurnID)
		}
		if event.QueryChainID == "" {
			event.QueryChainID = string(e.turnState.QueryChainID)
		}
		if event.Depth == 0 {
			event.Depth = int(e.turnState.Depth)
		}
		if event.TurnCount == 0 {
			event.TurnCount = e.turnState.TurnCount
		}
		if event.TransitionReason == "" {
			event.TransitionReason = e.turnState.TransitionReason
		}
	}
	e.sink.Emit(event)
}

func (e *runEventEmitter) runStarted() {
	e.emit(AwakenEvent{Type: RunStartedEventType})
}

func (e *runEventEmitter) turnStarted(state TurnState) {
	e.turnState = state
	e.emit(AwakenEvent{
		Type:             TurnStartedEventType,
		TransitionReason: state.TransitionReason,
	})
}

func (e *runEventEmitter) turnFinished(reason TerminalReason, transition TransitionReason) {
	if transition == "" {
		transition = e.turnState.TransitionReason
	}
	e.turnState.TerminalReason = reason
	e.turnState.TransitionReason = transition
	e.emit(AwakenEvent{
		Type:             TurnFinishedEventType,
		TerminalReason:   reason,
		TransitionReason: transition,
	})
}

func (e *runEventEmitter) runFinished(content string, state *ContextWindowState, reasons ...TerminalReason) {
	reason := firstTerminalReason(reasons, TerminalReasonCompleted)
	event := AwakenEvent{
		Type:           RunFinishedEventType,
		Content:        content,
		TerminalReason: reason,
	}
	if state != nil {
		applyContextWindowEstimate(&event, *state)
		event.ContextActualTokens = state.LastAPIUsageTokens
	}
	e.emit(event)
}

func (e *runEventEmitter) runErrored(err error, reasons ...TerminalReason) {
	if err == nil {
		return
	}
	event := AwakenEvent{
		Type:           RunErrorEventType,
		Error:          err.Error(),
		TerminalReason: firstTerminalReason(reasons, terminalReasonForError(err)),
	}
	if event.TerminalReason == TerminalReasonCancelled || errors.Is(err, stdcontext.Canceled) {
		event.Code = "RUN_CANCELLED"
	}
	switch {
	case errors.Is(err, errContextWindowBlocked):
		event.Code = "CONTEXT_BLOCKED"
	case errors.Is(err, llm.ErrMissingAPIKey):
		event.Code = "PROVIDER_AUTH_MISSING"
	case errors.Is(err, llm.ErrRequestFailed):
		event.Code = "PROVIDER_REQUEST_FAILED"
	}
	e.emit(event)
}

func (e *runEventEmitter) onLLMDelta(content string, isThinking bool) {
	if content == "" {
		return
	}
	if isThinking {
		e.startReasoningIfNeeded()
		e.emit(AwakenEvent{
			Type:      ReasoningDeltaEventType,
			MessageID: e.reasoningMessageID,
			Delta:     content,
		})
		return
	}
	e.startTextIfNeeded()
	e.emit(AwakenEvent{
		Type:      TextDeltaEventType,
		MessageID: e.textMessageID,
		Role:      "assistant",
		Delta:     content,
	})
}

func (e *runEventEmitter) onToolCallArgsDelta(delta llm.ToolCallDelta) {
	if delta.ArgumentsDelta == "" {
		return
	}
	e.ensureAssistantMessageID()
	toolCallID := delta.ID
	if toolCallID == "" {
		toolCallID = fmt.Sprintf("tool_index_%d", delta.Index)
	}
	e.emit(AwakenEvent{
		Type:       ToolCallArgsDeltaEventType,
		ToolCallID: toolCallID,
		ToolName:   delta.Name,
		Delta:      delta.ArgumentsDelta,
	})
}

func (e *runEventEmitter) emitStandaloneText(content string) {
	if content == "" {
		return
	}
	e.startTextIfNeeded()
	e.emit(AwakenEvent{
		Type:      TextDeltaEventType,
		MessageID: e.textMessageID,
		Role:      "assistant",
		Delta:     content,
	})
	e.finishTextIfOpen()
}

func (e *runEventEmitter) emitToolCall(call context.ToolCall) {
	parentMessageID := e.ensureAssistantMessageID()
	e.emit(AwakenEvent{
		Type:            ToolCallStartedEventType,
		MessageID:       parentMessageID,
		ParentMessageID: parentMessageID,
		ToolCallID:      call.ID,
		ToolName:        call.Function.Name,
	})
	if call.Function.Arguments != "" {
		e.emit(AwakenEvent{
			Type:       ToolCallArgsEventType,
			ToolCallID: call.ID,
			ToolName:   call.Function.Name,
			Arguments:  call.Function.Arguments,
		})
	}
	e.emit(AwakenEvent{
		Type:       ToolCallFinishedEventType,
		ToolCallID: call.ID,
		ToolName:   call.Function.Name,
	})
}

func (e *runEventEmitter) emitToolResult(result ToolExecutionResult) {
	event := AwakenEvent{
		Type:         ToolResultEventType,
		MessageID:    newAwakenID("toolmsg"),
		ToolCallID:   result.ToolCallID,
		ToolName:     result.Name,
		Content:      result.Content,
		Role:         "tool",
		TrashedPaths: result.TrashedPaths,
	}
	applyToolOutcomeFields(&event, result)
	e.emit(event)
}

// applyToolOutcomeFields copies canonical ToolOutcome fields onto an event.
// Prefers result.Outcome when present; otherwise maps legacy Status/IsError.
func applyToolOutcomeFields(event *AwakenEvent, result ToolExecutionResult) {
	if result.Outcome.Status != "" {
		event.ToolStatus = string(result.Outcome.Status)
		event.TransportStatus = string(result.Outcome.TransportStatus)
		event.SemanticStatus = string(result.Outcome.SemanticStatus)
		event.ProcessStatus = string(result.Outcome.ProcessStatus)
		event.ProcessExitCode = result.Outcome.ExitCode
		event.ProcessTimedOut = result.Outcome.TimedOut
		event.ErrorCode = result.Outcome.ErrorCode
		event.ErrorCategory = string(result.Outcome.ErrorCategory)
		event.Retryable = result.Outcome.Retryable
		event.RecoveryOptions = append([]string(nil), result.Outcome.RecoveryOptions...)
		event.DurationMs = result.Outcome.DurationMs
		if event.DurationMs <= 0 {
			event.DurationMs = result.DurationMs
		}
		if event.ErrorCode == "" {
			event.ErrorCode = result.ErrorCode
		}
		return
	}

	status := string(result.Status)
	if status == "" {
		if result.IsError {
			status = string(ToolExecutionFailed)
		} else {
			status = string(ToolExecutionSucceeded)
		}
	}
	event.ToolStatus = status
	event.ErrorCode = result.ErrorCode
	event.DurationMs = result.DurationMs
	event.Retryable = false
}

func (e *runEventEmitter) markMutation() {
	e.mutationOccurred = true
}

func (e *runEventEmitter) emitContextWindow(state ContextWindowState) {
	e.emit(contextWindowEvent(ContextEstimatedEventType, state, state.Summary()))
	if state.ShouldWarn() {
		e.emit(contextWindowEvent(ContextWarningEventType, state, "context window warning: "+state.Summary()))
	}
}

func (e *runEventEmitter) emitContextBlocked(state ContextWindowState, message string) {
	e.emit(contextWindowEvent(ContextBlockedEventType, state, message))
}

func (e *runEventEmitter) emitCompactStarted(state ContextWindowState, mode CompactMode) {
	event := contextWindowEvent(CompactStartedEventType, state, "compact started: "+string(mode))
	event.CompactMode = mode
	event.CompactPreEstimate = intPointer(state.EstimatedTokens)
	e.emit(event)
}

func (e *runEventEmitter) emitCompactFinished(state ContextWindowState, result CompactResult, summaryModel string) {
	mode := result.Mode
	if mode == "" {
		mode = CompactModeAuto
	}
	event := contextWindowEvent(CompactFinishedEventType, state, fmt.Sprintf(
		"compact finished: %s boundary=%s estimated_tokens=%d",
		mode,
		result.BoundaryID,
		result.EstimatedTokens,
	))
	postEstimate := result.PostEstimateTokens
	if postEstimate <= 0 {
		postEstimate = result.EstimatedTokens
	}
	toolResultsPruned := result.ToolResultsPruned > 0
	event.ContextEstimatedTokens = postEstimate
	event.ContextCompactTarget = state.CompactTarget
	event.CompactMode = mode
	event.CompactBoundaryID = result.BoundaryID
	event.CompactPreEstimate = intPointer(state.EstimatedTokens)
	event.CompactPostEstimate = intPointer(postEstimate)
	event.CompactSummaryModel = summaryModel
	event.CompactFallbackUsed = boolPointer(result.Fallback.Used)
	event.CompactToolResultsPruned = boolPointer(toolResultsPruned)
	event.CompactToolResultsPrunedCount = firstPositiveInt(result.ToolResultsPrunedCount, result.ToolResultsPruned)
	event.CompactPrunedSafeGroupCount = result.PrunedSafeGroupCount
	event.CompactPrunedMessageCount = result.PrunedMessageCount
	event.CompactSummaryMaxTokens = result.SummaryMaxTokens
	event.CompactSummaryRetryCount = result.SummaryRetryCount
	event.CompactSummaryRawChars = result.SummaryRawChars
	event.CompactSummaryParsedSections = result.SummaryParsedSections
	event.CompactSummaryMeaningfulSections = result.SummaryMeaningfulSections
	event.CompactSummaryQualityIssue = result.SummaryQualityIssue
	if result.SummarySourceHadUser || result.SummaryLatestUserPinned {
		event.CompactSummarySourceHadUser = boolPointer(result.SummarySourceHadUser)
		event.CompactSummaryLatestUserPinned = boolPointer(result.SummaryLatestUserPinned)
	}
	event.CompactFallbackMarker = firstNonEmptyCompactValue(result.FallbackMarker, result.Fallback.Marker)
	if result.Fallback.Used {
		event.CompactFailureReason = result.Fallback.Reason
	}
	e.emit(event)
}

func (e *runEventEmitter) emitCompactFailed(state ContextWindowState, mode CompactMode, message string, failureReason string) {
	event := contextWindowEvent(CompactFailedEventType, state, message)
	event.CompactMode = mode
	event.CompactFailureReason = failureReason
	e.emit(event)
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func contextWindowEvent(eventType AwakenEventType, state ContextWindowState, content string) AwakenEvent {
	event := AwakenEvent{
		Type:    eventType,
		Content: content,
	}
	applyContextWindowEstimate(&event, state)
	return event
}

func applyContextWindowEstimate(event *AwakenEvent, state ContextWindowState) {
	if event == nil {
		return
	}
	event.ContextModelWindow = state.ModelWindow
	event.ContextOutputReserve = state.OutputReserve
	event.ContextEffectiveWindow = state.EffectiveWindow
	event.ContextEstimatedTokens = state.EstimatedTokens
	event.ContextWarningThreshold = state.WarningThreshold
	event.ContextAutoCompactThreshold = state.AutoCompactThreshold
	event.ContextCompactTarget = state.CompactTarget
	event.ContextEstimatedDeltaTokens = state.EstimatedDeltaTokens
}

func intPointer(value int) *int {
	v := value
	return &v
}

func boolPointer(value bool) *bool {
	v := value
	return &v
}

func (e *runEventEmitter) emitTurnDiff(ctx stdcontext.Context, workspaceRoot string) {
	if !e.mutationOccurred {
		return
	}
	diff, truncated, ok := collectWorkspaceDiff(ctx, workspaceRoot)
	if !ok || strings.TrimSpace(diff) == "" {
		return
	}
	e.emit(AwakenEvent{
		Type:      TurnDiffEventType,
		Diff:      diff,
		Truncated: truncated,
	})
}

func collectWorkspaceDiff(ctx stdcontext.Context, workspaceRoot string) (string, bool, bool) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return "", false, false
	}
	trackedDiff, ok := gitOutput(ctx, workspaceRoot, "diff", "--")
	if !ok {
		return "", false, false
	}
	untrackedDiff, _ := collectUntrackedWorkspaceDiff(ctx, workspaceRoot)
	diff := trackedDiff
	if diff != "" && untrackedDiff != "" && !strings.HasSuffix(diff, "\n") {
		diff += "\n"
	}
	diff += untrackedDiff
	limited := outputlimit.Limit(diff, maxTurnDiffChars)
	return limited.Content, limited.Truncated, true
}

func collectUntrackedWorkspaceDiff(ctx stdcontext.Context, workspaceRoot string) (string, bool) {
	untracked, ok := gitOutput(ctx, workspaceRoot, "ls-files", "--others", "--exclude-standard", "-z")
	if !ok || untracked == "" {
		return "", ok
	}
	var combined strings.Builder
	for _, relPath := range strings.Split(untracked, "\x00") {
		if relPath == "" {
			continue
		}
		diff, ok := gitOutputAllowDiffExit(ctx, workspaceRoot, "diff", "--no-index", "--", os.DevNull, relPath)
		if !ok {
			return combined.String(), false
		}
		if combined.Len() > 0 && diff != "" && !strings.HasSuffix(combined.String(), "\n") {
			combined.WriteString("\n")
		}
		combined.WriteString(diff)
	}
	return combined.String(), true
}

func gitOutput(ctx stdcontext.Context, workspaceRoot string, args ...string) (string, bool) {
	cmdArgs := append([]string{"-C", workspaceRoot}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", false
	}
	return stdout.String(), true
}

func gitOutputAllowDiffExit(ctx stdcontext.Context, workspaceRoot string, args ...string) (string, bool) {
	cmdArgs := append([]string{"-C", workspaceRoot}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return "", false
		}
	}
	return stdout.String(), true
}

func (e *runEventEmitter) startTextIfNeeded() {
	if e.textOpen {
		return
	}
	e.textMessageID = e.ensureAssistantMessageID()
	e.textStartedThisTurn = true
	e.textOpen = true
	e.emit(AwakenEvent{
		Type:      TextMessageStartedEventType,
		MessageID: e.textMessageID,
		Role:      "assistant",
	})
}

func (e *runEventEmitter) finishTextIfOpen() {
	if !e.textOpen {
		return
	}
	e.emit(AwakenEvent{
		Type:      TextMessageFinishedEventType,
		MessageID: e.textMessageID,
	})
	e.textOpen = false
	e.textMessageID = ""
}

func (e *runEventEmitter) startReasoningIfNeeded() {
	if e.reasoningOpen {
		return
	}
	e.reasoningMessageID = newAwakenID("reasoning")
	e.reasoningOpen = true
	e.emit(AwakenEvent{
		Type:      ReasoningStartedEventType,
		MessageID: e.reasoningMessageID,
	})
}

func (e *runEventEmitter) finishReasoningIfOpen() {
	if !e.reasoningOpen {
		return
	}
	e.emit(AwakenEvent{
		Type:      ReasoningFinishedEventType,
		MessageID: e.reasoningMessageID,
	})
	e.reasoningOpen = false
	e.reasoningMessageID = ""
}

func (e *runEventEmitter) ensureAssistantMessageID() string {
	if e.currentAssistantID == "" {
		e.currentAssistantID = newAwakenID("msg")
	}
	return e.currentAssistantID
}

func (e *runEventEmitter) resetTurn() {
	e.currentAssistantID = ""
	e.textMessageID = ""
	e.reasoningMessageID = ""
	e.textOpen = false
	e.reasoningOpen = false
	e.textStartedThisTurn = false
}

func (e *runEventEmitter) textWasStartedThisTurn() bool {
	return e.textStartedThisTurn
}
