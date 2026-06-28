package assistantcore

import (
	"fmt"
	"strings"
)

func EffectiveConfig(assistant Assistant, overrides RunOverrides) (RunConfig, error) {
	normalized, err := NormalizeAssistant(assistant)
	if err != nil {
		return RunConfig{}, err
	}

	binding := normalized.Binding
	if overrides.Binding != nil {
		binding = normalizeBinding(*overrides.Binding)
		if problems := validateBinding(binding, "overrides.binding"); len(problems) > 0 {
			return RunConfig{}, &ValidationError{Problems: problems}
		}
	}

	model := scalarDefaultValue(normalized.Defaults.Model)
	if overrides.Model != nil {
		model = strings.TrimSpace(*overrides.Model)
	}

	permissionMode := PermissionModeDefault
	if normalized.Defaults.Permission.Mode == DefaultModeFixed && normalized.Defaults.Permission.Value != "" {
		permissionMode = PermissionMode(normalized.Defaults.Permission.Value)
	}
	if overrides.PermissionMode != nil {
		permissionMode = PermissionMode(strings.TrimSpace(string(*overrides.PermissionMode)))
		if !IsValidPermissionMode(permissionMode) {
			return RunConfig{}, fmt.Errorf("overrides.permission_mode %q is invalid", permissionMode)
		}
	}

	skillIDs := listDefaultValue(normalized.Defaults.Skills)
	if normalized.Defaults.Skills.Mode == DefaultModeAuto {
		skillIDs = append([]string(nil), normalized.Capabilities.DefaultSkillIDs...)
	}
	if overrides.SkillIDs != nil {
		skillIDs = normalizeStringSlice(overrides.SkillIDs)
	}

	disabledBuiltinSkillIDs := normalized.Capabilities.DisabledBuiltinSkillIDs
	if overrides.DisabledBuiltinSkillIDs != nil {
		disabledBuiltinSkillIDs = normalizeStringSlice(overrides.DisabledBuiltinSkillIDs)
	}
	skillIDs = withoutValues(skillIDs, disabledBuiltinSkillIDs)

	mcpServerIDs := listDefaultValue(normalized.Defaults.MCPServers)
	if overrides.MCPServerIDs != nil {
		mcpServerIDs = normalizeStringSlice(overrides.MCPServerIDs)
	}

	rules := normalized.Rules.Content
	if overrides.Rules != nil {
		rules = strings.TrimSpace(*overrides.Rules)
	}

	systemPrompt := normalized.Rules.SystemPrompt
	if overrides.SystemPrompt != nil {
		systemPrompt = strings.TrimSpace(*overrides.SystemPrompt)
	}

	return RunConfig{
		AssistantID:             normalized.ID,
		AssistantName:           normalized.Profile.Name,
		Binding:                 binding,
		Model:                   model,
		PermissionMode:          permissionMode,
		SkillIDs:                append([]string(nil), skillIDs...),
		DisabledBuiltinSkillIDs: append([]string(nil), disabledBuiltinSkillIDs...),
		MCPServerIDs:            append([]string(nil), mcpServerIDs...),
		Rules:                   rules,
		SystemPrompt:            systemPrompt,
	}, nil
}

func scalarDefaultValue(defaultValue ScalarDefault) string {
	if defaultValue.Mode != DefaultModeFixed {
		return ""
	}
	return defaultValue.Value
}

func listDefaultValue(defaultValue ListDefault) []string {
	if defaultValue.Mode != DefaultModeFixed {
		return nil
	}
	return append([]string(nil), defaultValue.Value...)
}

func withoutValues(values []string, disabled []string) []string {
	if len(values) == 0 || len(disabled) == 0 {
		return append([]string(nil), values...)
	}
	disabledSet := make(map[string]struct{}, len(disabled))
	for _, value := range disabled {
		disabledSet[value] = struct{}{}
	}
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := disabledSet[value]; ok {
			continue
		}
		filtered = append(filtered, value)
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}
