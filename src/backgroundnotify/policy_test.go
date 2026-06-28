package backgroundnotify

import "testing"

func TestNormalizeModeAcceptsKnownModes(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want Mode
	}{
		{name: "all", raw: "all", want: ModeAll},
		{name: "result", raw: "result", want: ModeResult},
		{name: "error", raw: "error", want: ModeError},
		{name: "off", raw: "off", want: ModeOff},
		{name: "trim and lower", raw: "  RESULT ", want: ModeResult},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NormalizeMode(tt.raw)
			if !ok {
				t.Fatalf("expected %q to normalize", tt.raw)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestNormalizeModeRejectsUnknownModes(t *testing.T) {
	if got, ok := NormalizeMode("banana"); ok || got != "" {
		t.Fatalf("expected unknown mode to be rejected, got mode=%q ok=%v", got, ok)
	}
}

func TestResolveModesAndSources(t *testing.T) {
	tests := []struct {
		name        string
		configValue any
		envValue    string
		wantMode    Mode
		wantSource  Source
		wantWarning bool
	}{
		{
			name:       "empty values default to all",
			wantMode:   ModeAll,
			wantSource: SourceDefault,
		},
		{
			name:        "config value resolves mode",
			configValue: "error",
			wantMode:    ModeError,
			wantSource:  SourceConfig,
		},
		{
			name:        "environment overrides config",
			configValue: "error",
			envValue:    "off",
			wantMode:    ModeOff,
			wantSource:  SourceEnv,
		},
		{
			name:        "yaml boolean false disables notifications",
			configValue: false,
			wantMode:    ModeOff,
			wantSource:  SourceConfig,
		},
		{
			name:        "invalid config defaults to all with warning",
			configValue: "banana",
			wantMode:    ModeAll,
			wantSource:  SourceConfig,
			wantWarning: true,
		},
		{
			name:        "invalid environment still takes priority and defaults to all",
			configValue: "off",
			envValue:    "banana",
			wantMode:    ModeAll,
			wantSource:  SourceEnv,
			wantWarning: true,
		},
		{
			name:        "yaml boolean true is invalid and defaults to all",
			configValue: true,
			wantMode:    ModeAll,
			wantSource:  SourceConfig,
			wantWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.configValue, tt.envValue)
			if got.Mode != tt.wantMode {
				t.Fatalf("expected mode %q, got %q", tt.wantMode, got.Mode)
			}
			if got.Source != tt.wantSource {
				t.Fatalf("expected source %q, got %q", tt.wantSource, got.Source)
			}
			if got.Reason == "" {
				t.Fatalf("expected resolution reason")
			}
			if tt.wantWarning && got.Warning == "" {
				t.Fatalf("expected warning for invalid value")
			}
			if !tt.wantWarning && got.Warning != "" {
				t.Fatalf("unexpected warning: %q", got.Warning)
			}
		})
	}
}

func TestShouldNotifyRunningUpdates(t *testing.T) {
	tests := []struct {
		mode Mode
		want bool
	}{
		{mode: ModeAll, want: true},
		{mode: ModeResult, want: false},
		{mode: ModeError, want: false},
		{mode: ModeOff, want: false},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			got := ShouldNotify(tt.mode, Event{Type: EventTypeRunningUpdate})
			if got.Notify != tt.want {
				t.Fatalf("expected notify=%v, got %v", tt.want, got.Notify)
			}
			if got.Reason == "" {
				t.Fatalf("expected decision reason")
			}
		})
	}
}

func TestShouldNotifyFinalResults(t *testing.T) {
	tests := []struct {
		name     string
		mode     Mode
		exitCode int
		want     bool
	}{
		{name: "all success", mode: ModeAll, exitCode: 0, want: true},
		{name: "all error", mode: ModeAll, exitCode: 1, want: true},
		{name: "result success", mode: ModeResult, exitCode: 0, want: true},
		{name: "result error", mode: ModeResult, exitCode: 1, want: true},
		{name: "error success", mode: ModeError, exitCode: 0, want: false},
		{name: "error error", mode: ModeError, exitCode: 1, want: true},
		{name: "off success", mode: ModeOff, exitCode: 0, want: false},
		{name: "off error", mode: ModeOff, exitCode: 1, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldNotify(tt.mode, Event{
				Type: EventTypeFinalResult,
				Result: ProcessResult{
					ExitCode: tt.exitCode,
				},
			})
			if got.Notify != tt.want {
				t.Fatalf("expected notify=%v, got %v", tt.want, got.Notify)
			}
			if got.Reason == "" {
				t.Fatalf("expected decision reason")
			}
		})
	}
}
