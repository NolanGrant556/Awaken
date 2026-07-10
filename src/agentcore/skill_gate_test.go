package agentcore

import (
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
	_ "awaken/src/tools"
)

func TestRequestBuilderFiltersResolvedToolPool(t *testing.T) {
	builder := NewRequestBuilder(AgentConfig{
		ResolvedToolPool: []string{"text_edit", "shell"},
		ToolExposure:     tool.CompatToolExposurePolicy(),
	})
	req := builder.Build(convcontext.NewConversation(nil).Context, nil)
	names := make([]string, 0, len(req.Tools))
	for _, schema := range req.Tools {
		names = append(names, schema.Function.Name)
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "text_edit") {
		t.Fatalf("expected text_edit in pool, got %v", names)
	}
	if !strings.Contains(joined, "shell") {
		t.Fatalf("expected shell in pool, got %v", names)
	}
	if strings.Contains(joined, "apply_patch") {
		t.Fatalf("apply_patch should be filtered out of resolved pool, got %v", names)
	}
}

func TestToolRuntimeRejectsOutsideResolvedPool(t *testing.T) {
	rt := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		ResolvedToolPool: []string{"file_read"},
	})
	result := rt.Execute(t.Context(), convcontext.ToolCall{
		ID: "tc1",
		Function: convcontext.FunctionCall{
			Name:      "shell",
			Arguments: `{"command":"echo hi"}`,
		},
	})
	if !result.IsError {
		t.Fatalf("expected denial, got %+v", result)
	}
	if result.ErrorCode != tool.ErrorCodeSkillToolNotAllowed && result.Outcome.ErrorCode != tool.ErrorCodeSkillToolNotAllowed {
		t.Fatalf("expected skill_tool_not_allowed, got code=%q outcome=%q content=%q",
			result.ErrorCode, result.Outcome.ErrorCode, result.Content)
	}
}

func TestCreateSubagentInheritsResolvedToolPool(t *testing.T) {
	parent := AgentConfig{
		ResolvedToolPool: []string{"file_read", "text_edit"},
		Workspace:        WorkspaceConfig{Root: "/tmp/ws"},
	}
	child, err := CreateSubagent(parent, SpawnConfig{
		AgentID:       "child",
		ParentAgentID: "parent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(child.ResolvedToolPool, ",") != "file_read,text_edit" {
		t.Fatalf("child pool=%v", child.ResolvedToolPool)
	}
}

func TestImplicitSkillParentPoolMatchesModelExposure(t *testing.T) {
	config := AgentConfig{}
	runner := NewRunner(config, convcontext.NewConversation(nil), NewRequestBuilder(config))
	pool := runner.currentImplicitParentPool()
	for _, name := range pool {
		if name == "apply_patch" {
			t.Fatalf("implicit Skill inherited hidden tool apply_patch from registry: %v", pool)
		}
	}
	if len(pool) == 0 {
		t.Fatal("expected ordinary model-visible tools")
	}
}
