package llm

import (
	"bufio"
	"bytes"
	stdctx "context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"awaken/src/context"
)

// openaiTransport 实现 OpenAI Chat Completions 协议
type openaiTransport struct{}

func newOpenAITransport() *openaiTransport {
	return &openaiTransport{}
}

// --- 请求结构 ---

type openaiChatRequest struct {
	Model       string                   `json:"model"`
	Messages    []openaiChatMessage      `json:"messages"`
	Temperature float64                  `json:"temperature,omitempty"`
	MaxTokens   int                      `json:"max_tokens,omitempty"`
	Stream      bool                     `json:"stream,omitempty"`
	Tools       []map[string]interface{} `json:"tools,omitempty"`
}

type openaiChatMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	Name       string           `json:"name,omitempty"`
}

type openaiToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openaiFunctionCall `json:"function"`
}

type openaiFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// --- 响应结构 ---

type openaiChatResponse struct {
	Choices []struct {
		Message struct {
			Content   string           `json:"content"`
			ToolCalls []openaiToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
	} `json:"choices"`
}

type openaiStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content   string                 `json:"content,omitempty"`
			ToolCalls []openaiStreamToolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
	} `json:"choices"`
}

type openaiStreamToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

// --- 实现 Transport 接口 ---

func (t *openaiTransport) Invoke(ctx stdctx.Context, cfg Config, req Request) (*Response, error) {
	url := cfg.BaseURL + "/chat/completions"

	chatReq := openaiChatRequest{
		Model:       resolveModel(cfg, req),
		Messages:    buildOpenAIMessages(req),
		Temperature: resolveTemperature(cfg, req),
		MaxTokens:   resolveMaxTokens(cfg, req),
	}

	if len(req.Tools) > 0 {
		chatReq.Tools = convertToolsToOpenAI(req.Tools)
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	applyAuthHeaders(httpReq, cfg, ProtocolOpenAIChat)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRequestFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, requestFailedError(resp)
	}

	var chatResp openaiChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, ErrEmptyResponse
	}

	return responseFromOpenAIMessage(chatResp.Choices[0].Message.Content, chatResp.Choices[0].Message.ToolCalls)
}

func (t *openaiTransport) StreamInvoke(ctx stdctx.Context, cfg Config, req Request, callback StreamCallback) (*Response, error) {
	url := cfg.BaseURL + "/chat/completions"

	chatReq := openaiChatRequest{
		Model:       resolveModel(cfg, req),
		Messages:    buildOpenAIMessages(req),
		Temperature: resolveTemperature(cfg, req),
		MaxTokens:   resolveMaxTokens(cfg, req),
		Stream:      true,
	}

	if len(req.Tools) > 0 {
		chatReq.Tools = convertToolsToOpenAI(req.Tools)
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	applyAuthHeaders(httpReq, cfg, ProtocolOpenAIChat)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRequestFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, requestFailedError(resp)
	}

	state := newOpenAIStreamState(callback)
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				break
			}
			state.consume(data)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read stream: %w", err)
	}

	return state.response()
}

// --- 辅助函数 ---

func buildOpenAIMessages(req Request) []openaiChatMessage {
	messages := []openaiChatMessage{}
	if req.SystemPrompt != "" {
		messages = append(messages, openaiChatMessage{Role: "system", Content: req.SystemPrompt})
	}

	if len(req.Messages) > 0 {
		for _, msg := range req.Messages {
			messages = append(messages, openaiChatMessage{
				Role:       string(msg.Role),
				Content:    msg.Content,
				ToolCalls:  contextToolCallsToOpenAI(msg.ToolCalls),
				ToolCallID: msg.ToolCallID,
				Name:       msg.Name,
			})
		}
	} else if req.UserPrompt != "" {
		messages = append(messages, openaiChatMessage{Role: "user", Content: req.UserPrompt})
	}
	return messages
}

func contextToolCallsToOpenAI(toolCalls []context.ToolCall) []openaiToolCall {
	if len(toolCalls) == 0 {
		return nil
	}

	result := make([]openaiToolCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		result = append(result, openaiToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: openaiFunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}
	return result
}

func responseFromOpenAIMessage(content string, toolCalls []openaiToolCall) (*Response, error) {
	result := &Response{
		Content:    content,
		ToolCalls:  make([]ToolCall, 0, len(toolCalls)),
		IsFinished: len(toolCalls) == 0,
	}

	for _, tc := range toolCalls {
		result.ToolCalls = append(result.ToolCalls, ToolCall{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: tc.Function.Arguments,
		})
	}

	if result.Content == "" && len(result.ToolCalls) == 0 {
		return nil, ErrEmptyResponse
	}
	return result, nil
}

func convertToolsToOpenAI(tools []context.ToolDefinition) []map[string]interface{} {
	result := make([]map[string]interface{}, len(tools))
	for i, t := range tools {
		result[i] = map[string]interface{}{
			"type": t.Type,
			"function": map[string]interface{}{
				"name":        t.Function.Name,
				"description": t.Function.Description,
				"parameters":  t.Function.Parameters,
			},
		}
	}
	return result
}

// --- OpenAI Stream State ---

type openAIStreamState struct {
	callback  StreamCallback
	content   strings.Builder
	toolCalls map[int]*openaiStreamToolAccumulator
}

type openaiStreamToolAccumulator struct {
	id        string
	callType  string
	name      strings.Builder
	arguments strings.Builder
}

func newOpenAIStreamState(callback StreamCallback) *openAIStreamState {
	return &openAIStreamState{
		callback:  callback,
		toolCalls: make(map[int]*openaiStreamToolAccumulator),
	}
}

func (s *openAIStreamState) consume(data string) {
	var chunk openaiStreamResponse
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return
	}

	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			s.content.WriteString(choice.Delta.Content)
			s.emit(choice.Delta.Content, false)
		}

		for _, toolCall := range choice.Delta.ToolCalls {
			acc := s.toolCalls[toolCall.Index]
			if acc == nil {
				acc = &openaiStreamToolAccumulator{}
				s.toolCalls[toolCall.Index] = acc
			}
			if toolCall.ID != "" {
				acc.id = toolCall.ID
			}
			if toolCall.Type != "" {
				acc.callType = toolCall.Type
			}
			if toolCall.Function.Name != "" {
				acc.name.WriteString(toolCall.Function.Name)
			}
			if toolCall.Function.Arguments != "" {
				acc.arguments.WriteString(toolCall.Function.Arguments)
			}
		}
	}
}

func (s *openAIStreamState) emit(content string, isThinking bool) {
	if s.callback != nil {
		s.callback(content, isThinking)
	}
}

func (s *openAIStreamState) response() (*Response, error) {
	toolCalls := s.sortedToolCalls()
	result := &Response{
		Content:    s.content.String(),
		ToolCalls:  toolCalls,
		IsFinished: len(toolCalls) == 0,
	}
	if result.Content == "" && len(result.ToolCalls) == 0 {
		return nil, ErrEmptyResponse
	}
	return result, nil
}

func (s *openAIStreamState) sortedToolCalls() []ToolCall {
	if len(s.toolCalls) == 0 {
		return nil
	}

	indexes := make([]int, 0, len(s.toolCalls))
	for index := range s.toolCalls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	result := make([]ToolCall, 0, len(indexes))
	for _, index := range indexes {
		acc := s.toolCalls[index]
		result = append(result, ToolCall{
			ID:    acc.id,
			Name:  acc.name.String(),
			Input: acc.arguments.String(),
		})
	}
	return result
}
