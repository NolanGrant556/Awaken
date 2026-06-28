package assistantcore

import (
	"fmt"
	"reflect"
	"strings"
)

type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return strings.Join(e.Problems, "; ")
}

func IsValidPermissionMode(mode PermissionMode) bool {
	switch mode {
	case PermissionModeDefault,
		PermissionModePlan,
		PermissionModeAcceptEdits,
		PermissionModeBypassPermissions,
		PermissionModeDontAsk,
		PermissionModeAuto,
		PermissionModeBubble:
		return true
	default:
		return false
	}
}

func ValidateAssistant(assistant Assistant) error {
	_, err := NormalizeAssistant(assistant)
	return err
}

func NormalizeAssistant(assistant Assistant) (Assistant, error) {
	normalized := cloneAssistant(assistant)
	normalized.ID = strings.TrimSpace(normalized.ID)
	if normalized.Source == "" {
		normalized.Source = AssistantSourceUser
	}

	normalized.Profile = normalizeProfile(normalized.Profile)
	normalized.Binding = normalizeBinding(normalized.Binding)
	normalized.Rules = normalizeRules(normalized.Rules)
	normalized.Defaults = normalizeDefaults(normalized.Defaults)
	normalized.Capabilities = normalizeCapabilities(normalized.Capabilities)
	normalized.Preferences = normalizePreferences(normalized.Preferences)
	normalized.Presets = normalizePresets(normalized.Presets)

	var problems []string
	if normalized.ID == "" {
		problems = append(problems, "id is required")
	}
	if normalized.Profile.Name == "" {
		problems = append(problems, "profile.name is required")
	}
	switch normalized.Source {
	case AssistantSourceBuiltin, AssistantSourceGenerated, AssistantSourceUser:
	default:
		problems = append(problems, fmt.Sprintf("source %q is invalid", normalized.Source))
	}
	problems = append(problems, validateBinding(normalized.Binding, "binding")...)
	problems = append(problems, validateDefaults(normalized.Defaults, "defaults")...)

	if len(problems) > 0 {
		return Assistant{}, &ValidationError{Problems: problems}
	}
	return normalized, nil
}

func ValidateAssistantUpdate(before Assistant, after Assistant) error {
	normalizedBefore, err := NormalizeAssistant(before)
	if err != nil {
		return fmt.Errorf("existing assistant is invalid: %w", err)
	}
	normalizedAfter, err := NormalizeAssistant(after)
	if err != nil {
		return err
	}

	var problems []string
	if normalizedBefore.ID != normalizedAfter.ID {
		problems = append(problems, "id is read-only")
	}
	if normalizedBefore.Source != normalizedAfter.Source {
		problems = append(problems, "source is read-only")
	}

	switch normalizedBefore.Source {
	case AssistantSourceBuiltin:
		problems = appendReadOnlyDiff(problems, "profile", normalizedBefore.Profile, normalizedAfter.Profile)
		problems = appendReadOnlyDiff(problems, "rules", normalizedBefore.Rules, normalizedAfter.Rules)
		problems = appendReadOnlyDiff(problems, "capabilities", normalizedBefore.Capabilities, normalizedAfter.Capabilities)
		problems = appendReadOnlyDiff(problems, "preferences", normalizedBefore.Preferences, normalizedAfter.Preferences)
		problems = appendReadOnlyDiff(problems, "presets", normalizedBefore.Presets, normalizedAfter.Presets)
		problems = appendReadOnlyDiff(problems, "defaults.skills", normalizedBefore.Defaults.Skills, normalizedAfter.Defaults.Skills)
		problems = appendReadOnlyDiff(problems, "defaults.mcps", normalizedBefore.Defaults.MCPServers, normalizedAfter.Defaults.MCPServers)
	case AssistantSourceGenerated:
		if normalizedBefore.Profile.Name != normalizedAfter.Profile.Name {
			problems = append(problems, "profile.name is read-only for generated assistants")
		}
		if !reflect.DeepEqual(normalizedBefore.Profile.NameI18N, normalizedAfter.Profile.NameI18N) {
			problems = append(problems, "profile.name_i18n is read-only for generated assistants")
		}
		if normalizedBefore.Profile.Avatar != normalizedAfter.Profile.Avatar {
			problems = append(problems, "profile.avatar is read-only for generated assistants")
		}
		problems = appendReadOnlyDiff(problems, "binding", normalizedBefore.Binding, normalizedAfter.Binding)
	}

	if len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}
	return nil
}

func appendReadOnlyDiff[T any](problems []string, field string, before T, after T) []string {
	if reflect.DeepEqual(before, after) {
		return problems
	}
	return append(problems, field+" is read-only")
}

func validateBinding(binding RuntimeBinding, prefix string) []string {
	var problems []string
	if binding.AgentID == "" {
		problems = append(problems, prefix+".agent_id is required")
	}
	if binding.RuntimeID == "" {
		problems = append(problems, prefix+".runtime_id is required")
	}
	return problems
}

func validateDefaults(defaults AssistantDefaults, prefix string) []string {
	var problems []string
	if defaults.Model.Mode == DefaultModeFixed && defaults.Model.Value == "" {
		problems = append(problems, prefix+".model.value is required when mode is fixed")
	}
	if defaults.Permission.Mode == DefaultModeFixed {
		if defaults.Permission.Value == "" {
			problems = append(problems, prefix+".permission.value is required when mode is fixed")
		} else if !IsValidPermissionMode(PermissionMode(defaults.Permission.Value)) {
			problems = append(problems, fmt.Sprintf("%s.permission.value %q is invalid", prefix, defaults.Permission.Value))
		}
	}
	problems = append(problems, validateDefaultMode(defaults.Model.Mode, prefix+".model.mode")...)
	problems = append(problems, validateDefaultMode(defaults.Permission.Mode, prefix+".permission.mode")...)
	problems = append(problems, validateDefaultMode(defaults.Skills.Mode, prefix+".skills.mode")...)
	problems = append(problems, validateDefaultMode(defaults.MCPServers.Mode, prefix+".mcps.mode")...)
	return problems
}

func validateDefaultMode(mode DefaultMode, field string) []string {
	switch mode {
	case DefaultModeAuto, DefaultModeFixed:
		return nil
	default:
		return []string{fmt.Sprintf("%s %q is invalid", field, mode)}
	}
}

func normalizeProfile(profile AssistantProfile) AssistantProfile {
	return AssistantProfile{
		Name:            strings.TrimSpace(profile.Name),
		NameI18N:        cloneStringMap(profile.NameI18N),
		Description:     strings.TrimSpace(profile.Description),
		DescriptionI18N: cloneStringMap(profile.DescriptionI18N),
		Avatar:          strings.TrimSpace(profile.Avatar),
	}
}

func normalizeBinding(binding RuntimeBinding) RuntimeBinding {
	return RuntimeBinding{
		AgentID:   strings.TrimSpace(binding.AgentID),
		RuntimeID: strings.TrimSpace(binding.RuntimeID),
		Source:    strings.TrimSpace(binding.Source),
		Backend:   strings.TrimSpace(binding.Backend),
	}
}

func normalizeRules(rules AssistantRules) AssistantRules {
	return AssistantRules{
		Content:      strings.TrimSpace(rules.Content),
		SystemPrompt: strings.TrimSpace(rules.SystemPrompt),
		StorageMode:  strings.TrimSpace(rules.StorageMode),
	}
}

func normalizeDefaults(defaults AssistantDefaults) AssistantDefaults {
	return AssistantDefaults{
		Model:      normalizeScalarDefault(defaults.Model),
		Permission: normalizeScalarDefault(defaults.Permission),
		Skills:     normalizeListDefault(defaults.Skills),
		MCPServers: normalizeListDefault(defaults.MCPServers),
	}
}

func normalizeScalarDefault(defaultValue ScalarDefault) ScalarDefault {
	mode := defaultValue.Mode
	if mode == "" {
		mode = DefaultModeAuto
	}
	value := strings.TrimSpace(defaultValue.Value)
	if mode != DefaultModeFixed {
		value = ""
	}
	return ScalarDefault{Mode: mode, Value: value}
}

func normalizeListDefault(defaultValue ListDefault) ListDefault {
	mode := defaultValue.Mode
	if mode == "" {
		mode = DefaultModeAuto
	}
	value := normalizeStringSlice(defaultValue.Value)
	if mode != DefaultModeFixed {
		value = nil
	}
	return ListDefault{Mode: mode, Value: value}
}

func normalizeCapabilities(capabilities AssistantCapabilities) AssistantCapabilities {
	return AssistantCapabilities{
		DefaultSkillIDs:         normalizeStringSlice(capabilities.DefaultSkillIDs),
		CustomSkillNames:        normalizeStringSlice(capabilities.CustomSkillNames),
		DisabledBuiltinSkillIDs: normalizeStringSlice(capabilities.DisabledBuiltinSkillIDs),
		RequiredMCPServerIDs:    normalizeStringSlice(capabilities.RequiredMCPServerIDs),
	}
}

func normalizePreferences(preferences AssistantPreferences) AssistantPreferences {
	mode := strings.TrimSpace(preferences.LastPermissionMode)
	if mode != "" && !IsValidPermissionMode(PermissionMode(mode)) {
		mode = ""
	}
	return AssistantPreferences{
		LastModelID:                 strings.TrimSpace(preferences.LastModelID),
		LastPermissionMode:          mode,
		LastSkillIDs:                normalizeStringSlice(preferences.LastSkillIDs),
		LastDisabledBuiltinSkillIDs: normalizeStringSlice(preferences.LastDisabledBuiltinSkillIDs),
		LastMCPServerIDs:            normalizeStringSlice(preferences.LastMCPServerIDs),
	}
}

func normalizePresets(presets []Preset) []Preset {
	if len(presets) == 0 {
		return nil
	}
	normalized := make([]Preset, 0, len(presets))
	for _, preset := range presets {
		preset.ID = strings.TrimSpace(preset.ID)
		preset.Name = strings.TrimSpace(preset.Name)
		preset.Description = strings.TrimSpace(preset.Description)
		preset.Binding = normalizeBinding(preset.Binding)
		preset.Rules = normalizeRules(preset.Rules)
		preset.Defaults = normalizeDefaults(preset.Defaults)
		preset.Metadata = cloneStringMap(preset.Metadata)
		normalized = append(normalized, preset)
	}
	return normalized
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func cloneAssistant(assistant Assistant) Assistant {
	assistant.Profile = normalizeProfile(assistant.Profile)
	assistant.Defaults = cloneDefaults(assistant.Defaults)
	assistant.Capabilities = cloneCapabilities(assistant.Capabilities)
	assistant.Preferences = clonePreferences(assistant.Preferences)
	assistant.Presets = clonePresets(assistant.Presets)
	assistant.Metadata = cloneStringMap(assistant.Metadata)
	return assistant
}

func cloneDefaults(defaults AssistantDefaults) AssistantDefaults {
	defaults.Skills.Value = append([]string(nil), defaults.Skills.Value...)
	defaults.MCPServers.Value = append([]string(nil), defaults.MCPServers.Value...)
	return defaults
}

func cloneCapabilities(capabilities AssistantCapabilities) AssistantCapabilities {
	capabilities.DefaultSkillIDs = append([]string(nil), capabilities.DefaultSkillIDs...)
	capabilities.CustomSkillNames = append([]string(nil), capabilities.CustomSkillNames...)
	capabilities.DisabledBuiltinSkillIDs = append([]string(nil), capabilities.DisabledBuiltinSkillIDs...)
	capabilities.RequiredMCPServerIDs = append([]string(nil), capabilities.RequiredMCPServerIDs...)
	return capabilities
}

func clonePreferences(preferences AssistantPreferences) AssistantPreferences {
	preferences.LastSkillIDs = append([]string(nil), preferences.LastSkillIDs...)
	preferences.LastDisabledBuiltinSkillIDs = append([]string(nil), preferences.LastDisabledBuiltinSkillIDs...)
	preferences.LastMCPServerIDs = append([]string(nil), preferences.LastMCPServerIDs...)
	return preferences
}

func clonePresets(presets []Preset) []Preset {
	if len(presets) == 0 {
		return nil
	}
	cloned := make([]Preset, len(presets))
	for i, preset := range presets {
		preset.Defaults = cloneDefaults(preset.Defaults)
		preset.Metadata = cloneStringMap(preset.Metadata)
		cloned[i] = preset
	}
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		cloned[key] = value
	}
	if len(cloned) == 0 {
		return nil
	}
	return cloned
}
