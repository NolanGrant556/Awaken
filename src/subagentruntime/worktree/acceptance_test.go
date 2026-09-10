package worktree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"awaken/src/subagentruntime"
)

func acceptanceGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func acceptanceRepository(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for retained worktree acceptance")
	}
	root := t.TempDir()
	globalConfig := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(globalConfig, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("LC_ALL", "C")
	acceptanceGit(t, root, "init", "-b", "main")
	acceptanceGit(t, root, "config", "user.name", "Awaken acceptance")
	acceptanceGit(t, root, "config", "user.email", "acceptance@example.invalid")
	acceptanceGit(t, root, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("original seed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	acceptanceGit(t, root, "add", "seed.txt")
	acceptanceGit(t, root, "commit", "-m", "初始化验收夹具")
	return root
}

func TestRetainedWorktreeUsesExactChangedPaths(t *testing.T) {
	root := acceptanceRepository(t)
	ctx := context.Background()
	service := New()
	receipt, err := service.Create(ctx, root, "acceptance-retain-001")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{" draft notes .txt", "清单 中文.json", "嵌套 目录/资料.md"}
	if runtime.GOOS != "windows" {
		want = append(want, "line\nbreak.txt", "tab\tfile.txt", "quote\"file.txt")
	}
	want = append(want, "重命名 final.txt")
	for _, name := range want[:len(want)-1] {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(receipt.Path, name)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(receipt.Path, name), []byte("keep user edit\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Rename(filepath.Join(receipt.Path, "seed.txt"), filepath.Join(receipt.Path, want[len(want)-1])); err != nil {
		t.Fatal(err)
	}
	acceptanceGit(t, receipt.Path, "add", "--", "seed.txt", want[len(want)-1])
	retained, err := service.Finalize(ctx, receipt, false)
	if err != nil {
		t.Fatal(err)
	}
	if retained.Cleaned || !retained.Dirty || !retained.NeedsReview {
		t.Fatalf("dirty worktree must remain reviewable: %+v", retained)
	}
	continued, err := service.Revalidate(ctx, root, retained)
	if err != nil {
		t.Fatal(err)
	}
	got := append([]string(nil), continued.Changed...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("changed paths lost filename identity: got %q, want %q", got, want)
	}
	for _, name := range want {
		if _, err := os.Stat(filepath.Join(continued.Path, name)); err != nil {
			t.Fatalf("retained file %q: %v", name, err)
		}
	}
	if branch := acceptanceGit(t, root, "branch", "--show-current"); branch != "main" {
		t.Fatalf("parent branch changed to %q", branch)
	}
}

func TestMalformedStatusDoesNotCleanWorktree(t *testing.T) {
	for _, status := range []string{" M file", "R  destination\x00", "?? \x00", "invalid\x00"} {
		t.Run(status, func(t *testing.T) {
			root := acceptanceRepository(t)
			service := New()
			receipt, err := service.Create(context.Background(), root, "malformed-status")
			if err != nil {
				t.Fatal(err)
			}
			service.Git = func(ctx context.Context, dir string, args ...string) (string, error) {
				if len(args) > 0 && args[0] == "status" {
					return status, nil
				}
				return runGit(ctx, dir, args...)
			}
			got, err := service.Finalize(context.Background(), receipt, false)
			if err != nil || got.Cleaned || !got.NeedsReview {
				t.Fatalf("uncertain status removed worktree: %+v %v", got, err)
			}
			if _, err := os.Stat(receipt.Path); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRetainedWorktreeRejectsChangedIdentity(t *testing.T) {
	root := acceptanceRepository(t)
	ctx := context.Background()
	service := New()
	receipt, err := service.Create(ctx, root, "acceptance-identity-002")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Revalidate(ctx, root, receipt); err != nil {
		t.Fatalf("valid worktree rejected: %v", err)
	}
	other := acceptanceRepository(t)
	if _, err := service.Revalidate(ctx, other, receipt); err == nil {
		t.Fatal("worktree from another repository was accepted")
	}
	acceptanceGit(t, receipt.Path, "switch", "-c", "changed-by-user")
	if _, err := service.Revalidate(ctx, root, receipt); err == nil {
		t.Fatal("changed worktree branch was accepted")
	}
	receipt.Path = filepath.Join(root, "missing-worktree")
	if _, err := service.Revalidate(ctx, root, receipt); err == nil {
		t.Fatal("missing worktree directory was accepted")
	}
	_, err = service.Create(ctx, t.TempDir(), "non-git")
	var issue *subagentruntime.Error
	if !errors.As(err, &issue) || issue.Code != subagentruntime.ErrorWorkspaceMode {
		t.Fatalf("non-Git rejection = %v", err)
	}
}
