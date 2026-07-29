package skillruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"awaken/src/skills"
	"awaken/src/tool"
)

func TestForkExecutorWithoutStarterFailsClosed(t *testing.T) {
	_, err := (&ForkExecutor{}).Execute(context.Background(), "test", "do something", nil)
	if err == nil || !strings.Contains(err.Error(), "skill_fork_unavailable") {
		t.Fatalf("fork silently degraded: %v", err)
	}
}

func TestForkExecutorUsesStarter(t *testing.T) {
	e := &ForkExecutor{StartChild: func(_ context.Context, body string) (string, error) {
		return "child-done: " + body, nil
	}}
	result, err := e.Execute(context.Background(), "test", "do something", nil)
	if err != nil || !strings.Contains(result, "child-done") {
		t.Fatalf("result=%q err=%v", result, err)
	}
}

func TestForkExecutorPropagatesStarterError(t *testing.T) {
	e := &ForkExecutor{StartChild: func(context.Context, string) (string, error) {
		return "", context.DeadlineExceeded
	}}
	_, err := e.Execute(context.Background(), "test", "do something", nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("fork error was hidden: %v", err)
	}
}

func TestManagerRegistersOnlyUnifiedCatalogTool(t *testing.T) {
	tool.UnregisterTool(CatalogToolName)
	reg := skills.NewSkillRegistry(nil)
	reg.RegisterSkill(&skills.SkillDefinition{
		ID: "test-manager", Name: "test-manager", Description: "manager test",
		State: skills.StateLoaded, Source: skills.SourceMcp, Body: "test body",
	})
	manager := NewManager(reg, nil)
	t.Cleanup(manager.Shutdown)

	if _, err := tool.GetTool(CatalogToolName); err != nil {
		t.Fatalf("unified catalog tool missing: %v", err)
	}
	manager.Shutdown()
	if _, err := tool.GetTool(CatalogToolName); err == nil {
		t.Fatal("catalog tool remained after shutdown")
	}
}

func TestManagerKeepsUnifiedSkillRegistrationStableAcrossNoOpReload(t *testing.T) {
	previous, previousErr := tool.GetRegisteredTool(CatalogToolName)
	t.Cleanup(func() {
		if previousErr == nil {
			_ = tool.RegisterToolForce(previous.Runtime)
		} else {
			tool.UnregisterTool(CatalogToolName)
		}
	})

	root := t.TempDir()
	packageDir := filepath.Join(root, "stable-skill")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	initial := "---\nname: stable-skill\ndescription: stable catalog\n---\nInitial instructions.\n"
	skillPath := filepath.Join(packageDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	dirs := map[skills.SourceOrigin]string{skills.SourceUser: root}
	registry := skills.NewSkillRegistry(nil)
	registry.LoadFromSources(dirs)
	manager := NewManager(registry, nil)
	t.Cleanup(manager.Shutdown)

	before, err := tool.GetRegisteredTool(CatalogToolName)
	if err != nil {
		t.Fatal(err)
	}
	beforeVersion := tool.RegistryVersion()
	manager.RefreshTools()
	registry.ReloadLocalSources(dirs)
	time.Sleep(50 * time.Millisecond)
	afterNoOp, err := tool.GetRegisteredTool(CatalogToolName)
	if err != nil {
		t.Fatal(err)
	}
	if afterNoOp.RegistrationID != before.RegistrationID || tool.RegistryVersion() != beforeVersion {
		t.Fatalf("same catalog reload churned unified Skill registration: before=%s after=%s versions=%d/%d", before.RegistrationID, afterNoOp.RegistrationID, beforeVersion, tool.RegistryVersion())
	}

	catalogChanged := "---\nname: stable-skill\ndescription: changed catalog\n---\nInitial instructions.\n"
	if err := os.WriteFile(skillPath, []byte(catalogChanged), 0o600); err != nil {
		t.Fatal(err)
	}
	registry.ReloadLocalSources(dirs)
	afterCatalogChange := waitForSkillRegistrationChange(t, before.RegistrationID)
	if afterCatalogChange.Definition.Description == before.Definition.Description {
		t.Fatal("catalog change replaced the runtime pin without updating the model-visible definition")
	}

	bodyChanged := "---\nname: stable-skill\ndescription: changed catalog\n---\nDifferent runtime instructions.\n"
	if err := os.WriteFile(skillPath, []byte(bodyChanged), 0o600); err != nil {
		t.Fatal(err)
	}
	registry.ReloadLocalSources(dirs)
	afterBodyChange := waitForSkillRegistrationChange(t, afterCatalogChange.RegistrationID)
	if afterBodyChange.Definition.Description != afterCatalogChange.Definition.Description {
		t.Fatal("body-only binding change unexpectedly changed the model-visible definition")
	}

	manager.SetOnActivated(func(*SkillActivationPlan) {})
	afterCallbackChange := waitForSkillRegistrationChange(t, afterBodyChange.RegistrationID)
	if afterCallbackChange.Definition.Description != afterBodyChange.Definition.Description {
		t.Fatal("runtime callback binding change unexpectedly changed the model-visible definition")
	}
}

func waitForSkillRegistrationChange(t *testing.T, previousID string) tool.RegisteredTool {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		registered, err := tool.GetRegisteredTool(CatalogToolName)
		if err == nil && registered.RegistrationID != previousID {
			return registered
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for unified Skill registration to change from %s", previousID)
	return tool.RegisteredTool{}
}
