package sessionbranch

import (
	"errors"
	"testing"
)

func TestDeriveFirstPrompt(t *testing.T) {
	tests := []struct {
		name     string
		messages []SerializedMessage
		want     string
	}{
		{
			name: "string content",
			messages: []SerializedMessage{{
				Type:    TypeUser,
				Content: "Implement the branch command",
			}},
			want: "Implement the branch command",
		},
		{
			name: "block content",
			messages: []SerializedMessage{{
				Type: TypeUser,
				Content: []ContentBlock{
					{Type: "image", Text: "ignored"},
					{Type: "text", Text: "Use the first text block"},
				},
			}},
			want: "Use the first text block",
		},
		{
			name: "generic block content",
			messages: []SerializedMessage{{
				Type: TypeUser,
				Content: []any{
					map[string]any{"type": "tool_result", "text": "ignored"},
					map[string]any{"type": "text", "text": "Text from decoded JSON"},
				},
			}},
			want: "Text from decoded JSON",
		},
		{
			name: "multiline collapse",
			messages: []SerializedMessage{{
				Type:    TypeUser,
				Content: "  first line\n\n\tsecond   line  ",
			}},
			want: "first line second line",
		},
		{
			name: "empty fallback",
			messages: []SerializedMessage{{
				Type:    TypeUser,
				Content: " \n\t ",
			}},
			want: DefaultFirstPrompt,
		},
		{
			name: "no user fallback",
			messages: []SerializedMessage{{
				Type:    TypeAssistant,
				Content: "assistant only",
			}},
			want: DefaultFirstPrompt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveFirstPrompt(tt.messages)
			if got != tt.want {
				t.Fatalf("DeriveFirstPrompt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildForkRewritesSessionAndForkedFrom(t *testing.T) {
	entries := []Entry{
		messageEntry("user-1", TypeUser, "session-original", nil, "Start here"),
		messageEntry("assistant-1", TypeAssistant, "session-original", ptr("user-1"), "Done"),
	}
	entries[0].Metadata = map[string]any{"timestamp": "2026-06-28T00:00:00Z"}
	entries[1].Metadata = map[string]any{"gitBranch": "main"}

	fork, err := BuildFork(entries, Options{ForkSessionID: "session-fork"})
	if err != nil {
		t.Fatalf("BuildFork() returned error: %v", err)
	}

	if fork.SessionID != "session-fork" {
		t.Fatalf("fork session id = %q, want session-fork", fork.SessionID)
	}
	if fork.OriginalSessionID != "session-original" {
		t.Fatalf("original session id = %q, want session-original", fork.OriginalSessionID)
	}
	if fork.Title != "Start here (Branch)" {
		t.Fatalf("title = %q, want Start here (Branch)", fork.Title)
	}
	if len(fork.Messages) != 2 {
		t.Fatalf("message count = %d, want 2", len(fork.Messages))
	}

	for i, message := range fork.Messages {
		if message.SessionID != "session-fork" {
			t.Fatalf("message %d session id = %q, want session-fork", i, message.SessionID)
		}
		if message.IsSidechain {
			t.Fatalf("message %d is sidechain", i)
		}
		if message.ForkedFrom == nil {
			t.Fatalf("message %d missing forkedFrom", i)
		}
		if message.ForkedFrom.SessionID != "session-original" {
			t.Fatalf("message %d forkedFrom session = %q, want session-original", i, message.ForkedFrom.SessionID)
		}
		if message.ForkedFrom.MessageUUID != entries[i].UUID {
			t.Fatalf("message %d forkedFrom uuid = %q, want %q", i, message.ForkedFrom.MessageUUID, entries[i].UUID)
		}
	}

	if fork.Messages[0].ParentUUID != nil {
		t.Fatalf("first parent = %q, want nil", *fork.Messages[0].ParentUUID)
	}
	assertStringPtr(t, fork.Messages[1].ParentUUID, "user-1")
	if fork.Messages[1].Metadata["gitBranch"] != "main" {
		t.Fatalf("metadata was not preserved: %#v", fork.Messages[1].Metadata)
	}

	fork.Messages[1].Metadata["gitBranch"] = "changed"
	if entries[1].Metadata["gitBranch"] != "main" {
		t.Fatalf("input metadata was mutated: %#v", entries[1].Metadata)
	}
}

func TestBuildForkProgressDoesNotBecomeParent(t *testing.T) {
	entries := []Entry{
		messageEntry("user-1", TypeUser, "session-original", nil, "Start"),
		messageEntry("progress-1", TypeProgress, "session-original", ptr("user-1"), "working"),
		messageEntry("assistant-1", TypeAssistant, "session-original", ptr("progress-1"), "Done"),
	}

	fork, err := BuildFork(entries, Options{ForkSessionID: "session-fork"})
	if err != nil {
		t.Fatalf("BuildFork() returned error: %v", err)
	}

	if len(fork.Messages) != 2 {
		t.Fatalf("message count = %d, want 2", len(fork.Messages))
	}
	for _, message := range fork.Messages {
		if message.Type == TypeProgress {
			t.Fatalf("progress message was copied into fork")
		}
	}
	assertStringPtr(t, fork.Messages[1].ParentUUID, "user-1")
}

func TestBuildForkExcludesSidechains(t *testing.T) {
	entries := []Entry{
		messageEntry("user-1", TypeUser, "session-original", nil, "Start"),
		messageEntry("sidechain-1", TypeAssistant, "session-original", ptr("user-1"), "side work"),
		messageEntry("assistant-1", TypeAssistant, "session-original", ptr("sidechain-1"), "Done"),
	}
	entries[1].IsSidechain = true

	fork, err := BuildFork(entries, Options{ForkSessionID: "session-fork"})
	if err != nil {
		t.Fatalf("BuildFork() returned error: %v", err)
	}

	if len(fork.Messages) != 2 {
		t.Fatalf("message count = %d, want 2", len(fork.Messages))
	}
	for _, message := range fork.Messages {
		if message.UUID == "sidechain-1" {
			t.Fatalf("sidechain message was copied into fork")
		}
	}
	assertStringPtr(t, fork.Messages[1].ParentUUID, "user-1")
}

func TestBuildForkCopiesContentReplacements(t *testing.T) {
	entries := []Entry{
		messageEntry("user-1", TypeUser, "session-original", nil, "Start"),
		{
			SerializedMessage: SerializedMessage{
				Type:      TypeContentReplacement,
				SessionID: "session-original",
			},
			Replacements: []ContentReplacementRecord{{
				Kind:        "tool-result",
				ToolUseID:   "tool-1",
				Replacement: "preview",
				Metadata:    map[string]any{"bytes": 42},
			}},
		},
		{
			SerializedMessage: SerializedMessage{
				Type:      TypeContentReplacement,
				SessionID: "other-session",
			},
			Replacements: []ContentReplacementRecord{{
				Kind:        "tool-result",
				ToolUseID:   "tool-other",
				Replacement: "other",
			}},
		},
	}

	fork, err := BuildFork(entries, Options{ForkSessionID: "session-fork"})
	if err != nil {
		t.Fatalf("BuildFork() returned error: %v", err)
	}

	if len(fork.ContentReplacements) != 1 {
		t.Fatalf("content replacement count = %d, want 1", len(fork.ContentReplacements))
	}
	record := fork.ContentReplacements[0]
	if record.ToolUseID != "tool-1" || record.Replacement != "preview" {
		t.Fatalf("replacement record = %#v, want original session record", record)
	}
	if record.Metadata["bytes"] != 42 {
		t.Fatalf("replacement metadata was not preserved: %#v", record.Metadata)
	}

	lastEntry := fork.Entries[len(fork.Entries)-1]
	if lastEntry.Type != TypeContentReplacement {
		t.Fatalf("last entry type = %q, want content-replacement", lastEntry.Type)
	}
	if lastEntry.SessionID != "session-fork" {
		t.Fatalf("replacement session id = %q, want session-fork", lastEntry.SessionID)
	}
	if len(lastEntry.Replacements) != 1 || lastEntry.Replacements[0].ToolUseID != "tool-1" {
		t.Fatalf("fork replacement entry = %#v, want copied original records", lastEntry.Replacements)
	}
}

func TestBuildForkTitleUsesCustomTitle(t *testing.T) {
	entries := []Entry{
		messageEntry("user-1", TypeUser, "session-original", nil, "Use first prompt"),
	}

	fork, err := BuildFork(entries, Options{
		ForkSessionID: "session-fork",
		CustomTitle:   "Custom title",
		ExistingTitles: []string{
			"Custom title (Branch)",
		},
	})
	if err != nil {
		t.Fatalf("BuildFork() returned error: %v", err)
	}

	if fork.Title != "Custom title (Branch 2)" {
		t.Fatalf("title = %q, want Custom title (Branch 2)", fork.Title)
	}
}

func TestBuildForkErrors(t *testing.T) {
	_, err := BuildFork(nil, Options{ForkSessionID: "session-fork"})
	if !errors.Is(err, ErrNoTranscript) {
		t.Fatalf("BuildFork(nil) error = %v, want ErrNoTranscript", err)
	}

	_, err = BuildFork([]Entry{{
		SerializedMessage: SerializedMessage{
			Type:      TypeContentReplacement,
			SessionID: "session-original",
		},
	}}, Options{ForkSessionID: "session-fork"})
	if !errors.Is(err, ErrNoMessages) {
		t.Fatalf("BuildFork(metadata only) error = %v, want ErrNoMessages", err)
	}
}

func TestUniqueForkName(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		existing []string
		want     string
	}{
		{
			name: "first branch",
			base: "Plan",
			want: "Plan (Branch)",
		},
		{
			name:     "second branch",
			base:     "Plan",
			existing: []string{"Plan (Branch)"},
			want:     "Plan (Branch 2)",
		},
		{
			name:     "fills gap",
			base:     "Plan",
			existing: []string{"Plan (Branch)", "Plan (Branch 3)"},
			want:     "Plan (Branch 2)",
		},
		{
			name:     "third branch",
			base:     "Plan",
			existing: []string{"Plan (Branch)", "Plan (Branch 2)"},
			want:     "Plan (Branch 3)",
		},
		{
			name:     "regex-like base",
			base:     "Fix (a+b)?",
			existing: []string{"Fix (a+b)? (Branch)"},
			want:     "Fix (a+b)? (Branch 2)",
		},
		{
			name:     "numbered branch does not block bare branch",
			base:     "Plan",
			existing: []string{"Plan (Branch 2)"},
			want:     "Plan (Branch)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UniqueForkName(tt.base, tt.existing)
			if got != tt.want {
				t.Fatalf("UniqueForkName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func messageEntry(uuid, messageType, sessionID string, parentUUID *string, content any) Entry {
	return Entry{
		SerializedMessage: SerializedMessage{
			UUID:       uuid,
			Type:       messageType,
			SessionID:  sessionID,
			ParentUUID: parentUUID,
			Content:    content,
		},
	}
}

func assertStringPtr(t *testing.T, got *string, want string) {
	t.Helper()
	if got == nil {
		t.Fatalf("got nil, want %q", want)
	}
	if *got != want {
		t.Fatalf("got %q, want %q", *got, want)
	}
}

func ptr(value string) *string {
	return &value
}
