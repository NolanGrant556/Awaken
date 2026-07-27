package scheduledtask

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"awaken/src/filestore"
)

var repoBase = time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)

func openTestRepo(t *testing.T) (*FileRepository, *FakeClock, string) {
	t.Helper()
	genDir := t.TempDir()
	clock := NewFakeClock(repoBase)
	repo, err := OpenFileRepository(genDir, clock)
	if err != nil {
		t.Fatalf("OpenFileRepository: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	return repo, clock, genDir
}

func reopenTestRepo(t *testing.T, genDir string, clock Clock) *FileRepository {
	t.Helper()
	repo, err := OpenFileRepository(genDir, clock)
	if err != nil {
		t.Fatalf("reopen repository: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	return repo
}

func sampleTask(name string) Task {
	return Task{
		Name:      name,
		Enabled:   true,
		ProjectID: "project_1",
		Schedule:  cronSchedule("30 9 * * 1-5", "Asia/Shanghai"),
		Action:    Action{Kind: ActionKindReminder, Content: "喝水休息一下"},
		Execution: ExecutionOptions{TimeoutSeconds: 600, MisfireGraceSeconds: 600},
	}
}

func TestCreateGeneratesIdentity(t *testing.T) {
	repo, clock, _ := openTestRepo(t)
	input := sampleTask("早报")
	input.TaskID = "sched_client_forged"
	input.Version = 42
	input.ThreadID = "thread_forged"
	input.CreatedAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	input.UpdatedAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	created, err := repo.Create(input)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(created.TaskID, TaskIDPrefix) || created.TaskID == "sched_client_forged" {
		t.Fatalf("client-forged task id survived: %q", created.TaskID)
	}
	if created.Version != 1 {
		t.Fatalf("version = %d, want 1", created.Version)
	}
	if created.ThreadID != "" {
		t.Fatalf("thread id must be Core-bound later, got %q", created.ThreadID)
	}
	if !created.CreatedAt.Equal(clock.Now().UTC()) || !created.UpdatedAt.Equal(clock.Now().UTC()) {
		t.Fatalf("timestamps not generated from the repository clock: %+v", created)
	}
}

func TestCreateGetListAndReopen(t *testing.T) {
	repo, clock, genDir := openTestRepo(t)
	first, err := repo.Create(sampleTask("first"))
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Minute)
	second, err := repo.Create(sampleTask("second"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(first.TaskID)
	if err != nil || got.Name != "first" {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if _, err := repo.Get("sched_missing"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("Get missing = %v, want ErrTaskNotFound", err)
	}
	list, err := repo.List()
	if err != nil || len(list) != 2 {
		t.Fatalf("List = %v, %v", list, err)
	}
	if list[0].TaskID != first.TaskID || list[1].TaskID != second.TaskID {
		t.Fatalf("List order = %v, want creation order", []string{list[0].TaskID, list[1].TaskID})
	}
	repo.Close()
	reopened := reopenTestRepo(t, genDir, clock)
	again, err := reopened.Get(second.TaskID)
	if err != nil || again.Name != "second" || again.Version != 1 {
		t.Fatalf("Get after reopen = %+v, %v", again, err)
	}
}

func TestUpdateVersionCAS(t *testing.T) {
	repo, clock, _ := openTestRepo(t)
	created, err := repo.Create(sampleTask("cas"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.Update(created.TaskID, 99, func(task Task) (Task, error) {
		task.Name = "renamed"
		return task, nil
	})
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale version = %v, want ErrVersionConflict", err)
	}
	clock.Advance(time.Minute)
	updated, err := repo.Update(created.TaskID, 1, func(task Task) (Task, error) {
		task.Name = "renamed"
		// Client attempts to forge identity fields; Core must override.
		task.TaskID = "sched_forged"
		task.Version = 100
		task.CreatedAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		return task, nil
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "renamed" || updated.Version != 2 {
		t.Fatalf("updated = %+v", updated)
	}
	if updated.TaskID != created.TaskID || !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("identity fields were not preserved: %+v", updated)
	}
	if !updated.UpdatedAt.Equal(clock.Now().UTC()) {
		t.Fatalf("updated_at not stamped from clock: %v", updated.UpdatedAt)
	}
	// A failing mutate must not append a mutation.
	mutateErr := errors.New("mutate failed")
	if _, err := repo.Update(created.TaskID, 2, func(Task) (Task, error) { return Task{}, mutateErr }); !errors.Is(err, mutateErr) {
		t.Fatalf("mutate error = %v", err)
	}
	current, _ := repo.Get(created.TaskID)
	if current.Version != 2 {
		t.Fatalf("failed mutate advanced the version to %d", current.Version)
	}
}

func TestPauseResume(t *testing.T) {
	repo, _, _ := openTestRepo(t)
	created, _ := repo.Create(sampleTask("pause"))
	paused, err := repo.SetPaused(created.TaskID, created.Version)
	if err != nil || paused.Enabled {
		t.Fatalf("SetPaused = %+v, %v", paused, err)
	}
	if state := paused.State(false); state != TaskStatePaused {
		t.Fatalf("state = %s, want paused", state)
	}
	resumed, err := repo.SetResumed(paused.TaskID, paused.Version)
	if err != nil || !resumed.Enabled {
		t.Fatalf("SetResumed = %+v, %v", resumed, err)
	}
	if resumed.Version != 3 {
		t.Fatalf("version = %d, want 3", resumed.Version)
	}
}

func TestBindThreadConsumeOccurrenceComplete(t *testing.T) {
	repo, _, _ := openTestRepo(t)
	created, _ := repo.Create(sampleTask("lifecycle"))
	bound, err := repo.BindThread(created.TaskID, created.Version, "thread_1")
	if err != nil || bound.ThreadID != "thread_1" {
		t.Fatalf("BindThread = %+v, %v", bound, err)
	}
	occurrence := repoBase.Add(time.Hour)
	consumed, err := repo.ConsumeOccurrence(created.TaskID, occurrence)
	if err != nil {
		t.Fatalf("ConsumeOccurrence: %v", err)
	}
	if consumed.ConsumedOccurrences != 1 || consumed.LastProcessedOccurrence == nil ||
		!consumed.LastProcessedOccurrence.Equal(occurrence) {
		t.Fatalf("consume = %+v", consumed)
	}
	// An out-of-order older occurrence still counts but must not move the
	// last-processed watermark backwards.
	older := repoBase.Add(30 * time.Minute)
	consumed, err = repo.ConsumeOccurrence(created.TaskID, older)
	if err != nil {
		t.Fatal(err)
	}
	if consumed.ConsumedOccurrences != 2 || !consumed.LastProcessedOccurrence.Equal(occurrence) {
		t.Fatalf("out-of-order consume regressed watermark: %+v", consumed)
	}
	completed, err := repo.MarkCompleted(created.TaskID)
	if err != nil || !completed.Completed {
		t.Fatalf("MarkCompleted = %+v, %v", completed, err)
	}
	if state := completed.State(false); state != TaskStateCompleted {
		t.Fatalf("state = %s, want completed", state)
	}
}

func TestScheduledOccurrenceDoesNotDeduplicateAgainstManualExecution(t *testing.T) {
	repo, _, _ := openTestRepo(t)
	created, _ := repo.Create(sampleTask("manual-scheduled-boundary"))
	occurrence := repoBase.Add(time.Hour)

	manual, createdManual, err := repo.AppendExecution(created.TaskID, Execution{
		Trigger:        TriggerManual,
		ScheduledFor:   occurrence,
		IdempotencyKey: "manual-key",
		Status:         ExecutionStatusQueued,
	})
	if err != nil || !createdManual {
		t.Fatalf("append manual = %+v, created=%v, err=%v", manual, createdManual, err)
	}
	scheduled, createdScheduled, err := repo.AppendExecution(created.TaskID, Execution{
		Trigger:      TriggerScheduled,
		ScheduledFor: occurrence,
		Status:       ExecutionStatusQueued,
	})
	if err != nil || !createdScheduled {
		t.Fatalf("append scheduled = %+v, created=%v, err=%v", scheduled, createdScheduled, err)
	}
	if scheduled.ExecutionID == manual.ExecutionID {
		t.Fatal("scheduled occurrence deduplicated against a manual execution")
	}
}

func TestDeleteTombstone(t *testing.T) {
	repo, clock, genDir := openTestRepo(t)
	created, _ := repo.Create(sampleTask("delete-me"))
	keep, _ := repo.Create(sampleTask("keep"))
	if err := repo.Delete(created.TaskID, created.Version, "del-key-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.Get(created.TaskID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("Get tombstoned = %v, want ErrTaskNotFound", err)
	}
	list, _ := repo.List()
	if len(list) != 1 || list[0].TaskID != keep.TaskID {
		t.Fatalf("List after tombstone = %+v", list)
	}
	if err := repo.Delete(created.TaskID, created.Version+1, "del-key-2"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("double delete = %v, want ErrTaskNotFound", err)
	}
	// Tombstone survives restart: the task stays invisible.
	repo.Close()
	reopened := reopenTestRepo(t, genDir, clock)
	if _, err := reopened.Get(created.TaskID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("Get tombstoned after reopen = %v", err)
	}
	if list, _ := reopened.List(); len(list) != 1 {
		t.Fatalf("List after reopen = %+v", list)
	}
}

func TestDeleteIdempotencyReplaysAcrossReopen(t *testing.T) {
	repo, clock, genDir := openTestRepo(t)
	created, _ := repo.Create(sampleTask("delete-idem"))
	key := "delete-key-reopen"
	if err := repo.Delete(created.TaskID, created.Version, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	repo.Close()

	reopened := reopenTestRepo(t, genDir, clock)
	defer reopened.Close()
	// Same key on the now-tombstoned task replays success.
	if err := reopened.Delete(created.TaskID, created.Version, key); err != nil {
		t.Fatalf("replayed delete with same key: %v", err)
	}
	// A different key after tombstone returns ErrTaskNotFound.
	if err := reopened.Delete(created.TaskID, created.Version, "other-key"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("delete with other key = %v, want ErrTaskNotFound", err)
	}
}

func corruptStateSnapshot(t *testing.T, genDir string) {
	t.Helper()
	path := filepath.Join(genDir, "scheduled-tasks", "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestStateRebuildFromJournal(t *testing.T) {
	for _, mode := range []string{"missing", "corrupt"} {
		t.Run(mode, func(t *testing.T) {
			repo, clock, genDir := openTestRepo(t)
			created, _ := repo.Create(sampleTask("rebuild"))
			if _, err := repo.Update(created.TaskID, 1, func(task Task) (Task, error) {
				task.Name = "rebuilt"
				return task, nil
			}); err != nil {
				t.Fatal(err)
			}
			repo.Close()
			statePath := filepath.Join(genDir, "scheduled-tasks", "state.json")
			if mode == "missing" {
				if err := os.Remove(statePath); err != nil {
					t.Fatal(err)
				}
			} else {
				corruptStateSnapshot(t, genDir)
			}
			reopened := reopenTestRepo(t, genDir, clock)
			got, err := reopened.Get(created.TaskID)
			if err != nil || got.Name != "rebuilt" || got.Version != 2 {
				t.Fatalf("rebuilt state = %+v, %v", got, err)
			}
			// The snapshot is rewritten on rebuild.
			info, err := os.Stat(statePath)
			if err != nil {
				t.Fatalf("state.json not rewritten: %v", err)
			}
			if info.Mode().Perm() != filestore.FilePerm {
				t.Fatalf("state.json perm = %o, want %o", info.Mode().Perm(), filestore.FilePerm)
			}
		})
	}
}

func TestMutationsJournalTailTruncation(t *testing.T) {
	repo, clock, genDir := openTestRepo(t)
	created, _ := repo.Create(sampleTask("tail"))
	repo.Close()
	// Simulate a crash mid-append: a partial final line.
	journalPath := filepath.Join(genDir, "scheduled-tasks", "mutations.jsonl")
	file, err := os.OpenFile(journalPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"seq":2,"payload":{"kind":"upd`); err != nil {
		t.Fatal(err)
	}
	file.Close()
	reopened := reopenTestRepo(t, genDir, clock)
	got, err := reopened.Get(created.TaskID)
	if err != nil || got.Version != 1 {
		t.Fatalf("after tail truncation = %+v, %v", got, err)
	}
	// The journal stays writable after recovery.
	if _, err := reopened.Update(created.TaskID, 1, func(task Task) (Task, error) {
		task.Name = "post-crash"
		return task, nil
	}); err != nil {
		t.Fatalf("append after tail truncation: %v", err)
	}
}

func TestMutationsJournalMidCorruptionFailsClosed(t *testing.T) {
	repo, _, genDir := openTestRepo(t)
	created, _ := repo.Create(sampleTask("corrupt"))
	if _, err := repo.Update(created.TaskID, 1, func(task Task) (Task, error) {
		task.Name = "v2"
		return task, nil
	}); err != nil {
		t.Fatal(err)
	}
	repo.Close()
	// Corrupt a byte in the middle of the first (non-final) record; the
	// checksum no longer matches and open must fail closed.
	journalPath := filepath.Join(genDir, "scheduled-tasks", "mutations.jsonl")
	raw, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	raw[10] ^= 0xFF
	if err := os.WriteFile(journalPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenFileRepository(genDir, RealClock{}); !errors.Is(err, filestore.ErrCorruptJournal) {
		t.Fatalf("mid-file corruption = %v, want ErrCorruptJournal", err)
	}
}

// A journal whose per-task version chain is broken (out-of-order versions)
// is corruption and fails closed, even if every line checksums.
func TestMutationVersionChainFailClosed(t *testing.T) {
	genDir := t.TempDir()
	for _, d := range []string{"locks", "scheduled-tasks", "scheduled-tasks/executions"} {
		if err := filestore.EnsureDir(filepath.Join(genDir, filepath.FromSlash(d))); err != nil {
			t.Fatal(err)
		}
	}
	journal, _, err := filestore.OpenJournal(filepath.Join(genDir, "scheduled-tasks", "mutations.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	task := sampleTask("chain")
	task.TaskID = "sched_chain"
	task.Version = 1
	task.CreatedAt = repoBase
	task.UpdatedAt = repoBase
	v1 := task
	if _, err := journal.Append(mutation{Kind: mutationKindCreate, TaskID: task.TaskID, Version: 1, Task: &v1, At: repoBase}); err != nil {
		t.Fatal(err)
	}
	v3 := task
	v3.Version = 3 // skips version 2
	if _, err := journal.Append(mutation{Kind: mutationKindUpdate, TaskID: task.TaskID, Version: 3, Task: &v3, At: repoBase}); err != nil {
		t.Fatal(err)
	}
	journal.Close()
	if _, err := OpenFileRepository(genDir, RealClock{}); !errors.Is(err, ErrCorruptState) {
		t.Fatalf("broken version chain = %v, want ErrCorruptState", err)
	}
}

func TestFileAndDirectoryPermissions(t *testing.T) {
	repo, _, genDir := openTestRepo(t)
	created, _ := repo.Create(sampleTask("perms"))
	if _, _, err := repo.AppendExecution(created.TaskID, Execution{
		Trigger: TriggerScheduled, ScheduledFor: repoBase, StartedAt: repoBase, Status: ExecutionStatusQueued,
	}); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{
		filepath.Join(genDir, "locks"),
		filepath.Join(genDir, "scheduled-tasks"),
		filepath.Join(genDir, "scheduled-tasks", "executions"),
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != filestore.DirPerm {
			t.Fatalf("dir %s perm = %o, want %o", dir, info.Mode().Perm(), filestore.DirPerm)
		}
	}
	for _, file := range []string{
		filepath.Join(genDir, "scheduled-tasks", "mutations.jsonl"),
		filepath.Join(genDir, "scheduled-tasks", "state.json"),
		filepath.Join(genDir, "scheduled-tasks", "executions", created.TaskID+".jsonl"),
	} {
		info, err := os.Stat(file)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != filestore.FilePerm {
			t.Fatalf("file %s perm = %o, want %o", file, info.Mode().Perm(), filestore.FilePerm)
		}
	}
	// Atomic replacement leaves no temp files behind.
	matches, err := filepath.Glob(filepath.Join(genDir, "scheduled-tasks", ".state.json.tmp-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("leftover temp files: %v", matches)
	}
}

// The generation store lock excludes a second writer: while another owner
// holds locks/store.lock, writes fail instead of degrading to lock-free
// mutation.
func TestStoreLockExcludesConcurrentWriter(t *testing.T) {
	repo, _, genDir := openTestRepo(t)
	repo.lockWaitTimeout = 200 * time.Millisecond
	created, _ := repo.Create(sampleTask("locked"))
	lockPath := filepath.Join(genDir, "locks", "store.lock")
	foreign, err := filestore.AcquireLock(lockPath, "other-core", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Release()
	if _, err := repo.Update(created.TaskID, 1, func(task Task) (Task, error) {
		task.Name = "never"
		return task, nil
	}); !errors.Is(err, filestore.ErrLockHeld) {
		t.Fatalf("write under foreign lock = %v, want ErrLockHeld", err)
	}
	foreign.Release()
	if _, err := repo.Update(created.TaskID, 1, func(task Task) (Task, error) {
		task.Name = "after-release"
		return task, nil
	}); err != nil {
		t.Fatalf("write after lock release: %v", err)
	}
}

func TestExecutionAppendAndFoldTransitions(t *testing.T) {
	repo, clock, _ := openTestRepo(t)
	created, _ := repo.Create(sampleTask("exec"))
	scheduledFor := repoBase.Add(time.Hour)
	queued, _, err := repo.AppendExecution(created.TaskID, Execution{
		TaskVersion: 1, Trigger: TriggerScheduled, ScheduledFor: scheduledFor,
		StartedAt: scheduledFor, Status: ExecutionStatusQueued,
	})
	if err != nil {
		t.Fatalf("AppendExecution: %v", err)
	}
	if !strings.HasPrefix(queued.ExecutionID, ExecutionIDPrefix) {
		t.Fatalf("execution id = %q", queued.ExecutionID)
	}
	// queued -> running -> success via same-id appended versions.
	running, _, err := repo.AppendExecution(created.TaskID, Execution{
		ExecutionID: queued.ExecutionID, TaskVersion: 1, Trigger: TriggerScheduled,
		ScheduledFor: scheduledFor, StartedAt: scheduledFor, Status: ExecutionStatusRunning,
		RunID: "run_1", ThreadID: "thread_1",
	})
	if err != nil {
		t.Fatal(err)
	}
	finished := clock.Now().UTC()
	if _, _, err := repo.AppendExecution(created.TaskID, Execution{
		ExecutionID: queued.ExecutionID, TaskVersion: 1, Trigger: TriggerScheduled,
		ScheduledFor: scheduledFor, StartedAt: scheduledFor, FinishedAt: &finished,
		Status: ExecutionStatusSuccess, RunID: "run_1", ThreadID: "thread_1",
	}); err != nil {
		t.Fatal(err)
	}
	list, err := repo.ListExecutions(created.TaskID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("folded executions = %d, want 1", len(list))
	}
	if list[0].Status != ExecutionStatusSuccess || list[0].FinishedAt == nil {
		t.Fatalf("fold kept a stale version: %+v", list[0])
	}
	// Transitions out of a terminal status are rejected.
	if _, _, err := repo.AppendExecution(created.TaskID, Execution{
		ExecutionID: queued.ExecutionID, Status: ExecutionStatusRunning,
	}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("terminal transition = %v, want ErrInvalidTransition", err)
	}
	_ = running
	// Transitions for unknown executions are rejected.
	if _, _, err := repo.AppendExecution(created.TaskID, Execution{
		ExecutionID: "exec_missing", Status: ExecutionStatusRunning,
	}); !errors.Is(err, ErrExecutionNotFound) {
		t.Fatalf("unknown execution = %v, want ErrExecutionNotFound", err)
	}
	// Executions of unknown tasks are rejected.
	if _, _, err := repo.AppendExecution("sched_missing", Execution{
		Trigger: TriggerScheduled, ScheduledFor: scheduledFor, Status: ExecutionStatusQueued,
	}); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("unknown task = %v, want ErrTaskNotFound", err)
	}
}

func TestExecutionIdempotency(t *testing.T) {
	repo, _, _ := openTestRepo(t)
	created, _ := repo.Create(sampleTask("idem"))
	scheduledFor := repoBase.Add(time.Hour)
	// (task_id, scheduled_for, trigger) dedup across restarts/retries.
	first, _, err := repo.AppendExecution(created.TaskID, Execution{
		TaskVersion: 1, Trigger: TriggerScheduled, ScheduledFor: scheduledFor, Status: ExecutionStatusQueued,
	})
	if err != nil {
		t.Fatal(err)
	}
	again, _, err := repo.AppendExecution(created.TaskID, Execution{
		TaskVersion: 1, Trigger: TriggerScheduled, ScheduledFor: scheduledFor, Status: ExecutionStatusQueued,
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.ExecutionID != first.ExecutionID {
		t.Fatalf("duplicate scheduled occurrence created a new execution: %s vs %s", again.ExecutionID, first.ExecutionID)
	}
	// A misfire for the same occurrence shares the scheduled occurrence
	// identity so that crash recovery cannot create a second execution.
	misfire, createdMisfire, err := repo.AppendExecution(created.TaskID, Execution{
		TaskVersion: 1, Trigger: TriggerMisfire, ScheduledFor: scheduledFor, Status: ExecutionStatusQueued,
	})
	if err != nil {
		t.Fatalf("misfire append: %v", err)
	}
	if createdMisfire || misfire.ExecutionID != first.ExecutionID {
		t.Fatalf("misfire for same occurrence must dedup: created=%v got=%s want=%s", createdMisfire, misfire.ExecutionID, first.ExecutionID)
	}
	// Manual runs dedupe on the caller idempotency key.
	manual, _, err := repo.AppendExecution(created.TaskID, Execution{
		TaskVersion: 1, Trigger: TriggerManual, ScheduledFor: repoBase, Status: ExecutionStatusQueued,
		IdempotencyKey: "req-123",
	})
	if err != nil {
		t.Fatal(err)
	}
	retry, _, err := repo.AppendExecution(created.TaskID, Execution{
		TaskVersion: 1, Trigger: TriggerManual, ScheduledFor: repoBase.Add(time.Second), Status: ExecutionStatusQueued,
		IdempotencyKey: "req-123",
	})
	if err != nil || retry.ExecutionID != manual.ExecutionID {
		t.Fatalf("manual retry created a new execution: %+v", retry)
	}
	other, _, err := repo.AppendExecution(created.TaskID, Execution{
		TaskVersion: 1, Trigger: TriggerManual, ScheduledFor: repoBase, Status: ExecutionStatusQueued,
		IdempotencyKey: "req-456",
	})
	if err != nil || other.ExecutionID == manual.ExecutionID {
		t.Fatalf("distinct idempotency key must create: %+v", other)
	}
	list, _ := repo.ListExecutions(created.TaskID, 0)
	if len(list) != 3 {
		t.Fatalf("executions = %d, want 3", len(list))
	}
}

func TestExecutionsPersistAcrossReopen(t *testing.T) {
	repo, clock, genDir := openTestRepo(t)
	created, _ := repo.Create(sampleTask("reopen-exec"))
	scheduledFor := repoBase.Add(time.Hour)
	if _, _, err := repo.AppendExecution(created.TaskID, Execution{
		TaskVersion: 1, Trigger: TriggerScheduled, ScheduledFor: scheduledFor, Status: ExecutionStatusQueued,
	}); err != nil {
		t.Fatal(err)
	}
	repo.Close()
	reopened := reopenTestRepo(t, genDir, clock)
	list, err := reopened.ListExecutions(created.TaskID, 0)
	if err != nil || len(list) != 1 || list[0].Status != ExecutionStatusQueued {
		t.Fatalf("executions after reopen = %+v, %v", list, err)
	}
	// Idempotency also survives restart.
	dup, _, err := reopened.AppendExecution(created.TaskID, Execution{
		TaskVersion: 1, Trigger: TriggerScheduled, ScheduledFor: scheduledFor, Status: ExecutionStatusQueued,
	})
	if err != nil || dup.ExecutionID != list[0].ExecutionID {
		t.Fatalf("restart dedup created a duplicate: %+v", dup)
	}
}

func TestListExecutionsOrderAndLimit(t *testing.T) {
	repo, _, _ := openTestRepo(t)
	created, _ := repo.Create(sampleTask("history"))
	for i := 0; i < 60; i++ {
		scheduledFor := repoBase.Add(time.Duration(i+1) * time.Hour)
		if _, _, err := repo.AppendExecution(created.TaskID, Execution{
			TaskVersion: 1, Trigger: TriggerScheduled, ScheduledFor: scheduledFor,
			StartedAt: scheduledFor.Add(time.Second), Status: ExecutionStatusSuccess,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Default limit projects the most recent 50, newest first.
	list, err := repo.ListExecutions(created.TaskID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != DefaultExecutionLimit {
		t.Fatalf("default list = %d, want %d", len(list), DefaultExecutionLimit)
	}
	for i := 1; i < len(list); i++ {
		if list[i-1].ScheduledFor.Before(list[i].ScheduledFor) {
			t.Fatalf("executions not newest-first at %d: %v then %v", i, list[i-1].ScheduledFor, list[i].ScheduledFor)
		}
	}
	if want := repoBase.Add(60 * time.Hour); !list[0].ScheduledFor.Equal(want) {
		t.Fatalf("newest = %v, want %v", list[0].ScheduledFor, want)
	}
	// Explicit smaller limit.
	if list, _ := repo.ListExecutions(created.TaskID, 10); len(list) != 10 {
		t.Fatalf("limit 10 = %d", len(list))
	}
	// Limits above the cap are clamped to 50.
	if list, _ := repo.ListExecutions(created.TaskID, 100); len(list) != MaxExecutionLimit {
		t.Fatalf("limit 100 = %d, want %d", len(list), MaxExecutionLimit)
	}
}

func TestExecutionsJournalTailTruncation(t *testing.T) {
	repo, clock, genDir := openTestRepo(t)
	created, _ := repo.Create(sampleTask("exec-tail"))
	scheduledFor := repoBase.Add(time.Hour)
	if _, _, err := repo.AppendExecution(created.TaskID, Execution{
		TaskVersion: 1, Trigger: TriggerScheduled, ScheduledFor: scheduledFor, Status: ExecutionStatusQueued,
	}); err != nil {
		t.Fatal(err)
	}
	repo.Close()
	path := filepath.Join(genDir, "scheduled-tasks", "executions", created.TaskID+".jsonl")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	file.WriteString(`{"seq":2,"payload":{"execution_id":"exec_partial`)
	file.Close()
	reopened := reopenTestRepo(t, genDir, clock)
	list, err := reopened.ListExecutions(created.TaskID, 0)
	if err != nil || len(list) != 1 {
		t.Fatalf("after tail truncation = %+v, %v", list, err)
	}
}

func TestExecutionsJournalMidCorruptionFailsClosed(t *testing.T) {
	repo, _, genDir := openTestRepo(t)
	created, _ := repo.Create(sampleTask("exec-corrupt"))
	for i := 0; i < 2; i++ {
		if _, _, err := repo.AppendExecution(created.TaskID, Execution{
			TaskVersion: 1, Trigger: TriggerScheduled,
			ScheduledFor: repoBase.Add(time.Duration(i+1) * time.Hour), Status: ExecutionStatusQueued,
		}); err != nil {
			t.Fatal(err)
		}
	}
	repo.Close()
	path := filepath.Join(genDir, "scheduled-tasks", "executions", created.TaskID+".jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[10] ^= 0xFF
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	reopened := reopenTestRepo(t, genDir, RealClock{})
	if _, err := reopened.ListExecutions(created.TaskID, 0); !errors.Is(err, filestore.ErrCorruptJournal) {
		t.Fatalf("mid-file corruption = %v, want ErrCorruptJournal", err)
	}
}

func TestSealInterrupted(t *testing.T) {
	repo, clock, genDir := openTestRepo(t)
	created, _ := repo.Create(sampleTask("seal"))
	queuedAt := repoBase.Add(time.Hour)
	queued, _, _ := repo.AppendExecution(created.TaskID, Execution{
		TaskVersion: 1, Trigger: TriggerScheduled, ScheduledFor: queuedAt, Status: ExecutionStatusQueued,
	})
	running, _, _ := repo.AppendExecution(created.TaskID, Execution{
		TaskVersion: 1, Trigger: TriggerScheduled, ScheduledFor: queuedAt.Add(time.Hour), Status: ExecutionStatusRunning,
	})
	terminal, _, _ := repo.AppendExecution(created.TaskID, Execution{
		TaskVersion: 1, Trigger: TriggerScheduled, ScheduledFor: queuedAt.Add(2 * time.Hour), Status: ExecutionStatusSuccess,
	})
	clock.Advance(3 * time.Hour)
	sealed, err := repo.SealInterrupted(created.TaskID, clock)
	if err != nil {
		t.Fatalf("SealInterrupted: %v", err)
	}
	if len(sealed) != 2 {
		t.Fatalf("sealed = %d, want 2 (queued+running)", len(sealed))
	}
	list, _ := repo.ListExecutions(created.TaskID, 0)
	byID := map[string]Execution{}
	for _, exec := range list {
		byID[exec.ExecutionID] = exec
	}
	for _, id := range []string{queued.ExecutionID, running.ExecutionID} {
		exec := byID[id]
		if exec.Status != ExecutionStatusInterrupted {
			t.Fatalf("execution %s status = %s, want interrupted", id, exec.Status)
		}
		if exec.FinishedAt == nil || !exec.FinishedAt.Equal(clock.Now().UTC()) {
			t.Fatalf("execution %s finished_at = %v", id, exec.FinishedAt)
		}
		if exec.ErrorCode == "" {
			t.Fatalf("execution %s has no error code after sealing", id)
		}
	}
	if byID[terminal.ExecutionID].Status != ExecutionStatusSuccess {
		t.Fatalf("terminal execution was touched: %+v", byID[terminal.ExecutionID])
	}
	// Sealing is stable across restart: nothing left to seal.
	repo.Close()
	reopened := reopenTestRepo(t, genDir, clock)
	again, err := reopened.SealInterrupted(created.TaskID, clock)
	if err != nil || len(again) != 0 {
		t.Fatalf("second seal = %v, %v", again, err)
	}
	if list, _ := reopened.ListExecutions(created.TaskID, 0); len(list) != 3 {
		t.Fatalf("executions after reopen = %d, want 3", len(list))
	}
}

// Repository errors carry identity and codes only; task content never leaks
// into them (contract §7).
func TestRepositoryErrorsNeverContainContent(t *testing.T) {
	repo, _, _ := openTestRepo(t)
	secret := "SECRET-PROMPT-7xk29q-do-not-leak"
	task := sampleTask("leak-check")
	task.Action.Content = secret
	created, err := repo.Create(task)
	if err != nil {
		t.Fatal(err)
	}
	collected := []error{}
	if _, err := repo.Get("sched_missing"); err != nil {
		collected = append(collected, err)
	}
	if _, err := repo.Update(created.TaskID, 42, func(t Task) (Task, error) { return t, nil }); err != nil {
		collected = append(collected, err)
	}
	if err := repo.Delete("sched_missing", 1, ""); err != nil {
		collected = append(collected, err)
	}
	if _, _, err := repo.AppendExecution(created.TaskID, Execution{ExecutionID: "exec_missing", Status: ExecutionStatusRunning}); err != nil {
		collected = append(collected, err)
	}
	if len(collected) != 4 {
		t.Fatalf("expected 4 errors, got %d", len(collected))
	}
	for _, err := range collected {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaks task content: %q", err.Error())
		}
	}
}

// Task definitions must not persist credentials or resolved profiles: the
// persisted snapshot contains only the opaque profile ID (contract §7).
func TestSnapshotContainsNoCredentials(t *testing.T) {
	repo, _, genDir := openTestRepo(t)
	task := sampleTask("no-secrets")
	task.Execution.ProfileID = "profile_opaque_id"
	if _, err := repo.Create(task); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{
		filepath.Join(genDir, "scheduled-tasks", "state.json"),
		filepath.Join(genDir, "scheduled-tasks", "mutations.jsonl"),
	} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"api_key", "apiKey", "Authorization", "Bearer "} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("%s contains forbidden material %q", file, forbidden)
			}
		}
	}
}

func TestListExecutionsTombstonedTask(t *testing.T) {
	repo, _, _ := openTestRepo(t)
	created, _ := repo.Create(sampleTask("gone"))
	if _, _, err := repo.AppendExecution(created.TaskID, Execution{
		TaskVersion: 1, Trigger: TriggerScheduled, ScheduledFor: repoBase, Status: ExecutionStatusSuccess,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(created.TaskID, created.Version, ""); err != nil {
		t.Fatal(err)
	}
	// User-visible history disappears with the task (contract §7).
	if _, err := repo.ListExecutions(created.TaskID, 0); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("ListExecutions tombstoned = %v, want ErrTaskNotFound", err)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	repo, _, _ := openTestRepo(t)
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}
}

func ExampleNextOccurrence() {
	sched := Schedule{Kind: ScheduleKindCron, Cron: "0 9 * * 1-5", Timezone: "Asia/Shanghai"}
	after := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC) // Saturday
	next, _ := NextOccurrence(sched, after)
	fmt.Println(next.UTC().Format(time.RFC3339))
	// Output: 2026-07-27T01:00:00Z
}
