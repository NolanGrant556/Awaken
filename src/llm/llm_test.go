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
	os.Unsetenv("LLM_PROTOCOL")
	os.Unsetenv("LLM_AUTH_SCHEME")

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
	os.Setenv("LLM_PROTOCOL", "openai_chat")
	os.Setenv("LLM_AUTH_SCHEME", "bearer")
	defer func() {
		os.Unsetenv("LLM_API_KEY")
		os.Unsetenv("LLM_BASE_URL")
		os.Unsetenv("LLM_MODEL_ID")
		os.Unsetenv("LLM_PROTOCOL")
		os.Unsetenv("LLM_AUTH_SCHEME")
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
	if cfg.Protocol != ProtocolOpenAIChat {
		t.Errorf("expected openai_chat protocol, got %q", cfg.Protocol)
	}
	if cfg.AuthScheme != "bearer" {
		t.Errorf("expected bearer auth scheme, got %q", cfg.AuthScheme)
	}
}

func TestLoadConfigUsesConfigTOMLForNonSecrets(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Fatalf("failed to restore cwd: %v", err)
		}
	})

	if err := os.WriteFile("config.toml", []byte(`
[llm]
base_url = "https://config.example.com/anthropic"
model_id = "config-model"
protocol = "anthropic_messages"
auth_scheme = "x-api-key"

[model]
temperature = 0.3
max_output_tokens = 8192
`), 0644); err != nil {
		t.Fatalf("failed to write config.toml: %v", err)
	}

	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_BASE_URL", "https://legacy.example.com/v1")
	t.Setenv("LLM_MODEL_ID", "legacy-model")
	t.Setenv("LLM_PROTOCOL", "openai_chat")
	t.Setenv("LLM_AUTH_SCHEME", "bearer")

	cfg := LoadConfig()

	if cfg.APIKey != "test-key" {
		t.Fatalf("expected API key from env, got %q", cfg.APIKey)
	}
	if cfg.BaseURL != "https://config.example.com/anthropic" {
		t.Fatalf("expected base URL from config.toml, got %q", cfg.BaseURL)
	}
	if cfg.Model != "config-model" {
		t.Fatalf("expected model from config.toml, got %q", cfg.Model)
	}
	if cfg.Protocol != ProtocolAnthropicMessages {
		t.Fatalf("expected protocol from config.toml, got %q", cfg.Protocol)
	}
	if cfg.AuthScheme != "x-api-key" {
		t.Fatalf("expected auth scheme from config.toml, got %q", cfg.AuthScheme)
	}
	if cfg.Temperature != 0.3 {
		t.Fatalf("expected temperature from config.toml, got %v", cfg.Temperature)
	}
	if cfg.MaxTokens != 8192 {
		t.Fatalf("expected max output tokens from config.toml, got %d", cfg.MaxTokens)
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
