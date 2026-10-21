package tool

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool/sandbox"
)

type degradedProbeSandbox struct{ applied int }

func (*degradedProbeSandbox) SupportLevel() sandbox.SandboxSupport { return sandbox.SandboxDegraded }
func (s *degradedProbeSandbox) ApplySandbox(*exec.Cmd, string, string) (sandbox.SandboxResult, error) {
	s.applied++
	return sandbox.SandboxResult{Support: sandbox.SandboxDegraded, Method: "degraded", Applied: false}, nil
}

func skipWithoutFullPlatformSandbox(t *testing.T) {
	t.Helper()
	if sandbox.DefaultAdapter().SupportLevel() != sandbox.SandboxFull {
		t.Skip("language package probe requires a full platform sandbox")
	}
}

func TestLanguagePackageProbeUsesExplicitBoundedProviderProbe(t *testing.T) {
	skipWithoutFullPlatformSandbox(t)
	workspace := t.TempDir()
	environment := ProbeExecutionEnvironment(t.Context(), ExecutionEnvironmentProbeOptions{
		WorkspaceRoot: workspace, WorkingDirectory: workspace,
		Executables: []string{"python3", "python", "node"}, PackageManagers: []string{},
		ProbeVersions: false,
	})
	if _, ok := environment.Executable("python3"); !ok {
		if _, ok := environment.Executable("python"); !ok {
			t.Skip("Python is unavailable for the bounded package probe")
		}
	}

	observations := observeDependencyRequirements([]DependencyRequirement{
		{Kind: DependencyLanguagePackage, Name: "json", Source: "python", ProbeID: "python_import:json"},
		{Kind: DependencyLanguagePackage, Name: "definitely-missing-awaken-package", Source: "python", ProbeID: "python_import:definitely_missing_awaken_package"},
		{Kind: DependencyLanguagePackage, Name: "untrusted", Source: "python", ProbeID: "python_import:../../escape"},
	}, environment)
	if len(observations) != 3 {
		t.Fatalf("unexpected observations: %+v", observations)
	}
	if observations[0].Status != DependencyReady || observations[0].Observed == "" {
		t.Fatalf("Python stdlib package was not observed: %+v", observations[0])
	}
	if observations[1].Status != DependencyMissing {
		t.Fatalf("missing Python package was not typed missing: %+v", observations[1])
	}
	if observations[2].Status != DependencyUnknown || observations[2].Diagnostic == "" {
		t.Fatalf("unsafe/free-form probe id must not execute: %+v", observations[2])
	}
	if len(observations[1].CandidateInstallPlans) != 0 {
		// No package managers were included in the environment snapshot, so the
		// resolver must not invent an install command.
		t.Fatalf("probe invented an unavailable installer: %+v", observations[1].CandidateInstallPlans)
	}
	if filepath.Clean(environment.WorkspaceRoot) != filepath.Clean(workspace) {
		t.Fatalf("probe changed the pinned environment workspace: %+v", environment)
	}
}

func TestNodePackageProbeResolvesWithoutExecutingPackageCode(t *testing.T) {
	skipWithoutFullPlatformSandbox(t)
	workspace := t.TempDir()
	environment := ProbeExecutionEnvironment(t.Context(), ExecutionEnvironmentProbeOptions{
		WorkspaceRoot: workspace, WorkingDirectory: workspace,
		Executables: []string{"node"}, PackageManagers: []string{}, ProbeVersions: false,
	})
	if _, ok := environment.Executable("node"); !ok {
		t.Skip("Node is unavailable for the bounded package probe")
	}
	observations := observeDependencyRequirements([]DependencyRequirement{{
		Kind: DependencyLanguagePackage, Name: "node:fs", Source: "node", ProbeID: "node_resolve:node:fs",
	}}, environment)
	if len(observations) != 1 || observations[0].Status != DependencyReady || observations[0].Observed == "" {
		t.Fatalf("Node builtin was not resolved: %+v", observations)
	}
}

func TestNodePackageProbeTreatsUnexportedPackageJSONAsInstalled(t *testing.T) {
	skipWithoutFullPlatformSandbox(t)
	workspace := t.TempDir()
	packageRoot := filepath.Join(workspace, "node_modules", "exports-only")
	if err := os.MkdirAll(packageRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "package.json"), []byte(`{
  "name": "exports-only",
  "version": "1.2.3",
  "main": "index.js",
  "exports": {".": "./index.js"}
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "index.js"), []byte(`require("node:fs").writeFileSync(__dirname + "/executed.txt", "executed"); module.exports = {};`), 0o644); err != nil {
		t.Fatal(err)
	}

	environment := ProbeExecutionEnvironment(t.Context(), ExecutionEnvironmentProbeOptions{
		WorkspaceRoot: workspace, WorkingDirectory: workspace,
		Executables: []string{"node"}, PackageManagers: []string{}, ProbeVersions: false,
	})
	if _, ok := environment.Executable("node"); !ok {
		t.Skip("Node is unavailable for the bounded package probe")
	}
	observation := probeLanguagePackageDependency(DependencyRequirement{
		Kind: DependencyLanguagePackage, Name: "exports-only", Source: "node",
		ProbeID: "node_resolve:exports-only/package.json", VersionConstraint: "==1.2.3",
	}, environment)
	if observation.Status != DependencyReady || !strings.Contains(observation.Observed, "1.2.3") {
		t.Fatalf("ERR_PACKAGE_PATH_NOT_EXPORTED was misclassified as missing: %+v", observation)
	}
	if _, err := os.Stat(filepath.Join(packageRoot, "executed.txt")); !os.IsNotExist(err) {
		t.Fatalf("dependency probe executed package code: %v", err)
	}
}

func TestExecutableAndLanguageProbesFailClosedWithoutAppliedReadOnlySandbox(t *testing.T) {
	adapter := &degradedProbeSandbox{}
	sandbox.SetAdapter(adapter)
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is unavailable")
	}
	environment := ExecutionEnvironmentSnapshot{
		SnapshotID: "env_degraded_probe", OS: "test", Architecture: "test",
		WorkspaceRoot: t.TempDir(), WorkingDirectory: t.TempDir(), PATH: []string{filepath.Dir(python)},
		Executables: []ExecutableObservation{{Name: "python3", Path: python, Provenance: "test"}},
	}
	observation := probeLanguagePackageDependency(DependencyRequirement{
		Kind: DependencyLanguagePackage, Name: "json", Source: "python", ProbeID: "python_import:json",
	}, environment)
	if observation.Status != DependencyError || !strings.Contains(observation.Diagnostic, "sandbox") || adapter.applied != 0 {
		t.Fatalf("language probe did not fail before unsandboxed transport: observation=%+v adapter=%+v", observation, adapter)
	}
	if _, err := boundedVersionProbe(context.Background(), python, 512); !errors.Is(err, ErrExecutionSandboxRequired) || adapter.applied != 0 {
		t.Fatalf("version probe did not fail before unsandboxed transport: err=%v adapter=%+v", err, adapter)
	}
}

func TestLanguageVersionConstraintComparator(t *testing.T) {
	tests := []struct {
		observed   string
		constraint string
		want       bool
		understood bool
	}{
		{observed: "1.2.3", constraint: ">=1.2,<2", want: true, understood: true},
		{observed: "v2.0.0", constraint: "<2", want: false, understood: true},
		{observed: "1.2.0", constraint: "1.2", want: true, understood: true},
		{observed: "2026.07.12+local", constraint: ">=2026.7", want: true, understood: true},
		{observed: "unknown", constraint: ">=1", want: false, understood: false},
		{observed: "1.2.3", constraint: "^1.2", want: false, understood: false},
	}
	for _, testCase := range tests {
		got, understood := languageVersionSatisfies(testCase.observed, testCase.constraint)
		if got != testCase.want || understood != testCase.understood {
			t.Errorf("%s %s => (%v,%v), want (%v,%v)", testCase.observed, testCase.constraint, got, understood, testCase.want, testCase.understood)
		}
	}
}

func TestCandidateInstallPlansAreSnapshotBoundAndDisablePackageScripts(t *testing.T) {
	workspace := t.TempDir()
	environment := ExecutionEnvironmentSnapshot{
		SnapshotID: "env_install_test", OS: "darwin", WorkspaceRoot: workspace, WorkingDirectory: workspace,
		AvailablePackageManagers: []string{"npm", "pip"},
		Executables: []ExecutableObservation{
			{Name: "npm", Path: "/usr/local/bin/npm", Provenance: "test", Fingerprint: "sha256:test-npm"},
			{Name: "pip", Path: "/usr/local/bin/pip", Provenance: "test", Fingerprint: "sha256:test-pip"},
		},
	}
	pythonPlans := candidateInstallPlans(DependencyRequirement{
		Kind: DependencyLanguagePackage, Name: "python-pptx", VersionConstraint: "==1.0.2", Source: "python",
	}, environment)
	if len(pythonPlans) != 1 {
		t.Fatalf("unexpected Python plans: %+v", pythonPlans)
	}
	pythonPlan := pythonPlans[0]
	pythonArgs := strings.Join(pythonPlan.Args, " ")
	if pythonPlan.PlanID == "" || pythonPlan.EnvironmentID != environment.SnapshotID || pythonPlan.InstallScope != InstallWorkspace || pythonPlan.RequiresAdmin ||
		pythonPlan.RequirementFingerprint == "" || pythonPlan.ExecutableFingerprint == "" ||
		!strings.Contains(pythonArgs, "--no-input") || !strings.Contains(pythonArgs, "--only-binary=:all:") || !strings.Contains(pythonArgs, "--target .awaken/deps/python") || !strings.Contains(pythonArgs, "python-pptx==1.0.2") {
		t.Fatalf("Python plan is not pinned/safe: %+v", pythonPlan)
	}
	if pythonPlan.PlanID != candidateInstallPlanIdentity(pythonPlan) {
		t.Fatalf("Python plan identity is not reproducible: %+v", pythonPlan)
	}

	nodePlans := candidateInstallPlans(DependencyRequirement{
		Kind: DependencyLanguagePackage, Name: "puppeteer", VersionConstraint: "=24.3.0", Source: "node",
	}, environment)
	if len(nodePlans) != 1 {
		t.Fatalf("unexpected Node plans: %+v", nodePlans)
	}
	nodePlan := nodePlans[0]
	nodeArgs := strings.Join(nodePlan.Args, " ")
	if nodePlan.PlanID == "" || nodePlan.EnvironmentID != environment.SnapshotID || !nodePlan.ModifiesManifest ||
		!strings.Contains(nodeArgs, "--ignore-scripts") || !strings.Contains(nodeArgs, "--no-audit") || !strings.Contains(nodeArgs, "puppeteer@24.3.0") {
		t.Fatalf("Node plan is not pinned/safe: %+v", nodePlan)
	}

	if plans := candidateInstallPlans(DependencyRequirement{
		Kind: DependencyLanguagePackage, Name: "safe-name", VersionConstraint: "@https://attacker.invalid/pkg.tgz", Source: "node",
	}, environment); len(plans) != 0 {
		t.Fatalf("untrusted version/source syntax produced an install plan: %+v", plans)
	}
	for _, malicious := range []DependencyRequirement{
		{Kind: DependencyLanguagePackage, Name: "https://attacker.invalid/pkg.whl", Source: "python"},
		{Kind: DependencyLanguagePackage, Name: "safe:alias", Source: "python"},
		{Kind: DependencyLanguagePackage, Name: "git+https://attacker.invalid/repo", Source: "python"},
		{Kind: DependencyLanguagePackage, Name: "alias:npm:real", Source: "node"},
		{Kind: DependencyLanguagePackage, Name: "https://attacker.invalid/pkg.tgz", Source: "node"},
		{Kind: DependencyLanguagePackage, Name: "pkg@npm:other", Source: "node"},
		{Kind: DependencyLanguagePackage, Name: "@scope/name@second", Source: "node"},
	} {
		if plans := candidateInstallPlans(malicious, environment); len(plans) != 0 {
			t.Errorf("malicious ecosystem package name produced plan: requirement=%+v plans=%+v", malicious, plans)
		}
	}

	for label, controlledManager := range map[string]string{
		"workspace": filepath.Join(workspace, "bin", "pip"),
		"temporary": filepath.Join(workspace, ".awaken", "tmp", "pip"),
		"skill":     filepath.Join(workspace, ".awaken", "skills", "package", "pip"),
	} {
		controlled := environment.Clone()
		controlled.TemporaryWorkRoot = filepath.Join(workspace, ".awaken", "tmp")
		controlled.SkillRoots = []SkillRootObservation{{Source: "project", Path: filepath.Join(workspace, ".awaken", "skills")}}
		controlled.Executables = []ExecutableObservation{{Name: "pip", Path: controlledManager, Provenance: "test", Fingerprint: "sha256:controlled"}}
		controlled.AvailablePackageManagers = []string{"pip"}
		if plans := candidateInstallPlans(DependencyRequirement{Kind: DependencyLanguagePackage, Name: "safe-package", Source: "python"}, controlled); len(plans) != 0 {
			t.Errorf("%s-controlled package manager produced an open-network install plan: %+v", label, plans)
		}
	}

	firstRequirement := DependencyRequirement{Kind: DependencyLanguagePackage, Name: "same-package", Source: "python", ProbeID: "python_import:first"}
	secondRequirement := DependencyRequirement{Kind: DependencyLanguagePackage, Name: "same-package", Source: "python", ProbeID: "python_import:second"}
	firstPlans := candidateInstallPlans(firstRequirement, environment)
	secondPlans := candidateInstallPlans(secondRequirement, environment)
	if len(firstPlans) != 1 || len(secondPlans) != 1 || firstPlans[0].PlanID == secondPlans[0].PlanID || firstPlans[0].RequirementFingerprint == secondPlans[0].RequirementFingerprint {
		t.Fatalf("plan identity was not bound one-to-one to dependency requirement: first=%+v second=%+v", firstPlans, secondPlans)
	}
}
