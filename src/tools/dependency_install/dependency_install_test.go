package dependencyinstall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/tool"
	"awaken/src/tool/sandbox"
	"awaken/src/tool/trash"
)

type dependencyProviderFixture struct {
	name        string
	requirement tool.DependencyRequirement
}

func (p *dependencyProviderFixture) Name() string { return p.name }
func (p *dependencyProviderFixture) Description() string {
	return "dependency install fixture provider"
}
func (p *dependencyProviderFixture) Parameters() []tool.ToolParameter                { return nil }
func (p *dependencyProviderFixture) Execute(context.Context, string) (string, error) { return "", nil }
func (p *dependencyProviderFixture) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name: p.name, Description: p.Description(), Source: tool.SourcePlugin,
		ProviderID: tool.ProviderID("fixture:" + p.name), Capabilities: []tool.Capability{tool.CapabilityPresentationRender},
		Dependencies: []tool.DependencyRequirement{p.requirement}, Annotations: tool.ToolAnnotations{ReadOnly: true},
	}
}

type appliedSandbox struct {
	mu    sync.Mutex
	tiers []string
}

type degradedSandbox struct{}

func (*degradedSandbox) SupportLevel() sandbox.SandboxSupport { return sandbox.SandboxDegraded }
func (*degradedSandbox) ApplySandbox(*exec.Cmd, string, string) (sandbox.SandboxResult, error) {
	return sandbox.SandboxResult{Support: sandbox.SandboxDegraded, Method: "degraded", Applied: false}, nil
}

func (*appliedSandbox) SupportLevel() sandbox.SandboxSupport { return sandbox.SandboxFull }
func (s *appliedSandbox) ApplySandbox(_ *exec.Cmd, tier string, _ string) (sandbox.SandboxResult, error) {
	s.mu.Lock()
	s.tiers = append(s.tiers, tier)
	s.mu.Unlock()
	return sandbox.SandboxResult{Support: sandbox.SandboxFull, Method: "test-applied", Applied: true}, nil
}

type movingTrash struct {
	root  string
	mu    sync.Mutex
	calls int
}

func (a *movingTrash) TrashPath(path string) (trash.TrashResult, error) {
	a.mu.Lock()
	a.calls++
	call := a.calls
	a.mu.Unlock()
	destination := filepath.Join(a.root, fmt.Sprintf("recovery-%d", call))
	if err := os.Rename(path, destination); err != nil {
		return trash.TrashResult{}, err
	}
	return trash.TrashResult{OriginalPath: path, TrashedPath: destination, Method: "test-rename"}, nil
}

type observedOutput struct {
	mu     sync.Mutex
	values []tool.CommandOutput
}

func (o *observedOutput) OnCommandOutput(value tool.CommandOutput) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.values = append(o.values, value)
}

type installFixture struct {
	workspace    string
	manager      string
	providerName string
	requirement  tool.DependencyRequirement
	environment  tool.ExecutionEnvironmentSnapshot
	snapshot     tool.RunToolAvailabilitySnapshot
	plan         tool.CandidateInstallPlan
	permission   tool.PermissionContext
	broker       *tool.WorkspaceExecutionBroker
	trash        *movingTrash
	adapter      *appliedSandbox
}

func newInstallFixture(t *testing.T, script string, writableRoots, protectedRoots []string) installFixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script package-manager fixture is Unix-only; Windows production install remains fail-closed without a full sandbox")
	}
	base := t.TempDir()
	workspace := filepath.Join(base, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	managerDir := filepath.Join(base, "bin")
	if err := os.MkdirAll(managerDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manager := filepath.Join(managerDir, "pip")
	if err := os.WriteFile(manager, []byte("#!/bin/sh\nset -eu\n"+script+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := tool.FingerprintExecutable(manager)
	if err != nil {
		t.Fatal(err)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required for the bounded post-install probe")
	}
	providerName := "dependency_install_fixture_" + strings.ReplaceAll(filepath.Base(base), "-", "_")
	requirement := tool.DependencyRequirement{
		Kind: tool.DependencyLanguagePackage, Name: "awaken-fake-dep", Source: "python", ProbeID: "python_import:awaken_fake_dep",
	}
	provider := &dependencyProviderFixture{name: providerName, requirement: requirement}
	if err := tool.RegisterToolForce(provider); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tool.UnregisterTool(providerName) })
	environment := tool.ExecutionEnvironmentSnapshot{
		SnapshotID: "env_" + filepath.Base(base), OS: runtime.GOOS, Architecture: runtime.GOARCH,
		WorkspaceRoot: workspace, WorkingDirectory: workspace,
		SkillRoots:        []tool.SkillRootObservation{{Source: "project", Path: filepath.Join(workspace, ".awaken", "skills")}},
		TemporaryWorkRoot: filepath.Join(base, "task-temp"), RefreshReason: "run_start",
		PATH: []string{managerDir, filepath.Dir(python), "/usr/bin", "/bin"}, PATHSource: "test",
		Executables: []tool.ExecutableObservation{
			{Name: "pip", Path: manager, Provenance: "test", Fingerprint: fingerprint},
			{Name: "python3", Path: python, Provenance: "test"},
		},
		AvailablePackageManagers: []string{"pip"}, Confidence: "observed",
	}
	snapshot := tool.NewRunToolAvailabilitySnapshot(environment)
	entry, err := snapshot.Lookup(providerName)
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Readiness.Dependencies) != 1 || len(entry.Readiness.Dependencies[0].CandidateInstallPlans) != 1 {
		t.Fatalf("fixture did not produce one pinned plan: %+v", entry.Readiness)
	}
	permission := tool.NewTaskScopePermissionContext("dependency-install-test", workspace, []string{workspace}, writableRoots, nil, nil, protectedRoots, environment.TemporaryWorkRoot)
	trashAdapter := &movingTrash{root: filepath.Join(base, "trash")}
	if err := os.MkdirAll(trashAdapter.root, 0o700); err != nil {
		t.Fatal(err)
	}
	broker := tool.NewWorkspaceExecutionBrokerWithOptions(tool.WorkspaceExecutionBrokerOptions{
		StagingParent: filepath.Join(base, "staging"), TrashAdapter: trashAdapter,
	})
	return installFixture{
		workspace: workspace, manager: manager, providerName: providerName, requirement: requirement,
		environment: environment, snapshot: snapshot, plan: entry.Readiness.Dependencies[0].CandidateInstallPlans[0],
		permission: permission, broker: broker, trash: trashAdapter, adapter: &appliedSandbox{},
	}
}

func (f installFixture) context() context.Context {
	ctx := tool.ContextWithRunAvailability(context.Background(), f.snapshot, f.environment)
	ctx = tool.ContextWithExecutionEnvironmentSnapshot(ctx, f.environment)
	ctx = tool.ContextWithPermissionContext(ctx, f.permission)
	return tool.ContextWithExecutionBroker(ctx, f.broker)
}

func successfulManagerScript() string {
	return `target=""
previous=""
for argument in "$@"; do
  if [ "$previous" = "--target" ]; then target="$argument"; fi
  previous="$argument"
done
test "$target" = ".awaken/deps/python"
mkdir -p "$target"
printf 'VALUE = 1\n' > "$target/awaken_fake_dep.py"
printf '%s\n' "$@" > "$target/argv.txt"
printf 'PATH=%s\n' "$PATH"
printf 'registry=https://alice:hunter2@registry.invalid/simple?token=query-secret\n'
printf '_authToken=assignment-secret\n' >&2`
}

func TestDependencyInstallExecutesPinnedArgvAndCommitsOnlyVerifiedDependency(t *testing.T) {
	fixture := newInstallFixture(t, successfulManagerScript(), nil, nil)
	observer := &observedOutput{}
	raw, _ := json.Marshal(input{PlanID: fixture.plan.PlanID})
	outcome, err := (&Tool{adapter: fixture.adapter}).ExecuteOutcome(fixture.context(), string(raw), observer)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != tool.OutcomeSucceeded || outcome.TransportStatus != tool.TransportCompleted || outcome.ProcessStatus != tool.ProcessExited || outcome.ExitCode == nil || *outcome.ExitCode != 0 {
		t.Fatalf("unexpected success outcome: %+v", outcome)
	}
	module := filepath.Join(fixture.workspace, ".awaken", "deps", "python", "awaken_fake_dep.py")
	if content, err := os.ReadFile(module); err != nil || string(content) != "VALUE = 1\n" {
		t.Fatalf("verified module was not committed: content=%q err=%v", content, err)
	}
	argv, err := os.ReadFile(filepath.Join(fixture.workspace, ".awaken", "deps", "python", "argv.txt"))
	if err != nil || !strings.Contains(string(argv), "--no-input") || !strings.Contains(string(argv), "--only-binary=:all:") || !strings.Contains(string(argv), ".awaken/deps/python") {
		t.Fatalf("safe direct argv was not preserved: %q err=%v", argv, err)
	}
	if strings.Contains(outcome.Content, "hunter2") || strings.Contains(outcome.Content, "query-secret") || strings.Contains(outcome.Content, "assignment-secret") {
		t.Fatalf("package-manager secrets leaked into persisted content: %s", outcome.Content)
	}
	if reason := outcome.Diagnostic["refreshed_environment_refresh_reason"]; reason != "dependency_install:"+fixture.plan.PlanID {
		t.Fatalf("post-install environment refresh provenance missing: %+v", outcome.Diagnostic)
	}
	observer.mu.Lock()
	streamed := fmt.Sprint(observer.values)
	observer.mu.Unlock()
	if strings.Contains(streamed, "hunter2") || strings.Contains(streamed, "query-secret") || strings.Contains(streamed, "assignment-secret") {
		t.Fatalf("package-manager secrets leaked into observer: %s", streamed)
	}
	fixture.adapter.mu.Lock()
	tiers := append([]string(nil), fixture.adapter.tiers...)
	fixture.adapter.mu.Unlock()
	if len(tiers) != 2 || tiers[0] != sandbox.TierOpenWorldSystem || tiers[1] != sandbox.TierInspectReadOnly {
		t.Fatalf("typed install did not use open_world_system sandbox: %v", tiers)
	}
}

func TestDependencyInstallNonzeroDiscardsPartialStaging(t *testing.T) {
	fixture := newInstallFixture(t, `mkdir -p .awaken/deps/python
printf partial > .awaken/deps/python/partial.txt
exit 7`, nil, nil)
	raw, _ := json.Marshal(input{PlanID: fixture.plan.PlanID})
	outcome, err := (&Tool{adapter: fixture.adapter}).ExecuteOutcome(fixture.context(), string(raw), nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != tool.OutcomeFailed || outcome.ErrorCode != tool.ErrorCodeProcessExitNonzero || outcome.TransportStatus != tool.TransportCompleted || outcome.ProcessStatus != tool.ProcessExited {
		t.Fatalf("nonzero transport truth was lost: %+v", outcome)
	}
	if _, err := os.Lstat(filepath.Join(fixture.workspace, ".awaken")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial install escaped discarded staging: %v", err)
	}
}

func TestDependencyInstallTimeoutDiscardsPartialStaging(t *testing.T) {
	fixture := newInstallFixture(t, `mkdir -p .awaken/deps/python
printf partial > .awaken/deps/python/partial.txt
sleep 5`, nil, nil)
	raw, _ := json.Marshal(input{PlanID: fixture.plan.PlanID})
	outcome, err := (&Tool{adapter: fixture.adapter, timeout: 25 * time.Millisecond}).ExecuteOutcome(fixture.context(), string(raw), nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != tool.OutcomeFailed || outcome.ErrorCode != tool.ErrorCodeProcessTimeout || outcome.TransportStatus != tool.TransportFailed || outcome.ProcessStatus != tool.ProcessTimedOut {
		t.Fatalf("timeout transport truth was lost: %+v", outcome)
	}
	if _, err := os.Lstat(filepath.Join(fixture.workspace, ".awaken")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("timed-out install escaped discarded staging: %v", err)
	}
}

func TestDependencyInstallCancellationDiscardsPartialStaging(t *testing.T) {
	fixture := newInstallFixture(t, `mkdir -p .awaken/deps/python
printf partial > .awaken/deps/python/partial.txt
sleep 5`, nil, nil)
	raw, _ := json.Marshal(input{PlanID: fixture.plan.PlanID})
	ctx, cancel := context.WithCancel(fixture.context())
	timer := time.AfterFunc(25*time.Millisecond, cancel)
	defer timer.Stop()
	outcome, err := (&Tool{adapter: fixture.adapter}).ExecuteOutcome(ctx, string(raw), nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != tool.OutcomeCancelled || outcome.ErrorCode != tool.ErrorCodeToolCancelled || outcome.TransportStatus != tool.TransportCancelled || outcome.ProcessStatus != tool.ProcessSignaled {
		t.Fatalf("cancel transport truth was lost: %+v", outcome)
	}
	if _, err := os.Lstat(filepath.Join(fixture.workspace, ".awaken")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled install escaped discarded staging: %v", err)
	}
}

func TestDependencyInstallExitZeroWithoutReadyProbeIsNotSuccess(t *testing.T) {
	fixture := newInstallFixture(t, `mkdir -p .awaken/deps/python
printf unrelated > .awaken/deps/python/unrelated.txt
exit 0`, nil, nil)
	raw, _ := json.Marshal(input{PlanID: fixture.plan.PlanID})
	outcome, err := (&Tool{adapter: fixture.adapter}).ExecuteOutcome(fixture.context(), string(raw), nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != tool.OutcomeNeedsRuntime || outcome.TransportStatus != tool.TransportCompleted || outcome.ProcessStatus != tool.ProcessExited || outcome.ErrorCode != tool.ErrorCodeNeedsRuntime {
		t.Fatalf("exit zero without verified dependency became false success: %+v", outcome)
	}
	if len(outcome.Changes) == 0 || outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectApplied {
		t.Fatalf("committed but unverified side effects were hidden: %+v", outcome)
	}
}

func TestDependencyInstallRejectsForgedStaleFreeInputAndManagerReplacementBeforeTransport(t *testing.T) {
	fixture := newInstallFixture(t, successfulManagerScript(), nil, nil)
	tests := []string{
		`{"plan_id":"install_plan_forged"}`,
		fmt.Sprintf(`{"plan_id":%q,"command":"curl attacker"}`, fixture.plan.PlanID),
		fmt.Sprintf(`{"plan_id":%q,"args":["evil"]}`, fixture.plan.PlanID),
		fmt.Sprintf(`{"plan_id":%q} {}`, fixture.plan.PlanID),
	}
	for _, raw := range tests {
		outcome, err := (&Tool{adapter: fixture.adapter}).ExecuteOutcome(fixture.context(), raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		if outcome.TransportStatus != tool.TransportNotStarted {
			t.Fatalf("invalid input started transport: raw=%s outcome=%+v", raw, outcome)
		}
	}

	staleEnvironment := fixture.environment.Clone()
	staleEnvironment.SnapshotID = "env_stale"
	staleCtx := tool.ContextWithRunAvailability(context.Background(), fixture.snapshot, staleEnvironment)
	staleCtx = tool.ContextWithPermissionContext(staleCtx, fixture.permission)
	staleCtx = tool.ContextWithExecutionBroker(staleCtx, fixture.broker)
	raw, _ := json.Marshal(input{PlanID: fixture.plan.PlanID})
	staleOutcome, _ := (&Tool{adapter: fixture.adapter}).ExecuteOutcome(staleCtx, string(raw), nil)
	if staleOutcome.TransportStatus != tool.TransportNotStarted {
		t.Fatalf("stale environment started transport: %+v", staleOutcome)
	}

	if err := os.WriteFile(fixture.manager, []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	replacedOutcome, _ := (&Tool{adapter: fixture.adapter}).ExecuteOutcome(fixture.context(), string(raw), nil)
	if replacedOutcome.TransportStatus != tool.TransportNotStarted || replacedOutcome.ErrorCode != "install_plan_invalid" {
		t.Fatalf("manager fingerprint replacement started transport: %+v", replacedOutcome)
	}
}

func TestDependencyInstallIsRepeatableAndTaskScopeProtected(t *testing.T) {
	fixture := newInstallFixture(t, successfulManagerScript(), []string{filepath.Join(".awaken", "deps")}, nil)
	if err := os.MkdirAll(filepath.Join(fixture.workspace, ".awaken", "deps"), 0o700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(input{PlanID: fixture.plan.PlanID})
	for iteration := 0; iteration < 2; iteration++ {
		outcome, err := (&Tool{adapter: fixture.adapter}).ExecuteOutcome(fixture.context(), string(raw), nil)
		if err != nil || outcome.Status != tool.OutcomeSucceeded {
			t.Fatalf("repeat %d failed: outcome=%+v err=%v", iteration, outcome, err)
		}
	}
	if _, err := os.Stat(filepath.Join(fixture.workspace, ".awaken", "deps", "python", "awaken_fake_dep.py")); err != nil {
		t.Fatalf("repeatable install lost committed dependency: %v", err)
	}

	protected := newInstallFixture(t, successfulManagerScript(), nil, []string{filepath.Join(".awaken", "deps", "python")})
	protectedRaw, _ := json.Marshal(input{PlanID: protected.plan.PlanID})
	outcome, _ := (&Tool{adapter: protected.adapter}).ExecuteOutcome(protected.context(), string(protectedRaw), nil)
	if outcome.Status == tool.OutcomeSucceeded || outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectNone {
		t.Fatalf("protected-root install was not denied at commit: %+v", outcome)
	}
	if _, err := os.Lstat(filepath.Join(protected.workspace, ".awaken")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("protected-root delta reached live workspace: %v", err)
	}
	protected.trash.mu.Lock()
	trashCalls := protected.trash.calls
	protected.trash.mu.Unlock()
	if trashCalls != 0 {
		t.Fatalf("protected-root denial invoked Trash %d time(s)", trashCalls)
	}

	outside := newInstallFixture(t, `printf outside > outside.txt
exit 0`, []string{filepath.Join(".awaken", "deps")}, nil)
	if err := os.MkdirAll(filepath.Join(outside.workspace, ".awaken", "deps"), 0o700); err != nil {
		t.Fatal(err)
	}
	outsideRaw, _ := json.Marshal(input{PlanID: outside.plan.PlanID})
	outsideOutcome, _ := (&Tool{adapter: outside.adapter}).ExecuteOutcome(outside.context(), string(outsideRaw), nil)
	if outsideOutcome.Status == tool.OutcomeSucceeded {
		t.Fatalf("outside-writable-root install was reported successful: %+v", outsideOutcome)
	}
	if _, err := os.Lstat(filepath.Join(outside.workspace, "outside.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside-writable-root delta reached live workspace: %v", err)
	}
	outside.trash.mu.Lock()
	outsideTrashCalls := outside.trash.calls
	outside.trash.mu.Unlock()
	if outsideTrashCalls != 0 {
		t.Fatalf("outside-writable-root denial invoked Trash %d time(s)", outsideTrashCalls)
	}
}

func TestDependencyInstallMinimalEnvironmentAndSplitSecretObserver(t *testing.T) {
	environment := installEnvironment([]string{
		"PATH=/usr/bin", "HOME=/Users/test", "HTTPS_PROXY=http://proxy", "NPM_TOKEN=npm-auth",
		"OPENAI_API_KEY=must-not-leak", "GITHUB_TOKEN=must-not-leak", "SLACK_TOKEN=must-not-leak",
	}, nil, map[string]string{"TMPDIR": "/private/stage"}, []string{"/pinned/bin", "/usr/bin"})
	joined := strings.Join(environment, "\n")
	if strings.Contains(joined, "OPENAI") || strings.Contains(joined, "GITHUB") || strings.Contains(joined, "SLACK") || !strings.Contains(joined, "HTTPS_PROXY") || !strings.Contains(joined, "NPM_TOKEN") || !strings.Contains(joined, "TMPDIR=/private/stage") || !strings.Contains(joined, "PATH=/pinned/bin"+string(os.PathListSeparator)+"/usr/bin") {
		t.Fatalf("dependency environment allowlist is wrong: %s", joined)
	}

	observer := &observedOutput{}
	writer := newBoundedOutput("stdout", observer)
	_, _ = writer.Write([]byte("https://alice:split-"))
	_, _ = writer.Write([]byte("secret@registry.invalid/?token=query-"))
	_, _ = writer.Write([]byte("secret"))
	observer.mu.Lock()
	if len(observer.values) != 0 {
		t.Fatalf("raw chunks were emitted before whole-output redaction: %+v", observer.values)
	}
	observer.mu.Unlock()
	value, truncated := writer.Result()
	writer.Emit(sanitizeOutput(value), truncated)
	observer.mu.Lock()
	streamed := fmt.Sprint(observer.values)
	observer.mu.Unlock()
	if strings.Contains(streamed, "split-secret") || strings.Contains(streamed, "query-secret") || !strings.Contains(streamed, "[REDACTED]") {
		t.Fatalf("split secret was not redacted before observer emission: %s", streamed)
	}
}

func TestDependencyInstallFiltersTaskControlledPATHFromEnvShebang(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses /usr/bin/env shebang semantics")
	}
	workspace := t.TempDir()
	temporaryRoot := t.TempDir()
	skillRoot := t.TempDir()
	externalBin := filepath.Join(t.TempDir(), "trusted-user-bin")
	workspaceBin := filepath.Join(workspace, "bin")
	for _, directory := range []string{externalBin, workspaceBin, temporaryRoot, skillRoot} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	marker := filepath.Join(workspace, "fake-node-executed")
	if err := os.WriteFile(filepath.Join(workspaceBin, "node"), []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	manager := filepath.Join(externalBin, "npm")
	if err := os.WriteFile(manager, []byte("#!/usr/bin/env sh\nnode --version >/dev/null 2>&1 || true\nprintf 'trusted external manager ran\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	environment := tool.ExecutionEnvironmentSnapshot{
		WorkspaceRoot: workspace, TemporaryWorkRoot: temporaryRoot,
		SkillRoots: []tool.SkillRootObservation{{Source: "additional", Path: skillRoot}},
		PATH:       []string{workspaceBin, temporaryRoot, skillRoot, externalBin, "/bin", "/usr/bin"},
	}
	permission := tool.NewTaskScopePermissionContext("filtered-install-path", workspace, []string{workspace}, []string{workspace}, nil, nil, nil, temporaryRoot)
	filtered := filteredInstallPATH(environment, permission)
	joined := strings.Join(filtered, string(os.PathListSeparator))
	for _, forbidden := range []string{workspaceBin, temporaryRoot, skillRoot} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("task-controlled PATH entry survived filtering: forbidden=%q filtered=%q", forbidden, joined)
		}
	}
	if !strings.Contains(joined, externalBin) {
		t.Fatalf("external user-installed package manager PATH was incorrectly removed: %q", joined)
	}
	cmd := exec.Command(manager, "--version")
	cmd.Env = installEnvironment(nil, nil, nil, filtered)
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "trusted external manager ran") {
		t.Fatalf("external /usr/bin/env manager did not run with filtered PATH: output=%q err=%v", output, err)
	}
	if _, err := os.Lstat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace fake node hijacked the manager shebang/PATH: %v", err)
	}
}

func TestValidatePinnedPlanRejectsTaskControlledManagerEvenWithMatchingFingerprint(t *testing.T) {
	fixture := newInstallFixture(t, successfulManagerScript(), nil, nil)
	resolution, err := fixture.snapshot.LookupCandidateInstallPlan(fixture.plan.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	manager := filepath.Join(fixture.workspace, "bin", "pip")
	if err := os.MkdirAll(filepath.Dir(manager), 0o700); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(fixture.manager)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager, bytes, 0o700); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := tool.FingerprintExecutable(manager)
	if err != nil {
		t.Fatal(err)
	}
	resolution.Plan.Executable = manager
	resolution.Plan.ExecutableFingerprint = fingerprint
	environment := fixture.environment.Clone()
	for index := range environment.Executables {
		if environment.Executables[index].Name == "pip" {
			environment.Executables[index].Path = manager
			environment.Executables[index].Fingerprint = fingerprint
		}
	}
	err = validatePinnedPlan(resolution, fixture.snapshot, environment, fixture.permission)
	if err == nil || !strings.Contains(err.Error(), "task-controlled") {
		t.Fatalf("matching fingerprint elevated a workspace manager into open-network execution: %v", err)
	}
}

func TestDependencyInstallFailsClosedWithoutFullSandbox(t *testing.T) {
	fixture := newInstallFixture(t, successfulManagerScript(), nil, nil)
	raw, _ := json.Marshal(input{PlanID: fixture.plan.PlanID})
	outcome, err := (&Tool{adapter: &degradedSandbox{}}).ExecuteOutcome(fixture.context(), string(raw), nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != tool.OutcomeUnsupported || outcome.TransportStatus != tool.TransportNotStarted || outcome.ErrorCode != tool.ErrorCodeSupervisedExecutionRequired {
		t.Fatalf("degraded sandbox did not fail closed: %+v", outcome)
	}
	if _, err := os.Lstat(filepath.Join(fixture.workspace, ".awaken")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("degraded sandbox started package manager: %v", err)
	}
}

func TestValidateManagerArgvRequiresNodeLifecycleScriptClosure(t *testing.T) {
	plan := tool.CandidateInstallPlan{Source: "npm", Args: []string{"install", "--save-exact", "--ignore-scripts", "--no-audit", "--no-fund", "safe-package@1.2.3"}}
	if err := validateManagerArgv(plan); err != nil {
		t.Fatalf("safe npm argv rejected: %v", err)
	}
	plan.Args = []string{"install", "--save-exact", "--no-audit", "--no-fund", "safe-package@1.2.3"}
	if err := validateManagerArgv(plan); err == nil {
		t.Fatal("npm argv without --ignore-scripts was accepted")
	}
}

func TestDependencyInstallToolRuntimeStrictSuccess(t *testing.T) {
	if sandbox.DefaultAdapter().SupportLevel() != sandbox.SandboxFull {
		t.Skip("strict dependency install requires a full platform sandbox")
	}
	fixture := newInstallFixture(t, successfulManagerScript(), nil, nil)
	runtime := agentcore.NewToolRuntimeWithConfig(nil, agentcore.ToolRuntimeConfig{
		PermissionContext: fixture.permission, ToolAvailabilitySnapshot: &fixture.snapshot,
		ExecutionEnvironmentSnapshot: &fixture.environment,
	})
	fixture.environment.PATH = []string{"/attacker/ambient/path"}
	ctx := tool.ContextWithExecutionBroker(context.Background(), fixture.broker)
	raw, _ := json.Marshal(input{PlanID: fixture.plan.PlanID})
	result := runtime.Execute(ctx, convcontext.ToolCall{ID: "strict_dependency_install", Function: convcontext.FunctionCall{Name: toolName, Arguments: string(raw)}})
	if result.IsError || result.Outcome.Status != tool.OutcomeSucceeded || result.ErrorCode != "" {
		t.Fatalf("strict ToolRuntime success was downgraded: %+v", result)
	}
	if strings.Contains(result.Outcome.Content, "/attacker/ambient/path") || !strings.Contains(result.Outcome.Content, filepath.Dir(fixture.manager)) {
		t.Fatalf("runtime did not retain pinned PATH for package-manager shebang: %s", result.Outcome.Content)
	}
	if result.Outcome.Diagnostic["tool_snapshot_id"] != fixture.snapshot.SnapshotID || result.Outcome.Diagnostic["environment_snapshot_id"] != fixture.environment.SnapshotID {
		t.Fatalf("runtime snapshot diagnostics were lost: %+v", result.Outcome.Diagnostic)
	}
}

func TestProviderReadinessNextActionUsesOnlyTypedPlanID(t *testing.T) {
	fixture := newInstallFixture(t, successfulManagerScript(), nil, nil)
	runtime := agentcore.NewToolRuntimeWithConfig(nil, agentcore.ToolRuntimeConfig{
		PermissionContext: fixture.permission, ToolAvailabilitySnapshot: &fixture.snapshot,
		ExecutionEnvironmentSnapshot: &fixture.environment,
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "needs_dependency", Function: convcontext.FunctionCall{Name: fixture.providerName, Arguments: `{}`},
	})
	if result.Outcome.Status != tool.OutcomeNeedsRuntime || result.Outcome.TransportStatus != tool.TransportNotStarted {
		t.Fatalf("unready provider did not stop before transport: %+v", result.Outcome)
	}
	var installAction *tool.NextAction
	for index := range result.Outcome.NextActions {
		if result.Outcome.NextActions[index].Tool == toolName {
			installAction = &result.Outcome.NextActions[index]
			break
		}
	}
	if installAction == nil || len(installAction.InputHint) != 1 || installAction.InputHint["plan_id"] != fixture.plan.PlanID {
		t.Fatalf("readiness did not route exact plan_id to typed installer: %+v", result.Outcome.NextActions)
	}
	for _, forbidden := range []string{"command", "args", "package", "url", "cwd"} {
		if _, exists := installAction.InputHint[forbidden]; exists {
			t.Fatalf("typed install action exposed free %s input: %+v", forbidden, installAction)
		}
	}
}

func TestRawShellCannotAuthorizeTypedDependencyInstallOperation(t *testing.T) {
	fixture := newInstallFixture(t, successfulManagerScript(), nil, nil)
	resolution, err := fixture.snapshot.LookupCandidateInstallPlan(fixture.plan.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := fixture.broker.Authorize(context.Background(), tool.ExecutionBrokerRequest{
		ProviderID: "builtin:shell", Capability: tool.CapabilityShellExecute,
		Operation: tool.BrokerOperationDependencyInstall, WorkspaceRoot: fixture.workspace, WorkingDirectory: fixture.workspace,
		EnvironmentSnapshotID: fixture.environment.SnapshotID,
		DependencyInstall:     &tool.BrokerDependencyInstallPlan{Plan: resolution.Plan, Requirement: resolution.Requirement, Providers: resolution.Providers, ToolSnapshotID: resolution.ToolSnapshotID},
	})
	if !errors.Is(err, tool.ErrExecutionBrokerDenied) || authorization.Action != tool.BrokerDeny {
		t.Fatalf("raw shell acquired typed dependency broker operation: auth=%+v err=%v", authorization, err)
	}
}
