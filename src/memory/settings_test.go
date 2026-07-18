package memory

import (
	"errors"
	"testing"

	"awaken/src/filestore"
)

func TestSettingsDefaultsAndRevisionCAS(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	store, err := NewSettingsStore()
	if err != nil {
		t.Fatal(err)
	}
	document, err := store.Get()
	if err != nil || document.Revision != 0 || !document.Value.Enabled || !document.Value.AutomaticRecall || !document.Value.ProactiveMemory || document.Value.AutoDream {
		t.Fatalf("default=%+v err=%v", document, err)
	}
	enabled := true
	updated, err := store.Update(0, SettingsPatch{AutoDream: &enabled})
	if err != nil || updated.Revision != 1 || !updated.Value.AutoDream {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	disabled := false
	if _, err := store.Update(0, SettingsPatch{Enabled: &disabled}); !errors.Is(err, filestore.ErrRevisionConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}
