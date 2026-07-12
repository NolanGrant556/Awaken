package dependencyinstall

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"awaken/src/tool"
	"awaken/src/tool/sandbox"
)

type liveDependencyProvider struct {
	dependencyProviderFixture
}

func (p *liveDependencyProvider) ProviderReadiness(tool.ExecutionEnvironmentSnapshot) tool.ProviderReadinessReport {
	return tool.ProviderReadinessReport{
		Readiness: tool.ProviderNeedsRuntime, Reason: "opt-in live smoke forces a workspace-local install",
		RetryableAfterRemediation: true,
		Dependencies: []tool.DependencyObservation{{
			Requirement: p.requirement, Status: tool.DependencyMissing,
			Diagnostic: "forced missing so the host resolver emits a pinned workspace plan",
		}},
	}
}

func TestLivePinnedPureWheelDependencyInstall(t *testing.T) {
	if os.Getenv("AWAKEN_LIVE_DEPENDENCY_INSTALL_SMOKE") != "1" {
		t.Skip("set AWAKEN_LIVE_DEPENDENCY_INSTALL_SMOKE=1 to run the networked pure-wheel install smoke")
	}
	if runtime.GOOS != "darwin" || sandbox.DefaultAdapter().SupportLevel() != sandbox.SandboxFull {
		t.Skip("live dependency smoke currently requires Darwin with a real Seatbelt boundary")
	}
	workspace := t.TempDir()
	temporaryRoot := filepath.Join(t.TempDir(), "task-temp")
	environment := tool.ProbeExecutionEnvironment(t.Context(), tool.ExecutionEnvironmentProbeOptions{
		WorkspaceRoot: workspace, WorkingDirectory: workspace,
		TemporaryWorkRoot: temporaryRoot, RefreshReason: "live_dependency_smoke",
		PATH: os.Getenv("PATH"), PATHSource: "test_process_environment",
		Executables: []string{"python3", "python"}, PackageManagers: []string{"pip", "pip3", "uv"},
		ProbeVersions: false,
	})
	if _, ok := environment.Executable("python3"); !ok {
		if _, ok := environment.Executable("python"); !ok {
			t.Skip("Python is unavailable")
		}
	}
	if _, ok := environment.Executable("pip"); !ok {
		if _, ok := environment.Executable("pip3"); !ok {
			t.Skip("pip/pip3 is unavailable; this smoke intentionally avoids changing manager semantics")
		}
	}

	providerName := "live_pure_wheel_dependency_provider"
	requirement := tool.DependencyRequirement{
		Kind: tool.DependencyLanguagePackage, Name: "humanize", VersionConstraint: "==4.9.0",
		Source: "python", ProbeID: "python_import:humanize",
	}
	provider := &liveDependencyProvider{dependencyProviderFixture{name: providerName, requirement: requirement}}
	if err := tool.RegisterToolForce(provider); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tool.UnregisterTool(providerName) })
	snapshot := tool.NewRunToolAvailabilitySnapshot(environment)
	entry, err := snapshot.Lookup(providerName)
	if err != nil {
		t.Fatal(err)
	}
	var plan tool.CandidateInstallPlan
	for _, candidate := range entry.Readiness.Dependencies[0].CandidateInstallPlans {
		if candidate.Source == "pip" || candidate.Source == "pip3" {
			plan = candidate
			break
		}
	}
	if plan.PlanID == "" {
		t.Fatalf("host resolver did not produce a pip workspace plan: %+v", entry.Readiness.Dependencies)
	}
	permission := tool.NewTaskScopePermissionContext(
		"live-dependency-smoke", workspace, []string{workspace}, []string{workspace}, []string{workspace}, nil, nil, temporaryRoot,
	)
	trashAdapter := &movingTrash{root: filepath.Join(t.TempDir(), "trash")}
	if err := os.MkdirAll(trashAdapter.root, 0o700); err != nil {
		t.Fatal(err)
	}
	broker := tool.NewWorkspaceExecutionBrokerWithOptions(tool.WorkspaceExecutionBrokerOptions{
		StagingParent: t.TempDir(), TrashAdapter: trashAdapter,
	})
	ctx := tool.ContextWithRunAvailability(context.Background(), snapshot, environment)
	ctx = tool.ContextWithExecutionEnvironmentSnapshot(ctx, environment)
	ctx = tool.ContextWithPermissionContext(ctx, permission)
	ctx = tool.ContextWithExecutionBroker(ctx, broker)
	outcome, err := (&Tool{}).ExecuteOutcome(ctx, `{"plan_id":"`+plan.PlanID+`"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != tool.OutcomeSucceeded || outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectApplied {
		t.Fatalf("live pinned dependency install failed: %+v", outcome)
	}
	observation, ok := outcome.Details["refreshed_dependency"].(*tool.DependencyObservation)
	if !ok || observation.Status != tool.DependencyReady || !strings.Contains(observation.Observed, filepath.Join(workspace, ".awaken", "deps", "python")) {
		t.Fatalf("live dependency was not verified from the workspace target: %+v", outcome.Details["refreshed_dependency"])
	}
	if reason := outcome.Diagnostic["refreshed_environment_refresh_reason"]; reason != "dependency_install:"+plan.PlanID {
		t.Fatalf("post-install refresh reason missing: %+v", outcome.Diagnostic)
	}
	receipt, ok := outcome.Details["broker_receipt"].(tool.BrokerExecutionReceipt)
	if !ok || (receipt.CommitStatus != "committed" && receipt.CommitStatus != "no_workspace_changes") || receipt.BrokerID == "" {
		t.Fatalf("live install did not return a committed broker receipt: %#v", outcome.Details["broker_receipt"])
	}
}
