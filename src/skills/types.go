package skills

// SourceOrigin labels where a skill definition came from.
type SourceOrigin string

const (
	SourceBundled  SourceOrigin = "bundled"
	SourceUser     SourceOrigin = "user"
	SourceProject  SourceOrigin = "project"
)

// sourcePriority returns integer priority for merging: higher wins.
func sourcePriority(o SourceOrigin) int {
	switch o {
	case SourceProject:
		return 3
	case SourceUser:
		return 2
	case SourceBundled:
		return 1
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

// SkillDefinition is the parsed, canonical representation of a SKILL.md file.
type SkillDefinition struct {
	// ID is the unique identifier derived from the filename (without .md).
	ID string `yaml:"-"`

	// Name is the short, human-readable name.
	Name string `yaml:"name"`

	// DisplayName is an optional longer name for UI display.
	DisplayName string `yaml:"displayName"`

	// Description is a short summary (shown in tool list).
	Description string `yaml:"description"`

	// WhenToUse guides the LLM on when to select this skill.
	WhenToUse string `yaml:"whenToUse"`

	// AllowedTools is an optional list of tool names allowed during skill
	// execution. When empty, the skill inherits the main run's tool pool.
	AllowedTools []string `yaml:"allowedTools"`

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

	// Body is the Markdown content after the frontmatter.
	Body string `yaml:"-"`

	// Source records where this skill was loaded from.
	Source SourceOrigin `yaml:"-"`

	// FilePath is the absolute path to the source SKILL.md.
	FilePath string `yaml:"-"`

	// State is the runtime availability state.
	State SkillState `yaml:"-"`

	// UnavailableReason explains why this skill is not available (when
	// State is StateUnavailable).
	UnavailableReason string `yaml:"-"`

	// ShadowedBy is set when another skill with the same ID from a higher-
	// priority source overrides this one.
	ShadowedBy string `yaml:"-"`
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