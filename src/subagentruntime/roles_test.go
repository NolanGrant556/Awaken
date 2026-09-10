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
		{AgentTypeGeneral, WorkspaceSharedReadOnly, ""},
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
	if NormalizeAgentType("") != AgentTypeGeneral {
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
// child progress uses native Subagent facts ([工具定义](docs/02_主智能体闭环/运行计划/实现方案.md#工具定义)).
func TestSpecialistRolesHideUpdatePlan(t *testing.T) {
	for agentType := range RoleCatalog() {
		if agentType == AgentTypeGeneral {
			continue
		}
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

func TestSpecialistRolesHideArchivedHistory(t *testing.T) {
	for agentType := range RoleCatalog() {
		if agentType == AgentTypeGeneral {
			continue
		}
		role, ok := LookupRole(agentType)
		if !ok {
			t.Fatalf("%s role missing", agentType)
		}
		hidden := false
		for _, name := range role.DisallowedTools {
			if name == "archived_history" {
				hidden = true
			}
		}
		if !hidden {
			t.Fatalf("%s must hide archived_history", agentType)
		}
	}
}

func TestPluginRoleResolverUsesNativeValidationAndDefaults(t *testing.T) {
	SetRoleResolver(func(agentType string) (Role, bool) {
		if agentType != "plugin__acme__reviewer" {
			return Role{}, false
		}
		return Role{Type: agentType, ReadOnly: true, SystemPrompt: "review", AllowedTools: []string{"Read"}}, true
	})
	t.Cleanup(func() { SetRoleResolver(nil) })
	role, ok := LookupRole("plugin__acme__reviewer")
	if !ok || !role.ReadOnly || NormalizeWorkspaceMode(role.Type, "") != WorkspaceSharedReadOnly {
		t.Fatalf("role=%#v ok=%v", role, ok)
	}
	if err := ValidateSpawnCombo(role.Type, WorkspaceSharedReadOnly); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSpawnCombo(role.Type, WorkspaceShared); err == nil {
		t.Fatal("read-only Plugin Agent gained write mode")
	}
}

func TestPluginRoleResolverAcceptsVendorAliasButKeepsCanonicalIdentityStrict(t *testing.T) {
	SetRoleResolver(func(agentType string) (Role, bool) {
		switch agentType {
		case "agents-memory-updater", "plugin__continual-learning__agents-memory-updater":
			return Role{Type: "plugin__continual-learning__agents-memory-updater", ReadOnly: false}, true
		case "plugin__forged__agents-memory-updater":
			return Role{Type: "plugin__continual-learning__agents-memory-updater", ReadOnly: false}, true
		default:
			return Role{}, false
		}
	})
	t.Cleanup(func() { SetRoleResolver(nil) })

	role, ok := LookupRole("agents-memory-updater")
	if !ok || role.Type != "plugin__continual-learning__agents-memory-updater" {
		t.Fatalf("vendor alias did not resolve to canonical Plugin Agent: %#v, %v", role, ok)
	}
	if role, ok := LookupRole("plugin__forged__agents-memory-updater"); ok {
		t.Fatalf("canonical identity mismatch must be rejected: %#v", role)
	}
}
