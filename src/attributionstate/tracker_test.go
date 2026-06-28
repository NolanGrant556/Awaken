package attributionstate

import "testing"

func TestTrackerSnapshotCopies(t *testing.T) {
	var tracker Tracker
	original := FileState{ContentHash: "hash-1", ContributionChars: 12, MTimeUnix: 100}
	updated := FileState{ContentHash: "hash-2", ContributionChars: 24, MTimeUnix: 200}

	tracker.TrackFile("notes/a.md", original)
	tracker.UpdatePromptStats(2, 1, 0)

	current := tracker.Snapshot()
	current.FileStates["notes/a.md"] = FileState{ContentHash: "mutated"}
	current.FileStates["notes/b.md"] = FileState{ContentHash: "new"}

	afterMutation := tracker.Snapshot()
	if got := afterMutation.FileStates["notes/a.md"]; got != original {
		t.Fatalf("Snapshot exposed internal file state, got %+v want %+v", got, original)
	}
	if _, ok := afterMutation.FileStates["notes/b.md"]; ok {
		t.Fatalf("Snapshot exposed internal map; unexpected notes/b.md entry")
	}

	made := tracker.MakeSnapshot("msg-1")
	made.FileStates["notes/a.md"] = FileState{ContentHash: "mutated-again"}

	tracker.TrackFile("notes/a.md", updated)

	latest, ok := tracker.Latest()
	if !ok {
		t.Fatalf("expected latest snapshot")
	}
	if latest.MessageID != "msg-1" {
		t.Fatalf("expected latest message id msg-1, got %q", latest.MessageID)
	}
	if got := latest.FileStates["notes/a.md"]; got != original {
		t.Fatalf("Latest should preserve snapshot copy, got %+v want %+v", got, original)
	}
	if latest.PromptCount != 2 || latest.PermissionPromptCount != 1 || latest.EscapeCount != 0 {
		t.Fatalf("unexpected prompt stats in latest snapshot: %+v", latest)
	}

	latest.FileStates["notes/a.md"] = FileState{ContentHash: "latest-mutated"}
	latestAgain, ok := tracker.Latest()
	if !ok {
		t.Fatalf("expected latest snapshot after mutation")
	}
	if got := latestAgain.FileStates["notes/a.md"]; got != original {
		t.Fatalf("Latest exposed internal map, got %+v want %+v", got, original)
	}

	currentAfterUpdate := tracker.Snapshot()
	if got := currentAfterUpdate.FileStates["notes/a.md"]; got != updated {
		t.Fatalf("Snapshot should reflect current tracked state, got %+v want %+v", got, updated)
	}
}

func TestTrackerSnapshotSequence(t *testing.T) {
	tracker := NewTracker()
	if tracker.SnapshotSequence() != 0 {
		t.Fatalf("expected initial sequence 0, got %d", tracker.SnapshotSequence())
	}

	tracker.MakeSnapshot("msg-1")
	tracker.MakeSnapshot("msg-2")

	if tracker.SnapshotSequence() != 2 {
		t.Fatalf("expected sequence 2, got %d", tracker.SnapshotSequence())
	}
	latest, ok := tracker.Latest()
	if !ok {
		t.Fatalf("expected latest snapshot")
	}
	if latest.MessageID != "msg-2" {
		t.Fatalf("expected latest snapshot to be msg-2, got %q", latest.MessageID)
	}
}

func TestCountUserPrompts(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		want     int
	}{
		{
			name:     "plain user text counts",
			messages: []Message{{Type: MessageTypeUser, Content: "summarize this"}},
			want:     1,
		},
		{
			name:     "empty text is ignored",
			messages: []Message{{Type: MessageTypeUser, Content: " \n\t "}},
			want:     0,
		},
		{
			name:     "sidechain user text is ignored",
			messages: []Message{{Type: MessageTypeUser, IsSidechain: true, Content: "hidden steer"}},
			want:     0,
		},
		{
			name:     "terminal output string is ignored",
			messages: []Message{{Type: MessageTypeUser, Content: "<bash-output>ok</bash-output>"}},
			want:     0,
		},
		{
			name: "tool result only is ignored",
			messages: []Message{{
				Type:          MessageTypeUser,
				ContentBlocks: []ContentBlock{{Type: BlockTypeToolResult, Text: "result"}},
			}},
			want: 0,
		},
		{
			name: "terminal text block is ignored",
			messages: []Message{{
				Type:          MessageTypeUser,
				ContentBlocks: []ContentBlock{{Type: BlockTypeText, Text: "<bash-input>ls</bash-input>"}},
			}},
			want: 0,
		},
		{
			name: "text block counts",
			messages: []Message{{
				Type:          MessageTypeUser,
				ContentBlocks: []ContentBlock{{Type: BlockTypeText, Text: "please inspect this"}},
			}},
			want: 1,
		},
		{
			name: "image block counts",
			messages: []Message{{
				Type:          MessageTypeUser,
				ContentBlocks: []ContentBlock{{Type: BlockTypeImage}},
			}},
			want: 1,
		},
		{
			name: "document block counts",
			messages: []Message{{
				Type:          MessageTypeUser,
				ContentBlocks: []ContentBlock{{Type: BlockTypeDocument}},
			}},
			want: 1,
		},
		{
			name:     "assistant text is ignored",
			messages: []Message{{Type: MessageTypeAssistant, Content: "assistant output"}},
			want:     0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := CountUserPrompts(test.messages); got != test.want {
				t.Fatalf("CountUserPrompts() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestCountMemoryFileAccess(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		want     int
	}{
		{
			name: "assistant file read memory path counts",
			messages: []Message{{
				Type: MessageTypeAssistant,
				ContentBlocks: []ContentBlock{{
					Type: BlockTypeToolUse,
					Name: "file_read",
					Input: ToolInput{
						Path: "/Users/nolangrant/.codex/memories/MEMORY.md",
					},
				}},
			}},
			want: 1,
		},
		{
			name: "assistant grep explicit memory access counts",
			messages: []Message{{
				Type: MessageTypeAssistant,
				ContentBlocks: []ContentBlock{{
					Type: BlockTypeToolUse,
					Name: "grep_search",
					Input: ToolInput{
						MemoryAccess: true,
					},
				}},
			}},
			want: 1,
		},
		{
			name: "block explicit memory access counts",
			messages: []Message{{
				Type: MessageTypeAssistant,
				ContentBlocks: []ContentBlock{{
					Type:         BlockTypeToolUse,
					Name:         "list_files",
					MemoryAccess: true,
				}},
			}},
			want: 1,
		},
		{
			name: "path list memory access counts",
			messages: []Message{{
				Type: MessageTypeAssistant,
				ContentBlocks: []ContentBlock{{
					Type: BlockTypeToolUse,
					Name: "apply_patch",
					Input: ToolInput{
						Paths: []string{"src/main.go", "/Users/nolangrant/.codex/memories/notes/today.md"},
					},
				}},
			}},
			want: 1,
		},
		{
			name: "non memory path does not count",
			messages: []Message{{
				Type: MessageTypeAssistant,
				ContentBlocks: []ContentBlock{{
					Type:  BlockTypeToolUse,
					Name:  "file_read",
					Input: ToolInput{Path: "AGENTS.md"},
				}},
			}},
			want: 0,
		},
		{
			name: "non file tool does not count even with memory path",
			messages: []Message{{
				Type: MessageTypeAssistant,
				ContentBlocks: []ContentBlock{{
					Type:  BlockTypeToolUse,
					Name:  "shell",
					Input: ToolInput{Path: "/Users/nolangrant/.codex/memories/MEMORY.md"},
				}},
			}},
			want: 0,
		},
		{
			name: "user tool use does not count",
			messages: []Message{{
				Type: MessageTypeUser,
				ContentBlocks: []ContentBlock{{
					Type:  BlockTypeToolUse,
					Name:  "file_read",
					Input: ToolInput{Path: "/Users/nolangrant/.codex/memories/MEMORY.md"},
				}},
			}},
			want: 0,
		},
		{
			name: "sidechain assistant tool use does not count",
			messages: []Message{{
				Type:        MessageTypeAssistant,
				IsSidechain: true,
				ContentBlocks: []ContentBlock{{
					Type:  BlockTypeToolUse,
					Name:  "file_read",
					Input: ToolInput{Path: "/Users/nolangrant/.codex/memories/MEMORY.md"},
				}},
			}},
			want: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := CountMemoryFileAccess(test.messages); got != test.want {
				t.Fatalf("CountMemoryFileAccess() = %d, want %d", got, test.want)
			}
		})
	}
}
