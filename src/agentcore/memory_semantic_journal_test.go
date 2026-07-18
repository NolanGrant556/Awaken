package agentcore

import (
	"strings"
	"testing"

	convcontext "awaken/src/context"
	_ "awaken/src/tools/memory"
)

func TestMemorySemanticRecordsPersistReceiptsWithoutBody(t *testing.T) {
	call := convcontext.ToolCall{ID: "call_memory", Function: convcontext.FunctionCall{Name: "memory", Arguments: `{"action":"create","scope":"user","type":"feedback","name":"Private","description":"private description","content":"private body"}`}}
	callRecord := ToolCallSemanticRecord("thread", "run", "turn", "session", "parent", call)
	arguments, _ := callRecord.Payload["arguments"].(string)
	if strings.Contains(arguments, "private") {
		t.Fatalf("memory call body persisted: %s", arguments)
	}

	resultRecord := ToolResultSemanticRecord("thread", "run", "turn", "session", ToolExecutionResult{Name: "memory", ToolCallID: "call_memory", Content: `{"memory_id":"mem_1","content":"private body","revision":1}`})
	content, _ := resultRecord.Payload["content"].(string)
	if strings.Contains(content, "private body") || !strings.Contains(content, "mem_1") {
		t.Fatalf("unexpected memory receipt: %s", content)
	}
}
