package agentcore

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/subagentruntime"
	"awaken/src/tool"
	"awaken/src/tool/profile"
)

func TestForkAssignmentBoundarySurvivesProtocolPreparation(t *testing.T) {
	for _, protocol := range []llm.Protocol{llm.ProtocolOpenAIChat, llm.ProtocolOpenAIResponses, llm.ProtocolAnthropicMessages} {
		t.Run(string(protocol), func(t *testing.T) {
			config := AgentConfig{ModelConfig: ModelConfig{Model: "fixture-model", Protocol: string(protocol)}}
			config.SubagentOrigin = &profile.SubagentOrigin{AgentID: "child", ParentAgentID: "parent"}
			role, _ := subagentruntime.LookupRole(subagentruntime.AgentTypeGeneral)
			applyResolvedChildRole(&config, role)
			conversation := convcontext.NewConversation(nil)
			conversation.Context.SetMessages([]convcontext.Message{{Role: convcontext.RoleUser, OriginAgentID: "parent", Content: childAssignmentContent("Parent work: create three children; preserve PARENT-618.", "parent")}})
			conversation.UserInput(childAssignmentContent("Only return the known parent marker; do not use tools.", "child"))
			request := NewRequestBuilder(config).Build(conversation.Context, nil)
			prepared, err := llm.PrepareRequest(llm.Config{APIKey: "fixture", BaseURL: "https://example.invalid", Model: config.ModelConfig.Model, Protocol: protocol},
				llm.Request{Model: request.Model, SystemPrompt: request.SystemPrompt, Messages: request.Messages}, true)
			if err != nil {
				t.Fatal(err)
			}
			var body any
			if err := json.Unmarshal(prepared.Body, &body); err != nil {
				t.Fatal(err)
			}
			plain := fmt.Sprint(body)
			for _, required := range []string{"awaken_child_assignment_contract", "Parent work: create three children", "source_agent=\"parent\"", "<awaken_assignment agent_id=\"child\">\nOnly return the known parent marker; do not use tools.\n</awaken_assignment>"} {
				if !strings.Contains(plain, required) {
					t.Fatalf("prepared body lost the assignment boundary: %s", required)
				}
			}
			if strings.Contains(plain, "<awaken_assignment agent_id=\"parent\">") {
				t.Fatal("inherited assignment remained executable")
			}
			if strings.Contains(conversation.Context.GetMessages()[0].Content, "awaken_inherited_user_context") {
				t.Fatal("request projection mutated durable history")
			}
		})
	}
}

func TestForkCopiesNeutralClosedContextAndKeepsAncestorGrant(t *testing.T) {
	config := AgentConfig{ModelConfig: ModelConfig{Model: "parent", APIKey: "private-key"}, HistoryAncestors: []tool.HistoryRange{{AgentID: "ancestor", ThroughSeq: 7}}}
	conversation := convcontext.NewConversation(nil)
	conversation.UserInput("Do not overwrite other files")
	conversation.AssistantOutput("previous task done", nil)
	conversation.Context.AddAssistantMessageWithProviderState("pending", []convcontext.ToolCall{{ID: "open", Function: convcontext.FunctionCall{Name: "Read"}}}, &convcontext.ProviderOpaqueMessageState{Content: "opaque-parent-state"})
	runner := NewRunner(config, conversation, NewRequestBuilder(config))
	seed, err := runner.captureChildContext("full", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(seed.Messages) != 2 || len(seed.Ancestors) != 2 {
		t.Fatalf("invalid fork: %+v", seed)
	}
	encoded, err := json.Marshal(seed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "private-key") || strings.Contains(string(encoded), "opaque-parent-state") || strings.Contains(string(encoded), `"open"`) {
		t.Fatal("fork copied credentials or an in-flight invocation")
	}
	child := AgentConfig{}
	childConversation := convcontext.NewConversation(nil)
	if err := installChildContextSeed(encoded, &child, childConversation); err != nil {
		t.Fatal(err)
	}
	childConversation.UserInput("new child task")
	if len(conversation.Context.GetMessages()) != 3 {
		t.Fatal("child mutated parent history")
	}
	scope := tool.AgentHistoryScope{AgentID: "child", Ancestors: child.HistoryAncestors}
	if !scope.Allows("ancestor", 7) || scope.Allows("ancestor", 8) || scope.Allows("sibling", 1) {
		t.Fatal("fork widened historical access")
	}
	seed.Messages[0].Content = "tampered"
	corrupt, _ := json.Marshal(seed)
	if err := installChildContextSeed(corrupt, &child, childConversation); err == nil {
		t.Fatal("accepted corrupt seed")
	}
}

func TestFullForkAfterCompactionRetainsCheckpointWithoutOpaqueState(t *testing.T) {
	r := NewRunner(AgentConfig{}, convcontext.NewConversation(nil), NewRequestBuilder(AgentConfig{}))
	messages := []convcontext.Message{{Role: convcontext.RoleUser, Content: "preserve user constraints", CompactBoundaryID: "parent-compact"}}
	bundle, err := r.buildPostCompactContextBundleV2(CompactResult{Mode: CompactModeAuto, BoundaryID: "parent-compact", Messages: messages}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.config.RestoredContextBundleV2 = &bundle
	r.conversation.Context.SetMessages(messages)
	seed, err := r.captureChildContext("full", 0)
	if err != nil {
		t.Fatal(err)
	}
	if seed.Bundle == nil || seed.Bundle.TaskCheckpoint.Digest != bundle.TaskCheckpoint.Digest || seed.Bundle == &bundle {
		t.Fatal("checkpoint was lost or shared")
	}
	if seed.Bundle.ProviderOpaqueState.Value != nil || len(seed.Bundle.PendingSubagents) > 0 {
		t.Fatal("fork inherited executable state")
	}
	if err := bundle.Validate(); err != nil {
		t.Fatal("fork modified parent bundle", err)
	}
}

func TestRecentForkKeepsClosedTaskAndEarlierUserConstraints(t *testing.T) {
	conv := convcontext.NewConversation(nil)
	conv.UserInput("Always preserve file A")
	conv.AssistantOutput("old investigation", nil)
	conv.UserInput("recent task")
	conv.AssistantOutput("recent result", nil)
	conv.UserInput("unfinished parent task")
	r := NewRunner(AgentConfig{}, conv, NewRequestBuilder(AgentConfig{}))
	seed, err := r.captureChildContext("recent", 1)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(seed.Messages)
	for _, text := range []string{"Always preserve file A", "recent task", "recent result", "unfinished parent task"} {
		if !strings.Contains(string(encoded), text) {
			t.Fatalf("lost %s", text)
		}
	}
	if strings.Contains(string(encoded), "old investigation") {
		t.Fatal("recent fork selected unfinished or older trace")
	}
}
