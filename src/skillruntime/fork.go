package skillruntime

import (
	"context"
	"fmt"
)

// ForkExecutor is a legacy adapter for callers that have not yet moved to
// CatalogTool.ForkStarter. It never silently degrades to inline execution.
type ForkExecutor struct {
	// StartChild is an optional function that starts a child run. When nil,
	// ForkExecutor degrades to inline execution.
	StartChild func(ctx context.Context, skillBody string) (string, error)
	Logger     Logger
}

// Execute attempts a fork and fails explicitly when no child runner exists.
func (e *ForkExecutor) Execute(ctx context.Context, skillID string, body string, allowedTools []string) (string, error) {
	if e.Logger == nil {
		e.Logger = noopLogger{}
	}

	if e.StartChild != nil {
		result, err := e.StartChild(ctx, body)
		if err != nil {
			return "", fmt.Errorf("skill %q fork failed: %w", skillID, err)
		}
		return result, nil
	}

	return "", &SkillCompatibilityError{Code: "skill_fork_unavailable", Message: fmt.Sprintf("skill %q requires a child Agent starter", skillID)}
}
