package skills

import (
	"context"
	"time"
)

// SourceOrigin labels where a skill definition came from.
type SourceOrigin string

const (
	SourceManaged    SourceOrigin = "managed"
	SourceUser       SourceOrigin = "user"
	SourceProject    SourceOrigin = "project"
	SourceAdditional SourceOrigin = "additional"
	SourcePlugin     SourceOrigin = "plugin"
	SourceAgents     SourceOrigin = "agents"
	SourceMcp        SourceOrigin = "mcp"
	SourceBundled    SourceOrigin = "bundled"
	SourceLegacy     SourceOrigin = "legacy"
)

// sourcePriority returns integer priority for merging: higher wins.
func sourcePriority(o SourceOrigin) int {
	switch o {
	case SourceManaged:
		return 8
	case SourceProject:
		return 7
	case SourceAdditional:
		return 6
	case SourceUser:
		return 5
	case SourcePlugin:
		return 4
	case SourceAgents:
		return 3
	case SourceMcp:
		return 2
	case SourceBundled:
		return 1
	case SourceLegacy:
		return 0
	default:
		return 0
	}
}

// ContextMode controls how the skill body is executed.
type ContextMode string

const (
	ContextInline ContextMode = "inline"
	ContextFork   ContextMode = "fork"
)

// SkillState describes package/catalog state. Runtime, authentication, and
// tool readiness are deliberately not represented here; those are evaluated
// only when the agent selects a real provider.
type SkillState string

const (
	StateLoaded           SkillState = "loaded"
	StateInvalid          SkillState = "invalid"
	StateShadowed         SkillState = "shadowed"
	StateDisabledByPolicy SkillState = "disabled_by_policy"
)

// ResourceLoadMode controls when a required resource is loaded.
type ResourceLoadMode string

const (
	ResourceModePreload  ResourceLoadMode = "preload"
	ResourceModeOnDemand ResourceLoadMode = "on_demand"
)

// ToolHint preserves author-supplied allowed-tools guidance without turning
// it into an execution allow-list. Canonical is an optional Awaken alias that
// helps the model understand common tool names used by Skill authors.
type ToolHint struct {
	Declared  string `json:"declared"`
	Canonical string `json:"canonical,omitempty"`
	Aliased   bool   `json:"aliased,omitempty"`
}

type SkillDiagnostic struct {
	Code     string `json:"code,omitempty"`
	Message  string `json:"message"`
	Severity string `json:"severity,omitempty"`
}

// ToolPolicy is a legacy input shape. Allow and Deny are retained as author
// hints only; they never change a run's tool availability snapshot.
type ToolPolicy struct {
	Allow []string `yaml:"allow" json:"allow,omitempty"`
	Deny  []string `yaml:"deny" json:"deny,omitempty"`
}

// RequiredResource preserves legacy package metadata. Resources are resolved
// on demand; this declaration never preloads or disables a Universal Skill.
type RequiredResource struct {
	Path     string           `yaml:"path" json:"path"`
	Mode     ResourceLoadMode `yaml:"mode" json:"mode,omitempty"`
	Required bool             `yaml:"required" json:"required"`
	MaxBytes int64            `yaml:"maxBytes" json:"maxBytes,omitempty"`
	SHA256   string           `yaml:"sha256,omitempty" json:"sha256,omitempty"`
}

// SkillDefinition is the parsed, canonical representation of a SKILL.md file.
type SkillDefinition struct {
	// ID is the unique identifier derived from frontmatter name or the
	// package directory name.
	ID string `yaml:"-"`

	// Name is the short, human-readable name.
	Name string `yaml:"name"`

	// DisplayName is an optional longer name for UI display.
	DisplayName string `yaml:"displayName"`

	// Description is a short summary (shown in tool list).
	Description string `yaml:"description"`

	// WhenToUse guides the LLM on when to select this skill.
	WhenToUse string `yaml:"when_to_use"`

	// Version is an optional package version recorded on activation receipts.
	Version string `yaml:"version"`

	// ToolPolicy is retained only as an input compatibility field. It is never
	// compiled into a run-level tool boundary.
	ToolPolicy ToolPolicy `yaml:"toolPolicy"`

	// AllowedTools is author guidance from allowed-tools/allowedTools. It is a
	// selection hint, not permission and not a tool-pool restriction.
	AllowedTools []string   `yaml:"allowed-tools"`
	ToolHints    []ToolHint `yaml:"-"`

	// UserInvocable defaults to true. DisableModelInvocation only prevents the
	// model-facing catalog tool from selecting the Skill; an explicit user
	// invocation remains valid.
	UserInvocable          bool `yaml:"user-invocable"`
	DisableModelInvocation bool `yaml:"disable-model-invocation"`
	UserInvocableSet       bool `yaml:"-"`

	// RequiredTools is legacy author guidance surfaced in diagnostics only.
	RequiredTools []string `yaml:"requiredTools"`

	// RequiredCapabilities is legacy author guidance surfaced in diagnostics.
	RequiredCapabilities []string `yaml:"requiredCapabilities"`

	// RequiredRuntimeComponents is resolved only when a concrete provider runs.
	RequiredRuntimeComponents []string `yaml:"requiredRuntimeComponents"`

	// RequiredResources lists package-relative resources for on-demand access.
	RequiredResources []RequiredResource `yaml:"requiredResources"`

	// Arguments declares the parameters this skill accepts.
	Arguments    []SkillArgument `yaml:"arguments"`
	ArgumentHint string          `yaml:"-"`

	// Model is an optional override for the LLM model during execution.
	Model string `yaml:"model"`

	// Effort is an optional hint (e.g. "low", "medium", "high").
	Effort string `yaml:"effort"`

	// Context controls execution mode: "inline" or "fork".
	Context ContextMode `yaml:"context"`

	// Agent is an optional role binding (e.g. Researcher, Planner).
	Agent string `yaml:"agent"`

	// Hooks are optional lifecycle callbacks.
	Hooks    SkillHooks     `yaml:"hooks"`
	HooksRaw any            `yaml:"-"`
	Paths    []string       `yaml:"paths"`
	Shell    string         `yaml:"shell"`
	Awaken   map[string]any `yaml:"awaken"`

	// UnknownFields preserves unrecognized frontmatter fields so installing and
	// reserializing a market Skill does not silently destroy its metadata.
	UnknownFields map[string]any `yaml:"-"`

	// RequiredConnectors is legacy execution guidance. Missing connectors are
	// reported as diagnostics and are resolved only when a concrete provider is
	// invoked; they never make a structurally valid package unavailable.
	RequiredConnectors []string `yaml:"requiredConnectors"`

	// RequiredMcpServers is the equivalent execution hint for MCP-backed
	// providers. It is present on bridged MCP Skills and may be declared by a
	// local package, but connection state does not gate Catalog loading.
	RequiredMcpServers []string `yaml:"requiredMcpServers"`

	// McpBinding is set when the Skill was bridged from an MCP server.
	// Contains server name, prompt/resource name, and kind. Absent for
	// purely local Skills.
	McpBinding *McpBinding `yaml:"mcpBinding,omitempty"`

	// BodyFetcher is set by the bridge for MCP-sourced skills. When
	// non-nil, the skill executor calls it at execution time to fetch
	// the actual prompt body from the MCP server.
	BodyFetcher McpBodyFetcher `yaml:"-"`

	// Body is the Markdown content after the frontmatter.
	Body string `yaml:"-"`

	// Source records where this skill was loaded from.
	Source SourceOrigin `yaml:"-"`

	// Scope mirrors Source for local API consumers. Local skills use
	// "user" or "project"; MCP skills use "mcp".
	Scope SourceOrigin `yaml:"-"`

	// RootDir is the absolute path to the skill package directory.
	RootDir string `yaml:"-"`
	// SourceRoot is the resolved discovery root that authorized RootDir. It is
	// used as the os.Root anchor for managed/additional/user/project/plugin/
	// bundled/legacy local package resources and is never model-authored.
	SourceRoot string `yaml:"-"`
	// ResolvedRootDir captures the real package root when the registry loads or
	// installs the Skill. Resource readers use it to detect later symlink
	// replacement of RootDir before opening bundled files.
	ResolvedRootDir string `yaml:"-"`

	// SkillFilePath is the absolute path to the package entrypoint.
	SkillFilePath string `yaml:"-"`

	// ProjectPath is set for project-scoped local skills.
	ProjectPath string `yaml:"-"`

	// FilePath is the absolute path to the source SKILL.md.
	FilePath string `yaml:"-"`

	// State is the package/catalog state.
	State SkillState `yaml:"-"`

	// UnavailableReason explains why this package is invalid, shadowed, or
	// disabled by policy.
	UnavailableReason string `yaml:"-"`

	// UnavailableCode is an optional machine-readable unavailability code
	// such as skill_platform_contract_conflict.
	UnavailableCode string `yaml:"-"`

	// ShadowedBy is set when another skill with the same ID from a higher-
	// priority source overrides this one.
	ShadowedBy string `yaml:"-"`

	// LastInvokedAt is registry-owned recent-use metadata. It is not parsed
	// from SKILL.md and survives local source reloads within the Core process.
	LastInvokedAt *time.Time        `yaml:"-"`
	Diagnostics   []SkillDiagnostic `yaml:"-"`

	// DeprecatedWarnings collects non-fatal frontmatter compatibility notes.
	DeprecatedWarnings []string `yaml:"-"`
}

// SkillArgument describes a single parameter the skill accepts. The agent
// passes values via the tool call input; the executor substitutes them into
// the skill body with ${arg_name} placeholders.
type SkillArgument struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
	Default     string `yaml:"default,omitempty"`
}

// SkillHooks holds optional lifecycle callbacks.
type SkillHooks struct {
	PostExecute string `yaml:"postExecute,omitempty"`
}

// McpBodyFetcher is implemented by the bridge to fetch prompt body at
// execution time. MCP prompts/list only returns names and descriptions;
// the actual prompt template must be fetched via prompts/get.
type McpBodyFetcher interface {
	FetchBody(ctx context.Context, binding McpBinding, args map[string]string) (string, error)
}

// McpBinding identifies the MCP prompt or resource that was bridged into
// this Skill. Present only when the Skill was auto-generated by the MCP
// bridge, not for local SKILL.md files.
type McpBinding struct {
	// ServerName is the MCP server that hosts this prompt or resource.
	ServerName string `json:"serverName" yaml:"serverName"`

	// ResourceURI is the exact URI used by resources/read.
	ResourceURI string `json:"resourceUri,omitempty" yaml:"resourceUri,omitempty"`

	// PromptName is the original prompt name on the server.
	PromptName string `json:"promptName" yaml:"promptName"`

	// Kind is "prompt" for prompts/list entries or "resource" for
	// resources/list entries with mimeType "text/x-prompt".
	Kind string `json:"kind" yaml:"kind"`
}
