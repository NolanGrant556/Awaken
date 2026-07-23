package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"awaken/src/acp"
)

func contextBackground() context.Context {
	return context.Background()
}

// testNoopClientFactory returns a factory that always errors. It is used for
// handler tests that never start live ACP sessions but still need a non-nil
// factory to satisfy Manager's constructor contract.
func testNoopClientFactory() acp.ClientFactory {
	return func(context.Context, acp.RunnerSnapshot, string) (acp.Client, error) {
		return nil, errors.New("live ACP client not available in test")
	}
}

func timeSleep(d time.Duration) {
	time.Sleep(d)
}

func TestHandleListRunnersRedactsEnvValues(t *testing.T) {
	registry := acp.NewRegistry(filepath.Join(t.TempDir(), "acp-agents.json"))
	if err := registry.UpsertUser(acp.RunnerConfig{
		ID:      "codex",
		Enabled: true,
		Command: "npx",
		Args:    []string{"-y", "@zed-industries/codex-acp"},
		Env: map[string]string{
			"OPENAI_API_KEY": "super-secret",
		},
	}); err != nil {
		t.Fatalf("upsert runner: %v", err)
	}

	h := &acpHandlers{manager: acp.NewManager(registry, testNoopClientFactory())}
	req := httptest.NewRequest(http.MethodGet, "/v1/acp/runners", nil)
	rec := httptest.NewRecorder()
	h.handleListRunners(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Runners []acpRunnerView `json:"runners"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Runners) != 1 {
		t.Fatalf("expected 1 runner, got %d", len(body.Runners))
	}
	runner := body.Runners[0]
	if len(runner.Env) != 1 || runner.Env[0].Key != "OPENAI_API_KEY" {
		t.Fatalf("expected redacted env key, got %+v", runner.Env)
	}
	if !runner.Env[0].HasValue {
		t.Fatalf("expected HasValue=true for non-empty secret")
	}
}

func TestHandleUpdateRunnerMergesEnvWithSetRemove(t *testing.T) {
	registry := acp.NewRegistry(filepath.Join(t.TempDir(), "acp-agents.json"))
	if err := registry.UpsertUser(acp.RunnerConfig{
		ID:      "codex",
		Enabled: true,
		Command: "npx",
		Env: map[string]string{
			"KEEP":     "old",
			"REMOVE":   "me",
			"REPLACE":  "old-value",
		},
	}); err != nil {
		t.Fatalf("upsert runner: %v", err)
	}

	h := &acpHandlers{manager: acp.NewManager(registry, testNoopClientFactory())}
	payload, _ := json.Marshal(acpRunnerPayload{
		EnvSet:    map[string]string{"REPLACE": "new-value", "ADD": "new"},
		EnvRemove: []string{"REMOVE"},
	})
	req := httptest.NewRequest(http.MethodPatch, "/v1/acp/runners/codex", bytes.NewReader(payload))
	req.SetPathValue("runner_id", "codex")
	rec := httptest.NewRecorder()
	h.handleUpdateRunner(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	cfg, ok := registry.Get("codex")
	if !ok {
		t.Fatal("runner missing after update")
	}
	if cfg.Env["KEEP"] != "old" {
		t.Fatalf("expected KEEP to be preserved, got %q", cfg.Env["KEEP"])
	}
	if _, ok := cfg.Env["REMOVE"]; ok {
		t.Fatal("expected REMOVE to be deleted")
	}
	if cfg.Env["REPLACE"] != "new-value" {
		t.Fatalf("expected REPLACE updated, got %q", cfg.Env["REPLACE"])
	}
	if cfg.Env["ADD"] != "new" {
		t.Fatalf("expected ADD inserted, got %q", cfg.Env["ADD"])
	}
}

func TestHandleResolvePermissionValidatesRunID(t *testing.T) {
	registry := acp.NewRegistry(filepath.Join(t.TempDir(), "acp-agents.json"))
	awaiter := acp.NewChanPermissionAwaiter()
	mgr := acp.NewManager(registry, testNoopClientFactory())
	mgr.SetPermissionAwaiter(awaiter)

	h := &acpHandlers{manager: mgr}

	// Seed a pending permission record directly in the awaiter.
	go func() {
		_, _ = awaiter.AwaitPermission(contextBackground(), acp.PermissionRequest{
			InteractionID: "perm-1",
			ThreadID:      "thread-1",
			RunID:         "run-1",
			RunnerID:      "codex",
			ACPSessionID:  "session-1",
			Options:       []acp.PermissionOption{{OptionID: "allow", Name: "Allow"}},
		})
	}()

	// Give the goroutine time to register.
	timeSleep(50 * time.Millisecond)

	payload, _ := json.Marshal(map[string]any{"option_id": "allow"})
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/wrong-run/acp-permissions/perm-1/resolve", bytes.NewReader(payload))
	req.SetPathValue("run_id", "wrong-run")
	req.SetPathValue("interaction_id", "perm-1")
	rec := httptest.NewRecorder()
	h.handleResolvePermission(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for run_id mismatch, got %d: %s", rec.Code, rec.Body.String())
	}
}
