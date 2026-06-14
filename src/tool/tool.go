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

type ApprovalRequiredTool interface {
	RequiresApproval() bool
}

type ToolParameter struct {
	Name        string
	Description string
	Type        string
	Required    bool
}
