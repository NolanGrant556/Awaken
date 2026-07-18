package memory

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	base := t.TempDir()
	store := NewStoreWithRoots(filepath.Join(base, "user"), filepath.Join(base, "project"))
	fixed := time.Date(2026, 7, 18, 3, 4, 5, 0, time.UTC)
	store.Now = func() time.Time { return fixed }
	return store
}

func testCreate(scope Scope) CreateInput {
	return CreateInput{Scope: scope, Type: TypeFeedback, Name: "Database testing", Description: "Integration tests use a real database", Content: "Use a real database for integration tests.", Keywords: []string{"database", "integration"}, Source: Source{ThreadID: "thread_test", RecordID: "call_test", CapturedBy: "main_agent"}}
}

func TestStoreCreateWritesMarkdownAndIndex(t *testing.T) {
	store := testStore(t)
	record, err := store.Create(testCreate(ScopeProject))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(record.Path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"---\n", "schema_version: 1", "memory_id: " + record.MemoryID, "source_thread_id: thread_test", "Use a real database"} {
		if !strings.Contains(text, want) {
			t.Fatalf("topic missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "scope:") || strings.Contains(text, "source_run_id:") {
		t.Fatalf("derived fields leaked into frontmatter:\n%s", text)
	}
	index, err := store.Index(ScopeProject)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(index, "[Database testing]") || !strings.Contains(index, filepath.Base(record.Path)) {
		t.Fatalf("unexpected index: %s", index)
	}
	info, err := os.Stat(record.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("topic mode = %o", info.Mode().Perm())
	}
}

func TestStoreUpdateRevisionSupersedeAndDelete(t *testing.T) {
	store := testStore(t)
	record, err := store.Create(testCreate(ScopeUser))
	if err != nil {
		t.Fatal(err)
	}
	value := "Prefer real database integration tests"
	updated, err := store.Update(record.MemoryID, 1, Patch{Description: &value}, Source{ThreadID: "thread_test", RecordID: "call_update"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || updated.Description != value {
		t.Fatalf("unexpected update: %+v", updated)
	}
	if _, err := store.Update(record.MemoryID, 1, Patch{Description: &value}, Source{ThreadID: "thread_test", RecordID: "call_stale"}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
	superseded, err := store.Supersede(record.MemoryID, 2, Source{ThreadID: "thread_test", RecordID: "call_supersede"})
	if err != nil {
		t.Fatal(err)
	}
	if superseded.Status != StatusSuperseded || superseded.Revision != 3 {
		t.Fatalf("unexpected supersede: %+v", superseded)
	}
	index, err := store.Index(ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(index, record.MemoryID) || strings.Contains(index, "Database testing") {
		t.Fatalf("superseded record remains indexed: %s", index)
	}
	if err := store.Delete(record.MemoryID, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(record.MemoryID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected deleted record to be absent, got %v", err)
	}
}

func TestStoreRejectsSensitiveDuplicateAndMissingHostSource(t *testing.T) {
	store := testStore(t)
	if _, err := store.Create(testCreate(ScopeProject)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(testCreate(ScopeProject)); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("expected duplicate rejection, got %v", err)
	}
	sensitive := testCreate(ScopeUser)
	sensitive.Content = "Authorization: Bearer abcdef"
	if _, err := store.Create(sensitive); !errors.Is(err, ErrSensitive) {
		t.Fatalf("expected sensitive rejection, got %v", err)
	}
	missing := testCreate(ScopeUser)
	missing.Source = Source{}
	if _, err := store.Create(missing); err == nil || !strings.Contains(err.Error(), "host source identity") {
		t.Fatalf("expected source rejection, got %v", err)
	}
}

func TestStoreRebuildsMissingIndexFromTopics(t *testing.T) {
	store := testStore(t)
	record, err := store.Create(testCreate(ScopeProject))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(store.ProjectRoot, IndexFilename)); err != nil {
		t.Fatal(err)
	}
	index, err := store.Index(ScopeProject)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(index, filepath.Base(record.Path)) {
		t.Fatalf("rebuilt index missing topic: %s", index)
	}
}

func TestStoreIgnoresSymlinkTopicEscape(t *testing.T) {
	store := testStore(t)
	if err := os.MkdirAll(store.ProjectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(store.ProjectRoot, "project-escape.md")); err != nil {
		t.Fatal(err)
	}
	records, err := store.Search("", nil, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("symlink topic was scanned: %+v", records)
	}
}

func TestStoreRevisionCASIsSharedAcrossInstances(t *testing.T) {
	base := t.TempDir()
	userRoot := filepath.Join(base, "user")
	projectRoot := filepath.Join(base, "project")
	first := NewStoreWithRoots(userRoot, projectRoot)
	second := NewStoreWithRoots(userRoot, projectRoot)
	record, err := first.Create(testCreate(ScopeProject))
	if err != nil {
		t.Fatal(err)
	}
	one, two := "first", "second"
	errorsCh := make(chan error, 2)
	go func() {
		_, err := first.Update(record.MemoryID, 1, Patch{Description: &one}, Source{ThreadID: "thread", RecordID: "call_one"})
		errorsCh <- err
	}()
	go func() {
		_, err := second.Update(record.MemoryID, 1, Patch{Description: &two}, Source{ThreadID: "thread", RecordID: "call_two"})
		errorsCh <- err
	}()
	var succeeded, conflicted int
	for range 2 {
		err := <-errorsCh
		if err == nil {
			succeeded++
		} else if errors.Is(err, ErrRevisionConflict) {
			conflicted++
		} else {
			t.Fatalf("unexpected update error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("CAS results success=%d conflict=%d", succeeded, conflicted)
	}
}

func TestCandidatesAndReadSelectedSupportNestedTopics(t *testing.T) {
	base := t.TempDir()
	store := NewStoreWithRoots(filepath.Join(base, "user"), filepath.Join(base, "project"))
	record, err := store.Create(testCreate(ScopeUser))
	if err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(store.UserRoot, "preferences")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(nested, filepath.Base(record.Path))
	if err := os.Rename(record.Path, target); err != nil {
		t.Fatal(err)
	}
	candidates, err := store.Candidates(ScopeUser)
	if err != nil || len(candidates) != 1 || !strings.HasPrefix(candidates[0].Filename, "preferences/") {
		t.Fatalf("candidates=%+v err=%v", candidates, err)
	}
	selected, err := store.ReadSelected(ScopeUser, []string{candidates[0].Filename})
	if err != nil || len(selected) != 1 || selected[0].MemoryID != record.MemoryID {
		t.Fatalf("selected=%+v err=%v", selected, err)
	}
}
