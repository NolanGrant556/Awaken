package runtime

import (
	"testing"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
)

func TestProviderOpaqueStateRestoresLocallyAcrossToolTurnReplay(t *testing.T) {
	state := &convcontext.ProviderOpaqueMessageState{
		ProviderID: "kimi-coding-plan", ProfileID: "profile-1", CredentialRevision: 1,
		ModelID: "kimi-for-coding", ProtocolFamily: "openai_chat",
		EndpointContractID: "openai_chat_completions_v1", Content: "opaque-reasoning",
	}
	records := []agentcore.SemanticRecord{
		agentcore.NewSemanticRecord(agentcore.SemanticKindUserMessage, "thread-1", "run-1", "turn-1", "", "", map[string]any{"content": "use a tool"}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindProviderOpaqueState, "thread-1", "run-1", "turn-1", "", "", map[string]any{"message_id": "assistant-1", "state": state}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindToolCall, "thread-1", "run-1", "turn-1", "assistant-1", "", map[string]any{"tool_call_id": "call-1", "tool_name": "Read", "arguments": `{}`}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindToolResult, "thread-1", "run-1", "turn-1", "call-1", "", map[string]any{"tool_call_id": "call-1", "tool_name": "Read", "content": "ok"}),
	}
	messages := messagesFromSemanticRecords(records)
	if len(messages) != 3 || messages[1].Role != convcontext.RoleAssistant || len(messages[1].ToolCalls) != 1 {
		t.Fatalf("unexpected restored tool exchange: %+v", messages)
	}
	if messages[1].ProviderOpaqueState == nil || messages[1].ProviderOpaqueState.Content != state.Content {
		t.Fatalf("provider state was not restored: %+v", messages[1])
	}
}
