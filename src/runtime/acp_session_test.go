package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalizeCWD(t *testing.T) {
	root := t.TempDir()
	canonical, err := CanonicalizeCWD(root)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if canonical == "" {
		t.Fatal("empty canonical cwd")
	}

	if _, err := CanonicalizeCWD(""); err == nil {
		t.Fatal("expected error for empty cwd")
	}
	if _, err := CanonicalizeCWD("relative/path"); err == nil {
		t.Fatal("expected error for relative cwd")
	}
	if _, err := CanonicalizeCWD(filepath.Join(root, "does-not-exist")); err == nil {
		t.Fatal("expected error for non-existent cwd")
	}
	filePath := filepath.Join(root, "file.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CanonicalizeCWD(filePath); err == nil {
		t.Fatal("expected error for file cwd")
	}
	if _, err := CanonicalizeCWD(root + "\x00"); err == nil {
		t.Fatal("expected error for NUL in cwd")
	}
}

func TestStableCWDHash(t *testing.T) {
	h1 := StableCWDHash("/foo/bar")
	h2 := StableCWDHash("/foo/bar")
	h3 := StableCWDHash("/foo/baz")
	if h1 != h2 {
		t.Fatal("same cwd produced different hashes")
	}
	if h1 == h3 {
		t.Fatal("different cwd produced same hash")
	}
}
