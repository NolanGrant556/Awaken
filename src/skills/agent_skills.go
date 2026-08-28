package skills

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

var agentSkillNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// ValidateAgentSkillEntrypoint applies the portable Agent Skills metadata
// contract used by Agent Plugins. Awaken keeps its permissive parser for
// existing project/user/legacy sources, but plugin components and newly
// installed authoring candidates must not rely on host-derived metadata.
func ValidateAgentSkillEntrypoint(raw string, expectedDirectoryName string) error {
	fm, _, err := splitFrontmatter(raw)
	if err != nil {
		return err
	}
	fields := map[string]any{}
	if err := yaml.Unmarshal([]byte(fm), &fields); err != nil {
		return fmt.Errorf("parse skill frontmatter: %w", err)
	}

	rawName, present := fields["name"]
	if !present {
		return fmt.Errorf("Agent Skills frontmatter name is required")
	}
	name, ok := rawName.(string)
	if !ok {
		return fmt.Errorf("Agent Skills frontmatter name must be a string")
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("Agent Skills frontmatter name is required")
	}
	if strings.TrimSpace(name) != name {
		return fmt.Errorf("Agent Skills name %q must not include surrounding whitespace", name)
	}
	if utf8.RuneCountInString(name) > 64 || !agentSkillNamePattern.MatchString(name) {
		return fmt.Errorf("Agent Skills name %q must be 1-64 lowercase letters, digits, and hyphens", name)
	}
	if expectedDirectoryName != "" && name != expectedDirectoryName {
		return fmt.Errorf("Agent Skills name %q must match parent directory %q", name, expectedDirectoryName)
	}

	rawDescription, present := fields["description"]
	if !present {
		return fmt.Errorf("Agent Skills frontmatter description is required")
	}
	description, ok := rawDescription.(string)
	if !ok {
		return fmt.Errorf("Agent Skills frontmatter description must be a string")
	}
	if strings.TrimSpace(description) == "" {
		return fmt.Errorf("Agent Skills frontmatter description is required")
	}
	if utf8.RuneCountInString(description) > 1024 {
		return fmt.Errorf("Agent Skills description must not exceed 1024 characters")
	}

	if err := validateOptionalAgentSkillStringField(fields, "license", 0, false); err != nil {
		return err
	}
	if err := validateOptionalAgentSkillStringField(fields, "compatibility", 500, true); err != nil {
		return err
	}
	if raw, present := fields["metadata"]; present {
		if err := validateAgentSkillMetadataField(raw); err != nil {
			return err
		}
	}
	if raw, present := fields["allowed-tools"]; present {
		if err := validateAgentSkillAllowedToolsField(raw); err != nil {
			return err
		}
	}
	return nil
}

func validateOptionalAgentSkillStringField(fields map[string]any, field string, maxRunes int, requireNonEmpty bool) error {
	raw, ok := fields[field]
	if !ok {
		return nil
	}
	if raw == nil {
		return fmt.Errorf("Agent Skills frontmatter %s must be a string when present", field)
	}
	value, ok := raw.(string)
	if !ok {
		return fmt.Errorf("Agent Skills frontmatter %s must be a string when present", field)
	}
	if requireNonEmpty && strings.TrimSpace(value) == "" {
		return fmt.Errorf("Agent Skills frontmatter %s must contain 1-%d characters when present", field, maxRunes)
	}
	if maxRunes > 0 && utf8.RuneCountInString(value) > maxRunes {
		return fmt.Errorf("Agent Skills frontmatter %s must not exceed %d characters", field, maxRunes)
	}
	return nil
}

func validateAgentSkillMetadataField(raw any) error {
	if raw == nil {
		return fmt.Errorf("Agent Skills frontmatter metadata must be a map of string keys to string values")
	}
	switch object := raw.(type) {
	case map[string]any:
		for key, value := range object {
			if value == nil {
				return fmt.Errorf("Agent Skills frontmatter metadata[%q] must be a string", key)
			}
			if _, ok := value.(string); !ok {
				return fmt.Errorf("Agent Skills frontmatter metadata[%q] must be a string", key)
			}
		}
		return nil
	case map[any]any:
		for key, value := range object {
			keyText, ok := key.(string)
			if !ok {
				return fmt.Errorf("Agent Skills frontmatter metadata keys must be strings")
			}
			if value == nil {
				return fmt.Errorf("Agent Skills frontmatter metadata[%q] must be a string", keyText)
			}
			if _, ok := value.(string); !ok {
				return fmt.Errorf("Agent Skills frontmatter metadata[%q] must be a string", keyText)
			}
		}
		return nil
	default:
		return fmt.Errorf("Agent Skills frontmatter metadata must be a map of string keys to string values")
	}
}

func validateAgentSkillAllowedToolsField(raw any) error {
	if raw == nil {
		return fmt.Errorf("Agent Skills frontmatter allowed-tools must be a string when present")
	}
	if _, ok := raw.(string); !ok {
		return fmt.Errorf("Agent Skills frontmatter allowed-tools must be a string when present")
	}
	return nil
}
