package skillruntime

import (
	"context"
	"fmt"
	"log"
)

// SkillExecutor executes an already activated Skill body. allowedTools contains
// non-enforcing author hints; it never narrows the parent run's tool pool.
type SkillExecutor interface {
	Execute(ctx context.Context, skillID string, body string, allowedTools []string) (string, error)
}

// SkillExecutorFunc adapts a function to SkillExecutor.
type SkillExecutorFunc func(ctx context.Context, skillID string, body string, allowedTools []string) (string, error)

func (f SkillExecutorFunc) Execute(ctx context.Context, skillID string, body string, allowedTools []string) (string, error) {
	return f(ctx, skillID, body, allowedTools)
}

// InlineExecutor executes a skill body by appending it as additional
// context to the current run. In practice the agentcore Runner will see
// the result string and continue processing.
//
// For now the inline executor simply returns the rendered body — the
// agentcore sees this as a tool result and continues the conversation.
// This matches the design: the skill body is formatted as a user message
// equivalent that the LLM should follow.
type InlineExecutor struct {
	Logger Logger
}

// Logger is the minimal logging surface consumed by skillruntime.
type Logger interface {
	Warnf(format string, args ...any)
}

type noopLogger struct{}

func (noopLogger) Warnf(string, ...any) {}

// Execute renders the skill body and returns it to the agent. The agent
// will naturally continue processing the instructions in the body.
func (e *InlineExecutor) Execute(ctx context.Context, skillID string, body string, allowedTools []string) (string, error) {
	if e.Logger == nil {
		e.Logger = noopLogger{}
	}
	_ = e.Logger // reserved for future use

	return fmt.Sprintf(
		"[skill %q executed inline]\n\n%s",
		skillID, body,
	), nil
}

// StdLogger is the default Logger implementation using stdlib log.
type StdLogger struct{}

func (StdLogger) Warnf(format string, args ...any) {
	log.Printf("[skillruntime warn] "+format, args...)
}
