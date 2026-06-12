package agentcore

import (
	stdcontext "context"
	"encoding/json"
	"fmt"

	"awaken/src/context"
	"awaken/src/llm"
	"awaken/src/tool"
)

type Runner struct {
	config       AgentConfig
	conversation *context.Conversation
	reqBuilder   *RequestBuilder
}

func NewRunner(config AgentConfig, conv *context.Conversation, reqBuilder *RequestBuilder) *Runner {
	return &Runner{
		config:       config,
		conversation: conv,
		reqBuilder:   reqBuilder,
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

func (r *Runner) RunStream(ctx stdcontext.Context, callback func(string)) (string, error) {
	for i := 0; i < r.config.MaxThoughts; i++ {
		response, err := r.invokeLLM(ctx)
		if err != nil {
			return "", fmt.Errorf("LLM call failed: %w", err)
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
		callback(response.Content)
		r.conversation.AssistantOutput(response.Content, nil)
		return response.Content, nil
	}

	return "", fmt.Errorf("达到最大思考次数 %d，未能完成任务", r.config.MaxThoughts)
}

func (r *Runner) buildLLMRequest() LLMRequest {
	return r.reqBuilder.Build(r.conversation.Context, nil)
}

func (r *Runner) invokeLLM(ctx stdcontext.Context) (*llm.Response, error) {
	llmReq := r.buildLLMRequest()
	return llm.Invoke(ctx, llm.Config{
		APIKey:      r.config.ModelConfig.APIKey,
		BaseURL:     r.config.ModelConfig.BaseURL,
		Model:       llmReq.Model,
		Temperature: llmReq.Temperature,
		MaxTokens:   llmReq.MaxTokens,
	}, llm.Request{
		SystemPrompt: llmReq.SystemPrompt,
		Messages:     llmReq.Messages,
		Tools:        llmReq.Tools,
	})
}

func (r *Runner) handleXMLFallback(ctx stdcontext.Context, response string, callback func(string)) (bool, string) {
	parsed, err := ParseAction(response)
	if err != nil {
		return false, ""
	}

	if parsed.ToolName == "final" || parsed.ToolName == "总结" {
		r.conversation.AssistantOutput(parsed.Input, nil)
		if callback != nil {
			callback(parsed.Input)
		}
		return true, parsed.Input
	}

	toolCall := context.ToolCall{
		ID:   context.GenerateToolCallID(),
		Type: "function",
		Function: context.FunctionCall{
			Name:      parsed.ToolName,
			Arguments: parsed.Input,
		},
	}
	r.conversation.AssistantOutput(response, []context.ToolCall{toolCall})
	r.executeToolCalls(ctx, []context.ToolCall{toolCall}, callback)
	return true, ""
}

func (r *Runner) executeToolCalls(ctx stdcontext.Context, toolCalls []context.ToolCall, callback func(string)) {
	for _, tc := range toolCalls {
		observation, err := executeToolCall(ctx, tc)
		if err != nil {
			observation = fmt.Sprintf("工具执行失败: %v", err)
		}
		if callback != nil {
			callback(fmt.Sprintf("\n\n[工具结果] %s: %s\n\n", tc.Function.Name, observation))
		}
		r.conversation.ToolResult(tc.ID, tc.Function.Name, observation)
	}
}

func executeToolCall(ctx stdcontext.Context, tc context.ToolCall) (string, error) {
	selectedTool, err := tool.GetTool(tc.Function.Name)
	if err != nil {
		return "", err
	}
	return selectedTool.Execute(ctx, normalizeToolInput(tc.Function.Arguments, selectedTool.Parameters()))
}

func normalizeToolInput(arguments string, params []tool.ToolParameter) string {
	if len(params) != 1 {
		return arguments
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &decoded); err != nil {
		return arguments
	}

	value, ok := decoded[params[0].Name]
	if !ok {
		return arguments
	}
	if str, ok := value.(string); ok {
		return str
	}

	data, err := json.Marshal(value)
	if err != nil {
		return arguments
	}
	return string(data)
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
