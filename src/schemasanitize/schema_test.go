package schemasanitize

import (
	"reflect"
	"testing"
)

func TestSanitizeToolParametersFallback(t *testing.T) {
	for _, tc := range []Tool{
		{Name: "missing"},
		{Name: "malformed", Parameters: "object"},
	} {
		got := SanitizeTool(tc)
		params, ok := got.Parameters.(map[string]any)
		if !ok {
			t.Fatalf("expected parameters map, got %T", got.Parameters)
		}
		assertObjectWithEmptyProperties(t, params)
	}
}

func TestSanitizeSchemaBareString(t *testing.T) {
	objectSchema, ok := SanitizeSchema("object").(map[string]any)
	if !ok {
		t.Fatalf("expected object schema map")
	}
	assertObjectWithEmptyProperties(t, objectSchema)

	stringSchema := SanitizeSchema("string")
	want := map[string]any{"type": "string"}
	if !reflect.DeepEqual(stringSchema, want) {
		t.Fatalf("unexpected string schema: got %#v want %#v", stringSchema, want)
	}
}

func TestSanitizeSchemaTypeArrayNullable(t *testing.T) {
	got := SanitizeSchema(map[string]any{
		"type": []any{"string", "null"},
	})
	want := map[string]any{
		"type":     "string",
		"nullable": true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected nullable type normalization: got %#v want %#v", got, want)
	}
}

func TestStripNullableUnionsPreservesMetadata(t *testing.T) {
	got := SanitizeSchema(map[string]any{
		"anyOf": []any{
			map[string]any{"type": "string"},
			map[string]any{"type": "null"},
		},
		"title":       "Label",
		"description": "Optional label",
		"default":     nil,
		"examples":    []any{"alpha"},
	})
	want := map[string]any{
		"type":        "string",
		"nullable":    true,
		"title":       "Label",
		"description": "Optional label",
		"default":     nil,
		"examples":    []any{"alpha"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected collapsed nullable union: got %#v want %#v", got, want)
	}

	gotWithoutHint := StripNullableUnions(map[string]any{
		"oneOf": []any{
			map[string]any{"type": "number"},
			map[string]any{"type": "null"},
		},
	}, false)
	wantWithoutHint := map[string]any{"type": "number"}
	if !reflect.DeepEqual(gotWithoutHint, wantWithoutHint) {
		t.Fatalf("unexpected collapse without hint: got %#v want %#v", gotWithoutHint, wantWithoutHint)
	}
}

func TestSanitizeSchemaObjectMissingProperties(t *testing.T) {
	got, ok := SanitizeSchema(map[string]any{"type": "object"}).(map[string]any)
	if !ok {
		t.Fatalf("expected object schema map")
	}
	assertObjectWithEmptyProperties(t, got)
}

func TestSanitizeSchemaAcceptsStringMaps(t *testing.T) {
	got, ok := SanitizeSchema(map[string]string{
		"type":        "string",
		"description": "Label",
	}).(map[string]any)
	if !ok {
		t.Fatalf("expected schema map")
	}
	want := map[string]any{
		"type":        "string",
		"description": "Label",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected string map schema: got %#v want %#v", got, want)
	}
}

func TestSanitizeSchemaRequiredPruned(t *testing.T) {
	got := SanitizeSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
		"required": []any{"path", "missing", 12},
	})
	want := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
		"required": []string{"path"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected required pruning: got %#v want %#v", got, want)
	}

	gotAllPruned := SanitizeSchema(map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []any{"missing"},
	}).(map[string]any)
	if _, exists := gotAllPruned["required"]; exists {
		t.Fatalf("expected empty required list to be removed, got %#v", gotAllPruned["required"])
	}
}

func TestSanitizeSchemaEnumExamplesUntouched(t *testing.T) {
	enumValue := []any{"object", map[string]any{"type": "object"}}
	examplesValue := []any{map[string]any{"type": "object"}, "string"}
	got := SanitizeSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"mode": map[string]any{
				"type": "string",
				"enum": enumValue,
			},
			"sample": map[string]any{
				"type":     "array",
				"examples": examplesValue,
			},
		},
	}).(map[string]any)

	props := got["properties"].(map[string]any)
	mode := props["mode"].(map[string]any)
	if !reflect.DeepEqual(mode["enum"], enumValue) {
		t.Fatalf("enum was treated as schema: got %#v want %#v", mode["enum"], enumValue)
	}
	sample := props["sample"].(map[string]any)
	if !reflect.DeepEqual(sample["examples"], examplesValue) {
		t.Fatalf("examples were treated as schema: got %#v want %#v", sample["examples"], examplesValue)
	}
}

func TestSanitizeSchemaDeepCopyNoInputMutation(t *testing.T) {
	input := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"child": map[string]any{"type": "object"},
		},
	}
	original := deepCopy(input)

	got := SanitizeSchema(input).(map[string]any)
	props := got["properties"].(map[string]any)
	child := props["child"].(map[string]any)
	child["description"] = "changed after sanitize"

	if !reflect.DeepEqual(input, original) {
		t.Fatalf("input mutated: got %#v want %#v", input, original)
	}
	inputChild := input["properties"].(map[string]any)["child"].(map[string]any)
	if _, exists := inputChild["properties"]; exists {
		t.Fatalf("input child unexpectedly received properties: %#v", inputChild)
	}
}

func TestSanitizeSchemaNestedAdditionalPropertiesAndItems(t *testing.T) {
	got := SanitizeSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"list": map[string]any{
				"type":  "array",
				"items": "object",
			},
			"dict": map[string]any{
				"type":                 "object",
				"additionalProperties": "object",
			},
		},
	}).(map[string]any)

	props := got["properties"].(map[string]any)
	list := props["list"].(map[string]any)
	items := list["items"].(map[string]any)
	assertObjectWithEmptyProperties(t, items)

	dict := props["dict"].(map[string]any)
	additional := dict["additionalProperties"].(map[string]any)
	assertObjectWithEmptyProperties(t, additional)
}

func assertObjectWithEmptyProperties(t *testing.T, schema map[string]any) {
	t.Helper()
	if schema["type"] != "object" {
		t.Fatalf("expected object type, got %#v", schema["type"])
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map, got %T", schema["properties"])
	}
	if len(properties) != 0 {
		t.Fatalf("expected empty properties, got %#v", properties)
	}
}
