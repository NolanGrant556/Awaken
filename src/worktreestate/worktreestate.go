package worktreestate

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const (
	maxSlugLength  = 64
	StateEntryType = "worktree-state"
)

var validSlugSegment = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

type Session struct {
	OriginalCwd        string `json:"originalCwd"`
	WorktreePath       string `json:"worktreePath"`
	WorktreeName       string `json:"worktreeName"`
	WorktreeBranch     string `json:"worktreeBranch,omitempty"`
	OriginalBranch     string `json:"originalBranch,omitempty"`
	OriginalHeadCommit string `json:"originalHeadCommit,omitempty"`
	SessionID          string `json:"sessionId"`
	TmuxSessionName    string `json:"tmuxSessionName,omitempty"`
	HookBased          bool   `json:"hookBased,omitempty"`
	CreationDurationMs int    `json:"creationDurationMs,omitempty"`
	UsedSparsePaths    bool   `json:"usedSparsePaths,omitempty"`
}

type StateEntry struct {
	Type            string   `json:"type"`
	SessionID       string   `json:"sessionId"`
	WorktreeSession *Session `json:"worktreeSession"`
}

type Manager struct {
	mu      sync.RWMutex
	current *Session
}

func ValidateSlug(slug string) error {
	if len(slug) > maxSlugLength {
		return fmt.Errorf("invalid worktree slug: must be %d characters or fewer (got %d)", maxSlugLength, len(slug))
	}

	for _, segment := range strings.Split(slug, "/") {
		if segment == "." || segment == ".." {
			return fmt.Errorf("invalid worktree slug %q: must not contain . or .. path segments", slug)
		}
		if !validSlugSegment.MatchString(segment) {
			return fmt.Errorf("invalid worktree slug %q: each segment must be non-empty and contain only letters, digits, dots, underscores, and dashes", slug)
		}
	}

	return nil
}

func FlattenSlug(slug string) string {
	return strings.ReplaceAll(slug, "/", "+")
}

func BranchName(slug string) string {
	return "worktree-" + FlattenSlug(slug)
}

func PathFor(repoRoot, slug string) string {
	return filepath.Join(repoRoot, ".claude", "worktrees", FlattenSlug(slug))
}

func GenerateTmuxSessionName(repoPath, branch string) string {
	repoName := filepath.Base(filepath.Clean(repoPath))
	name := repoName + "_" + branch
	name = strings.ReplaceAll(name, "/", "_")
	return strings.ReplaceAll(name, ".", "_")
}

func (m *Manager) Restore(session *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.current = copySession(session)
}

func (m *Manager) Current() *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return copySession(m.current)
}

func (m *Manager) Enter(session Session) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.current = copySession(&session)
}

func (m *Manager) Exit(sessionID string) StateEntry {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.current = nil
	return StateEntry{
		Type:            StateEntryType,
		SessionID:       sessionID,
		WorktreeSession: nil,
	}
}

func copySession(session *Session) *Session {
	if session == nil {
		return nil
	}
	copy := *session
	return &copy
}
