package progressmode

import (
	"strings"
	"testing"
)

func TestResolveDefaultsToAll(t *testing.T) {
	resolved := Resolve(ResolveRequest{Env: map[string]string{}})

	if resolved.EffectiveMode != ModeAll {
		t.Fatalf("expected default mode all, got %q", resolved.EffectiveMode)
	}
	if resolved.Source != SourceDefault || !resolved.Enabled {
		t.Fatalf("unexpected resolution: %+v", resolved)
	}
}

func TestResolveUsesEnvFallback(t *testing.T) {
	resolved := Resolve(ResolveRequest{
		Env: map[string]string{EnvToolProgressMode: "new"},
	})

	if resolved.EffectiveMode != ModeNew {
		t.Fatalf("expected env mode new, got %q", resolved.EffectiveMode)
	}
	if resolved.Source != SourceEnv || !resolved.Enabled {
		t.Fatalf("unexpected resolution: %+v", resolved)
	}
}

func TestResolvePlatformOverrideBeatsGlobalAndEnv(t *testing.T) {
	resolved := Resolve(ResolveRequest{
		Config: Config{
			ToolProgress: ModeOff,
			Platforms: map[string]PlatformConfig{
				"telegram": {ToolProgress: ModeVerbose},
			},
		},
		Platform: "telegram",
		Env:      map[string]string{EnvToolProgressMode: "all"},
	})

	if resolved.EffectiveMode != ModeVerbose {
		t.Fatalf("expected platform mode verbose, got %q", resolved.EffectiveMode)
	}
	if resolved.Source != SourcePlatform || !resolved.Enabled {
		t.Fatalf("unexpected resolution: %+v", resolved)
	}
}

func TestResolveInvalidModeFallsBack(t *testing.T) {
	resolved := Resolve(ResolveRequest{
		Config: Config{ToolProgress: Mode("loud")},
		Env:    map[string]string{},
	})

	if resolved.EffectiveMode != ModeAll {
		t.Fatalf("expected invalid mode to fall back to all, got %q", resolved.EffectiveMode)
	}
	if resolved.Source != SourceDefault || !resolved.Enabled {
		t.Fatalf("unexpected resolution: %+v", resolved)
	}
	if !strings.Contains(resolved.Reason, "invalid display.tool_progress") {
		t.Fatalf("expected invalid mode reason, got %q", resolved.Reason)
	}
}

func TestResolveWebhookDisabled(t *testing.T) {
	resolved := Resolve(ResolveRequest{
		Config:   Config{ToolProgress: ModeVerbose},
		Platform: "webhook",
		Env:      map[string]string{},
	})

	if resolved.EffectiveMode != ModeOff {
		t.Fatalf("expected webhook to force off, got %q", resolved.EffectiveMode)
	}
	if resolved.Source != SourceDisabled || resolved.Enabled {
		t.Fatalf("unexpected resolution: %+v", resolved)
	}
	if !strings.Contains(resolved.Reason, "webhook") {
		t.Fatalf("expected webhook reason, got %q", resolved.Reason)
	}
}

func TestNextModeCyclesInOrder(t *testing.T) {
	tests := []struct {
		current Mode
		next    Mode
	}{
		{ModeOff, ModeNew},
		{ModeNew, ModeAll},
		{ModeAll, ModeVerbose},
		{ModeVerbose, ModeOff},
	}

	for _, tt := range tests {
		if got := NextMode(tt.current); got != tt.next {
			t.Fatalf("NextMode(%q) = %q, want %q", tt.current, got, tt.next)
		}
	}
}

func TestCycleRequiresCommandGate(t *testing.T) {
	cycled := Cycle(ResolveRequest{
		Config: Config{
			ToolProgress:        ModeAll,
			ToolProgressCommand: false,
		},
		Platform: "telegram",
		Env:      map[string]string{},
	})

	if cycled.CanCycle {
		t.Fatalf("expected cycle to be blocked: %+v", cycled)
	}
	if cycled.NextMode != ModeAll {
		t.Fatalf("expected blocked cycle to keep current mode, got %q", cycled.NextMode)
	}
	if !strings.Contains(cycled.Reason, "display.tool_progress_command") {
		t.Fatalf("expected gate reason, got %q", cycled.Reason)
	}
}

func TestCycleWhenGateEnabled(t *testing.T) {
	cycled := Cycle(ResolveRequest{
		Config: Config{
			ToolProgress:        ModeAll,
			ToolProgressCommand: true,
		},
		Platform: "telegram",
		Env:      map[string]string{},
	})

	if !cycled.CanCycle {
		t.Fatalf("expected cycle to be allowed: %+v", cycled)
	}
	if cycled.NextMode != ModeVerbose {
		t.Fatalf("expected all to cycle to verbose, got %q", cycled.NextMode)
	}
	if cycled.Resolution.Source != SourceGlobal {
		t.Fatalf("unexpected source: %+v", cycled.Resolution)
	}
}
