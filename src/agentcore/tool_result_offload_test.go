package agentcore

import (
	stdcontext "context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
)

type fakeToolResultOffloadStore struct {
	content string
	receipt ToolResultOffloadReceipt
}

func (s *fakeToolResultOffloadStore) OffloadToolResult(_ stdcontext.Context, _ string, _ tool.ToolExecutionIdentity, content string) (ToolResultOffloadReceipt, error) {
	s.content = content
	return s.receipt, nil
}

func (s *fakeToolResultOffloadStore) HydrateOffloadedToolResult(_ stdcontext.Context, _ string, receipt ToolResultOffloadReceipt) (string, error) {
	s.receipt = receipt
	return s.content, nil
}

func TestRunnerOffloadsLargeToolResultBeforeAdmission(t *testing.T) {
	store := &fakeToolResultOffloadStore{receipt: ToolResultOffloadReceipt{StorageKey: "thread/tool-results/root/call/content.txt", Digest: "abc123", Preview: "stable preview"}}
	runner := &Runner{threadID: "thread", config: AgentConfig{ToolResultOffloadStore: store}}
	original := strings.Repeat("全量工具结果", 7000)
	result := ToolExecutionResult{ToolCallID: "call", Content: original}
	if err := runner.offloadLargeToolResult(stdcontext.Background(), &result); err != nil {
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

func TestBuildLLMRequestKeepsOffloadedToolResultStub(t *testing.T) {
	original := strings.Repeat("child result ", 4000)
	marker := "[TOOL_RESULT_OFFLOADED]\nstorage_key=thread/tool-results/task/call/content.txt\nsha256=abc123\nbytes=52000"
	store := &fakeToolResultOffloadStore{
		content: original,
		receipt: ToolResultOffloadReceipt{StorageKey: "thread/tool-results/task/call/content.txt", Digest: "abc123", ByteSize: len(original)},
	}
	conv := convcontext.NewConversation(nil)
	conv.Context.SetMessages([]convcontext.Message{
		{Role: convcontext.RoleUser, Content: "run the tool"},
		{Role: convcontext.RoleAssistant, ToolCalls: []convcontext.ToolCall{{
			ID: "call", Function: convcontext.FunctionCall{Name: "wait"},
		}}},
		{Role: convcontext.RoleTool, ToolCallID: "call", Name: "wait", Content: marker},
	})
	runner := NewRunner(AgentConfig{ToolResultOffloadStore: store}, conv, NewRequestBuilder(AgentConfig{}).BindThread("thread"))
	runner.threadID = "thread"
	req, err := runner.buildLLMRequest()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(req.Messages[2].Content, "[TOOL_RESULT_OFFLOADED]") {
		t.Fatalf("model request should keep the offload stub, got %q", req.Messages[2].Content)
	}
	if strings.Contains(req.Messages[2].Content, original) {
		t.Fatal("offloaded tool result was fully hydrated into the provider request")
	}
}

func TestHydrateOffloadedToolResultHelperStillRestoresFullText(t *testing.T) {
	original := strings.Repeat("full wait result ", 3000)
	marker := "[TOOL_RESULT_OFFLOADED]\nstorage_key=thread/tool-results/task/call/content.txt\nsha256=abc123\nbytes=51000"
	store := &fakeToolResultOffloadStore{content: original}
	messages := []convcontext.Message{{
		Role:       convcontext.RoleTool,
		ToolCallID: "call",
		Content:    `{"schema_version":2,"status":"succeeded","content":` + strconv.Quote(marker) + `,"truncated":true}`,
	}}
	runner := &Runner{threadID: "thread", config: AgentConfig{ToolResultOffloadStore: store}}
	if err := runner.hydrateOffloadedToolResults(stdcontext.Background(), messages); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(messages[0].Content), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["content"] != original || payload["truncated"] != false {
		t.Fatalf("canonical outcome helper did not hydrate: %+v", payload)
	}
}
