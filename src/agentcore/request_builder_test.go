package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/skillruntime"
	"awaken/src/skills"
	"awaken/src/tool"
	_ "awaken/src/tools/computer"
	_ "awaken/src/tools/edit"
	_ "awaken/src/tools/write"
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

func TestRequestBuilderDeclaresSameTaskHistoryContinuity(t *testing.T) {
	ctx := convcontext.New()
	ctx.AddUserMessage("你好")
	ctx.AddAssistantMessage("你好，Nolan！有什么我可以帮你的吗？", nil)
	ctx.AddUserMessage("你看到我上一个对话是什么不？")

	req := NewRequestBuilder(AgentConfig{}).Build(ctx, []tool.ToolSchema{})
	for _, expected := range []string{
		"If any messages appear before the current user input",
		"They are not a separate conversation",
		"上一个对话",
		"Do not claim that the system did not provide them",
	} {
		if !strings.Contains(req.SystemPrompt, expected) {
			t.Fatalf("same-task continuity contract missing %q from system prompt: %s", expected, req.SystemPrompt)
		}
	}

	if len(req.Messages) != 3 ||
		req.Messages[0].Role != convcontext.RoleUser || req.Messages[0].Content != "你好" ||
		req.Messages[1].Role != convcontext.RoleAssistant || req.Messages[1].Content != "你好，Nolan！有什么我可以帮你的吗？" ||
		req.Messages[2].Role != convcontext.RoleUser || req.Messages[2].Content != "你看到我上一个对话是什么不？" {
		t.Fatalf("continuity contract must preserve the standard ordered message history: %+v", req.Messages)
	}
	if req.Messages[0].Origin != convcontext.MessageOriginConversationHistory ||
		req.Messages[1].Origin != convcontext.MessageOriginConversationHistory ||
		req.Messages[2].Origin != convcontext.MessageOriginCurrentInput {
		t.Fatalf("unexpected host-owned history attribution: %+v", req.Messages)
	}
	fresh := convcontext.New()
	fresh.AddUserMessage("全新任务")
	freshReq := NewRequestBuilder(AgentConfig{}).Build(fresh, []tool.ToolSchema{})
	if freshReq.SystemPrompt != req.SystemPrompt {
		t.Fatal("conversation continuity block must remain byte-stable across history lengths")
	}

	prepared, err := llm.PrepareRequest(llm.Config{
		APIKey: "test", BaseURL: "https://api.kimi.com/coding/v1", Model: "k3",
		Protocol: llm.ProtocolOpenAIChat, ProviderID: "kimi-coding-plan", ProfileID: "k3-test",
	}, llm.Request{
		SystemPrompt: req.SystemPrompt,
		Messages:     req.Messages,
		Model:        "k3",
	}, true)
	if err != nil {
		t.Fatalf("prepare OpenAI-compatible continuity request: %v", err)
	}
	var body struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatalf("decode prepared continuity request: %v", err)
	}
	if len(body.Messages) != 4 {
		t.Fatalf("expected system plus three ordered messages, got %+v", body.Messages)
	}
	for index, expected := range []struct {
		role    string
		content string
	}{
		{role: "system", content: "If any messages appear before the current user input"},
		{role: "user", content: "你好"},
		{role: "assistant", content: "你好，Nolan！有什么我可以帮你的吗？"},
		{role: "user", content: "你看到我上一个对话是什么不？"},
	} {
		message := body.Messages[index]
		if message["role"] != expected.role || !strings.Contains(fmt.Sprint(message["content"]), expected.content) {
			t.Fatalf("unexpected provider message %d: %+v", index, message)
		}
		for _, hostField := range []string{"origin", "message_id", "turn_id", "thread_id", "run_id", "session_id", "created_at"} {
			if _, exists := message[hostField]; exists {
				t.Fatalf("provider message leaked host-only field %q: %+v", hostField, message)
			}
		}
	}
}

func TestRequestBuilderProjectsHistoricalImagesForTextOnlyModel(t *testing.T) {
	ctx := convcontext.New()
	ctx.AddUserMessageWithAttachments("", []convcontext.MessageAttachment{{
		ID: "image-1", Kind: "image", MimeType: "image/png", DisplayName: "history.png", DataBase64: "aGVsbG8=",
	}})

	builder := NewRequestBuilder(AgentConfig{ModelConfig: ModelConfig{
		Model: "deepseek-v4-pro", Protocol: string(llm.ProtocolOpenAIChat), ProviderID: "deepseek",
		Capabilities: tool.ModelCapabilities{SupportsVision: false},
	}})
	request := builder.Build(ctx, nil)
	if got := request.Messages[0].Attachments[0].DataBase64; got != "" {
		t.Fatalf("text-only request retained image bytes: %q", got)
	}
	if request.Messages[0].Content != "" {
		t.Fatalf("capability admission must not rewrite durable message content, got %q", request.Messages[0].Content)
	}
	if !strings.Contains(request.Messages[0].ProviderProjectionText, unsupportedVisionProjectionPrefix) ||
		!strings.Contains(request.Messages[0].ProviderProjectionText, "history.png") {
		t.Fatalf("expected deterministic request-only text projection, got %q", request.Messages[0].ProviderProjectionText)
	}
	if original := ctx.GetMessages()[0].Attachments[0].DataBase64; original != "aGVsbG8=" {
		t.Fatalf("request projection mutated durable conversation media: %q", original)
	}

	prepared, err := llm.PrepareRequest(llm.Config{
		APIKey: "test", BaseURL: "https://api.deepseek.com", Model: "deepseek-v4-pro",
		Protocol: llm.ProtocolOpenAIChat, ProviderID: "deepseek",
	}, llm.Request{
		SystemPrompt: request.SystemPrompt, Messages: request.Messages, Tools: request.Tools, Model: request.Model,
	}, true)
	if err != nil {
		t.Fatalf("prepare text-only request: %v", err)
	}
	if strings.Contains(string(prepared.Body), `"image_url"`) || strings.Contains(string(prepared.Body), "aGVsbG8=") {
		t.Fatalf("prepared DeepSeek body leaked an unsupported image block: %s", prepared.Body)
	}
}

func TestRequestBuilderPreservesHistoricalImagesForVisionModel(t *testing.T) {
	ctx := convcontext.New()
	ctx.AddUserMessageWithAttachments("inspect", []convcontext.MessageAttachment{{
		ID: "image-1", Kind: "image", MimeType: "image/png", DisplayName: "history.png", DataBase64: "aGVsbG8=",
	}})

	request := NewRequestBuilder(AgentConfig{ModelConfig: ModelConfig{
		Model: "vision-model", Capabilities: tool.ModelCapabilities{SupportsVision: true},
	}}).Build(ctx, nil)
	if got := request.Messages[0].Attachments[0].DataBase64; got != "aGVsbG8=" {
		t.Fatalf("vision request lost image bytes: %q", got)
	}
	if strings.Contains(request.Messages[0].ProviderProjectionText, unsupportedVisionProjectionPrefix) {
		t.Fatalf("vision request received text-only projection: %q", request.Messages[0].ProviderProjectionText)
	}
	stale := []convcontext.Message{{
		Role: convcontext.RoleUser, Content: "inspect", ProviderProjectionText: unsupportedVisionProjectionPrefix,
		Attachments: []convcontext.MessageAttachment{{Kind: "image", DataBase64: "aGVsbG8="}},
	}}
	if got := projectMessagesForModelCapabilities(stale, tool.ModelCapabilities{SupportsVision: true})[0].ProviderProjectionText; got != "" {
		t.Fatalf("vision projection inherited a prior text-only run note: %q", got)
	}
}

func TestModelCapabilityProjectionDropsUnsupportedToolMediaOnlyFromRequestClone(t *testing.T) {
	original := []convcontext.Message{{
		Role: convcontext.RoleTool,
		ModelContent: []tool.ModelContentPart{
			{Type: "image", MimeType: "image/png", DisplayName: "chart.png", DataBase64: "aW1hZ2U="},
			{Type: "file", MimeType: "application/pdf", DisplayName: "report.pdf", DataBase64: "cGRm"},
		},
	}}
	projected := projectMessagesForModelCapabilities(original, tool.ModelCapabilities{})
	if projected[0].ModelContent[0].DataBase64 != "" || projected[0].ModelContent[1].DataBase64 != "" {
		t.Fatalf("unsupported tool media remained provider-visible: %+v", projected[0].ModelContent)
	}
	if original[0].ModelContent[0].DataBase64 == "" || original[0].ModelContent[1].DataBase64 == "" {
		t.Fatalf("projection mutated original tool media: %+v", original[0].ModelContent)
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

func TestRequestBuilderExposesComputerToolsOnlyForReadyRunSnapshot(t *testing.T) {
	names := func(request LLMRequest) map[string]bool {
		got := make(map[string]bool, len(request.Tools))
		for _, schema := range request.Tools {
			got[schema.Function.Name] = true
		}
		return got
	}
	assertComputerVisibility := func(t *testing.T, got map[string]bool, want bool) {
		t.Helper()
		for _, name := range []string{"computer_observe", "computer_act", "computer_verify"} {
			if got[name] != want {
				t.Fatalf("computer tool %q visible=%v, want %v", name, got[name], want)
			}
		}
	}

	disabledEnvironment := tool.ExecutionEnvironmentSnapshot{SnapshotID: "computer-disabled"}
	disabledSnapshot := tool.NewRunToolAvailabilitySnapshot(disabledEnvironment)
	disabled := NewRequestBuilder(AgentConfig{
		ExecutionEnvironmentSnapshot: &disabledEnvironment,
		ToolAvailabilitySnapshot:     &disabledSnapshot,
	}).Build(convcontext.New(), nil)
	assertComputerVisibility(t, names(disabled), false)

	enabledEnvironment := tool.ExecutionEnvironmentSnapshot{SnapshotID: "computer-enabled", ComputerEnabled: true}
	enabledSnapshot := tool.NewRunToolAvailabilitySnapshot(enabledEnvironment)
	enabled := NewRequestBuilder(AgentConfig{
		ExecutionEnvironmentSnapshot: &enabledEnvironment,
		ToolAvailabilitySnapshot:     &enabledSnapshot,
	}).Build(convcontext.New(), nil)
	assertComputerVisibility(t, names(enabled), true)
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

func TestRequestBuilderInjectsSkillTriggerRulesAndCatalogIntoFinalToolSchema(t *testing.T) {
	root := t.TempDir()
	writeSkill := func(id, extraFrontmatter string) {
		t.Helper()
		dir := filepath.Join(root, id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		content := "---\nname: " + id + "\ndescription: Create polished " + id + " deliverables.\n" + extraFrontmatter + "---\nFollow the " + id + " workflow.\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeSkill("documents", "")
	writeSkill("manual-only", "disable-model-invocation: true\n")

	registry := skills.NewSkillRegistry(nil)
	registry.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	catalogTool := &skillruntime.CatalogTool{
		Registry:  registry,
		Activator: skillruntime.NewActivator(registry, nil),
	}
	previous, previousErr := tool.GetRegisteredTool(skillruntime.CatalogToolName)
	if err := tool.RegisterToolForce(catalogTool); err != nil {
		t.Fatalf("register skill catalog tool: %v", err)
	}
	t.Cleanup(func() {
		if previousErr == nil {
			if err := tool.RegisterToolForce(previous.Runtime); err != nil {
				t.Errorf("restore previous skill tool: %v", err)
			}
			return
		}
		tool.UnregisterTool(skillruntime.CatalogToolName)
	})

	snapshot := tool.NewRunToolAvailabilitySnapshot(tool.ExecutionEnvironmentSnapshot{})
	builder := NewRequestBuilder(AgentConfig{
		ModelConfig:              ModelConfig{ContextWindow: 100_000},
		ToolAvailabilitySnapshot: &snapshot,
	})
	request := builder.Build(convcontext.New(), nil)

	var skillDescription string
	var skillCatalog string
	for _, candidate := range request.Tools {
		if candidate.Function.Name == skillruntime.CatalogToolName {
			skillDescription = candidate.Function.Description
			break
		}
	}
	for _, candidate := range request.AttributedTools {
		if candidate.Schema.Function.Name == skillruntime.CatalogToolName {
			skillCatalog = candidate.Provenance.SkillCatalog
			break
		}
	}
	if skillDescription == "" {
		t.Fatalf("final provider request omitted the unified skill tool: %+v", request.Tools)
	}
	for _, expected := range []string{
		"Trigger rules (mandatory):",
		"If the user explicitly names an available Skill from this catalog, you MUST invoke it.",
		"BLOCKING REQUIREMENT",
		"If multiple Skills match",
		"If nothing in the catalog matches",
		"- documents [user]: Create polished documents deliverables.",
	} {
		if !strings.Contains(skillDescription, expected) {
			t.Fatalf("final skill tool description missing %q: %s", expected, skillDescription)
		}
	}
	if strings.Count(skillDescription, "Trigger rules (mandatory):") != 1 {
		t.Fatalf("final request duplicated the authoritative trigger rules: %s", skillDescription)
	}
	if strings.Contains(skillDescription, "manual-only") {
		t.Fatalf("model-disabled Skill leaked into the model-invocable catalog: %s", skillDescription)
	}
	if strings.Contains(request.SystemPrompt, "Trigger rules (mandatory):") {
		t.Fatalf("system prompt duplicated the authoritative trigger rules: %s", request.SystemPrompt)
	}
	if !strings.Contains(skillCatalog, "- documents [user]: Create polished documents deliverables.") {
		t.Fatalf("typed Skill provenance omitted the model-visible catalog listing: %q", skillCatalog)
	}
	if strings.Contains(skillCatalog, "Trigger rules (mandatory):") || strings.Contains(skillCatalog, "manual-only") {
		t.Fatalf("Skill catalog provenance must exclude the builtin wrapper and model-disabled Skills: %q", skillCatalog)
	}
}

func TestRequestBuilderDefaultExposureContainsCanonicalFileTools(t *testing.T) {
	for _, name := range []string{"Edit", "Write"} {
		if _, err := tool.GetTool(name); err != nil {
			t.Fatalf("%s must be registered: %v", name, err)
		}
	}

	builder := NewRequestBuilder(AgentConfig{})
	req := builder.Build(convcontext.New(), nil)
	names := map[string]bool{}
	for _, definition := range req.Tools {
		names[definition.Function.Name] = true
	}
	if !names["Edit"] || !names["Write"] {
		t.Fatalf("canonical file tools missing under default exposure: %v", names)
	}

	compat := NewRequestBuilder(AgentConfig{ToolExposure: tool.CompatToolExposurePolicy()})
	compatReq := compat.Build(convcontext.New(), nil)
	compatNames := map[string]bool{}
	for _, definition := range compatReq.Tools {
		compatNames[definition.Function.Name] = true
	}
	if !compatNames["Edit"] || !compatNames["Write"] {
		t.Fatalf("compat exposure must preserve canonical file tools: %v", compatNames)
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

func TestRequestBuilderInjectsHostMemoryBeforeSkillAndAddsDelta(t *testing.T) {
	builder := NewRequestBuilder(AgentConfig{
		MemoryContextProvider: func() (string, error) { return "<awaken_memory_context>index</awaken_memory_context>", nil },
		SkillContextBlocks:    []ContextBlock{{ID: "skill", Content: "skill body"}},
	})
	envelope := builder.BuildContextEnvelope(convcontext.New(), []tool.ToolSchema{})
	if len(envelope.SystemBlocks) < 3 || envelope.SystemBlocks[1].ID != "memory_context" || envelope.SystemBlocks[2].ID != "skill" {
		t.Fatalf("memory must be host-owned before Skill bodies: %+v", envelope.SystemBlocks)
	}
	found := false
	for _, delta := range envelope.Deltas {
		if delta.Type == ContextDeltaMemoryHit && strings.HasPrefix(delta.Summary, "index:") {
			found = true
		}
	}
	if !found {
		t.Fatalf("memory delta missing: %+v", envelope.Deltas)
	}
}

func TestRequestBuilderDegradesWhenMemoryProviderFails(t *testing.T) {
	builder := NewRequestBuilder(AgentConfig{MemoryContextProvider: func() (string, error) { return "", fmt.Errorf("offline") }})
	envelope := builder.BuildContextEnvelope(convcontext.New(), []tool.ToolSchema{})
	for _, block := range envelope.SystemBlocks {
		if block.ID == "memory_context" {
			t.Fatalf("failed provider injected a block: %+v", block)
		}
	}
}

func TestRequestBuilderSendsTheProjectFolderGroupToTheModel(t *testing.T) {
	ctx := convcontext.New()
	ctx.AddUserMessage("hello")
	base := t.TempDir()
	primary := filepath.Join(base, "01_汇总")
	secondary := filepath.Join(base, "02_跟踪")

	builder := NewRequestBuilder(AgentConfig{
		SystemPrompt: "static {{.RuntimeInfo}}",
		ModelConfig:  ModelConfig{Model: "test-model"},
		Workspace: WorkspaceConfig{
			ProjectID:     "project_test",
			ProjectName:   "测试项目",
			Root:          primary,
			ReadableRoots: []string{primary, secondary},
		},
	})

	envelope := builder.BuildContextEnvelope(ctx, nil)
	snapshot := envelope.RuntimeSnapshot
	if snapshot.ProjectName != "测试项目" {
		t.Fatalf("project name lost: %+v", snapshot)
	}
	if len(snapshot.ProjectFolders) != 2 {
		t.Fatalf("expected both folders, got %+v", snapshot.ProjectFolders)
	}
	if snapshot.ProjectFolders[0].Path != primary || !snapshot.ProjectFolders[0].IsPrimary {
		t.Fatalf("working directory must be the primary folder: %+v", snapshot.ProjectFolders)
	}
	if snapshot.ProjectFolders[1].IsPrimary {
		t.Fatalf("secondary folder marked primary: %+v", snapshot.ProjectFolders)
	}

	// The folder group only counts if it survives into the prompt the provider
	// actually receives.
	request := builder.BuildLLMRequestFromEnvelope(envelope)
	if !strings.Contains(request.SystemPrompt, `<project id="project_test" name="测试项目" />`) {
		t.Fatalf("project name missing from system prompt: %s", request.SystemPrompt)
	}
	if !strings.Contains(request.SystemPrompt, secondary) {
		t.Fatalf("secondary folder missing from system prompt: %s", request.SystemPrompt)
	}
}

func TestRequestBuilderKeepsASessionWorkspaceOutOfTheProjectFolderGroup(t *testing.T) {
	ctx := convcontext.New()
	ctx.AddUserMessage("hello")
	sessionRoot := filepath.Join(t.TempDir(), "sessions", "project_1", "workspace")

	builder := NewRequestBuilder(AgentConfig{
		SystemPrompt: "static {{.RuntimeInfo}}",
		ModelConfig:  ModelConfig{Model: "test-model"},
		Workspace: WorkspaceConfig{
			ProjectName:      "空项目",
			Root:             sessionRoot,
			SessionWorkspace: true,
		},
	})

	envelope := builder.BuildContextEnvelope(ctx, nil)
	if len(envelope.RuntimeSnapshot.ProjectFolders) != 0 {
		t.Fatalf("Awaken's own session directory is not a user folder: %+v", envelope.RuntimeSnapshot.ProjectFolders)
	}
	request := builder.BuildLLMRequestFromEnvelope(envelope)
	if !strings.Contains(request.SystemPrompt, "未绑定文件夹") {
		t.Fatalf("expected the folder-less note, got %s", request.SystemPrompt)
	}
}

func TestRuntimeSnapshotIdentityTracksProjectIdentity(t *testing.T) {
	base := RuntimeSnapshot{WorkingDirectory: "/data/a", ProjectName: "one"}
	renamed := base
	renamed.ProjectName = "two"
	if runtimeSnapshotIdentity(base) == runtimeSnapshotIdentity(renamed) {
		t.Fatal("a rename must change the runtime snapshot fingerprint")
	}
	regrouped := base
	regrouped.ProjectFolders = []ProjectFolderObservation{{Path: "/data/b"}}
	if runtimeSnapshotIdentity(base) == runtimeSnapshotIdentity(regrouped) {
		t.Fatal("a changed folder group must change the runtime snapshot fingerprint")
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
	if len(envelope.SystemBlocks) != 2 || envelope.SystemBlocks[0].Content != "static {{.RuntimeInfo}}" || envelope.SystemBlocks[1].ID != conversationContinuityBlockID {
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
