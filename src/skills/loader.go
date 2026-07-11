package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// sourceDir returns the default directory for a given local source origin.
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

// LoadDir scans a skills root for package directories in the form
// <skill-id>/SKILL.md and returns a map of ID -> definition. Flat .md files
// are intentionally ignored; skills are directory packages so references,
// scripts, and assets can stay beside SKILL.md.
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
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		if strings.TrimSpace(dirName) == "" || strings.HasPrefix(dirName, ".") {
			continue
		}

		rootDir := filepath.Join(dir, dirName)
		path := filepath.Join(rootDir, "SKILL.md")
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		def, err := ParseFrontmatter(string(raw))
		if err != nil {
			continue
		}

		id := SkillIDFromNameOrFallback(dirName, def.Name)
		if strings.TrimSpace(def.Name) == "" {
			def.Name = id
		}

		def.ID = id
		def.Source = source
		def.Scope = source
		def.RootDir = rootDir
		if resolvedRoot, resolveErr := filepath.EvalSymlinks(rootDir); resolveErr == nil {
			def.ResolvedRootDir = resolvedRoot
		}
		def.SkillFilePath = path
		def.FilePath = path
		if source == SourceProject {
			def.ProjectPath = projectPathFromSkillsRoot(dir)
		}
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
		if def.ToolPolicy.Mode == "" {
			def.ToolPolicy.Mode = ToolPolicyIntersect
		}
		if ApplyPlatformContractConflict(def) {
			result[id] = def
			continue
		}

		result[id] = def
	}
	return result
}

func projectPathFromSkillsRoot(dir string) string {
	clean := filepath.Clean(dir)
	if filepath.Base(clean) != "skills" {
		return ""
	}
	awakenDir := filepath.Dir(clean)
	if filepath.Base(awakenDir) != ".awaken" {
		return ""
	}
	return filepath.Dir(awakenDir)
}

func SkillIDFromNameOrFallback(fallback string, skillName string) string {
	base := strings.TrimSpace(skillName)
	if base == "" {
		base = strings.TrimSpace(fallback)
	}
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(base) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ' || r == '.':
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		default:
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	id := strings.Trim(b.String(), "-")
	if id == "" {
		return "custom-skill"
	}
	return id
}

// MergeSkills merges skill maps from multiple sources by priority:
// project > user. A higher-priority skill shadows a lower-
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
	c.RequiredTools = append([]string(nil), def.RequiredTools...)
	c.RequiredCapabilities = append([]string(nil), def.RequiredCapabilities...)
	c.RequiredRuntimeComponents = append([]string(nil), def.RequiredRuntimeComponents...)
	c.RequiredResources = append([]RequiredResource(nil), def.RequiredResources...)
	c.ToolPolicy.Allow = append([]string(nil), def.ToolPolicy.Allow...)
	c.ToolPolicy.Deny = append([]string(nil), def.ToolPolicy.Deny...)
	c.RequiredConnectors = append([]string(nil), def.RequiredConnectors...)
	c.RequiredMcpServers = append([]string(nil), def.RequiredMcpServers...)
	c.DeprecatedWarnings = append([]string(nil), def.DeprecatedWarnings...)
	if def.McpBinding != nil {
		mb := *def.McpBinding
		c.McpBinding = &mb
	}
	return &c
}
