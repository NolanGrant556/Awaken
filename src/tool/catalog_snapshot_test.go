package tool

import (
	"context"
	"testing"
)

type snapshotProviderTool struct {
	name         string
	marker       string
	source       ToolSource
	providerID   ProviderID
	capability   Capability
	readiness    ProviderReadiness
	dependencies []DependencyRequirement
}

type manifestOnlyDependencyTool struct {
	name       string
	dependency DependencyRequirement
}

func (t *manifestOnlyDependencyTool) Name() string                                    { return t.name }
func (t *manifestOnlyDependencyTool) Description() string                             { return "manifest-only dependency provider" }
func (t *manifestOnlyDependencyTool) Parameters() []ToolParameter                     { return nil }
func (t *manifestOnlyDependencyTool) Execute(context.Context, string) (string, error) { return "", nil }
func (t *manifestOnlyDependencyTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name: t.name, Description: t.Description(), Source: SourceBuiltin,
		ProviderID: ProviderID("builtin:" + t.name), Capabilities: []Capability{CapabilityPDFRender},
		Dependencies: []DependencyRequirement{t.dependency}, Annotations: ToolAnnotations{ReadOnly: true},
	}
}

func (t *snapshotProviderTool) Name() string        { return t.name }
func (t *snapshotProviderTool) Description() string { return "snapshot provider test tool " + t.marker }
func (t *snapshotProviderTool) Parameters() []ToolParameter {
	return nil
}
func (t *snapshotProviderTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name: t.name, DisplayName: t.marker, Source: t.source, Description: t.Description(),
		ProviderID: t.providerID, Capabilities: []Capability{t.capability}, Dependencies: cloneDependencyRequirements(t.dependencies),
		Annotations: ToolAnnotations{ReadOnly: true},
	}
}
func (t *snapshotProviderTool) Execute(context.Context, string) (string, error) { return t.marker, nil }
func (t *snapshotProviderTool) ProviderReadiness(ExecutionEnvironmentSnapshot) ProviderReadinessReport {
	report := ProviderReadinessReport{Readiness: t.readiness}
	if t.readiness != ProviderReady && len(t.dependencies) > 0 {
		report.Reason = "test dependency is missing"
		report.Dependencies = []DependencyObservation{{
			Requirement: t.dependencies[0], Status: DependencyMissing, Diagnostic: "not found in environment snapshot",
		}}
	}
	return report
}

func TestRunAvailabilitySnapshotPinsRuntimeAndDefinitionAcrossRefresh(t *testing.T) {
	const name = "snapshot_pin_test_tool"
	t.Cleanup(func() { UnregisterTool(name) })
	first := &snapshotProviderTool{
		name: name, marker: "v1", source: SourceBuiltin,
		providerID: "builtin:snapshot-v1", capability: CapabilityDocumentCreate, readiness: ProviderReady,
	}
	if err := RegisterToolForce(first); err != nil {
		t.Fatalf("register first: %v", err)
	}
	environment := ExecutionEnvironmentSnapshot{SnapshotID: "env_test_v1", OS: "test", Architecture: "test"}
	before := NewRunToolAvailabilitySnapshot(environment)
	beforeEntry, err := before.Lookup(name)
	if err != nil {
		t.Fatalf("lookup before: %v", err)
	}

	second := &snapshotProviderTool{
		name: name, marker: "v2", source: SourceBuiltin,
		providerID: "builtin:snapshot-v2", capability: CapabilityDocumentCreate, readiness: ProviderReady,
	}
	if err := RegisterToolForce(second); err != nil {
		t.Fatalf("register second: %v", err)
	}
	after := NewRunToolAvailabilitySnapshot(environment)
	afterEntry, err := after.Lookup(name)
	if err != nil {
		t.Fatalf("lookup after: %v", err)
	}

	if beforeEntry.Runtime != first || beforeEntry.Definition.DisplayName != "v1" || beforeEntry.Provider.ProviderID != "builtin:snapshot-v1" {
		t.Fatalf("old snapshot drifted after refresh: %+v", beforeEntry)
	}
	if afterEntry.Runtime != second || afterEntry.Definition.DisplayName != "v2" || afterEntry.Provider.ProviderID != "builtin:snapshot-v2" {
		t.Fatalf("new snapshot did not capture replacement: %+v", afterEntry)
	}
	if beforeEntry.RegistrationID == afterEntry.RegistrationID || before.SnapshotID == after.SnapshotID || before.CatalogVersion >= after.CatalogVersion {
		t.Fatalf("replacement identities not versioned: before=%+v after=%+v", beforeEntry, afterEntry)
	}
}

func TestResolveCapabilityOrdersProvidersAndReportsDependencyState(t *testing.T) {
	names := []string{"snapshot_builtin_candidate", "snapshot_mcp_candidate", "snapshot_skill_candidate"}
	for _, name := range names {
		name := name
		t.Cleanup(func() { UnregisterTool(name) })
	}
	dependency := DependencyRequirement{Kind: DependencyOSExecutable, Name: "missing-renderer"}
	providers := []*snapshotProviderTool{
		{name: names[2], marker: "skill", source: SourceSkill, providerID: "skill:render", capability: CapabilityPresentationRender, readiness: ProviderReady},
		{name: names[1], marker: "mcp", source: SourceMCP, providerID: "mcp:render", capability: CapabilityPresentationRender, readiness: ProviderNeedsRuntime, dependencies: []DependencyRequirement{dependency}},
		{name: names[0], marker: "builtin", source: SourceBuiltin, providerID: "builtin:render", capability: CapabilityPresentationRender, readiness: ProviderReady},
	}
	for _, provider := range providers {
		if err := RegisterToolForce(provider); err != nil {
			t.Fatalf("register %s: %v", provider.name, err)
		}
	}
	snapshot := NewRunToolAvailabilitySnapshot(ExecutionEnvironmentSnapshot{SnapshotID: "env_candidates", OS: "test", Architecture: "test"})
	diagnostic := snapshot.ResolveCapability(CapabilityPresentationRender)
	if diagnostic.ReadyCandidates < 2 || len(diagnostic.Candidates) < 3 {
		t.Fatalf("unexpected candidates: %+v", diagnostic)
	}
	positions := map[ProviderID]int{}
	for index, candidate := range diagnostic.Candidates {
		positions[candidate.ProviderID] = index
	}
	if !(positions["builtin:render"] < positions["mcp:render"] && positions["mcp:render"] < positions["skill:render"]) {
		t.Fatalf("unexpected provider preference order: %+v", diagnostic.Candidates)
	}
	mcp := diagnostic.Candidates[positions["mcp:render"]]
	if mcp.Readiness != ProviderNeedsRuntime || len(mcp.DependencyState) != 1 || mcp.DependencyState[0].Status != DependencyMissing {
		t.Fatalf("missing bounded dependency diagnostic: %+v", mcp)
	}
}

func TestRunAvailabilityContextRejectsMissingSnapshot(t *testing.T) {
	if _, _, ok := RunAvailabilityFromContext(context.Background()); ok {
		t.Fatal("context without snapshot must not fall back to global registry")
	}
	ctx := ContextWithRunAvailability(context.Background(), RunToolAvailabilitySnapshot{}, ExecutionEnvironmentSnapshot{})
	if _, _, ok := RunAvailabilityFromContext(ctx); ok {
		t.Fatal("zero snapshot must remain unavailable")
	}
}

func TestGenericDependencyResolverReportsPinnedProbeAndHostInstallCandidates(t *testing.T) {
	const name = "manifest_only_dependency_tool"
	t.Cleanup(func() { UnregisterTool(name) })
	provider := &manifestOnlyDependencyTool{name: name, dependency: DependencyRequirement{
		Kind: DependencyOSExecutable, Name: "pdftoppm", Source: "host executable",
	}}
	if err := RegisterToolForce(provider); err != nil {
		t.Fatal(err)
	}
	environment := ExecutionEnvironmentSnapshot{
		SnapshotID: "env_dependency_candidates", OS: "darwin", Architecture: "arm64",
		WorkspaceRoot: "/workspace", WorkingDirectory: "/workspace",
		Executables:              []ExecutableObservation{{Name: "brew", Path: "/opt/homebrew/bin/brew", Provenance: "test", Fingerprint: "sha256:test-brew"}},
		AvailablePackageManagers: []string{"brew"},
	}
	snapshot := NewRunToolAvailabilitySnapshot(environment)
	entry, err := snapshot.Lookup(name)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Readiness.Readiness != ProviderNeedsRuntime || len(entry.Readiness.Dependencies) != 1 {
		t.Fatalf("missing dependency readiness facts: %+v", entry.Readiness)
	}
	observation := entry.Readiness.Dependencies[0]
	if observation.Status != DependencyMissing || len(observation.CandidateInstallPlans) != 1 {
		t.Fatalf("missing host-generated install candidate: %+v", observation)
	}
	plan := observation.CandidateInstallPlans[0]
	if plan.PlanID == "" || plan.EnvironmentID != environment.SnapshotID || plan.RequirementFingerprint == "" || plan.ExecutableFingerprint == "" || plan.Executable != "/opt/homebrew/bin/brew" || len(plan.Args) != 2 || plan.Args[0] != "install" || plan.Args[1] != "poppler" || plan.RequiresAdmin || plan.Provenance != "host_dependency_resolver_v1" {
		t.Fatalf("unsafe or unexpected install candidate: %+v", plan)
	}
	entry.Readiness.Dependencies[0].CandidateInstallPlans[0].Args[0] = "tampered"
	entryAgain, err := snapshot.Lookup(name)
	if err != nil {
		t.Fatal(err)
	}
	if got := entryAgain.Readiness.Dependencies[0].CandidateInstallPlans[0].Args[0]; got != "install" {
		t.Fatalf("caller mutated the pinned snapshot's nested install plan: %q", got)
	}
}

func TestLookupCandidateInstallPlanMergesSameRequirementAndSeparatesDifferentRequirement(t *testing.T) {
	names := []string{"install_plan_provider_a", "install_plan_provider_b", "install_plan_provider_other_probe"}
	for _, name := range names {
		name := name
		t.Cleanup(func() { UnregisterTool(name) })
	}
	shared := DependencyRequirement{Kind: DependencyLanguagePackage, Name: "snapshot-package", Source: "python", ProbeID: "python_import:snapshot_package"}
	other := shared
	other.ProbeID = "python_import:other_snapshot_package"
	providers := []*snapshotProviderTool{
		{name: names[0], marker: "a", source: SourcePlugin, providerID: "plugin:install-a", capability: CapabilityPresentationRender, readiness: ProviderNeedsRuntime, dependencies: []DependencyRequirement{shared}},
		{name: names[1], marker: "b", source: SourcePlugin, providerID: "plugin:install-b", capability: CapabilityDocumentCreate, readiness: ProviderNeedsRuntime, dependencies: []DependencyRequirement{shared}},
		{name: names[2], marker: "other", source: SourcePlugin, providerID: "plugin:install-other", capability: CapabilitySpreadsheetCreate, readiness: ProviderNeedsRuntime, dependencies: []DependencyRequirement{other}},
	}
	for _, provider := range providers {
		if err := RegisterToolForce(provider); err != nil {
			t.Fatal(err)
		}
	}
	environment := ExecutionEnvironmentSnapshot{
		SnapshotID: "env_install_plan_lookup", OS: "darwin", Architecture: "arm64",
		WorkspaceRoot: "/workspace", WorkingDirectory: "/workspace",
		AvailablePackageManagers: []string{"pip"},
		Executables:              []ExecutableObservation{{Name: "pip", Path: "/usr/local/bin/pip", Provenance: "test", Fingerprint: "sha256:test-pip"}},
	}
	snapshot := NewRunToolAvailabilitySnapshot(environment)
	first, err := snapshot.Lookup(names[0])
	if err != nil {
		t.Fatal(err)
	}
	sharedPlan := first.Readiness.Dependencies[0].CandidateInstallPlans[0]
	resolution, err := snapshot.LookupCandidateInstallPlan(sharedPlan.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolution.Providers) != 2 || resolution.Requirement != shared || resolution.ToolSnapshotID != snapshot.SnapshotID {
		t.Fatalf("same requirement provenance was not merged: %+v", resolution)
	}
	otherEntry, err := snapshot.Lookup(names[2])
	if err != nil {
		t.Fatal(err)
	}
	otherPlan := otherEntry.Readiness.Dependencies[0].CandidateInstallPlans[0]
	if otherPlan.PlanID == sharedPlan.PlanID || otherPlan.RequirementFingerprint == sharedPlan.RequirementFingerprint {
		t.Fatalf("different requirement collided with shared plan: shared=%+v other=%+v", sharedPlan, otherPlan)
	}

	resolution.Plan.Args[0] = "tampered"
	resolution.Providers[0].Capabilities[0] = CapabilityShellExecute
	again, err := snapshot.LookupCandidateInstallPlan(sharedPlan.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Plan.Args[0] != "install" || again.Providers[0].Capabilities[0] == CapabilityShellExecute {
		t.Fatalf("resolved install plan/provenance was not clone-safe: %+v", again)
	}
}
