package protocol

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"awaken/src/acp"
)

func TestMissingManagerDoesNotLaunchPackageRunner(t *testing.T) {
	_, err := NewClient(context.Background(), acp.RunnerSnapshot{ID: "package", Command: "npx", Args: []string{"-y", "example-acp"}}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "environment_manager_unavailable") {
		t.Fatalf("unexpected runner result: %v", err)
	}
}

func TestClientProcessOutlivesStartCallContext(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	binaryName := "mock-agent"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./testdata/acp/mock-agent")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build mock ACP agent: %v\n%s", err, output)
	}

	startCtx, cancelStart := context.WithCancel(context.Background())
	client, err := NewClient(startCtx, acp.RunnerSnapshot{
		ID: "mock", Command: binaryPath, StartupTimeoutSeconds: 15,
		TurnTimeoutSeconds: 15, MaxFrameBytes: 1024 * 1024,
	}, t.TempDir())
	if err != nil {
		cancelStart()
		t.Fatalf("start client: %v", err)
	}
	defer client.Close()

	sessionID, err := client.NewSession(startCtx, t.TempDir())
	if err != nil {
		cancelStart()
		t.Fatalf("new session: %v", err)
	}
	cancelStart()

	select {
	case <-client.Done():
		t.Fatalf("ACP process exited with start-call context: %v", client.ExitError())
	case <-time.After(250 * time.Millisecond):
	}

	promptCtx, cancelPrompt := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelPrompt()
	result, err := client.Prompt(promptCtx, sessionID, "turn-after-start", "still alive", nil, nil)
	if err != nil {
		t.Fatalf("prompt after start context ended: %v", err)
	}
	if result.StopReason != "end_turn" || !strings.Contains(result.AssistantText, "still alive") {
		t.Fatalf("unexpected prompt result: %+v", result)
	}
}

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
