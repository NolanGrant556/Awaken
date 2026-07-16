package llm

import (
	"bufio"
	"bytes"
	stdctx "context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"awaken/src/context"
	"awaken/src/tool"
)

// anthropicTransport 实现 Anthropic Messages 协议
type anthropicTransport struct{}

func newAnthropicTransport() *anthropicTransport {
	return &anthropicTransport{}
}

// --- 请求结构 ---

type anthropicRequest struct {
	Model     string             `json:"model"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	MaxTokens int                `json:"max_tokens"`
	Stream    bool               `json:"stream,omitempty"`
}

type anthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type anthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// --- 响应结构 ---

type anthropicResponse struct {
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason"`
	Usage      *anthropicUsage         `json:"usage,omitempty"`
}

type anthropicContentBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	Thinking string          `json:"thinking,omitempty"`
	ID       string          `json:"id,omitempty"`
	Name     string          `json:"name,omitempty"`
	Input    json.RawMessage `json:"input,omitempty"`
}

// --- Stream 结构 ---

type anthropicStreamEvent struct {
	Type         string                 `json:"type"`
	Index        int                    `json:"index,omitempty"`
	ContentBlock *anthropicContentBlock `json:"content_block,omitempty"`
	Delta        *anthropicStreamDelta  `json:"delta,omitempty"`
}

type anthropicStreamDelta struct {
	Type        string `json:"type,omitempty"`
	Text        string `json:"text,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
}

// --- 实现 Transport 接口 ---

func buildAnthropicMessagesURL(baseURL string) string {
	baseURL = strings.TrimSuffix(baseURL, "/")
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + "/messages"
	}
	return baseURL + "/v1/messages"
}

func (t *anthropicTransport) Invoke(ctx stdctx.Context, cfg Config, req Request) (*Response, error) {
	url := buildAnthropicMessagesURL(cfg.BaseURL)
	messages, err := buildAnthropicMessages(req)
	if err != nil {
		return nil, err
	}

	anthropicReq := anthropicRequest{
		Model:     resolveModel(cfg, req),
		System:    req.SystemPrompt,
		Messages:  messages,
		Tools:     convertToolsToAnthropic(req.Tools),
		MaxTokens: resolveMaxTokens(cfg, req),
	}

	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, requestBuildError(err)
	}

	applyAuthHeaders(httpReq, cfg, ProtocolAnthropicMessages)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, transportError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, requestFailedError(resp)
	}

	var anthropicResp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return responseFromAnthropic(anthropicResp)
}

func (t *anthropicTransport) StreamInvoke(ctx stdctx.Context, cfg Config, req Request, callback StreamCallback) (*Response, error) {
	return t.StreamInvokeWithObserver(ctx, cfg, req, StreamObserver{OnContent: callback})
}

func (t *anthropicTransport) StreamInvokeWithObserver(ctx stdctx.Context, cfg Config, req Request, observer StreamObserver) (*Response, error) {
	url := buildAnthropicMessagesURL(cfg.BaseURL)
	messages, err := buildAnthropicMessages(req)
	if err != nil {
		return nil, err
	}

	anthropicReq := anthropicRequest{
		Model:     resolveModel(cfg, req),
		System:    req.SystemPrompt,
		Messages:  messages,
		Tools:     convertToolsToAnthropic(req.Tools),
		MaxTokens: resolveMaxTokens(cfg, req),
		Stream:    true,
	}

	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	streamCtx, cancel := stdctx.WithCancel(ctx)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, requestBuildError(err)
	}

	applyAuthHeaders(httpReq, cfg, ProtocolAnthropicMessages)

	resp, err := streamHTTPClient.Do(httpReq)
	if err != nil {
		return nil, transportError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, requestFailedError(resp)
	}

	state := newAnthropicStreamState(observer)
	scanner := bufio.NewScanner(resp.Body)
	currentEvent := ""
	watchdog := newStreamIdleWatchdog(cancel, streamIdleTimeoutForRequest(req))
	defer watchdog.Stop()

	for scanner.Scan() {
		watchdog.Reset()
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}

		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" || data == "[DONE]" {
				continue
			}
			state.consume(data, currentEvent)
		}
	}

	if err := scanner.Err(); err != nil {
		if watchdog.TimedOut() {
			return nil, streamIdleTimeoutError(watchdog.Timeout())
		}
		return nil, fmt.Errorf("failed to read stream: %w", err)
	}

	return state.response()
}

// --- 消息转换 ---

func buildAnthropicMessages(req Request) ([]anthropicMessage, error) {
	var messages []anthropicMessage

	if len(req.Messages) > 0 {
		if err := context.ValidateToolResultPairing(req.Messages); err != nil {
			return nil, fmt.Errorf("invalid Anthropic tool result pairing: %w", err)
		}

		i := 0
		for i < len(req.Messages) {
			msg := req.Messages[i]

			if msg.Role == context.RoleAssistant && len(msg.ToolCalls) > 0 {
				messages = append(messages, anthropicMessage{
					Role:    "assistant",
					Content: buildAssistantContentBlocks(msg),
				})

				var toolResults []map[string]interface{}
				i++
				for i < len(req.Messages) && req.Messages[i].Role == context.RoleTool {
					tool := req.Messages[i]
					block := map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": tool.ToolCallID,
						"content":     anthropicToolResultContent(tool),
					}
					if tool.IsError {
						block["is_error"] = true
					}
					toolResults = append(toolResults, block)
					i++
				}

				if len(toolResults) > 0 {
					messages = append(messages, anthropicMessage{
						Role:    "user",
						Content: toolResults,
					})
				}
			} else {
				converted, err := convertContextMessageToAnthropic(msg)
				if err != nil {
					return nil, err
				}
				messages = append(messages, converted)
				i++
			}
		}
	} else if req.UserPrompt != "" {
		messages = append(messages, anthropicMessage{
			Role:    "user",
			Content: req.UserPrompt,
		})
	}

	return messages, nil
}

func anthropicToolResultContent(message context.Message) interface{} {
	if len(message.ModelContent) == 0 {
		return message.Content
	}
	blocks := make([]map[string]interface{}, 0, len(message.ModelContent)+1)
	if strings.TrimSpace(message.Content) != "" {
		blocks = append(blocks, map[string]interface{}{"type": "text", "text": message.Content})
	}
	for _, part := range message.ModelContent {
		if strings.TrimSpace(part.DataBase64) == "" || strings.TrimSpace(part.MimeType) == "" {
			continue
		}
		source := map[string]interface{}{"type": "base64", "media_type": part.MimeType, "data": part.DataBase64}
		switch part.Type {
		case "image":
			blocks = append(blocks, map[string]interface{}{"type": "image", "source": source})
		case "file":
			if part.MimeType == "application/pdf" {
				blocks = append(blocks, map[string]interface{}{"type": "document", "source": source})
			}
		}
	}
	if len(blocks) == 0 {
		return message.Content
	}
	return blocks
}

func buildAssistantContentBlocks(msg context.Message) []map[string]interface{} {
	blocks := make([]map[string]interface{}, 0, len(msg.ToolCalls)+1)
	if msg.Content != "" {
		blocks = append(blocks, map[string]interface{}{
			"type": "text",
			"text": msg.Content,
		})
	}
	for _, tc := range msg.ToolCalls {
		blocks = append(blocks, map[string]interface{}{
			"type":  "tool_use",
			"id":    tc.ID,
			"name":  tc.Function.Name,
			"input": parseToolUseInput(tc.Function.Arguments),
		})
	}
	return blocks
}

func parseToolUseInput(arguments string) map[string]interface{} {
	var input map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil || input == nil {
		return map[string]interface{}{}
	}
	return input
}

func convertContextMessageToAnthropic(msg context.Message) (anthropicMessage, error) {
	switch msg.Role {
	case context.RoleUser:
		return anthropicMessage{
			Role:    "user",
			Content: anthropicUserContent(msg),
		}, nil

	case context.RoleAssistant:
		if len(msg.ToolCalls) > 0 {
			return anthropicMessage{}, fmt.Errorf("assistant tool calls must be converted by buildAnthropicMessages")
		}
		return anthropicMessage{
			Role:    "assistant",
			Content: msg.Content,
		}, nil

	case context.RoleTool:
		return anthropicMessage{}, fmt.Errorf("tool results must follow assistant tool calls and be merged by buildAnthropicMessages")

	default:
		return anthropicMessage{
			Role:    string(msg.Role),
			Content: msg.Content,
		}, nil
	}
}

func anthropicUserContent(msg context.Message) interface{} {
	if len(msg.Attachments) == 0 {
		return msg.Content
	}

	blocks := make([]map[string]interface{}, 0, len(msg.Attachments)+1)
	userText := userTextWithAttachmentPaths(msg.Content, msg.Attachments)
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
			"type": "image",
			"source": map[string]interface{}{
				"type":       "base64",
				"media_type": attachment.MimeType,
				"data":       attachment.DataBase64,
			},
		})
	}
	if len(blocks) == 0 {
		return msg.Content
	}
	return blocks
}

// --- 工具转换 ---

func convertToolsToAnthropic(tools []tool.ToolSchema) []anthropicTool {
	if len(tools) == 0 {
		return nil
	}

	result := make([]anthropicTool, 0, len(tools))
	for _, t := range tools {
		schema := tool.SchemaAsMap(t.Function.Parameters)
		if _, ok := schema["type"]; !ok {
			schema["type"] = "object"
		}
		result = append(result, anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: schema,
		})
	}
	return result
}

// --- 响应解析 ---

func responseFromAnthropic(resp anthropicResponse) (*Response, error) {
	stopReason := strings.TrimSpace(resp.StopReason)
	result := &Response{
		IsFinished:       stopReason != "tool_use",
		StopReason:       stopReason,
		MaxOutputReached: isMaxOutputStopReason(stopReason),
		Usage:            anthropicResponseUsage(resp.Usage),
	}

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			result.Content += block.Text
		case "thinking":
			result.Thinking += block.Thinking
		case "tool_use":
			inputJSON, _ := json.Marshal(block.Input)
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:    block.ID,
				Name:  block.Name,
				Input: string(inputJSON),
			})
			result.IsFinished = false
		}
	}

	if !responseHasSignal(result) {
		return nil, ErrEmptyResponse
	}

	return result, nil
}

// --- Anthropic Stream State ---

type anthropicStreamState struct {
	observer    StreamObserver
	content     strings.Builder
	thinking    strings.Builder
	toolCalls   []ToolCall
	currentTool *anthropicToolAccumulator
	usage       *Usage
	stopReason  string
}

type anthropicToolAccumulator struct {
	id        string
	index     int
	name      string
	arguments strings.Builder
}

func newAnthropicStreamState(observer StreamObserver) *anthropicStreamState {
	return &anthropicStreamState{
		observer:  observer,
		toolCalls: make([]ToolCall, 0),
	}
}

func (s *anthropicStreamState) consume(data string, event string) {
	var streamEvent anthropicStreamEvent
	if err := json.Unmarshal([]byte(data), &streamEvent); err != nil {
		return
	}

	switch streamEvent.Type {
	case "message_start":
		if streamEvent.ContentBlock == nil {
			var envelope struct {
				Message struct {
					Usage *anthropicUsage `json:"usage,omitempty"`
				} `json:"message"`
			}
			if err := json.Unmarshal([]byte(data), &envelope); err == nil {
				s.usage = anthropicResponseUsage(envelope.Message.Usage)
			}
		}

	case "message_delta":
		if streamEvent.Delta != nil && streamEvent.Delta.StopReason != "" {
			s.stopReason = strings.TrimSpace(streamEvent.Delta.StopReason)
		}

	case "content_block_start":
		if streamEvent.ContentBlock != nil && streamEvent.ContentBlock.Type == "tool_use" {
			s.currentTool = &anthropicToolAccumulator{
				id:    streamEvent.ContentBlock.ID,
				index: streamEvent.Index,
				name:  streamEvent.ContentBlock.Name,
			}
		}

	case "content_block_delta":
		if streamEvent.Delta == nil {
			return
		}
		switch streamEvent.Delta.Type {
		case "thinking_delta":
			if streamEvent.Delta.Thinking != "" {
				s.thinking.WriteString(streamEvent.Delta.Thinking)
				s.emit(streamEvent.Delta.Thinking, true)
			}
		case "text_delta":
			if streamEvent.Delta.Text != "" {
				s.content.WriteString(streamEvent.Delta.Text)
				s.emit(streamEvent.Delta.Text, false)
			}
		case "input_json_delta":
			if s.currentTool != nil && streamEvent.Delta.PartialJSON != "" {
				s.currentTool.arguments.WriteString(streamEvent.Delta.PartialJSON)
				s.emitToolCallDelta(ToolCallDelta{
					Index:          streamEvent.Index,
					ID:             s.currentTool.id,
					Name:           s.currentTool.name,
					ArgumentsDelta: streamEvent.Delta.PartialJSON,
				})
			}
		}

	case "content_block_stop":
		if s.currentTool != nil {
			id := s.currentTool.id
			if id == "" {
				id = fmt.Sprintf("tool_index_%d", s.currentTool.index)
			}
			s.toolCalls = append(s.toolCalls, ToolCall{
				ID:    id,
				Name:  s.currentTool.name,
				Input: s.currentTool.arguments.String(),
			})
			s.currentTool = nil
		}
	}
}

func (s *anthropicStreamState) emit(content string, isThinking bool) {
	if s.observer.OnContent != nil {
		s.observer.OnContent(content, isThinking)
	}
}

func (s *anthropicStreamState) emitToolCallDelta(delta ToolCallDelta) {
	if s.observer.OnToolCallDelta != nil {
		s.observer.OnToolCallDelta(delta)
	}
}

func (s *anthropicStreamState) response() (*Response, error) {
	result := &Response{
		Content:          s.content.String(),
		Thinking:         s.thinking.String(),
		ToolCalls:        s.toolCalls,
		Usage:            s.usage,
		IsFinished:       len(s.toolCalls) == 0,
		StopReason:       s.stopReason,
		MaxOutputReached: isMaxOutputStopReason(s.stopReason),
	}
	if !responseHasSignal(result) {
		return nil, ErrEmptyResponse
	}
	return result, nil
}

func anthropicResponseUsage(raw *anthropicUsage) *Usage {
	if raw == nil {
		return nil
	}
	total := raw.InputTokens + raw.OutputTokens
	if raw.InputTokens == 0 && raw.OutputTokens == 0 {
		return nil
	}
	return &Usage{
		InputTokens:  raw.InputTokens,
		OutputTokens: raw.OutputTokens,
		TotalTokens:  total,
	}
}
