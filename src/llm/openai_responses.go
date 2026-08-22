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

	convcontext "awaken/src/context"
	"awaken/src/tool"
)

// openAIResponsesTransport implements the stateless OpenAI Responses API.
// Awaken owns conversation persistence, so requests always use store=false and
// never chain provider-side previous_response_id state.
type openAIResponsesTransport struct{}

func newOpenAIResponsesTransport() *openAIResponsesTransport { return &openAIResponsesTransport{} }

type openAIResponsesRequest struct {
	Model                string                    `json:"model"`
	Instructions         string                    `json:"instructions,omitempty"`
	Input                []any                     `json:"input"`
	Tools                []openAIResponsesTool     `json:"tools,omitempty"`
	Temperature          *float64                  `json:"temperature,omitempty"`
	MaxOutputTokens      int                       `json:"max_output_tokens,omitempty"`
	Reasoning            *openAIResponsesReasoning `json:"reasoning,omitempty"`
	PromptCacheKey       string                    `json:"prompt_cache_key,omitempty"`
	PromptCacheRetention string                    `json:"prompt_cache_retention,omitempty"`
	Stream               bool                      `json:"stream,omitempty"`
	Store                bool                      `json:"store"`
}

type openAIResponsesReasoning struct {
	Effort string `json:"effort,omitempty"`
}

type openAIResponsesTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type openAIResponsesResponse struct {
	ID                string                      `json:"id"`
	Status            string                      `json:"status"`
	Output            []openAIResponsesOutputItem `json:"output"`
	Usage             *openAIResponsesUsage       `json:"usage,omitempty"`
	IncompleteDetails *openAIResponsesIncomplete  `json:"incomplete_details,omitempty"`
	Error             *openAIResponsesError       `json:"error,omitempty"`
}

type openAIResponsesOutputItem struct {
	ID        string                        `json:"id,omitempty"`
	Type      string                        `json:"type"`
	Role      string                        `json:"role,omitempty"`
	CallID    string                        `json:"call_id,omitempty"`
	Name      string                        `json:"name,omitempty"`
	Arguments string                        `json:"arguments,omitempty"`
	Content   []openAIResponsesContentBlock `json:"content,omitempty"`
	Summary   []openAIResponsesSummaryBlock `json:"summary,omitempty"`
}

type openAIResponsesContentBlock struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Refusal string `json:"refusal,omitempty"`
}

type openAIResponsesSummaryBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type openAIResponsesUsage struct {
	InputTokens        *int `json:"input_tokens,omitempty"`
	OutputTokens       *int `json:"output_tokens,omitempty"`
	TotalTokens        *int `json:"total_tokens,omitempty"`
	InputTokensDetails *struct {
		CachedTokens *int `json:"cached_tokens,omitempty"`
	} `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *struct {
		ReasoningTokens *int `json:"reasoning_tokens,omitempty"`
	} `json:"output_tokens_details,omitempty"`
}

type openAIResponsesIncomplete struct {
	Reason string `json:"reason,omitempty"`
}

type openAIResponsesError struct {
	Code string `json:"code,omitempty"`
}

type openAIResponsesStreamEvent struct {
	Type        string                     `json:"type"`
	Delta       string                     `json:"delta,omitempty"`
	OutputIndex int                        `json:"output_index,omitempty"`
	ItemID      string                     `json:"item_id,omitempty"`
	Item        *openAIResponsesOutputItem `json:"item,omitempty"`
	Response    *openAIResponsesResponse   `json:"response,omitempty"`
	Error       *openAIResponsesError      `json:"error,omitempty"`
}

func buildOpenAIResponsesURL(cfg Config) string {
	if endpoint := strings.TrimSpace(cfg.InferenceEndpoint); endpoint != "" {
		return endpoint
	}
	return strings.TrimRight(cfg.BaseURL, "/") + "/responses"
}

func (t *openAIResponsesTransport) Invoke(ctx stdctx.Context, cfg Config, req Request) (*Response, error) {
	prepared, err := prepareOpenAIResponsesRequest(cfg, req, false)
	if err != nil {
		return nil, err
	}
	return t.invokePrepared(ctx, cfg, prepared)
}

func prepareOpenAIResponsesRequest(cfg Config, req Request, stream bool) (PreparedRequest, error) {
	req = applyRequestPolicy(cfg, req)
	cacheMetadata, affinityKey := promptCacheMetadata(cfg, req)
	input, err := buildOpenAIResponsesInput(req)
	if err != nil {
		return PreparedRequest{}, err
	}
	body := openAIResponsesRequest{
		Model: resolveModel(cfg, req), Instructions: req.SystemPrompt, Input: input,
		Tools: convertToolsToOpenAIResponses(req.Tools), Temperature: resolveTemperature(cfg, req),
		MaxOutputTokens: resolveMaxTokens(cfg, req), Stream: stream, Store: false,
	}
	if effort := nativeReasoningEffort(cfg, req); effort != "" {
		body.Reasoning = &openAIResponsesReasoning{Effort: effort}
	}
	if affinityKey != "" {
		body.PromptCacheKey = affinityKey
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return PreparedRequest{}, fmt.Errorf("failed to marshal request: %w", err)
	}
	prepared := newPreparedRequest(req, ProtocolOpenAIResponses, buildOpenAIResponsesURL(cfg), encoded, stream)
	prepared.Cache = cacheMetadata
	return prepared, nil
}

func (t *openAIResponsesTransport) invokePrepared(ctx stdctx.Context, cfg Config, prepared PreparedRequest) (*Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, prepared.Endpoint, bytes.NewReader(prepared.Body))
	if err != nil {
		return nil, requestBuildError(err)
	}
	applyAuthHeaders(httpReq, cfg, ProtocolOpenAIResponses)
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, transportError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, requestFailedError(resp)
	}
	var payload openAIResponsesResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	result, responseErr := responseFromOpenAIResponses(payload)
	if result != nil {
		result.ProviderRequestID = firstNonEmpty(providerRequestID(resp), payload.ID)
		result.RateLimitObservation = providerRateLimitObservation(resp)
	}
	return result, responseErr
}

func (t *openAIResponsesTransport) StreamInvoke(ctx stdctx.Context, cfg Config, req Request, callback StreamCallback) (*Response, error) {
	return t.StreamInvokeWithObserver(ctx, cfg, req, StreamObserver{OnContent: callback})
}

func (t *openAIResponsesTransport) StreamInvokeWithObserver(ctx stdctx.Context, cfg Config, req Request, observer StreamObserver) (*Response, error) {
	prepared, err := prepareOpenAIResponsesRequest(cfg, req, true)
	if err != nil {
		return nil, err
	}
	return t.streamPrepared(ctx, cfg, prepared, observer)
}

func (t *openAIResponsesTransport) streamPrepared(ctx stdctx.Context, cfg Config, prepared PreparedRequest, observer StreamObserver) (*Response, error) {
	streamCtx, cancel := stdctx.WithCancel(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, prepared.Endpoint, bytes.NewReader(prepared.Body))
	if err != nil {
		return nil, requestBuildError(err)
	}
	applyAuthHeaders(httpReq, cfg, ProtocolOpenAIResponses)
	resp, err := streamHTTPClient.Do(httpReq)
	if err != nil {
		return nil, transportError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, requestFailedError(resp)
	}

	state := newOpenAIResponsesStreamState(observer)
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	watchdog := newStreamIdleWatchdog(cancel, streamIdleTimeoutForPrepared(prepared))
	defer watchdog.Stop()
	terminalSeen := false
	for scanner.Scan() {
		watchdog.Reset()
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			terminalSeen = true
			continue
		}
		state.consume(data)
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
		result.ProviderRequestID = firstNonEmpty(providerRequestID(resp), state.responseID)
		result.RateLimitObservation = providerRateLimitObservation(resp)
	}
	return result, responseErr
}

func buildOpenAIResponsesInput(req Request) ([]any, error) {
	if len(req.Messages) == 0 {
		if strings.TrimSpace(req.UserPrompt) == "" {
			return []any{}, nil
		}
		return []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": req.UserPrompt}}}}, nil
	}
	if err := convcontext.ValidateToolResultPairing(req.Messages); err != nil {
		return nil, fmt.Errorf("invalid Responses tool result pairing: %w", err)
	}
	input := make([]any, 0, len(req.Messages)*2)
	for index := 0; index < len(req.Messages); index++ {
		message := req.Messages[index]
		switch message.Role {
		case convcontext.RoleUser:
			input = append(input, map[string]any{"role": "user", "content": openAIResponsesUserContent(message)})
		case convcontext.RoleAssistant:
			if strings.TrimSpace(message.Content) != "" {
				input = append(input, map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": message.Content}}})
			}
			for _, call := range message.ToolCalls {
				input = append(input, map[string]any{"type": "function_call", "call_id": call.ID, "name": call.Function.Name, "arguments": call.Function.Arguments})
			}
		case convcontext.RoleTool:
			input = append(input, map[string]any{"type": "function_call_output", "call_id": message.ToolCallID, "output": openAIToolMessageContent(message)})
			if media := openAIResponsesToolMediaContent(message); len(media) > 0 {
				input = append(input, map[string]any{"role": "user", "content": media})
			}
		}
	}
	return input, nil
}

func openAIResponsesUserContent(message convcontext.Message) []any {
	blocks := make([]any, 0, len(message.Attachments)+1)
	text := userTextWithProviderProjection(message.Content, message.Attachments, message.ProviderProjectionText)
	if strings.TrimSpace(text) != "" {
		blocks = append(blocks, map[string]any{"type": "input_text", "text": text})
	}
	for _, attachment := range message.Attachments {
		if strings.TrimSpace(attachment.DataBase64) == "" || strings.TrimSpace(attachment.MimeType) == "" {
			continue
		}
		dataURL := fmt.Sprintf("data:%s;base64,%s", attachment.MimeType, attachment.DataBase64)
		if attachment.Kind == "image" {
			blocks = append(blocks, map[string]any{"type": "input_image", "image_url": dataURL})
		} else if attachment.MimeType == "application/pdf" {
			name := firstNonEmpty(strings.TrimSpace(attachment.DisplayName), "attachment.pdf")
			blocks = append(blocks, map[string]any{"type": "input_file", "filename": name, "file_data": dataURL})
		}
	}
	if len(blocks) == 0 {
		blocks = append(blocks, map[string]any{"type": "input_text", "text": ""})
	}
	return blocks
}

func openAIResponsesToolMediaContent(message convcontext.Message) []any {
	blocks := make([]any, 0, len(message.ModelContent)+1)
	for _, part := range message.ModelContent {
		if strings.TrimSpace(part.DataBase64) == "" || strings.TrimSpace(part.MimeType) == "" {
			continue
		}
		dataURL := fmt.Sprintf("data:%s;base64,%s", part.MimeType, part.DataBase64)
		switch part.Type {
		case "image":
			blocks = append(blocks, map[string]any{"type": "input_image", "image_url": dataURL})
		case "file":
			if part.MimeType == "application/pdf" {
				blocks = append(blocks, map[string]any{"type": "input_file", "filename": firstNonEmpty(part.DisplayName, "tool-result.pdf"), "file_data": dataURL})
			}
		}
	}
	if len(blocks) > 0 {
		blocks = append([]any{map[string]any{"type": "input_text", "text": "Multimodal content returned by tool_call_id: " + message.ToolCallID}}, blocks...)
	}
	return blocks
}

func convertToolsToOpenAIResponses(tools []tool.ToolSchema) []openAIResponsesTool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]openAIResponsesTool, 0, len(tools))
	for _, schema := range tools {
		result = append(result, openAIResponsesTool{Type: "function", Name: schema.Function.Name, Description: schema.Function.Description, Parameters: projectOpenAICompatibleToolParameters(schema.Function.Parameters)})
	}
	return result
}

func responseFromOpenAIResponses(payload openAIResponsesResponse) (*Response, error) {
	result := &Response{Usage: normalizeOpenAIResponsesUsage(payload.Usage), StopReason: strings.TrimSpace(payload.Status)}
	for _, item := range payload.Output {
		switch item.Type {
		case "message":
			for _, block := range item.Content {
				if block.Type == "output_text" {
					result.Content += block.Text
				}
			}
		case "reasoning":
			for _, summary := range item.Summary {
				result.Thinking += summary.Text
			}
		case "function_call":
			result.ToolCalls = append(result.ToolCalls, ToolCall{ID: firstNonEmpty(item.CallID, item.ID), Name: item.Name, Input: item.Arguments})
		}
	}
	if payload.IncompleteDetails != nil && strings.TrimSpace(payload.IncompleteDetails.Reason) != "" {
		result.StopReason = strings.TrimSpace(payload.IncompleteDetails.Reason)
	}
	result.MaxOutputReached = isMaxOutputStopReason(result.StopReason)
	result.IsFinished = len(result.ToolCalls) == 0 && payload.Status == "completed"
	if payload.Error != nil {
		return result, fmt.Errorf("%w: responses error %s", ErrRequestFailed, firstNonEmpty(payload.Error.Code, "unknown"))
	}
	if !responseHasSignal(result) {
		return nil, ErrEmptyResponse
	}
	return result, nil
}

func normalizeOpenAIResponsesUsage(raw *openAIResponsesUsage) *Usage {
	if raw == nil {
		return nil
	}
	input := validTokenPointer(raw.InputTokens)
	output := validTokenPointer(raw.OutputTokens)
	total := validTokenPointer(raw.TotalTokens)
	var cached, reasoning *int
	if raw.InputTokensDetails != nil {
		cached = validTokenPointer(raw.InputTokensDetails.CachedTokens)
	}
	if raw.OutputTokensDetails != nil {
		reasoning = validTokenPointer(raw.OutputTokensDetails.ReasoningTokens)
	}
	var uncached *int
	if input != nil && cached != nil && *cached <= *input {
		value := *input - *cached
		uncached = &value
	}
	if total == nil {
		total = sumTokenPointers(input, output)
	}
	if input == nil && output == nil && total == nil && cached == nil && reasoning == nil {
		return nil
	}
	return &Usage{InputTokens: input, CachedInputTokens: cached, UncachedInputTokens: uncached, OutputTokens: output, ReasoningTokens: reasoning, TotalTokens: total, Raw: RawUsage{NormalizerID: "openai_responses_usage_v1", InputTokens: raw.InputTokens, OutputTokens: raw.OutputTokens, TotalTokens: raw.TotalTokens, CachedTokens: cached, ReasoningTokens: reasoning}}
}

type openAIResponsesStreamState struct {
	observer    StreamObserver
	content     strings.Builder
	reasoning   strings.Builder
	toolCalls   map[int]*openAIResponsesToolAccumulator
	usage       *Usage
	stopReason  string
	completed   bool
	failureCode string
	responseID  string
}

type openAIResponsesToolAccumulator struct {
	id        string
	name      string
	arguments strings.Builder
}

func newOpenAIResponsesStreamState(observer StreamObserver) *openAIResponsesStreamState {
	return &openAIResponsesStreamState{observer: observer, toolCalls: map[int]*openAIResponsesToolAccumulator{}}
}

func (s *openAIResponsesStreamState) consume(data string) {
	var event openAIResponsesStreamEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return
	}
	switch event.Type {
	case "response.output_text.delta":
		s.content.WriteString(event.Delta)
		s.emitContent(event.Delta, false)
	case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		s.reasoning.WriteString(event.Delta)
		s.emitContent(event.Delta, true)
	case "response.output_item.added":
		if event.Item != nil && event.Item.Type == "function_call" {
			s.toolCalls[event.OutputIndex] = &openAIResponsesToolAccumulator{id: firstNonEmpty(event.Item.CallID, event.Item.ID), name: event.Item.Name}
			started := ToolCall{ID: firstNonEmpty(event.Item.CallID, event.Item.ID), Name: event.Item.Name}
			s.emit(StreamEvent{Type: StreamEventToolCallStart, ToolCall: &started})
		}
	case "response.function_call_arguments.delta":
		acc := s.toolCalls[event.OutputIndex]
		if acc == nil {
			acc = &openAIResponsesToolAccumulator{id: event.ItemID}
			s.toolCalls[event.OutputIndex] = acc
		}
		acc.arguments.WriteString(event.Delta)
		delta := ToolCallDelta{Index: event.OutputIndex, ID: acc.id, Name: acc.name, ArgumentsDelta: event.Delta}
		s.emitToolDelta(delta)
	case "response.output_item.done":
		if event.Item != nil && event.Item.Type == "function_call" {
			acc := s.toolCalls[event.OutputIndex]
			if acc == nil {
				acc = &openAIResponsesToolAccumulator{}
				s.toolCalls[event.OutputIndex] = acc
			}
			acc.id = firstNonEmpty(event.Item.CallID, event.Item.ID, acc.id)
			acc.name = firstNonEmpty(event.Item.Name, acc.name)
			if acc.arguments.Len() == 0 {
				acc.arguments.WriteString(event.Item.Arguments)
			}
			completed := ToolCall{ID: acc.id, Name: acc.name, Input: acc.arguments.String()}
			s.emit(StreamEvent{Type: StreamEventToolCallDone, ToolCall: &completed})
		}
	case "response.completed", "response.incomplete":
		s.completed = true
		if event.Response != nil {
			s.responseID = event.Response.ID
			s.usage = normalizeOpenAIResponsesUsage(event.Response.Usage)
			s.stopReason = event.Response.Status
			if event.Response.IncompleteDetails != nil && event.Response.IncompleteDetails.Reason != "" {
				s.stopReason = event.Response.IncompleteDetails.Reason
			}
			s.emit(StreamEvent{Type: StreamEventUsage, Usage: s.usage})
			s.emit(StreamEvent{Type: StreamEventCompleted, StopReason: s.stopReason})
		}
	case "response.failed", "error":
		s.completed = true
		if event.Response != nil {
			s.responseID = event.Response.ID
			if event.Response.Error != nil {
				s.failureCode = event.Response.Error.Code
			}
		}
		if event.Error != nil {
			s.failureCode = event.Error.Code
		}
		s.emit(StreamEvent{Type: StreamEventFailed, ErrorCode: firstNonEmpty(s.failureCode, "unknown")})
	}
}

func (s *openAIResponsesStreamState) emitContent(content string, thinking bool) {
	if content == "" {
		return
	}
	if s.observer.OnContent != nil {
		s.observer.OnContent(content, thinking)
	}
	typeName := StreamEventTextDelta
	if thinking {
		typeName = StreamEventReasoningDelta
	}
	s.emit(StreamEvent{Type: typeName, Content: content, IsThinking: thinking})
}

func (s *openAIResponsesStreamState) emitToolDelta(delta ToolCallDelta) {
	if s.observer.OnToolCallDelta != nil {
		s.observer.OnToolCallDelta(delta)
	}
	s.emit(StreamEvent{Type: StreamEventToolCallDelta, ToolCallDelta: &delta})
}

func (s *openAIResponsesStreamState) emit(event StreamEvent) {
	if s.observer.OnEvent != nil {
		s.observer.OnEvent(event)
	}
}

func (s *openAIResponsesStreamState) response() (*Response, error) {
	if s.failureCode != "" {
		return nil, fmt.Errorf("%w: responses stream error %s", ErrRequestFailed, s.failureCode)
	}
	indexes := make([]int, 0, len(s.toolCalls))
	for index := range s.toolCalls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	toolCalls := make([]ToolCall, 0, len(indexes))
	for _, index := range indexes {
		call := s.toolCalls[index]
		toolCalls = append(toolCalls, ToolCall{ID: firstNonEmpty(call.id, fmt.Sprintf("tool_index_%d", index)), Name: call.name, Input: call.arguments.String()})
	}
	result := &Response{Content: s.content.String(), Thinking: s.reasoning.String(), ToolCalls: toolCalls, Usage: s.usage, IsFinished: len(toolCalls) == 0 && (s.stopReason == "completed" || s.stopReason == ""), StopReason: s.stopReason, MaxOutputReached: isMaxOutputStopReason(s.stopReason)}
	if !responseHasSignal(result) {
		return nil, ErrEmptyResponse
	}
	return result, nil
}
