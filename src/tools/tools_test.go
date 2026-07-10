package tools

import (
	"testing"

	"awaken/src/tool"
)

func TestBuiltInToolNamesAreProviderSafe(t *testing.T) {
	expected := map[string]bool{
		"apply_patch":      false,
		"calculator":       false,
		"grep_search":      false,
		"list_files":       false,
		"read_file":        false,
		"shell":            false,
		"text_edit":        false,
		"text_file_create": false,
		"web_search":       false,
	}

	for _, candidate := range tool.ListTools() {
		name := candidate.Name()
		if err := tool.ValidateTool(candidate); err != nil {
			t.Fatalf("built-in tool %q failed validation: %v", name, err)
		}
		if _, ok := expected[name]; ok {
			expected[name] = true
		}
	}

	for name, found := range expected {
		if !found {
			t.Fatalf("expected built-in tool %q to be registered", name)
		}
	}

	for _, candidate := range tool.ListTools() {
		if candidate.Name() == "write_file" {
			t.Fatal("write_file should not be registered")
		}
	}
}
