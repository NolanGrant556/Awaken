package convrecovery

import (
	"path/filepath"
	"testing"
)

func TestRecoverFiltersUnresolvedToolUses(t *testing.T) {
	result := Recover([]Message{
		{
			Type: MessageTypeAssistant,
			ContentBlocks: []Block{
				{Type: BlockTypeToolUse, ID: "call_ok", Name: "lookup"},
				{Type: BlockTypeToolUse, ID: "call_missing", Name: "lookup"},
			},
		},
		{
			Type: MessageTypeUser,
			ContentBlocks: []Block{
				{Type: BlockTypeToolResult, ToolUseID: "call_ok", Text: "found"},
			},
		},
		{
			Type:          MessageTypeAssistant,
			ContentBlocks: []Block{{Type: BlockTypeText, Text: "done"}},
		},
	}, Options{})

	if len(result.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result.Messages))
	}
	blocks := result.Messages[0].ContentBlocks
	if len(blocks) != 1 || blocks[0].ID != "call_ok" {
		t.Fatalf("expected only resolved tool_use to remain, got %#v", blocks)
	}
	if result.TurnInterruptionState != TurnInterruptionNone {
		t.Fatalf("expected no interruption, got %q", result.TurnInterruptionState)
	}
}

func TestRecoverFiltersOrphanedThinkingOnlyAssistant(t *testing.T) {
	result := Recover([]Message{
		{
			Type:          MessageTypeAssistant,
			ID:            "orphan",
			ContentBlocks: []Block{{Type: BlockTypeThinking, Text: "private"}},
		},
		{
			Type:          MessageTypeAssistant,
			ID:            "paired",
			ContentBlocks: []Block{{Type: BlockTypeThinking, Text: "keep with paired content"}},
		},
		{
			Type:          MessageTypeAssistant,
			ID:            "paired",
			ContentBlocks: []Block{{Type: BlockTypeText, Text: "visible"}},
		},
	}, Options{})

	if len(result.Messages) != 2 {
		t.Fatalf("expected orphaned thinking-only message to be removed, got %#v", result.Messages)
	}
	if result.Messages[0].ID != "paired" || result.Messages[1].ID != "paired" {
		t.Fatalf("expected paired thinking/text messages to remain, got %#v", result.Messages)
	}
}

func TestRecoverFiltersWhitespaceOnlyAssistant(t *testing.T) {
	result := Recover([]Message{
		{
			Type: MessageTypeAssistant,
			ContentBlocks: []Block{
				{Type: BlockTypeText, Text: "\n\n\t"},
			},
		},
	}, Options{})

	if len(result.Messages) != 0 {
		t.Fatalf("expected whitespace-only assistant to be removed, got %#v", result.Messages)
	}
}

func TestRecoverClearsInvalidPermissionMode(t *testing.T) {
	result := Recover([]Message{
		{
			Type:           MessageTypeUser,
			PermissionMode: "definitely-not-a-mode",
			ContentBlocks:  []Block{{Type: BlockTypeText, Text: "hello"}},
		},
		{
			Type:           MessageTypeUser,
			PermissionMode: "default",
			ContentBlocks:  []Block{{Type: BlockTypeText, Text: "still valid"}},
		},
		{
			Type:          MessageTypeAssistant,
			ContentBlocks: []Block{{Type: BlockTypeText, Text: "done"}},
		},
	}, Options{})

	if result.Messages[0].PermissionMode != "" {
		t.Fatalf("expected invalid permission mode to be cleared, got %q", result.Messages[0].PermissionMode)
	}
	if result.Messages[1].PermissionMode != "default" {
		t.Fatalf("expected valid permission mode to remain, got %q", result.Messages[1].PermissionMode)
	}
}

func TestRecoverMigratesLegacyAttachmentDisplayPaths(t *testing.T) {
	cwd := t.TempDir()
	filePath := filepath.Join(cwd, "docs", "notes.md")
	dirPath := filepath.Join(cwd, "drafts")

	result := Recover([]Message{
		{
			Type:               MessageTypeAttachment,
			AttachmentType:     "new_file",
			AttachmentFilename: filePath,
		},
		{
			Type:           MessageTypeAttachment,
			AttachmentType: "new_directory",
			AttachmentPath: dirPath,
		},
		{
			Type:          MessageTypeAssistant,
			ContentBlocks: []Block{{Type: BlockTypeText, Text: "attachments processed"}},
		},
	}, Options{CWD: cwd})

	if got := result.Messages[0].AttachmentType; got != "file" {
		t.Fatalf("expected new_file to become file, got %q", got)
	}
	if got := result.Messages[0].DisplayPath; got != filepath.Join("docs", "notes.md") {
		t.Fatalf("expected file displayPath, got %q", got)
	}
	if got := result.Messages[1].AttachmentType; got != "directory" {
		t.Fatalf("expected new_directory to become directory, got %q", got)
	}
	if got := result.Messages[1].DisplayPath; got != "drafts" {
		t.Fatalf("expected directory displayPath, got %q", got)
	}
}

func TestRecoverPlainUserInterruptedPromptAddsSentinel(t *testing.T) {
	result := Recover([]Message{
		{
			Type:          MessageTypeUser,
			ContentBlocks: []Block{{Type: BlockTypeText, Text: "please continue"}},
		},
	}, Options{})

	if result.TurnInterruptionState != TurnInterruptionInterruptedPrompt {
		t.Fatalf("expected interrupted_prompt, got %q", result.TurnInterruptionState)
	}
	if result.ContinuationAdded {
		t.Fatal("did not expect continuation for interrupted prompt")
	}
	if !result.SentinelAdded {
		t.Fatal("expected sentinel to be added")
	}
	if len(result.Messages) != 2 || result.Messages[1].ContentBlocks[0].Text != NoResponseRequested {
		t.Fatalf("expected assistant sentinel after user, got %#v", result.Messages)
	}
}

func TestRecoverAttachmentInterruptedTurnAddsContinuationAndSentinel(t *testing.T) {
	result := Recover([]Message{
		{
			Type:           MessageTypeAttachment,
			AttachmentType: "file",
			AttachmentPath: "/tmp/example.txt",
		},
	}, Options{})

	if result.TurnInterruptionState != TurnInterruptionInterruptedTurn {
		t.Fatalf("expected interrupted_turn, got %q", result.TurnInterruptionState)
	}
	if !result.ContinuationAdded {
		t.Fatal("expected continuation to be added")
	}
	if !result.SentinelAdded {
		t.Fatal("expected sentinel to be added")
	}
	if len(result.Messages) != 3 {
		t.Fatalf("expected attachment, continuation, sentinel messages, got %#v", result.Messages)
	}
	if !result.Messages[1].IsMeta || result.Messages[1].ContentBlocks[0].Text != ContinuationPrompt {
		t.Fatalf("expected meta continuation user message, got %#v", result.Messages[1])
	}
	if result.Messages[2].Type != MessageTypeAssistant || result.Messages[2].ContentBlocks[0].Text != NoResponseRequested {
		t.Fatalf("expected assistant sentinel, got %#v", result.Messages[2])
	}
}

func TestRecoverTerminalToolResultDoesNotTriggerInterruptedTurn(t *testing.T) {
	result := Recover([]Message{
		{
			Type: MessageTypeAssistant,
			ContentBlocks: []Block{
				{Type: BlockTypeToolUse, ID: "call_terminal", Name: "SendUserMessage"},
			},
		},
		{
			Type: MessageTypeUser,
			ContentBlocks: []Block{
				{Type: BlockTypeToolResult, ToolUseID: "call_terminal", Text: "sent"},
			},
		},
	}, Options{})

	if result.TurnInterruptionState != TurnInterruptionNone {
		t.Fatalf("expected no interruption, got %q", result.TurnInterruptionState)
	}
	if result.ContinuationAdded {
		t.Fatal("terminal tool_result should not add continuation")
	}
	if !result.SentinelAdded {
		t.Fatal("expected sentinel after final user tool_result")
	}
}
