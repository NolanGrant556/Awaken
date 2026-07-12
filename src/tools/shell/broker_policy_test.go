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
	"time"

	"awaken/src/tool"
	"awaken/src/tool/sandbox"
)

func TestShellDirectDeleteRunsWithoutFabricatingTrashRecovery(t *testing.T) {
	root := t.TempDir()
	permissionContext := tool.NewPermissionContext(root, []string{root})
	victim := filepath.Join(root, "victim.txt")
	if err := os.WriteFile(victim, []byte("delete-me"), 0o644); err != nil {
		t.Fatal(err)
	}
	policy := EvaluateShellExecutionPolicy("rm -rf ./victim.txt", root, permissionContext)
	if policy.DenyReason != "" || !policy.DirectAllow {
		t.Fatalf("default local-user policy must allow direct delete: %+v", policy)
	}

	result, err := NewShellExecutor().Execute(
		context.Background(), shellParams{Command: "rm -rf ./victim.txt"}, permissionContext, nil,
	)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("direct delete failed: result=%+v err=%v", result, err)
	}
	if _, statErr := os.Lstat(victim); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("direct delete did not affect the real user path: %v", statErr)
	}
	if result.BrokerReceipt.BrokerID != "local_user_direct_v1" || result.BrokerReceipt.CommitStatus != "side_effects_unobserved" || len(result.BrokerReceipt.WorkspaceChanges) != 0 {
		t.Fatalf("direct provider fabricated recoverable change evidence: %+v", result.BrokerReceipt)
	}
}

func TestShellPolicyAllowsWildcardAndCompoundDeleteAsHostShell(t *testing.T) {
	root := t.TempDir()
	permissionContext := tool.NewPermissionContext(root, []string{root})
	for _, command := range []string{"rm -rf *.tmp", "rm -rf victim.txt && echo done"} {
		policy := EvaluateShellExecutionPolicy(command, root, permissionContext)
		if policy.DenyReason != "" || !policy.DirectAllow {
			t.Fatalf("current-user shell must not impose a hidden delete gate for %q: %+v", command, policy)
		}
	}
}

func TestShellIndirectDeletesCommitThroughRecoverableWorkspaceBroker(t *testing.T) {
	commands := []string{
		`python3 -c 'import os; os.remove("victim.txt")'`,
		`node -e 'require("fs").unlinkSync("victim.txt")'`,
		`sh -c 'rm -f victim.txt'`,
		`find . -name victim.txt -delete`,
		`printf 'victim.txt\n' | xargs rm -f`,
	}
	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			sandbox.SetAdapter(&fullSandboxMock{})
			t.Cleanup(func() { sandbox.SetAdapter(nil) })
			root := t.TempDir()
			victim := filepath.Join(root, "victim.txt")
			if err := os.WriteFile(victim, []byte("preserve-me"), 0o644); err != nil {
				t.Fatalf("seed victim: %v", err)
			}
			broker, adapter := newShellTestWorkspaceBroker(t)
			ctx := tool.ContextWithExecutionBroker(context.Background(), broker)
			result, err := NewShellExecutor().Execute(
				ctx, shellParams{Command: command}, tool.NewPermissionContext(root, []string{root}), nil,
			)
			if err != nil || result.ExitCode != 0 {
				t.Fatalf("expected supervised delete to execute, result=%+v err=%v", result, err)
			}
			if _, statErr := os.Lstat(victim); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("delete was not committed to live workspace: %v", statErr)
			}
			if adapter.calls != 1 || result.BrokerReceipt.CommitStatus != "committed" {
				t.Fatalf("missing single recovery bundle commit: result=%+v calls=%d", result, adapter.calls)
			}
			recovery := recoveryPathForBasename(result.BrokerReceipt, "victim.txt")
			content, readErr := os.ReadFile(recovery)
			if readErr != nil || strings.TrimSpace(string(content)) != "preserve-me" {
				t.Fatalf("deleted preimage is not recoverable: path=%q content=%q err=%v", recovery, content, readErr)
			}
		})
	}
}

func TestShellWorkspaceBrokerRejectsNarrowAndProtectedDeltasBeforeTrash(t *testing.T) {
	testCases := []struct {
		name             string
		command          string
		writableRoots    func(workspace string) []string
		protectedRoots   func(workspace string) []string
		wantCommitStatus string
	}{
		{
			name: "outside create", command: "printf created > outside/created.txt",
			writableRoots:    func(workspace string) []string { return []string{filepath.Join(workspace, "allowed")} },
			wantCommitStatus: "denied_writable_scope",
		},
		{
			name: "outside update", command: "printf changed-content > outside/updated.txt",
			writableRoots:    func(workspace string) []string { return []string{filepath.Join(workspace, "allowed")} },
			wantCommitStatus: "denied_writable_scope",
		},
		{
			name: "outside indirect delete", command: `sh -c 'rm -f outside/deleted.txt'`,
			writableRoots:    func(workspace string) []string { return []string{filepath.Join(workspace, "allowed")} },
			wantCommitStatus: "denied_writable_scope",
		},
		{
			name: "protected update", command: "printf protected-change > protected/state.txt",
			writableRoots:    func(workspace string) []string { return []string{workspace} },
			protectedRoots:   func(workspace string) []string { return []string{filepath.Join(workspace, "protected")} },
			wantCommitStatus: "denied_protected_root",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			sandbox.SetAdapter(&fullSandboxMock{})
			t.Cleanup(func() { sandbox.SetAdapter(nil) })
			workspace := t.TempDir()
			for _, directory := range []string{"allowed", "outside", "protected"} {
				if err := os.MkdirAll(filepath.Join(workspace, directory), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(workspace, "outside", "updated.txt"), []byte("old-update"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(workspace, "outside", "deleted.txt"), []byte("old-delete"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(workspace, "protected", "state.txt"), []byte("protected-original"), 0o644); err != nil {
				t.Fatal(err)
			}
			var protectedRoots []string
			if testCase.protectedRoots != nil {
				protectedRoots = testCase.protectedRoots(workspace)
			}
			permissionContext := tool.NewTaskScopePermissionContext(
				"scope", workspace, nil, testCase.writableRoots(workspace), nil, nil,
				protectedRoots, filepath.Join(t.TempDir(), "scratch"),
			)
			broker, adapter := newShellTestWorkspaceBroker(t)
			ctx := tool.ContextWithExecutionBroker(context.Background(), broker)
			result, err := NewShellExecutor().Execute(ctx, shellParams{Command: testCase.command}, permissionContext, nil)
			if !errors.Is(err, tool.ErrExecutionBrokerDenied) || result.BrokerReceipt.Action != tool.BrokerDeny || result.BrokerReceipt.CommitStatus != testCase.wantCommitStatus {
				t.Fatalf("expected structured scope refusal: result=%+v err=%v", result, err)
			}
			if adapter.calls != 0 {
				t.Fatalf("scope refusal reached Trash, calls=%d", adapter.calls)
			}
			if _, statErr := os.Lstat(filepath.Join(workspace, "outside", "created.txt")); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("outside create reached live workspace: %v", statErr)
			}
			for path, expected := range map[string]string{
				filepath.Join(workspace, "outside", "updated.txt"): "old-update",
				filepath.Join(workspace, "outside", "deleted.txt"): "old-delete",
				filepath.Join(workspace, "protected", "state.txt"): "protected-original",
			} {
				content, readErr := os.ReadFile(path)
				if readErr != nil || string(content) != expected {
					t.Fatalf("live workspace changed at %s: content=%q err=%v", path, content, readErr)
				}
			}
		})
	}
}

func TestShellScopeDenialOutcomeReportsCompletedStagingAndNoLiveEffects(t *testing.T) {
	sandbox.SetAdapter(&fullSandboxMock{})
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	workspace := t.TempDir()
	protectedFile := filepath.Join(workspace, "protected", "state.txt")
	if err := os.MkdirAll(filepath.Dir(protectedFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(protectedFile, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	permissionContext := tool.NewTaskScopePermissionContext(
		"scope", workspace, nil, []string{workspace}, nil, nil,
		[]string{filepath.Join(workspace, "protected")}, filepath.Join(t.TempDir(), "scratch"),
	)
	broker, adapter := newShellTestWorkspaceBroker(t)
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	ctx = tool.ContextWithExecutionBroker(ctx, broker)
	outcome, err := (&ShellTool{}).ExecuteOutcome(ctx, `{"command":"printf changed > protected/state.txt"}`, nil)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeDenied || outcome.TransportStatus != tool.TransportCompleted || outcome.ProcessStatus != tool.ProcessExited || outcome.ErrorCode != tool.ErrorCodeToolDenied {
		t.Fatalf("commit-time scope denial lost staging/process truth: %+v", outcome)
	}
	receipt, ok := outcome.Diagnostic["broker_receipt"].(tool.BrokerExecutionReceipt)
	if !ok || receipt.Action != tool.BrokerDeny || receipt.CommitStatus != "denied_protected_root" || receipt.SideEffectState != tool.OutcomeSideEffectNone {
		t.Fatalf("missing structured commit denial receipt: %#v", outcome.Diagnostic["broker_receipt"])
	}
	content, readErr := os.ReadFile(protectedFile)
	if readErr != nil || string(content) != "original" || adapter.calls != 0 {
		t.Fatalf("scope denial changed live state: content=%q err=%v trash_calls=%d", content, readErr, adapter.calls)
	}
}

func TestShellGitCleanAndRenameOverwritePreserveAllPreimages(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		command string
		setup   func(t *testing.T, root string)
		checks  map[string]string
	}{
		{
			name: "git clean", command: "git clean -f victim.txt",
			setup: func(t *testing.T, root string) {
				command := exec.Command("git", "init", "-q")
				command.Dir = root
				if err := command.Run(); err != nil {
					t.Skipf("git unavailable: %v", err)
				}
			},
			checks: map[string]string{"victim.txt": "victim-old"},
		},
		{
			name: "rename overwrite", command: "mv replacement.txt victim.txt",
			setup: func(t *testing.T, root string) {
				if err := os.WriteFile(filepath.Join(root, "replacement.txt"), []byte("replacement"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			checks: map[string]string{"victim.txt": "victim-old", "replacement.txt": "replacement"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			sandbox.SetAdapter(&fullSandboxMock{})
			t.Cleanup(func() { sandbox.SetAdapter(nil) })
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "victim.txt"), []byte("victim-old"), 0o644); err != nil {
				t.Fatal(err)
			}
			testCase.setup(t, root)
			broker, _ := newShellTestWorkspaceBroker(t)
			ctx := tool.ContextWithExecutionBroker(context.Background(), broker)
			result, err := NewShellExecutor().Execute(ctx, shellParams{Command: testCase.command}, tool.NewPermissionContext(root, nil), nil)
			if err != nil || result.ExitCode != 0 || result.BrokerReceipt.CommitStatus != "committed" {
				t.Fatalf("supervised command failed: result=%+v err=%v", result, err)
			}
			for basename, expected := range testCase.checks {
				recovery := recoveryPathForBasename(result.BrokerReceipt, basename)
				content, readErr := os.ReadFile(recovery)
				if readErr != nil || string(content) != expected {
					t.Fatalf("%s preimage missing from Trash bundle: path=%q content=%q err=%v", basename, recovery, content, readErr)
				}
			}
		})
	}
}

func TestShellExecutorTerminatesBackgroundProcessBeforeStagingCommit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows production broker remains fail closed until Job Object containment exists")
	}
	sandbox.SetAdapter(&fullSandboxMock{})
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	root := t.TempDir()
	victim := filepath.Join(root, "victim.txt")
	if err := os.WriteFile(victim, []byte("preserve"), 0o644); err != nil {
		t.Fatal(err)
	}
	broker, adapter := newShellTestWorkspaceBroker(t)
	ctx := tool.ContextWithExecutionBroker(context.Background(), broker)
	result, err := NewShellExecutor().Execute(ctx, shellParams{
		Command: `sh -c '(sleep 1; rm -f victim.txt) >/dev/null 2>&1 & exit 0'`,
	}, tool.NewPermissionContext(root, nil), nil)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("background supervision command failed: result=%+v err=%v", result, err)
	}
	time.Sleep(1200 * time.Millisecond)
	content, readErr := os.ReadFile(victim)
	if readErr != nil || string(content) != "preserve" {
		t.Fatalf("background child escaped staging process supervision: content=%q err=%v", content, readErr)
	}
	if adapter.calls != 0 || result.BrokerReceipt.CommitStatus != "no_workspace_changes" {
		t.Fatalf("background child produced an unexpected commit: result=%+v calls=%d", result, adapter.calls)
	}
}

func recoveryPathForBasename(receipt tool.BrokerExecutionReceipt, basename string) string {
	for _, change := range receipt.WorkspaceChanges {
		if filepath.Base(change.Path) == basename {
			return change.RecoveryPath
		}
	}
	return ""
}

func TestShellRawNetworkOutcomeRunsAndReportsUnobservedEffects(t *testing.T) {
	mock := &fullSandboxMock{}
	sandbox.SetAdapter(mock)
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "curl"), []byte("#!/bin/sh\nprintf escaped > spawned.txt\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, nil))
	outcome, err := (&ShellTool{}).ExecuteOutcome(ctx, `{"command":"./curl https://example.com"}`, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeSucceeded || outcome.ProcessStatus != tool.ProcessExited {
		t.Fatalf("raw network command did not execute: %+v", outcome)
	}
	if mock.applied != 0 {
		t.Fatalf("default direct provider unexpectedly applied sandbox, applied=%d", mock.applied)
	}
	if content, err := os.ReadFile(filepath.Join(root, "spawned.txt")); err != nil || string(content) != "escaped" {
		t.Fatalf("raw network-shaped process did not run: content=%q err=%v", content, err)
	}
	if outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectUnknown || len(outcome.Changes) != 0 {
		t.Fatalf("default provider must preserve unknown side-effect truth: %+v", outcome)
	}
}

func TestShellOutcomeCarriesBrokerObservedTrashRecovery(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 unavailable: %v", err)
	}
	sandbox.SetAdapter(&fullSandboxMock{})
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "victim.txt"), []byte("recoverable"), 0o644); err != nil {
		t.Fatal(err)
	}
	broker, _ := newShellTestWorkspaceBroker(t)
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, nil))
	ctx = tool.ContextWithExecutionBroker(ctx, broker)
	outcome, err := (&ShellTool{}).ExecuteOutcome(ctx, `{"command":"python3 -c 'import os; os.remove(\"victim.txt\")'"}`, nil)
	if err != nil || outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("supervised outcome failed: outcome=%+v err=%v", outcome, err)
	}
	if len(outcome.Changes) == 0 || outcome.Changes[0].Action != "deleted" || outcome.Changes[0].Details["recovery_path"] == "" {
		t.Fatalf("canonical outcome lost recoverable delete facts: %+v", outcome)
	}
	if outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectApplied {
		t.Fatalf("canonical outcome lost broker side-effect state: %+v", outcome.Operation)
	}
}

func TestShellNonzeroExitPreservesCommittedWorkspaceSideEffects(t *testing.T) {
	sandbox.SetAdapter(&fullSandboxMock{})
	t.Cleanup(func() { sandbox.SetAdapter(nil) })

	root := t.TempDir()
	broker, _ := newShellTestWorkspaceBroker(t)
	permissionContext := tool.NewPermissionContext(root, nil)
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	ctx = tool.ContextWithExecutionBroker(ctx, broker)

	outcome, err := (&ShellTool{}).ExecuteOutcome(
		ctx,
		`{"command":"sh -c 'printf partial-result > partial.txt; exit 7'"}`,
		nil,
	)
	if err != nil {
		t.Fatalf("ExecuteOutcome transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeFailed || outcome.ErrorCode != tool.ErrorCodeProcessExitNonzero || outcome.ExitCode == nil || *outcome.ExitCode != 7 {
		t.Fatalf("non-zero process truth was lost: %+v", outcome)
	}
	if outcome.Retryable {
		t.Fatalf("a failed command with committed side effects must not invite blind retry: %+v", outcome)
	}
	if outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectApplied {
		t.Fatalf("committed side-effect state was lost: %+v", outcome.Operation)
	}
	if len(outcome.Changes) != 1 || outcome.Changes[0].Action != "created" || outcome.Changes[0].Certainty != tool.ChangeObserved {
		t.Fatalf("committed change was not projected canonically: %+v", outcome.Changes)
	}
	content, readErr := os.ReadFile(filepath.Join(root, "partial.txt"))
	if readErr != nil || string(content) != "partial-result" {
		t.Fatalf("staged partial result was not committed: content=%q err=%v", content, readErr)
	}
}
