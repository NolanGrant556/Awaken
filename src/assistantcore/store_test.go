package assistantcore

import (
	"errors"
	"testing"
)

func TestMemoryStoreCRUDReorderAndEnableDisable(t *testing.T) {
	first := validAssistant("first", AssistantSourceUser, 1)
	first.Profile.Name = "First"
	second := validAssistant("second", AssistantSourceUser, 0)
	second.Profile.Name = "Second"

	store, err := NewMemoryStore(first, second)
	if err != nil {
		t.Fatalf("expected store: %v", err)
	}

	list := store.List()
	assertIDs(t, list, []string{"second", "first"})

	got, ok := store.Get("first")
	if !ok {
		t.Fatalf("expected first assistant")
	}
	got.Profile.Description = "updated"
	updated, err := store.Update(got)
	if err != nil {
		t.Fatalf("expected update: %v", err)
	}
	if updated.Profile.Description != "updated" {
		t.Fatalf("expected updated description, got %q", updated.Profile.Description)
	}

	disabled, err := store.EnableDisable("first", false)
	if err != nil {
		t.Fatalf("expected enable/disable: %v", err)
	}
	if disabled.State.Enabled {
		t.Fatalf("expected assistant to be disabled")
	}

	if err := store.Reorder([]string{"first", "second"}); err != nil {
		t.Fatalf("expected reorder: %v", err)
	}
	assertIDs(t, store.List(), []string{"first", "second"})

	if err := store.Delete("second"); err != nil {
		t.Fatalf("expected delete: %v", err)
	}
	if _, ok := store.Get("second"); ok {
		t.Fatalf("expected second assistant to be deleted")
	}
}

func TestMemoryStoreRejectsDuplicateCreateAndInvalidReorder(t *testing.T) {
	assistant := validAssistant("assistant", AssistantSourceUser, 0)
	store, err := NewMemoryStore(assistant)
	if err != nil {
		t.Fatalf("expected store: %v", err)
	}

	if _, err := store.Create(assistant); !errors.Is(err, ErrAssistantAlreadyExists) {
		t.Fatalf("expected duplicate create error, got %v", err)
	}
	if err := store.Reorder([]string{"assistant", "assistant"}); !errors.Is(err, ErrInvalidReorder) {
		t.Fatalf("expected invalid reorder error, got %v", err)
	}
}

func TestMemoryStoreAppliesReadOnlyRestrictions(t *testing.T) {
	builtin := validAssistant("builtin", AssistantSourceBuiltin, 0)
	generated := validAssistant("generated", AssistantSourceGenerated, 1)

	store, err := NewMemoryStore(builtin, generated)
	if err != nil {
		t.Fatalf("expected store: %v", err)
	}

	if err := store.Delete("builtin"); !errors.Is(err, ErrAssistantReadOnly) {
		t.Fatalf("expected builtin delete to be read-only, got %v", err)
	}
	if err := store.Delete("generated"); !errors.Is(err, ErrAssistantReadOnly) {
		t.Fatalf("expected generated delete to be read-only, got %v", err)
	}

	builtin.Profile.Name = "Renamed"
	if _, err := store.Update(builtin); err == nil {
		t.Fatalf("expected builtin profile update to be rejected")
	}

	generated.Binding.RuntimeID = "other-runtime"
	if _, err := store.Update(generated); err == nil {
		t.Fatalf("expected generated binding update to be rejected")
	}
}
