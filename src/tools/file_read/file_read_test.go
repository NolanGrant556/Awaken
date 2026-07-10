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
	if err == nil || !strings.Contains(err.Error(), "二进制") || !strings.Contains(err.Error(), "read_artifact") || !strings.Contains(err.Error(), "pdf_extract") {
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

func TestReadFileAllowsAbsolutePathOutsideWorkspace(t *testing.T) {
	workspaceRoot := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "notes.txt")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0644); err != nil {
		t.Fatalf("failed to write outside file: %v", err)
	}
	pc := tool.NewPermissionContext(workspaceRoot, []string{workspaceRoot})
	ctx := tool.ContextWithPermissionContext(context.Background(), pc)

	out, err := (&FileReadTool{}).Execute(ctx, `{"path":"`+escapePath(outsideFile)+`"}`)
	if err != nil {
		t.Fatalf("expected outside path to be readable, got %v", err)
	}
	if out != "outside" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestReadFileAllowsTraversalOutsideWorkspace(t *testing.T) {
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
	out, err := (&FileReadTool{}).Execute(ctx, `{"path":"../outside.txt"}`)
	if err != nil {
		t.Fatalf("expected traversal outside workspace to be readable, got %v", err)
	}
	if out != "secret" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestFileReadAllowsOutsideWorkspacePermission(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	permissionContext := tool.NewPermissionContext(root, []string{root})
	decision := (&FileReadTool{}).CheckPermission(context.Background(), `{"path":"`+escapePath(outsideFile)+`"}`, permissionContext)
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("expected allow for outside workspace, got %v", decision.Behavior)
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

func TestFileReadAllowsSymlinkOutsideWorkspace(t *testing.T) {
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
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("expected allow for symlink outside workspace, got %v", decision.Behavior)
	}

	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	out, err := (&FileReadTool{}).Execute(ctx, `{"path":"link"}`)
	if err != nil {
		t.Fatalf("expected Execute to read symlink outside workspace, got %v", err)
	}
	if out != "secret" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestFileReadOutcomeIncludesDigest(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	content := []byte("hello\nworld\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	outcome, err := (&FileReadTool{}).ExecuteOutcome(context.Background(), path, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome transport error: %v", err)
	}
	if outcome.IsError() {
		t.Fatalf("expected success, got %+v", outcome)
	}
	if outcome.Content != "hello\nworld" {
		t.Fatalf("pure-text content changed: %q", outcome.Content)
	}
	sha, _ := outcome.Diagnostic["sha256"].(string)
	expected, _ := outcome.Diagnostic["expected_sha256"].(string)
	size, _ := outcome.Diagnostic["size"].(int64)
	mtime, _ := outcome.Diagnostic["mtime"].(string)
	if sha == "" || expected == "" || sha != expected {
		t.Fatalf("expected sha256/expected_sha256, got diagnostic=%v", outcome.Diagnostic)
	}
	if size != int64(len(content)) {
		t.Fatalf("size=%v want %d", size, len(content))
	}
	if mtime == "" {
		t.Fatal("expected mtime in diagnostic")
	}
	payload := outcome.ModelPayload()
	if !strings.Contains(payload, "expected_sha256") || !strings.Contains(payload, sha) {
		t.Fatalf("model payload missing digest: %s", payload)
	}
}

func escapePath(path string) string {
	return strings.ReplaceAll(path, `\`, `\\`)
}
