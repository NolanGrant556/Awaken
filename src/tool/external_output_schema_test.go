package tool

import "testing"

func TestAdaptExternalDetailsOutputSchemaPreservesOrdinaryObject(t *testing.T) {
	additionalProperties := false
	schema := ToolParam{
		Type: "object",
		Properties: map[string]ToolParam{
			"record_id": {Type: "string"},
		},
		Required:             []string{"record_id"},
		AdditionalProperties: &additionalProperties,
	}

	adapted := AdaptExternalDetailsOutputSchema(schema)
	if adapted.WrapperKey != "" || len(adapted.Warnings) != 0 {
		t.Fatalf("ordinary details schema should not be wrapped: %+v", adapted)
	}
	if adapted.Schema.Properties["record_id"].Type != "string" {
		t.Fatalf("expected property to be preserved: %+v", adapted.Schema)
	}

	delete(adapted.Schema.Properties, "record_id")
	if _, exists := schema.Properties["record_id"]; !exists {
		t.Fatal("adaptation must not mutate the caller's schema")
	}
}

func TestAdaptExternalDetailsOutputSchemaWrapsCanonicalConflicts(t *testing.T) {
	schema := ToolParam{
		Type: "object",
		Properties: map[string]ToolParam{
			"status":    {Type: "string"},
			"record_id": {Type: "string"},
		},
		Required: []string{"status", "record_id"},
	}

	adapted := AdaptExternalDetailsOutputSchema(schema)
	if adapted.WrapperKey != externalOutputPayloadKey || len(adapted.Warnings) == 0 {
		t.Fatalf("expected conflict-safe wrapper: %+v", adapted)
	}
	payload, exists := adapted.Schema.Properties[externalOutputPayloadKey]
	if !exists || payload.Properties["status"].Type != "string" || payload.Properties["record_id"].Type != "string" {
		t.Fatalf("expected complete external payload schema to be preserved: %+v", adapted.Schema)
	}
	if err := validateOutputSchemaForDefinition(adapted.Schema); err != nil {
		t.Fatalf("wrapped external schema must satisfy details-only boundary: %v", err)
	}
}
