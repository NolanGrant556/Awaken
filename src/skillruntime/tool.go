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
	SkillID      string
	NameStr      string
	Desc         string
	WhenToUse    string
	Body         string
	RootDir      string
	AllowedTools []string
	ArgDefs      []SkillArgDef
	Executor     SkillExecutor
	AllowFork    bool
	Activator    *Activator
	ParentPool   func() []string
	OnActivated  func(plan *SkillActivationPlan)
	lastPlan     *SkillActivationPlan
	// FetchBody is called to obtain the prompt template body at execution
	// time. Set for MCP-bridged skills whose Body is empty at registration
	// time (prompts/list only returns metadata, not the template).
	FetchBody func(ctx context.Context) (string, error)
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

// Execute implements tool.Tool. Parses JSON input, activates the skill via
// SkillActivator when available, and delegates to the executor.
func (st *SkillTool) Execute(ctx context.Context, input string) (string, error) {
	args, err := parseSkillArgs(input, st.ArgDefs)
	if err != nil {
		return "", fmt.Errorf("skill %q parse args: %w", st.SkillID, err)
	}

	if st.Activator != nil {
		var parentPool []string
		if st.ParentPool != nil {
			parentPool = st.ParentPool()
		} else {
			parentPool = CurrentParentToolPool()
		}
		plan, activateErr := st.Activator.Activate(ActivationRequest{
			SkillID:        st.SkillID,
			ParentToolPool: parentPool,
			Args:           args,
		})
		if activateErr != nil {
			st.lastPlan = plan
			if plan != nil && st.OnActivated != nil {
				st.OnActivated(plan)
			}
			return "", activateErr
		}
		st.lastPlan = plan
		if st.OnActivated != nil {
			st.OnActivated(plan)
		}
		if st.Executor == nil {
			return plan.MainBody, nil
		}
		return st.Executor.Execute(ctx, st.SkillID, plan.MainBody, append([]string(nil), plan.EffectiveTools...))
	}

	body := st.Body
	if st.FetchBody != nil {
		var err error
		body, err = st.FetchBody(ctx)
		if err != nil {
			return "", err
		}
	}
	body = strings.ReplaceAll(body, "${AWAKEN_SKILL_DIR}", st.RootDir)

	body, err = SubstituteArgs(body, st.SkillID, st.ArgDefs, args)
	if err != nil {
		return "", err
	}
	body = st.renderBodyWithPackageContext(body)

	if st.Executor == nil {
		return fmt.Sprintf("[skill %q] body after substitution:\n%s", st.SkillID, body), nil
	}
	return st.Executor.Execute(ctx, st.SkillID, body, append([]string(nil), st.AllowedTools...))
}

// LastActivationPlan returns the plan produced by the most recent Execute.
func (st *SkillTool) LastActivationPlan() *SkillActivationPlan {
	if st == nil {
		return nil
	}
	return st.lastPlan
}

// ResolvedToolPoolAfterExecute exposes the effective tool pool for the dual
// gate after a successful activation.
func (st *SkillTool) ResolvedToolPoolAfterExecute() []string {
	if st == nil || st.lastPlan == nil {
		return nil
	}
	if st.lastPlan.CompatibilityStatus != CompatibilityOK {
		return nil
	}
	return append([]string(nil), st.lastPlan.EffectiveTools...)
}

// LastActivationMainBody returns the host-loaded skill body from the last plan.
func (st *SkillTool) LastActivationMainBody() string {
	if st == nil || st.lastPlan == nil {
		return ""
	}
	return st.lastPlan.MainBody
}

// LastActivationSkillID returns the skill id from the last plan.
func (st *SkillTool) LastActivationSkillID() string {
	if st == nil || st.lastPlan == nil {
		return st.SkillID
	}
	return st.lastPlan.SkillID
}

// IsReadOnly implements tool.ReadOnlyTool. Skills are considered read-only
// because they are prompt templates; any mutation is done by tools they
// call, not the skill itself.
func (st *SkillTool) IsReadOnly() bool { return true }

// IsSkillTool implements the SkillToolMarker interface so the server layer
// can identify skill-derived tools in the unified tool list.
func (st *SkillTool) IsSkillTool() bool { return true }

// Definition implements tool.Tool.
func (st *SkillTool) Definition() tool.ToolDefinition {
	requiresAuth := st.FetchBody != nil
	return tool.ToolDefinition{
		Name:        st.NameStr,
		DisplayName: st.SkillID,
		Source:      tool.SourceSkill,
		Description: st.Description(),
		Annotations: tool.ToolAnnotations{
			ReadOnly:     true,
			Destructive:  false,
			OpenWorld:    false,
			RequiresAuth: requiresAuth,
		},
	}
}

// SkillToolName builds the namespaced tool name for a skill ID.
// Hyphens in the skill ID are converted to underscores so the result
// satisfies the tool registry's name pattern.
func SkillToolName(skillID string) string {
	return SkillToolPrefix + strings.ReplaceAll(skillID, "-", "_")
}

func (st *SkillTool) renderBodyWithPackageContext(body string) string {
	if strings.TrimSpace(st.RootDir) == "" {
		return body
	}
	return fmt.Sprintf(
		"Base directory for this skill: %s\nRequired and preload resources are host-loaded when the skill activator is used. Use skill_resource_read only for optional on-demand package files.\n\n%s",
		st.RootDir,
		body,
	)
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
