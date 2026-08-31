package memory

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func createRecallRecords(t *testing.T, store *Store, count int) []Record {
	t.Helper()
	records := make([]Record, 0, count)
	for i := 0; i < count; i++ {
		record, err := store.Create(CreateInput{
			Scope: ScopeProject, Type: TypeProject, Name: fmt.Sprintf("Topic %d", i),
			Description: fmt.Sprintf("Relevant project decision %d", i), Content: fmt.Sprintf("Decision body %d", i),
			Source: Source{ThreadID: "thread", RecordID: fmt.Sprintf("call_%d", i)},
		})
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	return records
}

func waitPending(t *testing.T, manager *RecallManager) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		manager.mu.RLock()
		ready := len(manager.pending) > 0
		manager.mu.RUnlock()
		if ready {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("recall prefetch did not become ready")
}

func TestRecallManagerIsNonBlockingAndPromotesOnlyOnContextBuild(t *testing.T) {
	store := testStore(t)
	records := createRecallRecords(t, store, 1)
	release := make(chan struct{})
	manager := NewRecallManagerWithSelector(store, func(ctx context.Context, _ string, candidates []Candidate) ([]Selection, error) {
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return []Selection{{Scope: ScopeProject, Filename: candidates[0].Filename}}, nil
	})
	started := time.Now()
	manager.StartTurn(context.Background(), "project decision")
	if time.Since(started) > 50*time.Millisecond {
		t.Fatal("StartTurn blocked on selector")
	}
	close(release)
	waitPending(t, manager)
	if len(manager.Receipts()) != 0 {
		t.Fatal("prefetched recall became active before a model context build")
	}
	contextBlock, err := manager.Context()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(contextBlock, records[0].MemoryID) || !strings.Contains(contextBlock, "Decision body 0") {
		t.Fatalf("recall missing from context: %s", contextBlock)
	}
	if len(manager.Receipts()) != 1 {
		t.Fatalf("receipt count = %d", len(manager.Receipts()))
	}
}

func TestRecallManagerCompactedRefsExcludeOldBodiesAndFutureReselection(t *testing.T) {
	store := testStore(t)
	records := createRecallRecords(t, store, 1)
	manager := NewRecallManagerWithSelector(store, func(_ context.Context, _ string, candidates []Candidate) ([]Selection, error) {
		return []Selection{{Scope: candidates[0].Scope, Filename: candidates[0].Filename}}, nil
	})
	manager.StartTurn(context.Background(), "decision")
	waitPending(t, manager)
	before, err := manager.Context()
	if err != nil || !strings.Contains(before, "Decision body 0") {
		t.Fatalf("initial recall missing: context=%q err=%v", before, err)
	}
	manager.MarkCompacted([]string{records[0].MemoryID})
	after, err := manager.Context()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(after, "Decision body 0") || len(manager.Snapshot("workspace").Items) != 0 {
		t.Fatalf("compacted Memory body remained in provider/snapshot projection: %s", after)
	}

	restartedCalls := atomic.Int32{}
	restarted := NewRecallManagerWithSelector(store, func(_ context.Context, _ string, candidates []Candidate) ([]Selection, error) {
		restartedCalls.Add(1)
		return nil, nil
	})
	restarted.MarkCompacted([]string{records[0].MemoryID})
	restarted.StartTurn(context.Background(), "decision")
	time.Sleep(30 * time.Millisecond)
	if restartedCalls.Load() != 0 {
		t.Fatalf("restart reselected an identity restored from compact refs")
	}
}

func TestRecallManagerValidatesAllowlistDeduplicatesAndCapsFive(t *testing.T) {
	store := testStore(t)
	createRecallRecords(t, store, 7)
	var calls atomic.Int32
	manager := NewRecallManagerWithSelector(store, func(_ context.Context, _ string, candidates []Candidate) ([]Selection, error) {
		calls.Add(1)
		selections := []Selection{{Scope: ScopeProject, Filename: "../outside.md"}}
		for _, candidate := range candidates {
			selections = append(selections, Selection{Scope: candidate.Scope, Filename: candidate.Filename})
		}
		return selections, nil
	})
	manager.StartTurn(context.Background(), "decisions")
	waitPending(t, manager)
	if _, err := manager.Context(); err != nil {
		t.Fatal(err)
	}
	if got := len(manager.Receipts()); got != 5 {
		t.Fatalf("recall count = %d, want 5", got)
	}
	manager.StartTurn(context.Background(), "decisions")
	deadline := time.Now().Add(time.Second)
	for calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	manager.mu.RLock()
	pending := len(manager.pending)
	manager.mu.RUnlock()
	if pending > 2 {
		t.Fatalf("already recalled identities were not excluded, pending=%d", pending)
	}
}

func TestRecallManagerEndTurnDiscardsUnconsumedResult(t *testing.T) {
	store := testStore(t)
	createRecallRecords(t, store, 1)
	manager := NewRecallManagerWithSelector(store, func(_ context.Context, _ string, candidates []Candidate) ([]Selection, error) {
		return []Selection{{Scope: ScopeProject, Filename: candidates[0].Filename}}, nil
	})
	manager.StartTurn(context.Background(), "decision")
	waitPending(t, manager)
	manager.EndTurn()
	contextBlock, err := manager.Context()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(contextBlock, "Decision body") || len(manager.Receipts()) != 0 {
		t.Fatalf("late recall survived EndTurn: %s", contextBlock)
	}
}

func TestValidateSelectionsRejectsWrongScopeAndTraversal(t *testing.T) {
	candidates := []Candidate{{Scope: ScopeProject, Filename: "project-topic.md"}}
	got := validateSelections([]Selection{{Scope: ScopeUser, Filename: "project-topic.md"}, {Scope: ScopeProject, Filename: "../outside.md"}, {Scope: ScopeProject, Filename: "project-topic.md"}}, candidates)
	if len(got) != 1 || got[0].Filename != "project-topic.md" || got[0].Scope != ScopeProject {
		t.Fatalf("unexpected validated selections: %+v", got)
	}
}
