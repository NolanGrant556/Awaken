package sessiontags

import (
	"reflect"
	"testing"
)

func TestApplyKeepsCustomTitleAsDisplayTitle(t *testing.T) {
	metadata := Apply(Metadata{}, AITitleEntry{
		SessionID: "session-1",
		AITitle:   "AI generated title",
	})
	if got := metadata.DisplayTitle(); got != "AI generated title" {
		t.Fatalf("expected AI title before custom title, got %q", got)
	}

	metadata = Apply(metadata, CustomTitleEntry{
		SessionID:   "session-1",
		CustomTitle: "User title",
	})
	metadata = Apply(metadata, AITitleEntry{
		SessionID: "session-1",
		AITitle:   "New AI title",
	})

	if metadata.AITitle != "New AI title" {
		t.Fatalf("expected AI title to update, got %q", metadata.AITitle)
	}
	if got := metadata.DisplayTitle(); got != "User title" {
		t.Fatalf("expected custom display title to win, got %q", got)
	}
}

func TestIndexAppliesLastWinsMetadata(t *testing.T) {
	worktree := &WorktreeSession{
		OriginalCwd:        "/repo",
		WorktreePath:       "/repo-worktrees/new",
		WorktreeName:       "new",
		WorktreeBranch:     "codex/new",
		OriginalBranch:     "main",
		OriginalHeadCommit: "abc123",
		SessionID:          "worktree-session",
		TmuxSessionName:    "awaken-new",
		HookBased:          true,
	}

	index := NewIndex(
		TagEntry{SessionID: "session-1", Tag: "old-tag"},
		TagEntry{SessionID: "session-1", Tag: "new-tag"},
		LastPromptEntry{SessionID: "session-1", LastPrompt: "old prompt"},
		LastPromptEntry{SessionID: "session-1", LastPrompt: "new prompt"},
		TaskSummaryEntry{SessionID: "session-1", Summary: "old summary", Timestamp: "2026-06-27T00:00:00Z"},
		TaskSummaryEntry{SessionID: "session-1", Summary: "new summary", Timestamp: "2026-06-27T00:01:00Z"},
		AgentNameEntry{SessionID: "session-1", AgentName: "old-agent"},
		AgentNameEntry{SessionID: "session-1", AgentName: "new-agent"},
		AgentColorEntry{SessionID: "session-1", AgentColor: "blue"},
		AgentColorEntry{SessionID: "session-1", AgentColor: "green"},
		AgentSettingEntry{SessionID: "session-1", AgentSetting: "default"},
		AgentSettingEntry{SessionID: "session-1", AgentSetting: "reviewer"},
		ModeEntry{SessionID: "session-1", Mode: ModeNormal},
		ModeEntry{SessionID: "session-1", Mode: ModeCoordinator},
		WorktreeStateEntry{SessionID: "session-1", WorktreeSession: nil},
		WorktreeStateEntry{SessionID: "session-1", WorktreeSession: worktree},
	)

	metadata, ok := index.Metadata("session-1")
	if !ok {
		t.Fatalf("expected metadata for session-1")
	}

	if metadata.Tag != "new-tag" {
		t.Fatalf("expected last tag, got %q", metadata.Tag)
	}
	if metadata.LastPrompt != "new prompt" {
		t.Fatalf("expected last prompt, got %q", metadata.LastPrompt)
	}
	if metadata.TaskSummary != "new summary" || metadata.TaskSummaryTimestamp != "2026-06-27T00:01:00Z" {
		t.Fatalf("expected last task summary, got %+v", metadata)
	}
	if metadata.AgentName != "new-agent" || metadata.AgentColor != "green" || metadata.AgentSetting != "reviewer" {
		t.Fatalf("expected last agent metadata, got %+v", metadata)
	}
	if metadata.Mode != ModeCoordinator {
		t.Fatalf("expected last mode, got %q", metadata.Mode)
	}
	if metadata.WorktreeSession == nil || metadata.WorktreeSession.WorktreePath != "/repo-worktrees/new" {
		t.Fatalf("expected last worktree state, got %+v", metadata.WorktreeSession)
	}

	worktree.WorktreePath = "/mutated"
	metadata.WorktreeSession.WorktreePath = "/returned-mutated"

	metadata, ok = index.Metadata("session-1")
	if !ok {
		t.Fatalf("expected metadata for session-1")
	}
	if metadata.WorktreeSession == nil || metadata.WorktreeSession.WorktreePath != "/repo-worktrees/new" {
		t.Fatalf("expected index to keep an immutable worktree snapshot, got %+v", metadata.WorktreeSession)
	}
}

func TestIndexUpdatesTagLookup(t *testing.T) {
	index := NewIndex(
		TagEntry{SessionID: "session-1", Tag: "frontend"},
		TagEntry{SessionID: "session-2", Tag: "frontend"},
		TagEntry{SessionID: "session-1", Tag: "backend"},
	)

	if got, want := index.SessionIDsByTag("frontend"), []string{"session-2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SessionIDsByTag(frontend) = %#v, want %#v", got, want)
	}
	if got, want := index.SessionIDsByTag("backend"), []string{"session-1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SessionIDsByTag(backend) = %#v, want %#v", got, want)
	}

	index.Apply(TagEntry{SessionID: "session-2", Tag: ""})
	if got := index.SessionIDsByTag("frontend"); len(got) != 0 {
		t.Fatalf("expected empty frontend lookup after clearing tag, got %#v", got)
	}
}

func TestIndexSearchMatchesDisplayTitleAndLastPrompt(t *testing.T) {
	index := NewIndex(
		AITitleEntry{SessionID: "session-1", AITitle: "Draft AI title"},
		CustomTitleEntry{SessionID: "session-1", CustomTitle: "User rename"},
		AITitleEntry{SessionID: "session-2", AITitle: "Backend audit"},
		LastPromptEntry{SessionID: "session-2", LastPrompt: "Fix startup command"},
		LastPromptEntry{SessionID: "session-3", LastPrompt: "Review session tags"},
	)

	if got, want := index.SearchSessionIDs("user"), []string{"session-1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SearchSessionIDs(user) = %#v, want %#v", got, want)
	}
	if got, want := index.SearchSessionIDs("STARTUP"), []string{"session-2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SearchSessionIDs(STARTUP) = %#v, want %#v", got, want)
	}
	if got, want := index.SearchSessionIDs("session tags"), []string{"session-3"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SearchSessionIDs(session tags) = %#v, want %#v", got, want)
	}
	if got := index.SearchSessionIDs("draft"); len(got) != 0 {
		t.Fatalf("expected search to use display title instead of hidden AI title, got %#v", got)
	}
}

func TestFoldReturnsCurrentMetadataBySession(t *testing.T) {
	folded := Fold(
		LastPromptEntry{SessionID: "session-1", LastPrompt: "first"},
		LastPromptEntry{SessionID: "session-1", LastPrompt: "second"},
		TagEntry{SessionID: "session-2", Tag: "tagged"},
	)

	if folded["session-1"].LastPrompt != "second" {
		t.Fatalf("expected folded last prompt, got %+v", folded["session-1"])
	}
	if folded["session-2"].Tag != "tagged" {
		t.Fatalf("expected folded tag, got %+v", folded["session-2"])
	}
}
