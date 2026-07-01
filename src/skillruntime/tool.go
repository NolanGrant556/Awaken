package skillruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"awaken/src/tool"
)

// SkillToolPrefix is the naming prefix for skill-derived tools.
const SkillToolPrefix = "skill__"

// SkillTool wraps a single skill as a tool.Tool, registered under the
// skill__{id} namespaced name. The executor is injected so it can be
// swapped in tests.
type SkillTool struct {
	SkillID     string
	NameStr     string
	Desc        string
	WhenToUse   string
	Body        string
	ArgDefs     []SkillArgDef
	Executor    SkillExecutor
	AllowFork   bool
}

// SkillExecutor executes a skill body (after argument substitution) and
// returns the result text.
type SkillExecutor interface {
	Execute(ctx context.Context, skillID string, body string, allowedTools []string) (string, error)
}

// SkillExecutorFunc adapts a function to the SkillExecutor interface.
type SkillExecutorFunc func(ctx context.Context, skillID string, body string, allowedTools []string) (string, error)

func (f SkillExecutorFunc) Execute(ctx context.Context, skillID string, body string, allowedTools []string) (string, error) {
	return f(ctx, skillID, body, allowedTools)
}

// Name implements tool.Tool.
func (st *SkillTool) Name() string {
	return st.NameStr
}

// Description implements tool.Tool. Uses whenToUse when available,
// falling back to Description.
func (st *SkillTool) Description() string {
	if st.WhenToUse != "" {
		return st.WhenToUse
	}
	return st.Desc
}

// Parameters implements tool.Tool. Converts SkillArgDefs to ToolParameters
// for JSON Schema generation.
func (st *SkillTool) Parameters() []tool.ToolParameter {
	params := make([]tool.ToolParameter, 0, len(st.ArgDefs))
	for _, arg := range st.ArgDefs {
		params = append(params, tool.ToolParameter{
			Name:        arg.Name,
			Type:        arg.Type,
			Description: arg.Description,
			Required:    arg.Required,
		})
	}
	return params
}

// Execute implements tool.Tool. Parses JSON input, substitutes arguments
// into the skill body, and delegates to the executor.
func (st *SkillTool) Execute(ctx context.Context, input string) (string, error) {
	args, err := parseSkillArgs(input, st.ArgDefs)
	if err != nil {
		return "", fmt.Errorf("skill %q parse args: %w", st.SkillID, err)
	}

	body, err := SubstituteArgs(st.Body, st.SkillID, st.ArgDefs, args)
	if err != nil {
		return "", err
	}

	if st.Executor == nil {
		return fmt.Sprintf("[skill %q] body after substitution:\n%s", st.SkillID, body), nil
	}
	return st.Executor.Execute(ctx, st.SkillID, body, nil)
}

// IsReadOnly implements tool.ReadOnlyTool. Skills are considered read-only
// because they are prompt templates; any mutation is done by tools they
// call, not the skill itself.
func (st *SkillTool) IsReadOnly() bool { return true }

// IsSkillTool implements the SkillToolMarker interface so the server layer
// can identify skill-derived tools in the unified tool list.
func (st *SkillTool) IsSkillTool() bool { return true }

// SkillToolName builds the namespaced tool name for a skill ID.
// Hyphens in the skill ID are converted to underscores so the result
// satisfies the tool registry's name pattern.
func SkillToolName(skillID string) string {
	return SkillToolPrefix + strings.ReplaceAll(skillID, "-", "_")
}

func parseSkillArgs(input string, argDefs []SkillArgDef) (map[string]string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return map[string]string{}, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		// Fall back: if there is a single string parameter, treat input as
		// its value.
		if len(argDefs) == 1 && argDefs[0].Type == "string" {
			return map[string]string{argDefs[0].Name: trimmed}, nil
		}
		return nil, fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	if decoded == nil {
		return map[string]string{}, nil
	}
	args := make(map[string]string, len(decoded))
	for k, v := range decoded {
		args[k] = fmt.Sprintf("%v", v)
	}
	return args, nil
}