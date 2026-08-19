package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"awaken/src/subagentruntime"
)

func TestCreateInspectAndCleanupCleanWorktree(t *testing.T) {
	repo := initGitRepo(t)
	service := New()
	receipt, err := service.Create(context.Background(), repo, "subtask_abc123def456")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if receipt.Path == "" || receipt.Branch == "" || receipt.BaseCommit == "" {
		t.Fatalf("receipt = %+v", receipt)
	}
	if _, err := os.Stat(receipt.Path); err != nil {
		t.Fatalf("worktree missing: %v", err)
	}
	final, err := service.Finalize(context.Background(), receipt, false)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if !final.Cleaned || final.Dirty {
		t.Fatalf("clean worktree should be removed: %+v", final)
	}
	if _, err := os.Stat(receipt.Path); !os.IsNotExist(err) {
		t.Fatalf("worktree path still exists")
	}
}

func TestDirtyWorktreeIsRetained(t *testing.T) {
	repo := initGitRepo(t)
	service := New()
	receipt, err := service.Create(context.Background(), repo, "subtask_dirty001")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(receipt.Path, "child.txt"), []byte("from child"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, "child.txt")); !os.IsNotExist(err) {
		t.Fatalf("parent tree was polluted")
	}
	final, err := service.Finalize(context.Background(), receipt, false)
	if err != nil {
		t.Fatal(err)
	}
	if final.Cleaned || !final.Dirty || !final.NeedsReview {
		t.Fatalf("dirty receipt = %+v", final)
	}
	if _, err := os.Stat(receipt.Path); err != nil {
		t.Fatalf("dirty worktree was deleted")
	}
}

func TestNonGitWorkspaceIsUnsupported(t *testing.T) {
	dir := t.TempDir()
	_, err := New().Create(context.Background(), dir, "subtask_nogit")
	if subagentruntime.ErrorCode(err) != subagentruntime.ErrorWorkspaceMode {
		t.Fatalf("non-git = %v", err)
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for worktree tests")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("commit", "-m", "init")
	return dir
}
