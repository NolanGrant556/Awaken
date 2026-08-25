package computer

import (
	"encoding/json"
	"testing"
	"time"
)

func TestComputerSettingsExposeOnlyRuntimePreferences(t *testing.T) {
	encoded, err := json.Marshal(DefaultSettings())
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if len(fields) != 2 {
		t.Fatalf("settings fields = %v, want only enable and background preferences", fields)
	}
	if _, ok := fields["enabled"]; !ok {
		t.Fatal("enabled setting missing")
	}
	if _, ok := fields["allow_background_window_actions"]; !ok {
		t.Fatal("allow_background_window_actions setting missing")
	}
}

func TestComputerDefaultsAllowBothScopesAndUseFixedMediaRetention(t *testing.T) {
	policy := PolicyFromSettings(DefaultSettings())
	hasScope := func(want CaptureScope) bool {
		for _, scope := range policy.AllowedScopes {
			if scope == want {
				return true
			}
		}
		return false
	}
	if !hasScope(CaptureWindow) || !hasScope(CaptureDesktop) {
		t.Fatalf("default policy scopes = %v, want window and desktop", policy.AllowedScopes)
	}
	retention := DefaultMediaRetentionOptions()
	if retention.MaxAge != 24*time.Hour {
		t.Fatalf("max age = %v, want 24h", retention.MaxAge)
	}
	if retention.MaxTotalBytes != 500*1024*1024 {
		t.Fatalf("max bytes = %d, want 500 MiB", retention.MaxTotalBytes)
	}
}

func TestPolicyAndBindingPinBackgroundWindowPreference(t *testing.T) {
	policy := PolicyFromSettings(Settings{Enabled: true, AllowBackgroundWindowActions: true})
	if !policy.AllowBackgroundWindowActions {
		t.Fatal("policy did not pin enabled background window delivery")
	}
	policy, err := NewPolicySnapshot(policy)
	if err != nil {
		t.Fatalf("new policy snapshot: %v", err)
	}
	binding, err := NewBinding("run", "thread", "agent", "host://local/desktop", policy)
	if err != nil {
		t.Fatalf("new binding: %v", err)
	}
	if !binding.AllowBackgroundWindowActions {
		t.Fatal("binding did not preserve background window delivery preference")
	}
}
