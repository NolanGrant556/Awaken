package tool

import (
	"os"
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

func TestPermissionContextResolvesSymlinksForWorkspaceBoundary(t *testing.T) {
	workspace := t.TempDir()
	ctx := NewPermissionContext(workspace, nil)

	insideFile := filepath.Join(workspace, "inside.txt")
	if err := os.WriteFile(insideFile, []byte("inside"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	if !ctx.IsWithinWorkspaceResolved("inside.txt") {
		t.Fatalf("expected regular file inside workspace to be allowed")
	}

	externalLink := filepath.Join(workspace, "external-link")
	if err := os.Symlink(outsideFile, externalLink); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}
	if ctx.IsWithinWorkspaceResolved("external-link") {
		t.Fatalf("expected symlink to external file to be rejected")
	}

	relativeLink := filepath.Join(workspace, "relative-link")
	if err := os.Symlink("../outside.txt", relativeLink); err != nil {
		t.Fatalf("failed to create relative symlink: %v", err)
	}
	if ctx.IsWithinWorkspaceResolved("relative-link") {
		t.Fatalf("expected relative symlink escaping workspace to be rejected")
	}

	internalLink := filepath.Join(workspace, "internal-link")
	if err := os.Symlink("inside.txt", internalLink); err != nil {
		t.Fatalf("failed to create internal symlink: %v", err)
	}
	if !ctx.IsWithinWorkspaceResolved("internal-link") {
		t.Fatalf("expected symlink pointing inside workspace to be allowed")
	}

	if ctx.IsWithinWorkspaceResolved("does-not-exist.txt") {
		t.Fatalf("expected non-existent path to be rejected conservatively")
	}

	resolved, err := ctx.ResolvePathChecked("inside.txt")
	if err != nil {
		t.Fatalf("expected ResolvePathChecked to succeed for inside file: %v", err)
	}
	if resolved != insideFile {
		t.Fatalf("expected resolved path %q, got %q", insideFile, resolved)
	}

	if _, err := ctx.ResolvePathChecked("external-link"); err == nil {
		t.Fatalf("expected ResolvePathChecked to fail for external symlink")
	}
}
