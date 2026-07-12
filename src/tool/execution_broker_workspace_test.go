package tool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"awaken/src/tool/trash"
)

type brokerTestTrashAdapter struct {
	directory string
	fail      bool
	calls     int
}

func (a *brokerTestTrashAdapter) TrashPath(path string) (trash.TrashResult, error) {
	a.calls++
	if a.fail {
		return trash.TrashResult{}, fmt.Errorf("injected Trash failure")
	}
	if err := os.MkdirAll(a.directory, 0o700); err != nil {
		return trash.TrashResult{}, err
	}
	target := filepath.Join(a.directory, fmt.Sprintf("recovery-%d", a.calls))
	if err := os.Rename(path, target); err != nil {
		return trash.TrashResult{}, err
	}
	return trash.TrashResult{OriginalPath: path, TrashedPath: target, Method: "test-rename"}, nil
}

func TestWorkspaceExecutionBrokerCommitsCreateUpdateAndRecoverableDelete(t *testing.T) {
	workspace := t.TempDir()
	writeBrokerTestFile(t, filepath.Join(workspace, "updated.txt"), "old-update")
	writeBrokerTestFile(t, filepath.Join(workspace, "obsolete", "nested.txt"), "old-delete")
	writeBrokerTestFile(t, filepath.Join(workspace, "kept.txt"), "keep")

	adapter := &brokerTestTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
	broker := NewWorkspaceExecutionBrokerWithOptions(WorkspaceExecutionBrokerOptions{
		StagingParent: t.TempDir(), TrashAdapter: adapter,
	})
	authorization, err := broker.Authorize(context.Background(), ExecutionBrokerRequest{
		ProviderID: "builtin:shell", Capability: CapabilityShellExecute,
		Operation: BrokerOperationMutation, WorkspaceRoot: workspace, WorkingDirectory: workspace,
		Command: "simulated workspace mutation",
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if authorization.Action != BrokerAllow || !authorization.RequiresAppliedSandbox || authorization.ExecutionWorkspaceRoot == workspace {
		t.Fatalf("authorization did not establish a private staging root: %+v", authorization)
	}
	writeBrokerTestFile(t, filepath.Join(authorization.ExecutionWorkspaceRoot, "updated.txt"), "new-update")
	if err := os.RemoveAll(filepath.Join(authorization.ExecutionWorkspaceRoot, "obsolete")); err != nil {
		t.Fatalf("remove staged directory: %v", err)
	}
	writeBrokerTestFile(t, filepath.Join(authorization.ExecutionWorkspaceRoot, "created", "new.txt"), "new-file")

	receipt, err := broker.Complete(context.Background(), authorization, BrokerExecutionResult{
		TransportStatus: TransportCompleted, ProcessStatus: ProcessExited, SideEffectState: OutcomeSideEffectUnknown,
	})
	if err != nil {
		t.Fatalf("Complete: %v (receipt=%+v)", err, receipt)
	}
	if receipt.CommitStatus != "committed" || receipt.SideEffectState != OutcomeSideEffectApplied || adapter.calls != 1 {
		t.Fatalf("unexpected receipt or Trash calls: receipt=%+v calls=%d", receipt, adapter.calls)
	}
	assertBrokerTestFile(t, filepath.Join(workspace, "updated.txt"), "new-update")
	assertBrokerTestFile(t, filepath.Join(workspace, "kept.txt"), "keep")
	assertBrokerTestFile(t, filepath.Join(workspace, "created", "new.txt"), "new-file")
	if _, err := os.Lstat(filepath.Join(workspace, "obsolete")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("obsolete directory was not removed from live workspace: %v", err)
	}

	updatedPath := filepath.Join(receipt.WorkspaceRoot, "updated.txt")
	obsoletePath := filepath.Join(receipt.WorkspaceRoot, "obsolete")
	createdPath := filepath.Join(receipt.WorkspaceRoot, "created", "new.txt")
	updatedRecovery := brokerRecoveryForPath(receipt, updatedPath)
	deletedRecovery := brokerRecoveryForPath(receipt, obsoletePath)
	assertBrokerTestFile(t, updatedRecovery, "old-update")
	assertBrokerTestFile(t, filepath.Join(deletedRecovery, "nested.txt"), "old-delete")
	if brokerActionForPath(receipt, updatedPath) != "updated" ||
		brokerActionForPath(receipt, obsoletePath) != "deleted" ||
		brokerActionForPath(receipt, createdPath) != "created" {
		t.Fatalf("missing canonical workspace changes: %+v", receipt.WorkspaceChanges)
	}

	secondReceipt, secondErr := broker.Complete(context.Background(), authorization, BrokerExecutionResult{})
	if secondErr != nil || secondReceipt.CommitStatus != receipt.CommitStatus || adapter.calls != 1 {
		t.Fatalf("completion must be idempotent: receipt=%+v err=%v calls=%d", secondReceipt, secondErr, adapter.calls)
	}
}

func TestCanonicalBrokerWorkspaceScopeDefaultsAndResolvesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated Windows privileges")
	}
	workspace := t.TempDir()
	realWritable := filepath.Join(workspace, "generated")
	realProtected := filepath.Join(workspace, "protected")
	if err := os.MkdirAll(realWritable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(realProtected, 0o755); err != nil {
		t.Fatal(err)
	}
	writableAlias := filepath.Join(workspace, "generated-alias")
	protectedAlias := filepath.Join(workspace, "protected-alias")
	if err := os.Symlink("generated", writableAlias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("protected", protectedAlias); err != nil {
		t.Fatal(err)
	}

	defaultScope, err := CanonicalBrokerWorkspaceScope(NewPermissionContext(workspace, nil))
	if err != nil || len(defaultScope.WritableRoots) != 1 || defaultScope.WritableRoots[0] != "." {
		t.Fatalf("empty WritableRoots must retain the workspace default: scope=%+v err=%v", defaultScope, err)
	}

	permissionContext := NewTaskScopePermissionContext(
		"scope", workspace, nil, []string{writableAlias}, nil, nil,
		[]string{protectedAlias}, filepath.Join(t.TempDir(), "scratch"),
	)
	scope, err := CanonicalBrokerWorkspaceScope(permissionContext)
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.WritableRoots) != 1 || scope.WritableRoots[0] != "generated" {
		t.Fatalf("writable symlink root was not canonicalized: %+v", scope.WritableRoots)
	}
	if !containsBrokerTestString(scope.ProtectedRoots, "protected") || !containsBrokerTestString(scope.ProtectedRoots, "protected-alias") {
		t.Fatalf("protected root must retain lexical and resolved locations: scope=%+v context=%+v", scope.ProtectedRoots, permissionContext.ProtectedRoots)
	}

	externalOnly, err := CanonicalBrokerWorkspaceScope(NewPermissionContext(workspace, []string{t.TempDir()}))
	if err != nil {
		t.Fatal(err)
	}
	if len(externalOnly.WritableRoots) != 0 {
		t.Fatalf("workspace broker must not widen an external-only writable scope: %+v", externalOnly.WritableRoots)
	}
}

func TestWorkspaceExecutionBrokerCommitsOnlyInsideNarrowWritableRoot(t *testing.T) {
	workspace := t.TempDir()
	writeBrokerTestFile(t, filepath.Join(workspace, "allowed", "updated.txt"), "old-update")
	writeBrokerTestFile(t, filepath.Join(workspace, "allowed", "deleted.txt"), "old-delete")
	writeBrokerTestFile(t, filepath.Join(workspace, "outside", "kept.txt"), "outside")
	adapter := &brokerTestTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
	broker := NewWorkspaceExecutionBrokerWithOptions(WorkspaceExecutionBrokerOptions{StagingParent: t.TempDir(), TrashAdapter: adapter})
	authorization, err := broker.Authorize(context.Background(), ExecutionBrokerRequest{
		Operation: BrokerOperationMutation, WorkspaceRoot: workspace, WorkingDirectory: workspace,
		WorkspaceScopeDeclared: true, WritableRoots: []string{"allowed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeBrokerTestFile(t, filepath.Join(authorization.ExecutionWorkspaceRoot, "allowed", "updated.txt"), "new-updated-content")
	if err := os.Remove(filepath.Join(authorization.ExecutionWorkspaceRoot, "allowed", "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	writeBrokerTestFile(t, filepath.Join(authorization.ExecutionWorkspaceRoot, "allowed", "created.txt"), "created")

	receipt, err := broker.Complete(context.Background(), authorization, BrokerExecutionResult{
		TransportStatus: TransportCompleted, ProcessStatus: ProcessExited,
	})
	if err != nil || receipt.CommitStatus != "committed" || adapter.calls != 1 {
		t.Fatalf("narrow writable-root commit failed: receipt=%+v err=%v trash_calls=%d", receipt, err, adapter.calls)
	}
	assertBrokerTestFile(t, filepath.Join(workspace, "allowed", "updated.txt"), "new-updated-content")
	assertBrokerTestFile(t, filepath.Join(workspace, "allowed", "created.txt"), "created")
	if _, err := os.Lstat(filepath.Join(workspace, "allowed", "deleted.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("allowed deletion did not commit: %v", err)
	}
	assertBrokerTestFile(t, filepath.Join(workspace, "outside", "kept.txt"), "outside")
}

func TestWorkspaceExecutionBrokerRejectsCreateUpdateDeleteOutsideWritableRootsBeforeTrash(t *testing.T) {
	for _, action := range []string{"create", "update", "delete"} {
		t.Run(action, func(t *testing.T) {
			workspace := t.TempDir()
			writeBrokerTestFile(t, filepath.Join(workspace, "allowed", "keep.txt"), "keep")
			writeBrokerTestFile(t, filepath.Join(workspace, "outside", "updated.txt"), "old-update")
			writeBrokerTestFile(t, filepath.Join(workspace, "outside", "deleted.txt"), "old-delete")
			adapter := &brokerTestTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
			broker := NewWorkspaceExecutionBrokerWithOptions(WorkspaceExecutionBrokerOptions{StagingParent: t.TempDir(), TrashAdapter: adapter})
			authorization, err := broker.Authorize(context.Background(), ExecutionBrokerRequest{
				Operation: BrokerOperationMutation, WorkspaceRoot: workspace, WorkingDirectory: workspace,
				WorkspaceScopeDeclared: true, WritableRoots: []string{"allowed"},
			})
			if err != nil {
				t.Fatal(err)
			}
			switch action {
			case "create":
				writeBrokerTestFile(t, filepath.Join(authorization.ExecutionWorkspaceRoot, "outside", "created.txt"), "created")
			case "update":
				writeBrokerTestFile(t, filepath.Join(authorization.ExecutionWorkspaceRoot, "outside", "updated.txt"), "new-updated-content")
			case "delete":
				if err := os.Remove(filepath.Join(authorization.ExecutionWorkspaceRoot, "outside", "deleted.txt")); err != nil {
					t.Fatal(err)
				}
			}

			receipt, err := broker.Complete(context.Background(), authorization, BrokerExecutionResult{
				TransportStatus: TransportCompleted, ProcessStatus: ProcessExited,
			})
			if !errors.Is(err, ErrExecutionBrokerDenied) || receipt.Action != BrokerDeny || receipt.CommitStatus != "denied_writable_scope" || receipt.SideEffectState != OutcomeSideEffectNone {
				t.Fatalf("outside %s must return structured denial: receipt=%+v err=%v", action, receipt, err)
			}
			if adapter.calls != 0 || !containsBrokerTestString(receipt.Evidence, "trash_not_invoked") {
				t.Fatalf("outside %s reached Trash/publication: receipt=%+v trash_calls=%d", action, receipt, adapter.calls)
			}
			assertBrokerTestFile(t, filepath.Join(workspace, "outside", "updated.txt"), "old-update")
			assertBrokerTestFile(t, filepath.Join(workspace, "outside", "deleted.txt"), "old-delete")
			if _, statErr := os.Lstat(filepath.Join(workspace, "outside", "created.txt")); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("outside create changed live workspace: %v", statErr)
			}
		})
	}
}

func TestWorkspaceExecutionBrokerRejectsCreateUpdateDeleteInProtectedRootsBeforeTrash(t *testing.T) {
	for _, action := range []string{"create", "update", "delete"} {
		t.Run(action, func(t *testing.T) {
			workspace := t.TempDir()
			writeBrokerTestFile(t, filepath.Join(workspace, "protected", "updated.txt"), "old-update")
			writeBrokerTestFile(t, filepath.Join(workspace, "protected", "deleted.txt"), "old-delete")
			adapter := &brokerTestTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
			broker := NewWorkspaceExecutionBrokerWithOptions(WorkspaceExecutionBrokerOptions{StagingParent: t.TempDir(), TrashAdapter: adapter})
			authorization, err := broker.Authorize(context.Background(), ExecutionBrokerRequest{
				Operation: BrokerOperationMutation, WorkspaceRoot: workspace, WorkingDirectory: workspace,
				WorkspaceScopeDeclared: true, WritableRoots: []string{"."}, ProtectedRoots: []string{"protected"},
			})
			if err != nil {
				t.Fatal(err)
			}
			switch action {
			case "create":
				writeBrokerTestFile(t, filepath.Join(authorization.ExecutionWorkspaceRoot, "protected", "created.txt"), "created")
			case "update":
				writeBrokerTestFile(t, filepath.Join(authorization.ExecutionWorkspaceRoot, "protected", "updated.txt"), "new-updated-content")
			case "delete":
				if err := os.Remove(filepath.Join(authorization.ExecutionWorkspaceRoot, "protected", "deleted.txt")); err != nil {
					t.Fatal(err)
				}
			}

			receipt, err := broker.Complete(context.Background(), authorization, BrokerExecutionResult{
				TransportStatus: TransportCompleted, ProcessStatus: ProcessExited,
			})
			if !errors.Is(err, ErrExecutionBrokerDenied) || receipt.Action != BrokerDeny || receipt.CommitStatus != "denied_protected_root" || receipt.SideEffectState != OutcomeSideEffectNone {
				t.Fatalf("protected %s must return structured denial: receipt=%+v err=%v", action, receipt, err)
			}
			if adapter.calls != 0 {
				t.Fatalf("protected %s reached Trash/publication, calls=%d", action, adapter.calls)
			}
			assertBrokerTestFile(t, filepath.Join(workspace, "protected", "updated.txt"), "old-update")
			assertBrokerTestFile(t, filepath.Join(workspace, "protected", "deleted.txt"), "old-delete")
			if _, statErr := os.Lstat(filepath.Join(workspace, "protected", "created.txt")); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("protected create changed live workspace: %v", statErr)
			}
		})
	}
}

func TestWorkspaceExecutionBrokerCanonicalSymlinkScopeCannotEscapeOrBypassProtection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated Windows privileges")
	}
	t.Run("internal writable alias commits to canonical target", func(t *testing.T) {
		workspace := t.TempDir()
		if err := os.MkdirAll(filepath.Join(workspace, "generated"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("generated", filepath.Join(workspace, "alias")); err != nil {
			t.Fatal(err)
		}
		scope, err := CanonicalBrokerWorkspaceScope(NewPermissionContext(workspace, []string{filepath.Join(workspace, "alias")}))
		if err != nil || len(scope.WritableRoots) != 1 || scope.WritableRoots[0] != "generated" {
			t.Fatalf("unexpected canonical scope: %+v err=%v", scope, err)
		}
		adapter := &brokerTestTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
		broker := NewWorkspaceExecutionBrokerWithOptions(WorkspaceExecutionBrokerOptions{StagingParent: t.TempDir(), TrashAdapter: adapter})
		authorization, err := broker.Authorize(context.Background(), ExecutionBrokerRequest{
			Operation: BrokerOperationMutation, WorkspaceRoot: workspace, WorkingDirectory: workspace,
			WorkspaceScopeDeclared: true, WritableRoots: scope.WritableRoots,
		})
		if err != nil {
			t.Fatal(err)
		}
		writeBrokerTestFile(t, filepath.Join(authorization.ExecutionWorkspaceRoot, "alias", "created.txt"), "created")
		receipt, err := broker.Complete(context.Background(), authorization, BrokerExecutionResult{TransportStatus: TransportCompleted, ProcessStatus: ProcessExited})
		if err != nil || receipt.CommitStatus != "committed" {
			t.Fatalf("canonical alias mutation did not commit: receipt=%+v err=%v", receipt, err)
		}
		assertBrokerTestFile(t, filepath.Join(workspace, "generated", "created.txt"), "created")
	})

	t.Run("external writable alias remains empty and cannot be removed", func(t *testing.T) {
		workspace := t.TempDir()
		outside := t.TempDir()
		writeBrokerTestFile(t, filepath.Join(outside, "outside.txt"), "outside")
		alias := filepath.Join(workspace, "escape")
		if err := os.Symlink(outside, alias); err != nil {
			t.Fatal(err)
		}
		scope, err := CanonicalBrokerWorkspaceScope(NewPermissionContext(workspace, []string{alias}))
		if err != nil || len(scope.WritableRoots) != 0 {
			t.Fatalf("external alias must not become workspace-writable: %+v err=%v", scope, err)
		}
		adapter := &brokerTestTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
		broker := NewWorkspaceExecutionBrokerWithOptions(WorkspaceExecutionBrokerOptions{StagingParent: t.TempDir(), TrashAdapter: adapter})
		authorization, err := broker.Authorize(context.Background(), ExecutionBrokerRequest{
			Operation: BrokerOperationMutation, WorkspaceRoot: workspace, WorkingDirectory: workspace,
			WorkspaceScopeDeclared: true, WritableRoots: scope.WritableRoots,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(authorization.ExecutionWorkspaceRoot, "escape")); err != nil {
			t.Fatal(err)
		}
		receipt, err := broker.Complete(context.Background(), authorization, BrokerExecutionResult{TransportStatus: TransportCompleted, ProcessStatus: ProcessExited})
		if !errors.Is(err, ErrExecutionBrokerDenied) || receipt.CommitStatus != "denied_writable_scope" || adapter.calls != 0 {
			t.Fatalf("external alias deletion did not fail closed: receipt=%+v err=%v calls=%d", receipt, err, adapter.calls)
		}
		if target, readErr := os.Readlink(alias); readErr != nil || target != outside {
			t.Fatalf("live escape symlink changed: target=%q err=%v", target, readErr)
		}
		assertBrokerTestFile(t, filepath.Join(outside, "outside.txt"), "outside")
	})

	t.Run("protected alias also protects resolved target", func(t *testing.T) {
		workspace := t.TempDir()
		writeBrokerTestFile(t, filepath.Join(workspace, "protected", "state.txt"), "original")
		alias := filepath.Join(workspace, "protected-alias")
		if err := os.Symlink("protected", alias); err != nil {
			t.Fatal(err)
		}
		permissionContext := NewTaskScopePermissionContext(
			"scope", workspace, nil, []string{workspace}, nil, nil,
			[]string{alias}, filepath.Join(t.TempDir(), "scratch"),
		)
		scope, err := CanonicalBrokerWorkspaceScope(permissionContext)
		if err != nil {
			t.Fatal(err)
		}
		adapter := &brokerTestTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
		broker := NewWorkspaceExecutionBrokerWithOptions(WorkspaceExecutionBrokerOptions{StagingParent: t.TempDir(), TrashAdapter: adapter})
		authorization, err := broker.Authorize(context.Background(), ExecutionBrokerRequest{
			Operation: BrokerOperationMutation, WorkspaceRoot: workspace, WorkingDirectory: workspace,
			WorkspaceScopeDeclared: true, WritableRoots: scope.WritableRoots, ProtectedRoots: scope.ProtectedRoots,
		})
		if err != nil {
			t.Fatal(err)
		}
		writeBrokerTestFile(t, filepath.Join(authorization.ExecutionWorkspaceRoot, "protected-alias", "state.txt"), "changed")
		receipt, err := broker.Complete(context.Background(), authorization, BrokerExecutionResult{TransportStatus: TransportCompleted, ProcessStatus: ProcessExited})
		if !errors.Is(err, ErrExecutionBrokerDenied) || receipt.CommitStatus != "denied_protected_root" || adapter.calls != 0 {
			t.Fatalf("protected alias bypassed commit guard: receipt=%+v err=%v calls=%d", receipt, err, adapter.calls)
		}
		assertBrokerTestFile(t, filepath.Join(workspace, "protected", "state.txt"), "original")
	})
}

func TestWorkspaceExecutionBrokerScopeValidationPrecedesConcurrentWorkspaceCheck(t *testing.T) {
	workspace := t.TempDir()
	writeBrokerTestFile(t, filepath.Join(workspace, "allowed", "file.txt"), "original")
	adapter := &brokerTestTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
	broker := NewWorkspaceExecutionBrokerWithOptions(WorkspaceExecutionBrokerOptions{StagingParent: t.TempDir(), TrashAdapter: adapter})
	authorization, err := broker.Authorize(context.Background(), ExecutionBrokerRequest{
		Operation: BrokerOperationMutation, WorkspaceRoot: workspace, WorkingDirectory: workspace,
		WorkspaceScopeDeclared: true, WritableRoots: []string{"allowed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeBrokerTestFile(t, filepath.Join(authorization.ExecutionWorkspaceRoot, "blocked", "created.txt"), "blocked")
	writeBrokerTestFile(t, filepath.Join(workspace, "allowed", "file.txt"), "concurrent-user-change")

	receipt, err := broker.Complete(context.Background(), authorization, BrokerExecutionResult{TransportStatus: TransportCompleted, ProcessStatus: ProcessExited})
	if !errors.Is(err, ErrExecutionBrokerDenied) || receipt.CommitStatus != "denied_writable_scope" {
		t.Fatalf("staged scope validation must precede the concurrent recheck: receipt=%+v err=%v", receipt, err)
	}
	assertBrokerTestFile(t, filepath.Join(workspace, "allowed", "file.txt"), "concurrent-user-change")
	if _, statErr := os.Lstat(filepath.Join(workspace, "blocked", "created.txt")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("blocked staged change reached live workspace: %v", statErr)
	}
	if adapter.calls != 0 {
		t.Fatalf("scope refusal reached Trash, calls=%d", adapter.calls)
	}
}

func TestWorkspaceExecutionBrokerConcurrentWorkspaceChangeFailsWithoutCommit(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "file.txt")
	writeBrokerTestFile(t, path, "original")
	adapter := &brokerTestTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
	broker := NewWorkspaceExecutionBrokerWithOptions(WorkspaceExecutionBrokerOptions{StagingParent: t.TempDir(), TrashAdapter: adapter})
	authorization, err := broker.Authorize(context.Background(), ExecutionBrokerRequest{
		Operation: BrokerOperationMutation, WorkspaceRoot: workspace, WorkingDirectory: workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	writeBrokerTestFile(t, filepath.Join(authorization.ExecutionWorkspaceRoot, "file.txt"), "staged")
	writeBrokerTestFile(t, path, "concurrent-user-change")

	receipt, err := broker.Complete(context.Background(), authorization, BrokerExecutionResult{
		TransportStatus: TransportCompleted, ProcessStatus: ProcessExited,
	})
	if !errors.Is(err, ErrExecutionWorkspaceChanged) || receipt.CommitStatus != "concurrent_workspace_change" {
		t.Fatalf("expected concurrent-change refusal, receipt=%+v err=%v", receipt, err)
	}
	assertBrokerTestFile(t, path, "concurrent-user-change")
	if adapter.calls != 0 {
		t.Fatalf("concurrent refusal must not touch Trash, calls=%d", adapter.calls)
	}
}

func TestWorkspaceExecutionBrokerTrashFailureRollsBackPreimages(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "file.txt")
	writeBrokerTestFile(t, path, "original")
	adapter := &brokerTestTrashAdapter{directory: filepath.Join(t.TempDir(), "trash"), fail: true}
	broker := NewWorkspaceExecutionBrokerWithOptions(WorkspaceExecutionBrokerOptions{StagingParent: t.TempDir(), TrashAdapter: adapter})
	authorization, err := broker.Authorize(context.Background(), ExecutionBrokerRequest{
		Operation: BrokerOperationMutation, WorkspaceRoot: workspace, WorkingDirectory: workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	writeBrokerTestFile(t, filepath.Join(authorization.ExecutionWorkspaceRoot, "file.txt"), "staged")
	receipt, err := broker.Complete(context.Background(), authorization, BrokerExecutionResult{
		TransportStatus: TransportCompleted, ProcessStatus: ProcessExited,
	})
	if !errors.Is(err, ErrExecutionBrokerCommit) || receipt.CommitStatus != "partial" {
		t.Fatalf("expected fail-closed Trash commit error, receipt=%+v err=%v", receipt, err)
	}
	assertBrokerTestFile(t, path, "original")
	if entries, _ := filepath.Glob(filepath.Join(workspace, "Awaken-Shell-Recovery-*")); len(entries) != 0 {
		t.Fatalf("rollback left recovery bundle in workspace: %v", entries)
	}
}

func TestWorkspaceExecutionBrokerRejectsStagedSymlinkAndOutsideWorkdir(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	broker := NewWorkspaceExecutionBrokerWithOptions(WorkspaceExecutionBrokerOptions{
		StagingParent: t.TempDir(), TrashAdapter: &brokerTestTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")},
	})
	if _, err := broker.Authorize(context.Background(), ExecutionBrokerRequest{
		Operation: BrokerOperationMutation, WorkspaceRoot: workspace, WorkingDirectory: outside,
	}); !errors.Is(err, ErrExecutionBrokerDenied) {
		t.Fatalf("outside workdir must fail closed: %v", err)
	}

	authorization, err := broker.Authorize(context.Background(), ExecutionBrokerRequest{
		Operation: BrokerOperationMutation, WorkspaceRoot: workspace, WorkingDirectory: workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(authorization.ExecutionWorkspaceRoot, "escape")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink creation is unavailable: %v", err)
		}
		t.Fatal(err)
	}
	receipt, err := broker.Complete(context.Background(), authorization, BrokerExecutionResult{
		TransportStatus: TransportCompleted, ProcessStatus: ProcessExited,
	})
	if !errors.Is(err, ErrExecutionBrokerCommit) || receipt.CommitStatus != "unsupported_delta" {
		t.Fatalf("staged symlink must fail closed: receipt=%+v err=%v", receipt, err)
	}
	if _, err := os.Lstat(filepath.Join(workspace, "escape")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("symlink escaped into live workspace: %v", err)
	}
}

func TestWorkspaceExecutionBrokerRecoversDeletedSymlinkInsideTrashBundle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated Windows privileges")
	}
	workspace := t.TempDir()
	writeBrokerTestFile(t, filepath.Join(workspace, "target.txt"), "target")
	link := filepath.Join(workspace, "link.txt")
	if err := os.Symlink("target.txt", link); err != nil {
		t.Fatal(err)
	}
	adapter := &brokerTestTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
	broker := NewWorkspaceExecutionBrokerWithOptions(WorkspaceExecutionBrokerOptions{StagingParent: t.TempDir(), TrashAdapter: adapter})
	authorization, err := broker.Authorize(context.Background(), ExecutionBrokerRequest{
		Operation: BrokerOperationUnknown, WorkspaceRoot: workspace, WorkingDirectory: workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(authorization.ExecutionWorkspaceRoot, "link.txt")); err != nil {
		t.Fatal(err)
	}
	receipt, err := broker.Complete(context.Background(), authorization, BrokerExecutionResult{
		TransportStatus: TransportCompleted, ProcessStatus: ProcessExited,
	})
	if err != nil || receipt.CommitStatus != "committed" {
		t.Fatalf("deleted symlink did not commit recoverably: receipt=%+v err=%v", receipt, err)
	}
	if _, err := os.Lstat(link); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("live symlink still exists: %v", err)
	}
	recovery := brokerRecoveryForPath(receipt, filepath.Join(receipt.WorkspaceRoot, "link.txt"))
	if target, err := os.Readlink(recovery); err != nil || target != "target.txt" {
		t.Fatalf("symlink preimage was not preserved in Trash bundle: target=%q err=%v", target, err)
	}
}

func TestWorkspaceExecutionBrokerDiscardsUnstartedExecution(t *testing.T) {
	workspace := t.TempDir()
	adapter := &brokerTestTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
	broker := NewWorkspaceExecutionBrokerWithOptions(WorkspaceExecutionBrokerOptions{StagingParent: t.TempDir(), TrashAdapter: adapter})
	authorization, err := broker.Authorize(context.Background(), ExecutionBrokerRequest{
		Operation: BrokerOperationUnknown, WorkspaceRoot: workspace, WorkingDirectory: workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	writeBrokerTestFile(t, filepath.Join(authorization.ExecutionWorkspaceRoot, "created.txt"), "staged")
	receipt, err := broker.Complete(context.Background(), authorization, BrokerExecutionResult{
		TransportStatus: TransportNotStarted, ProcessStatus: ProcessNotApplicable,
	})
	if err != nil || receipt.CommitStatus != "discarded_not_completed" {
		t.Fatalf("unexpected discard receipt: %+v err=%v", receipt, err)
	}
	if _, err := os.Lstat(filepath.Join(workspace, "created.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unstarted execution committed a file: %v", err)
	}
	if _, err := os.Lstat(authorization.SandboxRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging root was not removed: %v", err)
	}
}

func TestWorkspaceExecutionBrokerDirectDeleteStillRequiresTypedTrash(t *testing.T) {
	broker := NewWorkspaceExecutionBroker()
	authorization, err := broker.Authorize(context.Background(), ExecutionBrokerRequest{
		Operation: BrokerOperationDelete, ReferencedPaths: []string{"/workspace/a"},
	})
	if !errors.Is(err, ErrExecutionTransformRequired) || authorization.Action != BrokerTransform || authorization.Transform == nil || authorization.Transform.ToolName != "trash_file" {
		t.Fatalf("direct delete did not stay on typed Trash path: authorization=%+v err=%v", authorization, err)
	}
}

func TestWorkspaceExecutionBrokerRawNetworkFailsWithoutEgressGateway(t *testing.T) {
	broker := NewWorkspaceExecutionBroker()
	authorization, err := broker.Authorize(context.Background(), ExecutionBrokerRequest{
		Operation: BrokerOperationNetwork, WorkspaceRoot: t.TempDir(), Command: "curl https://example.com",
	})
	if !errors.Is(err, ErrExecutionBrokerDenied) || authorization.Action != BrokerDeny ||
		!strings.Contains(authorization.Reason, "egress gateway") {
		t.Fatalf("raw network must fail closed without recipient-observable egress: authorization=%+v err=%v", authorization, err)
	}
}

func writeBrokerTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertBrokerTestFile(t *testing.T, path string, expected string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil || string(content) != expected {
		t.Fatalf("file %s content=%q err=%v, want %q", path, content, err, expected)
	}
}

func brokerRecoveryForPath(receipt BrokerExecutionReceipt, path string) string {
	for _, change := range receipt.WorkspaceChanges {
		if change.Path == path {
			return change.RecoveryPath
		}
	}
	return ""
}

func brokerActionForPath(receipt BrokerExecutionReceipt, path string) string {
	for _, change := range receipt.WorkspaceChanges {
		if change.Path == path {
			return change.Action
		}
	}
	return ""
}

func containsBrokerTestString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestWorkspaceScratchEnvironmentStaysInsideSandboxRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "scratch")
	environment := workspaceScratchEnvironment(root)
	for key, value := range environment {
		relative, err := filepath.Rel(root, value)
		if err != nil || relativePathEscapes(relative) {
			t.Fatalf("%s escaped scratch root: %s", key, value)
		}
		if strings.TrimSpace(value) == "" {
			t.Fatalf("%s has empty override", key)
		}
	}
}
