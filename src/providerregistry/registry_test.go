package providerregistry

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRegistryCRUDNormalizesAndSnapshots(t *testing.T) {
	registry := NewRegistry()

	created, err := registry.Add(Provider{
		ID:         " openai ",
		Name:       " OpenAI Gateway ",
		Kind:       ProviderKindOpenAICompatible,
		BaseURL:    "https://api.example.com/v1/",
		AuthScheme: AuthSchemeBearer,
		Credential: CredentialRef{
			Source:  CredentialSourceEnv,
			Name:    "OPENAI_API_KEY",
			Present: true,
		},
		Enabled: true,
		Models: []Model{
			{ID: "gpt-4.1", Enabled: true, Capabilities: []ModelCapability{ModelCapabilityText, ModelCapabilityText, ModelCapabilityToolCalling}},
			{ID: "gpt-4.1"},
			{ID: " gpt-4o ", Enabled: true},
		},
		Annotations: map[string]string{"source": "test"},
	})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	if created.ID != "openai" {
		t.Fatalf("expected normalized id, got %q", created.ID)
	}
	if created.Name != "OpenAI Gateway" {
		t.Fatalf("expected normalized name, got %q", created.Name)
	}
	if created.BaseURL != "https://api.example.com/v1" {
		t.Fatalf("expected normalized base url, got %q", created.BaseURL)
	}
	if created.Protocol != ProtocolOpenAIChat {
		t.Fatalf("expected default openai protocol, got %q", created.Protocol)
	}
	if !created.Enabled {
		t.Fatalf("expected provider enabled flag to be preserved")
	}
	if len(created.Models) != 2 {
		t.Fatalf("expected duplicate models to be removed, got %d", len(created.Models))
	}
	if created.Models[0].ID != "gpt-4.1" || created.Models[1].ID != "gpt-4o" {
		t.Fatalf("unexpected model order: %#v", created.Models)
	}
	if !created.Models[0].Enabled || !created.Models[1].Enabled {
		t.Fatalf("expected model enabled flags to be preserved: %#v", created.Models)
	}
	if len(created.Models[0].Capabilities) != 2 {
		t.Fatalf("expected duplicate capabilities to be removed, got %#v", created.Models[0].Capabilities)
	}

	created.Models[0].ID = "mutated"
	got, ok := registry.Get("openai")
	if !ok {
		t.Fatalf("expected provider to exist")
	}
	if got.Models[0].ID != "gpt-4.1" {
		t.Fatalf("snapshot mutation leaked into registry: %#v", got.Models)
	}

	list := registry.List()
	if len(list) != 1 || list[0].ID != "openai" {
		t.Fatalf("unexpected list result: %#v", list)
	}

	updated, err := registry.Update(Provider{
		ID:         "openai",
		Name:       "Updated Gateway",
		Kind:       ProviderKindOpenAICompatible,
		BaseURL:    "https://api.example.com/v1",
		AuthScheme: AuthSchemeBearer,
		Credential: CredentialRef{Source: CredentialSourceEnv, Name: "OPENAI_API_KEY", Present: true},
		Models:     []Model{{ID: "gpt-4.1", Enabled: true}},
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if updated.Name != "Updated Gateway" || len(updated.Models) != 1 {
		t.Fatalf("unexpected updated snapshot: %#v", updated)
	}

	if err := registry.Remove("openai"); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if _, ok := registry.Get("openai"); ok {
		t.Fatalf("expected provider to be removed")
	}
	if err := registry.Remove("openai"); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("expected ErrProviderNotFound, got %v", err)
	}
}

func TestSnapshotsRedactSecretLikeValues(t *testing.T) {
	registry := NewRegistry()

	created, err := registry.Add(Provider{
		ID:         "anthropic",
		Name:       "Anthropic",
		Kind:       ProviderKindAnthropic,
		BaseURL:    "https://api.anthropic.com/",
		AuthScheme: AuthSchemeXAPIKey,
		Credential: CredentialRef{
			Source:  CredentialSourceEnv,
			Name:    "sk-test-secretvalue",
			Present: true,
		},
		Models: []Model{{ID: "claude-sonnet"}},
	})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if created.Credential.Name != "<redacted>" {
		t.Fatalf("expected secret-like credential name to be redacted, got %q", created.Credential.Name)
	}
	if !created.Credential.Present {
		t.Fatalf("expected credential presence to be preserved")
	}

	checkedAt := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	snap, err := registry.SetHealth("anthropic", "claude-sonnet", HealthCheck{
		Status:        HealthUnhealthy,
		CheckedAt:     checkedAt,
		LatencyMillis: 42,
		Message:       "request failed Authorization: Bearer sk-test-secretvalue api_key=secret-token",
	})
	if err != nil {
		t.Fatalf("SetHealth returned error: %v", err)
	}

	rendered := fmt.Sprintf("%#v", snap)
	for _, forbidden := range []string{"sk-test-secretvalue", "secret-token"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("snapshot leaked %q: %s", forbidden, rendered)
		}
	}
	if !strings.Contains(snap.Health["claude-sonnet"].Message, "<redacted>") {
		t.Fatalf("expected health message to include redaction marker, got %q", snap.Health["claude-sonnet"].Message)
	}
}

func TestEnableProviderAndModel(t *testing.T) {
	registry := NewRegistry()
	if _, err := registry.Add(Provider{
		ID:      "gemini",
		Name:    "Gemini",
		Kind:    ProviderKindGemini,
		BaseURL: "https://generativelanguage.googleapis.com",
		Enabled: true,
		Models:  []Model{{ID: "gemini-2.5-pro", Enabled: true}, {ID: "gemini-2.5-flash", Enabled: true}},
	}); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	disabledProvider, err := registry.EnableProvider("gemini", false)
	if err != nil {
		t.Fatalf("EnableProvider returned error: %v", err)
	}
	if disabledProvider.Enabled {
		t.Fatalf("expected provider disabled")
	}

	updated, err := registry.EnableModel("gemini", "gemini-2.5-flash", false)
	if err != nil {
		t.Fatalf("EnableModel returned error: %v", err)
	}
	if updated.Models[0].Enabled != true || updated.Models[1].Enabled != false {
		t.Fatalf("unexpected model enabled states: %#v", updated.Models)
	}
	if _, err := registry.EnableModel("gemini", "missing", false); !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("expected ErrModelNotFound, got %v", err)
	}
}

func TestNormalizeProviderPreservesDisabledState(t *testing.T) {
	normalized, err := NormalizeProvider(Provider{
		ID:      " custom ",
		Name:    " Custom ",
		Kind:    ProviderKindCustom,
		BaseURL: "https://custom.example.com/",
		Models: []Model{
			{ID: "primary", Enabled: false},
			{ID: "secondary", Enabled: true},
		},
	})
	if err != nil {
		t.Fatalf("NormalizeProvider returned error: %v", err)
	}

	if normalized.Enabled {
		t.Fatalf("expected disabled provider state to be preserved")
	}
	if normalized.Models[0].Enabled {
		t.Fatalf("expected disabled model state to be preserved")
	}
	if !normalized.Models[1].Enabled {
		t.Fatalf("expected enabled model state to be preserved")
	}
}

func TestDiscoveryPlan(t *testing.T) {
	tests := []struct {
		name      string
		provider  Provider
		path      string
		authStyle string
		protocol  Protocol
	}{
		{
			name: "openai compatible",
			provider: Provider{
				ID:      "openai-compatible",
				Name:    "OpenAI Compatible",
				Kind:    ProviderKindOpenAICompatible,
				BaseURL: "https://gateway.example.com/v1/",
				Models:  []Model{{ID: "gpt-4.1"}},
			},
			path:      "/v1/models",
			authStyle: "Authorization: Bearer <redacted>",
			protocol:  ProtocolOpenAIChat,
		},
		{
			name: "anthropic",
			provider: Provider{
				ID:      "anthropic",
				Name:    "Anthropic",
				Kind:    ProviderKindAnthropic,
				BaseURL: "https://api.anthropic.com",
				Models:  []Model{{ID: "claude-sonnet"}},
			},
			path:      "/v1/models",
			authStyle: "x-api-key: <redacted>",
			protocol:  ProtocolAnthropicMessages,
		},
		{
			name: "gemini",
			provider: Provider{
				ID:      "gemini",
				Name:    "Gemini",
				Kind:    ProviderKindGemini,
				BaseURL: "https://generativelanguage.googleapis.com",
				Models:  []Model{{ID: "gemini-2.5-pro"}},
			},
			path:      "/v1beta/models",
			authStyle: "x-goog-api-key: <redacted>",
			protocol:  ProtocolGeminiGenerateContent,
		},
		{
			name: "bedrock",
			provider: Provider{
				ID:     "bedrock",
				Name:   "Bedrock",
				Kind:   ProviderKindBedrock,
				Models: []Model{{ID: "anthropic.claude-sonnet"}},
			},
			path:      "/foundation-models",
			authStyle: "AWS SigV4",
			protocol:  ProtocolBedrockConverse,
		},
		{
			name: "vertex",
			provider: Provider{
				ID:     "vertex",
				Name:   "Vertex",
				Kind:   ProviderKindVertex,
				Models: []Model{{ID: "gemini-2.5-pro"}},
			},
			path:      "/v1/projects/{project}/locations/{location}/publishers/google/models",
			authStyle: "Authorization: Bearer <google-adc-token>",
			protocol:  ProtocolVertexGemini,
		},
		{
			name: "custom",
			provider: Provider{
				ID:       "custom",
				Name:     "Custom",
				Kind:     ProviderKindCustom,
				Protocol: ProtocolAnthropicMessages,
				BaseURL:  "https://custom.example.com/api/",
				Models:   []Model{{ID: "custom-model"}},
			},
			path:      "/api/models",
			authStyle: "custom",
			protocol:  ProtocolAnthropicMessages,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()
			if _, err := registry.Add(tt.provider); err != nil {
				t.Fatalf("Add returned error: %v", err)
			}

			plan, err := registry.DiscoveryPlan(tt.provider.ID)
			if err != nil {
				t.Fatalf("DiscoveryPlan returned error: %v", err)
			}
			if plan.Method != "GET" {
				t.Fatalf("expected GET, got %q", plan.Method)
			}
			if plan.Path != tt.path {
				t.Fatalf("expected path %q, got %q", tt.path, plan.Path)
			}
			if plan.AuthHeaderStyle != tt.authStyle {
				t.Fatalf("expected auth style %q, got %q", tt.authStyle, plan.AuthHeaderStyle)
			}
			if plan.ExpectedProtocol != tt.protocol {
				t.Fatalf("expected protocol %q, got %q", tt.protocol, plan.ExpectedProtocol)
			}
		})
	}
}

func TestSetHealthState(t *testing.T) {
	registry := NewRegistry()
	if _, err := registry.Add(Provider{
		ID:      "openai",
		Name:    "OpenAI",
		Kind:    ProviderKindOpenAICompatible,
		BaseURL: "https://api.example.com/v1",
		Models:  []Model{{ID: "gpt-4.1"}},
	}); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	checkedAt := time.Date(2026, 6, 28, 12, 30, 0, 0, time.UTC)
	snap, err := registry.SetHealth("openai", "gpt-4.1", HealthCheck{
		Status:        HealthHealthy,
		CheckedAt:     checkedAt,
		LatencyMillis: 120,
	})
	if err != nil {
		t.Fatalf("SetHealth returned error: %v", err)
	}

	health := snap.Health["gpt-4.1"]
	if health.Status != HealthHealthy {
		t.Fatalf("expected healthy status, got %q", health.Status)
	}
	if !health.CheckedAt.Equal(checkedAt) {
		t.Fatalf("expected checked_at to be preserved, got %v", health.CheckedAt)
	}
	if health.LatencyMillis != 120 {
		t.Fatalf("expected latency 120, got %d", health.LatencyMillis)
	}
	if _, err := registry.SetHealth("openai", "missing", HealthCheck{}); !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("expected ErrModelNotFound, got %v", err)
	}
	if _, err := registry.SetHealth("openai", "gpt-4.1", HealthCheck{Status: "broken"}); err == nil {
		t.Fatalf("expected invalid health status error")
	}
}

func TestValidation(t *testing.T) {
	valid := Provider{
		ID:      "provider",
		Name:    "Provider",
		Kind:    ProviderKindOpenAICompatible,
		BaseURL: "https://api.example.com/v1",
		Models:  []Model{{ID: "model"}},
	}

	tests := []struct {
		name     string
		mutate   func(*Provider)
		contains string
	}{
		{
			name:     "missing provider id",
			mutate:   func(provider *Provider) { provider.ID = "" },
			contains: "provider id is required",
		},
		{
			name:     "missing provider name",
			mutate:   func(provider *Provider) { provider.Name = "" },
			contains: "provider name is required",
		},
		{
			name:     "invalid protocol",
			mutate:   func(provider *Provider) { provider.Protocol = "bad_protocol" },
			contains: "invalid protocol",
		},
		{
			name:     "invalid base url",
			mutate:   func(provider *Provider) { provider.BaseURL = "ftp://api.example.com" },
			contains: "unsupported base url scheme",
		},
		{
			name:     "missing model id",
			mutate:   func(provider *Provider) { provider.Models = []Model{{ID: " "}} },
			contains: "model id is required",
		},
		{
			name:     "missing models",
			mutate:   func(provider *Provider) { provider.Models = nil },
			contains: "at least one model is required",
		},
		{
			name:     "invalid model protocol",
			mutate:   func(provider *Provider) { provider.Models = []Model{{ID: "model", Protocol: "bad_protocol"}} },
			contains: "invalid model protocol",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := valid
			tt.mutate(&provider)
			err := ValidateProvider(provider)
			if err == nil {
				t.Fatalf("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.contains) {
				t.Fatalf("expected error containing %q, got %v", tt.contains, err)
			}
		})
	}
}
