package tools

import (
	"testing"

	"awaken/src/tool"
)

func TestBuiltInToolNamesAreProviderSafe(t *testing.T) {
	expected := map[string]bool{
		"Bash": false, "Read": false, "Edit": false, "Write": false,
		"Glob": false, "Grep": false, "WebSearch": false,
		"artifact_publish": false, "trash_file": false, "calculator": false,
		"ask_user": false, "dependency_install": false, "git_remote": false,
		"configure_hook": false,
		"subagent":         false,
		"computer_observe": false, "computer_act": false, "computer_verify": false,
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

	for _, retired := range []string{"apply_patch", "artifact_runtime", "read_artifact", "file_read", "read_file", "text_edit", "text_file_create", "list_files", "grep_search", "shell", "web_search", "capability_diagnose"} {
		if _, err := tool.GetTool(retired); err == nil {
			t.Fatalf("retired tool %q is still registered", retired)
		}
	}
}
