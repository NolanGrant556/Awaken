package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEntryCannotLeaveBundle(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "办公 包")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "external")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []string{"../external", outside, "C:/external.exe", "..\\external"} {
		if _, err := resolve(root, entry); err == nil {
			t.Fatalf("external entry accepted: %s", entry)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "alias")); err == nil {
		if _, err = resolve(root, "alias"); err == nil {
			t.Fatal("out-of-bundle symlink accepted")
		}
	}
}
