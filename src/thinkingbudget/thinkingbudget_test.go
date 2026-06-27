package thinkingbudget

import (
	"math"
	"testing"
)

func TestNormalizeMapCamelAndSnake(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
		want  Config
	}{
		{
			name: "camel case budget",
			input: map[string]any{
				"thinkingBudget":  4096,
				"includeThoughts": true,
			},
			want: Config{
				Mode:            ModeEnabled,
				BudgetTokens:    4096,
				IncludeThoughts: true,
			},
		},
		{
			name: "snake case level",
			input: map[string]any{
				"thinking_budget":  2048.9,
				"include_thoughts": false,
				"thinking_level":   " XHIGH ",
			},
			want: Config{
				Mode:         ModeAdaptive,
				BudgetTokens: 2048,
				Level:        LevelXHigh,
			},
		},
		{
			name: "explicit mode",
			input: map[string]any{
				"mode":            " adaptive ",
				"thinkingLevel":   "High",
				"includeThoughts": true,
			},
			want: Config{
				Mode:            ModeAdaptive,
				IncludeThoughts: true,
				Level:           LevelHigh,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Normalize(tt.input)
			if err != nil {
				t.Fatalf("Normalize returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Normalize() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNormalizeOptions(t *testing.T) {
	got, err := Normalize(Options{
		Mode:            " ADAPTIVE ",
		BudgetTokens:    1024,
		IncludeThoughts: true,
		Level:           " Medium ",
	})
	if err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}

	want := Config{
		Mode:            ModeAdaptive,
		BudgetTokens:    1024,
		IncludeThoughts: true,
		Level:           LevelMedium,
	}
	if got != want {
		t.Fatalf("Normalize() = %+v, want %+v", got, want)
	}
}

func TestNormalizeInvalidBudget(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
	}{
		{
			name:  "negative",
			input: map[string]any{"thinkingBudget": -1},
		},
		{
			name:  "non numeric",
			input: map[string]any{"thinkingBudget": "4096"},
		},
		{
			name:  "bool is not numeric",
			input: map[string]any{"thinkingBudget": true},
		},
		{
			name:  "nan",
			input: map[string]any{"thinkingBudget": math.NaN()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Normalize(tt.input); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestNormalizeInvalidLevel(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
	}{
		{
			name:  "unknown",
			input: map[string]any{"thinkingLevel": "extreme"},
		},
		{
			name:  "wrong type",
			input: map[string]any{"thinkingLevel": 123},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Normalize(tt.input); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestHasUltrathinkKeyword(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{text: "please ultrathink this", want: true},
		{text: "ULTRATHINK, then answer", want: true},
		{text: "do not match ultrathinking", want: false},
		{text: "preultrathink suffix", want: false},
		{text: "ultrathink2", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			if got := HasUltrathinkKeyword(tt.text); got != tt.want {
				t.Fatalf("HasUltrathinkKeyword(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestSupportsThinking(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		provider string
		want     bool
	}{
		{
			name:     "sonnet four on third party",
			model:    "claude-sonnet-4-20250514",
			provider: "bedrock",
			want:     true,
		},
		{
			name:     "opus four on third party",
			model:    "anthropic/claude-opus-4.5",
			provider: "vertex",
			want:     true,
		},
		{
			name:     "claude three remains disabled",
			model:    "claude-3-5-sonnet-20241022",
			provider: "firstParty",
			want:     false,
		},
		{
			name:     "third party haiku four is not assumed",
			model:    "claude-haiku-4-5",
			provider: "bedrock",
			want:     false,
		},
		{
			name:     "first party unknown newer model is allowed",
			model:    "claude-frontier",
			provider: "firstParty",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SupportsThinking(tt.model, tt.provider); got != tt.want {
				t.Fatalf("SupportsThinking(%q, %q) = %v, want %v", tt.model, tt.provider, got, tt.want)
			}
		})
	}
}

func TestSupportsAdaptive(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		provider string
		want     bool
	}{
		{
			name:     "opus four six",
			model:    "claude-opus-4-6-20260101",
			provider: "bedrock",
			want:     true,
		},
		{
			name:     "sonnet four seven dot version",
			model:    "anthropic/claude-sonnet-4.7",
			provider: "vertex",
			want:     true,
		},
		{
			name:     "sonnet four date is not four point six",
			model:    "claude-sonnet-4-20250514",
			provider: "firstParty",
			want:     false,
		},
		{
			name:     "haiku four five is known non adaptive",
			model:    "claude-haiku-4-5",
			provider: "foundry",
			want:     false,
		},
		{
			name:     "unknown first party model can be adaptive",
			model:    "claude-frontier",
			provider: "foundry",
			want:     true,
		},
		{
			name:     "third party unknown model stays disabled",
			model:    "claude-frontier",
			provider: "bedrock",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SupportsAdaptive(tt.model, tt.provider); got != tt.want {
				t.Fatalf("SupportsAdaptive(%q, %q) = %v, want %v", tt.model, tt.provider, got, tt.want)
			}
		})
	}
}

func TestResolveMaxTokens(t *testing.T) {
	tests := []struct {
		name             string
		requested        int
		modelDefault     int
		contextRemaining int
		want             int
		wantErr          bool
	}{
		{
			name:         "requested wins",
			requested:    1000,
			modelDefault: 2000,
			want:         1000,
		},
		{
			name:         "default fallback",
			requested:    0,
			modelDefault: 2000,
			want:         2000,
		},
		{
			name:         "negative requested falls back to default",
			requested:    -10,
			modelDefault: 2000,
			want:         2000,
		},
		{
			name:             "requested clamps to context",
			requested:        4000,
			modelDefault:     8000,
			contextRemaining: 3000,
			want:             3000,
		},
		{
			name:             "default clamps to context",
			requested:        0,
			modelDefault:     8000,
			contextRemaining: 3000,
			want:             3000,
		},
		{
			name:    "no positive value",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveMaxTokens(tt.requested, tt.modelDefault, tt.contextRemaining)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveMaxTokens returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveMaxTokens() = %d, want %d", got, tt.want)
			}
		})
	}
}
