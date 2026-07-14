package main

import (
	"context"
	"os"
	"testing"

	"awaken/src/mcpclient"
	"awaken/src/runtime"
	"awaken/src/tool"
)

func TestResolveDefaultWorkspaceUsesEnv(t *testing.T) {
	t.Setenv("AWAKEN_DEFAULT_WORKSPACE", "/tmp/awaken-workspace")

	if got := resolveDefaultWorkspace(); got != "/tmp/awaken-workspace" {
		t.Fatalf("expected workspace from env, got %q", got)
	}
}

func TestResolveDefaultWorkspaceHonorsExplicitEmptyEnv(t *testing.T) {
	// The Desktop host always sets the variable; an explicit empty value means
	// "no default workspace" and must not fall back to cwd or home.
	t.Setenv("AWAKEN_DEFAULT_WORKSPACE", "")

	if got := resolveDefaultWorkspace(); got != "" {
		t.Fatalf("expected empty workspace for explicit empty env, got %q", got)
	}
}

func TestResolveDefaultWorkspaceFallsBackToCwdWhenUnset(t *testing.T) {
	// t.Setenv records the original value for cleanup; unset afterwards to
	// simulate a standalone launch without the variable.
	t.Setenv("AWAKEN_DEFAULT_WORKSPACE", "placeholder")
	if err := os.Unsetenv("AWAKEN_DEFAULT_WORKSPACE"); err != nil {
		t.Fatalf("failed to unset env: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if got := resolveDefaultWorkspace(); got != cwd {
		t.Fatalf("expected workspace from cwd %q, got %q", cwd, got)
	}
}

func TestModelConfigFromProfileUsesOnlyProfile(t *testing.T) {
	profile := &runtime.ModelProfileConfig{
		APIKey:        "profile-key",
		BaseURL:       "https://profile.example",
		ModelID:       "profile-model",
		Protocol:      "openai_chat",
		AuthScheme:    "bearer",
		ContextWindow: 1000000,
	}

	got := modelConfigFromProfile(profile)
	if got.APIKey != "profile-key" || got.Model != "profile-model" {
		t.Fatalf("expected profile model config, got %+v", got)
	}
	if got.ContextWindow != 1000000 {
		t.Fatalf("expected profile context window, got %+v", got)
	}
	if got.AutoCompactTokenLimit != 0 || got.Temperature != 0 || got.MaxTokens != 0 {
		t.Fatalf("profile config inherited non-profile values: %+v", got)
	}
}

func TestMCPManagerAndConnectorsExistWithoutStaticConfig(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("AWAKEN_HOME", home)

	manager := buildMCPManager(workspace)
	if manager == nil {
		t.Fatal("expected empty MCP manager without static config")
	}

	registry := buildConnectorRegistry(manager, workspace)
	if registry == nil {
		t.Fatal("expected connector registry without static MCP config")
	}
	if got := len(registry.List()); got == 0 {
		t.Fatal("expected builtin connector definitions")
	}
}

func TestBuildConnectorRegistryRestoresPersistedBuiltinOwnership(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AWAKEN_HOME", home)
	disabled := false
	if err := mcpclient.SaveUserConfig(mcpclient.ConfigFile{Servers: map[string]mcpclient.McpServerConfig{
		"playwright": {
			Stdio:   &mcpclient.StdioTransportConfig{Command: "npx"},
			Enabled: &disabled,
			Provider: &mcpclient.ServerProviderManifest{
				ProviderID: "connector:playwright",
				Source:     tool.SourceConnector,
			},
		},
	}}); err != nil {
		t.Fatalf("save user config: %v", err)
	}

	manager := buildMCPManager("")
	t.Cleanup(func() { manager.Shutdown(context.Background()) })
	registry := buildConnectorRegistry(manager, "")
	status, err := registry.Status("playwright")
	if err != nil {
		t.Fatalf("playwright status: %v", err)
	}
	if len(status.ServerNames) != 1 || status.ServerNames[0] != "playwright" {
		t.Fatalf("persisted builtin owner was not wired into registry: %+v", status)
	}
	if err := registry.Disconnect(context.Background(), "playwright"); err != nil {
		t.Fatalf("disconnect restored builtin: %v", err)
	}
	userCfg, err := mcpclient.LoadUserConfig()
	if err != nil {
		t.Fatalf("load user config after disconnect: %v", err)
	}
	if _, ok := userCfg.Servers["playwright"]; ok {
		t.Fatal("disconnect did not remove restored builtin config")
	}
}
