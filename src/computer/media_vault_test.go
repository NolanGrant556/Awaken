package computer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileMediaVaultRoundTripAndSessionIsolation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	vault, err := NewFileMediaVault(VaultOptions{RootDir: root})
	if err != nil {
		t.Fatalf("create media store: %v", err)
	}
	data := []byte("screenshot-pixels")
	ref, err := vault.Store(ctx, "session-1", MediaKindScreenshot, data)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	wantHash := sha256.Sum256(data)
	if ref.SHA256 != hex.EncodeToString(wantHash[:]) {
		t.Fatal("sha256 mismatch")
	}
	stored, err := os.ReadFile(filepath.Join(root, ref.StorageKey))
	if err != nil || string(stored) != string(data) {
		t.Fatalf("expected private plaintext media file, err=%v", err)
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		t.Fatalf("stat media root: %v", err)
	}
	if rootInfo.Mode().Perm() != 0o700 {
		t.Fatalf("media root permissions = %v", rootInfo.Mode().Perm())
	}
	fileInfo, err := os.Stat(filepath.Join(root, ref.StorageKey))
	if err != nil {
		t.Fatalf("stat media file: %v", err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("media file permissions = %v", fileInfo.Mode().Perm())
	}
	if _, err := vault.Read(ctx, "other-session", ref); err == nil {
		t.Fatal("expected cross-session read to fail")
	}
	read, err := vault.Read(ctx, "session-1", ref)
	if err != nil || string(read) != string(data) {
		t.Fatalf("round trip failed: %v", err)
	}
}

func TestFileMediaVaultRejectsEscapingStorageKey(t *testing.T) {
	vault, err := NewFileMediaVault(VaultOptions{RootDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = vault.Read(context.Background(), "session-1", MediaRef{
		SessionID:  "session-1",
		StorageKey: "../../outside",
	})
	if err == nil {
		t.Fatal("expected escaping storage key to fail")
	}
}

func TestFileMediaVaultAuditAndPurge(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	var auditedRef MediaRef
	var auditedSession SessionID
	var deletedRef MediaRef
	var deleteReason string
	vault, err := NewFileMediaVault(VaultOptions{
		RootDir: root,
		OnAccess: func(_ context.Context, sessionID SessionID, ref MediaRef) {
			auditedSession, auditedRef = sessionID, ref
		},
		OnDelete: func(_ context.Context, _ SessionID, ref MediaRef, reason string) {
			deletedRef, deleteReason = ref, reason
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := vault.Store(ctx, "session-purge", MediaKindAccessibilityTree, []byte("tree"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Read(ctx, "session-purge", ref); err != nil {
		t.Fatal(err)
	}
	if auditedSession != "session-purge" || auditedRef.StorageKey != ref.StorageKey {
		t.Fatal("access audit mismatch")
	}
	if err := vault.PurgeSession(ctx, "session-purge"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ref.StorageKey)); !os.IsNotExist(err) {
		t.Fatal("expected session media to be removed")
	}
	if deletedRef.StorageKey != ref.StorageKey || deleteReason != "session_purge" {
		t.Fatalf("delete audit mismatch: ref=%+v reason=%q", deletedRef, deleteReason)
	}
}

func TestFileMediaVaultAgeAndCapacityCleanup(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	var deleted int
	vault, err := NewFileMediaVault(VaultOptions{
		RootDir:  root,
		OnDelete: func(context.Context, SessionID, MediaRef, string) { deleted++ },
		Retention: RetentionOptions{
			MaxAge:          10 * time.Hour,
			MaxTotalBytes:   4,
			CleanupInterval: time.Hour,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer vault.Close()

	refs := make([]MediaRef, 3)
	for i, payload := range []string{"aaa", "bbb", "ccc"} {
		refs[i], err = vault.Store(ctx, "session-cap", MediaKindScreenshot, []byte(payload))
		if err != nil {
			t.Fatal(err)
		}
		mtime := time.Now().Add(-time.Duration(3-i) * time.Hour)
		if err := os.Chtimes(filepath.Join(root, refs[i].StorageKey), mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	if err := vault.(*fileMediaVault).cleanupOnce(ctx); err != nil {
		t.Fatal(err)
	}
	for i, ref := range refs {
		_, statErr := os.Stat(filepath.Join(root, ref.StorageKey))
		if i == 2 && statErr != nil {
			t.Fatalf("expected newest file to remain: %v", statErr)
		}
		if i != 2 && !os.IsNotExist(statErr) {
			t.Fatalf("expected old file %d to be removed", i)
		}
	}
	if deleted != 2 {
		t.Fatalf("expected two delete audits, got %d", deleted)
	}
}

func TestFileMediaVaultCapacityGracePeriod(t *testing.T) {
	root := t.TempDir()
	vault, err := NewFileMediaVault(VaultOptions{
		RootDir:   root,
		Retention: RetentionOptions{MaxTotalBytes: 1, CleanupInterval: time.Hour},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer vault.Close()
	ref, err := vault.Store(context.Background(), "session-grace", MediaKindScreenshot, []byte("fresh"))
	if err != nil {
		t.Fatal(err)
	}
	if err := vault.(*fileMediaVault).cleanupOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ref.StorageKey)); err != nil {
		t.Fatal("expected fresh file to survive capacity cleanup")
	}
}

func TestFileMediaVaultCloseIsIdempotent(t *testing.T) {
	vault, err := NewFileMediaVault(VaultOptions{
		RootDir:   t.TempDir(),
		Retention: RetentionOptions{MaxAge: time.Hour, CleanupInterval: time.Millisecond},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := vault.Close(); err != nil {
		t.Fatal(err)
	}
	if err := vault.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Store(context.Background(), "session-closed", MediaKindScreenshot, []byte("late")); err == nil {
		t.Fatal("expected store after close to fail")
	}
}
