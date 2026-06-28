package llm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	t.Setenv("AWAKEN_CONFIG_DIR", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_MODEL_ID", "")
	t.Setenv("LLM_PROTOCOL", "")
	t.Setenv("LLM_AUTH_SCHEME", "")

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
	t.Setenv("AWAKEN_CONFIG_DIR", "")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_BASE_URL", "https://custom.api.com/v1")
	t.Setenv("LLM_MODEL_ID", "test-model")
	t.Setenv("LLM_PROTOCOL", "openai_chat")
	t.Setenv("LLM_AUTH_SCHEME", "bearer")

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
	t.Setenv("AWAKEN_CONFIG_DIR", "")
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
	if cfg.ActiveProfile != DefaultProfileName {
		t.Fatalf("expected default active profile, got %q", cfg.ActiveProfile)
	}
	if len(cfg.Profiles) != 1 {
		t.Fatalf("expected synthesized default profile, got %d", len(cfg.Profiles))
	}
	if cfg.Profiles[0].Name != DefaultProfileName {
		t.Fatalf("expected synthesized profile name %q, got %q", DefaultProfileName, cfg.Profiles[0].Name)
	}
	if cfg.Profiles[0].Model != "config-model" {
		t.Fatalf("expected synthesized profile model config-model, got %q", cfg.Profiles[0].Model)
	}
}

func TestLoadConfigFromConfigDir(t *testing.T) {
	// Clear LLM env vars so the config dir files are the source of truth.
	// t.Setenv restores the original values (including unset) after the test,
	// so a real LLM_API_KEY from the parent process is not leaked away for
	// later smoke tests in the same process.
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_MODEL_ID", "")
	t.Setenv("LLM_PROTOCOL", "")
	t.Setenv("LLM_AUTH_SCHEME", "")

	tmpDir := t.TempDir()
	t.Setenv("AWAKEN_CONFIG_DIR", tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("LLM_API_KEY=test-key-from-dir\n"), 0644); err != nil {
		t.Fatalf("failed to write .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte("[llm]\nbase_url = \"https://example.com/v1\"\nmodel_id = \"test-model\"\n"), 0644); err != nil {
		t.Fatalf("failed to write config.toml: %v", err)
	}

	cfg := LoadConfig()

	if cfg.APIKey != "test-key-from-dir" {
		t.Errorf("expected APIKey from config dir .env, got %q", cfg.APIKey)
	}
	if cfg.BaseURL != "https://example.com/v1" {
		t.Errorf("expected BaseURL from config dir config.toml, got %q", cfg.BaseURL)
	}
	if cfg.Model != "test-model" {
		t.Errorf("expected Model from config dir config.toml, got %q", cfg.Model)
	}
}

func TestLoadConfigPreservesParentEnvWhenDotEnvEmpty(t *testing.T) {
	// Simulate a real key injected by the parent process.
	t.Setenv("LLM_API_KEY", "real-key")

	tmpDir := t.TempDir()
	t.Setenv("AWAKEN_CONFIG_DIR", tmpDir)

	// The .env template ships an empty LLM_API_KEY= placeholder; it must not
	// clobber the real key from the parent environment.
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("LLM_API_KEY=\n"), 0644); err != nil {
		t.Fatalf("failed to write .env: %v", err)
	}

	cfg := LoadConfig()

	if cfg.APIKey != "real-key" {
		t.Errorf("expected parent env key to be preserved, got %q", cfg.APIKey)
	}
}

func TestLoadConfigParsesProfilesAndEnvSelectedActiveProfile(t *testing.T) {
	t.Setenv("LLM_API_KEY", "global-key")
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-key")
	t.Setenv("LLM_ACTIVE_PROFILE", "deepseek")
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_MODEL_ID", "")
	t.Setenv("LLM_PROTOCOL", "")
	t.Setenv("LLM_AUTH_SCHEME", "")

	tmpDir := t.TempDir()
	t.Setenv("AWAKEN_CONFIG_DIR", tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(`
[llm]
active_profile = "openai"

[[llm.profiles]]
name = "openai"
base_url = "https://api.openai.com/v1"
model_id = "gpt-4.1"
protocol = "openai_chat"
auth_scheme = "bearer"

[[llm.profiles]]
name = "deepseek"
base_url = "https://api.deepseek.com/anthropic"
model_id = "deepseek-v4-pro"
protocol = "anthropic_messages"
auth_scheme = "auto"
api_key_env = "DEEPSEEK_API_KEY"
`), 0644); err != nil {
		t.Fatalf("failed to write config.toml: %v", err)
	}

	cfg := LoadConfig()

	if cfg.ActiveProfile != "deepseek" {
		t.Fatalf("expected env-selected active profile deepseek, got %q", cfg.ActiveProfile)
	}
	if cfg.APIKey != "deepseek-key" {
		t.Fatalf("expected API key from DEEPSEEK_API_KEY, got %q", cfg.APIKey)
	}
	if cfg.BaseURL != "https://api.deepseek.com/anthropic" {
		t.Fatalf("expected active profile base URL, got %q", cfg.BaseURL)
	}
	if cfg.Model != "deepseek-v4-pro" {
		t.Fatalf("expected active profile model, got %q", cfg.Model)
	}
	if cfg.Protocol != ProtocolAnthropicMessages {
		t.Fatalf("expected anthropic_messages protocol, got %q", cfg.Protocol)
	}
	if cfg.AuthScheme != "auto" {
		t.Fatalf("expected auth scheme auto, got %q", cfg.AuthScheme)
	}
	if len(cfg.Profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(cfg.Profiles))
	}
	if !cfg.Profiles[1].HasAPIKey {
		t.Fatalf("expected deepseek profile to report API key availability")
	}
}

func TestLoadConfigFallsBackToFirstProfileWhenRequestedActiveProfileMissing(t *testing.T) {
	t.Setenv("LLM_API_KEY", "fallback-key")
	t.Setenv("LLM_ACTIVE_PROFILE", "missing")
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_MODEL_ID", "")
	t.Setenv("LLM_PROTOCOL", "")
	t.Setenv("LLM_AUTH_SCHEME", "")

	tmpDir := t.TempDir()
	t.Setenv("AWAKEN_CONFIG_DIR", tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(`
[[llm.profiles]]
name = "openai"
base_url = "https://api.openai.com/v1"
model_id = "gpt-4o-mini"

[[llm.profiles]]
name = "anthropic"
base_url = "https://api.anthropic.com/v1"
model_id = "claude-sonnet"
protocol = "anthropic_messages"
`), 0644); err != nil {
		t.Fatalf("failed to write config.toml: %v", err)
	}

	cfg := LoadConfig()

	if cfg.ActiveProfile != "openai" {
		t.Fatalf("expected missing active profile to fall back to first profile, got %q", cfg.ActiveProfile)
	}
	if cfg.Model != "gpt-4o-mini" {
		t.Fatalf("expected first profile model gpt-4o-mini, got %q", cfg.Model)
	}
	if len(cfg.Profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(cfg.Profiles))
	}
	if !cfg.Profiles[0].HasAPIKey {
		t.Fatalf("expected fallback profile to inherit global API key")
	}
}

func TestLoadConfigDoesNotFallbackToGlobalKeyWhenProfileRequiresDedicatedEnv(t *testing.T) {
	t.Setenv("LLM_API_KEY", "global-key")
	t.Setenv("DEEPSEEK_API_KEY", "")
	t.Setenv("LLM_ACTIVE_PROFILE", "deepseek")
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_MODEL_ID", "")
	t.Setenv("LLM_PROTOCOL", "")
	t.Setenv("LLM_AUTH_SCHEME", "")

	tmpDir := t.TempDir()
	t.Setenv("AWAKEN_CONFIG_DIR", tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(`
[[llm.profiles]]
name = "deepseek"
base_url = "https://api.deepseek.com/anthropic"
model_id = "deepseek-v4-pro"
protocol = "anthropic_messages"
api_key_env = "DEEPSEEK_API_KEY"
`), 0644); err != nil {
		t.Fatalf("failed to write config.toml: %v", err)
	}

	cfg := LoadConfig()

	if cfg.APIKey != "" {
		t.Fatalf("expected missing dedicated env to keep API key empty, got %q", cfg.APIKey)
	}
	if len(cfg.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(cfg.Profiles))
	}
	if cfg.Profiles[0].HasAPIKey {
		t.Fatalf("expected profile to report missing API key when dedicated env is unset")
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
