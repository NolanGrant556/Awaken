package tool

import (
	"context"
	"strings"
)

// ExposureMode controls whether a tool is offered to the model tool pool.
type ExposureMode string

const (
	// ExposureDefault uses ordinary provider rules (hide fragile raw tools).
	ExposureDefault ExposureMode = "default"
	// ExposureCompat exposes tools needed for internal transforms and
	// compatibility tests (including raw apply_patch).
	ExposureCompat ExposureMode = "compat"
	// ExposureGrammarCapable exposes grammar-constrained apply_patch when the
	// provider transport truly supports constrained decoding.
	ExposureGrammarCapable ExposureMode = "grammar_capable"
	// ExposureAll exposes every registered tool without filtering.
	ExposureAll ExposureMode = "all"
)

// ToolExposurePolicy filters the model-visible tool pool.
// Ordinary providers hide raw apply_patch by default; text_edit remains visible.
// Internal shell→patch transforms and grammar-capable providers may still use
// apply_patch via ExposureCompat / ExposureGrammarCapable.
type ToolExposurePolicy struct {
	Mode ExposureMode
	// ExtraHidden names are always removed from the model pool.
	ExtraHidden []string
	// ExtraVisible names are force-kept even if Mode would hide them.
	ExtraVisible []string
}

// DefaultToolExposurePolicy is the ordinary-provider policy: hide raw apply_patch.
func DefaultToolExposurePolicy() ToolExposurePolicy {
	return ToolExposurePolicy{Mode: ExposureDefault}
}

// CompatToolExposurePolicy keeps apply_patch visible for transforms/tests.
func CompatToolExposurePolicy() ToolExposurePolicy {
	return ToolExposurePolicy{Mode: ExposureCompat}
}

// GrammarCapableToolExposurePolicy keeps apply_patch visible when the provider
// transport can actually constrain the patch mini-language.
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
	switch mode {
	case ExposureAll, ExposureCompat, ExposureGrammarCapable:
		return false
	default:
		// Ordinary providers: hide fragile raw apply_patch mini-language.
		return name == "apply_patch"
	}
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
