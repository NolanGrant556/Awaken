package fileread

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func escapePath(path string) string {
	return strings.ReplaceAll(path, `\`, `\\`)
}
