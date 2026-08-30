package plugins

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"awaken/src/hookruntime"
	"awaken/src/pluginhooks"
	"awaken/src/plugins/ir"
)

// CompilePluginHooks projects enabled, integrity-valid Plugin Hook components
// into the existing hookruntime for one new run snapshot.
func (s *Store) CompilePluginHooks(projectRoot string) ([]hookruntime.CompiledHook, []hookruntime.SourceReceipt, []hookruntime.Diagnostic, error) {
	var entries []pluginhooks.Entry
	var receipts []hookruntime.SourceReceipt
	err := s.withLock(func() error {
		state, err := s.readStateUnlocked()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		names := make([]string, 0, len(state.Plugins))
		for name := range state.Plugins {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			record := state.Plugins[name]
			plugin := s.loadRecord(name, record)
			if plugin == nil || !plugin.Enabled || !plugin.IntegrityValid {
				continue
			}
			values, _ := s.pluginVariableValues(plugin, record)
			count := 0
			for _, componentState := range plugin.ComponentState {
				component := componentState.Component
				if component.Kind != ir.KindHook || component.Hook == nil || componentState.DesiredState != "enabled" || !componentState.Switchable || componentState.Configuration != "ready" {
					continue
				}
				entries = append(entries, pluginhooks.Entry{
					PluginName: name, PluginDigest: plugin.Digest, PluginRoot: plugin.Root,
					PluginData: plugin.DataDir, ProjectRoot: strings.TrimSpace(projectRoot),
					Dialect: plugin.Dialect, Values: values, Component: component,
				})
				count++
			}
			if count > 0 {
				receipts = append(receipts, hookruntime.SourceReceipt{
					Kind: hookruntime.SourcePlugin, Path: plugin.Root, Digest: plugin.Digest,
					Status: hookruntime.StatusEnabled, HookCount: count,
					PluginName: name, PluginDigest: plugin.Digest,
				})
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, nil, err
	}
	hooks, diagnostics := pluginhooks.Compile(entries)
	return hooks, receipts, diagnostics, nil
}

func (s *Store) ResolveHookMCPTool(name, digest, serverShort, toolName string) (string, error) {
	plugin, err := s.Get(name)
	if err != nil {
		return "", err
	}
	if plugin == nil || !plugin.Enabled || !plugin.IntegrityValid || plugin.Digest != strings.TrimSpace(digest) {
		return "", fmt.Errorf("%w: %s", ErrDigestChanged, name)
	}
	for _, state := range plugin.ComponentState {
		if state.Component.Kind != ir.KindMCPServer || state.Component.MCP == nil || state.Component.MCP.ShortName != serverShort {
			continue
		}
		if state.DesiredState != "enabled" || !state.Switchable || state.Configuration != "ready" {
			return "", fmt.Errorf("%w: MCP server %s", ErrComponentNotSwitchable, serverShort)
		}
		for _, loaded := range plugin.MCPServers {
			if loaded.ShortName != serverShort || !loaded.Enabled || loaded.Invalid != nil {
				continue
			}
			for _, disabled := range loaded.Config.DisabledTools {
				if disabled == toolName {
					return "", fmt.Errorf("%w: %s", ErrToolNotFound, toolName)
				}
			}
			return loaded.Identity, nil
		}
	}
	return "", fmt.Errorf("%w: MCP server %s", ErrNotFound, serverShort)
}
