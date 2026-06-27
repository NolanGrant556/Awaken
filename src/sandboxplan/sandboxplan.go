// Package sandboxplan builds sandbox policy plans from already-loaded settings.
package sandboxplan

import (
	"path/filepath"
	"sort"
	"strings"
)

type Source string

const (
	SourcePolicy  Source = "policy"
	SourceUser    Source = "user"
	SourceProject Source = "project"
	SourceLocal   Source = "local"
	SourceSession Source = "session"
)

type RuleAction string

const (
	RuleAllow RuleAction = "allow"
	RuleDeny  RuleAction = "deny"
)

const (
	ToolWebFetch = "WebFetch"
	ToolFileRead = "FileRead"
	ToolFileEdit = "FileEdit"
)

type Settings struct {
	Source Source
	Reason string

	Enabled                  *bool
	FailIfUnavailable        *bool
	AllowUnsandboxedCommands *bool

	AllowManagedDomainsOnly   bool
	AllowManagedReadPathsOnly bool

	Network    NetworkSettings
	Filesystem FilesystemSettings
	Rules      []Rule
}

type NetworkSettings struct {
	AllowDomains []string
	DenyDomains  []string
}

type FilesystemSettings struct {
	AllowReadPaths  []string
	DenyReadPaths   []string
	AllowWritePaths []string
	DenyWritePaths  []string
}

type Rule struct {
	Tool    string
	Action  RuleAction
	Pattern string
	Domain  string
	Path    string
	Source  Source
	Reason  string
}

type Options struct {
	ProtectedDenyWritePaths               []string
	DisableDefaultProtectedDenyWritePaths bool
}

type Plan struct {
	Decision   Decision
	Network    NetworkPlan
	Filesystem FilesystemPlan
}

type Decision struct {
	Enabled                  bool
	FailIfUnavailable        bool
	AllowUnsandboxedCommands bool
}

type NetworkPlan struct {
	AllowDomains []Entry
	DenyDomains  []Entry
}

type FilesystemPlan struct {
	AllowReadPaths  []Entry
	DenyReadPaths   []Entry
	AllowWritePaths []Entry
	DenyWritePaths  []Entry
}

type Entry struct {
	Value    string
	Metadata []Metadata
}

type Metadata struct {
	Source Source
	Reason string
}

// Build merges settings into a deterministic plan without reading the real
// filesystem. Scalar decision fields use the caller-provided input order; later
// non-nil settings override earlier ones. Managed-only allow switches are
// sticky once any input enables them, while deny entries are always retained.
func Build(inputs []Settings, opts Options) Plan {
	builder := planBuilder{
		allowDomains:    map[string]*Entry{},
		denyDomains:     map[string]*Entry{},
		allowReadPaths:  map[string]*Entry{},
		denyReadPaths:   map[string]*Entry{},
		allowWritePaths: map[string]*Entry{},
		denyWritePaths:  map[string]*Entry{},
	}

	for _, settings := range inputs {
		builder.applyDecision(settings)
		builder.allowManagedDomainsOnly = builder.allowManagedDomainsOnly || settings.AllowManagedDomainsOnly
		builder.allowManagedReadPathsOnly = builder.allowManagedReadPathsOnly || settings.AllowManagedReadPathsOnly
	}

	for _, settings := range inputs {
		builder.applySettings(settings)
	}
	builder.applyProtectedDenyWritePaths(opts)

	return builder.plan()
}

// DefaultProtectedDenyWritePaths returns the built-in write-deny paths that
// protect agent settings and skills from sandbox escape edits.
func DefaultProtectedDenyWritePaths() []string {
	return append([]string(nil), defaultProtectedDenyWritePaths...)
}

type planBuilder struct {
	decision Decision

	allowManagedDomainsOnly   bool
	allowManagedReadPathsOnly bool

	allowDomains    map[string]*Entry
	denyDomains     map[string]*Entry
	allowReadPaths  map[string]*Entry
	denyReadPaths   map[string]*Entry
	allowWritePaths map[string]*Entry
	denyWritePaths  map[string]*Entry
}

var defaultProtectedDenyWritePaths = []string{
	".claude/settings.json",
	".claude/settings.local.json",
	".claude/skills",
	".codex/config.toml",
	".codex/skills",
	"~/.claude/settings.json",
	"~/.claude/settings.local.json",
	"~/.claude/skills",
	"~/.codex/config.toml",
	"~/.codex/skills",
}

func (b *planBuilder) applyDecision(settings Settings) {
	if settings.Enabled != nil {
		b.decision.Enabled = *settings.Enabled
	}
	if settings.FailIfUnavailable != nil {
		b.decision.FailIfUnavailable = *settings.FailIfUnavailable
	}
	if settings.AllowUnsandboxedCommands != nil {
		b.decision.AllowUnsandboxedCommands = *settings.AllowUnsandboxedCommands
	}
}

func (b *planBuilder) applySettings(settings Settings) {
	metadata := Metadata{Source: sourceOrUnknown(settings.Source), Reason: settings.Reason}
	for _, domain := range settings.Network.AllowDomains {
		if b.allowManagedDomainsOnly && settings.Source != SourcePolicy {
			continue
		}
		addEntry(b.allowDomains, normalizeDomain(domain), metadata)
	}
	for _, domain := range settings.Network.DenyDomains {
		addEntry(b.denyDomains, normalizeDomain(domain), metadata)
	}

	for _, path := range settings.Filesystem.AllowReadPaths {
		if b.allowManagedReadPathsOnly && settings.Source != SourcePolicy {
			continue
		}
		addEntry(b.allowReadPaths, normalizePath(path), metadata)
	}
	for _, path := range settings.Filesystem.DenyReadPaths {
		addEntry(b.denyReadPaths, normalizePath(path), metadata)
	}
	for _, path := range settings.Filesystem.AllowWritePaths {
		addEntry(b.allowWritePaths, normalizePath(path), metadata)
	}
	for _, path := range settings.Filesystem.DenyWritePaths {
		addEntry(b.denyWritePaths, normalizePath(path), metadata)
	}

	for _, rule := range settings.Rules {
		b.applyRule(settings, rule)
	}
}

func (b *planBuilder) applyRule(settings Settings, rule Rule) {
	source := rule.Source
	if source == "" {
		source = settings.Source
	}
	metadata := Metadata{
		Source: sourceOrUnknown(source),
		Reason: firstNonEmpty(rule.Reason, settings.Reason),
	}

	switch rule.Tool {
	case ToolWebFetch:
		domain := normalizeDomain(firstNonEmpty(rule.Domain, domainFromPattern(rule.Pattern)))
		if domain == "" {
			return
		}
		if rule.Action == RuleDeny {
			addEntry(b.denyDomains, domain, metadata)
			return
		}
		if rule.Action == RuleAllow && (!b.allowManagedDomainsOnly || source == SourcePolicy) {
			addEntry(b.allowDomains, domain, metadata)
		}
	case ToolFileRead:
		path := normalizePath(firstNonEmpty(rule.Path, pathFromPattern(rule.Pattern)))
		if path == "" {
			return
		}
		if rule.Action == RuleDeny {
			addEntry(b.denyReadPaths, path, metadata)
			return
		}
		if rule.Action == RuleAllow && (!b.allowManagedReadPathsOnly || source == SourcePolicy) {
			addEntry(b.allowReadPaths, path, metadata)
		}
	case ToolFileEdit:
		path := normalizePath(firstNonEmpty(rule.Path, pathFromPattern(rule.Pattern)))
		if path == "" {
			return
		}
		if rule.Action == RuleDeny {
			addEntry(b.denyWritePaths, path, metadata)
			return
		}
		if rule.Action == RuleAllow {
			addEntry(b.allowWritePaths, path, metadata)
		}
	}
}

func (b *planBuilder) applyProtectedDenyWritePaths(opts Options) {
	if !opts.DisableDefaultProtectedDenyWritePaths {
		for _, path := range defaultProtectedDenyWritePaths {
			addEntry(b.denyWritePaths, normalizePath(path), Metadata{
				Source: SourcePolicy,
				Reason: "default protected sandbox path",
			})
		}
	}
	for _, path := range opts.ProtectedDenyWritePaths {
		addEntry(b.denyWritePaths, normalizePath(path), Metadata{
			Source: SourcePolicy,
			Reason: "protected sandbox path",
		})
	}
}

func (b *planBuilder) plan() Plan {
	return Plan{
		Decision: b.decision,
		Network: NetworkPlan{
			AllowDomains: sortedEntries(b.allowDomains),
			DenyDomains:  sortedEntries(b.denyDomains),
		},
		Filesystem: FilesystemPlan{
			AllowReadPaths:  sortedEntries(b.allowReadPaths),
			DenyReadPaths:   sortedEntries(b.denyReadPaths),
			AllowWritePaths: sortedEntries(b.allowWritePaths),
			DenyWritePaths:  sortedEntries(b.denyWritePaths),
		},
	}
}

func addEntry(entries map[string]*Entry, value string, metadata Metadata) {
	if value == "" {
		return
	}
	entry, ok := entries[value]
	if !ok {
		entries[value] = &Entry{Value: value, Metadata: []Metadata{metadata}}
		return
	}
	entry.Metadata = append(entry.Metadata, metadata)
}

func sortedEntries(entries map[string]*Entry) []Entry {
	result := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, Entry{
			Value:    entry.Value,
			Metadata: append([]Metadata(nil), entry.Metadata...),
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Value < result[j].Value
	})
	return result
}

func normalizeDomain(domain string) string {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return ""
	}
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	if slash := strings.Index(domain, "/"); slash >= 0 {
		domain = domain[:slash]
	}
	if colon := strings.Index(domain, ":"); colon >= 0 {
		domain = domain[:colon]
	}
	return strings.ToLower(strings.TrimSpace(domain))
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

func domainFromPattern(pattern string) string {
	return tokenFromPattern(pattern, "domain:")
}

func pathFromPattern(pattern string) string {
	if value := tokenFromPattern(pattern, "path:"); value != "" {
		return value
	}
	return strings.TrimSpace(pattern)
}

func tokenFromPattern(pattern, prefix string) string {
	pattern = strings.TrimSpace(pattern)
	start := strings.Index(pattern, prefix)
	if start < 0 {
		return ""
	}
	value := pattern[start+len(prefix):]
	for i, char := range value {
		if char == ')' || char == ',' || char == ' ' || char == '\t' || char == '\n' {
			return strings.TrimSpace(value[:i])
		}
	}
	return strings.TrimSpace(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sourceOrUnknown(source Source) Source {
	if source == "" {
		return "unknown"
	}
	return source
}
