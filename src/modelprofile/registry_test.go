package modelprofile

import (
	"reflect"
	"testing"
)

var expectedProtocols = map[string][]string{
	"openai":                []string{"openai_chat", "openai_responses"},
	"anthropic":             []string{"anthropic_messages"},
	"google":                []string{"openai_chat"},
	"custom":                []string{"openai_chat", "openai_responses", "anthropic_messages"},
	"deepseek":              []string{"openai_chat", "openai_responses", "anthropic_messages"},
	"kimi-cn":               []string{"openai_chat"},
	"kimi-coding-plan":      []string{"openai_chat", "anthropic_messages"},
	"minimax-token-plan":    []string{"openai_chat", "openai_responses", "anthropic_messages"},
	"glm-coding-plan":       []string{"openai_chat", "anthropic_messages"},
	"volcengine":            []string{"openai_chat", "anthropic_messages"},
	"qwen-token-plan":       []string{"openai_chat", "openai_responses", "anthropic_messages"},
	"minimax":               []string{"openai_chat", "openai_responses", "anthropic_messages"},
	"glm":                   []string{"openai_chat", "anthropic_messages"},
	"qwen":                  []string{"openai_chat", "openai_responses", "anthropic_messages"},
	"doubao":                []string{"openai_chat", "openai_responses"},
	"volcengine-agent-plan": []string{"openai_chat", "anthropic_messages"},
}

func TestBuiltinProvidersIncludeRequiredDefaults(t *testing.T) {
	providers := BuiltinProviders()
	want := []string{
		"openai", "anthropic", "google", "custom",
		"deepseek", "kimi-cn", "kimi-coding-plan", "minimax-token-plan", "glm-coding-plan",
		"volcengine", "qwen-token-plan", "minimax", "glm", "qwen", "doubao", "volcengine-agent-plan",
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
		if provider.DefaultModel == "" && provider.ID != "custom" {
			t.Fatalf("provider %q is missing default model", id)
		}
		if provider.BaseURL == "" && provider.ID != "custom" {
			t.Fatalf("provider %q is missing base URL", id)
		}
		if got := provider.SupportedProtocols; !reflect.DeepEqual(got, expectedProtocols[id]) {
			t.Fatalf("provider %q protocols mismatch: got %v want %v", id, got, expectedProtocols[id])
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
	google := seen["google"].ModelCapabilities["gemini-3.6-flash"]
	if !google.SupportsVision || google.SupportsNativePDF || google.SupportsVideo || google.SupportsPromptCache {
		t.Fatalf("Google OpenAI-compatible Chat capability must stay within its documented contract: %+v", google)
	}
	if got := google.SupportedProtocols; !reflect.DeepEqual(got, []string{"openai_chat"}) {
		t.Fatalf("Google model protocol contract mismatch: %v", got)
	}
	if got := google.ReasoningEffort; got == nil || !reflect.DeepEqual(got.Levels, []string{"low", "high"}) || got.DefaultLevel != "high" {
		t.Fatalf("Google reasoning effort contract mismatch: %+v", got)
	}
	if got := seen["glm"].ModelCapabilities["glm-5.3"].ReasoningEffort; got == nil || !reflect.DeepEqual(got.Levels, []string{"low", "high", "max"}) || got.DefaultLevel != "max" {
		t.Fatalf("GLM API reasoning effort contract mismatch: %+v", got)
	}
	if got := seen["glm"].ModelCapabilities["glm-5.2"].ReasoningEffort; got == nil || !reflect.DeepEqual(got.Levels, []string{"high", "max"}) || got.DefaultLevel != "max" {
		t.Fatalf("GLM 5.2 compatibility effort contract mismatch: %+v", got)
	}
	if got := seen["glm-coding-plan"].ModelCapabilities["glm-5.3"].ReasoningEffort; got == nil || !reflect.DeepEqual(got.Levels, []string{"low", "high", "max"}) || got.DefaultLevel != "max" {
		t.Fatalf("GLM Coding Plan reasoning effort contract mismatch: %+v", got)
	}
	if got := seen["deepseek"].ModelCapabilities["deepseek-v4-pro"].SupportedProtocols; !reflect.DeepEqual(got, allLLMProtocols()) {
		t.Fatalf("DeepSeek V4 Pro protocol contract mismatch: %v", got)
	}
	if got := seen["deepseek"].ModelCapabilities["deepseek-v4-flash"].SupportedProtocols; !reflect.DeepEqual(got, allLLMProtocols()) {
		t.Fatalf("DeepSeek V4 Flash protocol contract mismatch: %v", got)
	}
	if got := seen["qwen"].ModelCapabilities["qwen3.8-max"].SupportedProtocols; !reflect.DeepEqual(got, allLLMProtocols()) {
		t.Fatalf("Qwen model protocol contract mismatch: %v", got)
	}
	qwen37Plus := seen["qwen"].ModelCapabilities["qwen3.7-plus"]
	if got := qwen37Plus.SupportedProtocols; !reflect.DeepEqual(got, allLLMProtocols()) {
		t.Fatalf("Qwen 3.7 Plus protocol contract mismatch: %v", got)
	}
	if qwen37Plus.ContextWindow != 1000000 || qwen37Plus.MaxInputTokens != 983616 || qwen37Plus.MaxOutputTokens != 131072 {
		t.Fatalf("Qwen 3.7 Plus token contract mismatch: %+v", qwen37Plus)
	}
	if !qwen37Plus.SupportsVision || !qwen37Plus.SupportsVideo || qwen37Plus.SupportsTools != "ready" || qwen37Plus.ReasoningEffort != nil {
		t.Fatalf("Qwen 3.7 Plus capability contract mismatch: %+v", qwen37Plus)
	}
	if got := seen["qwen"].Models; !reflect.DeepEqual(got, []string{"qwen3.8-max", "qwen3.8-flash", "qwen3.7-plus"}) {
		t.Fatalf("Qwen built-in models mismatch: %v", got)
	}
	if seen["openai"].CatalogStrategy != "openai" {
		t.Fatalf("OpenAI must keep its live model catalog strategy, got %q", seen["openai"].CatalogStrategy)
	}
	if seen["custom"].CatalogStrategy != "none" {
		t.Fatalf("custom API models must be entered manually, got catalog strategy %q", seen["custom"].CatalogStrategy)
	}
	if seen["custom"].AccessType != "custom" {
		t.Fatalf("custom API must use its dedicated access type, got %q", seen["custom"].AccessType)
	}
	if got := seen["minimax-token-plan"].BaseURL; got != "https://api.minimaxi.com/v1" {
		t.Fatalf("MiniMax Token Plan must use the China endpoint, got %q", got)
	}
	if got := seen["minimax"].BaseURL; got != "https://api.minimaxi.com/v1" {
		t.Fatalf("MiniMax API must use the China endpoint, got %q", got)
	}
	if !seen["qwen-token-plan"].AllowCustomModel {
		t.Fatal("Qwen Token Plan must retain manual model ID fallback")
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
	if got := volc.ModelCapabilities["glm-5.3"].ContextWindow; got != 1000000 {
		t.Fatalf("expected GLM 5.3 context metadata, got %d", got)
	}
	for _, retired := range []string{"glm-4.7", "glm-5.2", "kimi-k2.6", "deepseek-v3.2", "doubao-seed-code"} {
		for _, model := range volc.Models {
			if model == retired {
				t.Fatalf("Coding Plan picker still lists retired model %q", retired)
			}
		}
	}
	if got := volc.ModelCapabilities["deepseek-v4-pro"].ContextWindow; got != 128000 {
		t.Fatalf("expected DeepSeek V4 Pro context metadata, got %d", got)
	}
	if got := volc.ModelCapabilities["doubao-seed-evolving"].ContextWindow; got != 1024000 {
		t.Fatalf("expected Coding Plan Evolving context metadata, got %d", got)
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
		t.Fatal("doubao must resolve as a dual Model ID / Endpoint ID provider")
	} else {
		if provider.DisplayName != "火山方舟" {
			t.Fatalf("unexpected Doubao display name %q", provider.DisplayName)
		}
		if provider.DefaultModel != "doubao-seed-2-1-pro-260628" {
			t.Fatalf("unexpected Doubao default model %q", provider.DefaultModel)
		}
		if !reflect.DeepEqual(provider.Models, []string{
			"doubao-seed-2-1-pro-260628",
			"doubao-seed-2-1-turbo-260628",
			"doubao-seed-evolving",
			"doubao-seed-2-0-code-preview-260215",
			"doubao-seed-2-0-pro-260215",
			"doubao-seed-2-0-lite-260428",
			"doubao-seed-2-0-mini-260428",
			"deepseek-v4-pro-ga-260813",
			"deepseek-v4-flash-ga-260731",
			"glm-5-3-flash-260828",
			"glm-5-2-260617",
		}) {
			t.Fatalf("unexpected Doubao model manifest: %v", provider.Models)
		}
		if got := provider.ModelDisplayNames["doubao-seed-2-1-pro-260628"]; got != "Doubao-Seed-2.1-Pro" {
			t.Fatalf("unexpected Doubao model display name %q", got)
		}
		if got := provider.ModelDisplayNames["deepseek-v4-pro-ga-260813"]; got != "DeepSeek-V4-Pro" {
			t.Fatalf("unexpected Doubao hosted DeepSeek display name %q", got)
		}
		if got := provider.ModelDisplayNames["glm-5-3-flash-260828"]; got != "GLM-5.3-Flash" {
			t.Fatalf("unexpected Doubao hosted GLM-5.3-Flash display name %q", got)
		}
		if got := provider.ModelCapabilities["doubao-seed-evolving"].ContextWindow; got != 1024000 {
			t.Fatalf("expected Doubao Evolving context metadata, got %d", got)
		}
		if got := provider.ModelCapabilities["doubao-seed-evolving"].ReasoningEffort; got == nil ||
			!reflect.DeepEqual(got.Levels, []string{"low", "high", "max"}) ||
			got.DefaultLevel != "max" ||
			got.DisplayLabels["max"] != "high" {
			t.Fatalf("unexpected Doubao Evolving effort: %+v", got)
		}
		if got := provider.ModelCapabilities["doubao-seed-2-1-turbo-260628"].ReasoningEffort; got == nil ||
			got.DefaultLevel != "max" {
			t.Fatalf("unexpected Doubao 2.1 Turbo effort: %+v", got)
		}
		if got := provider.ModelCapabilities["doubao-seed-2-0-lite-260428"].ReasoningEffort; got == nil ||
			got.DefaultLevel != "high" {
			t.Fatalf("unexpected Doubao 2.0 Lite effort: %+v", got)
		}
		if got := provider.ModelCapabilities["deepseek-v4-pro-ga-260813"].ReasoningEffort; got == nil ||
			!reflect.DeepEqual(got.Levels, []string{"low", "high"}) {
			t.Fatalf("unexpected hosted DeepSeek effort: %+v", got)
		}
		if got := provider.ModelCapabilities["doubao-seed-2-1-turbo-260628"].ContextWindow; got != 256000 {
			t.Fatalf("expected Doubao 2.1 Turbo context metadata, got %d", got)
		}
		if got := provider.ModelCapabilities["glm-5-3-flash-260828"].ContextWindow; got != 1024000 {
			t.Fatalf("expected Doubao hosted GLM-5.3-Flash context metadata, got %d", got)
		}
		if got := provider.ModelCapabilities["glm-5-2-260617"].ContextWindow; got != 1024000 {
			t.Fatalf("expected Doubao hosted GLM-5.2 context metadata, got %d", got)
		}
		if got := provider.ModelCapabilities["deepseek-v4-pro-ga-260813"].SupportedProtocols; !reflect.DeepEqual(got, []string{"openai_chat", "openai_responses"}) {
			t.Fatalf("hosted DeepSeek must keep Chat + Responses: %v", got)
		}
		for _, model := range provider.Models {
			if model == "glm-5.3" {
				t.Fatal("pay-as-you-go picker must not list Coding Plan glm-5.3")
			}
		}
	}
}
