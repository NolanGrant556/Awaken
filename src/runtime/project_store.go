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

type Project struct {
	ProjectID     string       `json:"project_id"`
	WorkspaceRoot string       `json:"workspace_root"`
	DisplayName   string       `json:"display_name"`
	State         ProjectState `json:"state"`
	RemovalReason string       `json:"removal_reason,omitempty"`
	RemovedAt     *time.Time   `json:"removed_at,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
	Version       int64        `json:"version"`
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
	WorkspaceRoot        string                 `json:"workspace_root,omitempty"`
	PathFingerprint      string                 `json:"path_fingerprint,omitempty"`
	AttemptID            string                 `json:"attempt_id,omitempty"`
	TrashStagingPath     string                 `json:"trash_staging_path,omitempty"`
	MatchedThreads       int                    `json:"matched_threads"`
	PendingCleanup       int                    `json:"pending_cleanup"`
	LogicalCommittedAt   *time.Time             `json:"logical_committed_at,omitempty"`
	PhysicalCompletedAt  *time.Time             `json:"physical_completed_at,omitempty"`
	TrashInvokedAt       *time.Time             `json:"trash_invoked_at,omitempty"`
	ErrorCode            string                 `json:"error_code,omitempty"`
	ErrorMessage         string                 `json:"error_message,omitempty"`
	CreatedAt            time.Time              `json:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at"`
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
		WorkspaceRoot:        project.WorkspaceRoot,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}
