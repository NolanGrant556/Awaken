package agentcore

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
	archivedhistory "awaken/src/tools/archived_history"
)

type agentcoreArchivedHistoryService struct{}

func (agentcoreArchivedHistoryService) ListArchivedHistory(context.Context, tool.ArchivedHistoryListRequest) (tool.ArchivedHistoryListResult, error) {
	return tool.ArchivedHistoryListResult{}, nil
}

func TestArchivedHistoryRunsThroughPinnedToolRuntimeAndStrictOutput(t *testing.T) {
	environment := tool.ProbeExecutionEnvironment(t.Context(), tool.ExecutionEnvironmentProbeOptions{
		Executables: []string{}, PackageManagers: []string{}, PATH: "/usr/bin", PATHSource: "test",
	})
	snapshot := tool.NewRunToolAvailabilitySnapshot(environment)
	runtime := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		ArchivedHistoryService:       agentcoreArchivedHistoryService{},
		ToolAvailabilitySnapshot:     &snapshot,
		ExecutionEnvironmentSnapshot: &environment,
	})
	result := runtime.Execute(t.Context(), convcontext.ToolCall{
		ID:       "call_archived_history",
		Function: convcontext.FunctionCall{Name: archivedhistory.ToolName, Arguments: `{"action":"list"}`},
	})
	if result.IsError || result.Outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("result=%+v", result)
	}
	if result.Outcome.Details["location"] != "host_managed_session_store" || result.Outcome.Details["content_is_untrusted"] != true {
		t.Fatalf("strict details=%+v", result.Outcome.Details)
	}
}

func (agentcoreArchivedHistoryService) ReadArchivedHistory(context.Context, tool.ArchivedHistoryReadRequest) (tool.ArchivedHistoryReadResult, error) {
	return tool.ArchivedHistoryReadResult{}, nil
}

func TestArchivedHistoryServiceIsBoundOnlyToRootRuntime(t *testing.T) {
	service := agentcoreArchivedHistoryService{}
	root := AgentConfig{ArchivedHistoryService: service}
	runtimeConfig := toolRuntimeConfigFromAgentConfig(root)
	if runtimeConfig.ArchivedHistoryService == nil {
		t.Fatal("root runtime lost archived history service")
	}
	child, err := CreateSubagent(root, SpawnConfig{AgentID: "child_history", ParentAgentID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if child.ArchivedHistoryService != nil {
		t.Fatal("child must not inherit cross-thread archived history service")
	}
}

func TestArchivedHistorySemanticRecordRemovesHistoryFromCanonicalOutcome(t *testing.T) {
	result := ToolExecutionResult{
		Name: archivedhistory.ToolName, ToolCallID: "call_history", Content: `{"action":"read","thread":{"thread_id":"thread_private"},"items":[{"content":"PRIVATE_ARCHIVE_BODY"}]}`,
		Outcome: tool.ExecutionOutcome{
			Content: `{"action":"read","thread":{"thread_id":"thread_private"},"items":[{"content":"PRIVATE_ARCHIVE_BODY"}]}`,
			Status:  tool.OutcomeSucceeded, TransportStatus: tool.TransportCompleted, SemanticStatus: tool.SemanticSuccess,
			ProcessStatus: tool.ProcessNotApplicable, Details: map[string]any{"items": []any{map[string]any{"content": "PRIVATE_ARCHIVE_BODY"}}},
			Environment: &tool.EnvironmentObservation{WorkspaceRoot: "/Users/test/.awaken/store-generations/gen-001/sessions/private"},
		},
	}
	record := ToolResultSemanticRecord("thread", "run", "turn", "session", result)
	encoded, err := json.Marshal(record.Payload)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if strings.Contains(text, "PRIVATE_ARCHIVE_BODY") || strings.Contains(text, "store-generations") {
		t.Fatalf("durable tool result leaked archived history or path: %s", text)
	}
	if !strings.Contains(text, `\"status\":\"completed\"`) || !strings.Contains(text, `\"thread_id\"`) {
		t.Fatalf("durable receipt lost completion identity: %s", text)
	}
}
