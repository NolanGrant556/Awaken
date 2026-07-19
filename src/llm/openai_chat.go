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
	"awaken/src/tool"
)

// openaiTransport 实现 OpenAI Chat Completions 协议
type openaiTransport struct{}

func newOpenAITransport() *openaiTransport {
	return &openaiTransport{}
}

// --- 请求结构 ---

type openaiChatRequest struct {
	Model          string                   `json:"model"`
	Messages       []openaiChatMessage      `json:"messages"`
	Temperature    *float64                 `json:"temperature,omitempty"`
	MaxTokens      int                      `json:"max_tokens,omitempty"`
	Stream         bool                     `json:"stream,omitempty"`
	Thinking       *openaiThinkingConfig    `json:"thinking,omitempty"`
	EnableThinking *bool                    `json:"enable_thinking,omitempty"`
	Reasoning      *openaiReasoningConfig   `json:"reasoning,omitempty"`
	StreamOptions  *openaiStreamOptions     `json:"stream_options,omitempty"`
	PromptCacheKey string                   `json:"prompt_cache_key,omitempty"`
	Tools          []map[string]interface{} `json:"tools,omitempty"`
}

type openaiStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openaiChatMessage struct {
	Role             string           `json:"role"`
	Content          interface{}      `json:"content,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCalls        []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
	Name             string           `json:"name,omitempty"`
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

type openaiThinkingConfig struct {
	Type string `json:"type"`
}

type openaiReasoningConfig struct {
	Enabled bool `json:"enabled"`
}

// --- 响应结构 ---

type openaiChatResponse struct {
	Usage   *openAIUsage `json:"usage,omitempty"`
	Choices []struct {
		Message struct {
			Content          string           `json:"content"`
			ReasoningContent string           `json:"reasoning_content,omitempty"`
			ToolCalls        []openaiToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
}

type openaiStreamResponse struct {
	Usage   *openAIUsage `json:"usage,omitempty"`
	Choices []struct {
		Delta struct {
			Content          string                 `json:"content,omitempty"`
			ReasoningContent string                 `json:"reasoning_content,omitempty"`
			ToolCalls        []openaiStreamToolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
}

type openAIUsage struct {
	PromptTokens             *int `json:"prompt_tokens,omitempty"`
	CompletionTokens         *int `json:"completion_tokens,omitempty"`
	TotalTokens              *int `json:"total_tokens,omitempty"`
	InputTokens              *int `json:"input_tokens,omitempty"`
	OutputTokens             *int `json:"output_tokens,omitempty"`
	CachedTokens             *int `json:"cached_tokens,omitempty"`
	PromptCacheHitTokens     *int `json:"prompt_cache_hit_tokens,omitempty"`
	PromptCacheMissTokens    *int `json:"prompt_cache_miss_tokens,omitempty"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens,omitempty"`
	PromptTokensDetails      *struct {
		CachedTokens *int `json:"cached_tokens,omitempty"`
	} `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *struct {
		ReasoningTokens *int `json:"reasoning_tokens,omitempty"`
	} `json:"completion_tokens_details,omitempty"`
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
	prepared, err := prepareOpenAIRequest(cfg, req, false)
	if err != nil {
		return nil, err
	}
	return t.invokePrepared(ctx, cfg, prepared)
}

func prepareOpenAIRequest(cfg Config, req Request, stream bool) (PreparedRequest, error) {
	url := cfg.BaseURL + "/chat/completions"
	req = applyRequestPolicy(cfg, req)
	cacheMetadata, affinityKey := promptCacheMetadata(cfg, req)

	chatReq := openaiChatRequest{
		Model:       resolveModel(cfg, req),
		Messages:    buildOpenAIMessages(cfg, req, cacheMetadata),
		Temperature: resolveTemperature(cfg, req),
		MaxTokens:   resolveMaxTokens(cfg, req),
		Stream:      stream,
	}
	applyOpenAIReasoningControls(&chatReq, cfg, req)
	if affinityKey != "" {
		chatReq.PromptCacheKey = affinityKey
		if stream {
			chatReq.StreamOptions = &openaiStreamOptions{IncludeUsage: true}
		}
	}

	if len(req.Tools) > 0 {
		chatReq.Tools = convertToolsToOpenAI(req.Tools)
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return PreparedRequest{}, fmt.Errorf("failed to marshal request: %w", err)
	}
	prepared := newPreparedRequest(req, ProtocolOpenAIChat, url, body, stream)
	prepared.Cache = cacheMetadata
	return prepared, nil
}

func (t *openaiTransport) invokePrepared(ctx stdctx.Context, cfg Config, prepared PreparedRequest) (*Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, prepared.Endpoint, bytes.NewReader(prepared.Body))
	if err != nil {
		return nil, requestBuildError(err)
	}

	applyAuthHeaders(httpReq, cfg, ProtocolOpenAIChat)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, transportError(err)
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

	choice := chatResp.Choices[0]
	result, responseErr := responseFromOpenAIMessage(choice.Message.Content, choice.Message.ReasoningContent, choice.Message.ToolCalls, choice.FinishReason, choiceUsage(chatResp.Usage, usageNormalizerID(cfg)))
	if result != nil {
		result.ProviderRequestID = providerRequestID(resp)
	}
	return result, responseErr
}

func (t *openaiTransport) StreamInvoke(ctx stdctx.Context, cfg Config, req Request, callback StreamCallback) (*Response, error) {
	return t.StreamInvokeWithObserver(ctx, cfg, req, StreamObserver{OnContent: callback})
}

func (t *openaiTransport) StreamInvokeWithObserver(ctx stdctx.Context, cfg Config, req Request, observer StreamObserver) (*Response, error) {
	prepared, err := prepareOpenAIRequest(cfg, req, true)
	if err != nil {
		return nil, err
	}
	return t.streamPrepared(ctx, cfg, prepared, observer)
}

func (t *openaiTransport) streamPrepared(ctx stdctx.Context, cfg Config, prepared PreparedRequest, observer StreamObserver) (*Response, error) {
	streamCtx, cancel := stdctx.WithCancel(ctx)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, prepared.Endpoint, bytes.NewReader(prepared.Body))
	if err != nil {
		return nil, requestBuildError(err)
	}

	applyAuthHeaders(httpReq, cfg, ProtocolOpenAIChat)

	resp, err := streamHTTPClient.Do(httpReq)
	if err != nil {
		return nil, transportError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, requestFailedError(resp)
	}

	state := newOpenAIStreamState(observer, usageNormalizerID(cfg))
	scanner := bufio.NewScanner(resp.Body)
	watchdog := newStreamIdleWatchdog(cancel, streamIdleTimeoutForPrepared(prepared))
	defer watchdog.Stop()

	for scanner.Scan() {
		watchdog.Reset()
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
		if watchdog.TimedOut() {
			return nil, streamIdleTimeoutError(watchdog.Timeout())
		}
		return nil, fmt.Errorf("failed to read stream: %w", err)
	}

	result, responseErr := state.response()
	if result != nil {
		result.ProviderRequestID = providerRequestID(resp)
	}
	return result, responseErr
}

// --- 辅助函数 ---

func buildOpenAIMessages(cfg Config, req Request, cache PreparedPromptCache) []openaiChatMessage {
	messages := []openaiChatMessage{}
	if req.SystemPrompt != "" {
		messages = append(messages, openaiChatMessage{Role: "system", Content: req.SystemPrompt})
	}

	if len(req.Messages) > 0 {
		for index := 0; index < len(req.Messages); {
			msg := req.Messages[index]
			message := openaiChatMessage{
				Role: string(msg.Role), Content: openAIContentForMessage(msg),
				ToolCalls: contextToolCallsToOpenAI(msg.ToolCalls), ToolCallID: msg.ToolCallID, Name: msg.Name,
			}
			if msg.Role == context.RoleAssistant {
				message.ReasoningContent = replayProviderOpaqueReasoning(cfg, req, cache, msg.ProviderOpaqueState)
			}
			messages = append(messages, message)
			index++
			if msg.Role != context.RoleAssistant || len(msg.ToolCalls) == 0 {
				continue
			}
			toolMessages := make([]context.Message, 0, len(msg.ToolCalls))
			for index < len(req.Messages) && req.Messages[index].Role == context.RoleTool {
				toolMessage := req.Messages[index]
				messages = append(messages, openaiChatMessage{
					Role: string(toolMessage.Role), Content: openAIToolMessageContent(toolMessage),
					ToolCallID: toolMessage.ToolCallID, Name: toolMessage.Name,
				})
				toolMessages = append(toolMessages, toolMessage)
				index++
			}
			if content := openAIToolMediaUserContent(toolMessages); content != nil {
				// OpenAI-compatible APIs require every tool-role result for the
				// assistant batch before the associated multimodal user message.
				messages = append(messages, openaiChatMessage{Role: "user", Content: content})
			}
		}
	} else if req.UserPrompt != "" {
		messages = append(messages, openaiChatMessage{Role: "user", Content: req.UserPrompt})
	}
	return messages
}

func replayProviderOpaqueReasoning(cfg Config, req Request, cache PreparedPromptCache, state *context.ProviderOpaqueMessageState) string {
	if state == nil || !cache.ReasoningReplay || strings.TrimSpace(state.Content) == "" {
		return ""
	}
	revision := cfg.CredentialRevision
	if revision == 0 {
		revision = 1
	}
	if state.ProviderID != normalizeProviderID(cfg.ProviderID) ||
		state.ProfileID != strings.TrimSpace(cfg.ProfileID) ||
		state.CredentialRevision != revision ||
		state.ModelID != resolveModel(cfg, req) ||
		state.ProtocolFamily != string(resolveProtocol(cfg)) ||
		state.EndpointContractID != cache.EndpointContract {
		return ""
	}
	return state.Content
}

func openAIToolMediaUserContent(messages []context.Message) interface{} {
	blocks := make([]map[string]interface{}, 0)
	toolCallIDs := make([]string, 0)
	for _, message := range messages {
		if len(message.ModelContent) == 0 {
			continue
		}
		toolCallIDs = append(toolCallIDs, message.ToolCallID)
		for _, part := range message.ModelContent {
			if strings.TrimSpace(part.DataBase64) == "" || strings.TrimSpace(part.MimeType) == "" {
				continue
			}
			switch part.Type {
			case "image":
				blocks = append(blocks, map[string]interface{}{
					"type": "image_url", "image_url": map[string]interface{}{
						"url": fmt.Sprintf("data:%s;base64,%s", part.MimeType, part.DataBase64),
					},
				})
			case "file":
				if part.MimeType == "application/pdf" {
					filename := strings.TrimSpace(part.DisplayName)
					if filename == "" {
						filename = "tool-result.pdf"
					}
					blocks = append(blocks, map[string]interface{}{
						"type": "file", "file": map[string]interface{}{
							"filename":  filename,
							"file_data": fmt.Sprintf("data:%s;base64,%s", part.MimeType, part.DataBase64),
						},
					})
				}
			}
		}
	}
	if len(blocks) == 0 {
		return nil
	}
	blocks = append([]map[string]interface{}{{
		"type": "text", "text": "Multimodal content returned by tool_call_id(s): " + strings.Join(toolCallIDs, ", "),
	}}, blocks...)
	return blocks
}

func openAIContentForMessage(msg context.Message) interface{} {
	if msg.Role == context.RoleTool {
		return openAIToolMessageContent(msg)
	}
	if msg.Role != context.RoleUser || len(msg.Attachments) == 0 {
		return msg.Content
	}

	blocks := make([]map[string]interface{}, 0, len(msg.Attachments)+1)
	userText := userTextWithProviderProjection(msg.Content, msg.Attachments, msg.ProviderProjectionText)
	if strings.TrimSpace(userText) != "" {
		blocks = append(blocks, map[string]interface{}{
			"type": "text",
			"text": userText,
		})
	}
	for _, attachment := range msg.Attachments {
		if attachment.Kind != "image" ||
			strings.TrimSpace(attachment.MimeType) == "" ||
			strings.TrimSpace(attachment.DataBase64) == "" {
			continue
		}
		blocks = append(blocks, map[string]interface{}{
			"type": "image_url",
			"image_url": map[string]interface{}{
				"url": fmt.Sprintf("data:%s;base64,%s", attachment.MimeType, attachment.DataBase64),
			},
		})
	}
	if len(blocks) == 0 {
		return msg.Content
	}
	return blocks
}

// openAIToolMessageContent builds tool-role content for OpenAI-compatible APIs.
// When content is already ModelPayload-style JSON, preserve it and ensure
// is_error / status semantics are present. Otherwise wrap plain text with
// structured status/error fields derived from Message.IsError.
func openAIToolMessageContent(msg context.Message) string {
	content := msg.Content
	if looksLikeModelPayloadJSON(content) {
		return ensureModelPayloadIsError(content, msg.IsError)
	}
	if !msg.IsError {
		return content
	}
	payload := map[string]any{
		"content":         content,
		"status":          "failed",
		"semantic_status": "failure",
		"is_error":        true,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return content
	}
	return string(data)
}

func looksLikeModelPayloadJSON(content string) bool {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "{") {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return false
	}
	_, hasStatus := payload["status"]
	_, hasTransport := payload["transport_status"]
	_, hasSemantic := payload["semantic_status"]
	_, hasProcess := payload["process_status"]
	return hasStatus || hasTransport || hasSemantic || hasProcess
}

func ensureModelPayloadIsError(content string, isError bool) string {
	if !isError {
		return content
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return content
	}
	payload["is_error"] = true
	if status, _ := payload["status"].(string); status == "" || status == "succeeded" {
		payload["status"] = "failed"
	}
	if _, ok := payload["semantic_status"]; !ok {
		payload["semantic_status"] = "failure"
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return content
	}
	return string(data)
}

func applyOpenAIReasoningControls(chatReq *openaiChatRequest, cfg Config, req Request) {
	if !req.DisableReasoning {
		return
	}
	if !vendorPrivateFieldsAllowed(cfg) {
		return
	}
	if isKimiK27Code(strings.ToLower(resolveModel(cfg, req))) {
		return
	}
	providerKey := strings.ToLower(strings.Join([]string{
		cfg.BaseURL,
		cfg.Model,
		req.Model,
	}, " "))
	switch {
	case strings.Contains(providerKey, "deepseek") ||
		strings.Contains(providerKey, "kimi") ||
		strings.Contains(providerKey, "moonshot") ||
		strings.Contains(providerKey, "volces") ||
		strings.Contains(providerKey, "volcengine") ||
		strings.Contains(providerKey, "byteplus") ||
		strings.Contains(providerKey, "ark"):
		chatReq.Thinking = &openaiThinkingConfig{Type: "disabled"}
	case strings.Contains(providerKey, "dashscope") ||
		strings.Contains(providerKey, "aliyuncs") ||
		strings.Contains(providerKey, "qwen"):
		disabled := false
		chatReq.EnableThinking = &disabled
	case strings.Contains(providerKey, "openrouter"):
		chatReq.Reasoning = &openaiReasoningConfig{Enabled: false}
	}
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

func responseFromOpenAIMessage(content string, thinking string, toolCalls []openaiToolCall, stopReason string, usage *Usage) (*Response, error) {
	stopReason = strings.TrimSpace(stopReason)
	result := &Response{
		Content:          content,
		Thinking:         thinking,
		ToolCalls:        make([]ToolCall, 0, len(toolCalls)),
		Usage:            usage,
		IsFinished:       len(toolCalls) == 0,
		StopReason:       stopReason,
		MaxOutputReached: isMaxOutputStopReason(stopReason),
	}

	for _, tc := range toolCalls {
		result.ToolCalls = append(result.ToolCalls, ToolCall{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: tc.Function.Arguments,
		})
	}

	if !responseHasSignal(result) {
		return nil, ErrEmptyResponse
	}
	return result, nil
}

func convertToolsToOpenAI(tools []tool.ToolSchema) []map[string]interface{} {
	result := make([]map[string]interface{}, len(tools))
	for i, t := range tools {
		result[i] = map[string]interface{}{
			"type": t.Type,
			"function": map[string]interface{}{
				"name":        t.Function.Name,
				"description": t.Function.Description,
				"parameters":  tool.SchemaAsMap(t.Function.Parameters),
			},
		}
	}
	return result
}

// --- OpenAI Stream State ---

type openAIStreamState struct {
	observer          StreamObserver
	content           strings.Builder
	thinking          strings.Builder
	toolCalls         map[int]*openaiStreamToolAccumulator
	usage             *Usage
	stopReason        string
	usageNormalizerID string
}

type openaiStreamToolAccumulator struct {
	id        string
	callType  string
	name      strings.Builder
	arguments strings.Builder
}

func newOpenAIStreamState(observer StreamObserver, normalizerID string) *openAIStreamState {
	return &openAIStreamState{
		observer: observer, toolCalls: make(map[int]*openaiStreamToolAccumulator),
		usageNormalizerID: normalizerID,
	}
}

func (s *openAIStreamState) consume(data string) {
	var chunk openaiStreamResponse
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return
	}
	if usage := choiceUsage(chunk.Usage, s.usageNormalizerID); usage != nil {
		s.usage = usage
	}

	for _, choice := range chunk.Choices {
		if choice.FinishReason != "" {
			s.stopReason = strings.TrimSpace(choice.FinishReason)
		}
		if choice.Delta.ReasoningContent != "" {
			s.thinking.WriteString(choice.Delta.ReasoningContent)
			s.emit(choice.Delta.ReasoningContent, true)
		}
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
				s.emitToolCallDelta(ToolCallDelta{
					Index:          toolCall.Index,
					ID:             acc.id,
					Name:           acc.name.String(),
					ArgumentsDelta: toolCall.Function.Arguments,
				})
			}
		}
	}
}

func (s *openAIStreamState) emit(content string, isThinking bool) {
	if s.observer.OnContent != nil {
		s.observer.OnContent(content, isThinking)
	}
}

func (s *openAIStreamState) emitToolCallDelta(delta ToolCallDelta) {
	if s.observer.OnToolCallDelta != nil {
		s.observer.OnToolCallDelta(delta)
	}
}

func (s *openAIStreamState) response() (*Response, error) {
	toolCalls := s.sortedToolCalls()
	result := &Response{
		Content:          s.content.String(),
		Thinking:         s.thinking.String(),
		ToolCalls:        toolCalls,
		Usage:            s.usage,
		IsFinished:       len(toolCalls) == 0,
		StopReason:       s.stopReason,
		MaxOutputReached: isMaxOutputStopReason(s.stopReason),
	}
	if !responseHasSignal(result) {
		return nil, ErrEmptyResponse
	}
	return result, nil
}

func choiceUsage(raw *openAIUsage, normalizerID string) *Usage {
	if raw == nil {
		return nil
	}
	input := firstValidTokenPointer(raw.PromptTokens, raw.InputTokens)
	output := firstValidTokenPointer(raw.CompletionTokens, raw.OutputTokens)
	total := validTokenPointer(raw.TotalTokens)
	cached := firstValidTokenPointer(
		raw.PromptCacheHitTokens,
		raw.CachedTokens,
		promptDetailsCachedTokens(raw.PromptTokensDetails),
		raw.CacheReadInputTokens,
	)
	cacheWrite := validTokenPointer(raw.CacheCreationInputTokens)
	rawReasoning := completionDetailsReasoningTokens(raw.CompletionTokensDetails)
	reasoning := rawReasoning
	if output != nil && reasoning != nil && *reasoning > *output {
		value := *output
		reasoning = &value
	}
	var uncached *int
	if normalizerID == "deepseek_cache_usage_v1" &&
		raw.PromptCacheHitTokens != nil && raw.PromptCacheMissTokens != nil &&
		*raw.PromptCacheHitTokens >= 0 && *raw.PromptCacheMissTokens >= 0 {
		value := *raw.PromptCacheHitTokens + *raw.PromptCacheMissTokens
		if input == nil {
			input = &value
		}
	}
	if input != nil && (cached != nil || cacheWrite != nil) {
		cachedValue := 0
		if cached != nil {
			cachedValue = *cached
		}
		cacheWriteValue := 0
		if cacheWrite != nil {
			cacheWriteValue = *cacheWrite
		}
		if cachedValue+cacheWriteValue <= *input {
			value := *input - cachedValue - cacheWriteValue
			uncached = &value
		}
	}
	if raw.PromptCacheHitTokens != nil && raw.PromptCacheMissTokens != nil && input != nil &&
		*raw.PromptCacheHitTokens >= 0 && *raw.PromptCacheMissTokens >= 0 &&
		*raw.PromptCacheHitTokens+*raw.PromptCacheMissTokens == *input {
		value := *raw.PromptCacheMissTokens
		uncached = &value
	}
	if input == nil && output == nil && total == nil && cached == nil && cacheWrite == nil && raw.PromptCacheMissTokens == nil {
		return nil
	}
	if total == nil {
		total = sumTokenPointers(input, output)
	}
	return &Usage{
		InputTokens: input, CachedInputTokens: cached, UncachedInputTokens: uncached,
		CacheWriteTokens: cacheWrite, OutputTokens: output, ReasoningTokens: reasoning, TotalTokens: total,
		Raw: RawUsage{
			NormalizerID: normalizerID, PromptTokens: raw.PromptTokens, InputTokens: raw.InputTokens,
			CompletionTokens: raw.CompletionTokens, OutputTokens: raw.OutputTokens, TotalTokens: raw.TotalTokens,
			CachedTokens: raw.CachedTokens, PromptCacheHitTokens: raw.PromptCacheHitTokens,
			PromptCacheMissTokens: raw.PromptCacheMissTokens, CacheReadInputTokens: raw.CacheReadInputTokens,
			CacheCreationInputTokens: raw.CacheCreationInputTokens, ReasoningTokens: rawReasoning,
		},
	}
}

func completionDetailsReasoningTokens(details *struct {
	ReasoningTokens *int `json:"reasoning_tokens,omitempty"`
}) *int {
	if details == nil {
		return nil
	}
	return validTokenPointer(details.ReasoningTokens)
}

func promptDetailsCachedTokens(details *struct {
	CachedTokens *int `json:"cached_tokens,omitempty"`
}) *int {
	if details == nil {
		return nil
	}
	return details.CachedTokens
}

func firstValidTokenPointer(values ...*int) *int {
	for _, value := range values {
		if normalized := validTokenPointer(value); normalized != nil {
			return normalized
		}
	}
	return nil
}

func validTokenPointer(value *int) *int {
	if value == nil || *value < 0 {
		return nil
	}
	normalized := *value
	return &normalized
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
		id := acc.id
		if id == "" {
			id = fmt.Sprintf("tool_index_%d", index)
		}
		result = append(result, ToolCall{
			ID:    id,
			Name:  acc.name.String(),
			Input: acc.arguments.String(),
		})
	}
	return result
}
