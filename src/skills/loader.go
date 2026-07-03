package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// sourceDir returns the default directory for a given source origin.
// bundled skills are embedded, so we only resolve user and project dirs.
func sourceDir(source SourceOrigin) string {
	switch source {
	case SourceUser:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".awaken", "skills")
	case SourceProject:
		return ".awaken/skills"
	default:
		return ""
	}
}

// LoadDir scans a directory for .md files, parses each as a SKILL.md,
// and returns a map of ID -> definition. Files that fail to parse are
// skipped (the caller should log warnings).
// source labels every loaded definition.
func LoadDir(dir string, source SourceOrigin) map[string]*SkillDefinition {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	result := make(map[string]*SkillDefinition)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}

		id := strings.TrimSuffix(name, ".md")
		if id == "" {
			continue
		}

		path := filepath.Join(dir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		def, err := ParseFrontmatter(string(raw))
		if err != nil {
			continue
		}

		def.ID = id
		def.Source = source
		def.FilePath = path
		def.State = StateAvailable

		if err := ValidateNameReserved(def.Name); err != nil {
			def.State = StateUnavailable
			def.UnavailableReason = err.Error()
			result[id] = def
			continue
		}

		if err := ValidateSchema(def); err != nil {
			def.State = StateUnavailable
			def.UnavailableReason = err.Error()
			result[id] = def
			continue
		}

		// Default context to inline.
		if def.Context == "" {
			def.Context = ContextInline
		}

		result[id] = def
	}
	return result
}

// MergeSkills merges skill maps from multiple sources by priority:
// project > user > bundled. A higher-priority skill shadows a lower-
// priority one: the lower-priority entry is kept with StateShadowed and
// ShadowedBy set.
func MergeSkills(sources ...map[string]*SkillDefinition) map[string]*SkillDefinition {
	merged := make(map[string]*SkillDefinition)
	for _, src := range sources {
		if src == nil {
			continue
		}
		for id, def := range src {
			if def == nil {
				continue
			}
			existing, ok := merged[id]
			if !ok {
				merged[id] = shallowCopy(def)
				continue
			}
			if sourcePriority(def.Source) > sourcePriority(existing.Source) {
				// existing gets shadowed.
				existing.State = StateShadowed
				existing.ShadowedBy = fmt.Sprintf("%s(%s)", def.Source, def.FilePath)
				merged[id] = shallowCopy(def)
				// Store shadowed entry under a synthetic key so it appears in listing.
				merged[id+"__shadowed"] = existing
			} else {
				// New entry is lower priority; shadow it.
				def.State = StateShadowed
				def.ShadowedBy = fmt.Sprintf("%s(%s)", existing.Source, existing.FilePath)
				merged[id+"__shadowed"] = def
			}
		}
	}
	return merged
}

func shallowCopy(def *SkillDefinition) *SkillDefinition {
	c := *def
	c.Arguments = append([]SkillArgument(nil), def.Arguments...)
	c.AllowedTools = append([]string(nil), def.AllowedTools...)
	c.RequiredConnectors = append([]string(nil), def.RequiredConnectors...)
	c.RequiredMcpServers = append([]string(nil), def.RequiredMcpServers...)
	if def.McpBinding != nil {
		mb := *def.McpBinding
		c.McpBinding = &mb
	}
	return &c
}