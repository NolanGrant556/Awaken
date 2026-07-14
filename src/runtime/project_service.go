package runtime

import (
	"context"
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

// ProjectService owns the project lifecycle state machine, filesystem staging
// and run-service coordination. Durable state changes go through the
// ProjectLifecycleRepository domain commands; the service never touches the
// storage engine directly.
type ProjectService struct {
	runService *RunService
	store      ProjectLifecycleRepository
	wake       chan struct{}
}

func newProjectService(runService *RunService, store ProjectLifecycleRepository) *ProjectService {
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
	state, found, err := s.store.ProjectStateForWorkspace(root)
	if err != nil {
		return false, err
	}
	if !found {
		return true, nil
	}
	return state == ProjectStateActive, nil
}

func (s *ProjectService) ListProjects(includeRemoved bool) ([]Project, error) {
	return s.store.ListProjects(includeRemoved)
}

func (s *ProjectService) UpsertProject(workspaceRoot, displayName string) (Project, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	root := normalizeProjectWorkspaceRoot(workspaceRoot)
	if root == "" {
		return Project{}, ErrProjectPathInvalid
	}
	existingState, found, err := s.store.ProjectStateForWorkspace(root)
	if err != nil {
		return Project{}, fmt.Errorf("read project before upsert: %w", err)
	}
	if found && existingState == ProjectStateTrashPending {
		return Project{}, ErrProjectOperationConflict
	}
	project, err := s.store.EnsureProjectActive(root, displayName, time.Now().UTC())
	if err != nil {
		return Project{}, err
	}
	s.runService.setProjectDeletionFence(root, false)
	return project, nil
}

func (s *ProjectService) DetachProject(projectID, idempotencyKey string) (ProjectOperation, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	if err := requireIdempotencyKey(idempotencyKey); err != nil {
		return ProjectOperation{}, err
	}
	if existing, found, err := s.store.LookupIdempotentOperation(idempotencyKey, projectID, ProjectOperationDetach); err != nil || found {
		return existing, err
	}
	operation, err := s.store.CommitDetach(projectID, idempotencyKey, time.Now().UTC())
	if err != nil {
		return ProjectOperation{}, err
	}
	s.runService.setProjectDeletionFence(operation.WorkspaceRoot, true)
	s.runService.cancelRunsForThreads(s.runService.threadIDsForWorkspace(operation.WorkspaceRoot))
	return operation, nil
}

func (s *ProjectService) DeleteProjectThreads(projectID, idempotencyKey string) (ProjectOperation, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	if err := requireIdempotencyKey(idempotencyKey); err != nil {
		return ProjectOperation{}, err
	}
	if existing, found, err := s.store.LookupIdempotentOperation(idempotencyKey, projectID, ProjectOperationDeleteHistory); err != nil || found {
		return existing, err
	}
	project, err := s.store.ProjectByID(projectID)
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
	operation, threadIDs, err := s.store.CommitDeleteHistory(project, idempotencyKey, time.Now().UTC())
	if err != nil {
		return ProjectOperation{}, err
	}
	committed = true
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
	projectID, deleted, found, err := s.store.ThreadDeletionState(threadID)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if deleted {
		s.Wake()
		return nil
	}
	project, err := s.store.ProjectByID(projectID)
	if err != nil {
		return err
	}
	if project.State == ProjectStateTrashPending {
		return ErrProjectOperationConflict
	}
	idempotencyKey := "thread-delete:" + threadID
	if _, found, err := s.store.LookupIdempotentOperation(idempotencyKey, projectID, ProjectOperationDeleteThread); err != nil {
		return err
	} else if found {
		s.Wake()
		return nil
	}
	if _, err := s.store.CommitDeleteThread(project, threadID, idempotencyKey, time.Now().UTC()); err != nil {
		return err
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
	if existing, found, err := s.store.LookupIdempotentOperation(idempotencyKey, projectID, ProjectOperationTrashAndPurge); err != nil || found {
		if found && (existing.Status == ProjectOperationAccepted || existing.Status == ProjectOperationRunning) {
			s.runService.setProjectDeletionFence(existing.WorkspaceRoot, true)
		}
		return existing, err
	}
	project, err := s.store.ProjectByID(projectID)
	if err != nil {
		return ProjectOperation{}, err
	}
	if project.State == ProjectStateTrashPending {
		return ProjectOperation{}, ErrProjectOperationConflict
	}
	if err := validateTrashProjectPath(project.WorkspaceRoot, s.store.UserDataRoot()); err != nil {
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
	operation, threadIDs, err := s.store.CommitPrepareTrash(project, idempotencyKey, fingerprint, time.Now().UTC())
	if err != nil {
		return ProjectOperation{}, err
	}
	committed = true
	s.runService.cancelRunsForThreads(threadIDs)
	return operation, nil
}

func (s *ProjectService) GetOperation(operationID string) (ProjectOperation, error) {
	operation, err := s.store.OperationByID(operationID)
	if err != nil {
		return ProjectOperation{}, err
	}
	operation.PendingCleanup, _ = s.store.PendingCleanupCount(operation.OperationID)
	return operation, nil
}

func (s *ProjectService) ListPendingTrashOperations() ([]ProjectOperation, error) {
	return s.store.ListPendingTrashOperations()
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
	operation, err := s.store.OperationByID(operationID)
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
	threadIDs, err := s.store.OperationThreadIDs(operationID)
	if err != nil {
		return ProjectOperation{}, err
	}
	if s.runService.anyThreadActive(threadIDs) {
		return ProjectOperation{}, ErrProjectRunsBusy
	}
	if err := s.store.ClaimTrash(operation, workerID, lease, time.Now().UTC()); err != nil {
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
	operation, err := s.store.OperationByID(operationID)
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
				if err := s.store.PersistTrashStagingPath(operationID, attemptID, stagedPath, time.Now().UTC()); err != nil {
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

	nextPhase := projectOperationPhaseTrashStaged
	if operation.Phase == projectOperationPhaseTrashing {
		nextPhase = projectOperationPhaseTrashing
	}
	if err := s.store.MarkTrashStaged(operationID, attemptID, nextPhase, stagedPath, time.Now().UTC()); err != nil {
		return ProjectOperation{}, err
	}
	return s.GetOperation(operationID)
}

func (s *ProjectService) AttestTrashOperationPath(operationID, attemptID string) (ProjectOperation, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	operation, err := s.store.OperationByID(operationID)
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
	operation, err := s.store.OperationByID(operationID)
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
	if err := s.store.CommitBeginTrash(operationID, attemptID, time.Now().UTC()); err != nil {
		return ProjectOperation{}, err
	}
	return s.GetOperation(operationID)
}

func (s *ProjectService) CompleteTrashOperation(operationID, attemptID string) (ProjectOperation, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	operation, err := s.store.OperationByID(operationID)
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
	threadIDs, err := s.store.OperationThreadIDs(operationID)
	if err != nil {
		return ProjectOperation{}, err
	}
	if err := s.store.CommitCompleteTrash(operation, threadIDs, time.Now().UTC()); err != nil {
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
	operation, err := s.store.OperationByID(operationID)
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
	if err := s.store.CommitTrashFailure(operation, status, phase, code, message, time.Now().UTC()); err != nil {
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
	jobs, err := s.store.ClaimThreadCleanupJobs(limit, 30*time.Second, time.Now().UTC())
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
			if finalizeErr := s.store.CompleteThreadCleanupJob(job, time.Now().UTC()); finalizeErr != nil {
				err = finalizeErr
			}
		}
		if err != nil {
			if retryErr := s.store.RetryCleanupJob(job, err, time.Now().UTC()); retryErr != nil {
				return processed, retryErr
			}
			continue
		}
		processed++
	}
	return processed, nil
}

func (s *ProjectService) purgeThread(threadID string) error {
	if s.runService.isThreadActive(threadID) {
		return ErrThreadBusy
	}
	if err := s.runService.closeAndEvictDeletedThread(threadID); err != nil {
		return err
	}
	if err := deleteThreadTranscriptAtPath(s.store.ThreadTranscriptPath(threadID)); err != nil && !errors.Is(err, ErrThreadNotFound) {
		return err
	}
	if err := deleteThreadInputAttachments(threadID); err != nil && !errors.Is(err, ErrThreadNotFound) {
		return err
	}
	return nil
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
