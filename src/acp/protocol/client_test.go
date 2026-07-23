package protocol

import (
	"testing"
	"unicode/utf8"

	"awaken/src/acp"
)

func TestPromptStateAccumulatesTextAndTruncatesAtMaxTurn(t *testing.T) {
	client := &Client{maxTurn: 10}
	state := &promptState{
		client:     client,
		toolBuffer: make(map[string]acp.ToolCallSnapshot),
	}

	state.onUpdate(acp.SessionUpdate{
		Kind:      acp.SessionUpdateKindAgentText,
		TextDelta: "hello",
	})
	if state.textBuffer.String() != "hello" {
		t.Fatalf("expected text to accumulate, got %q", state.textBuffer.String())
	}
	if state.truncated {
		t.Fatal("expected not truncated yet")
	}

	// Add 10 more bytes; only the first 5 should be kept and truncation flagged.
	state.onUpdate(acp.SessionUpdate{
		Kind:      acp.SessionUpdateKindAgentText,
		TextDelta: "worldextra",
	})
	if !state.truncated {
		t.Fatal("expected truncated after crossing maxTurn")
	}
	if state.textBuffer.String() != "helloworld" {
		t.Fatalf("expected truncation at 10 bytes, got %q", state.textBuffer.String())
	}

	// Further updates must be ignored once truncated.
	state.onUpdate(acp.SessionUpdate{
		Kind:      acp.SessionUpdateKindAgentText,
		TextDelta: "more",
	})
	if state.textBuffer.String() != "helloworld" {
		t.Fatalf("expected no change after truncation, got %q", state.textBuffer.String())
	}
}

func TestPromptStateTruncatesAtUTF8RuneBoundary(t *testing.T) {
	// "中" is 3 bytes; maxTurn=4 should keep only one complete rune (3 bytes),
	// not 4 bytes which would split the second character.
	client := &Client{maxTurn: 4}
	state := &promptState{
		client:     client,
		toolBuffer: make(map[string]acp.ToolCallSnapshot),
	}

	state.onUpdate(acp.SessionUpdate{
		Kind:      acp.SessionUpdateKindAgentText,
		TextDelta: "中文",
	})

	if !state.truncated {
		t.Fatal("expected truncated")
	}
	if state.textBuffer.String() != "中" {
		t.Fatalf("expected one complete rune, got %q", state.textBuffer.String())
	}
	if !utf8.ValidString(state.textBuffer.String()) {
		t.Fatalf("accumulated text is not valid UTF-8: %q", state.textBuffer.String())
	}
}

func TestPromptStateComputesToolSummary(t *testing.T) {
	client := &Client{maxTurn: 1024}
	state := &promptState{
		client:     client,
		toolBuffer: make(map[string]acp.ToolCallSnapshot),
	}

	state.onUpdate(acp.SessionUpdate{
		Kind: acp.SessionUpdateKindToolCall,
		ToolCall: &acp.ToolCallSnapshot{
			ToolCallID: "tc-1",
			Status:     "completed",
		},
	})
	state.onUpdate(acp.SessionUpdate{
		Kind: acp.SessionUpdateKindToolCallUpdate,
		ToolCall: &acp.ToolCallSnapshot{
			ToolCallID: "tc-2",
			Status:     "failed",
		},
	})
	state.onUpdate(acp.SessionUpdate{
		Kind: acp.SessionUpdateKindToolCall,
		ToolCall: &acp.ToolCallSnapshot{
			ToolCallID: "tc-3",
			Status:     "pending",
		},
	})

	summary := state.toolSummary()
	if summary.Total != 3 {
		t.Fatalf("expected total=3, got %d", summary.Total)
	}
	if summary.Completed != 1 {
		t.Fatalf("expected completed=1, got %d", summary.Completed)
	}
	if summary.Failed != 1 {
		t.Fatalf("expected failed=1, got %d", summary.Failed)
	}
}
