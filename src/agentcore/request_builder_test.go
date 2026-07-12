package agentcore

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
	_ "awaken/src/tools/apply_patch"
	_ "awaken/src/tools/text_edit"
)

type requestBuilderDefinitionTool struct{}

func (requestBuilderDefinitionTool) Name() string { return "request_builder_v2_contract" }

func (requestBuilderDefinitionTool) Description() string { return "LEGACY_METHOD_DESCRIPTION" }

func (requestBuilderDefinitionTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{{Name: "legacy", Description: "Legacy-only parameter.", Type: "string"}}
}

func (requestBuilderDefinitionTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		SchemaVersion: tool.ToolDefinitionSchemaVersion,
		Name:          "request_builder_v2_contract",
		DisplayName:   "Request Builder v2 Contract",
		Source:        tool.SourceBuiltin,
		Description:   "Inspect one canonical request-builder contract.",
		Purpose:       "Prove that model schemas are compiled from resolved ToolDefinition v2 facts.",
		WhenToUse:     []string{"the request builder needs the canonical query object"},
		WhenNotToUse:  []string{"the legacy flat parameter is requested"},
		InputSchema: tool.ToolParametersSchema{
			Type:                 "object",
			AdditionalProperties: tool.BoolPtr(false),
			Required:             []string{"query"},
			Properties: map[string]tool.ToolParam{
				"query": {Type: "string", Description: "Canonical query text."},
			},
		},
		OutputSchema: tool.NoDetailsOutputSchema(),
		SideEffects: []tool.ToolSideEffect{{
			Kind:          tool.SideEffectNone,
			Description:   "Does not modify state.",
			Reversibility: tool.ReversibilityNotApplicable,
		}},
		Permissions: tool.ToolPermissionSpec{
			CapabilityClass: tool.CapabilityArtifactRead,
			WorkspaceScope:  tool.WorkspaceScopeRead,
			FileBridgeScope: tool.FileBridgeNone,
			NetworkScope:    tool.NetworkScopeNone,
		},
		Examples: []tool.ToolExample{{
			Scenario: "Compile a canonical query.",
			Input:    map[string]any{"query": "status"},
		}},
		FailureModes: []tool.ToolFailureMode{{
			Code:  "invalid_query",
			Cause: "The canonical query is invalid.",
		}},
		Annotations: tool.ToolAnnotations{
			ReadOnly:        true,
			CapabilityClass: tool.CapabilityArtifactRead,
			FileBridgeScope: tool.FileBridgeNone,
			NetworkScope:    tool.NetworkScopeNone,
		},
	}
}

func (requestBuilderDefinitionTool) Execute(context.Context, string) (string, error) {
	return "", nil
}

func TestRequestBuilderBuildsCompleteRequest(t *testing.T) {
	ctx := convcontext.New()
	ctx.AddUserMessage("hello")

	tools := []tool.ToolSchema{
		{
			Type: "function",
			Function: tool.ToolFunctionSchema{
				Name:        "explicit_tool",
				Description: "explicit tool",
				Parameters: tool.ToolParametersSchema{
					Type:       "object",
					Properties: map[string]tool.ToolParam{},
				},
			},
		},
	}

	builder := NewRequestBuilder(AgentConfig{
		EntryPoint: "test-entry",
		ModelConfig: ModelConfig{
			Model:         "test-model",
			Temperature:   0.2,
			MaxTokens:     777,
			ContextWindow: 400000,
		},
	})

	req := builder.Build(ctx, tools)
	if req.SystemPrompt == "" {
		t.Fatal("expected system prompt")
	}
	if len(req.Messages) != 1 || req.Messages[0].Content != "hello" {
		t.Fatalf("unexpected messages: %+v", req.Messages)
	}
	if len(req.Tools) != 1 || req.Tools[0].Function.Name != "explicit_tool" {
		t.Fatalf("unexpected tools: %+v", req.Tools)
	}
	if req.Model != "test-model" || req.Temperature != 0.2 || req.MaxTokens != 777 {
		t.Fatalf("unexpected model config: %+v", req)
	}
	if req.ContextWindowState.EstimatedTokens == 0 {
		t.Fatalf("expected context window estimate on request")
	}
	if req.ContextWindowState.ModelWindow != 400000 {
		t.Fatalf("expected configured context window, got %+v", req.ContextWindowState)
	}
	if !strings.Contains(req.SystemPrompt, "test-model") || !strings.Contains(req.SystemPrompt, "test-entry") {
		t.Fatalf("expected runtime info in system prompt: %s", req.SystemPrompt)
	}
}

func TestRequestBuilderUsesRegisteredTools(t *testing.T) {
	toolName := "request_builder_registered_tool"
	if err := tool.RegisterTool(runnerTestTool{name: toolName}); err != nil {
		t.Fatalf("failed to register test tool %s: %v", toolName, err)
	}

	builder := NewRequestBuilder(AgentConfig{})
	req := builder.Build(convcontext.New(), nil)

	for _, definition := range req.Tools {
		if definition.Function.Name == toolName {
			if !strings.Contains(req.SystemPrompt, toolName) {
				t.Fatalf("expected system prompt tool summary to mention %s", toolName)
			}
			return
		}
	}
	t.Fatalf("expected registered tool %s in request tools", toolName)
}

func TestRequestBuilderCompilesDefinitionV2WithoutSkillPoolNarrowing(t *testing.T) {
	runtime := requestBuilderDefinitionTool{}
	if err := tool.RegisterToolForceWithMode(runtime, tool.DefinitionValidationStrictBuiltin); err != nil {
		t.Fatalf("register strict v2 tool: %v", err)
	}
	defer tool.UnregisterTool(runtime.Name())

	builder := NewRequestBuilder(AgentConfig{})
	req := builder.Build(convcontext.New(), nil)
	var schema tool.ToolFunctionSchema
	found := false
	for _, candidate := range req.Tools {
		if candidate.Function.Name == runtime.Name() {
			schema = candidate.Function
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected v2 tool in unified catalog, got %d tools", len(req.Tools))
	}
	if _, exists := schema.Parameters.Properties["query"]; !exists {
		t.Fatalf("expected explicit v2 recursive schema, got %+v", schema.Parameters)
	}
	if _, exists := schema.Parameters.Properties["legacy"]; exists {
		t.Fatalf("legacy Parameters must not override explicit v2 schema: %+v", schema.Parameters)
	}
	for _, expected := range []string{"Inspect one canonical", "Use when:", "Do not use when:", "Effects:", "Returns:"} {
		if !strings.Contains(schema.Description, expected) {
			t.Fatalf("compiled description missing %q: %s", expected, schema.Description)
		}
	}
	if strings.Contains(schema.Description, "LEGACY_METHOD_DESCRIPTION") {
		t.Fatalf("legacy method description overrode canonical v2 facts: %s", schema.Description)
	}
}

func TestRequestBuilderDefaultExposureHidesApplyPatch(t *testing.T) {
	if _, err := tool.GetTool("text_edit"); err != nil {
		t.Fatalf("text_edit must be registered: %v", err)
	}
	if _, err := tool.GetTool("apply_patch"); err != nil {
		t.Fatalf("apply_patch must be registered: %v", err)
	}

	builder := NewRequestBuilder(AgentConfig{})
	req := builder.Build(convcontext.New(), nil)
	names := map[string]bool{}
	for _, definition := range req.Tools {
		names[definition.Function.Name] = true
	}
	if !names["text_edit"] {
		t.Fatal("expected text_edit visible under default exposure")
	}
	if names["apply_patch"] {
		t.Fatal("ordinary providers must hide apply_patch by default")
	}

	compat := NewRequestBuilder(AgentConfig{ToolExposure: tool.CompatToolExposurePolicy()})
	compatReq := compat.Build(convcontext.New(), nil)
	compatNames := map[string]bool{}
	for _, definition := range compatReq.Tools {
		compatNames[definition.Function.Name] = true
	}
	if !compatNames["apply_patch"] {
		t.Fatal("compat exposure must keep apply_patch visible")
	}
}

func TestRequestBuilderRefreshesRuntimeInfo(t *testing.T) {
	builder := NewRequestBuilder(AgentConfig{
		EntryPoint:  "first",
		ModelConfig: ModelConfig{Model: "first-model"},
	})

	first := builder.Build(convcontext.New(), nil)
	builder.config.EntryPoint = "second"
	builder.config.ModelConfig.Model = "second-model"
	second := builder.Build(convcontext.New(), nil)

	if first.SystemPrompt == second.SystemPrompt {
		t.Fatal("expected runtime prompt to change after config update")
	}
	if !strings.Contains(second.SystemPrompt, "second") || !strings.Contains(second.SystemPrompt, "second-model") {
		t.Fatalf("expected updated runtime info, got %s", second.SystemPrompt)
	}
}

func TestRequestBuilderUsesConfiguredWorkspaceRoot(t *testing.T) {
	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	builder := NewRequestBuilder(AgentConfig{
		Workspace: WorkspaceConfig{Root: workspaceRoot},
	})

	req := builder.Build(convcontext.New(), nil)
	if !strings.Contains(req.SystemPrompt, workspaceRoot) {
		t.Fatalf("expected system prompt to include workspace root %q, got %s", workspaceRoot, req.SystemPrompt)
	}
}

func TestRequestBuilderIncludesSkillSummary(t *testing.T) {
	skillSummary := "- id: daily-followup; name: daily-followup; scope: project"
	builder := NewRequestBuilder(AgentConfig{
		SystemPrompt: "base {{.RuntimeInfo}}",
		SkillSummary: skillSummary,
	})

	envelope := builder.BuildContextEnvelope(convcontext.New(), []tool.ToolSchema{})
	if envelope.RuntimeSnapshot.SkillSummary != skillSummary {
		t.Fatalf("expected skill summary in snapshot, got %q", envelope.RuntimeSnapshot.SkillSummary)
	}
	if len(envelope.SystemContextItems) != 1 || !strings.Contains(envelope.SystemContextItems[0].Content, "skill_summary="+skillSummary) {
		t.Fatalf("expected skill summary in runtime system item, got %+v", envelope.SystemContextItems)
	}
	foundCacheKey := false
	for _, key := range envelope.CacheKeys {
		if key.Name == "runtime_snapshot" && strings.Contains(key.Value, "skills:"+skillSummary) {
			foundCacheKey = true
			break
		}
	}
	if !foundCacheKey {
		t.Fatalf("expected skill summary in runtime snapshot cache key, got %+v", envelope.CacheKeys)
	}

	req := builder.BuildLLMRequestFromEnvelope(envelope)
	if !strings.Contains(req.SystemPrompt, skillSummary) {
		t.Fatalf("expected skill summary in system prompt, got %s", req.SystemPrompt)
	}
}

func TestRequestBuilderBuildsContextEnvelope(t *testing.T) {
	ctx := convcontext.New()
	ctx.AddUserMessage("hello")
	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	writableRoot := filepath.Join(workspaceRoot, "safe")
	tools := []tool.ToolSchema{
		{
			Type: "function",
			Function: tool.ToolFunctionSchema{
				Name:        "explicit_tool",
				Description: "explicit tool",
				Parameters: tool.ToolParametersSchema{
					Type:       "object",
					Properties: map[string]tool.ToolParam{},
				},
			},
		},
	}

	builder := NewRequestBuilder(AgentConfig{
		SystemPrompt: "static {{.RuntimeInfo}}",
		EntryPoint:   "test-entry",
		ModelConfig: ModelConfig{
			Model:       "test-model",
			Temperature: 0.2,
			MaxTokens:   777,
		},
		Workspace: WorkspaceConfig{
			Root:          workspaceRoot,
			WritableRoots: []string{writableRoot},
		},
	})

	envelope := builder.BuildContextEnvelope(ctx, tools)
	if len(envelope.SystemBlocks) != 1 || envelope.SystemBlocks[0].Content != "static {{.RuntimeInfo}}" {
		t.Fatalf("unexpected system blocks: %+v", envelope.SystemBlocks)
	}
	if len(envelope.SystemContextItems) != 1 || envelope.SystemContextItems[0].Type != "runtime_snapshot" {
		t.Fatalf("expected runtime snapshot system context item, got %+v", envelope.SystemContextItems)
	}
	if len(envelope.Messages) != 1 || envelope.Messages[0].Content != "hello" {
		t.Fatalf("unexpected envelope messages: %+v", envelope.Messages)
	}
	if len(envelope.ToolSchemas) != 1 || envelope.ToolSchemas[0].Function.Name != "explicit_tool" {
		t.Fatalf("unexpected tool schemas: %+v", envelope.ToolSchemas)
	}
	if envelope.RuntimeSnapshot.ModelName != "test-model" || envelope.RuntimeSnapshot.EntryPoint != "test-entry" {
		t.Fatalf("unexpected runtime snapshot: %+v", envelope.RuntimeSnapshot)
	}
	if envelope.RuntimeSnapshot.WorkspaceRoot != workspaceRoot {
		t.Fatalf("expected workspace root %q, got %q", workspaceRoot, envelope.RuntimeSnapshot.WorkspaceRoot)
	}
	if len(envelope.RuntimeSnapshot.WritableRoots) != 1 || envelope.RuntimeSnapshot.WritableRoots[0] != writableRoot {
		t.Fatalf("unexpected writable roots: %+v", envelope.RuntimeSnapshot.WritableRoots)
	}
	if !contextEnvelopeHasDelta(envelope, ContextDeltaRuntimeSnapshot) || !contextEnvelopeHasDelta(envelope, ContextDeltaToolPool) {
		t.Fatalf("expected runtime snapshot and tool pool deltas, got %+v", envelope.Deltas)
	}
	if len(envelope.CacheKeys) == 0 {
		t.Fatalf("expected cache keys")
	}
	if envelope.ContextWindowState.EstimatedTokens == 0 {
		t.Fatalf("expected context window estimate")
	}
}

func TestRequestBuilderBuildsLLMRequestFromEnvelope(t *testing.T) {
	ctx := convcontext.New()
	ctx.AddUserMessage("hello")
	tools := []tool.ToolSchema{
		{
			Type: "function",
			Function: tool.ToolFunctionSchema{
				Name:        "explicit_tool",
				Description: "explicit tool",
				Parameters: tool.ToolParametersSchema{
					Type:       "object",
					Properties: map[string]tool.ToolParam{},
				},
			},
		},
	}
	builder := NewRequestBuilder(AgentConfig{
		SystemPrompt: "base {{.RuntimeInfo}}",
		EntryPoint:   "test-entry",
		ModelConfig: ModelConfig{
			Model:       "test-model",
			Temperature: 0.2,
			MaxTokens:   777,
		},
	})

	envelope := builder.BuildContextEnvelope(ctx, tools)
	req := builder.BuildLLMRequestFromEnvelope(envelope)
	if !strings.Contains(req.SystemPrompt, "base") ||
		!strings.Contains(req.SystemPrompt, "test-entry") ||
		!strings.Contains(req.SystemPrompt, "test-model") ||
		!strings.Contains(req.SystemPrompt, "explicit_tool") {
		t.Fatalf("expected request system prompt to include static prompt, runtime, and tool summary, got %s", req.SystemPrompt)
	}
	if len(req.Messages) != 1 || req.Messages[0].Content != "hello" {
		t.Fatalf("unexpected request messages: %+v", req.Messages)
	}
	if len(req.Tools) != 1 || req.Tools[0].Function.Name != "explicit_tool" {
		t.Fatalf("unexpected request tools: %+v", req.Tools)
	}
	if req.Model != "test-model" || req.Temperature != 0.2 || req.MaxTokens != 777 {
		t.Fatalf("unexpected model config: %+v", req)
	}
}

func contextEnvelopeHasDelta(envelope ContextEnvelope, deltaType ContextDeltaType) bool {
	for _, delta := range envelope.Deltas {
		if delta.Type == deltaType {
			return true
		}
	}
	return false
}
