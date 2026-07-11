package tools

import (
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestBuiltinToolCatalogHasResolvedV2Contracts(t *testing.T) {
	registered := tool.ListRegisteredTools()
	if len(registered) == 0 {
		t.Fatal("expected builtin tool catalog")
	}
	qualityCounts := map[tool.DefinitionQuality]int{}
	for _, entry := range registered {
		if entry.Origin != tool.SourceBuiltin {
			continue
		}
		def := entry.Definition
		qualityCounts[entry.Quality]++
		if def.SchemaVersion != tool.ToolDefinitionSchemaVersion || def.Name == "" || def.Purpose == "" {
			t.Errorf("tool %s lacks resolved v2 identity/purpose: %+v", def.Name, def)
		}
		if len(def.WhenToUse) == 0 || len(def.WhenNotToUse) == 0 || len(def.SideEffects) == 0 {
			t.Errorf("tool %s lacks selection/effect boundaries", def.Name)
		}
		if def.InputSchema.Type != "object" || def.OutputSchema.Type != "object" {
			t.Errorf("tool %s lacks object input/details schema", def.Name)
		}
		if def.Permissions.CapabilityClass == "" || def.Permissions.WorkspaceScope == "" || def.Permissions.FileBridgeScope == "" || def.Permissions.NetworkScope == "" {
			t.Errorf("tool %s has incomplete permission facts: %+v", def.Name, def.Permissions)
		}
		if entry.Quality == tool.DefinitionQualityIncomplete {
			t.Errorf("builtin tool %s remains definition quality incomplete: %v", def.Name, entry.Warnings)
		}
		if entry.Quality != tool.DefinitionQualityStrict {
			t.Errorf("builtin tool %s is not strict: quality=%s warnings=%v", def.Name, entry.Quality, entry.Warnings)
		}
		if err := tool.ValidateToolWithMode(entry.Runtime, tool.DefinitionValidationStrictBuiltin); err != nil {
			t.Errorf("builtin tool %s fails the registration-time strict contract: %v", def.Name, err)
		}
		if len(def.Examples) == 0 || len(def.FailureModes) == 0 {
			t.Errorf("builtin tool %s lacks examples or failure modes", def.Name)
		}
		if def.Name != "ask_user" {
			if _, ok := entry.Runtime.(tool.OutcomeTool); !ok {
				t.Errorf("builtin tool %s still lacks OutcomeTool", def.Name)
			}
		}
	}
	t.Logf("builtin definition quality counts: strict=%d adapted=%d incomplete=%d",
		qualityCounts[tool.DefinitionQualityStrict], qualityCounts[tool.DefinitionQualityAdapted], qualityCounts[tool.DefinitionQualityIncomplete])
}

func TestCoreModelDescriptionsDeclareNeighborToolBoundaries(t *testing.T) {
	cases := map[string][]string{
		"read_file":        {"read_artifact", "list_files", "grep_search"},
		"read_artifact":    {"read_file"},
		"list_files":       {"grep_search", "read_file"},
		"grep_search":      {"list_files", "web_search"},
		"text_edit":        {"read_file", "text_file_create", "apply_patch", "trash_file"},
		"text_file_create": {"text_edit", "trash_file"},
		"apply_patch":      {"text_edit", "text_file_create", "trash_file"},
		"trash_file":       {"rm", "rmdir", "unlink"},
		"shell":            {"read_file", "grep_search", "text_edit", "trash_file"},
	}
	for name, boundaries := range cases {
		entry, err := tool.GetRegisteredTool(name)
		if err != nil {
			t.Fatalf("get %s: %v", name, err)
		}
		description := tool.ModelDescription(entry.Definition)
		if !strings.Contains(description, "Do not use when:") || !strings.Contains(description, "Effects:") || !strings.Contains(description, "Returns:") {
			t.Errorf("%s description lacks compiled selection/result sections:\n%s", name, description)
		}
		for _, boundary := range boundaries {
			if !strings.Contains(description, boundary) {
				t.Errorf("%s model description lacks neighbor boundary %q:\n%s", name, boundary, description)
			}
		}
	}
}

func TestBuiltinFailureModeRetryabilityMatchesRuntimeSemantics(t *testing.T) {
	tests := []struct {
		toolName  string
		code      string
		retryable bool
	}{
		{toolName: "artifact_publish", code: "artifact_publish_file_unavailable", retryable: true},
		{toolName: "artifact_publish", code: "artifact_publish_permission_denied", retryable: false},
		{toolName: "artifact_publish", code: "artifact_publish_not_regular_file", retryable: false},
		{toolName: "presentation_create", code: tool.ErrorCodeNeedsRuntime, retryable: false},
		{toolName: "presentation_create", code: tool.ErrorCodeToolExecutionFailed, retryable: false},
		{toolName: "read_artifact", code: "read_artifact_runtime_failed", retryable: false},
		{toolName: "trash_file", code: tool.ErrorCodeToolExecutionFailed, retryable: false},
		{toolName: "shell", code: tool.ErrorCodeProcessSpawnFailed, retryable: true},
	}
	for _, test := range tests {
		entry, err := tool.GetRegisteredTool(test.toolName)
		if err != nil {
			t.Fatalf("get %s: %v", test.toolName, err)
		}
		mode, ok := failureModeByCode(entry.Definition.FailureModes, test.code)
		if !ok {
			t.Errorf("%s does not declare failure mode %s", test.toolName, test.code)
			continue
		}
		if mode.Retryable != test.retryable {
			t.Errorf("%s/%s retryable=%v, want %v", test.toolName, test.code, mode.Retryable, test.retryable)
		}
	}
}

func failureModeByCode(modes []tool.ToolFailureMode, code string) (tool.ToolFailureMode, bool) {
	for _, mode := range modes {
		if mode.Code == code {
			return mode, true
		}
	}
	return tool.ToolFailureMode{}, false
}
