package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrProjectNotFound          = errors.New("project not found")
	ErrProjectOperationNotFound = errors.New("project operation not found")
	ErrProjectOperationConflict = errors.New("project operation conflict")
	ErrProjectRunsBusy          = errors.New("project still has active runs")
	ErrProjectPathChanged       = errors.New("project path identity changed")
	ErrProjectTrashAttempt      = errors.New("project trash attempt is no longer current")
	ErrIdempotencyKeyRequired   = errors.New("idempotency key is required")
	ErrIdempotencyKeyConflict   = errors.New("idempotency key already belongs to another operation")
	ErrProjectPathInvalid       = errors.New("project path is invalid")
)

func ProjectErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrProjectNotFound):
		return "PROJECT_NOT_FOUND"
	case errors.Is(err, ErrProjectOperationNotFound):
		return "PROJECT_OPERATION_NOT_FOUND"
	case errors.Is(err, ErrProjectOperationConflict):
		return "PROJECT_OPERATION_CONFLICT"
	case errors.Is(err, ErrProjectRunsBusy):
		return "PROJECT_RUNS_BUSY"
	case errors.Is(err, ErrProjectPathChanged):
		return "PROJECT_PATH_FINGERPRINT_CHANGED"
	case errors.Is(err, ErrProjectTrashAttempt):
		return "PROJECT_TRASH_ATTEMPT_INVALID"
	case errors.Is(err, ErrIdempotencyKeyRequired):
		return "IDEMPOTENCY_KEY_REQUIRED"
	case errors.Is(err, ErrIdempotencyKeyConflict):
		return "IDEMPOTENCY_KEY_CONFLICT"
	case errors.Is(err, ErrProjectPathInvalid):
		return "PROJECT_PATH_INVALID"
	default:
		return "INTERNAL_ERROR"
	}
}

type ProjectService struct {
	runService *RunService
	store      *sqliteThreadStore
	wake       chan struct{}
}

func newProjectService(runService *RunService, store *sqliteThreadStore) *ProjectService {
	if runService == nil || store == nil {
		return nil
	}
	return &ProjectService{
		runService: runService,
		store:      store,
		wake:       make(chan struct{}, 1),
	}
}

func (s *ProjectService) CanStartRun(workspaceRoot string) (bool, error) {
	root := normalizeProjectWorkspaceRoot(workspaceRoot)
	if root == "" {
		return false, ErrProjectPathInvalid
	}
	db, err := s.store.open()
	if err != nil {
		return false, err
	}
	var state string
	err = db.QueryRow(`SELECT state FROM projects WHERE workspace_key = ?`, root).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return ProjectState(state) == ProjectStateActive, nil
}

func (s *ProjectService) ListProjects(includeRemoved bool) ([]Project, error) {
	db, err := s.store.open()
	if err != nil {
		return nil, err
	}
	query := `
SELECT project_id, workspace_root, display_name, state, removal_reason, removed_at, created_at, updated_at, version
FROM projects`
	if !includeRemoved {
		query += ` WHERE state <> 'removed'`
	}
	query += ` ORDER BY updated_at DESC, created_at DESC`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	var projects []Project
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate projects: %w", err)
	}
	return projects, nil
}

func (s *ProjectService) UpsertProject(workspaceRoot, displayName string) (Project, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	root := normalizeProjectWorkspaceRoot(workspaceRoot)
	if root == "" {
		return Project{}, ErrProjectPathInvalid
	}
	db, err := s.store.open()
	if err != nil {
		return Project{}, err
	}
	var existingState string
	err = db.QueryRow(`SELECT state FROM projects WHERE workspace_key = ?`, root).Scan(&existingState)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Project{}, fmt.Errorf("read project before upsert: %w", err)
	}
	if ProjectState(existingState) == ProjectStateTrashPending {
		return Project{}, ErrProjectOperationConflict
	}
	projectID, err := ensureProjectForWorkspace(db, root, true)
	if err != nil {
		return Project{}, fmt.Errorf("%w: %v", ErrProjectPathInvalid, err)
	}
	if strings.TrimSpace(displayName) != "" {
		if _, err := db.Exec(`UPDATE projects SET display_name = ?, updated_at = ?, version = version + 1 WHERE project_id = ?`, strings.TrimSpace(displayName), formatStoredTime(time.Now().UTC()), projectID); err != nil {
			return Project{}, fmt.Errorf("update project display name: %w", err)
		}
	}
	if _, err := db.Exec(`
INSERT INTO desktop_state(key, value, updated_at) VALUES('last_workspace_path', ?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
`, root, formatStoredTime(time.Now().UTC())); err != nil {
		return Project{}, fmt.Errorf("persist last project workspace: %w", err)
	}
	s.runService.setProjectDeletionFence(root, false)
	return projectByID(db, projectID)
}

func (s *ProjectService) DetachProject(projectID, idempotencyKey string) (ProjectOperation, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	if err := requireIdempotencyKey(idempotencyKey); err != nil {
		return ProjectOperation{}, err
	}
	db, err := s.store.open()
	if err != nil {
		return ProjectOperation{}, err
	}
	if existing, found, err := lookupIdempotentOperation(db, idempotencyKey, projectID, ProjectOperationDetach); err != nil || found {
		return existing, err
	}
	tx, err := db.Begin()
	if err != nil {
		return ProjectOperation{}, err
	}
	defer tx.Rollback()
	project, err := projectByID(tx, projectID)
	if err != nil {
		return ProjectOperation{}, err
	}
	if project.State == ProjectStateTrashPending {
		return ProjectOperation{}, ErrProjectOperationConflict
	}
	now := time.Now().UTC()
	operation := newProjectOperation(project, idempotencyKey, ProjectOperationDetach, ProjectOperationSucceeded, projectOperationPhaseCompleted, now)
	operation.LogicalCommittedAt = &now
	operation.PhysicalCompletedAt = &now
	if err := insertProjectOperation(tx, operation, 0); err != nil {
		return ProjectOperation{}, err
	}
	if _, err := tx.Exec(`
UPDATE projects
SET state = 'removed', removal_reason = 'detached', removed_at = ?, updated_at = ?, version = version + 1
WHERE project_id = ?
`, formatStoredTime(now), formatStoredTime(now), project.ProjectID); err != nil {
		return ProjectOperation{}, fmt.Errorf("detach project: %w", err)
	}
	if err := selectFallbackWorkspaceTx(tx, project.ProjectID, now); err != nil {
		return ProjectOperation{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProjectOperation{}, fmt.Errorf("commit project detach: %w", err)
	}
	s.runService.setProjectDeletionFence(project.WorkspaceRoot, true)
	s.runService.cancelRunsForThreads(s.runService.threadIDsForWorkspace(project.WorkspaceRoot))
	return operation, nil
}

func (s *ProjectService) DeleteProjectThreads(projectID, idempotencyKey string) (ProjectOperation, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	if err := requireIdempotencyKey(idempotencyKey); err != nil {
		return ProjectOperation{}, err
	}
	db, err := s.store.open()
	if err != nil {
		return ProjectOperation{}, err
	}
	if existing, found, err := lookupIdempotentOperation(db, idempotencyKey, projectID, ProjectOperationDeleteHistory); err != nil || found {
		return existing, err
	}
	project, err := projectByID(db, projectID)
	if err != nil {
		return ProjectOperation{}, err
	}
	if project.State == ProjectStateTrashPending {
		return ProjectOperation{}, ErrProjectOperationConflict
	}
	s.runService.setProjectDeletionFence(project.WorkspaceRoot, true)
	committed := false
	defer func() {
		if !committed || project.State == ProjectStateActive {
			s.runService.setProjectDeletionFence(project.WorkspaceRoot, false)
		}
	}()
	tx, err := db.Begin()
	if err != nil {
		return ProjectOperation{}, err
	}
	defer tx.Rollback()
	threadIDs, err := liveThreadIDsForProject(tx, projectID)
	if err != nil {
		return ProjectOperation{}, err
	}
	now := time.Now().UTC()
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
	if err := insertProjectOperation(tx, operation, len(threadIDs)); err != nil {
		return ProjectOperation{}, err
	}
	if err := softDeleteThreadsTx(tx, operation.OperationID, threadIDs, now); err != nil {
		return ProjectOperation{}, err
	}
	if err := insertThreadCleanupJobsTx(tx, operation.OperationID, threadIDs, now); err != nil {
		return ProjectOperation{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProjectOperation{}, fmt.Errorf("commit project history deletion: %w", err)
	}
	committed = true
	operation.MatchedThreads = len(threadIDs)
	operation.PendingCleanup = len(threadIDs)
	s.runService.markThreadsDeletionRequested(threadIDs)
	s.runService.cancelRunsForThreads(threadIDs)
	s.Wake()
	return operation, nil
}

func (s *ProjectService) DeleteThread(threadID string) error {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil
	}
	db, err := s.store.open()
	if err != nil {
		return err
	}
	var projectID string
	var deleted int
	err = db.QueryRow(`SELECT project_id, deleted FROM threads WHERE thread_id = ?`, threadID).Scan(&projectID, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read thread before delete: %w", err)
	}
	if deleted != 0 {
		s.Wake()
		return nil
	}
	project, err := projectByID(db, projectID)
	if err != nil {
		return err
	}
	if project.State == ProjectStateTrashPending {
		return ErrProjectOperationConflict
	}
	idempotencyKey := "thread-delete:" + threadID
	if _, found, err := lookupIdempotentOperation(db, idempotencyKey, projectID, ProjectOperationDeleteThread); err != nil {
		return err
	} else if found {
		s.Wake()
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	operation := newProjectOperation(project, idempotencyKey, ProjectOperationDeleteThread, ProjectOperationAccepted, projectOperationPhaseCleaning, now)
	operation.LogicalCommittedAt = &now
	if err := insertProjectOperation(tx, operation, 1); err != nil {
		return err
	}
	if err := softDeleteThreadsTx(tx, operation.OperationID, []string{threadID}, now); err != nil {
		return err
	}
	if err := insertThreadCleanupJobsTx(tx, operation.OperationID, []string{threadID}, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit thread deletion: %w", err)
	}
	s.runService.markThreadsDeletionRequested([]string{threadID})
	s.runService.cancelRunsForThreads([]string{threadID})
	s.Wake()
	return nil
}

func (s *ProjectService) PrepareTrash(projectID, idempotencyKey string) (ProjectOperation, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	if err := requireIdempotencyKey(idempotencyKey); err != nil {
		return ProjectOperation{}, err
	}
	db, err := s.store.open()
	if err != nil {
		return ProjectOperation{}, err
	}
	if existing, found, err := lookupIdempotentOperation(db, idempotencyKey, projectID, ProjectOperationTrashAndPurge); err != nil || found {
		if found && (existing.Status == ProjectOperationAccepted || existing.Status == ProjectOperationRunning) {
			s.runService.setProjectDeletionFence(existing.WorkspaceRoot, true)
		}
		return existing, err
	}
	project, err := projectByID(db, projectID)
	if err != nil {
		return ProjectOperation{}, err
	}
	if project.State == ProjectStateTrashPending {
		return ProjectOperation{}, ErrProjectOperationConflict
	}
	if err := validateTrashProjectPath(project.WorkspaceRoot, s.store.paths.Root()); err != nil {
		return ProjectOperation{}, err
	}
	fingerprint, err := projectPathFingerprint(project.WorkspaceRoot)
	if err != nil {
		return ProjectOperation{}, fmt.Errorf("%w: %v", ErrProjectPathInvalid, err)
	}
	s.runService.setProjectDeletionFence(project.WorkspaceRoot, true)
	committed := false
	defer func() {
		if !committed {
			s.runService.setProjectDeletionFence(project.WorkspaceRoot, false)
		}
	}()
	tx, err := db.Begin()
	if err != nil {
		return ProjectOperation{}, err
	}
	defer tx.Rollback()
	threadIDs, err := liveThreadIDsForProject(tx, projectID)
	if err != nil {
		return ProjectOperation{}, err
	}
	now := time.Now().UTC()
	operation := newProjectOperation(project, idempotencyKey, ProjectOperationTrashAndPurge, ProjectOperationAccepted, projectOperationPhaseWaitingForRuns, now)
	operation.PathFingerprint = fingerprint
	if err := insertProjectOperation(tx, operation, len(threadIDs)); err != nil {
		return ProjectOperation{}, err
	}
	for _, threadID := range threadIDs {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO project_operation_threads(operation_id, thread_id) VALUES(?, ?)`, operation.OperationID, threadID); err != nil {
			return ProjectOperation{}, fmt.Errorf("capture trash operation thread: %w", err)
		}
	}
	if _, err := tx.Exec(`
UPDATE projects SET state = 'trash_pending', updated_at = ?, version = version + 1 WHERE project_id = ?
`, formatStoredTime(now), projectID); err != nil {
		return ProjectOperation{}, fmt.Errorf("mark project trash pending: %w", err)
	}
	if _, err := tx.Exec(`
INSERT INTO cleanup_jobs(job_id, operation_id, kind, target_id, status, next_attempt_at, created_at, updated_at)
VALUES(?, ?, 'project_trash', ?, 'pending', ?, ?, ?)
`, newRuntimeID("job"), operation.OperationID, projectID, formatStoredTime(now), formatStoredTime(now), formatStoredTime(now)); err != nil {
		return ProjectOperation{}, fmt.Errorf("enqueue project trash job: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ProjectOperation{}, fmt.Errorf("commit trash prepare: %w", err)
	}
	committed = true
	operation.MatchedThreads = len(threadIDs)
	s.runService.cancelRunsForThreads(threadIDs)
	return operation, nil
}

func (s *ProjectService) GetOperation(operationID string) (ProjectOperation, error) {
	db, err := s.store.open()
	if err != nil {
		return ProjectOperation{}, err
	}
	operation, err := projectOperationByID(db, operationID)
	if err != nil {
		return ProjectOperation{}, err
	}
	operation.PendingCleanup, _ = pendingCleanupCount(db, operation.OperationID)
	return operation, nil
}

func (s *ProjectService) ListPendingTrashOperations() ([]ProjectOperation, error) {
	db, err := s.store.open()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(projectOperationSelect + `
 WHERE action = 'trash_and_purge'
   AND status IN ('accepted', 'running')
   AND phase IN ('waiting_for_runs', 'trash_claimed', 'trash_staged', 'trashing')
 ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var operations []ProjectOperation
	for rows.Next() {
		operation, err := scanProjectOperation(rows)
		if err != nil {
			return nil, err
		}
		operations = append(operations, operation)
	}
	return operations, rows.Err()
}

func (s *ProjectService) ClaimTrashOperation(operationID, workerID string, lease time.Duration) (ProjectOperation, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		workerID = "desktop-main"
	}
	if lease <= 0 {
		lease = 30 * time.Second
	}
	db, err := s.store.open()
	if err != nil {
		return ProjectOperation{}, err
	}
	operation, err := projectOperationByID(db, operationID)
	if err != nil {
		return ProjectOperation{}, err
	}
	if operation.Action != ProjectOperationTrashAndPurge ||
		(operation.Status != ProjectOperationAccepted && operation.Status != ProjectOperationRunning) ||
		(operation.Phase != projectOperationPhaseWaitingForRuns &&
			operation.Phase != projectOperationPhaseTrashClaimed &&
			operation.Phase != projectOperationPhaseTrashStaged &&
			operation.Phase != projectOperationPhaseTrashing) {
		return ProjectOperation{}, ErrProjectOperationConflict
	}
	threadIDs, err := operationThreadIDs(db, operationID)
	if err != nil {
		return ProjectOperation{}, err
	}
	if s.runService.anyThreadActive(threadIDs) {
		return ProjectOperation{}, ErrProjectRunsBusy
	}
	tx, err := db.Begin()
	if err != nil {
		return ProjectOperation{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	attemptID := newRuntimeID("trash_attempt")
	result, err := tx.Exec(`
UPDATE cleanup_jobs
SET status = 'leased', lease_owner = ?, lease_expires_at = ?, attempts = attempts + 1, updated_at = ?
WHERE operation_id = ? AND kind = 'project_trash'
  AND (status IN ('pending', 'retry') OR (status = 'leased' AND lease_expires_at <= ?))
`, workerID, formatStoredTime(now.Add(lease)), formatStoredTime(now), operationID, formatStoredTime(now))
	if err != nil {
		return ProjectOperation{}, fmt.Errorf("claim project trash job: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ProjectOperation{}, ErrProjectOperationConflict
	}
	nextPhase := projectOperationPhaseTrashClaimed
	if operation.Phase == projectOperationPhaseTrashStaged {
		nextPhase = projectOperationPhaseTrashStaged
	} else if operation.Phase == projectOperationPhaseTrashing {
		// A previous worker durably crossed the invocation boundary. Preserve that
		// fact so a replacement worker can distinguish post-trash ENOENT from a
		// source that vanished before any system trash call was attempted.
		nextPhase = projectOperationPhaseTrashing
	}
	if _, err := tx.Exec(`
UPDATE project_operations
SET status = 'running', phase = ?, attempt_id = ?, updated_at = ?
WHERE operation_id = ?
`, nextPhase, attemptID, formatStoredTime(now), operationID); err != nil {
		return ProjectOperation{}, fmt.Errorf("mark trash operation running: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ProjectOperation{}, err
	}
	return s.GetOperation(operationID)
}

// StageTrashOperation atomically moves the project into an operation-owned
// sibling directory before Electron invokes the platform trash API. The
// post-rename identity check closes the attest-to-trash path replacement race:
// a swapped-in directory may be staged temporarily, but it is restored and is
// never passed to shell.trashItem.
func (s *ProjectService) StageTrashOperation(operationID, attemptID string) (ProjectOperation, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	db, err := s.store.open()
	if err != nil {
		return ProjectOperation{}, err
	}
	operation, err := projectOperationByID(db, operationID)
	if err != nil {
		return ProjectOperation{}, err
	}
	if err := validateTrashAttempt(operation, attemptID); err != nil {
		return ProjectOperation{}, err
	}
	if operation.Action != ProjectOperationTrashAndPurge ||
		(operation.Phase != projectOperationPhaseTrashClaimed &&
			operation.Phase != projectOperationPhaseTrashStaged &&
			operation.Phase != projectOperationPhaseTrashing) {
		return ProjectOperation{}, ErrProjectOperationConflict
	}
	container, stagedPath, err := projectTrashStagingPaths(operation)
	if err != nil {
		return ProjectOperation{}, err
	}
	if operation.TrashStagingPath != "" && filepath.Clean(operation.TrashStagingPath) != filepath.Clean(stagedPath) {
		return ProjectOperation{}, ErrProjectOperationConflict
	}

	stagedExists, err := pathExists(stagedPath)
	if err != nil {
		return ProjectOperation{}, err
	}
	if !stagedExists {
		sourceExists, sourceErr := pathExists(operation.WorkspaceRoot)
		if sourceErr != nil {
			return ProjectOperation{}, sourceErr
		}
		if !sourceExists {
			if operation.Phase == projectOperationPhaseTrashing && operation.TrashInvokedAt != nil {
				if _, err := db.Exec(`UPDATE project_operations SET trash_staging_path = ?, updated_at = ? WHERE operation_id = ? AND attempt_id = ?`, stagedPath, formatStoredTime(time.Now().UTC()), operationID, strings.TrimSpace(attemptID)); err != nil {
					return ProjectOperation{}, err
				}
				return s.GetOperation(operationID)
			}
			return ProjectOperation{}, fmt.Errorf("%w: project source disappeared before staging", ErrProjectPathChanged)
		}
		matches, matchErr := projectFingerprintMatchesPath(operation.PathFingerprint, operation.WorkspaceRoot, operation.WorkspaceRoot)
		if matchErr != nil || !matches {
			return ProjectOperation{}, ErrProjectPathChanged
		}
		if err := ensureTrashStagingContainer(container); err != nil {
			return ProjectOperation{}, err
		}
		if err := os.Rename(operation.WorkspaceRoot, stagedPath); err != nil {
			return ProjectOperation{}, fmt.Errorf("stage project for system trash: %w", err)
		}
		matches, matchErr = projectFingerprintMatchesPath(operation.PathFingerprint, operation.WorkspaceRoot, stagedPath)
		if matchErr != nil || !matches {
			if restoreErr := os.Rename(stagedPath, operation.WorkspaceRoot); restoreErr != nil {
				return ProjectOperation{}, fmt.Errorf("%w: staged identity changed and restore failed: %v", ErrProjectPathChanged, restoreErr)
			}
			_ = os.Remove(container)
			return ProjectOperation{}, ErrProjectPathChanged
		}
	} else {
		matches, matchErr := projectFingerprintMatchesPath(operation.PathFingerprint, operation.WorkspaceRoot, stagedPath)
		if matchErr != nil || !matches {
			return ProjectOperation{}, ErrProjectPathChanged
		}
	}

	now := time.Now().UTC()
	nextPhase := projectOperationPhaseTrashStaged
	if operation.Phase == projectOperationPhaseTrashing {
		nextPhase = projectOperationPhaseTrashing
	}
	if _, err := db.Exec(`
UPDATE project_operations
SET status = 'running', phase = ?, trash_staging_path = ?, updated_at = ?
WHERE operation_id = ? AND attempt_id = ?
`, nextPhase, stagedPath, formatStoredTime(now), operationID, strings.TrimSpace(attemptID)); err != nil {
		return ProjectOperation{}, fmt.Errorf("persist staged trash path: %w", err)
	}
	return s.GetOperation(operationID)
}

func (s *ProjectService) AttestTrashOperationPath(operationID, attemptID string) (ProjectOperation, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	db, err := s.store.open()
	if err != nil {
		return ProjectOperation{}, err
	}
	operation, err := projectOperationByID(db, operationID)
	if err != nil {
		return ProjectOperation{}, err
	}
	if err := validateTrashAttempt(operation, attemptID); err != nil {
		return ProjectOperation{}, err
	}
	if operation.Action != ProjectOperationTrashAndPurge || operation.Phase != projectOperationPhaseTrashClaimed {
		return ProjectOperation{}, ErrProjectOperationConflict
	}
	current, err := projectPathFingerprint(operation.WorkspaceRoot)
	if err != nil {
		return ProjectOperation{}, fmt.Errorf("%w: %v", ErrProjectPathChanged, err)
	}
	if current != operation.PathFingerprint {
		return ProjectOperation{}, ErrProjectPathChanged
	}
	return operation, nil
}

func (s *ProjectService) BeginTrashInvocation(operationID, attemptID string) (ProjectOperation, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	db, err := s.store.open()
	if err != nil {
		return ProjectOperation{}, err
	}
	tx, err := db.Begin()
	if err != nil {
		return ProjectOperation{}, err
	}
	defer tx.Rollback()
	operation, err := projectOperationByID(tx, operationID)
	if err != nil {
		return ProjectOperation{}, err
	}
	if err := validateTrashAttempt(operation, attemptID); err != nil {
		return ProjectOperation{}, err
	}
	if operation.Action != ProjectOperationTrashAndPurge ||
		(operation.Phase != projectOperationPhaseTrashStaged && operation.Phase != projectOperationPhaseTrashing) {
		return ProjectOperation{}, ErrProjectOperationConflict
	}
	if operation.TrashStagingPath == "" {
		return ProjectOperation{}, ErrProjectOperationConflict
	}
	if operation.Phase == projectOperationPhaseTrashStaged {
		exists, pathErr := pathExists(operation.TrashStagingPath)
		if pathErr != nil || !exists {
			return ProjectOperation{}, ErrProjectOperationConflict
		}
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(`
UPDATE project_operations
SET status = 'running', phase = 'trashing', trash_invoked_at = ?, updated_at = ?
WHERE operation_id = ? AND attempt_id = ?
`, formatStoredTime(now), formatStoredTime(now), operationID, strings.TrimSpace(attemptID)); err != nil {
		return ProjectOperation{}, fmt.Errorf("mark trash invocation durable: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ProjectOperation{}, err
	}
	return s.GetOperation(operationID)
}

func (s *ProjectService) CompleteTrashOperation(operationID, attemptID string) (ProjectOperation, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	db, err := s.store.open()
	if err != nil {
		return ProjectOperation{}, err
	}
	tx, err := db.Begin()
	if err != nil {
		return ProjectOperation{}, err
	}
	defer tx.Rollback()
	operation, err := projectOperationByID(tx, operationID)
	if err != nil {
		return ProjectOperation{}, err
	}
	if err := validateTrashAttempt(operation, attemptID); err != nil {
		return ProjectOperation{}, err
	}
	if operation.Action != ProjectOperationTrashAndPurge {
		return ProjectOperation{}, ErrProjectOperationConflict
	}
	if operation.Phase == projectOperationPhaseCleaning || operation.Phase == projectOperationPhaseCompleted {
		return operation, nil
	}
	if operation.Phase != projectOperationPhaseTrashing || operation.TrashInvokedAt == nil {
		return ProjectOperation{}, ErrProjectOperationConflict
	}
	container, stagedPath, err := projectTrashStagingPaths(operation)
	if err != nil {
		return ProjectOperation{}, err
	}
	if operation.TrashStagingPath != "" {
		stagedPath = operation.TrashStagingPath
	}
	stagedExists, err := pathExists(stagedPath)
	if err != nil {
		return ProjectOperation{}, err
	}
	if stagedExists {
		return ProjectOperation{}, fmt.Errorf("%w: staged project still exists after trash invocation", ErrProjectOperationConflict)
	}
	if originalExists, existsErr := pathExists(operation.WorkspaceRoot); existsErr != nil {
		return ProjectOperation{}, existsErr
	} else if originalExists {
		matchesOriginal, matchErr := projectFingerprintMatchesPath(operation.PathFingerprint, operation.WorkspaceRoot, operation.WorkspaceRoot)
		if matchErr != nil {
			return ProjectOperation{}, matchErr
		}
		if matchesOriginal {
			return ProjectOperation{}, fmt.Errorf("%w: original project was not moved to trash", ErrProjectOperationConflict)
		}
	}
	_ = os.Remove(container)
	threadIDs, err := operationThreadIDs(tx, operationID)
	if err != nil {
		return ProjectOperation{}, err
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(`
UPDATE projects
SET state = 'removed', removal_reason = 'trashed', removed_at = ?, updated_at = ?, version = version + 1
WHERE project_id = ?
`, formatStoredTime(now), formatStoredTime(now), operation.ProjectID); err != nil {
		return ProjectOperation{}, fmt.Errorf("finalize trashed project: %w", err)
	}
	if err := softDeleteThreadsTx(tx, operationID, threadIDs, now); err != nil {
		return ProjectOperation{}, err
	}
	if err := insertThreadCleanupJobsTx(tx, operationID, threadIDs, now); err != nil {
		return ProjectOperation{}, err
	}
	status := ProjectOperationAccepted
	phase := projectOperationPhaseCleaning
	physicalCompleted := ""
	if len(threadIDs) == 0 {
		status = ProjectOperationSucceeded
		phase = projectOperationPhaseCompleted
		physicalCompleted = formatStoredTime(now)
	}
	resultJSON, _ := json.Marshal(map[string]int{"matched_threads": len(threadIDs)})
	if _, err := tx.Exec(`
UPDATE project_operations
SET status = ?, phase = ?, logical_committed_at = ?, physical_completed_at = ?, result_json = ?, updated_at = ?
WHERE operation_id = ?
`, status, phase, formatStoredTime(now), physicalCompleted, string(resultJSON), formatStoredTime(now), operationID); err != nil {
		return ProjectOperation{}, fmt.Errorf("finalize trash operation: %w", err)
	}
	if _, err := tx.Exec(`
UPDATE cleanup_jobs SET status = 'succeeded', completed_at = ?, lease_owner = '', lease_expires_at = '', updated_at = ?
WHERE operation_id = ? AND kind = 'project_trash'
`, formatStoredTime(now), formatStoredTime(now), operationID); err != nil {
		return ProjectOperation{}, fmt.Errorf("complete trash job: %w", err)
	}
	if err := selectFallbackWorkspaceTx(tx, operation.ProjectID, now); err != nil {
		return ProjectOperation{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProjectOperation{}, err
	}
	s.runService.markThreadsDeletionRequested(threadIDs)
	s.Wake()
	return s.GetOperation(operationID)
}

func (s *ProjectService) FailTrashOperation(operationID, attemptID, code, message string) (ProjectOperation, error) {
	return s.finishTrashWithoutDeletion(operationID, attemptID, ProjectOperationFailed, projectOperationPhaseFailed, code, message)
}

func (s *ProjectService) CancelOperation(operationID string) (ProjectOperation, error) {
	return s.finishTrashWithoutDeletion(operationID, "", ProjectOperationFailed, projectOperationPhaseCancelled, "PROJECT_OPERATION_CANCELLED", "project operation cancelled")
}

func (s *ProjectService) finishTrashWithoutDeletion(operationID, attemptID string, status ProjectOperationStatus, phase, code, message string) (ProjectOperation, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	db, err := s.store.open()
	if err != nil {
		return ProjectOperation{}, err
	}
	tx, err := db.Begin()
	if err != nil {
		return ProjectOperation{}, err
	}
	defer tx.Rollback()
	operation, err := projectOperationByID(tx, operationID)
	if err != nil {
		return ProjectOperation{}, err
	}
	if operation.Action != ProjectOperationTrashAndPurge || operation.Phase == projectOperationPhaseCleaning || operation.Phase == projectOperationPhaseCompleted {
		return ProjectOperation{}, ErrProjectOperationConflict
	}
	if phase != projectOperationPhaseCancelled {
		if err := validateTrashAttempt(operation, attemptID); err != nil {
			return ProjectOperation{}, err
		}
	}
	if phase == projectOperationPhaseCancelled && operation.Phase != projectOperationPhaseWaitingForRuns {
		return ProjectOperation{}, ErrProjectOperationConflict
	}
	if err := restoreStagedProject(operation); err != nil {
		return ProjectOperation{}, err
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(`
UPDATE projects SET state = ?, updated_at = ?, version = version + 1 WHERE project_id = ?
`, operation.PreviousProjectState, formatStoredTime(now), operation.ProjectID); err != nil {
		return ProjectOperation{}, fmt.Errorf("restore project after trash failure: %w", err)
	}
	if _, err := tx.Exec(`
UPDATE project_operations
SET status = ?, phase = ?, error_code = ?, error_message = ?, updated_at = ?
WHERE operation_id = ?
`, status, phase, strings.TrimSpace(code), strings.TrimSpace(message), formatStoredTime(now), operationID); err != nil {
		return ProjectOperation{}, err
	}
	if _, err := tx.Exec(`
UPDATE cleanup_jobs SET status = 'blocked', last_error = ?, lease_owner = '', lease_expires_at = '', updated_at = ?
WHERE operation_id = ? AND kind = 'project_trash'
`, strings.TrimSpace(message), formatStoredTime(now), operationID); err != nil {
		return ProjectOperation{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProjectOperation{}, err
	}
	if operation.PreviousProjectState == ProjectStateActive {
		s.runService.setProjectDeletionFence(operation.WorkspaceRoot, false)
	}
	return s.GetOperation(operationID)
}

func (s *ProjectService) RunCleanupWorker(ctx context.Context) {
	if s == nil {
		return
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, _ = s.ProcessCleanupJobs(ctx, 16)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.wake:
		}
	}
}

func (s *ProjectService) Wake() {
	if s == nil {
		return
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *ProjectService) ProcessCleanupJobs(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 16
	}
	jobs, err := s.claimThreadCleanupJobs(limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, job := range jobs {
		if err := ctx.Err(); err != nil {
			return processed, err
		}
		err := s.purgeThread(job.TargetID)
		if err == nil {
			if finalizeErr := s.completeThreadCleanupJob(job); finalizeErr != nil {
				err = finalizeErr
			}
		}
		if err != nil {
			if retryErr := s.retryCleanupJob(job, err); retryErr != nil {
				return processed, retryErr
			}
			continue
		}
		processed++
	}
	return processed, nil
}

func (s *ProjectService) claimThreadCleanupJobs(limit int) ([]cleanupJob, error) {
	db, err := s.store.open()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	rows, err := db.Query(`
SELECT job_id, operation_id, kind, target_id, status, attempts, next_attempt_at, lease_owner, lease_expires_at
FROM cleanup_jobs
WHERE kind = 'thread_purge'
  AND (status = 'pending' OR (status = 'retry' AND (next_attempt_at = '' OR next_attempt_at <= ?)) OR (status = 'leased' AND lease_expires_at <= ?))
ORDER BY created_at ASC LIMIT ?
`, formatStoredTime(now), formatStoredTime(now), limit)
	if err != nil {
		return nil, err
	}
	var jobs []cleanupJob
	for rows.Next() {
		var job cleanupJob
		var nextRaw, leaseRaw string
		if err := rows.Scan(&job.JobID, &job.OperationID, &job.Kind, &job.TargetID, &job.Status, &job.Attempts, &nextRaw, &job.LeaseOwner, &leaseRaw); err != nil {
			rows.Close()
			return nil, err
		}
		job.NextAttemptAt = parseStoredTime(nextRaw)
		job.LeaseExpiresAt = parseStoredTime(leaseRaw)
		jobs = append(jobs, job)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	workerID := newRuntimeID("cleanup_worker")
	claimed := jobs[:0]
	for _, job := range jobs {
		result, err := db.Exec(`
UPDATE cleanup_jobs
SET status = 'leased', lease_owner = ?, lease_expires_at = ?, attempts = attempts + 1, updated_at = ?
WHERE job_id = ? AND (status IN ('pending', 'retry') OR (status = 'leased' AND lease_expires_at <= ?))
`, workerID, formatStoredTime(now.Add(30*time.Second)), formatStoredTime(now), job.JobID, formatStoredTime(now))
		if err != nil {
			return nil, err
		}
		if affected, _ := result.RowsAffected(); affected > 0 {
			job.LeaseOwner = workerID
			claimed = append(claimed, job)
		}
	}
	return claimed, nil
}

func (s *ProjectService) purgeThread(threadID string) error {
	if s.runService.isThreadActive(threadID) {
		return ErrThreadBusy
	}
	if err := s.runService.closeAndEvictDeletedThread(threadID); err != nil {
		return err
	}
	if err := deleteThreadTranscriptAtPath(s.store.paths.ThreadTranscriptPath(threadID)); err != nil && !errors.Is(err, ErrThreadNotFound) {
		return err
	}
	if err := deleteThreadInputAttachments(threadID); err != nil && !errors.Is(err, ErrThreadNotFound) {
		return err
	}
	return nil
}

func (s *ProjectService) completeThreadCleanupJob(job cleanupJob) error {
	db, err := s.store.open()
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	if _, err := tx.Exec(`DELETE FROM threads WHERE thread_id = ? AND deleted <> 0`, job.TargetID); err != nil {
		return fmt.Errorf("finalize deleted thread %q: %w", job.TargetID, err)
	}
	if _, err := tx.Exec(`
UPDATE cleanup_jobs SET status = 'succeeded', completed_at = ?, lease_owner = '', lease_expires_at = '', updated_at = ?
WHERE job_id = ?
`, formatStoredTime(now), formatStoredTime(now), job.JobID); err != nil {
		return err
	}
	remaining, err := pendingCleanupCount(tx, job.OperationID)
	if err != nil {
		return err
	}
	if remaining == 0 {
		if _, err := tx.Exec(`
UPDATE project_operations SET status = 'succeeded', phase = 'completed', physical_completed_at = ?, updated_at = ?
WHERE operation_id = ? AND status IN ('accepted', 'running')
`, formatStoredTime(now), formatStoredTime(now), job.OperationID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *ProjectService) retryCleanupJob(job cleanupJob, cause error) error {
	db, err := s.store.open()
	if err != nil {
		return err
	}
	attempts := job.Attempts + 1
	delay := time.Duration(attempts) * 250 * time.Millisecond
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	now := time.Now().UTC()
	_, err = db.Exec(`
UPDATE cleanup_jobs
SET status = 'retry', next_attempt_at = ?, lease_owner = '', lease_expires_at = '', last_error = ?, updated_at = ?
WHERE job_id = ?
`, formatStoredTime(now.Add(delay)), cause.Error(), formatStoredTime(now), job.JobID)
	return err
}

func requireIdempotencyKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return ErrIdempotencyKeyRequired
	}
	return nil
}

func validateTrashAttempt(operation ProjectOperation, attemptID string) error {
	attemptID = strings.TrimSpace(attemptID)
	if attemptID == "" || operation.AttemptID == "" || attemptID != operation.AttemptID {
		return ErrProjectTrashAttempt
	}
	return nil
}

func lookupIdempotentOperation(db projectDBTX, key, projectID string, action ProjectOperationAction) (ProjectOperation, bool, error) {
	operation, err := projectOperationByIdempotencyKey(db, key)
	if errors.Is(err, ErrProjectOperationNotFound) {
		return ProjectOperation{}, false, nil
	}
	if err != nil {
		return ProjectOperation{}, false, err
	}
	if operation.ProjectID != projectID || operation.Action != action {
		return ProjectOperation{}, false, ErrIdempotencyKeyConflict
	}
	operation.PendingCleanup, _ = pendingCleanupCount(db, operation.OperationID)
	return operation, true, nil
}

func newProjectOperation(project Project, key string, action ProjectOperationAction, status ProjectOperationStatus, phase string, now time.Time) ProjectOperation {
	return ProjectOperation{
		OperationID:          newRuntimeID("project_operation"),
		IdempotencyKey:       strings.TrimSpace(key),
		ProjectID:            project.ProjectID,
		Action:               action,
		Status:               status,
		Phase:                phase,
		PreviousProjectState: project.State,
		WorkspaceRoot:        project.WorkspaceRoot,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}

func insertProjectOperation(tx *sql.Tx, operation ProjectOperation, matchedThreads int) error {
	resultJSON, _ := json.Marshal(map[string]int{"matched_threads": matchedThreads})
	logical := ""
	if operation.LogicalCommittedAt != nil {
		logical = formatStoredTime(*operation.LogicalCommittedAt)
	}
	physical := ""
	if operation.PhysicalCompletedAt != nil {
		physical = formatStoredTime(*operation.PhysicalCompletedAt)
	}
	_, err := tx.Exec(`
INSERT INTO project_operations(
  operation_id, idempotency_key, project_id, action, status, phase,
  previous_project_state, workspace_root, path_fingerprint, attempt_id,
  trash_staging_path, trash_invoked_at, cutoff_at, logical_committed_at,
  physical_completed_at, result_json, created_at, updated_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, operation.OperationID, operation.IdempotencyKey, operation.ProjectID, operation.Action, operation.Status, operation.Phase,
		operation.PreviousProjectState, operation.WorkspaceRoot, operation.PathFingerprint, operation.AttemptID, operation.TrashStagingPath, "", formatStoredTime(operation.CreatedAt),
		logical, physical, string(resultJSON), formatStoredTime(operation.CreatedAt), formatStoredTime(operation.UpdatedAt))
	if err != nil {
		return fmt.Errorf("insert project operation: %w", err)
	}
	return nil
}

func liveThreadIDsForProject(tx *sql.Tx, projectID string) ([]string, error) {
	rows, err := tx.Query(`SELECT thread_id FROM threads WHERE project_id = ? AND deleted = 0 ORDER BY created_at`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project threads: %w", err)
	}
	defer rows.Close()
	var threadIDs []string
	for rows.Next() {
		var threadID string
		if err := rows.Scan(&threadID); err != nil {
			return nil, err
		}
		threadIDs = append(threadIDs, threadID)
	}
	return threadIDs, rows.Err()
}

func operationThreadIDs(db projectDBTX, operationID string) ([]string, error) {
	queryer, ok := db.(interface {
		Query(query string, args ...any) (*sql.Rows, error)
	})
	if !ok {
		return nil, fmt.Errorf("operation thread query unavailable")
	}
	rows, err := queryer.Query(`SELECT thread_id FROM project_operation_threads WHERE operation_id = ? ORDER BY thread_id`, operationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var threadIDs []string
	for rows.Next() {
		var threadID string
		if err := rows.Scan(&threadID); err != nil {
			return nil, err
		}
		threadIDs = append(threadIDs, threadID)
	}
	return threadIDs, rows.Err()
}

func softDeleteThreadsTx(tx *sql.Tx, operationID string, threadIDs []string, now time.Time) error {
	for _, threadID := range threadIDs {
		if _, err := tx.Exec(`
UPDATE threads SET deleted = 1, deleted_at = ?, delete_operation_id = ?, updated_at = ?
WHERE thread_id = ? AND deleted = 0
`, formatStoredTime(now), operationID, formatStoredTime(now), threadID); err != nil {
			return fmt.Errorf("soft delete thread %q: %w", threadID, err)
		}
	}
	return nil
}

func insertThreadCleanupJobsTx(tx *sql.Tx, operationID string, threadIDs []string, now time.Time) error {
	for _, threadID := range threadIDs {
		if _, err := tx.Exec(`
INSERT OR IGNORE INTO cleanup_jobs(job_id, operation_id, kind, target_id, status, next_attempt_at, created_at, updated_at)
VALUES(?, ?, 'thread_purge', ?, 'pending', ?, ?, ?)
`, newRuntimeID("job"), operationID, threadID, formatStoredTime(now), formatStoredTime(now), formatStoredTime(now)); err != nil {
			return fmt.Errorf("enqueue cleanup for thread %q: %w", threadID, err)
		}
	}
	return nil
}

func pendingCleanupCount(db projectDBTX, operationID string) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM cleanup_jobs WHERE operation_id = ? AND status <> 'succeeded' AND kind = 'thread_purge'`, operationID).Scan(&count)
	return count, err
}

func projectPathFingerprint(root string) (string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project path is not a directory")
	}
	identity, err := projectDirectoryIdentity(root, info)
	if err != nil {
		return "", err
	}
	return normalizeProjectWorkspaceRoot(root) + ":" + identity, nil
}

func projectFingerprintMatchesPath(fingerprint, originalRoot, candidate string) (bool, error) {
	prefix := normalizeProjectWorkspaceRoot(originalRoot) + ":"
	if !strings.HasPrefix(fingerprint, prefix) {
		return false, nil
	}
	info, err := os.Lstat(candidate)
	if err != nil {
		return false, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, nil
	}
	identity, err := projectDirectoryIdentity(candidate, info)
	if err != nil {
		return false, err
	}
	return strings.TrimPrefix(fingerprint, prefix) == identity, nil
}

func projectTrashStagingPaths(operation ProjectOperation) (string, string, error) {
	operationID := strings.TrimSpace(operation.OperationID)
	if operationID == "" || strings.ContainsAny(operationID, `/\\`) || operationID == "." || operationID == ".." {
		return "", "", ErrProjectOperationConflict
	}
	root := filepath.Clean(operation.WorkspaceRoot)
	base := filepath.Base(root)
	parent := filepath.Dir(root)
	if base == "." || base == string(filepath.Separator) || parent == root {
		return "", "", ErrProjectPathInvalid
	}
	container := filepath.Join(parent, ".awaken-trash-"+operationID)
	return container, filepath.Join(container, base), nil
}

func ensureTrashStagingContainer(container string) error {
	info, err := os.Lstat(container)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(container, 0o700); err != nil {
			return fmt.Errorf("create project trash staging directory: %w", err)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: trash staging path is not a private directory", ErrProjectPathChanged)
	}
	return nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func restoreStagedProject(operation ProjectOperation) error {
	container, stagedPath, err := projectTrashStagingPaths(operation)
	if err != nil {
		return err
	}
	if operation.TrashStagingPath != "" && filepath.Clean(operation.TrashStagingPath) != filepath.Clean(stagedPath) {
		return ErrProjectOperationConflict
	}
	stagedExists, err := pathExists(stagedPath)
	if err != nil {
		return err
	}
	if !stagedExists {
		matches, matchErr := projectFingerprintMatchesPath(operation.PathFingerprint, operation.WorkspaceRoot, operation.WorkspaceRoot)
		if matchErr != nil || !matches {
			return fmt.Errorf("%w: original project cannot be safely restored", ErrProjectPathChanged)
		}
		_ = os.Remove(container)
		return nil
	}
	matches, matchErr := projectFingerprintMatchesPath(operation.PathFingerprint, operation.WorkspaceRoot, stagedPath)
	if matchErr != nil || !matches {
		return ErrProjectPathChanged
	}
	originalExists, err := pathExists(operation.WorkspaceRoot)
	if err != nil {
		return err
	}
	if originalExists {
		return fmt.Errorf("%w: original project path is occupied", ErrProjectPathChanged)
	}
	if err := os.Rename(stagedPath, operation.WorkspaceRoot); err != nil {
		return fmt.Errorf("restore project after trash failure: %w", err)
	}
	matches, matchErr = projectFingerprintMatchesPath(operation.PathFingerprint, operation.WorkspaceRoot, operation.WorkspaceRoot)
	if matchErr != nil || !matches {
		return fmt.Errorf("%w: restored project identity changed", ErrProjectPathChanged)
	}
	_ = os.Remove(container)
	return nil
}

func selectFallbackWorkspaceTx(tx *sql.Tx, excludedProjectID string, now time.Time) error {
	var fallback string
	err := tx.QueryRow(`
SELECT workspace_root FROM projects
WHERE state = 'active' AND project_id <> ?
ORDER BY updated_at DESC, created_at DESC LIMIT 1
`, excludedProjectID).Scan(&fallback)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.Exec(`DELETE FROM desktop_state WHERE key = 'last_workspace_path'`)
		return err
	}
	if err != nil {
		return fmt.Errorf("select fallback workspace: %w", err)
	}
	_, err = tx.Exec(`
INSERT INTO desktop_state(key, value, updated_at) VALUES('last_workspace_path', ?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
`, fallback, formatStoredTime(now))
	if err != nil {
		return fmt.Errorf("persist fallback workspace: %w", err)
	}
	return nil
}

func validateTrashProjectPath(root, userDataRoot string) error {
	root = normalizeProjectWorkspaceRoot(root)
	if root == "" || !filepath.IsAbs(root) {
		return ErrProjectPathInvalid
	}
	volumeRoot := filepath.VolumeName(root) + string(filepath.Separator)
	if filepath.Clean(root) == filepath.Clean(volumeRoot) {
		return fmt.Errorf("%w: filesystem root is protected", ErrProjectPathInvalid)
	}
	protected := []string{normalizeProjectWorkspaceRoot(userDataRoot)}
	if home, err := os.UserHomeDir(); err == nil {
		protected = append(protected, normalizeProjectWorkspaceRoot(home))
	}
	if executable, err := os.Executable(); err == nil {
		protected = append(protected, normalizeProjectWorkspaceRoot(executable))
	}
	for _, candidate := range protected {
		if candidate == "" {
			continue
		}
		if root == candidate || pathContains(root, candidate) {
			return fmt.Errorf("%w: protected path %q", ErrProjectPathInvalid, root)
		}
	}
	return nil
}

func pathContains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	if err != nil || relative == "." {
		return relative == "."
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
