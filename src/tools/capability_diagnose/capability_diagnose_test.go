package capabilitydiagnose

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestCapabilityDiagnoseUsesPinnedRunSnapshot(t *testing.T) {
	environment := tool.ProbeExecutionEnvironment(context.Background(), tool.ExecutionEnvironmentProbeOptions{
		Executables: []string{}, PackageManagers: []string{}, ProbeVersions: false,
	})
	snapshot := tool.NewRunToolAvailabilitySnapshot(environment)
	ctx := tool.ContextWithRunAvailability(context.Background(), snapshot, environment)

	output, err := (&Tool{}).Execute(ctx, `{"capability":"tool.diagnose"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var diagnostic tool.CapabilityDiagnostic
	if err := json.Unmarshal([]byte(output), &diagnostic); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if diagnostic.SnapshotID != snapshot.SnapshotID || diagnostic.EnvironmentID != environment.SnapshotID {
		t.Fatalf("diagnostic not bound to context snapshots: %+v", diagnostic)
	}
	if diagnostic.ReadyCandidates < 1 || len(diagnostic.Candidates) < 1 {
		t.Fatalf("expected registered diagnose provider, got %+v", diagnostic)
	}
	if diagnostic.Candidates[0].ToolName != "capability_diagnose" {
		t.Fatalf("unexpected first candidate: %+v", diagnostic.Candidates[0])
	}
}

func TestCapabilityDiagnoseDoesNotFallBackToMutableRegistry(t *testing.T) {
	outcome, err := (&Tool{}).ExecuteOutcome(context.Background(), `{"capability":"tool.diagnose"}`, nil)
	if err != nil || outcome.Status != tool.OutcomeUnsupported || !strings.Contains(outcome.ErrorMessage, "run_availability_snapshot_missing") {
		t.Fatalf("expected explicit missing snapshot failure, outcome=%+v err=%v", outcome, err)
	}
}
