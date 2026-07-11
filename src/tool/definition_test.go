package tool

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

type definitionTestTool struct {
	name        string
	description string
	parameters  []ToolParameter
	schema      ToolParametersSchema
	definition  ToolDefinition
}

func (t *definitionTestTool) Name() string { return t.name }

func (t *definitionTestTool) Description() string { return t.description }

func (t *definitionTestTool) Parameters() []ToolParameter {
	return append([]ToolParameter(nil), t.parameters...)
}

func (t *definitionTestTool) InputSchema() ToolParametersSchema {
	if !hasInputSchema(t.schema) {
		return SchemaFromParameters(t.parameters)
	}
	return cloneParametersSchema(t.schema)
}

func (t *definitionTestTool) Definition() ToolDefinition {
	return cloneToolDefinition(t.definition)
}

func (t *definitionTestTool) Execute(context.Context, string) (string, error) { return "", nil }

func TestResolveDefinitionAdaptsLegacyWithoutInventingExamples(t *testing.T) {
	runtime := &definitionTestTool{
		name:        "legacy_reader",
		description: "Read a task-scoped text resource.",
		parameters: []ToolParameter{{
			Name:        "path",
			Description: "Task-scoped path to read.",
			Type:        "string",
			Required:    true,
		}},
		definition: ToolDefinition{
			Name:        "legacy_reader",
			Source:      SourceBuiltin,
			Description: "Read a task-scoped text resource.",
			Annotations: ToolAnnotations{
				ReadOnly:        true,
				CapabilityClass: CapabilityArtifactRead,
				FileBridgeScope: FileBridgeTaskScopeRead,
				NetworkScope:    NetworkScopeNone,
			},
		},
	}

	report := ResolveDefinitionReport(runtime)
	if report.Quality != DefinitionQualityAdapted {
		t.Fatalf("expected adapted definition, got %s (%v)", report.Quality, report.Warnings)
	}
	def := report.Definition
	if def.SchemaVersion != ToolDefinitionSchemaVersion {
		t.Fatalf("expected schema version %d, got %d", ToolDefinitionSchemaVersion, def.SchemaVersion)
	}
	if def.Purpose == "" || len(def.WhenToUse) == 0 || len(def.WhenNotToUse) == 0 {
		t.Fatalf("expected complete selection guidance, got %+v", def)
	}
	if def.InputSchema.Properties["path"].Type != "string" {
		t.Fatalf("expected input schema adapted from Parameters, got %+v", def.InputSchema)
	}
	if def.OutputSchema.Type != "object" || len(def.OutputSchema.Properties) != 0 {
		t.Fatalf("expected no-details output schema, got %+v", def.OutputSchema)
	}
	if len(def.SideEffects) != 1 || def.SideEffects[0].Kind != SideEffectNone {
		t.Fatalf("expected honest read-only side effect, got %+v", def.SideEffects)
	}
	if def.Permissions.WorkspaceScope != WorkspaceScopeRead || def.Permissions.FileBridgeScope != FileBridgeTaskScopeRead {
		t.Fatalf("expected derived read permission facts, got %+v", def.Permissions)
	}
	if def.Examples != nil || def.FailureModes != nil {
		t.Fatalf("compatibility must not invent examples or failure modes: %+v / %+v", def.Examples, def.FailureModes)
	}
}

func TestBuildBuiltinDefinitionProducesStrictAuthoringContract(t *testing.T) {
	runtime := strictDefinitionTestTool(t)

	if err := ValidateToolWithMode(runtime, DefinitionValidationStrictBuiltin); err != nil {
		t.Fatalf("expected strict builtin definition to validate, got %v", err)
	}
	report := ResolveDefinitionReport(runtime)
	if report.Quality != DefinitionQualityStrict {
		t.Fatalf("expected strict quality, got %s (%v)", report.Quality, report.Warnings)
	}
	if report.Definition.InputSchema.AdditionalProperties == nil || *report.Definition.InputSchema.AdditionalProperties {
		t.Fatalf("expected closed input schema, got %+v", report.Definition.InputSchema)
	}
	if report.Definition.OutputSchema.Type != "object" {
		t.Fatalf("expected typed-details output object, got %+v", report.Definition.OutputSchema)
	}
}

func TestExplicitV2InputSchemaWinsOverRuntimeProvider(t *testing.T) {
	runtime := strictDefinitionTestTool(t)
	runtime.schema = ToolParametersSchema{
		Type:                 "object",
		AdditionalProperties: BoolPtr(false),
		Properties: map[string]ToolParam{
			"provider_only": {Type: "string", Description: "Provider field."},
		},
	}
	runtime.definition.InputSchema = ToolParametersSchema{
		Type:                 "object",
		AdditionalProperties: BoolPtr(false),
		Properties: map[string]ToolParam{
			"explicit": {Type: "string", Description: "Explicit v2 field."},
		},
		Required: []string{"explicit"},
	}
	runtime.definition.Examples[0].Input = map[string]any{"explicit": "value"}

	resolved := ResolveDefinition(runtime)
	if _, exists := resolved.InputSchema.Properties["explicit"]; !exists {
		t.Fatalf("expected explicit v2 schema, got %+v", resolved.InputSchema)
	}
	if _, exists := resolved.InputSchema.Properties["provider_only"]; exists {
		t.Fatalf("provider schema must not override explicit v2 schema: %+v", resolved.InputSchema)
	}
}

func TestStrictValidationRejectsAdaptedAndInvalidContracts(t *testing.T) {
	legacy := catalogTestTool{name: "legacy_contract"}
	if err := ValidateToolWithMode(legacy, DefinitionValidationStrictBuiltin); err == nil || !strings.Contains(err.Error(), "definition quality") {
		t.Fatalf("expected strict mode to reject adapted legacy definition, got %v", err)
	}

	mismatch := catalogTestTool{name: "runtime_name"}
	mismatchName := definitionTestTool{
		name:        mismatch.name,
		description: "test",
		definition: ToolDefinition{
			Name:        "different_name",
			Source:      SourceBuiltin,
			Description: "test",
		},
	}
	if err := ValidateTool(&mismatchName); err == nil || !strings.Contains(err.Error(), "must equal runtime name") {
		t.Fatalf("expected identity mismatch rejection, got %v", err)
	}

	invalidOutput := strictDefinitionTestTool(t)
	invalidOutput.definition.OutputSchema.Properties = map[string]ToolParam{
		"status": {Type: "string", Description: "Must not shadow envelope status."},
	}
	if err := ValidateResolvedDefinition(invalidOutput.definition); err == nil || !strings.Contains(err.Error(), "canonical ExecutionOutcome") {
		t.Fatalf("expected canonical output collision rejection, got %v", err)
	}

	invalidExample := strictDefinitionTestTool(t)
	invalidExample.definition.Examples[0].Input = map[string]any{"path": 42}
	if err := ValidateResolvedDefinition(invalidExample.definition); err == nil || !strings.Contains(err.Error(), "examples[0].input") {
		t.Fatalf("expected invalid example rejection, got %v", err)
	}
}

func TestModelDescriptionPreservesSafetyWithinHardBudget(t *testing.T) {
	runtime := strictDefinitionTestTool(t)
	def := ResolveDefinition(runtime)
	def.WhenToUse = []string{"the workspace file must be inspected before editing"}
	def.WhenNotToUse = []string{"the path is outside task scope", "the task requires a write"}
	def.SideEffects = []ToolSideEffect{{
		Kind:          SideEffectRead,
		Description:   "Reads only the declared task-scoped file.",
		Reversibility: ReversibilityNotApplicable,
	}}
	def.Permissions.RequiresAuth = true

	description := ModelDescriptionWithBudget(def, 320)
	for _, required := range []string{"Do not use when:", "path is outside task scope", "Effects:", "requires authenticated access"} {
		if !strings.Contains(description, required) {
			t.Fatalf("expected safety boundary %q to survive budget, got %q", required, description)
		}
	}
	if strings.Contains(description, "Use when:") {
		t.Fatalf("expected optional usage detail to be omitted under tiny budget, got %q", description)
	}
	if got := utf8.RuneCountInString(description); got > 320 {
		t.Fatalf("description exceeded hard budget: %d", got)
	}
}

func TestModelDescriptionBoundsUntrustedSummary(t *testing.T) {
	def := ResolveDefinition(strictDefinitionTestTool(t))
	def.Description = strings.Repeat("external description ", 2000)
	description := ModelDescription(def)
	if got := utf8.RuneCountInString(description); got > DefaultModelDescriptionBudget {
		t.Fatalf("untrusted summary exceeded default budget: %d", got)
	}
	for _, label := range []string{"Do not use when:", "Effects:", "Permissions:"} {
		if !strings.Contains(description, label) {
			t.Fatalf("required boundary %q was lost: %q", label, description)
		}
	}
}

func TestModelDescriptionListsTypedDetailsDeterministically(t *testing.T) {
	def := ResolveDefinition(strictDefinitionTestTool(t))
	def.OutputSchema = ToolParam{
		Type:                 "object",
		AdditionalProperties: BoolPtr(false),
		Properties: map[string]ToolParam{
			"stderr": {Type: "string"},
			"stdout": {Type: "string"},
		},
	}
	description := ModelDescription(def)
	if !strings.Contains(description, "tool-specific details include stderr, stdout") {
		t.Fatalf("expected sorted typed details, got %q", description)
	}
}

func TestRegistryStoresResolvedDefinitionSnapshot(t *testing.T) {
	runtime := strictDefinitionTestTool(t)
	runtime.name = "registry_definition_snapshot"
	runtime.definition.Name = runtime.name
	runtime.definition.DisplayName = runtime.name
	if err := RegisterToolForceWithMode(runtime, DefinitionValidationStrictBuiltin); err != nil {
		t.Fatalf("register strict tool: %v", err)
	}
	defer UnregisterTool(runtime.name)

	runtime.definition.WhenToUse[0] = "mutated after registration"
	runtime.definition.InputSchema.Properties["path"] = ToolParam{Type: "number", Description: "mutated"}

	registered, err := GetRegisteredTool(runtime.name)
	if err != nil {
		t.Fatalf("get registered tool: %v", err)
	}
	if registered.Origin != SourceBuiltin || registered.Quality != DefinitionQualityStrict {
		t.Fatalf("expected separate origin/quality facts, got %+v", registered)
	}
	if !registered.PreserveInputObject {
		t.Fatal("authored v2 input shape must be frozen in the registry snapshot")
	}
	if registered.Definition.WhenToUse[0] == "mutated after registration" || registered.Definition.InputSchema.Properties["path"].Type != "string" {
		t.Fatalf("registry definition must be an immutable snapshot: %+v", registered.Definition)
	}

	registered.Definition.WhenNotToUse[0] = "mutated returned copy"
	again, err := GetRegisteredTool(runtime.name)
	if err != nil {
		t.Fatalf("get registered tool again: %v", err)
	}
	if again.Definition.WhenNotToUse[0] == "mutated returned copy" {
		t.Fatal("GetRegisteredTool must return a cloned definition")
	}
}

func TestRegistryTextDescriptionUsesResolvedInputSchema(t *testing.T) {
	runtime := strictDefinitionTestTool(t)
	runtime.name = "registry_text_uses_resolved_schema"
	runtime.definition.Name = runtime.name
	runtime.definition.DisplayName = runtime.name
	runtime.definition.InputSchema = ToolParametersSchema{
		Type: "object", AdditionalProperties: BoolPtr(false), Required: []string{"canonical_query"},
		Properties: map[string]ToolParam{"canonical_query": {Type: "string", Description: "Canonical query value."}},
	}
	runtime.definition.Examples[0].Input = map[string]any{"canonical_query": "status"}
	if err := RegisterToolForceWithMode(runtime, DefinitionValidationStrictBuiltin); err != nil {
		t.Fatalf("register strict tool: %v", err)
	}
	defer UnregisterTool(runtime.name)

	description := GetToolsDescription()
	start := strings.Index(description, "- "+runtime.name+":")
	if start < 0 {
		t.Fatalf("registry text omitted test tool: %s", description)
	}
	block := description[start:]
	if end := strings.Index(block[1:], "\n- "); end >= 0 {
		block = block[:end+1]
	}
	if !strings.Contains(block, "canonical_query (string) (required)") {
		t.Fatalf("registry text did not use resolved input schema: %s", block)
	}
	if strings.Contains(block, "\n  - path ") {
		t.Fatal("registry text leaked legacy Parameters instead of the resolved schema")
	}
}

func strictDefinitionTestTool(t *testing.T) *definitionTestTool {
	t.Helper()
	runtime := &definitionTestTool{
		name:        "strict_reader",
		description: "Read one task-scoped UTF-8 file.",
		parameters: []ToolParameter{{
			Name:        "path",
			Description: "Task-scoped path, resolved from the workspace root.",
			Type:        "string",
			Required:    true,
		}},
		definition: ToolDefinition{
			Purpose:      "Read one task-scoped UTF-8 file without modifying it.",
			WhenToUse:    []string{"the exact contents of a known text file are needed"},
			WhenNotToUse: []string{"the resource is binary or the task requires modifying it"},
			SideEffects: []ToolSideEffect{{
				Kind:          SideEffectNone,
				Description:   "Does not modify workspace or external state.",
				Reversibility: ReversibilityNotApplicable,
			}},
			Examples: []ToolExample{{
				Scenario: "Read the workspace README.",
				Input:    map[string]any{"path": "README.md"},
				Outcome:  "Returns file content in typed details.",
			}},
			FailureModes: []ToolFailureMode{{
				Code:      "file_not_found",
				Cause:     "The resolved path does not exist.",
				Retryable: false,
				Recovery:  []string{"List the parent directory and choose an existing path."},
			}},
			Annotations: ToolAnnotations{
				ReadOnly:        true,
				CapabilityClass: CapabilityArtifactRead,
				FileBridgeScope: FileBridgeTaskScopeRead,
				NetworkScope:    NetworkScopeNone,
			},
		},
	}
	runtime.definition = BuildBuiltinDefinition(runtime, runtime.definition)
	return runtime
}
