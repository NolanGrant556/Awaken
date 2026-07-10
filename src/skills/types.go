package skills

import "context"

// SourceOrigin labels where a skill definition came from.
type SourceOrigin string

const (
	SourceUser    SourceOrigin = "user"
	SourceProject SourceOrigin = "project"
	SourceMcp     SourceOrigin = "mcp"
)

// sourcePriority returns integer priority for merging: higher wins.
func sourcePriority(o SourceOrigin) int {
	switch o {
	case SourceProject:
		return 3
	case SourceUser:
		return 2
	case SourceMcp:
		return 2
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

// SkillState describes the runtime availability of a skill.
type SkillState string

const (
	StateAvailable   SkillState = "available"
	StateUnavailable SkillState = "unavailable"
	StateShadowed    SkillState = "shadowed"
)

// ToolPolicyMode controls how a skill intersects the parent tool pool.
type ToolPolicyMode string

const (
	ToolPolicyIntersect ToolPolicyMode = "intersect"
)

// ResourceLoadMode controls when a required resource is loaded.
type ResourceLoadMode string

const (
	ResourceModePreload  ResourceLoadMode = "preload"
	ResourceModeOnDemand ResourceLoadMode = "on_demand"
)

// Error codes used when a skill cannot enter the execution state.
const (
	ErrorCodePlatformContractConflict = "skill_platform_contract_conflict"
	ErrorCodeToolNotAllowed           = "skill_tool_not_allowed"
)

// ToolPolicy narrows the parent run tool pool for an active skill.
// Empty Allow inherits the parent pool; it never opens a global pool.
type ToolPolicy struct {
	Mode  ToolPolicyMode `yaml:"mode" json:"mode,omitempty"`
	Allow []string       `yaml:"allow" json:"allow,omitempty"`
	Deny  []string       `yaml:"deny" json:"deny,omitempty"`
}

// RequiredResource declares a package-relative resource that must be validated
// (and optionally preloaded) before the skill may execute.
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
	WhenToUse string `yaml:"whenToUse"`

	// Version is an optional package version recorded on activation receipts.
	Version string `yaml:"version"`

	// ToolPolicy is the v2 tool-pool contract. Prefer this over AllowedTools.
	ToolPolicy ToolPolicy `yaml:"toolPolicy"`

	// AllowedTools is the legacy allow-list alias for ToolPolicy.Allow.
	// Deprecated: loaders map it into ToolPolicy.Allow and emit a warning.
	AllowedTools []string `yaml:"allowedTools"`

	// RequiredTools must exist in the compiled effective tool pool.
	RequiredTools []string `yaml:"requiredTools"`

	// RequiredCapabilities must be satisfied by ≥1 tool in the effective pool.
	RequiredCapabilities []string `yaml:"requiredCapabilities"`

	// RequiredRuntimeComponents lists packaged runtime component names.
	RequiredRuntimeComponents []string `yaml:"requiredRuntimeComponents"`

	// RequiredResources lists package-relative resources to validate/preload.
	RequiredResources []RequiredResource `yaml:"requiredResources"`

	// Arguments declares the parameters this skill accepts.
	Arguments []SkillArgument `yaml:"arguments"`

	// Model is an optional override for the LLM model during execution.
	Model string `yaml:"model"`

	// Effort is an optional hint (e.g. "low", "medium", "high").
	Effort string `yaml:"effort"`

	// Context controls execution mode: "inline" or "fork".
	Context ContextMode `yaml:"context"`

	// Agent is an optional role binding (e.g. Researcher, Planner).
	Agent string `yaml:"agent"`

	// Hooks are optional lifecycle callbacks.
	Hooks SkillHooks `yaml:"hooks"`

	// RequiredConnectors lists connector names that must be available for
	// this skill to be usable. Any missing connector makes the skill
	// unavailable.
	RequiredConnectors []string `yaml:"requiredConnectors"`

	// RequiredMcpServers lists MCP server names that must be in the
	// connected state for this skill to be available. Present on both
	// bridged MCP skills (auto-derived) and local SKILL.md files
	// (manually declared). A skill is available only when both
	// requiredConnectors AND requiredMcpServers are satisfied.
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

	// SkillFilePath is the absolute path to the package entrypoint.
	SkillFilePath string `yaml:"-"`

	// ProjectPath is set for project-scoped local skills.
	ProjectPath string `yaml:"-"`

	// FilePath is the absolute path to the source SKILL.md.
	FilePath string `yaml:"-"`

	// State is the runtime availability state.
	State SkillState `yaml:"-"`

	// UnavailableReason explains why this skill is not available (when
	// State is StateUnavailable).
	UnavailableReason string `yaml:"-"`

	// UnavailableCode is an optional machine-readable unavailability code
	// such as skill_platform_contract_conflict.
	UnavailableCode string `yaml:"-"`

	// ShadowedBy is set when another skill with the same ID from a higher-
	// priority source overrides this one.
	ShadowedBy string `yaml:"-"`

	// DeprecatedWarnings collects non-fatal frontmatter compatibility notes.
	DeprecatedWarnings []string `yaml:"-"`
}

// EffectiveToolPolicy returns the canonical tool policy after applying the
// legacy allowedTools alias. Empty Allow means inherit the parent pool.
func (d *SkillDefinition) EffectiveToolPolicy() ToolPolicy {
	if d == nil {
		return ToolPolicy{Mode: ToolPolicyIntersect}
	}
	policy := d.ToolPolicy
	if policy.Mode == "" {
		policy.Mode = ToolPolicyIntersect
	}
	if len(policy.Allow) == 0 && len(d.AllowedTools) > 0 {
		policy.Allow = append([]string(nil), d.AllowedTools...)
	}
	return policy
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
	FetchPromptBody(ctx context.Context, serverName, promptName string) (string, error)
}

// McpBinding identifies the MCP prompt or resource that was bridged into
// this Skill. Present only when the Skill was auto-generated by the MCP
// bridge, not for local SKILL.md files.
type McpBinding struct {
	// ServerName is the MCP server that hosts this prompt or resource.
	ServerName string `json:"serverName" yaml:"serverName"`

	// PromptName is the original prompt or resource name on the server.
	PromptName string `json:"promptName" yaml:"promptName"`

	// Kind is "prompt" for prompts/list entries or "resource" for
	// resources/list entries with mimeType "text/x-prompt".
	Kind string `json:"kind" yaml:"kind"`
}
