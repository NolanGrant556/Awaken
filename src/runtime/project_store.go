package runtime

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"awaken/src/desktopdata"
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

func ensureProjectLifecycleSchema(db *sql.DB, paths desktopdata.Paths) error {
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS projects (
  project_id TEXT PRIMARY KEY,
  workspace_root TEXT NOT NULL,
  workspace_key TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'active',
  removal_reason TEXT NOT NULL DEFAULT '',
  removed_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  version INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_projects_state_updated ON projects(state, updated_at DESC);
CREATE TABLE IF NOT EXISTS desktop_state (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS project_operations (
  operation_id TEXT PRIMARY KEY,
  idempotency_key TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL,
  status TEXT NOT NULL,
  phase TEXT NOT NULL,
  previous_project_state TEXT NOT NULL DEFAULT '',
  workspace_root TEXT NOT NULL DEFAULT '',
  path_fingerprint TEXT NOT NULL DEFAULT '',
  attempt_id TEXT NOT NULL DEFAULT '',
  trash_staging_path TEXT NOT NULL DEFAULT '',
  trash_invoked_at TEXT NOT NULL DEFAULT '',
  cutoff_at TEXT NOT NULL DEFAULT '',
  logical_committed_at TEXT NOT NULL DEFAULT '',
  physical_completed_at TEXT NOT NULL DEFAULT '',
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  result_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_project_operations_project ON project_operations(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_project_operations_pending ON project_operations(action, status, phase, updated_at);
CREATE TABLE IF NOT EXISTS project_operation_threads (
  operation_id TEXT NOT NULL,
  thread_id TEXT NOT NULL,
  PRIMARY KEY(operation_id, thread_id)
);
CREATE TABLE IF NOT EXISTS cleanup_jobs (
  job_id TEXT PRIMARY KEY,
  operation_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  target_id TEXT NOT NULL,
  payload_json TEXT NOT NULL DEFAULT '{}',
  status TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  next_attempt_at TEXT NOT NULL DEFAULT '',
  lease_owner TEXT NOT NULL DEFAULT '',
  lease_expires_at TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  completed_at TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_cleanup_thread_purge_unique
  ON cleanup_jobs(kind, target_id) WHERE kind = 'thread_purge';
CREATE INDEX IF NOT EXISTS idx_cleanup_jobs_due ON cleanup_jobs(kind, status, next_attempt_at);
CREATE INDEX IF NOT EXISTS idx_threads_project_live ON threads(project_id, deleted, updated_at DESC);
`); err != nil {
		return fmt.Errorf("initialize project lifecycle schema: %w", err)
	}
	if err := ensureProjectOperationColumns(db); err != nil {
		return err
	}

	var applied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 1`).Scan(&applied); err != nil {
		return fmt.Errorf("inspect project lifecycle migration: %w", err)
	}
	if applied == 0 {
		activeRoots, lastWorkspace := readLegacyWorkspaceState(paths)
		activeRootSet := make(map[string]struct{}, len(activeRoots))
		for _, root := range activeRoots {
			activeRootSet[normalizeProjectWorkspaceRoot(root)] = struct{}{}
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin project lifecycle migration: %w", err)
		}
		defer tx.Rollback()

		for _, root := range activeRoots {
			if _, err := ensureProjectForWorkspaceTx(tx, root, true); err != nil {
				return err
			}
		}
		rows, err := tx.Query(`SELECT DISTINCT workspace_root FROM threads WHERE TRIM(workspace_root) <> ''`)
		if err != nil {
			return fmt.Errorf("query thread workspaces for project migration: %w", err)
		}
		var threadRoots []string
		for rows.Next() {
			var root string
			if err := rows.Scan(&root); err != nil {
				rows.Close()
				return fmt.Errorf("scan thread workspace for project migration: %w", err)
			}
			threadRoots = append(threadRoots, root)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close project migration workspace rows: %w", err)
		}
		for _, root := range threadRoots {
			_, explicitlyActive := activeRootSet[normalizeProjectWorkspaceRoot(root)]
			projectID, err := ensureMigratedProjectForWorkspaceTx(tx, root, explicitlyActive)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE threads SET project_id = ? WHERE workspace_root = ? AND TRIM(project_id) = ''`, projectID, root); err != nil {
				return fmt.Errorf("backfill project id for workspace %q: %w", root, err)
			}
		}
		if normalized := normalizeProjectWorkspaceRoot(lastWorkspace); normalized != "" {
			if _, err := tx.Exec(`
INSERT INTO desktop_state(key, value, updated_at) VALUES('last_workspace_path', ?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
`, normalized, formatStoredTime(time.Now().UTC())); err != nil {
				return fmt.Errorf("migrate last workspace state: %w", err)
			}
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES(1, ?)`, formatStoredTime(time.Now().UTC())); err != nil {
			return fmt.Errorf("record project lifecycle migration: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit project lifecycle migration: %w", err)
		}
	}
	return ensureProjectForeignKeySchema(db, paths)
}

func ensureProjectOperationColumns(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(project_operations)`)
	if err != nil {
		return fmt.Errorf("inspect project operations schema: %w", err)
	}
	existing := map[string]struct{}{}
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &primaryKey); err != nil {
			rows.Close()
			return fmt.Errorf("scan project operations schema: %w", err)
		}
		existing[name] = struct{}{}
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close project operations schema rows: %w", err)
	}
	for _, column := range []struct {
		name string
		ddl  string
	}{
		{name: "attempt_id", ddl: "attempt_id TEXT NOT NULL DEFAULT ''"},
		{name: "trash_staging_path", ddl: "trash_staging_path TEXT NOT NULL DEFAULT ''"},
		{name: "trash_invoked_at", ddl: "trash_invoked_at TEXT NOT NULL DEFAULT ''"},
	} {
		if _, ok := existing[column.name]; ok {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE project_operations ADD COLUMN ` + column.ddl); err != nil {
			return fmt.Errorf("migrate project operations schema add %s: %w", column.name, err)
		}
	}
	return nil
}

const projectForeignKeySchemaVersion = 2

type projectForeignKeySpec struct {
	table      string
	fromColumn string
	toTable    string
	toColumn   string
	onUpdate   string
	onDelete   string
}

var requiredProjectForeignKeys = []projectForeignKeySpec{
	{table: "threads", fromColumn: "project_id", toTable: "projects", toColumn: "project_id", onUpdate: "CASCADE", onDelete: "RESTRICT"},
	{table: "project_operations", fromColumn: "project_id", toTable: "projects", toColumn: "project_id", onUpdate: "CASCADE", onDelete: "RESTRICT"},
	{table: "project_operation_threads", fromColumn: "operation_id", toTable: "project_operations", toColumn: "operation_id", onUpdate: "CASCADE", onDelete: "CASCADE"},
	{table: "project_operation_threads", fromColumn: "thread_id", toTable: "threads", toColumn: "thread_id", onUpdate: "CASCADE", onDelete: "CASCADE"},
	{table: "cleanup_jobs", fromColumn: "operation_id", toTable: "project_operations", toColumn: "operation_id", onUpdate: "CASCADE", onDelete: "CASCADE"},
}

// ensureProjectForeignKeySchema upgrades databases created before the unified
// DesktopStore had relational constraints. Legacy orphan metadata is repaired
// with hidden recovery parents so migration never drops a thread, operation,
// cutoff row, or cleanup job merely to satisfy a new foreign key.
func ensureProjectForeignKeySchema(db *sql.DB, paths desktopdata.Paths) error {
	var applied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, projectForeignKeySchemaVersion).Scan(&applied); err != nil {
		return fmt.Errorf("inspect project foreign-key migration: %w", err)
	}
	if applied != 0 {
		return verifyProjectForeignKeySchema(db)
	}

	if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("disable foreign keys for project schema migration: %w", err)
	}
	migrationErr := func() error {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin project foreign-key migration: %w", err)
		}
		defer tx.Rollback()

		if err := repairLegacyProjectRelationsTx(tx, paths); err != nil {
			return err
		}
		if err := rebuildProjectRelationTablesTx(tx); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`, projectForeignKeySchemaVersion, formatStoredTime(time.Now().UTC())); err != nil {
			return fmt.Errorf("record project foreign-key migration: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit project foreign-key migration: %w", err)
		}
		return nil
	}()
	_, enableErr := db.Exec(`PRAGMA foreign_keys = ON`)
	if migrationErr != nil {
		if enableErr != nil {
			return fmt.Errorf("%v; additionally failed to re-enable foreign keys: %w", migrationErr, enableErr)
		}
		return migrationErr
	}
	if enableErr != nil {
		return fmt.Errorf("re-enable foreign keys after project schema migration: %w", enableErr)
	}
	return verifyProjectForeignKeySchema(db)
}

func repairLegacyProjectRelationsTx(tx *sql.Tx, paths desktopdata.Paths) error {
	type threadProjectGap struct {
		threadID      string
		workspaceRoot string
	}
	rows, err := tx.Query(`
SELECT t.thread_id, t.workspace_root
FROM threads t
LEFT JOIN projects p ON p.project_id = t.project_id
WHERE TRIM(t.project_id) = '' OR p.project_id IS NULL`)
	if err != nil {
		return fmt.Errorf("inspect legacy thread project references: %w", err)
	}
	var threadGaps []threadProjectGap
	for rows.Next() {
		var gap threadProjectGap
		if err := rows.Scan(&gap.threadID, &gap.workspaceRoot); err != nil {
			rows.Close()
			return fmt.Errorf("scan legacy thread project reference: %w", err)
		}
		threadGaps = append(threadGaps, gap)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close legacy thread project references: %w", err)
	}
	for _, gap := range threadGaps {
		projectID, err := migratedOrRecoveryProjectTx(tx, paths, gap.workspaceRoot)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE threads SET project_id = ? WHERE thread_id = ?`, projectID, gap.threadID); err != nil {
			return fmt.Errorf("repair thread %q project reference: %w", gap.threadID, err)
		}
	}

	type operationProjectGap struct {
		operationID   string
		workspaceRoot string
	}
	rows, err = tx.Query(`
SELECT o.operation_id, o.workspace_root
FROM project_operations o
LEFT JOIN projects p ON p.project_id = o.project_id
WHERE TRIM(o.project_id) = '' OR p.project_id IS NULL`)
	if err != nil {
		return fmt.Errorf("inspect legacy operation project references: %w", err)
	}
	var operationGaps []operationProjectGap
	for rows.Next() {
		var gap operationProjectGap
		if err := rows.Scan(&gap.operationID, &gap.workspaceRoot); err != nil {
			rows.Close()
			return fmt.Errorf("scan legacy operation project reference: %w", err)
		}
		operationGaps = append(operationGaps, gap)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close legacy operation project references: %w", err)
	}
	for _, gap := range operationGaps {
		projectID, err := migratedOrRecoveryProjectTx(tx, paths, gap.workspaceRoot)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE project_operations SET project_id = ? WHERE operation_id = ?`, projectID, gap.operationID); err != nil {
			return fmt.Errorf("repair operation %q project reference: %w", gap.operationID, err)
		}
	}

	rows, err = tx.Query(`
SELECT child.operation_id
FROM (
  SELECT operation_id FROM project_operation_threads
  UNION
  SELECT operation_id FROM cleanup_jobs
) child
LEFT JOIN project_operations o ON o.operation_id = child.operation_id
WHERE o.operation_id IS NULL`)
	if err != nil {
		return fmt.Errorf("inspect orphan operation references: %w", err)
	}
	var orphanOperationIDs []string
	for rows.Next() {
		var operationID string
		if err := rows.Scan(&operationID); err != nil {
			rows.Close()
			return fmt.Errorf("scan orphan operation reference: %w", err)
		}
		orphanOperationIDs = append(orphanOperationIDs, operationID)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close orphan operation references: %w", err)
	}
	if len(orphanOperationIDs) > 0 {
		recoveryProjectID, recoveryRoot, err := ensureRecoveryProjectTx(tx, paths)
		if err != nil {
			return err
		}
		for _, operationID := range orphanOperationIDs {
			now := formatStoredTime(time.Now().UTC())
			idempotencyKey := recoveredOperationIdempotencyKey(operationID)
			if _, err := tx.Exec(`
INSERT INTO project_operations(
  operation_id, idempotency_key, project_id, action, status, phase,
  previous_project_state, workspace_root, error_code, error_message,
  result_json, created_at, updated_at
) VALUES(?, ?, ?, 'delete_history', 'blocked', 'failed', 'removed', ?,
         'MIGRATED_ORPHAN_OPERATION', 'Recovered parent for legacy relation',
         '{"migrated_orphan":true}', ?, ?)
`, operationID, idempotencyKey, recoveryProjectID, recoveryRoot, now, now); err != nil {
				return fmt.Errorf("recover orphan operation %q: %w", operationID, err)
			}
		}
	}

	rows, err = tx.Query(`
SELECT DISTINCT pot.thread_id, o.project_id, p.workspace_root
FROM project_operation_threads pot
JOIN project_operations o ON o.operation_id = pot.operation_id
JOIN projects p ON p.project_id = o.project_id
LEFT JOIN threads t ON t.thread_id = pot.thread_id
WHERE t.thread_id IS NULL`)
	if err != nil {
		return fmt.Errorf("inspect orphan thread references: %w", err)
	}
	type orphanThread struct {
		threadID      string
		projectID     string
		workspaceRoot string
	}
	var orphanThreads []orphanThread
	for rows.Next() {
		var orphan orphanThread
		if err := rows.Scan(&orphan.threadID, &orphan.projectID, &orphan.workspaceRoot); err != nil {
			rows.Close()
			return fmt.Errorf("scan orphan thread reference: %w", err)
		}
		orphanThreads = append(orphanThreads, orphan)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close orphan thread references: %w", err)
	}
	for _, orphan := range orphanThreads {
		workspaceJSON, _ := json.Marshal(WorkspaceConfig{
			ScopeID:       "recovered",
			WorkspaceRoot: orphan.workspaceRoot,
		})
		now := formatStoredTime(time.Now().UTC())
		if _, err := tx.Exec(`
INSERT INTO threads(
  thread_id, title, title_source, title_status, title_updated_at,
  workspace_root, project_id, scope_id, created_at, updated_at,
  archived, deleted, deleted_at, preview, model_profile_key, workspace_json
) VALUES(?, 'Recovered deleted thread metadata', 'manual', 'ready', ?,
         ?, ?, 'recovered', ?, ?, 0, 1, ?, '', '', ?)
`, orphan.threadID, now, orphan.workspaceRoot, orphan.projectID, now, now, now, string(workspaceJSON)); err != nil {
			return fmt.Errorf("recover orphan thread %q: %w", orphan.threadID, err)
		}
	}
	return nil
}

func migratedOrRecoveryProjectTx(tx *sql.Tx, paths desktopdata.Paths, workspaceRoot string) (string, error) {
	normalized := normalizeProjectWorkspaceRoot(workspaceRoot)
	if normalized != "" && filepath.IsAbs(normalized) {
		return ensureMigratedProjectForWorkspaceTx(tx, normalized, false)
	}
	projectID, _, err := ensureRecoveryProjectTx(tx, paths)
	return projectID, err
}

func ensureRecoveryProjectTx(tx *sql.Tx, paths desktopdata.Paths) (string, string, error) {
	root := normalizeProjectWorkspaceRoot(filepath.Join(paths.Root(), "recovered-project-metadata"))
	const workspaceKey = "awaken-internal:recovered-project-metadata"
	projectID := stableProjectID(workspaceKey)
	now := formatStoredTime(time.Now().UTC())
	if _, err := tx.Exec(`
INSERT INTO projects(
  project_id, workspace_root, workspace_key, display_name, state,
  removal_reason, removed_at, created_at, updated_at
) VALUES(?, ?, ?, 'Recovered project metadata', 'removed', 'detached', ?, ?, ?)
ON CONFLICT DO NOTHING
`, projectID, root, workspaceKey, now, now, now); err != nil {
		return "", "", fmt.Errorf("create recovery project metadata: %w", err)
	}
	var state string
	if err := tx.QueryRow(`SELECT project_id, state FROM projects WHERE workspace_key = ?`, workspaceKey).Scan(&projectID, &state); err != nil {
		return "", "", fmt.Errorf("read recovery project metadata: %w", err)
	}
	if ProjectState(state) != ProjectStateRemoved {
		return "", "", fmt.Errorf("recovery project metadata is not hidden")
	}
	return projectID, root, nil
}

func recoveredOperationIdempotencyKey(operationID string) string {
	sum := sha256.Sum256([]byte(operationID))
	return "migration:recovered-operation:" + hex.EncodeToString(sum[:])
}

func rebuildProjectRelationTablesTx(tx *sql.Tx) error {
	for _, statement := range []string{
		`CREATE TABLE threads_fk_v2 (
  thread_id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  title_source TEXT NOT NULL DEFAULT '',
  title_status TEXT NOT NULL DEFAULT '',
  title_updated_at TEXT NOT NULL DEFAULT '',
  title_seed_run_id TEXT NOT NULL DEFAULT '',
  workspace_root TEXT NOT NULL,
  project_id TEXT NOT NULL,
  scope_id TEXT NOT NULL,
  latest_run_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  archived INTEGER NOT NULL DEFAULT 0,
  deleted INTEGER NOT NULL DEFAULT 0,
  deleted_at TEXT NOT NULL DEFAULT '',
  delete_operation_id TEXT NOT NULL DEFAULT '',
  preview TEXT NOT NULL DEFAULT '',
  model_profile_key TEXT NOT NULL DEFAULT '',
  workspace_json TEXT NOT NULL,
  FOREIGN KEY(project_id) REFERENCES projects(project_id) ON UPDATE CASCADE ON DELETE RESTRICT
)`,
		`INSERT INTO threads_fk_v2 SELECT
  thread_id, title, title_source, title_status, title_updated_at, title_seed_run_id,
  workspace_root, project_id, scope_id, latest_run_id, created_at, updated_at,
  archived, deleted, deleted_at, delete_operation_id, preview, model_profile_key, workspace_json
FROM threads`,
		`CREATE TABLE project_operations_fk_v2 (
  operation_id TEXT PRIMARY KEY,
  idempotency_key TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL,
  action TEXT NOT NULL,
  status TEXT NOT NULL,
  phase TEXT NOT NULL,
  previous_project_state TEXT NOT NULL DEFAULT '',
  workspace_root TEXT NOT NULL DEFAULT '',
  path_fingerprint TEXT NOT NULL DEFAULT '',
  attempt_id TEXT NOT NULL DEFAULT '',
  trash_staging_path TEXT NOT NULL DEFAULT '',
  trash_invoked_at TEXT NOT NULL DEFAULT '',
  cutoff_at TEXT NOT NULL DEFAULT '',
  logical_committed_at TEXT NOT NULL DEFAULT '',
  physical_completed_at TEXT NOT NULL DEFAULT '',
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  result_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY(project_id) REFERENCES projects(project_id) ON UPDATE CASCADE ON DELETE RESTRICT
)`,
		`INSERT INTO project_operations_fk_v2 SELECT
  operation_id, idempotency_key, project_id, action, status, phase,
  previous_project_state, workspace_root, path_fingerprint, attempt_id,
  trash_staging_path, trash_invoked_at, cutoff_at, logical_committed_at,
  physical_completed_at, error_code, error_message, result_json, created_at, updated_at
FROM project_operations`,
		`CREATE TABLE project_operation_threads_fk_v2 (
  operation_id TEXT NOT NULL,
  thread_id TEXT NOT NULL,
  PRIMARY KEY(operation_id, thread_id),
  FOREIGN KEY(operation_id) REFERENCES project_operations_fk_v2(operation_id) ON UPDATE CASCADE ON DELETE CASCADE,
  FOREIGN KEY(thread_id) REFERENCES threads_fk_v2(thread_id) ON UPDATE CASCADE ON DELETE CASCADE
)`,
		`INSERT INTO project_operation_threads_fk_v2 SELECT operation_id, thread_id FROM project_operation_threads`,
		`CREATE TABLE cleanup_jobs_fk_v2 (
  job_id TEXT PRIMARY KEY,
  operation_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  target_id TEXT NOT NULL,
  payload_json TEXT NOT NULL DEFAULT '{}',
  status TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  next_attempt_at TEXT NOT NULL DEFAULT '',
  lease_owner TEXT NOT NULL DEFAULT '',
  lease_expires_at TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  completed_at TEXT NOT NULL DEFAULT '',
  FOREIGN KEY(operation_id) REFERENCES project_operations_fk_v2(operation_id) ON UPDATE CASCADE ON DELETE CASCADE
)`,
		`INSERT INTO cleanup_jobs_fk_v2 SELECT
  job_id, operation_id, kind, target_id, payload_json, status, attempts,
  next_attempt_at, lease_owner, lease_expires_at, last_error, created_at, updated_at, completed_at
FROM cleanup_jobs`,
		`DROP TABLE project_operation_threads`,
		`DROP TABLE cleanup_jobs`,
		`DROP TABLE threads`,
		`DROP TABLE project_operations`,
		`ALTER TABLE threads_fk_v2 RENAME TO threads`,
		`ALTER TABLE project_operations_fk_v2 RENAME TO project_operations`,
		`ALTER TABLE project_operation_threads_fk_v2 RENAME TO project_operation_threads`,
		`ALTER TABLE cleanup_jobs_fk_v2 RENAME TO cleanup_jobs`,
		`CREATE INDEX idx_threads_updated_at ON threads(updated_at DESC, created_at DESC)`,
		`CREATE INDEX idx_threads_project_live ON threads(project_id, deleted, updated_at DESC)`,
		`CREATE INDEX idx_project_operations_project ON project_operations(project_id, created_at DESC)`,
		`CREATE INDEX idx_project_operations_pending ON project_operations(action, status, phase, updated_at)`,
		`CREATE UNIQUE INDEX idx_cleanup_thread_purge_unique ON cleanup_jobs(kind, target_id) WHERE kind = 'thread_purge'`,
		`CREATE INDEX idx_cleanup_jobs_due ON cleanup_jobs(kind, status, next_attempt_at)`,
	} {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("rebuild project relation schema: %w", err)
		}
	}
	return nil
}

func verifyProjectForeignKeySchema(db *sql.DB) error {
	for _, expected := range requiredProjectForeignKeys {
		rows, err := db.Query(`PRAGMA foreign_key_list(` + expected.table + `)`)
		if err != nil {
			return fmt.Errorf("inspect %s foreign keys: %w", expected.table, err)
		}
		matched := false
		for rows.Next() {
			var id, seq int
			var table, from, to, onUpdate, onDelete, match string
			if err := rows.Scan(&id, &seq, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				rows.Close()
				return fmt.Errorf("scan %s foreign key: %w", expected.table, err)
			}
			if table == expected.toTable && from == expected.fromColumn && to == expected.toColumn && strings.EqualFold(onUpdate, expected.onUpdate) && strings.EqualFold(onDelete, expected.onDelete) {
				matched = true
			}
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close %s foreign keys: %w", expected.table, err)
		}
		if !matched {
			return fmt.Errorf("%s is missing foreign key %s -> %s.%s", expected.table, expected.fromColumn, expected.toTable, expected.toColumn)
		}
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("check project foreign keys: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		var table string
		var rowID sql.NullInt64
		var parent string
		var constraintID int
		if err := rows.Scan(&table, &rowID, &parent, &constraintID); err != nil {
			return fmt.Errorf("scan project foreign-key violation: %w", err)
		}
		return fmt.Errorf("project foreign-key violation: table=%s row=%v parent=%s constraint=%d", table, rowID, parent, constraintID)
	}
	return rows.Err()
}

func ensureMigratedProjectForWorkspaceTx(tx *sql.Tx, root string, active bool) (string, error) {
	if active {
		return ensureProjectForWorkspaceTx(tx, root, true)
	}
	normalized := normalizeProjectWorkspaceRoot(root)
	if normalized == "" || !filepath.IsAbs(normalized) {
		return "", fmt.Errorf("project workspace root must be an absolute path")
	}
	projectID := stableProjectID(normalized)
	now := time.Now().UTC()
	if _, err := tx.Exec(`
INSERT INTO projects(
  project_id, workspace_root, workspace_key, display_name, state,
  removal_reason, removed_at, created_at, updated_at
) VALUES(?, ?, ?, ?, 'removed', 'detached', ?, ?, ?)
ON CONFLICT(workspace_key) DO NOTHING
`, projectID, normalized, normalized, projectDisplayName(normalized), formatStoredTime(now), formatStoredTime(now), formatStoredTime(now)); err != nil {
		return "", fmt.Errorf("migrate detached project for workspace %q: %w", normalized, err)
	}
	if err := tx.QueryRow(`SELECT project_id FROM projects WHERE workspace_key = ?`, normalized).Scan(&projectID); err != nil {
		return "", fmt.Errorf("read migrated project id for workspace %q: %w", normalized, err)
	}
	return projectID, nil
}

func readLegacyWorkspaceState(paths desktopdata.Paths) ([]string, string) {
	var registry struct {
		Workspaces []string `json:"workspaces"`
	}
	if raw, err := os.ReadFile(filepath.Join(paths.Root(), "workspace-registry.json")); err == nil {
		_ = json.Unmarshal(raw, &registry)
	}
	var last struct {
		Path string `json:"path"`
	}
	if raw, err := os.ReadFile(filepath.Join(paths.Root(), "last-workspace.json")); err == nil {
		_ = json.Unmarshal(raw, &last)
	}
	seen := map[string]struct{}{}
	roots := make([]string, 0, len(registry.Workspaces)+1)
	for _, root := range append([]string{last.Path}, registry.Workspaces...) {
		normalized := normalizeProjectWorkspaceRoot(root)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		roots = append(roots, normalized)
	}
	return roots, normalizeProjectWorkspaceRoot(last.Path)
}

type projectDBTX interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

func ensureProjectForWorkspace(db projectDBTX, root string, reactivate bool) (string, error) {
	return ensureProjectForWorkspaceDB(db, root, reactivate)
}

func ensureProjectForWorkspaceTx(tx *sql.Tx, root string, reactivate bool) (string, error) {
	return ensureProjectForWorkspaceDB(tx, root, reactivate)
}

func ensureProjectForWorkspaceDB(db projectDBTX, root string, reactivate bool) (string, error) {
	normalized := normalizeProjectWorkspaceRoot(root)
	if normalized == "" || !filepath.IsAbs(normalized) {
		return "", fmt.Errorf("project workspace root must be an absolute path")
	}
	projectID := stableProjectID(normalized)
	now := formatStoredTime(time.Now().UTC())
	if _, err := db.Exec(`
INSERT INTO projects(project_id, workspace_root, workspace_key, display_name, state, created_at, updated_at)
VALUES(?, ?, ?, ?, 'active', ?, ?)
ON CONFLICT(workspace_key) DO UPDATE SET
  workspace_root = excluded.workspace_root,
  display_name = excluded.display_name,
  state = CASE WHEN ? THEN 'active' ELSE projects.state END,
  removal_reason = CASE WHEN ? THEN '' ELSE projects.removal_reason END,
  removed_at = CASE WHEN ? THEN '' ELSE projects.removed_at END,
  updated_at = excluded.updated_at,
  version = projects.version + 1
`, projectID, normalized, normalized, projectDisplayName(normalized), now, now, reactivate, reactivate, reactivate); err != nil {
		return "", fmt.Errorf("ensure project for workspace %q: %w", normalized, err)
	}
	if err := db.QueryRow(`SELECT project_id FROM projects WHERE workspace_key = ?`, normalized).Scan(&projectID); err != nil {
		return "", fmt.Errorf("read project id for workspace %q: %w", normalized, err)
	}
	return projectID, nil
}

func scanProject(scanner rowScanner) (Project, error) {
	var project Project
	var state string
	var removedAtRaw, createdAtRaw, updatedAtRaw string
	if err := scanner.Scan(
		&project.ProjectID,
		&project.WorkspaceRoot,
		&project.DisplayName,
		&state,
		&project.RemovalReason,
		&removedAtRaw,
		&createdAtRaw,
		&updatedAtRaw,
		&project.Version,
	); err != nil {
		return Project{}, err
	}
	project.State = ProjectState(state)
	project.CreatedAt = parseStoredTime(createdAtRaw)
	project.UpdatedAt = parseStoredTime(updatedAtRaw)
	if removed := parseStoredTime(removedAtRaw); !removed.IsZero() {
		project.RemovedAt = &removed
	}
	return project, nil
}

func projectByID(db projectDBTX, projectID string) (Project, error) {
	project, err := scanProject(db.QueryRow(`
SELECT project_id, workspace_root, display_name, state, removal_reason, removed_at, created_at, updated_at, version
FROM projects WHERE project_id = ?
`, strings.TrimSpace(projectID)))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Project{}, ErrProjectNotFound
		}
		return Project{}, err
	}
	return project, nil
}

func scanProjectOperation(scanner rowScanner) (ProjectOperation, error) {
	var operation ProjectOperation
	var action, status, previousState string
	var cutoffRaw, logicalRaw, physicalRaw, trashInvokedRaw, createdRaw, updatedRaw string
	var resultRaw string
	if err := scanner.Scan(
		&operation.OperationID,
		&operation.IdempotencyKey,
		&operation.ProjectID,
		&action,
		&status,
		&operation.Phase,
		&previousState,
		&operation.WorkspaceRoot,
		&operation.PathFingerprint,
		&operation.AttemptID,
		&operation.TrashStagingPath,
		&trashInvokedRaw,
		&cutoffRaw,
		&logicalRaw,
		&physicalRaw,
		&operation.ErrorCode,
		&operation.ErrorMessage,
		&resultRaw,
		&createdRaw,
		&updatedRaw,
	); err != nil {
		return ProjectOperation{}, err
	}
	operation.Action = ProjectOperationAction(action)
	operation.Status = ProjectOperationStatus(status)
	operation.PreviousProjectState = ProjectState(previousState)
	operation.CreatedAt = parseStoredTime(createdRaw)
	operation.UpdatedAt = parseStoredTime(updatedRaw)
	if value := parseStoredTime(logicalRaw); !value.IsZero() {
		operation.LogicalCommittedAt = &value
	}
	if value := parseStoredTime(physicalRaw); !value.IsZero() {
		operation.PhysicalCompletedAt = &value
	}
	if value := parseStoredTime(trashInvokedRaw); !value.IsZero() {
		operation.TrashInvokedAt = &value
	}
	var result struct {
		MatchedThreads int `json:"matched_threads"`
	}
	_ = json.Unmarshal([]byte(resultRaw), &result)
	operation.MatchedThreads = result.MatchedThreads
	return operation, nil
}

const projectOperationSelect = `
SELECT operation_id, idempotency_key, project_id, action, status, phase,
       previous_project_state, workspace_root, path_fingerprint, attempt_id,
       trash_staging_path, trash_invoked_at, cutoff_at,
       logical_committed_at, physical_completed_at, error_code, error_message,
       result_json, created_at, updated_at
FROM project_operations`

func projectOperationByID(db projectDBTX, operationID string) (ProjectOperation, error) {
	operation, err := scanProjectOperation(db.QueryRow(projectOperationSelect+` WHERE operation_id = ?`, strings.TrimSpace(operationID)))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProjectOperation{}, ErrProjectOperationNotFound
		}
		return ProjectOperation{}, err
	}
	return operation, nil
}

func projectOperationByIdempotencyKey(db projectDBTX, key string) (ProjectOperation, error) {
	operation, err := scanProjectOperation(db.QueryRow(projectOperationSelect+` WHERE idempotency_key = ?`, strings.TrimSpace(key)))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProjectOperation{}, ErrProjectOperationNotFound
		}
		return ProjectOperation{}, err
	}
	return operation, nil
}
