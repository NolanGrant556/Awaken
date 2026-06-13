package tool

import (
	"regexp"
	"testing"
)

func TestBuiltInToolNamesAreProviderSafe(t *testing.T) {
	safeName := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	validJSONSchemaTypes := map[string]bool{
		"string":  true,
		"number":  true,
		"integer": true,
		"boolean": true,
		"object":  true,
		"array":   true,
	}
	expected := map[string]bool{
		"calculator": false,
		"read_file":  false,
		"write_file": false,
		"web_search": false,
	}

	for _, tool := range ListTools() {
		name := tool.Name()
		if !safeName.MatchString(name) {
			t.Fatalf("tool name %q is not provider-safe", name)
		}
		if _, ok := expected[name]; ok {
			expected[name] = true
		}
		for _, param := range tool.Parameters() {
			if !validJSONSchemaTypes[param.Type] {
				t.Fatalf("tool %q parameter %q has invalid JSON Schema type %q", name, param.Name, param.Type)
			}
		}
	}

	for name, found := range expected {
		if !found {
			t.Fatalf("expected built-in tool %q to be registered", name)
		}
	}
}
