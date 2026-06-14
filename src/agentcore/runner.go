package agentcore

import (
	stdcontext "context"
	"fmt"

	"awaken/src/context"
	"awaken/src/llm"
)

type Runner struct {
	config       AgentConfig
	conversation *context.Conversation
	reqBuilder   *RequestBuilder
	toolRuntime  *ToolRuntime
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
			r.executeToolCalls(ctx, toolCalls, nil)
			continue
		}

		if handled, finalAnswer := r.handleXMLFallback(ctx, response.Content, nil); handled {
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
		response, err := r.streamLLM(ctx, callback)
		if err != nil {
			return "", fmt.Errorf("LLM stream call failed: %w", err)
		}

		if len(response.ToolCalls) > 0 {
			toolCalls := llmToolCallsToContext(response.ToolCalls)
			r.conversation.AssistantOutput(response.Content, toolCalls)
			r.executeToolCalls(ctx, toolCalls, callback)
			continue
		}

		if handled, finalAnswer := r.handleXMLFallback(ctx, response.Content, callback); handled {
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

func (r *Runner) streamLLM(ctx stdcontext.Context, callback StreamCallback) (*llm.Response, error) {
	llmReq, err := r.buildLLMRequest()
	if err != nil {
		return nil, err
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
	}, llm.StreamCallback(callback))
}

func (r *Runner) handleXMLFallback(ctx stdcontext.Context, response string, callback StreamCallback) (bool, string) {
	parsed, err := ParseAction(response)
	if err != nil {
		return false, ""
	}

	if parsed.ToolName == "final" || parsed.ToolName == "总结" {
		r.conversation.AssistantOutput(parsed.Input, nil)
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
	r.executeToolCalls(ctx, []context.ToolCall{toolCall}, callback)
	return true, ""
}

func (r *Runner) executeToolCalls(ctx stdcontext.Context, toolCalls []context.ToolCall, callback StreamCallback) {
	for _, result := range r.toolRuntime.ExecuteToolCalls(ctx, toolCalls) {
		if callback != nil {
			callback(fmt.Sprintf("\n\n[工具结果] %s: %s\n\n", result.Name, result.Content), false)
		}
		r.conversation.ToolResult(result.ToolCallID, result.Name, result.Content)
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
