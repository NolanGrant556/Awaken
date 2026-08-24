package plugins

import (
	"os"
	"path/filepath"
	"sort"
)

// LoadPackage builds the runtime view of an already-copied plugin tree.
// Manifest fatal errors isolate the package: no skills root and no MCP
// servers are discovered. mcp.json top-level failure only disables MCP.
func LoadPackage(pluginRoot, dataDir string, rec Record) *InstalledPlugin {
	plugin := &InstalledPlugin{
		Name:    recName(pluginRoot, rec),
		Root:    pluginRoot,
		DataDir: dataDir,
		Enabled:     rec.Enabled,
		Source:      rec.Source,
		Digest:      rec.Digest,
		InstalledAt: rec.InstalledAt,
	}
	parsed, err := loadManifestFile(pluginRoot)
	if err != nil {
		plugin.Diagnostics = append(plugin.Diagnostics, errorDiag("plugin_manifest_invalid", err.Error()))
		return plugin
	}
	plugin.Manifest = parsed.Manifest
	plugin.Name = parsed.Manifest.Name
	plugin.Diagnostics = append(plugin.Diagnostics, parsed.Diagnostics...)

	skillsDir := filepath.Join(pluginRoot, "skills")
	if info, err := os.Lstat(skillsDir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			plugin.Diagnostics = append(plugin.Diagnostics, errorDiag("plugin_skills_invalid", "skills/ must be a regular directory"))
		} else if stayErr := resolvedStayWithin(pluginRoot, skillsDir); stayErr != nil {
			plugin.Diagnostics = append(plugin.Diagnostics, errorDiag("plugin_skills_invalid", "skills/ escapes the plugin root"))
		} else {
			plugin.SkillRoot = skillsDir
		}
	}

	mcp := loadMCPFile(pluginRoot, dataDir, plugin.Name, rec.MCPServers, parsed.Manifest.Schema)
	if mcp.Invalid != nil {
		plugin.Diagnostics = append(plugin.Diagnostics, *mcp.Invalid)
	} else {
		sort.Slice(mcp.Servers, func(i, j int) bool {
			return mcp.Servers[i].ShortName < mcp.Servers[j].ShortName
		})
		plugin.MCPServers = mcp.Servers
		plugin.Diagnostics = append(plugin.Diagnostics, mcp.Diagnostics...)
	}
	return plugin
}

func recName(pluginRoot string, rec Record) string {
	if rec.Version != "" {
		return filepath.Base(pluginRoot)
	}
	return filepath.Base(pluginRoot)
}
