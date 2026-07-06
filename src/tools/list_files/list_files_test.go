package listfiles

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestListFilesMatchesAndSorts(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "b.go", "package b")
	writeTestFile(t, root, "a.go", "package a")
	writeTestFile(t, root, "notes.md", "# notes")

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	out, err := (&ListFilesTool{}).Execute(ctx, `{"root":"`+escapePath(root)+`","pattern":"*.go"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out != "a.go\nb.go" {
		t.Fatalf("expected sorted go files, got %q", out)
	}
}

func TestListFilesSupportsRecursivePatternAndLimit(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "a.go", "package a")
	writeTestFile(t, root, "nested/b.go", "package b")
	writeTestFile(t, root, "nested/c.go", "package c")

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	out, err := (&ListFilesTool{}).Execute(ctx, `{"root":"`+escapePath(root)+`","pattern":"**/*.go","max_results":2}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 2 truncated results, got %q", out)
	}
	if lines[0] != "a.go" || lines[1] != "nested/b.go" {
		t.Fatalf("unexpected results: %q", out)
	}
	if !strings.Contains(lines[2], "truncated") {
		t.Fatalf("expected truncated notice, got %q", out)
	}
}

func TestListFilesEmptyResult(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "notes.md", "# notes")

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	out, err := (&ListFilesTool{}).Execute(ctx, `{"root":"`+escapePath(root)+`","pattern":"*.go"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out != "未找到匹配文件" {
		t.Fatalf("unexpected empty output: %q", out)
	}
}

func TestListFilesSkipsNoisyDirectories(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "src/main.go", "package main")
	writeTestFile(t, root, "node_modules/pkg/index.go", "package pkg")

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	out, err := (&ListFilesTool{}).Execute(ctx, `{"root":"`+escapePath(root)+`","pattern":"**/*.go"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out != "src/main.go" {
		t.Fatalf("unexpected results: %q", out)
	}
}

func TestListFilesResolvesRelativePath(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "README.md", "# readme")

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	out, err := (&ListFilesTool{}).Execute(ctx, `{"pattern":"*.md"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out != "README.md" {
		t.Fatalf("expected relative workspace path, got %q", out)
	}
}

func TestListFilesDeniesHomeDesktop(t *testing.T) {
	workspaceRoot := t.TempDir()
	pc := tool.NewPermissionContext(workspaceRoot, []string{workspaceRoot})
	ctx := tool.ContextWithPermissionContext(context.Background(), pc)

	_, err := (&ListFilesTool{}).Execute(ctx, `{"root":"~/Desktop"}`)
	if err == nil || (!strings.Contains(err.Error(), "outside workspace") && !strings.Contains(err.Error(), "outside readable roots")) {
		t.Fatalf("expected ~/Desktop to be denied, got %v", err)
	}
}

func TestListFilesBlocksTraversal(t *testing.T) {
	parent := t.TempDir()
	workspaceRoot := filepath.Join(parent, "workspace")
	if err := os.MkdirAll(workspaceRoot, 0755); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}
	outsideDir := filepath.Join(parent, "outside")
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatalf("failed to create outside dir: %v", err)
	}

	pc := tool.NewPermissionContext(workspaceRoot, []string{workspaceRoot})
	ctx := tool.ContextWithPermissionContext(context.Background(), pc)
	_, err := (&ListFilesTool{}).Execute(ctx, `{"root":"../outside"}`)
	if err == nil || (!strings.Contains(err.Error(), "outside workspace") && !strings.Contains(err.Error(), "outside readable roots")) {
		t.Fatalf("expected traversal to be denied, got %v", err)
	}
}

func TestListFilesResolvesRelativeRoot(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "src/main.go", "package main")

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	out, err := (&ListFilesTool{}).Execute(ctx, `{"root":"src"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out != "src/main.go" {
		t.Fatalf("expected src/main.go, got %q", out)
	}
}

func TestListFilesDeniesOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeTestFile(t, outside, "secret.txt", "secret")

	permissionContext := tool.NewPermissionContext(root, []string{root})
	decision := (&ListFilesTool{}).CheckPermission(context.Background(), `{"root":"`+escapePath(outside)+`"}`, permissionContext)
	if decision.Behavior != tool.PermissionDeny {
		t.Fatalf("expected deny for outside workspace, got %v", decision.Behavior)
	}
}

func TestListFilesAllowsInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "a.go", "package a")

	permissionContext := tool.NewPermissionContext(root, []string{root})
	decision := (&ListFilesTool{}).CheckPermission(context.Background(), `{"root":"`+escapePath(root)+`"}`, permissionContext)
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("expected allow for inside workspace, got %v", decision.Behavior)
	}
}

func TestListFilesDeniesSymlinkRootEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeTestFile(t, outside, "secret.txt", "secret")
	link := filepath.Join(root, "outside-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	permissionContext := tool.NewPermissionContext(root, []string{root})
	decision := (&ListFilesTool{}).CheckPermission(context.Background(), `{"root":"outside-link"}`, permissionContext)
	if decision.Behavior != tool.PermissionDeny {
		t.Fatalf("expected deny for symlink root escape, got %v", decision.Behavior)
	}
}

func TestListFilesSkipsSymlinkFileEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeTestFile(t, outside, "secret.txt", "secret")
	link := filepath.Join(root, "secret-link")
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), link); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}
	writeTestFile(t, root, "real.txt", "real")

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	out, err := (&ListFilesTool{}).Execute(ctx, `{}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(out, "secret-link") || strings.Contains(out, "secret.txt") {
		t.Fatalf("expected symlink escape to be skipped, got %q", out)
	}
	if !strings.Contains(out, "real.txt") {
		t.Fatalf("expected real file to be listed, got %q", out)
	}
}

func writeTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
}

func escapePath(path string) string {
	return strings.ReplaceAll(path, `\`, `\\`)
}
