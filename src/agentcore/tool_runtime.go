package agentcore

import (
	stdcontext "context"
	"fmt"
	"strings"
	"time"

	"awaken/src/agentcore/executors"
	"awaken/src/agentcore/guardian"
	"awaken/src/agentcore/subagent"
	convcontext "awaken/src/context"
	"awaken/src/tool"
	"awaken/src/tool/profile"
	"awaken/src/tools/ask_user"
)

const maxPatchPreviewChars = 50000

type ToolRuntime struct {
	sink              EventSink
	permissionContext tool.PermissionContext
	autoPolicy        AutoToolPolicy
	serverConnected   func(serverName string) bool
	auditSink         tool.AuditSink
	runID             string
	toolSnapshot      *tool.RunToolAvailabilitySnapshot
	environment       *tool.ExecutionEnvironmentSnapshot
	// profile is the unified ExecutionProfile applied to every tool call
	// this runtime executes. It is populated from ToolRuntimeConfig.Profile
	// and defaults to profile.DefaultProfile() (WorkspaceWrite) when unset.
	// The profile is currently surfaced on ToolExecutionResult and logged;
	// Task 11 (executors) and Task 12/13 (subagent/guardian) consume it
	// more deeply.
	profile profile.ExecutionProfile
	// subagentBoundary, when set, marks this runtime as belonging to a
	// sub-agent and captures the boundary it inherited from its parent.
	// Execute uses it as a defense-in-depth check: every tool call must
	// not exceed this boundary (workspace roots must match, runtime
	// profile must not escalate the boundary profile). nil for the root
	// agent.
	subagentBoundary *subagent.ParentBoundary
	// subagentOrigin, when set, is propagated into every ExecutionIntent
	// this runtime issues so the decision layer can attribute the call to
	// the spawning sub-agent. nil for the root agent.
	subagentOrigin *profile.SubagentOrigin
	// router maps an ExecutionIntent + ExecutionProfile to the appropriate
	// ToolExecutor (FileOperationExecutor / BashExecutor / McpExecutor /
	// GuardianExecutor). Task 11 of "零审批自动执行策略": the Router sits
	// between the auto policy decision and the raw tool execution, adding
	// executor-specific enforcement (boundary checks, sandbox consultation,
	// risk annotation checks, read-only enforcement). When nil, Execute
	// falls back to the legacy executeTool path.
	router            *executors.Router
	threadID          string
	modelCapabilities tool.ModelCapabilities
	toolSessionState  *tool.ToolSessionState
	modelContentStore tool.ModelContentStore
}

type ToolRuntimeConfig struct {
	PermissionContext            tool.PermissionContext
	AutoToolPolicy               AutoToolPolicy
	ServerConnected              func(serverName string) bool
	AuditSink                    tool.AuditSink
	RunID                        string
	ToolAvailabilitySnapshot     *tool.RunToolAvailabilitySnapshot
	ExecutionEnvironmentSnapshot *tool.ExecutionEnvironmentSnapshot
	// Profile is the unified ExecutionProfile applied to every tool call
	// this runtime executes. When nil, the runtime falls back to
	// profile.DefaultProfile() (WorkspaceWrite).
	Profile *profile.ExecutionProfile
	// SubagentBoundary, when set, marks this runtime as a sub-agent and
	// captures the inherited boundary. Execute enforces it as a
	// defense-in-depth check.
	SubagentBoundary *subagent.ParentBoundary
	// SubagentOrigin, when set, is propagated into every ExecutionIntent
	// so the decision layer can attribute the call.
	SubagentOrigin    *profile.SubagentOrigin
	ModelCapabilities tool.ModelCapabilities
	ToolSessionState  *tool.ToolSessionState
	ModelContentStore tool.ModelContentStore
}

type ToolExecutionResult struct {
	ToolCallID string
	Name       string
	Content    string
	IsError    bool
	Truncated  bool
	Status     ToolExecutionStatus
	ErrorCode  string
	// Outcome is the canonical structured tool result when available.
	// Compatibility fields (Status/IsError/ErrorCode/DurationMs) are derived
	// from it; emitToolResult prefers Outcome when Status is set.
	Outcome        tool.ExecutionOutcome
	DecisionSource ToolDecisionSource
	DecisionReason string
	StartedAt      time.Time
	FinishedAt     time.Time
	DurationMs     int64
	AffectedPaths  []string
	// ActualAffectedPaths contains executor-observed resources. AffectedPaths
	// may also contain permission-time predictions for compatibility, so only
	// this field is eligible to become canonical Outcome changes.
	ActualAffectedPaths []string
	PatchSummary        string
	ResultSummary       string
	// Intent is the unified ExecutionIntent produced from the ToolCall.
	// Task 8 only populates it; Task 9 will consume it in the decision layer.
	Intent *profile.ExecutionIntent
	// Decision is the unified ExecutionDecision produced by the auto tool
	// policy. It is the primary decision record for this execution.
	Decision *profile.ExecutionDecision
	// ProfileName is the name of the ExecutionProfile applied to this
	// execution. It is the human-readable profile name (e.g.
	// "workspace_write") surfaced for audit/logging. The full profile is
	// available on the ToolRuntime via Profile().
	ProfileName string
	// ToolSnapshotID and EnvironmentSnapshotID identify the immutable catalog
	// and bounded user-machine probe used for both selection and execution.
	ToolSnapshotID         string
	EnvironmentSnapshotID  string
	ProviderID             tool.ProviderID
	ProviderCapabilities   []tool.Capability
	ProviderReadiness      tool.ProviderReadiness
	ProviderDependencies   []tool.DependencyRequirement
	DependencyObservations []tool.DependencyObservation
	// TransformedFrom is the original tool name when this execution is the
	// target of a transform decision. Empty for direct (non-transformed)
	// calls and surfaced on the audit record and ToolPermissionDecided event.
	TransformedFrom string
	// TrashedPaths lists the absolute paths moved to the OS recycle bin
	// during this call. Populated from structured executor outcomes and
	// surfaced on the audit record and ToolPermissionDecided event.
	TrashedPaths []string
	// GuardianViolation is populated when a guardian constraint was
	// violated (i.e. the call was denied because a guardian agent attempted
	// a non-read-only action). Task 16: surfaced on the audit record and
	// the ToolPermissionDecided event so the UI can attribute the denial.
	GuardianViolation string
	// outcomeContext is consumed by the single finalizer. It is intentionally
	// private so permission predictions cannot be promoted to execution facts.
	outcomeContext tool.OutcomeNormalizationContext
}

func NewToolRuntime(sink EventSink) *ToolRuntime {
	return NewToolRuntimeWithConfig(sink, ToolRuntimeConfig{})
}

func NewToolRuntimeWithConfig(sink EventSink, config ToolRuntimeConfig) *ToolRuntime {
	policy := config.AutoToolPolicy
	if policy == nil {
		policy = DefaultAutoToolPolicy{}
	}
	auditSink := config.AuditSink
	if auditSink == nil {
		auditSink = &tool.NoopSink{}
	}
	execProfile := profile.DefaultProfile()
	if config.Profile != nil {
		execProfile = *config.Profile
	}
	var pinnedToolSnapshot *tool.RunToolAvailabilitySnapshot
	if config.ToolAvailabilitySnapshot != nil {
		clone := config.ToolAvailabilitySnapshot.Clone()
		pinnedToolSnapshot = &clone
	}
	var pinnedEnvironment *tool.ExecutionEnvironmentSnapshot
	if config.ExecutionEnvironmentSnapshot != nil {
		clone := config.ExecutionEnvironmentSnapshot.Clone()
		pinnedEnvironment = &clone
	}
	return &ToolRuntime{
		sink:              normalizeEventSink(sink),
		permissionContext: config.PermissionContext.Normalize(),
		autoPolicy:        policy,
		serverConnected:   config.ServerConnected,
		auditSink:         auditSink,
		runID:             config.RunID,
		toolSnapshot:      pinnedToolSnapshot,
		environment:       pinnedEnvironment,
		profile:           execProfile,
		subagentBoundary:  config.SubagentBoundary,
		subagentOrigin:    config.SubagentOrigin,
		router:            executors.NewRouter(),
		modelCapabilities: config.ModelCapabilities,
		toolSessionState:  config.ToolSessionState,
		modelContentStore: config.ModelContentStore,
	}
}

func toolPoolSet(names []string) map[string]bool {
	if names == nil {
		return nil
	}
	set := make(map[string]bool, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			set[name] = true
		}
	}
	return set
}

// Profile returns the ExecutionProfile applied to every tool call this
// runtime executes. The returned value is a copy so callers may freely
// inspect or mutate it without affecting the runtime.
func (rt *ToolRuntime) Profile() profile.ExecutionProfile {
	return rt.profile
}

// validateSubagentBoundary is the runtime-side defense-in-depth check for
// sub-agent runs. It verifies that the runtime's permission context and
// profile have not drifted from the boundary the sub-agent inherited from
// its parent:
//
//   - The runtime's WorkspaceRoot MUST match the boundary's WorkspaceRoot.
//     A mismatch means the sub-agent was configured with a wider workspace
//     than its parent, which is an escalation.
//   - The runtime's WritableRoots MUST be a subset of the boundary's
//     WritableRoots. Extra writable roots are an escalation.
//   - The runtime's profile MUST NOT escalate the boundary's profile, as
//     defined by subagent.ValidateNoEscalation.
//
// This is a backstop: the primary enforcement happens at sub-agent
// creation via subagent.ResolveSubagentBoundary. The runtime re-checks on
// every Execute so a misconfigured or hostile caller cannot bypass the
// boundary by mutating the runtime after construction.
func (rt *ToolRuntime) validateSubagentBoundary() error {
	boundary := rt.subagentBoundary
	if boundary == nil {
		return nil
	}
	normalizedPerm := rt.permissionContext.Normalize()
	if strings.TrimSpace(boundary.WorkspaceRoot) != "" &&
		normalizedPerm.WorkspaceRoot != boundary.WorkspaceRoot {
		return fmt.Errorf(
			"workspace root mismatch: runtime %q does not match inherited boundary %q",
			normalizedPerm.WorkspaceRoot, boundary.WorkspaceRoot,
		)
	}
	if !subsetStrings(normalizedPerm.WritableRoots, boundary.WritableRoots) {
		return fmt.Errorf(
			"writable roots escalation: runtime %v is not a subset of inherited boundary %v",
			normalizedPerm.WritableRoots, boundary.WritableRoots,
		)
	}
	if boundary.Profile != nil {
		runtimeProfile := rt.profile
		if err := subagent.ValidateNoEscalation(&runtimeProfile, boundary.Profile); err != nil {
			return err
		}
	}
	return nil
}

// subsetStrings reports whether every value in `needles` is present in
// `haystack`. An empty `needles` is always a subset (including of an empty
// haystack). Order is not significant.
func subsetStrings(needles, haystack []string) bool {
	if len(needles) == 0 {
		return true
	}
	seen := make(map[string]struct{}, len(haystack))
	for _, h := range haystack {
		seen[h] = struct{}{}
	}
	for _, n := range needles {
		if _, ok := seen[n]; !ok {
			return false
		}
	}
	return true
}

func (rt *ToolRuntime) Execute(ctx stdcontext.Context, call convcontext.ToolCall) ToolExecutionResult {
	id := call.ID
	if id == "" {
		id = convcontext.GenerateToolCallID()
	}

	result := ToolExecutionResult{
		ToolCallID:  id,
		Name:        call.Function.Name,
		StartedAt:   timeNowUTC(),
		ProfileName: rt.profile.Name,
		ToolSnapshotID: func() string {
			if rt.toolSnapshot != nil {
				return rt.toolSnapshot.SnapshotID
			}
			return ""
		}(),
		EnvironmentSnapshotID: func() string {
			if rt.environment != nil {
				return rt.environment.SnapshotID
			}
			return ""
		}(),
		outcomeContext: tool.OutcomeNormalizationContext{
			OperationType:     call.Function.Name,
			WorkspaceRoot:     rt.permissionContext.WorkspaceRoot,
			ObservationSource: "runtime",
		},
	}

	// Defense-in-depth: if this runtime belongs to a sub-agent, validate
	// that the runtime's permission context and profile have not been
	// tampered with to exceed the inherited boundary. This catches the
	// case where a sub-agent's config was constructed with a wider
	// workspace root or a higher-privileged profile than the parent
	// allowed. The primary enforcement happens at sub-agent creation
	// (subagent.ResolveSubagentBoundary); this is the runtime-side
	// backstop.
	if rt.subagentBoundary != nil {
		if err := rt.validateSubagentBoundary(); err != nil {
			denied := ToolDecision{
				Action: ToolDecisionDeny,
				Source: ToolDecisionSourcePolicyError,
				Reason: fmt.Sprintf("subagent boundary violated: %v", err),
			}
			result = synthesizeDeniedToolResult(result, denied)
			result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
			return finalizeToolResult(result)
		}
	}

	var (
		selectedTool      tool.Tool
		def               tool.ToolDefinition
		definitionQuality tool.DefinitionQuality
		preserveInput     bool
		readiness         tool.ProviderReadinessReport
		provider          tool.ProviderManifest
		lookupErr         error
	)
	if rt.toolSnapshot != nil {
		entry, err := rt.toolSnapshot.Lookup(call.Function.Name)
		if err == nil {
			selectedTool = entry.Runtime
			def = entry.Definition
			definitionQuality = entry.Quality
			preserveInput = entry.PreserveInputObject
			readiness = entry.Readiness
			provider = entry.Provider
		}
		lookupErr = err
	} else {
		registeredTool, err := tool.GetRegisteredTool(call.Function.Name)
		if err == nil {
			selectedTool = registeredTool.Runtime
			def = registeredTool.Definition
			definitionQuality = registeredTool.Quality
			preserveInput = registeredTool.PreserveInputObject
			provider = registeredTool.Provider
			readiness = tool.ProviderReadinessReport{Readiness: provider.Readiness}
		}
		lookupErr = err
	}
	if lookupErr != nil || selectedTool == nil {
		return finalizeToolResult(synthesizeUnsupportedToolResult(
			result,
			"unknown_tool",
			fmt.Sprintf("工具执行失败: %v: %s", lookupErr, call.Function.Name),
			[]string{"refresh_tool_registry", "select_an_available_tool"},
			[]tool.NextAction{
				{ID: "refresh_tool_registry", Description: "Refresh the available tool registry before making another selection."},
				{ID: "switch_tool", Description: "Select a currently registered tool that provides the required capability.", Preconditions: []string{"Do not replay the missing tool name unchanged."}},
			},
		))
	}
	result.outcomeContext.DeclaredSideEffects = append([]tool.ToolSideEffect(nil), def.SideEffects...)
	result.outcomeContext.OutputSchema = def.OutputSchema
	result.outcomeContext.StrictOutputSchema = definitionQuality == tool.DefinitionQualityStrict
	result.ProviderID = provider.ProviderID
	result.ProviderCapabilities = append([]tool.Capability(nil), provider.Capabilities...)
	result.ProviderReadiness = readiness.Readiness
	result.ProviderDependencies = append([]tool.DependencyRequirement(nil), provider.Dependencies...)
	result.DependencyObservations = append([]tool.DependencyObservation(nil), readiness.Dependencies...)
	if readiness.Readiness == tool.ProviderNeedsAuth || readiness.Readiness == tool.ProviderNeedsRuntime || readiness.Readiness == tool.ProviderUnavailable {
		return finalizeToolResult(rt.providerReadinessResult(result, provider, readiness))
	}

	input, err := normalizeToolInputWithSchema(call.Function.Arguments, selectedTool, def.InputSchema, preserveInput)
	if err != nil {
		return finalizeToolResult(synthesizeValidationToolResult(result, err.Error()))
	}

	// Build the unified ExecutionIntent. Task 8 only defines the type and
	// converts to it; Task 9 will consume the intent in the decision layer.
	// Task 12: propagate the subagent origin so the decision layer can
	// attribute the call to the spawning sub-agent.
	intent := profile.BuildExecutionIntentWithDefinition(call, selectedTool, def, rt.permissionContext, rt.subagentOrigin)
	result.Intent = &intent

	// Guardian constraint enforcement (Task 13 of "零审批自动执行策略"):
	// guardian/review agents are forced to read-only. This check runs BEFORE
	// the normal auto-tool-policy decision flow — guardian constraints are
	// absolute and cannot be overridden by the policy. This complements any
	// executor-level enforcement (Task 11) as defense in depth.
	if guardian.IsGuardianProfile(rt.profile) {
		if gErr := guardian.EnforceGuardianConstraints(intent, rt.profile); gErr != nil {
			gReason := gErr.Error()
			if gViolation, ok := gErr.(*guardian.GuardianViolationError); ok {
				gReason = gViolation.Reason
			}
			gDecision := ToolDecision{
				Action: ToolDecisionDeny,
				Source: ToolDecisionSourcePolicyError,
				Reason: gReason,
			}
			gExecDecision := profile.DenyDecision(
				string(profile.DecisionSourcePolicyError),
				gReason,
				false,
			)
			result.Decision = &gExecDecision
			// Task 16: record the guardian violation reason on the result so
			// the audit record and the ToolPermissionDecided event can
			// attribute the denial to a guardian constraint.
			result.GuardianViolation = gReason
			result = synthesizeDeniedToolResult(result, gDecision)
			result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
			gPermission := tool.DenyPermission(gReason)
			rt.emitToolPermissionDecided(result, gPermission, gDecision)
			rt.writeAudit(def, result, gPermission, string(gDecision.Source), "denied", "")
			return finalizeToolResult(result)
		}
	}

	toolCtx := tool.ContextWithPermissionContext(ctx, rt.permissionContext)
	toolCtx = tool.ContextWithToolThreadID(toolCtx, rt.threadID)
	toolCtx = tool.ContextWithModelCapabilities(toolCtx, rt.modelCapabilities)
	toolCtx = tool.ContextWithToolSessionState(toolCtx, rt.toolSessionState)
	toolCtx = tool.ContextWithModelContentStore(toolCtx, rt.modelContentStore)
	toolCtx = ContextWithEventSink(toolCtx, rt.sink)
	toolCtx = tool.ContextWithACPEventEmitter(toolCtx, func(e tool.ACPEvent) {
		rt.sink.Emit(MapACPEvent(e))
	})
	toolCtx = tool.ContextWithToolExecutionIdentity(toolCtx, tool.ToolExecutionIdentity{
		RunID:      rt.runID,
		ToolCallID: result.ToolCallID,
		ToolName:   result.Name,
	})
	if rt.toolSnapshot != nil && rt.environment != nil {
		toolCtx = tool.ContextWithRunAvailability(toolCtx, *rt.toolSnapshot, *rt.environment)
		// Process-backed tools (for example Shell and the typed dependency
		// installer) consume the same immutable environment through the narrow
		// environment context API.  Pin both views here so execution cannot
		// silently re-probe a newer PATH after provider selection.
		toolCtx = tool.ContextWithExecutionEnvironmentSnapshot(toolCtx, *rt.environment)
	}
	permission := rt.decideToolPermission(toolCtx, selectedTool, def, input)
	rt.emitPatchPreview(result, selectedTool, input)

	decisionRequest := inferToolDecisionRequest(
		id,
		call.Function.Name,
		def,
		call.Function.Arguments,
		input,
		permission,
		rt.permissionContext,
	)
	decision := ToolDecision{}
	var execDecision profile.ExecutionDecision
	switch permission.Behavior {
	case tool.PermissionAllow:
		execDecision = rt.autoPolicy.DecideExecution(toolCtx, intent, decisionRequest)
	case tool.PermissionDeny, tool.PermissionNeedsAuth, tool.PermissionNeedsRuntime, tool.PermissionUnsupported:
		execDecision = DefaultAutoToolPolicy{}.DecideExecution(toolCtx, intent, decisionRequest)
	default:
		execDecision = profile.DenyDecision(
			string(profile.DecisionSourcePolicyError),
			fmt.Sprintf("invalid permission decision %q", permission.Behavior),
			false,
		)
	}

	decision = executionDecisionToToolDecision(execDecision)
	// Preserve the normalized input for allow decisions that do not rewrite it.
	if decision.Action == ToolDecisionAllow && strings.TrimSpace(decision.UpdatedInput) == "" {
		decision.UpdatedInput = input
	}
	result.Decision = &execDecision

	decision = normalizeToolDecision(decision, firstNonEmpty(permission.Reason, "AUTO policy denied tool execution"))
	// Task 16: when the decision is a transform, record the original tool
	// name on the result before emitting ToolPermissionDecided so the UI can
	// show the rewrite.
	if decision.Action == ToolDecisionTransform {
		result.TransformedFrom = call.Function.Name
	}
	rt.emitToolPermissionDecided(result, permission, decision)
	result.DecisionSource = decision.Source
	result.DecisionReason = decision.Reason
	if strings.TrimSpace(decision.UpdatedInput) != "" {
		input = decision.UpdatedInput
	}

	// ask_user is handled specially: it does not execute a real tool but
	// blocks the agent loop until the user answers the question(s) in the UI.
	if call.Function.Name == askuser.ToolName {
		questions, parseErr := askuser.ParseQuestions(input)
		if parseErr != nil {
			result = synthesizeValidationFailure(
				result,
				"ask_user_invalid_input",
				fmt.Sprintf("提问格式错误: %v", parseErr),
				[]string{"correct_the_question_structure", "provide_two_to_four_options_per_question"},
			)
			result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
			result = finalizeToolResult(result)
			rt.writeAudit(def, result, permission, string(decision.Source), auditOutcomeLabel(result), parseErr.Error())
			return result
		}

		request := rt.userQuestionEvent(result, def, questions)
		awaiter, ok := rt.sink.(UserQuestionAwaiter)
		if !ok {
			rt.sink.Emit(request)
			result = synthesizeUnsupportedToolResult(
				result,
				"ask_user_no_awaiter",
				"当前运行环境不支持用户提问",
				[]string{"use_an_interactive_runtime", "ask_the_user_in_the_parent_channel"},
				[]tool.NextAction{{ID: "switch_to_interactive_runtime", Description: "Continue in a runtime that can await a user response.", Preconditions: []string{"Do not claim that an answer was received."}}},
			)
			result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
			result = finalizeToolResult(result)
			rt.writeAudit(def, result, permission, string(decision.Source), auditOutcomeLabel(result), "no user question awaiter")
			return result
		}

		answer, err := awaiter.AwaitUserQuestion(ctx, request)
		if err != nil {
			result = synthesizeAskUserAwaitFailure(result, ctx, err)
			result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
			result = finalizeToolResult(result)
			rt.writeAudit(def, result, permission, string(decision.Source), auditOutcomeLabel(result), err.Error())
			return result
		}

		result.Content = askuser.FormatAnswerTextResult(questions, answer)
		result.Outcome = tool.SucceededOutcome(result.Content)
		result.Outcome.Summary = "User answered the requested clarification."
		result.Outcome.Details = map[string]any{"answer_text": answer, "question_count": len(questions)}
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		result = finalizeToolResult(result)
		rt.writeAudit(def, result, permission, string(decision.Source), auditOutcomeLabel(result), terminalAuditError(result))
		return result
	}

	switch decision.Action {
	case ToolDecisionAllow:
	case ToolDecisionDeny:
		result = synthesizeDeniedToolResult(result, decision)
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		rt.writeAudit(def, result, permission, string(decision.Source), "denied", "")
		return finalizeToolResult(result)
	case ToolDecisionNeedsAuth:
		rt.emitToolProgress(result, "needs_auth", firstNonEmpty(decision.Reason, "tool requires authentication"), 0)
		rt.emitAuthRequired(result, def, decision)
		result = synthesizeStructuredToolResult(result, decision, "needs_auth", "需要授权")
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		rt.writeAudit(def, result, permission, string(decision.Source), "needs_auth", "")
		return finalizeToolResult(result)
	case ToolDecisionNeedsRuntime:
		rt.emitToolProgress(result, "needs_runtime", firstNonEmpty(decision.Reason, "tool requires local runtime"), 0)
		rt.emitRuntimeMissing(result, decision)
		result = synthesizeStructuredToolResult(result, decision, "needs_runtime", "缺少本地 runtime")
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		rt.writeAudit(def, result, permission, string(decision.Source), "needs_runtime", "")
		return finalizeToolResult(result)
	case ToolDecisionUnsupported:
		rt.emitToolProgress(result, "unsupported", firstNonEmpty(decision.Reason, "tool capability is unsupported"), 0)
		result = synthesizeStructuredToolResult(result, decision, firstNonEmpty(permission.ErrorCode, "unsupported"), "暂不支持该工具能力")
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		rt.writeAudit(def, result, permission, string(decision.Source), "unsupported", "")
		return finalizeToolResult(result)
	case ToolDecisionAbort:
		result = synthesizeAbortedToolResult(result, decision)
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		rt.writeAudit(def, result, permission, string(decision.Source), "aborted", result.DecisionReason)
		return finalizeToolResult(result)
	case ToolDecisionTransform:
		return rt.executeTransformedTool(toolCtx, result, call, decision, permission, def, intent)
	}

	rt.emitHostActivity(result, selectedTool, input)

	output, trashedPaths, execOut, err := rt.routeToExecutor(toolCtx, result, intent, selectedTool, input)
	result.TrashedPaths = append([]string(nil), trashedPaths...)
	if err != nil {
		failContent := output
		if strings.TrimSpace(failContent) == "" {
			failContent = fmt.Sprintf("工具执行失败: %v", err)
		}
		applyExecutorOutcome(&result, execOut)
		result.Content = failContent
		if handled, done := rt.maybeAwaitCLIAuthInteraction(toolCtx, result, def, permission, decision, selectedTool, intent, input, false); done {
			return handled
		}
		result = synthesizeExecutionErrorToolResult(result, "tool_execution_failed", failContent, err.Error())
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		return rt.finalizeExecutedToolResult(result, def, permission, string(decision.Source), selectedTool, input, output)
	}

	result.Content = output
	applyExecutorOutcome(&result, execOut)
	result.ResultSummary = excerptString(strings.TrimSpace(output), 200)
	// TrashedPaths are sourced from structured executor outcomes.
	result.TrashedPaths = trashedPaths
	if handled, done := rt.maybeAwaitCLIAuthInteraction(toolCtx, result, def, permission, decision, selectedTool, intent, input, false); done {
		return handled
	}
	return rt.finalizeExecutedToolResult(result, def, permission, string(decision.Source), selectedTool, input, output)
}
