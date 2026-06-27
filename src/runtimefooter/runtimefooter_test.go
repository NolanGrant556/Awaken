package runtimefooter

import "testing"

func TestBuildDefaultDisabled(t *testing.T) {
	state := RuntimeState{
		Model:         "openai/gpt-5.4",
		ContextTokens: 25,
		ContextLength: 100,
		CWD:           "/Users/nolangrant/Awaken",
		Home:          "/Users/nolangrant",
	}

	resolved := ResolveConfig(Config{}, "")
	if resolved.Enabled {
		t.Fatalf("expected default config to be disabled")
	}
	if got := Format(RuntimeState{Model: "x"}, resolved.Fields); got != "x" {
		t.Fatalf("expected default fields to include model, got %q", got)
	}
	if got := Build(Config{}, "", state); got != "" {
		t.Fatalf("expected disabled footer to be empty, got %q", got)
	}
}

func TestBuildGlobalEnable(t *testing.T) {
	config := Config{Enabled: boolPtr(true)}
	state := RuntimeState{
		Model:         "openai/gpt-5.4",
		ContextTokens: 250,
		ContextLength: 1000,
		CWD:           "/Users/nolangrant/Awaken",
		Home:          "/Users/nolangrant",
	}

	if got := Build(config, "", state); got != "gpt-5.4 · 25% · ~/Awaken" {
		t.Fatalf("unexpected footer: %q", got)
	}
}

func TestBuildPlatformOverride(t *testing.T) {
	config := Config{
		Enabled: boolPtr(true),
		Fields:  []string{FieldModel},
		Platforms: map[string]PlatformConfig{
			"desktop": {
				Enabled: boolPtr(false),
				Fields:  []string{FieldCWD},
			},
			"cli": {
				Fields: []string{FieldCWD, FieldContextPct},
			},
		},
	}
	state := RuntimeState{
		Model:         "anthropic/claude-sonnet-4",
		ContextTokens: 3,
		ContextLength: 10,
		CWD:           "/Users/nolangrant/Awaken",
		Home:          "/Users/nolangrant",
	}

	if got := Build(config, "desktop", state); got != "" {
		t.Fatalf("expected platform disabled footer to be empty, got %q", got)
	}
	if got := Build(config, "cli", state); got != "~/Awaken · 30%" {
		t.Fatalf("expected platform fields to override global fields, got %q", got)
	}
	if got := Build(config, "unknown", state); got != "claude-sonnet-4" {
		t.Fatalf("expected unknown platform to use global config, got %q", got)
	}
}

func TestFormatPreservesFieldOrderAndSkipsPartialData(t *testing.T) {
	state := RuntimeState{
		Model: "anthropic/claude-sonnet-4",
		CWD:   "/workspace/project",
		Home:  "/workspace",
	}

	got := Format(state, []string{FieldCWD, FieldContextPct, FieldModel})
	if got != "~/project · claude-sonnet-4" {
		t.Fatalf("unexpected ordered partial footer: %q", got)
	}
}

func TestFormatShortensModel(t *testing.T) {
	if got := Format(RuntimeState{Model: "openai/gpt-5.4"}, []string{FieldModel}); got != "gpt-5.4" {
		t.Fatalf("expected vendor prefix to be dropped, got %q", got)
	}
	if got := Format(RuntimeState{Model: "local-model"}, []string{FieldModel}); got != "local-model" {
		t.Fatalf("expected model without vendor prefix to be unchanged, got %q", got)
	}
}

func TestFormatClampsContextPercent(t *testing.T) {
	if got := Format(RuntimeState{ContextTokens: 1500, ContextLength: 1000}, []string{FieldContextPct}); got != "100%" {
		t.Fatalf("expected context pct to clamp at 100, got %q", got)
	}
	if got := Format(RuntimeState{ContextTokens: 0, ContextLength: 1000}, []string{FieldContextPct}); got != "0%" {
		t.Fatalf("expected zero context pct, got %q", got)
	}
}

func TestFormatCollapsesHomeInCWD(t *testing.T) {
	state := RuntimeState{CWD: "/Users/nolangrant/Awaken", Home: "/Users/nolangrant"}
	if got := Format(state, []string{FieldCWD}); got != "~/Awaken" {
		t.Fatalf("expected home-relative cwd, got %q", got)
	}

	state = RuntimeState{CWD: "/Users/nolangrant", Home: "/Users/nolangrant"}
	if got := Format(state, []string{FieldCWD}); got != "~" {
		t.Fatalf("expected exact home cwd to collapse, got %q", got)
	}

	state = RuntimeState{CWD: "/tmp/Awaken", Home: "/Users/nolangrant"}
	if got := Format(state, []string{FieldCWD}); got != "/tmp/Awaken" {
		t.Fatalf("expected cwd outside home to remain absolute, got %q", got)
	}
}

func TestFormatUnknownFieldAndNoDataReturnEmpty(t *testing.T) {
	if got := Format(RuntimeState{Model: "openai/gpt-5.4"}, []string{"unknown"}); got != "" {
		t.Fatalf("expected unknown field to be ignored, got %q", got)
	}
	if got := Format(RuntimeState{}, []string{FieldModel, FieldContextPct, FieldCWD}); got != "" {
		t.Fatalf("expected no-data footer to be empty, got %q", got)
	}
}

func boolPtr(value bool) *bool {
	return &value
}
