package tool

import (
	"context"
	"strings"
)

// ExposureMode controls whether a tool is offered to the model tool pool.
type ExposureMode string

const (
	// ExposureDefault uses ordinary provider rules.
	ExposureDefault ExposureMode = "default"
	// ExposureCompat is retained as a wire-compatible generic mode.
	ExposureCompat ExposureMode = "compat"
	// ExposureGrammarCapable is retained as a wire-compatible generic mode.
	ExposureGrammarCapable ExposureMode = "grammar_capable"
	// ExposureAll exposes every registered tool without filtering.
	ExposureAll ExposureMode = "all"
)

// ToolExposurePolicy filters the model-visible tool pool through explicit
// generic hide/show lists. Legacy modes remain wire-compatible but no longer
// contain tool-specific rules.
type ToolExposurePolicy struct {
	Mode ExposureMode
	// ExtraHidden names are always removed from the model pool.
	ExtraHidden []string
	// ExtraVisible names are force-kept even if Mode would hide them.
	ExtraVisible []string
}

// DefaultToolExposurePolicy is the ordinary-provider policy.
func DefaultToolExposurePolicy() ToolExposurePolicy {
	return ToolExposurePolicy{Mode: ExposureDefault}
}

// CompatToolExposurePolicy returns the legacy-compatible generic mode.
func CompatToolExposurePolicy() ToolExposurePolicy {
	return ToolExposurePolicy{Mode: ExposureCompat}
}

// GrammarCapableToolExposurePolicy returns the grammar-capable generic mode.
func GrammarCapableToolExposurePolicy() ToolExposurePolicy {
	return ToolExposurePolicy{Mode: ExposureGrammarCapable}
}

// FilterToolsForExposure returns tools visible to the model under policy.
func FilterToolsForExposure(tools []Tool, policy ToolExposurePolicy) []Tool {
	mode := policy.Mode
	if mode == "" {
		mode = ExposureDefault
	}
	hidden := map[string]bool{}
	for _, name := range policy.ExtraHidden {
		if n := strings.TrimSpace(name); n != "" {
			hidden[n] = true
		}
	}
	visible := map[string]bool{}
	for _, name := range policy.ExtraVisible {
		if n := strings.TrimSpace(name); n != "" {
			visible[n] = true
		}
	}

	out := make([]Tool, 0, len(tools))
	for _, t := range tools {
		name := t.Name()
		if visible[name] {
			out = append(out, t)
			continue
		}
		if hidden[name] {
			continue
		}
		if shouldHideTool(name, mode) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func shouldHideTool(name string, mode ExposureMode) bool {
	_ = name
	_ = mode
	return false
}

// IsToolExposed reports whether a single tool name is visible under policy.
func IsToolExposed(name string, policy ToolExposurePolicy) bool {
	dummy := []Tool{namedTool(name)}
	filtered := FilterToolsForExposure(dummy, policy)
	return len(filtered) == 1
}

type namedOnlyTool struct{ name string }

func namedTool(name string) Tool { return namedOnlyTool{name: name} }

func (t namedOnlyTool) Name() string                { return t.name }
func (t namedOnlyTool) Description() string         { return t.name }
func (t namedOnlyTool) Parameters() []ToolParameter { return nil }
func (t namedOnlyTool) Definition() ToolDefinition {
	return ToolDefinition{Name: t.name, DisplayName: t.name, Source: SourceBuiltin, Description: t.name}
}
func (t namedOnlyTool) Execute(context.Context, string) (string, error) { return "", nil }
