// Package dialect detects and parses supported Plugin package dialects into IR.
// Parsing is deliberately side-effect free: it reads package files but never
// executes package code, downloads dependencies, or mutates the source tree.
package dialect

import (
	"fmt"
	"sort"

	"awaken/src/plugins/ir"
)

type ParseOptions struct {
	PluginDigest string
}

type Adapter interface {
	Dialect() ir.Dialect
	Parse(root string, candidate ir.ManifestCandidate, options ParseOptions) (ir.NormalizedPlugin, error)
}

type Registry struct {
	adapters map[ir.Dialect]Adapter
}

func NewRegistry(adapters ...Adapter) *Registry {
	registry := &Registry{adapters: make(map[ir.Dialect]Adapter, len(adapters))}
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		registry.adapters[adapter.Dialect()] = adapter
	}
	return registry
}

func NewDefaultRegistry() *Registry {
	return NewRegistry(
		agentPluginsAdapter{},
		codexAdapter{},
		claudeCodeAdapter{},
		cursorAdapter{},
	)
}

func (r *Registry) Parse(root string, candidate ir.ManifestCandidate, options ParseOptions) (ir.NormalizedPlugin, error) {
	if r == nil {
		return ir.NormalizedPlugin{}, fmt.Errorf("plugin dialect registry is unavailable")
	}
	adapter := r.adapters[candidate.Dialect]
	if adapter == nil {
		return ir.NormalizedPlugin{}, fmt.Errorf("unsupported plugin dialect %q", candidate.Dialect)
	}
	plugin, err := adapter.Parse(root, candidate, options)
	if err != nil {
		return ir.NormalizedPlugin{}, err
	}
	sort.SliceStable(plugin.Components, func(i, j int) bool {
		return plugin.Components[i].Kind < plugin.Components[j].Kind
	})
	for i := range plugin.Components {
		ir.FinalizeComponent(&plugin.Components[i], options.PluginDigest)
	}
	return plugin, nil
}
