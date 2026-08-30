package agentcore

import (
	stdcontext "context"
	"strings"
	"testing"

	"awaken/src/subagentruntime"
	"awaken/src/tool"
	"awaken/src/tool/profile"
)

func TestApplyChildRoleHidesMutationToolsForPlan(t *testing.T) {
	child := AgentConfig{ToolExposure: tool.ToolExposurePolicy{Mode: tool.ExposureAll}}
	applyChildRole(&child, subagentruntime.TaskSnapshot{AgentType: subagentruntime.AgentTypePlan})
	hidden := strings.Join(child.ToolExposure.ExtraHidden, ",")
	for _, name := range []string{"Write", "Edit", "subagent", "ask_user"} {
		if !strings.Contains(hidden, name) {
			t.Fatalf("plan ExtraHidden=%q missing %s", hidden, name)
		}
	}
	if !strings.Contains(child.SystemPrompt, "Critical Files for Implementation") {
		t.Fatalf("plan prompt missing critical files section: %s", child.SystemPrompt)
	}
}

func TestRequestedChildProfileRejectsReadOnlyParentForGeneral(t *testing.T) {
	readOnly := profile.WorkspaceReadOnly()
	_, err := requestedChildProfile(AgentConfig{ExecutionProfile: &readOnly}, subagentruntime.TaskSnapshot{
		AgentType: subagentruntime.AgentTypeGeneral, WorkspaceMode: subagentruntime.WorkspaceShared,
	})
	if err == nil || subagentruntime.ErrorCode(err) != subagentruntime.ErrorBoundaryViolation {
		t.Fatalf("expected boundary violation, got %v", err)
	}
}

func TestInheritParentMemorySnapshotProvidesFrozenContent(t *testing.T) {
	startCalled := false
	child := AgentConfig{
		MemoryRecallStart: func(_ stdcontext.Context, _ string, _ []string) {
			startCalled = true
		},
		MemoryContextProvider: func() (string, error) { return "parent-live", nil },
	}
	inheritParentMemorySnapshot(&child, &MemoryRecallSnapshot{
		WorkspaceRoot: "/workspace",
		Items: []MemoryRecallSnapshotItem{{
			MemoryID: "mem_auth", Scope: "project", Content: "<memory id=\"mem_auth\">jwt</memory>",
		}},
	}, "/workspace")
	if child.MemoryRecallStart != nil {
		t.Fatalf("child must not keep parent selector")
	}
	content, err := child.MemoryContextProvider()
	if err != nil || !strings.Contains(content, "mem_auth") || !strings.Contains(content, "inherited_parent_snapshot") {
		t.Fatalf("inherited context = %q err=%v", content, err)
	}
	if startCalled {
		t.Fatalf("selector must not run during inherit")
	}
}

func TestApplyPluginChildRoleEnforcesAllowlistAndMaxTurns(t *testing.T) {
	subagentruntime.SetRoleResolver(func(agentType string) (subagentruntime.Role, bool) {
		if agentType != "plugin__acme__reader" {
			return subagentruntime.Role{}, false
		}
		return subagentruntime.Role{Type: agentType, SystemPrompt: "Plugin prompt", ReadOnly: true, AllowedTools: []string{"Read"}, MaxTurns: 3}, true
	})
	t.Cleanup(func() { subagentruntime.SetRoleResolver(nil) })
	child := AgentConfig{MaxThoughts: 20, ToolExposure: tool.ToolExposurePolicy{Mode: tool.ExposureAll}}
	applyChildRole(&child, subagentruntime.TaskSnapshot{AgentType: "plugin__acme__reader"})
	if child.MaxThoughts != 3 || !strings.Contains(child.SystemPrompt, "Plugin prompt") {
		t.Fatalf("child=%#v", child)
	}
	hidden := strings.Join(child.ToolExposure.ExtraHidden, ",")
	if !strings.Contains(hidden, "configure_hook") || !strings.Contains(hidden, "Bash") {
		t.Fatalf("allowlist did not hide non-allowed tools: %s", hidden)
	}
}
