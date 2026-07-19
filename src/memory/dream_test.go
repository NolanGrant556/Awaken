package memory

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"awaken/src/filestore"
	"awaken/src/llm"
)

func TestDreamPromptPreservesDurablePreferencesAndAvoidsChurn(t *testing.T) {
	for _, required := range []string{"strict JSON only", "near duplicates", "Recency alone is not evidence", "forms of address", "Avoid cosmetic rewrites", "empty actions array"} {
		if !strings.Contains(dreamMaintenanceSystemPrompt, required) {
			t.Fatalf("Dream prompt missing %q", required)
		}
	}
}

func TestDreamRunnerRecoversInterruptedTaskAfterRestart(t *testing.T) {
	root := t.TempDir()
	created := time.Now().UTC().Add(-time.Minute)
	task := DreamTask{TaskID: "dream_restart", Status: DreamRunning, Phase: "planning", Scopes: []Scope{ScopeUser}, CreatedAt: created}
	data, _ := json.Marshal(task)
	if err := filestore.WriteFileAtomic(filepath.Join(root, task.TaskID+".json"), data); err != nil {
		t.Fatal(err)
	}
	runner := NewDreamRunnerWithRoot(root)
	recovered, ok := runner.Get(task.TaskID)
	if !ok || recovered.Status != DreamInterrupted || recovered.ErrorCode != "DREAM_INTERRUPTED_RESTART" || recovered.FinishedAt == nil {
		t.Fatalf("unexpected recovered task: %+v", recovered)
	}
	reloaded := NewDreamRunnerWithRoot(root)
	if task, ok := reloaded.Get(task.TaskID); !ok || task.Status != DreamInterrupted {
		t.Fatalf("interrupted receipt was not durable: %+v", task)
	}
}

func TestDreamRunnerRecoversOrphanModelRequestInsideTaskAuthority(t *testing.T) {
	root := t.TempDir()
	task := DreamTask{
		TaskID: "dream_usage_restart", Status: DreamRunning, Phase: "planning",
		Scopes: []Scope{ScopeUser}, CreatedAt: time.Now().UTC().Add(-time.Minute),
		ModelRequests: []DreamModelRequest{{
			RequestID: "dream_request_orphan", Purpose: "memory_maintenance", ModelID: "deepseek-v4-flash",
			ProtocolFamily: "openai_chat", RequestShapeDigest: "digest", CreatedAt: time.Now().UTC().Add(-time.Second),
		}},
	}
	data, _ := json.Marshal(task)
	if err := filestore.WriteFileAtomic(filepath.Join(root, task.TaskID+".json"), data); err != nil {
		t.Fatal(err)
	}
	runner := NewDreamRunnerWithRoot(root)
	recovered, ok := runner.Get(task.TaskID)
	if !ok || len(recovered.ModelUsage) != 1 || recovered.ModelUsage[0].Status != "interrupted" || recovered.ModelUsage[0].ErrorCode != "usage_unknown" {
		t.Fatalf("orphan Dream usage was not recovered: %+v", recovered)
	}
}

func TestDreamRunnerClassifiesCancellationDuringModelResponseAsCancelled(t *testing.T) {
	requestStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		close(requestStarted)
		<-r.Context().Done()
	}))
	defer server.Close()

	store := NewStoreWithRoots(t.TempDir(), t.TempDir())
	if _, err := store.Create(CreateInput{
		Scope: ScopeUser, Type: TypeFeedback, Name: "称呼偏好",
		Description: "称呼用户为 Nolan", Content: "以后称呼用户为 Nolan。",
		Source: Source{ThreadID: "thread", RecordID: "record", CapturedBy: "main_agent"},
	}); err != nil {
		t.Fatal(err)
	}
	runner := NewDreamRunnerWithRoot(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	task, err := runner.Start(ctx, store, llm.Config{
		APIKey: "test-key", BaseURL: server.URL, Model: "test-model", Protocol: llm.ProtocolOpenAIChat,
	}, []Scope{ScopeUser})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("Dream model request did not start")
	}
	cancel()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		current, ok := runner.Get(task.TaskID)
		if ok && current.Status == DreamCancelled {
			if current.ErrorCode != "DREAM_CANCELLED" || current.ErrorMessage != context.Canceled.Error() {
				t.Fatalf("unexpected cancellation receipt: %+v", current)
			}
			return
		}
		if ok && (current.Status == DreamFailed || current.Status == DreamCompleted) {
			t.Fatalf("Dream cancellation was misclassified: %+v", current)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("Dream did not reach cancelled status: %+v", task)
}

func TestMaintenanceRecoveryRollsBackPartiallyPublishedBatch(t *testing.T) {
	store := NewStoreWithRoots(t.TempDir(), t.TempDir())
	source := Source{ThreadID: "thread", RecordID: "record", CapturedBy: "main_agent"}
	record, err := store.Create(CreateInput{Scope: ScopeUser, Type: TypeUser, Name: "称呼", Description: "原始称呼", Content: "称呼用户为博士。", Source: source})
	if err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(record.Path)
	modified := record
	modified.Description = "错误的部分提交"
	modified.Content = "不应被观察到。"
	modified.Revision++
	modified.UpdatedAt = time.Now().UTC()
	after, _ := encodeRecord(modified)
	root := store.UserRoot
	txn := maintenanceTransaction{TransactionID: "txn_recovery", State: "committing", CreatedAt: time.Now().UTC(), Entries: []maintenanceTransactionFile{{Target: filepath.Base(record.Path), BeforeExists: true, StageName: "000.bin"}}}
	txnRoot := filepath.Join(root, ".maintenance-transactions", txn.TransactionID)
	if err := filestore.EnsureDir(filepath.Join(txnRoot, "before")); err != nil {
		t.Fatal(err)
	}
	if err := filestore.EnsureDir(filepath.Join(txnRoot, "after")); err != nil {
		t.Fatal(err)
	}
	if err := filestore.WriteFileAtomic(filepath.Join(txnRoot, "before", "000.bin"), original); err != nil {
		t.Fatal(err)
	}
	if err := filestore.WriteFileAtomic(filepath.Join(txnRoot, "after", "000.bin"), after); err != nil {
		t.Fatal(err)
	}
	if err := writeMaintenanceManifest(filepath.Join(txnRoot, "manifest.json"), txn); err != nil {
		t.Fatal(err)
	}
	marker, _ := json.Marshal(maintenanceMarker{TransactionID: txn.TransactionID})
	if err := filestore.WriteFileAtomic(filepath.Join(root, maintenanceMarkerFilename), marker); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(record.Path, after); err != nil {
		t.Fatal(err)
	}

	recovered, err := store.Get(record.MemoryID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Revision != record.Revision || recovered.Content != record.Content {
		t.Fatalf("partial batch was not rolled back: %+v", recovered)
	}
	if _, err := os.Stat(filepath.Join(root, maintenanceMarkerFilename)); !os.IsNotExist(err) {
		t.Fatalf("maintenance marker survived recovery: %v", err)
	}
}

func TestMaintenanceRecoveryRejectsManifestPathEscape(t *testing.T) {
	store := NewStoreWithRoots(t.TempDir(), t.TempDir())
	root := store.UserRoot
	if err := filestore.EnsureDir(root); err != nil {
		t.Fatal(err)
	}
	txn := maintenanceTransaction{TransactionID: "txn_escape", State: "committing", CreatedAt: time.Now().UTC(), Entries: []maintenanceTransactionFile{{Target: "../../outside.md", BeforeExists: false, StageName: "000.bin"}}}
	txnRoot := filepath.Join(root, ".maintenance-transactions", txn.TransactionID)
	if err := filestore.EnsureDir(filepath.Join(txnRoot, "before")); err != nil {
		t.Fatal(err)
	}
	if err := filestore.EnsureDir(filepath.Join(txnRoot, "after")); err != nil {
		t.Fatal(err)
	}
	if err := filestore.WriteFileAtomic(filepath.Join(txnRoot, "after", "000.bin"), []byte("malicious")); err != nil {
		t.Fatal(err)
	}
	if err := writeMaintenanceManifest(filepath.Join(txnRoot, "manifest.json"), txn); err != nil {
		t.Fatal(err)
	}
	marker, _ := json.Marshal(maintenanceMarker{TransactionID: txn.TransactionID})
	if err := filestore.WriteFileAtomic(filepath.Join(root, maintenanceMarkerFilename), marker); err != nil {
		t.Fatal(err)
	}
	if _, err := store.All(ScopeUser, true); err == nil || !strings.Contains(err.Error(), "escapes scope") {
		t.Fatalf("expected fail-closed path validation, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "outside.md")); !os.IsNotExist(err) {
		t.Fatalf("recovery touched an out-of-scope path: %v", err)
	}
}

func TestApplyMaintenanceUsesExpectedRevisionAndNeverDeletes(t *testing.T) {
	store := NewStoreWithRoots(t.TempDir(), t.TempDir())
	source := Source{ThreadID: "thread", RecordID: "record", CapturedBy: "main_agent"}
	first, err := store.Create(CreateInput{Scope: ScopeUser, Type: TypeUser, Name: "Preference", Description: "Use concise answers", Content: "Prefer concise answers.", Source: source})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Create(CreateInput{Scope: ScopeUser, Type: TypeFeedback, Name: "Old preference", Description: "Use very verbose answers", Content: "Prefer very verbose answers for every request.", Source: source})
	if err != nil {
		t.Fatal(err)
	}
	replacement := "Prefer concise answers unless detail is requested."
	report, err := store.ApplyMaintenance(MaintenancePlan{Scope: ScopeUser, Actions: []MaintenanceAction{
		{Action: "update", MemoryID: first.MemoryID, ExpectedRevision: first.Revision, Patch: Patch{Content: &replacement}, Reason: "clarify preference"},
		{Action: "supersede", MemoryID: second.MemoryID, ExpectedRevision: second.Revision, Reason: "obsolete duplicate"},
	}}, Source{ThreadID: "system_dream", RecordID: "dream_1", CapturedBy: "dream"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Updated != 1 || report.Superseded != 1 {
		t.Fatalf("report=%+v", report)
	}
	updated, _ := store.Get(first.MemoryID)
	superseded, _ := store.Get(second.MemoryID)
	if updated.Revision != 2 || updated.Content != replacement || superseded.Status != StatusSuperseded {
		t.Fatalf("updated=%+v superseded=%+v", updated, superseded)
	}

	_, err = store.ApplyMaintenance(MaintenancePlan{Scope: ScopeUser, Actions: []MaintenanceAction{{Action: "update", MemoryID: first.MemoryID, ExpectedRevision: 1, Patch: Patch{Content: &replacement}, Reason: "stale"}}}, Source{ThreadID: "system_dream", RecordID: "dream_2", CapturedBy: "dream"})
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
	_, err = store.ApplyMaintenance(MaintenancePlan{Scope: ScopeUser, Actions: []MaintenanceAction{{Action: "delete", MemoryID: first.MemoryID, ExpectedRevision: 2, Reason: "not allowed"}}}, Source{ThreadID: "system_dream", RecordID: "dream_3", CapturedBy: "dream"})
	if err == nil {
		t.Fatal("Dream unexpectedly accepted delete")
	}
}
