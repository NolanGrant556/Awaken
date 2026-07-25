package runtime

import "time"

// ThreadRepository is the FileStore seam for durable thread metadata and
// transcripts. Callers depend on the contract rather than the concrete file
// layout so lifecycle and replay ownership remain explicit.
type ThreadRepository interface {
	ThreadStore
	Prepare() error
	ThreadTranscriptPath(threadID string) string
}

// ProjectRepository owns durable project rows and the project operation state
// machine. Every Commit* method is a single atomic storage transaction; the
// caller (ProjectService) owns validation ordering, filesystem staging and
// run-service side effects, and serializes mutations through the run service
// project mutation lock.
type ProjectRepository interface {
	ListProjects(includeRemoved bool) ([]Project, error)
	ProjectByID(projectID string) (Project, error)
	ThreadDeletionState(threadID string) (projectID string, deleted bool, found bool, err error)
	LookupIdempotentOperation(key, projectID string, action ProjectOperationAction) (ProjectOperation, bool, error)
	OperationByID(operationID string) (ProjectOperation, error)
	PendingCleanupCount(operationID string) (int, error)
	ListPendingTrashOperations() ([]ProjectOperation, error)
	OperationThreadIDs(operationID string) ([]string, error)

	// ProjectsHoldingFolder answers which projects a folder is attached to. A
	// folder may be shared, so the answer is a list and the caller decides what
	// a second holder means for the operation it is about to run.
	ProjectsHoldingFolder(root string) ([]Project, error)

	EnsureProjectActive(root, displayName string, now time.Time) (Project, error)
	CreateProject(folders []ProjectFolder, displayName string, now time.Time) (Project, error)
	AttachProjectFolder(projectID, path string, now time.Time) (Project, error)
	DetachProjectFolder(projectID, path string, now time.Time) (Project, error)
	SetPrimaryProjectFolder(projectID, path string, now time.Time) (Project, error)
	ReorderProjects(projectIDs []string, now time.Time) ([]Project, error)
	CommitDetach(projectID, idempotencyKey string, now time.Time) (ProjectOperation, error)
	CommitDeleteHistory(project Project, idempotencyKey string, now time.Time) (ProjectOperation, []string, error)
	CommitDeleteThread(project Project, threadID, idempotencyKey string, now time.Time) (ProjectOperation, error)
	CommitPrepareTrash(project Project, folderPath, idempotencyKey, fingerprint string, now time.Time) (ProjectOperation, []string, error)
	ClaimTrash(operation ProjectOperation, workerID string, lease time.Duration, now time.Time) error
	PersistTrashStagingPath(operationID, attemptID, stagedPath string, now time.Time) error
	MarkTrashStaged(operationID, attemptID, phase, stagedPath string, now time.Time) error
	CommitBeginTrash(operationID, attemptID string, now time.Time) error
	CommitCompleteTrash(operation ProjectOperation, now time.Time) error
	CommitTrashFailure(operation ProjectOperation, status ProjectOperationStatus, phase, code, message string, now time.Time) error
}

// CleanupJobStore owns the durable cleanup outbox used by the background
// worker to physically purge deleted threads after their logical commit.
type CleanupJobStore interface {
	ClaimThreadCleanupJobs(limit int, lease time.Duration, now time.Time) ([]cleanupJob, error)
	CompleteThreadCleanupJob(job cleanupJob, now time.Time) error
	RetryCleanupJob(job cleanupJob, cause error, now time.Time) error
}

// ProjectLifecycleRepository is the full storage capability ProjectService
// consumes. RunService registers a ProjectService only when its thread store
// also satisfies this contract; no concrete store type checks remain.
type ProjectLifecycleRepository interface {
	ProjectRepository
	CleanupJobStore
	ThreadTranscriptPath(threadID string) string
	UserDataRoot() string
}
