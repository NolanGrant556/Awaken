package bridgetoken

import (
	"sync/atomic"
	"testing"
)

func TestTrustedDeviceTokenSourceUsesEnvTokenBeforeStorage(t *testing.T) {
	var storageReads atomic.Int32
	source := NewTrustedDeviceTokenSource(TrustedDeviceTokenSourceConfig{
		EnvToken:    func() string { return "env-token" },
		GateEnabled: func() bool { return true },
		GetStoredToken: func() (string, error) {
			storageReads.Add(1)
			return "stored-token", nil
		},
	})

	if token := source.Token(); token != "env-token" {
		t.Fatalf("expected env token, got %q", token)
	}
	if reads := storageReads.Load(); reads != 0 {
		t.Fatalf("expected storage not to be read, got %d reads", reads)
	}
}

func TestTrustedDeviceTokenSourceGateOffReturnsEmpty(t *testing.T) {
	var storageReads atomic.Int32
	source := NewTrustedDeviceTokenSource(TrustedDeviceTokenSourceConfig{
		EnvToken:    func() string { return "env-token" },
		GateEnabled: func() bool { return false },
		GetStoredToken: func() (string, error) {
			storageReads.Add(1)
			return "stored-token", nil
		},
	})

	if token := source.Token(); token != "" {
		t.Fatalf("expected empty token when gate is off, got %q", token)
	}
	if reads := storageReads.Load(); reads != 0 {
		t.Fatalf("expected storage not to be read, got %d reads", reads)
	}
}

func TestTrustedDeviceTokenSourceClearCache(t *testing.T) {
	storedToken := "stored-token-1"
	source := NewTrustedDeviceTokenSource(TrustedDeviceTokenSourceConfig{
		EnvToken:    func() string { return "" },
		GateEnabled: func() bool { return true },
		GetStoredToken: func() (string, error) {
			return storedToken, nil
		},
	})

	if token := source.Token(); token != "stored-token-1" {
		t.Fatalf("expected first stored token, got %q", token)
	}

	storedToken = "stored-token-2"
	if token := source.Token(); token != "stored-token-1" {
		t.Fatalf("expected cached stored token, got %q", token)
	}

	source.ClearCache()
	if token := source.Token(); token != "stored-token-2" {
		t.Fatalf("expected refreshed stored token, got %q", token)
	}
}

func TestTrustedDeviceTokenSourceClearToken(t *testing.T) {
	storedToken := "stored-token"
	source := NewTrustedDeviceTokenSource(TrustedDeviceTokenSourceConfig{
		EnvToken:    func() string { return "" },
		GateEnabled: func() bool { return true },
		GetStoredToken: func() (string, error) {
			return storedToken, nil
		},
		SetStoredToken: func(token string) error {
			storedToken = token
			return nil
		},
	})

	if token := source.Token(); token != "stored-token" {
		t.Fatalf("expected stored token, got %q", token)
	}

	if err := source.ClearToken(); err != nil {
		t.Fatalf("clear token failed: %v", err)
	}
	if storedToken != "" {
		t.Fatalf("expected stored token to be cleared, got %q", storedToken)
	}
	if token := source.Token(); token != "" {
		t.Fatalf("expected empty token after clear, got %q", token)
	}
}
