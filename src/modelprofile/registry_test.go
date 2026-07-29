package modelprofile

import "testing"

func TestBuiltinProvidersIncludeRequiredDefaults(t *testing.T) {
	providers := BuiltinProviders()
	want := []string{
		"deepseek", "kimi-cn", "kimi-coding-plan", "volcengine", "qwen-token-plan",
		"minimax", "glm", "qwen", "doubao", "volcengine-agent-plan",
	}
	if len(providers) != len(want) {
		t.Fatalf("expected %d providers, got %d", len(want), len(providers))
	}

	seen := map[string]Provider{}
	for _, provider := range providers {
		seen[provider.ID] = provider
	}
	for _, id := range want {
		provider, ok := seen[id]
		if !ok {
			t.Fatalf("missing provider %q", id)
		}
		if provider.DefaultModel == "" {
			t.Fatalf("provider %q is missing default model", id)
		}
		if provider.BaseURL == "" {
			t.Fatalf("provider %q is missing base URL", id)
		}
		for modelID, capability := range provider.ModelCapabilities {
			if capability.SupportsTools != "ready" {
				t.Fatalf("provider %q model %q must default to tools ready, got %q", id, modelID, capability.SupportsTools)
			}
		}
	}
	kimiCode := seen["kimi-coding-plan"]
	if kimiCode.BaseURL != "https://api.kimi.com/coding/v1" {
		t.Fatalf("expected Kimi Code base URL, got %q", kimiCode.BaseURL)
	}
	if kimiCode.DefaultModel != "k3" {
		t.Fatalf("expected k3 default model, got %q", kimiCode.DefaultModel)
	}
	wantKimiModels := []string{"k3", "k3-256k", "kimi-for-coding", "kimi-for-coding-highspeed"}
	if len(kimiCode.Models) != len(wantKimiModels) {
		t.Fatalf("expected Kimi Code models %v, got %v", wantKimiModels, kimiCode.Models)
	}
	for index, model := range wantKimiModels {
		if kimiCode.Models[index] != model {
			t.Fatalf("expected Kimi Code models %v, got %v", wantKimiModels, kimiCode.Models)
		}
	}
	if got := kimiCode.ModelCapabilities["k3"].ContextWindow; got != 1024*1024 {
		t.Fatalf("expected K3 1048576 context metadata, got %d", got)
	}
	if got := kimiCode.ModelCapabilities["kimi-for-coding"].MaxOutputTokens; got != 32*1024 {
		t.Fatalf("expected Kimi Code max output metadata, got %d", got)
	}
	if got := kimiCode.ModelCapabilities["kimi-for-coding-highspeed"].ContextWindow; got != 256*1024 {
		t.Fatalf("expected Kimi Code HighSpeed context metadata, got %d", got)
	}
	if got := kimiCode.ModelCapabilities["kimi-for-coding"].DefaultTemperature; got != 1 {
		t.Fatalf("expected Kimi Code default temperature 1, got %v", got)
	}

	volc := seen["volcengine"]
	if volc.DisplayName != "火山 Coding Plan" {
		t.Fatalf("unexpected Volcengine display name %q", volc.DisplayName)
	}
	if volc.BaseURL != "https://ark.cn-beijing.volces.com/api/coding/v3" {
		t.Fatalf("expected Volcengine Coding Plan base URL, got %q", volc.BaseURL)
	}
	if volc.DefaultModel != "ark-code-latest" {
		t.Fatalf("expected ark-code-latest default model, got %q", volc.DefaultModel)
	}
	if !volc.AllowCustomModel {
		t.Fatal("expected Volcengine Coding Plan to allow custom model IDs")
	}
	if got := volc.ModelCapabilities["doubao-seed-2.0-pro"].MaxOutputTokens; got != 128000 {
		t.Fatalf("expected Doubao Seed 2.0 Pro output metadata, got %d", got)
	}
	if got := volc.ModelCapabilities["glm-5.2"].DefaultMaxTokens; got != 32000 {
		t.Fatalf("expected GLM 5.2 default output metadata, got %d", got)
	}
	if got := volc.ModelCapabilities["deepseek-v4-pro"].ContextWindow; got != 128000 {
		t.Fatalf("expected DeepSeek V4 Pro context metadata, got %d", got)
	}
}

func TestProviderByID(t *testing.T) {
	provider, ok := ProviderByID("volcengine")
	if !ok {
		t.Fatal("expected volcengine provider")
	}
	if provider.DisplayName != "火山 Coding Plan" {
		t.Fatalf("unexpected display name %q", provider.DisplayName)
	}
	if provider, ok := ProviderByID("doubao"); !ok || !provider.EndpointModel {
		t.Fatal("doubao must resolve as an endpoint-model provider")
	}
}
