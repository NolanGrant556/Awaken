package modeldisplay

import "testing"

func TestSanitizeModelNameMapsKnownFamilies(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{name: "opus 4.6 before broader opus 4", model: "internal-opus-4-6-fast", want: "claude-opus-4-6"},
		{name: "opus 4.5", model: "opus-4-5", want: "claude-opus-4-5"},
		{name: "opus 4.1", model: "company-opus-4-1-preview", want: "claude-opus-4-1"},
		{name: "opus 4", model: "opus-4-latest", want: "claude-opus-4"},
		{name: "sonnet 4.6", model: "sonnet-4-6-thinking", want: "claude-sonnet-4-6"},
		{name: "sonnet 4.5", model: "internal-sonnet-4-5-fast", want: "claude-sonnet-4-5"},
		{name: "sonnet 4", model: "sonnet-4", want: "claude-sonnet-4"},
		{name: "sonnet 3.7", model: "sonnet-3-7", want: "claude-sonnet-3-7"},
		{name: "haiku 4.5", model: "haiku-4-5", want: "claude-haiku-4-5"},
		{name: "haiku 3.5", model: "haiku-3-5-small", want: "claude-haiku-3-5"},
		{name: "unknown fallback", model: "capybara-v2-fast", want: "claude"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeModelName(tt.model); got != tt.want {
				t.Fatalf("SanitizeModelName(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestSanitizeSurfaceKeyOnlySanitizesLastSlashSegment(t *testing.T) {
	tests := []struct {
		name       string
		surfaceKey string
		want       string
	}{
		{
			name:       "simple surface model",
			surfaceKey: "cli/internal-opus-4-6-fast",
			want:       "cli/claude-opus-4-6",
		},
		{
			name:       "nested surface keeps earlier slashes",
			surfaceKey: "desktop/run/internal-sonnet-4-5-fast",
			want:       "desktop/run/claude-sonnet-4-5",
		},
		{
			name:       "unknown model segment falls back safely",
			surfaceKey: "cli/capybara-v2-fast",
			want:       "cli/claude",
		},
		{
			name:       "no slash is not a surface model key",
			surfaceKey: "capybara-v2-fast",
			want:       "capybara-v2-fast",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeSurfaceKey(tt.surfaceKey); got != tt.want {
				t.Fatalf("SanitizeSurfaceKey(%q) = %q, want %q", tt.surfaceKey, got, tt.want)
			}
		})
	}
}

func TestPublicModelDisplayName(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{model: "claude-opus-4-6", want: "Opus 4.6"},
		{model: "claude-opus-4-6[1m]", want: "Opus 4.6 (1M context)"},
		{model: "claude-opus-4-5", want: "Opus 4.5"},
		{model: "claude-opus-4-1", want: "Opus 4.1"},
		{model: "claude-opus-4", want: "Opus 4"},
		{model: "claude-sonnet-4-6", want: "Sonnet 4.6"},
		{model: "claude-sonnet-4-5[1m]", want: "Sonnet 4.5 (1M context)"},
		{model: "claude-sonnet-4", want: "Sonnet 4"},
		{model: "claude-sonnet-3-7", want: "Sonnet 3.7"},
		{model: "claude-haiku-4-5", want: "Haiku 4.5"},
		{model: "claude-haiku-3-5", want: "Haiku 3.5"},
		{model: "claude-3-7-sonnet", want: "Sonnet 3.7"},
		{model: "claude-3-5-haiku", want: "Haiku 3.5"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got, ok := PublicModelDisplayName(tt.model)
			if !ok {
				t.Fatalf("PublicModelDisplayName(%q) returned ok=false", tt.model)
			}
			if got != tt.want {
				t.Fatalf("PublicModelDisplayName(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestPublicModelDisplayNameUnknown(t *testing.T) {
	if got, ok := PublicModelDisplayName("capybara-v2-fast"); ok || got != "" {
		t.Fatalf("PublicModelDisplayName returned (%q, %v), want empty false", got, ok)
	}
}

func TestRenderModelNameMasksUnknownInternalNames(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{model: "claude-opus-4-6", want: "Opus 4.6"},
		{model: "capybara-v2-fast", want: "cap*****-v2-fast"},
		{model: "capybara-v2-fast[1m]", want: "cap*****-v2-fast[1m]"},
		{model: "fox-v2", want: "fox-v2"},
		{model: "customdeployment", want: "cus*************"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := RenderModelName(tt.model); got != tt.want {
				t.Fatalf("RenderModelName(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestPublicAuthorName(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{model: "claude-opus-4-6", want: "Claude Opus 4.6"},
		{model: "claude-opus-4-6[1m]", want: "Claude Opus 4.6 (1M context)"},
		{model: "capybara-v2-fast", want: "Claude (capybara-v2-fast)"},
		{model: "", want: "Claude"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := PublicAuthorName(tt.model); got != tt.want {
				t.Fatalf("PublicAuthorName(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}
