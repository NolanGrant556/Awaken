package llm

import "testing"

func TestAuxiliaryConfigUsesConfirmedProviderMappings(t *testing.T) {
	tests := []struct {
		provider string
		main     string
		want     string
		fallback bool
	}{
		{"deepseek", "deepseek-v4-pro", "deepseek-v4-flash", false},
		{"kimi-cn", "kimi-k2.7-code", "kimi-k2.6", false},
		{"kimi-coding-plan", "k3", "kimi-for-coding", false},
		{"volcengine", "ark-code-latest", "ark-code-latest", true},
		{"custom", "private-model", "private-model", true},
	}
	for _, test := range tests {
		t.Run(test.provider, func(t *testing.T) {
			got, fallback := AuxiliaryConfig(Config{ProviderID: test.provider, Model: test.main})
			if got.Model != test.want || fallback != test.fallback {
				t.Fatalf("AuxiliaryConfig() model=%q fallback=%v, want %q/%v", got.Model, fallback, test.want, test.fallback)
			}
		})
	}
}
