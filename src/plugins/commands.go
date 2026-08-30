package plugins

import (
	"fmt"

	"awaken/src/plugincommands"
	"awaken/src/plugins/ir"
)

func (s *Store) ExpandCommand(name, digest, logicalID string, arguments []string) (plugincommands.Expansion, error) {
	plugin, err := s.Get(name)
	if err != nil {
		return plugincommands.Expansion{}, err
	}
	if plugin == nil || !plugin.Enabled || !plugin.IntegrityValid || plugin.Digest != digest {
		return plugincommands.Expansion{}, fmt.Errorf("%w: %s", ErrDigestChanged, name)
	}
	for _, state := range plugin.ComponentState {
		if state.Component.LogicalID != logicalID || state.Component.Kind != ir.KindCommand {
			continue
		}
		if state.DesiredState != "enabled" || !state.Switchable || state.Configuration != "ready" {
			return plugincommands.Expansion{}, fmt.Errorf("%w: %s", ErrComponentNotSwitchable, logicalID)
		}
		return plugincommands.Expand(state.Component, arguments)
	}
	return plugincommands.Expansion{}, fmt.Errorf("%w: command %s", ErrNotFound, logicalID)
}
