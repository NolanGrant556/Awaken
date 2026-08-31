package plugins

import (
	"context"
	"path/filepath"
	"testing"

	"awaken/src/plugins/ir"
)

func TestOfficialOutlookPluginDeclaresHostConnectorForEverySkill(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "examples", "plugins", "awaken-outlook"))
	if err != nil {
		t.Fatal(err)
	}
	store := NewStoreAt(t.TempDir())
	validated, err := store.Validate(context.Background(), Source{Kind: SourceLocalPath, Path: root, DialectHint: ir.DialectAgentPlugins})
	if err != nil {
		t.Fatal(err)
	}
	if !validated.Valid {
		t.Fatalf("validation = %+v", validated)
	}
	if validated.Name != "awaken-outlook" || len(validated.Setup) != 1 || validated.Setup[0].Kind != ir.SetupHostConnector {
		t.Fatalf("validation = %+v", validated)
	}
	requirement := validated.Setup[0]
	if requirement.ConnectorID != "awaken.microsoft.outlook" || len(requirement.ComponentIDs) != 4 {
		t.Fatalf("setup = %+v", requirement)
	}
	for _, component := range validated.Components {
		if component.Kind != ir.KindSkill {
			continue
		}
		if !containsOutlookTestString(requirement.ComponentIDs, component.LogicalID) {
			t.Fatalf("skill %s is not gated by Outlook account setup", component.LogicalID)
		}
	}
}

func containsOutlookTestString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
