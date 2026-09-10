package agentcore

import (
	"context"
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestAcceptanceFullResultSurvivesRuntimeFinalization(t *testing.T) {
	for _, size := range []int{15000, 45614} {
		original := strings.Repeat("a", size/2) + "MIDDLE-MUST-SURVIVE-582" + strings.Repeat("z", size/2)
		store := &fakeToolResultOffloadStore{receipt: ToolResultOffloadReceipt{StorageKey: "thread/tool-results/root/call/content.txt", Digest: "test-digest", ByteSize: len(original)}}
		runner := &Runner{threadID: "thread", config: AgentConfig{ToolResultOffloadStore: store}}
		result := finalizeToolResult(ToolExecutionResult{ToolCallID: "call", Name: "subagent", Content: original, Outcome: tool.SucceededOutcome(original)})
		if err := runner.offloadLargeToolResult(context.Background(), &result); err != nil {
			t.Fatal(err)
		}
		if store.content != original {
			t.Errorf("%d-byte result was lost before offload: stored %d bytes; projected middle present=%v", len(original), len(store.content), strings.Contains(result.Outcome.ModelPayload(), "MIDDLE-MUST-SURVIVE-582"))
		}
		if !strings.Contains(result.Outcome.ModelPayload(), "[TOOL_RESULT_OFFLOADED]") {
			t.Errorf("%d-byte result has no model-visible full-result retrieval handle", len(original))
		}
	}
}
