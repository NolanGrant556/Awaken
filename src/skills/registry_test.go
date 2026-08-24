package skills

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSkillRegistryDisabledStorePersistsAcrossReloads(t *testing.T) {
	tmp := t.TempDir()
	storePath := filepath.Join(tmp, "disabled-skills.json")

	registry := NewSkillRegistry(nil)
	registry.SetDisabledStorePath(storePath)

	// Initially disabling a skill writes the store.
	registry.SetDisabledByPolicy("test-skill", true, "disabled by user")

	data, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("expected disabled store to be written: %v", err)
	}
	if string(data) != `["test-skill"]` {
		t.Fatalf("expected disabled store to contain test-skill, got %s", string(data))
	}

	// Re-enabling removes it from the store.
	registry.SetDisabledByPolicy("test-skill", false, "")
	data, err = os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("expected disabled store to remain readable: %v", err)
	}
	if string(data) != `[]` {
		t.Fatalf("expected disabled store to be empty, got %s", string(data))
	}

	// A fresh registry loading the same store should restore disabled state.
	registry.SetDisabledByPolicy("test-skill", true, "disabled by user")
	fresh := NewSkillRegistry(nil)
	fresh.SetDisabledStorePath(storePath)
	if _, ok := fresh.disabledByPolicy["test-skill"]; !ok {
		t.Fatalf("expected fresh registry to load disabled skill from store")
	}
}

func TestSkillRegistryCatalogListenerDoesNotReplaceToolChangeOwner(t *testing.T) {
	registry := NewSkillRegistry(nil)
	toolChanged := make(chan struct{}, 1)
	catalogChanged := make(chan struct{}, 1)
	registry.SetOnChange(func() { toolChanged <- struct{}{} })
	unsubscribe := registry.SubscribeCatalogChanges(func() { catalogChanged <- struct{}{} })
	defer unsubscribe()
	registry.RegisterSkill(&SkillDefinition{
		ID: "dynamic-skill", Name: "dynamic-skill", Description: "Dynamic Skill",
		State: StateLoaded, Source: SourceUser, UserInvocable: true,
	})
	for name, ch := range map[string]<-chan struct{}{"tool": toolChanged, "catalog": catalogChanged} {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatalf("%s listener was not called", name)
		}
	}
}
