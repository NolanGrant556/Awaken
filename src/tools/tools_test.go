package tools

import (
	"testing"

	"awaken/src/tool"
)

func TestBuiltInToolNamesAreProviderSafe(t *testing.T) {
	expected := map[string]bool{
		"apply_patch":          false,
		"browser_navigate":     false,
		"browser_open":         false,
		"browser_page_summary": false,
		"calculator":           false,
		"grep_search":          false,
		"list_files":           false,
		"read_file":            false,
		"shell":                false,
		"todo_write":           false,
		"web_fetch":            false,
		"web_search":           false,
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

func TestTodoWriteIsRegisteredReadOnly(t *testing.T) {
	candidate, err := tool.GetTool("todo_write")
	if err != nil {
		t.Fatalf("expected todo_write to be registered: %v", err)
	}
	readOnly, ok := candidate.(tool.ReadOnlyTool)
	if !ok {
		t.Fatalf("expected todo_write to implement ReadOnlyTool")
	}
	if !readOnly.IsReadOnly() {
		t.Fatalf("expected todo_write to be read-only")
	}
}
