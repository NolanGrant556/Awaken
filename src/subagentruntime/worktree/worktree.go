package worktree

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"awaken/src/subagentruntime"
)

const branchPrefix = "awaken/subagent-"

type Service struct {
	Git func(ctx context.Context, dir string, args ...string) (string, error)
}

func New() *Service {
	return &Service{Git: runGit}
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
	status, err := s.git(ctx, receipt.Path, "status", "--porcelain")
	if err != nil {
		receipt.NeedsReview = true
		return receipt, nil
	}
	changed := changedPaths(status)
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
	cmd := exec.CommandContext(ctx, "git", args...)
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
	return strings.TrimSpace(stdout.String()), nil
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

func changedPaths(status string) []string {
	if strings.TrimSpace(status) == "" {
		return nil
	}
	var paths []string
	for _, line := range strings.Split(status, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		path := fields[len(fields)-1]
		if path != "" {
			paths = append(paths, filepath.ToSlash(path))
		}
	}
	return paths
}
