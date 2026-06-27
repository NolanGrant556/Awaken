package filestate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateWriteAllowsUnchangedFullRead(t *testing.T) {
	registry, root := newTestRegistry(t)
	path := writeFile(t, root, "notes/todo.txt", "alpha\n")

	read, err := registry.RecordRead("task-a", "notes/todo.txt", false)
	if err != nil {
		t.Fatalf("record read: %v", err)
	}
	if read.Path != path {
		t.Fatalf("expected normalized read path %q, got %q", path, read.Path)
	}
	if read.Partial {
		t.Fatalf("expected full read record")
	}
	if !read.Snapshot.HashAvailable {
		t.Fatalf("expected small file hash to be available")
	}

	decision, err := registry.ValidateWrite("task-a", "notes/todo.txt", ValidateOptions{})
	if err != nil {
		t.Fatalf("validate write: %v", err)
	}
	assertDecision(t, decision, true, DecisionAllow)
}

func TestValidateWriteBlocksUnreadExistingFile(t *testing.T) {
	registry, root := newTestRegistry(t)
	writeFile(t, root, "notes/todo.txt", "alpha\n")

	decision, err := registry.ValidateWrite("task-a", "notes/todo.txt", ValidateOptions{})
	if err != nil {
		t.Fatalf("validate write: %v", err)
	}
	assertDecision(t, decision, false, DecisionBlockUnreadExisting)
	if !strings.Contains(decision.Reason, "has not read") {
		t.Fatalf("expected unread reason, got %q", decision.Reason)
	}
}

func TestValidateWriteBlocksStaleDiskChange(t *testing.T) {
	registry, root := newTestRegistry(t)
	path := writeFile(t, root, "notes/todo.txt", "alpha\n")

	if _, err := registry.RecordRead("task-a", "notes/todo.txt", false); err != nil {
		t.Fatalf("record read: %v", err)
	}
	if err := os.WriteFile(path, []byte("bravo\n"), 0644); err != nil {
		t.Fatalf("external write: %v", err)
	}

	decision, err := registry.ValidateWrite("task-a", "notes/todo.txt", ValidateOptions{})
	if err != nil {
		t.Fatalf("validate write: %v", err)
	}
	assertDecision(t, decision, false, DecisionBlockDiskChanged)
	if !strings.Contains(decision.Reason, "hash") {
		t.Fatalf("expected hash drift reason, got %q", decision.Reason)
	}
}

func TestValidateWriteBlocksPartialRead(t *testing.T) {
	registry, root := newTestRegistry(t)
	writeFile(t, root, "notes/todo.txt", "alpha\nbeta\n")

	if _, err := registry.RecordRead("task-a", "notes/todo.txt", true); err != nil {
		t.Fatalf("record read: %v", err)
	}

	decision, err := registry.ValidateWrite("task-a", "notes/todo.txt", ValidateOptions{})
	if err != nil {
		t.Fatalf("validate write: %v", err)
	}
	assertDecision(t, decision, false, DecisionBlockPartialRead)
	if decision.Read == nil || !decision.Read.Partial {
		t.Fatalf("expected partial read details in decision")
	}
}

func TestValidateWriteBlocksOtherTaskWriteConflict(t *testing.T) {
	registry, root := newTestRegistry(t)
	path := writeFile(t, root, "notes/todo.txt", "alpha\n")

	if _, err := registry.RecordRead("task-a", "notes/todo.txt", false); err != nil {
		t.Fatalf("record read: %v", err)
	}
	if err := os.WriteFile(path, []byte("bravo\n"), 0644); err != nil {
		t.Fatalf("task-b write: %v", err)
	}
	if _, err := registry.RecordWrite("task-b", "notes/todo.txt"); err != nil {
		t.Fatalf("record write: %v", err)
	}

	decision, err := registry.ValidateWrite("task-a", "notes/todo.txt", ValidateOptions{})
	if err != nil {
		t.Fatalf("validate write: %v", err)
	}
	assertDecision(t, decision, false, DecisionBlockOtherTaskWrite)
	if decision.LastWriter == nil || decision.LastWriter.TaskID != "task-b" {
		t.Fatalf("expected last writer task-b, got %#v", decision.LastWriter)
	}
}

func TestValidateWriteAllowsNewFileWhenRequested(t *testing.T) {
	registry, _ := newTestRegistry(t)

	decision, err := registry.ValidateWrite("task-a", "notes/new.txt", ValidateOptions{})
	if err != nil {
		t.Fatalf("validate write: %v", err)
	}
	assertDecision(t, decision, false, DecisionBlockNewFile)

	decision, err = registry.ValidateWrite("task-a", "notes/new.txt", ValidateOptions{AllowNewFile: true})
	if err != nil {
		t.Fatalf("validate write with allow new file: %v", err)
	}
	assertDecision(t, decision, true, DecisionAllowNewFile)
}

func TestValidateWriteAllowsExplicitOverride(t *testing.T) {
	registry, root := newTestRegistry(t)
	writeFile(t, root, "notes/todo.txt", "alpha\n")

	if _, err := registry.RecordRead("task-a", "notes/todo.txt", true); err != nil {
		t.Fatalf("record read: %v", err)
	}

	decision, err := registry.ValidateWrite("task-a", "notes/todo.txt", ValidateOptions{Override: true})
	if err != nil {
		t.Fatalf("validate write: %v", err)
	}
	assertDecision(t, decision, true, DecisionAllowOverride)
	if !decision.Override {
		t.Fatalf("expected override flag")
	}
}

func TestRecordWriteAllowsSameWriterWhenRequested(t *testing.T) {
	registry, root := newTestRegistry(t)
	path := writeFile(t, root, "notes/todo.txt", "alpha\n")

	if err := os.WriteFile(path, []byte("bravo\n"), 0644); err != nil {
		t.Fatalf("same task write: %v", err)
	}
	if _, err := registry.RecordWrite("task-a", "notes/todo.txt"); err != nil {
		t.Fatalf("record write: %v", err)
	}

	decision, err := registry.ValidateWrite("task-a", "notes/todo.txt", ValidateOptions{AllowSameWriter: true})
	if err != nil {
		t.Fatalf("validate write: %v", err)
	}
	assertDecision(t, decision, true, DecisionAllow)
}

func TestValidateWriteAllowsSameWriterWithoutReadWhenRequested(t *testing.T) {
	registry, root := newTestRegistry(t)
	path := writeFile(t, root, "notes/todo.txt", "alpha\n")

	write, err := registry.RecordWrite("task-a", "notes/todo.txt")
	if err != nil {
		t.Fatalf("record write: %v", err)
	}
	registry.mu.Lock()
	delete(registry.reads["task-a"], path)
	registry.writes[path] = write
	registry.mu.Unlock()

	decision, err := registry.ValidateWrite("task-a", "notes/todo.txt", ValidateOptions{})
	if err != nil {
		t.Fatalf("validate write: %v", err)
	}
	assertDecision(t, decision, false, DecisionBlockUnreadExisting)

	decision, err = registry.ValidateWrite("task-a", "notes/todo.txt", ValidateOptions{AllowSameWriter: true})
	if err != nil {
		t.Fatalf("validate write with same writer: %v", err)
	}
	assertDecision(t, decision, true, DecisionAllowSameWriter)
}

func TestRecordExternalSnapshot(t *testing.T) {
	registry, root := newTestRegistry(t)
	path := writeFile(t, root, "notes/todo.txt", "alpha\n")

	record, err := registry.RecordExternalSnapshot("notes/todo.txt")
	if err != nil {
		t.Fatalf("record external snapshot: %v", err)
	}
	if record.Path != path || !record.Snapshot.Exists {
		t.Fatalf("unexpected external snapshot: %#v", record)
	}

	stored, ok, err := registry.ExternalSnapshot("notes/todo.txt")
	if err != nil {
		t.Fatalf("lookup external snapshot: %v", err)
	}
	if !ok || stored.Path != path {
		t.Fatalf("expected stored external snapshot, got ok=%v record=%#v", ok, stored)
	}
}

func TestSnapshotSkipsHashAboveThreshold(t *testing.T) {
	root := t.TempDir()
	registry, err := NewRegistry(root, WithMaxHashBytes(4))
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	writeFile(t, root, "large.txt", "0123456789")

	read, err := registry.RecordRead("task-a", "large.txt", false)
	if err != nil {
		t.Fatalf("record read: %v", err)
	}
	if read.Snapshot.HashAvailable {
		t.Fatalf("expected hash to be skipped above threshold")
	}
	if !read.Snapshot.HashSkipped {
		t.Fatalf("expected hash skipped flag")
	}
}

func TestNormalizePathBlocksTraversalAndSymlinkEscape(t *testing.T) {
	registry, root := newTestRegistry(t)
	inside := writeFile(t, root, "nested/file.txt", "alpha\n")

	normalized, err := registry.NormalizePath("nested/../nested/file.txt")
	if err != nil {
		t.Fatalf("normalize safe path: %v", err)
	}
	if normalized != inside {
		t.Fatalf("expected normalized path %q, got %q", inside, normalized)
	}

	if _, err := registry.NormalizePath("../escape.txt"); err == nil {
		t.Fatalf("expected traversal path to be rejected")
	}

	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "outside-link")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	if _, err := registry.NormalizePath("outside-link/new.txt"); err == nil {
		t.Fatalf("expected symlink escape to be rejected")
	}
}

func newTestRegistry(t *testing.T) (*Registry, string) {
	t.Helper()
	root := t.TempDir()
	registry, err := NewRegistry(root)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	return registry, root
}

func writeFile(t *testing.T, root string, rel string, content string) string {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}
	return filepath.Clean(realPath)
}

func assertDecision(t *testing.T, decision Decision, allowed bool, code DecisionCode) {
	t.Helper()
	if decision.Allowed != allowed || decision.Code != code {
		t.Fatalf("expected allowed=%v code=%s, got allowed=%v code=%s reason=%q", allowed, code, decision.Allowed, decision.Code, decision.Reason)
	}
}
