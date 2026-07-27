package skills

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	EnvManagedSkillDirs    = "AWAKEN_MANAGED_SKILL_DIRS"
	EnvAdditionalSkillDirs = "AWAKEN_ADDITIONAL_SKILL_DIRS"
	EnvPluginSkillDirs     = "AWAKEN_PLUGIN_SKILL_DIRS"
	EnvBundledSkillDirs    = "AWAKEN_BUNDLED_SKILL_DIRS"
	EnvLegacyCommandDirs   = "AWAKEN_LEGACY_COMMAND_DIRS"
)

// LocalSourceDirs builds the complete production discovery contract consumed
// by the existing Core boot path. Map values are OS path lists, not single
// directories; SkillRegistry expands them deterministically in listed order.
func LocalSourceDirs(projectRoot string) map[SourceOrigin]string {
	return LocalSourceDirsForProjectRoots([]string{projectRoot})
}

// LocalSourceDirsForProjectRoots builds the discovery contract for one
// project whose attached folders are ordered primary-first. Every attached
// folder contributes its .awaken/skills directory; the first folder wins
// same-ID conflicts within the project. Project-level .agents/skills and
// legacy commands intentionally remain anchored to the primary folder so the
// separate compatibility sources do not silently change scope.
func LocalSourceDirsForProjectRoots(projectRoots []string) map[SourceOrigin]string {
	dirs := make(map[SourceOrigin]string)
	home, _ := os.UserHomeDir()
	home = cleanAbsoluteDir(home, home)
	projectRoots = cleanSourceDirs(projectRoots, home)
	projectRoot := ""
	if len(projectRoots) > 0 {
		projectRoot = projectRoots[0]
	}

	projectSkillDirs := make([]string, 0, len(projectRoots))
	for _, root := range projectRoots {
		projectSkillDirs = append(projectSkillDirs, filepath.Join(root, ".awaken", "skills"))
	}
	setSourceDirList(dirs, SourceProject, projectSkillDirs)
	if home != "" {
		dirs[SourceUser] = joinSourceDirList([]string{filepath.Join(home, ".awaken", "skills")})
	}

	agentDirs := make([]string, 0, 2)
	if projectRoot != "" {
		agentDirs = append(agentDirs, filepath.Join(projectRoot, ".agents", "skills"))
	}
	if home != "" {
		agentDirs = append(agentDirs, filepath.Join(home, ".agents", "skills"))
	}
	setSourceDirList(dirs, SourceAgents, agentDirs)

	managed := configuredSourceDirs(os.Getenv(EnvManagedSkillDirs), home)
	if len(managed) == 0 {
		if defaultDir := defaultManagedSkillsDir(home); defaultDir != "" {
			managed = []string{defaultDir}
		}
	}
	setSourceDirList(dirs, SourceManaged, managed)
	setSourceDirList(dirs, SourceAdditional, configuredSourceDirs(os.Getenv(EnvAdditionalSkillDirs), home))
	setSourceDirList(dirs, SourcePlugin, configuredSourceDirs(os.Getenv(EnvPluginSkillDirs), home))
	// Bundled Skills have no implicit repository/package fallback. A packaged
	// application must explicitly set AWAKEN_BUNDLED_SKILL_DIRS when it ships any.
	setSourceDirList(dirs, SourceBundled, configuredSourceDirs(os.Getenv(EnvBundledSkillDirs), home))

	legacy := make([]string, 0, 4)
	if projectRoot != "" {
		legacy = append(legacy, filepath.Join(projectRoot, ".awaken", "commands"))
	}
	if home != "" {
		legacy = append(legacy, filepath.Join(home, ".awaken", "commands"))
	}
	legacy = append(legacy, configuredSourceDirs(os.Getenv(EnvLegacyCommandDirs), home)...)
	setSourceDirList(dirs, SourceLegacy, legacy)
	return dirs
}

func defaultManagedSkillsDir(home string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(string(filepath.Separator), "Library", "Application Support", "Awaken", "skills")
	case "windows":
		if programData := strings.TrimSpace(os.Getenv("ProgramData")); programData != "" {
			return filepath.Join(programData, "Awaken", "skills")
		}
	case "linux":
		return filepath.Join(string(filepath.Separator), "etc", "awaken", "skills")
	}
	if home != "" {
		return filepath.Join(home, ".awaken", "managed", "skills")
	}
	return ""
}

func configuredSourceDirs(raw, home string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return cleanSourceDirs(filepath.SplitList(raw), home)
}

func cleanSourceDirs(values []string, home string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = cleanAbsoluteDir(value, home)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func cleanAbsoluteDir(value, home string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if value == "~" && home != "" {
		value = home
	} else if strings.HasPrefix(value, "~"+string(filepath.Separator)) && home != "" {
		value = filepath.Join(home, strings.TrimPrefix(value, "~"+string(filepath.Separator)))
	}
	if !filepath.IsAbs(value) {
		absolute, err := filepath.Abs(value)
		if err != nil {
			return ""
		}
		value = absolute
	}
	return filepath.Clean(value)
}

func setSourceDirList(target map[SourceOrigin]string, source SourceOrigin, values []string) {
	if joined := joinSourceDirList(cleanSourceDirs(values, "")); joined != "" {
		target[source] = joined
	}
}

func joinSourceDirList(values []string) string {
	return strings.Join(values, string(os.PathListSeparator))
}

func splitSourceDirList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return cleanSourceDirs(filepath.SplitList(value), "")
}
