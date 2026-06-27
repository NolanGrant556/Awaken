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
	"awaken/src/tool"
)

const maxTurnDiffChars = 50000

type Runner struct {
	config       AgentConfig
	conversation *context.Conversation
	reqBuilder   *RequestBuilder
	toolRuntime  *ToolRuntime
	threadID     string
}

func NewRunner(config AgentConfig, conv *context.Conversation, reqBuilder *RequestBuilder) *Runner {
	return &Runner{
		config:       config,
		conversation: conv,
		reqBuilder:   reqBuilder,
		toolRuntime:  NewToolRuntimeWithConfig(nil, toolRuntimeConfigFromAgentConfig(config)),
	}
}

func (r *Runner) RunStreamWithEvents(ctx stdcontext.Context, sink EventSink) (string, error) {
	events := newRunEventEmitter(r.threadID, sink)
	r.toolRuntime = NewToolRuntimeWithConfig(sink, toolRuntimeConfigFromAgentConfig(r.config))
	events.runStarted()

	for i := 0; i < r.config.MaxThoughts; i++ {
		select {
		case <-ctx.Done():
			r.injectSyntheticToolResultsForCancel()
			events.runErrored(ctx.Err())
			return "", ctx.Err()
		default:
		}

		response, err := r.streamLLM(ctx, events)
		if err != nil {
			if ctx.Err() != nil {
				r.injectSyntheticToolResultsForCancel()
				events.runErrored(ctx.Err())
				return "", ctx.Err()
			}
			runErr := fmt.Errorf("LLM stream call failed: %w", err)
			events.runErrored(runErr)
			return "", runErr
		}

		events.finishReasoningIfOpen()
		events.finishTextIfOpen()

		if len(response.ToolCalls) > 0 {
			toolCalls := llmToolCallsToContext(response.ToolCalls)
			results := r.executeToolCalls(ctx, toolCalls, events)
			r.conversation.ToolExchange(response.Content, toolCalls, toolExecutionResultsToContext(results))
			events.resetTurn()
			continue
		}

		if handled, finalAnswer := r.handleXMLFallback(ctx, response.Content, events); handled {
			if finalAnswer != "" {
				events.emitTurnDiff(ctx, r.config.Workspace.Root)
				events.runFinished(finalAnswer)
				return finalAnswer, nil
			}
			events.resetTurn()
			continue
		}

		if response.Content == "" {
			events.runErrored(llm.ErrEmptyResponse)
			return "", llm.ErrEmptyResponse
		}
		r.conversation.AssistantOutput(response.Content, nil)
		events.finishTextIfOpen()
		events.emitTurnDiff(ctx, r.config.Workspace.Root)
		events.runFinished(response.Content)
		return response.Content, nil
	}

	err := fmt.Errorf("达到最大思考次数 %d，未能完成任务", r.config.MaxThoughts)
	events.runErrored(err)
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
		PermissionContext: tool.NewPermissionContext(config.Workspace.Root, config.Workspace.WritableRoots),
	}
}

func (r *Runner) buildLLMRequest() (LLMRequest, error) {
	llmReq := r.reqBuilder.Build(r.conversation.Context, nil)
	if err := context.ValidateToolResultPairing(llmReq.Messages); err != nil {
		return LLMRequest{}, fmt.Errorf("invalid tool result pairing: %w", err)
	}
	return llmReq, nil
}

func (r *Runner) streamLLM(ctx stdcontext.Context, events *runEventEmitter) (*llm.Response, error) {
	llmReq, err := r.buildLLMRequest()
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
		Temperature: llmReq.Temperature,
		MaxTokens:   llmReq.MaxTokens,
		AuthScheme:  r.config.ModelConfig.AuthScheme,
	}, llm.Request{
		SystemPrompt: llmReq.SystemPrompt,
		Messages:     llmReq.Messages,
		Tools:        llmReq.Tools,
	}, observer)
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
			events.emitToolProgress(call, "Executing tool")
		}
		result := r.toolRuntime.Execute(ctx, call)
		results = append(results, result)
		if events != nil && call.Function.Name == "apply_patch" && !result.IsError {
			events.markMutation()
		}
		if events != nil {
			events.emitToolProgress(call, toolProgressForResult(result))
			events.emitToolResult(result)
		}
	}
	return results
}

func toolProgressForResult(result ToolExecutionResult) string {
	if result.IsError {
		return "Tool execution failed"
	}
	return "Tool execution complete"
}

func syntheticCancelledToolResult(call context.ToolCall) ToolExecutionResult {
	return ToolExecutionResult{
		ToolCallID: call.ID,
		Name:       call.Function.Name,
		Content:    "工具执行已被取消",
		IsError:    true,
	}
}

func toolExecutionResultsToContext(results []ToolExecutionResult) []context.ToolResult {
	out := make([]context.ToolResult, 0, len(results))
	for _, result := range results {
		out = append(out, context.ToolResult{
			ToolCallID: result.ToolCallID,
			Name:       result.Name,
			Content:    result.Content,
			IsError:    result.IsError,
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
	e.sink.Emit(DisplaySafeEvent(event))
}

func (e *runEventEmitter) runStarted() {
	e.emit(AwakenEvent{Type: RunStartedEventType})
}

func (e *runEventEmitter) runFinished(content string) {
	e.emit(AwakenEvent{Type: RunFinishedEventType, Content: content})
}

func (e *runEventEmitter) runErrored(err error) {
	if err == nil {
		return
	}
	event := AwakenEvent{Type: RunErrorEventType, Error: err.Error()}
	switch {
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

func (e *runEventEmitter) emitToolProgress(call context.ToolCall, progress string) {
	if progress == "" {
		return
	}
	e.emit(AwakenEvent{
		Type:       ToolProgressEventType,
		ToolCallID: call.ID,
		ToolName:   call.Function.Name,
		Progress:   progress,
	})
}

func (e *runEventEmitter) emitToolResult(result ToolExecutionResult) {
	e.emit(AwakenEvent{
		Type:       ToolResultEventType,
		MessageID:  newAwakenID("toolmsg"),
		ToolCallID: result.ToolCallID,
		ToolName:   result.Name,
		Content:    result.Content,
		Role:       "tool",
	})
}

func (e *runEventEmitter) markMutation() {
	e.mutationOccurred = true
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
