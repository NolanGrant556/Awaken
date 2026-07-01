package skills

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseFrontmatter extracts YAML frontmatter and Markdown body from raw
// SKILL.md content. The frontmatter is delimited by --- lines.
// Returns the parsed definition and any error. On YAML parse failure the
// error is returned so callers can log and skip the file.
func ParseFrontmatter(raw string) (*SkillDefinition, error) {
	fm, body, err := splitFrontmatter(raw)
	if err != nil {
		return nil, err
	}

	var def SkillDefinition
	if err := yaml.Unmarshal([]byte(fm), &def); err != nil {
		return nil, fmt.Errorf("parse skill frontmatter: %w", err)
	}
	def.Body = strings.TrimSpace(body)
	return &def, nil
}

// ValidateSchema checks required fields and type constraints on a parsed
// SkillDefinition. Returns nil when the definition is usable.
func ValidateSchema(def *SkillDefinition) error {
	if def == nil {
		return fmt.Errorf("skill definition is nil")
	}
	if strings.TrimSpace(def.Name) == "" {
		return fmt.Errorf("skill name is required")
	}
	if strings.TrimSpace(def.Description) == "" {
		return fmt.Errorf("skill description is required (id=%q)", def.ID)
	}
	for i, arg := range def.Arguments {
		if strings.TrimSpace(arg.Name) == "" {
			return fmt.Errorf("skill %q argument[%d]: name is required", def.ID, i)
		}
		if !isValidArgType(arg.Type) {
			return fmt.Errorf("skill %q argument %q: invalid type %q (must be string, number, integer, boolean)", def.ID, arg.Name, arg.Type)
		}
	}
	if def.Context != "" && def.Context != ContextInline && def.Context != ContextFork {
		return fmt.Errorf("skill %q: invalid context mode %q (must be inline or fork)", def.ID, def.Context)
	}
	return nil
}

func isValidArgType(t string) bool {
	switch t {
	case "string", "number", "integer", "boolean":
		return true
	default:
		return false
	}
}

// splitFrontmatter extracts the YAML block between the first two ---
// delimiters. Returns frontmatter text, body text, and any error.
func splitFrontmatter(raw string) (string, string, error) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "---") {
		return "", "", fmt.Errorf("skill file must start with --- frontmatter delimiter")
	}

	rest := trimmed[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		// Maybe the second --- is at end-of-line without a preceding newline
		// (single-line frontmatter degeneracy).
		idx = strings.Index(rest, "---")
		if idx < 0 {
			return "", "", fmt.Errorf("skill file missing closing --- frontmatter delimiter")
		}
		// The --- is at the beginning of rest if there's no content.
	}

	fm := rest[:idx]
	// idx points at the '\n' before "---", so body starts after "\n---" (4 chars).
	bodyStart := idx + 4
	// Handle degenerate case where "---" is at the beginning without a
	// preceding newline (single-line or bare ---).
	if bodyStart > len(rest) {
		bodyStart = idx + 3
	}
	body := rest[bodyStart:]

	return strings.TrimSpace(fm), body, nil
}