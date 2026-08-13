package hookruntime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileSecretStoreRequiresOwnerOnlyRegularFile(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "hook-secrets.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"secrets":{"api-token":"Bearer fixture"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewFileSecretStore(root)
	value, err := store.ResolveHookSecret(context.Background(), "api-token")
	if err != nil || value != "Bearer fixture" {
		t.Fatalf("value=%q err=%v", value, err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveHookSecret(context.Background(), "api-token"); err == nil {
		t.Fatal("world-readable Hook secret store was accepted")
	}
}

func TestFileSecretStoreRejectsSymlinkAndHeaderInjection(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "actual.json")
	if err := os.WriteFile(target, []byte(`{"version":1,"secrets":{"api-token":"safe"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(configDir, "hook-secrets.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileSecretStore(root).ResolveHookSecret(context.Background(), "api-token"); err == nil {
		t.Fatal("symlink Hook secret store was accepted")
	}

	root = t.TempDir()
	configDir = filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "hook-secrets.json"), []byte(`{"version":1,"secrets":{"api-token":"safe\r\nInjected: yes"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileSecretStore(root).ResolveHookSecret(context.Background(), "api-token"); err == nil {
		t.Fatal("header injection secret was accepted")
	}
}
