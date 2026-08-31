package dialect

import (
	"os"
	"path/filepath"
	"testing"

	"awaken/src/plugins/ir"
)

func TestParseAppComponentsReadsCodexAppsEnvelope(t *testing.T) {
	root := t.TempDir()
	appPath := filepath.Join(root, ".app.json")
	if err := os.WriteFile(appPath, []byte(`{
  "apps": {
    "outlook-email": {
      "id": "connector_4aaab2856305417b993eca9a216aaf6e"
    }
  }
}`), 0o600); err != nil {
		t.Fatal(err)
	}

	components := parseAppComponents(root, ".app.json", ".codex-plugin/plugin.json")
	if len(components) != 1 {
		t.Fatalf("components=%#v", components)
	}
	component := components[0]
	if component.LogicalID != "app/.app.json#outlook-email" || component.DisplayName != "outlook-email" {
		t.Fatalf("unexpected App identity: %+v", component)
	}
	if component.Mapping != ir.MappingRecognizedUnsupported || component.Validation != "valid" {
		t.Fatalf("unexpected App state: %+v", component)
	}
	if component.App == nil || component.App.ConnectorID != "connector_4aaab2856305417b993eca9a216aaf6e" {
		t.Fatalf("connector ID was not preserved: %+v", component.App)
	}
}

func TestParseAppComponentsRejectsInvalidJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".app.json"), []byte(`{"apps":`), 0o600); err != nil {
		t.Fatal(err)
	}

	components := parseAppComponents(root, ".app.json", ".codex-plugin/plugin.json")
	if len(components) != 1 || components[0].Mapping != ir.MappingInvalid || len(components[0].Diagnostics) != 1 {
		t.Fatalf("invalid App declaration was not diagnosed: %#v", components)
	}
}
