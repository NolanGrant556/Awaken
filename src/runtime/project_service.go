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
	ErrProjectOrderInvalid      = errors.New("project order must contain every active project exactly once")
	ErrProjectNameInvalid       = errors.New("project name is required")

	ErrProjectFolderNotFound = errors.New("folder is not attached to this project")
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
	case errors.Is(err, ErrProjectOrderInvalid):
		return "PROJECT_ORDER_INVALID"
	case errors.Is(err, ErrProjectNameInvalid):
		return "PROJECT_NAME_INVALID"
	case errors.Is(err, ErrProjectFolderNotFound):
		return "PROJECT_FOLDER_NOT_FOUND"
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

// CanStartRun answers whether the folder a run is anchored on is still usable.
// A shared folder is judged as one directory rather than per project: any holder
// on its way to the trash is about to take the directory away from all of them,
// so that blocks everyone, while a single active holder is enough to keep the
// folder open. A folder no project holds is not this gate's business.
func (s *ProjectService) CanStartRun(workspaceRoot string) (bool, error) {
	root := normalizeProjectWorkspaceRoot(workspaceRoot)
	if root == "" {
		return false, ErrProjectPathInvalid
	}
	holders, err := s.store.ProjectsHoldingFolder(root)
	if err != nil {
		return false, err
	}
	if len(holders) == 0 {
		return true, nil
	}
	runnable := false
	for _, holder := range holders {
		if holder.State == ProjectStateTrashPending {
			return false, nil
		}
		if holder.State == ProjectStateActive {
			runnable = true
		}
	}
	return runnable, nil
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
	holders, err := s.store.ProjectsHoldingFolder(root)
	if err != nil {
		return Project{}, fmt.Errorf("read project before upsert: %w", err)
	}
	for _, holder := range holders {
		if holder.State == ProjectStateTrashPending {
			return Project{}, ErrProjectOperationConflict
		}
	}
	project, err := s.store.EnsureProjectActive(root, displayName, time.Now().UTC())
	if err != nil {
		return Project{}, err
	}
	s.runService.clearProjectDeletionFences(root)
	return project, nil
}

// CreateProject mints a project from an explicit folder list. Passing no folder
// is legal: the project is then backed by its Awaken-managed working directory
// and stays runnable, which is what keeps "a project without folders" a real
// product state rather than a broken one.
//
// Folders are not owned exclusively. The same directory may be part of several
// projects, because which pieces of work a directory belongs to is the user's
// call, not something the store gets to arbitrate.
func (s *ProjectService) CreateProject(folders []ProjectFolder, displayName string) (Project, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()

	prepared := make([]ProjectFolder, 0, len(folders))
	seen := make(map[string]struct{}, len(folders))
	primaryAssigned := false
	for _, folder := range folders {
		root := normalizeProjectWorkspaceRoot(folder.Path)
		if root == "" || !filepath.IsAbs(root) {
			return Project{}, ErrProjectPathInvalid
		}
		if _, duplicate := seen[root]; duplicate {
			continue
		}
		seen[root] = struct{}{}
		isPrimary := folder.IsPrimary && !primaryAssigned
		if isPrimary {
			primaryAssigned = true
		}
		prepared = append(prepared, ProjectFolder{
			Path:      root,
			Role:      ProjectFolderRoleUser,
			IsPrimary: isPrimary,
		})
	}
	if len(prepared) > 0 && !primaryAssigned {
		prepared[0].IsPrimary = true
	}

	project, err := s.store.CreateProject(prepared, displayName, time.Now().UTC())
	if err != nil {
		return Project{}, err
	}
	for _, root := range project.AllRoots() {
		s.runService.clearProjectDeletionFences(root)
	}
	return project, nil
}

// RenameProject changes only the user-visible name of a stable project. It is
// not an operation: there is no external side effect to coordinate and callers
// do not need an idempotency key. The repository keeps equal-name retries a
// no-op and rejects a project while its folder is being moved to the trash.
func (s *ProjectService) RenameProject(projectID, displayName string) (Project, error) {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return Project{}, ErrProjectNameInvalid
	}
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	return s.store.RenameProject(projectID, displayName, time.Now().UTC())
}

// AttachProjectFolder adds one more readable/writable folder to a project. The
// folder may already be attached to other projects.
func (s *ProjectService) AttachProjectFolder(projectID, path string) (Project, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	root := normalizeProjectWorkspaceRoot(path)
	if root == "" || !filepath.IsAbs(root) {
		return Project{}, ErrProjectPathInvalid
	}
	project, err := s.store.AttachProjectFolder(projectID, root, time.Now().UTC())
	if err != nil {
		return Project{}, err
	}
	s.runService.clearProjectDeletionFences(root)
	return project, nil
}

// DetachProjectFolder unbinds a folder from the project without touching the
// directory on disk.
func (s *ProjectService) DetachProjectFolder(projectID, path string) (Project, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	project, err := s.store.DetachProjectFolder(projectID, path, time.Now().UTC())
	if err != nil {
		return Project{}, err
	}
	// Only this project's threads on that folder end; another project sharing it
	// keeps working there, which is also why the folder is fenced off only once
	// nothing holds it anymore.
	s.runService.cancelRunsForThreads(s.runService.threadIDsForProjectFolder(project.ProjectID, path))
	remaining, err := s.store.ProjectsHoldingFolder(path)
	if err != nil {
		return Project{}, err
	}
	if len(remaining) == 0 {
		s.runService.setProjectDeletionFence(project.ProjectID, path, true)
	}
	return project, nil
}

// SetPrimaryProjectFolder moves the working directory and project-level
// discovery anchor onto another attached folder.
func (s *ProjectService) SetPrimaryProjectFolder(projectID, path string) (Project, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	return s.store.SetPrimaryProjectFolder(projectID, path, time.Now().UTC())
}

// fenceProjectRoots blocks or unblocks run creation for every folder of a
// project. A project-scoped removal must fence all of its roots, not just the
// primary one.
func (s *ProjectService) fenceProjectRoots(projectID string, fenced bool) {
	project, err := s.store.ProjectByID(projectID)
	if err != nil {
		return
	}
	for _, root := range project.AllRoots() {
		// Raising a fence on a folder another live project holds would stop work
		// that has nothing to do with this operation: removing a project or
		// deleting its history never touches the disk. Trashing does, and fences
		// the shared folder for its holders explicitly. Releasing is never
		// filtered, so a fence raised while the folder was still exclusive
		// cannot be left behind.
		if fenced && s.folderHasOtherLiveHolder(project.ProjectID, root) {
			continue
		}
		s.runService.setProjectDeletionFence(project.ProjectID, root, fenced)
	}
}

func (s *ProjectService) folderHasOtherLiveHolder(projectID, root string) bool {
	holders, err := s.store.ProjectsHoldingFolder(root)
	if err != nil {
		return false
	}
	for _, holder := range holders {
		if holder.ProjectID != projectID && holder.State != ProjectStateRemoved {
			return true
		}
	}
	return false
}

// ReorderProjects replaces the complete active-project order in one durable
// mutation. Requiring the full set prevents concurrent or stale clients from
// silently dropping a project from navigation.
func (s *ProjectService) ReorderProjects(projectIDs []string) ([]Project, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	return s.store.ReorderProjects(projectIDs, time.Now().UTC())
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
	s.fenceProjectRoots(operation.ProjectID, true)
	s.runService.cancelRunsForThreads(s.runService.threadIDsForProject(operation.ProjectID))
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
	s.fenceProjectRoots(project.ProjectID, true)
	committed := false
	defer func() {
		if !committed || project.State == ProjectStateActive {
			s.fenceProjectRoots(project.ProjectID, false)
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
	// A project-less conversation has no project lifecycle row to transact
	// against. It is still an ordinary durable thread, so evict its live state
	// and delete its FileStore record directly under the same mutation lock.
	if strings.TrimSpace(projectID) == "" {
		if s.runService.isThreadActive(threadID) {
			return ErrThreadBusy
		}
		if err := s.runService.closeAndEvictDeletedThread(threadID); err != nil {
			return err
		}
		return s.store.DeleteThread(threadID)
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

// PrepareTrash starts moving one attached folder to the system trash. Trash is
// folder-scoped: the project and its threads survive, only the directory leaves
// the disk and the folder leaves the project. The Awaken-managed working
// directory is never eligible, because it is owned by the project itself.
func (s *ProjectService) PrepareTrash(projectID, folderPath, idempotencyKey string) (ProjectOperation, error) {
	s.runService.lockProjectMutation()
	defer s.runService.unlockProjectMutation()
	if err := requireIdempotencyKey(idempotencyKey); err != nil {
		return ProjectOperation{}, err
	}
	if existing, found, err := s.store.LookupIdempotentOperation(idempotencyKey, projectID, ProjectOperationTrashAndPurge); err != nil || found {
		if found && (existing.Status == ProjectOperationAccepted || existing.Status == ProjectOperationRunning) {
			s.fenceProjectRoots(existing.ProjectID, true)
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
	folder, ok := project.FolderByPath(folderPath)
	if !ok {
		return ProjectOperation{}, ErrProjectFolderNotFound
	}
	if err := validateTrashProjectPath(folder.Path, s.store.UserDataRoot()); err != nil {
		return ProjectOperation{}, err
	}
	fingerprint, err := projectPathFingerprint(folder.Path)
	if err != nil {
		return ProjectOperation{}, fmt.Errorf("%w: %v", ErrProjectPathInvalid, err)
	}
	// The folder may be shared. Moving it to the trash takes the directory away
	// from every project holding it, so all of them are fenced off it, not just
	// the one that asked.
	coHolders, err := s.otherFolderHolders(project.ProjectID, folder.Path)
	if err != nil {
		return ProjectOperation{}, err
	}
	s.fenceProjectRoots(project.ProjectID, true)
	s.fenceCoHolders(coHolders, folder.Path, true)
	committed := false
	defer func() {
		if !committed {
			s.fenceProjectRoots(project.ProjectID, false)
			s.fenceCoHolders(coHolders, folder.Path, false)
		}
	}()
	operation, threadIDs, err := s.store.CommitPrepareTrash(project, folder.Path, idempotencyKey, fingerprint, time.Now().UTC())
	if err != nil {
		return ProjectOperation{}, err
	}
	committed = true
	// Threads of another holder anchored on this folder are stopped too: the
	// directory is about to leave the disk under them.
	s.runService.cancelRunsForThreads(threadIDs)
	s.runService.cancelRunsForThreads(s.runService.threadIDsForWorkspace(folder.Path))
	return operation, nil
}

// otherFolderHolders lists the projects sharing a folder with the one acting on
// it. It must be read before the folder is detached, because the holder list is
// derived from the folder lists that the operation is about to change.
func (s *ProjectService) otherFolderHolders(actingProjectID, folderPath string) ([]string, error) {
	holders, err := s.store.ProjectsHoldingFolder(folderPath)
	if err != nil {
		return nil, err
	}
	others := make([]string, 0, len(holders))
	for _, holder := range holders {
		if holder.ProjectID == actingProjectID {
			continue
		}
		others = append(others, holder.ProjectID)
	}
	return others, nil
}

// fenceCoHolders fences exactly the shared folder for the other holders, rather
// than all of their roots: their remaining folders are untouched by this trash.
func (s *ProjectService) fenceCoHolders(projectIDs []string, folderPath string, fenced bool) {
	for _, projectID := range projectIDs {
		s.runService.setProjectDeletionFence(projectID, folderPath, fenced)
	}
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
	// Read the co-holders before the commit detaches the folder from them.
	coHolders, err := s.otherFolderHolders(operation.ProjectID, operation.WorkspaceRoot)
	if err != nil {
		return ProjectOperation{}, err
	}
	if err := s.store.CommitCompleteTrash(operation, time.Now().UTC()); err != nil {
		return ProjectOperation{}, err
	}
	// The folder is gone but the projects stay; run creation must be allowed
	// again on the roots they still hold. The trashed path itself stays fenced.
	s.fenceProjectRoots(operation.ProjectID, false)
	s.fenceCoHolders(coHolders, operation.WorkspaceRoot, false)
	s.runService.setProjectDeletionFence(operation.ProjectID, operation.WorkspaceRoot, true)
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
	// The directory is back where it was, so the fence this operation put on the
	// other holders is released whatever happens to the acting project.
	coHolders, err := s.otherFolderHolders(operation.ProjectID, operation.WorkspaceRoot)
	if err != nil {
		return ProjectOperation{}, err
	}
	if err := s.store.CommitTrashFailure(operation, status, phase, code, message, time.Now().UTC()); err != nil {
		return ProjectOperation{}, err
	}
	s.fenceCoHolders(coHolders, operation.WorkspaceRoot, false)
	if operation.PreviousProjectState == ProjectStateActive {
		s.fenceProjectRoots(operation.ProjectID, false)
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
	if err := deleteThreadToolResults(threadID); err != nil && !errors.Is(err, ErrThreadNotFound) {
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
