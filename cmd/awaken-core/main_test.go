package main

import (
	"os"
	"testing"

	"awaken/src/llm"
	"awaken/src/runtime"
)

func TestResolveDefaultWorkspaceUsesEnv(t *testing.T) {
	t.Setenv("AWAKEN_DEFAULT_WORKSPACE", "/tmp/awaken-workspace")

	if got := resolveDefaultWorkspace(); got != "/tmp/awaken-workspace" {
		t.Fatalf("expected workspace from env, got %q", got)
	}
}

func TestResolveDefaultWorkspaceFallsBackToCwd(t *testing.T) {
	t.Setenv("AWAKEN_DEFAULT_WORKSPACE", "")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if got := resolveDefaultWorkspace(); got != cwd {
		t.Fatalf("expected workspace from cwd %q, got %q", cwd, got)
	}
}

func TestModelConfigFromProfileDoesNotInheritFileConfig(t *testing.T) {
	fileCfg := llm.Config{
		APIKey:                "file-key",
		BaseURL:               "https://file.example",
		Model:                 "file-model",
		Temperature:           0.7,
		MaxTokens:             8192,
		ContextWindow:         400000,
		AutoCompactTokenLimit: 300000,
	}
	profile := &runtime.ModelProfileConfig{
		APIKey:        "profile-key",
		BaseURL:       "https://profile.example",
		ModelID:       "profile-model",
		Protocol:      "openai_chat",
		AuthScheme:    "bearer",
		ContextWindow: 1000000,
	}

	fromFile := modelConfigFromLLMConfig(fileCfg)
	fromProfile := modelConfigFromProfile(profile)

	if fromFile.ContextWindow != 400000 {
		t.Fatalf("expected file fallback context window, got %+v", fromFile)
	}
	if fromProfile.APIKey != "profile-key" || fromProfile.Model != "profile-model" {
		t.Fatalf("expected profile model config, got %+v", fromProfile)
	}
	if fromProfile.ContextWindow != 1000000 {
		t.Fatalf("expected profile context window, got %+v", fromProfile)
	}
	if fromProfile.AutoCompactTokenLimit != 0 || fromProfile.Temperature != 0 || fromProfile.MaxTokens != 0 {
		t.Fatalf("profile config inherited file-only values: %+v", fromProfile)
	}
}
