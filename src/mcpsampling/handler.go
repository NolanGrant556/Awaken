package mcpsampling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	defaultMaxRPM        = 10
	defaultTimeout       = 30 * time.Second
	defaultMaxTokensCap  = 4096
	defaultMaxToolRounds = 5
)

type Handler struct {
	cfg Config
	llm LLMClient

	mu             sync.Mutex
	rateTimestamps []time.Time
	toolLoopCount  int
	metrics        Metrics
}

func NewHandler(cfg Config, client LLMClient) *Handler {
	if cfg.MaxRPM <= 0 {
		cfg.MaxRPM = defaultMaxRPM
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.MaxTokensCap <= 0 {
		cfg.MaxTokensCap = defaultMaxTokensCap
	}
	if cfg.DisableToolLoops {
		cfg.MaxToolRounds = 0
	} else if cfg.MaxToolRounds <= 0 {
		cfg.MaxToolRounds = defaultMaxToolRounds
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Handler{cfg: cfg, llm: client}
}

func (h *Handler) Handle(ctx context.Context, req Request) (*Result, *ErrorData) {
	if h.llm == nil {
		return nil, h.recordError(ErrorCodeInvalidRequest, "sampling LLM client is not configured")
	}
	if !h.checkRateLimit() {
		h.incrementRateLimited()
		return nil, h.recordError(
			ErrorCodeRateLimited,
			fmt.Sprintf("Sampling rate limit exceeded for server %q (%d requests/minute)", h.cfg.ServerName, h.cfg.MaxRPM),
		)
	}

	model := h.resolveModel(req.ModelPreferences)
	if !h.modelAllowed(model) {
		return nil, h.recordError(
			ErrorCodeModelNotAllowed,
			fmt.Sprintf("Model %q is not allowed for server %q", model, h.cfg.ServerName),
		)
	}

	llmReq := LLMRequest{
		Model:       model,
		Messages:    h.convertMessages(req),
		Tools:       convertTools(req.Tools),
		MaxTokens:   h.applyMaxTokenCap(req.MaxTokens),
		Temperature: req.Temperature,
	}

	callCtx, cancel := context.WithTimeout(ctx, h.cfg.Timeout)
	defer cancel()

	resp, err := h.callLLM(callCtx, llmReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return nil, h.recordError(
				ErrorCodeTimeout,
				fmt.Sprintf("Sampling LLM call timed out after %s for server %q", h.cfg.Timeout, h.cfg.ServerName),
			)
		}
		if errors.Is(err, context.Canceled) || errors.Is(callCtx.Err(), context.Canceled) {
			return nil, h.recordError(ErrorCodeLLMFailure, "Sampling LLM call canceled")
		}
		return nil, h.recordError(ErrorCodeLLMFailure, "Sampling LLM call failed: "+sanitizeText(err.Error()))
	}
	if len(resp.Choices) == 0 {
		return nil, h.recordError(
			ErrorCodeEmptyResponse,
			fmt.Sprintf("LLM returned empty response for server %q", h.cfg.ServerName),
		)
	}

	choice := resp.Choices[0]
	if len(choice.Message.ToolCalls) > 0 {
		result, errData := h.buildToolUseResult(choice, resp)
		if errData != nil {
			return nil, errData
		}
		h.recordSuccess(resp.Usage.TotalTokens)
		return result, nil
	}
	result := h.buildTextResult(choice, resp)
	h.recordSuccess(resp.Usage.TotalTokens)
	return result, nil
}

func (h *Handler) Metrics() Metrics {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.metrics
}

func (h *Handler) callLLM(ctx context.Context, req LLMRequest) (LLMResponse, error) {
	type llmCallResult struct {
		resp LLMResponse
		err  error
	}
	done := make(chan llmCallResult, 1)
	go func() {
		resp, err := h.llm.CreateMessage(ctx, req)
		done <- llmCallResult{resp: resp, err: err}
	}()

	select {
	case result := <-done:
		return result.resp, result.err
	case <-ctx.Done():
		return LLMResponse{}, ctx.Err()
	}
}

func (h *Handler) resolveModel(preferences ModelPreferences) string {
	if strings.TrimSpace(h.cfg.Model) != "" {
		return strings.TrimSpace(h.cfg.Model)
	}
	for _, hint := range preferences.Hints {
		if strings.TrimSpace(hint.Name) != "" {
			return strings.TrimSpace(hint.Name)
		}
	}
	return strings.TrimSpace(h.cfg.DefaultModel)
}

func (h *Handler) modelAllowed(model string) bool {
	if len(h.cfg.AllowedModels) == 0 {
		return true
	}
	for _, allowed := range h.cfg.AllowedModels {
		if strings.TrimSpace(allowed) == model {
			return true
		}
	}
	return false
}

func (h *Handler) applyMaxTokenCap(requested int) int {
	if requested <= 0 {
		return h.cfg.MaxTokensCap
	}
	if requested > h.cfg.MaxTokensCap {
		return h.cfg.MaxTokensCap
	}
	return requested
}

func (h *Handler) checkRateLimit() bool {
	now := h.cfg.Now()
	windowStart := now.Add(-time.Minute)

	h.mu.Lock()
	defer h.mu.Unlock()

	kept := h.rateTimestamps[:0]
	for _, ts := range h.rateTimestamps {
		if ts.After(windowStart) {
			kept = append(kept, ts)
		}
	}
	h.rateTimestamps = kept
	if len(h.rateTimestamps) >= h.cfg.MaxRPM {
		return false
	}
	h.rateTimestamps = append(h.rateTimestamps, now)
	return true
}

func (h *Handler) convertMessages(req Request) []LLMMessage {
	messages := make([]LLMMessage, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.SystemPrompt) != "" {
		messages = append(messages, LLMMessage{Role: RoleSystem, Content: req.SystemPrompt})
	}
	for _, msg := range req.Messages {
		messages = append(messages, convertMessage(msg)...)
	}
	return messages
}

func convertMessage(msg SamplingMessage) []LLMMessage {
	toolResults := make([]ContentBlock, 0)
	toolUses := make([]ContentBlock, 0)
	ordinary := make([]ContentBlock, 0)
	for _, block := range msg.Content {
		switch block.Type {
		case ContentToolResult:
			toolResults = append(toolResults, block)
		case ContentToolUse:
			toolUses = append(toolUses, block)
		default:
			ordinary = append(ordinary, block)
		}
	}

	result := make([]LLMMessage, 0, len(toolResults)+1)
	for _, block := range toolResults {
		result = append(result, LLMMessage{
			Role:       RoleTool,
			ToolCallID: block.ToolUseID,
			Name:       block.Name,
			Content:    sanitizeText(extractToolResultText(block)),
		})
	}
	if len(toolUses) > 0 {
		toolCalls := make([]LLMToolCall, 0, len(toolUses))
		for i, block := range toolUses {
			id := block.ID
			if id == "" {
				id = fmt.Sprintf("call_%d", i)
			}
			toolCalls = append(toolCalls, LLMToolCall{
				ID:   id,
				Type: "function",
				Function: LLMFunctionCall{
					Name:      block.Name,
					Arguments: marshalToolInput(block.Input),
				},
			})
		}
		llmMsg := LLMMessage{Role: msg.Role, ToolCalls: toolCalls}
		if text := joinTextBlocks(ordinary, false); text != "" {
			llmMsg.Content = text
		}
		result = append(result, llmMsg)
		return result
	}
	if len(ordinary) > 0 {
		result = append(result, ordinaryMessage(msg.Role, ordinary))
	}
	return result
}

func ordinaryMessage(role Role, blocks []ContentBlock) LLMMessage {
	if len(blocks) == 1 && blocks[0].Type == ContentText {
		return LLMMessage{Role: role, Content: blocks[0].Text}
	}

	parts := make([]LLMContentPart, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case ContentText:
			parts = append(parts, LLMContentPart{Type: "text", Text: block.Text})
		case ContentImage:
			parts = append(parts, LLMContentPart{
				Type: "image_url",
				ImageURL: &LLMImageURL{
					URL: fmt.Sprintf("data:%s;base64,%s", block.MIMEType, block.Data),
				},
			})
		}
	}
	return LLMMessage{Role: role, Content: parts}
}

func extractToolResultText(block ContentBlock) string {
	if len(block.Content) == 0 {
		return block.Text
	}
	return joinTextBlocks(block.Content, true)
}

func joinTextBlocks(blocks []ContentBlock, includeNested bool) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == ContentText && block.Text != "" {
			parts = append(parts, block.Text)
		}
		if includeNested && len(block.Content) > 0 {
			if nested := joinTextBlocks(block.Content, true); nested != "" {
				parts = append(parts, nested)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func marshalToolInput(input any) string {
	if input == nil {
		return "{}"
	}
	data, err := json.Marshal(input)
	if err != nil {
		return fmt.Sprintf(`{"_raw":%q}`, sanitizeText(fmt.Sprint(input)))
	}
	return string(data)
}

func convertTools(tools []Tool) []LLMTool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]LLMTool, 0, len(tools))
	for _, tool := range tools {
		parameters := tool.InputSchema
		if parameters == nil {
			parameters = map[string]any{"type": "object"}
		}
		result = append(result, LLMTool{
			Type: "function",
			Function: LLMFunctionTool{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  parameters,
			},
		})
	}
	return result
}

func (h *Handler) buildToolUseResult(choice LLMChoice, resp LLMResponse) (*Result, *ErrorData) {
	h.mu.Lock()
	if h.cfg.DisableToolLoops {
		h.metrics.Errors++
		h.toolLoopCount = 0
		h.mu.Unlock()
		return nil, &ErrorData{
			Code:    ErrorCodeToolLoopExceeded,
			Message: fmt.Sprintf("Tool loops disabled for server %q", h.cfg.ServerName),
		}
	}
	h.toolLoopCount++
	if h.toolLoopCount > h.cfg.MaxToolRounds {
		h.metrics.Errors++
		h.toolLoopCount = 0
		h.mu.Unlock()
		return nil, &ErrorData{
			Code:    ErrorCodeToolLoopExceeded,
			Message: fmt.Sprintf("Tool loop limit exceeded for server %q (max %d rounds)", h.cfg.ServerName, h.cfg.MaxToolRounds),
		}
	}
	h.metrics.ToolUseCount += int64(len(choice.Message.ToolCalls))
	h.mu.Unlock()

	content := make([]ContentBlock, 0, len(choice.Message.ToolCalls))
	for _, call := range choice.Message.ToolCalls {
		content = append(content, ContentBlock{
			Type:  ContentToolUse,
			ID:    call.ID,
			Name:  call.Function.Name,
			Input: parseToolArguments(call.Function.Arguments),
		})
	}
	return &Result{
		Role:       RoleAssistant,
		Content:    content,
		Model:      resp.Model,
		StopReason: StopReasonToolUse,
	}, nil
}

func (h *Handler) buildTextResult(choice LLMChoice, resp LLMResponse) *Result {
	h.mu.Lock()
	h.toolLoopCount = 0
	h.mu.Unlock()

	return &Result{
		Role: RoleAssistant,
		Content: []ContentBlock{
			{Type: ContentText, Text: sanitizeText(choice.Message.ContentString())},
		},
		Model:      resp.Model,
		StopReason: mapStopReason(choice.FinishReason),
	}
}

func parseToolArguments(args string) any {
	if strings.TrimSpace(args) == "" {
		return map[string]any{}
	}
	var parsed any
	if err := json.Unmarshal([]byte(args), &parsed); err != nil {
		return map[string]any{"_raw": sanitizeText(args)}
	}
	return parsed
}

func (m LLMMessage) ContentString() string {
	switch value := m.Content.(type) {
	case string:
		return value
	case nil:
		return ""
	default:
		return fmt.Sprint(value)
	}
}

func mapStopReason(reason string) StopReason {
	switch reason {
	case "length", "max_tokens":
		return StopReasonMaxTokens
	case "tool_calls", "tool_use":
		return StopReasonToolUse
	default:
		return StopReasonEndTurn
	}
}

func (h *Handler) recordSuccess(tokens int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.metrics.Requests++
	if tokens > 0 {
		h.metrics.TokensUsed += int64(tokens)
	}
}

func (h *Handler) recordError(code int, message string) *ErrorData {
	h.mu.Lock()
	h.metrics.Errors++
	h.mu.Unlock()
	return &ErrorData{Code: code, Message: sanitizeText(message)}
}

func (h *Handler) incrementRateLimited() {
	h.mu.Lock()
	h.metrics.RateLimited++
	h.mu.Unlock()
}
