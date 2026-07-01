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
)

// ToolAnnotations carries the behavioural hints every tool must declare.
type ToolAnnotations struct {
	ReadOnly     bool
	Destructive  bool
	OpenWorld    bool
	RequiresAuth bool
	ServerName   string
	ToolName     string
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
	PolicyAlwaysAsk   PermissionPolicy = "always_ask"
	PolicyPerSession  PermissionPolicy = "per_session"
	PolicyOneTime     PermissionPolicy = "one_time"
)

// ToolDefinition is the canonical metadata block for any registered tool.
type ToolDefinition struct {
	Name        string
	DisplayName string
	Source      ToolSource
	Description string
	Annotations ToolAnnotations
	// Policy is the default permission posture; empty means "use global default".
	Policy PermissionPolicy
}

type Tool interface {
	Name() string
	Description() string
	Parameters() []ToolParameter
	Definition() ToolDefinition
	Execute(ctx context.Context, input string) (string, error)
}

type ReadOnlyTool interface {
	IsReadOnly() bool
}

type ApprovalRequest struct {
	Title         string
	Description   string
	RiskTone      string
	AffectedPaths []string
	PatchSummary  string
}

type ApprovalDescribingTool interface {
	ApprovalRequest(input string) ApprovalRequest
}

type ArtifactAction string

const (
	ArtifactActionCreated ArtifactAction = "created"
	ArtifactActionUpdated ArtifactAction = "updated"
)

type ArtifactDescriptor struct {
	Action      ArtifactAction
	ID          string
	Title       string
	Description string
	Kind        string
	Path        string
	Content     string
	Status      string
}

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
