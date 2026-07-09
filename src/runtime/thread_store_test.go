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
