package agentcore

import (
	"encoding/json"
	"testing"

	convcontext "awaken/src/context"
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
		SystemPrompt:          "base instructions\n{{.RuntimeInfo}}",
		ModelConfig:           ModelConfig{Model: "test-model", ContextWindow: 128000},
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
	if snapshot.SchemaVersion != 4 || snapshot.EstimatorID != "awaken_leaf_v4" {
		t.Fatalf("expected v4 context estimator, got schema=%d estimator=%s", snapshot.SchemaVersion, snapshot.EstimatorID)
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
}
