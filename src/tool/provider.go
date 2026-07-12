package tool

import (
	"fmt"
	"sort"
	"strings"
)

type ProviderID string

type ProviderReadiness string

const (
	ProviderReady        ProviderReadiness = "ready"
	ProviderNeedsAuth    ProviderReadiness = "needs_auth"
	ProviderNeedsRuntime ProviderReadiness = "needs_runtime"
	ProviderUnavailable  ProviderReadiness = "unavailable"
)

// ProviderManifest is the catalog contract for one concrete implementation.
// A tool name is the model-call entrypoint; ProviderID is the implementation
// identity and Capability is the implementation-independent semantic layer.
type ProviderManifest struct {
	ProviderID   ProviderID              `json:"provider_id"`
	ToolName     string                  `json:"tool_name"`
	DisplayName  string                  `json:"display_name,omitempty"`
	Source       ToolSource              `json:"source"`
	Capabilities []Capability            `json:"capabilities"`
	Readiness    ProviderReadiness       `json:"readiness"`
	Dependencies []DependencyRequirement `json:"dependencies,omitempty"`
	SideEffects  []ToolSideEffect        `json:"side_effects,omitempty"`
	Quality      DefinitionQuality       `json:"quality"`
	Warnings     []string                `json:"warnings,omitempty"`
}

// ProviderManifestProvider lets a runtime report invocation-independent
// provider facts that cannot be represented by legacy ToolDefinition fields.
// Registry resolution still normalizes and clones the returned value.
type ProviderManifestProvider interface {
	ProviderManifest() ProviderManifest
}

// ProviderReadinessProvider performs a bounded readiness check while a run
// snapshot is being built. It must not mutate external state.
type ProviderReadinessProvider interface {
	ProviderReadiness(environment ExecutionEnvironmentSnapshot) ProviderReadinessReport
}

type ProviderReadinessReport struct {
	Readiness                 ProviderReadiness       `json:"readiness"`
	Dependencies              []DependencyObservation `json:"dependencies,omitempty"`
	Reason                    string                  `json:"reason,omitempty"`
	RetryableAfterRemediation bool                    `json:"retryable_after_remediation,omitempty"`
}

func defaultProviderID(def ToolDefinition) ProviderID {
	source := strings.TrimSpace(string(def.Source))
	if source == "" {
		source = "unknown"
	}
	name := strings.TrimSpace(def.Name)
	if name == "" {
		name = "unnamed"
	}
	return ProviderID(source + ":" + name)
}

func resolveProviderManifest(runtime Tool, def ToolDefinition, quality DefinitionQuality, warnings []string) ProviderManifest {
	manifest := ProviderManifest{}
	if provider, ok := runtime.(ProviderManifestProvider); ok {
		manifest = provider.ProviderManifest()
	}
	if manifest.ProviderID == "" {
		manifest.ProviderID = def.ProviderID
	}
	if manifest.ProviderID == "" {
		manifest.ProviderID = defaultProviderID(def)
	}
	if strings.TrimSpace(manifest.ToolName) == "" {
		manifest.ToolName = def.Name
	}
	if strings.TrimSpace(manifest.DisplayName) == "" {
		manifest.DisplayName = def.DisplayName
	}
	if manifest.Source == "" {
		manifest.Source = def.Source
	}
	if len(manifest.Capabilities) == 0 {
		manifest.Capabilities = inferCapabilitiesFromDefinition(def)
	} else {
		manifest.Capabilities = NormalizeCapabilities(manifest.Capabilities)
	}
	if manifest.Readiness == "" {
		manifest.Readiness = ProviderReady
	}
	if len(manifest.Dependencies) == 0 {
		manifest.Dependencies = cloneDependencyRequirements(def.Dependencies)
	} else {
		manifest.Dependencies = cloneDependencyRequirements(manifest.Dependencies)
	}
	if len(manifest.SideEffects) == 0 {
		manifest.SideEffects = cloneSideEffects(def.SideEffects)
	} else {
		manifest.SideEffects = cloneSideEffects(manifest.SideEffects)
	}
	if manifest.Quality == "" {
		manifest.Quality = quality
	}
	manifest.Warnings = uniqueProviderStrings(append(manifest.Warnings, warnings...))
	return manifest
}

func cloneProviderManifest(value ProviderManifest) ProviderManifest {
	value.Capabilities = append([]Capability(nil), value.Capabilities...)
	value.Dependencies = cloneDependencyRequirements(value.Dependencies)
	value.SideEffects = cloneSideEffects(value.SideEffects)
	value.Warnings = append([]string(nil), value.Warnings...)
	return value
}

func validateProviderManifest(value ProviderManifest) error {
	if strings.TrimSpace(string(value.ProviderID)) == "" {
		return fmt.Errorf("provider_id is required")
	}
	if strings.TrimSpace(value.ToolName) == "" {
		return fmt.Errorf("tool_name is required")
	}
	switch value.Readiness {
	case ProviderReady, ProviderNeedsAuth, ProviderNeedsRuntime, ProviderUnavailable:
	default:
		return fmt.Errorf("invalid provider readiness %q", value.Readiness)
	}
	return nil
}

func uniqueProviderStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
