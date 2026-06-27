package workspaceguard

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvaluateNonGitWorkspaceAllowsRequireClean(t *testing.T) {
	decision, err := Evaluate(context.Background(), t.TempDir(), Policy{Mode: PolicyRequireClean})
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected non-git workspace to be allowed, got %+v", decision)
	}
	if decision.Repository.IsGitRepo {
		t.Fatalf("expected non-git workspace")
	}
}

func TestEvaluateCleanRepoAllowsRequireClean(t *testing.T) {
	repo := initCleanRepo(t)

	decision, err := Evaluate(context.Background(), repo, Policy{Mode: PolicyRequireClean})
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected clean repo to be allowed, got %+v", decision)
	}
	if !decision.Repository.IsGitRepo {
		t.Fatalf("expected git repo")
	}
	if decision.Repository.Dirty {
		t.Fatalf("expected clean repo, got summary %q", decision.Repository.Summary)
	}
	if decision.Repository.Branch == "" || decision.Repository.Head == "" {
		t.Fatalf("expected branch and head metadata, got %+v", decision.Repository)
	}
}

func TestEvaluateDirtyRepoRequireCleanDeniedIncludesSummary(t *testing.T) {
	repo := initCleanRepo(t)
	writeFile(t, filepath.Join(repo, "README.md"), "changed\n")
	writeFile(t, filepath.Join(repo, "scratch.txt"), "draft\n")

	decision, err := Evaluate(context.Background(), repo, Policy{Mode: PolicyRequireClean})
	if err == nil {
		t.Fatalf("expected require_clean to reject dirty repo")
	}
	var policyErr *PolicyError
	if !errors.As(err, &policyErr) {
		t.Fatalf("expected PolicyError, got %T: %v", err, err)
	}
	if !errors.Is(err, ErrWorkspaceDirty) {
		t.Fatalf("expected ErrWorkspaceDirty, got %v", err)
	}
	if policyErr.Decision.Allowed {
		t.Fatalf("expected decision to reject run")
	}
	if !strings.Contains(policyErr.Error(), "modified") || !strings.Contains(policyErr.Error(), "untracked") {
		t.Fatalf("expected error summary to mention modified and untracked files, got %q", policyErr.Error())
	}
	if !contains(decision.Repository.ModifiedFiles, "README.md") {
		t.Fatalf("expected README.md in modified files, got %+v", decision.Repository.ModifiedFiles)
	}
	if !contains(decision.Repository.UntrackedFiles, "scratch.txt") {
		t.Fatalf("expected scratch.txt in untracked files, got %+v", decision.Repository.UntrackedFiles)
	}
}

func TestEvaluateAllowDirtyDoesNotDenyDirtyRepo(t *testing.T) {
	repo := initCleanRepo(t)
	writeFile(t, filepath.Join(repo, "README.md"), "changed\n")

	decision, err := Evaluate(context.Background(), repo, Policy{Mode: PolicyAllowDirty})
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected allow_dirty to allow dirty repo, got %+v", decision)
	}
	if !decision.Repository.Dirty {
		t.Fatalf("expected dirty status")
	}
}

func TestPlanWorktreeBuildsCommandWithoutExecuting(t *testing.T) {
	repo := initCleanRepo(t)
	decision, err := Evaluate(context.Background(), repo, Policy{
		Mode: PolicyAllowDirty,
		Worktree: WorktreePolicy{
			Mode:      WorktreePlanOnly,
			ParentDir: filepath.Dir(repo),
			Name:      "awaken-run-1",
			Branch:    "codex/run-1",
		},
	})
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if decision.WorktreePlan == nil {
		t.Fatalf("expected worktree plan")
	}
	wantPath := filepath.Join(filepath.Dir(repo), "awaken-run-1")
	if decision.WorktreePlan.Path != wantPath {
		t.Fatalf("expected plan path %q, got %q", wantPath, decision.WorktreePlan.Path)
	}
	wantCommand := []string{"git", "-C", decision.Repository.RepositoryRoot, "worktree", "add", "-b", "codex/run-1", wantPath, decision.Repository.Head}
	if strings.Join(decision.WorktreePlan.Command, "\x00") != strings.Join(wantCommand, "\x00") {
		t.Fatalf("unexpected command\nwant: %v\ngot:  %v", wantCommand, decision.WorktreePlan.Command)
	}
	if _, err := os.Stat(wantPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("planner should not create worktree path, stat err=%v", err)
	}
}

func initCleanRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "checkout", "-B", "main")
	runGit(t, repo, "config", "user.email", "awaken@example.invalid")
	runGit(t, repo, "config", "user.name", "Awaken Test")
	writeFile(t, filepath.Join(repo, "README.md"), "initial\n")
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "initial")
	return repo
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmdArgs := append([]string{"-C", repo}, args...)
	cmd := exec.Command("git", cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
