package agentcore

import (
	stdcontext "context"
	"fmt"
	"strings"
	"time"

	"awaken/src/context"
	"awaken/src/llm"
)

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
		toolRuntime:  NewToolRuntime(),
	}
}

func (r *Runner) Run(ctx stdcontext.Context) (string, error) {
	for i := 0; i < r.config.MaxThoughts; i++ {
		response, err := r.invokeLLM(ctx)
		if err != nil {
			return "", fmt.Errorf("LLM call failed: %w", err)
		}

		if len(response.ToolCalls) > 0 {
			toolCalls := llmToolCallsToContext(response.ToolCalls)
			r.conversation.AssistantOutput(response.Content, toolCalls)
			r.executeToolCalls(ctx, toolCalls, nil, nil)
			continue
		}

		if handled, finalAnswer := r.handleXMLFallback(ctx, response.Content, nil, nil); handled {
			if finalAnswer != "" {
				return finalAnswer, nil
			}
			continue
		}

		if response.Content == "" {
			return "", llm.ErrEmptyResponse
		}
		r.conversation.AssistantOutput(response.Content, nil)
		return response.Content, nil
	}

	return "", fmt.Errorf("达到最大思考次数 %d，未能完成任务", r.config.MaxThoughts)
}

func (r *Runner) RunStream(ctx stdcontext.Context, callback StreamCallback) (string, error) {
	for i := 0; i < r.config.MaxThoughts; i++ {
		response, err := r.streamLLM(ctx, callback, nil)
		if err != nil {
			return "", fmt.Errorf("LLM stream call failed: %w", err)
		}

		if len(response.ToolCalls) > 0 {
			toolCalls := llmToolCallsToContext(response.ToolCalls)
			r.conversation.AssistantOutput(response.Content, toolCalls)
			r.executeToolCalls(ctx, toolCalls, callback, nil)
			continue
		}

		if handled, finalAnswer := r.handleXMLFallback(ctx, response.Content, callback, nil); handled {
			if finalAnswer != "" {
				return finalAnswer, nil
			}
			continue
		}

		if response.Content == "" {
			return "", llm.ErrEmptyResponse
		}
		r.conversation.AssistantOutput(response.Content, nil)
		return response.Content, nil
	}

	return "", fmt.Errorf("达到最大思考次数 %d，未能完成任务", r.config.MaxThoughts)
}

func (r *Runner) RunStreamWithEvents(ctx stdcontext.Context, callback StreamCallback, sink EventSink) (string, error) {
	events := newRunEventEmitter(r.threadID, sink)
	events.runStarted()

	for i := 0; i < r.config.MaxThoughts; i++ {
		response, err := r.streamLLM(ctx, callback, events)
		if err != nil {
			runErr := fmt.Errorf("LLM stream call failed: %w", err)
			events.runErrored(runErr)
			return "", runErr
		}

		events.finishReasoningIfOpen()
		events.finishTextIfOpen()

		if len(response.ToolCalls) > 0 {
			toolCalls := llmToolCallsToContext(response.ToolCalls)
			r.conversation.AssistantOutput(response.Content, toolCalls)
			r.emitToolCallEvents(toolCalls, events)
			r.executeToolCalls(ctx, toolCalls, callback, events)
			events.resetTurn()
			continue
		}

		if handled, finalAnswer := r.handleXMLFallback(ctx, response.Content, callback, events); handled {
			if finalAnswer != "" {
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
		events.runFinished(response.Content)
		return response.Content, nil
	}

	err := fmt.Errorf("达到最大思考次数 %d，未能完成任务", r.config.MaxThoughts)
	events.runErrored(err)
	return "", err
}

func (r *Runner) buildLLMRequest() (LLMRequest, error) {
	llmReq := r.reqBuilder.Build(r.conversation.Context, nil)
	if err := context.ValidateToolResultPairing(llmReq.Messages); err != nil {
		return LLMRequest{}, fmt.Errorf("invalid tool result pairing: %w", err)
	}
	return llmReq, nil
}

func (r *Runner) invokeLLM(ctx stdcontext.Context) (*llm.Response, error) {
	llmReq, err := r.buildLLMRequest()
	if err != nil {
		return nil, err
	}
	return llm.Invoke(ctx, llm.Config{
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
	})
}

func (r *Runner) streamLLM(ctx stdcontext.Context, callback StreamCallback, events *runEventEmitter) (*llm.Response, error) {
	llmReq, err := r.buildLLMRequest()
	if err != nil {
		return nil, err
	}
	streamCallback := callback
	if events != nil {
		streamCallback = func(content string, isThinking bool) {
			events.onLLMDelta(content, isThinking)
			if callback != nil {
				callback(content, isThinking)
			}
		}
	}
	return llm.StreamInvoke(ctx, llm.Config{
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
	}, llm.StreamCallback(streamCallback))
}

func (r *Runner) handleXMLFallback(ctx stdcontext.Context, response string, callback StreamCallback, events *runEventEmitter) (bool, string) {
	parsed, err := ParseAction(response)
	if err != nil {
		return false, ""
	}

	if parsed.ToolName == "final" || parsed.ToolName == "总结" {
		r.conversation.AssistantOutput(parsed.Input, nil)
		if events != nil && !events.textWasStartedThisTurn() && parsed.Input != "" {
			events.emitStandaloneText(parsed.Input)
		}
		if callback != nil {
			callback(parsed.Input, false)
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
	r.conversation.AssistantOutput(response, []context.ToolCall{toolCall})
	if events != nil {
		events.emitToolCall(toolCall)
	}
	r.executeToolCalls(ctx, []context.ToolCall{toolCall}, callback, events)
	return true, ""
}

func (r *Runner) emitToolCallEvents(toolCalls []context.ToolCall, events *runEventEmitter) {
	if events == nil {
		return
	}
	for _, toolCall := range toolCalls {
		events.emitToolCall(toolCall)
	}
}

func (r *Runner) executeToolCalls(ctx stdcontext.Context, toolCalls []context.ToolCall, callback StreamCallback, events *runEventEmitter) {
	for _, result := range r.toolRuntime.ExecuteToolCalls(ctx, toolCalls) {
		if callback != nil {
			callback(fmt.Sprintf("\n\n[工具结果] %s: %s\n\n", result.Name, result.Content), false)
		}
		r.conversation.ToolResult(result.ToolCallID, result.Name, result.Content)
		if events != nil {
			events.emitToolResult(result)
		}
	}
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
	e.sink.Emit(event)
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
	e.emit(AwakenEvent{Type: RunErrorEventType, Error: err.Error()})
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
			Arguments:  call.Function.Arguments,
		})
	}
	e.emit(AwakenEvent{
		Type:       ToolCallFinishedEventType,
		ToolCallID: call.ID,
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
