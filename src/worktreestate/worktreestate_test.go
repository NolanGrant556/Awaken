package worktreestate

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateSlugValidNested(t *testing.T) {
	valid := []string{
		"feature",
		"user/feature-1",
		"team.name/feature_2/fix.3",
		"a_b-c.1",
	}

	for _, slug := range valid {
		t.Run(slug, func(t *testing.T) {
			if err := ValidateSlug(slug); err != nil {
				t.Fatalf("ValidateSlug(%q) returned error: %v", slug, err)
			}
		})
	}
}

func TestValidateSlugInvalid(t *testing.T) {
	tests := []struct {
		name string
		slug string
	}{
		{name: "empty slug", slug: ""},
		{name: "too long", slug: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{name: "empty segment", slug: "team//feature"},
		{name: "leading slash", slug: "/feature"},
		{name: "trailing slash", slug: "feature/"},
		{name: "dot segment", slug: "team/./feature"},
		{name: "dotdot segment", slug: "team/../feature"},
		{name: "bad chars", slug: "team/feature+one"},
		{name: "absolute drive", slug: "C:/feature"},
		{name: "backslash", slug: `team\feature`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateSlug(tt.slug); err == nil {
				t.Fatalf("ValidateSlug(%q) returned nil error", tt.slug)
			}
		})
	}
}

func TestNamingHelpers(t *testing.T) {
	slug := "team/feature.v1"

	if got, want := FlattenSlug(slug), "team+feature.v1"; got != want {
		t.Fatalf("FlattenSlug() = %q, want %q", got, want)
	}
	if got, want := BranchName(slug), "worktree-team+feature.v1"; got != want {
		t.Fatalf("BranchName() = %q, want %q", got, want)
	}
	if got, want := PathFor("/repo/root", slug), "/repo/root/.claude/worktrees/team+feature.v1"; got != want {
		t.Fatalf("PathFor() = %q, want %q", got, want)
	}
}

func TestManagerRestoreCurrentReturnsCopy(t *testing.T) {
	manager := &Manager{}
	session := sampleSession("session-1")

	manager.Restore(&session)
	session.WorktreePath = "/changed"

	current := manager.Current()
	if current == nil {
		t.Fatalf("Current() returned nil")
	}
	if got, want := current.WorktreePath, "/repo/.claude/worktrees/team+feature"; got != want {
		t.Fatalf("Current().WorktreePath = %q, want %q", got, want)
	}

	current.WorktreePath = "/mutated"
	current = manager.Current()
	if got, want := current.WorktreePath, "/repo/.claude/worktrees/team+feature"; got != want {
		t.Fatalf("Current() did not return a copy, got path %q", got)
	}

	manager.Restore(nil)
	if current := manager.Current(); current != nil {
		t.Fatalf("Current() after Restore(nil) = %+v, want nil", current)
	}
}

func TestManagerEnterStoresCopyAndExitWritesNull(t *testing.T) {
	manager := &Manager{}
	session := sampleSession("session-2")

	manager.Enter(session)
	session.WorktreeName = "changed"

	current := manager.Current()
	if current == nil {
		t.Fatalf("Current() returned nil")
	}
	if got, want := current.WorktreeName, "team/feature"; got != want {
		t.Fatalf("Current().WorktreeName = %q, want %q", got, want)
	}

	entry := manager.Exit("session-2")
	if got, want := entry.Type, StateEntryType; got != want {
		t.Fatalf("Exit().Type = %q, want %q", got, want)
	}
	if got, want := entry.SessionID, "session-2"; got != want {
		t.Fatalf("Exit().SessionID = %q, want %q", got, want)
	}
	if entry.WorktreeSession != nil {
		t.Fatalf("Exit().WorktreeSession = %+v, want nil", entry.WorktreeSession)
	}
	if current := manager.Current(); current != nil {
		t.Fatalf("Current() after Exit() = %+v, want nil", current)
	}

	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal(Exit()) returned error: %v", err)
	}
	if !strings.Contains(string(encoded), `"worktreeSession":null`) {
		t.Fatalf("Marshal(Exit()) = %s, want worktreeSession null", encoded)
	}
}

func TestGenerateTmuxSessionName(t *testing.T) {
	got := GenerateTmuxSessionName("/Users/me/my.repo", "worktree-team/feature.v1")
	want := "my_repo_worktree-team_feature_v1"
	if got != want {
		t.Fatalf("GenerateTmuxSessionName() = %q, want %q", got, want)
	}
}

func sampleSession(sessionID string) Session {
	return Session{
		OriginalCwd:        "/repo",
		WorktreePath:       "/repo/.claude/worktrees/team+feature",
		WorktreeName:       "team/feature",
		WorktreeBranch:     "worktree-team+feature",
		OriginalBranch:     "main",
		OriginalHeadCommit: "abc123",
		SessionID:          sessionID,
		TmuxSessionName:    "Awaken_worktree-team+feature",
		HookBased:          true,
		CreationDurationMs: 12,
		UsedSparsePaths:    true,
	}
}
