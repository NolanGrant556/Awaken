package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"awaken/src/pluginidentity"
)

const reservedSystemSkillsDirName = "system-skills"

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
	resolvedSkillsRoot, _ := filepath.EvalSymlinks(dir)
	seenRealPaths := make(map[string]string)
	for _, entry := range entries {
		if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		dirName := entry.Name()
		if strings.TrimSpace(dirName) == "" || strings.HasPrefix(dirName, ".") || strings.EqualFold(dirName, reservedSystemSkillsDirName) {
			continue
		}

		rootDir := filepath.Join(dir, dirName)
		path := filepath.Join(rootDir, "SKILL.md")
		fallbackID := namespacedSkillID(source, dir, SkillIDFromNameOrFallback(dirName, ""))
		base := &SkillDefinition{
			ID: fallbackID, Name: fallbackID, Source: source, Scope: source,
			RootDir: rootDir, SourceRoot: resolvedSkillsRoot, SkillFilePath: path, FilePath: path,
			State: StateInvalid, Context: ContextInline, UserInvocable: true,
		}
		resolvedRoot, resolveErr := filepath.EvalSymlinks(rootDir)
		if resolveErr != nil {
			base.UnavailableReason = fmt.Sprintf("resolve skill package root: %v", resolveErr)
			base.Diagnostics = []SkillDiagnostic{{Code: "skill_package_root_invalid", Message: base.UnavailableReason, Severity: "error"}}
			result[fallbackID] = base
			continue
		}
		base.ResolvedRootDir = resolvedRoot
		if resolvedSkillsRoot == "" || !pathWithinRoot(resolvedRoot, resolvedSkillsRoot) {
			base.UnavailableReason = "skill package symlink escapes the configured skills root"
			base.Diagnostics = []SkillDiagnostic{{Code: "skill_package_root_escape", Message: base.UnavailableReason, Severity: "error"}}
			result[fallbackID] = base
			continue
		}
		if firstID, duplicate := seenRealPaths[resolvedRoot]; duplicate {
			base.State = StateShadowed
			base.ShadowedBy = firstID + "(" + resolvedRoot + ")"
			result[shadowedMapKey(fallbackID, source, len(result))] = base
			continue
		}
		seenRealPaths[resolvedRoot] = fallbackID
		raw, err := os.ReadFile(path)
		if err != nil {
			base.UnavailableReason = fmt.Sprintf("read SKILL.md: %v", err)
			base.Diagnostics = []SkillDiagnostic{{Code: "skill_file_unreadable", Message: base.UnavailableReason, Severity: "error"}}
			result[fallbackID] = base
			continue
		}
		if source == SourcePlugin {
			if err := ValidateAgentSkillEntrypoint(string(raw), dirName); err != nil {
				base.UnavailableReason = err.Error()
				base.Diagnostics = []SkillDiagnostic{{Code: "skill_agent_spec_invalid", Message: base.UnavailableReason, Severity: "error"}}
				result[fallbackID] = base
				continue
			}
		}

		def, err := ParseFrontmatter(string(raw))
		if err != nil {
			base.UnavailableReason = err.Error()
			base.Diagnostics = []SkillDiagnostic{{Code: "skill_frontmatter_invalid", Message: base.UnavailableReason, Severity: "error"}}
			result[fallbackID] = base
			continue
		}

		id := namespacedSkillID(source, dir, SkillIDFromNameOrFallback(dirName, def.Name))
		if strings.TrimSpace(def.Name) == "" {
			def.Name = SkillIDFromNameOrFallback(dirName, def.Name)
		}

		def.ID = id
		def.Source = source
		def.Scope = source
		def.RootDir = rootDir
		def.SourceRoot = resolvedSkillsRoot
		def.ResolvedRootDir = resolvedRoot
		def.SkillFilePath = path
		def.FilePath = path
		if source == SourceProject {
			def.ProjectPath = projectPathFromSkillsRoot(dir)
		}
		if source == SourceAgents {
			if projectPath := projectPathFromSkillsRoot(dir); projectPath != "" {
				if home, homeErr := os.UserHomeDir(); homeErr != nil || !isSameRealPath(projectPath, home) {
					def.ProjectPath = projectPath
				}
			}
		}
		def.State = StateLoaded

		if err := ValidateNameReserved(def.Name); err != nil {
			def.State = StateInvalid
			def.UnavailableReason = err.Error()
			def.Diagnostics = append(def.Diagnostics, SkillDiagnostic{Code: "skill_reserved_name", Message: err.Error(), Severity: "error"})
			result[id] = def
			continue
		}

		if err := ValidateSchema(def); err != nil {
			def.State = StateInvalid
			def.UnavailableReason = err.Error()
			def.Diagnostics = append(def.Diagnostics, SkillDiagnostic{Code: "skill_schema_invalid", Message: err.Error(), Severity: "error"})
			result[id] = def
			continue
		}

		// Default context to inline.
		if def.Context == "" {
			def.Context = ContextInline
		}
		// Catalog discovery retains metadata only. The full body is re-read from
		// the anchored SKILL.md path when the unified skill tool is invoked.
		def.Body = ""
		result[id] = def
	}
	return result
}

// LoadPluginSkillSource loads either one exact Store-selected Skill package
// or, for legacy test/compatibility callers, a fixed Skill collection root.
func LoadPluginSkillSource(dir string) map[string]*SkillDefinition {
	selection, ok := pluginSkillSelection(dir)
	if !ok {
		return LoadDir(dir, SourcePlugin)
	}
	return LoadPluginSkillSelection(selection)
}

// LoadPluginSkillSelection parses one exact Plugin-owned Skill package without
// consulting mutable discovery globals. Callers use it for digest-bound token
// revalidation while registry discovery uses LoadPluginSkillSource.
func LoadPluginSkillSelection(selection PluginSkillSelection) map[string]*SkillDefinition {
	rootDir := filepath.Clean(selection.PackageRoot)
	skillPath := filepath.Join(rootDir, "SKILL.md")
	resolvedRoot, resolveErr := filepath.EvalSymlinks(rootDir)
	resolvedSourceRoot, sourceErr := filepath.EvalSymlinks(filepath.Dir(rootDir))
	fallbackName := SkillIDFromNameOrFallback(filepath.Base(rootDir), "")
	fallbackID := pluginidentity.SkillCatalogID(selection.PluginName, fallbackName)
	base := &SkillDefinition{
		ID: fallbackID, Name: fallbackName, Source: SourcePlugin, Scope: SourcePlugin,
		RootDir: rootDir, SourceRoot: resolvedSourceRoot, ResolvedRootDir: resolvedRoot,
		SkillFilePath: skillPath, FilePath: skillPath, State: StateInvalid,
		Context: ContextInline, UserInvocable: true,
	}
	if resolveErr != nil || sourceErr != nil || !pathWithinRoot(resolvedRoot, resolvedSourceRoot) {
		base.UnavailableReason = "resolve selected plugin Skill package root"
		base.Diagnostics = []SkillDiagnostic{{Code: "skill_package_root_invalid", Message: base.UnavailableReason, Severity: "error"}}
		return map[string]*SkillDefinition{fallbackID: base}
	}
	info, statErr := os.Stat(skillPath)
	if statErr != nil || !info.Mode().IsRegular() {
		base.UnavailableReason = "selected plugin Skill entrypoint is not a regular SKILL.md"
		base.Diagnostics = []SkillDiagnostic{{Code: "skill_file_unreadable", Message: base.UnavailableReason, Severity: "error"}}
		return map[string]*SkillDefinition{fallbackID: base}
	}
	raw, readErr := os.ReadFile(skillPath)
	if readErr != nil {
		base.UnavailableReason = fmt.Sprintf("read SKILL.md: %v", readErr)
		base.Diagnostics = []SkillDiagnostic{{Code: "skill_file_unreadable", Message: base.UnavailableReason, Severity: "error"}}
		return map[string]*SkillDefinition{fallbackID: base}
	}
	if err := ValidateAgentSkillEntrypoint(string(raw), filepath.Base(rootDir)); err != nil {
		base.UnavailableReason = err.Error()
		base.Diagnostics = []SkillDiagnostic{{Code: "skill_agent_spec_invalid", Message: err.Error(), Severity: "error"}}
		return map[string]*SkillDefinition{fallbackID: base}
	}
	def, parseErr := ParseFrontmatter(string(raw))
	if parseErr != nil {
		base.UnavailableReason = parseErr.Error()
		base.Diagnostics = []SkillDiagnostic{{Code: "skill_frontmatter_invalid", Message: parseErr.Error(), Severity: "error"}}
		return map[string]*SkillDefinition{fallbackID: base}
	}
	name := SkillIDFromNameOrFallback(filepath.Base(rootDir), def.Name)
	if strings.TrimSpace(def.Name) == "" {
		def.Name = name
	}
	def.ID = pluginidentity.SkillCatalogID(selection.PluginName, name)
	def.Source, def.Scope = SourcePlugin, SourcePlugin
	def.RootDir, def.SourceRoot, def.ResolvedRootDir = rootDir, resolvedSourceRoot, resolvedRoot
	def.SkillFilePath, def.FilePath = skillPath, skillPath
	def.State = StateLoaded
	if err := ValidateNameReserved(def.Name); err != nil {
		def.State, def.UnavailableReason = StateInvalid, err.Error()
		def.Diagnostics = append(def.Diagnostics, SkillDiagnostic{Code: "skill_reserved_name", Message: err.Error(), Severity: "error"})
	} else if err := ValidateSchema(def); err != nil {
		def.State, def.UnavailableReason = StateInvalid, err.Error()
		def.Diagnostics = append(def.Diagnostics, SkillDiagnostic{Code: "skill_schema_invalid", Message: err.Error(), Severity: "error"})
	}
	if def.Context == "" {
		def.Context = ContextInline
	}
	def.Body = ""
	return map[string]*SkillDefinition{def.ID: def}
}

func pathWithinRoot(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

func shadowedMapKey(id string, source SourceOrigin, ordinal int) string {
	return fmt.Sprintf("%s__shadowed__%s__%d", id, source, ordinal)
}

func projectPathFromSkillsRoot(dir string) string {
	clean := filepath.Clean(dir)
	if filepath.Base(clean) != "skills" {
		return ""
	}
	parentDir := filepath.Dir(clean)
	parentName := strings.ToLower(filepath.Base(parentDir))
	if parentName != ".awaken" && parentName != ".agents" {
		return ""
	}
	return filepath.Dir(parentDir)
}

func isSameRealPath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return filepath.Clean(ra) == filepath.Clean(rb)
}

// LoadLegacyCommandsDir discovers both nested <name>/SKILL.md packages and
// flat <name>.md legacy commands. Flat files may omit frontmatter; their file
// name and first meaningful body line provide the minimum catalog metadata.
func LoadLegacyCommandsDir(dir string) map[string]*SkillDefinition {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	packages := LoadDir(dir, SourceLegacy)
	for key, def := range packages {
		if def != nil && def.State == StateInvalid {
			if _, err := os.Stat(def.SkillFilePath); os.IsNotExist(err) {
				delete(packages, key)
			}
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return packages
	}
	resolvedRoot, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return packages
	}
	flat := make(map[string]*SkillDefinition)
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || !strings.EqualFold(filepath.Ext(name), ".md") {
			continue
		}
		path := filepath.Join(dir, name)
		resolvedFile, resolveErr := filepath.EvalSymlinks(path)
		fallbackID := SkillIDFromNameOrFallback(strings.TrimSuffix(name, filepath.Ext(name)), "")
		base := &SkillDefinition{
			ID: fallbackID, Name: fallbackID, Source: SourceLegacy, Scope: SourceLegacy,
			RootDir: dir, SourceRoot: resolvedRoot, ResolvedRootDir: resolvedRoot, SkillFilePath: path, FilePath: path,
			State: StateInvalid, Context: ContextInline, UserInvocable: true,
		}
		if resolveErr != nil || !pathWithinRoot(resolvedFile, resolvedRoot) {
			base.UnavailableReason = "legacy command path escapes its configured commands root"
			base.Diagnostics = []SkillDiagnostic{{Code: "skill_package_root_escape", Message: base.UnavailableReason, Severity: "error"}}
			flat[fallbackID] = base
			continue
		}
		info, statErr := os.Stat(resolvedFile)
		if statErr != nil || !info.Mode().IsRegular() {
			continue
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			base.UnavailableReason = fmt.Sprintf("read legacy command: %v", readErr)
			base.Diagnostics = []SkillDiagnostic{{Code: "skill_file_unreadable", Message: base.UnavailableReason, Severity: "error"}}
			flat[fallbackID] = base
			continue
		}
		var def *SkillDefinition
		var parseErr error
		trimmed := strings.TrimSpace(string(raw))
		if strings.HasPrefix(trimmed, "---") {
			def, parseErr = ParseFrontmatter(trimmed)
		} else {
			def = &SkillDefinition{
				Name: fallbackID, Description: descriptionFromBody(trimmed), Body: trimmed,
				Context: ContextInline, UserInvocable: true,
			}
		}
		if parseErr != nil {
			base.UnavailableReason = parseErr.Error()
			base.Diagnostics = []SkillDiagnostic{{Code: "skill_frontmatter_invalid", Message: parseErr.Error(), Severity: "error"}}
			flat[fallbackID] = base
			continue
		}
		id := SkillIDFromNameOrFallback(fallbackID, def.Name)
		if strings.TrimSpace(def.Name) == "" {
			def.Name = id
		}
		def.ID, def.Source, def.Scope = id, SourceLegacy, SourceLegacy
		def.RootDir, def.SourceRoot, def.ResolvedRootDir = dir, resolvedRoot, resolvedRoot
		def.SkillFilePath, def.FilePath = path, path
		def.State = StateLoaded
		if err := ValidateNameReserved(def.Name); err != nil {
			def.State, def.UnavailableReason = StateInvalid, err.Error()
			def.Diagnostics = append(def.Diagnostics, SkillDiagnostic{Code: "skill_reserved_name", Message: err.Error(), Severity: "error"})
		} else if err := ValidateSchema(def); err != nil {
			def.State, def.UnavailableReason = StateInvalid, err.Error()
			def.Diagnostics = append(def.Diagnostics, SkillDiagnostic{Code: "skill_schema_invalid", Message: err.Error(), Severity: "error"})
		}
		def.Body = ""
		flat[id] = def
	}
	return MergeSkills(packages, flat)
}

func namespacedSkillID(source SourceOrigin, skillsRoot, skillID string) string {
	if source != SourcePlugin {
		return skillID
	}
	return pluginidentity.SkillCatalogID(pluginidentity.PluginNameFromSkillsRoot(skillsRoot), skillID)
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

// MergeSkills merges sources using deterministic first-wins priority while
// retaining lower-priority entries as shadowed diagnostics.
func MergeSkills(sources ...map[string]*SkillDefinition) map[string]*SkillDefinition {
	merged := make(map[string]*SkillDefinition)
	identities := make(map[string]*SkillDefinition)
	shadowOrdinal := 0
	for _, src := range sources {
		if src == nil {
			continue
		}
		keys := make([]string, 0, len(src))
		for key := range src {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, mapKey := range keys {
			def := src[mapKey]
			if def == nil {
				continue
			}
			def = shallowCopy(def)
			id := strings.TrimSpace(def.ID)
			if id == "" {
				id = strings.TrimSpace(mapKey)
			}
			if isShadowedMapKey(mapKey) || def.State == StateShadowed {
				def.State = StateShadowed
				merged[shadowedMapKey(id, def.Source, shadowOrdinal)] = def
				shadowOrdinal++
				continue
			}
			if identity := skillFileIdentity(def); identity != "" {
				if owner := identities[identity]; owner != nil {
					if sourcePriority(def.Source) <= sourcePriority(owner.Source) {
						def.State = StateShadowed
						def.ShadowedBy = fmt.Sprintf("%s(%s)", owner.Source, owner.FilePath)
						def.Diagnostics = append(def.Diagnostics, SkillDiagnostic{Code: "skill_realpath_duplicate", Message: "same Skill file already discovered from " + string(owner.Source), Severity: "warning"})
						merged[shadowedMapKey(id, def.Source, shadowOrdinal)] = def
						shadowOrdinal++
						continue
					}
				}
				identities[identity] = def
			}
			existing, ok := merged[id]
			if !ok {
				merged[id] = def
				continue
			}
			if sourcePriority(def.Source) > sourcePriority(existing.Source) {
				// existing gets shadowed.
				existing.State = StateShadowed
				existing.ShadowedBy = fmt.Sprintf("%s(%s)", def.Source, def.FilePath)
				merged[id] = def
				merged[shadowedMapKey(id, existing.Source, shadowOrdinal)] = existing
				shadowOrdinal++
			} else {
				// New entry is lower priority; shadow it.
				def.State = StateShadowed
				def.ShadowedBy = fmt.Sprintf("%s(%s)", existing.Source, existing.FilePath)
				merged[shadowedMapKey(id, def.Source, shadowOrdinal)] = def
				shadowOrdinal++
			}
		}
	}
	return merged
}

func skillFileIdentity(def *SkillDefinition) string {
	if def == nil {
		return ""
	}
	path := strings.TrimSpace(def.SkillFilePath)
	if path == "" {
		path = strings.TrimSpace(def.FilePath)
	}
	if path != "" {
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			return filepath.Clean(resolved)
		}
	}
	return ""
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
	c.ToolHints = append([]ToolHint(nil), def.ToolHints...)
	c.Diagnostics = append([]SkillDiagnostic(nil), def.Diagnostics...)
	c.Paths = append([]string(nil), def.Paths...)
	if def.UnknownFields != nil {
		c.UnknownFields = make(map[string]any, len(def.UnknownFields))
		for key, value := range def.UnknownFields {
			c.UnknownFields[key] = value
		}
	}
	if def.Awaken != nil {
		c.Awaken = make(map[string]any, len(def.Awaken))
		for key, value := range def.Awaken {
			c.Awaken[key] = value
		}
	}
	if def.McpBinding != nil {
		mb := *def.McpBinding
		c.McpBinding = &mb
	}
	if def.LastInvokedAt != nil {
		invokedAt := *def.LastInvokedAt
		c.LastInvokedAt = &invokedAt
	}
	return &c
}
