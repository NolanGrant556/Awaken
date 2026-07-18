package memory

import (
	"errors"
	"strings"
	"testing"
)

func TestDreamPromptPreservesDurablePreferencesAndAvoidsChurn(t *testing.T) {
	for _, required := range []string{"strict JSON only", "near duplicates", "Recency alone is not evidence", "forms of address", "Avoid cosmetic rewrites", "empty actions array"} {
		if !strings.Contains(dreamMaintenanceSystemPrompt, required) {
			t.Fatalf("Dream prompt missing %q", required)
		}
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
