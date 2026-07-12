//go:build darwin

package sandbox

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateSandboxProfileInspectReadOnly verifies that the
// inspect_read_only profile denies file writes and network access
// while allowing reads within the workspace.
func TestGenerateSandboxProfileInspectReadOnly(t *testing.T) {
	root := "/Users/test/workspace"
	profile := GenerateSandboxProfile(TierInspectReadOnly, root)

	if !strings.Contains(profile, "(version 1)") {
		t.Error("profile missing (version 1)")
	}
	if !strings.Contains(profile, "(allow default)") {
		t.Error("profile must preserve normal macOS runtime services")
	}
	if !strings.Contains(profile, "(allow process-exec*)") {
		t.Error("profile missing (allow process-exec*)")
	}
	if !strings.Contains(profile, `(subpath "/Users/test/workspace")`) {
		t.Errorf("profile does not scope reads to workspace root %q", root)
	}
	if !strings.Contains(profile, "(allow file-read*") {
		t.Error("profile should allow file reads within workspace")
	}
	if !strings.Contains(profile, `(deny file-write* (require-not (literal "/dev/null")))`) {
		t.Error("profile must deny file writes for inspect_read_only tier")
	}
	if !strings.Contains(profile, "(deny network*)") {
		t.Error("profile must deny network for inspect_read_only tier")
	}
	if strings.Contains(profile, "(allow network") {
		t.Error("profile must NOT allow network for inspect_read_only tier")
	}
	if strings.Contains(profile, "(allow file-write*") {
		t.Error("profile must NOT allow file writes for inspect_read_only tier")
	}
}

// TestGenerateSandboxProfileWorkspaceMutation verifies that the
// workspace_mutation profile allows file writes within the workspace
// but denies writes outside and denies network.
func TestGenerateSandboxProfileWorkspaceMutation(t *testing.T) {
	root := "/Users/test/ws"
	profile := GenerateSandboxProfile(TierWorkspaceMutation, root)

	if !strings.Contains(profile, `(allow file-write* (subpath "/Users/test/ws"))`) {
		t.Error("profile should allow file writes within workspace")
	}
	if !strings.Contains(profile, `(deny file-write* (require-all (require-not (subpath "/Users/test/ws")) (require-not (literal "/dev/null"))))`) {
		t.Error("profile must scope its file-write deny to paths outside workspace")
	}
	if !strings.Contains(profile, "(deny network*)") {
		t.Error("profile must deny network for workspace_mutation tier")
	}
	if strings.Contains(profile, "(allow network") {
		t.Error("profile must NOT allow network for workspace_mutation tier")
	}
	if !strings.Contains(profile, "(allow process-exec*)") {
		t.Error("profile missing (allow process-exec*)")
	}
}

func TestGenerateSandboxProfileAllowsSysctlRead(t *testing.T) {
	for _, tier := range []string{TierInspectReadOnly, TierLocalCLI, TierWorkspaceMutation, TierOpenWorldSystem} {
		profile := GenerateSandboxProfile(tier, "/Users/test/ws")
		if !strings.Contains(profile, "(allow sysctl-read)") {
			t.Errorf("%s profile should allow sysctl-read for runtime/tool startup", tier)
		}
	}
}

func TestGenerateSandboxProfileBlocksAppleEventEgress(t *testing.T) {
	for _, tier := range []string{TierInspectReadOnly, TierLocalCLI, TierWorkspaceMutation, TierOpenWorldSystem, "unknown"} {
		profile := GenerateSandboxProfile(tier, "/Users/test/ws")
		if !strings.Contains(profile, "(deny appleevent-send)") {
			t.Errorf("%s profile must not delegate egress to another app", tier)
		}
	}
}

// TestGenerateSandboxProfileLocalCLI verifies that local CLIs keep network and
// normal macOS services while their writes remain inside the broker root.
func TestGenerateSandboxProfileLocalCLI(t *testing.T) {
	root := "/Users/test/cli"
	profile := GenerateSandboxProfile(TierLocalCLI, root)

	if !strings.Contains(profile, "(allow network*)") {
		t.Error("profile should allow network for local_cli tier")
	}
	if !strings.Contains(profile, `(allow file-write* (subpath "/Users/test/cli"))`) {
		t.Error("profile should allow file writes within workspace")
	}
	if !strings.Contains(profile, `(deny file-write* (require-all (require-not (subpath "/Users/test/cli")) (require-not (literal "/dev/null"))))`) {
		t.Error("profile must deny file writes outside workspace")
	}
	if strings.Contains(profile, "(deny network") {
		t.Error("profile must NOT deny network for local_cli tier")
	}
}

func TestSandboxAdapterMacApplySandboxLocalCLIUsesSeatbelt(t *testing.T) {
	adapter := SandboxAdapterMac{execPath: "/usr/bin/sandbox-exec", available: true}
	cmd := exec.Command("/bin/sh", "-c", "dws auth login")

	result, err := adapter.ApplySandbox(cmd, TierLocalCLI, "/workspace")
	if err != nil {
		t.Fatalf("ApplySandbox returned error: %v", err)
	}
	if !result.Applied {
		t.Fatal("expected local_cli to use sandbox-exec on macOS")
	}
	if result.Support != SandboxFull {
		t.Fatalf("expected local_cli support to be full, got %q", result.Support)
	}
	if result.Method != "sandbox-exec" {
		t.Fatalf("expected sandbox-exec method, got %q", result.Method)
	}
	if cmd.Path != "/usr/bin/sandbox-exec" {
		t.Fatalf("expected sandbox-exec wrapper, got %q", cmd.Path)
	}
	if len(cmd.Args) < 6 || cmd.Args[3] != "/bin/sh" || !strings.Contains(cmd.Args[2], `(require-not (subpath "/workspace"))`) {
		t.Fatalf("local CLI wrapper lost command or scoped profile: %#v", cmd.Args)
	}
}

func TestSandboxAdapterMacReallyAllowsStagingWritesAndDeniesOutside(t *testing.T) {
	adapter := newSandboxAdapterMac()
	if adapter.SupportLevel() != SandboxFull {
		t.Skip("sandbox-exec cannot establish a Seatbelt boundary in this environment")
	}
	stage := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	inside := filepath.Join(stage, "inside.txt")
	command := "printf inside > " + quoteShellTestPath(inside) + "; printf outside > " + quoteShellTestPath(outside)
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Dir = stage
	result, err := adapter.ApplySandbox(cmd, TierWorkspaceMutation, stage)
	if err != nil || !result.Applied {
		t.Fatalf("ApplySandbox: result=%+v err=%v", result, err)
	}
	runErr := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(runErr, &exitErr) {
		t.Fatalf("outside write should make the shell fail, got %v", runErr)
	}
	if content, err := os.ReadFile(inside); err != nil || string(content) != "inside" {
		t.Fatalf("staging write was not allowed: content=%q err=%v", content, err)
	}
	if _, err := os.Lstat(outside); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside write escaped Seatbelt: %v", err)
	}
}

func quoteShellTestPath(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}

// TestGenerateSandboxProfileOpenWorldSystem verifies that the
// open_world_system profile allows network but still restricts file
// writes to the workspace.
func TestGenerateSandboxProfileOpenWorldSystem(t *testing.T) {
	root := "/Users/test/open"
	profile := GenerateSandboxProfile(TierOpenWorldSystem, root)

	if !strings.Contains(profile, "(allow network*)") {
		t.Error("profile should allow network for open_world_system tier")
	}
	if !strings.Contains(profile, `(allow file-write* (subpath "/Users/test/open"))`) {
		t.Error("profile should allow file writes within workspace")
	}
	if !strings.Contains(profile, `(deny file-write* (require-all (require-not (subpath "/Users/test/open")) (require-not (literal "/dev/null"))))`) {
		t.Error("profile must deny file writes outside workspace")
	}
	if !strings.Contains(profile, `(allow file-write* (literal "/dev/null"))`) {
		t.Error("profile should allow the null device without widening regular filesystem writes")
	}
	if strings.Contains(profile, "(deny network") {
		t.Error("profile must NOT deny network for open_world_system tier")
	}
	if !strings.Contains(profile, `(allow file-read* (subpath "/"))`) {
		t.Error("profile should allow reads anywhere for open_world_system tier")
	}
}

// TestGenerateSandboxProfileUnknownTierDefaultsToReadOnly verifies that
// an unknown tier falls back to the most restrictive (read-only) profile.
func TestGenerateSandboxProfileUnknownTierDefaultsToReadOnly(t *testing.T) {
	profile := GenerateSandboxProfile("bogus_tier", "/Users/test/ws")

	if !strings.Contains(profile, `(deny file-write* (require-not (literal "/dev/null")))`) {
		t.Error("unknown tier should deny writes (conservative default)")
	}
	if !strings.Contains(profile, "(deny network*)") {
		t.Error("unknown tier should deny network (conservative default)")
	}
	if strings.Contains(profile, "(allow network") {
		t.Error("unknown tier must NOT allow network")
	}
	if strings.Contains(profile, "(allow file-write*") {
		t.Error("unknown tier must NOT allow file writes")
	}
}

// TestGenerateSandboxProfileEmptyRootUsesSlash verifies that an empty
// workspace root still produces a valid profile.
func TestGenerateSandboxProfileEmptyRootUsesSlash(t *testing.T) {
	profile := GenerateSandboxProfile(TierInspectReadOnly, "")

	if !strings.Contains(profile, `(subpath "/")`) {
		t.Errorf("empty root should fall back to / ; profile=%q", profile)
	}
	if !strings.Contains(profile, `(deny file-write* (require-not (literal "/dev/null")))`) {
		t.Error("empty root profile must still deny writes")
	}
}

// TestGenerateSandboxProfileEscapesQuotes verifies that special
// characters in the workspace root are escaped in the profile.
func TestGenerateSandboxProfileEscapesQuotes(t *testing.T) {
	// A path containing a double quote is unusual but must not break
	// the Seatbelt profile syntax.
	profile := GenerateSandboxProfile(TierInspectReadOnly, `/Users/test/"weird"/path`)
	if !strings.Contains(profile, `\"weird\"`) {
		t.Errorf("profile did not escape embedded quotes: %q", profile)
	}
}

// TestSandboxAdapterMacSupportLevelWhenAvailable verifies that
// SupportLevel returns SandboxFull when sandbox-exec is available.
func TestSandboxAdapterMacSupportLevelWhenAvailable(t *testing.T) {
	// On a standard macOS install /usr/bin/sandbox-exec exists, so the
	// default adapter should report SandboxFull. If sandbox-exec is
	// somehow missing on this machine, the test is skipped rather than
	// failed — the missing-binary path is covered by the next test.
	if _, err := exec.LookPath(sandboxExecPath); err != nil {
		t.Skipf("sandbox-exec not available on this machine: %v", err)
	}

	adapter := newSandboxAdapterMac()
	if got := adapter.SupportLevel(); got == SandboxDegraded {
		t.Skip("sandbox-exec exists but cannot establish a Seatbelt boundary in this environment")
	} else if got != SandboxFull {
		t.Fatalf("expected SandboxFull when sandbox-exec is available, got %q", got)
	}
}

// TestSandboxAdapterMacSupportLevelWhenUnavailable verifies that
// SupportLevel returns SandboxDegraded when sandbox-exec is missing.
// This simulates a non-standard macOS setup by constructing the
// adapter with a forced-unavailable state.
func TestSandboxAdapterMacSupportLevelWhenUnavailable(t *testing.T) {
	adapter := SandboxAdapterMac{execPath: "", available: false}
	if got := adapter.SupportLevel(); got != SandboxDegraded {
		t.Fatalf("expected SandboxDegraded when sandbox-exec is missing, got %q", got)
	}
}

// TestSandboxAdapterMacApplySandboxWhenUnavailable verifies that
// ApplySandbox is a no-op returning Applied=false when sandbox-exec
// is not available.
func TestSandboxAdapterMacApplySandboxWhenUnavailable(t *testing.T) {
	adapter := SandboxAdapterMac{execPath: "", available: false}
	cmd := exec.Command("/bin/sh", "-c", "ls")

	result, err := adapter.ApplySandbox(cmd, TierInspectReadOnly, "/workspace")
	if err != nil {
		t.Fatalf("ApplySandbox returned error when unavailable: %v", err)
	}
	if result.Applied {
		t.Error("expected Applied=false when sandbox-exec is unavailable")
	}
	if result.Support != SandboxDegraded {
		t.Errorf("expected Support=SandboxDegraded, got %q", result.Support)
	}
	if cmd.Path != "/bin/sh" {
		t.Errorf("cmd.Path should be unchanged when sandbox-exec is unavailable, got %q", cmd.Path)
	}
}

// TestSandboxAdapterMacApplySandboxWrapsCommand verifies that
// ApplySandbox modifies cmd to run through sandbox-exec with the
// generated profile. This test does NOT run sandbox-exec; it only
// checks that cmd.Path and cmd.Args are set correctly.
func TestSandboxAdapterMacApplySandboxWrapsCommand(t *testing.T) {
	// Use a fake execPath so the test does not depend on sandbox-exec
	// being installed on the CI machine.
	adapter := SandboxAdapterMac{execPath: "/usr/bin/sandbox-exec", available: true}

	cmd := exec.Command("/bin/sh", "-c", "echo hello")
	cmd.Dir = "/workspace"

	result, err := adapter.ApplySandbox(cmd, TierInspectReadOnly, "/workspace")
	if err != nil {
		t.Fatalf("ApplySandbox returned error: %v", err)
	}
	if !result.Applied {
		t.Fatal("expected Applied=true")
	}
	if result.Method != "sandbox-exec" {
		t.Errorf("expected Method=sandbox-exec, got %q", result.Method)
	}
	if result.Support != SandboxFull {
		t.Errorf("expected Support=SandboxFull, got %q", result.Support)
	}

	if cmd.Path != "/usr/bin/sandbox-exec" {
		t.Errorf("expected cmd.Path=/usr/bin/sandbox-exec, got %q", cmd.Path)
	}
	if len(cmd.Args) < 5 {
		t.Fatalf("cmd.Args too short: %v", cmd.Args)
	}
	if cmd.Args[0] != "sandbox-exec" {
		t.Errorf("expected Args[0]=sandbox-exec, got %q", cmd.Args[0])
	}
	if cmd.Args[1] != "-p" {
		t.Errorf("expected Args[1]=-p, got %q", cmd.Args[1])
	}
	profile := cmd.Args[2]
	if !strings.Contains(profile, "(version 1)") {
		t.Errorf("profile at Args[2] is not a valid Seatbelt profile: %q", profile)
	}
	if !strings.Contains(profile, "/workspace") {
		t.Errorf("profile should reference workspace root /workspace: %q", profile)
	}
	// The original command (/bin/sh) should follow the profile arg.
	if cmd.Args[3] != "/bin/sh" {
		t.Errorf("expected Args[3]=/bin/sh (original cmd.Path), got %q", cmd.Args[3])
	}
	// The remaining original args (-c, echo hello) should be preserved.
	if len(cmd.Args) != 6 {
		t.Fatalf("expected 6 args total, got %d: %v", len(cmd.Args), cmd.Args)
	}
	if cmd.Args[4] != "-c" {
		t.Errorf("expected Args[4]=-c, got %q", cmd.Args[4])
	}
	if cmd.Args[5] != "echo hello" {
		t.Errorf("expected Args[5]=echo hello, got %q", cmd.Args[5])
	}
}

// TestSandboxAdapterMacApplySandboxUsesWorkspaceRootInProfile verifies
// that the generated profile references the workspace root passed to
// ApplySandbox.
func TestSandboxAdapterMacApplySandboxUsesWorkspaceRootInProfile(t *testing.T) {
	adapter := SandboxAdapterMac{execPath: "/usr/bin/sandbox-exec", available: true}
	cmd := exec.Command("/bin/sh", "-c", "ls")
	cmd.Dir = "/some/cwd"

	result, err := adapter.ApplySandbox(cmd, TierWorkspaceMutation, "/the/workspace/root")
	if err != nil {
		t.Fatalf("ApplySandbox returned error: %v", err)
	}
	if !result.Applied {
		t.Fatal("expected Applied=true")
	}

	profile := cmd.Args[2]
	if !strings.Contains(profile, "/the/workspace/root") {
		t.Errorf("profile should reference workspace root /the/workspace/root: %q", profile)
	}
	if !strings.Contains(profile, `(allow file-write* (subpath "/the/workspace/root"))`) {
		t.Error("workspace_mutation profile should allow writes within workspace root")
	}
}

// TestSandboxAdapterMacApplySandboxFallsBackToCmdDir verifies that when
// workspaceRoot is empty, the adapter falls back to cmd.Dir for the
// profile root.
func TestSandboxAdapterMacApplySandboxFallsBackToCmdDir(t *testing.T) {
	adapter := SandboxAdapterMac{execPath: "/usr/bin/sandbox-exec", available: true}
	cmd := exec.Command("/bin/sh", "-c", "ls")
	cmd.Dir = "/cmd/dir"

	_, err := adapter.ApplySandbox(cmd, TierInspectReadOnly, "")
	if err != nil {
		t.Fatalf("ApplySandbox returned error: %v", err)
	}

	profile := cmd.Args[2]
	if !strings.Contains(profile, "/cmd/dir") {
		t.Errorf("profile should fall back to cmd.Dir /cmd/dir: %q", profile)
	}
}

// TestSandboxAdapterMacApplySandboxPreservesOriginalArgs verifies that
// ApplySandbox preserves all original command arguments after the
// sandbox-exec wrapper.
func TestSandboxAdapterMacApplySandboxPreservesOriginalArgs(t *testing.T) {
	adapter := SandboxAdapterMac{execPath: "/usr/bin/sandbox-exec", available: true}

	// Simulate a command with multiple args, like /bin/sh -c "echo a b c".
	cmd := exec.Command("/bin/sh", "-c", "echo a b c")

	_, err := adapter.ApplySandbox(cmd, TierOpenWorldSystem, "/ws")
	if err != nil {
		t.Fatalf("ApplySandbox returned error: %v", err)
	}

	// Expected: sandbox-exec -p <profile> /bin/sh -c "echo a b c"
	if cmd.Args[0] != "sandbox-exec" {
		t.Errorf("expected Args[0]=sandbox-exec, got %q", cmd.Args[0])
	}
	if cmd.Args[1] != "-p" {
		t.Errorf("expected Args[1]=-p, got %q", cmd.Args[1])
	}
	if cmd.Args[3] != "/bin/sh" {
		t.Errorf("expected Args[3]=/bin/sh, got %q", cmd.Args[3])
	}
	if cmd.Args[4] != "-c" {
		t.Errorf("expected Args[4]=-c, got %q", cmd.Args[4])
	}
	if cmd.Args[5] != "echo a b c" {
		t.Errorf("expected Args[5]=echo a b c, got %q", cmd.Args[5])
	}
}

// TestDefaultAdapterMacReportsSupportLevel verifies that the default
// platform adapter on macOS reports a concrete support level (either
// SandboxFull when sandbox-exec exists, or SandboxDegraded when it
// doesn't). This complements the generic test in sandbox_test.go.
func TestDefaultAdapterMacReportsSupportLevel(t *testing.T) {
	adapter := defaultPlatformAdapter()
	mac, ok := adapter.(SandboxAdapterMac)
	if !ok {
		t.Fatalf("expected defaultPlatformAdapter to return SandboxAdapterMac on darwin, got %T", adapter)
	}
	support := mac.SupportLevel()
	switch support {
	case SandboxFull, SandboxDegraded:
		// ok
	default:
		t.Fatalf("unexpected support level %q on macOS", support)
	}
}
