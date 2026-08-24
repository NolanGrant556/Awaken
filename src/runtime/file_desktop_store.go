package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/filestore"
)

const fileStoreGenerationID = "gen-001"

// activeSessionsDir resolves the sessions directory of the active store
// generation for package-level helpers (attachments, transcript paths) that
// have no store handle. Before the store is first opened it anchors to the
// default generation under the same root so both sides agree on one
// namespace; with no configured root it uses an isolated temp namespace and
// never touches the real ~/.awaken implicitly.
func activeSessionsDir() string {
	root := resolveRuntimeStoreRoot()
	if root == "" {
		return filepath.Join(os.TempDir(), "awaken-runtime-sessions")
	}
	if genDir, _, err := filestore.ReadCurrentGeneration(root); err == nil {
		return filepath.Join(genDir, "sessions")
	}
	return filepath.Join(root, "store-generations", fileStoreGenerationID, "sessions")
}

// FileDesktopStore implements ThreadRepository and
// ProjectLifecycleRepository on the frozen FileStore layout:
//
//	<root>/CURRENT
//	<root>/store-generations/<gen>/
//	  FORMAT.json
//	  sessions/<thread-id>/{metadata.json, journal.jsonl}
//	  lifecycle/mutations.jsonl   (transaction journal: source of truth)
//	  lifecycle/state.json        (materialized snapshot: rebuildable)
//	  locks/store.lock            (short-lived cross-process mutation lock)
//	  scheduled-tasks/            (scheduled task authority; see src/scheduledtask)
//	  scheduled-tasks/executions/ (per-task execution history journals)
//
// Every lifecycle mutation is journal-first: the mutation record carries all
// generated identity (operation IDs, job IDs, timestamps) so replay after a
// crash or a corrupt snapshot deterministically rebuilds the same state.
type FileDesktopStore struct {
	root string
	gen  string

	mu      sync.Mutex
	journal *filestore.Journal
	state   *fileLifecycleState
}

var _ ThreadRepository = (*FileDesktopStore)(nil)
var _ ProjectLifecycleRepository = (*FileDesktopStore)(nil)

type fileLifecycleState struct {
	AppliedSeq       uint64                      `json:"applied_seq"`
	Projects         map[string]Project          `json:"projects"`          // by project_id
	Operations       map[string]ProjectOperation `json:"operations"`        // by operation_id
	OperationKeys    map[string]string           `json:"operation_keys"`    // idempotency key -> operation_id
	OperationThreads map[string][]string         `json:"operation_threads"` // operation_id -> thread ids
	CleanupJobs      map[string]fileCleanupJob   `json:"cleanup_jobs"`      // by job_id
	DesktopState     map[string]string           `json:"desktop_state"`
	ProjectOrder     []string                    `json:"project_order,omitempty"`
}

func newFileLifecycleState() *fileLifecycleState {
	return &fileLifecycleState{
		Projects:         map[string]Project{},
		Operations:       map[string]ProjectOperation{},
		OperationKeys:    map[string]string{},
		OperationThreads: map[string][]string{},
		CleanupJobs:      map[string]fileCleanupJob{},
		DesktopState:     map[string]string{},
	}
}

type fileCleanupJob struct {
	JobID          string    `json:"job_id"`
	OperationID    string    `json:"operation_id"`
	Kind           string    `json:"kind"`
	TargetID       string    `json:"target_id"`
	Status         string    `json:"status"`
	Attempts       int       `json:"attempts"`
	NextAttemptAt  time.Time `json:"next_attempt_at,omitempty"`
	LeaseOwner     string    `json:"lease_owner,omitempty"`
	LeaseExpiresAt time.Time `json:"lease_expires_at,omitempty"`
	LastError      string    `json:"last_error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	CompletedAt    time.Time `json:"completed_at,omitempty"`
}

func (j fileCleanupJob) toCleanupJob() cleanupJob {
	return cleanupJob{
		JobID:          j.JobID,
		OperationID:    j.OperationID,
		Kind:           j.Kind,
		TargetID:       j.TargetID,
		Status:         j.Status,
		Attempts:       j.Attempts,
		NextAttemptAt:  j.NextAttemptAt,
		LeaseOwner:     j.LeaseOwner,
		LeaseExpiresAt: j.LeaseExpiresAt,
	}
}

// fileLifecycleMutation is one durable lifecycle transaction. Fields carry the
// complete post-transaction facts (full project/operation/job states) so
// applying the record is a deterministic set, never a re-computation.
type fileLifecycleMutation struct {
	Kind              string              `json:"kind"`
	Now               time.Time           `json:"now"`
	Projects          []Project           `json:"projects,omitempty"`
	Operations        []ProjectOperation  `json:"operations,omitempty"`
	OperationThreads  map[string][]string `json:"operation_threads,omitempty"`
	CleanupJobs       []fileCleanupJob    `json:"cleanup_jobs,omitempty"`
	SoftDeleteThreads []string            `json:"soft_delete_threads,omitempty"`
	SoftDeleteOpID    string              `json:"soft_delete_op_id,omitempty"`
	PurgeSessions     []string            `json:"purge_sessions,omitempty"`
	LastWorkspace     *string             `json:"last_workspace,omitempty"`
	ProjectOrder      *[]string           `json:"project_order,omitempty"`
}

type fileSessionMetadata struct {
	SchemaVersion            int               `json:"schema_version"`
	ThreadID                 string            `json:"thread_id"`
	Title                    string            `json:"title"`
	TitleSource              string            `json:"title_source,omitempty"`
	TitleStatus              string            `json:"title_status,omitempty"`
	TitleUpdatedAt           string            `json:"title_updated_at,omitempty"`
	TitleSeedRunID           string            `json:"title_seed_run_id,omitempty"`
	WorkspaceRoot            string            `json:"workspace_root"`
	ProjectID                string            `json:"project_id,omitempty"`
	ScopeID                  string            `json:"scope_id,omitempty"`
	LatestRunID              string            `json:"latest_run_id,omitempty"`
	CreatedAt                string            `json:"created_at"`
	UpdatedAt                string            `json:"updated_at"`
	Archived                 bool              `json:"archived,omitempty"`
	ArchivedAt               string            `json:"archived_at,omitempty"`
	Deleted                  bool              `json:"deleted,omitempty"`
	DeletedAt                string            `json:"deleted_at,omitempty"`
	DeleteOperationID        string            `json:"delete_operation_id,omitempty"`
	Preview                  string            `json:"preview,omitempty"`
	ModelProfileKey          string            `json:"model_profile_key,omitempty"`
	SelectedModelProfileID   string            `json:"selected_model_profile_id,omitempty"`
	ReasoningEffortByProfile map[string]string `json:"reasoning_effort_by_profile,omitempty"`
	// Source records the thread origin ("" for regular Desktop threads,
	// "scheduled_task" for scheduled-task dedicated threads).
	Source    string          `json:"source,omitempty"`
	Workspace WorkspaceConfig `json:"workspace"`
}

func newFileDesktopStore(root string) (*FileDesktopStore, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || !filepath.IsAbs(root) {
		return nil, fmt.Errorf("filestore root must be an absolute path, got %q", root)
	}
	if err := filestore.EnsureDir(root); err != nil {
		return nil, err
	}
	genDir, format, err := filestore.ReadCurrentGeneration(root)
	if err != nil {
		// A brand new isolated root may be initialized; a root that already
		// contains generations but has a broken CURRENT pointer fails closed.
		if _, statErr := os.Stat(filepath.Join(root, "store-generations")); statErr == nil {
			return nil, err
		}
		if _, initErr := filestore.InitGeneration(root, fileStoreGenerationID, filestore.Format{}); initErr != nil {
			return nil, initErr
		}
		if publishErr := filestore.PublishGeneration(root, fileStoreGenerationID); publishErr != nil {
			return nil, publishErr
		}
		genDir, format, err = filestore.ReadCurrentGeneration(root)
		if err != nil {
			return nil, err
		}
	}
	if err := format.CheckWritable(filestore.CurrentFormatVersion); err != nil {
		return nil, err
	}
	for _, dir := range []string{
		filepath.Join(genDir, "sessions"),
		filepath.Join(genDir, "lifecycle"),
		filepath.Join(genDir, "locks"),
		filepath.Join(genDir, "scheduled-tasks"),
		filepath.Join(genDir, "scheduled-tasks", "executions"),
	} {
		if err := filestore.EnsureDir(dir); err != nil {
			return nil, err
		}
	}
	store := &FileDesktopStore{root: root, gen: genDir}
	if err := store.openLifecycle(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *FileDesktopStore) openLifecycle() error {
	journal, _, err := filestore.OpenJournal(filepath.Join(s.gen, "lifecycle", "mutations.jsonl"))
	if err != nil {
		return err
	}
	s.journal = journal

	state := newFileLifecycleState()
	statePath := s.statePath()
	if err := filestore.ReadJSON(statePath, state); err != nil {
		if !os.IsNotExist(err) {
			// The snapshot only accelerates startup; a corrupt snapshot is
			// rebuilt from the lifecycle journal, which stays authoritative.
			state = newFileLifecycleState()
		} else {
			state = newFileLifecycleState()
		}
	}
	normalizeLoadedProjects(state)
	records, err := journal.Records()
	if err != nil {
		return err
	}
	rolledForward := false
	for _, record := range records {
		if record.Seq <= state.AppliedSeq {
			continue
		}
		var mutation fileLifecycleMutation
		if err := json.Unmarshal(record.Payload, &mutation); err != nil {
			return fmt.Errorf("filestore lifecycle record %d undecodable: %w", record.Seq, err)
		}
		if err := s.applyMutation(state, mutation); err != nil {
			return err
		}
		state.AppliedSeq = record.Seq
		rolledForward = true
	}
	s.state = state
	if rolledForward {
		return s.persistState()
	}
	return nil
}

// normalizeLoadedProjects upgrades a snapshot written before multi-folder
// projects existed. Such a snapshot has a WorkspaceRoot per project and no
// folder list; promoting it here means the rest of the store only ever sees the
// folder-list invariant.
func normalizeLoadedProjects(state *fileLifecycleState) {
	if state == nil || len(state.Projects) == 0 {
		return
	}
	for projectID, project := range state.Projects {
		state.Projects[projectID] = project.withNormalizedFolders()
	}
}

func (s *FileDesktopStore) statePath() string {
	return filepath.Join(s.gen, "lifecycle", "state.json")
}

func (s *FileDesktopStore) lockPath() string {
	return filepath.Join(s.gen, "locks", "store.lock")
}

// Prepare satisfies the store preparation hook used before run creation.
func (s *FileDesktopStore) Prepare() error {
	return nil
}

func (s *FileDesktopStore) UserDataRoot() string {
	return s.root
}

func (s *FileDesktopStore) sessionDir(threadID string) string {
	return filepath.Join(s.gen, "sessions", sanitizeSessionID(threadID))
}

func (s *FileDesktopStore) ThreadTranscriptPath(threadID string) string {
	return filepath.Join(s.sessionDir(threadID), "journal.jsonl")
}

func sanitizeSessionID(threadID string) string {
	threadID = strings.TrimSpace(threadID)
	replaced := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':':
			return '_'
		default:
			return r
		}
	}, threadID)
	if replaced == "" || replaced == "." || replaced == ".." {
		return "_invalid"
	}
	return replaced
}

// lockStore takes the short-lived cross-process mutation lock. Acquisition
// failure is a hard error: there is no lock-free fallback.
func (s *FileDesktopStore) lockStore() (*filestore.FileLock, error) {
	deadline := time.Now().Add(5 * time.Second)
	for {
		lock, err := filestore.AcquireLock(s.lockPath(), "awaken-core", 10*time.Second)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, filestore.ErrLockHeld) || time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// commitMutation appends the mutation to the lifecycle journal (fsynced,
// journal-first), then applies it to the in-memory state, materializes side
// files and persists the snapshot.
func (s *FileDesktopStore) commitMutation(mutation fileLifecycleMutation) error {
	record, err := s.journal.Append(mutation)
	if err != nil {
		return err
	}
	if err := s.applyMutation(s.state, mutation); err != nil {
		return err
	}
	s.state.AppliedSeq = record.Seq
	return s.persistState()
}

func (s *FileDesktopStore) persistState() error {
	encoded, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode lifecycle state: %w", err)
	}
	return filestore.WriteFileAtomic(s.statePath(), append(encoded, '\n'))
}

// applyMutation is deterministic: every field of the record is a complete
// post-transaction fact. Side-file applications (soft deletes, purges) are
// idempotent so journal replay after a crash converges.
func (s *FileDesktopStore) applyMutation(state *fileLifecycleState, mutation fileLifecycleMutation) error {
	for _, project := range mutation.Projects {
		// Normalizing on apply is what lets pre-multi-folder snapshots and
		// journal records replay unchanged: their WorkspaceRoot is promoted to a
		// single primary user folder here, once, for every code path.
		project = project.withNormalizedFolders()
		state.Projects[project.ProjectID] = project
	}
	for _, operation := range mutation.Operations {
		state.Operations[operation.OperationID] = operation
		if operation.IdempotencyKey != "" {
			state.OperationKeys[operation.IdempotencyKey] = operation.OperationID
		}
	}
	for operationID, threads := range mutation.OperationThreads {
		state.OperationThreads[operationID] = append([]string(nil), threads...)
	}
	for _, job := range mutation.CleanupJobs {
		state.CleanupJobs[job.JobID] = job
	}
	if len(mutation.SoftDeleteThreads) > 0 {
		for _, threadID := range mutation.SoftDeleteThreads {
			if err := s.softDeleteSession(threadID, mutation.SoftDeleteOpID, mutation.Now); err != nil {
				return err
			}
		}
	}
	for _, threadID := range mutation.PurgeSessions {
		if err := s.purgeSessionDir(threadID); err != nil {
			return err
		}
	}
	if mutation.LastWorkspace != nil {
		if *mutation.LastWorkspace == "" {
			delete(state.DesktopState, "last_workspace_path")
		} else {
			state.DesktopState["last_workspace_path"] = *mutation.LastWorkspace
		}
	}
	if mutation.ProjectOrder != nil {
		state.ProjectOrder = append([]string(nil), (*mutation.ProjectOrder)...)
	}
	return nil
}

func (s *FileDesktopStore) softDeleteSession(threadID, operationID string, now time.Time) error {
	meta, err := s.readSessionMetadata(threadID)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if meta.Deleted {
		return nil
	}
	meta.Deleted = true
	meta.DeletedAt = formatStoredTime(now)
	meta.DeleteOperationID = operationID
	meta.UpdatedAt = formatStoredTime(now)
	return s.writeSessionMetadata(meta)
}

func (s *FileDesktopStore) purgeSessionDir(threadID string) error {
	meta, err := s.readSessionMetadata(threadID)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !meta.Deleted {
		// Finalization only removes sessions already soft
		// deleted; a live thread is never purged by a stale job.
		return nil
	}
	return os.RemoveAll(s.sessionDir(threadID))
}

func (s *FileDesktopStore) readSessionMetadata(threadID string) (fileSessionMetadata, error) {
	var meta fileSessionMetadata
	err := filestore.ReadJSON(filepath.Join(s.sessionDir(threadID), "metadata.json"), &meta)
	return meta, err
}

func (s *FileDesktopStore) writeSessionMetadata(meta fileSessionMetadata) error {
	dir := s.sessionDir(meta.ThreadID)
	if err := filestore.EnsureDir(dir); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session metadata: %w", err)
	}
	return filestore.WriteFileAtomic(filepath.Join(dir, "metadata.json"), append(encoded, '\n'))
}

func (s *FileDesktopStore) threadRecord(threadID string) (storedThreadRecord, error) {
	meta, err := s.readSessionMetadata(threadID)
	if os.IsNotExist(err) {
		return storedThreadRecord{}, ErrThreadNotFound
	}
	if err != nil {
		return storedThreadRecord{}, err
	}
	if meta.Deleted {
		return storedThreadRecord{}, ErrThreadNotFound
	}
	return storedRecordFromSessionMetadata(meta), nil
}

func storedRecordFromSessionMetadata(meta fileSessionMetadata) storedThreadRecord {
	workspace := meta.Workspace
	if strings.TrimSpace(workspace.WorkspaceRoot) == "" {
		workspace.WorkspaceRoot = meta.WorkspaceRoot
	}
	if strings.TrimSpace(workspace.ScopeID) == "" {
		workspace.ScopeID = meta.ScopeID
	}
	source := ThreadTitleSource(strings.TrimSpace(meta.TitleSource))
	status := ThreadTitleStatus(strings.TrimSpace(meta.TitleStatus))
	if source == "" && status == "" && strings.TrimSpace(meta.Title) != "" {
		source = ThreadTitleSourceManual
		status = ThreadTitleStatusReady
	}
	// archived_at only exists while archived=true. Older metadata may carry
	// archived without archived_at; fall back to updated_at for display and
	// ordering instead of rewriting history (archive contract §4.1).
	var archivedAt time.Time
	if meta.Archived {
		archivedAt = parseStoredTime(meta.ArchivedAt)
		if archivedAt.IsZero() {
			archivedAt = parseStoredTime(meta.UpdatedAt)
		}
	}
	return storedThreadRecord{
		Snapshot: ThreadSnapshot{
			ThreadID:                 meta.ThreadID,
			ProjectID:                meta.ProjectID,
			Label:                    trimThreadLabel(meta.Title),
			TitleSource:              source,
			TitleStatus:              status,
			TitleUpdatedAt:           parseStoredTime(meta.TitleUpdatedAt),
			TitleSeedRunID:           strings.TrimSpace(meta.TitleSeedRunID),
			RunID:                    meta.LatestRunID,
			Workspace:                workspace,
			CreatedAt:                parseStoredTime(meta.CreatedAt),
			UpdatedAt:                parseStoredTime(meta.UpdatedAt),
			Archived:                 meta.Archived,
			ArchivedAt:               archivedAt,
			Source:                   strings.TrimSpace(meta.Source),
			SelectedModelProfileID:   strings.TrimSpace(meta.SelectedModelProfileID),
			ReasoningEffortByProfile: cloneStringMap(meta.ReasoningEffortByProfile),
		},
		ModelProfileKey: meta.ModelProfileKey,
		Preview:         meta.Preview,
	}
}

// upsertThread applies the frozen metadata merge semantics column by column,
// including the guard that a first_user/pending title write never
// downgrades an already-decided title.
func (s *FileDesktopStore) upsertThread(snapshot ThreadSnapshot, modelProfileKey, preview string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return err
	}
	defer lock.Release()

	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now().UTC()
	}
	if snapshot.UpdatedAt.IsZero() {
		snapshot.UpdatedAt = snapshot.CreatedAt
	}
	projectID := ""
	// A thread-owned session workspace is not a project folder. In particular,
	// ignore a stale top-level ProjectID that older path inference may have
	// written: an actual folder-less project always carries its identity inside
	// Workspace.ProjectID after normalization.
	if !snapshot.Workspace.SessionWorkspace || strings.TrimSpace(snapshot.Workspace.ProjectID) != "" {
		var err error
		projectID, err = s.ensureProjectLocked(snapshot.Workspace.WorkspaceRoot, false)
		if err != nil {
			return err
		}
	}

	incomingTitle := trimThreadLabel(firstNonEmpty(snapshot.Label, snapshot.ThreadID))
	incomingSource := strings.TrimSpace(string(snapshot.TitleSource))
	incomingStatus := strings.TrimSpace(string(snapshot.TitleStatus))
	incomingTitleUpdated := formatStoredTime(snapshot.TitleUpdatedAt)
	incomingSeed := strings.TrimSpace(snapshot.TitleSeedRunID)

	existing, err := s.readSessionMetadata(snapshot.ThreadID)
	if os.IsNotExist(err) {
		meta := fileSessionMetadata{
			SchemaVersion:            1,
			ThreadID:                 snapshot.ThreadID,
			Title:                    incomingTitle,
			TitleSource:              incomingSource,
			TitleStatus:              incomingStatus,
			TitleUpdatedAt:           incomingTitleUpdated,
			TitleSeedRunID:           incomingSeed,
			WorkspaceRoot:            snapshot.Workspace.WorkspaceRoot,
			ProjectID:                projectID,
			ScopeID:                  snapshot.Workspace.ScopeID,
			LatestRunID:              snapshot.RunID,
			CreatedAt:                formatStoredTime(snapshot.CreatedAt),
			UpdatedAt:                formatStoredTime(snapshot.UpdatedAt),
			Preview:                  strings.TrimSpace(preview),
			ModelProfileKey:          strings.TrimSpace(modelProfileKey),
			SelectedModelProfileID:   strings.TrimSpace(snapshot.SelectedModelProfileID),
			ReasoningEffortByProfile: cloneStringMap(snapshot.ReasoningEffortByProfile),
			Source:                   strings.TrimSpace(snapshot.Source),
			Workspace:                snapshot.Workspace,
		}
		return s.writeSessionMetadata(meta)
	}
	if err != nil {
		return err
	}

	downgrade := incomingSource == string(ThreadTitleSourceFirstUser) &&
		incomingStatus == string(ThreadTitleStatusPending) &&
		strings.TrimSpace(existing.TitleSource) != "" &&
		!(existing.TitleSource == string(ThreadTitleSourceFirstUser) && existing.TitleStatus == string(ThreadTitleStatusPending))
	keepTitleFields := incomingSource == "" || downgrade

	next := existing
	if !keepTitleFields {
		next.Title = incomingTitle
		next.TitleSource = incomingSource
		if incomingStatus != "" {
			next.TitleStatus = incomingStatus
		}
		if incomingTitleUpdated != "" {
			next.TitleUpdatedAt = incomingTitleUpdated
		}
		if incomingSeed != "" {
			next.TitleSeedRunID = incomingSeed
		}
	}
	next.WorkspaceRoot = snapshot.Workspace.WorkspaceRoot
	next.ProjectID = projectID
	next.ScopeID = snapshot.Workspace.ScopeID
	if strings.TrimSpace(snapshot.RunID) != "" {
		next.LatestRunID = snapshot.RunID
	}
	if strings.TrimSpace(existing.CreatedAt) == "" {
		next.CreatedAt = formatStoredTime(snapshot.CreatedAt)
	}
	next.UpdatedAt = formatStoredTime(snapshot.UpdatedAt)
	if strings.TrimSpace(preview) != "" {
		next.Preview = strings.TrimSpace(preview)
	}
	if strings.TrimSpace(modelProfileKey) != "" {
		next.ModelProfileKey = strings.TrimSpace(modelProfileKey)
	}
	next.SelectedModelProfileID = strings.TrimSpace(snapshot.SelectedModelProfileID)
	next.ReasoningEffortByProfile = cloneStringMap(snapshot.ReasoningEffortByProfile)
	if source := strings.TrimSpace(snapshot.Source); source != "" {
		next.Source = source
	}
	next.Workspace = snapshot.Workspace
	return s.writeSessionMetadata(next)
}

// folderHolderToReopen picks which of a folder's holders the single-folder
// upsert should reuse. An active holder is the one the user is working in; a
// removed one is only reactivated when nothing active holds the folder.
func folderHolderToReopen(holders []Project) (Project, bool) {
	if len(holders) == 0 {
		return Project{}, false
	}
	for _, holder := range holders {
		if holder.State == ProjectStateActive {
			return holder, true
		}
	}
	return holders[0], true
}

// ensureProjectLocked only journals a lifecycle mutation when project facts actually
// change; the Version counter therefore advances per material change, not per
// thread append.
func (s *FileDesktopStore) ensureProjectLocked(root string, reactivate bool) (string, error) {
	normalized := normalizeProjectWorkspaceRoot(root)
	if normalized == "" || !filepath.IsAbs(normalized) {
		return "", fmt.Errorf("project workspace root must be an absolute path")
	}
	now := time.Now().UTC()
	// Reopening a known folder must land in a project that already holds it
	// rather than mint a duplicate. Several projects may hold it, so an active
	// holder wins; reactivation is only for the case where none is active.
	if project, ok := folderHolderToReopen(s.projectsHoldingFolderLocked(normalized)); ok {
		projectID := project.ProjectID
		if !reactivate || (project.State == ProjectStateActive && project.RemovalReason == "" && project.RemovedAt == nil) {
			return projectID, nil
		}
		project.State = ProjectStateActive
		project.RemovalReason = ""
		project.RemovedAt = nil
		project.UpdatedAt = now
		project.Version++
		if err := s.commitMutation(fileLifecycleMutation{
			Kind:     "EnsureProject",
			Now:      now,
			Projects: []Project{project},
		}); err != nil {
			return "", err
		}
		return projectID, nil
	}
	project := Project{
		ProjectID:     stableProjectID(normalized),
		WorkspaceRoot: normalized,
		Folders:       []ProjectFolder{{Path: normalized, Role: ProjectFolderRoleUser, IsPrimary: true}},
		DisplayName:   projectDisplayName(normalized),
		State:         ProjectStateActive,
		CreatedAt:     now,
		UpdatedAt:     now,
		Version:       1,
	}
	projectOrder := append([]string{project.ProjectID}, s.orderedProjectIDsLocked()...)
	if err := s.commitMutation(fileLifecycleMutation{
		Kind:         "EnsureProject",
		Now:          now,
		Projects:     []Project{project},
		ProjectOrder: &projectOrder,
	}); err != nil {
		return "", err
	}
	return project.ProjectID, nil
}

// --- ThreadStore ---

func (s *FileDesktopStore) CreateThread(snapshot ThreadSnapshot, modelProfileKey, preview string) error {
	return s.upsertThread(snapshot, modelProfileKey, preview)
}

func (s *FileDesktopStore) AppendRunInput(snapshot ThreadSnapshot, run Run, sessionID string, input string, attachments []RunInputAttachment) error {
	path := s.ThreadTranscriptPath(run.ThreadID)
	schema, err := DetectJournalSchema(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if schema == 1 {
		if err := appendUserTranscriptRecordAtPath(path, run, sessionID, input, attachments); err != nil {
			return err
		}
	}
	next := snapshot
	next.RunID = run.RunID
	next.UpdatedAt = maxTime(next.UpdatedAt, run.UpdatedAt, run.CreatedAt)
	preview := strings.TrimSpace(input)
	if preview == "" {
		preview = snapshot.Label
	}
	return s.upsertThread(next, "", preview)
}

func (s *FileDesktopStore) AppendConversation(snapshot ThreadSnapshot, run Run, messages []convcontext.Message) error {
	path := s.ThreadTranscriptPath(run.ThreadID)
	schema, err := DetectJournalSchema(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if schema != agentcore.SemanticSchemaVersion {
		if err := appendConversationTranscriptAtPath(path, run, messages); err != nil {
			return err
		}
	}
	next := snapshot
	next.RunID = run.RunID
	next.UpdatedAt = maxTime(next.UpdatedAt, run.UpdatedAt, run.CreatedAt)
	return s.upsertThread(next, "", "")
}

func (s *FileDesktopStore) AppendRunEvents(snapshot ThreadSnapshot, run Run, events []Event) error {
	path := s.ThreadTranscriptPath(run.ThreadID)
	schema, err := DetectJournalSchema(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if schema != agentcore.SemanticSchemaVersion {
		if err := appendRunEventTranscriptAtPath(path, run, events); err != nil {
			return err
		}
	}
	next := snapshot
	next.RunID = run.RunID
	next.UpdatedAt = maxTime(next.UpdatedAt, run.UpdatedAt, run.CreatedAt)
	return s.upsertThread(next, "", "")
}

func (s *FileDesktopStore) listSessionMetadata() ([]fileSessionMetadata, error) {
	entries, err := os.ReadDir(filepath.Join(s.gen, "sessions"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var sessions []fileSessionMetadata
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		var meta fileSessionMetadata
		if err := filestore.ReadJSON(filepath.Join(s.gen, "sessions", entry.Name(), "metadata.json"), &meta); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		sessions = append(sessions, meta)
	}
	return sessions, nil
}

func (s *FileDesktopStore) ListThreads(view ThreadListView) ([]ThreadSnapshot, error) {
	if view != ThreadListViewActive && view != ThreadListViewArchived && view != ThreadListViewAll {
		return nil, fmt.Errorf("invalid thread list view %q", view)
	}
	sessions, err := s.listSessionMetadata()
	if err != nil {
		return nil, err
	}
	var records []storedThreadRecord
	for _, meta := range sessions {
		// Deletion always wins over archiving: deleted records never appear
		// in any view.
		if meta.Deleted {
			continue
		}
		switch view {
		case ThreadListViewActive:
			if meta.Archived {
				continue
			}
		case ThreadListViewArchived:
			if !meta.Archived {
				continue
			}
		}
		records = append(records, storedRecordFromSessionMetadata(meta))
	}
	if view == ThreadListViewArchived {
		// Frozen archived query contract: archivedAt DESC, updatedAt DESC,
		// createdAt DESC, threadId ASC.
		sort.Slice(records, func(i, j int) bool {
			a, b := records[i].Snapshot, records[j].Snapshot
			if !a.ArchivedAt.Equal(b.ArchivedAt) {
				return a.ArchivedAt.After(b.ArchivedAt)
			}
			if !a.UpdatedAt.Equal(b.UpdatedAt) {
				return a.UpdatedAt.After(b.UpdatedAt)
			}
			if !a.CreatedAt.Equal(b.CreatedAt) {
				return a.CreatedAt.After(b.CreatedAt)
			}
			return a.ThreadID < b.ThreadID
		})
	} else {
		// Frozen query contract: updatedAt DESC, createdAt DESC, threadId ASC.
		sort.Slice(records, func(i, j int) bool {
			a, b := records[i].Snapshot, records[j].Snapshot
			if !a.UpdatedAt.Equal(b.UpdatedAt) {
				return a.UpdatedAt.After(b.UpdatedAt)
			}
			if !a.CreatedAt.Equal(b.CreatedAt) {
				return a.CreatedAt.After(b.CreatedAt)
			}
			return a.ThreadID < b.ThreadID
		})
	}
	out := make([]ThreadSnapshot, 0, len(records))
	for _, record := range records {
		out = append(out, record.Snapshot)
	}
	return out, nil
}

// SetThreadArchived flips the durable archived flag in one locked atomic
// metadata write. Archiving never touches updated_at, so a management action
// cannot push a conversation to the top of the regular list; the same target
// state is idempotent and never rewrites archived_at; unarchiving clears
// archived_at together with the flag (archive contract §4.1/§4.3).
func (s *FileDesktopStore) SetThreadArchived(threadID string, archived bool, now time.Time) (ThreadSnapshot, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return ThreadSnapshot{}, ErrThreadNotFound
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return ThreadSnapshot{}, err
	}
	defer lock.Release()

	meta, err := s.readSessionMetadata(threadID)
	if os.IsNotExist(err) {
		return ThreadSnapshot{}, ErrThreadNotFound
	}
	if err != nil {
		return ThreadSnapshot{}, err
	}
	if meta.Deleted {
		return ThreadSnapshot{}, ErrThreadNotFound
	}
	if meta.Archived == archived {
		return storedRecordFromSessionMetadata(meta).Snapshot, nil
	}
	meta.Archived = archived
	if archived {
		meta.ArchivedAt = formatStoredTime(now)
	} else {
		meta.ArchivedAt = ""
	}
	if err := s.writeSessionMetadata(meta); err != nil {
		return ThreadSnapshot{}, err
	}
	return storedRecordFromSessionMetadata(meta).Snapshot, nil
}

func (s *FileDesktopStore) RestoreThread(threadID string) (storedThreadRecord, restoredTranscript, error) {
	record, err := s.threadRecord(threadID)
	if err != nil {
		return storedThreadRecord{}, restoredTranscript{}, err
	}
	return restoreThreadFromDurableRecord(record, s.ThreadTranscriptPath(threadID), threadID)
}

func (s *FileDesktopStore) GetTranscript(threadID string) (ThreadTranscript, error) {
	record, err := s.threadRecord(threadID)
	if err != nil {
		return ThreadTranscript{}, err
	}
	return threadTranscriptFromDurableRecord(record, s.ThreadTranscriptPath(threadID), threadID)
}

func (s *FileDesktopStore) RenameThread(threadID, label string, updatedAt time.Time) (ThreadSnapshot, error) {
	record, err := s.threadRecord(threadID)
	if err != nil {
		return ThreadSnapshot{}, err
	}
	record.Snapshot.Label = trimThreadLabel(strings.TrimSpace(label))
	record.Snapshot.TitleSource = ThreadTitleSourceManual
	record.Snapshot.TitleStatus = ThreadTitleStatusReady
	record.Snapshot.TitleUpdatedAt = updatedAt.UTC()
	record.Snapshot.UpdatedAt = updatedAt.UTC()
	if record.Snapshot.Label == "" {
		return ThreadSnapshot{}, ErrThreadLabelInvalid
	}
	path := s.ThreadTranscriptPath(threadID)
	schema, err := DetectJournalSchema(path)
	if err != nil && !os.IsNotExist(err) {
		return ThreadSnapshot{}, err
	}
	if schema == 1 {
		if err := appendThreadLabelTranscriptRecordAtPath(path, threadID, record.Snapshot.Label, updatedAt); err != nil {
			return ThreadSnapshot{}, err
		}
	}
	if err := s.upsertThread(record.Snapshot, record.ModelProfileKey, ""); err != nil {
		return ThreadSnapshot{}, err
	}
	return record.Snapshot, nil
}

func (s *FileDesktopStore) UpdateThreadTitleMetadata(threadID, label string, source ThreadTitleSource, status ThreadTitleStatus, seedRunID string, updatedAt time.Time) (ThreadSnapshot, error) {
	threadID = strings.TrimSpace(threadID)
	seedRunID = strings.TrimSpace(seedRunID)
	if threadID == "" {
		return ThreadSnapshot{}, ErrThreadNotFound
	}
	nextLabel := trimThreadLabel(strings.TrimSpace(label))
	if status == ThreadTitleStatusReady && nextLabel == "" {
		return ThreadSnapshot{}, ErrThreadLabelInvalid
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	} else {
		updatedAt = updatedAt.UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return ThreadSnapshot{}, err
	}
	defer lock.Release()

	meta, err := s.readSessionMetadata(threadID)
	if os.IsNotExist(err) {
		return ThreadSnapshot{}, ErrThreadNotFound
	}
	if err != nil {
		return ThreadSnapshot{}, err
	}
	if meta.Deleted {
		return ThreadSnapshot{}, ErrThreadNotFound
	}
	// Task-owned titles follow scheduled-task renames until the user renames
	// the thread manually (which flips the source to manual and ends the CAS).
	if meta.TitleSource == string(ThreadTitleSourceTask) {
		if nextLabel == "" {
			return storedRecordFromSessionMetadata(meta).Snapshot, nil
		}
		meta.Title = nextLabel
		meta.TitleSource = string(source)
		meta.TitleStatus = string(status)
		meta.TitleUpdatedAt = formatStoredTime(updatedAt)
		meta.UpdatedAt = formatStoredTime(updatedAt)
		if err := s.writeSessionMetadata(meta); err != nil {
			return ThreadSnapshot{}, err
		}
		return storedRecordFromSessionMetadata(meta).Snapshot, nil
	}
	if seedRunID == "" {
		// The first_user CAS below keys on the seed run; without one there is
		// nothing safe to transition.
		return storedRecordFromSessionMetadata(meta).Snapshot, nil
	}
	// CAS: only a pending first_user seed for the same run may transition.
	if meta.TitleSource != string(ThreadTitleSourceFirstUser) ||
		meta.TitleStatus != string(ThreadTitleStatusPending) ||
		meta.TitleSeedRunID != seedRunID {
		return storedRecordFromSessionMetadata(meta).Snapshot, nil
	}
	if nextLabel != "" {
		meta.Title = nextLabel
	}
	meta.TitleSource = string(source)
	meta.TitleStatus = string(status)
	meta.TitleUpdatedAt = formatStoredTime(updatedAt)
	meta.TitleSeedRunID = seedRunID
	meta.UpdatedAt = formatStoredTime(updatedAt)
	if err := s.writeSessionMetadata(meta); err != nil {
		return ThreadSnapshot{}, err
	}
	return storedRecordFromSessionMetadata(meta).Snapshot, nil
}

func (s *FileDesktopStore) DeleteThread(threadID string) error {
	if _, err := s.threadRecord(threadID); err != nil {
		return err
	}
	if err := deleteThreadTranscriptAtPath(s.ThreadTranscriptPath(threadID)); err != nil && !errors.Is(err, ErrThreadNotFound) {
		return err
	}
	if err := deleteThreadInputAttachments(threadID); err != nil && !errors.Is(err, ErrThreadNotFound) {
		return err
	}
	if err := deleteThreadToolResults(threadID); err != nil && !errors.Is(err, ErrThreadNotFound) {
		return err
	}
	return os.RemoveAll(s.sessionDir(threadID))
}

// --- ProjectRepository queries ---

// ProjectsHoldingFolder lists every project this folder is attached to. A
// directory may be shared by any number of projects, so callers must handle a
// list; those needing a single project take the first, which is the topmost one
// in the order the user sees.
func (s *FileDesktopStore) ProjectsHoldingFolder(root string) ([]Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.projectsHoldingFolderLocked(root), nil
}

// projectsHoldingFolderLocked derives folder ownership from the folder lists
// themselves. There is deliberately no reverse index to keep in sync: the lists
// are the only fact, and a project count small enough to scan makes a cache a
// drift risk rather than a win.
func (s *FileDesktopStore) projectsHoldingFolderLocked(root string) []Project {
	normalized := normalizeProjectWorkspaceRoot(root)
	if normalized == "" {
		return nil
	}
	holders := make([]Project, 0, 1)
	for _, project := range s.listProjectsLocked(true) {
		if project.HasFolder(normalized) {
			holders = append(holders, project)
		}
	}
	return holders
}

func (s *FileDesktopStore) ListProjects(includeRemoved bool) ([]Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listProjectsLocked(includeRemoved), nil
}

func (s *FileDesktopStore) listProjectsLocked(includeRemoved bool) []Project {
	projects := make([]Project, 0, len(s.state.Projects))
	for _, project := range s.state.Projects {
		if !includeRemoved && project.State == ProjectStateRemoved {
			continue
		}
		projects = append(projects, project)
	}
	orderRank := make(map[string]int, len(s.state.ProjectOrder))
	for index, projectID := range s.state.ProjectOrder {
		if _, exists := orderRank[projectID]; !exists {
			orderRank[projectID] = index
		}
	}
	sort.SliceStable(projects, func(i, j int) bool {
		iRank, iOrdered := orderRank[projects[i].ProjectID]
		jRank, jOrdered := orderRank[projects[j].ProjectID]
		if iOrdered != jOrdered {
			return iOrdered
		}
		if iOrdered && iRank != jRank {
			return iRank < jRank
		}
		if !projects[i].UpdatedAt.Equal(projects[j].UpdatedAt) {
			return projects[i].UpdatedAt.After(projects[j].UpdatedAt)
		}
		if !projects[i].CreatedAt.Equal(projects[j].CreatedAt) {
			return projects[i].CreatedAt.After(projects[j].CreatedAt)
		}
		return projects[i].ProjectID < projects[j].ProjectID
	})
	return projects
}

func (s *FileDesktopStore) orderedProjectIDsLocked() []string {
	projects := s.listProjectsLocked(true)
	ids := make([]string, 0, len(projects))
	for _, project := range projects {
		ids = append(ids, project.ProjectID)
	}
	return ids
}

func (s *FileDesktopStore) ProjectByID(projectID string) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	project, ok := s.state.Projects[strings.TrimSpace(projectID)]
	if !ok {
		return Project{}, ErrProjectNotFound
	}
	return project, nil
}

func (s *FileDesktopStore) ThreadDeletionState(threadID string) (string, bool, bool, error) {
	meta, err := s.readSessionMetadata(threadID)
	if os.IsNotExist(err) {
		return "", false, false, nil
	}
	if err != nil {
		return "", false, false, err
	}
	return meta.ProjectID, meta.Deleted, true, nil
}

func (s *FileDesktopStore) LookupIdempotentOperation(key, projectID string, action ProjectOperationAction) (ProjectOperation, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	operationID, ok := s.state.OperationKeys[strings.TrimSpace(key)]
	if !ok {
		return ProjectOperation{}, false, nil
	}
	operation := s.state.Operations[operationID]
	if operation.ProjectID != projectID || operation.Action != action {
		return ProjectOperation{}, false, ErrIdempotencyKeyConflict
	}
	operation.PendingCleanup = s.pendingCleanupLocked(operation.OperationID)
	return operation, true, nil
}

func (s *FileDesktopStore) OperationByID(operationID string) (ProjectOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	operation, ok := s.state.Operations[strings.TrimSpace(operationID)]
	if !ok {
		return ProjectOperation{}, ErrProjectOperationNotFound
	}
	return operation, nil
}

func (s *FileDesktopStore) pendingCleanupLocked(operationID string) int {
	count := 0
	for _, job := range s.state.CleanupJobs {
		if job.OperationID == operationID && job.Kind == cleanupJobThreadPurge && job.Status != cleanupJobSucceeded {
			count++
		}
	}
	return count
}

func (s *FileDesktopStore) PendingCleanupCount(operationID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pendingCleanupLocked(operationID), nil
}

func (s *FileDesktopStore) ListPendingTrashOperations() ([]ProjectOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	operations := make([]ProjectOperation, 0, len(s.state.Operations))
	for _, operation := range s.state.Operations {
		if operation.Action != ProjectOperationTrashAndPurge {
			continue
		}
		if operation.Status != ProjectOperationAccepted && operation.Status != ProjectOperationRunning {
			continue
		}
		switch operation.Phase {
		case projectOperationPhaseWaitingForRuns, projectOperationPhaseTrashClaimed, projectOperationPhaseTrashStaged, projectOperationPhaseTrashing:
			operations = append(operations, operation)
		}
	}
	sort.Slice(operations, func(i, j int) bool {
		if !operations[i].CreatedAt.Equal(operations[j].CreatedAt) {
			return operations[i].CreatedAt.Before(operations[j].CreatedAt)
		}
		return operations[i].OperationID < operations[j].OperationID
	})
	return operations, nil
}

func (s *FileDesktopStore) OperationThreadIDs(operationID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	threads := append([]string(nil), s.state.OperationThreads[operationID]...)
	sort.Strings(threads)
	return threads, nil
}

func (s *FileDesktopStore) liveThreadIDsForProjectLocked(projectID string) ([]string, error) {
	sessions, err := s.listSessionMetadata()
	if err != nil {
		return nil, err
	}
	type liveThread struct {
		id      string
		created time.Time
	}
	var threads []liveThread
	for _, meta := range sessions {
		if meta.Deleted || meta.ProjectID != projectID {
			continue
		}
		threads = append(threads, liveThread{id: meta.ThreadID, created: parseStoredTime(meta.CreatedAt)})
	}
	sort.Slice(threads, func(i, j int) bool {
		if !threads[i].created.Equal(threads[j].created) {
			return threads[i].created.Before(threads[j].created)
		}
		return threads[i].id < threads[j].id
	})
	ids := make([]string, 0, len(threads))
	for _, thread := range threads {
		ids = append(ids, thread.id)
	}
	return ids, nil
}

// --- ProjectRepository commands ---

func (s *FileDesktopStore) EnsureProjectActive(root, displayName string, now time.Time) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return Project{}, err
	}
	defer lock.Release()

	normalized := normalizeProjectWorkspaceRoot(root)
	projectID, err := s.ensureProjectLocked(normalized, true)
	if err != nil {
		return Project{}, fmt.Errorf("%w: %v", ErrProjectPathInvalid, err)
	}
	project := s.state.Projects[projectID]
	changed := false
	if trimmed := strings.TrimSpace(displayName); trimmed != "" && project.DisplayName != trimmed {
		project.DisplayName = trimmed
		project.UpdatedAt = now
		project.Version++
		changed = true
	}
	last := normalized
	mutation := fileLifecycleMutation{Kind: "EnsureProjectActive", Now: now, LastWorkspace: &last}
	if changed {
		mutation.Projects = []Project{project}
	}
	if err := s.commitMutation(mutation); err != nil {
		return Project{}, err
	}
	return s.state.Projects[projectID], nil
}

// CreateProject mints a project with an explicit folder list. The list may be
// empty: a project is a named piece of work, and binding it to a folder is a
// choice the user makes when they have one to bind. A folder-less project gets
// its working directory from the run service instead.
func (s *FileDesktopStore) CreateProject(folders []ProjectFolder, displayName string, now time.Time) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return Project{}, err
	}
	defer lock.Release()

	project := Project{
		ProjectID:   newProjectID(),
		Folders:     append([]ProjectFolder(nil), folders...),
		DisplayName: strings.TrimSpace(displayName),
		State:       ProjectStateActive,
		CreatedAt:   now,
		UpdatedAt:   now,
		Version:     1,
	}.withNormalizedFolders()
	projectID := project.ProjectID
	if project.DisplayName == "" {
		if primary, ok := project.PrimaryFolder(); ok {
			project.DisplayName = projectDisplayName(primary.Path)
		} else {
			project.DisplayName = "Untitled project"
		}
	}
	projectOrder := append([]string{projectID}, s.orderedProjectIDsLocked()...)
	last := project.PrimaryRoot()
	if err := s.commitMutation(fileLifecycleMutation{
		Kind:          "CreateProject",
		Now:           now,
		Projects:      []Project{project},
		ProjectOrder:  &projectOrder,
		LastWorkspace: &last,
	}); err != nil {
		return Project{}, err
	}
	return s.state.Projects[projectID], nil
}

// AttachProjectFolder adds a readable/writable folder to an existing project.
// The folder may already belong to other projects: a directory is shared work,
// not property. Attaching the first folder of a folder-less project promotes it
// to primary, because that is where the user now expects work to happen.
func (s *FileDesktopStore) AttachProjectFolder(projectID, path string, now time.Time) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return Project{}, err
	}
	defer lock.Release()

	project, ok := s.state.Projects[strings.TrimSpace(projectID)]
	if !ok {
		return Project{}, ErrProjectNotFound
	}
	if project.State == ProjectStateTrashPending {
		return Project{}, ErrProjectOperationConflict
	}
	normalized := normalizeProjectWorkspaceRoot(path)
	if normalized == "" || !filepath.IsAbs(normalized) {
		return Project{}, ErrProjectPathInvalid
	}
	if project.HasFolder(normalized) {
		return project, nil
	}

	// The first folder of a folder-less project becomes its primary; anything
	// attached later joins as a secondary folder.
	promote := len(project.Folders) == 0
	if promote {
		for index := range project.Folders {
			project.Folders[index].IsPrimary = false
		}
	}
	project.Folders = append(project.Folders, ProjectFolder{
		Path:      normalized,
		Role:      ProjectFolderRoleUser,
		IsPrimary: promote,
	})
	project.State = ProjectStateActive
	project.RemovalReason = ""
	project.RemovedAt = nil
	project.UpdatedAt = now
	project.Version++
	project = project.withNormalizedFolders()
	if err := s.commitMutation(fileLifecycleMutation{
		Kind:     "AttachProjectFolder",
		Now:      now,
		Projects: []Project{project},
	}); err != nil {
		return Project{}, err
	}
	return s.state.Projects[project.ProjectID], nil
}

// DetachProjectFolder unbinds a user folder without touching the directory on
// disk. Detaching the last user folder falls back to the Awaken-managed working
// directory so the project never ends up rootless.
func (s *FileDesktopStore) DetachProjectFolder(projectID, path string, now time.Time) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return Project{}, err
	}
	defer lock.Release()

	project, ok := s.state.Projects[strings.TrimSpace(projectID)]
	if !ok {
		return Project{}, ErrProjectNotFound
	}
	if project.State == ProjectStateTrashPending {
		return Project{}, ErrProjectOperationConflict
	}
	next, err := s.projectWithoutFolderLocked(project, path, now)
	if err != nil {
		return Project{}, err
	}
	if err := s.commitMutation(fileLifecycleMutation{
		Kind:     "DetachProjectFolder",
		Now:      now,
		Projects: []Project{next},
	}); err != nil {
		return Project{}, err
	}
	return s.state.Projects[next.ProjectID], nil
}

// projectWithoutFolderLocked computes the post-detach project without
// committing, so folder detach and folder trash completion share one rule set.
func (s *FileDesktopStore) projectWithoutFolderLocked(project Project, path string, now time.Time) (Project, error) {
	index := project.folderIndex(path)
	if index < 0 {
		return Project{}, ErrProjectFolderNotFound
	}
	remaining := make([]ProjectFolder, 0, len(project.Folders))
	remaining = append(remaining, project.Folders[:index]...)
	remaining = append(remaining, project.Folders[index+1:]...)
	// Detaching the last folder leaves the project itself intact. It keeps its
	// name, threads and history, and falls back to the run service's working
	// directory the same way a project created without a folder does.
	project.Folders = remaining
	if len(remaining) == 0 {
		// withNormalizedFolders promotes the mirrored WorkspaceRoot when a
		// project carries no folders, which is how a row written before
		// multi-folder projects gets upgraded on load. An intentional detach of
		// the last folder must not be mistaken for that case, so the mirror is
		// dropped here rather than being left to resurrect the folder.
		project.WorkspaceRoot = ""
	}
	project.UpdatedAt = now
	project.Version++
	return project.withNormalizedFolders(), nil
}

// SetPrimaryProjectFolder moves the discovery and working-directory anchor onto
// another attached folder.
func (s *FileDesktopStore) SetPrimaryProjectFolder(projectID, path string, now time.Time) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return Project{}, err
	}
	defer lock.Release()

	project, ok := s.state.Projects[strings.TrimSpace(projectID)]
	if !ok {
		return Project{}, ErrProjectNotFound
	}
	if project.State == ProjectStateTrashPending {
		return Project{}, ErrProjectOperationConflict
	}
	index := project.folderIndex(path)
	if index < 0 {
		return Project{}, ErrProjectFolderNotFound
	}
	if project.Folders[index].IsPrimary {
		return project, nil
	}
	folders := append([]ProjectFolder(nil), project.Folders...)
	for i := range folders {
		folders[i].IsPrimary = i == index
	}
	project.Folders = folders
	project.UpdatedAt = now
	project.Version++
	project = project.withNormalizedFolders()
	last := project.PrimaryRoot()
	if err := s.commitMutation(fileLifecycleMutation{
		Kind:          "SetPrimaryProjectFolder",
		Now:           now,
		Projects:      []Project{project},
		LastWorkspace: &last,
	}); err != nil {
		return Project{}, err
	}
	return s.state.Projects[project.ProjectID], nil
}

func (s *FileDesktopStore) ReorderProjects(projectIDs []string, now time.Time) ([]Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return nil, err
	}
	defer lock.Release()

	active := make(map[string]struct{})
	for projectID, project := range s.state.Projects {
		if project.State == ProjectStateActive {
			active[projectID] = struct{}{}
		}
	}
	requested := make([]string, 0, len(projectIDs))
	seen := make(map[string]struct{}, len(projectIDs))
	for _, rawProjectID := range projectIDs {
		projectID := strings.TrimSpace(rawProjectID)
		if projectID == "" {
			return nil, ErrProjectOrderInvalid
		}
		if _, duplicate := seen[projectID]; duplicate {
			return nil, ErrProjectOrderInvalid
		}
		if _, exists := active[projectID]; !exists {
			return nil, ErrProjectOrderInvalid
		}
		seen[projectID] = struct{}{}
		requested = append(requested, projectID)
	}
	if len(seen) != len(active) {
		return nil, ErrProjectOrderInvalid
	}

	projectOrder := append([]string(nil), requested...)
	for _, projectID := range s.orderedProjectIDsLocked() {
		if _, isActive := active[projectID]; !isActive {
			projectOrder = append(projectOrder, projectID)
		}
	}
	if err := s.commitMutation(fileLifecycleMutation{
		Kind:         "ReorderProjects",
		Now:          now,
		ProjectOrder: &projectOrder,
	}); err != nil {
		return nil, err
	}
	return s.listProjectsLocked(true), nil
}

func (s *FileDesktopStore) CommitDetach(projectID, idempotencyKey string, now time.Time) (ProjectOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return ProjectOperation{}, err
	}
	defer lock.Release()

	project, ok := s.state.Projects[strings.TrimSpace(projectID)]
	if !ok {
		return ProjectOperation{}, ErrProjectNotFound
	}
	if project.State == ProjectStateTrashPending {
		return ProjectOperation{}, ErrProjectOperationConflict
	}
	operation := newProjectOperation(project, idempotencyKey, ProjectOperationDetach, ProjectOperationSucceeded, projectOperationPhaseCompleted, now)
	operation.LogicalCommittedAt = &now
	operation.PhysicalCompletedAt = &now
	removedAt := now
	project.State = ProjectStateRemoved
	project.RemovalReason = "detached"
	project.RemovedAt = &removedAt
	project.UpdatedAt = now
	project.Version++
	mutation := fileLifecycleMutation{
		Kind:       "CommitDetach",
		Now:        now,
		Projects:   []Project{project},
		Operations: []ProjectOperation{operation},
	}
	mutation.LastWorkspace = s.fallbackWorkspaceLocked(project.ProjectID)
	if err := s.commitMutation(mutation); err != nil {
		return ProjectOperation{}, err
	}
	return operation, nil
}

// fallbackWorkspaceLocked mirrors selectFallbackWorkspaceTx: the most recently
// updated active project other than the excluded one becomes the durable last
// workspace; with none left the preference is cleared.
func (s *FileDesktopStore) fallbackWorkspaceLocked(excludedProjectID string) *string {
	var fallback *Project
	for id := range s.state.Projects {
		project := s.state.Projects[id]
		if project.ProjectID == excludedProjectID || project.State != ProjectStateActive {
			continue
		}
		if fallback == nil ||
			project.UpdatedAt.After(fallback.UpdatedAt) ||
			(project.UpdatedAt.Equal(fallback.UpdatedAt) && project.CreatedAt.After(fallback.CreatedAt)) {
			copied := project
			fallback = &copied
		}
	}
	if fallback == nil {
		empty := ""
		return &empty
	}
	root := fallback.PrimaryRoot()
	return &root
}

func (s *FileDesktopStore) CommitDeleteHistory(project Project, idempotencyKey string, now time.Time) (ProjectOperation, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return ProjectOperation{}, nil, err
	}
	defer lock.Release()

	threadIDs, err := s.liveThreadIDsForProjectLocked(project.ProjectID)
	if err != nil {
		return ProjectOperation{}, nil, err
	}
	status := ProjectOperationAccepted
	phase := projectOperationPhaseCleaning
	if len(threadIDs) == 0 {
		status = ProjectOperationSucceeded
		phase = projectOperationPhaseCompleted
	}
	operation := newProjectOperation(project, idempotencyKey, ProjectOperationDeleteHistory, status, phase, now)
	operation.LogicalCommittedAt = &now
	if len(threadIDs) == 0 {
		operation.PhysicalCompletedAt = &now
	}
	operation.MatchedThreads = len(threadIDs)
	mutation := fileLifecycleMutation{
		Kind:              "CommitDeleteHistory",
		Now:               now,
		Operations:        []ProjectOperation{operation},
		SoftDeleteThreads: threadIDs,
		SoftDeleteOpID:    operation.OperationID,
		CleanupJobs:       newThreadPurgeJobs(operation.OperationID, threadIDs, now),
	}
	if err := s.commitMutation(mutation); err != nil {
		return ProjectOperation{}, nil, err
	}
	operation.PendingCleanup = len(threadIDs)
	return operation, threadIDs, nil
}

func newThreadPurgeJobs(operationID string, threadIDs []string, now time.Time) []fileCleanupJob {
	jobs := make([]fileCleanupJob, 0, len(threadIDs))
	for _, threadID := range threadIDs {
		jobs = append(jobs, fileCleanupJob{
			JobID:         newRuntimeID("job"),
			OperationID:   operationID,
			Kind:          cleanupJobThreadPurge,
			TargetID:      threadID,
			Status:        cleanupJobPending,
			NextAttemptAt: now,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	}
	return jobs
}

func (s *FileDesktopStore) CommitDeleteThread(project Project, threadID, idempotencyKey string, now time.Time) (ProjectOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return ProjectOperation{}, err
	}
	defer lock.Release()

	operation := newProjectOperation(project, idempotencyKey, ProjectOperationDeleteThread, ProjectOperationAccepted, projectOperationPhaseCleaning, now)
	operation.LogicalCommittedAt = &now
	operation.MatchedThreads = 1
	mutation := fileLifecycleMutation{
		Kind:              "CommitDeleteThread",
		Now:               now,
		Operations:        []ProjectOperation{operation},
		SoftDeleteThreads: []string{threadID},
		SoftDeleteOpID:    operation.OperationID,
		CleanupJobs:       newThreadPurgeJobs(operation.OperationID, []string{threadID}, now),
	}
	if err := s.commitMutation(mutation); err != nil {
		return ProjectOperation{}, err
	}
	return operation, nil
}

// CommitPrepareTrash starts a folder-scoped trash operation. The project enters
// trash_pending so no new run can start while the directory is being staged; the
// thread list is collected only to gate on active runs, because folder trash no
// longer deletes Awaken history.
func (s *FileDesktopStore) CommitPrepareTrash(project Project, folderPath, idempotencyKey, fingerprint string, now time.Time) (ProjectOperation, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return ProjectOperation{}, nil, err
	}
	defer lock.Release()

	threadIDs, err := s.liveThreadIDsForProjectLocked(project.ProjectID)
	if err != nil {
		return ProjectOperation{}, nil, err
	}
	operation := newProjectFolderOperation(project, folderPath, idempotencyKey, ProjectOperationTrashAndPurge, ProjectOperationAccepted, projectOperationPhaseWaitingForRuns, now)
	operation.PathFingerprint = fingerprint
	operation.MatchedThreads = len(threadIDs)
	current := s.state.Projects[project.ProjectID]
	current.State = ProjectStateTrashPending
	current.UpdatedAt = now
	current.Version++
	trashJob := fileCleanupJob{
		JobID:         newRuntimeID("job"),
		OperationID:   operation.OperationID,
		Kind:          cleanupJobProjectTrash,
		TargetID:      project.ProjectID,
		Status:        cleanupJobPending,
		NextAttemptAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	mutation := fileLifecycleMutation{
		Kind:             "CommitPrepareTrash",
		Now:              now,
		Projects:         []Project{current},
		Operations:       []ProjectOperation{operation},
		OperationThreads: map[string][]string{operation.OperationID: threadIDs},
		CleanupJobs:      []fileCleanupJob{trashJob},
	}
	if err := s.commitMutation(mutation); err != nil {
		return ProjectOperation{}, nil, err
	}
	return operation, threadIDs, nil
}

func (s *FileDesktopStore) trashJobForOperationLocked(operationID string) (fileCleanupJob, bool) {
	for _, job := range s.state.CleanupJobs {
		if job.OperationID == operationID && job.Kind == cleanupJobProjectTrash {
			return job, true
		}
	}
	return fileCleanupJob{}, false
}

func (s *FileDesktopStore) ClaimTrash(operation ProjectOperation, workerID string, lease time.Duration, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return err
	}
	defer lock.Release()

	job, ok := s.trashJobForOperationLocked(operation.OperationID)
	if !ok {
		return ErrProjectOperationConflict
	}
	claimable := job.Status == cleanupJobPending || job.Status == cleanupJobRetry ||
		(job.Status == cleanupJobLeased && !job.LeaseExpiresAt.After(now))
	if !claimable {
		return ErrProjectOperationConflict
	}
	job.Status = cleanupJobLeased
	job.LeaseOwner = workerID
	job.LeaseExpiresAt = now.Add(lease)
	job.Attempts++
	job.UpdatedAt = now

	current := s.state.Operations[operation.OperationID]
	nextPhase := projectOperationPhaseTrashClaimed
	if operation.Phase == projectOperationPhaseTrashStaged {
		nextPhase = projectOperationPhaseTrashStaged
	} else if operation.Phase == projectOperationPhaseTrashing {
		// A previous worker durably crossed the invocation boundary; preserve
		// that fact for the replacement worker.
		nextPhase = projectOperationPhaseTrashing
	}
	current.Status = ProjectOperationRunning
	current.Phase = nextPhase
	current.AttemptID = newRuntimeID("trash_attempt")
	current.UpdatedAt = now
	return s.commitMutation(fileLifecycleMutation{
		Kind:        "ClaimTrash",
		Now:         now,
		Operations:  []ProjectOperation{current},
		CleanupJobs: []fileCleanupJob{job},
	})
}

func (s *FileDesktopStore) mutateOperationLocked(kind, operationID, attemptID string, now time.Time, mutate func(op *ProjectOperation)) error {
	current, ok := s.state.Operations[strings.TrimSpace(operationID)]
	if !ok {
		return ErrProjectOperationNotFound
	}
	if strings.TrimSpace(attemptID) != "" && current.AttemptID != strings.TrimSpace(attemptID) {
		// A stale attempt writes
		// nothing and the caller's subsequent read observes no change.
		return nil
	}
	mutate(&current)
	current.UpdatedAt = now
	return s.commitMutation(fileLifecycleMutation{
		Kind:       kind,
		Now:        now,
		Operations: []ProjectOperation{current},
	})
}

func (s *FileDesktopStore) PersistTrashStagingPath(operationID, attemptID, stagedPath string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return err
	}
	defer lock.Release()
	return s.mutateOperationLocked("PersistTrashStagingPath", operationID, attemptID, now, func(op *ProjectOperation) {
		op.TrashStagingPath = stagedPath
	})
}

func (s *FileDesktopStore) MarkTrashStaged(operationID, attemptID, phase, stagedPath string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return err
	}
	defer lock.Release()
	return s.mutateOperationLocked("MarkTrashStaged", operationID, attemptID, now, func(op *ProjectOperation) {
		op.Status = ProjectOperationRunning
		op.Phase = phase
		op.TrashStagingPath = stagedPath
	})
}

func (s *FileDesktopStore) CommitBeginTrash(operationID, attemptID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return err
	}
	defer lock.Release()
	return s.mutateOperationLocked("CommitBeginTrash", operationID, attemptID, now, func(op *ProjectOperation) {
		op.Status = ProjectOperationRunning
		op.Phase = projectOperationPhaseTrashing
		invoked := now
		op.TrashInvokedAt = &invoked
	})
}

// CommitCompleteTrash finalizes a folder-scoped trash: the directory is already
// gone from disk, so the folder is detached from the project and the project
// returns to the state it had before the operation. The project itself and its
// threads survive; removing the project or its history are separate actions.
//
// A shared folder is detached from every project holding it. The directory no
// longer exists, and leaving a row pointing at it in another project would be a
// path that resolves to nothing.
func (s *FileDesktopStore) CommitCompleteTrash(operation ProjectOperation, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return err
	}
	defer lock.Release()

	project, ok := s.state.Projects[operation.ProjectID]
	if !ok {
		return ErrProjectNotFound
	}
	restoredState := operation.PreviousProjectState
	if restoredState == "" {
		restoredState = ProjectStateActive
	}
	project.State = restoredState
	next, err := s.projectWithoutFolderLocked(project, operation.WorkspaceRoot, now)
	if err != nil {
		if !errors.Is(err, ErrProjectFolderNotFound) {
			return err
		}
		// A replayed completion finds the folder already detached; converge on
		// the current project instead of failing the idempotent retry.
		next = project
		next.UpdatedAt = now
		next.Version++
		next = next.withNormalizedFolders()
	}

	projects := []Project{next}
	for _, holder := range s.projectsHoldingFolderLocked(operation.WorkspaceRoot) {
		if holder.ProjectID == next.ProjectID {
			continue
		}
		detached, detachErr := s.projectWithoutFolderLocked(holder, operation.WorkspaceRoot, now)
		if detachErr != nil {
			if errors.Is(detachErr, ErrProjectFolderNotFound) {
				continue
			}
			return detachErr
		}
		projects = append(projects, detached)
	}

	current := s.state.Operations[operation.OperationID]
	current.Status = ProjectOperationSucceeded
	current.Phase = projectOperationPhaseCompleted
	current.LogicalCommittedAt = &now
	current.PhysicalCompletedAt = &now
	current.MatchedThreads = 0
	current.UpdatedAt = now

	jobs := make([]fileCleanupJob, 0, 1)
	if trashJob, ok := s.trashJobForOperationLocked(operation.OperationID); ok {
		trashJob.Status = cleanupJobSucceeded
		trashJob.CompletedAt = now
		trashJob.LeaseOwner = ""
		trashJob.LeaseExpiresAt = time.Time{}
		trashJob.UpdatedAt = now
		jobs = append(jobs, trashJob)
	}
	last := next.PrimaryRoot()
	mutation := fileLifecycleMutation{
		Kind:          "CommitCompleteTrash",
		Now:           now,
		Projects:      projects,
		Operations:    []ProjectOperation{current},
		CleanupJobs:   jobs,
		LastWorkspace: &last,
	}
	return s.commitMutation(mutation)
}

func (s *FileDesktopStore) CommitTrashFailure(operation ProjectOperation, status ProjectOperationStatus, phase, code, message string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return err
	}
	defer lock.Release()

	project, ok := s.state.Projects[operation.ProjectID]
	if !ok {
		return ErrProjectNotFound
	}
	project.State = operation.PreviousProjectState
	project.UpdatedAt = now
	project.Version++

	current := s.state.Operations[operation.OperationID]
	current.Status = status
	current.Phase = phase
	current.ErrorCode = strings.TrimSpace(code)
	current.ErrorMessage = strings.TrimSpace(message)
	current.UpdatedAt = now

	mutation := fileLifecycleMutation{
		Kind:       "CommitTrashFailure",
		Now:        now,
		Projects:   []Project{project},
		Operations: []ProjectOperation{current},
	}
	if trashJob, ok := s.trashJobForOperationLocked(operation.OperationID); ok {
		trashJob.Status = cleanupJobBlocked
		trashJob.LastError = strings.TrimSpace(message)
		trashJob.LeaseOwner = ""
		trashJob.LeaseExpiresAt = time.Time{}
		trashJob.UpdatedAt = now
		mutation.CleanupJobs = []fileCleanupJob{trashJob}
	}
	return s.commitMutation(mutation)
}

// --- CleanupJobStore ---

func (s *FileDesktopStore) ClaimThreadCleanupJobs(limit int, lease time.Duration, now time.Time) ([]cleanupJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return nil, err
	}
	defer lock.Release()

	var candidates []fileCleanupJob
	for _, job := range s.state.CleanupJobs {
		if job.Kind != cleanupJobThreadPurge {
			continue
		}
		switch job.Status {
		case cleanupJobPending:
			candidates = append(candidates, job)
		case cleanupJobRetry:
			if job.NextAttemptAt.IsZero() || !job.NextAttemptAt.After(now) {
				candidates = append(candidates, job)
			}
		case cleanupJobLeased:
			if !job.LeaseExpiresAt.After(now) {
				candidates = append(candidates, job)
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if !candidates[i].CreatedAt.Equal(candidates[j].CreatedAt) {
			return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
		}
		return candidates[i].JobID < candidates[j].JobID
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	workerID := newRuntimeID("cleanup_worker")
	claimed := make([]fileCleanupJob, 0, len(candidates))
	for _, job := range candidates {
		job.Status = cleanupJobLeased
		job.LeaseOwner = workerID
		job.LeaseExpiresAt = now.Add(lease)
		job.Attempts++
		job.UpdatedAt = now
		claimed = append(claimed, job)
	}
	if err := s.commitMutation(fileLifecycleMutation{
		Kind:        "ClaimCleanupJobs",
		Now:         now,
		CleanupJobs: claimed,
	}); err != nil {
		return nil, err
	}
	out := make([]cleanupJob, 0, len(claimed))
	for _, job := range claimed {
		out = append(out, job.toCleanupJob())
	}
	return out, nil
}

func (s *FileDesktopStore) CompleteThreadCleanupJob(job cleanupJob, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return err
	}
	defer lock.Release()

	stored, ok := s.state.CleanupJobs[job.JobID]
	if !ok {
		return nil
	}
	stored.Status = cleanupJobSucceeded
	stored.CompletedAt = now
	stored.LeaseOwner = ""
	stored.LeaseExpiresAt = time.Time{}
	stored.UpdatedAt = now

	mutation := fileLifecycleMutation{
		Kind:          "CompleteCleanupJob",
		Now:           now,
		CleanupJobs:   []fileCleanupJob{stored},
		PurgeSessions: []string{stored.TargetID},
	}
	// Finalize the parent operation when this was the last pending purge.
	remaining := 0
	for _, other := range s.state.CleanupJobs {
		if other.OperationID == stored.OperationID && other.Kind == cleanupJobThreadPurge &&
			other.Status != cleanupJobSucceeded && other.JobID != stored.JobID {
			remaining++
		}
	}
	if remaining == 0 {
		if operation, ok := s.state.Operations[stored.OperationID]; ok &&
			(operation.Status == ProjectOperationAccepted || operation.Status == ProjectOperationRunning) {
			operation.Status = ProjectOperationSucceeded
			operation.Phase = projectOperationPhaseCompleted
			operation.PhysicalCompletedAt = &now
			operation.UpdatedAt = now
			mutation.Operations = []ProjectOperation{operation}
		}
	}
	return s.commitMutation(mutation)
}

func (s *FileDesktopStore) RetryCleanupJob(job cleanupJob, cause error, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lockStore()
	if err != nil {
		return err
	}
	defer lock.Release()

	stored, ok := s.state.CleanupJobs[job.JobID]
	if !ok {
		return nil
	}
	attempts := job.Attempts + 1
	delay := time.Duration(attempts) * 250 * time.Millisecond
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	stored.Status = cleanupJobRetry
	stored.NextAttemptAt = now.Add(delay)
	stored.LeaseOwner = ""
	stored.LeaseExpiresAt = time.Time{}
	stored.LastError = cause.Error()
	stored.UpdatedAt = now
	return s.commitMutation(fileLifecycleMutation{
		Kind:        "RetryCleanupJob",
		Now:         now,
		CleanupJobs: []fileCleanupJob{stored},
	})
}
