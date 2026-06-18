package fileread

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestFileReadRawPath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("hello\nworld\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	out, err := (&FileReadTool{}).Execute(context.Background(), path)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out != "hello\nworld" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestFileReadJSONRange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	out, err := (&FileReadTool{}).Execute(context.Background(), `{"path":"`+escapePath(path)+`","offset":2,"limit":1}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out != "two\n\n[read_file truncated: start_line=2 returned_lines=1 max_bytes=262144]" {
		t.Fatalf("unexpected range output: %q", out)
	}
}

func TestFileReadMaxBytes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "large.txt")
	if err := os.WriteFile(path, []byte("123456789"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	out, err := (&FileReadTool{}).Execute(context.Background(), `{"path":"`+escapePath(path)+`","max_bytes":4}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out, "1234") || !strings.Contains(out, "truncated") {
		t.Fatalf("expected truncated output, got %q", out)
	}
}

func TestFileReadRejectsBinary(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "blob.dat")
	if err := os.WriteFile(path, []byte{0, 1, 2, 3}, 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	_, err := (&FileReadTool{}).Execute(context.Background(), `{"path":"`+escapePath(path)+`"}`)
	if err == nil || !strings.Contains(err.Error(), "二进制") {
		t.Fatalf("expected binary rejection, got %v", err)
	}
}

func TestFileReadResolvesRelativePath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "README.md")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	out, err := (&FileReadTool{}).Execute(ctx, `{"path":"README.md"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out != "hello" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestReadFileDeniesEtcHosts(t *testing.T) {
	workspaceRoot := t.TempDir()
	pc := tool.NewPermissionContext(workspaceRoot, []string{workspaceRoot})
	ctx := tool.ContextWithPermissionContext(context.Background(), pc)

	_, err := (&FileReadTool{}).Execute(ctx, `{"path":"/etc/hosts"}`)
	if err == nil || !strings.Contains(err.Error(), "outside workspace") {
		t.Fatalf("expected /etc/hosts to be denied, got %v", err)
	}
}

func TestReadFileBlocksTraversal(t *testing.T) {
	parent := t.TempDir()
	workspaceRoot := filepath.Join(parent, "workspace")
	if err := os.MkdirAll(workspaceRoot, 0755); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}
	outsideFile := filepath.Join(parent, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0644); err != nil {
		t.Fatalf("failed to write outside file: %v", err)
	}

	pc := tool.NewPermissionContext(workspaceRoot, []string{workspaceRoot})
	ctx := tool.ContextWithPermissionContext(context.Background(), pc)
	_, err := (&FileReadTool{}).Execute(ctx, `{"path":"../outside.txt"}`)
	if err == nil || !strings.Contains(err.Error(), "outside workspace") {
		t.Fatalf("expected traversal to be denied, got %v", err)
	}
}

func TestFileReadDeniesOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	permissionContext := tool.NewPermissionContext(root, []string{root})
	decision := (&FileReadTool{}).CheckPermission(context.Background(), `{"path":"`+escapePath(outsideFile)+`"}`, permissionContext)
	if decision.Behavior != tool.PermissionDeny {
		t.Fatalf("expected deny for outside workspace, got %v", decision.Behavior)
	}
}

func TestFileReadAllowsInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	permissionContext := tool.NewPermissionContext(root, []string{root})
	decision := (&FileReadTool{}).CheckPermission(context.Background(), `{"path":"`+escapePath(path)+`"}`, permissionContext)
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("expected allow for inside workspace, got %v", decision.Behavior)
	}
}

func TestFileReadDeniesSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0644); err != nil {
		t.Fatalf("failed to write outside file: %v", err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(outsideFile, link); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	permissionContext := tool.NewPermissionContext(root, []string{root})
	decision := (&FileReadTool{}).CheckPermission(context.Background(), `{"path":"link"}`, permissionContext)
	if decision.Behavior != tool.PermissionDeny {
		t.Fatalf("expected deny for symlink escape, got %v", decision.Behavior)
	}

	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	_, err := (&FileReadTool{}).Execute(ctx, `{"path":"link"}`)
	if err == nil || !strings.Contains(err.Error(), "outside workspace") {
		t.Fatalf("expected Execute to fail for symlink escape, got %v", err)
	}
}

func escapePath(path string) string {
	return strings.ReplaceAll(path, `\`, `\\`)
}
