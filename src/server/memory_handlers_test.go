package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/memory"
)

func TestMemoryEndpointListsAndSearchesWorkspaceRecords(t *testing.T) {
	store := newServerMemoryStore(t)
	workspaceA := filepath.Join(t.TempDir(), "workspace-a")
	workspaceB := filepath.Join(t.TempDir(), "workspace-b")

	first, err := store.Add(memory.AddRequest{
		Scope: workspaceA,
		Text:  "Use explicit durable memory APIs only.",
		Tags:  []string{"runtime"},
	})
	if err != nil {
		t.Fatalf("Add workspace A failed: %v", err)
	}
	if _, err := store.Add(memory.AddRequest{
		Scope: workspaceB,
		Text:  "This belongs to another workspace.",
	}); err != nil {
		t.Fatalf("Add workspace B failed: %v", err)
	}

	router := NewRouterWithMemory(nil, nil, store)

	req := httptest.NewRequest(http.MethodGet, "/v1/memory?workspace="+workspaceA, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var list memoryListResponse
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list failed: %v", err)
	}
	if len(list.Records) != 1 || list.Records[0].ID != first.ID {
		t.Fatalf("expected only workspace A record, got %+v", list.Records)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/memory?workspace="+workspaceA+"&q=DURABLE", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var search memorySearchResponse
	if err := json.NewDecoder(rec.Body).Decode(&search); err != nil {
		t.Fatalf("decode search failed: %v", err)
	}
	if len(search.Results) != 1 || search.Results[0].ID != first.ID {
		t.Fatalf("expected workspace A search result, got %+v", search.Results)
	}
}

func TestMemoryEndpointRequiresWorkspaceScope(t *testing.T) {
	store := newServerMemoryStore(t)
	router := NewRouterWithMemory(nil, nil, store)

	req := httptest.NewRequest(http.MethodGet, "/v1/memory", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "workspace") {
		t.Fatalf("expected workspace error, got %s", rec.Body.String())
	}
}

func newServerMemoryStore(t *testing.T) *memory.JSONLStore {
	t.Helper()
	store, err := memory.NewJSONLStore(filepath.Join(t.TempDir(), "memory.jsonl"))
	if err != nil {
		t.Fatalf("NewJSONLStore failed: %v", err)
	}
	return store
}
