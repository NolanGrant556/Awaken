package ensureenvironment

import (
	"context"
	"strings"
	"testing"

	"awaken/src/managedenv"
	"awaken/src/tool"
)

func TestRejectModelSuppliedInstallInstructions(t *testing.T) {
	for _, raw := range []string{`{"capability":"python","path":"/tmp/install"}`, `{"requirements":[{"ecosystem":"python","name":"x","url":"https://example.com"}]}`, `{"capability":"python"} {}`} {
		if _, err := parse(raw); err == nil {
			t.Fatalf("accepted install instructions: %s", raw)
		}
	}
}

func TestUnknownCapabilityReturnsActionableFailure(t *testing.T) {
	c, err := managedenv.BuiltinCatalog()
	if err != nil {
		t.Fatal(err)
	}
	m, err := managedenv.New(managedenv.Options{Root: t.TempDir(), Catalog: c})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	ctx := managedenv.WithManager(context.Background(), m)
	ctx = tool.ContextWithToolThreadID(ctx, "thread-a")
	ctx = tool.ContextWithToolExecutionIdentity(ctx, tool.ToolExecutionIdentity{RunID: "run-a", AgentID: "root", ToolCallID: "call-a", ToolName: "ensure_environment"})
	out, err := (&Tool{}).ExecuteOutcome(ctx, `{"capability":"excel"}`, nil)
	if err != nil || !out.IsError() || out.ErrorCode != "environment_capability_unknown" || !strings.Contains(out.Content, "load_workspace_dependencies") || strings.Contains(out.Content, "excel.recalc") {
		t.Fatalf("unhelpful failure: %+v %v", out, err)
	}
	if _, err = (&Tool{}).Execute(ctx, `{"capability":"excel"}`); err == nil {
		t.Fatal("legacy Execute reported failure as success")
	}
	if len(m.Operations()) != 0 {
		t.Fatal("invalid capability started preparation")
	}
}

func TestModelSchemaEnumeratesActualCapabilities(t *testing.T) {
	definition := (&Tool{}).Definition()
	values := definition.InputSchema.Properties["capability"].Enum
	found := false
	for _, value := range values {
		if value == "tesseract" {
			found = true
		}
		if value == "excel" || value == "excel.recalc" || value == "python-office" {
			t.Fatal("Skill name advertised as capability")
		}
	}
	if !found {
		t.Fatal("model cannot discover exact route")
	}
}

func TestLegacyOfficeCapabilityStillResolves(t *testing.T) {
	c, err := managedenv.BuiltinCatalog()
	if err != nil {
		t.Fatal(err)
	}
	request, err := parse(`{"capability":"excel.recalc"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Resolve(request); err != nil {
		t.Fatal("legacy caller lost compatibility", err)
	}
}
