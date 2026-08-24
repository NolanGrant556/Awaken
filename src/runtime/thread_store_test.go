package runtime

import (
	"testing"
	"time"
)

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

	threads, err := store.ListThreads(ThreadListViewActive)
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
