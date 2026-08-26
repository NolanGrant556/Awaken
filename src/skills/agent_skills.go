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

	name, ok := fields["name"].(string)
	name = strings.TrimSpace(name)
	if !ok || name == "" {
		return fmt.Errorf("Agent Skills frontmatter name is required")
	}
	if len(name) > 64 || !agentSkillNamePattern.MatchString(name) {
		return fmt.Errorf("Agent Skills name %q must be 1-64 lowercase ASCII letters, digits, and single hyphens", name)
	}
	if expected := strings.TrimSpace(expectedDirectoryName); expected != "" && name != expected {
		return fmt.Errorf("Agent Skills name %q must match parent directory %q", name, expected)
	}

	description, ok := fields["description"].(string)
	description = strings.TrimSpace(description)
	if !ok || description == "" {
		return fmt.Errorf("Agent Skills frontmatter description is required")
	}
	if utf8.RuneCountInString(description) > 1024 {
		return fmt.Errorf("Agent Skills description must not exceed 1024 characters")
	}
	return nil
}
