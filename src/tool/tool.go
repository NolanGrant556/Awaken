package tool

import "context"

type Tool interface {
	Name() string
	Description() string
	Parameters() []ToolParameter
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
	DiffReview    *DiffReviewMetadata
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
	DiffReview    *DiffReviewMetadata
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
