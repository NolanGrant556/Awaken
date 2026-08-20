package subagentruntime

import "testing"

func TestValidateSpawnCombo(t *testing.T) {
	cases := []struct {
		agentType, mode, code string
	}{
		{AgentTypeExplore, WorkspaceSharedReadOnly, ""},
		{AgentTypePlan, WorkspaceSharedReadOnly, ""},
		{AgentTypeReview, WorkspaceSharedReadOnly, ""},
		{AgentTypeGeneral, WorkspaceShared, ""},
		{AgentTypeGeneral, WorkspaceGitWorktree, ""},
		{AgentTypeExplore, WorkspaceGitWorktree, ErrorWorkspaceMode},
		{AgentTypePlan, WorkspaceShared, ErrorWorkspaceMode},
		{AgentTypeGeneral, WorkspaceSharedReadOnly, ErrorWorkspaceMode},
		{"custom", WorkspaceSharedReadOnly, ErrorInvalidInput},
	}
	for _, testCase := range cases {
		err := ValidateSpawnCombo(testCase.agentType, testCase.mode)
		if testCase.code == "" {
			if err != nil {
				t.Fatalf("%s/%s: unexpected %v", testCase.agentType, testCase.mode, err)
			}
			continue
		}
		if ErrorCode(err) != testCase.code {
			t.Fatalf("%s/%s: got %q want %q (%v)", testCase.agentType, testCase.mode, ErrorCode(err), testCase.code, err)
		}
	}
}

func TestNormalizeDefaults(t *testing.T) {
	if NormalizeAgentType("") != AgentTypeExplore {
		t.Fatalf("empty type default")
	}
	if NormalizeWorkspaceMode(AgentTypeExplore, "") != WorkspaceSharedReadOnly {
		t.Fatalf("readonly default mode")
	}
	if NormalizeWorkspaceMode(AgentTypeGeneral, "") != WorkspaceShared {
		t.Fatalf("general default mode should be shared")
	}
}

func TestGeneralDoesNotHideFileMutationTools(t *testing.T) {
	role, ok := LookupRole(AgentTypeGeneral)
	if !ok || role.ReadOnly {
		t.Fatalf("general role: %+v ok=%v", role, ok)
	}
	for _, name := range role.DisallowedTools {
		if name == "Write" || name == "Edit" {
			t.Fatalf("general must keep file tools, hid %s", name)
		}
	}
}

func TestReadOnlyRolesHideMutationTools(t *testing.T) {
	for _, agentType := range []string{AgentTypeExplore, AgentTypePlan, AgentTypeReview} {
		role, ok := LookupRole(agentType)
		if !ok || !role.ReadOnly {
			t.Fatalf("%s role missing", agentType)
		}
		hidden := map[string]bool{}
		for _, name := range role.DisallowedTools {
			hidden[name] = true
		}
		for _, name := range []string{"Write", "Edit", "subagent", "ask_user"} {
			if !hidden[name] {
				t.Fatalf("%s must hide %s", agentType, name)
			}
		}
	}
}

// RP-G10: the root-only update_plan tool is hidden from every child role;
// child progress uses native Subagent facts (运行计划契约 §6.1).
func TestAllRolesHideUpdatePlan(t *testing.T) {
	for agentType := range RoleCatalog() {
		role, ok := LookupRole(agentType)
		if !ok {
			t.Fatalf("%s role missing", agentType)
		}
		hidden := false
		for _, name := range role.DisallowedTools {
			if name == "update_plan" {
				hidden = true
			}
		}
		if !hidden {
			t.Fatalf("%s must hide update_plan", agentType)
		}
	}
}
