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
	Model           string                   `json:"model"`
	Messages        []openaiChatMessage      `json:"messages"`
	Temperature     *float64                 `json:"temperature,omitempty"`
	MaxTokens       int                      `json:"max_tokens,omitempty"`
	Stream          bool                     `json:"stream,omitempty"`
	Thinking        *openaiThinkingConfig    `json:"thinking,omitempty"`
	EnableThinking  *bool                    `json:"enable_thinking,omitempty"`
	Reasoning       *openaiReasoningConfig   `json:"reasoning,omitempty"`
	ReasoningEffort string                   `json:"reasoning_effort,omitempty"`
	ReasoningSplit  *bool                    `json:"reasoning_split,omitempty"`
	StreamOptions   *openaiStreamOptions     `json:"stream_options,omitempty"`
	PromptCacheKey  string                   `json:"prompt_cache_key,omitempty"`
	Tools           []map[string]interface{} `json:"tools,omitempty"`
}

type openaiStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openaiChatMessage struct {
	Role             string                  `json:"role"`
	Content          interface{}             `json:"content,omitempty"`
	ReasoningContent string                  `json:"reasoning_content,omitempty"`
	ReasoningDetails []openAIReasoningDetail `json:"reasoning_details,omitempty"`
	ToolCalls        []openaiToolCall        `json:"tool_calls,omitempty"`
	ToolCallID       string                  `json:"tool_call_id,omitempty"`
	Name             string                  `json:"name,omitempty"`
}

type openAIReasoningDetail struct {
	Type      string `json:"type,omitempty"`
	Text      string `json:"text,omitempty"`
	Signature string `json:"signature,omitempty"`
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
			Content          string                  `json:"content"`
			ReasoningContent string                  `json:"reasoning_content,omitempty"`
			ReasoningDetails []openAIReasoningDetail `json:"reasoning_details,omitempty"`
			ToolCalls        []openaiToolCall        `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
}

type openaiStreamResponse struct {
	Usage   *openAIUsage `json:"usage,omitempty"`
	Choices []struct {
		Delta struct {
			Content          string                  `json:"content,omitempty"`
			ReasoningContent string                  `json:"reasoning_content,omitempty"`
			ReasoningDetails []openAIReasoningDetail `json:"reasoning_details,omitempty"`
			ToolCalls        []openaiStreamToolCall  `json:"tool_calls,omitempty"`
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
	protocol := resolveProtocol(cfg)
	url := strings.TrimSpace(cfg.InferenceEndpoint)
	if url == "" {
		url = strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"
	}
	if protocol == ProtocolMiniMaxChat {
		if strings.TrimSpace(cfg.InferenceEndpoint) == "" {
			url = strings.TrimRight(cfg.BaseURL, "/") + "/text/chatcompletion_v2"
		}
	}
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
	if !omitsEffortWhenThinkingDisabled(cfg, req) {
		chatReq.ReasoningEffort = nativeReasoningEffort(cfg, req)
	}
	if usesMiniMaxReasoningSplit(cfg) {
		enabled := true
		chatReq.ReasoningSplit = &enabled
		cacheMetadata.ReasoningReplay = true
		if cacheMetadata.EndpointContract == "" {
			cacheMetadata.EndpointContract = EndpointContractOpenAIChat
		}
	}
	if affinityKey != "" {
		chatReq.PromptCacheKey = affinityKey
	}
	if stream && supportsOpenAIChatStreamingUsage(cfg) {
		chatReq.StreamOptions = &openaiStreamOptions{IncludeUsage: true}
	}

	if len(req.Tools) > 0 {
		chatReq.Tools = convertToolsToOpenAI(req.Tools)
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return PreparedRequest{}, fmt.Errorf("failed to marshal request: %w", err)
	}
	prepared := newPreparedRequest(req, protocol, url, body, stream)
	prepared.Cache = cacheMetadata
	return prepared, nil
}

func (t *openaiTransport) invokePrepared(ctx stdctx.Context, cfg Config, prepared PreparedRequest) (*Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, prepared.Endpoint, bytes.NewReader(prepared.Body))
	if err != nil {
		return nil, requestBuildError(err)
	}

	applyAuthHeaders(httpReq, cfg, prepared.Protocol)

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
	result, responseErr := responseFromOpenAIMessage(choice.Message.Content, choice.Message.ReasoningContent, choice.Message.ReasoningDetails, choice.Message.ToolCalls, choice.FinishReason, choiceUsage(chatResp.Usage, usageNormalizerID(cfg)))
	if result != nil {
		result.ProviderRequestID = providerRequestID(resp)
		result.RateLimitObservation = providerRateLimitObservation(resp)
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

	applyAuthHeaders(httpReq, cfg, prepared.Protocol)

	resp, err := streamHTTPClient.Do(httpReq)
	if err != nil {
		return nil, transportError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, requestFailedError(resp)
	}

	state := newOpenAIStreamState(observer, usageNormalizerID(cfg), usesMiniMaxReasoningSplit(cfg))
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	watchdog := newStreamIdleWatchdog(cancel, streamIdleTimeoutForPrepared(prepared))
	defer watchdog.Stop()
	terminalSeen := false

	for scanner.Scan() {
		watchdog.Reset()
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				terminalSeen = true
				break
			}
			state.consume(data)
		}
	}

	if err := scanner.Err(); err != nil {
		if watchdog.TimedOut() {
			return nil, streamIdleTimeoutError(watchdog.Timeout())
		}
		return nil, streamReadError(streamCtx, err)
	}
	if !terminalSeen && !state.completed {
		return nil, &StreamDisconnectedError{Reason: "stream eof"}
	}

	result, responseErr := state.response()
	if result != nil {
		result.ProviderRequestID = providerRequestID(resp)
		result.RateLimitObservation = providerRateLimitObservation(resp)
		state.emitEvent(StreamEvent{Type: StreamEventCompleted, Usage: result.Usage, StopReason: result.StopReason})
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
				reasoning := replayProviderOpaqueReasoning(cfg, req, cache, msg.ProviderOpaqueState)
				if usesMiniMaxReasoningSplit(cfg) && reasoning != "" {
					message.ReasoningDetails = []openAIReasoningDetail{{Type: "reasoning.text", Text: reasoning}}
				} else {
					message.ReasoningContent = reasoning
				}
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
func openAIToolMessageContent(msg context.Message) (output string) {
	defer func() {
		if msg.ProviderProjectionText != "" {
			output += "\n\n" + msg.ProviderProjectionText
		}
	}()
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
	if isKimiK27Code(strings.ToLower(resolveModel(cfg, req))) {
		return
	}
	provider := normalizeProviderID(cfg.ProviderID)
	model := strings.ToLower(resolveModel(cfg, req))
	if (provider == "qwen" && (model == "qwen3.8-max" || model == "qwen3.8-flash")) ||
		(provider == "qwen-token-plan" && (model == "qwen3.8-max-preview" || model == "qwen3.8-max" || model == "qwen3.8-flash")) {
		// These contracts are reasoning-only. Auxiliary purposes omit effort but
		// must not attempt to disable thinking, otherwise title/compact probes fail.
		return
	}
	capability, ok := ResolveReasoningControlCapability(cfg, req)
	if !ok {
		return
	}
	switch capability.WireStrategy {
	case ReasoningControlThinkingType:
		chatReq.Thinking = &openaiThinkingConfig{Type: "disabled"}
	case ReasoningControlEnableThinking:
		disabled := false
		chatReq.EnableThinking = &disabled
	}
}

func omitsEffortWhenThinkingDisabled(cfg Config, req Request) bool {
	if !req.DisableReasoning {
		return false
	}
	capability, ok := ResolveReasoningControlCapability(cfg, req)
	return ok && capability.WireStrategy == ReasoningControlThinkingType
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

func responseFromOpenAIMessage(content string, thinking string, reasoningDetails []openAIReasoningDetail, toolCalls []openaiToolCall, stopReason string, usage *Usage) (*Response, error) {
	if detailsThinking := reasoningDetailsText(reasoningDetails); detailsThinking != "" {
		thinking = detailsThinking
	}
	cleanContent, inlineThinking, split := splitLeadingThinkBlock(content)
	if split {
		content = cleanContent
		if thinking == "" {
			thinking = inlineThinking
		}
	}
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

func usesMiniMaxReasoningSplit(cfg Config) bool {
	provider := normalizeProviderID(cfg.ProviderID)
	return resolveProtocol(cfg) == ProtocolOpenAIChat &&
		(provider == "minimax" || provider == "minimax-token-plan")
}

func reasoningDetailsText(details []openAIReasoningDetail) string {
	var text strings.Builder
	for _, detail := range details {
		text.WriteString(detail.Text)
	}
	return text.String()
}

func convertToolsToOpenAI(tools []tool.ToolSchema) []map[string]interface{} {
	result := make([]map[string]interface{}, len(tools))
	for i, t := range tools {
		result[i] = map[string]interface{}{
			"type": t.Type,
			"function": map[string]interface{}{
				"name":        t.Function.Name,
				"description": t.Function.Description,
				"parameters":  projectOpenAICompatibleToolParameters(t.Function.Parameters),
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
	completed         bool
	usageNormalizerID string
	miniMaxSnapshots  bool
	contentSnapshot   string
	reasoningSnapshot string
	inlineThinking    leadingThinkStream
}

type openaiStreamToolAccumulator struct {
	id        string
	callType  string
	name      strings.Builder
	arguments strings.Builder
}

func newOpenAIStreamState(observer StreamObserver, normalizerID string, miniMaxSnapshots bool) *openAIStreamState {
	return &openAIStreamState{
		observer: observer, toolCalls: make(map[int]*openaiStreamToolAccumulator),
		usageNormalizerID: normalizerID, miniMaxSnapshots: miniMaxSnapshots,
	}
}

func (s *openAIStreamState) consume(data string) {
	var chunk openaiStreamResponse
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return
	}
	if usage := choiceUsage(chunk.Usage, s.usageNormalizerID); usage != nil {
		s.usage = usage
		s.emitEvent(StreamEvent{Type: StreamEventUsage, Usage: usage})
	}

	for _, choice := range chunk.Choices {
		if choice.FinishReason != "" {
			s.stopReason = strings.TrimSpace(choice.FinishReason)
			s.completed = true
		}
		reasoningDelta := choice.Delta.ReasoningContent
		if detailsSnapshot := reasoningDetailsText(choice.Delta.ReasoningDetails); detailsSnapshot != "" {
			reasoningDelta = snapshotSuffix(s.reasoningSnapshot, detailsSnapshot)
			s.reasoningSnapshot = detailsSnapshot
		}
		if reasoningDelta != "" {
			s.writeReasoning(reasoningDelta)
		}
		if choice.Delta.Content != "" {
			contentDelta := choice.Delta.Content
			if s.miniMaxSnapshots {
				contentDelta = snapshotSuffix(s.contentSnapshot, choice.Delta.Content)
				s.contentSnapshot = choice.Delta.Content
			}
			s.inlineThinking.consume(contentDelta, s.writeReasoning, s.writeContent)
		}

		for _, toolCall := range choice.Delta.ToolCalls {
			acc := s.toolCalls[toolCall.Index]
			if acc == nil {
				acc = &openaiStreamToolAccumulator{}
				s.toolCalls[toolCall.Index] = acc
				started := ToolCall{ID: toolCall.ID, Name: toolCall.Function.Name}
				s.emitEvent(StreamEvent{Type: StreamEventToolCallStart, ToolCall: &started})
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
	eventType := StreamEventTextDelta
	if isThinking {
		eventType = StreamEventReasoningDelta
	}
	s.emitEvent(StreamEvent{Type: eventType, Content: content, IsThinking: isThinking})
}

func (s *openAIStreamState) writeContent(content string) {
	if content == "" {
		return
	}
	s.content.WriteString(content)
	s.emit(content, false)
}

func (s *openAIStreamState) writeReasoning(content string) {
	if content == "" {
		return
	}
	s.thinking.WriteString(content)
	s.emit(content, true)
}

func (s *openAIStreamState) emitToolCallDelta(delta ToolCallDelta) {
	if s.observer.OnToolCallDelta != nil {
		s.observer.OnToolCallDelta(delta)
	}
	s.emitEvent(StreamEvent{Type: StreamEventToolCallDelta, ToolCallDelta: &delta})
}

func (s *openAIStreamState) emitEvent(event StreamEvent) {
	if s.observer.OnEvent != nil {
		s.observer.OnEvent(event)
	}
}

func (s *openAIStreamState) response() (*Response, error) {
	s.inlineThinking.finish(s.writeReasoning, s.writeContent)
	toolCalls := s.sortedToolCalls()
	for index := range toolCalls {
		call := toolCalls[index]
		s.emitEvent(StreamEvent{Type: StreamEventToolCallDone, ToolCall: &call})
	}
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

func snapshotSuffix(previous, current string) string {
	if previous == "" {
		return current
	}
	if strings.HasPrefix(current, previous) {
		return current[len(previous):]
	}
	// Some compatible endpoints switch from cumulative snapshots to real
	// deltas mid-stream. Preserve the new frame instead of dropping content.
	return current
}

func splitLeadingThinkBlock(content string) (answer string, thinking string, ok bool) {
	start := len(content) - len(strings.TrimLeft(content, " \t\r\n"))
	if !strings.HasPrefix(content[start:], "<think>") {
		return content, "", false
	}
	rest := content[start+len("<think>"):]
	if end := strings.Index(rest, "</think>"); end >= 0 {
		return strings.TrimLeft(rest[end+len("</think>"):], "\r\n"), rest[:end], true
	}
	return "", rest, true
}

type leadingThinkStream struct {
	mode   uint8
	buffer string
}

const (
	thinkStreamUndecided uint8 = iota
	thinkStreamReasoning
	thinkStreamText
)

func (s *leadingThinkStream) consume(delta string, reasoning func(string), text func(string)) {
	if delta == "" {
		return
	}
	switch s.mode {
	case thinkStreamText:
		text(delta)
		return
	case thinkStreamReasoning:
		s.consumeReasoning(delta, reasoning, text)
		return
	}

	s.buffer += delta
	trimmed := strings.TrimLeft(s.buffer, " \t\r\n")
	if strings.HasPrefix(trimmed, "<think>") {
		s.mode = thinkStreamReasoning
		s.buffer = strings.TrimPrefix(trimmed, "<think>")
		s.consumeReasoning("", reasoning, text)
		return
	}
	if strings.HasPrefix("<think>", trimmed) {
		return
	}
	s.mode = thinkStreamText
	text(s.buffer)
	s.buffer = ""
}

func (s *leadingThinkStream) consumeReasoning(delta string, reasoning func(string), text func(string)) {
	s.buffer += delta
	if end := strings.Index(s.buffer, "</think>"); end >= 0 {
		reasoning(s.buffer[:end])
		remainder := strings.TrimLeft(s.buffer[end+len("</think>"):], "\r\n")
		s.buffer = ""
		s.mode = thinkStreamText
		text(remainder)
		return
	}
	// Keep only the possible closing-tag prefix buffered so reasoning remains
	// truly streaming while a </think> tag may be split across SSE chunks.
	keep := longestSuffixPrefix(s.buffer, "</think>")
	emitUntil := len(s.buffer) - keep
	if emitUntil > 0 {
		reasoning(s.buffer[:emitUntil])
		s.buffer = s.buffer[emitUntil:]
	}
}

func (s *leadingThinkStream) finish(reasoning func(string), text func(string)) {
	switch s.mode {
	case thinkStreamReasoning:
		reasoning(s.buffer)
	case thinkStreamText:
		text(s.buffer)
	default:
		// A truncated prefix such as "<thi" is ordinary answer text; only a
		// complete leading <think> tag authorizes reclassification.
		text(s.buffer)
	}
	s.buffer = ""
}

func longestSuffixPrefix(value, marker string) int {
	limit := len(marker) - 1
	if len(value) < limit {
		limit = len(value)
	}
	for length := limit; length > 0; length-- {
		if strings.HasSuffix(value, marker[:length]) {
			return length
		}
	}
	return 0
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
