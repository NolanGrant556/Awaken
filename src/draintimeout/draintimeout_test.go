package draintimeout

import (
	"strings"
	"testing"
	"time"
)

func TestParseEmptyUsesDefault(t *testing.T) {
	result := Parse(" ", 12*time.Second)
	if result.Timeout != 12*time.Second || result.Source != "default" || !result.UsedDefault || result.Warning != "" {
		t.Fatalf("unexpected resolution: %+v", result)
	}
}

func TestResolveUsesConfigWhenEnvIsEmpty(t *testing.T) {
	result := Resolve("", "15", DefaultTimeout)
	if result.Timeout != 15*time.Second || result.Source != "config" || result.UsedDefault {
		t.Fatalf("unexpected resolution: %+v", result)
	}
}

func TestResolveEnvTakesPriorityOverConfig(t *testing.T) {
	result := Resolve("3", "15", DefaultTimeout)
	if result.Timeout != 3*time.Second || result.Source != "env" || result.UsedDefault {
		t.Fatalf("unexpected resolution: %+v", result)
	}
}

func TestParseInvalidUsesDefault(t *testing.T) {
	result := Parse("oops", 7*time.Second)
	if result.Timeout != 7*time.Second || result.Source != "default" || !result.UsedDefault {
		t.Fatalf("unexpected resolution: %+v", result)
	}
	if !strings.Contains(result.Warning, "invalid") {
		t.Fatalf("expected invalid warning, got %q", result.Warning)
	}
}

func TestParseNegativeClampsToZero(t *testing.T) {
	result := Parse("-2", DefaultTimeout)
	if result.Timeout != 0 || result.Source != "raw" || result.UsedDefault {
		t.Fatalf("unexpected resolution: %+v", result)
	}
	if !strings.Contains(result.Warning, "negative") {
		t.Fatalf("expected negative warning, got %q", result.Warning)
	}
}

func TestParseFractionalSeconds(t *testing.T) {
	result := Parse("0.05", DefaultTimeout)
	if result.Timeout != 50*time.Millisecond || result.Source != "raw" || result.UsedDefault || result.Warning != "" {
		t.Fatalf("unexpected resolution: %+v", result)
	}
}

func TestServiceRestartExitCode(t *testing.T) {
	if ServiceRestartExitCode != 75 {
		t.Fatalf("unexpected service restart exit code: %d", ServiceRestartExitCode)
	}
}
