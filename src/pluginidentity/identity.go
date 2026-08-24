package pluginidentity

import (
	"path/filepath"
	"strings"
)

const Prefix = "plugin__"

// SkillCatalogID returns the stable model-facing identity for a Skill bundled
// by one installed plugin. Dots remain valid in Agent Plugin names but are
// normalized in the Skill namespace so the ID stays compatible with the
// existing Skill catalog grammar.
func SkillCatalogID(pluginName, skillID string) string {
	return Prefix + strings.ReplaceAll(pluginName, ".", "-") + "__" + skillID
}

// PluginNameFromSkillsRoot derives the package directory name from the fixed
// <plugin-root>/skills component path.
func PluginNameFromSkillsRoot(skillsRoot string) string {
	return filepath.Base(filepath.Dir(filepath.Clean(skillsRoot)))
}
