package agentcore

import (
	"awaken/src/agentcore/subagent"
	"awaken/src/context"
	"awaken/src/prompt"
	"awaken/src/tool"
	"awaken/src/tool/profile"
)

type AgentConfig struct {
	SystemPrompt string
	// MaxThoughts <= 0 means the runner has no turn-count limit.
	MaxThoughts    int
	ModelConfig    ModelConfig
	EntryPoint     string
	Workspace      WorkspaceConfig
	SkillSummary   string
	AutoToolPolicy AutoToolPolicy
	// ToolExposure selects the model-visible tool pool. Empty Mode uses
	// DefaultToolExposurePolicy (hides raw apply_patch for ordinary providers).
	// Compat/grammar-capable modes and ExtraVisible keep apply_patch available
	// for internal transforms, compatibility tests, or true grammar providers.
	ToolExposure tool.ToolExposurePolicy
	// ResolvedToolPool, when non-nil, is the dual-gate effective tool name
	// set after SkillActivationPlan compilation. RequestBuilder only exposes
	// schemas in this pool; ToolRuntime rejects names outside it.
	// nil means no skill gate (full exposure-filtered registry).
	ResolvedToolPool []string
	// SkillContextBlocks are typed system context blocks injected from
	// explicit Skill pre-activation (main SKILL.md + preload resources).
	SkillContextBlocks []ContextBlock
	// ExecutionProfile is the unified policy bundle applied to every tool
	// call this agent makes. When nil, EffectiveProfile falls back to
	// profile.DefaultProfile() (WorkspaceWrite).
	ExecutionProfile *profile.ExecutionProfile
	// SubagentBoundary, when set, marks this agent as a sub-agent and
	// captures the boundary it inherited from its parent. The runtime
	// uses it as a defense-in-depth check: tool calls must not exceed
	// this boundary (workspace roots must match, profile must not
	// escalate). Set when spawning a sub-agent via
	// subagent.ResolveSubagentBoundary; nil for the root agent.
	SubagentBoundary *subagent.ParentBoundary
	// SubagentOrigin, when set, records the agent's place in the
	// sub-agent tree (agent ID, parent agent ID, guardian flag, depth).
	// It is propagated into every ExecutionIntent this agent issues so
	// the decision layer can attribute the call. nil for the root agent.
	SubagentOrigin *profile.SubagentOrigin
}

// EffectiveProfile returns the profile that should be used for tool
// execution. If ExecutionProfile is set, it is returned directly; otherwise
// profile.DefaultProfile() (WorkspaceWrite) is returned as the default for
// normal agent runs.
func (c AgentConfig) EffectiveProfile() profile.ExecutionProfile {
	if c.ExecutionProfile != nil {
		return *c.ExecutionProfile
	}
	return profile.DefaultProfile()
}

type TaskScope struct {
	ScopeID           string
	Root              string
	ReadableRoots     []string
	WritableRoots     []string
	OutputRoots       []string
	AttachedFiles     []tool.ScopedFile
	ProtectedRoots    []string
	TemporaryWorkRoot string
}

type WorkspaceConfig = TaskScope

type ModelConfig struct {
	APIKey                string
	BaseURL               string
	Model                 string
	Protocol              string
	Temperature           float64
	MaxTokens             int
	AuthScheme            string
	ContextWindow         int
	AutoCompactTokenLimit int
}

type LLMRequest struct {
	SystemPrompt       string
	Messages           []context.Message
	Tools              []tool.ToolSchema
	Model              string
	Temperature        float64
	MaxTokens          int
	ContextWindowState ContextWindowState
	EnvelopeDebug      *ContextEnvelope
}

type RuntimeInfoProvider interface {
	GetRuntimeInfo() prompt.RuntimeInfo
}

type ContextProvider interface {
	GetMessages() []context.Message
	AddUserMessage(content string)
	AddAssistantMessage(content string, toolCalls []context.ToolCall)
	AddToolResult(toolCallID, name, content string)
	Clear()
}
