package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/filestore"
)

// setFileStoreTestRoots points AWAKEN_HOME at an isolated temp root.
func setFileStoreTestRoots(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv(awakenHomeEnv, root)
	return root
}

func TestStoreRootResolutionPrecedence(t *testing.T) {
	home := t.TempDir()
	userData := t.TempDir()
	t.Setenv(awakenHomeEnv, home)
	t.Setenv(runtimeUserDataDirEnv, userData)
	if got := resolveRuntimeStoreRoot(); got != filepath.Clean(home) {
		t.Fatalf("AWAKEN_HOME must win, got %q", got)
	}
	t.Setenv(awakenHomeEnv, "")
	if got := resolveRuntimeStoreRoot(); got != "" {
		t.Fatalf("Electron userData must not become the business root, got %q", got)
	}
	t.Setenv(runtimeUserDataDirEnv, "")
	if got := resolveRuntimeStoreRoot(); got != "" {
		t.Fatalf("no env must resolve empty (temp fallback), got %q", got)
	}

	t.Setenv(awakenHomeEnv, home)
	store, ok := newThreadStore().(*FileDesktopStore)
	if !ok {
		t.Fatalf("expected FileDesktopStore")
	}
	if _, err := os.Stat(filepath.Join(home, "CURRENT")); err != nil {
		t.Fatalf("CURRENT pointer missing after init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "store-generations", fileStoreGenerationID, "FORMAT.json")); err != nil {
		t.Fatalf("FORMAT.json missing after init: %v", err)
	}
	_ = store
}

func TestFileStoreBrokenCurrentPointerFailsClosed(t *testing.T) {
	root := t.TempDir()
	if _, err := filestore.InitGeneration(root, "gen-real", filestore.Format{}); err != nil {
		t.Fatal(err)
	}
	// Generations exist but CURRENT is missing: must fail closed rather than
	// guess a latest generation or initialize an empty store.
	if _, err := newFileDesktopStore(root); err == nil {
		t.Fatalf("missing CURRENT with existing generations must fail closed")
	}
}

func TestFileStoreThreadCRUDListOrderAndTitleCAS(t *testing.T) {
	setFileStoreTestRoots(t)
	store, ok := newThreadStore().(*FileDesktopStore)
	if !ok {
		t.Fatal("expected FileDesktopStore")
	}
	workspace := t.TempDir()
	base := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	for i := 1; i <= 3; i++ {
		snapshot := ThreadSnapshot{
			ThreadID:       fmt.Sprintf("thread_%d", i),
			Label:          fmt.Sprintf("thread %d", i),
			TitleSource:    ThreadTitleSourceFirstUser,
			TitleStatus:    ThreadTitleStatusPending,
			TitleSeedRunID: fmt.Sprintf("run_%d", i),
			RunID:          fmt.Sprintf("run_%d", i),
			Workspace:      WorkspaceConfig{WorkspaceRoot: workspace},
			CreatedAt:      base.Add(time.Duration(i) * time.Minute),
			UpdatedAt:      base.Add(time.Duration(i) * time.Minute),
		}
		if err := store.CreateThread(snapshot, "profile-key", "preview "+snapshot.ThreadID); err != nil {
			t.Fatalf("create %s: %v", snapshot.ThreadID, err)
		}
	}

	threads, err := store.ListThreads()
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 3 || threads[0].ThreadID != "thread_3" || threads[2].ThreadID != "thread_1" {
		t.Fatalf("unexpected list order: %+v", threads)
	}

	// LLM title lands only on the pending first_user seed for the same run.
	updated, err := store.UpdateThreadTitleMetadata("thread_1", "自动标题", ThreadTitleSourceLLM, ThreadTitleStatusReady, "run_1", base.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Label != "自动标题" || updated.TitleSource != ThreadTitleSourceLLM {
		t.Fatalf("LLM title not applied: %+v", updated)
	}
	// A second write with a stale seed must be a no-op CAS miss.
	unchanged, err := store.UpdateThreadTitleMetadata("thread_1", "迟到标题", ThreadTitleSourceLLM, ThreadTitleStatusReady, "run_1", base.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Label != "自动标题" {
		t.Fatalf("decided title must not be overwritten: %+v", unchanged)
	}

	// Manual rename wins and survives a later first_user/pending upsert.
	renamed, err := store.RenameThread("thread_2", "手动标题", base.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if renamed.TitleSource != ThreadTitleSourceManual || renamed.Label != "手动标题" {
		t.Fatalf("manual rename failed: %+v", renamed)
	}
	if err := store.CreateThread(ThreadSnapshot{
		ThreadID:    "thread_2",
		Label:       "later prompt",
		TitleSource: ThreadTitleSourceFirstUser,
		TitleStatus: ThreadTitleStatusPending,
		Workspace:   WorkspaceConfig{WorkspaceRoot: workspace},
		UpdatedAt:   base.Add(3 * time.Hour),
	}, "", ""); err != nil {
		t.Fatal(err)
	}
	record, err := store.threadRecord("thread_2")
	if err != nil {
		t.Fatal(err)
	}
	if record.Snapshot.Label != "手动标题" || record.Snapshot.TitleSource != ThreadTitleSourceManual {
		t.Fatalf("first_user/pending upsert downgraded a manual title: %+v", record.Snapshot)
	}

	if err := store.DeleteThread("thread_3"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.threadRecord("thread_3"); !errors.Is(err, ErrThreadNotFound) {
		t.Fatalf("deleted thread must be gone, got %v", err)
	}
	threads, err = store.ListThreads()
	if err != nil || len(threads) != 2 {
		t.Fatalf("expected 2 threads after delete, got %+v err=%v", threads, err)
	}
}

func TestFileStoreStructuredInputSurvivesServiceRestart(t *testing.T) {
	setFileStoreTestRoots(t)
	workspace := t.TempDir()

	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer model.Close()

	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				MaxThoughts: 1,
				ModelConfig: agentcore.ModelConfig{
					APIKey:  "test-key",
					BaseURL: model.URL,
					Model:   "test-model",
				},
				Workspace: agentcore.WorkspaceConfig{Root: workspace.WorkspaceRoot},
			}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				return agent.RunStreamWithEvents(ctx, input, sink)
			})
		},
	)
	if service.ProjectService() == nil {
		t.Fatal("FileStore must register the project service (route parity)")
	}

	leadingText := "先分析财务风险，然后使用 "
	token := ComposerToken{
		ID:          "prompt:pptx:user",
		Type:        "prompt",
		LeadingText: leadingText,
		Label:       "pptx",
		Payload:     ComposerTokenPayload{SkillID: "pptx", SourceOrigin: "user"},
	}
	run, err := service.StartRun(StartRunRequest{
		Input:          leadingText + "输出报告",
		ComposerTokens: []ComposerToken{token},
		Workspace:      WorkspaceConfig{WorkspaceRoot: workspace},
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	waitForRunTerminal(t, service, run.RunID)

	restarted := NewRunService(nil, nil)
	transcript, err := restarted.GetThreadTranscript(run.ThreadID)
	if err != nil {
		t.Fatalf("GetThreadTranscript after restart: %v", err)
	}
	if len(transcript.Runs) != 1 {
		t.Fatalf("expected one durable run, got %+v", transcript.Runs)
	}
	replayed := transcript.Runs[0].Run
	if len(replayed.ComposerTokens) != 1 || replayed.ComposerTokens[0] != token {
		t.Fatalf("composer token changed after restart: %+v", replayed.ComposerTokens)
	}
	threads := restarted.ListThreads()
	if len(threads) != 1 || threads[0].ThreadID != run.ThreadID {
		t.Fatalf("thread listing after restart: %+v", threads)
	}
}

func TestFileStoreCompactedContextSurvivesServiceRestart(t *testing.T) {
	setFileStoreTestRoots(t)
	workspace := t.TempDir()
	threadID := "thread_compacted_restart"
	oldRunID := "run_before_restart"
	store := newThreadStore().(*FileDesktopStore)
	now := time.Now().UTC()
	if err := store.CreateThread(ThreadSnapshot{
		ThreadID:    threadID,
		Label:       "compacted thread",
		RunID:       oldRunID,
		Workspace:   WorkspaceConfig{WorkspaceRoot: workspace},
		CreatedAt:   now,
		UpdatedAt:   now,
		TitleSource: ThreadTitleSourceFirstUser,
		TitleStatus: ThreadTitleStatusReady,
	}, "", ""); err != nil {
		t.Fatal(err)
	}

	journal, err := NewExecutionJournal(store.ThreadTranscriptPath(threadID), threadID)
	if err != nil {
		t.Fatal(err)
	}
	records := []agentcore.SemanticRecord{
		agentcore.NewSemanticRecord(agentcore.SemanticKindThread, threadID, "", "", "", "", map[string]any{"label": "compacted thread"}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindUserMessage, threadID, oldRunID, "turn_old", "", "", map[string]any{"content": "OLD USER MESSAGE MUST DISAPPEAR"}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindAssistantMessage, threadID, oldRunID, "turn_old", "", "", map[string]any{"content": "OLD ASSISTANT MESSAGE MUST DISAPPEAR"}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindContextCompacted, threadID, oldRunID, "turn_old", "", "", map[string]any{
			"compact_boundary_id": "compact_restart",
			"mode":                string(agentcore.CompactModeAuto),
			"messages": []agentcore.SemanticCompactedMessage{
				{Role: string(convcontext.RoleUser), Content: "summary after compaction", CompactBoundaryID: "compact_restart"},
				{Role: string(convcontext.RoleAssistant), Content: "retained tail"},
			},
		}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindRunTerminal, threadID, oldRunID, "turn_old", "", "", map[string]any{"status": "finished", "result": "done"}),
	}
	if err := journal.AppendBatch(context.Background(), records); err != nil {
		t.Fatal(err)
	}
	if err := journal.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	restoredMessages := make(chan []convcontext.Message, 1)
	restarted := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				Workspace: agentcore.WorkspaceConfig{Root: workspace.WorkspaceRoot},
			}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(context.Context, string, agentcore.EventSink) (string, error) {
				restoredMessages <- agent.GetConversation().Context.GetMessages()
				return "follow-up complete", nil
			})
		},
	)
	run, err := restarted.StartRun(StartRunRequest{
		ThreadID: threadID,
		Input:    "follow up",
		Workspace: WorkspaceConfig{
			WorkspaceRoot: workspace,
		},
	})
	if err != nil {
		t.Fatalf("StartRun after restart: %v", err)
	}
	waitForRunTerminal(t, restarted, run.RunID)

	var got []convcontext.Message
	select {
	case got = <-restoredMessages:
	case <-time.After(2 * time.Second):
		t.Fatal("follow-up executor did not observe restored context")
	}
	if len(got) != 2 || got[0].Content != "summary after compaction" || got[0].CompactBoundaryID != "compact_restart" || got[1].Content != "retained tail" {
		t.Fatalf("restart did not restore summary + tail: %+v", got)
	}
	for _, message := range got {
		if strings.Contains(message.Content, "MUST DISAPPEAR") {
			t.Fatalf("pre-compaction history leaked into follow-up context: %+v", got)
		}
	}
}

func TestFileStoreDetachKeepsHistoryAndReAddReusesProjectID(t *testing.T) {
	setFileStoreTestRoots(t)
	workspaceRoot := t.TempDir()
	service := newDurableThreadTestService()
	projects := service.ProjectService()
	if projects == nil {
		t.Fatal("project service missing under FileStore")
	}
	project, err := projects.UpsertProject(workspaceRoot, "demo")
	if err != nil {
		t.Fatal(err)
	}
	createProjectTestThread(t, service, workspaceRoot, "thread_detach")

	first, err := projects.DetachProject(project.ProjectID, "detach-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := projects.DetachProject(project.ProjectID, "detach-1")
	if err != nil {
		t.Fatal(err)
	}
	if first.OperationID != second.OperationID {
		t.Fatalf("detach must be idempotent: %q vs %q", first.OperationID, second.OperationID)
	}
	if got := service.ListThreads(); len(got) != 1 {
		t.Fatalf("detach must keep thread history, got %+v", got)
	}
	visible, err := projects.ListProjects(false)
	if err != nil || len(visible) != 0 {
		t.Fatalf("detached project must be hidden, got %+v err=%v", visible, err)
	}
	all, err := projects.ListProjects(true)
	if err != nil || len(all) != 1 || all[0].State != ProjectStateRemoved || all[0].RemovalReason != "detached" {
		t.Fatalf("detached durable state wrong: %+v err=%v", all, err)
	}

	// Detach fences new runs; explicit re-add restores the same project ID.
	if _, err := service.StartRun(StartRunRequest{
		Input:     "must be fenced",
		Workspace: WorkspaceConfig{WorkspaceRoot: workspaceRoot},
	}); err == nil {
		t.Fatalf("detached project must fence new runs")
	}
	readded, err := projects.UpsertProject(workspaceRoot, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if readded.ProjectID != project.ProjectID || readded.State != ProjectStateActive {
		t.Fatalf("re-add must reuse the original project id, got %+v", readded)
	}
	if got := service.ListThreads(); len(got) != 1 {
		t.Fatalf("re-add must restore thread history, got %+v", got)
	}
	if _, err := projects.DeleteProjectThreads(project.ProjectID, "detach-1"); !errors.Is(err, ErrIdempotencyKeyConflict) {
		t.Fatalf("expected idempotency conflict for reused key, got %v", err)
	}
}

func TestFileStoreProjectOrderPersistsAcrossRestart(t *testing.T) {
	root := setFileStoreTestRoots(t)
	store, err := newFileDesktopStore(root)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
	first, err := store.EnsureProjectActive(t.TempDir(), "first", base)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.EnsureProjectActive(t.TempDir(), "second", base.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	third, err := store.EnsureProjectActive(t.TempDir(), "third", base.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	wanted := []string{first.ProjectID, third.ProjectID, second.ProjectID}
	projects, err := store.ReorderProjects(wanted, base.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if got := projectIDs(projects); strings.Join(got, ",") != strings.Join(wanted, ",") {
		t.Fatalf("unexpected project order: got %v want %v", got, wanted)
	}
	if _, err := store.ReorderProjects([]string{first.ProjectID, second.ProjectID}, base.Add(4*time.Minute)); !errors.Is(err, ErrProjectOrderInvalid) {
		t.Fatalf("incomplete project order must fail, got %v", err)
	}
	if _, err := store.ReorderProjects([]string{first.ProjectID, first.ProjectID, second.ProjectID}, base.Add(4*time.Minute)); !errors.Is(err, ErrProjectOrderInvalid) {
		t.Fatalf("duplicate project order must fail, got %v", err)
	}

	reopened, err := newFileDesktopStore(root)
	if err != nil {
		t.Fatal(err)
	}
	restartedProjects, err := reopened.ListProjects(false)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectIDs(restartedProjects); strings.Join(got, ",") != strings.Join(wanted, ",") {
		t.Fatalf("project order did not survive restart: got %v want %v", got, wanted)
	}

	newest, err := reopened.EnsureProjectActive(t.TempDir(), "newest", base.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	withNewest, err := reopened.ListProjects(false)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectIDs(withNewest); len(got) != 4 || got[0] != newest.ProjectID || strings.Join(got[1:], ",") != strings.Join(wanted, ",") {
		t.Fatalf("new project must prepend without disturbing saved order: got %v", got)
	}
}

func projectIDs(projects []Project) []string {
	ids := make([]string, 0, len(projects))
	for _, project := range projects {
		ids = append(ids, project.ProjectID)
	}
	return ids
}

func TestFileStoreDeleteHistorySoftDeletesAndCleanupPurges(t *testing.T) {
	setFileStoreTestRoots(t)
	workspaceRoot := t.TempDir()
	service := newDurableThreadTestService()
	projects := service.ProjectService()
	project, err := projects.UpsertProject(workspaceRoot, "bulk")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		createProjectTestThread(t, service, workspaceRoot, fmt.Sprintf("thread_bulk_%d", i))
	}

	operation, err := projects.DeleteProjectThreads(project.ProjectID, "delete-history-1")
	if err != nil {
		t.Fatal(err)
	}
	if operation.MatchedThreads != 5 || operation.PendingCleanup != 5 {
		t.Fatalf("unexpected operation counts: %+v", operation)
	}
	if got := service.ListThreads(); len(got) != 0 {
		t.Fatalf("soft-deleted threads must disappear from listing, got %+v", got)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		processed, err := projects.ProcessCleanupJobs(context.Background(), 16)
		if err != nil {
			t.Fatal(err)
		}
		final, err := projects.GetOperation(operation.OperationID)
		if err != nil {
			t.Fatal(err)
		}
		if final.Status == ProjectOperationSucceeded && final.Phase == projectOperationPhaseCompleted && final.PendingCleanup == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("cleanup did not complete: processed=%d op=%+v", processed, final)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Session directories are physically purged.
	store := service.threadStore.(*FileDesktopStore)
	for i := 0; i < 5; i++ {
		dir := store.sessionDir(fmt.Sprintf("thread_bulk_%d", i))
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("session dir %s must be purged, err=%v", dir, err)
		}
	}
}

func TestFileStoreTrashLifecycleHappyPathAndFailureRestore(t *testing.T) {
	setFileStoreTestRoots(t)
	parent := t.TempDir()
	workspaceRoot := filepath.Join(parent, "victim")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	service := newDurableThreadTestService()
	projects := service.ProjectService()
	project, err := projects.UpsertProject(workspaceRoot, "victim")
	if err != nil {
		t.Fatal(err)
	}
	createProjectTestThread(t, service, workspaceRoot, "thread_trash")

	prepared, err := projects.PrepareTrash(project.ProjectID, workspaceRoot, "trash-1")
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Phase != projectOperationPhaseWaitingForRuns {
		t.Fatalf("unexpected prepare phase: %+v", prepared)
	}
	if ok, _ := projects.CanStartRun(workspaceRoot); ok {
		t.Fatalf("trash_pending must block new runs")
	}

	claimed, err := projects.ClaimTrashOperation(prepared.OperationID, "test-worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.AttemptID == "" || claimed.Phase != projectOperationPhaseTrashClaimed {
		t.Fatalf("claim failed: %+v", claimed)
	}
	staged, err := projects.StageTrashOperation(prepared.OperationID, claimed.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if staged.TrashStagingPath == "" || staged.Phase != projectOperationPhaseTrashStaged {
		t.Fatalf("stage failed: %+v", staged)
	}
	if _, err := os.Stat(workspaceRoot); !os.IsNotExist(err) {
		t.Fatalf("workspace must be moved to staging, err=%v", err)
	}
	begun, err := projects.BeginTrashInvocation(prepared.OperationID, claimed.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if begun.Phase != projectOperationPhaseTrashing || begun.TrashInvokedAt == nil {
		t.Fatalf("begin failed: %+v", begun)
	}
	// Simulate the platform trash call consuming the staged directory.
	if err := os.RemoveAll(staged.TrashStagingPath); err != nil {
		t.Fatal(err)
	}
	completed, err := projects.CompleteTrashOperation(prepared.OperationID, claimed.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != ProjectOperationAccepted && completed.Status != ProjectOperationSucceeded {
		t.Fatalf("complete failed: %+v", completed)
	}
	// Trash is folder-scoped: the directory is gone and the folder is detached,
	// but the project survives on its Awaken-managed working directory.
	afterTrash, err := projects.ListProjects(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterTrash) != 1 || afterTrash[0].State != ProjectStateActive {
		t.Fatalf("folder trash must keep the project active: %+v", afterTrash)
	}
	if afterTrash[0].HasFolder(workspaceRoot) {
		t.Fatalf("trashed folder must be detached: %+v", afterTrash[0].Folders)
	}
	if _, ok := afterTrash[0].PrimaryFolder(); ok {
		t.Fatalf("trashing the only folder must leave the project folderless: %+v", afterTrash[0].Folders)
	}
	if got := service.ListThreads(); len(got) != 1 {
		t.Fatalf("folder trash must keep Awaken history, got %+v", got)
	}

	// Failure path on a second project restores the previous state and
	// returns the staged directory to its original location.
	secondRoot := filepath.Join(parent, "restored")
	if err := os.MkdirAll(secondRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	second, err := projects.UpsertProject(secondRoot, "restored")
	if err != nil {
		t.Fatal(err)
	}
	preparedSecond, err := projects.PrepareTrash(second.ProjectID, secondRoot, "trash-2")
	if err != nil {
		t.Fatal(err)
	}
	claimedSecond, err := projects.ClaimTrashOperation(preparedSecond.OperationID, "test-worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projects.StageTrashOperation(preparedSecond.OperationID, claimedSecond.AttemptID); err != nil {
		t.Fatal(err)
	}
	failed, err := projects.FailTrashOperation(preparedSecond.OperationID, claimedSecond.AttemptID, "TRASH_FAILED", "platform trash unavailable")
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != ProjectOperationFailed || failed.Phase != projectOperationPhaseFailed {
		t.Fatalf("fail transition wrong: %+v", failed)
	}
	if _, err := os.Stat(secondRoot); err != nil {
		t.Fatalf("failed trash must restore the workspace directory: %v", err)
	}
	restored, err := projects.ListProjects(false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range restored {
		if p.ProjectID == second.ProjectID && p.State == ProjectStateActive {
			found = true
		}
	}
	if !found {
		t.Fatalf("failed trash must restore active project state: %+v", restored)
	}
	if ok, _ := projects.CanStartRun(secondRoot); !ok {
		t.Fatalf("restored project must accept runs again")
	}
}

func TestFileStoreLifecycleJournalRollForwardAndSnapshotRebuild(t *testing.T) {
	root := t.TempDir()
	t.Setenv(awakenHomeEnv, t.TempDir())
	store, err := newFileDesktopStore(root)
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(t.TempDir(), "ws")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	project, err := store.EnsureProjectActive(normalizeProjectWorkspaceRoot(workspace), "ws", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}

	// Crash between journal append and snapshot write: append a mutation
	// directly to the journal without touching state.json.
	detachTime := time.Now().UTC()
	removed := project
	removed.State = ProjectStateRemoved
	removed.RemovalReason = "detached"
	removed.RemovedAt = &detachTime
	removed.UpdatedAt = detachTime
	removed.Version++
	journal, _, err := filestore.OpenJournal(filepath.Join(store.gen, "lifecycle", "mutations.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Append(fileLifecycleMutation{
		Kind:     "CommitDetach",
		Now:      detachTime,
		Projects: []Project{removed},
	}); err != nil {
		t.Fatal(err)
	}
	journal.Close()

	reopened, err := newFileDesktopStore(root)
	if err != nil {
		t.Fatalf("roll forward on reopen: %v", err)
	}
	got, err := reopened.ProjectByID(project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != ProjectStateRemoved || got.RemovalReason != "detached" {
		t.Fatalf("journal record must be rolled forward into state: %+v", got)
	}

	// Corrupt snapshot: rebuilt from the journal, which stays authoritative.
	statePath := filepath.Join(reopened.gen, "lifecycle", "state.json")
	if err := os.WriteFile(statePath, []byte("{corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := newFileDesktopStore(root)
	if err != nil {
		t.Fatalf("rebuild from journal: %v", err)
	}
	got, err = rebuilt.ProjectByID(project.ProjectID)
	if err != nil || got.State != ProjectStateRemoved {
		t.Fatalf("state must rebuild from journal: %+v err=%v", got, err)
	}

	// Verify the snapshot was re-persisted as valid JSON.
	var snapshot fileLifecycleState
	raw, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatalf("re-persisted snapshot unreadable: %v", err)
	}
	if snapshot.AppliedSeq == 0 {
		t.Fatalf("snapshot applied seq missing: %+v", snapshot)
	}
}
