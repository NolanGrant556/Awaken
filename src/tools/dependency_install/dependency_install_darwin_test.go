//go:build darwin

package dependencyinstall

import (
	"encoding/json"
	"testing"

	"awaken/src/tool"
	"awaken/src/tool/sandbox"
)

func TestDependencyInstallReallyUsesDarwinSeatbeltAndStaging(t *testing.T) {
	if sandbox.DefaultAdapter().SupportLevel() != sandbox.SandboxFull {
		t.Skip("sandbox-exec cannot establish a real Seatbelt boundary in this environment")
	}
	fixture := newInstallFixture(t, successfulManagerScript(), nil, nil)
	raw, _ := json.Marshal(input{PlanID: fixture.plan.PlanID})
	outcome, err := (&Tool{}).ExecuteOutcome(fixture.context(), string(raw), nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != tool.OutcomeSucceeded || outcome.TransportStatus != tool.TransportCompleted || outcome.ProcessStatus != tool.ProcessExited {
		t.Fatalf("real Seatbelt typed install failed: %+v", outcome)
	}
	if method := outcome.Diagnostic["sandbox_method"]; method != "sandbox-exec" {
		t.Fatalf("typed install did not report real sandbox-exec enforcement: %+v", outcome.Diagnostic)
	}
}
