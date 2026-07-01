package modelprofile

import "testing"

func TestBuiltinProvidersIncludeRequiredDefaults(t *testing.T) {
	providers := BuiltinProviders()
	want := []string{"deepseek", "kimi-cn", "kimi-coding-plan", "volcengine"}
	if len(providers) != len(want) {
		t.Fatalf("expected %d providers, got %d", len(want), len(providers))
	}

	seen := map[string]Provider{}
	for _, provider := range providers {
		seen[provider.ID] = provider
		if provider.ID == "doubao" {
			t.Fatal("doubao must not be exposed as a separate built-in provider")
		}
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
	}
}

func TestProviderByID(t *testing.T) {
	provider, ok := ProviderByID("volcengine")
	if !ok {
		t.Fatal("expected volcengine provider")
	}
	if provider.DisplayName != "火山引擎" {
		t.Fatalf("unexpected display name %q", provider.DisplayName)
	}
	if _, ok := ProviderByID("doubao"); ok {
		t.Fatal("doubao must not resolve as a provider")
	}
}
