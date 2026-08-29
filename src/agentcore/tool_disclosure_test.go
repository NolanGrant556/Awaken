package agentcore

import (
	stdcontext "context"
	"encoding/json"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/tool"

	_ "awaken/src/tools/tool_find"
)

type disclosureAgentFakeTool struct {
	name        string
	source      tool.ToolSource
	server      string
	rawName     string
	description string
}

func (t *disclosureAgentFakeTool) Name() string                     { return t.name }
func (t *disclosureAgentFakeTool) Description() string              { return t.description }
func (t *disclosureAgentFakeTool) Parameters() []tool.ToolParameter { return nil }
func (t *disclosureAgentFakeTool) Execute(stdcontext.Context, string) (string, error) {
	return "ok", nil
}
func (t *disclosureAgentFakeTool) Definition() tool.ToolDefinition {
	source := t.source
	if source == "" {
		source = tool.SourceMCP
	}
	definition := tool.ToolDefinition{
		Name: t.name, DisplayName: t.name, Source: source, Description: t.description,
		InputSchema: tool.ToolParametersSchema{
			Type:       "object",
			Properties: map[string]tool.ToolParam{"value": {Type: "string"}},
		},
		Annotations: tool.ToolAnnotations{ReadOnly: true},
	}
	if source == tool.SourceMCP {
		definition.ProviderID = tool.ProviderID("mcp:" + t.server)
		definition.Annotations.ServerName = t.server
		definition.Annotations.ToolName = t.rawName
	}
	return definition
}

func registerAgentDisclosureFixture(t *testing.T) *tool.RunToolAvailabilitySnapshot {
	t.Helper()
	fakes := []*disclosureAgentFakeTool{
		{name: "mcp__ghfix__create_issue", server: "ghfix", rawName: "create_issue", description: "Create a GitHub issue with title and body fields for the selected repository."},
		{name: "mcp__ghfix__list_issues", server: "ghfix", rawName: "list_issues", description: "List repository issues filtered by state, label and assignee."},
		{name: "mcp__plugin__notepack__store__write_note", server: "plugin__notepack__store", rawName: "write_note", description: "Write one note document into the plugin-owned note store."},
		{name: "configure_hook", source: tool.SourceBuiltin, description: "View or modify Awaken Hook configurations from natural language requests."},
		{name: "calculator", source: tool.SourceBuiltin, description: "Evaluate a basic arithmetic expression."},
		{name: "trash_file", source: tool.SourceBuiltin, description: "Move a file into the recoverable system trash."},
	}
	for _, fake := range fakes {
		fake := fake
		if err := tool.RegisterToolForce(fake); err != nil {
			t.Fatalf("register %s: %v", fake.name, err)
		}
		t.Cleanup(func() { tool.UnregisterTool(fake.name) })
	}
	snapshot := tool.NewRunToolAvailabilitySnapshot(tool.ExecutionEnvironmentSnapshot{SnapshotID: "env_disclosure_agent"})
	return &snapshot
}

func TestBuildToolDisclosureProjectionModes(t *testing.T) {
	snapshot := registerAgentDisclosureFixture(t)
	modelConfig := ModelConfig{ContextWindow: 128000}
	exposure := tool.DefaultToolExposurePolicy()
	offPolicy := tool.DisclosurePolicy{Mode: tool.DisclosureOff}
	alwaysPolicy := tool.DisclosurePolicy{Mode: tool.DisclosureAlways}

	off := buildToolDisclosureProjection(snapshot, modelConfig, nil, exposure, offPolicy, offPolicy)
	if off.Enabled() {
		t.Fatalf("off policies must disable disclosure: %+v", off)
	}

	always := buildToolDisclosureProjection(snapshot, modelConfig,
		[]string{"mcp__ghfix__list_issues", "configure_hook"}, exposure, alwaysPolicy, alwaysPolicy)
	if !always.MCPEnabled || !always.BuiltinEnabled {
		t.Fatalf("always policies must enable both groups: %+v", always)
	}
	if !always.DeferredMCP["mcp__ghfix__create_issue"] || always.DeferredMCP["mcp__ghfix__list_issues"] {
		t.Fatalf("activated MCP tool must leave the deferred set: %+v", always.DeferredMCP)
	}
	if !always.DeferredBuiltin["calculator"] || always.DeferredBuiltin["configure_hook"] {
		t.Fatalf("activated builtin tool must leave the deferred set: %+v", always.DeferredBuiltin)
	}
	if always.Deferred("trash_file") {
		t.Fatalf("non-deferrable builtin tools must never defer: %+v", always.DeferredBuiltin)
	}
	if !strings.Contains(always.MCPCatalog, "ghfix") || !strings.Contains(always.MCPCatalog, "plugin__notepack__store") {
		t.Fatalf("MCP catalog must list plain and plugin servers: %q", always.MCPCatalog)
	}
	if !strings.Contains(always.BuiltinCatalog, "configure_hook") || !strings.Contains(always.BuiltinCatalog, "calculator") {
		t.Fatalf("builtin catalog must list deferrable builtins: %q", always.BuiltinCatalog)
	}
	if strings.Contains(always.BuiltinCatalog, "trash_file") {
		t.Fatalf("builtin catalog must exclude non-deferrable tools: %q", always.BuiltinCatalog)
	}
	if !strings.HasPrefix(always.Description, toolFindGuidancePrefix) {
		t.Fatalf("description must lead with guidance: %q", always.Description)
	}
	if !strings.Contains(always.Description, strings.TrimSpace(toolFindBuiltinSectionHeader)) ||
		!strings.Contains(always.Description, strings.TrimSpace(toolFindMCPSectionHeader)) {
		t.Fatalf("description must carry both catalog sections: %q", always.Description)
	}
	// The test binary registers real deferrable tools from other packages, so
	// assert fixture membership rather than an exact count. Activation state
	// must not shrink the space.
	spaceNames := map[string]bool{}
	for _, entry := range always.SearchSpace.Builtin {
		spaceNames[entry.Name] = true
	}
	for _, server := range always.SearchSpace.Servers {
		for _, item := range server.Tools {
			spaceNames[item.Name] = true
		}
	}
	for _, required := range []string{
		"mcp__ghfix__create_issue", "mcp__ghfix__list_issues",
		"mcp__plugin__notepack__store__write_note", "configure_hook", "calculator",
	} {
		if !spaceNames[required] {
			t.Fatalf("search space missing %q (activation must not shrink it): %+v", required, spaceNames)
		}
	}
	if spaceNames["trash_file"] {
		t.Fatalf("non-deferrable builtin leaked into the search space: %+v", spaceNames)
	}
	// The catalogs deliberately ignore the activated set: activations must be
	// append-only tool additions, not catalog rewrites.
	baseline := buildToolDisclosureProjection(snapshot, modelConfig, nil, exposure, alwaysPolicy, alwaysPolicy)
	if baseline.MCPCatalog != always.MCPCatalog || baseline.BuiltinCatalog != always.BuiltinCatalog {
		t.Fatalf("catalogs must stay stable across activations")
	}

	// One group can defer while the other loads fully.
	mcpOnly := buildToolDisclosureProjection(snapshot, modelConfig, nil, exposure, alwaysPolicy, offPolicy)
	if !mcpOnly.MCPEnabled || mcpOnly.BuiltinEnabled {
		t.Fatalf("builtin off must keep MCP-only disclosure: %+v", mcpOnly)
	}
	if strings.Contains(mcpOnly.Description, strings.TrimSpace(toolFindBuiltinSectionHeader)) {
		t.Fatalf("builtin section must vanish when the group is off: %q", mcpOnly.Description)
	}
	if len(mcpOnly.SearchSpace.Builtin) != 0 {
		t.Fatalf("builtin search space must vanish when the group is off: %+v", mcpOnly.SearchSpace.Builtin)
	}
	builtinOnly := buildToolDisclosureProjection(snapshot, modelConfig, nil, exposure, offPolicy, alwaysPolicy)
	if builtinOnly.MCPEnabled || !builtinOnly.BuiltinEnabled {
		t.Fatalf("mcp off must keep builtin-only disclosure: %+v", builtinOnly)
	}

	// Auto with a huge threshold stays disabled but reports the measurements.
	autoHigh := tool.DisclosurePolicy{Mode: tool.DisclosureAuto, ThresholdPercent: 100}
	below := buildToolDisclosureProjection(snapshot, modelConfig, nil, exposure, autoHigh, autoHigh)
	if below.Enabled() || below.EstimatedMCPTokens <= 0 || below.MCPThresholdTokens <= 0 ||
		below.EstimatedBuiltinTokens <= 0 || below.BuiltinThresholdTokens <= 0 {
		t.Fatalf("auto below threshold must stay disabled with measurements: %+v", below)
	}

	// A tiny window forces the same catalogs above the auto threshold.
	autoLow := tool.DisclosurePolicy{Mode: tool.DisclosureAuto, ThresholdPercent: 1}
	tiny := buildToolDisclosureProjection(snapshot, ModelConfig{ContextWindow: 16500}, nil, exposure, autoLow, autoLow)
	if !tiny.MCPEnabled || !tiny.BuiltinEnabled {
		t.Fatalf("auto above threshold must enable disclosure: %+v", tiny)
	}
}

func TestBuildToolDisclosureProjectionRespectsExposurePolicy(t *testing.T) {
	snapshot := registerAgentDisclosureFixture(t)
	always := tool.DisclosurePolicy{Mode: tool.DisclosureAlways}
	hidden := buildToolDisclosureProjection(snapshot, ModelConfig{ContextWindow: 128000}, nil,
		tool.ToolExposurePolicy{ExtraHidden: []string{"configure_hook"}}, always, always)
	if hidden.DeferredBuiltin["configure_hook"] {
		t.Fatalf("hidden builtin must not enter the deferred set: %+v", hidden.DeferredBuiltin)
	}
	if strings.Contains(hidden.BuiltinCatalog, "configure_hook") {
		t.Fatalf("hidden builtin must not enter the catalog: %q", hidden.BuiltinCatalog)
	}
	for _, entry := range hidden.SearchSpace.Builtin {
		if entry.Name == "configure_hook" {
			t.Fatalf("hidden builtin must not enter the search space: %+v", hidden.SearchSpace.Builtin)
		}
	}
	if !hidden.DeferredBuiltin["calculator"] {
		t.Fatalf("visible deferrable builtins must still defer: %+v", hidden.DeferredBuiltin)
	}
}

func TestRequestBuilderGetToolsDefersBothGroupsByDefaultUntilActivated(t *testing.T) {
	snapshot := registerAgentDisclosureFixture(t)
	t.Setenv(tool.MCPDisclosureEnvVar, "")
	t.Setenv(tool.BuiltinDisclosureEnvVar, "")

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
	if _, visible := names["configure_hook"]; visible {
		t.Fatalf("deferred builtin tool must not be projected: %v", keysOf(names))
	}
	if _, visible := names["trash_file"]; !visible {
		t.Fatalf("non-deferrable builtin must stay projected: %v", keysOf(names))
	}
	description, hasFind := names[tool.ToolFindToolName]
	if !hasFind {
		t.Fatalf("tool_find must be projected while disclosure is active: %v", keysOf(names))
	}
	if !strings.Contains(description, "ghfix") || !strings.Contains(description, "create_issue") {
		t.Fatalf("tool_find description must embed the MCP catalog: %q", description)
	}
	if !strings.Contains(description, "configure_hook") || !strings.Contains(description, "calculator") {
		t.Fatalf("tool_find description must embed the builtin catalog: %q", description)
	}

	config.ActivatedDeferredTools = []string{"mcp__ghfix__create_issue", "configure_hook"}
	builder = NewRequestBuilder(config)
	names = map[string]string{}
	for _, schema := range builder.getTools() {
		names[schema.Function.Name] = schema.Function.Description
	}
	if _, visible := names["mcp__ghfix__create_issue"]; !visible {
		t.Fatalf("activated MCP tool must be projected by original name: %v", keysOf(names))
	}
	if _, visible := names["configure_hook"]; !visible {
		t.Fatalf("activated builtin tool must be projected by original name: %v", keysOf(names))
	}
	if _, visible := names["mcp__ghfix__list_issues"]; visible {
		t.Fatalf("non-activated MCP tool must stay deferred: %v", keysOf(names))
	}
	if _, visible := names["calculator"]; visible {
		t.Fatalf("non-activated builtin tool must stay deferred: %v", keysOf(names))
	}

	// Attribution must carry the embedded MCP catalog for context accounting
	// and mark on-demand builtin definitions as dynamic for Context Epoch.
	attributed := builder.attributeToolSchemas(builder.getTools())
	foundCatalog := false
	for _, item := range attributed {
		switch item.Schema.Function.Name {
		case tool.ToolFindToolName:
			if strings.TrimSpace(item.Provenance.MCPCatalog) != "" {
				foundCatalog = true
			}
			if item.Provenance.LoadedOnDemand {
				t.Fatalf("tool_find itself must not be marked on demand: %+v", item.Provenance)
			}
		case "configure_hook":
			if !item.Provenance.LoadedOnDemand {
				t.Fatalf("activated builtin must be marked loaded on demand: %+v", item.Provenance)
			}
			if !ContextEpochToolIsDynamic(ContextEpochToolCandidate{Schema: item.Schema, Provenance: item.Provenance}) {
				t.Fatalf("on-demand builtin must be epoch-dynamic: %+v", item.Provenance)
			}
		case "trash_file":
			if item.Provenance.LoadedOnDemand {
				t.Fatalf("always-loaded builtin must stay canonical: %+v", item.Provenance)
			}
			if ContextEpochToolIsDynamic(ContextEpochToolCandidate{Schema: item.Schema, Provenance: item.Provenance}) {
				t.Fatalf("always-loaded builtin must stay epoch-canonical: %+v", item.Provenance)
			}
		}
	}
	if !foundCatalog {
		t.Fatalf("tool_find provenance must carry the MCP catalog")
	}

	t.Setenv(tool.MCPDisclosureEnvVar, "off")
	t.Setenv(tool.BuiltinDisclosureEnvVar, "off")
	builder = NewRequestBuilder(config)
	names = map[string]string{}
	for _, schema := range builder.getTools() {
		names[schema.Function.Name] = schema.Function.Description
	}
	if _, visible := names[tool.ToolFindToolName]; visible {
		t.Fatalf("tool_find must be hidden when disclosure is off: %v", keysOf(names))
	}
	if _, visible := names["mcp__ghfix__list_issues"]; !visible {
		t.Fatalf("off policy must project every MCP tool: %v", keysOf(names))
	}
	if _, visible := names["calculator"]; !visible {
		t.Fatalf("off policy must project every builtin tool: %v", keysOf(names))
	}

	// Off builtin policy must not mark projected deferrable builtins dynamic.
	attributed = builder.attributeToolSchemas(builder.getTools())
	for _, item := range attributed {
		if item.Schema.Function.Name == "configure_hook" && item.Provenance.LoadedOnDemand {
			t.Fatalf("off policy must keep builtins canonical: %+v", item.Provenance)
		}
	}
}

func TestToolDisclosurePreparedRequestMatrixBeforeAndAfterActivation(t *testing.T) {
	snapshot := registerAgentDisclosureFixture(t)
	t.Setenv(tool.MCPDisclosureEnvVar, "")
	t.Setenv(tool.BuiltinDisclosureEnvVar, "")

	config := AgentConfig{
		ModelConfig:              ModelConfig{Model: "test-model", ContextWindow: 128000},
		ToolAvailabilitySnapshot: snapshot,
	}
	conversation := convcontext.New()
	conversation.AddUserMessage("hello")
	before := NewRequestBuilder(config).Build(conversation, nil)

	config.ActivatedDeferredTools = []string{"mcp__ghfix__create_issue", "configure_hook"}
	after := NewRequestBuilder(config).Build(conversation, nil)

	for _, protocol := range []llm.Protocol{
		llm.ProtocolOpenAIChat,
		llm.ProtocolOpenAIResponses,
		llm.ProtocolAnthropicMessages,
	} {
		t.Run(string(protocol), func(t *testing.T) {
			beforeTools := preparedDisclosureTools(t, protocol, before)
			if beforeTools[tool.ToolFindToolName] == nil {
				t.Fatalf("default disclosure request omitted tool_find: %v", keysOfPreparedTools(beforeTools))
			}
			for _, deferred := range []string{"mcp__ghfix__create_issue", "mcp__ghfix__list_issues", "configure_hook", "calculator"} {
				if beforeTools[deferred] != nil {
					t.Fatalf("default disclosure leaked deferred tool %q: %v", deferred, keysOfPreparedTools(beforeTools))
				}
			}
			if beforeTools["trash_file"] == nil {
				t.Fatalf("non-deferrable builtin missing from request: %v", keysOfPreparedTools(beforeTools))
			}

			afterTools := preparedDisclosureTools(t, protocol, after)
			for _, activated := range []string{"mcp__ghfix__create_issue", "configure_hook"} {
				if afterTools[activated] == nil {
					t.Fatalf("activated tool %q missing from request: %v", activated, keysOfPreparedTools(afterTools))
				}
			}
			if afterTools["mcp__ghfix__list_issues"] != nil || afterTools["calculator"] != nil {
				t.Fatalf("non-activated deferred tools leaked into request: %v", keysOfPreparedTools(afterTools))
			}
			if afterTools[tool.ToolFindToolName] == nil {
				t.Fatalf("tool_find must remain available after activations: %v", keysOfPreparedTools(afterTools))
			}
			if beforeTools[tool.ToolFindToolName]["description"] != afterTools[tool.ToolFindToolName]["description"] {
				t.Fatal("tool_find catalog description changed after activation")
			}

			schemaKey := "parameters"
			if protocol == llm.ProtocolAnthropicMessages {
				schemaKey = "input_schema"
			}
			activated := afterTools["mcp__ghfix__create_issue"]
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

func preparedDisclosureTools(t *testing.T, protocol llm.Protocol, request LLMRequest) map[string]map[string]any {
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

func TestContextUsageSplitsToolFindCatalogIntoMCPCategory(t *testing.T) {
	catalog := "- ghfix (2 tools): create_issue, list_issues"
	builtinCatalog := "- configure_hook: View or modify Hook configurations."
	req := LLMRequest{
		PromptSections: []PromptSection{{ID: "system", Kind: ContextContributionSystemPrompt, Content: "system"}},
		Messages: []convcontext.Message{
			{Role: convcontext.RoleUser, Content: "hello", Origin: convcontext.MessageOriginCurrentInput},
		},
		AttributedTools: []AttributedToolSchema{
			{
				Schema: tool.ToolSchema{Type: "function", Function: tool.ToolFunctionSchema{
					Name:        tool.ToolFindToolName,
					Description: "Search deferred tools.\n\nDeferred builtin tools:\n" + builtinCatalog + "\n\nConnected MCP servers and deferred tools:\n" + catalog,
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
		t.Fatalf("embedded MCP catalog must count toward the MCP category: %+v", snapshot.Contributions)
	}
	if byKind[ContextContributionBuiltinTools] <= 0 {
		t.Fatalf("the tool_find wrapper and builtin catalog must stay builtin: %+v", snapshot.Contributions)
	}
	mcpTokens := byKind[ContextContributionMCP]
	catalogOnly := convcontext.EstimateTokens(catalog)
	if mcpTokens > catalogOnly+2 {
		t.Fatalf("builtin catalog section must not leak into the MCP category: mcp=%d catalog=%d", mcpTokens, catalogOnly)
	}
	total := 0
	for _, contribution := range snapshot.Contributions {
		total += contribution.EstimatedTokens
	}
	if total != snapshot.EstimatedInputTokens {
		t.Fatalf("contribution sum invariant broken: %d != %d", total, snapshot.EstimatedInputTokens)
	}
}

// TestToolFindThroughToolRuntimeKeepsStrictOutcome runs tool_find through the
// full ToolRuntime pipeline (permission, input schema validation, execution,
// strict v2 outcome finalization). A details/output-schema mismatch would
// rewrite the success into tool_output_schema_mismatch, which unit tests that
// call Execute directly can never catch.
func TestToolFindThroughToolRuntimeKeepsStrictOutcome(t *testing.T) {
	snapshot := registerAgentDisclosureFixture(t)
	environment := tool.ExecutionEnvironmentSnapshot{SnapshotID: "env_disclosure_agent"}
	runtime := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		ToolAvailabilitySnapshot:     snapshot,
		ExecutionEnvironmentSnapshot: &environment,
	})
	projection := buildToolDisclosureProjection(snapshot, ModelConfig{ContextWindow: 128000}, nil,
		tool.DefaultToolExposurePolicy(),
		tool.DisclosurePolicy{Mode: tool.DisclosureAlways},
		tool.DisclosurePolicy{Mode: tool.DisclosureAlways})
	ctx := tool.ContextWithDeferredToolSearchSpace(stdcontext.Background(), projection.SearchSpace)

	call := convcontext.ToolCall{ID: "call_rt"}
	call.Function.Name = tool.ToolFindToolName
	call.Function.Arguments = `{"query":"ghfix create issue","limit":2}`
	result := runtime.Execute(ctx, call)
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

	builtinCall := convcontext.ToolCall{ID: "call_rt_builtin"}
	builtinCall.Function.Name = tool.ToolFindToolName
	builtinCall.Function.Arguments = `{"query":"configure_hook"}`
	builtinResult := runtime.Execute(ctx, builtinCall)
	if builtinResult.IsError || builtinResult.Outcome.IsError() {
		t.Fatalf("builtin search failed: %+v", builtinResult.Outcome)
	}
	if !strings.Contains(builtinResult.Content, "\"origin\": \"builtin\"") {
		t.Fatalf("builtin match must carry builtin origin: %s", builtinResult.Content)
	}
	for _, warning := range builtinResult.Outcome.Warnings {
		if warning.Code == "tool_output_schema_mismatch" {
			t.Fatalf("output schema mismatch warning: %+v", builtinResult.Outcome.Warnings)
		}
	}

	// The structured failure path must also survive strict finalization as a
	// semantic failure, not a validation crash.
	invalid := convcontext.ToolCall{ID: "call_rt_bad"}
	invalid.Function.Name = tool.ToolFindToolName
	invalid.Function.Arguments = `{"query":"   "}`
	failed := runtime.Execute(ctx, invalid)
	if !failed.Outcome.IsError() || failed.Outcome.ErrorCode != "tool_find_query_required" {
		t.Fatalf("empty query must fail with typed code: %+v", failed.Outcome)
	}
}

// TestCreateSubagentInheritsActivatedDeferredTools proves children sharing the
// parent's pinned snapshot also inherit the parent's on-demand loaded set
// (an independent copy, so child-local growth never mutates the parent).
func TestCreateSubagentInheritsActivatedDeferredTools(t *testing.T) {
	parent := AgentConfig{ActivatedDeferredTools: []string{"mcp__ghfix__create_issue", "configure_hook"}}
	child, err := CreateSubagent(parent, SpawnConfig{AgentID: "child-deferred", ParentAgentID: "root"})
	if err != nil {
		t.Fatalf("create subagent: %v", err)
	}
	if len(child.ActivatedDeferredTools) != 2 || child.ActivatedDeferredTools[1] != "configure_hook" {
		t.Fatalf("child must inherit parent activations: %+v", child.ActivatedDeferredTools)
	}
	child.ActivatedDeferredTools = append(child.ActivatedDeferredTools, "mcp__ghfix__list_issues")
	if len(parent.ActivatedDeferredTools) != 2 {
		t.Fatalf("child growth must not mutate parent: %+v", parent.ActivatedDeferredTools)
	}
}

type disclosureFakeRecorder struct {
	records []SemanticRecord
}

func (r *disclosureFakeRecorder) Append(_ stdcontext.Context, record SemanticRecord) error {
	r.records = append(r.records, record)
	return nil
}
func (r *disclosureFakeRecorder) AppendBatch(_ stdcontext.Context, records []SemanticRecord) error {
	r.records = append(r.records, records...)
	return nil
}
func (r *disclosureFakeRecorder) Flush(stdcontext.Context) error { return nil }

func TestRunnerAppliesAndPersistsDeferredToolActivation(t *testing.T) {
	snapshot := registerAgentDisclosureFixture(t)
	t.Setenv(tool.MCPDisclosureEnvVar, "always")
	t.Setenv(tool.BuiltinDisclosureEnvVar, "always")

	config := AgentConfig{
		ModelConfig:              ModelConfig{ContextWindow: 128000},
		ToolAvailabilitySnapshot: snapshot,
	}
	recorder := &disclosureFakeRecorder{}
	runner := NewRunner(config, convcontext.NewConversation(nil), NewRequestBuilder(config))
	runner.recorder = recorder
	runner.threadID = "thread_deferred"
	runner.runID = "run_deferred"

	// Filtering: unknown names, non-deferrable builtins and duplicates never
	// activate.
	newly := runner.newlyActivatedDeferredToolNames([]string{
		"mcp__ghfix__create_issue", "mcp__ghfix__create_issue", "configure_hook",
		"trash_file", "not_a_tool", tool.ToolFindToolName, " ",
	})
	if len(newly) != 2 || newly[0] != "mcp__ghfix__create_issue" || newly[1] != "configure_hook" {
		t.Fatalf("activation filter wrong: %+v", newly)
	}

	// Direct call of a deferred MCP tool auto-loads it for later turns.
	call := convcontext.ToolCall{ID: "call_1"}
	call.Function.Name = "mcp__ghfix__create_issue"
	runner.applyDeferredToolActivation(stdcontext.Background(), nil, call, nil)
	if len(runner.config.ActivatedDeferredTools) != 1 || runner.config.ActivatedDeferredTools[0] != "mcp__ghfix__create_issue" {
		t.Fatalf("direct call must auto-activate: %+v", runner.config.ActivatedDeferredTools)
	}
	if len(recorder.records) != 1 || recorder.records[0].Kind != SemanticKindDeferredToolsActivated {
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

	// Direct call of a deferred builtin tool auto-loads it the same way.
	builtinCall := convcontext.ToolCall{ID: "call_2"}
	builtinCall.Function.Name = "configure_hook"
	runner.applyDeferredToolActivation(stdcontext.Background(), nil, builtinCall, nil)
	if len(runner.config.ActivatedDeferredTools) != 2 || runner.config.ActivatedDeferredTools[1] != "configure_hook" {
		t.Fatalf("builtin direct call must auto-activate: %+v", runner.config.ActivatedDeferredTools)
	}
	visible = map[string]bool{}
	for _, schema := range runner.reqBuilder.getTools() {
		visible[schema.Function.Name] = true
	}
	if !visible["configure_hook"] {
		t.Fatalf("rebuilt request builder must project the activated builtin tool")
	}
	if visible["calculator"] {
		t.Fatalf("other deferrable builtins must stay deferred")
	}

	// tool_find loads: observer-provided names apply without a second record.
	findCall := convcontext.ToolCall{ID: "call_3"}
	findCall.Function.Name = tool.ToolFindToolName
	runner.applyDeferredToolActivation(stdcontext.Background(), nil, findCall, []string{"mcp__ghfix__list_issues"})
	if len(runner.config.ActivatedDeferredTools) != 3 {
		t.Fatalf("find-loaded tool must apply: %+v", runner.config.ActivatedDeferredTools)
	}

	// Re-activation is idempotent: no new record, no rebuild loop.
	recordsBefore := len(recorder.records)
	runner.applyDeferredToolActivation(stdcontext.Background(), nil, call, nil)
	if len(recorder.records) != recordsBefore || len(runner.config.ActivatedDeferredTools) != 3 {
		t.Fatalf("re-activation must be a no-op: records=%d activated=%+v", len(recorder.records), runner.config.ActivatedDeferredTools)
	}
}

// TestRunnerActivationRespectsExposureAndGroupPolicies proves a hidden tool
// never activates (even by direct call) and per-group off policies keep the
// other group's lenient path working.
func TestRunnerActivationRespectsExposureAndGroupPolicies(t *testing.T) {
	snapshot := registerAgentDisclosureFixture(t)
	t.Setenv(tool.MCPDisclosureEnvVar, "always")
	t.Setenv(tool.BuiltinDisclosureEnvVar, "always")

	hiddenConfig := AgentConfig{
		ModelConfig:              ModelConfig{ContextWindow: 128000},
		ToolAvailabilitySnapshot: snapshot,
		ToolExposure:             tool.ToolExposurePolicy{ExtraHidden: []string{"configure_hook"}},
	}
	runner := NewRunner(hiddenConfig, convcontext.NewConversation(nil), NewRequestBuilder(hiddenConfig))
	if newly := runner.newlyActivatedDeferredToolNames([]string{"configure_hook"}); len(newly) != 0 {
		t.Fatalf("hidden tool must never activate: %+v", newly)
	}
	if newly := runner.newlyActivatedDeferredToolNames([]string{"calculator"}); len(newly) != 1 {
		t.Fatalf("visible deferrable builtin must activate: %+v", newly)
	}

	t.Setenv(tool.BuiltinDisclosureEnvVar, "off")
	offConfig := AgentConfig{
		ModelConfig:              ModelConfig{ContextWindow: 128000},
		ToolAvailabilitySnapshot: snapshot,
	}
	offRunner := NewRunner(offConfig, convcontext.NewConversation(nil), NewRequestBuilder(offConfig))
	if newly := offRunner.newlyActivatedDeferredToolNames([]string{"configure_hook"}); len(newly) != 0 {
		t.Fatalf("builtin-off policy must not activate builtins: %+v", newly)
	}
	if newly := offRunner.newlyActivatedDeferredToolNames([]string{"mcp__ghfix__create_issue"}); len(newly) != 1 {
		t.Fatalf("MCP group must keep activating while builtin is off: %+v", newly)
	}
}
