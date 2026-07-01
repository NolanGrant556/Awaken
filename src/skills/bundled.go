package skills

import "embed"

//go:embed bundled/*.md
var bundledFS embed.FS

// LoadBundledSkills reads all embedded SKILL.md files from the bundled/
// directory and returns them parsed and validated.
func LoadBundledSkills() map[string]*SkillDefinition {
	entries, err := bundledFS.ReadDir("bundled")
	if err != nil {
		return nil
	}
	result := make(map[string]*SkillDefinition)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) <= 3 || name[len(name)-3:] != ".md" {
			continue
		}
		id := name[:len(name)-3]
		raw, err := bundledFS.ReadFile("bundled/" + name)
		if err != nil {
			continue
		}
		def, err := ParseFrontmatter(string(raw))
		if err != nil {
			continue
		}
		def.ID = id
		def.Source = SourceBundled
		def.FilePath = "bundled://" + name
		def.State = StateAvailable
		if def.Context == "" {
			def.Context = ContextInline
		}
		if err := ValidateSchema(def); err != nil {
			def.State = StateUnavailable
			def.UnavailableReason = err.Error()
		}
		result[id] = def
	}
	return result
}