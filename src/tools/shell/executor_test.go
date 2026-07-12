package shelltool

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"awaken/src/tool"
	"awaken/src/tool/profile"
	"awaken/src/tool/sandbox"
)

// fullSandboxMock is a test-only boundary that reports Applied=true. Tests use
// relative staging paths only; this mock is not a production confinement.
type fullSandboxMock struct {
	applied int
}

func (m *fullSandboxMock) SupportLevel() sandbox.SandboxSupport {
	return sandbox.SandboxFull
}

func (m *fullSandboxMock) ApplySandbox(cmd *exec.Cmd, tier string, workspaceRoot string) (sandbox.SandboxResult, error) {
	m.applied++
	return sandbox.SandboxResult{Support: sandbox.SandboxFull, Method: "mock-full", Applied: true}, nil
}

// noneSandboxMock simulates a platform with no isolation at all.
type noneSandboxMock struct{}

func (noneSandboxMock) SupportLevel() sandbox.SandboxSupport {
	return sandbox.SandboxNone
}

func (noneSandboxMock) ApplySandbox(cmd *exec.Cmd, tier string, workspaceRoot string) (sandbox.SandboxResult, error) {
	return sandbox.SandboxResult{Support: sandbox.SandboxNone, Method: "mock-none", Applied: false}, nil
}

// degradedSandboxMock is a SandboxAdapter that claims SandboxDegraded
// support, simulating the macOS/Windows placeholder adapters without
// depending on the platform-specific types (which are behind build tags).
type degradedSandboxMock struct{}

func (degradedSandboxMock) SupportLevel() sandbox.SandboxSupport {
	return sandbox.SandboxDegraded
}

func (degradedSandboxMock) ApplySandbox(cmd *exec.Cmd, tier string, workspaceRoot string) (sandbox.SandboxResult, error) {
	return sandbox.SandboxResult{Support: sandbox.SandboxDegraded, Method: "mock-degraded", Applied: false}, nil
}

type allowExecutionBroker struct {
	requests          []tool.ExecutionBrokerRequest
	completions       int
	requiresSandbox   bool
	enforcesWorkspace bool
}

func (b *allowExecutionBroker) ID() string { return "test_supervised_broker" }
func (b *allowExecutionBroker) EnforcesWorkspaceScope() bool {
	return b.enforcesWorkspace
}

func (b *allowExecutionBroker) Authorize(_ context.Context, request tool.ExecutionBrokerRequest) (tool.BrokerAuthorization, error) {
	b.requests = append(b.requests, request)
	return tool.BrokerAuthorization{
		AuthorizationID:        "test-authorization",
		Action:                 tool.BrokerAllow,
		EnforcementMode:        "test_staging_overlay",
		Evidence:               []string{"test_overlay_receipt"},
		RequiresAppliedSandbox: b.requiresSandbox,
	}, nil
}

func (b *allowExecutionBroker) Complete(_ context.Context, authorization tool.BrokerAuthorization, _ tool.BrokerExecutionResult) (tool.BrokerExecutionReceipt, error) {
	b.completions++
	return tool.BrokerExecutionReceipt{
		AuthorizationID: authorization.AuthorizationID,
		BrokerID:        b.ID(),
		EnforcementMode: authorization.EnforcementMode,
		Action:          authorization.Action,
		Evidence:        append([]string(nil), authorization.Evidence...),
	}, nil
}

// TestShellExecutorInspectReadOnlyExecutes verifies that inspect_read_only
// commands (e.g. `ls`) run successfully through the executor even when the
// platform sandbox is degraded (the default on macOS).
func TestShellExecutorInspectReadOnlyExecutes(t *testing.T) {
	t.Cleanup(func() { sandbox.SetAdapter(nil) })

	root := t.TempDir()
	permCtx := tool.NewPermissionContext(root, nil)
	executor := NewShellExecutor()

	result, err := executor.Execute(
		context.Background(),
		shellParams{Command: "ls"},
		permCtx,
		nil,
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", result.ExitCode, result.Stderr)
	}
	if result.Tier != ShellTierInspectReadOnly {
		t.Fatalf("expected tier %q, got %q", ShellTierInspectReadOnly, result.Tier)
	}
}

func TestShellExecutorLocalCLIRunsDirectlyAsCurrentUser(t *testing.T) {
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	sandbox.SetAdapter(degradedSandboxMock{})

	root := t.TempDir()
	cliPath := filepath.Join(root, "custom-cli")
	if err := os.WriteFile(cliPath, []byte("#!/bin/sh\nprintf local-cli-ok\n"), 0755); err != nil {
		t.Fatalf("failed to write local cli: %v", err)
	}

	permCtx := tool.NewPermissionContext(root, nil)
	executor := NewShellExecutor()

	result, err := executor.Execute(
		context.Background(),
		shellParams{Command: "./custom-cli"},
		permCtx,
		nil,
	)
	if err != nil || result.ExitCode != 0 || result.Stdout != "local-cli-ok" {
		t.Fatalf("expected current-user direct execution, result=%+v err=%v", result, err)
	}
	if result.Tier != ShellTierLocalCLI {
		t.Fatalf("expected tier %q, got %q", ShellTierLocalCLI, result.Tier)
	}
	if result.BrokerReceipt.BrokerID != "local_user_direct_v1" || result.BrokerReceipt.SideEffectState != tool.OutcomeSideEffectUnknown {
		t.Fatalf("direct provider must report unobserved effects honestly: %+v", result.BrokerReceipt)
	}
}

// TestShellExecutorWorkspaceMutationExecutes verifies that workspace_mutation
// commands (e.g. `touch file`) run successfully through the executor even
// when the platform sandbox is degraded.
func TestShellExecutorWorkspaceMutationRunsDirectlyWithoutOverlay(t *testing.T) {
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	sandbox.SetAdapter(degradedSandboxMock{})

	root := t.TempDir()
	permCtx := tool.NewPermissionContext(root, nil)
	executor := NewShellExecutor()

	result, err := executor.Execute(
		context.Background(),
		shellParams{Command: "touch created.txt"},
		permCtx,
		nil,
	)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("expected direct mutation, result=%+v err=%v", result, err)
	}
	if result.Tier != ShellTierWorkspaceMutation {
		t.Fatalf("expected tier %q, got %q", ShellTierWorkspaceMutation, result.Tier)
	}
	if _, statErr := os.Stat(filepath.Join(root, "created.txt")); statErr != nil {
		t.Fatalf("workspace mutation did not reach the real directory: %v", statErr)
	}
	if result.SandboxSupport != sandbox.SandboxNone || result.BrokerReceipt.CommitStatus != "side_effects_unobserved" {
		t.Fatalf("direct execution fabricated sandbox/commit evidence: %+v", result)
	}
}

func TestShellExecutorUsesAbsoluteWorkdirOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	userDirectory := t.TempDir()
	result, err := NewShellExecutor().Execute(
		context.Background(),
		shellParams{Command: "printf outside-ok > initialized.txt; pwd", Workdir: userDirectory},
		tool.NewPermissionContext(workspace, []string{workspace}), nil,
	)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("outside-workspace current-user command failed: result=%+v err=%v", result, err)
	}
	if got := resolveTestPath(t, strings.TrimSpace(result.Stdout)); got != resolveTestPath(t, userDirectory) {
		t.Fatalf("command did not use requested user directory: got=%q want=%q", got, userDirectory)
	}
	if content, readErr := os.ReadFile(filepath.Join(userDirectory, "initialized.txt")); readErr != nil || string(content) != "outside-ok" {
		t.Fatalf("outside-workspace write failed: content=%q err=%v", content, readErr)
	}
	if result.BrokerReceipt.BrokerID != "local_user_direct_v1" || result.BrokerReceipt.SideEffectState != tool.OutcomeSideEffectUnknown {
		t.Fatalf("unexpected direct provider truth: %+v", result.BrokerReceipt)
	}
}

func TestShellExecutorRunsNPMInstallScriptInUserSkillDirectory(t *testing.T) {
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skipf("npm unavailable: %v", err)
	}
	workspace := t.TempDir()
	skillRoot := t.TempDir()
	packageJSON := `{"name":"awaken-shell-install-smoke","version":"1.0.0","private":true,"scripts":{"install":"node -e \"require('fs').writeFileSync('install-ran.txt','ok')\""}}`
	if err := os.WriteFile(filepath.Join(skillRoot, "package.json"), []byte(packageJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := NewShellExecutor().Execute(
		context.Background(),
		shellParams{Command: "npm install --no-audit --no-fund", Workdir: skillRoot, TimeoutMS: 120000},
		tool.NewPermissionContext(workspace, []string{workspace}), nil,
	)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("npm install lifecycle failed: result=%+v err=%v", result, err)
	}
	if content, readErr := os.ReadFile(filepath.Join(skillRoot, "install-ran.txt")); readErr != nil || string(content) != "ok" {
		t.Fatalf("npm install script did not write its user Skill directory: content=%q err=%v", content, readErr)
	}
	if result.BrokerReceipt.BrokerID != "local_user_direct_v1" || len(result.BrokerReceipt.WorkspaceChanges) != 0 {
		t.Fatalf("npm direct execution fabricated staged changes: %+v", result.BrokerReceipt)
	}
}

func TestShellExecutorAllowsRealRawNetworkConnection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("curl invocation differs on Windows")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skipf("curl unavailable: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("network-ok"))
	}))
	defer server.Close()
	result, err := NewShellExecutor().Execute(
		context.Background(),
		shellParams{Command: "curl --noproxy '*' --silent --show-error " + server.URL},
		tool.NewPermissionContext(t.TempDir(), nil), nil,
	)
	if err != nil || result.ExitCode != 0 || result.Stdout != "network-ok" {
		t.Fatalf("raw host network did not reach local recipient: result=%+v err=%v", result, err)
	}
	if result.SandboxSupport != sandbox.SandboxNone || result.BrokerReceipt.SideEffectState != tool.OutcomeSideEffectUnknown {
		t.Fatalf("network execution fabricated containment/observation: %+v", result)
	}
}

func TestShellExecutorWorkspaceMutationExecutesWithFullSandboxBroker(t *testing.T) {
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	mock := &fullSandboxMock{}
	sandbox.SetAdapter(mock)
	root := t.TempDir()
	broker, _ := newShellTestWorkspaceBroker(t)
	ctx := tool.ContextWithExecutionBroker(context.Background(), broker)
	result, err := NewShellExecutor().Execute(
		ctx, shellParams{Command: "mkdir -p generated && printf hello > generated/file.txt"},
		tool.NewPermissionContext(root, nil), nil,
	)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("workspace broker did not execute mutation: result=%+v err=%v", result, err)
	}
	content, readErr := os.ReadFile(filepath.Join(root, "generated", "file.txt"))
	if readErr != nil || string(content) != "hello" {
		t.Fatalf("workspace mutation was not committed: content=%q err=%v", content, readErr)
	}
	if result.BrokerReceipt.BrokerID != "workspace_staging_trash_commit_v1" || result.BrokerReceipt.CommitStatus != "committed" || mock.applied != 1 {
		t.Fatalf("missing staging/sandbox/commit evidence: %+v applied=%d", result, mock.applied)
	}
}

func TestShellExecutorWorkspaceBrokerRejectsUnappliedSandbox(t *testing.T) {
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	sandbox.SetAdapter(degradedSandboxMock{})
	root := t.TempDir()
	broker, _ := newShellTestWorkspaceBroker(t)
	ctx := tool.ContextWithExecutionBroker(context.Background(), broker)
	result, err := NewShellExecutor().Execute(
		ctx, shellParams{Command: "touch escaped.txt"}, tool.NewPermissionContext(root, nil), nil,
	)
	if !errors.Is(err, tool.ErrExecutionSandboxRequired) {
		t.Fatalf("workspace broker must reject unapplied sandbox: result=%+v err=%v", result, err)
	}
	if result.BrokerReceipt.CommitStatus != "discarded_not_completed" {
		t.Fatalf("staging workspace was not discarded: %+v", result.BrokerReceipt)
	}
	if _, statErr := os.Lstat(filepath.Join(root, "escaped.txt")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("unapplied sandbox reached live workspace: %v", statErr)
	}
}

func TestShellExecutorCancellationDiscardsStagedMutation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows production broker remains fail closed until Job Object containment exists")
	}
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	sandbox.SetAdapter(&fullSandboxMock{})
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	broker, adapter := newShellTestWorkspaceBroker(t)
	ctx, cancel := context.WithCancel(tool.ContextWithExecutionBroker(context.Background(), broker))
	time.AfterFunc(100*time.Millisecond, cancel)
	result, err := NewShellExecutor().Execute(
		ctx, shellParams{Command: "printf changed > target.txt; sleep 5", TimeoutMS: 10000},
		tool.NewPermissionContext(root, nil), nil,
	)
	if err != nil {
		t.Fatalf("cancelled process should return a process result: result=%+v err=%v", result, err)
	}
	if !result.Cancelled || result.BrokerReceipt.CommitStatus != "discarded_not_completed" {
		t.Fatalf("cancel did not discard staging: %+v", result)
	}
	content, readErr := os.ReadFile(target)
	if readErr != nil || string(content) != "original" {
		t.Fatalf("cancelled staging mutation reached workspace: content=%q err=%v", content, readErr)
	}
	if adapter.calls != 0 {
		t.Fatalf("cancelled execution should not create a Trash recovery bundle, calls=%d", adapter.calls)
	}
}

// The default provider does not apply an available sandbox implicitly. Network
// and package-manager commands run with the current user's ambient access.
func TestShellExecutorOpenWorldRunsDirectRegardlessOfSandboxSupport(t *testing.T) {
	adapters := map[string]sandbox.SandboxAdapter{
		"none": noneSandboxMock{}, "degraded": degradedSandboxMock{}, "full": &fullSandboxMock{},
	}
	for name, adapter := range adapters {
		t.Run(name, func(t *testing.T) {
			t.Cleanup(func() { sandbox.SetAdapter(nil) })
			sandbox.SetAdapter(adapter)
			root := t.TempDir()
			writeFakeCurl(t, root)
			result, err := NewShellExecutor().Execute(
				context.Background(), shellParams{Command: "./curl --version"},
				tool.NewPermissionContext(root, nil), nil,
			)
			if err != nil || result.ExitCode != 0 || result.Stdout != "fake-curl" {
				t.Fatalf("expected direct host execution, result=%+v err=%v", result, err)
			}
			if result.Tier != ShellTierOpenWorldSystem {
				t.Fatalf("expected tier %q, got %q", ShellTierOpenWorldSystem, result.Tier)
			}
			if result.SandboxSupport != sandbox.SandboxNone || result.BrokerReceipt.BrokerID != "local_user_direct_v1" {
				t.Fatalf("default provider must not claim sandbox application: %+v", result)
			}
		})
	}
}

// An explicitly injected broker remains responsible for its own egress
// contract; this test only verifies the Shell executor honors that host-owned
// authorization and applies the requested sandbox adapter.
func TestShellExecutorSupervisingBrokerCanAuthorizeLocalCLI(t *testing.T) {
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	mock := &fullSandboxMock{}
	sandbox.SetAdapter(mock)

	root := t.TempDir()
	writeFakeCurl(t, root)
	permCtx := tool.NewPermissionContext(root, nil)
	executor := NewShellExecutor()
	broker := &allowExecutionBroker{requiresSandbox: true}
	ctx := tool.ContextWithExecutionBroker(context.Background(), broker)

	result, err := executor.Execute(
		ctx,
		shellParams{Command: "./curl --version"},
		permCtx,
		nil,
	)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result.ExitCode != 0 || result.Stdout != "fake-curl" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Tier != ShellTierOpenWorldSystem {
		t.Fatalf("expected tier %q, got %q", ShellTierOpenWorldSystem, result.Tier)
	}
	if result.SandboxSupport != sandbox.SandboxFull {
		t.Fatalf("expected SandboxFull support, got %q", result.SandboxSupport)
	}
	if mock.applied != 1 {
		t.Fatalf("expected one sandbox application for supervised shell, got %d", mock.applied)
	}
	if len(broker.requests) != 1 || broker.completions != 1 {
		t.Fatalf("expected one broker authorization and receipt, requests=%d completions=%d", len(broker.requests), broker.completions)
	}
	if result.BrokerReceipt.BrokerID != broker.ID() || result.BrokerAuthorization.EnforcementMode != "test_staging_overlay" {
		t.Fatalf("expected broker facts on result, got authorization=%+v receipt=%+v", result.BrokerAuthorization, result.BrokerReceipt)
	}
}

func TestShellExecutorPassesCanonicalPermissionScopeToBroker(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated Windows privileges")
	}
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	sandbox.SetAdapter(&fullSandboxMock{})
	workspace := t.TempDir()
	for _, directory := range []string{"generated", "protected", "output-only"} {
		if err := os.MkdirAll(filepath.Join(workspace, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("generated", filepath.Join(workspace, "generated-alias")); err != nil {
		t.Fatal(err)
	}
	permissionContext := tool.NewTaskScopePermissionContext(
		"scope", workspace, nil, []string{filepath.Join(workspace, "generated-alias")},
		[]string{filepath.Join(workspace, "output-only")}, nil,
		[]string{filepath.Join(workspace, "protected")}, filepath.Join(t.TempDir(), "scratch"),
	)
	broker := &allowExecutionBroker{enforcesWorkspace: true}
	ctx := tool.ContextWithExecutionBroker(context.Background(), broker)
	result, err := NewShellExecutor().Execute(ctx, shellParams{Command: "pwd"}, permissionContext, nil)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("shell inspection failed: result=%+v err=%v", result, err)
	}
	if len(broker.requests) != 1 {
		t.Fatalf("expected one broker request, got %d", len(broker.requests))
	}
	request := broker.requests[0]
	if !request.WorkspaceScopeDeclared || len(request.WritableRoots) != 1 || request.WritableRoots[0] != "generated" {
		t.Fatalf("canonical writable roots missing from request: %+v", request)
	}
	if len(request.ProtectedRoots) != 1 || request.ProtectedRoots[0] != "protected" {
		t.Fatalf("canonical protected roots missing from request: %+v", request)
	}
	for _, root := range request.WritableRoots {
		if root == "output-only" {
			t.Fatalf("OutputRoots must not widen WritableRoots: %+v", request)
		}
	}
}

// An injected adapter is capability, not policy: the default direct provider
// does not apply it unless an explicit broker requests a sandbox.
func TestShellExecutorDoesNotApplyUnrequestedAdapter(t *testing.T) {
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	// Set a degraded global adapter to prove the constructor bypasses it.
	sandbox.SetAdapter(degradedSandboxMock{})

	mock := &fullSandboxMock{}
	executor := NewShellExecutorWithAdapter(mock)

	root := t.TempDir()
	permCtx := tool.NewPermissionContext(root, nil)

	result, err := executor.Execute(
		context.Background(),
		shellParams{Command: "pwd"},
		permCtx,
		nil,
	)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result.SandboxSupport != sandbox.SandboxNone || mock.applied != 0 {
		t.Fatalf("unrequested adapter must remain unapplied: result=%+v applied=%d", result, mock.applied)
	}
}

func TestShellExecutorExplicitNilBrokerFailsClosed(t *testing.T) {
	ctx := tool.ContextWithExecutionBroker(context.Background(), nil)
	result, err := NewShellExecutor().Execute(
		ctx,
		shellParams{Command: "pwd"},
		tool.NewPermissionContext(t.TempDir(), nil),
		nil,
	)
	if !errors.Is(err, tool.ErrExecutionBrokerUnavailable) {
		t.Fatalf("expected unavailable broker error, result=%+v err=%v", result, err)
	}
}

// TestShellExecutorResolvesWorkdir verifies that the executor resolves the
// working directory against the permission context, both for explicit
// workdir and for the default (workspace root).
func TestShellExecutorResolvesWorkdir(t *testing.T) {
	t.Cleanup(func() { sandbox.SetAdapter(nil) })

	root := t.TempDir()
	permCtx := tool.NewPermissionContext(root, nil)
	executor := NewShellExecutor()

	// Default: runs from workspace root.
	result, err := executor.Execute(
		context.Background(),
		shellParams{Command: "pwd"},
		permCtx,
		nil,
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	expectedRoot := resolveTestPath(t, root)
	gotRoot := resolveTestPath(t, strings.TrimSpace(result.Stdout))
	if gotRoot != expectedRoot {
		t.Fatalf("expected pwd to run from workspace root %q, got %q", expectedRoot, gotRoot)
	}
}

// TestShellExecutorEmptyCommandReturnsError verifies that an empty command
// is rejected before any sandbox or execution logic runs.
func TestShellExecutorEmptyCommandReturnsError(t *testing.T) {
	t.Cleanup(func() { sandbox.SetAdapter(nil) })

	permCtx := tool.NewPermissionContext(t.TempDir(), nil)
	executor := NewShellExecutor()

	_, err := executor.Execute(
		context.Background(),
		shellParams{Command: "   "},
		permCtx,
		nil,
	)
	if err == nil {
		t.Fatal("expected error for empty command, got nil")
	}
	if !strings.Contains(err.Error(), "command") {
		t.Fatalf("expected error to mention 'command', got %q", err.Error())
	}
}

// TestShellExecutorNonZeroExitIsReturned verifies that a non-zero exit code
// is captured in ShellExecResult.ExitCode rather than returned as an error.
func TestShellExecutorNonZeroExitIsReturned(t *testing.T) {
	t.Cleanup(func() { sandbox.SetAdapter(nil) })

	permCtx := tool.NewPermissionContext(t.TempDir(), nil)
	executor := NewShellExecutor()

	result, err := executor.Execute(
		context.Background(),
		shellParams{Command: "exit 7"},
		permCtx,
		nil,
	)
	if err != nil {
		t.Fatalf("Execute returned error for non-zero exit: %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("expected exit code 7, got %d", result.ExitCode)
	}
}

// TestShellExecutorTimeout verifies that a command that exceeds its timeout
// is reported as timed out with exit code -1.
func TestShellExecutorTimeout(t *testing.T) {
	t.Cleanup(func() { sandbox.SetAdapter(nil) })

	permCtx := tool.NewPermissionContext(t.TempDir(), nil)
	executor := NewShellExecutor()

	result, err := executor.Execute(
		context.Background(),
		shellParams{Command: "sleep 1", TimeoutMS: 10},
		permCtx,
		nil,
	)
	if err != nil {
		t.Fatalf("Execute returned error for timeout: %v", err)
	}
	if !result.TimedOut {
		t.Fatal("expected TimedOut=true")
	}
	if result.ExitCode != -1 {
		t.Fatalf("expected exit code -1 for timeout, got %d", result.ExitCode)
	}
}

// TestShellExecutorEnvMinimalWhitelistsEnv verifies that when the context
// carries EnvPolicy=EnvMinimal, the shell command's environment is
// constrained to the minimal allowlist (PATH, HOME, USER, SHELL, TMPDIR,
// LANG, LC_ALL). A secret set via t.Setenv must NOT appear in the child
// process's environment.
//
// Note: /bin/sh itself adds a few intrinsic variables (PWD, SHLVL, _) that
// are not inherited from the parent — those are shell behavior, not env
// leakage. The test focuses on the security-relevant check: the parent's
// non-allowlisted variables must not reach the child.
func TestShellExecutorEnvMinimalWhitelistsEnv(t *testing.T) {
	t.Cleanup(func() { sandbox.SetAdapter(nil) })

	// Set a secret in the test process's environment. t.Setenv restores
	// the original value when the test ends.
	t.Setenv("AWAKEN_TEST_SECRET", "sensitive-value")

	permCtx := tool.NewPermissionContext(t.TempDir(), nil)
	executor := NewShellExecutor()

	ctx := profile.WithEnvPolicy(context.Background(), profile.EnvMinimal)
	result, err := executor.Execute(
		ctx,
		shellParams{Command: "env"},
		permCtx,
		nil,
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", result.ExitCode, result.Stderr)
	}

	// The secret must NOT appear in the child's environment.
	if strings.Contains(result.Stdout, "AWAKEN_TEST_SECRET") {
		t.Fatalf("EnvMinimal leaked AWAKEN_TEST_SECRET into child environment:\n%s", result.Stdout)
	}

	// Allowlisted variables that are set in the parent SHOULD appear.
	// PATH is always set in the test environment, so it must survive the
	// allowlist filter.
	if !strings.Contains(result.Stdout, "PATH=") {
		t.Fatalf("EnvMinimal should preserve PATH, got stdout:\n%s", result.Stdout)
	}
}

// TestShellExecutorEnvInheritExposesAllEnv verifies that when the context
// carries EnvPolicy=EnvInherit (or no EnvPolicy), the shell command
// inherits the full parent environment, including the test secret.
func TestShellExecutorEnvInheritExposesAllEnv(t *testing.T) {
	t.Cleanup(func() { sandbox.SetAdapter(nil) })

	t.Setenv("AWAKEN_TEST_SECRET", "sensitive-value")

	permCtx := tool.NewPermissionContext(t.TempDir(), nil)
	executor := NewShellExecutor()

	// No EnvPolicy in context → defaults to EnvInherit.
	result, err := executor.Execute(
		context.Background(),
		shellParams{Command: "env"},
		permCtx,
		nil,
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", result.ExitCode, result.Stderr)
	}
	if !strings.Contains(result.Stdout, "AWAKEN_TEST_SECRET=sensitive-value") {
		t.Fatalf("EnvInherit should expose AWAKEN_TEST_SECRET, got stdout:\n%s", result.Stdout)
	}
}

func TestShellExecutorDefaultEnvironmentPreservesHostCanariesAndHomebrewPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell canary syntax")
	}
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	home := t.TempDir()
	cache := filepath.Join(home, "cache-canary")
	cert := filepath.Join(home, "cert-canary.pem")
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("HTTPS_PROXY", "http://proxy-canary.invalid:18443")
	t.Setenv("SSL_CERT_FILE", cert)
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("PUPPETEER_CACHE_DIR", filepath.Join(cache, "puppeteer"))

	command := `test "$HOME" = '` + home + `' && ` +
		`test "$HTTPS_PROXY" = 'http://proxy-canary.invalid:18443' && ` +
		`test "$SSL_CERT_FILE" = '` + cert + `' && ` +
		`test "$XDG_CACHE_HOME" = '` + cache + `' && ` +
		`test "$PUPPETEER_CACHE_DIR" = '` + filepath.Join(cache, "puppeteer") + `' && ` +
		`case ":$PATH:" in *":/opt/homebrew/bin:"*) ;; *) exit 23 ;; esac && printf environment-ok`
	result, err := NewShellExecutor().Execute(
		context.Background(), shellParams{Command: command},
		tool.NewPermissionContext(t.TempDir(), nil), nil,
	)
	if err != nil || result.ExitCode != 0 || result.Stdout != "environment-ok" {
		t.Fatalf("default shell did not preserve host environment canaries: result=%+v err=%v", result, err)
	}
}

func TestEssentialGUIEnvironmentFallbackPreservesExistingValues(t *testing.T) {
	environment := ensureEssentialShellEnvironmentForOS([]string{
		"HOME=", "PATH=", "HTTPS_PROXY=http://proxy-canary.invalid:18443",
		"SSL_CERT_FILE=/cert-canary.pem", "XDG_CACHE_HOME=/cache-canary",
	}, "darwin", "/Users/gui-user", "/tmp/gui-user")
	for key, want := range map[string]string{
		"HOME": "/Users/gui-user", "TMPDIR": "/tmp/gui-user", "SHELL": "/bin/sh",
		"PATH":          "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
		"HTTPS_PROXY":   "http://proxy-canary.invalid:18443",
		"SSL_CERT_FILE": "/cert-canary.pem", "XDG_CACHE_HOME": "/cache-canary",
	} {
		if got := envValueForOS(environment, key, "darwin"); got != want {
			t.Fatalf("%s=%q, want %q", key, got, want)
		}
	}
}

func TestShellExecutorRepairsUnsetHomeForActualChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell environment assertion")
	}
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	fallbackHome, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(fallbackHome) == "" {
		t.Skipf("current user home is unavailable: %v", err)
	}
	t.Setenv("HOME", "")

	result, err := NewShellExecutor().Execute(
		context.Background(),
		shellParams{Command: `printf 'home=%s\npath=%s\nshell=%s' "$HOME" "$PATH" "$SHELL"`},
		tool.NewPermissionContext(t.TempDir(), nil),
		nil,
	)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("actual child failed with HOME unset: result=%+v err=%v", result, err)
	}
	wantPATH := result.EnvironmentSnapshot.PinnedTransportPATH()
	want := "home=" + fallbackHome + "\npath=" + wantPATH + "\nshell=" + result.EnvironmentSnapshot.Shell
	if result.Stdout != want {
		t.Fatalf("actual child environment drifted from repaired snapshot:\n got %q\nwant %q", result.Stdout, want)
	}
}

func TestShellExecutorDoesNotExecuteImplicitBashEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX bash startup hook fixture")
	}
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skipf("bash unavailable: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "implicit-rc-ran")
	hook := filepath.Join(t.TempDir(), "bash-env")
	if err := os.WriteFile(hook, []byte("printf sourced > '"+marker+"'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", bashPath)
	t.Setenv("BASH_ENV", hook)

	result, err := NewShellExecutor().Execute(
		context.Background(), shellParams{Command: "printf no-rc"},
		tool.NewPermissionContext(t.TempDir(), nil), nil,
	)
	if err != nil || result.ExitCode != 0 || result.Stdout != "no-rc" {
		t.Fatalf("bash no-rc transport failed: result=%+v err=%v", result, err)
	}
	if _, err := os.Lstat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("BASH_ENV executed implicitly: %v", err)
	}
}

func TestShellExecutorUsesPinnedSnapshotShellAndPATHForActualChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture")
	}
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	fixtureRoot := t.TempDir()
	binDir := filepath.Join(fixtureRoot, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	canaryPath := filepath.Join(binDir, "awaken-path-canary")
	if err := os.WriteFile(canaryPath, []byte("#!/bin/sh\nprintf 'canary-ok\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	shellDir := filepath.Join(fixtureRoot, "pinned-shell")
	if err := os.MkdirAll(shellDir, 0o700); err != nil {
		t.Fatal(err)
	}
	shellPath := filepath.Join(shellDir, "zsh")
	if err := os.WriteFile(shellPath, []byte("#!/bin/sh\ntest \"$1\" = -f && shift\nexport AWAKEN_ACTUAL_SHELL=pinned\nexec /bin/sh \"$@\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	pinnedPATH := strings.Join([]string{binDir, "/usr/bin", "/bin"}, string(os.PathListSeparator))
	snapshot := tool.ProbeExecutionEnvironment(context.Background(), tool.ExecutionEnvironmentProbeOptions{
		PATH: pinnedPATH, PATHSource: "pinned_test", Shell: shellPath,
		Executables: []string{}, PackageManagers: []string{}, RefreshReason: "pinned_transport_test",
	})
	// The process PATH intentionally excludes the fixture. Only reconstructing
	// the child from the pinned snapshot can find the canary.
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("SHELL", "/bin/sh")
	ctx := tool.ContextWithExecutionEnvironmentSnapshot(context.Background(), snapshot)
	result, err := NewShellExecutor().Execute(
		ctx,
		shellParams{Command: `awaken-path-canary && printf 'shell=%s\npath=%s' "$AWAKEN_ACTUAL_SHELL" "$PATH"`},
		tool.NewPermissionContext(t.TempDir(), nil),
		nil,
	)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("pinned child failed: result=%+v err=%v", result, err)
	}
	want := "canary-ok\nshell=pinned\npath=" + snapshot.PinnedTransportPATH()
	if result.Stdout != want {
		t.Fatalf("pinned snapshot did not drive actual child shell/PATH:\n got %q\nwant %q", result.Stdout, want)
	}
	if result.EnvironmentSnapshot.SnapshotID != snapshot.SnapshotID {
		t.Fatalf("executor replaced pinned snapshot: got %q want %q", result.EnvironmentSnapshot.SnapshotID, snapshot.SnapshotID)
	}
}

func TestShellExecutorPreservesFullPinnedPATHBeyondSnapshotPreview(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture")
	}
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	fixtureRoot := t.TempDir()
	entries := make([]string, 0, 130)
	for index := 0; index < 129; index++ {
		entries = append(entries, filepath.Join(string(os.PathSeparator), fmt.Sprintf("awaken-path-%03d", index)))
	}
	tailDir := filepath.Join(fixtureRoot, "tail-bin")
	if err := os.MkdirAll(tailDir, 0o700); err != nil {
		t.Fatal(err)
	}
	entries = append(entries, tailDir)
	canaryPath := filepath.Join(tailDir, "awaken-tail-canary")
	if err := os.WriteFile(canaryPath, []byte("#!/bin/sh\nprintf tail-ok\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	fullPATH := strings.Join(entries, string(os.PathListSeparator))
	fixedNow := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	snapshot := tool.ProbeExecutionEnvironment(context.Background(), tool.ExecutionEnvironmentProbeOptions{
		PATH: fullPATH, PATHSource: "long_pinned_test", Shell: "/bin/sh",
		Executables: []string{}, PackageManagers: []string{}, RefreshReason: "long_pinned_transport_test", Now: func() time.Time { return fixedNow },
	})
	if !snapshot.PATHTruncated || len(snapshot.PATH) != 128 || snapshot.PATHEntryCount != len(entries) {
		t.Fatalf("long PATH metadata is not bounded and explicit: preview=%d count=%d truncated=%v", len(snapshot.PATH), snapshot.PATHEntryCount, snapshot.PATHTruncated)
	}
	if snapshot.PinnedTransportPATH() != fullPATH || containsString(snapshot.PATH, tailDir) {
		t.Fatalf("full transport PATH was not retained separately from preview")
	}
	otherEntries := append([]string(nil), entries...)
	otherEntries[len(otherEntries)-1] = filepath.Join(fixtureRoot, "different-tail")
	other := tool.ProbeExecutionEnvironment(context.Background(), tool.ExecutionEnvironmentProbeOptions{
		PATH: strings.Join(otherEntries, string(os.PathListSeparator)), PATHSource: "long_pinned_test", Shell: "/bin/sh",
		Executables: []string{}, PackageManagers: []string{}, RefreshReason: "long_pinned_transport_test", Now: func() time.Time { return fixedNow },
	})
	if snapshot.SnapshotID == other.SnapshotID || snapshot.PATHDigest == other.PATHDigest {
		t.Fatalf("snapshot identity did not bind the non-preview PATH tail")
	}

	t.Setenv("PATH", "/usr/bin:/bin")
	ctx := tool.ContextWithExecutionEnvironmentSnapshot(context.Background(), snapshot)
	result, err := NewShellExecutor().Execute(
		ctx,
		shellParams{Command: `awaken-tail-canary && printf '\npath=%s' "$PATH"`, MaxOutputChars: maxOutputCharsLimit},
		tool.NewPermissionContext(t.TempDir(), nil),
		nil,
	)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("long pinned PATH child failed: result=%+v err=%v", result, err)
	}
	if want := "tail-ok\npath=" + fullPATH; result.Stdout != want {
		t.Fatalf("child lost PATH entries beyond preview:\n got %q\nwant %q", result.Stdout, want)
	}
}

func TestShellTimeoutBudgetSupportsLongDependencySetup(t *testing.T) {
	if got := normalizeTimeout(0); got != 10*time.Minute {
		t.Fatalf("default timeout = %s, want 10m", got)
	}
	if got := normalizeTimeout(25 * 60 * 1000); got != 25*time.Minute {
		t.Fatalf("25 minute timeout was unexpectedly clamped: %s", got)
	}
	if got := normalizeTimeout(60 * 60 * 1000); got != 30*time.Minute {
		t.Fatalf("maximum timeout = %s, want 30m", got)
	}
}

func TestAugmentShellPathAddsCommonUserCLIDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-style user CLI paths are not added on Windows")
	}

	home := t.TempDir()
	env := augmentShellPath([]string{
		"HOME=" + home,
		"PATH=/usr/bin:/bin",
	})
	pathValue := envValue(env, "PATH")
	for _, want := range []string{
		filepath.Join(home, ".local", "bin"),
		"/opt/homebrew/bin",
		"/usr/local/bin",
	} {
		if !containsPath(pathValue, want) {
			t.Fatalf("expected PATH to include %q, got %q", want, pathValue)
		}
	}
}

func TestAugmentShellPathDoesNotDuplicateEntries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-style user CLI paths are not added on Windows")
	}

	home := t.TempDir()
	localBin := filepath.Join(home, ".local", "bin")
	env := augmentShellPath([]string{
		"HOME=" + home,
		"PATH=" + strings.Join([]string{"/usr/bin", localBin}, string(os.PathListSeparator)),
	})
	pathValue := envValue(env, "PATH")
	if countPath(pathValue, localBin) != 1 {
		t.Fatalf("expected PATH to include %q once, got %q", localBin, pathValue)
	}
}

func TestWindowsShellPathAdditionsAddsCommonUserCLIDirs(t *testing.T) {
	env := []string{
		`USERPROFILE=C:\Users\Nolan`,
		`APPDATA=C:\Users\Nolan\AppData\Roaming`,
		`LOCALAPPDATA=C:\Users\Nolan\AppData\Local`,
	}
	additions := shellPathAdditionsForOS("windows", env)
	for _, want := range []string{
		`C:\Users\Nolan\AppData\Roaming\npm`,
		`C:\Users\Nolan\AppData\Local\pnpm`,
		`C:\Users\Nolan\AppData\Local\Microsoft\WindowsApps`,
		`C:\Users\Nolan\.local\bin`,
		`C:\Users\Nolan\scoop\shims`,
	} {
		if !containsString(additions, want) {
			t.Fatalf("expected Windows PATH additions to include %q, got %#v", want, additions)
		}
	}
}

func TestWindowsShellPathAdditionsFallsBackToUserProfile(t *testing.T) {
	additions := shellPathAdditionsForOS("windows", []string{
		`USERPROFILE=C:\Users\Nolan`,
	})
	for _, want := range []string{
		`C:\Users\Nolan\AppData\Roaming\npm`,
		`C:\Users\Nolan\AppData\Local\pnpm`,
		`C:\Users\Nolan\AppData\Local\Microsoft\WindowsApps`,
	} {
		if !containsString(additions, want) {
			t.Fatalf("expected fallback Windows PATH additions to include %q, got %#v", want, additions)
		}
	}
}

func TestAugmentShellPathForWindowsPreservesPathKeyAndDeduplicates(t *testing.T) {
	existingNPMBin := `C:\Users\Nolan\AppData\Roaming\npm`
	env := augmentShellPathForOS([]string{
		`Path=C:\Windows\System32;` + existingNPMBin,
		`USERPROFILE=C:\Users\Nolan`,
		`APPDATA=C:\Users\Nolan\AppData\Roaming`,
		`LOCALAPPDATA=C:\Users\Nolan\AppData\Local`,
	}, "windows", ";")

	if !strings.HasPrefix(env[0], "Path=") {
		t.Fatalf("expected PATH augmentation to preserve original Path key, got %q", env[0])
	}
	pathValue := envValueForOS(env, "PATH", "windows")
	if countPathWithSeparator(pathValue, existingNPMBin, ";") != 1 {
		t.Fatalf("expected PATH to include existing npm bin once, got %q", pathValue)
	}
	if !containsPathWithSeparator(pathValue, `C:\Users\Nolan\scoop\shims`, ";") {
		t.Fatalf("expected PATH to include Scoop shims, got %q", pathValue)
	}
	if !containsPathWithSeparator(pathValue, `C:\Users\Nolan\AppData\Local\Microsoft\WindowsApps`, ";") {
		t.Fatalf("expected PATH to include WindowsApps, got %q", pathValue)
	}
}

func containsPath(pathValue string, want string) bool {
	return countPath(pathValue, want) > 0
}

func countPath(pathValue string, want string) int {
	count := 0
	for _, path := range strings.Split(pathValue, string(os.PathListSeparator)) {
		if path == want {
			count++
		}
	}
	return count
}

func containsPathWithSeparator(pathValue string, want string, separator string) bool {
	return countPathWithSeparator(pathValue, want, separator) > 0
}

func countPathWithSeparator(pathValue string, want string, separator string) int {
	count := 0
	for _, path := range strings.Split(pathValue, separator) {
		if strings.EqualFold(path, want) {
			count++
		}
	}
	return count
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func resolveTestPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return resolved
}

func writeFakeCurl(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, "curl")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf fake-curl\n"), 0755); err != nil {
		t.Fatalf("failed to write fake curl: %v", err)
	}
}
