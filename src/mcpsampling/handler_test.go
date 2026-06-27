package mcpsampling

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeLLMClient struct {
	mu        sync.Mutex
	requests  []LLMRequest
	responses []LLMResponse
	errors    []error
	fn        func(context.Context, LLMRequest) (LLMResponse, error)
}

func (f *fakeLLMClient) CreateMessage(ctx context.Context, req LLMRequest) (LLMResponse, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	index := len(f.requests) - 1
	f.mu.Unlock()

	if f.fn != nil {
		return f.fn(ctx, req)
	}
	if index < len(f.errors) && f.errors[index] != nil {
		return LLMResponse{}, f.errors[index]
	}
	if index < len(f.responses) {
		return f.responses[index], nil
	}
	return textResponse("test-model", "ok", "stop", 3), nil
}

func (f *fakeLLMClient) lastRequest(t *testing.T) LLMRequest {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		t.Fatal("expected LLM client to be called")
	}
	return f.requests[len(f.requests)-1]
}

func (f *fakeLLMClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func textResponse(model, text, finishReason string, tokens int) LLMResponse {
	return LLMResponse{
		Model: model,
		Choices: []LLMChoice{{
			Message:      LLMMessage{Role: RoleAssistant, Content: text},
			FinishReason: finishReason,
		}},
		Usage: LLMUsage{TotalTokens: tokens},
	}
}

func toolResponse(model string, calls ...LLMToolCall) LLMResponse {
	return LLMResponse{
		Model: model,
		Choices: []LLMChoice{{
			Message:      LLMMessage{Role: RoleAssistant, ToolCalls: calls},
			FinishReason: "tool_calls",
		}},
		Usage: LLMUsage{TotalTokens: 11},
	}
}

func TestHandleAppliesSlidingWindowRateLimit(t *testing.T) {
	now := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	client := &fakeLLMClient{}
	handler := NewHandler(Config{
		ServerName:   "filesystem",
		DefaultModel: "default-model",
		MaxRPM:       2,
		Now:          func() time.Time { return now },
	}, client)

	for i := 0; i < 2; i++ {
		if _, errData := handler.Handle(context.Background(), Request{}); errData != nil {
			t.Fatalf("request %d should pass rate limit: %+v", i, errData)
		}
	}
	if _, errData := handler.Handle(context.Background(), Request{}); errData == nil || errData.Code != ErrorCodeRateLimited {
		t.Fatalf("expected rate-limit ErrorData, got %+v", errData)
	}
	if client.callCount() != 2 {
		t.Fatalf("rate-limited request must not call LLM, got %d calls", client.callCount())
	}

	now = now.Add(61 * time.Second)
	if _, errData := handler.Handle(context.Background(), Request{}); errData != nil {
		t.Fatalf("request should pass after sliding window expires: %+v", errData)
	}

	metrics := handler.Metrics()
	if metrics.Requests != 3 || metrics.Errors != 1 || metrics.RateLimited != 1 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
}

func TestHandleResolvesModelOverrideBeforeHintAndAllowlist(t *testing.T) {
	client := &fakeLLMClient{}
	handler := NewHandler(Config{
		ServerName:    "analysis",
		Model:         "config-model",
		DefaultModel:  "default-model",
		AllowedModels: []string{"config-model"},
	}, client)

	if _, errData := handler.Handle(context.Background(), Request{
		ModelPreferences: ModelPreferences{Hints: []ModelHint{{Name: "server-hint"}}},
	}); errData != nil {
		t.Fatalf("expected config model to be allowed: %+v", errData)
	}

	req := client.lastRequest(t)
	if req.Model != "config-model" {
		t.Fatalf("expected config override model, got %q", req.Model)
	}

	blockedClient := &fakeLLMClient{}
	blocked := NewHandler(Config{
		ServerName:    "analysis",
		DefaultModel:  "default-model",
		AllowedModels: []string{"other-model"},
	}, blockedClient)
	if _, errData := blocked.Handle(context.Background(), Request{}); errData == nil || errData.Code != ErrorCodeModelNotAllowed {
		t.Fatalf("expected model allowlist error, got %+v", errData)
	}
	if blockedClient.callCount() != 0 {
		t.Fatalf("disallowed model must not call LLM")
	}
}

func TestHandleCapsMaxTokensAndPassesThroughTools(t *testing.T) {
	client := &fakeLLMClient{}
	handler := NewHandler(Config{
		ServerName:    "search",
		DefaultModel:  "default-model",
		MaxTokensCap:  128,
		MaxToolRounds: 2,
	}, client)

	if _, errData := handler.Handle(context.Background(), Request{
		MaxTokens: 999,
		Tools: []Tool{{
			Name:        "lookup",
			Description: "Lookup records",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
			},
		}},
	}); errData != nil {
		t.Fatalf("unexpected error: %+v", errData)
	}

	req := client.lastRequest(t)
	if req.MaxTokens != 128 {
		t.Fatalf("expected max_tokens cap 128, got %d", req.MaxTokens)
	}
	if len(req.Tools) != 1 || req.Tools[0].Type != "function" || req.Tools[0].Function.Name != "lookup" {
		t.Fatalf("unexpected tools conversion: %+v", req.Tools)
	}
	if req.Tools[0].Function.Parameters["type"] != "object" {
		t.Fatalf("expected input schema passthrough, got %+v", req.Tools[0].Function.Parameters)
	}
}

func TestHandleConvertsSamplingMessagesToLLMMessages(t *testing.T) {
	client := &fakeLLMClient{}
	handler := NewHandler(Config{ServerName: "browser", DefaultModel: "default-model"}, client)

	_, errData := handler.Handle(context.Background(), Request{
		SystemPrompt: "Be precise.",
		Messages: []SamplingMessage{
			{
				Role: RoleUser,
				Content: []ContentBlock{
					{Type: ContentText, Text: "describe this"},
					{Type: ContentImage, MIMEType: "image/png", Data: "aW1hZ2U="},
				},
			},
			{
				Role: RoleAssistant,
				Content: []ContentBlock{
					{Type: ContentText, Text: "I will call a tool."},
					{Type: ContentToolUse, ID: "call_1", Name: "lookup", Input: map[string]any{"query": "docs"}},
				},
			},
			{
				Role: RoleUser,
				Content: []ContentBlock{
					{
						Type:      ContentToolResult,
						ToolUseID: "call_1",
						Name:      "lookup",
						Content:   []ContentBlock{{Type: ContentText, Text: "found sk-test-secret"}},
					},
				},
			},
		},
	})
	if errData != nil {
		t.Fatalf("unexpected error: %+v", errData)
	}

	req := client.lastRequest(t)
	if len(req.Messages) != 4 {
		t.Fatalf("expected 4 LLM messages, got %+v", req.Messages)
	}
	if req.Messages[0].Role != RoleSystem || req.Messages[0].Content != "Be precise." {
		t.Fatalf("unexpected system prompt message: %+v", req.Messages[0])
	}
	parts, ok := req.Messages[1].Content.([]LLMContentPart)
	if !ok || len(parts) != 2 || parts[1].ImageURL == nil || !strings.HasPrefix(parts[1].ImageURL.URL, "data:image/png;base64,") {
		t.Fatalf("expected multimodal user content, got %#v", req.Messages[1].Content)
	}
	if len(req.Messages[2].ToolCalls) != 1 || req.Messages[2].ToolCalls[0].Function.Arguments != `{"query":"docs"}` {
		t.Fatalf("unexpected assistant tool call conversion: %+v", req.Messages[2])
	}
	if req.Messages[3].Role != RoleTool || req.Messages[3].ToolCallID != "call_1" {
		t.Fatalf("unexpected tool result conversion: %+v", req.Messages[3])
	}
	if content, _ := req.Messages[3].Content.(string); strings.Contains(content, "sk-test-secret") || !strings.Contains(content, "[REDACTED]") {
		t.Fatalf("expected sanitized tool result content, got %q", content)
	}
}

func TestHandleConvertsLLMToolCallsToMCPToolUse(t *testing.T) {
	client := &fakeLLMClient{responses: []LLMResponse{
		toolResponse("model-a", LLMToolCall{
			ID:   "call_1",
			Type: "function",
			Function: LLMFunctionCall{
				Name:      "lookup",
				Arguments: `{"query":"docs","limit":2}`,
			},
		}),
	}}
	handler := NewHandler(Config{ServerName: "search", DefaultModel: "model-a", MaxToolRounds: 2}, client)

	result, errData := handler.Handle(context.Background(), Request{})
	if errData != nil {
		t.Fatalf("unexpected error: %+v", errData)
	}
	if result.StopReason != StopReasonToolUse || len(result.Content) != 1 {
		t.Fatalf("expected toolUse result, got %+v", result)
	}
	block := result.Content[0]
	input, ok := block.Input.(map[string]any)
	if !ok || input["query"] != "docs" || input["limit"].(float64) != 2 {
		t.Fatalf("unexpected parsed tool input: %#v", block.Input)
	}

	metrics := handler.Metrics()
	if metrics.Requests != 1 || metrics.ToolUseCount != 1 || metrics.TokensUsed != 11 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
}

func TestHandleEnforcesToolLoopLimitAndResetsOnText(t *testing.T) {
	client := &fakeLLMClient{responses: []LLMResponse{
		toolResponse("model-a", LLMToolCall{ID: "call_1", Type: "function", Function: LLMFunctionCall{Name: "lookup", Arguments: `{}`}}),
		toolResponse("model-a", LLMToolCall{ID: "call_2", Type: "function", Function: LLMFunctionCall{Name: "lookup", Arguments: `{}`}}),
		textResponse("model-a", "done", "stop", 1),
		toolResponse("model-a", LLMToolCall{ID: "call_3", Type: "function", Function: LLMFunctionCall{Name: "lookup", Arguments: `{}`}}),
	}}
	handler := NewHandler(Config{ServerName: "search", DefaultModel: "model-a", MaxToolRounds: 1}, client)

	if _, errData := handler.Handle(context.Background(), Request{}); errData != nil {
		t.Fatalf("first tool round should pass: %+v", errData)
	}
	if _, errData := handler.Handle(context.Background(), Request{}); errData == nil || errData.Code != ErrorCodeToolLoopExceeded {
		t.Fatalf("expected tool-loop limit error, got %+v", errData)
	}
	if _, errData := handler.Handle(context.Background(), Request{}); errData != nil {
		t.Fatalf("text response should reset loop: %+v", errData)
	}
	if _, errData := handler.Handle(context.Background(), Request{}); errData != nil {
		t.Fatalf("tool round after text reset should pass: %+v", errData)
	}
}

func TestHandleCanDisableToolLoopsExplicitly(t *testing.T) {
	client := &fakeLLMClient{responses: []LLMResponse{
		toolResponse("model-a", LLMToolCall{ID: "call_1", Type: "function", Function: LLMFunctionCall{Name: "lookup", Arguments: `{}`}}),
	}}
	handler := NewHandler(Config{
		ServerName:       "search",
		DefaultModel:     "model-a",
		MaxToolRounds:    5,
		DisableToolLoops: true,
	}, client)

	_, errData := handler.Handle(context.Background(), Request{})
	if errData == nil || errData.Code != ErrorCodeToolLoopExceeded {
		t.Fatalf("expected disabled tool-loop error, got %+v", errData)
	}
	if !strings.Contains(errData.Message, "disabled") {
		t.Fatalf("expected disabled message, got %q", errData.Message)
	}

	metrics := handler.Metrics()
	if metrics.Requests != 0 || metrics.Errors != 1 || metrics.ToolUseCount != 0 {
		t.Fatalf("unexpected metrics after disabled tool loop: %+v", metrics)
	}
}

func TestHandleReturnsTimeoutErrorData(t *testing.T) {
	client := &fakeLLMClient{
		fn: func(ctx context.Context, req LLMRequest) (LLMResponse, error) {
			time.Sleep(50 * time.Millisecond)
			return textResponse("model-a", "late", "stop", 1), nil
		},
	}
	handler := NewHandler(Config{
		ServerName:   "slow",
		DefaultModel: "model-a",
		Timeout:      5 * time.Millisecond,
	}, client)

	_, errData := handler.Handle(context.Background(), Request{})
	if errData == nil || errData.Code != ErrorCodeTimeout {
		t.Fatalf("expected timeout ErrorData, got %+v", errData)
	}
}

func TestHandleReturnsSanitizedErrorsAndTextResults(t *testing.T) {
	client := &fakeLLMClient{
		responses: []LLMResponse{textResponse("model-a", "token=sk-test-secret finished", "length", 4)},
	}
	handler := NewHandler(Config{ServerName: "safe", DefaultModel: "model-a"}, client)

	result, errData := handler.Handle(context.Background(), Request{})
	if errData != nil {
		t.Fatalf("unexpected error: %+v", errData)
	}
	if result.StopReason != StopReasonMaxTokens {
		t.Fatalf("expected maxTokens stop reason, got %q", result.StopReason)
	}
	if strings.Contains(result.Content[0].Text, "sk-test-secret") || !strings.Contains(result.Content[0].Text, "[REDACTED]") {
		t.Fatalf("expected sanitized text result, got %q", result.Content[0].Text)
	}

	errorClient := &fakeLLMClient{errors: []error{errors.New("provider rejected api_key=sk-test-secret")}}
	errorHandler := NewHandler(Config{ServerName: "safe", DefaultModel: "model-a"}, errorClient)
	_, errData = errorHandler.Handle(context.Background(), Request{})
	if errData == nil || errData.Code != ErrorCodeLLMFailure {
		t.Fatalf("expected LLM failure, got %+v", errData)
	}
	if strings.Contains(errData.Message, "sk-test-secret") || !strings.Contains(errData.Message, "[REDACTED]") {
		t.Fatalf("expected sanitized ErrorData, got %q", errData.Message)
	}
}

func TestHandleEmptyChoicesReturnsErrorData(t *testing.T) {
	client := &fakeLLMClient{responses: []LLMResponse{{Model: "model-a"}}}
	handler := NewHandler(Config{ServerName: "empty", DefaultModel: "model-a"}, client)

	_, errData := handler.Handle(context.Background(), Request{})
	if errData == nil || errData.Code != ErrorCodeEmptyResponse {
		t.Fatalf("expected empty-response ErrorData, got %+v", errData)
	}
}
