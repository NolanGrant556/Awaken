package applypatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestApplyPatchAddsUpdatesAndDeletesFiles(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "note.md")

	out := applyPatchText(t, "*** Begin Patch\n"+
		"*** Add File: "+target+"\n"+
		"+hello\n"+
		"+world\n"+
		"*** End Patch")
	if !strings.Contains(out, "A "+target) {
		t.Fatalf("unexpected add summary: %q", out)
	}
	assertFileContent(t, target, "hello\nworld\n")

	out = applyPatchText(t, "*** Begin Patch\n"+
		"*** Update File: "+target+"\n"+
		"@@\n"+
		" hello\n"+
		"-world\n"+
		"+Awaken\n"+
		"*** End Patch")
	if !strings.Contains(out, "M "+target) {
		t.Fatalf("unexpected update summary: %q", out)
	}
	assertFileContent(t, target, "hello\nAwaken\n")

	out = applyPatchText(t, "*** Begin Patch\n"+
		"*** Delete File: "+target+"\n"+
		"*** End Patch")
	if !strings.Contains(out, "D "+target) {
		t.Fatalf("unexpected delete summary: %q", out)
	}
	assertPathMissing(t, target)
}

func TestApplyPatchSupportsMultipleHunks(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.txt")
	second := filepath.Join(root, "second.txt")

	out := applyPatchText(t, "*** Begin Patch\n"+
		"*** Add File: "+first+"\n"+
		"+one\n"+
		"*** Add File: "+second+"\n"+
		"+two\n"+
		"*** End Patch")
	if !strings.Contains(out, "A "+first) || !strings.Contains(out, "A "+second) {
		t.Fatalf("unexpected summary: %q", out)
	}
	assertFileContent(t, first, "one\n")
	assertFileContent(t, second, "two\n")
}

func TestApplyPatchAddOverwritesExistingFile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "duplicate.txt")
	if err := os.WriteFile(target, []byte("old content\n"), 0644); err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}

	applyPatchText(t, "*** Begin Patch\n"+
		"*** Add File: "+target+"\n"+
		"+new content\n"+
		"*** End Patch")
	assertFileContent(t, target, "new content\n")
}

func TestApplyPatchMoveToNewDirectory(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old", "name.txt")
	otherPath := filepath.Join(root, "old", "other.txt")
	newPath := filepath.Join(root, "renamed", "dir", "name.txt")
	writeSeedFile(t, oldPath, "old content\n")
	writeSeedFile(t, otherPath, "keep\n")

	out := applyPatchText(t, "*** Begin Patch\n"+
		"*** Update File: "+oldPath+"\n"+
		"*** Move to: "+newPath+"\n"+
		"@@\n"+
		"-old content\n"+
		"+new content\n"+
		"*** End Patch")
	if !strings.Contains(out, "M "+newPath) {
		t.Fatalf("unexpected move summary: %q", out)
	}
	assertPathMissing(t, oldPath)
	assertFileContent(t, newPath, "new content\n")
	assertFileContent(t, otherPath, "keep\n")
}

func TestApplyPatchMoveOverwritesExistingDestination(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old", "name.txt")
	newPath := filepath.Join(root, "renamed", "dir", "name.txt")
	writeSeedFile(t, oldPath, "from\n")
	writeSeedFile(t, newPath, "destination\n")

	applyPatchText(t, "*** Begin Patch\n"+
		"*** Update File: "+oldPath+"\n"+
		"*** Move to: "+newPath+"\n"+
		"@@\n"+
		"-from\n"+
		"+new\n"+
		"*** End Patch")
	assertPathMissing(t, oldPath)
	assertFileContent(t, newPath, "new\n")
}

func TestApplyPatchEndOfFileMarker(t *testing.T) {
	target := filepath.Join(t.TempDir(), "tail.txt")
	writeSeedFile(t, target, "first\nsecond\n")

	applyPatchText(t, "*** Begin Patch\n"+
		"*** Update File: "+target+"\n"+
		"@@\n"+
		" first\n"+
		"-second\n"+
		"+second updated\n"+
		"*** End of File\n"+
		"*** End Patch")
	assertFileContent(t, target, "first\nsecond updated\n")
}

func TestApplyPatchPureAdditionUpdateChunk(t *testing.T) {
	target := filepath.Join(t.TempDir(), "input.txt")
	writeSeedFile(t, target, "base\n")

	applyPatchText(t, "*** Begin Patch\n"+
		"*** Update File: "+target+"\n"+
		"@@\n"+
		"+added line 1\n"+
		"+added line 2\n"+
		"*** End Patch")
	assertFileContent(t, target, "base\nadded line 1\nadded line 2\n")
}

func TestApplyPatchUpdateFileDeletionOnly(t *testing.T) {
	target := filepath.Join(t.TempDir(), "lines.txt")
	writeSeedFile(t, target, "line1\nline2\nline3\n")

	applyPatchText(t, "*** Begin Patch\n"+
		"*** Update File: "+target+"\n"+
		"@@\n"+
		" line1\n"+
		"-line2\n"+
		" line3\n"+
		"*** End Patch")
	assertFileContent(t, target, "line1\nline3\n")
}

func TestApplyPatchSupportsHeredocWrapper(t *testing.T) {
	target := filepath.Join(t.TempDir(), "wrapped.txt")

	applyPatchText(t, "<<'EOF'\n"+
		"*** Begin Patch\n"+
		"*** Add File: "+target+"\n"+
		"+wrapped\n"+
		"*** End Patch\n"+
		"EOF")
	assertFileContent(t, target, "wrapped\n")
}

func TestApplyPatchSupportsWhitespacePaddedMarkers(t *testing.T) {
	target := filepath.Join(t.TempDir(), "file.txt")
	writeSeedFile(t, target, "one\n")

	applyPatchText(t, " *** Begin Patch\n"+
		"  *** Update File: "+target+"\n"+
		"@@\n"+
		"-one\n"+
		"+two\n"+
		" *** End Patch ")
	assertFileContent(t, target, "two\n")
}

func TestApplyPatchSupportsContextHeader(t *testing.T) {
	target := filepath.Join(t.TempDir(), "context.txt")
	writeSeedFile(t, target, "before\nfunc target\nold\nend\n")

	applyPatchText(t, "*** Begin Patch\n"+
		"*** Update File: "+target+"\n"+
		"@@ func target\n"+
		"-old\n"+
		"+new\n"+
		"*** End Patch")
	assertFileContent(t, target, "before\nfunc target\nnew\nend\n")
}

func TestApplyPatchRejectsInvalidPatch(t *testing.T) {
	_, err := (&ApplyPatchTool{}).Execute(context.Background(), "*** Add File: x\n+bad")
	if err == nil || !strings.Contains(err.Error(), "Begin Patch") {
		t.Fatalf("expected invalid patch error, got %v", err)
	}
}

func TestApplyPatchDeleteMissingFileReturnsError(t *testing.T) {
	target := filepath.Join(t.TempDir(), "missing.txt")
	_, err := (&ApplyPatchTool{}).Execute(context.Background(), "*** Begin Patch\n"+
		"*** Delete File: "+target+"\n"+
		"*** End Patch")
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected missing delete error, got %v", err)
	}
}

func TestApplyPatchDeleteDirectoryReturnsError(t *testing.T) {
	target := filepath.Join(t.TempDir(), "dir")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatalf("failed to seed directory: %v", err)
	}
	_, err := (&ApplyPatchTool{}).Execute(context.Background(), "*** Begin Patch\n"+
		"*** Delete File: "+target+"\n"+
		"*** End Patch")
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("expected directory delete error, got %v", err)
	}
}

func TestApplyPatchPermissionAsksWithPatchSummary(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	permissionContext := tool.NewPermissionContext(root, nil)

	decision := (&ApplyPatchTool{}).CheckPermission(context.Background(), "*** Begin Patch\n"+
		"*** Add File: note.md\n"+
		"+hello\n"+
		"*** End Patch", permissionContext)

	if decision.Behavior != tool.PermissionAsk {
		t.Fatalf("expected apply_patch to ask, got %+v", decision)
	}
	if decision.ApprovalRequest.RiskTone != "warning" || !strings.Contains(decision.ApprovalRequest.Description, "A note.md") {
		t.Fatalf("unexpected approval request: %+v", decision.ApprovalRequest)
	}
}

func TestApplyPatchPermissionMarksDeleteAsDanger(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	permissionContext := tool.NewPermissionContext(root, nil)

	decision := (&ApplyPatchTool{}).CheckPermission(context.Background(), "*** Begin Patch\n"+
		"*** Delete File: note.md\n"+
		"*** End Patch", permissionContext)

	if decision.Behavior != tool.PermissionAsk || decision.ApprovalRequest.RiskTone != "danger" {
		t.Fatalf("expected delete patch to ask with danger risk, got %+v", decision)
	}
}

func TestApplyPatchPermissionUsesWritableRoots(t *testing.T) {
	workspace := t.TempDir()
	writable := t.TempDir()
	permissionContext := tool.NewPermissionContext(workspace, []string{writable})

	insideWritable := filepath.Join(writable, "note.md")
	decision := (&ApplyPatchTool{}).CheckPermission(context.Background(), "*** Begin Patch\n"+
		"*** Add File: "+insideWritable+"\n"+
		"+hello\n"+
		"*** End Patch", permissionContext)
	if decision.Behavior != tool.PermissionAsk || decision.ApprovalRequest.RiskTone != "warning" {
		t.Fatalf("expected writable root patch to ask with warning risk, got %+v", decision)
	}

	outsideWritable := filepath.Join(workspace, "note.md")
	decision = (&ApplyPatchTool{}).CheckPermission(context.Background(), "*** Begin Patch\n"+
		"*** Add File: "+outsideWritable+"\n"+
		"+hello\n"+
		"*** End Patch", permissionContext)
	if decision.Behavior != tool.PermissionAsk || decision.ApprovalRequest.RiskTone != "danger" {
		t.Fatalf("expected outside writable roots patch to ask with danger risk, got %+v", decision)
	}
}

func TestApplyPatchExecutesRelativePathsUnderWorkspaceRoot(t *testing.T) {
	workspace := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(workspace, nil))

	out, err := (&ApplyPatchTool{}).Execute(ctx, "*** Begin Patch\n"+
		"*** Add File: nested/note.md\n"+
		"+hello\n"+
		"*** End Patch")
	if err != nil {
		t.Fatalf("patch failed: %v", err)
	}

	target := filepath.Join(workspace, "nested", "note.md")
	if !strings.Contains(out, "A "+target) {
		t.Fatalf("expected resolved path in summary, got %q", out)
	}
	assertFileContent(t, target, "hello\n")
}

func applyPatchText(t *testing.T, patch string) string {
	t.Helper()
	out, err := (&ApplyPatchTool{}).Execute(context.Background(), patch)
	if err != nil {
		t.Fatalf("patch failed: %v", err)
	}
	return out
}

func writeSeedFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create parent dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to seed file %s: %v", path, err)
	}
}

func assertFileContent(t *testing.T, path, expected string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	if string(content) != expected {
		t.Fatalf("expected %q, got %q", expected, string(content))
	}
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be missing, stat err=%v", path, err)
	}
}
