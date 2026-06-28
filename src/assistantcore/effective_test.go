package assistantcore

import "testing"

func TestEffectiveConfigUsesAssistantDefaults(t *testing.T) {
	assistant := validAssistant("assistant", AssistantSourceUser, 0)
	assistant.Defaults = AssistantDefaults{
		Model:      ScalarDefault{Mode: DefaultModeFixed, Value: "claude-sonnet"},
		Permission: ScalarDefault{Mode: DefaultModeFixed, Value: string(PermissionModePlan)},
		Skills:     ListDefault{Mode: DefaultModeFixed, Value: []string{"read", "review", "read", "auto-skill"}},
		MCPServers: ListDefault{Mode: DefaultModeFixed, Value: []string{"fs", "git", "fs"}},
	}
	assistant.Capabilities.DisabledBuiltinSkillIDs = []string{"auto-skill", "auto-skill"}
	assistant.Rules = AssistantRules{
		Content:      "follow project rules",
		SystemPrompt: "you are an assistant",
	}

	config, err := EffectiveConfig(assistant, RunOverrides{})
	if err != nil {
		t.Fatalf("expected effective config: %v", err)
	}

	if config.AssistantID != "assistant" {
		t.Fatalf("unexpected assistant id: %q", config.AssistantID)
	}
	if config.Model != "claude-sonnet" {
		t.Fatalf("unexpected model: %q", config.Model)
	}
	if config.PermissionMode != PermissionModePlan {
		t.Fatalf("unexpected permission mode: %q", config.PermissionMode)
	}
	assertStrings(t, config.SkillIDs, []string{"read", "review"})
	assertStrings(t, config.DisabledBuiltinSkillIDs, []string{"auto-skill"})
	assertStrings(t, config.MCPServerIDs, []string{"fs", "git"})
	if config.Rules != "follow project rules" {
		t.Fatalf("unexpected rules: %q", config.Rules)
	}
	if config.SystemPrompt != "you are an assistant" {
		t.Fatalf("unexpected system prompt: %q", config.SystemPrompt)
	}
}

func TestEffectiveConfigAppliesRunOverrides(t *testing.T) {
	assistant := validAssistant("assistant", AssistantSourceUser, 0)
	assistant.Defaults.Skills = ListDefault{Mode: DefaultModeFixed, Value: []string{"read", "write"}}
	assistant.Defaults.MCPServers = ListDefault{Mode: DefaultModeFixed, Value: []string{"fs"}}

	model := "claude-opus"
	permission := PermissionModeAcceptEdits
	rules := "run-specific rules"
	systemPrompt := "run-specific system prompt"
	binding := RuntimeBinding{AgentID: "worker", RuntimeID: "codex", Source: "custom"}

	config, err := EffectiveConfig(assistant, RunOverrides{
		Binding:                 &binding,
		Model:                   &model,
		PermissionMode:          &permission,
		SkillIDs:                []string{"debug", "debug", "disabled-auto"},
		DisabledBuiltinSkillIDs: []string{"disabled-auto"},
		MCPServerIDs:            []string{"db", "db"},
		Rules:                   &rules,
		SystemPrompt:            &systemPrompt,
	})
	if err != nil {
		t.Fatalf("expected effective config: %v", err)
	}

	if config.Binding.AgentID != "worker" || config.Binding.RuntimeID != "codex" {
		t.Fatalf("unexpected binding: %+v", config.Binding)
	}
	if config.Model != model {
		t.Fatalf("unexpected model: %q", config.Model)
	}
	if config.PermissionMode != permission {
		t.Fatalf("unexpected permission: %q", config.PermissionMode)
	}
	assertStrings(t, config.SkillIDs, []string{"debug"})
	assertStrings(t, config.DisabledBuiltinSkillIDs, []string{"disabled-auto"})
	assertStrings(t, config.MCPServerIDs, []string{"db"})
	if config.Rules != rules {
		t.Fatalf("unexpected rules: %q", config.Rules)
	}
	if config.SystemPrompt != systemPrompt {
		t.Fatalf("unexpected system prompt: %q", config.SystemPrompt)
	}
}

func TestEffectiveConfigUsesCapabilitySkillsForAutoSkillDefaults(t *testing.T) {
	assistant := validAssistant("assistant", AssistantSourceUser, 0)
	assistant.Defaults.Skills = ListDefault{Mode: DefaultModeAuto}
	assistant.Capabilities.DefaultSkillIDs = []string{"auto-read", "auto-write"}
	assistant.Capabilities.DisabledBuiltinSkillIDs = []string{"auto-write"}

	config, err := EffectiveConfig(assistant, RunOverrides{})
	if err != nil {
		t.Fatalf("expected effective config: %v", err)
	}

	assertStrings(t, config.SkillIDs, []string{"auto-read"})
	assertStrings(t, config.DisabledBuiltinSkillIDs, []string{"auto-write"})
}

func TestEffectiveConfigRejectsInvalidOverrideBinding(t *testing.T) {
	assistant := validAssistant("assistant", AssistantSourceUser, 0)
	binding := RuntimeBinding{AgentID: "worker"}

	_, err := EffectiveConfig(assistant, RunOverrides{Binding: &binding})
	if err == nil {
		t.Fatalf("expected missing runtime binding error")
	}
}
