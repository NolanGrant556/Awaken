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
	Artifacts(input string, output string) []ArtifactDescriptor
}

type ToolParameter struct {
	Name        string
	Description string
	Type        string
	Required    bool
}
