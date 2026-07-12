package tool

import (
	"context"
	"strings"
	"testing"
)

func TestValidateInputAgainstSchemaNestedPath(t *testing.T) {
	schema := ToolParametersSchema{
		Type: "object",
		Properties: map[string]ToolParam{
			"slides": {
				Type: "array",
				Items: &ToolParam{
					Type: "object",
					Properties: map[string]ToolParam{
						"images": {
							Type: "array",
							Items: &ToolParam{
								Type: "object",
								Properties: map[string]ToolParam{
									"width": {Type: "number"},
								},
							},
						},
					},
				},
			},
		},
	}
	err := ValidateInputAgainstSchema(`{"slides":[{},{},{"images":[{"width":"bad"}]}]}`, schema)
	if err == nil {
		t.Fatal("expected validation error")
	}
	got := err.Error()
	want := "$.slides[2].images[0].width expected number, got string"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestValidateInputAgainstSchemaEnumRequiredAndAdditionalProperties(t *testing.T) {
	schema := ToolParametersSchema{
		Type: "object",
		Properties: map[string]ToolParam{
			"layout": {
				Type: "string",
				Enum: []any{"title", "bullets"},
			},
			"title": {Type: "string", MinLength: IntPtr(1)},
		},
		Required:             []string{"layout"},
		AdditionalProperties: BoolPtr(false),
	}
	if err := ValidateInputAgainstSchema(`{"layout":"title","title":"Hi"}`, schema); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := ValidateInputAgainstSchema(`{"title":"Hi"}`, schema); err == nil || !strings.Contains(err.Error(), "missing required property") {
		t.Fatalf("expected required error, got %v", err)
	}
	if err := ValidateInputAgainstSchema(`{"layout":"unknown"}`, schema); err == nil || !strings.Contains(err.Error(), "expected one of") {
		t.Fatalf("expected enum error, got %v", err)
	}
	if err := ValidateInputAgainstSchema(`{"layout":"title","extra":1}`, schema); err == nil || !strings.Contains(err.Error(), "unexpected properties") {
		t.Fatalf("expected additionalProperties error, got %v", err)
	}
}

func TestValidateInputAgainstSchemaOneOfAndBounds(t *testing.T) {
	schema := ToolParametersSchema{
		Type: "object",
		Properties: map[string]ToolParam{
			"count": {Type: "integer", Minimum: FloatPtr(1), Maximum: FloatPtr(3)},
			"items": {Type: "array", MinItems: IntPtr(1), MaxItems: IntPtr(2), Items: &ToolParam{Type: "string"}},
			"value": {
				OneOf: []ToolParam{
					{Type: "string"},
					{Type: "number"},
				},
			},
		},
	}
	if err := ValidateInputAgainstSchema(`{"count":2,"items":["a"],"value":"x"}`, schema); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := ValidateInputAgainstSchema(`{"count":0}`, schema); err == nil || !strings.Contains(err.Error(), "minimum") {
		t.Fatalf("expected minimum error, got %v", err)
	}
	if err := ValidateInputAgainstSchema(`{"items":[]}`, schema); err == nil || !strings.Contains(err.Error(), "at least 1 items") {
		t.Fatalf("expected minItems error, got %v", err)
	}
	if err := ValidateInputAgainstSchema(`{"value":true}`, schema); err == nil || !strings.Contains(err.Error(), "oneOf") {
		t.Fatalf("expected oneOf error, got %v", err)
	}
}

func TestSchemaFromParametersAndResolve(t *testing.T) {
	flat := SchemaFromParameters([]ToolParameter{
		{Name: "path", Type: "string", Required: true, Description: "file"},
	})
	if flat.Properties["path"].Type != "string" || len(flat.Required) != 1 {
		t.Fatalf("unexpected flat schema: %+v", flat)
	}
	provider := schemaProviderTool{schema: ToolParametersSchema{
		Type: "object",
		Properties: map[string]ToolParam{
			"slides": {Type: "array", Items: &ToolParam{Type: "object"}},
		},
	}}
	resolved := ResolveInputSchema(provider)
	if resolved.Properties["slides"].Items == nil {
		t.Fatalf("expected recursive schema from provider")
	}
}

type schemaProviderTool struct {
	schema ToolParametersSchema
}

func (schemaProviderTool) Name() string        { return "demo" }
func (schemaProviderTool) Description() string { return "demo" }
func (schemaProviderTool) Parameters() []ToolParameter {
	return []ToolParameter{{Name: "slides", Type: "array"}}
}
func (schemaProviderTool) Definition() ToolDefinition {
	return ToolDefinition{Name: "demo", Description: "demo"}
}
func (schemaProviderTool) Execute(context.Context, string) (string, error) { return "", nil }
func (t schemaProviderTool) InputSchema() ToolParametersSchema             { return t.schema }
