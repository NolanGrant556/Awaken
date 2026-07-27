package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"awaken/src/filestore"
)

// The generation directory always carries the scheduled-task authority
// layout so src/scheduledtask can open its repository without creating
// directories itself.
func TestFileStoreInitializesScheduledTasksDirs(t *testing.T) {
	root := setFileStoreTestRoots(t)
	if _, err := newFileDesktopStore(root); err != nil {
		t.Fatal(err)
	}
	genDir, _, err := filestore.ReadCurrentGeneration(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{
		filepath.Join(genDir, "scheduled-tasks"),
		filepath.Join(genDir, "scheduled-tasks", "executions"),
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("scheduled task dir missing: %v", err)
		}
		if info.Mode().Perm() != filestore.DirPerm {
			t.Fatalf("dir %s perm = %o, want %o", dir, info.Mode().Perm(), filestore.DirPerm)
		}
	}
}
