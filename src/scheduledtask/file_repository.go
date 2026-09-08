package scheduledtask

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"awaken/src/filestore"

	"github.com/google/uuid"
)

// FileRepository persists scheduled tasks inside a FileStore generation
// directory (contract [持久化与 authority](docs/12_定时任务/定时执行与提醒/实现方案.md#持久化与-authority)):
//
//	<generation>/scheduled-tasks/
//	  mutations.jsonl          (definition/version/enable/thread/complete/tombstone authority)
//	  state.json               (rebuildable snapshot)
//	  executions/<task-id>.jsonl (per-task append-only execution history)
//
// Every definition mutation is journal-first: append (fsynced) -> apply ->
// rewrite state.json via WriteFileAtomic. Execution history uses one
// filestore.Journal per task, inheriting committed-offset tail truncation
// and mid-file fail-closed recovery.
//
// Lock policy: definition and execution writes hold the generation-level
// <generation>/locks/store.lock (owner "awaken-core", TTL 10s) for the
// duration of one write transaction only — the same short-transaction
// pattern FileDesktopStore uses for the very same lock file. Holding the
// lock for the repository lifetime was considered and rejected: the
// FileDesktopStore write path acquires this lock too and would starve
// behind a permanently held lock, deadlocking project lifecycle writes.
// Short transactions cannot deadlock: there is a single shared lock (no
// lock ordering), and the repository never calls back into the runtime
// package while holding it. In-process writers are serialized by the
// repository mutex, so the lock only arbitrates across processes.
type FileRepository struct {
	genDir string
	dir    string

	mu    sync.Mutex
	clock Clock

	journal *filestore.Journal
	state   *repositoryState

	execJournals map[string]*filestore.Journal
	execIndex    map[string]map[string]Execution // task_id -> execution_id -> newest record

	// deleteReceipts maps Idempotency-Key -> receipt, rebuilt from the
	// state snapshot which is itself journal-authoritative.
	deleteReceipts map[string]deleteReceipt

	// lockWaitTimeout bounds how long a write polls for the store lock;
	// tests shorten it. Not configurable in production.
	lockWaitTimeout time.Duration

	closed bool
}

var _ Repository = (*FileRepository)(nil)

const (
	mutationKindCreate            = "create"
	mutationKindUpdate            = "update"
	mutationKindPause             = "pause"
	mutationKindResume            = "resume"
	mutationKindBindThread        = "bind_thread"
	mutationKindConsumeOccurrence = "consume_occurrence"
	mutationKindComplete          = "complete"
	mutationKindTombstone         = "tombstone"
	defaultLockWaitTimeout        = 5 * time.Second
	storeLockOwner                = "awaken-core"
	storeLockTTL                  = 10 * time.Second
	interruptedErrorCode          = "INTERRUPTED"
	maxDeleteReceipts             = 1000
)

// deleteReceipt records the successful outcome of a DELETE so that a
// transport retry with the same Idempotency-Key can replay the 204 instead
// of returning 404 after the task is tombstoned.
type deleteReceipt struct {
	IdempotencyKey string    `json:"idempotency_key"`
	TaskID         string    `json:"task_id"`
	Version        int64     `json:"version"`
	DeletedAt      time.Time `json:"deleted_at"`
}

// mutation is one durable definition transaction. All kinds except
// tombstone carry the full post-transaction task snapshot so replay is a
// deterministic set, never a recomputation. Tombstones carry an optional
// idempotency key so the receipt is atomically committed with the tombstone.
type mutation struct {
	Kind           string    `json:"kind"`
	TaskID         string    `json:"task_id"`
	Version        int64     `json:"version"`
	Task           *Task     `json:"task,omitempty"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	At             time.Time `json:"at"`
}

// taskRecord keeps tombstoned tasks in the snapshot so the journal version
// chain stays verifiable across restart.
type taskRecord struct {
	Task    Task `json:"task"`
	Deleted bool `json:"deleted"`
}

// repositoryState is the rebuildable state.json snapshot.
type repositoryState struct {
	AppliedSeq     uint64                   `json:"applied_seq"`
	Tasks          map[string]taskRecord    `json:"tasks"`
	DeleteReceipts map[string]deleteReceipt `json:"delete_receipts,omitempty"`
}

func newRepositoryState() *repositoryState {
	return &repositoryState{
		Tasks:          map[string]taskRecord{},
		DeleteReceipts: map[string]deleteReceipt{},
	}
}

// OpenFileRepository opens (or initializes) the scheduled-task store inside
// genDir, the absolute path of a FileStore generation directory resolved by
// the caller via filestore.ReadCurrentGeneration. A nil clock uses
// RealClock. A missing or corrupt state.json is rebuilt from the mutation
// journal and rewritten; a mid-file corrupt journal fails closed.
func OpenFileRepository(genDir string, clock Clock) (*FileRepository, error) {
	if clock == nil {
		clock = RealClock{}
	}
	dir := filepath.Join(genDir, "scheduled-tasks")
	for _, d := range []string{
		filepath.Join(genDir, "locks"),
		dir,
		filepath.Join(dir, "executions"),
	} {
		if err := filestore.EnsureDir(d); err != nil {
			return nil, err
		}
	}
	repo := &FileRepository{
		genDir:          genDir,
		dir:             dir,
		clock:           clock,
		execJournals:    map[string]*filestore.Journal{},
		execIndex:       map[string]map[string]Execution{},
		lockWaitTimeout: defaultLockWaitTimeout,
	}
	if err := repo.openState(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *FileRepository) mutationsPath() string {
	return filepath.Join(r.dir, "mutations.jsonl")
}

func (r *FileRepository) statePath() string {
	return filepath.Join(r.dir, "state.json")
}

func (r *FileRepository) lockPath() string {
	return filepath.Join(r.genDir, "locks", "store.lock")
}

func (r *FileRepository) executionsDir() string {
	return filepath.Join(r.dir, "executions")
}

func (r *FileRepository) openState() error {
	journal, _, err := filestore.OpenJournal(r.mutationsPath())
	if err != nil {
		return err
	}
	r.journal = journal

	state := newRepositoryState()
	if err := filestore.ReadJSON(r.statePath(), state); err != nil {
		// Missing or corrupt snapshot: rebuild from the journal, which stays
		// authoritative. A corrupt snapshot is never treated as an empty
		// store silently — the journal must fold cleanly or Open fails.
		state = newRepositoryState()
	}
	if state.Tasks == nil {
		state.Tasks = map[string]taskRecord{}
	}
	records, err := journal.Records()
	if err != nil {
		return err
	}
	rolledForward := false
	for _, record := range records {
		if record.Seq <= state.AppliedSeq {
			continue
		}
		var m mutation
		if err := json.Unmarshal(record.Payload, &m); err != nil {
			return fmt.Errorf("%w: undecodable mutation at seq %d", ErrCorruptState, record.Seq)
		}
		if err := applyMutation(state, m); err != nil {
			return fmt.Errorf("%w at seq %d: %v", ErrCorruptState, record.Seq, err)
		}
		state.AppliedSeq = record.Seq
		rolledForward = true
	}
	r.state = state
	if state.DeleteReceipts != nil {
		r.deleteReceipts = state.DeleteReceipts
	} else {
		r.deleteReceipts = map[string]deleteReceipt{}
	}
	if rolledForward {
		return r.persistStateLocked()
	}
	return nil
}

// applyMutation folds one mutation into the state, enforcing a strictly
// increasing version chain per task. Any gap or replay of an existing
// version is corruption and fails closed.
func applyMutation(state *repositoryState, m mutation) error {
	existing, exists := state.Tasks[m.TaskID]
	switch m.Kind {
	case mutationKindCreate:
		if exists || m.Version != 1 || m.Task == nil {
			return fmt.Errorf("invalid create mutation for task %s", m.TaskID)
		}
		state.Tasks[m.TaskID] = taskRecord{Task: *m.Task}
		return nil
	case mutationKindUpdate, mutationKindPause, mutationKindResume,
		mutationKindBindThread, mutationKindConsumeOccurrence, mutationKindComplete:
		if !exists || existing.Deleted || m.Task == nil {
			return fmt.Errorf("mutation %s without live task %s", m.Kind, m.TaskID)
		}
		if m.Version != existing.Task.Version+1 {
			return fmt.Errorf("version chain broken for task %s: have %d, mutation %d", m.TaskID, existing.Task.Version, m.Version)
		}
		state.Tasks[m.TaskID] = taskRecord{Task: *m.Task}
		return nil
	case mutationKindTombstone:
		if !exists || existing.Deleted {
			return fmt.Errorf("tombstone without live task %s", m.TaskID)
		}
		if m.Version != existing.Task.Version+1 {
			return fmt.Errorf("version chain broken for task %s: have %d, mutation %d", m.TaskID, existing.Task.Version, m.Version)
		}
		state.Tasks[m.TaskID] = taskRecord{Task: existing.Task, Deleted: true}
		if m.IdempotencyKey != "" {
			if state.DeleteReceipts == nil {
				state.DeleteReceipts = map[string]deleteReceipt{}
			}
			state.DeleteReceipts[m.IdempotencyKey] = deleteReceipt{
				IdempotencyKey: m.IdempotencyKey,
				TaskID:         m.TaskID,
				Version:        m.Version,
				DeletedAt:      m.At,
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown mutation kind %q", m.Kind)
	}
}

// persistStateLocked rewrites state.json atomically. Caller holds r.mu and
// the store lock.
func (r *FileRepository) persistStateLocked() error {
	encoded, err := json.MarshalIndent(r.state, "", "  ")
	if err != nil {
		return fmt.Errorf("scheduledtask: encode state snapshot: %w", err)
	}
	if err := filestore.WriteFileAtomic(r.statePath(), append(encoded, '\n')); err != nil {
		return err
	}
	return nil
}

// acquireLock takes the short-lived generation store lock, polling briefly
// like the FileDesktopStore write path. Acquisition failure is a hard error;
// there is no lock-free fallback.
func (r *FileRepository) acquireLock() (*filestore.FileLock, error) {
	deadline := time.Now().Add(r.lockWaitTimeout)
	for {
		lock, err := filestore.AcquireLock(r.lockPath(), storeLockOwner, storeLockTTL)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, filestore.ErrLockHeld) || time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// commitMutationLocked appends the mutation journal-first, applies it and
// rewrites the snapshot, all under the store lock. Caller holds r.mu.
func (r *FileRepository) commitMutationLocked(m mutation) error {
	lock, err := r.acquireLock()
	if err != nil {
		return err
	}
	defer lock.Release()
	return r.commitMutationWithStoreLockLocked(m)
}

// commitMutationWithStoreLockLocked commits one definition mutation when the
// caller already holds both r.mu and the generation store lock.
func (r *FileRepository) commitMutationWithStoreLockLocked(m mutation) error {
	record, err := r.journal.Append(m)
	if err != nil {
		return err
	}
	if err := applyMutation(r.state, m); err != nil {
		return err
	}
	r.state.AppliedSeq = record.Seq
	return r.persistStateLocked()
}

func (r *FileRepository) liveTaskLocked(taskID string) (Task, error) {
	record, ok := r.state.Tasks[taskID]
	if !ok || record.Deleted {
		return Task{}, ErrTaskNotFound
	}
	return record.Task, nil
}

// Create implements Repository.Create.
func (r *FileRepository) Create(task Task) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return Task{}, fmt.Errorf("scheduledtask: repository is closed")
	}
	now := r.clock.Now().UTC()
	// Client-supplied identity, state and timestamps are untrusted: Core
	// generates all of them (contract [校验](docs/12_定时任务/定时执行与提醒/实现方案.md#校验)).
	created := task
	created.TaskID = TaskIDPrefix + uuid.NewString()
	created.Version = 1
	created.ThreadID = ""
	created.ConsumedOccurrences = 0
	created.LastProcessedOccurrence = nil
	created.Completed = false
	created.CreatedAt = now
	created.UpdatedAt = now
	if err := r.commitMutationLocked(mutation{
		Kind:    mutationKindCreate,
		TaskID:  created.TaskID,
		Version: 1,
		Task:    &created,
		At:      now,
	}); err != nil {
		return Task{}, err
	}
	return created, nil
}

// Get implements Repository.Get.
func (r *FileRepository) Get(taskID string) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.liveTaskLocked(taskID)
}

// List implements Repository.List.
func (r *FileRepository) List() ([]Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	tasks := make([]Task, 0, len(r.state.Tasks))
	for _, record := range r.state.Tasks {
		if record.Deleted {
			continue
		}
		tasks = append(tasks, record.Task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		if !tasks[i].CreatedAt.Equal(tasks[j].CreatedAt) {
			return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
		}
		return tasks[i].TaskID < tasks[j].TaskID
	})
	return tasks, nil
}

// Update implements Repository.Update. The mutate callback runs while the
// repository mutex and the generation store lock are held; it must be fast
// and must never call back into this repository or the runtime package.
func (r *FileRepository) Update(taskID string, expectedVersion int64, mutate func(Task) (Task, error)) (Task, error) {
	return r.updateWithKind(taskID, expectedVersion, mutationKindUpdate, mutate)
}

func (r *FileRepository) updateWithKind(taskID string, expectedVersion int64, kind string, mutate func(Task) (Task, error)) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return Task{}, fmt.Errorf("scheduledtask: repository is closed")
	}
	lock, err := r.acquireLock()
	if err != nil {
		return Task{}, err
	}
	defer lock.Release()
	current, err := r.liveTaskLocked(taskID)
	if err != nil {
		return Task{}, err
	}
	if current.Version != expectedVersion {
		return Task{}, fmt.Errorf("%w: task %s at version %d, expected %d", ErrVersionConflict, taskID, current.Version, expectedVersion)
	}
	updated, err := mutate(current)
	if err != nil {
		return Task{}, err
	}
	// Identity and bookkeeping fields are owned by Core.
	updated.TaskID = current.TaskID
	updated.CreatedAt = current.CreatedAt
	updated.Version = current.Version + 1
	updated.UpdatedAt = r.clock.Now().UTC()
	m := mutation{Kind: kind, TaskID: taskID, Version: updated.Version, Task: &updated, At: updated.UpdatedAt}
	if err := r.commitMutationWithStoreLockLocked(m); err != nil {
		return Task{}, err
	}
	return updated, nil
}

// Delete implements Repository.Delete. It performs the version check,
// tombstone write and idempotency receipt atomically under the store lock.
// An idempotency key lets a lost 204 response be replayed as another 204
// instead of 404.
func (r *FileRepository) Delete(taskID string, expectedVersion int64, idempotencyKey string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("scheduledtask: repository is closed")
	}
	lock, err := r.acquireLock()
	if err != nil {
		return err
	}
	defer lock.Release()
	if idempotencyKey != "" {
		if receipt, ok := r.deleteReceipts[idempotencyKey]; ok {
			if receipt.TaskID == taskID {
				return nil
			}
			return fmt.Errorf("%w: idempotency key %q already used for task %s", ErrInvalidRequest, idempotencyKey, receipt.TaskID)
		}
	}
	current, err := r.liveTaskLocked(taskID)
	if err != nil {
		return err
	}
	if current.Version != expectedVersion {
		return fmt.Errorf("%w: task %s at version %d, expected %d", ErrVersionConflict, taskID, current.Version, expectedVersion)
	}
	if err := r.commitMutationWithStoreLockLocked(mutation{
		Kind:           mutationKindTombstone,
		TaskID:         taskID,
		Version:        current.Version + 1,
		IdempotencyKey: idempotencyKey,
		At:             r.clock.Now().UTC(),
	}); err != nil {
		return err
	}
	// Receipt is applied atomically by applyMutation; keep the in-memory map
	// in sync in case state.json is not rewritten.
	if idempotencyKey != "" {
		if r.state.DeleteReceipts == nil {
			r.state.DeleteReceipts = map[string]deleteReceipt{}
		}
		r.state.DeleteReceipts[idempotencyKey] = r.deleteReceipts[idempotencyKey]
		if len(r.deleteReceipts) > maxDeleteReceipts {
			r.trimDeleteReceiptsLocked()
		}
	}
	return nil
}

func (r *FileRepository) trimDeleteReceiptsLocked() {
	// Keep the most recent receipts; the journal is append-only and compacted
	// by FileStore generation rotation, so in-memory map size is bounded here.
	type keyed struct {
		key     string
		deleted time.Time
	}
	all := make([]keyed, 0, len(r.deleteReceipts))
	for key, receipt := range r.deleteReceipts {
		all = append(all, keyed{key: key, deleted: receipt.DeletedAt})
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].deleted.Before(all[j].deleted)
	})
	for i := 0; i < len(all)-maxDeleteReceipts; i++ {
		delete(r.deleteReceipts, all[i].key)
		delete(r.state.DeleteReceipts, all[i].key)
	}
}

// SetPaused implements Repository.SetPaused.
func (r *FileRepository) SetPaused(taskID string, expectedVersion int64) (Task, error) {
	return r.updateWithKind(taskID, expectedVersion, mutationKindPause, func(task Task) (Task, error) {
		task.Enabled = false
		return task, nil
	})
}

// SetResumed implements Repository.SetResumed.
func (r *FileRepository) SetResumed(taskID string, expectedVersion int64) (Task, error) {
	return r.updateWithKind(taskID, expectedVersion, mutationKindResume, func(task Task) (Task, error) {
		task.Enabled = true
		return task, nil
	})
}

// BindThread implements Repository.BindThread.
func (r *FileRepository) BindThread(taskID string, expectedVersion int64, threadID string) (Task, error) {
	return r.updateWithKind(taskID, expectedVersion, mutationKindBindThread, func(task Task) (Task, error) {
		task.ThreadID = threadID
		return task, nil
	})
}

// ConsumeOccurrence implements Repository.ConsumeOccurrence. The read, version
// check and mutation are performed under one repository mutex/store-lock
// transaction so concurrent user edits cannot race with occurrence consumption.
func (r *FileRepository) ConsumeOccurrence(taskID string, occurrence time.Time) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return Task{}, fmt.Errorf("scheduledtask: repository is closed")
	}
	lock, err := r.acquireLock()
	if err != nil {
		return Task{}, err
	}
	defer lock.Release()
	current, err := r.liveTaskLocked(taskID)
	if err != nil {
		return Task{}, err
	}
	occurrence = occurrence.UTC()
	updated := current
	updated.ConsumedOccurrences++
	if updated.LastProcessedOccurrence == nil || occurrence.After(*updated.LastProcessedOccurrence) {
		updated.LastProcessedOccurrence = &occurrence
	}
	updated.Version = current.Version + 1
	updated.UpdatedAt = r.clock.Now().UTC()
	m := mutation{
		Kind:    mutationKindConsumeOccurrence,
		TaskID:  taskID,
		Version: updated.Version,
		Task:    &updated,
		At:      updated.UpdatedAt,
	}
	if err := r.commitMutationWithStoreLockLocked(m); err != nil {
		return Task{}, err
	}
	return updated, nil
}

// MarkCompleted implements Repository.MarkCompleted.
func (r *FileRepository) MarkCompleted(taskID string) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return Task{}, fmt.Errorf("scheduledtask: repository is closed")
	}
	lock, err := r.acquireLock()
	if err != nil {
		return Task{}, err
	}
	defer lock.Release()
	current, err := r.liveTaskLocked(taskID)
	if err != nil {
		return Task{}, err
	}
	if current.Completed {
		return current, nil
	}
	updated := current
	updated.Completed = true
	updated.Version = current.Version + 1
	updated.UpdatedAt = r.clock.Now().UTC()
	m := mutation{
		Kind:    mutationKindComplete,
		TaskID:  taskID,
		Version: updated.Version,
		Task:    &updated,
		At:      updated.UpdatedAt,
	}
	if err := r.commitMutationWithStoreLockLocked(m); err != nil {
		return Task{}, err
	}
	return updated, nil
}

// executionJournalLocked returns the open per-task execution journal,
// opening and indexing it on first use. Caller holds r.mu.
func (r *FileRepository) executionJournalLocked(taskID string) (*filestore.Journal, error) {
	if journal, ok := r.execJournals[taskID]; ok {
		return journal, nil
	}
	path := filepath.Join(r.executionsDir(), taskID+".jsonl")
	journal, _, err := filestore.OpenJournal(path)
	if err != nil {
		return nil, err
	}
	records, err := journal.Records()
	if err != nil {
		journal.Close()
		return nil, err
	}
	index := map[string]Execution{}
	for _, record := range records {
		var exec Execution
		if err := json.Unmarshal(record.Payload, &exec); err != nil {
			journal.Close()
			return nil, fmt.Errorf("%w: undecodable execution record at seq %d for task %s", ErrCorruptState, record.Seq, taskID)
		}
		index[exec.ExecutionID] = exec
	}
	r.execJournals[taskID] = journal
	r.execIndex[taskID] = index
	return journal, nil
}

// AppendExecution implements Repository.AppendExecution.
func (r *FileRepository) AppendExecution(taskID string, exec Execution) (Execution, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return Execution{}, false, fmt.Errorf("scheduledtask: repository is closed")
	}
	if _, err := r.liveTaskLocked(taskID); err != nil {
		return Execution{}, false, err
	}
	journal, err := r.executionJournalLocked(taskID)
	if err != nil {
		return Execution{}, false, err
	}
	index := r.execIndex[taskID]

	if exec.ExecutionID != "" {
		current, ok := index[exec.ExecutionID]
		if !ok {
			return Execution{}, false, fmt.Errorf("%w: %s", ErrExecutionNotFound, exec.ExecutionID)
		}
		if current.Status.Terminal() {
			return Execution{}, false, fmt.Errorf("%w: execution %s already %s", ErrInvalidTransition, exec.ExecutionID, current.Status)
		}
		// Identity fields belong to the creation record; transitions never
		// rewrite them. Forcing continuity keeps the folded index usable for
		// idempotency dedup even when a caller omits them on a transition.
		exec.TaskID = taskID
		exec.Trigger = current.Trigger
		exec.ScheduledFor = current.ScheduledFor
		exec.IdempotencyKey = current.IdempotencyKey
		record, err := r.appendExecutionRecordLocked(journal, index, exec)
		return record, false, err
	}

	// New execution: deduplicate on the idempotency key before creating.
	for _, existing := range index {
		if executionKeyMatches(existing, exec) {
			return existing, false, nil
		}
	}
	exec.ExecutionID = ExecutionIDPrefix + uuid.NewString()
	exec.TaskID = taskID
	record, err := r.appendExecutionRecordLocked(journal, index, exec)
	return record, err == nil, err
}

// executionKeyMatches reports whether an incoming create duplicates an
// existing execution under the contract [并发与幂等](docs/12_定时任务/定时执行与提醒/实现方案.md#并发与幂等) idempotency rules. Manual
// triggers dedupe on the caller-supplied idempotency key. Scheduled and
// misfire triggers share the same occurrence identity: the same scheduled
// for time can only create one execution, even if a crash changes the
// trigger label from scheduled to misfire during recovery.
func executionKeyMatches(existing, incoming Execution) bool {
	if incoming.Trigger == TriggerManual {
		return incoming.IdempotencyKey != "" &&
			existing.Trigger == TriggerManual &&
			existing.IdempotencyKey == incoming.IdempotencyKey
	}
	return existing.Trigger != TriggerManual &&
		existing.ScheduledFor.Equal(incoming.ScheduledFor)
}

func (r *FileRepository) appendExecutionRecordLocked(journal *filestore.Journal, index map[string]Execution, exec Execution) (Execution, error) {
	lock, err := r.acquireLock()
	if err != nil {
		return Execution{}, err
	}
	defer lock.Release()
	if _, err := journal.Append(exec); err != nil {
		return Execution{}, err
	}
	index[exec.ExecutionID] = exec
	return exec, nil
}

// ListExecutions implements Repository.ListExecutions.
func (r *FileRepository) ListExecutions(taskID string, limit int) ([]Execution, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.liveTaskLocked(taskID); err != nil {
		// Executions of tombstoned tasks are history of a deleted task and
		// disappear with it (contract [持久化与 authority](docs/12_定时任务/定时执行与提醒/实现方案.md#持久化与-authority)); an unknown task is not found.
		return nil, err
	}
	if _, err := r.executionJournalLocked(taskID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > MaxExecutionLimit {
		limit = DefaultExecutionLimit
	}
	index := r.execIndex[taskID]
	executions := make([]Execution, 0, len(index))
	for _, exec := range index {
		executions = append(executions, exec)
	}
	sort.Slice(executions, func(i, j int) bool {
		if !executions[i].ScheduledFor.Equal(executions[j].ScheduledFor) {
			return executions[i].ScheduledFor.After(executions[j].ScheduledFor)
		}
		if !executions[i].StartedAt.Equal(executions[j].StartedAt) {
			return executions[i].StartedAt.After(executions[j].StartedAt)
		}
		return executions[i].ExecutionID > executions[j].ExecutionID
	})
	if len(executions) > limit {
		executions = executions[:limit]
	}
	return executions, nil
}

// SealInterrupted implements Repository.SealInterrupted.
func (r *FileRepository) SealInterrupted(taskID string, clock Clock) ([]Execution, error) {
	if clock == nil {
		clock = r.clock
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.liveTaskLocked(taskID); err != nil {
		return nil, err
	}
	journal, err := r.executionJournalLocked(taskID)
	if err != nil {
		return nil, err
	}
	index := r.execIndex[taskID]
	finishedAt := clock.Now().UTC()
	var sealed []Execution
	// Deterministic order for reproducible journals.
	ids := make([]string, 0, len(index))
	for id := range index {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		current := index[id]
		if current.Status != ExecutionStatusQueued && current.Status != ExecutionStatusRunning {
			continue
		}
		next := current
		next.Status = ExecutionStatusInterrupted
		next.FinishedAt = &finishedAt
		if next.ErrorCode == "" {
			next.ErrorCode = interruptedErrorCode
		}
		if _, err := r.appendExecutionRecordLocked(journal, index, next); err != nil {
			return sealed, err
		}
		sealed = append(sealed, next)
	}
	return sealed, nil
}

// Close releases the mutation journal and all per-task execution journals.
func (r *FileRepository) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	var errs []error
	if r.journal != nil {
		errs = append(errs, r.journal.Close())
	}
	for _, journal := range r.execJournals {
		errs = append(errs, journal.Close())
	}
	return errors.Join(errs...)
}
