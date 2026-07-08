package trashfile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"awaken/src/tool"
	"awaken/src/tool/trash"
)

type recordingTrashAdapter struct {
	calls []string
	dir   string
}

func (a *recordingTrashAdapter) TrashPath(path string) (trash.TrashResult, error) {
	a.calls = append(a.calls, path)
	if err := os.MkdirAll(a.dir, 0o700); err != nil {
		return trash.TrashResult{}, err
	}
	target := filepath.Join(a.dir, filepath.Base(path))
	if err := os.Rename(path, target); err != nil {
		return trash.TrashResult{}, err
	}
	return trash.TrashResult{
		OriginalPath: path,
		TrashedPath:  target,
		Method:       "test-rename",
	}, nil
}

func TestTrashFileMovesPathThroughTrashAdapter(t *testing.T) {
	adapter := &recordingTrashAdapter{dir: filepath.Join(t.TempDir(), "trash")}
	trash.SetAdapter(adapter)
	t.Cleanup(func() { trash.SetAdapter(nil) })

	outsideRoot := t.TempDir()
	target := filepath.Join(outsideRoot, "outside.txt")
	if err := os.WriteFile(target, []byte("bye"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	trashTool := &TrashFileTool{}
	decision := trashTool.CheckPermission(
		context.Background(),
		`{"path":`+mustJSONQuote(t, target)+`}`,
		toolpkgPermissionContext(t),
	)
	if decision.Behavior != toolpkgPermissionAllow() {
		t.Fatalf("expected trash_file to be allowed for project-external path, got %+v", decision)
	}

	output, err := trashTool.Execute(context.Background(), `{"path":`+mustJSONQuote(t, target)+`}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected original path to be moved, stat err=%v", err)
	}
	var result trashFileResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if result.TrashedPath == "" || result.Method != "test-rename" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(adapter.calls) != 1 {
		t.Fatalf("expected one trash adapter call, got %d", len(adapter.calls))
	}
}

func TestTrashFileResolvesRelativeDirectoryThroughPermissionContext(t *testing.T) {
	adapter := &recordingTrashAdapter{dir: filepath.Join(t.TempDir(), "trash")}
	trash.SetAdapter(adapter)
	t.Cleanup(func() { trash.SetAdapter(nil) })

	workspace := t.TempDir()
	target := filepath.Join(workspace, "build")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("create target dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "artifact.txt"), []byte("bye"), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	permCtx := tool.NewPermissionContext(workspace, []string{workspace})
	ctx := tool.ContextWithPermissionContext(context.Background(), permCtx)
	trashTool := &TrashFileTool{}
	decision := trashTool.CheckPermission(context.Background(), `{"path":"build"}`, permCtx)
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("expected trash_file relative directory to be allowed, got %+v", decision)
	}
	if len(decision.Metadata.AffectedPaths) != 1 || decision.Metadata.AffectedPaths[0] != target {
		t.Fatalf("expected affected path %q, got %+v", target, decision.Metadata.AffectedPaths)
	}

	output, err := trashTool.Execute(ctx, `{"path":"build"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected original directory to be moved, stat err=%v", err)
	}
	var result trashFileResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if result.OriginalPath != target || result.TrashedPath == "" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(result.TrashedPath, "artifact.txt")); err != nil {
		t.Fatalf("expected directory contents to move to trash: %v", err)
	}
}

func mustJSONQuote(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return string(data)
}

func toolpkgPermissionContext(t *testing.T) tool.PermissionContext {
	t.Helper()
	root := t.TempDir()
	return tool.NewPermissionContext(root, []string{root})
}

func toolpkgPermissionAllow() tool.PermissionBehavior {
	return tool.PermissionAllow
}
