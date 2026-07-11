package tool

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// DefinitionValidationMode controls migration strictness without changing the
// default registration contract for legacy and third-party tools.
type DefinitionValidationMode string

const (
	DefinitionValidationCompat        DefinitionValidationMode = "compat"
	DefinitionValidationStrictBuiltin DefinitionValidationMode = "strict_builtin"
	DefinitionValidationStrictAll     DefinitionValidationMode = "strict_all"
)

var builtinNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

var canonicalOutcomeFields = map[string]struct{}{
	"schema_version": {}, "content": {}, "status": {}, "transport_status": {}, "semantic_status": {}, "process_status": {},
	"exit_code": {}, "signal": {}, "timed_out": {}, "error_code": {}, "error_category": {}, "error_message": {},
	"retryable": {}, "recovery_options": {}, "diagnostic": {}, "affected_paths": {}, "patch_summary": {}, "artifacts": {},
	"summary": {}, "operation": {}, "changes": {}, "environment": {}, "warnings": {}, "errors": {}, "next_actions": {}, "details": {},
	"started_at": {}, "finished_at": {}, "duration_ms": {}, "truncated": {},
}

// ValidateToolWithMode validates the stable legacy interface in all modes and
// the complete authored v2 contract in strict modes. strict_builtin remains
// permissive for external MCP/Skill tools during the additive migration.
func ValidateToolWithMode(runtime Tool, mode DefinitionValidationMode) error {
	if runtime == nil {
		return fmt.Errorf("%w: nil tool", ErrInvalidTool)
	}
	if !isValidToolName(runtime.Name()) {
		return fmt.Errorf("%w: tool name %q must match %s", ErrInvalidTool, runtime.Name(), mcpSafeNamePattern.String())
	}
	for _, param := range runtime.Parameters() {
		if !safeNamePattern.MatchString(param.Name) {
			return fmt.Errorf("%w: parameter name %q must match %s", ErrInvalidTool, param.Name, safeNamePattern.String())
		}
		if !isJSONSchemaType(param.Type) {
			return fmt.Errorf("%w: parameter %q has invalid JSON Schema type %q", ErrInvalidTool, param.Name, param.Type)
		}
	}

	raw := runtime.Definition()
	if strings.TrimSpace(raw.Name) == "" {
		return fmt.Errorf("%w: ToolDefinition.Name is required", ErrInvalidTool)
	}
	if raw.Name != runtime.Name() {
		return fmt.Errorf("%w: ToolDefinition.Name %q must equal runtime name %q", ErrInvalidTool, raw.Name, runtime.Name())
	}
	if raw.Source == "" {
		return fmt.Errorf("%w: ToolDefinition.Source is required", ErrInvalidTool)
	}
	if strings.TrimSpace(raw.Description) == "" {
		return fmt.Errorf("%w: ToolDefinition.Description is required", ErrInvalidTool)
	}

	strict := mode == DefinitionValidationStrictAll || (mode == DefinitionValidationStrictBuiltin && raw.Source == SourceBuiltin)
	if !strict {
		return nil
	}
	report := ResolveDefinitionReport(runtime)
	if report.Quality != DefinitionQualityStrict {
		return fmt.Errorf("%w: tool %q definition quality is %s, strict is required (%s)", ErrInvalidTool, runtime.Name(), report.Quality, strings.Join(report.Warnings, "; "))
	}
	if err := ValidateResolvedDefinition(report.Definition); err != nil {
		return fmt.Errorf("%w: tool %q: %v", ErrInvalidTool, runtime.Name(), err)
	}
	return nil
}

// ValidateResolvedDefinition performs the authoring/lint checks for a complete
// v2 definition. It does not grant permissions or prove runtime behaviour.
func ValidateResolvedDefinition(def ToolDefinition) error {
	if def.SchemaVersion != ToolDefinitionSchemaVersion {
		return fmt.Errorf("schema_version must be %d", ToolDefinitionSchemaVersion)
	}
	if strings.TrimSpace(def.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if def.Source == SourceBuiltin && !builtinNamePattern.MatchString(def.Name) {
		return fmt.Errorf("builtin name %q must use lower snake_case", def.Name)
	}
	if !isKnownSource(def.Source) {
		return fmt.Errorf("source %q is not recognized", def.Source)
	}
	if strings.TrimSpace(def.Description) == "" {
		return fmt.Errorf("description is required")
	}
	if strings.TrimSpace(def.Purpose) == "" {
		return fmt.Errorf("purpose is required")
	}
	if len(cleanStrings(def.WhenToUse)) == 0 {
		return fmt.Errorf("when_to_use must contain actionable guidance")
	}
	if len(cleanStrings(def.WhenNotToUse)) == 0 {
		return fmt.Errorf("when_not_to_use must contain a boundary")
	}
	if err := validateInputSchemaForDefinition(def.InputSchema); err != nil {
		return fmt.Errorf("input_schema: %w", err)
	}
	if err := validateOutputSchemaForDefinition(def.OutputSchema); err != nil {
		return fmt.Errorf("output_schema: %w", err)
	}
	if err := validateSideEffects(def); err != nil {
		return err
	}
	if err := validatePermissions(def); err != nil {
		return err
	}
	if err := validateExamples(def); err != nil {
		return err
	}
	if err := validateFailureModes(def.FailureModes); err != nil {
		return err
	}
	return nil
}

func validateInputSchemaForDefinition(schema ToolParametersSchema) error {
	if schema.Type != "object" {
		return fmt.Errorf("root type must be object, got %q", schema.Type)
	}
	root := ToolParam{
		Type:                 schema.Type,
		Description:          schema.Description,
		Properties:           schema.Properties,
		Required:             schema.Required,
		AdditionalProperties: schema.AdditionalProperties,
		OneOf:                schema.OneOf,
	}
	return validateDefinitionSchemaNode("$", root, true)
}

func validateOutputSchemaForDefinition(schema ToolParam) error {
	if schema.Type != "object" {
		return fmt.Errorf("typed details root must be object, got %q", schema.Type)
	}
	if err := validateDefinitionSchemaNode("$.details", schema, false); err != nil {
		return err
	}
	for name := range schema.Properties {
		if _, forbidden := canonicalOutcomeFields[name]; forbidden {
			return fmt.Errorf("property %q duplicates a canonical ExecutionOutcome field", name)
		}
	}
	return nil
}

func validateDefinitionSchemaNode(path string, schema ToolParam, requireDescriptions bool) error {
	if schema.Type == "" {
		if len(schema.OneOf) == 0 {
			return fmt.Errorf("%s type is required", path)
		}
	} else if !isRecursiveSchemaType(schema.Type) {
		return fmt.Errorf("%s has unsupported type %q", path, schema.Type)
	}
	if schema.Type == "object" {
		if schema.AdditionalProperties == nil {
			return fmt.Errorf("%s must declare additionalProperties", path)
		}
		required := make(map[string]struct{}, len(schema.Required))
		for _, name := range schema.Required {
			if _, duplicate := required[name]; duplicate {
				return fmt.Errorf("%s required property %q is duplicated", path, name)
			}
			required[name] = struct{}{}
			if _, exists := schema.Properties[name]; !exists {
				return fmt.Errorf("%s required property %q is not declared", path, name)
			}
		}
		keys := make([]string, 0, len(schema.Properties))
		for name := range schema.Properties {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		for _, name := range keys {
			child := schema.Properties[name]
			if requireDescriptions && strings.TrimSpace(child.Description) == "" {
				return fmt.Errorf("%s.%s description is required", path, name)
			}
			if err := validateDefinitionSchemaNode(path+"."+name, child, requireDescriptions); err != nil {
				return err
			}
		}
	}
	if schema.Type == "array" {
		if schema.Items == nil {
			return fmt.Errorf("%s array items schema is required", path)
		}
		if err := validateDefinitionSchemaNode(path+"[]", *schema.Items, requireDescriptions); err != nil {
			return err
		}
	}
	for i, option := range schema.OneOf {
		if err := validateDefinitionSchemaNode(fmt.Sprintf("%s.oneOf[%d]", path, i), option, requireDescriptions); err != nil {
			return err
		}
	}
	return nil
}

func validateSideEffects(def ToolDefinition) error {
	if len(def.SideEffects) == 0 {
		return fmt.Errorf("side_effects must be declared")
	}
	for i, effect := range def.SideEffects {
		if strings.TrimSpace(effect.Kind) == "" {
			return fmt.Errorf("side_effects[%d].kind is required", i)
		}
		if strings.TrimSpace(effect.Description) == "" {
			return fmt.Errorf("side_effects[%d].description is required", i)
		}
		if !isKnownReversibility(effect.Reversibility) {
			return fmt.Errorf("side_effects[%d].reversibility %q is invalid", i, effect.Reversibility)
		}
		if def.Annotations.ReadOnly && isMutationSideEffect(effect.Kind) {
			return fmt.Errorf("read-only annotation conflicts with %q side effect", effect.Kind)
		}
	}
	if def.Annotations.Destructive {
		destructiveFact := false
		for _, effect := range def.SideEffects {
			if isMutationSideEffect(effect.Kind) && effect.Reversibility != ReversibilityNotApplicable {
				destructiveFact = true
				break
			}
		}
		if !destructiveFact {
			return fmt.Errorf("destructive annotation requires a mutation side effect and reversibility fact")
		}
	}
	return nil
}

func validatePermissions(def ToolDefinition) error {
	spec := def.Permissions
	if spec.CapabilityClass == "" || spec.CapabilityClass == CapabilityUnknown {
		return fmt.Errorf("permissions.capability_class must declare a concrete capability")
	}
	if strings.TrimSpace(spec.WorkspaceScope) == "" || spec.FileBridgeScope == "" || spec.NetworkScope == "" {
		return fmt.Errorf("permissions must declare workspace, file bridge, and network scopes")
	}
	ann := def.Annotations
	if ann.CapabilityClass != "" && spec.CapabilityClass != ann.CapabilityClass {
		return fmt.Errorf("permissions capability_class conflicts with annotations")
	}
	if ann.FileBridgeScope != "" && spec.FileBridgeScope != ann.FileBridgeScope {
		return fmt.Errorf("permissions file_bridge_scope conflicts with annotations")
	}
	if ann.NetworkScope != "" && spec.NetworkScope != ann.NetworkScope {
		return fmt.Errorf("permissions network_scope conflicts with annotations")
	}
	if ann.RequiresAuth && !spec.RequiresAuth {
		return fmt.Errorf("permissions requires_auth conflicts with annotations")
	}
	if ann.Destructive && strings.TrimSpace(spec.RequiresReview) == "" {
		return fmt.Errorf("destructive tool must declare permissions.requires_review")
	}
	return nil
}

func validateExamples(def ToolDefinition) error {
	if len(def.Examples) == 0 {
		return fmt.Errorf("examples must contain at least one validated invocation")
	}
	for i, example := range def.Examples {
		if strings.TrimSpace(example.Scenario) == "" {
			return fmt.Errorf("examples[%d].scenario is required", i)
		}
		if example.Input == nil {
			return fmt.Errorf("examples[%d].input is required", i)
		}
		raw, err := json.Marshal(example.Input)
		if err != nil {
			return fmt.Errorf("examples[%d].input is not JSON-serializable: %w", i, err)
		}
		if err := ValidateInputAgainstSchema(string(raw), def.InputSchema); err != nil {
			return fmt.Errorf("examples[%d].input is invalid: %w", i, err)
		}
	}
	return nil
}

func validateFailureModes(modes []ToolFailureMode) error {
	if len(modes) == 0 {
		return fmt.Errorf("failure_modes must contain at least one stable error contract")
	}
	seen := make(map[string]struct{}, len(modes))
	for i, mode := range modes {
		code := strings.TrimSpace(mode.Code)
		if code == "" {
			return fmt.Errorf("failure_modes[%d].code is required", i)
		}
		if _, duplicate := seen[code]; duplicate {
			return fmt.Errorf("failure_modes code %q is duplicated", code)
		}
		seen[code] = struct{}{}
		if strings.TrimSpace(mode.Cause) == "" {
			return fmt.Errorf("failure_modes[%d].cause is required", i)
		}
	}
	return nil
}

func isRecursiveSchemaType(value string) bool {
	switch value {
	case "string", "number", "integer", "boolean", "object", "array", "null", "any":
		return true
	default:
		return false
	}
}

func isKnownSource(source ToolSource) bool {
	switch source {
	case SourceBuiltin, SourceMCP, SourceConnector, SourceSkill, SourcePlugin:
		return true
	default:
		return false
	}
}

func isKnownReversibility(value string) bool {
	switch value {
	case ReversibilityNotApplicable, ReversibilityReversible, ReversibilityConditional, ReversibilityIrreversible, ReversibilityUnknown:
		return true
	default:
		return false
	}
}

func isMutationSideEffect(kind string) bool {
	switch kind {
	case SideEffectCreate, SideEffectUpdate, SideEffectDelete, SideEffectExecute, SideEffectUnknown:
		return true
	default:
		return false
	}
}
