package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"
)

type ProjectState string

const (
	ProjectStateActive       ProjectState = "active"
	ProjectStateRemoved      ProjectState = "removed"
	ProjectStateTrashPending ProjectState = "trash_pending"
)

// ProjectFolderRole records where a folder came from. Every folder Awaken
// attaches today is one the user picked, so the role carries no behaviour of its
// own; `managed` survives only so a value persisted earlier decodes unchanged
// instead of being silently rewritten.
type ProjectFolderRole string

const (
	ProjectFolderRoleUser    ProjectFolderRole = "user"
	ProjectFolderRoleManaged ProjectFolderRole = "managed"
)

// ProjectFolder is one attached root of a project. A project may have no folder
// at all. When it has folders, exactly one is primary: it owns the default
// working directory, relative path resolution, and project-level discovery of
// skills and memory. Secondary folders are readable, writable and searchable but
// carry no discovery.
type ProjectFolder struct {
	Path      string            `json:"path"`
	Role      ProjectFolderRole `json:"role"`
	IsPrimary bool              `json:"is_primary"`
}

type Project struct {
	ProjectID string `json:"project_id"`
	// WorkspaceRoot mirrors the primary folder path. It stays in the schema
	// because threads, operations and the desktop wire contract already key on
	// it; Folders is the authoritative list.
	WorkspaceRoot string          `json:"workspace_root"`
	Folders       []ProjectFolder `json:"folders,omitempty"`
	DisplayName   string          `json:"display_name"`
	State         ProjectState    `json:"state"`
	RemovalReason string          `json:"removal_reason,omitempty"`
	RemovedAt     *time.Time      `json:"removed_at,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	Version       int64           `json:"version"`
}

type ProjectOperationAction string

const (
	ProjectOperationDetach        ProjectOperationAction = "detach"
	ProjectOperationDeleteHistory ProjectOperationAction = "delete_history"
	ProjectOperationTrashAndPurge ProjectOperationAction = "trash_and_purge"
	ProjectOperationDeleteThread  ProjectOperationAction = "delete_thread"
)

type ProjectOperationStatus string

const (
	ProjectOperationAccepted  ProjectOperationStatus = "accepted"
	ProjectOperationRunning   ProjectOperationStatus = "running"
	ProjectOperationSucceeded ProjectOperationStatus = "succeeded"
	ProjectOperationFailed    ProjectOperationStatus = "failed"
	ProjectOperationBlocked   ProjectOperationStatus = "blocked"
)

const (
	projectOperationPhaseCommitted      = "committed"
	projectOperationPhaseCleaning       = "cleaning"
	projectOperationPhaseWaitingForRuns = "waiting_for_runs"
	projectOperationPhaseTrashClaimed   = "trash_claimed"
	projectOperationPhaseTrashStaged    = "trash_staged"
	projectOperationPhaseTrashing       = "trashing"
	projectOperationPhaseCompleted      = "completed"
	projectOperationPhaseFailed         = "failed"
	projectOperationPhaseCancelled      = "cancelled"
)

type ProjectOperation struct {
	OperationID          string                 `json:"operation_id"`
	IdempotencyKey       string                 `json:"idempotency_key"`
	ProjectID            string                 `json:"project_id,omitempty"`
	Action               ProjectOperationAction `json:"action"`
	Status               ProjectOperationStatus `json:"status"`
	Phase                string                 `json:"phase"`
	PreviousProjectState ProjectState           `json:"previous_project_state,omitempty"`
	// WorkspaceRoot is the filesystem path this operation targets. For
	// project-scoped actions it is the project's primary folder; for
	// trash_and_purge it is the single folder being moved to the system trash,
	// which is what the staging, fingerprint and restore steps operate on.
	WorkspaceRoot       string     `json:"workspace_root,omitempty"`
	PathFingerprint     string     `json:"path_fingerprint,omitempty"`
	AttemptID           string     `json:"attempt_id,omitempty"`
	TrashStagingPath    string     `json:"trash_staging_path,omitempty"`
	MatchedThreads      int        `json:"matched_threads"`
	PendingCleanup      int        `json:"pending_cleanup"`
	LogicalCommittedAt  *time.Time `json:"logical_committed_at,omitempty"`
	PhysicalCompletedAt *time.Time `json:"physical_completed_at,omitempty"`
	TrashInvokedAt      *time.Time `json:"trash_invoked_at,omitempty"`
	ErrorCode           string     `json:"error_code,omitempty"`
	ErrorMessage        string     `json:"error_message,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type cleanupJob struct {
	JobID          string
	OperationID    string
	Kind           string
	TargetID       string
	Status         string
	Attempts       int
	NextAttemptAt  time.Time
	LeaseOwner     string
	LeaseExpiresAt time.Time
}

const (
	cleanupJobThreadPurge  = "thread_purge"
	cleanupJobProjectTrash = "project_trash"
	cleanupJobPending      = "pending"
	cleanupJobLeased       = "leased"
	cleanupJobRetry        = "retry"
	cleanupJobSucceeded    = "succeeded"
	cleanupJobBlocked      = "blocked"
)

func normalizeProjectWorkspaceRoot(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	root = filepath.Clean(root)
	if goruntime.GOOS == "windows" {
		root = strings.ToLower(root)
	}
	return root
}

func stableProjectID(root string) string {
	normalized := normalizeProjectWorkspaceRoot(root)
	sum := sha256.Sum256([]byte(normalized))
	return "project_" + hex.EncodeToString(sum[:12])
}

// newProjectID mints an identity that is independent of any path. A project can
// hold several folders and can change its primary, so identity must not be
// derived from a path the way single-folder projects used to be. Existing
// path-derived IDs stay valid and are never rewritten.
func newProjectID() string {
	return newRuntimeID("project")
}

// PrimaryFolder returns the folder that owns the working directory and all
// project-level discovery.
func (p Project) PrimaryFolder() (ProjectFolder, bool) {
	for _, folder := range p.Folders {
		if folder.IsPrimary {
			return folder, true
		}
	}
	return ProjectFolder{}, false
}

// PrimaryRoot is the primary folder path, falling back to the mirrored
// WorkspaceRoot so callers reading legacy rows still resolve a root.
func (p Project) PrimaryRoot() string {
	if folder, ok := p.PrimaryFolder(); ok {
		return folder.Path
	}
	return normalizeProjectWorkspaceRoot(p.WorkspaceRoot)
}

// SecondaryRoots lists every attached folder except the primary, in attach
// order. They are readable, writable and searchable.
func (p Project) SecondaryRoots() []string {
	roots := make([]string, 0, len(p.Folders))
	for _, folder := range p.Folders {
		if folder.IsPrimary {
			continue
		}
		roots = append(roots, folder.Path)
	}
	return roots
}

// AllRoots lists every attached folder with the primary first.
func (p Project) AllRoots() []string {
	primary := p.PrimaryRoot()
	roots := make([]string, 0, len(p.Folders)+1)
	if primary != "" {
		roots = append(roots, primary)
	}
	return append(roots, p.SecondaryRoots()...)
}

func (p Project) folderIndex(path string) int {
	normalized := normalizeProjectWorkspaceRoot(path)
	for index, folder := range p.Folders {
		if folder.Path == normalized {
			return index
		}
	}
	return -1
}

// HasFolder reports whether the path is already attached to this project.
func (p Project) HasFolder(path string) bool {
	return p.folderIndex(path) >= 0
}

// FolderByPath resolves an attached folder so callers can check its role before
// detaching or trashing it.
func (p Project) FolderByPath(path string) (ProjectFolder, bool) {
	index := p.folderIndex(path)
	if index < 0 {
		return ProjectFolder{}, false
	}
	return p.Folders[index], true
}

// withNormalizedFolders enforces the storage invariant that every project has a
// deduplicated folder list with exactly one primary, and that WorkspaceRoot
// mirrors that primary. Rows written before multi-folder support carry no
// Folders, so their WorkspaceRoot is promoted to a single user folder; this is
// what makes old snapshots and old journal records replay without migration.
func (p Project) withNormalizedFolders() Project {
	folders := make([]ProjectFolder, 0, len(p.Folders))
	seen := make(map[string]struct{}, len(p.Folders))
	primaryFound := false
	for _, folder := range p.Folders {
		normalized := normalizeProjectWorkspaceRoot(folder.Path)
		if normalized == "" || !filepath.IsAbs(normalized) {
			continue
		}
		if _, duplicate := seen[normalized]; duplicate {
			continue
		}
		seen[normalized] = struct{}{}
		role := folder.Role
		if role != ProjectFolderRoleManaged {
			role = ProjectFolderRoleUser
		}
		isPrimary := folder.IsPrimary && !primaryFound
		if isPrimary {
			primaryFound = true
		}
		folders = append(folders, ProjectFolder{Path: normalized, Role: role, IsPrimary: isPrimary})
	}

	if len(folders) == 0 {
		legacy := normalizeProjectWorkspaceRoot(p.WorkspaceRoot)
		if legacy != "" && filepath.IsAbs(legacy) {
			folders = append(folders, ProjectFolder{
				Path:      legacy,
				Role:      ProjectFolderRoleUser,
				IsPrimary: true,
			})
			primaryFound = true
		}
	}

	if !primaryFound && len(folders) > 0 {
		folders[0].IsPrimary = true
	}

	p.Folders = folders
	if len(folders) == 0 {
		p.WorkspaceRoot = ""
		return p
	}
	for _, folder := range folders {
		if folder.IsPrimary {
			p.WorkspaceRoot = folder.Path
			break
		}
	}
	return p
}

func projectDisplayName(root string) string {
	base := filepath.Base(filepath.Clean(root))
	if base == "." || base == string(filepath.Separator) || strings.TrimSpace(base) == "" {
		return root
	}
	return base
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
		WorkspaceRoot:        project.PrimaryRoot(),
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}

// newProjectFolderOperation targets one attached folder instead of the project's
// primary root. Trash is folder-scoped, so the staging and fingerprint steps
// must see the folder the user actually asked to delete.
func newProjectFolderOperation(project Project, folderPath, key string, action ProjectOperationAction, status ProjectOperationStatus, phase string, now time.Time) ProjectOperation {
	operation := newProjectOperation(project, key, action, status, phase, now)
	operation.WorkspaceRoot = normalizeProjectWorkspaceRoot(folderPath)
	return operation
}
