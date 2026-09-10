package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// RunToolAvailabilityEntry pins one executable and its resolved definition to
// the registration identity captured for a run. Runtime is intentionally not
// serialized, but remains the exact interface instance execution must use.
type RunToolAvailabilityEntry struct {
	Runtime             Tool                    `json:"-"`
	Definition          ToolDefinition          `json:"definition"`
	Origin              ToolSource              `json:"origin"`
	Quality             DefinitionQuality       `json:"quality"`
	Warnings            []string                `json:"warnings,omitempty"`
	PreserveInputObject bool                    `json:"preserve_input_object"`
	RegistrationID      string                  `json:"registration_id"`
	Provider            ProviderManifest        `json:"provider"`
	Readiness           ProviderReadinessReport `json:"readiness"`
}

// RunToolAvailabilitySnapshot is an immutable, versioned view of the registry.
// A live registry refresh cannot replace the runtime or definition returned by
// Lookup. Callers explicitly build a new snapshot to observe provider changes.
type RunToolAvailabilitySnapshot struct {
	SnapshotID            string    `json:"snapshot_id"`
	CatalogVersion        uint64    `json:"catalog_version"`
	EnvironmentSnapshotID string    `json:"environment_snapshot_id,omitempty"`
	GeneratedAt           time.Time `json:"generated_at"`

	entries map[string]RunToolAvailabilityEntry
	order   []string
}

var (
	ErrCandidateInstallPlanNotFound = errors.New("candidate install plan not found in pinned run snapshot")
	ErrCandidateInstallPlanInvalid  = errors.New("candidate install plan is invalid for pinned run snapshot")
)

// RunSnapshotFilter lets a builtin omit itself from a run availability
// snapshot. WebFetch uses this when search channels are disabled.
type RunSnapshotFilter interface {
	IncludeInRunSnapshot(ExecutionEnvironmentSnapshot) bool
}

// InstallPlanProviderProvenance identifies every pinned provider/tool whose
// dependency observation produced an install plan. Identical dependencies may
// legitimately produce the same plan for several tools, so provenance is a
// stable list rather than an ambiguous first-match choice.
type InstallPlanProviderProvenance struct {
	ProviderID     ProviderID   `json:"provider_id"`
	ToolName       string       `json:"tool_name"`
	RegistrationID string       `json:"registration_id"`
	Capabilities   []Capability `json:"capabilities,omitempty"`
}

// CandidateInstallPlanResolution is a clone-safe result from the immutable
// run snapshot. The model supplies only PlanID; executable, argv, dependency,
// environment, and provider provenance all come from this host-owned value.
type CandidateInstallPlanResolution struct {
	Plan           CandidateInstallPlan            `json:"plan"`
	Requirement    DependencyRequirement           `json:"requirement"`
	Providers      []InstallPlanProviderProvenance `json:"providers"`
	ToolSnapshotID string                          `json:"tool_snapshot_id"`
}

// NewRunToolAvailabilitySnapshot captures the current registry and evaluates
// bounded provider readiness against environment. Passing a zero environment is
// supported; providers with declared dependencies then remain needs_runtime
// unless they implement ProviderReadinessProvider.
func NewRunToolAvailabilitySnapshot(environment ExecutionEnvironmentSnapshot) RunToolAvailabilitySnapshot {
	mu.RLock()
	version := registryVersion
	registered := make([]RegisteredTool, 0, len(toolRegistry))
	for _, entry := range toolRegistry {
		registered = append(registered, cloneRegisteredTool(entry))
	}
	mu.RUnlock()

	sort.Slice(registered, func(i, j int) bool {
		return registered[i].Definition.Name < registered[j].Definition.Name
	})

	snapshot := RunToolAvailabilitySnapshot{
		CatalogVersion:        version,
		EnvironmentSnapshotID: environment.SnapshotID,
		GeneratedAt:           time.Now().UTC(),
		entries:               make(map[string]RunToolAvailabilityEntry, len(registered)),
		order:                 make([]string, 0, len(registered)),
	}
	for _, registeredTool := range registered {
		if filter, ok := registeredTool.Runtime.(RunSnapshotFilter); ok && !filter.IncludeInRunSnapshot(environment) {
			continue
		}
		provider := cloneProviderManifest(registeredTool.Provider)
		readiness := resolveProviderReadiness(registeredTool.Runtime, provider, environment)
		provider.Readiness = readiness.Readiness
		entry := RunToolAvailabilityEntry{
			Runtime:             registeredTool.Runtime,
			Definition:          cloneToolDefinition(registeredTool.Definition),
			Origin:              registeredTool.Origin,
			Quality:             registeredTool.Quality,
			Warnings:            append([]string(nil), registeredTool.Warnings...),
			PreserveInputObject: registeredTool.PreserveInputObject,
			RegistrationID:      registeredTool.RegistrationID,
			Provider:            provider,
			Readiness:           cloneReadinessReport(readiness),
		}
		name := registeredTool.Definition.Name
		snapshot.entries[name] = entry
		snapshot.order = append(snapshot.order, name)
	}
	snapshot.SnapshotID = snapshotIdentity(snapshot)
	return snapshot
}

func resolveProviderReadiness(runtime Tool, manifest ProviderManifest, environment ExecutionEnvironmentSnapshot) (report ProviderReadinessReport) {
	defer func() {
		if recovered := recover(); recovered != nil {
			report = ProviderReadinessReport{
				Readiness: ProviderUnavailable,
				Reason:    fmt.Sprintf("provider readiness probe panicked: %v", recovered),
			}
		}
	}()
	if provider, ok := runtime.(ProviderReadinessProvider); ok {
		report = provider.ProviderReadiness(environment)
		report.Dependencies = enrichDependencyObservations(report.Dependencies, environment)
		if report.Readiness == "" {
			report.Readiness = ProviderUnavailable
			report.Reason = firstNonBlank(report.Reason, "provider readiness probe returned an empty state")
		}
		return report
	}
	if len(manifest.Dependencies) > 0 {
		observations := observeDependencyRequirements(manifest.Dependencies, environment)
		readiness := ProviderReady
		reason := "all generic executable dependencies were observed"
		for _, observation := range observations {
			if observation.Status != DependencyReady && !observation.Requirement.Optional {
				readiness = ProviderNeedsRuntime
				reason = "one or more provider dependencies require a concrete readiness or installation step"
				break
			}
		}
		return ProviderReadinessReport{Readiness: readiness, Dependencies: observations, Reason: reason, RetryableAfterRemediation: readiness != ProviderReady}
	}
	readiness := manifest.Readiness
	if readiness == "" {
		readiness = ProviderReady
	}
	return ProviderReadinessReport{Readiness: readiness}
}

func cloneReadinessReport(value ProviderReadinessReport) ProviderReadinessReport {
	value.Dependencies = cloneDependencyObservations(value.Dependencies)
	return value
}

func snapshotIdentity(snapshot RunToolAvailabilitySnapshot) string {
	type identityEntry struct {
		Name           string            `json:"name"`
		RegistrationID string            `json:"registration_id"`
		ProviderID     ProviderID        `json:"provider_id"`
		Readiness      ProviderReadiness `json:"readiness"`
	}
	identity := struct {
		CatalogVersion uint64          `json:"catalog_version"`
		EnvironmentID  string          `json:"environment_id"`
		Entries        []identityEntry `json:"entries"`
	}{CatalogVersion: snapshot.CatalogVersion, EnvironmentID: snapshot.EnvironmentSnapshotID}
	for _, name := range snapshot.order {
		entry := snapshot.entries[name]
		identity.Entries = append(identity.Entries, identityEntry{
			Name: name, RegistrationID: entry.RegistrationID,
			ProviderID: entry.Provider.ProviderID, Readiness: entry.Readiness.Readiness,
		})
	}
	data, _ := json.Marshal(identity)
	digest := sha256.Sum256(data)
	return "tools_" + hex.EncodeToString(digest[:12])
}

func (s RunToolAvailabilitySnapshot) Lookup(name string) (RunToolAvailabilityEntry, error) {
	entry, ok := s.entries[strings.TrimSpace(name)]
	if !ok {
		return RunToolAvailabilityEntry{}, ErrToolNotFound
	}
	return cloneAvailabilityEntry(entry), nil
}

func (s RunToolAvailabilitySnapshot) Entries() []RunToolAvailabilityEntry {
	result := make([]RunToolAvailabilityEntry, 0, len(s.order))
	for _, name := range s.order {
		if entry, exists := s.entries[name]; exists {
			result = append(result, cloneAvailabilityEntry(entry))
		}
	}
	return result
}

// Clone retains the exact runtime interfaces while deep-copying all mutable
// definition/provider/readiness slices and the private entry map.
func (s RunToolAvailabilitySnapshot) Clone() RunToolAvailabilitySnapshot {
	clone := s
	clone.order = append([]string(nil), s.order...)
	clone.entries = make(map[string]RunToolAvailabilityEntry, len(s.entries))
	for name, entry := range s.entries {
		clone.entries[name] = cloneAvailabilityEntry(entry)
	}
	return clone
}

// WithComputerState rebinds only the pinned Computer providers to an Agent's
// own device binding. It never observes newly registered tools or providers.
func (s RunToolAvailabilitySnapshot) WithComputerState(environment ExecutionEnvironmentSnapshot, enabled bool) (RunToolAvailabilitySnapshot, ExecutionEnvironmentSnapshot) {
	environment = environment.Clone()
	environment.ComputerEnabled = enabled
	environment.SnapshotID = environmentSnapshotIdentity(environment)
	clone := s.Clone()
	clone.EnvironmentSnapshotID = environment.SnapshotID
	for name, entry := range clone.entries {
		switch entry.Definition.Annotations.CapabilityClass {
		case CapabilityClassComputerObserve, CapabilityClassComputerAct, CapabilityClassComputerVerify:
			entry.Readiness = resolveProviderReadiness(entry.Runtime, entry.Provider, environment)
			entry.Provider.Readiness = entry.Readiness.Readiness
			clone.entries[name] = entry
		}
	}
	clone.SnapshotID = snapshotIdentity(clone)
	return clone, environment
}

// LookupCandidateInstallPlan resolves an exact plan ID only from this pinned
// snapshot. It revalidates the deterministic plan identity and environment
// binding before returning a deep clone. Registry refreshes or caller mutation
// therefore cannot change what a current run is authorized to execute.
func (s RunToolAvailabilitySnapshot) LookupCandidateInstallPlan(planID string) (CandidateInstallPlanResolution, error) {
	planID = strings.TrimSpace(planID)
	if planID == "" || strings.TrimSpace(s.SnapshotID) == "" || strings.TrimSpace(s.EnvironmentSnapshotID) == "" {
		return CandidateInstallPlanResolution{}, ErrCandidateInstallPlanNotFound
	}

	var resolved CandidateInstallPlanResolution
	found := false
	for _, toolName := range s.order {
		entry, exists := s.entries[toolName]
		if !exists {
			continue
		}
		for _, observation := range entry.Readiness.Dependencies {
			for _, candidate := range observation.CandidateInstallPlans {
				if candidate.PlanID != planID {
					continue
				}
				if candidate.PlanID != candidateInstallPlanIdentity(candidate) ||
					candidate.EnvironmentID != s.EnvironmentSnapshotID ||
					candidate.RequirementFingerprint != dependencyRequirementFingerprint(observation.Requirement) {
					return CandidateInstallPlanResolution{}, fmt.Errorf("%w: plan %s identity or environment binding changed", ErrCandidateInstallPlanInvalid, planID)
				}
				if !found {
					resolved = CandidateInstallPlanResolution{
						Plan:           cloneCandidateInstallPlans([]CandidateInstallPlan{candidate})[0],
						Requirement:    observation.Requirement,
						ToolSnapshotID: s.SnapshotID,
					}
					found = true
				} else if !candidateInstallPlansEqual(resolved.Plan, candidate) || resolved.Requirement != observation.Requirement {
					return CandidateInstallPlanResolution{}, fmt.Errorf("%w: plan %s has conflicting pinned provenance", ErrCandidateInstallPlanInvalid, planID)
				}
				resolved.Providers = append(resolved.Providers, InstallPlanProviderProvenance{
					ProviderID: entry.Provider.ProviderID, ToolName: toolName,
					RegistrationID: entry.RegistrationID,
					Capabilities:   append([]Capability(nil), entry.Provider.Capabilities...),
				})
			}
		}
	}
	if !found {
		return CandidateInstallPlanResolution{}, fmt.Errorf("%w: %s", ErrCandidateInstallPlanNotFound, planID)
	}
	sort.Slice(resolved.Providers, func(i, j int) bool {
		if resolved.Providers[i].ProviderID != resolved.Providers[j].ProviderID {
			return resolved.Providers[i].ProviderID < resolved.Providers[j].ProviderID
		}
		return resolved.Providers[i].ToolName < resolved.Providers[j].ToolName
	})
	resolved.Providers = dedupeInstallPlanProviders(resolved.Providers)
	return cloneCandidateInstallPlanResolution(resolved), nil
}

func candidateInstallPlansEqual(left, right CandidateInstallPlan) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}

func dedupeInstallPlanProviders(values []InstallPlanProviderProvenance) []InstallPlanProviderProvenance {
	result := make([]InstallPlanProviderProvenance, 0, len(values))
	last := ""
	for _, value := range values {
		key := string(value.ProviderID) + "\x00" + value.ToolName + "\x00" + value.RegistrationID
		if key == last {
			continue
		}
		value.Capabilities = append([]Capability(nil), value.Capabilities...)
		result = append(result, value)
		last = key
	}
	return result
}

func cloneCandidateInstallPlanResolution(value CandidateInstallPlanResolution) CandidateInstallPlanResolution {
	value.Plan = cloneCandidateInstallPlans([]CandidateInstallPlan{value.Plan})[0]
	value.Providers = dedupeInstallPlanProviders(append([]InstallPlanProviderProvenance(nil), value.Providers...))
	return value
}

func cloneAvailabilityEntry(value RunToolAvailabilityEntry) RunToolAvailabilityEntry {
	value.Definition = cloneToolDefinition(value.Definition)
	value.Warnings = append([]string(nil), value.Warnings...)
	value.Provider = cloneProviderManifest(value.Provider)
	value.Readiness = cloneReadinessReport(value.Readiness)
	return value
}

func (s RunToolAvailabilitySnapshot) MarshalJSON() ([]byte, error) {
	type wireSnapshot struct {
		SnapshotID            string                     `json:"snapshot_id"`
		CatalogVersion        uint64                     `json:"catalog_version"`
		EnvironmentSnapshotID string                     `json:"environment_snapshot_id,omitempty"`
		GeneratedAt           time.Time                  `json:"generated_at"`
		Entries               []RunToolAvailabilityEntry `json:"entries"`
	}
	return json.Marshal(wireSnapshot{
		SnapshotID: s.SnapshotID, CatalogVersion: s.CatalogVersion,
		EnvironmentSnapshotID: s.EnvironmentSnapshotID, GeneratedAt: s.GeneratedAt,
		Entries: s.Entries(),
	})
}

// CapabilityCandidate is diagnostic resolver output. Resolver ordering is a
// preference only; the model/runtime selects a provider explicitly.
type CapabilityCandidate struct {
	Capability      Capability              `json:"capability"`
	ProviderID      ProviderID              `json:"provider_id"`
	ToolName        string                  `json:"tool_name"`
	RegistrationID  string                  `json:"registration_id"`
	Source          ToolSource              `json:"source"`
	Readiness       ProviderReadiness       `json:"readiness"`
	Dependencies    []DependencyRequirement `json:"dependencies,omitempty"`
	DependencyState []DependencyObservation `json:"dependency_state,omitempty"`
	SideEffects     []ToolSideEffect        `json:"side_effects,omitempty"`
	Quality         DefinitionQuality       `json:"quality"`
	Preference      int                     `json:"preference"`
	ReadinessReason string                  `json:"readiness_reason,omitempty"`
}

type CapabilityDiagnostic struct {
	Capability      Capability            `json:"capability"`
	SnapshotID      string                `json:"snapshot_id"`
	EnvironmentID   string                `json:"environment_snapshot_id,omitempty"`
	Candidates      []CapabilityCandidate `json:"candidates"`
	ReadyCandidates int                   `json:"ready_candidates"`
}

func (s RunToolAvailabilitySnapshot) ResolveCapability(capability Capability) CapabilityDiagnostic {
	diagnostic := CapabilityDiagnostic{
		Capability: capability, SnapshotID: s.SnapshotID, EnvironmentID: s.EnvironmentSnapshotID,
	}
	for _, name := range s.order {
		entry := s.entries[name]
		if !HasCapability(entry.Provider.Capabilities, capability) {
			continue
		}
		candidate := CapabilityCandidate{
			Capability: capability, ProviderID: entry.Provider.ProviderID,
			ToolName: entry.Definition.Name, RegistrationID: entry.RegistrationID,
			Source: entry.Provider.Source, Readiness: entry.Readiness.Readiness,
			Dependencies:    cloneDependencyRequirements(entry.Provider.Dependencies),
			DependencyState: cloneDependencyObservations(entry.Readiness.Dependencies),
			SideEffects:     cloneSideEffects(entry.Provider.SideEffects), Quality: entry.Quality,
			Preference:      providerPreference(entry.Provider, entry.Definition.Name),
			ReadinessReason: entry.Readiness.Reason,
		}
		if candidate.Readiness == ProviderReady {
			diagnostic.ReadyCandidates++
		}
		diagnostic.Candidates = append(diagnostic.Candidates, candidate)
	}
	sort.SliceStable(diagnostic.Candidates, func(i, j int) bool {
		left, right := diagnostic.Candidates[i], diagnostic.Candidates[j]
		if left.Preference != right.Preference {
			return left.Preference < right.Preference
		}
		if left.ProviderID != right.ProviderID {
			return left.ProviderID < right.ProviderID
		}
		return left.ToolName < right.ToolName
	})
	return diagnostic
}

type runAvailabilityContextValue struct {
	Snapshot    RunToolAvailabilitySnapshot
	Environment ExecutionEnvironmentSnapshot
}

type runAvailabilityContextKey struct{}

// ContextWithRunAvailability pins the exact catalog and environment observed
// for a run. Callers must create a new snapshot explicitly to refresh; tools
// reading this context never fall back to the mutable process registry.
func ContextWithRunAvailability(
	ctx context.Context,
	snapshot RunToolAvailabilitySnapshot,
	environment ExecutionEnvironmentSnapshot,
) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, runAvailabilityContextKey{}, runAvailabilityContextValue{
		Snapshot: snapshot.Clone(), Environment: environment.Clone(),
	})
}

func RunAvailabilityFromContext(ctx context.Context) (RunToolAvailabilitySnapshot, ExecutionEnvironmentSnapshot, bool) {
	if ctx == nil {
		return RunToolAvailabilitySnapshot{}, ExecutionEnvironmentSnapshot{}, false
	}
	value, ok := ctx.Value(runAvailabilityContextKey{}).(runAvailabilityContextValue)
	if !ok || strings.TrimSpace(value.Snapshot.SnapshotID) == "" ||
		strings.TrimSpace(value.Snapshot.EnvironmentSnapshotID) == "" ||
		value.Snapshot.EnvironmentSnapshotID != value.Environment.SnapshotID {
		return RunToolAvailabilitySnapshot{}, ExecutionEnvironmentSnapshot{}, false
	}
	return value.Snapshot.Clone(), value.Environment.Clone(), true
}

func providerPreference(provider ProviderManifest, toolName string) int {
	if strings.EqualFold(strings.TrimSpace(toolName), "Bash") {
		return 50
	}
	switch provider.Source {
	case SourceBuiltin, SourceRuntime:
		return 10
	case SourceMCP, SourceConnector:
		return 20
	case SourcePlugin:
		return 30
	case SourceSkill:
		return 40
	default:
		return 45
	}
}
