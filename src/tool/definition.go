package tool

import (
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	// ToolDefinitionSchemaVersion is the current resolved definition contract.
	ToolDefinitionSchemaVersion = 2

	SideEffectNone    = "none"
	SideEffectRead    = "read"
	SideEffectCreate  = "create"
	SideEffectUpdate  = "update"
	SideEffectDelete  = "delete"
	SideEffectExecute = "execute"
	SideEffectNetwork = "network"
	SideEffectUnknown = "unknown"

	ReversibilityNotApplicable = "not_applicable"
	ReversibilityReversible    = "reversible"
	ReversibilityConditional   = "conditional"
	ReversibilityIrreversible  = "irreversible"
	ReversibilityUnknown       = "unknown"

	WorkspaceScopeNone      = "none"
	WorkspaceScopeRead      = "workspace_read"
	WorkspaceScopeWrite     = "workspace_write"
	WorkspaceScopeTask      = "task_scope"
	WorkspaceScopeUncertain = "unknown"

	DefaultModelDescriptionBudget = 1800
)

// DefinitionQuality reports the trustworthiness of author-supplied metadata.
// It is deliberately separate from ToolDefinition.Source: origin identifies
// who supplied a tool, while quality identifies how much adaptation was needed.
type DefinitionQuality string

const (
	DefinitionQualityStrict     DefinitionQuality = "strict"
	DefinitionQualityAdapted    DefinitionQuality = "adapted"
	DefinitionQualityIncomplete DefinitionQuality = "incomplete"
)

// DefinitionResolution is the result of adapting a raw definition to v2.
// Warnings are registry/audit metadata and are not sent to the model.
type DefinitionResolution struct {
	Definition ToolDefinition
	Origin     ToolSource
	Quality    DefinitionQuality
	Warnings   []string
}

// BuildBuiltinDefinition supplies the mechanical parts of a builtin v2
// definition while leaving semantic authoring facts explicit. Authors should
// provide Purpose, WhenToUse, WhenNotToUse, SideEffects, Examples, and
// FailureModes. The input schema is resolved from the same runtime contract
// used by validation, and OutputSchema defaults to an empty typed-details
// object rather than redefining the canonical ExecutionOutcome envelope.
func BuildBuiltinDefinition(runtime Tool, authored ToolDefinition) ToolDefinition {
	if runtime == nil {
		return authored
	}
	authored.SchemaVersion = ToolDefinitionSchemaVersion
	if strings.TrimSpace(authored.Name) == "" {
		authored.Name = runtime.Name()
	}
	if strings.TrimSpace(authored.DisplayName) == "" {
		authored.DisplayName = authored.Name
	}
	authored.Source = SourceBuiltin
	if strings.TrimSpace(authored.Description) == "" {
		authored.Description = strings.TrimSpace(runtime.Description())
	}
	if !hasInputSchema(authored.InputSchema) {
		authored.InputSchema = ResolveInputSchema(runtime)
	}
	if authored.InputSchema.Type == "object" && authored.InputSchema.AdditionalProperties == nil {
		authored.InputSchema.AdditionalProperties = BoolPtr(false)
	}
	if !hasToolParamSchema(authored.OutputSchema) {
		authored.OutputSchema = NoDetailsOutputSchema()
	}
	if authored.OutputSchema.Type == "object" && authored.OutputSchema.AdditionalProperties == nil {
		authored.OutputSchema.AdditionalProperties = BoolPtr(false)
	}
	authored.Permissions = mergePermissionFacts(authored.Permissions, authored.Annotations)
	return authored
}

// NoDetailsOutputSchema declares that a tool has no typed details beyond the
// canonical ExecutionOutcome envelope.
func NoDetailsOutputSchema() ToolParam {
	return ToolParam{
		Type:                 "object",
		Properties:           map[string]ToolParam{},
		AdditionalProperties: BoolPtr(false),
	}
}

// ResolveDefinition returns a canonical v2 view. Use ResolveDefinitionReport
// when registry quality or compatibility warnings are also needed.
func ResolveDefinition(runtime Tool) ToolDefinition {
	return ResolveDefinitionReport(runtime).Definition
}

// ResolveDefinitionReport applies explicit-v2 > compatibility-adapter >
// legacy-method precedence without inventing examples or failure modes.
func ResolveDefinitionReport(runtime Tool) DefinitionResolution {
	if runtime == nil {
		return DefinitionResolution{
			Definition: ToolDefinition{SchemaVersion: ToolDefinitionSchemaVersion},
			Quality:    DefinitionQualityIncomplete,
			Warnings:   []string{"runtime tool is nil"},
		}
	}

	raw := cloneToolDefinition(runtime.Definition())
	resolved := cloneToolDefinition(raw)
	warnings := make([]string, 0, 8)
	quality := DefinitionQualityStrict

	if raw.SchemaVersion != ToolDefinitionSchemaVersion {
		quality = DefinitionQualityAdapted
		warnings = append(warnings, "definition was adapted from a legacy schema version")
	}
	resolved.SchemaVersion = ToolDefinitionSchemaVersion

	if strings.TrimSpace(resolved.Name) == "" {
		resolved.Name = strings.TrimSpace(runtime.Name())
		quality = DefinitionQualityIncomplete
		warnings = append(warnings, "name was recovered from the runtime method")
	}
	if strings.TrimSpace(resolved.DisplayName) == "" {
		resolved.DisplayName = resolved.Name
		markAdapted(&quality)
	}
	if resolved.Source == "" {
		quality = DefinitionQualityIncomplete
		warnings = append(warnings, "tool origin is not declared")
	}

	legacyDescription := strings.TrimSpace(runtime.Description())
	resolved.Description = strings.TrimSpace(resolved.Description)
	if resolved.Description == "" {
		resolved.Description = legacyDescription
		markAdapted(&quality)
		warnings = append(warnings, "description was recovered from the legacy runtime method")
	}
	resolved.Purpose = strings.TrimSpace(resolved.Purpose)
	if resolved.Purpose == "" {
		resolved.Purpose = resolved.Description
		markAdapted(&quality)
		warnings = append(warnings, "purpose was adapted from the description")
	}

	resolved.WhenToUse = cleanStrings(resolved.WhenToUse)
	if len(resolved.WhenToUse) == 0 {
		resolved.WhenToUse = fallbackWhenToUse(resolved.Source, resolved.Purpose)
		markAdapted(&quality)
		warnings = append(warnings, "when_to_use uses a conservative compatibility boundary")
	}
	resolved.WhenNotToUse = cleanStrings(resolved.WhenNotToUse)
	if len(resolved.WhenNotToUse) == 0 {
		resolved.WhenNotToUse = fallbackWhenNotToUse(resolved.Annotations)
		markAdapted(&quality)
		warnings = append(warnings, "when_not_to_use uses a conservative compatibility boundary")
	}

	if !hasInputSchema(resolved.InputSchema) {
		resolved.InputSchema = ResolveInputSchema(runtime)
		markAdapted(&quality)
		warnings = append(warnings, "input_schema was adapted from the runtime parameter contract")
	}
	resolved.InputSchema = cloneParametersSchema(resolved.InputSchema)
	if !hasToolParamSchema(resolved.OutputSchema) {
		resolved.OutputSchema = NoDetailsOutputSchema()
		markAdapted(&quality)
		warnings = append(warnings, "output_schema defaults to no tool-specific details")
	}
	resolved.OutputSchema = cloneToolParam(resolved.OutputSchema)

	resolved.SideEffects = cloneSideEffects(resolved.SideEffects)
	if len(resolved.SideEffects) == 0 {
		resolved.SideEffects = fallbackSideEffects(resolved.Annotations)
		markAdapted(&quality)
		warnings = append(warnings, "side_effects were conservatively adapted from annotations")
		if hasUnknownSideEffect(resolved.SideEffects) {
			quality = DefinitionQualityIncomplete
		}
	}
	if permissionFactsIncomplete(resolved.Permissions) {
		markAdapted(&quality)
		warnings = append(warnings, "permission facts were adapted from annotations")
	}
	resolved.Permissions = mergePermissionFacts(resolved.Permissions, resolved.Annotations)
	if permissionFactsIncomplete(resolved.Permissions) {
		quality = DefinitionQualityIncomplete
		warnings = append(warnings, "permission scope remains partially unknown")
	}

	resolved.Examples = cloneExamples(resolved.Examples)
	if len(resolved.Examples) == 0 {
		markAdapted(&quality)
		warnings = append(warnings, "no validated invocation examples are declared")
	}
	resolved.FailureModes = cloneFailureModes(resolved.FailureModes)
	if len(resolved.FailureModes) == 0 {
		markAdapted(&quality)
		warnings = append(warnings, "no stable failure modes are declared")
	}
	if quality == DefinitionQualityStrict {
		if err := ValidateResolvedDefinition(resolved); err != nil {
			quality = DefinitionQualityIncomplete
			warnings = append(warnings, "authored v2 definition is invalid: "+err.Error())
		}
	}

	return DefinitionResolution{
		Definition: resolved,
		Origin:     resolved.Source,
		Quality:    quality,
		Warnings:   cleanStrings(warnings),
	}
}

// ModelDescription compiles a model-facing description from the resolved
// capability facts using the default provider budget.
func ModelDescription(definition ToolDefinition) string {
	return ModelDescriptionWithBudget(definition, DefaultModelDescriptionBudget)
}

// ModelDescriptionWithBudget compiles a hard-bounded model description.
// Required boundary sections are retained and share the available budget;
// untrusted external descriptions therefore cannot inflate every request.
func ModelDescriptionWithBudget(definition ToolDefinition, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = DefaultModelDescriptionBudget
	}
	def := cloneToolDefinition(definition)
	summary := strings.TrimSpace(firstNonEmpty(def.Description, def.Purpose, def.Name))
	segments := []descriptionSegment{{text: summary, required: true}}
	if purpose := strings.TrimSpace(def.Purpose); purpose != "" && purpose != summary {
		segments = append(segments, descriptionSegment{text: "Purpose: " + purpose})
	}
	if values := cleanStrings(def.WhenToUse); len(values) > 0 {
		segments = append(segments, descriptionSegment{text: "Use when: " + strings.Join(values, "; ")})
	}
	if values := cleanStrings(def.WhenNotToUse); len(values) > 0 {
		segments = append(segments, descriptionSegment{text: "Do not use when: " + strings.Join(values, "; "), required: true})
	}
	if effects := describeSideEffects(def.SideEffects); effects != "" {
		segments = append(segments, descriptionSegment{text: "Effects: " + effects, required: true})
	}
	if permissions := describePermissionBoundary(def.Permissions); permissions != "" {
		segments = append(segments, descriptionSegment{text: "Permissions: " + permissions, required: true})
	}
	segments = append(segments, descriptionSegment{text: describeReturns(def.OutputSchema)})

	selected := make([]bool, len(segments))
	for i, segment := range segments {
		selected[i] = segment.required
	}
	clipRequiredDescriptionSegments(segments, selected, maxRunes)
	// Usage guidance is the highest-value optional section, followed by purpose
	// and typed return details. Select only complete sections that fit.
	for _, prefix := range []string{"Use when:", "Purpose:", "Returns:"} {
		for i, segment := range segments {
			if selected[i] || !strings.HasPrefix(segment.text, prefix) {
				continue
			}
			selected[i] = true
			if descriptionRuneCount(segments, selected) > maxRunes {
				selected[i] = false
			}
		}
	}
	return truncateDescriptionRunes(joinDescriptionSegments(segments, selected), maxRunes)
}

type descriptionSegment struct {
	text     string
	required bool
}

func joinDescriptionSegments(segments []descriptionSegment, selected []bool) string {
	parts := make([]string, 0, len(segments))
	for i, segment := range segments {
		if i < len(selected) && selected[i] && strings.TrimSpace(segment.text) != "" {
			parts = append(parts, strings.TrimSpace(segment.text))
		}
	}
	return strings.Join(parts, "\n")
}

func descriptionRuneCount(segments []descriptionSegment, selected []bool) int {
	return utf8.RuneCountInString(joinDescriptionSegments(segments, selected))
}

func clipRequiredDescriptionSegments(segments []descriptionSegment, selected []bool, maxRunes int) {
	required := make([]int, 0, len(segments))
	for i, segment := range segments {
		if i < len(selected) && selected[i] && strings.TrimSpace(segment.text) != "" {
			required = append(required, i)
		}
	}
	if len(required) == 0 || descriptionRuneCount(segments, selected) <= maxRunes {
		return
	}
	separatorRunes := len(required) - 1
	available := maxRunes - separatorRunes
	if available < len(required) {
		available = len(required)
	}
	base := available / len(required)
	extra := available % len(required)
	for position, index := range required {
		budget := base
		if position < extra {
			budget++
		}
		segments[index].text = truncateDescriptionRunes(strings.TrimSpace(segments[index].text), budget)
	}
}

func truncateDescriptionRunes(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	if maxRunes == 1 {
		return "…"
	}
	return string(runes[:maxRunes-1]) + "…"
}

func describeReturns(schema ToolParam) string {
	base := "Returns: canonical execution status, summary, changes, environment observations, warnings, errors, and next actions"
	if schema.Type != "object" || len(schema.Properties) == 0 {
		return base + "; no tool-specific details are declared."
	}
	keys := make([]string, 0, len(schema.Properties))
	for key := range schema.Properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return base + "; tool-specific details include " + strings.Join(keys, ", ") + "."
}

func describeSideEffects(effects []ToolSideEffect) string {
	parts := make([]string, 0, len(effects))
	for _, effect := range effects {
		description := strings.TrimSpace(effect.Description)
		if description == "" {
			description = strings.TrimSpace(effect.Kind)
		}
		if description == "" {
			continue
		}
		if reversibility := strings.TrimSpace(effect.Reversibility); reversibility != "" {
			description += " (reversibility: " + reversibility + ")"
		}
		parts = append(parts, description)
	}
	return strings.Join(cleanStrings(parts), "; ")
}

func describePermissionBoundary(spec ToolPermissionSpec) string {
	parts := make([]string, 0, 5)
	if spec.RequiresAuth {
		parts = append(parts, "requires authenticated access")
	}
	if value := strings.TrimSpace(spec.RequiresReview); value != "" {
		parts = append(parts, "review required for "+value)
	}
	if spec.WorkspaceScope != "" && spec.WorkspaceScope != WorkspaceScopeNone {
		parts = append(parts, "workspace scope: "+spec.WorkspaceScope)
	}
	if spec.FileBridgeScope != "" && spec.FileBridgeScope != FileBridgeNone {
		parts = append(parts, "file scope: "+string(spec.FileBridgeScope))
	}
	if spec.NetworkScope != "" && spec.NetworkScope != NetworkScopeNone {
		parts = append(parts, "network scope: "+string(spec.NetworkScope))
	}
	return strings.Join(parts, "; ")
}

func fallbackWhenToUse(source ToolSource, purpose string) []string {
	purpose = strings.TrimSpace(purpose)
	switch source {
	case SourceMCP, SourceConnector:
		return []string{"Use when this declared external capability directly matches the requested operation: " + purpose}
	case SourceSkill:
		return []string{"Use when the active skill delegates an operation that directly matches: " + purpose}
	default:
		return []string{"Use when the requested operation directly matches this purpose: " + purpose}
	}
}

func fallbackWhenNotToUse(annotations ToolAnnotations) []string {
	values := make([]string, 0, 3)
	if annotations.ReadOnly {
		values = append(values, "the task requires modifying state")
	}
	if annotations.Destructive {
		values = append(values, "the destructive operation was not explicitly requested or its required review is incomplete")
	}
	if annotations.OpenWorld {
		values = append(values, "the needed fact is workspace-local and a local read or search capability can answer it")
	}
	if len(values) == 0 {
		values = append(values, "the requested resource or operation does not match the stated purpose")
	}
	return values
}

func fallbackSideEffects(annotations ToolAnnotations) []ToolSideEffect {
	if annotations.ReadOnly {
		return []ToolSideEffect{{
			Kind:          SideEffectNone,
			Description:   "Does not intentionally modify workspace or external state.",
			Reversibility: ReversibilityNotApplicable,
		}}
	}
	description := "May change state, but the legacy definition does not declare the exact resources or reversibility."
	if annotations.Destructive {
		description = "May destructively change state; exact recoverability is not declared by the legacy definition."
	}
	return []ToolSideEffect{{
		Kind:          SideEffectUnknown,
		Description:   description,
		Reversibility: ReversibilityUnknown,
	}}
}

func derivePermissionFacts(annotations ToolAnnotations) ToolPermissionSpec {
	capabilityClass := annotations.CapabilityClass
	if capabilityClass == "" {
		capabilityClass = CapabilityUnknown
	}
	fileScope := annotations.FileBridgeScope
	if fileScope == "" {
		fileScope = FileBridgeNone
	}
	networkScope := annotations.NetworkScope
	if networkScope == "" {
		if annotations.OpenWorld {
			networkScope = NetworkScopeOpen
		} else {
			networkScope = NetworkScopeNone
		}
	}
	workspaceScope := WorkspaceScopeNone
	switch fileScope {
	case FileBridgeTaskScopeRead:
		workspaceScope = WorkspaceScopeRead
	case FileBridgeTaskScopeWrite:
		workspaceScope = WorkspaceScopeWrite
	default:
		if annotations.CapabilityClass == CapabilityArtifactRead {
			workspaceScope = WorkspaceScopeRead
		} else if annotations.CapabilityClass == CapabilityArtifactWrite || annotations.CapabilityClass == CapabilityLocalTransform || annotations.CapabilityClass == CapabilitySystemOperation {
			workspaceScope = WorkspaceScopeWrite
		} else if annotations.CapabilityClass == "" && !annotations.ReadOnly {
			workspaceScope = WorkspaceScopeUncertain
		}
	}
	review := ""
	if annotations.Destructive {
		review = "destructive_operation"
	}
	return ToolPermissionSpec{
		CapabilityClass: capabilityClass,
		WorkspaceScope:  workspaceScope,
		FileBridgeScope: fileScope,
		NetworkScope:    networkScope,
		RequiresAuth:    annotations.RequiresAuth,
		RequiresReview:  review,
	}
}

func mergePermissionFacts(explicit ToolPermissionSpec, annotations ToolAnnotations) ToolPermissionSpec {
	derived := derivePermissionFacts(annotations)
	if explicit.CapabilityClass == "" {
		explicit.CapabilityClass = derived.CapabilityClass
	}
	if strings.TrimSpace(explicit.WorkspaceScope) == "" {
		explicit.WorkspaceScope = derived.WorkspaceScope
	}
	if explicit.FileBridgeScope == "" {
		explicit.FileBridgeScope = derived.FileBridgeScope
	}
	if explicit.NetworkScope == "" {
		explicit.NetworkScope = derived.NetworkScope
	}
	if annotations.RequiresAuth {
		explicit.RequiresAuth = true
	}
	if strings.TrimSpace(explicit.RequiresReview) == "" {
		explicit.RequiresReview = derived.RequiresReview
	}
	return explicit
}

func permissionFactsIncomplete(spec ToolPermissionSpec) bool {
	return spec.CapabilityClass == "" || spec.CapabilityClass == CapabilityUnknown || strings.TrimSpace(spec.WorkspaceScope) == "" || spec.WorkspaceScope == WorkspaceScopeUncertain || spec.FileBridgeScope == "" || spec.NetworkScope == ""
}

func hasUnknownSideEffect(effects []ToolSideEffect) bool {
	for _, effect := range effects {
		if effect.Kind == SideEffectUnknown || effect.Reversibility == ReversibilityUnknown {
			return true
		}
	}
	return false
}

func markAdapted(quality *DefinitionQuality) {
	if *quality == DefinitionQualityStrict {
		*quality = DefinitionQualityAdapted
	}
}

func hasInputSchema(schema ToolParametersSchema) bool {
	return strings.TrimSpace(schema.Type) != "" || strings.TrimSpace(schema.Description) != "" || len(schema.Properties) > 0 || len(schema.Required) > 0 || schema.AdditionalProperties != nil || len(schema.OneOf) > 0
}

func hasToolParamSchema(schema ToolParam) bool {
	return strings.TrimSpace(schema.Type) != "" || strings.TrimSpace(schema.Description) != "" || len(schema.Properties) > 0 || schema.Items != nil || len(schema.Enum) > 0 || len(schema.Required) > 0 || schema.AdditionalProperties != nil || len(schema.OneOf) > 0
}

func cleanStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func cloneToolDefinition(def ToolDefinition) ToolDefinition {
	def.WhenToUse = append([]string(nil), def.WhenToUse...)
	def.WhenNotToUse = append([]string(nil), def.WhenNotToUse...)
	def.InputSchema = cloneParametersSchema(def.InputSchema)
	def.OutputSchema = cloneToolParam(def.OutputSchema)
	def.SideEffects = cloneSideEffects(def.SideEffects)
	def.Examples = cloneExamples(def.Examples)
	def.FailureModes = cloneFailureModes(def.FailureModes)
	return def
}

func cloneParametersSchema(schema ToolParametersSchema) ToolParametersSchema {
	result := schema
	result.Properties = cloneToolParamMap(schema.Properties)
	result.Required = append([]string(nil), schema.Required...)
	result.AdditionalProperties = cloneBool(schema.AdditionalProperties)
	result.OneOf = cloneToolParams(schema.OneOf)
	return result
}

func cloneToolParam(param ToolParam) ToolParam {
	result := param
	result.Properties = cloneToolParamMap(param.Properties)
	if param.Items != nil {
		item := cloneToolParam(*param.Items)
		result.Items = &item
	}
	result.Enum = cloneJSONSlice(param.Enum)
	result.Required = append([]string(nil), param.Required...)
	result.AdditionalProperties = cloneBool(param.AdditionalProperties)
	result.MinItems = cloneInt(param.MinItems)
	result.MaxItems = cloneInt(param.MaxItems)
	result.MinLength = cloneInt(param.MinLength)
	result.MaxLength = cloneInt(param.MaxLength)
	result.Minimum = cloneFloat(param.Minimum)
	result.Maximum = cloneFloat(param.Maximum)
	result.OneOf = cloneToolParams(param.OneOf)
	return result
}

func cloneToolParamMap(values map[string]ToolParam) map[string]ToolParam {
	if values == nil {
		return nil
	}
	result := make(map[string]ToolParam, len(values))
	for key, value := range values {
		result[key] = cloneToolParam(value)
	}
	return result
}

func cloneToolParams(values []ToolParam) []ToolParam {
	if values == nil {
		return nil
	}
	result := make([]ToolParam, len(values))
	for i, value := range values {
		result[i] = cloneToolParam(value)
	}
	return result
}

func cloneSideEffects(values []ToolSideEffect) []ToolSideEffect {
	if values == nil {
		return nil
	}
	result := make([]ToolSideEffect, len(values))
	for i, value := range values {
		result[i] = value
		result[i].Resources = append([]string(nil), value.Resources...)
	}
	return result
}

func cloneExamples(values []ToolExample) []ToolExample {
	if values == nil {
		return nil
	}
	result := make([]ToolExample, len(values))
	for i, value := range values {
		result[i] = value
		if value.Input != nil {
			result[i].Input = make(map[string]any, len(value.Input))
			for key, inputValue := range value.Input {
				result[i].Input[key] = cloneJSONValue(inputValue)
			}
		}
	}
	return result
}

func cloneFailureModes(values []ToolFailureMode) []ToolFailureMode {
	if values == nil {
		return nil
	}
	result := make([]ToolFailureMode, len(values))
	for i, value := range values {
		result[i] = value
		result[i].Recovery = append([]string(nil), value.Recovery...)
	}
	return result
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneJSONSlice(values []any) []any {
	if values == nil {
		return nil
	}
	result := make([]any, len(values))
	for i, value := range values {
		result[i] = cloneJSONValue(value)
	}
	return result
}

func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			result[key] = cloneJSONValue(child)
		}
		return result
	case []any:
		return cloneJSONSlice(typed)
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}
