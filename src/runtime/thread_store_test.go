package runtime

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestThreadStoreMigratesLegacyTitleSchema(t *testing.T) {
	userDataDir := t.TempDir()
	t.Setenv(runtimeUserDataDirEnv, userDataDir)

	dataDir := filepath.Join(userDataDir, "data")
	if err := os.MkdirAll(filepath.Join(dataDir, "threads"), 0o755); err != nil {
		t.Fatalf("mkdir data dirs: %v", err)
	}

	db, err := sql.Open("sqlite", filepath.Join(dataDir, "threads.sqlite"))
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	_, err = db.Exec(`
CREATE TABLE threads (
	thread_id TEXT PRIMARY KEY,
	title TEXT NOT NULL,
	workspace_root TEXT NOT NULL,
	scope_id TEXT NOT NULL,
	latest_run_id TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	archived INTEGER NOT NULL DEFAULT 0,
	deleted INTEGER NOT NULL DEFAULT 0,
	preview TEXT NOT NULL DEFAULT '',
	model_profile_key TEXT NOT NULL DEFAULT '',
	workspace_json TEXT NOT NULL
);
`)
	if err != nil {
		t.Fatalf("create legacy threads table: %v", err)
	}
	workspaceRoot := t.TempDir()
	workspace := WorkspaceConfig{
		WorkspaceRoot: workspaceRoot,
		ScopeID:       "scope_legacy",
		ReadableRoots: []string{workspaceRoot},
		WritableRoots: []string{workspaceRoot},
		OutputRoots:   []string{workspaceRoot},
	}
	workspaceJSON, err := json.Marshal(workspace)
	if err != nil {
		t.Fatalf("marshal workspace: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.Exec(`
INSERT INTO threads (
	thread_id, title, workspace_root, scope_id, latest_run_id,
	created_at, updated_at, archived, deleted, preview, model_profile_key, workspace_json
) VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?, ?)
`, "thread_legacy", "legacy title", workspaceRoot, "scope_legacy", "run_legacy", now, now, "legacy preview", "profile-a", string(workspaceJSON))
	if err != nil {
		t.Fatalf("insert legacy thread: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	store := newThreadStore()
	threads, err := store.ListThreads()
	if err != nil {
		t.Fatalf("ListThreads returned error: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected one migrated thread, got %d", len(threads))
	}
	if threads[0].ThreadID != "thread_legacy" || threads[0].Label != "legacy title" {
		t.Fatalf("unexpected migrated thread: %+v", threads[0])
	}
	if threads[0].TitleSource != ThreadTitleSourceManual || threads[0].TitleStatus != ThreadTitleStatusReady {
		t.Fatalf("expected legacy title to default to manual ready, got %+v", threads[0])
	}
	if threads[0].Workspace.WorkspaceRoot != workspaceRoot {
		t.Fatalf("expected workspace root %q, got %q", workspaceRoot, threads[0].Workspace.WorkspaceRoot)
	}
	sqliteStore, ok := store.(*sqliteThreadStore)
	if !ok {
		t.Fatalf("expected sqlite thread store, got %T", store)
	}
	migratedDB, err := sqliteStore.open()
	if err != nil {
		t.Fatal(err)
	}
	var projectID, state string
	if err := migratedDB.QueryRow(`
SELECT t.project_id, p.state
FROM threads t JOIN projects p ON p.project_id = t.project_id
WHERE t.thread_id = 'thread_legacy'
`).Scan(&projectID, &state); err != nil {
		t.Fatalf("query migrated project association: %v", err)
	}
	if projectID != stableProjectID(workspaceRoot) || state != string(ProjectStateRemoved) {
		t.Fatalf("unexpected migrated project id/state: %q %q", projectID, state)
	}
	var journalMode string
	var busyTimeout, foreignKeys int
	if err := migratedDB.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if err := migratedDB.QueryRow(`PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	if err := migratedDB.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if journalMode != "wal" || busyTimeout != 5000 || foreignKeys != 1 {
		t.Fatalf("unexpected sqlite durability settings: mode=%q timeout=%d foreign_keys=%d", journalMode, busyTimeout, foreignKeys)
	}
}

func TestThreadStoreMigrationKeepsExplicitWorkspaceStateActive(t *testing.T) {
	tests := []struct {
		name       string
		fileName   string
		legacyJSON func(string) any
		checkLast  bool
	}{
		{
			name:     "workspace registry",
			fileName: "workspace-registry.json",
			legacyJSON: func(root string) any {
				return map[string]any{"workspaces": []string{root}}
			},
		},
		{
			name:     "last workspace",
			fileName: "last-workspace.json",
			legacyJSON: func(root string) any {
				return map[string]any{"path": root}
			},
			checkLast: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userDataDir := t.TempDir()
			t.Setenv(runtimeUserDataDirEnv, userDataDir)
			workspaceRoot := t.TempDir()
			raw, err := json.Marshal(tt.legacyJSON(workspaceRoot))
			if err != nil {
				t.Fatalf("marshal legacy workspace state: %v", err)
			}
			if err := os.WriteFile(filepath.Join(userDataDir, tt.fileName), raw, 0o600); err != nil {
				t.Fatalf("write legacy workspace state: %v", err)
			}

			store := newThreadStore()
			if _, err := store.ListThreads(); err != nil {
				t.Fatalf("open migrated thread store: %v", err)
			}
			sqliteStore, ok := store.(*sqliteThreadStore)
			if !ok {
				t.Fatalf("expected sqlite thread store, got %T", store)
			}
			db, err := sqliteStore.open()
			if err != nil {
				t.Fatal(err)
			}
			var state string
			if err := db.QueryRow(`SELECT state FROM projects WHERE project_id = ?`, stableProjectID(workspaceRoot)).Scan(&state); err != nil {
				t.Fatalf("query migrated project: %v", err)
			}
			if state != string(ProjectStateActive) {
				t.Fatalf("expected explicit workspace to remain active, got %q", state)
			}
			if tt.checkLast {
				var last string
				if err := db.QueryRow(`SELECT value FROM desktop_state WHERE key = 'last_workspace_path'`).Scan(&last); err != nil {
					t.Fatalf("query migrated last workspace: %v", err)
				}
				if last != normalizeProjectWorkspaceRoot(workspaceRoot) {
					t.Fatalf("expected last workspace %q, got %q", workspaceRoot, last)
				}
			}
		})
	}
}

func TestProjectLifecycleForeignKeysRejectOrphansAndCascadeOwnedMetadata(t *testing.T) {
	userDataDir := t.TempDir()
	t.Setenv(runtimeUserDataDirEnv, userDataDir)
	workspaceRoot := t.TempDir()
	store := newThreadStore()
	now := time.Now().UTC()
	workspace := WorkspaceConfig{
		WorkspaceRoot: workspaceRoot,
		ScopeID:       "scope_foreign_keys",
		ReadableRoots: []string{workspaceRoot},
		WritableRoots: []string{workspaceRoot},
		OutputRoots:   []string{workspaceRoot},
	}
	snapshot := ThreadSnapshot{
		ThreadID:    "thread_foreign_keys",
		Label:       "Foreign key fixture",
		Workspace:   workspace,
		CreatedAt:   now,
		UpdatedAt:   now,
		TitleSource: ThreadTitleSourceManual,
		TitleStatus: ThreadTitleStatusReady,
	}
	if err := store.CreateThread(snapshot, "", "fixture"); err != nil {
		t.Fatalf("create thread fixture: %v", err)
	}
	sqliteStore := store.(*sqliteThreadStore)
	db, err := sqliteStore.open()
	if err != nil {
		t.Fatal(err)
	}
	foreignKeyParents := map[string]bool{}
	foreignKeyRows, err := db.Query(`PRAGMA foreign_key_list(project_operation_threads)`)
	if err != nil {
		t.Fatal(err)
	}
	for foreignKeyRows.Next() {
		var id, seq int
		var table, from, to, onUpdate, onDelete, match string
		if err := foreignKeyRows.Scan(&id, &seq, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			foreignKeyRows.Close()
			t.Fatal(err)
		}
		foreignKeyParents[table] = true
	}
	if err := foreignKeyRows.Close(); err != nil {
		t.Fatal(err)
	}
	if !foreignKeyParents["project_operations"] || !foreignKeyParents["threads"] || len(foreignKeyParents) != 2 {
		t.Fatalf("foreign keys still reference migration staging tables: %+v", foreignKeyParents)
	}
	projectID := stableProjectID(workspaceRoot)
	nowRaw := formatStoredTime(now)
	if _, err := db.Exec(`
INSERT INTO project_operations(
  operation_id, idempotency_key, project_id, action, status, phase, created_at, updated_at
) VALUES('operation_fk', 'operation-fk-key', ?, 'delete_history', 'accepted', 'cleaning', ?, ?)
`, projectID, nowRaw, nowRaw); err != nil {
		t.Fatalf("insert operation fixture: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO project_operation_threads(operation_id, thread_id) VALUES('operation_fk', 'thread_foreign_keys')`); err != nil {
		t.Fatalf("insert operation thread fixture: %v", err)
	}
	if _, err := db.Exec(`
INSERT INTO cleanup_jobs(job_id, operation_id, kind, target_id, status, created_at, updated_at)
VALUES('job_fk', 'operation_fk', 'thread_purge', 'thread_foreign_keys', 'pending', ?, ?)
`, nowRaw, nowRaw); err != nil {
		t.Fatalf("insert cleanup fixture: %v", err)
	}

	if _, err := db.Exec(`UPDATE threads SET project_id = 'missing-project' WHERE thread_id = 'thread_foreign_keys'`); err == nil {
		t.Fatal("thread project foreign key accepted an orphan reference")
	}
	if _, err := db.Exec(`DELETE FROM projects WHERE project_id = ?`, projectID); err == nil {
		t.Fatal("project delete bypassed RESTRICT while owned records still exist")
	}
	if _, err := db.Exec(`DELETE FROM project_operations WHERE operation_id = 'operation_fk'`); err != nil {
		t.Fatalf("delete operation fixture: %v", err)
	}
	for table, query := range map[string]string{
		"project_operation_threads": `SELECT COUNT(*) FROM project_operation_threads WHERE operation_id = 'operation_fk'`,
		"cleanup_jobs":              `SELECT COUNT(*) FROM cleanup_jobs WHERE operation_id = 'operation_fk'`,
	} {
		var count int
		if err := db.QueryRow(query).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("deleting an operation did not cascade %s: %d rows remain", table, count)
		}
	}
	var threadCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM threads WHERE thread_id = 'thread_foreign_keys'`).Scan(&threadCount); err != nil {
		t.Fatal(err)
	}
	if threadCount != 1 {
		t.Fatalf("operation cascade unexpectedly deleted the thread: count=%d", threadCount)
	}
}

func TestProjectForeignKeyMigrationRepairsOrphansWithoutDroppingMetadata(t *testing.T) {
	userDataDir := t.TempDir()
	t.Setenv(runtimeUserDataDirEnv, userDataDir)
	store := newThreadStore().(*sqliteThreadStore)
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("disable foreign keys for legacy fixture: %v", err)
	}
	workspaceRoot := t.TempDir()
	workspaceJSON, err := json.Marshal(WorkspaceConfig{
		WorkspaceRoot: workspaceRoot,
		ScopeID:       "scope_orphan_migration",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := formatStoredTime(time.Now().UTC())
	if _, err := db.Exec(`
INSERT INTO threads(
  thread_id, title, workspace_root, project_id, scope_id, created_at, updated_at, workspace_json
) VALUES('thread_orphan_project', 'orphan project', ?, 'missing-project', 'scope_orphan_migration', ?, ?, ?)
`, workspaceRoot, now, now, string(workspaceJSON)); err != nil {
		t.Fatalf("insert orphan thread fixture: %v", err)
	}
	if _, err := db.Exec(`
INSERT INTO project_operations(
  operation_id, idempotency_key, project_id, action, status, phase,
  workspace_root, created_at, updated_at
) VALUES('operation_orphan_project', 'orphan-project-key', 'missing-project',
         'delete_history', 'blocked', 'failed', ?, ?, ?)
`, workspaceRoot, now, now); err != nil {
		t.Fatalf("insert orphan operation project fixture: %v", err)
	}
	if _, err := db.Exec(`
INSERT INTO project_operation_threads(operation_id, thread_id)
VALUES('operation_missing_parent', 'thread_missing_parent')
`); err != nil {
		t.Fatalf("insert orphan cutoff fixture: %v", err)
	}
	if _, err := db.Exec(`
INSERT INTO cleanup_jobs(
  job_id, operation_id, kind, target_id, payload_json, status, created_at, updated_at
) VALUES('job_orphan_parent', 'operation_missing_parent', 'thread_purge',
         'thread_missing_parent', '{"keep":"yes"}', 'blocked', ?, ?)
`, now, now); err != nil {
		t.Fatalf("insert orphan cleanup fixture: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version = ?`, projectForeignKeySchemaVersion); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	if err := store.db.Close(); err != nil {
		store.mu.Unlock()
		t.Fatalf("close legacy fixture: %v", err)
	}
	store.db = nil
	store.mu.Unlock()

	reopened := &sqliteThreadStore{paths: store.paths}
	migratedDB, err := reopened.open()
	if err != nil {
		t.Fatalf("migrate orphan relations: %v", err)
	}
	if err := verifyProjectForeignKeySchema(migratedDB); err != nil {
		t.Fatalf("verify migrated foreign keys: %v", err)
	}
	if err := ensureProjectForeignKeySchema(migratedDB, reopened.paths); err != nil {
		t.Fatalf("repeat foreign-key migration must be idempotent: %v", err)
	}
	var migrationRows int
	if err := migratedDB.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, projectForeignKeySchemaVersion).Scan(&migrationRows); err != nil {
		t.Fatal(err)
	}
	if migrationRows != 1 {
		t.Fatalf("foreign-key migration was recorded %d times", migrationRows)
	}
	for label, query := range map[string]string{
		"thread project":      `SELECT COUNT(*) FROM threads t JOIN projects p ON p.project_id = t.project_id WHERE t.thread_id = 'thread_orphan_project'`,
		"operation project":   `SELECT COUNT(*) FROM project_operations o JOIN projects p ON p.project_id = o.project_id WHERE o.operation_id = 'operation_orphan_project'`,
		"recovered operation": `SELECT COUNT(*) FROM project_operations WHERE operation_id = 'operation_missing_parent' AND error_code = 'MIGRATED_ORPHAN_OPERATION'`,
		"recovered thread":    `SELECT COUNT(*) FROM threads WHERE thread_id = 'thread_missing_parent' AND deleted = 1`,
		"preserved cutoff":    `SELECT COUNT(*) FROM project_operation_threads WHERE operation_id = 'operation_missing_parent' AND thread_id = 'thread_missing_parent'`,
	} {
		var count int
		if err := migratedDB.QueryRow(query).Scan(&count); err != nil {
			t.Fatalf("query %s: %v", label, err)
		}
		if count != 1 {
			t.Fatalf("migration did not preserve %s: count=%d", label, count)
		}
	}
	var payload string
	if err := migratedDB.QueryRow(`SELECT payload_json FROM cleanup_jobs WHERE job_id = 'job_orphan_parent'`).Scan(&payload); err != nil {
		t.Fatalf("read preserved cleanup payload: %v", err)
	}
	if payload != `{"keep":"yes"}` {
		t.Fatalf("cleanup payload changed during migration: %q", payload)
	}
	projectService := &ProjectService{store: reopened}
	activeProjects, err := projectService.ListProjects(false)
	if err != nil {
		t.Fatalf("list active projects after orphan repair: %v", err)
	}
	for _, project := range activeProjects {
		if project.DisplayName == "Recovered project metadata" {
			t.Fatalf("orphan repair leaked its hidden recovery project into the active list: %+v", project)
		}
	}
	if _, err := migratedDB.Exec(`
INSERT INTO cleanup_jobs(job_id, operation_id, kind, target_id, status, created_at, updated_at)
VALUES('job_rejected_orphan', 'still-missing', 'thread_purge', 'none', 'pending', ?, ?)
`, now, now); err == nil {
		t.Fatal("migrated cleanup_jobs accepted a new orphan operation")
	}
}

func TestThreadStoreStalePendingSnapshotDoesNotOverwriteGeneratedTitle(t *testing.T) {
	setRuntimeUserDataDir(t)
	store := newThreadStore()
	now := time.Now().UTC()
	snapshot := storeTitleTestSnapshot(t, "thread_generated", "run_seed", "first message", now)

	if err := store.CreateThread(snapshot, "profile-a", "first message"); err != nil {
		t.Fatalf("CreateThread returned error: %v", err)
	}
	generatedAt := now.Add(time.Second)
	generated, err := store.UpdateThreadTitleMetadata(snapshot.ThreadID, "Generated title", ThreadTitleSourceLLM, ThreadTitleStatusReady, snapshot.TitleSeedRunID, generatedAt)
	if err != nil {
		t.Fatalf("UpdateThreadTitleMetadata returned error: %v", err)
	}
	if generated.Label != "Generated title" || generated.TitleSource != ThreadTitleSourceLLM || generated.TitleStatus != ThreadTitleStatusReady {
		t.Fatalf("unexpected generated title snapshot: %+v", generated)
	}

	stale := snapshot
	stale.UpdatedAt = generatedAt.Add(time.Second)
	if err := store.CreateThread(stale, "", ""); err != nil {
		t.Fatalf("stale CreateThread returned error: %v", err)
	}

	threads, err := store.ListThreads()
	if err != nil {
		t.Fatalf("ListThreads returned error: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected one thread in store, got %d", len(threads))
	}
	if threads[0].Label != "Generated title" {
		t.Fatalf("expected generated label to survive stale upsert, got %q", threads[0].Label)
	}
	if threads[0].TitleSource != ThreadTitleSourceLLM {
		t.Fatalf("expected title source %q, got %q", ThreadTitleSourceLLM, threads[0].TitleSource)
	}
	if threads[0].TitleStatus != ThreadTitleStatusReady {
		t.Fatalf("expected title status %q, got %q", ThreadTitleStatusReady, threads[0].TitleStatus)
	}
}

func TestThreadStoreGeneratedTitleDoesNotOverwriteManualTitle(t *testing.T) {
	setRuntimeUserDataDir(t)
	store := newThreadStore()
	now := time.Now().UTC()
	snapshot := storeTitleTestSnapshot(t, "thread_manual", "run_seed", "first message", now)

	if err := store.CreateThread(snapshot, "profile-a", "first message"); err != nil {
		t.Fatalf("CreateThread returned error: %v", err)
	}
	manual, err := store.RenameThread(snapshot.ThreadID, "manual title", now.Add(time.Second))
	if err != nil {
		t.Fatalf("RenameThread returned error: %v", err)
	}
	if manual.Label != "manual title" || manual.TitleSource != ThreadTitleSourceManual {
		t.Fatalf("unexpected manual title snapshot: %+v", manual)
	}

	updated, err := store.UpdateThreadTitleMetadata(snapshot.ThreadID, "Generated title", ThreadTitleSourceLLM, ThreadTitleStatusReady, snapshot.TitleSeedRunID, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("UpdateThreadTitleMetadata returned error: %v", err)
	}
	if updated.Label != "manual title" {
		t.Fatalf("expected manual title to survive generated update, got %q", updated.Label)
	}
	if updated.TitleSource != ThreadTitleSourceManual {
		t.Fatalf("expected title source %q, got %q", ThreadTitleSourceManual, updated.TitleSource)
	}
	if updated.TitleStatus != ThreadTitleStatusReady {
		t.Fatalf("expected title status %q, got %q", ThreadTitleStatusReady, updated.TitleStatus)
	}
}

func TestThreadStoreOpensWithNativeWindowsPath(t *testing.T) {
	userDataDir := t.TempDir()
	t.Setenv(runtimeUserDataDirEnv, userDataDir)

	store := newThreadStore().(*sqliteThreadStore)
	db, err := store.open()
	if err != nil {
		t.Fatalf("open thread store: %v", err)
	}
	defer db.Close()

	var journalMode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("expected wal, got %q", journalMode)
	}
}

func storeTitleTestSnapshot(t *testing.T, threadID, runID, label string, now time.Time) ThreadSnapshot {
	t.Helper()
	root := t.TempDir()
	return ThreadSnapshot{
		ThreadID:       threadID,
		Label:          label,
		TitleSource:    ThreadTitleSourceFirstUser,
		TitleStatus:    ThreadTitleStatusPending,
		TitleUpdatedAt: now,
		TitleSeedRunID: runID,
		RunID:          runID,
		Workspace: WorkspaceConfig{
			WorkspaceRoot: root,
			ScopeID:       "scope_" + threadID,
			ReadableRoots: []string{root},
			WritableRoots: []string{root},
			OutputRoots:   []string{root},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}
