package memory

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAddListSearch(t *testing.T) {
	store := newTestStore(t)
	scope := filepath.Join(t.TempDir(), "workspace")

	record, err := store.Add(AddRequest{
		Scope:  scope,
		Text:   "Remember that runtime memory is explicit and durable.",
		Tags:   []string{"runtime", "memory", "memory", " "},
		Source: "test",
	})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if record.ID == "" {
		t.Fatalf("expected record id")
	}
	if record.Scope != scope {
		t.Fatalf("expected normalized scope %q, got %q", scope, record.Scope)
	}
	if len(record.Tags) != 2 {
		t.Fatalf("expected duplicate and empty tags to be removed, got %+v", record.Tags)
	}

	listed, err := store.List(scope, 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 record, got %d", len(listed))
	}

	results, err := store.Search(scope, "RUNTIME durable", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != record.ID {
		t.Fatalf("expected result %q, got %q", record.ID, results[0].ID)
	}
	if results[0].Score <= 0 {
		t.Fatalf("expected positive score, got %d", results[0].Score)
	}
}

func TestScopeIsolation(t *testing.T) {
	store := newTestStore(t)
	scopeA := filepath.Join(t.TempDir(), "a")
	scopeB := filepath.Join(t.TempDir(), "b")

	if _, err := store.Add(AddRequest{Scope: scopeA, Text: "provider keys stay outside memory"}); err != nil {
		t.Fatalf("Add scope A failed: %v", err)
	}
	if _, err := store.Add(AddRequest{Scope: scopeB, Text: "provider setup belongs to another workspace"}); err != nil {
		t.Fatalf("Add scope B failed: %v", err)
	}

	listed, err := store.List(scopeA, 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(listed) != 1 || listed[0].Scope != scopeA {
		t.Fatalf("expected only scope A records, got %+v", listed)
	}

	results, err := store.Search(scopeA, "another workspace", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no cross-scope search results, got %+v", results)
	}
}

func TestRejectsEmptyText(t *testing.T) {
	store := newTestStore(t)
	scope := t.TempDir()

	if _, err := store.Add(AddRequest{Scope: scope, Text: "   "}); !errors.Is(err, ErrEmptyText) {
		t.Fatalf("expected ErrEmptyText, got %v", err)
	}

	listed, err := store.List(scope, 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("expected no stored records, got %+v", listed)
	}
}

func TestPersistenceReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.jsonl")
	scope := filepath.Join(t.TempDir(), "workspace")

	store, err := NewJSONLStore(path)
	if err != nil {
		t.Fatalf("NewJSONLStore failed: %v", err)
	}
	record, err := store.Add(AddRequest{Scope: scope, Text: "reload keeps durable memory"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	reloaded, err := NewJSONLStore(path)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	listed, err := reloaded.List(scope, 0)
	if err != nil {
		t.Fatalf("List after reload failed: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 reloaded record, got %d", len(listed))
	}
	if listed[0].ID != record.ID || listed[0].Text != record.Text {
		t.Fatalf("unexpected reloaded record: %+v", listed[0])
	}
}

func TestAddRedactsHighConfidenceSecrets(t *testing.T) {
	store := newTestStore(t)
	scope := filepath.Join(t.TempDir(), "workspace")

	record, err := store.Add(AddRequest{
		Scope:  scope,
		Text:   "keep provider status, never persist sk-secret-key-must-not-appear",
		Tags:   []string{"sk-secret-key-must-not-appear", "runtime"},
		Source: "AKIA1234567890ABCDEF",
	})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if record.Text == "" || record.Source == "" {
		t.Fatalf("expected redacted record fields to remain populated: %+v", record)
	}
	for _, value := range []string{record.Text, record.Source, record.Tags[0]} {
		if value == "sk-secret-key-must-not-appear" || value == "AKIA1234567890ABCDEF" {
			t.Fatalf("expected secret to be redacted, got %q", value)
		}
	}
}

func TestLoadSkipsCorruptLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.jsonl")
	scope := filepath.Join(t.TempDir(), "workspace")
	if err := os.WriteFile(path, []byte("{bad json}\n"+`{"id":"mem_1","scope":"`+scope+`","text":"kept"}`+"\n"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	store, err := NewJSONLStore(path)
	if err != nil {
		t.Fatalf("NewJSONLStore failed: %v", err)
	}
	listed, err := store.List(scope, 10)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(listed) != 1 || listed[0].Text != "kept" {
		t.Fatalf("expected valid lines to survive corruption, got %+v", listed)
	}
}

func TestNewDefaultStoreRequiresAbsoluteEnvPath(t *testing.T) {
	t.Setenv(EnvStorePath, "relative/memory.jsonl")

	store, err := NewDefaultStore()
	if err == nil {
		t.Fatalf("expected error for relative memory path, got store %+v", store)
	}
}

func TestNewDefaultStoreDisabledWithoutEnv(t *testing.T) {
	t.Setenv(EnvStorePath, "")

	store, err := NewDefaultStore()
	if err != nil {
		t.Fatalf("NewDefaultStore failed: %v", err)
	}
	if store != nil {
		t.Fatalf("expected nil store when %s is unset, got %+v", EnvStorePath, store)
	}
}

func newTestStore(t *testing.T) *JSONLStore {
	t.Helper()
	store, err := NewJSONLStore(filepath.Join(t.TempDir(), "memory.jsonl"))
	if err != nil {
		t.Fatalf("NewJSONLStore failed: %v", err)
	}
	return store
}
