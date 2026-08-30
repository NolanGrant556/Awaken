package plugins

import (
	"errors"
	"fmt"
	"os"

	"awaken/src/instructions"
	"awaken/src/pluginrules"
	"awaken/src/plugins/ir"
)

func (s *Store) RuleInstructionSources(manual []pluginrules.Selection, filePaths []string) []instructions.ExtraSource {
	var entries []pluginrules.Entry
	_ = s.withLock(func() error {
		state, err := s.readStateUnlocked()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		for name, record := range state.Plugins {
			plugin := s.loadRecord(name, record)
			if plugin == nil || !plugin.Enabled || !plugin.IntegrityValid {
				continue
			}
			states := make(map[string]ComponentState, len(plugin.ComponentState))
			for _, componentState := range plugin.ComponentState {
				states[componentState.Component.LogicalID] = componentState
			}
			for _, component := range plugin.Components {
				if component.Kind != ir.KindRule || component.Rule == nil {
					continue
				}
				componentState := states[component.LogicalID]
				activation := ""
				if override, ok := record.Components[component.LogicalID]; ok {
					activation = override.ActivationOverride
				}
				entries = append(entries, pluginrules.Entry{
					PluginName: name, PluginDigest: plugin.Digest, PluginRoot: plugin.Root,
					Component: component, DesiredState: componentState.DesiredState,
					ActivationOverride: activation,
				})
			}
		}
		return nil
	})
	return pluginrules.Assemble(pluginrules.Request{Entries: entries, Manual: manual, FilePaths: filePaths})
}

func (s *Store) ResolveRuleSelection(name, digest, logicalID string) error {
	plugin, err := s.Get(name)
	if err != nil {
		return err
	}
	if plugin == nil || !plugin.Enabled || !plugin.IntegrityValid || plugin.Digest != digest {
		return fmt.Errorf("%w: %s", ErrDigestChanged, name)
	}
	for _, state := range plugin.ComponentState {
		if state.Component.LogicalID == logicalID && state.Component.Kind == ir.KindRule && state.Switchable && state.DesiredState == "enabled" {
			return nil
		}
	}
	return fmt.Errorf("%w: rule %s", ErrNotFound, logicalID)
}
