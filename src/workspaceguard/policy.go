package workspaceguard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type DirtyPolicyMode string

const (
	PolicyAllowDirty   DirtyPolicyMode = "allow_dirty"
	PolicyRequireClean DirtyPolicyMode = "require_clean"
)

type WorktreeMode string

const (
	WorktreeNone     WorktreeMode = "none"
	WorktreePlanOnly WorktreeMode = "plan_only"
)

var (
	ErrPolicyDenied   = errors.New("workspace guard policy denied run")
	ErrWorkspaceDirty = errors.New("workspace has uncommitted changes")
)

type Policy struct {
	Mode     DirtyPolicyMode `json:"mode,omitempty"`
	Worktree WorktreePolicy  `json:"worktree,omitempty"`
}

type WorktreePolicy struct {
	Mode      WorktreeMode `json:"mode,omitempty"`
	ParentDir string       `json:"parent_dir,omitempty"`
	Name      string       `json:"name,omitempty"`
	Branch    string       `json:"branch,omitempty"`
}

type Decision struct {
	Allowed      bool            `json:"allowed"`
	PolicyMode   DirtyPolicyMode `json:"policy_mode"`
	ErrorCode    string          `json:"error_code,omitempty"`
	Reason       string          `json:"reason,omitempty"`
	Summary      string          `json:"summary,omitempty"`
	Repository   GitStatus       `json:"repository"`
	WorktreePlan *WorktreePlan   `json:"worktree_plan,omitempty"`
}

type PolicyError struct {
	Decision Decision
}

func (e *PolicyError) Error() string {
	if e == nil {
		return ErrPolicyDenied.Error()
	}
	summary := strings.TrimSpace(e.Decision.Summary)
	if summary == "" {
		summary = strings.TrimSpace(e.Decision.Reason)
	}
	if summary == "" {
		return ErrPolicyDenied.Error()
	}
	return fmt.Sprintf("%s: %s", ErrPolicyDenied, summary)
}

func (e *PolicyError) Is(target error) bool {
	if target == ErrPolicyDenied {
		return true
	}
	return target == ErrWorkspaceDirty && e != nil && e.Decision.ErrorCode == "WORKSPACE_DIRTY"
}

func (p Policy) IsZero() bool {
	return p.Mode == "" && p.Worktree.Mode == "" && p.Worktree.ParentDir == "" && p.Worktree.Name == "" && p.Worktree.Branch == ""
}

func Evaluate(ctx context.Context, workspaceRoot string, policy Policy) (Decision, error) {
	mode := policy.mode()
	status, err := Inspect(ctx, workspaceRoot)
	if err != nil {
		return Decision{}, err
	}

	decision := Decision{
		Allowed:    true,
		PolicyMode: mode,
		Summary:    status.Summary,
		Repository: status,
	}

	if status.IsGitRepo {
		plan, err := maybePlanWorktree(status, policy.Worktree)
		if err != nil {
			return Decision{}, err
		}
		decision.WorktreePlan = plan
	}

	if mode == PolicyRequireClean && status.IsGitRepo && status.Dirty {
		decision.Allowed = false
		decision.ErrorCode = "WORKSPACE_DIRTY"
		decision.Reason = "workspace must be clean before starting a run"
		return decision, &PolicyError{Decision: decision}
	}

	if !status.IsGitRepo {
		decision.Reason = "workspace is not a git repository"
	}
	return decision, nil
}

func (p Policy) mode() DirtyPolicyMode {
	switch p.Mode {
	case "", PolicyAllowDirty:
		return PolicyAllowDirty
	case PolicyRequireClean:
		return PolicyRequireClean
	default:
		return PolicyAllowDirty
	}
}

type GitStatus struct {
	WorkspaceRoot  string       `json:"workspace_root"`
	RepositoryRoot string       `json:"repository_root,omitempty"`
	IsGitRepo      bool         `json:"is_git_repo"`
	Branch         string       `json:"branch,omitempty"`
	Head           string       `json:"head,omitempty"`
	Dirty          bool         `json:"dirty"`
	Summary        string       `json:"summary"`
	Changes        []FileChange `json:"changes,omitempty"`
	ModifiedFiles  []string     `json:"modified_files,omitempty"`
	UntrackedFiles []string     `json:"untracked_files,omitempty"`
	StagedFiles    []string     `json:"staged_files,omitempty"`
	DeletedFiles   []string     `json:"deleted_files,omitempty"`
}

type FileChange struct {
	Path     string `json:"path"`
	Index    string `json:"index"`
	Worktree string `json:"worktree"`
	Original string `json:"original,omitempty"`
}

func Inspect(ctx context.Context, workspaceRoot string) (GitStatus, error) {
	root := filepath.Clean(strings.TrimSpace(workspaceRoot))
	status := GitStatus{WorkspaceRoot: root}
	if root == "." || root == "" {
		return status, fmt.Errorf("workspace_root is required")
	}
	info, err := os.Stat(root)
	if err != nil {
		return status, err
	}
	if !info.IsDir() {
		return status, fmt.Errorf("workspace_root %q is not a directory", root)
	}

	inside, err := gitOutput(ctx, root, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(inside) != "true" {
		status.Summary = "not a git repository"
		return status, nil
	}
	status.IsGitRepo = true

	repoRoot, err := gitOutput(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil {
		return status, err
	}
	status.RepositoryRoot = filepath.Clean(strings.TrimSpace(repoRoot))
	status.Branch = currentBranch(ctx, status.RepositoryRoot)
	status.Head = currentHead(ctx, status.RepositoryRoot)

	porcelain, err := gitOutput(ctx, status.RepositoryRoot, "status", "--porcelain=v1", "-z", "--untracked-files=normal")
	if err != nil {
		return status, err
	}
	parseStatusPorcelain(porcelain, &status)
	status.Dirty = len(status.Changes) > 0
	status.Summary = summarize(status)
	return status, nil
}

func currentBranch(ctx context.Context, repoRoot string) string {
	branch, err := gitOutput(ctx, repoRoot, "symbolic-ref", "--short", "-q", "HEAD")
	if err == nil {
		return strings.TrimSpace(branch)
	}
	name, err := gitOutput(ctx, repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	if err == nil {
		return strings.TrimSpace(name)
	}
	return ""
}

func currentHead(ctx context.Context, repoRoot string) string {
	head, err := gitOutput(ctx, repoRoot, "rev-parse", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(head)
}

func gitOutput(ctx context.Context, root string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", root}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("git %s failed: %s", strings.Join(args, " "), detail)
	}
	return stdout.String(), nil
}

func parseStatusPorcelain(output string, status *GitStatus) {
	if output == "" || status == nil {
		return
	}
	entries := strings.Split(output, "\x00")
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if len(entry) < 3 {
			continue
		}
		index := string(entry[0])
		worktree := string(entry[1])
		path := filepath.ToSlash(entry[3:])
		change := FileChange{
			Path:     path,
			Index:    index,
			Worktree: worktree,
		}
		if isRenameOrCopy(entry[0], entry[1]) && i+1 < len(entries) {
			i++
			change.Original = filepath.ToSlash(entries[i])
		}
		status.Changes = append(status.Changes, change)
		addChangeCategories(status, change)
	}
}

func isRenameOrCopy(index, worktree byte) bool {
	return index == 'R' || index == 'C' || worktree == 'R' || worktree == 'C'
}

func addChangeCategories(status *GitStatus, change FileChange) {
	index := firstStatusByte(change.Index)
	worktree := firstStatusByte(change.Worktree)
	if index == '?' && worktree == '?' {
		status.UntrackedFiles = appendUnique(status.UntrackedFiles, change.Path)
		return
	}
	if index != ' ' {
		status.StagedFiles = appendUnique(status.StagedFiles, change.Path)
	}
	if index == 'D' || worktree == 'D' {
		status.DeletedFiles = appendUnique(status.DeletedFiles, change.Path)
		return
	}
	status.ModifiedFiles = appendUnique(status.ModifiedFiles, change.Path)
}

func firstStatusByte(value string) byte {
	if value == "" {
		return ' '
	}
	return value[0]
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func summarize(status GitStatus) string {
	if !status.IsGitRepo {
		return "not a git repository"
	}
	if len(status.Changes) == 0 {
		return "clean"
	}
	parts := make([]string, 0, 4)
	if len(status.ModifiedFiles) > 0 {
		parts = append(parts, fmt.Sprintf("%d modified", len(status.ModifiedFiles)))
	}
	if len(status.StagedFiles) > 0 {
		parts = append(parts, fmt.Sprintf("%d staged", len(status.StagedFiles)))
	}
	if len(status.DeletedFiles) > 0 {
		parts = append(parts, fmt.Sprintf("%d deleted", len(status.DeletedFiles)))
	}
	if len(status.UntrackedFiles) > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked", len(status.UntrackedFiles)))
	}
	if len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%d changed", len(status.Changes)))
	}
	return fmt.Sprintf("%d dirty file(s): %s", len(status.Changes), strings.Join(parts, ", "))
}

func maybePlanWorktree(status GitStatus, policy WorktreePolicy) (*WorktreePlan, error) {
	switch policy.Mode {
	case "", WorktreeNone:
		return nil, nil
	case WorktreePlanOnly:
		plan := PlanWorktree(status, policy)
		return &plan, nil
	default:
		return nil, fmt.Errorf("unsupported worktree mode %q", policy.Mode)
	}
}

type WorktreePlan struct {
	RepositoryRoot string   `json:"repository_root"`
	Path           string   `json:"path"`
	Branch         string   `json:"branch"`
	Head           string   `json:"head,omitempty"`
	Command        []string `json:"command"`
}

func PlanWorktree(status GitStatus, policy WorktreePolicy) WorktreePlan {
	parent := filepath.Clean(strings.TrimSpace(policy.ParentDir))
	if parent == "." || parent == "" {
		parent = filepath.Dir(status.RepositoryRoot)
	}
	name := strings.TrimSpace(policy.Name)
	if name == "" {
		name = filepath.Base(status.RepositoryRoot) + "-isolated"
	}
	path := filepath.Clean(filepath.Join(parent, name))
	branch := strings.TrimSpace(policy.Branch)
	if branch == "" {
		branch = defaultWorktreeBranch(status)
	}
	head := status.Head
	if head == "" {
		head = "HEAD"
	}
	return WorktreePlan{
		RepositoryRoot: status.RepositoryRoot,
		Path:           path,
		Branch:         branch,
		Head:           head,
		Command: []string{
			"git",
			"-C",
			status.RepositoryRoot,
			"worktree",
			"add",
			"-b",
			branch,
			path,
			head,
		},
	}
}

func defaultWorktreeBranch(status GitStatus) string {
	base := strings.TrimSpace(status.Branch)
	if base == "" || base == "HEAD" {
		base = "run"
	}
	return base + "-isolated"
}
