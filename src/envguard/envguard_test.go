package envguard

import (
	"errors"
	"testing"
)

func TestSessionEnvAllowlistIsIsolated(t *testing.T) {
	manager := NewManager()
	first := manager.NewSession()
	second := manager.NewSession()

	if err := first.AddAllowedEnv("CUSTOM_SERVICE_URL"); err != nil {
		t.Fatalf("AddAllowedEnv returned error: %v", err)
	}

	source := map[string]string{"CUSTOM_SERVICE_URL": "https://example.test"}
	if got := first.BuildPassthroughEnv(source)["CUSTOM_SERVICE_URL"]; got != "https://example.test" {
		t.Fatalf("expected first session to pass through env, got %q", got)
	}
	if _, ok := second.BuildPassthroughEnv(source)["CUSTOM_SERVICE_URL"]; ok {
		t.Fatalf("did not expect second session to inherit first session env")
	}
}

func TestAddAllowedEnvBlocksSensitiveNames(t *testing.T) {
	session := NewSession()
	blocked := []string{
		"OPENAI_API_KEY",
		"ANTHROPIC_API_KEY",
		"CUSTOM_API_KEY",
		"SERVICE_TOKEN",
		"APP_SECRET",
		"PRIVATE_KEY",
	}

	for _, name := range blocked {
		t.Run(name, func(t *testing.T) {
			err := session.AddAllowedEnv(name)
			if !errors.Is(err, ErrSensitiveEnv) {
				t.Fatalf("expected ErrSensitiveEnv, got %v", err)
			}
		})
	}
}

func TestBuildPassthroughEnvIncludesAllowedOnlyWhenPresent(t *testing.T) {
	session := NewSession()
	if err := session.AddAllowedEnv("SAFE_FEATURE_FLAG"); err != nil {
		t.Fatalf("AddAllowedEnv returned error: %v", err)
	}
	if err := session.AddAllowedEnv("MISSING_SAFE_ENV"); err != nil {
		t.Fatalf("AddAllowedEnv returned error: %v", err)
	}

	got := session.BuildPassthroughEnv(map[string]string{
		"SAFE_FEATURE_FLAG": "enabled",
		"UNDECLARED_ENV":    "ignored",
	})

	if got["SAFE_FEATURE_FLAG"] != "enabled" {
		t.Fatalf("expected allowed env to pass through, got %q", got["SAFE_FEATURE_FLAG"])
	}
	if _, ok := got["MISSING_SAFE_ENV"]; ok {
		t.Fatalf("did not expect missing env in passthrough result")
	}
	if _, ok := got["UNDECLARED_ENV"]; ok {
		t.Fatalf("did not expect undeclared env in passthrough result")
	}
}
