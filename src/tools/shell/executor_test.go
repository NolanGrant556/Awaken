package shelltool

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"awaken/src/tool"
	"awaken/src/tool/profile"
	"awaken/src/tool/sandbox"
)

// fullSandboxMock is a SandboxAdapter that claims SandboxFull support and
// applies no real constraints. It lets open_world_system commands reach
// /bin/sh so we can verify the executor does NOT fail-closed when a real
// sandbox is present.
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

// noneSandboxMock is a SandboxAdapter that claims SandboxNone support,
// simulating a platform with no isolation at all (e.g. Linux). It is used
// to verify fail-closed behavior without depending on the platform-specific
// SandboxAdapterOther type (which is behind a build tag).
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

func TestShellExecutorLocalCLIExecutesWithDegradedSandbox(t *testing.T) {
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
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.ExitCode != 0 || result.Stdout != "local-cli-ok" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Tier != ShellTierLocalCLI {
		t.Fatalf("expected tier %q, got %q", ShellTierLocalCLI, result.Tier)
	}
	if result.SandboxSupport != sandbox.SandboxDegraded {
		t.Fatalf("expected SandboxDegraded support, got %q", result.SandboxSupport)
	}
}

// TestShellExecutorWorkspaceMutationExecutes verifies that workspace_mutation
// commands (e.g. `touch file`) run successfully through the executor even
// when the platform sandbox is degraded.
func TestShellExecutorWorkspaceMutationExecutes(t *testing.T) {
	t.Cleanup(func() { sandbox.SetAdapter(nil) })

	root := t.TempDir()
	permCtx := tool.NewPermissionContext(root, nil)
	executor := NewShellExecutor()

	result, err := executor.Execute(
		context.Background(),
		shellParams{Command: "touch created.txt"},
		permCtx,
		nil,
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", result.ExitCode, result.Stderr)
	}
	if result.Tier != ShellTierWorkspaceMutation {
		t.Fatalf("expected tier %q, got %q", ShellTierWorkspaceMutation, result.Tier)
	}
}

// TestShellExecutorOpenWorldFailsClosedWithDegradedSandbox verifies that
// open_world_system commands (e.g. `curl`) are refused with
// ErrSandboxUnavailable when the platform sandbox is degraded. This is the
// defense-in-depth fail-closed behavior required by the spec.
func TestShellExecutorOpenWorldFailsClosedWithDegradedSandbox(t *testing.T) {
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	sandbox.SetAdapter(degradedSandboxMock{})

	root := t.TempDir()
	permCtx := tool.NewPermissionContext(root, nil)
	executor := NewShellExecutor()

	result, err := executor.Execute(
		context.Background(),
		shellParams{Command: "curl https://example.com"},
		permCtx,
		nil,
	)
	if !errors.Is(err, ErrSandboxUnavailable) {
		t.Fatalf("expected ErrSandboxUnavailable, got err=%v", err)
	}
	if result.Tier != ShellTierOpenWorldSystem {
		t.Fatalf("expected tier %q on result, got %q", ShellTierOpenWorldSystem, result.Tier)
	}
	if result.SandboxSupport != sandbox.SandboxDegraded {
		t.Fatalf("expected SandboxDegraded support on result, got %q", result.SandboxSupport)
	}
}

// TestShellExecutorOpenWorldFailsClosedWithNoneSandbox verifies that
// open_world_system commands are refused on platforms with SandboxNone
// support (e.g. Linux). Uses a local mock so the test is not gated by the
// platform-specific SandboxAdapterOther build tag.
func TestShellExecutorOpenWorldFailsClosedWithNoneSandbox(t *testing.T) {
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	sandbox.SetAdapter(noneSandboxMock{})

	root := t.TempDir()
	permCtx := tool.NewPermissionContext(root, nil)
	executor := NewShellExecutor()

	result, err := executor.Execute(
		context.Background(),
		shellParams{Command: "curl https://example.com"},
		permCtx,
		nil,
	)
	if !errors.Is(err, ErrSandboxUnavailable) {
		t.Fatalf("expected ErrSandboxUnavailable, got err=%v", err)
	}
	if result.Tier != ShellTierOpenWorldSystem {
		t.Fatalf("expected tier %q on result, got %q", ShellTierOpenWorldSystem, result.Tier)
	}
	if result.SandboxSupport != sandbox.SandboxNone {
		t.Fatalf("expected SandboxNone support on result, got %q", result.SandboxSupport)
	}
}

// TestShellExecutorOpenWorldExecutesWithFullSandbox verifies that
// open_world_system commands are allowed to run when a SandboxFull adapter
// is injected. The test does not require curl to actually succeed — it only
// asserts that the executor did not fail-closed and that the command was
// attempted (the mock adapter's ApplySandbox was called).
func TestShellExecutorOpenWorldExecutesWithFullSandbox(t *testing.T) {
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	mock := &fullSandboxMock{}
	sandbox.SetAdapter(mock)

	root := t.TempDir()
	permCtx := tool.NewPermissionContext(root, nil)
	executor := NewShellExecutor()

	result, err := executor.Execute(
		context.Background(),
		shellParams{Command: "curl --version"},
		permCtx,
		nil,
	)
	// The executor must NOT fail-closed when a full sandbox is present.
	// curl --version may or may not succeed depending on the environment,
	// but the error must not be ErrSandboxUnavailable.
	if errors.Is(err, ErrSandboxUnavailable) {
		t.Fatalf("executor fail-closed despite SandboxFull adapter: %v", err)
	}
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result.Tier != ShellTierOpenWorldSystem {
		t.Fatalf("expected tier %q, got %q", ShellTierOpenWorldSystem, result.Tier)
	}
	if result.SandboxSupport != sandbox.SandboxFull {
		t.Fatalf("expected SandboxFull support, got %q", result.SandboxSupport)
	}
	if mock.applied == 0 {
		t.Fatal("expected ApplySandbox to be called on the full sandbox mock")
	}
}

// TestShellExecutorNewShellExecutorWithAdapter verifies that the
// NewShellExecutorWithAdapter constructor bypasses the global override
// and uses the injected adapter directly.
func TestShellExecutorNewShellExecutorWithAdapter(t *testing.T) {
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	// Set a degraded global adapter to prove the constructor bypasses it.
	sandbox.SetAdapter(degradedSandboxMock{})

	mock := &fullSandboxMock{}
	executor := NewShellExecutorWithAdapter(mock)

	root := t.TempDir()
	permCtx := tool.NewPermissionContext(root, nil)

	result, err := executor.Execute(
		context.Background(),
		shellParams{Command: "curl --version"},
		permCtx,
		nil,
	)
	if errors.Is(err, ErrSandboxUnavailable) {
		t.Fatalf("executor fail-closed despite injected full sandbox adapter: %v", err)
	}
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result.SandboxSupport != sandbox.SandboxFull {
		t.Fatalf("expected SandboxFull from injected adapter, got %q", result.SandboxSupport)
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
