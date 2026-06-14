package filesafe

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShouldSkipDir(t *testing.T) {
	if !ShouldSkipDir(filepath.Join("repo", "node_modules")) {
		t.Fatalf("expected node_modules to be skipped")
	}
	if ShouldSkipDir(filepath.Join("repo", "src")) {
		t.Fatalf("did not expect src to be skipped")
	}
}

func TestLooksBinaryByExtension(t *testing.T) {
	if !HasBinaryExtension("image.png") {
		t.Fatalf("expected png to be binary")
	}
}

func TestLooksBinaryByContent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "blob.txt")
	if err := os.WriteFile(path, []byte{0, 1, 2, 3}, 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	binary, err := LooksBinary(path)
	if err != nil {
		t.Fatalf("LooksBinary returned error: %v", err)
	}
	if !binary {
		t.Fatalf("expected file to look binary")
	}
}
