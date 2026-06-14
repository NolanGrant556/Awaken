package grepsearch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrepSearchFindsMatchesWithLineNumbers(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "a.go", "package main\nfunc main() {}\n")
	writeTestFile(t, root, "b.md", "# main\n")

	out, err := (&GrepSearchTool{}).Execute(context.Background(), `{"root":"`+escapePath(root)+`","query":"func main","include":"**/*.go"}`)
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

	out, err := (&GrepSearchTool{}).Execute(context.Background(), `{"root":"`+escapePath(root)+`","query":"not_found"}`)
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

	out, err := (&GrepSearchTool{}).Execute(context.Background(), `{"root":"`+escapePath(root)+`","query":"hit","max_results":2}`)
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

	out, err := (&GrepSearchTool{}).Execute(context.Background(), `{"root":"`+escapePath(root)+`","query":"hit"}`)
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

	out, err := (&GrepSearchTool{}).Execute(context.Background(), `{"root":"`+escapePath(root)+`","query":"hit","max_line_chars":6}`)
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

	out, err := (&GrepSearchTool{}).Execute(context.Background(), `{"root":"`+escapePath(root)+`","query":"hit","output_mode":"count","offset":1}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out != "b.txt:1" {
		t.Fatalf("unexpected count output: %q", out)
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
