package applypatch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
	"awaken/src/tool/trash"
)

// testTrashAdapter moves files into a per-test temp directory using
// os.Rename, mirroring the real mac/windows adapters' semantics without
// touching the developer's real ~/.Trash. It is installed via
// useTestTrashAdapter so apply_patch's delete path can be exercised in
// tests without side effects on the host.
type testTrashAdapter struct {
	trashDir string
	calls    []string
}

func (a *testTrashAdapter) TrashPath(path string) (trash.TrashResult, error) {
	a.calls = append(a.calls, path)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return trash.TrashResult{}, err
	}
	if err := os.MkdirAll(a.trashDir, 0o700); err != nil {
		return trash.TrashResult{}, err
	}
	target := filepath.Join(a.trashDir, filepath.Base(absPath))
	if _, err := os.Stat(target); err == nil {
		// Disambiguate collisions so parallel deletes don't clobber.
		target = filepath.Join(a.trashDir, strings.ReplaceAll(filepath.Base(absPath), ".", "_")+"_"+filepath.Base(filepath.Dir(absPath)))
	}
	if err := os.Rename(absPath, target); err != nil {
		return trash.TrashResult{}, err
	}
	return trash.TrashResult{
		OriginalPath: absPath,
		TrashedPath:  target,
		Method:       "test-rename",
	}, nil
}

// useTestTrashAdapter installs a testTrashAdapter backed by a per-test
// temp directory and restores the default adapter on cleanup. Tests that
// exercise the delete path should call this to avoid polluting ~/.Trash.
func useTestTrashAdapter(t *testing.T) *testTrashAdapter {
	t.Helper()
	adapter := &testTrashAdapter{trashDir: filepath.Join(t.TempDir(), "trash")}
	trash.SetAdapter(adapter)
	t.Cleanup(func() { trash.SetAdapter(nil) })
	return adapter
}

// TestDeleteFileMovesToTrash verifies that deleteFile routes through the
// TrashAdapter rather than os.Remove: the original path no longer exists
// and the trashed file is recoverable in the adapter's trash dir.
func TestDeleteFileMovesToTrash(t *testing.T) {
	adapter := useTestTrashAdapter(t)

	src := filepath.Join(t.TempDir(), "doomed.txt")
	if err := os.WriteFile(src, []byte("bye"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := deleteFile(src)
	if err != nil {
		t.Fatalf("deleteFile failed: %v", err)
	}

	if _, statErr := os.Stat(src); !os.IsNotExist(statErr) {
		t.Fatalf("expected original path gone, stat err=%v", statErr)
	}
	if result.TrashedPath == "" {
		t.Fatal("expected TrashedPath to be populated")
	}
	if _, statErr := os.Stat(result.TrashedPath); statErr != nil {
		t.Fatalf("trashed file missing at %s: %v", result.TrashedPath, statErr)
	}
	if len(adapter.calls) != 1 {
		t.Fatalf("expected 1 TrashPath call, got %d", len(adapter.calls))
	}
}

// TestDeleteFileFailsClosedWhenAdapterErrors verifies that when the
// TrashAdapter returns an error, deleteFile does NOT fall back to
// os.Remove — the file must remain in place.
func TestDeleteFileFailsClosedWhenAdapterErrors(t *testing.T) {
	useTestTrashAdapter(t)
	trash.SetAdapter(failingTrashAdapter{})

	src := filepath.Join(t.TempDir(), "keep.txt")
	if err := os.WriteFile(src, []byte("stay"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	_, err := deleteFile(src)
	if err == nil {
		t.Fatal("expected deleteFile to fail when adapter errors")
	}
	if !strings.Contains(err.Error(), "trash unavailable") {
		t.Fatalf("expected adapter error to propagate, got %v", err)
	}
	// File must still exist — no os.Remove fallback.
	if _, statErr := os.Stat(src); statErr != nil {
		t.Fatalf("expected file to still exist after failed trash, got %v", statErr)
	}
}

type failingTrashAdapter struct{}

func (failingTrashAdapter) TrashPath(path string) (trash.TrashResult, error) {
	return trash.TrashResult{}, fmt.Errorf("trash unavailable for %s", path)
}

// TestApplyPatchDeleteSurfacesTrashHint verifies that a successful delete
// via apply_patch includes the "已移到回收站" hint in the tool output so
// users can see the file was trashed, not permanently deleted.
func TestApplyPatchDeleteSurfacesTrashHint(t *testing.T) {
	useTestTrashAdapter(t)
	workspace := t.TempDir()
	permissionContext := tool.NewPermissionContext(workspace, nil)
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)

	target := filepath.Join(workspace, "note.md")
	writeSeedFile(t, target, "old\n")

	out, err := (&ApplyPatchTool{}).Execute(ctx, "*** Begin Patch\n"+
		"*** Delete File: note.md\n"+
		"*** End Patch")
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if !strings.Contains(out, "已移到回收站") {
		t.Fatalf("expected trash hint in summary, got %q", out)
	}
	if !strings.Contains(out, "D ") {
		t.Fatalf("expected D marker in summary, got %q", out)
	}
	assertPathMissing(t, target)
}

// TestApplyPatchDeleteNormalFileMovesToTrash verifies the end-to-end
// flow: deleting a normal workspace file through apply_patch moves it
// to trash and the file is no longer at the original path.
func TestApplyPatchDeleteNormalFileMovesToTrash(t *testing.T) {
	adapter := useTestTrashAdapter(t)
	workspace := t.TempDir()
	permissionContext := tool.NewPermissionContext(workspace, nil)
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)

	target := filepath.Join(workspace, "deleteme.txt")
	writeSeedFile(t, target, "payload\n")

	_, err := (&ApplyPatchTool{}).Execute(ctx, "*** Begin Patch\n"+
		"*** Delete File: deleteme.txt\n"+
		"*** End Patch")
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	assertPathMissing(t, target)
	if len(adapter.calls) != 1 {
		t.Fatalf("expected exactly 1 trash call, got %d", len(adapter.calls))
	}
	// The file should be recoverable in the trash dir.
	entries, err := os.ReadDir(adapter.trashDir)
	if err != nil {
		t.Fatalf("read trash dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 trashed file, got %d", len(entries))
	}
}

// TestApplyPatchDeleteRejectsSensitiveGitConfig verifies that deleting a
// sensitive path (.git/config) is rejected by validateMutationPath before
// the trash adapter is invoked, and the file is NOT trashed or removed.
func TestApplyPatchDeleteRejectsSensitiveGitConfig(t *testing.T) {
	withIsolatedHomeForApplyPatch(t)
	adapter := useTestTrashAdapter(t)
	workspace := t.TempDir()
	permissionContext := tool.NewPermissionContext(workspace, nil)
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)

	// Seed .git/config so the delete target exists.
	target := filepath.Join(workspace, ".git", "config")
	writeSeedFile(t, target, "[core]\n")

	_, err := (&ApplyPatchTool{}).Execute(ctx, "*** Begin Patch\n"+
		"*** Delete File: .git/config\n"+
		"*** End Patch")
	if err == nil || !strings.Contains(err.Error(), "sensitive path") {
		t.Fatalf("expected sensitive path error, got %v", err)
	}
	// File must still exist — not trashed, not removed.
	assertFileContent(t, target, "[core]\n")
	if len(adapter.calls) != 0 {
		t.Fatalf("expected trash adapter to NOT be called, got %d calls", len(adapter.calls))
	}
}

// TestApplyPatchDeleteRejectsSensitiveEnvFile verifies that deleting a
// .env file is rejected by the sensitive-path guard.
func TestApplyPatchDeleteRejectsSensitiveEnvFile(t *testing.T) {
	withIsolatedHomeForApplyPatch(t)
	adapter := useTestTrashAdapter(t)
	workspace := t.TempDir()
	permissionContext := tool.NewPermissionContext(workspace, nil)
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)

	target := filepath.Join(workspace, ".env")
	writeSeedFile(t, target, "SECRET=leaked\n")

	_, err := (&ApplyPatchTool{}).Execute(ctx, "*** Begin Patch\n"+
		"*** Delete File: .env\n"+
		"*** End Patch")
	if err == nil || !strings.Contains(err.Error(), "sensitive path") {
		t.Fatalf("expected sensitive path error, got %v", err)
	}
	assertFileContent(t, target, "SECRET=leaked\n")
	if len(adapter.calls) != 0 {
		t.Fatalf("expected trash adapter to NOT be called, got %d calls", len(adapter.calls))
	}
}

// TestApplyPatchDeleteRejectsSensitiveKeyFile verifies that deleting a
// private key file is rejected by the sensitive-path guard.
func TestApplyPatchDeleteRejectsSensitiveKeyFile(t *testing.T) {
	withIsolatedHomeForApplyPatch(t)
	adapter := useTestTrashAdapter(t)
	workspace := t.TempDir()
	permissionContext := tool.NewPermissionContext(workspace, nil)
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)

	for _, rel := range []string{"id_rsa", "server.pem", "deploy/cert.crt"} {
		target := filepath.Join(workspace, rel)
		writeSeedFile(t, target, "PRIVATE\n")
		_, err := (&ApplyPatchTool{}).Execute(ctx, "*** Begin Patch\n"+
			"*** Delete File: "+rel+"\n"+
			"*** End Patch")
		if err == nil || !strings.Contains(err.Error(), "sensitive path") {
			t.Fatalf("expected sensitive path error for %q, got %v", rel, err)
		}
		assertFileContent(t, target, "PRIVATE\n")
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("expected trash adapter to NOT be called, got %d calls", len(adapter.calls))
	}
}

// TestApplyPatchMoveDeletesOriginalViaTrash verifies that a move
// operation trashes the original file path through the TrashAdapter
// rather than os.Remove.
func TestApplyPatchMoveDeletesOriginalViaTrash(t *testing.T) {
	adapter := useTestTrashAdapter(t)
	workspace := t.TempDir()
	permissionContext := tool.NewPermissionContext(workspace, nil)
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)

	oldPath := filepath.Join(workspace, "old", "name.txt")
	newPath := filepath.Join(workspace, "renamed", "name.txt")
	writeSeedFile(t, oldPath, "old\n")

	_, err := (&ApplyPatchTool{}).Execute(ctx, "*** Begin Patch\n"+
		"*** Update File: old/name.txt\n"+
		"*** Move to: renamed/name.txt\n"+
		"@@\n"+
		"-old\n"+
		"+new\n"+
		"*** End Patch")
	if err != nil {
		t.Fatalf("move failed: %v", err)
	}
	assertPathMissing(t, oldPath)
	assertFileContent(t, newPath, "new\n")
	if len(adapter.calls) != 1 {
		t.Fatalf("expected exactly 1 trash call for moved original, got %d", len(adapter.calls))
	}
}
