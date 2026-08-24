package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"awaken/src/tool"
	_ "awaken/src/tools"
)

func TestToolsEndpointReturnsResolvedDefinitionV2(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/tools", nil)
	rec := httptest.NewRecorder()
	handleListTools(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var tools []toolInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &tools); err != nil {
		t.Fatalf("decode tool catalog: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected registered builtin tools")
	}
	var rawTools []map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &rawTools); err != nil {
		t.Fatalf("decode raw tool catalog: %v", err)
	}
	for _, raw := range rawTools {
		for _, field := range []string{"examples", "failureModes", "modelDescription"} {
			if _, present := raw[field]; !present {
				t.Fatalf("tool catalog omitted required field %q: %s", field, rec.Body.String())
			}
		}
	}
	for _, info := range tools {
		if info.SchemaVersion != tool.ToolDefinitionSchemaVersion {
			t.Fatalf("tool %s returned schema version %d", info.Name, info.SchemaVersion)
		}
		if info.Name == "" || info.Source == "" || info.Purpose == "" || len(info.WhenToUse) == 0 || len(info.WhenNotToUse) == 0 {
			t.Fatalf("tool %s is missing resolved selection facts: %+v", info.Name, info)
		}
		if info.ModelDescription == "" || len([]rune(info.ModelDescription)) > tool.DefaultModelDescriptionBudget {
			t.Fatalf("tool %s returned invalid bounded model description: %q", info.Name, info.ModelDescription)
		}
		if info.InputSchema.Type != "object" || info.OutputSchema.Type != "object" {
			t.Fatalf("tool %s is missing executable schemas: %+v / %+v", info.Name, info.InputSchema, info.OutputSchema)
		}
		if len(info.SideEffects) == 0 || info.Permissions.FileBridgeScope == "" || info.Permissions.NetworkScope == "" {
			t.Fatalf("tool %s is missing safety facts: %+v / %+v", info.Name, info.SideEffects, info.Permissions)
		}
		switch info.DefinitionQuality {
		case string(tool.DefinitionQualityStrict), string(tool.DefinitionQualityAdapted), string(tool.DefinitionQualityIncomplete):
		default:
			t.Fatalf("tool %s returned unknown definition quality %q", info.Name, info.DefinitionQuality)
		}
	}
}

func TestMigratedCoreToolsPassStrictDefinitionValidation(t *testing.T) {
	expected := map[string]bool{
		"Bash":               false,
		"Read":               false,
		"Edit":               false,
		"Write":              false,
		"Glob":               false,
		"Grep":               false,
		"WebSearch":          false,
		"artifact_publish":   false,
		"ask_user":           false,
		"calculator":         false,
		"dependency_install": false,
		"git_remote":         false,
		"trash_file":         false,
		"plugin":             false,
	}
	for _, registered := range tool.ListRegisteredTools() {
		if _, selected := expected[registered.Definition.Name]; !selected {
			continue
		}
		if err := tool.ValidateToolWithMode(registered.Runtime, tool.DefinitionValidationStrictBuiltin); err != nil {
			t.Errorf("migrated core tool %s failed strict definition validation: %v", registered.Definition.Name, err)
			continue
		}
		expected[registered.Definition.Name] = true
	}
	for name, passed := range expected {
		if !passed {
			t.Errorf("migrated core tool %s was not registered with a strict definition", name)
		}
	}
}
