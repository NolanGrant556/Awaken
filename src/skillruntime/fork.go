package skillruntime

import (
	"context"
	"fmt"
)

// ForkExecutor executes a skill in a sub-run (fork mode). When the fork
// capability is not available, it degrades to inline execution and logs
// a warning.
type ForkExecutor struct {
	// StartChild is an optional function that starts a child run. When nil,
	// ForkExecutor degrades to inline execution.
	StartChild func(ctx context.Context, skillBody string) (string, error)
	Logger     Logger
}

// Execute attempts a fork; falls back to inline when StartChild is nil
// or returns an error.
func (e *ForkExecutor) Execute(ctx context.Context, skillID string, body string, allowedTools []string) (string, error) {
	if e.Logger == nil {
		e.Logger = noopLogger{}
	}

	if e.StartChild != nil {
		result, err := e.StartChild(ctx, body)
		if err != nil {
			e.Logger.Warnf("skill %q fork failed, degrading to inline: %v", skillID, err)
			return e.inlineResult(skillID, body), nil
		}
		return result, nil
	}

	// No fork capability; degrade to inline.
	e.Logger.Warnf("skill %q requested fork mode but no StartChild available, executing inline", skillID)
	return e.inlineResult(skillID, body), nil
}

func (e *ForkExecutor) inlineResult(skillID string, body string) string {
	return fmt.Sprintf("[skill %q executed inline (fork degraded)]\n\n%s", skillID, body)
}