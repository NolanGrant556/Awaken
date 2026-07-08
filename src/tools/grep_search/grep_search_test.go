package grepsearch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestGrepSearchFindsMatchesWithLineNumbers(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "a.go", "package main\nfunc main() {}\n")
	writeTestFile(t, root, "b.md", "# main\n")

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	out, err := (&GrepSearchTool{}).Execute(ctx, `{"root":"`+escapePath(root)+`","query":"func main","include":"**/*.go"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out != "a.go:2:func main() {}" {
		t.Fatalf("unexpected grep output: %q", out)
	}
}

func TestGrepSearchEmptyResult(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "a.go", "package main\n")

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	out, err := (&GrepSearchTool{}).Execute(ctx, `{"root":"`+escapePath(root)+`","query":"not_found"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out != "未找到匹配内容" {
		t.Fatalf("unexpected empty output: %q", out)
	}
}

func TestGrepSearchInvalidRegex(t *testing.T) {
	_, err := (&GrepSearchTool{}).Execute(context.Background(), `{"query":"["}`)
	if err == nil || !strings.Contains(err.Error(), "有效正则表达式") {
		t.Fatalf("expected invalid regex error, got %v", err)
	}
}

func TestGrepSearchLimitsResults(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "a.txt", "hit 1\nhit 2\nhit 3\n")

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	out, err := (&GrepSearchTool{}).Execute(ctx, `{"root":"`+escapePath(root)+`","query":"hit","max_results":2}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Count(out, "\n") != 2 || !strings.Contains(out, "truncated") {
		t.Fatalf("expected 2 results, got %q", out)
	}
}

func TestGrepSearchSkipsNoisyDirectoriesAndBinary(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "src/main.go", "package main\n// hit\n")
	writeTestFile(t, root, "node_modules/pkg/index.go", "package pkg\n// hit\n")
	if err := os.WriteFile(filepath.Join(root, "blob.dat"), []byte("hit\x00binary"), 0644); err != nil {
		t.Fatalf("failed to write binary file: %v", err)
	}

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	out, err := (&GrepSearchTool{}).Execute(ctx, `{"root":"`+escapePath(root)+`","query":"hit"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(out, "node_modules") || strings.Contains(out, "blob.dat") {
		t.Fatalf("expected noisy/binary paths to be skipped, got %q", out)
	}
	if !strings.Contains(out, "src/main.go") {
		t.Fatalf("expected source match, got %q", out)
	}
}

func TestGrepSearchTruncatesLongLines(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "a.txt", "hit "+strings.Repeat("x", 20)+"\n")

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	out, err := (&GrepSearchTool{}).Execute(ctx, `{"root":"`+escapePath(root)+`","query":"hit","max_line_chars":6}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out, "line truncated") {
		t.Fatalf("expected long line truncation, got %q", out)
	}
}

func TestGrepSearchOutputModesAndOffset(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "a.txt", "hit\nhit\n")
	writeTestFile(t, root, "b.txt", "hit\n")

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	out, err := (&GrepSearchTool{}).Execute(ctx, `{"root":"`+escapePath(root)+`","query":"hit","output_mode":"count","offset":1}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out != "b.txt:1" {
		t.Fatalf("unexpected count output: %q", out)
	}
}

func TestGrepSearchResolvesRelativePath(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "README.md", "# README\n")

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	out, err := (&GrepSearchTool{}).Execute(ctx, `{"query":"README"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out != "README.md:1:# README" {
		t.Fatalf("expected relative workspace path, got %q", out)
	}
}

func TestGrepSearchDeniesOutsideWorkspace(t *testing.T) {
	workspaceRoot := t.TempDir()
	pc := tool.NewPermissionContext(workspaceRoot, []string{workspaceRoot})
	ctx := tool.ContextWithPermissionContext(context.Background(), pc)

	_, err := (&GrepSearchTool{}).Execute(ctx, `{"query":"foo","root":"/etc"}`)
	if err == nil || (!strings.Contains(err.Error(), "outside workspace") && !strings.Contains(err.Error(), "outside readable roots")) {
		t.Fatalf("expected /etc to be denied, got %v", err)
	}
}

func TestGrepSearchBlocksTraversal(t *testing.T) {
	parent := t.TempDir()
	workspaceRoot := filepath.Join(parent, "workspace")
	if err := os.MkdirAll(workspaceRoot, 0755); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}
	writeTestFile(t, parent, "outside/secret.txt", "secret")

	pc := tool.NewPermissionContext(workspaceRoot, []string{workspaceRoot})
	ctx := tool.ContextWithPermissionContext(context.Background(), pc)
	_, err := (&GrepSearchTool{}).Execute(ctx, `{"query":"secret","root":"../outside"}`)
	if err == nil || (!strings.Contains(err.Error(), "outside workspace") && !strings.Contains(err.Error(), "outside readable roots")) {
		t.Fatalf("expected traversal to be denied, got %v", err)
	}
}

func TestGrepSearchResolvesRelativeRoot(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "src/main.go", "hello")

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	out, err := (&GrepSearchTool{}).Execute(ctx, `{"query":"hello","root":"src"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out != "src/main.go:1:hello" {
		t.Fatalf("expected src/main.go:1:hello, got %q", out)
	}
}

func TestGrepSearchDeniesArbitraryOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeTestFile(t, outside, "secret.txt", "secret")

	permissionContext := tool.NewPermissionContext(root, []string{root})
	decision := (&GrepSearchTool{}).CheckPermission(context.Background(), `{"root":"`+escapePath(outside)+`","query":"secret"}`, permissionContext)
	if decision.Behavior != tool.PermissionDeny {
		t.Fatalf("expected deny for outside workspace, got %v", decision.Behavior)
	}
	if !strings.Contains(decision.Reason, "workspace_dependencies") || !strings.Contains(decision.Reason, "系统目录") {
		t.Fatalf("expected routing guidance in deny reason, got %q", decision.Reason)
	}
}

func TestGrepSearchAllowsInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "a.go", "package main\n")

	permissionContext := tool.NewPermissionContext(root, []string{root})
	decision := (&GrepSearchTool{}).CheckPermission(context.Background(), `{"root":"`+escapePath(root)+`","query":"package"}`, permissionContext)
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("expected allow for inside workspace, got %v", decision.Behavior)
	}
}

func TestGrepSearchDeniesSymlinkRootEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeTestFile(t, outside, "secret.txt", "secret")
	link := filepath.Join(root, "outside-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	permissionContext := tool.NewPermissionContext(root, []string{root})
	decision := (&GrepSearchTool{}).CheckPermission(context.Background(), `{"root":"outside-link","query":"secret"}`, permissionContext)
	if decision.Behavior != tool.PermissionDeny {
		t.Fatalf("expected deny for symlink root escape, got %v", decision.Behavior)
	}
}

func TestGrepSearchSkipsSymlinkFileEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeTestFile(t, outside, "secret.txt", "secret content")
	link := filepath.Join(root, "secret-link")
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), link); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}
	writeTestFile(t, root, "real.txt", "real content")

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	out, err := (&GrepSearchTool{}).Execute(ctx, `{"query":"content"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(out, "secret") {
		t.Fatalf("expected symlink escape to be skipped, got %q", out)
	}
	if !strings.Contains(out, "real.txt") {
		t.Fatalf("expected real file to be searched, got %q", out)
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
