package tool

import "context"

// ToolSource identifies the origin of a tool.
type ToolSource string

const (
	SourceBuiltin   ToolSource = "builtin"
	SourceMCP       ToolSource = "mcp"
	SourceConnector ToolSource = "connector"
	SourceSkill     ToolSource = "skill"
	SourcePlugin    ToolSource = "plugin"
	SourceRuntime   ToolSource = "runtime_provider"
)

// ToolAnnotations carries the behavioural hints every tool must declare.
type ToolAnnotations struct {
	ReadOnly        bool
	Destructive     bool
	OpenWorld       bool
	RequiresAuth    bool
	ServerName      string
	ToolName        string
	CapabilityClass CapabilityClass
	TrustedOrigin   TrustedOrigin
	FileBridgeScope FileBridgeScope
	NetworkScope    NetworkScope
}

// RiskHints summarises the risk profile of a tool invocation.
type RiskHints struct {
	Destructive     bool
	OpenWorld       bool
	RequiresAuth    bool
	AffectedSystems []string
}

// PermissionPolicy defines the default permission posture for a tool.
type PermissionPolicy string

const (
	PolicyAlwaysAllow PermissionPolicy = "always_allow"
	PolicyAlwaysDeny  PermissionPolicy = "always_deny"
)

// ToolDefinition is the canonical capability contract for a registered tool.
//
// SchemaVersion 2 separates model-selection guidance (Purpose, WhenToUse,
// WhenNotToUse), executable contracts (InputSchema, OutputSchema), and safety
// facts (SideEffects, Permissions). Legacy tools may continue returning the
// original subset during the additive migration; ResolveDefinition fills an
// honest compatibility view and records its DefinitionQuality in the registry.
type ToolDefinition struct {
	SchemaVersion int                  `json:"schema_version"`
	Name          string               `json:"name"`
	DisplayName   string               `json:"display_name,omitempty"`
	Source        ToolSource           `json:"source"`
	Description   string               `json:"description"`
	Purpose       string               `json:"purpose"`
	WhenToUse     []string             `json:"when_to_use"`
	WhenNotToUse  []string             `json:"when_not_to_use"`
	InputSchema   ToolParametersSchema `json:"input_schema"`
	// OutputSchema describes only the tool-specific details nested inside the
	// canonical ExecutionOutcome envelope. It must not redefine envelope fields
	// such as status, errors, changes, or next_actions.
	OutputSchema ToolParam          `json:"output_schema"`
	SideEffects  []ToolSideEffect   `json:"side_effects"`
	Permissions  ToolPermissionSpec `json:"permissions"`
	Examples     []ToolExample      `json:"examples"`
	FailureModes []ToolFailureMode  `json:"failure_modes"`
	Annotations  ToolAnnotations    `json:"annotations"`
	// ProviderID is the stable implementation identity within the unified tool
	// catalog. Empty values receive a deterministic source:name compatibility ID
	// at registration.
	ProviderID ProviderID `json:"provider_id,omitempty"`
	// Capabilities are provider-neutral semantics. They drive capability
	// discovery/routing only; they do not authorize execution.
	Capabilities []Capability `json:"capabilities,omitempty"`
	// Dependencies describe provider readiness probes. They are evaluated when
	// building a run availability snapshot, never as a Skill-loading whitelist.
	Dependencies []DependencyRequirement `json:"dependencies,omitempty"`
	// Policy is the default permission posture; empty means "use global default".
	Policy PermissionPolicy `json:"policy,omitempty"`
}

// ToolSideEffect describes a class of externally observable state change.
// Reversibility is a fact about the operation, not an authorization decision.
type ToolSideEffect struct {
	Kind          string   `json:"kind"`
	Resources     []string `json:"resources,omitempty"`
	Description   string   `json:"description"`
	Reversibility string   `json:"reversibility"`
}

// ToolPermissionSpec states the capability and scope a tool may require. It is
// model/audit metadata only; the policy engine remains the authorization source.
type ToolPermissionSpec struct {
	CapabilityClass CapabilityClass `json:"capability_class,omitempty"`
	WorkspaceScope  string          `json:"workspace_scope,omitempty"`
	FileBridgeScope FileBridgeScope `json:"file_bridge_scope,omitempty"`
	NetworkScope    NetworkScope    `json:"network_scope,omitempty"`
	RequiresAuth    bool            `json:"requires_auth"`
	RequiresReview  string          `json:"requires_review,omitempty"`
}

// ToolExample gives the model a valid invocation scenario without duplicating
// the input schema prose.
type ToolExample struct {
	Scenario string         `json:"scenario"`
	Input    map[string]any `json:"input"`
	Outcome  string         `json:"outcome,omitempty"`
}

// ToolFailureMode documents a stable failure code and an actionable recovery
// boundary. Runtime errors remain authoritative for a particular invocation.
type ToolFailureMode struct {
	Code      string   `json:"code"`
	Cause     string   `json:"cause"`
	Retryable bool     `json:"retryable"`
	Recovery  []string `json:"recovery,omitempty"`
}

type Tool interface {
	Name() string
	Description() string
	Parameters() []ToolParameter
	Definition() ToolDefinition
	Execute(ctx context.Context, input string) (string, error)
}

// SemanticJournalSanitizer lets a tool replace its durable call/result payload
// with a receipt. It does not alter the live model-facing execution result.
type SemanticJournalSanitizer interface {
	SemanticJournalArguments(input string) string
	SemanticJournalResult(output string) string
}

type ReadOnlyTool interface {
	IsReadOnly() bool
}

type ArtifactAction string

const (
	ArtifactActionCreated ArtifactAction = "created"
	ArtifactActionUpdated ArtifactAction = "updated"
	// ArtifactActionPublished classifies an existing, already-verified file as
	// a deliverable. It does not claim that the publishing tool created or
	// modified the file bytes.
	ArtifactActionPublished ArtifactAction = "published"
)

// ArtifactRole separates user-facing deliverables from technical render
// previews and implementation-supporting file changes. Kind remains available
// as the legacy file/type classification for backwards compatibility.
type ArtifactRole string

const (
	ArtifactRoleDeliverable ArtifactRole = "deliverable"
	ArtifactRolePreview     ArtifactRole = "preview"
	ArtifactRoleSupport     ArtifactRole = "support"

	ArtifactContractVersion       = 2
	ArtifactLegacyContractVersion = 1
)

type ArtifactDescriptor struct {
	ContractVersion int            `json:"contract_version"`
	Role            ArtifactRole   `json:"role"`
	Producer        string         `json:"producer"`
	Action          ArtifactAction `json:"action"`
	ID              string         `json:"id,omitempty"`
	Title           string         `json:"title,omitempty"`
	Description     string         `json:"description,omitempty"`
	Kind            string         `json:"kind,omitempty"`
	Path            string         `json:"path,omitempty"`
	Content         string         `json:"content,omitempty"`
	Status          string         `json:"status,omitempty"`
}

// ScopedFile describes a user-attached or scope-bridged file path that the
// runtime may expose to tools as part of the task scope.
type ScopedFile struct {
	Path        string `json:"path"`
	DisplayName string `json:"display_name,omitempty"`
}

type CapabilityClass string

const (
	CapabilityUnknown            CapabilityClass = "unknown"
	CapabilityArtifactRead       CapabilityClass = "artifact_read"
	CapabilityArtifactWrite      CapabilityClass = "artifact_write"
	CapabilityConnectorRead      CapabilityClass = "connector_read"
	CapabilityConnectorWrite     CapabilityClass = "connector_write"
	CapabilityBrowserRead        CapabilityClass = "browser_read"
	CapabilityBrowserInteract    CapabilityClass = "browser_interaction"
	CapabilityLocalTransform     CapabilityClass = "local_transform"
	CapabilityExternalPublish    CapabilityClass = "external_publish"
	CapabilitySystemOperation    CapabilityClass = "system_operation"
	CapabilityClassComputerObserve CapabilityClass = "computer_observe"
	CapabilityClassComputerAct     CapabilityClass = "computer_act"
	CapabilityClassComputerVerify  CapabilityClass = "computer_verify"
)

type TrustedOrigin string

const (
	TrustedOriginBuiltin           TrustedOrigin = "builtin"
	TrustedOriginOfficialHosted    TrustedOrigin = "official_hosted"
	TrustedOriginOfficialConnector TrustedOrigin = "official_connector"
	TrustedOriginThirdPartyMCP     TrustedOrigin = "third_party_mcp"
	TrustedOriginLocalSkill        TrustedOrigin = "local_skill"
)

type FileBridgeScope string

const (
	FileBridgeNone           FileBridgeScope = "none"
	FileBridgeTaskScopeRead  FileBridgeScope = "task_scope_read"
	FileBridgeTaskScopeWrite FileBridgeScope = "task_scope_write"
)

type NetworkScope string

const (
	NetworkScopeNone       NetworkScope = "none"
	NetworkScopeRestricted NetworkScope = "restricted"
	NetworkScopeOpen       NetworkScope = "open"
)

type ArtifactProducingTool interface {
	Artifacts(input string, output string, permissionContext PermissionContext) []ArtifactDescriptor
}

type CommandOutput struct {
	Stream    string
	Delta     string
	Truncated bool
}

type PatchPreview struct {
	Patch         string
	Summary       string
	AffectedPaths []string
	Truncated     bool
}

type ToolObserver interface {
	OnCommandOutput(output CommandOutput)
}

type ObservableTool interface {
	ExecuteWithObserver(ctx context.Context, input string, observer ToolObserver) (string, error)
}

type PatchPreviewingTool interface {
	PatchPreview(input string, permissionContext PermissionContext) (PatchPreview, bool)
}

type ToolParameter struct {
	Name        string
	Description string
	Type        string
	Required    bool
}
