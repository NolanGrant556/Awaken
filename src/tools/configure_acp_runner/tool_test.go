package configureacprunner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"awaken/src/acp"
)

func TestConfigureACPRunner_CRUD(t *testing.T) {
	tmp := t.TempDir()
	registry := acp.NewRegistry(filepath.Join(tmp, "acp-agents.json"))
	if err := registry.Load(); err != nil {
		t.Fatalf("load registry: %v", err)
	}
	tool := &ConfigureACPRunnerTool{registry: registry}
	ctx := context.Background()

	// Create
	out, err := tool.Execute(ctx, mustJSON(t, map[string]any{
		"action":    "create",
		"runner_id": "test_runner",
		"command":   "test-cmd",
		"enabled":   true,
		"args":      []string{"--acp"},
	}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !contains(t, out, "configured") {
		t.Fatalf("expected configured status, got %s", out)
	}

	cfg, ok := registry.Get("test_runner")
	if !ok {
		t.Fatal("runner not found after create")
	}
	if cfg.Command != "test-cmd" || !cfg.Enabled || len(cfg.Args) != 1 || cfg.Args[0] != "--acp" {
		t.Fatalf("unexpected config after create: %+v", cfg)
	}

	// Create duplicate should fail
	if _, err := tool.Execute(ctx, mustJSON(t, map[string]any{
		"action":    "create",
		"runner_id": "test_runner",
		"command":   "other-cmd",
	})); err == nil {
		t.Fatal("expected error creating duplicate runner")
	}

	// Update
	out, err = tool.Execute(ctx, mustJSON(t, map[string]any{
		"action":    "update",
		"runner_id": "test_runner",
		"enabled":   false,
	}))
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !contains(t, out, "configured") {
		t.Fatalf("expected configured status, got %s", out)
	}
	cfg, ok = registry.Get("test_runner")
	if !ok || cfg.Enabled {
		t.Fatalf("runner should be disabled after update: %+v", cfg)
	}

	// Upsert existing
	out, err = tool.Execute(ctx, mustJSON(t, map[string]any{
		"action":    "upsert",
		"runner_id": "test_runner",
		"enabled":   true,
		"command":   "test-cmd-v2",
	}))
	if err != nil {
		t.Fatalf("upsert existing: %v", err)
	}
	cfg, ok = registry.Get("test_runner")
	if !ok || !cfg.Enabled || cfg.Command != "test-cmd-v2" {
		t.Fatalf("unexpected config after upsert: %+v", cfg)
	}

	// Upsert new
	out, err = tool.Execute(ctx, mustJSON(t, map[string]any{
		"action":    "upsert",
		"runner_id": "second_runner",
		"command":   "second-cmd",
	}))
	if err != nil {
		t.Fatalf("upsert new: %v", err)
	}
	if _, ok := registry.Get("second_runner"); !ok {
		t.Fatal("second runner not found after upsert")
	}

	// Delete
	out, err = tool.Execute(ctx, mustJSON(t, map[string]any{
		"action":    "delete",
		"runner_id": "test_runner",
	}))
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !contains(t, out, "deleted") {
		t.Fatalf("expected deleted status, got %s", out)
	}
	if _, ok := registry.Get("test_runner"); ok {
		t.Fatal("runner should be deleted")
	}

	// Delete missing should fail
	if _, err := tool.Execute(ctx, mustJSON(t, map[string]any{
		"action":    "delete",
		"runner_id": "missing",
	})); err == nil {
		t.Fatal("expected error deleting missing runner")
	}

	// Update missing should fail
	if _, err := tool.Execute(ctx, mustJSON(t, map[string]any{
		"action":    "update",
		"runner_id": "missing",
		"enabled":   true,
	})); err == nil {
		t.Fatal("expected error updating missing runner")
	}

	// Persistence check
	data, err := os.ReadFile(registry.UserPath())
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	var fileCfg acp.ConfigFile
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		t.Fatalf("parse config file: %v", err)
	}
	if len(fileCfg.Runners) != 1 || fileCfg.Runners[0].ID != "second_runner" {
		t.Fatalf("unexpected persisted config: %+v", fileCfg.Runners)
	}
}

func TestConfigureACPRunner_RegistryUnavailable(t *testing.T) {
	tool := &ConfigureACPRunnerTool{registry: nil}
	_, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"action":    "create",
		"runner_id": "x",
		"command":   "y",
	}))
	if err == nil {
		t.Fatal("expected error when registry unavailable")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func contains(t *testing.T, s, substr string) bool {
	t.Helper()
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsInternal(s, substr))
}

func containsInternal(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
