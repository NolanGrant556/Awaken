package listfiles

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListFilesMatchesAndSorts(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "b.go", "package b")
	writeTestFile(t, root, "a.go", "package a")
	writeTestFile(t, root, "notes.md", "# notes")

	out, err := (&ListFilesTool{}).Execute(context.Background(), `{"root":"`+escapePath(root)+`","pattern":"*.go"}`)
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

	out, err := (&ListFilesTool{}).Execute(context.Background(), `{"root":"`+escapePath(root)+`","pattern":"**/*.go","max_results":2}`)
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

	out, err := (&ListFilesTool{}).Execute(context.Background(), `{"root":"`+escapePath(root)+`","pattern":"*.go"}`)
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

	out, err := (&ListFilesTool{}).Execute(context.Background(), `{"root":"`+escapePath(root)+`","pattern":"**/*.go"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out != "src/main.go" {
		t.Fatalf("unexpected results: %q", out)
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
