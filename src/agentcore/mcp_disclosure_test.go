package agentcore

import (
	stdcontext "context"
	"encoding/json"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/tool"

	_ "awaken/src/tools/mcp_find"
)

type mcpDisclosureAgentFakeTool struct {
	name        string
	server      string
	rawName     string
	description string
}

func (t *mcpDisclosureAgentFakeTool) Name() string                     { return t.name }
func (t *mcpDisclosureAgentFakeTool) Description() string              { return t.description }
func (t *mcpDisclosureAgentFakeTool) Parameters() []tool.ToolParameter { return nil }
func (t *mcpDisclosureAgentFakeTool) Execute(stdcontext.Context, string) (string, error) {
	return "ok", nil
}
func (t *mcpDisclosureAgentFakeTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name: t.name, DisplayName: t.name, Source: tool.SourceMCP, Description: t.description,
		ProviderID: tool.ProviderID("mcp:" + t.server),
		InputSchema: tool.ToolParametersSchema{
			Type:       "object",
			Properties: map[string]tool.ToolParam{"value": {Type: "string"}},
		},
		Annotations: tool.ToolAnnotations{ReadOnly: true, ServerName: t.server, ToolName: t.rawName},
	}
}

func registerAgentMCPDisclosureFixture(t *testing.T) *tool.RunToolAvailabilitySnapshot {
	t.Helper()
	fakes := []*mcpDisclosureAgentFakeTool{
		{name: "mcp__ghfix__create_issue", server: "ghfix", rawName: "create_issue", description: "Create a GitHub issue with title and body fields for the selected repository."},
		{name: "mcp__ghfix__list_issues", server: "ghfix", rawName: "list_issues", description: "List repository issues filtered by state, label and assignee."},
		{name: "mcp__plugin__notepack__store__write_note", server: "plugin__notepack__store", rawName: "write_note", description: "Write one note document into the plugin-owned note store."},
	}
	for _, fake := range fakes {
		fake := fake
		if err := tool.RegisterToolForce(fake); err != nil {
			t.Fatalf("register %s: %v", fake.name, err)
		}
		t.Cleanup(func() { tool.UnregisterTool(fake.name) })
	}
	snapshot := tool.NewRunToolAvailabilitySnapshot(tool.ExecutionEnvironmentSnapshot{SnapshotID: "env_mcp_agent"})
	return &snapshot
}

func TestBuildMCPDisclosureProjectionModes(t *testing.T) {
	snapshot := registerAgentMCPDisclosureFixture(t)
	modelConfig := ModelConfig{ContextWindow: 128000}

	off := buildMCPDisclosureProjection(snapshot, modelConfig, nil, tool.MCPDisclosurePolicy{Mode: tool.MCPDisclosureOff})
	if off.Enabled {
		t.Fatalf("off policy must disable disclosure: %+v", off)
	}

	always := buildMCPDisclosureProjection(snapshot, modelConfig, []string{"mcp__ghfix__list_issues"}, tool.MCPDisclosurePolicy{Mode: tool.MCPDisclosureAlways})
	if !always.Enabled {
		t.Fatalf("always policy must enable disclosure: %+v", always)
	}
	if !always.Deferred["mcp__ghfix__create_issue"] || always.Deferred["mcp__ghfix__list_issues"] {
		t.Fatalf("activated tool must leave the deferred set: %+v", always.Deferred)
	}
	if !strings.Contains(always.Catalog, "ghfix") || !strings.Contains(always.Catalog, "plugin__notepack__store") {
		t.Fatalf("catalog must list plain and plugin servers: %q", always.Catalog)
	}
	if !strings.HasPrefix(always.Description, mcpFindGuidancePrefix) {
		t.Fatalf("description must lead with guidance: %q", always.Description)
	}
	// The catalog deliberately ignores the activated set: activations must be
	// append-only tool additions, not catalog rewrites.
	baseline := buildMCPDisclosureProjection(snapshot, modelConfig, nil, tool.MCPDisclosurePolicy{Mode: tool.MCPDisclosureAlways})
	if baseline.Catalog != always.Catalog {
		t.Fatalf("catalog must stay stable across activations")
	}

	// Auto with a huge threshold stays disabled but reports the measurement.
	below := buildMCPDisclosureProjection(snapshot, modelConfig, nil, tool.MCPDisclosurePolicy{Mode: tool.MCPDisclosureAuto, ThresholdPercent: 100})
	if below.Enabled || below.EstimatedMCPTokens <= 0 || below.ThresholdTokens <= 0 {
		t.Fatalf("auto below threshold must stay disabled with measurements: %+v", below)
	}

	// A tiny window forces the same catalog above the auto threshold.
	tiny := buildMCPDisclosureProjection(snapshot, ModelConfig{ContextWindow: 16500}, nil, tool.MCPDisclosurePolicy{Mode: tool.MCPDisclosureAuto, ThresholdPercent: 1})
	if !tiny.Enabled {
		t.Fatalf("auto above threshold must enable disclosure: %+v", tiny)
	}
}

func TestRequestBuilderGetToolsDefersMCPByDefaultUntilActivated(t *testing.T) {
	snapshot := registerAgentMCPDisclosureFixture(t)
	t.Setenv(tool.MCPDisclosureEnvVar, "")

	config := AgentConfig{
		ModelConfig:              ModelConfig{ContextWindow: 128000},
		ToolAvailabilitySnapshot: snapshot,
	}
	builder := NewRequestBuilder(config)
	names := map[string]string{}
	for _, schema := range builder.getTools() {
		names[schema.Function.Name] = schema.Function.Description
	}
	if _, visible := names["mcp__ghfix__create_issue"]; visible {
		t.Fatalf("deferred MCP tool must not be projected: %v", keysOf(names))
	}
	description, hasFind := names[tool.MCPFindToolName]
	if !hasFind {
		t.Fatalf("mcp_find must be projected while disclosure is active: %v", keysOf(names))
	}
	if !strings.Contains(description, "ghfix") || !strings.Contains(description, "create_issue") {
		t.Fatalf("mcp_find description must embed the catalog: %q", description)
	}

	config.MCPActivatedTools = []string{"mcp__ghfix__create_issue"}
	builder = NewRequestBuilder(config)
	names = map[string]string{}
	for _, schema := range builder.getTools() {
		names[schema.Function.Name] = schema.Function.Description
	}
	if _, visible := names["mcp__ghfix__create_issue"]; !visible {
		t.Fatalf("activated MCP tool must be projected by original name: %v", keysOf(names))
	}
	if _, visible := names["mcp__ghfix__list_issues"]; visible {
		t.Fatalf("non-activated MCP tool must stay deferred: %v", keysOf(names))
	}

	// Attribution must carry the embedded catalog for context accounting.
	attributed := builder.attributeToolSchemas(builder.getTools())
	foundCatalog := false
	for _, item := range attributed {
		if item.Schema.Function.Name == tool.MCPFindToolName && strings.TrimSpace(item.Provenance.MCPCatalog) != "" {
			foundCatalog = true
		}
	}
	if !foundCatalog {
		t.Fatalf("mcp_find provenance must carry the catalog")
	}

	t.Setenv(tool.MCPDisclosureEnvVar, "off")
	builder = NewRequestBuilder(config)
	names = map[string]string{}
	for _, schema := range builder.getTools() {
		names[schema.Function.Name] = schema.Function.Description
	}
	if _, visible := names[tool.MCPFindToolName]; visible {
		t.Fatalf("mcp_find must be hidden when disclosure is off: %v", keysOf(names))
	}
	if _, visible := names["mcp__ghfix__list_issues"]; !visible {
		t.Fatalf("off policy must project every MCP tool: %v", keysOf(names))
	}
}

func TestMCPDisclosurePreparedRequestMatrixBeforeAndAfterActivation(t *testing.T) {
	snapshot := registerAgentMCPDisclosureFixture(t)
	t.Setenv(tool.MCPDisclosureEnvVar, "")

	config := AgentConfig{
		ModelConfig:              ModelConfig{Model: "test-model", ContextWindow: 128000},
		ToolAvailabilitySnapshot: snapshot,
	}
	conversation := convcontext.New()
	conversation.AddUserMessage("hello")
	before := NewRequestBuilder(config).Build(conversation, nil)

	config.MCPActivatedTools = []string{"mcp__ghfix__create_issue"}
	after := NewRequestBuilder(config).Build(conversation, nil)

	for _, protocol := range []llm.Protocol{
		llm.ProtocolOpenAIChat,
		llm.ProtocolOpenAIResponses,
		llm.ProtocolAnthropicMessages,
	} {
		t.Run(string(protocol), func(t *testing.T) {
			beforeTools := preparedMCPDisclosureTools(t, protocol, before)
			if beforeTools[tool.MCPFindToolName] == nil {
				t.Fatalf("default disclosure request omitted mcp_find: %v", keysOfPreparedTools(beforeTools))
			}
			for _, deferred := range []string{"mcp__ghfix__create_issue", "mcp__ghfix__list_issues"} {
				if beforeTools[deferred] != nil {
					t.Fatalf("default disclosure leaked deferred tool %q: %v", deferred, keysOfPreparedTools(beforeTools))
				}
			}

			afterTools := preparedMCPDisclosureTools(t, protocol, after)
			activated := afterTools["mcp__ghfix__create_issue"]
			if activated == nil {
				t.Fatalf("activated MCP tool missing from request: %v", keysOfPreparedTools(afterTools))
			}
			if afterTools["mcp__ghfix__list_issues"] != nil {
				t.Fatalf("non-activated MCP tool leaked into request: %v", keysOfPreparedTools(afterTools))
			}
			if afterTools[tool.MCPFindToolName] == nil {
				t.Fatalf("mcp_find must remain available after one activation: %v", keysOfPreparedTools(afterTools))
			}
			if beforeTools[tool.MCPFindToolName]["description"] != afterTools[tool.MCPFindToolName]["description"] {
				t.Fatal("mcp_find catalog description changed after activation")
			}

			schemaKey := "parameters"
			if protocol == llm.ProtocolAnthropicMessages {
				schemaKey = "input_schema"
			}
			schema, ok := activated[schemaKey].(map[string]any)
			if !ok {
				t.Fatalf("activated tool missing %s schema: %+v", schemaKey, activated)
			}
			properties, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("activated tool properties missing: %+v", schema)
			}
			value, ok := properties["value"].(map[string]any)
			if !ok || value["type"] != "string" {
				t.Fatalf("activated tool schema was not preserved: %+v", properties)
			}
		})
	}
}

func preparedMCPDisclosureTools(t *testing.T, protocol llm.Protocol, request LLMRequest) map[string]map[string]any {
	t.Helper()
	prepared, err := llm.PrepareRequest(llm.Config{
		APIKey: "test", BaseURL: "https://example.test/v1", Model: "test-model", Protocol: protocol,
	}, llm.Request{
		SystemPrompt: request.SystemPrompt,
		Messages:     request.Messages,
		Tools:        request.Tools,
		Model:        request.Model,
	}, false)
	if err != nil {
		t.Fatalf("prepare %s request: %v", protocol, err)
	}
	var body map[string]any
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatalf("decode %s request: %v", protocol, err)
	}
	rawTools, ok := body["tools"].([]any)
	if !ok {
		t.Fatalf("%s request omitted tools array: %s", protocol, prepared.Body)
	}
	result := make(map[string]map[string]any, len(rawTools))
	for _, raw := range rawTools {
		wireTool, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("%s request contains malformed tool: %+v", protocol, raw)
		}
		if protocol == llm.ProtocolOpenAIChat {
			function, ok := wireTool["function"].(map[string]any)
			if !ok {
				t.Fatalf("chat request contains malformed function tool: %+v", wireTool)
			}
			wireTool = function
		}
		name, _ := wireTool["name"].(string)
		if name == "" {
			t.Fatalf("%s request contains unnamed tool: %+v", protocol, wireTool)
		}
		result[name] = wireTool
	}
	return result
}

func keysOfPreparedTools(values map[string]map[string]any) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	return out
}

func keysOf(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	return out
}

func TestContextUsageSplitsMCPFindCatalogIntoMCPCategory(t *testing.T) {
	catalog := "- ghfix (2 tools): create_issue, list_issues"
	req := LLMRequest{
		PromptSections: []PromptSection{{ID: "system", Kind: ContextContributionSystemPrompt, Content: "system"}},
		Messages: []convcontext.Message{
			{Role: convcontext.RoleUser, Content: "hello", Origin: convcontext.MessageOriginCurrentInput},
		},
		AttributedTools: []AttributedToolSchema{
			{
				Schema: tool.ToolSchema{Type: "function", Function: tool.ToolFunctionSchema{
					Name:        tool.MCPFindToolName,
					Description: "Search deferred MCP tools.\n\n" + catalog,
					Parameters:  tool.ToolParametersSchema{Type: "object"},
				}},
				Provenance: ToolSchemaProvenance{Origin: tool.SourceBuiltin, MCPCatalog: catalog},
			},
		},
		Model: "test-model",
	}
	for _, attributed := range req.AttributedTools {
		req.Tools = append(req.Tools, attributed.Schema)
	}
	snapshot := buildContextUsageSnapshot(req, "current_projection", "", "openai_chat", "", nil)
	byKind := map[ContextContributionKind]int{}
	for _, contribution := range snapshot.Contributions {
		byKind[contribution.Kind] = contribution.EstimatedTokens
	}
	if byKind[ContextContributionMCP] <= 0 {
		t.Fatalf("embedded catalog must count toward the MCP category: %+v", snapshot.Contributions)
	}
	if byKind[ContextContributionBuiltinTools] <= 0 {
		t.Fatalf("the mcp_find wrapper must stay builtin: %+v", snapshot.Contributions)
	}
	total := 0
	for _, contribution := range snapshot.Contributions {
		total += contribution.EstimatedTokens
	}
	if total != snapshot.EstimatedInputTokens {
		t.Fatalf("contribution sum invariant broken: %d != %d", total, snapshot.EstimatedInputTokens)
	}
}

// TestMCPFindThroughToolRuntimeKeepsStrictOutcome runs mcp_find through the
// full ToolRuntime pipeline (permission, input schema validation, execution,
// strict v2 outcome finalization). A details/output-schema mismatch would
// rewrite the success into tool_output_schema_mismatch, which unit tests that
// call Execute directly can never catch.
func TestMCPFindThroughToolRuntimeKeepsStrictOutcome(t *testing.T) {
	snapshot := registerAgentMCPDisclosureFixture(t)
	environment := tool.ExecutionEnvironmentSnapshot{SnapshotID: "env_mcp_agent"}
	runtime := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		ToolAvailabilitySnapshot:     snapshot,
		ExecutionEnvironmentSnapshot: &environment,
	})

	call := convcontext.ToolCall{ID: "call_rt"}
	call.Function.Name = tool.MCPFindToolName
	call.Function.Arguments = `{"query":"ghfix create issue","limit":2}`
	result := runtime.Execute(stdcontext.Background(), call)
	if result.IsError || result.Outcome.IsError() {
		t.Fatalf("strict runtime execution downgraded: status=%s code=%s message=%s",
			result.Outcome.Status, result.Outcome.ErrorCode, result.Outcome.ErrorMessage)
	}
	if !strings.Contains(result.Content, "mcp__ghfix__create_issue") {
		t.Fatalf("result content missing match: %s", result.Content)
	}
	for _, warning := range result.Outcome.Warnings {
		if warning.Code == "tool_output_schema_mismatch" {
			t.Fatalf("output schema mismatch warning: %+v", result.Outcome.Warnings)
		}
	}

	// The structured failure path must also survive strict finalization as a
	// semantic failure, not a validation crash.
	invalid := convcontext.ToolCall{ID: "call_rt_bad"}
	invalid.Function.Name = tool.MCPFindToolName
	invalid.Function.Arguments = `{"query":"   "}`
	failed := runtime.Execute(stdcontext.Background(), invalid)
	if !failed.Outcome.IsError() || failed.Outcome.ErrorCode != "mcp_find_query_required" {
		t.Fatalf("empty query must fail with typed code: %+v", failed.Outcome)
	}
}

// TestCreateSubagentInheritsMCPActivatedTools proves children sharing the
// parent's pinned snapshot also inherit the parent's on-demand loaded MCP set
// (an independent copy, so child-local growth never mutates the parent).
func TestCreateSubagentInheritsMCPActivatedTools(t *testing.T) {
	parent := AgentConfig{MCPActivatedTools: []string{"mcp__ghfix__create_issue"}}
	child, err := CreateSubagent(parent, SpawnConfig{AgentID: "child-mcp", ParentAgentID: "root"})
	if err != nil {
		t.Fatalf("create subagent: %v", err)
	}
	if len(child.MCPActivatedTools) != 1 || child.MCPActivatedTools[0] != "mcp__ghfix__create_issue" {
		t.Fatalf("child must inherit parent activations: %+v", child.MCPActivatedTools)
	}
	child.MCPActivatedTools = append(child.MCPActivatedTools, "mcp__ghfix__list_issues")
	if len(parent.MCPActivatedTools) != 1 {
		t.Fatalf("child growth must not mutate parent: %+v", parent.MCPActivatedTools)
	}
}

type mcpDisclosureFakeRecorder struct {
	records []SemanticRecord
}

func (r *mcpDisclosureFakeRecorder) Append(_ stdcontext.Context, record SemanticRecord) error {
	r.records = append(r.records, record)
	return nil
}
func (r *mcpDisclosureFakeRecorder) AppendBatch(_ stdcontext.Context, records []SemanticRecord) error {
	r.records = append(r.records, records...)
	return nil
}
func (r *mcpDisclosureFakeRecorder) Flush(stdcontext.Context) error { return nil }

func TestRunnerAppliesAndPersistsMCPToolActivation(t *testing.T) {
	snapshot := registerAgentMCPDisclosureFixture(t)
	t.Setenv(tool.MCPDisclosureEnvVar, "always")

	config := AgentConfig{
		ModelConfig:              ModelConfig{ContextWindow: 128000},
		ToolAvailabilitySnapshot: snapshot,
	}
	recorder := &mcpDisclosureFakeRecorder{}
	runner := NewRunner(config, convcontext.NewConversation(nil), NewRequestBuilder(config))
	runner.recorder = recorder
	runner.threadID = "thread_mcp"
	runner.runID = "run_mcp"

	// Filtering: unknown names, builtins and duplicates never activate.
	newly := runner.newlyActivatedMCPToolNames([]string{
		"mcp__ghfix__create_issue", "mcp__ghfix__create_issue", "not_a_tool", tool.MCPFindToolName, " ",
	})
	if len(newly) != 1 || newly[0] != "mcp__ghfix__create_issue" {
		t.Fatalf("activation filter wrong: %+v", newly)
	}

	// Direct call of a deferred MCP tool auto-loads it for later turns.
	call := convcontext.ToolCall{ID: "call_1"}
	call.Function.Name = "mcp__ghfix__create_issue"
	runner.applyMCPToolActivation(stdcontext.Background(), nil, call, nil)
	if len(runner.config.MCPActivatedTools) != 1 || runner.config.MCPActivatedTools[0] != "mcp__ghfix__create_issue" {
		t.Fatalf("direct call must auto-activate: %+v", runner.config.MCPActivatedTools)
	}
	if len(recorder.records) != 1 || recorder.records[0].Kind != SemanticKindMCPToolsActivated {
		t.Fatalf("activation must persist a durable record: %+v", recorder.records)
	}
	payloadTools, _ := recorder.records[0].Payload["tools"].([]string)
	if len(payloadTools) != 1 || payloadTools[0] != "mcp__ghfix__create_issue" {
		t.Fatalf("record payload wrong: %+v", recorder.records[0].Payload)
	}
	visible := map[string]bool{}
	for _, schema := range runner.reqBuilder.getTools() {
		visible[schema.Function.Name] = true
	}
	if !visible["mcp__ghfix__create_issue"] {
		t.Fatalf("rebuilt request builder must project the activated tool")
	}
	if visible["mcp__ghfix__list_issues"] {
		t.Fatalf("other MCP tools must stay deferred")
	}

	// mcp_find loads: observer-provided names apply without a second record.
	findCall := convcontext.ToolCall{ID: "call_2"}
	findCall.Function.Name = tool.MCPFindToolName
	runner.applyMCPToolActivation(stdcontext.Background(), nil, findCall, []string{"mcp__ghfix__list_issues"})
	if len(runner.config.MCPActivatedTools) != 2 {
		t.Fatalf("find-loaded tool must apply: %+v", runner.config.MCPActivatedTools)
	}

	// Re-activation is idempotent: no new record, no rebuild loop.
	recordsBefore := len(recorder.records)
	runner.applyMCPToolActivation(stdcontext.Background(), nil, call, nil)
	if len(recorder.records) != recordsBefore || len(runner.config.MCPActivatedTools) != 2 {
		t.Fatalf("re-activation must be a no-op: records=%d activated=%+v", len(recorder.records), runner.config.MCPActivatedTools)
	}
}
