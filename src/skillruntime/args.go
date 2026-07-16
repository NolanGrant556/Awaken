package skillruntime

import (
	"fmt"
	"strings"
)

// SubstitutionError describes a missing required argument or type mismatch.
type SubstitutionError struct {
	SkillID string
	Message string
}

func (e *SubstitutionError) Error() string {
	return fmt.Sprintf("skill %q argument error: %s", e.SkillID, e.Message)
}

// SubstituteArgs replaces ${arg_name} placeholders in the skill body with
// values from the provided arguments map. It validates required arguments
// and handles default values. Escaped \${...} sequences are preserved as
// literal ${...}.
func SubstituteArgs(body string, skillID string, argDefs []SkillArgDef, args map[string]string) (string, error) {
	// Validate and apply defaults.
	resolved := make(map[string]string, len(argDefs))
	for _, def := range argDefs {
		val, ok := args[def.Name]
		if !ok || val == "" {
			if def.Required {
				return "", &SubstitutionError{
					SkillID: skillID,
					Message: fmt.Sprintf("missing required argument %q", def.Name),
				}
			}
			val = def.Default
		}
		// Type validation: all values arrive as strings; boolean/number
		// checks are best-effort hints.
		if val != "" && def.Type == "boolean" {
			val = strings.ToLower(val)
			if val != "true" && val != "false" {
				return "", &SubstitutionError{
					SkillID: skillID,
					Message: fmt.Sprintf("argument %q must be boolean, got %q", def.Name, val),
				}
			}
		}
		resolved[def.Name] = val
	}

	// Protect escaped placeholders from substitution by temporarily replacing
	// \${ with a sentinel that cannot appear in skill bodies.
	const sentinel = "\x00ESC\x00"
	result := strings.ReplaceAll(body, "\\${", sentinel)

	for _, def := range argDefs {
		placeholder := "${" + def.Name + "}"
		result = strings.ReplaceAll(result, placeholder, resolved[def.Name])
	}

	// Restore escaped placeholders as literal ${.
	result = strings.ReplaceAll(result, sentinel, "${")
	return result, nil
}

// SkillArgDef is the argument definition structure used by the executor.
// It mirrors skills.SkillArgument but lives in this package to avoid
// circular imports.
type SkillArgDef struct {
	Name        string
	Type        string
	Description string
	Required    bool
	Default     string
}
