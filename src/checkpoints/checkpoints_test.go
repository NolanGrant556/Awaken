package checkpoints

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestStoreCreateSnapshotsExistingAndMissingFiles(t *testing.T) {
	root := t.TempDir()
	existingPath := filepath.Join(root, "note.md")
	if err := os.WriteFile(existingPath, []byte("before\n"), 0644); err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}

	store := NewStore()
	checkpoint, err := store.Create(context.Background(), tool.NewPermissionContext(root, nil), []string{
		"note.md",
		"new.md",
		"note.md",
	})
	if err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}

	if !strings.HasPrefix(checkpoint.ID, "checkpoint_") {
		t.Fatalf("unexpected checkpoint id %q", checkpoint.ID)
	}
	if checkpoint.WorkspaceRoot != root {
		t.Fatalf("expected workspace root %q, got %q", root, checkpoint.WorkspaceRoot)
	}
	if len(checkpoint.Snapshots) != 2 {
		t.Fatalf("expected 2 deduped snapshots, got %+v", checkpoint.Snapshots)
	}

	existing := checkpoint.Snapshots[0]
	if existing.Path != "note.md" || existing.AbsPath != existingPath || existing.State != FileStateExisting {
		t.Fatalf("unexpected existing snapshot: %+v", existing)
	}
	if string(existing.Content) != "before\n" || existing.Size != int64(len("before\n")) || existing.SHA256 == "" {
		t.Fatalf("existing snapshot did not capture content metadata: %+v", existing)
	}

	missing := checkpoint.Snapshots[1]
	if missing.Path != "new.md" || missing.State != FileStateMissing || len(missing.Content) != 0 {
		t.Fatalf("unexpected missing snapshot: %+v", missing)
	}

	if checkpoint.Summary().Existing != 1 || checkpoint.Summary().Missing != 1 {
		t.Fatalf("unexpected checkpoint summary: %+v", checkpoint.Summary())
	}

	stored, ok := store.Get(checkpoint.ID)
	if !ok {
		t.Fatalf("expected checkpoint to be stored")
	}
	stored.Snapshots[0].Content[0] = 'X'

	storedAgain, ok := store.Get(checkpoint.ID)
	if !ok {
		t.Fatalf("expected checkpoint to remain stored")
	}
	if string(storedAgain.Snapshots[0].Content) != "before\n" {
		t.Fatalf("store returned mutable snapshot content: %q", string(storedAgain.Snapshots[0].Content))
	}
}

func TestCreateRejectsDirectories(t *testing.T) {
	root := t.TempDir()
	dirPath := filepath.Join(root, "dir")
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		t.Fatalf("failed to seed dir: %v", err)
	}

	_, err := NewStore().Create(context.Background(), tool.NewPermissionContext(root, nil), []string{"dir"})
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("expected directory snapshot error, got %v", err)
	}
}

func TestCreateRejectsOutsideWritableRoots(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	_, err := NewStore().Create(context.Background(), tool.NewPermissionContext(root, []string{root}), []string{
		filepath.Join(outside, "note.md"),
	})
	if err == nil || !strings.Contains(err.Error(), "outside writable roots") {
		t.Fatalf("expected outside writable root error, got %v", err)
	}
}

func TestCreateRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret\n"), 0644); err != nil {
		t.Fatalf("failed to seed outside file: %v", err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(root, "link")); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	_, err := NewStore().Create(context.Background(), tool.NewPermissionContext(root, []string{root}), []string{"link"})
	if err == nil || !strings.Contains(err.Error(), "outside writable roots") {
		t.Fatalf("expected symlink escape error, got %v", err)
	}
}

func TestCreateAllowsSymlinkInsideWritableRoot(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("inside\n"), 0644); err != nil {
		t.Fatalf("failed to seed target: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(root, "link")); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	checkpoint, err := NewStore().Create(context.Background(), tool.NewPermissionContext(root, []string{root}), []string{"link"})
	if err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}
	if len(checkpoint.Snapshots) != 1 || checkpoint.Snapshots[0].State != FileStateExisting {
		t.Fatalf("unexpected checkpoint: %+v", checkpoint)
	}
	if string(checkpoint.Snapshots[0].Content) != "inside\n" {
		t.Fatalf("expected symlink target content, got %q", string(checkpoint.Snapshots[0].Content))
	}
}
