package plugins

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolvePluginRelativePathRejectsEscape(t *testing.T) {
	root := t.TempDir()
	if _, err := resolvePluginRelativePath(root, "../etc/passwd", "command"); err == nil {
		t.Fatal("expected escape rejection")
	}
	if _, err := resolvePluginRelativePath(root, "bin/server", "command"); err == nil {
		t.Fatal("expected ./ prefix")
	}
	abs, err := resolvePluginRelativePath(root, "./bin/server", "command")
	if err != nil {
		t.Fatal(err)
	}
	if abs != filepath.Join(root, "bin", "server") {
		t.Fatalf("abs=%q", abs)
	}
}

func TestRejectSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture is unix-oriented")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	link := filepath.Join(dir, "plugin.json")
	if err := os.WriteFile(target, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := rejectSymlink(link); err == nil {
		t.Fatal("expected symlink rejection")
	}
}
