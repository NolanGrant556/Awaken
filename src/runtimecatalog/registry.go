package runtimecatalog

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

var (
	ErrInvalidRuntime   = errors.New("invalid runtime")
	ErrDuplicateRuntime = errors.New("runtime already registered")
	ErrRuntimeNotFound  = errors.New("runtime not found")
)

type Registry struct {
	mu      sync.RWMutex
	entries map[string]RuntimeEntry
}

func NewRegistry(entries ...RuntimeEntry) (*Registry, error) {
	registry := &Registry{
		entries: make(map[string]RuntimeEntry, len(entries)),
	}
	for _, entry := range entries {
		if err := registry.Register(entry); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *Registry) Register(entry RuntimeEntry) error {
	if r == nil {
		return fmt.Errorf("%w: nil registry", ErrInvalidRuntime)
	}
	entry = Normalize(entry)
	if err := Validate(entry); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entries == nil {
		r.entries = make(map[string]RuntimeEntry)
	}
	if _, ok := r.entries[entry.ID]; ok {
		return fmt.Errorf("%w: %s", ErrDuplicateRuntime, entry.ID)
	}
	r.entries[entry.ID] = cloneEntry(entry)
	return nil
}

func (r *Registry) UpdateHealth(id string, health HealthSnapshot) error {
	if r == nil {
		return fmt.Errorf("%w: nil registry", ErrInvalidRuntime)
	}
	id = normalizeRuntimeID(id)
	health = normalizeHealthSnapshot(health)
	if !isValidHealthStatus(health.Status) {
		return fmt.Errorf("%w: invalid health status %q", ErrInvalidRuntime, health.Status)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrRuntimeNotFound, id)
	}
	entry.Health = health
	r.entries[id] = entry
	return nil
}

func (r *Registry) List() []RuntimeEntry {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries := make([]RuntimeEntry, 0, len(r.entries))
	for _, entry := range r.entries {
		entries = append(entries, cloneEntry(entry))
	}
	sortEntries(entries)
	return entries
}

func (r *Registry) Get(id string) (RuntimeEntry, bool) {
	if r == nil {
		return RuntimeEntry{}, false
	}
	id = normalizeRuntimeID(id)
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.entries[id]
	if !ok {
		return RuntimeEntry{}, false
	}
	return cloneEntry(entry), true
}

func (r *Registry) FindByKind(kind RuntimeKind) []RuntimeEntry {
	kind = normalizeRuntimeKind(kind)
	entries := r.List()
	matches := make([]RuntimeEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Kind == kind {
			matches = append(matches, entry)
		}
	}
	return matches
}

func (r *Registry) Enabled() []RuntimeEntry {
	entries := r.List()
	enabled := make([]RuntimeEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Enabled {
			enabled = append(enabled, entry)
		}
	}
	return enabled
}

func (r *Registry) DiagnosticSummary() DiagnosticSummary {
	entries := r.List()
	summary := DiagnosticSummary{Total: len(entries)}
	for _, entry := range entries {
		issue := diagnosticIssue(entry)
		if !entry.Enabled || entry.Health.Status == HealthStatusDisabled {
			summary.Disabled++
		} else if entry.InstallStatus == InstallStatusMissing || entry.InstallStatus == InstallStatusNeedsInstall {
			summary.NeedsInstall++
		} else if entry.Health.Status == HealthStatusUnreachable {
			summary.Unreachable++
		} else if entry.InstallStatus == InstallStatusInstalled && entry.Health.Status == HealthStatusHealthy {
			summary.Available++
		}
		if issue != nil {
			summary.Issues = append(summary.Issues, *issue)
		}
	}
	sort.SliceStable(summary.Issues, func(i, j int) bool {
		left := summary.Issues[i]
		right := summary.Issues[j]
		if severityRank(left.Severity) != severityRank(right.Severity) {
			return severityRank(left.Severity) > severityRank(right.Severity)
		}
		if left.RuntimeName != right.RuntimeName {
			return left.RuntimeName < right.RuntimeName
		}
		if left.RuntimeID != right.RuntimeID {
			return left.RuntimeID < right.RuntimeID
		}
		return left.Code < right.Code
	})
	return summary
}

func diagnosticIssue(entry RuntimeEntry) *DiagnosticIssue {
	base := DiagnosticIssue{
		RuntimeID:   entry.ID,
		RuntimeName: entry.Name,
		Guidance:    entry.Health.Guidance,
	}
	switch {
	case !entry.Enabled:
		base.Severity = IssueSeverityInfo
		base.Code = "disabled"
		base.Message = fmt.Sprintf("%s is disabled", entry.Name)
		return &base
	case entry.Health.Status == HealthStatusDisabled:
		base.Severity = IssueSeverityInfo
		base.Code = "health_disabled"
		base.Message = fmt.Sprintf("%s health checks are disabled", entry.Name)
		return &base
	case entry.InstallStatus == InstallStatusMissing:
		base.Severity = IssueSeverityError
		base.Code = "runtime_missing"
		base.Message = fmt.Sprintf("%s is missing", entry.Name)
		return &base
	case entry.InstallStatus == InstallStatusNeedsInstall:
		base.Severity = IssueSeverityError
		base.Code = "needs_install"
		base.Message = fmt.Sprintf("%s needs installation", entry.Name)
		return &base
	case entry.InstallStatus == InstallStatusError:
		base.Severity = IssueSeverityError
		base.Code = "install_error"
		base.Message = fmt.Sprintf("%s install status is error", entry.Name)
		return &base
	case entry.Health.Status == HealthStatusUnreachable:
		base.Severity = IssueSeverityCritical
		base.Code = "unreachable"
		base.Message = fmt.Sprintf("%s is unreachable", entry.Name)
		if entry.Health.Message != "" {
			base.Message = entry.Health.Message
		}
		return &base
	case entry.Health.Status == HealthStatusDegraded:
		base.Severity = IssueSeverityWarning
		base.Code = "degraded"
		base.Message = fmt.Sprintf("%s is degraded", entry.Name)
		if entry.Health.Message != "" {
			base.Message = entry.Health.Message
		}
		return &base
	case entry.InstallStatus == InstallStatusUnknown:
		base.Severity = IssueSeverityWarning
		base.Code = "install_unknown"
		base.Message = fmt.Sprintf("%s install status is unknown", entry.Name)
		return &base
	case entry.Health.Status == HealthStatusUnknown:
		base.Severity = IssueSeverityWarning
		base.Code = "health_unknown"
		base.Message = fmt.Sprintf("%s health status is unknown", entry.Name)
		return &base
	case entry.InstallStatus == InstallStatusInstalling:
		base.Severity = IssueSeverityInfo
		base.Code = "installing"
		base.Message = fmt.Sprintf("%s is installing", entry.Name)
		return &base
	case entry.InstallStatus == InstallStatusUpgradable:
		base.Severity = IssueSeverityInfo
		base.Code = "upgradable"
		base.Message = fmt.Sprintf("%s has an upgrade available", entry.Name)
		return &base
	default:
		return nil
	}
}

func severityRank(severity IssueSeverity) int {
	switch severity {
	case IssueSeverityCritical:
		return 4
	case IssueSeverityError:
		return 3
	case IssueSeverityWarning:
		return 2
	case IssueSeverityInfo:
		return 1
	default:
		return 0
	}
}

func sortEntries(entries []RuntimeEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ID != entries[j].ID {
			return entries[i].ID < entries[j].ID
		}
		return entries[i].Name < entries[j].Name
	})
}

func cloneEntry(entry RuntimeEntry) RuntimeEntry {
	entry.Args = cloneSlice(entry.Args)
	entry.Env = cloneSlice(entry.Env)
	entry.Capabilities = cloneCapabilities(entry.Capabilities)
	entry.ConfigOptions = cloneConfigOptions(entry.ConfigOptions)
	entry.Models = cloneSlice(entry.Models)
	entry.Modes = cloneSlice(entry.Modes)
	entry.MCPTransports = cloneSlice(entry.MCPTransports)
	return entry
}

func cloneCapabilities(capabilities []Capability) []Capability {
	if len(capabilities) == 0 {
		return nil
	}
	cloned := make([]Capability, len(capabilities))
	copy(cloned, capabilities)
	for i := range cloned {
		cloned[i].MCPTransports = cloneSlice(cloned[i].MCPTransports)
	}
	return cloned
}

func cloneConfigOptions(options []ConfigOption) []ConfigOption {
	if len(options) == 0 {
		return nil
	}
	cloned := make([]ConfigOption, len(options))
	copy(cloned, options)
	for i := range cloned {
		cloned[i].Options = cloneSlice(cloned[i].Options)
	}
	return cloned
}

func cloneSlice[T any](values []T) []T {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]T, len(values))
	copy(cloned, values)
	return cloned
}
