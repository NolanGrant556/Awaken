package agentcore

import (
	stdcontext "context"

	"awaken/src/agentcore/subagent"
	"awaken/src/context"
	"awaken/src/prompt"
	"awaken/src/subagentruntime"
	"awaken/src/tool"
	"awaken/src/tool/profile"
)

type AgentConfig struct {
	SystemPrompt string
	// MaxThoughts is the max agent turns. <= 0 uses DefaultMaxAgentTurns (128);
	// it no longer means unlimited.
	MaxThoughts int
	// MaxToolExecutions caps real tool executions per run. <= 0 uses
	// DefaultMaxToolExecutions (512).
	MaxToolExecutions int
	ModelConfig       ModelConfig
	EntryPoint        string
	Workspace         WorkspaceConfig
	SkillSummary      string
	// MemoryContextProvider reads the current host-owned Memory indexes at
	// request build time. Failures degrade to no Memory block.
	MemoryContextProvider func() (string, error)
	// MemoryRecallStart launches non-blocking manifest selection for a user
	// turn. MemoryContextProvider exposes a completed result later.
	MemoryRecallStart        func(stdcontext.Context, string, []string)
	MemoryRecallSetAuthority func(ModelCallAuthority)
	MemoryRecallEnd          func()
	MemoryRecallDrain        func() []MemoryRecallReceipt
	MemoryRecallReset        func()
	AutoToolPolicy           AutoToolPolicy
	// ToolExposure selects the model-visible tool pool. Empty Mode uses
	// DefaultToolExposurePolicy applies generic explicit hide/show rules.
	ToolExposure tool.ToolExposurePolicy
	// ToolAvailabilitySnapshot pins the exact executable instances and
	// definitions available for one run. Request compilation and execution must
	// consume the same snapshot so registry refreshes cannot create TOCTOU drift.
	ToolAvailabilitySnapshot *tool.RunToolAvailabilitySnapshot
	// ExecutionEnvironmentSnapshot is the bounded, secret-free observation used
	// to evaluate provider readiness for ToolAvailabilitySnapshot.
	ExecutionEnvironmentSnapshot *tool.ExecutionEnvironmentSnapshot
	// SkillContextBlocks are typed system context blocks injected from
	// explicit or model-selected Skill activation. Package resources stay
	// on-demand and are not preloaded here.
	SkillContextBlocks []ContextBlock
	// ComposerSkillSelection is a run-scoped, host-generated binding for the
	// Skills explicitly selected in the current composer turn. It is distinct
	// from SkillContextBlocks: restored or model-invoked Skill bodies must never
	// be promoted into an explicit user selection.
	ComposerSkillSelection *ComposerSkillSelectionContext
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
	// SubagentTaskID binds native child records to the host-owned task identity.
	// It is empty for the root Agent and legacy Skill fork children.
	SubagentTaskID string
	// SubagentController is run-scoped and is only exposed to the root runner.
	// It is never copied into child configs.
	SubagentController subagentruntime.Controller
	// ToolSessionState is mutable thread-local file/cwd state. It is never
	// journaled and must not be shared with child agents.
	ToolSessionState *tool.ToolSessionState
	// ModelContentStore owns tool-result media blobs beneath the thread session.
	ModelContentStore tool.ModelContentStore
	// ToolResultOffloadStore durably admits large text results once before
	// either the journal or model context observes them.
	ToolResultOffloadStore ToolResultOffloadStore
	// ContextEpochStore pins the exact provider prefix and host-owned source
	// snapshots beneath the owning thread. nil keeps lightweight unit runners
	// storage-free.
	ContextEpochStore ContextEpochStore
}

type ToolResultOffloadReceipt struct {
	StorageKey string
	Digest     string
	ByteSize   int
	Preview    string
}

type ToolResultOffloadStore interface {
	OffloadToolResult(stdcontext.Context, string, string, string) (ToolResultOffloadReceipt, error)
}

type MemoryRecallReceipt struct {
	MemoryID  string `json:"memory_id"`
	Scope     string `json:"scope"`
	Type      string `json:"type"`
	Revision  int    `json:"revision"`
	Bytes     int    `json:"bytes"`
	Action    string `json:"action,omitempty"`
	Status    string `json:"status,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

// ComposerSkillSelectionContext records the current run and the ordered Skills
// that Runtime revalidated from composer tokens. The invocation mode is not a
// field here: RequestBuilder always emits the host-owned value user_composer.
type ComposerSkillSelectionContext struct {
	RunID      string
	Selections []ComposerSkillSelection
}

// ComposerSkillSelection contains registry-owned identity. Runtime populates
// it from a successful activation plan, never from Skill body text.
type ComposerSkillSelection struct {
	SkillID        string
	Source         string
	ActivationMode string
	RootDir        string
	SkillFilePath  string
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
	ScopeID string
	// ProjectID is the durable Host-owned project identity. It is exposed to
	// the model only as structured environment context; paths and permissions
	// are never derived from it.
	ProjectID string
	// ProjectName is the display name of the project this scope belongs to. It
	// is model-facing context only; no path or permission is derived from it.
	ProjectName             string
	Root                    string
	ReadableRoots           []string
	WritableRoots           []string
	OutputRoots             []string
	AttachedFiles           []tool.ScopedFile
	ProtectedRoots          []string
	ProtectedRootExceptions []string
	TemporaryWorkRoot       string
	// SessionWorkspace marks Root as the Awaken-managed session directory of a
	// folder-less project, which is why the readable roots must not then be
	// presented to the model as folders the user picked.
	SessionWorkspace bool
}

type WorkspaceConfig = TaskScope

type ModelConfig struct {
	APIKey                string
	BaseURL               string
	Model                 string
	CapabilityModelID     string
	EndpointSelector      string
	ModelSelectorKind     string
	Protocol              string
	ProviderID            string
	ProfileID             string
	CredentialRevision    uint64
	Temperature           float64
	MaxTokens             int
	AuthScheme            string
	ContextWindow         int
	AutoCompactTokenLimit int
	Capabilities          tool.ModelCapabilities
	ReasoningEffort       string
}

type LLMRequest struct {
	SystemPrompt       string
	PromptSections     []PromptSection
	Messages           []context.Message
	Tools              []tool.ToolSchema
	AttributedTools    []AttributedToolSchema
	Model              string
	CapabilityModelID  string
	EndpointSelector   string
	ModelSelectorKind  string
	Temperature        float64
	MaxTokens          int
	ContextWindowState ContextWindowState
	ContextUsage       ContextUsageSnapshot
	EnvelopeDebug      *ContextEnvelope
	EpochSources       []ContextEpochSourceCandidate
	ContextEpoch       *ContextEpochReceipt
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
