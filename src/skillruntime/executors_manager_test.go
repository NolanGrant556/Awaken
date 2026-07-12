package skillruntime

import (
	"context"
	"errors"
	"strings"
	"testing"

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
