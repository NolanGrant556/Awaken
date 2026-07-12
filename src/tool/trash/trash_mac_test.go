//go:build darwin

package trash

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestTrashAdapterMacMovesRegularDirectoryAndReturnsReceipt(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "package")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "content.txt"), []byte("recoverable"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := TrashAdapterMac{trashDir: filepath.Join(root, "Trash")}
	receipt, err := adapter.TrashPath(source)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.OriginalPath != source || receipt.TrashedPath == "" || receipt.Method != "rename" {
		t.Fatalf("unexpected Trash receipt: %+v", receipt)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source still exists after Trash move: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(receipt.TrashedPath, "content.txt"))
	if err != nil || string(content) != "recoverable" {
		t.Fatalf("Trash receipt does not recover original bytes: content=%q err=%v", content, err)
	}
}

func TestTrashAdapterMacCrossVolumeFailsClosedWithoutRemovingSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("preserve"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := TrashAdapterMac{
		trashDir: filepath.Join(root, "trash"),
		rename: func(string, string) error {
			return syscall.EXDEV
		},
	}
	_, err := adapter.TrashPath(source)
	if !errors.Is(err, ErrCrossVolumeTrashUnsupported) {
		t.Fatalf("expected typed cross-volume failure, got %v", err)
	}
	content, readErr := os.ReadFile(source)
	if readErr != nil || string(content) != "preserve" {
		t.Fatalf("cross-volume failure changed source: content=%q err=%v", content, readErr)
	}
}

func TestTrashAdapterMacRejectsSymlinkBeforeRename(t *testing.T) {
	root := t.TempDir()
	realFile := filepath.Join(root, "real.txt")
	link := filepath.Join(root, "link.txt")
	if err := os.WriteFile(realFile, []byte("preserve"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realFile, link); err != nil {
		t.Fatal(err)
	}
	called := false
	adapter := TrashAdapterMac{
		trashDir: filepath.Join(root, "trash"),
		rename: func(string, string) error {
			called = true
			return nil
		},
	}
	if _, err := adapter.TrashPath(link); err == nil {
		t.Fatal("expected symlink rejection")
	}
	if called {
		t.Fatal("rename called for symlink")
	}
}
