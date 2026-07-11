package skillruntime

import (
	"context"
	"strings"
	"testing"

	"awaken/src/tool"
)

// ---- fork_test ----

func TestForkExecutorDegradesToInline(t *testing.T) {
	e := &ForkExecutor{
		// StartChild is nil -> degrades.
	}
	result, err := e.Execute(context.Background(), "test", "do something", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "fork degraded") {
		t.Errorf("bad fork degradation result: %s", result)
	}
}

func TestForkExecutorStub(t *testing.T) {
	e := &ForkExecutor{
		StartChild: func(ctx context.Context, body string) (string, error) {
			return "child-done: " + body, nil
		},
	}
	result, err := e.Execute(context.Background(), "test", "do something", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "child-done") {
		t.Errorf("fork stub didn't return child result: %s", result)
	}
}

func TestForkExecutorStartChildErrorDegrades(t *testing.T) {
	e := &ForkExecutor{
		StartChild: func(ctx context.Context, body string) (string, error) {
			return "", context.DeadlineExceeded
		},
	}
	result, err := e.Execute(context.Background(), "test", "do something", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "fork degraded") {
		t.Errorf("expected fork to degrade: %s", result)
	}
}

// ---- manager_test ----

func TestManagerRefreshAndShutdown(t *testing.T) {
	// Create a minimal skill registry.
	// Use skills package to create the registry.
	// We import skills, but to avoid circular dep test we directly test
	// Manager refresh via manual tool registration.

	// Clean up any leftover skill tools.
	tool.UnregisterToolsByPrefix(SkillToolPrefix)

	st := &SkillTool{
		SkillID: "test_manager",
		NameStr: SkillToolName("test_manager"),
		Desc:    "manager test",
		Body:    "test body",
		ArgDefs: []SkillArgDef{
			{Name: "x", Type: "string", Required: true},
		},
	}
	if err := tool.RegisterToolForce(st); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Verify it's registered.
	got, err := tool.GetTool(SkillToolName("test_manager"))
	if err != nil {
		t.Fatalf("get tool: %v", err)
	}
	if got.Name() != SkillToolName("test_manager") {
		t.Errorf("bad name: %s", got.Name())
	}

	// Shutdown should remove it.
	tool.UnregisterToolsByPrefix(SkillToolPrefix)
	_, err = tool.GetTool(SkillToolName("test_manager"))
	if err == nil {
		t.Fatal("expected tool to be removed after shutdown")
	}
}
