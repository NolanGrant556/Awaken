package runtime

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"awaken/src/agentcore"
)

func TestFileStoreSharedFolderThreadKeepsExplicitProject(t *testing.T) {
	home := setFileStoreTestRoots(t)
	store, err := newFileDesktopStore(home)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	a, err := store.CreateProject([]ProjectFolder{{Path: root, IsPrimary: true}}, "A", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.CreateProject([]ProjectFolder{{Path: root, IsPrimary: true}}, "B", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateThread(ThreadSnapshot{ThreadID: "audit-thread-a", ProjectID: a.ProjectID, Workspace: WorkspaceConfig{WorkspaceRoot: root, ProjectID: a.ProjectID}, CreatedAt: time.Now()}, "", ""); err != nil {
		t.Fatal(err)
	}
	threads, err := store.ListThreads(ThreadListViewAll)
	if err != nil {
		t.Fatal(err)
	}
	got := threads[0]
	t.Logf("requested=A(%s), other=B(%s), stored project=%s, stored workspace.project=%s", a.ProjectID, b.ProjectID, got.ProjectID, got.Workspace.ProjectID)
	if got.ProjectID != a.ProjectID {
		t.Errorf("thread assigned to another project")
	}
	victims, err := store.liveThreadIDsForProjectLocked(b.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(victims) > 0 {
		t.Errorf("deleting B history would select A thread: %v", victims)
	}
}

func TestFileStoreFolderlessProjectDoesNotCreatePhantom(t *testing.T) {
	home := setFileStoreTestRoots(t)
	store, err := newFileDesktopStore(home)
	if err != nil {
		t.Fatal(err)
	}
	p, err := store.CreateProject(nil, "MyProject", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	w, err := normalizeWorkspace(WorkspaceConfig{ProjectID: p.ProjectID})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateThread(ThreadSnapshot{ThreadID: "audit-folderless", ProjectID: p.ProjectID, Workspace: w, CreatedAt: time.Now()}, "", ""); err != nil {
		t.Fatal(err)
	}
	ps, err := store.ListProjects(false)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := store.ListThreads(ThreadListViewAll)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("project count=%d, expected=%s actual=%s", len(ps), p.ProjectID, ts[0].ProjectID)
	if len(ps) != 1 || ts[0].ProjectID != p.ProjectID {
		t.Errorf("folderless thread created a phantom project")
	}
}

func TestFileStoreSequentialStoreWritersRetainBothProjects(t *testing.T) {
	home := setFileStoreTestRoots(t)
	a, err := newFileDesktopStore(home)
	if err != nil {
		t.Fatal(err)
	}
	b, err := newFileDesktopStore(home)
	if err != nil {
		t.Fatal(err)
	}
	pa, err := a.CreateProject(nil, "A", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.CreateProject(nil, "B", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	c, err := newFileDesktopStore(home)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := c.ListProjects(false)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("reopened count=%d, journal seq=%d, snapshot applied=%d", len(ps), c.journal.LastSeq(), c.state.AppliedSeq)
	if _, err := c.ProjectByID(pa.ProjectID); err != nil {
		t.Errorf("first writer project disappeared on reopen: %v", err)
	}
}

func TestFileStoreSharedProjectOwnershipThroughStartRunAndRestart(t *testing.T) {
	setFileStoreTestRoots(t)
	root := t.TempDir()
	service := NewRunService(func(w WorkspaceConfig, id string) *agentcore.Agent {
		return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{Workspace: agentcore.WorkspaceConfig{Root: w.WorkspaceRoot}}, id)
	}, func(_ *agentcore.Agent) Executor {
		return ExecutorFunc(func(_ context.Context, _ string, _ agentcore.EventSink) (string, error) { return "done", nil })
	})
	t.Cleanup(service.drainRunWorkers)
	a, err := service.ProjectService().CreateProject([]ProjectFolder{{Path: root, IsPrimary: true}}, "A")
	if err != nil {
		t.Fatal(err)
	}
	b, err := service.ProjectService().CreateProject([]ProjectFolder{{Path: root, IsPrimary: true}}, "B")
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.StartRun(StartRunRequest{Input: "Audit ownership", Workspace: WorkspaceConfig{WorkspaceRoot: root, ProjectID: a.ProjectID, ProjectName: "A"}})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunTerminal(t, service, run.RunID)
	service.drainRunWorkers()
	if err := service.FlushTranscripts(); err != nil {
		t.Fatal(err)
	}
	live := service.ListThreads(ThreadListViewAll)
	restarted := NewRunService(nil, nil)
	cold := restarted.ListThreads(ThreadListViewAll)
	t.Logf("live=%s requested=%s other=%s cold=%s", live[0].ProjectID, a.ProjectID, b.ProjectID, cold[0].ProjectID)
	if cold[0].ProjectID != a.ProjectID {
		t.Errorf("StartRun accepts A, but cold restart assigns its thread to B")
	}
}

func TestFileStoreRebuildsValidButIncompleteSnapshot(t *testing.T) {
	home := setFileStoreTestRoots(t)
	store, err := newFileDesktopStore(home)
	if err != nil {
		t.Fatal(err)
	}
	a, err := store.CreateProject(nil, "A", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateProject(nil, "B", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	delete(store.state.Projects, a.ProjectID)
	if err := store.persistState(); err != nil {
		t.Fatal(err)
	}
	reopened, err := newFileDesktopStore(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.ProjectByID(a.ProjectID); err != nil {
		t.Fatalf("journal project omitted by snapshot was lost: %v", err)
	}
	var repaired fileLifecycleState
	raw, err := os.ReadFile(store.statePath())
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &repaired); err != nil {
		t.Fatal(err)
	}
	if len(repaired.Projects) != 2 || repaired.AppliedSeq != 2 {
		t.Fatalf("snapshot not repaired: %+v", repaired)
	}
}

func TestFileStoreReadersAndWritersCatchUpBeforeUsingProjectState(t *testing.T) {
	home := setFileStoreTestRoots(t)
	a, err := newFileDesktopStore(home)
	if err != nil {
		t.Fatal(err)
	}
	b, err := newFileDesktopStore(home)
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.CreateProject(nil, "original", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.ProjectByID(p.ProjectID); err != nil {
		t.Fatalf("stale reader missed project: %v", err)
	}
	firstRoot := t.TempDir()
	if _, err := a.AttachProjectFolder(p.ProjectID, firstRoot, time.Now()); err != nil {
		t.Fatal(err)
	}
	updated, err := b.RenameProject(p.ProjectID, "renamed", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !updated.HasFolder(firstRoot) || updated.Version != 3 {
		t.Fatalf("rename lost newer folder: %+v", updated)
	}
	got, err := a.ProjectByID(p.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DisplayName != "renamed" || !got.HasFolder(firstRoot) {
		t.Fatalf("reader stayed stale: %+v", got)
	}
}

func TestFileStoreThreadOwnerSurvivesFolderChangesAndRepairsOldProjection(t *testing.T) {
	home := setFileStoreTestRoots(t)
	store, err := newFileDesktopStore(home)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	a, err := store.CreateProject([]ProjectFolder{{Path: root, IsPrimary: true}}, "A", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.CreateProject([]ProjectFolder{{Path: root, IsPrimary: true}}, "B", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := ThreadSnapshot{ThreadID: "stable-owner", Workspace: WorkspaceConfig{WorkspaceRoot: root, ProjectID: a.ProjectID}, CreatedAt: time.Now()}
	if err := store.CreateThread(snapshot, "", ""); err != nil {
		t.Fatal(err)
	}
	// Emulate metadata produced by the older path-based writer.
	meta, err := store.readSessionMetadata(snapshot.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	meta.ProjectID = b.ProjectID
	if err := store.writeSessionMetadata(meta); err != nil {
		t.Fatal(err)
	}
	listed, err := store.ListThreads(ThreadListViewAll)
	if err != nil {
		t.Fatal(err)
	}
	if listed[0].ProjectID != a.ProjectID {
		t.Fatalf("old metadata projected wrong owner: %+v", listed)
	}
	if ids, err := store.liveThreadIDsForProjectLocked(b.ProjectID); err != nil || len(ids) != 0 {
		t.Fatalf("wrong delete scope: %v %v", ids, err)
	}
	if _, err := store.DetachProjectFolder(a.ProjectID, root, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateThread(snapshot, "", "updated"); err != nil {
		t.Fatal(err)
	}
	meta, err = store.readSessionMetadata(snapshot.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ProjectID != a.ProjectID || meta.Workspace.ProjectID != a.ProjectID {
		t.Fatalf("upsert changed owner: %+v", meta)
	}
	snapshot.Workspace.ProjectID = b.ProjectID
	if err := store.CreateThread(snapshot, "", ""); err == nil {
		t.Fatal("existing thread was reassigned")
	}
}
