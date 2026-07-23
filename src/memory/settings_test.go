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
	if err != nil || document.Revision != 0 || !document.Value.Enabled || !document.Value.AutomaticRecall || !document.Value.ProactiveMemory || !document.Value.AutoDream {
		t.Fatalf("default=%+v err=%v", document, err)
	}
	disabled := false
	updated, err := store.Update(0, SettingsPatch{ProactiveMemory: &disabled})
	if err != nil || updated.Revision != 1 || updated.Value.ProactiveMemory || !updated.Value.AutoDream {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	if _, err := store.Update(0, SettingsPatch{Enabled: &disabled}); !errors.Is(err, filestore.ErrRevisionConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	disabledDocument, err := store.Update(1, SettingsPatch{Enabled: &disabled})
	if err != nil || disabledDocument.Revision != 2 || disabledDocument.Value.Enabled || disabledDocument.Value.AutomaticRecall || disabledDocument.Value.ProactiveMemory || disabledDocument.Value.AutoDream {
		t.Fatalf("disabled=%+v err=%v", disabledDocument, err)
	}
	enabled := true
	reenabledDocument, err := store.Update(2, SettingsPatch{Enabled: &enabled})
	if err != nil || reenabledDocument.Revision != 3 || !reenabledDocument.Value.Enabled || !reenabledDocument.Value.AutomaticRecall || !reenabledDocument.Value.ProactiveMemory || !reenabledDocument.Value.AutoDream {
		t.Fatalf("reenabled=%+v err=%v", reenabledDocument, err)
	}
}
