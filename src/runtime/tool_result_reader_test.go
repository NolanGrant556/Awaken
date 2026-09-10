package runtime

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"awaken/src/tool"
	toolresultread "awaken/src/tools/tool_result_read"
)

func TestToolResultReaderPagesOwnTextAndRejectsOtherAgents(t *testing.T) {
	setRuntimeUserDataDir(t)
	ctx := context.Background()
	store := NewToolResultStore()
	content := strings.Repeat("中文/🙂/line\n", 3500) + "TAIL-938"
	receipt, err := store.OffloadToolResult(ctx, "thread-owned", tool.ToolExecutionIdentity{AgentID: "child-A", SubagentTaskID: "old-assignment", RunID: "old-run", ToolCallID: "call"}, content)
	if err != nil {
		t.Fatal(err)
	}
	// Rebuild the store and read with stable Agent identity after a new assignment.
	store = NewToolResultStore()
	var joined strings.Builder
	for offset := 0; ; {
		page, err := store.ReadToolResult(ctx, "thread-owned", "child-A", tool.ToolResultReadRequest{StorageKey: receipt.StorageKey, Offset: offset, Limit: 2048})
		if err != nil {
			t.Fatal(err)
		}
		joined.WriteString(page.Content)
		if page.SHA256 != receipt.Digest || page.ByteSize != len(content) {
			t.Fatal("integrity metadata changed")
		}
		if !page.HasMore {
			break
		}
		if page.NextOffset <= offset {
			t.Fatal("cursor did not advance")
		}
		offset = page.NextOffset
	}
	if joined.String() != content {
		t.Fatal("paging lost exact Unicode content")
	}
	for _, scope := range [][2]string{{"thread-owned", "child-B"}, {"thread-owned", "root"}, {"another-thread", "child-A"}, {"thread-owned", ""}} {
		_, err := store.ReadToolResult(ctx, scope[0], scope[1], tool.ToolResultReadRequest{StorageKey: receipt.StorageKey})
		var denied *tool.ToolResultReadError
		if !errors.As(err, &denied) || denied.Code != "tool_result_not_owned" {
			t.Fatalf("scope %v not denied: %v", scope, err)
		}
	}
	for _, req := range []tool.ToolResultReadRequest{{StorageKey: receipt.StorageKey, Offset: -1}, {StorageKey: receipt.StorageKey, Limit: 2049}, {StorageKey: receipt.StorageKey, Offset: len([]rune(content)) + 1}, {StorageKey: "../outside/content.txt"}} {
		if _, err := store.ReadToolResult(ctx, "thread-owned", "child-A", req); err == nil {
			t.Fatalf("invalid request accepted: %+v", req)
		}
	}
	path, _ := ownedToolResultPath(receipt.StorageKey)
	if err := os.WriteFile(path, []byte("changed after receipt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadToolResult(ctx, "thread-owned", "child-A", tool.ToolResultReadRequest{StorageKey: receipt.StorageKey}); err == nil {
		t.Fatal("corrupted content accepted")
	}
}

func TestToolResultReadToolBindsIdentityAndKeepsPagesInline(t *testing.T) {
	setRuntimeUserDataDir(t)
	store := NewToolResultStore()
	text := strings.Repeat("原文🙂\n", 3000)
	receipt, err := store.OffloadToolResult(context.Background(), "thread-page", tool.ToolExecutionIdentity{AgentID: "agent-A", ToolCallID: "source-call"}, text)
	if err != nil {
		t.Fatal(err)
	}
	ctx := tool.ContextWithToolThreadID(context.Background(), "thread-page")
	ctx = tool.ContextWithModelContentStore(ctx, store)
	ctx = tool.ContextWithToolExecutionIdentity(ctx, tool.ToolExecutionIdentity{RunID: "new-run", AgentID: "agent-A", SubagentTaskID: "new-assignment", ToolName: toolresultread.ToolName, ToolCallID: "read-call"})
	raw := `{"storage_key":"` + receipt.StorageKey + `","offset":1024,"limit":2048}`
	outcome, err := (&toolresultread.Tool{}).ExecuteOutcome(ctx, raw, nil)
	if err != nil || outcome.IsError() {
		t.Fatalf("owned invocation failed: %+v %v", outcome, err)
	}
	if outcome.Content != string([]rune(text)[1024:3072]) || len(outcome.Content) > 12*1024 {
		t.Fatal("page changed text or exceeded inline budget")
	}
	if !strings.Contains(outcome.ModelPayload(), `"next_offset":3072`) {
		t.Fatal("continuation cursor missing from model payload")
	}
	ctx = tool.ContextWithToolExecutionIdentity(ctx, tool.ToolExecutionIdentity{RunID: "new-run", AgentID: "agent-B", ToolName: toolresultread.ToolName, ToolCallID: "read-call"})
	outcome, err = (&toolresultread.Tool{}).ExecuteOutcome(ctx, raw, nil)
	if err != nil || outcome.ErrorCode != "tool_result_not_owned" || strings.Contains(outcome.Content, "原文") {
		t.Fatalf("sibling read was not rejected: %+v %v", outcome, err)
	}
}
