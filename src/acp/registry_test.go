package acp

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestRegistryConcurrentUpsertDoesNotLoseUpdates(t *testing.T) {
	registry := NewRegistry(filepath.Join(t.TempDir(), "acp-agents.json"))

	if err := registry.UpsertUser(RunnerConfig{
		ID:      "runner-a",
		Enabled: true,
		Command: "cmd-a",
	}); err != nil {
		t.Fatalf("initial upsert: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = registry.UpsertUser(RunnerConfig{
			ID:      "runner-b",
			Enabled: true,
			Command: "cmd-b",
		})
	}()
	go func() {
		defer wg.Done()
		_ = registry.UpsertUser(RunnerConfig{
			ID:      "runner-c",
			Enabled: true,
			Command: "cmd-c",
		})
	}()
	wg.Wait()

	all := registry.All()
	ids := make(map[string]bool)
	for _, cfg := range all {
		ids[cfg.ID] = true
	}
	for _, id := range []string{"runner-a", "runner-b", "runner-c"} {
		if !ids[id] {
			t.Fatalf("missing runner %q after concurrent upserts: %+v", id, ids)
		}
	}
}

func TestSaveConfigFileReplacesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "acp-agents.json")

	cfg1 := ConfigFile{SchemaVersion: 1, Runners: []RunnerConfig{{ID: "a", Command: "cmd-a"}}}
	if err := SaveConfigFile(path, cfg1); err != nil {
		t.Fatalf("first save: %v", err)
	}

	cfg2 := ConfigFile{SchemaVersion: 1, Runners: []RunnerConfig{{ID: "a", Command: "cmd-a"}, {ID: "b", Command: "cmd-b"}}}
	if err := SaveConfigFile(path, cfg2); err != nil {
		t.Fatalf("second save: %v", err)
	}

	loaded, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Runners) != 2 {
		t.Fatalf("expected 2 runners after replace, got %d", len(loaded.Runners))
	}
}
