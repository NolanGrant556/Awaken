package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"awaken/src/agentcore"
)

func TestProjectDetachIsIdempotentAndKeepsThreads(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	workspaceRoot := t.TempDir()
	service := newDurableThreadTestService()
	projects := service.ProjectService()
	project, err := projects.UpsertProject(workspaceRoot, "demo")
	if err != nil {
		t.Fatal(err)
	}
	createProjectTestThread(t, service, workspaceRoot, "thread_detach")

	first, err := projects.DetachProject(project.ProjectID, "detach-request-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := projects.DetachProject(project.ProjectID, "detach-request-1")
	if err != nil {
		t.Fatal(err)
	}
	if first.OperationID != second.OperationID {
		t.Fatalf("expected idempotent operation id, got %q and %q", first.OperationID, second.OperationID)
	}
	if got := service.ListThreads(); len(got) != 1 {
		t.Fatalf("detach must keep thread history, got %+v", got)
	}
	visible, err := projects.ListProjects(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 0 {
		t.Fatalf("expected detached project to be hidden, got %+v", visible)
	}
	if _, err := projects.DeleteProjectThreads(project.ProjectID, "detach-request-1"); !errors.Is(err, ErrIdempotencyKeyConflict) {
		t.Fatalf("expected idempotency conflict for reused key, got %v", err)
	}
}

func TestProjectUpsertCannotReactivateTrashPendingOrClearFence(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	workspaceRoot := t.TempDir()
	service := newDurableThreadTestService()
	projects := service.ProjectService()
	project, err := projects.UpsertProject(workspaceRoot, "pending")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := projects.PrepareTrash(project.ProjectID, "trash-pending-upsert")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projects.UpsertProject(workspaceRoot, "must-not-reactivate"); !errors.Is(err, ErrProjectOperationConflict) {
		t.Fatalf("trash-pending project must reject upsert, got %v", err)
	}
	if _, err := service.StartRun(StartRunRequest{
		Input:     "must remain fenced",
		Workspace: WorkspaceConfig{WorkspaceRoot: workspaceRoot},
	}); !errors.Is(err, ErrProjectOperationConflict) {
		t.Fatalf("rejected upsert must not clear the deletion fence, got %v", err)
	}
	operation, err := projects.GetOperation(prepared.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if operation.Phase != projectOperationPhaseWaitingForRuns {
		t.Fatalf("upsert must not mutate the pending trash operation, got %+v", operation)
	}
}

func TestProjectHistoryDeleteSoftDeletesOneHundredThreadsAndCleansUp(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	workspaceRoot := t.TempDir()
	service := newDurableThreadTestService()
	projects := service.ProjectService()
	project, err := projects.UpsertProject(workspaceRoot, "bulk")
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 100; index++ {
		createProjectTestThread(t, service, workspaceRoot, fmt.Sprintf("thread_bulk_%03d", index))
	}

	operation, err := projects.DeleteProjectThreads(project.ProjectID, "bulk-delete-1")
	if err != nil {
		t.Fatal(err)
	}
	if operation.MatchedThreads != 100 || operation.PendingCleanup != 100 {
		t.Fatalf("unexpected bulk operation: %+v", operation)
	}
	if got := service.ListThreads(); len(got) != 0 {
		t.Fatalf("soft-deleted threads must disappear immediately, got %d", len(got))
	}
	replayed, err := projects.DeleteProjectThreads(project.ProjectID, "bulk-delete-1")
	if err != nil {
		t.Fatal(err)
	}
	if replayed.OperationID != operation.OperationID {
		t.Fatalf("expected idempotent replay, got %+v", replayed)
	}
	processed, err := projects.ProcessCleanupJobs(context.Background(), 200)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 100 {
		t.Fatalf("expected 100 cleanup jobs, got %d", processed)
	}
	completed, err := projects.GetOperation(operation.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != ProjectOperationSucceeded || completed.PendingCleanup != 0 {
		t.Fatalf("expected completed operation, got %+v", completed)
	}
}

func TestProjectHistoryDeleteCancelsBusyRunAndCleansAfterTerminal(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	workspaceRoot := t.TempDir()
	started := make(chan struct{})
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				Workspace: agentcore.WorkspaceConfig{Root: workspace.WorkspaceRoot},
			}, threadID)
		},
		func(_ *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, _ string, _ agentcore.EventSink) (string, error) {
				select {
				case <-started:
				default:
					close(started)
				}
				<-ctx.Done()
				return "", ctx.Err()
			})
		},
	)
	projects := service.ProjectService()
	project, err := projects.UpsertProject(workspaceRoot, "busy")
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.StartRun(StartRunRequest{Input: "wait", Workspace: WorkspaceConfig{WorkspaceRoot: workspaceRoot}})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	operation, err := projects.DeleteProjectThreads(project.ProjectID, "busy-delete-1")
	if err != nil {
		t.Fatalf("busy history delete must be accepted: %v", err)
	}
	if operation.MatchedThreads != 1 {
		t.Fatalf("expected one matched thread, got %+v", operation)
	}
	waitForRunTerminal(t, service, run.RunID)
	if snapshot, err := service.GetRun(run.RunID); err != nil || snapshot.Status != RunStatusCancelled {
		t.Fatalf("expected cancelled run, got %+v err=%v", snapshot, err)
	}
	// Join the run goroutine's trailing transcript flush before purging so
	// TempDir teardown does not race late store writes.
	service.drainRunWorkers()
	if _, err := projects.ProcessCleanupJobs(context.Background(), 16); err != nil {
		t.Fatal(err)
	}
	if got := service.ListThreads(); len(got) != 0 {
		t.Fatalf("expected busy thread to be purged after terminal, got %+v", got)
	}
}

func TestTrashTwoPhaseFailureKeepsHistoryAndSuccessDeletesIt(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	workspaceRoot := t.TempDir()
	service := newDurableThreadTestService()
	projects := service.ProjectService()
	project, err := projects.UpsertProject(workspaceRoot, "trash")
	if err != nil {
		t.Fatal(err)
	}
	createProjectTestThread(t, service, workspaceRoot, "thread_trash")

	prepared, err := projects.PrepareTrash(project.ProjectID, "trash-attempt-1")
	if err != nil {
		t.Fatal(err)
	}
	if got := service.ListThreads(); len(got) != 1 {
		t.Fatalf("prepare must not delete history, got %+v", got)
	}
	claimed, err := projects.ClaimTrashOperation(prepared.OperationID, "desktop-main", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.AttemptID == "" || claimed.Phase != projectOperationPhaseTrashClaimed {
		t.Fatalf("claim must issue an attempt token before invocation, got %+v", claimed)
	}
	failed, err := projects.FailTrashOperation(prepared.OperationID, claimed.AttemptID, "TRASH_FAILED", "injected trash failure")
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != ProjectOperationFailed {
		t.Fatalf("expected failed operation, got %+v", failed)
	}
	if got := service.ListThreads(); len(got) != 1 {
		t.Fatalf("trash failure must keep history, got %+v", got)
	}
	visible, err := projects.ListProjects(false)
	if err != nil || len(visible) != 1 || visible[0].State != ProjectStateActive {
		t.Fatalf("trash failure must restore project, got %+v err=%v", visible, err)
	}

	prepared, err = projects.PrepareTrash(project.ProjectID, "trash-attempt-2")
	if err != nil {
		t.Fatal(err)
	}
	claimed, err = projects.ClaimTrashOperation(prepared.OperationID, "desktop-main", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projects.CompleteTrashOperation(prepared.OperationID, claimed.AttemptID); !errors.Is(err, ErrProjectOperationConflict) {
		t.Fatalf("claim alone must not prove the system trash call began, got %v", err)
	}
	staged, err := projects.StageTrashOperation(prepared.OperationID, claimed.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if staged.Phase != projectOperationPhaseTrashStaged || filepath.Base(staged.TrashStagingPath) != filepath.Base(workspaceRoot) {
		t.Fatalf("expected operation-owned staging that preserves the folder name, got %+v", staged)
	}
	invoking, err := projects.BeginTrashInvocation(prepared.OperationID, claimed.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if invoking.Phase != projectOperationPhaseTrashing || invoking.TrashInvokedAt == nil {
		t.Fatalf("expected durable trash invocation boundary, got %+v", invoking)
	}
	if err := os.RemoveAll(invoking.TrashStagingPath); err != nil {
		t.Fatal(err)
	}
	completed, err := projects.CompleteTrashOperation(prepared.OperationID, claimed.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.LogicalCommittedAt == nil || completed.MatchedThreads != 1 {
		t.Fatalf("expected logical trash commit, got %+v", completed)
	}
	if got := service.ListThreads(); len(got) != 0 {
		t.Fatalf("successful trash must hide history immediately, got %+v", got)
	}
	if _, err := projects.ProcessCleanupJobs(context.Background(), 16); err != nil {
		t.Fatal(err)
	}
	completed, err = projects.GetOperation(prepared.OperationID)
	if err != nil || completed.Status != ProjectOperationSucceeded {
		t.Fatalf("expected fully completed trash, got %+v err=%v", completed, err)
	}
}

func TestTrashLeaseCanBeReclaimedAfterCrash(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	workspaceRoot := t.TempDir()
	service := newDurableThreadTestService()
	projects := service.ProjectService()
	project, err := projects.UpsertProject(workspaceRoot, "lease")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := projects.PrepareTrash(project.ProjectID, "trash-lease-1")
	if err != nil {
		t.Fatal(err)
	}
	// The lease must comfortably outlive the fsync-heavy FileStore claim path
	// so the live-conflict assertion below cannot race lease expiry.
	firstClaim, err := projects.ClaimTrashOperation(prepared.OperationID, "worker-a", 300*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projects.ClaimTrashOperation(prepared.OperationID, "worker-b", time.Second); !errors.Is(err, ErrProjectOperationConflict) {
		t.Fatalf("expected live lease conflict, got %v", err)
	}
	time.Sleep(350 * time.Millisecond)
	secondClaim, err := projects.ClaimTrashOperation(prepared.OperationID, "worker-b", time.Second)
	if err != nil {
		t.Fatalf("expected expired lease reclaim, got %v", err)
	}
	if firstClaim.AttemptID == "" || secondClaim.AttemptID == "" || firstClaim.AttemptID == secondClaim.AttemptID {
		t.Fatalf("each claimed lease must receive a unique attempt token: first=%+v second=%+v", firstClaim, secondClaim)
	}
	if _, err := projects.AttestTrashOperationPath(prepared.OperationID, firstClaim.AttemptID); !errors.Is(err, ErrProjectTrashAttempt) {
		t.Fatalf("stale attempt must not attest a replacement lease, got %v", err)
	}
	if _, err := projects.FailTrashOperation(prepared.OperationID, firstClaim.AttemptID, "STALE", "must not restore"); !errors.Is(err, ErrProjectTrashAttempt) {
		t.Fatalf("stale attempt must not fail a replacement lease, got %v", err)
	}
	staged, err := projects.StageTrashOperation(prepared.OperationID, secondClaim.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projects.BeginTrashInvocation(prepared.OperationID, secondClaim.AttemptID); err != nil {
		t.Fatal(err)
	}
	if _, err := projects.CompleteTrashOperation(prepared.OperationID, firstClaim.AttemptID); !errors.Is(err, ErrProjectTrashAttempt) {
		t.Fatalf("stale attempt must not complete a replacement lease, got %v", err)
	}
	if err := os.RemoveAll(staged.TrashStagingPath); err != nil {
		t.Fatal(err)
	}
	if _, err := projects.CompleteTrashOperation(prepared.OperationID, secondClaim.AttemptID); err != nil {
		t.Fatal(err)
	}
}

func TestTrashFailureAfterStagingAtomicallyRestoresProject(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	parent := t.TempDir()
	workspaceRoot := filepath.Join(parent, "restore-me")
	if err := os.Mkdir(workspaceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(workspaceRoot, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := newDurableThreadTestService()
	projects := service.ProjectService()
	project, err := projects.UpsertProject(workspaceRoot, "restore")
	if err != nil {
		t.Fatal(err)
	}
	createProjectTestThread(t, service, workspaceRoot, "thread_restore")
	prepared, err := projects.PrepareTrash(project.ProjectID, "trash-restore-1")
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := projects.ClaimTrashOperation(prepared.OperationID, "desktop-main", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := projects.StageTrashOperation(prepared.OperationID, claimed.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projects.BeginTrashInvocation(prepared.OperationID, claimed.AttemptID); err != nil {
		t.Fatal(err)
	}
	failed, err := projects.FailTrashOperation(prepared.OperationID, claimed.AttemptID, "TRASH_FAILED", "injected")
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != ProjectOperationFailed {
		t.Fatalf("expected failed operation, got %+v", failed)
	}
	if payload, err := os.ReadFile(marker); err != nil || string(payload) != "keep" {
		t.Fatalf("project contents were not restored: %q err=%v", payload, err)
	}
	if _, err := os.Stat(staged.TrashStagingPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging path must be gone after restore, got %v", err)
	}
	if got := service.ListThreads(); len(got) != 1 {
		t.Fatalf("failed system trash must preserve history, got %+v", got)
	}
	visible, err := projects.ListProjects(false)
	if err != nil || len(visible) != 1 || visible[0].State != ProjectStateActive {
		t.Fatalf("failed system trash must restore active project, got %+v err=%v", visible, err)
	}
}

func TestTrashCrashRecoveryAcrossDurableFilesystemPhases(t *testing.T) {
	for _, crashPoint := range []string{"after_stage", "after_invocation", "after_system_trash"} {
		t.Run(crashPoint, func(t *testing.T) {
			t.Setenv(awakenHomeEnv, t.TempDir())
			parent := t.TempDir()
			workspaceRoot := filepath.Join(parent, "crash-project")
			if err := os.Mkdir(workspaceRoot, 0o755); err != nil {
				t.Fatal(err)
			}
			service := newDurableThreadTestService()
			projects := service.ProjectService()
			project, err := projects.UpsertProject(workspaceRoot, crashPoint)
			if err != nil {
				t.Fatal(err)
			}
			createProjectTestThread(t, service, workspaceRoot, "thread_"+crashPoint)
			prepared, err := projects.PrepareTrash(project.ProjectID, "trash-crash-"+crashPoint)
			if err != nil {
				t.Fatal(err)
			}
			first, err := projects.ClaimTrashOperation(prepared.OperationID, "worker-before-crash", time.Millisecond)
			if err != nil {
				t.Fatal(err)
			}
			staged, err := projects.StageTrashOperation(prepared.OperationID, first.AttemptID)
			if err != nil {
				t.Fatal(err)
			}
			if crashPoint != "after_stage" {
				if _, err := projects.BeginTrashInvocation(prepared.OperationID, first.AttemptID); err != nil {
					t.Fatal(err)
				}
			}
			if crashPoint == "after_system_trash" {
				if err := os.RemoveAll(staged.TrashStagingPath); err != nil {
					t.Fatal(err)
				}
			}
			time.Sleep(5 * time.Millisecond)
			recovered, err := projects.ClaimTrashOperation(prepared.OperationID, "worker-after-crash", time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if recovered.AttemptID == first.AttemptID {
				t.Fatal("crash recovery must own a fresh attempt token")
			}
			recovered, err = projects.StageTrashOperation(prepared.OperationID, recovered.AttemptID)
			if err != nil {
				t.Fatal(err)
			}
			stagingExists, err := pathExists(recovered.TrashStagingPath)
			if err != nil {
				t.Fatal(err)
			}
			if crashPoint == "after_system_trash" {
				if stagingExists || recovered.Phase != projectOperationPhaseTrashing || recovered.TrashInvokedAt == nil {
					t.Fatalf("post-trash recovery lost durable proof: %+v exists=%v", recovered, stagingExists)
				}
			} else {
				if !stagingExists {
					t.Fatal("staged project disappeared during crash recovery")
				}
				if _, err := projects.BeginTrashInvocation(prepared.OperationID, recovered.AttemptID); err != nil {
					t.Fatal(err)
				}
				if err := os.RemoveAll(recovered.TrashStagingPath); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := projects.CompleteTrashOperation(prepared.OperationID, recovered.AttemptID); err != nil {
				t.Fatal(err)
			}
			if got := service.ListThreads(); len(got) != 0 {
				t.Fatalf("completed recovery must hide cutoff history, got %+v", got)
			}
		})
	}
}

func TestTrashStagingSurvivesContentChangesAndNeverTrashesSamePathReplacement(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	parent := t.TempDir()
	workspaceRoot := filepath.Join(parent, "workspace")
	if err := os.Mkdir(workspaceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	service := newDurableThreadTestService()
	projects := service.ProjectService()
	project, err := projects.UpsertProject(workspaceRoot, "identity")
	if err != nil {
		t.Fatal(err)
	}
	createProjectTestThread(t, service, workspaceRoot, "thread_identity")
	prepared, err := projects.PrepareTrash(project.ProjectID, "trash-identity-1")
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := projects.ClaimTrashOperation(prepared.OperationID, "desktop-main", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "late-output.txt"), []byte("ready"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := projects.AttestTrashOperationPath(prepared.OperationID, claimed.AttemptID); err != nil {
		t.Fatalf("content changes inside the same directory must not invalidate trash: %v", err)
	}
	staged, err := projects.StageTrashOperation(prepared.OperationID, claimed.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workspaceRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source must be atomically removed after staging, got %v", err)
	}
	if filepath.Base(staged.TrashStagingPath) != filepath.Base(workspaceRoot) {
		t.Fatalf("system trash must receive the original folder name, got %s", staged.TrashStagingPath)
	}
	if _, err := projects.BeginTrashInvocation(prepared.OperationID, claimed.AttemptID); err != nil {
		t.Fatal(err)
	}
	// A new directory appearing at the old path after staging must never be
	// passed to shell.trashItem or removed during finalize.
	if err := os.Mkdir(workspaceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(workspaceRoot, "replacement.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(staged.TrashStagingPath); err != nil {
		t.Fatal(err)
	}
	if _, err := projects.CompleteTrashOperation(prepared.OperationID, claimed.AttemptID); err != nil {
		t.Fatal(err)
	}
	if payload, err := os.ReadFile(marker); err != nil || string(payload) != "keep" {
		t.Fatalf("same-path replacement was changed during trash finalize: %q err=%v", payload, err)
	}
}

func TestTrashStageRejectsReplacementBeforeRenameWithoutDeletingEitherDirectory(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	parent := t.TempDir()
	workspaceRoot := filepath.Join(parent, "workspace")
	if err := os.Mkdir(workspaceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	service := newDurableThreadTestService()
	projects := service.ProjectService()
	project, err := projects.UpsertProject(workspaceRoot, "identity")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := projects.PrepareTrash(project.ProjectID, "trash-identity-replaced")
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := projects.ClaimTrashOperation(prepared.OperationID, "desktop-main", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	original := workspaceRoot + ".original"
	if err := os.Rename(workspaceRoot, original); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workspaceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := projects.StageTrashOperation(prepared.OperationID, claimed.AttemptID); !errors.Is(err, ErrProjectPathChanged) {
		t.Fatalf("same-path replacement must be rejected before trash, got %v", err)
	}
	if _, err := os.Stat(original); err != nil {
		t.Fatalf("original directory must remain untouched: %v", err)
	}
	if _, err := os.Stat(workspaceRoot); err != nil {
		t.Fatalf("replacement directory must remain untouched: %v", err)
	}
}

func createProjectTestThread(t *testing.T, service *RunService, workspaceRoot, threadID string) {
	t.Helper()
	now := time.Now().UTC()
	snapshot := ThreadSnapshot{
		ThreadID: threadID,
		Label:    threadID,
		RunID:    "run_" + threadID,
		Workspace: WorkspaceConfig{
			WorkspaceRoot: workspaceRoot,
			ReadableRoots: []string{workspaceRoot},
			WritableRoots: []string{workspaceRoot},
			OutputRoots:   []string{workspaceRoot},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := service.threadStore.CreateThread(snapshot, "", threadID); err != nil {
		t.Fatalf("create test thread %q: %v", threadID, err)
	}
}
