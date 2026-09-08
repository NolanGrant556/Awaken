package skillruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"awaken/src/skills"
	"awaken/src/tool"
)

const CatalogToolName = "skill"

type SkillInvocationRequest struct {
	Skill          string
	Args           string
	Source         InvocationSource
	ParentToolPool []string
}

type SkillInvocationResult struct {
	Plan    *SkillActivationPlan
	Content string
	Forked  bool
	ChildID string
}

// SkillForkRequest is the complete, already-loaded handoff for a true child
// Agent. The adapter owned by runtime/agentcore can pass Body as the child's
// first meta context and call agentcore.CreateSubagent with the remaining
// model/agent hints. ToolHints never restrict the inherited parent tool pool.
type SkillForkRequest struct {
	SkillID   string
	Body      string
	RootDir   string
	SkillPath string
	Agent     string
	Model     string
	Effort    string
	ToolHints []skills.ToolHint
}

type SkillForkResult struct {
	ChildID string
	Content string
}

type ChildAgentStarter interface {
	StartSkillChild(ctx context.Context, request SkillForkRequest) (SkillForkResult, error)
}

type ChildAgentStarterFunc func(context.Context, SkillForkRequest) (SkillForkResult, error)

func (f ChildAgentStarterFunc) StartSkillChild(ctx context.Context, request SkillForkRequest) (SkillForkResult, error) {
	return f(ctx, request)
}

type childAgentStarterContextKey struct{}

// WithChildAgentStarter binds a run-scoped child starter. Runner should use
// this for every `skill` tool call so parent config, recorder, cancellation,
// and identity never leak through Manager-global mutable state.
func WithChildAgentStarter(ctx context.Context, starter ChildAgentStarter) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, childAgentStarterContextKey{}, starter)
}

func childAgentStarterFromContext(ctx context.Context) ChildAgentStarter {
	if ctx == nil {
		return nil
	}
	starter, _ := ctx.Value(childAgentStarterContextKey{}).(ChildAgentStarter)
	return starter
}

// CatalogTool is the single model-facing Skill entrypoint. Catalog metadata is
// exposed in its definition; full SKILL.md content is loaded only by Invoke.
type CatalogTool struct {
	Registry      *skills.SkillRegistry
	Activator     *Activator
	Executor      SkillExecutor
	ForkStarter   ChildAgentStarter
	ContextTokens func() int
	OnActivated   func(plan *SkillActivationPlan)
}

func (t *CatalogTool) Name() string { return CatalogToolName }

func (t *CatalogTool) Description() string {
	snapshot := t.Catalog()
	if snapshot.Listing == "" {
		return "Load an installed Skill by name and apply its instructions to the current task."
	}
	return skillTriggerRulesPrefix + snapshot.Listing
}

// ModelDescriptionForContext lets RequestBuilder preserve the Skill Catalog's
// own 1%-of-context budget instead of applying the generic 1,800-rune tool
// description cap a second time.
func (t *CatalogTool) ModelDescriptionForContext(contextWindowTokens int) string {
	listing := t.ModelCatalogForContext(contextWindowTokens)
	if listing == "" {
		return "Load an installed Skill by name and apply its instructions to the current task."
	}
	return skillTriggerRulesPrefix + listing
}

// ModelCatalogForContext exposes only the model-visible Skill discovery
// listing (name, source, description and when-to-use). Agent Core uses this
// typed boundary to account catalog metadata separately from the builtin
// Skill tool wrapper and from full activated Skill instructions.
func (t *CatalogTool) ModelCatalogForContext(contextWindowTokens int) string {
	if t == nil || t.Registry == nil {
		return ""
	}
	return strings.TrimSpace(CompileCatalog(t.Registry.ListModelInvocable(), contextWindowTokens).Listing)
}

// skillTriggerRulesPrefix carries the mandatory Skill selection rules right
// next to the Catalog listing, so the model sees "when to call" and "what is
// available" in the same place. This is a deterministic Prompt constraint,
// not a host-side matcher: the model still judges whether a task matches a
// Skill's description. It sits outside the Catalog's own 1%-of-context
// budget so tightening the listing budget cannot weaken Awaken's selection
// and invocation contract.
const skillTriggerRulesPrefix = `Load one installed Skill by exact catalog name.

Trigger rules (mandatory):
- If the user explicitly names an available Skill from this catalog, you MUST invoke it.
- If the task clearly matches a Skill's description below, invoking it is a BLOCKING REQUIREMENT: call this tool BEFORE using ordinary tools for that part of the task.
- If multiple Skills match, invoke the minimal set that covers the request and state the order you will apply them.
- Announce which Skill(s) you are using and why (one short line). If you skip an obviously matching Skill, say why.
- NEVER claim that you are using or have loaded a Skill without actually invoking it. Do not re-invoke a Skill whose instructions are already in your context.
- If a named Skill is not in the catalog, say so briefly and continue with the best fallback.
- If nothing in the catalog matches, proceed with ordinary tools; do not invoke a Skill just to satisfy this rule.

Available Skills:
`

func (t *CatalogTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "skill", Type: "string", Description: "Catalog Skill name.", Required: true},
		{Name: "args", Type: "string", Description: "Optional arguments passed to the Skill. For MCP prompt skills, pass a JSON object string with named string values, e.g. {\"name\":\"Ada\"}.", Required: false},
	}
}

func (t *CatalogTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		SchemaVersion: tool.ToolDefinitionSchemaVersion,
		Name:          CatalogToolName, DisplayName: "Skill", Source: tool.SourceSkill,
		Description:  t.Description(),
		Purpose:      "Load the selected Skill instructions on demand without changing the run tool pool.",
		WhenToUse:    []string{"Use when a Skill in the catalog matches the current task."},
		WhenNotToUse: []string{"Do not invent a Skill name or use this tool to execute arbitrary files directly."},
		InputSchema: tool.ToolParametersSchema{Type: "object", Properties: map[string]tool.ToolParam{
			"skill": {Type: "string", Description: "Catalog Skill name."},
			"args":  {Type: "string", Description: "Optional arguments passed to the Skill."},
		}, Required: []string{"skill"}, AdditionalProperties: tool.BoolPtr(false)},
		OutputSchema: tool.NoDetailsOutputSchema(),
		SideEffects:  []tool.ToolSideEffect{{Kind: tool.SideEffectRead, Resources: []string{"installed Skill package"}, Description: "Reads the selected SKILL.md on demand.", Reversibility: tool.ReversibilityNotApplicable}},
		Permissions:  tool.ToolPermissionSpec{CapabilityClass: tool.CapabilityArtifactRead, WorkspaceScope: tool.WorkspaceScopeRead, FileBridgeScope: tool.FileBridgeTaskScopeRead, NetworkScope: tool.NetworkScopeNone},
		Examples:     []tool.ToolExample{{Scenario: "Load a PDF workflow Skill", Input: map[string]any{"skill": "pdf", "args": "analyze report.pdf"}, Outcome: "Returns the loaded Skill context or the result of its forked child Agent."}},
		FailureModes: []tool.ToolFailureMode{
			{Code: "skill_not_found", Cause: "The requested Skill is not in the catalog.", Retryable: false, Recovery: []string{"Select a listed Skill name."}},
			{Code: "skill_content_load_failed", Cause: "The installed SKILL.md could not be read safely.", Retryable: true, Recovery: []string{"Repair or reinstall the Skill package."}},
			{Code: "skill_model_invocation_disabled", Cause: "The Skill only permits explicit user invocation.", Retryable: false, Recovery: []string{"Ask the user to invoke the Skill explicitly."}},
			{Code: "skill_fork_unavailable", Cause: "A fork Skill was selected without a child-Agent starter.", Retryable: true, Recovery: []string{"Connect the runtime ChildAgentStarter and retry."}},
		},
		Annotations: tool.ToolAnnotations{ReadOnly: true, Destructive: false, OpenWorld: false, CapabilityClass: tool.CapabilityArtifactRead, TrustedOrigin: tool.TrustedOriginBuiltin, FileBridgeScope: tool.FileBridgeTaskScopeRead, NetworkScope: tool.NetworkScopeNone},
	}
}

func (t *CatalogTool) IsReadOnly() bool { return true }

func (t *CatalogTool) Catalog() CatalogSnapshot {
	if t == nil || t.Registry == nil {
		return CompileCatalog(nil, 0)
	}
	contextTokens := 0
	if t.ContextTokens != nil {
		contextTokens = t.ContextTokens()
	}
	return CompileCatalog(t.Registry.ListModelInvocable(), contextTokens)
}

func (t *CatalogTool) Execute(ctx context.Context, input string) (string, error) {
	var request struct {
		Skill string `json:"skill"`
		Args  string `json:"args"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(input)), &request); err != nil {
		return "", fmt.Errorf("skill arguments must be a JSON object: %w", err)
	}
	result, err := t.Invoke(ctx, SkillInvocationRequest{Skill: request.Skill, Args: request.Args, Source: InvocationModel})
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

func (t *CatalogTool) Invoke(ctx context.Context, request SkillInvocationRequest) (SkillInvocationResult, error) {
	if t == nil || t.Activator == nil {
		return SkillInvocationResult{}, fmt.Errorf("skill activator unavailable")
	}
	if request.Source == "" {
		request.Source = InvocationUser
	}
	if request.ParentToolPool == nil {
		if scope, ok := activationScopeFromContext(ctx); ok {
			request.ParentToolPool = scope.parentToolPool
		}
	}
	plan, err := t.Activator.Activate(ActivationRequest{
		Context: ctx, SkillID: request.Skill, RawArgs: request.Args,
		InvocationSource: request.Source, ParentToolPool: request.ParentToolPool,
	})
	if plan != nil {
		scope, _ := activationScopeFromContext(ctx)
		if notifyErr := notifyActivation(ctx, scope, plan); notifyErr != nil {
			return SkillInvocationResult{Plan: plan}, notifyErr
		}
		if t.OnActivated != nil {
			t.OnActivated(plan.Clone())
		}
	}
	if err != nil {
		return SkillInvocationResult{Plan: plan}, err
	}
	if t.Registry != nil {
		t.Registry.MarkInvoked(plan.SkillID, time.Now().UTC())
	}
	if plan.Context == skills.ContextFork {
		starter := childAgentStarterFromContext(ctx)
		if starter == nil {
			starter = t.ForkStarter
		}
		if starter == nil {
			return SkillInvocationResult{Plan: plan}, &SkillCompatibilityError{Code: "skill_fork_unavailable", Message: "context: fork requires a configured ChildAgentStarter"}
		}
		child, childErr := starter.StartSkillChild(ctx, SkillForkRequest{
			SkillID: plan.SkillID, Body: plan.MainBody, RootDir: plan.RootDir,
			SkillPath: plan.SkillFilePath, Agent: plan.Agent, Model: plan.Model,
			Effort: plan.Effort, ToolHints: append([]skills.ToolHint(nil), plan.ToolHints...),
		})
		if childErr != nil {
			return SkillInvocationResult{Plan: plan}, fmt.Errorf("fork skill %q: %w", plan.SkillID, childErr)
		}
		return SkillInvocationResult{Plan: plan, Content: child.Content, Forked: true, ChildID: child.ChildID}, nil
	}
	if t.Executor == nil {
		return SkillInvocationResult{Plan: plan, Content: plan.MainBody}, nil
	}
	hints := make([]string, 0, len(plan.ToolHints))
	for _, hint := range plan.ToolHints {
		hints = append(hints, hint.Canonical)
	}
	content, err := t.Executor.Execute(ctx, plan.SkillID, plan.MainBody, hints)
	return SkillInvocationResult{Plan: plan, Content: content}, err
}
