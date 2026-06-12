package llm

import (
	"context"
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	os.Unsetenv("LLM_API_KEY")
	os.Unsetenv("LLM_BASE_URL")
	os.Unsetenv("LLM_MODEL_ID")

	cfg := LoadConfig()

	if cfg.APIKey != "" {
		t.Errorf("expected empty APIKey, got %q", cfg.APIKey)
	}
	if cfg.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("expected default base URL, got %q", cfg.BaseURL)
	}
	if cfg.Model != "" {
		t.Errorf("expected empty model, got %q", cfg.Model)
	}
}

func TestLoadConfigWithEnv(t *testing.T) {
	os.Setenv("LLM_API_KEY", "test-key")
	os.Setenv("LLM_BASE_URL", "https://custom.api.com/v1")
	os.Setenv("LLM_MODEL_ID", "test-model")
	defer func() {
		os.Unsetenv("LLM_API_KEY")
		os.Unsetenv("LLM_BASE_URL")
		os.Unsetenv("LLM_MODEL_ID")
	}()

	cfg := LoadConfig()

	if cfg.APIKey != "test-key" {
		t.Errorf("expected test-key, got %q", cfg.APIKey)
	}
	if cfg.BaseURL != "https://custom.api.com/v1" {
		t.Errorf("expected custom URL, got %q", cfg.BaseURL)
	}
	if cfg.Model != "test-model" {
		t.Errorf("expected test-model, got %q", cfg.Model)
	}
}

func TestCallMissingAPIKey(t *testing.T) {
	cfg := Config{
		APIKey:  "",
		BaseURL: "https://api.openai.com/v1",
		Model:   "test-model",
	}

	req := Request{
		UserPrompt: "Hello",
	}

	_, err := Call(context.Background(), cfg, req)
	if err != ErrMissingAPIKey {
		t.Errorf("expected ErrMissingAPIKey, got %v", err)
	}
}

func TestCallMissingModel(t *testing.T) {
	cfg := Config{
		APIKey:  "test-key",
		BaseURL: "https://api.openai.com/v1",
		Model:   "",
	}

	req := Request{
		UserPrompt: "Hello",
		Model:      "",
	}

	_, err := Call(context.Background(), cfg, req)
	if err != ErrMissingModel {
		t.Errorf("expected ErrMissingModel, got %v", err)
	}
}

func TestCallContextCancellation(t *testing.T) {
	cfg := Config{
		APIKey:  "test-key",
		BaseURL: "https://api.openai.com/v1",
		Model:   "test-model",
	}

	req := Request{
		UserPrompt: "Hello",
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Call(ctx, cfg, req)
	if err == nil {
		t.Error("expected error due to cancelled context")
	}
}

func TestStreamCallMissingAPIKey(t *testing.T) {
	cfg := Config{
		APIKey:  "",
		BaseURL: "https://api.openai.com/v1",
		Model:   "test-model",
	}

	req := Request{
		UserPrompt: "Hello",
		Stream:     true,
	}

	err := StreamCall(context.Background(), cfg, req, func(content string, isThinking bool) {})
	if err != ErrMissingAPIKey {
		t.Errorf("expected ErrMissingAPIKey, got %v", err)
	}
}

func TestStreamCallContextCancellation(t *testing.T) {
	cfg := Config{
		APIKey:  "test-key",
		BaseURL: "https://api.openai.com/v1",
		Model:   "test-model",
	}

	req := Request{
		UserPrompt: "Hello",
		Stream:     true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := StreamCall(ctx, cfg, req, func(content string, isThinking bool) {})
	if err == nil {
		t.Error("expected error due to cancelled context")
	}
}

func TestCallWithSmoke(t *testing.T) {
	apiKey := os.Getenv("LLM_API_KEY")
	model := os.Getenv("LLM_MODEL_ID")
	baseURL := os.Getenv("LLM_BASE_URL")

	if apiKey == "" || apiKey == "YOUR-API-KEY" {
		t.Skip("skipping smoke test: LLM_API_KEY not configured")
	}

	cfg := Config{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
	}

	req := Request{
		UserPrompt: "Say exactly: Hello, World!",
		Model:      model,
	}

	resp, err := Call(context.Background(), cfg, req)
	if err != nil {
		t.Fatalf("smoke test failed: %v", err)
	}

	if resp == "" {
		t.Error("smoke test got empty response")
	}

	t.Logf("smoke test response: %s", resp)
}

func TestStreamCallWithSmoke(t *testing.T) {
	apiKey := os.Getenv("LLM_API_KEY")
	model := os.Getenv("LLM_MODEL_ID")
	baseURL := os.Getenv("LLM_BASE_URL")

	if apiKey == "" || apiKey == "YOUR-API-KEY" {
		t.Skip("skipping stream smoke test: LLM_API_KEY not configured")
	}

	cfg := Config{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
	}

	req := Request{
		UserPrompt: "Say exactly: Hello, Stream!",
		Model:      model,
		Stream:     true,
	}

	var result string
	var thinkingContent string

	err := StreamCall(context.Background(), cfg, req, func(content string, isThinking bool) {
		if isThinking {
			thinkingContent += content
		} else {
			result += content
		}
	})

	if err != nil {
		t.Fatalf("stream smoke test failed: %v", err)
	}

	if result == "" {
		t.Error("stream smoke test got empty response")
	}

	t.Logf("stream result: %s", result)
	if thinkingContent != "" {
		t.Logf("thinking content: %s", thinkingContent)
	}
}
