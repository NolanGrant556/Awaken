package worktree

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"awaken/src/managedenv"
	"awaken/src/subagentruntime"
)

const branchPrefix = "awaken/subagent-"

type Service struct {
	Git func(ctx context.Context, dir string, args ...string) (string, error)
}

func New() *Service {
	return &Service{Git: runGit}
}

// Revalidate checks the retained worktree against the current repository and
// its exact branch before a continuing Agent may regain file access.
func (s *Service) Revalidate(ctx context.Context, workspaceRoot string, receipt subagentruntime.WorktreeReceipt) (subagentruntime.WorktreeReceipt, error) {
	info, err := os.Lstat(receipt.Path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || receipt.Cleaned || !strings.HasPrefix(receipt.Branch, branchPrefix) {
		return receipt, fmt.Errorf("saved worktree is unavailable or changed")
	}
	commonDir := func(dir string) (string, error) {
		value, err := s.git(ctx, dir, "rev-parse", "--git-common-dir")
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(value) {
			value = filepath.Join(dir, value)
		}
		return filepath.EvalSymlinks(value)
	}
	parentCommon, err := commonDir(workspaceRoot)
	if err != nil {
		return receipt, err
	}
	childCommon, err := commonDir(receipt.Path)
	if err != nil || childCommon != parentCommon {
		return receipt, fmt.Errorf("saved worktree belongs to a different repository")
	}
	branch, err := s.git(ctx, receipt.Path, "branch", "--show-current")
	if err != nil || branch != receipt.Branch {
		return receipt, fmt.Errorf("saved worktree branch changed")
	}
	return s.Inspect(ctx, receipt)
}

func (s *Service) Create(ctx context.Context, workspaceRoot, taskID string) (subagentruntime.WorktreeReceipt, error) {
	if s == nil {
		s = New()
	}
	repoRoot, err := s.gitRoot(ctx, workspaceRoot)
	if err != nil {
		return subagentruntime.WorktreeReceipt{}, err
	}
	shortID := shortTaskID(taskID)
	branch := branchPrefix + shortID
	path := filepath.Join(repoRoot, ".awaken", "worktrees", "subagent-"+shortID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return subagentruntime.WorktreeReceipt{}, fmt.Errorf("%s: %w", subagentruntime.ErrorWorktreeFailed, err)
	}
	base, err := s.git(ctx, repoRoot, "rev-parse", "HEAD")
	if err != nil {
		return subagentruntime.WorktreeReceipt{}, unsupportedOrFailed(err)
	}
	if _, err := s.git(ctx, repoRoot, "worktree", "add", "-b", branch, path, "HEAD"); err != nil {
		return subagentruntime.WorktreeReceipt{}, worktreeFailed(err)
	}
	return subagentruntime.WorktreeReceipt{
		Path: path, Branch: branch, BaseCommit: base, HeadCommit: base,
	}, nil
}

func (s *Service) Inspect(ctx context.Context, receipt subagentruntime.WorktreeReceipt) (subagentruntime.WorktreeReceipt, error) {
	if s == nil {
		s = New()
	}
	if strings.TrimSpace(receipt.Path) == "" {
		return receipt, nil
	}
	if _, err := os.Stat(receipt.Path); err != nil {
		return receipt, nil
	}
	head, err := s.git(ctx, receipt.Path, "rev-parse", "HEAD")
	if err == nil {
		receipt.HeadCommit = head
	}
	status, err := s.git(ctx, receipt.Path, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		receipt.NeedsReview = true
		return receipt, nil
	}
	changed, err := changedPaths(status)
	if err != nil {
		receipt.NeedsReview = true
		return receipt, nil
	}
	receipt.Changed = changed
	receipt.Dirty = len(changed) > 0 || (receipt.BaseCommit != "" && receipt.HeadCommit != "" && receipt.HeadCommit != receipt.BaseCommit)
	receipt.NeedsReview = receipt.Dirty
	return receipt, nil
}

func (s *Service) Finalize(ctx context.Context, receipt subagentruntime.WorktreeReceipt, keep bool) (subagentruntime.WorktreeReceipt, error) {
	inspected, err := s.Inspect(ctx, receipt)
	if err != nil {
		return receipt, err
	}
	if keep || inspected.Dirty || inspected.NeedsReview {
		inspected.NeedsReview = inspected.Dirty || inspected.NeedsReview
		return inspected, nil
	}
	repoRoot, err := s.gitRoot(ctx, inspected.Path)
	if err != nil {
		inspected.NeedsReview = true
		return inspected, nil
	}
	if _, removeErr := s.git(ctx, repoRoot, "worktree", "remove", "--force", inspected.Path); removeErr != nil {
		inspected.NeedsReview = true
		return inspected, nil
	}
	_, _ = s.git(ctx, repoRoot, "branch", "-D", inspected.Branch)
	inspected.Cleaned = true
	inspected.Dirty = false
	inspected.NeedsReview = false
	inspected.Changed = nil
	return inspected, nil
}

func (s *Service) gitRoot(ctx context.Context, start string) (string, error) {
	output, err := s.git(ctx, start, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", unsupportedOrFailed(err)
	}
	return output, nil
}

func (s *Service) git(ctx context.Context, dir string, args ...string) (string, error) {
	if s.Git == nil {
		s.Git = runGit
	}
	return s.Git(ctx, dir, args...)
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	executable := "git"
	if m := managedenv.FromContext(ctx); m != nil {
		owner := managedenv.Owner{ThreadID: "worktree", AgentID: "host", RunID: fmt.Sprintf("worktree-%d", time.Now().UnixNano()), Kind: "service"}
		if launch, ok := m.AcquireExecutable("git", owner); ok {
			executable = launch.Executable
			defer m.ReleaseOwner(owner)
		}
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), message)
	}
	for _, arg := range args {
		if arg == "-z" {
			return stdout.String(), nil
		}
	}
	return strings.TrimSuffix(stdout.String(), "\n"), nil
}

func unsupportedOrFailed(err error) error {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "not a git repository") || strings.Contains(message, "not a git repo") {
		return &subagentruntime.Error{Code: subagentruntime.ErrorWorkspaceMode, Message: "git_worktree requires a git repository"}
	}
	return worktreeFailed(err)
}

func worktreeFailed(err error) error {
	return &subagentruntime.Error{Code: subagentruntime.ErrorWorktreeFailed, Message: err.Error()}
}

func shortTaskID(taskID string) string {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, taskID)
	if len(cleaned) > 12 {
		cleaned = cleaned[len(cleaned)-12:]
	}
	if cleaned == "" {
		cleaned = "task"
	}
	return cleaned
}

func changedPaths(status string) ([]string, error) {
	if status == "" {
		return nil, nil
	}
	if !strings.HasSuffix(status, "\x00") {
		return nil, fmt.Errorf("incomplete NUL-delimited git status")
	}
	entries := strings.Split(status, "\x00")
	var paths []string
	for i := 0; i < len(entries)-1; i++ {
		entry := entries[i]
		if len(entry) < 4 || entry[2] != ' ' {
			return nil, fmt.Errorf("invalid git status entry")
		}
		paths = append(paths, filepath.ToSlash(entry[3:]))
		// Porcelain -z puts the destination first, followed by the source
		// as a separate NUL-terminated path for renames and copies.
		if strings.ContainsAny(entry[:2], "RC") {
			i++
			if i >= len(entries)-1 || entries[i] == "" {
				return nil, fmt.Errorf("incomplete git rename source")
			}
		}
	}
	return paths, nil
}
