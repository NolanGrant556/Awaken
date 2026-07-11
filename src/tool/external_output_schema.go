package tool

import "strings"

const (
	externalOutputPayloadKey = "mcp_payload"
	externalOutputValueKey   = "mcp_value"
)

// ExternalOutputSchemaAdaptation is the compatibility-safe representation of
// an externally authored output schema inside ExecutionOutcome.Details.
// WrapperKey is non-empty when the external root must be nested to avoid
// colliding with canonical envelope fields or to adapt a non-object root.
type ExternalOutputSchemaAdaptation struct {
	Schema     ToolParam
	WrapperKey string
	Warnings   []string
}

// AdaptExternalDetailsOutputSchema preserves an externally authored schema as
// a details-only contract. Canonical ExecutionOutcome field names at the
// details root are ambiguous to model consumers, so the complete external
// payload is nested rather than dropping or renaming individual properties.
// Invalid MCP non-object roots are handled the same way for compatibility.
func AdaptExternalDetailsOutputSchema(schema ToolParam) ExternalOutputSchemaAdaptation {
	adapted := cloneToolParam(schema)
	normalizeExternalOutputSchema(&adapted)
	if strings.TrimSpace(adapted.Type) == "" && len(adapted.Properties) > 0 {
		adapted.Type = "object"
	}

	if adapted.Type != "object" {
		return wrapExternalOutputSchema(
			externalOutputValueKey,
			adapted,
			"external output schema root is not an object; structured output is nested under mcp_value",
		)
	}

	conflicts := make([]string, 0)
	for name := range adapted.Properties {
		if _, conflict := canonicalOutcomeFields[name]; conflict {
			conflicts = append(conflicts, name)
		}
	}
	if len(conflicts) == 0 {
		return ExternalOutputSchemaAdaptation{Schema: adapted}
	}

	return wrapExternalOutputSchema(
		externalOutputPayloadKey,
		adapted,
		"external output schema duplicates canonical ExecutionOutcome fields; structured output is nested under mcp_payload",
	)
}

func normalizeExternalOutputSchema(schema *ToolParam) {
	if schema == nil {
		return
	}
	if strings.TrimSpace(schema.Type) == "" {
		switch {
		case len(schema.Properties) > 0:
			schema.Type = "object"
		case schema.Items != nil:
			schema.Type = "array"
		}
	}
	if schema.Type == "object" && schema.AdditionalProperties == nil {
		// JSON Schema defaults additionalProperties to true. Preserve that
		// meaning explicitly because Awaken's authored-schema validator requires
		// every object node to state the policy.
		schema.AdditionalProperties = BoolPtr(true)
	}
	for name, property := range schema.Properties {
		normalizeExternalOutputSchema(&property)
		schema.Properties[name] = property
	}
	if schema.Items != nil {
		normalizeExternalOutputSchema(schema.Items)
	}
	for i := range schema.OneOf {
		normalizeExternalOutputSchema(&schema.OneOf[i])
	}
}

func wrapExternalOutputSchema(wrapperKey string, schema ToolParam, warning string) ExternalOutputSchemaAdaptation {
	return ExternalOutputSchemaAdaptation{
		Schema: ToolParam{
			Type: "object",
			Properties: map[string]ToolParam{
				wrapperKey: schema,
			},
			Required:             []string{wrapperKey},
			AdditionalProperties: BoolPtr(false),
		},
		WrapperKey: wrapperKey,
		Warnings:   []string{warning},
	}
}
