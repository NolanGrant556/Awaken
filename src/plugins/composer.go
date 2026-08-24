package plugins

import (
	"fmt"
	"sort"
	"strings"

	"awaken/src/skills"
)

// ResolveComposerSelection revalidates a Plugin token against the current
// Store. Renderer metadata is never authority: the package must still exist,
// remain enabled, keep the selected digest, and expose at least one usable
// Skill or enabled MCP server.
func (s *Store) ResolveComposerSelection(name, digest string) (ComposerSelection, error) {
	name = strings.TrimSpace(name)
	digest = strings.TrimSpace(digest)
	if name == "" || digest == "" {
		return ComposerSelection{}, fmt.Errorf("plugin identity is incomplete")
	}
	plugin, err := s.Get(name)
	if err != nil {
		return ComposerSelection{}, err
	}
	if plugin == nil || strings.TrimSpace(plugin.Name) != name {
		return ComposerSelection{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if !plugin.Enabled {
		return ComposerSelection{}, fmt.Errorf("%w: %s", ErrDisabled, name)
	}
	if strings.TrimSpace(plugin.Digest) != digest {
		return ComposerSelection{}, fmt.Errorf("%w: %s", ErrDigestChanged, name)
	}

	selection := ComposerSelection{
		Name:        plugin.Name,
		Version:     plugin.Manifest.Version,
		Description: plugin.Manifest.Description,
		Digest:      plugin.Digest,
	}
	if plugin.SkillRoot != "" {
		for _, definition := range skills.LoadDir(plugin.SkillRoot, skills.SourcePlugin) {
			if definition != nil && definition.State == skills.StateLoaded {
				selection.SkillIDs = append(selection.SkillIDs, definition.ID)
			}
		}
	}
	for _, server := range plugin.MCPServers {
		if server.Enabled && server.Invalid == nil && strings.TrimSpace(server.Identity) != "" {
			selection.ServerIDs = append(selection.ServerIDs, server.Identity)
		}
	}
	sort.Strings(selection.SkillIDs)
	sort.Strings(selection.ServerIDs)
	if len(selection.SkillIDs) == 0 && len(selection.ServerIDs) == 0 {
		return ComposerSelection{}, fmt.Errorf("%w: %s", ErrNoUsableComponents, name)
	}
	return selection, nil
}
