package agentcore

import (
	"encoding/json"
	"testing"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/tool"
)

func TestContextUsageSnapshotClassifiesTypedSourcesWithoutDebugDuplication(t *testing.T) {
	memory := "<awaken_memory_context>remember this</awaken_memory_context>"
	skillBody := "full activated skill body"
	ctx := convcontext.New()
	ctx.AddUserMessage("older question")
	ctx.AddAssistantMessage("older answer", nil)
	ctx.AddUserMessage("current question")
	tools := []tool.ToolSchema{{
		Type:     "function",
		Function: tool.ToolFunctionSchema{Name: "builtin_test", Description: "builtin", Parameters: tool.ToolParametersSchema{Type: "object"}},
	}}
	builder := NewRequestBuilder(AgentConfig{
		SystemPrompt: "base instructions\n{{.RuntimeInfo}}",
		ModelConfig: ModelConfig{
			Model: "ep-test", CapabilityModelID: "doubao-seed-2.1-pro",
			EndpointSelector: "ep-test", ModelSelectorKind: "endpoint", ContextWindow: 128000,
		},
		MemoryContextProvider: func() (string, error) { return memory, nil },
		SkillContextBlocks:    []ContextBlock{{ID: "skill", Content: skillBody}},
	})
	envelope := builder.BuildContextEnvelope(ctx, tools)
	req := builder.BuildLLMRequestFromEnvelope(envelope)
	snapshot := req.ContextUsage

	sum := 0
	byKind := map[ContextContributionKind]ContextUsageContribution{}
	for _, contribution := range snapshot.Contributions {
		sum += contribution.EstimatedTokens
		byKind[contribution.Kind] = contribution
	}
	if sum != snapshot.EstimatedInputTokens {
		t.Fatalf("contributions must equal total: sum=%d total=%d", sum, snapshot.EstimatedInputTokens)
	}
	if _, exists := byKind[ContextContributionSkills]; exists {
		t.Fatalf("activated Skill body must belong to conversation, not Skill catalog: %+v", byKind)
	}
	if got := byKind[ContextContributionMemory].EstimatedTokens; got != convcontext.EstimateTokens(memory) {
		t.Fatalf("Memory must be counted independently once: got=%d; debug items=%+v", got, envelope.SystemContextItems)
	}
	if byKind[ContextContributionMemory].SourceCount != 1 {
		t.Fatalf("expected one typed Memory source: %+v", byKind)
	}
	if byKind[ContextContributionConversation].EstimatedTokens <= convcontext.EstimateTokens("current question")+convcontext.EstimateTokens(skillBody) {
		t.Fatalf("runtime, activated Skill body, history and current input were not combined into conversation: %+v", byKind)
	}
	if _, exists := byKind[ContextContributionSkillsMemory]; exists {
		t.Fatalf("v2 snapshot emitted legacy combined Skill/Memory contribution: %+v", byKind)
	}
	if snapshot.SchemaVersion != 4 || snapshot.EstimatorID != "awaken_leaf_v5" {
		t.Fatalf("expected v4 context estimator, got schema=%d estimator=%s", snapshot.SchemaVersion, snapshot.EstimatorID)
	}
	if snapshot.ModelID != "ep-test" || snapshot.CapabilityModelID != "doubao-seed-2.1-pro" || snapshot.EndpointSelector != "ep-test" || snapshot.ModelSelectorKind != "endpoint" {
		t.Fatalf("context snapshot lost capability/endpoint identity: %+v", snapshot)
	}
}

func TestContextUsageSnapshotUsesToolProvenanceAndBalancesPreparedOverhead(t *testing.T) {
	state := ContextWindowState{ModelWindow: 128000, OutputReserve: 16000, EffectiveWindow: 112000}
	req := LLMRequest{
		SystemPrompt:   "system",
		PromptSections: []PromptSection{{ID: "system", Kind: ContextContributionSystemPrompt, Content: "system"}},
		Messages: []convcontext.Message{
			{Role: convcontext.RoleUser, Content: "hello", Origin: convcontext.MessageOriginCurrentInput},
			{Role: convcontext.RoleTool, Content: "MCP returned content", ToolCallID: "call_1", Origin: convcontext.MessageOriginToolResult},
		},
		AttributedTools: []AttributedToolSchema{
			{Schema: tool.ToolSchema{Type: "function", Function: tool.ToolFunctionSchema{Name: "builtin", Parameters: tool.ToolParametersSchema{Type: "object"}}}, Provenance: ToolSchemaProvenance{Origin: tool.SourceBuiltin}},
			{Schema: tool.ToolSchema{Type: "function", Function: tool.ToolFunctionSchema{Name: "skill_tool", Parameters: tool.ToolParametersSchema{Type: "object"}}}, Provenance: ToolSchemaProvenance{Origin: tool.SourceSkill, SkillCatalog: "- documents: create documents"}},
			{Schema: tool.ToolSchema{Type: "function", Function: tool.ToolFunctionSchema{Name: "mcp", Parameters: tool.ToolParametersSchema{Type: "object"}}}, Provenance: ToolSchemaProvenance{Origin: tool.SourceMCP}},
		},
		ContextWindowState: state,
		Model:              "test-model",
		ContextEpoch:       &ContextEpochReceipt{EpochID: "epoch-1", CreationBoundaryReason: "tool_schema_changed"},
	}
	for _, attributed := range req.AttributedTools {
		req.Tools = append(req.Tools, attributed.Schema)
	}
	body := []byte(`{"model":"test-model","messages":[{"role":"system","content":"system"},{"role":"user","content":"hello"},{"role":"tool","content":"MCP returned content","tool_call_id":"call_1"}],"tools":[{"name":"builtin","description":"builtin wire definition","input_schema":{"type":"object"}},{"name":"skill_tool","description":"- documents: create documents","input_schema":{"type":"object"}},{"name":"mcp","description":"MCP server tool","input_schema":{"type":"object"}}]}`)
	snapshot := buildContextUsageSnapshot(req, "sent_request", "req_1", "openai_chat", "digest", body)
	byKind := map[ContextContributionKind]int{}
	sourceCounts := map[ContextContributionKind]int{}
	sum := 0
	for _, contribution := range snapshot.Contributions {
		byKind[contribution.Kind] = contribution.EstimatedTokens
		sourceCounts[contribution.Kind] = contribution.SourceCount
		sum += contribution.EstimatedTokens
	}
	if byKind[ContextContributionBuiltinTools] == 0 || byKind[ContextContributionSkills] == 0 || byKind[ContextContributionMCP] == 0 {
		t.Fatalf("tool provenance was lost: %+v", byKind)
	}
	if sourceCounts[ContextContributionBuiltinTools] != 2 || sourceCounts[ContextContributionSkills] != 1 || sourceCounts[ContextContributionMCP] != 1 {
		t.Fatalf("builtin tools, Skill wrapper/catalog and MCP definitions must remain independently attributed: counts=%+v values=%+v", sourceCounts, byKind)
	}
	if byKind[ContextContributionConversation] <= convcontext.EstimateTokens("MCP returned content") {
		t.Fatalf("MCP results, messages and prepared JSON overhead must be included in conversation: %+v", byKind)
	}
	var prepared struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(body, &prepared); err != nil {
		t.Fatal(err)
	}
	if got, want := byKind[ContextContributionMCP], convcontext.EstimateTokens(string(prepared.Tools[2])); got != want {
		t.Fatalf("MCP must use exact provider-lowered tool JSON: got=%d want=%d", got, want)
	}
	if got, want := byKind[ContextContributionSkills], estimateSkillCatalogTokens(prepared.Tools, true, 1, req.AttributedTools[1], convcontext.EstimateTokens(string(prepared.Tools[1]))); got != want || got == 0 {
		t.Fatalf("Skill must include only the catalog listing delta: got=%d want=%d", got, want)
	}
	if byKind[ContextContributionBuiltinTools] <= convcontext.EstimateTokens(string(prepared.Tools[0])) {
		t.Fatalf("the unified Skill tool wrapper must remain in builtin tools: %+v", byKind)
	}
	if _, exists := byKind[ContextContributionMCPDynamicTools]; exists {
		t.Fatalf("v2 snapshot emitted legacy MCP/dynamic bucket: %+v", byKind)
	}
	if sum != snapshot.EstimatedInputTokens {
		t.Fatalf("prepared composition is unbalanced: sum=%d total=%d", sum, snapshot.EstimatedInputTokens)
	}
	if snapshot.ContextEpochID != "epoch-1" {
		t.Fatalf("context snapshot lost epoch identity: %+v", snapshot)
	}
	if snapshot.ContextEpochBoundaryReason != "tool_schema_changed" {
		t.Fatalf("context snapshot lost epoch boundary reason: %+v", snapshot)
	}
}

func TestReasoningPolicyEntersEpochAndModelUsageIdentity(t *testing.T) {
	candidate := contextEpochCandidate("thread", ModelConfig{
		ProviderID: "doubao", ProfileID: "profile", ReasoningEffort: " MAX ",
		CapabilityModelID: "doubao-seed-2.1-pro", EndpointSelector: "ep-test", ModelSelectorKind: "endpoint",
	}, LLMRequest{Model: "ep-test"}, "openai_chat_completions_v1", "https://ark.cn-beijing.volces.com/api/v3")
	if candidate.ReasoningPolicy != "max" {
		t.Fatalf("context epoch reasoning policy = %q", candidate.ReasoningPolicy)
	}
	if candidate.ModelID != "ep-test" || candidate.CapabilityModelID != "doubao-seed-2.1-pro" || candidate.EndpointSelector != "ep-test" || candidate.ModelSelectorKind != "endpoint" {
		t.Fatalf("context epoch lost capability/endpoint identity: %+v", candidate)
	}

	usage := modelUsageRecord(
		"request", "ep-test", "openai_chat",
		llm.Config{ProviderID: "doubao", ProfileID: "profile", CapabilityModelID: "doubao-seed-2.1-pro", EndpointSelector: "ep-test", ModelSelectorKind: "endpoint", ReasoningEffort: " MAX "},
		llm.RequestPurposeAgentTurn, llm.PreparedRequest{}, time.Now(), nil, nil, nil,
	)
	if usage.ReasoningPolicy != "max" {
		t.Fatalf("agent usage reasoning policy = %q", usage.ReasoningPolicy)
	}
	if usage.ModelID != "ep-test" || usage.CapabilityModelID != "doubao-seed-2.1-pro" || usage.EndpointSelector != "ep-test" || usage.ModelSelectorKind != "endpoint" {
		t.Fatalf("model usage lost capability/endpoint identity: %+v", usage)
	}
	auxiliary := modelUsageRecord(
		"title", "kimi-k2.6", "openai_chat",
		llm.Config{ProviderID: "kimi-cn", ProfileID: "profile", ReasoningEffort: "max"},
		llm.RequestPurposeThreadTitle, llm.PreparedRequest{}, time.Now(), nil, nil, nil,
	)
	if auxiliary.ReasoningPolicy != "" {
		t.Fatalf("auxiliary usage inherited agent reasoning policy: %+v", auxiliary)
	}
}

func TestContextWindowUsageAnchorFromModelUsagePrefersTotalThenInput(t *testing.T) {
	sentEstimate := 73_000
	inputTokens := 91_000
	totalTokens := 97_000
	usage := ModelUsageRecord{
		RequestID:                "req-1",
		Purpose:                  "agent_turn",
		Status:                   ModelUsageSucceeded,
		UsageComplete:            true,
		ProviderID:               "minimax",
		ProfileID:                "profile-a",
		CredentialRevision:       3,
		ModelID:                  "ep-1",
		CapabilityModelID:        "MiniMax-M1",
		EndpointSelector:         "ep-1",
		ModelSelectorKind:        "endpoint",
		ProtocolFamily:           "openai_chat",
		ReasoningPolicy:          " HIGH ",
		SentEstimatedInputTokens: &sentEstimate,
		InputTokens:              &inputTokens,
		TotalTokens:              &totalTokens,
	}

	anchor := ContextWindowUsageAnchorFromModelUsage(usage)
	if anchor == nil || anchor.UsageTokens != totalTokens || anchor.SentEstimatedInputTokens != sentEstimate {
		t.Fatalf("expected total-tokens anchor, got %+v", anchor)
	}

	usage.TotalTokens = nil
	anchor = ContextWindowUsageAnchorFromModelUsage(usage)
	if anchor == nil || anchor.UsageTokens != inputTokens {
		t.Fatalf("expected input-tokens fallback anchor, got %+v", anchor)
	}
}

func TestContextWindowUsageAnchorCompatibilityUsesExactIdentity(t *testing.T) {
	anchor := &ContextWindowUsageAnchor{
		ProviderID: "doubao", ProfileID: "profile-a", CredentialRevision: 7,
		ModelID: "ep-1", CapabilityModelID: "doubao-seed-2.1-pro", EndpointSelector: "ep-1",
		ModelSelectorKind: "endpoint", ProtocolFamily: "openai_chat", ReasoningPolicy: "max",
		UsageTokens: 88_000, SentEstimatedInputTokens: 76_000,
	}
	matching := ModelConfig{
		ProviderID: "doubao", ProfileID: "profile-a", CredentialRevision: 7,
		Model: "ep-1", CapabilityModelID: "doubao-seed-2.1-pro", EndpointSelector: "ep-1",
		ModelSelectorKind: "endpoint", Protocol: "openai_chat", ReasoningEffort: " MAX ",
	}
	if !ContextWindowUsageAnchorCompatibleWithModelConfig(anchor, matching) {
		t.Fatalf("expected exact-compatible anchor to match: %+v", anchor)
	}

	mismatchedReasoning := matching
	mismatchedReasoning.ReasoningEffort = "high"
	if ContextWindowUsageAnchorCompatibleWithModelConfig(anchor, mismatchedReasoning) {
		t.Fatalf("reasoning mismatch must invalidate usage anchor")
	}
}
