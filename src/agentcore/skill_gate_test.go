package agentcore

import (
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
	_ "awaken/src/tools"
)

func TestRequestBuilderUsesUnifiedCatalogWithConfiguredExposure(t *testing.T) {
	builder := NewRequestBuilder(AgentConfig{
		ToolExposure: tool.CompatToolExposurePolicy(),
	})
	req := builder.Build(convcontext.NewConversation(nil).Context, nil)
	names := make([]string, 0, len(req.Tools))
	for _, schema := range req.Tools {
		names = append(names, schema.Function.Name)
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "Edit") {
		t.Fatalf("expected Edit in pool, got %v", names)
	}
	if !strings.Contains(joined, "Bash") {
		t.Fatalf("expected Bash in pool, got %v", names)
	}
	if strings.Contains(joined, "apply_patch") {
		t.Fatalf("retired apply_patch leaked into model exposure: %v", names)
	}
}

func TestToolRuntimeExecutesUnifiedCatalogTool(t *testing.T) {
	name := "skill_pool_noop_test"
	calls := 0
	if err := tool.RegisterToolForce(&snapshotTestTool{name: name, version: "ok", readiness: tool.ProviderReady, calls: &calls}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tool.UnregisterTool(name) })
	rt := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{})
	result := rt.Execute(t.Context(), convcontext.ToolCall{
		ID: "tc1",
		Function: convcontext.FunctionCall{
			Name:      name,
			Arguments: `{}`,
		},
	})
	if result.IsError || calls != 1 {
		t.Fatalf("unified catalog blocked ordinary execution: calls=%d result=%+v", calls, result)
	}
}

func TestCreateSubagentInheritsImmutableRunSnapshots(t *testing.T) {
	snapshot := &tool.RunToolAvailabilitySnapshot{SnapshotID: "tools_parent", CatalogVersion: 7}
	environment := &tool.ExecutionEnvironmentSnapshot{SnapshotID: "env_parent"}
	parent := AgentConfig{
		Workspace:                    WorkspaceConfig{Root: "/tmp/ws"},
		ToolAvailabilitySnapshot:     snapshot,
		ExecutionEnvironmentSnapshot: environment,
	}
	child, err := CreateSubagent(parent, SpawnConfig{
		AgentID:       "child",
		ParentAgentID: "parent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if child.ToolAvailabilitySnapshot != snapshot || child.ExecutionEnvironmentSnapshot != environment {
		t.Fatalf("child did not inherit immutable run snapshots: %+v", child)
	}
}

func TestImplicitSkillParentPoolMatchesModelExposure(t *testing.T) {
	config := AgentConfig{}
	runner := NewRunner(config, convcontext.NewConversation(nil), NewRequestBuilder(config))
	pool := runner.currentImplicitParentPool()
	for _, name := range pool {
		if name == "apply_patch" {
			t.Fatalf("implicit Skill inherited retired tool apply_patch from registry: %v", pool)
		}
	}
	if len(pool) == 0 {
		t.Fatal("expected ordinary model-visible tools")
	}
}
