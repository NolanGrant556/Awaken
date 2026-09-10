package agentcore

import (
	"context"
	"testing"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/tool"
)

func TestExplicitSameProfileKeepsComposerEffort(t *testing.T) {
	parent := AgentConfig{ModelConfig: ModelConfig{Model: "A", ProfileID: "a", ReasoningEffort: "max"}}
	parent.ResolveModelBinding = func(_ context.Context, id, effort string) (ModelConfig, error) {
		if effort == "" {
			effort = "high"
		}
		return ModelConfig{ProfileID: id, Model: id, ReasoningEffort: effort}, nil
	}
	for _, tc := range []struct{ id, effort, want string }{{"a", "", "max"}, {"b", "", "high"}, {"a", "high", "high"}} {
		binding, err := resolveChildModel(context.Background(), parent, tc.id, tc.effort, "general")
		if err != nil || binding.ReasoningEffort != tc.want {
			t.Fatalf("profile=%s effort=%s: %+v, %v", tc.id, tc.effort, binding, err)
		}
	}
}

func TestChildBindingInheritsEffectiveModelAndResolvesSummaryAgainstChild(t *testing.T) {
	parent := AgentConfig{ModelConfig: ModelConfig{Model: "A", ProfileID: "a", ReasoningEffort: "max", ContextWindow: 200000}}
	for _, explicitSummary := range []bool{false, true} {
		parent.SubagentModelConfig = ModelConfig{Model: "B", ProfileID: "b", ReasoningEffort: "high", ContextWindow: 100000, CapabilityModelID: "family-b"}
		if explicitSummary {
			parent.CompactModelConfig = ModelConfig{Model: "C", ProfileID: "c", ContextWindow: 400000}
		}
		child, err := CreateSubagent(parent, SpawnConfig{AgentID: "child", ParentAgentID: "root"})
		if err != nil {
			t.Fatal(err)
		}
		req := NewRequestBuilder(child).Build(convcontext.NewConversation(nil).Context, nil)
		want := "B"
		if explicitSummary {
			want = "C"
		}
		if child.ModelConfig.Model != "B" || req.EnvelopeDebug.CompactModelConfig.Model != want || req.EnvelopeDebug.ExecutionModelConfig.ContextWindow != 100000 {
			t.Fatalf("execution/summary binding was mixed: child=%s summary=%s", child.ModelConfig.Model, req.EnvelopeDebug.CompactModelConfig.Model)
		}
		projection := contextProjectionRequest(*req.EnvelopeDebug, req.ContextWindowState)
		if projection.CapabilityModelID != "family-b" {
			t.Fatalf("projection used summary identity: %+v", projection)
		}
		_, limit := compactTailExceedsBudget(CompactRequest{Envelope: *req.EnvelopeDebug, TargetTokens: 50000}, []convcontext.Message{{Role: convcontext.RoleAssistant, Content: "tail"}})
		if limit != 10000 {
			t.Fatalf("tail target=%d, want B's 10%% input capacity", limit)
		}
	}
	parent.SubagentModelConfig, parent.CompactModelConfig = ModelConfig{}, ModelConfig{}
	child, err := CreateSubagent(parent, SpawnConfig{AgentID: "inherited", ParentAgentID: "root"})
	if err != nil || child.ModelConfig.ReasoningEffort != "max" {
		t.Fatalf("effective effort lost: %v", err)
	}
}

func TestSubagentPreparedEffortAndUsageAgreeAcrossProtocols(t *testing.T) {
	for _, protocol := range []llm.Protocol{llm.ProtocolOpenAIChat, llm.ProtocolOpenAIResponses, llm.ProtocolAnthropicMessages} {
		t.Run(string(protocol), func(t *testing.T) {
			cfg := llm.Config{ProviderID: "deepseek", ProfileID: "child", BaseURL: "http://127.0.0.1", AuthScheme: "none", Protocol: protocol, Model: "deepseek-v4-pro", ReasoningEffort: "max"}
			prepared, err := llm.PrepareRequest(cfg, llm.Request{UserPrompt: "test", Purpose: llm.RequestPurposeSubagent, ReasoningPolicy: "max", MaxTokens: 4096}, true)
			if err != nil {
				t.Fatal(err)
			}
			usage := modelUsageRecord("request", cfg.Model, string(protocol), cfg, llm.RequestPurposeSubagent, prepared, time.Now(), nil, nil, nil)
			if usage.ReasoningPolicy != "max" || prepared.ReasoningPolicy != "max" {
				t.Fatalf("usage=%s prepared=%s", usage.ReasoningPolicy, prepared.ReasoningPolicy)
			}
		})
	}
}

func TestChildToolExposureDoesNotMutateParent(t *testing.T) {
	hidden := make([]string, 1, 16)
	hidden[0] = "parent_hidden"
	parent := AgentConfig{ToolExposure: tool.ToolExposurePolicy{ExtraHidden: hidden}}
	child, err := CreateSubagent(parent, SpawnConfig{AgentID: "child", ParentAgentID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	child.ToolExposure.ExtraHidden[0] = "changed"
	if parent.ToolExposure.ExtraHidden[0] != "parent_hidden" {
		t.Fatal("child changed parent tool policy")
	}
}
