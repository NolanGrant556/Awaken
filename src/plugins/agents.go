package plugins

import (
	"fmt"
	"sort"
	"strings"

	"awaken/src/pluginagents"
	"awaken/src/plugins/ir"
	"awaken/src/subagentruntime"
)

func (s *Store) PluginAgentRoles() map[string]subagentruntime.Role {
	plugins, err := s.List()
	if err != nil {
		return nil
	}
	roles := map[string]subagentruntime.Role{}
	for _, plugin := range plugins {
		if plugin == nil || !plugin.Enabled || !plugin.IntegrityValid {
			continue
		}
		for _, state := range plugin.ComponentState {
			if state.Component.Kind != ir.KindAgent || state.DesiredState != "enabled" || !state.Switchable || state.Configuration != "ready" {
				continue
			}
			definition, compileErr := pluginagents.Compile(plugin.Name, plugin.Digest, state.Component)
			if compileErr == nil {
				roles[definition.Role.Type] = definition.Role
			}
		}
	}
	return roles
}

func (s *Store) ResolvePluginAgentRole(agentType string) (subagentruntime.Role, bool) {
	return resolvePluginAgentRole(s.PluginAgentRoles(), agentType)
}

func resolvePluginAgentRole(roles map[string]subagentruntime.Role, agentType string) (subagentruntime.Role, bool) {
	normalized := strings.ToLower(strings.TrimSpace(agentType))
	if role, ok := roles[normalized]; ok {
		return role, true
	}
	if normalized == "" || strings.HasPrefix(normalized, "plugin__") {
		return subagentruntime.Role{}, false
	}

	// Vendor Skills commonly refer to a sibling Agent by its manifest name
	// rather than Awaken's namespaced runtime identity. Accept that spelling
	// only when it resolves to exactly one enabled Plugin Agent.
	suffix := "__" + normalized
	var matched subagentruntime.Role
	found := false
	for roleType, role := range roles {
		if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(roleType)), suffix) {
			continue
		}
		if found {
			return subagentruntime.Role{}, false
		}
		matched = role
		found = true
	}
	return matched, found
}

func (s *Store) ResolveAgentSelection(name, digest, logicalID string) (subagentruntime.Role, error) {
	plugin, err := s.Get(name)
	if err != nil {
		return subagentruntime.Role{}, err
	}
	if plugin == nil || !plugin.Enabled || !plugin.IntegrityValid || plugin.Digest != strings.TrimSpace(digest) {
		return subagentruntime.Role{}, fmt.Errorf("%w: %s", ErrDigestChanged, name)
	}
	states := append([]ComponentState(nil), plugin.ComponentState...)
	sort.Slice(states, func(i, j int) bool { return states[i].Component.LogicalID < states[j].Component.LogicalID })
	for _, state := range states {
		if state.Component.LogicalID != logicalID || state.Component.Kind != ir.KindAgent {
			continue
		}
		if state.DesiredState != "enabled" || !state.Switchable || state.Configuration != "ready" {
			return subagentruntime.Role{}, fmt.Errorf("%w: %s", ErrComponentNotSwitchable, logicalID)
		}
		definition, err := pluginagents.Compile(plugin.Name, plugin.Digest, state.Component)
		if err != nil {
			return subagentruntime.Role{}, err
		}
		return definition.Role, nil
	}
	return subagentruntime.Role{}, fmt.Errorf("%w: Agent %s", ErrNotFound, logicalID)
}
