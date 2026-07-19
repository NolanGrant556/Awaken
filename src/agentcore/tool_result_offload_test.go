package agentcore

import (
	"context"
	"strings"
	"testing"
)

type fakeToolResultOffloadStore struct {
	content string
}

func (s *fakeToolResultOffloadStore) OffloadToolResult(_ context.Context, _, _ string, content string) (ToolResultOffloadReceipt, error) {
	s.content = content
	return ToolResultOffloadReceipt{StorageKey: "thread/tool-results/call/content.txt", Digest: "abc123", ByteSize: len(content), Preview: "stable preview"}, nil
}

func TestRunnerOffloadsLargeToolResultBeforeAdmission(t *testing.T) {
	store := &fakeToolResultOffloadStore{}
	runner := &Runner{threadID: "thread", config: AgentConfig{ToolResultOffloadStore: store}}
	original := strings.Repeat("全量工具结果", 7000)
	result := ToolExecutionResult{ToolCallID: "call", Content: original}
	if err := runner.offloadLargeToolResult(context.Background(), &result); err != nil {
		t.Fatal(err)
	}
	if store.content != original {
		t.Fatal("offload store did not receive the exact full result")
	}
	if !strings.Contains(result.Content, "[TOOL_RESULT_OFFLOADED]") || !strings.Contains(result.Content, "sha256=abc123") || !result.Truncated {
		t.Fatalf("missing stable admission projection: %+v", result)
	}
	if strings.Contains(result.Content, strings.Repeat("全量工具结果", 50)) {
		t.Fatal("full content leaked into the model admission projection")
	}
}
