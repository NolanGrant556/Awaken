package assistantcore

import (
	"strings"
	"testing"
)

func TestNormalizeAssistantValidatesRequiredFieldsAndPermissionMode(t *testing.T) {
	assistant := Assistant{
		ID:      " ",
		Profile: AssistantProfile{Name: " "},
		Binding: RuntimeBinding{},
		Defaults: AssistantDefaults{
			Permission: ScalarDefault{Mode: DefaultModeFixed, Value: "danger"},
		},
	}

	_, err := NormalizeAssistant(assistant)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	for _, want := range []string{
		"id is required",
		"profile.name is required",
		"binding.agent_id is required",
		"binding.runtime_id is required",
		"defaults.permission.value \"danger\" is invalid",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %q", want, err.Error())
		}
	}
}

func TestNormalizeAssistantDedupesSkillsAndMCPServers(t *testing.T) {
	assistant := validAssistant("assistant", AssistantSourceUser, 0)
	assistant.Defaults.Skills = ListDefault{
		Mode:  DefaultModeFixed,
		Value: []string{"read", "write", "read", " ", "review"},
	}
	assistant.Defaults.MCPServers = ListDefault{
		Mode:  DefaultModeFixed,
		Value: []string{"fs", "git", "fs", ""},
	}
	assistant.Capabilities.DisabledBuiltinSkillIDs = []string{"read", "read", " "}
	assistant.Capabilities.RequiredMCPServerIDs = []string{"fs", "fs", "db"}

	normalized, err := NormalizeAssistant(assistant)
	if err != nil {
		t.Fatalf("expected assistant to be valid: %v", err)
	}

	assertStrings(t, normalized.Defaults.Skills.Value, []string{"read", "write", "review"})
	assertStrings(t, normalized.Defaults.MCPServers.Value, []string{"fs", "git"})
	assertStrings(t, normalized.Capabilities.DisabledBuiltinSkillIDs, []string{"read"})
	assertStrings(t, normalized.Capabilities.RequiredMCPServerIDs, []string{"fs", "db"})
}

func TestValidateAssistantUpdateEnforcesBuiltinReadOnlyFields(t *testing.T) {
	before := validAssistant("builtin", AssistantSourceBuiltin, 0)
	allowed := before
	allowed.Binding.AgentID = "worker"
	allowed.Defaults.Model.Value = "claude-opus"
	allowed.Defaults.Permission.Value = string(PermissionModePlan)

	if err := ValidateAssistantUpdate(before, allowed); err != nil {
		t.Fatalf("expected builtin binding/model/permission update to be allowed: %v", err)
	}

	after := before
	after.Profile.Name = "Renamed"
	after.Rules.Content = "new rules"
	after.Defaults.Skills.Value = []string{"other"}

	err := ValidateAssistantUpdate(before, after)
	if err == nil {
		t.Fatalf("expected builtin read-only validation error")
	}
	for _, want := range []string{"profile is read-only", "rules is read-only", "defaults.skills is read-only"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %q", want, err.Error())
		}
	}
}

func TestValidateAssistantUpdateEnforcesGeneratedReadOnlyFields(t *testing.T) {
	before := validAssistant("generated", AssistantSourceGenerated, 0)
	allowed := before
	allowed.Profile.Description = "locally edited description"
	allowed.Defaults.Skills.Value = []string{"review"}
	allowed.Rules.Content = "local generated assistant rules"

	if err := ValidateAssistantUpdate(before, allowed); err != nil {
		t.Fatalf("expected generated description/default/rules update to be allowed: %v", err)
	}

	after := before
	after.Profile.Name = "Renamed"
	after.Profile.Avatar = "avatar.png"
	after.Binding.RuntimeID = "other-runtime"

	err := ValidateAssistantUpdate(before, after)
	if err == nil {
		t.Fatalf("expected generated read-only validation error")
	}
	for _, want := range []string{
		"profile.name is read-only for generated assistants",
		"profile.avatar is read-only for generated assistants",
		"binding is read-only",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %q", want, err.Error())
		}
	}
}
