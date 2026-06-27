package envguard

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegisterCredentialFileRejectsTraversal(t *testing.T) {
	home := t.TempDir()

	_, err := NewSession().RegisterCredentialFile(home, "../secrets/token.json")
	if !errors.Is(err, ErrPathTraversal) {
		t.Fatalf("expected ErrPathTraversal, got %v", err)
	}
}

func TestRegisterCredentialFileRejectsAbsolutePath(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "token.json")
	if err := os.WriteFile(path, []byte("secret"), 0600); err != nil {
		t.Fatalf("failed to write credential file: %v", err)
	}

	_, err := NewSession().RegisterCredentialFile(home, path)
	if !errors.Is(err, ErrAbsolutePath) {
		t.Fatalf("expected ErrAbsolutePath, got %v", err)
	}
}

func TestRegisterCredentialFileRejectsSymlinkEscape(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "token.json")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0600); err != nil {
		t.Fatalf("failed to write outside file: %v", err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(home, "token-link.json")); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	_, err := NewSession().RegisterCredentialFile(home, "token-link.json")
	if !errors.Is(err, ErrPathEscapesHome) {
		t.Fatalf("expected ErrPathEscapesHome, got %v", err)
	}
}

func TestRegisterCredentialFileAcceptsHomeRelativeFile(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "providers"), 0700); err != nil {
		t.Fatalf("failed to create providers dir: %v", err)
	}
	path := filepath.Join(home, "providers", "token.json")
	if err := os.WriteFile(path, []byte("secret"), 0600); err != nil {
		t.Fatalf("failed to write credential file: %v", err)
	}

	session := NewSession()
	mount, err := session.RegisterCredentialFileWithMountBase(home, "providers/token.json", "/sandbox/home")
	if err != nil {
		t.Fatalf("RegisterCredentialFileWithMountBase returned error: %v", err)
	}

	if mount.RelativePath() != "providers/token.json" {
		t.Fatalf("unexpected relative path: %q", mount.RelativePath())
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("failed to resolve credential file path: %v", err)
	}
	if mount.HostPath() != resolvedPath {
		t.Fatalf("unexpected host path: %q", mount.HostPath())
	}
	if mount.MountPath() != "/sandbox/home/providers/token.json" {
		t.Fatalf("unexpected mount path: %q", mount.MountPath())
	}
	if got := session.CredentialFileMounts(); len(got) != 1 || got[0].RelativePath() != "providers/token.json" {
		t.Fatalf("unexpected registered mounts: %#v", got)
	}
}

func TestCredentialFileMetadataIsSanitized(t *testing.T) {
	home := t.TempDir()
	secret := "sk-test-secret-value"
	path := filepath.Join(home, "token.json")
	if err := os.WriteFile(path, []byte(secret), 0600); err != nil {
		t.Fatalf("failed to write credential file: %v", err)
	}

	mount, err := NewSession().RegisterCredentialFile(home, "token.json")
	if err != nil {
		t.Fatalf("RegisterCredentialFile returned error: %v", err)
	}

	metadataJSON, err := json.Marshal(mount)
	if err != nil {
		t.Fatalf("failed to marshal mount: %v", err)
	}
	combined := strings.Join([]string{
		fmt.Sprint(mount),
		fmt.Sprint(mount.Metadata()),
		string(metadataJSON),
	}, "\n")

	if strings.Contains(combined, secret) {
		t.Fatalf("metadata leaked secret content: %s", combined)
	}
	if strings.Contains(combined, mount.HostPath()) {
		t.Fatalf("metadata leaked host path: %s", combined)
	}
	if !strings.Contains(combined, "token.json") {
		t.Fatalf("expected metadata to include sanitized relative path: %s", combined)
	}
}
