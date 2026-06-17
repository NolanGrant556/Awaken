package tool

import (
	"path/filepath"
	"testing"
)

func TestPermissionContextDefaultsWritableRootsToWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	ctx := NewPermissionContext(root, nil)

	if !ctx.IsWithinWorkspace(filepath.Join(root, "file.txt")) {
		t.Fatalf("expected path under workspace root to be in workspace")
	}
	if !ctx.IsWithinWritableRoots(filepath.Join(root, "file.txt")) {
		t.Fatalf("expected workspace root to be writable by default")
	}
	if ctx.IsWithinWorkspace(filepath.Join(t.TempDir(), "file.txt")) {
		t.Fatalf("expected unrelated path to be outside workspace")
	}
}

func TestPermissionContextExplicitWritableRootsCanNarrowWrites(t *testing.T) {
	workspace := t.TempDir()
	writable := t.TempDir()
	ctx := NewPermissionContext(workspace, []string{writable})

	if !ctx.IsWithinWorkspace(filepath.Join(workspace, "file.txt")) {
		t.Fatalf("expected workspace path to be in workspace")
	}
	if ctx.IsWithinWritableRoots(filepath.Join(workspace, "file.txt")) {
		t.Fatalf("expected workspace path outside explicit writable roots to be non-writable")
	}
	if !ctx.IsWithinWritableRoots(filepath.Join(writable, "file.txt")) {
		t.Fatalf("expected explicit writable root to allow writes")
	}
}

func TestPermissionContextResolvesRelativePathsFromWorkspaceRoot(t *testing.T) {
	workspace := t.TempDir()
	ctx := NewPermissionContext(workspace, []string{"generated"})

	if got := ctx.ResolvePath("src/main.go"); got != filepath.Join(workspace, "src", "main.go") {
		t.Fatalf("unexpected resolved path: %q", got)
	}
	if !ctx.IsWithinWritableRoots("generated/file.txt") {
		t.Fatalf("expected relative writable root to be resolved under workspace")
	}
	if ctx.IsWithinWritableRoots("src/main.go") {
		t.Fatalf("expected relative path outside explicit writable roots to be non-writable")
	}
}
