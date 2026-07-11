package tool

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ToolSchema is the OpenAI function-calling JSON schema for a tool.
// It is distinct from the annotations-based ToolDefinition in tool.go.
type ToolSchema struct {
	Type     string             `json:"type"`
	Function ToolFunctionSchema `json:"function"`
}

type ToolFunctionSchema struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Parameters  ToolParametersSchema `json:"parameters"`
}

// ToolParametersSchema is a recursive JSON Schema object used for both
// model-facing tool parameters and pre-execution validation.
type ToolParametersSchema struct {
	Type                 string               `json:"type"`
	Description          string               `json:"description,omitempty"`
	Properties           map[string]ToolParam `json:"properties,omitempty"`
	Required             []string             `json:"required,omitempty"`
	AdditionalProperties *bool                `json:"additionalProperties,omitempty"`
	OneOf                []ToolParam          `json:"oneOf,omitempty"`
}

// ToolParam is a recursive JSON Schema node.
type ToolParam struct {
	Type                 string               `json:"type,omitempty"`
	Description          string               `json:"description,omitempty"`
	Properties           map[string]ToolParam `json:"properties,omitempty"`
	Items                *ToolParam           `json:"items,omitempty"`
	Enum                 []any                `json:"enum,omitempty"`
	Required             []string             `json:"required,omitempty"`
	AdditionalProperties *bool                `json:"additionalProperties,omitempty"`
	MinItems             *int                 `json:"minItems,omitempty"`
	MaxItems             *int                 `json:"maxItems,omitempty"`
	MinLength            *int                 `json:"minLength,omitempty"`
	MaxLength            *int                 `json:"maxLength,omitempty"`
	Minimum              *float64             `json:"minimum,omitempty"`
	Maximum              *float64             `json:"maximum,omitempty"`
	OneOf                []ToolParam          `json:"oneOf,omitempty"`
}

// InputSchemaProvider lets a tool supply a recursive JSON Schema that is
// shared by RequestBuilder (model) and ToolRuntime (pre-execution validator).
type InputSchemaProvider interface {
	InputSchema() ToolParametersSchema
}

// LegacySingleValueInput marks the small set of legacy executors that consume
// the value of their sole parameter instead of the canonical JSON argument
// object. New tools should consume the object described by InputSchema.
//
// The marker is deliberately opt-in: model validation and execution otherwise
// share the exact registered v2 schema and the runtime preserves object shape.
type LegacySingleValueInput interface {
	UnwrapSingleParameterInput() bool
}

// ResolveInputSchema returns the recursive schema for a tool, preferring
// InputSchemaProvider when available and falling back to flat Parameters().
func ResolveInputSchema(t Tool) ToolParametersSchema {
	if provider, ok := t.(InputSchemaProvider); ok {
		schema := provider.InputSchema()
		if strings.TrimSpace(schema.Type) == "" {
			schema.Type = "object"
		}
		return schema
	}
	return SchemaFromParameters(t.Parameters())
}

// SchemaFromParameters builds a shallow object schema from flat ToolParameter lists.
func SchemaFromParameters(params []ToolParameter) ToolParametersSchema {
	properties := make(map[string]ToolParam, len(params))
	required := make([]string, 0)
	for _, p := range params {
		properties[p.Name] = ToolParam{
			Type:        p.Type,
			Description: p.Description,
		}
		if p.Required {
			required = append(required, p.Name)
		}
	}
	return ToolParametersSchema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
}

// SchemaAsMap converts a ToolParametersSchema into a generic JSON-object map
// suitable for Anthropic input_schema and similar transports.
func SchemaAsMap(schema ToolParametersSchema) map[string]any {
	raw, err := json.Marshal(schema)
	if err != nil {
		return map[string]any{"type": "object"}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{"type": "object"}
	}
	return out
}

// ValidateInputAgainstSchema validates JSON tool arguments against schema.
// Errors use JSON paths such as:
//
//	$.slides[2].images[0].width expected number, got string
func ValidateInputAgainstSchema(arguments string, schema ToolParametersSchema) error {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		trimmed = "{}"
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	obj, ok := decoded.(map[string]any)
	if !ok || decoded == nil {
		return fmt.Errorf("arguments must be a JSON object")
	}
	root := ToolParam{
		Type:                 firstNonEmpty(schema.Type, "object"),
		Description:          schema.Description,
		Properties:           schema.Properties,
		Required:             schema.Required,
		AdditionalProperties: schema.AdditionalProperties,
		OneOf:                schema.OneOf,
	}
	return validateValue("$", obj, root)
}

func validateValue(path string, value any, schema ToolParam) error {
	if len(schema.OneOf) > 0 {
		return validateOneOf(path, value, schema.OneOf)
	}
	if len(schema.Enum) > 0 {
		if !enumContains(schema.Enum, value) {
			return fmt.Errorf("%s expected one of %s, got %s", path, formatEnum(schema.Enum), describeJSONType(value))
		}
	}
	if schema.Type == "" {
		if len(schema.Properties) > 0 {
			schema.Type = "object"
		} else if schema.Items != nil {
			schema.Type = "array"
		}
	}
	switch schema.Type {
	case "", "any":
		return nil
	case "object":
		return validateObject(path, value, schema)
	case "array":
		return validateArray(path, value, schema)
	case "string":
		return validateString(path, value, schema)
	case "number":
		return validateNumber(path, value, schema, false)
	case "integer":
		return validateNumber(path, value, schema, true)
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s expected boolean, got %s", path, describeJSONType(value))
		}
		return nil
	case "null":
		if value != nil {
			return fmt.Errorf("%s expected null, got %s", path, describeJSONType(value))
		}
		return nil
	default:
		return fmt.Errorf("%s has unsupported schema type %q", path, schema.Type)
	}
}

func validateOneOf(path string, value any, options []ToolParam) error {
	var matched int
	var lastErr error
	for _, option := range options {
		if err := validateValue(path, value, option); err != nil {
			lastErr = err
			continue
		}
		matched++
	}
	if matched == 1 {
		return nil
	}
	if matched == 0 {
		if lastErr != nil {
			return fmt.Errorf("%s did not match any oneOf schema: %v", path, lastErr)
		}
		return fmt.Errorf("%s did not match any oneOf schema", path)
	}
	return fmt.Errorf("%s matched %d oneOf schemas, expected exactly 1", path, matched)
}

func validateObject(path string, value any, schema ToolParam) error {
	obj, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s expected object, got %s", path, describeJSONType(value))
	}
	for _, key := range schema.Required {
		if _, exists := obj[key]; !exists {
			return fmt.Errorf("%s missing required property %q", joinPath(path, key), key)
		}
	}
	for key, child := range schema.Properties {
		childValue, exists := obj[key]
		if !exists {
			continue
		}
		if err := validateValue(joinPath(path, key), childValue, child); err != nil {
			return err
		}
	}
	if schema.AdditionalProperties != nil && !*schema.AdditionalProperties {
		known := make(map[string]struct{}, len(schema.Properties))
		for key := range schema.Properties {
			known[key] = struct{}{}
		}
		extras := make([]string, 0)
		for key := range obj {
			if _, ok := known[key]; !ok {
				extras = append(extras, key)
			}
		}
		sort.Strings(extras)
		if len(extras) > 0 {
			return fmt.Errorf("%s has unexpected properties: %s", path, strings.Join(extras, ", "))
		}
	}
	return nil
}

func validateArray(path string, value any, schema ToolParam) error {
	arr, ok := value.([]any)
	if !ok {
		return fmt.Errorf("%s expected array, got %s", path, describeJSONType(value))
	}
	if schema.MinItems != nil && len(arr) < *schema.MinItems {
		return fmt.Errorf("%s expected at least %d items, got %d", path, *schema.MinItems, len(arr))
	}
	if schema.MaxItems != nil && len(arr) > *schema.MaxItems {
		return fmt.Errorf("%s expected at most %d items, got %d", path, *schema.MaxItems, len(arr))
	}
	if schema.Items == nil {
		return nil
	}
	for i, item := range arr {
		if err := validateValue(fmt.Sprintf("%s[%d]", path, i), item, *schema.Items); err != nil {
			return err
		}
	}
	return nil
}

func validateString(path string, value any, schema ToolParam) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("%s expected string, got %s", path, describeJSONType(value))
	}
	length := utf8.RuneCountInString(str)
	if schema.MinLength != nil && length < *schema.MinLength {
		return fmt.Errorf("%s expected minLength %d, got %d", path, *schema.MinLength, length)
	}
	if schema.MaxLength != nil && length > *schema.MaxLength {
		return fmt.Errorf("%s expected maxLength %d, got %d", path, *schema.MaxLength, length)
	}
	return nil
}

func validateNumber(path string, value any, schema ToolParam, integerOnly bool) error {
	num, ok := asFloat(value)
	if !ok {
		want := "number"
		if integerOnly {
			want = "integer"
		}
		return fmt.Errorf("%s expected %s, got %s", path, want, describeJSONType(value))
	}
	if integerOnly && math.Mod(num, 1) != 0 {
		return fmt.Errorf("%s expected integer, got number", path)
	}
	if schema.Minimum != nil && num < *schema.Minimum {
		return fmt.Errorf("%s expected minimum %v, got %v", path, *schema.Minimum, num)
	}
	if schema.Maximum != nil && num > *schema.Maximum {
		return fmt.Errorf("%s expected maximum %v, got %v", path, *schema.Maximum, num)
	}
	return nil
}

func asFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		v, err := typed.Float64()
		return v, err == nil
	default:
		return 0, false
	}
}

func enumContains(enum []any, value any) bool {
	for _, candidate := range enum {
		if jsonEqual(candidate, value) {
			return true
		}
	}
	return false
}

func jsonEqual(left, right any) bool {
	leftRaw, err := json.Marshal(left)
	if err != nil {
		return false
	}
	rightRaw, err := json.Marshal(right)
	if err != nil {
		return false
	}
	return string(leftRaw) == string(rightRaw)
}

func formatEnum(enum []any) string {
	parts := make([]string, 0, len(enum))
	for _, item := range enum {
		raw, err := json.Marshal(item)
		if err != nil {
			parts = append(parts, fmt.Sprintf("%v", item))
			continue
		}
		parts = append(parts, string(raw))
	}
	return strings.Join(parts, ", ")
}

func describeJSONType(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case float64, float32, int, int64, json.Number:
		if integer, ok := asFloat(typed); ok && math.Mod(integer, 1) == 0 {
			return "integer"
		}
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return fmt.Sprintf("%T", value)
	}
}

func joinPath(base, key string) string {
	if base == "" || base == "$" {
		return "$." + key
	}
	if isIndexKey(key) {
		return base + key
	}
	return base + "." + key
}

func isIndexKey(key string) bool {
	return strings.HasPrefix(key, "[") && strings.HasSuffix(key, "]")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// IntPtr returns a pointer to v for schema constraints.
func IntPtr(v int) *int { return &v }

// FloatPtr returns a pointer to v for schema constraints.
func FloatPtr(v float64) *float64 { return &v }

// BoolPtr returns a pointer to v for schema flags.
func BoolPtr(v bool) *bool { return &v }

// FormatJSONPath is exported for tests that assert path formatting helpers.
func FormatJSONPath(parts ...string) string {
	path := "$"
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "[") {
			path += part
			continue
		}
		if _, err := strconv.Atoi(part); err == nil {
			path += "[" + part + "]"
			continue
		}
		path = joinPath(path, part)
	}
	return path
}
