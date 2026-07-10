package agentcore

import (
	stdcontext "context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"awaken/src/agentcore/executors"
	"awaken/src/agentcore/guardian"
	"awaken/src/agentcore/subagent"
	convcontext "awaken/src/context"
	"awaken/src/internal/outputlimit"
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
	// resolvedToolPool, when non-nil, is the dual-gate effective tool set.
	resolvedToolPool map[string]bool
	// router maps an ExecutionIntent + ExecutionProfile to the appropriate
	// ToolExecutor (FileChainExecutor / ShellExecutor / McpExecutor /
	// GuardianExecutor). Task 11 of "零审批自动执行策略": the Router sits
	// between the auto policy decision and the raw tool execution, adding
	// executor-specific enforcement (boundary checks, sandbox consultation,
	// risk annotation checks, read-only enforcement). When nil, Execute
	// falls back to the legacy executeTool path.
	router *executors.Router
}

type ToolRuntimeConfig struct {
	PermissionContext tool.PermissionContext
	AutoToolPolicy    AutoToolPolicy
	ServerConnected   func(serverName string) bool
	AuditSink         tool.AuditSink
	RunID             string
	// ResolvedToolPool, when non-nil, is the dual-gate effective tool set.
	// Execute rejects tool names outside this pool before GetTool.
	ResolvedToolPool []string
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
	SubagentOrigin *profile.SubagentOrigin
}

type ToolExecutionResult struct {
	ToolCallID     string
	Name           string
	Content        string
	IsError        bool
	Truncated      bool
	Status         ToolExecutionStatus
	ErrorCode      string
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
	PatchSummary   string
	ResultSummary  string
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
	// TransformedFrom is the original tool name when this execution is the
	// target of a transform decision (e.g. "shell" when a shell echo-write
	// was rewritten to apply_patch). Empty for direct (non-transformed)
	// calls. Task 16: surfaced on the audit record and the
	// ToolPermissionDecided event so the UI can show "shell → apply_patch".
	TransformedFrom string
	// TrashedPaths lists the absolute paths moved to the OS recycle bin
	// during this call. Populated by parsing the apply_patch output (which
	// encodes "D path (已移到回收站: trashedPath)" lines). Task 16: surfaced
	// on the audit record and the ToolPermissionDecided event so the UI can
	// show "Deleted file moved to recycle bin".
	TrashedPaths []string
	// GuardianViolation is populated when a guardian constraint was
	// violated (i.e. the call was denied because a guardian agent attempted
	// a non-read-only action). Task 16: surfaced on the audit record and
	// the ToolPermissionDecided event so the UI can attribute the denial.
	GuardianViolation string
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
	return &ToolRuntime{
		sink:              normalizeEventSink(sink),
		permissionContext: config.PermissionContext.Normalize(),
		autoPolicy:        policy,
		serverConnected:   config.ServerConnected,
		auditSink:         auditSink,
		runID:             config.RunID,
		profile:           execProfile,
		subagentBoundary:  config.SubagentBoundary,
		subagentOrigin:    config.SubagentOrigin,
		resolvedToolPool:  toolPoolSet(config.ResolvedToolPool),
		router:            executors.NewRouter(),
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

// SetResolvedToolPool updates the dual-gate tool pool for subsequent Execute
// calls. nil clears the gate; non-nil rejects tools outside the set.
func (rt *ToolRuntime) SetResolvedToolPool(names []string) {
	if rt == nil {
		return
	}
	rt.resolvedToolPool = toolPoolSet(names)
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

	if rt.resolvedToolPool != nil && !rt.resolvedToolPool[call.Function.Name] {
		denied := ToolDecision{
			Action: ToolDecisionDeny,
			Source: ToolDecisionSourcePolicyError,
			Reason: fmt.Sprintf("%s: tool %q is outside the resolved skill tool pool", tool.ErrorCodeSkillToolNotAllowed, call.Function.Name),
		}
		result = synthesizeDeniedToolResult(result, denied)
		result.ErrorCode = tool.ErrorCodeSkillToolNotAllowed
		result.Outcome.ErrorCode = tool.ErrorCodeSkillToolNotAllowed
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		return finalizeToolResult(result)
	}

	selectedTool, err := tool.GetTool(call.Function.Name)
	if err != nil {
		return finalizeToolResult(synthesizeFailedToolResult(result, "unknown_tool", fmt.Sprintf("工具执行失败: %v: %s", err, call.Function.Name)))
	}

	input, err := normalizeToolInputWithSchema(call.Function.Arguments, selectedTool)
	if err != nil {
		return finalizeToolResult(synthesizeValidationToolResult(result, err.Error()))
	}

	// Build the unified ExecutionIntent. Task 8 only defines the type and
	// converts to it; Task 9 will consume the intent in the decision layer.
	// Task 12: propagate the subagent origin so the decision layer can
	// attribute the call to the spawning sub-agent.
	intent := profile.BuildExecutionIntent(call, selectedTool, rt.permissionContext, rt.subagentOrigin)
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
			rt.writeAudit(selectedTool.Definition(), result, gPermission, string(gDecision.Source), "denied", "")
			return finalizeToolResult(result)
		}
	}

	toolCtx := tool.ContextWithPermissionContext(ctx, rt.permissionContext)
	permission := rt.decideToolPermission(toolCtx, selectedTool, input)
	def := selectedTool.Definition()
	result.AffectedPaths = append([]string(nil), permission.Metadata.AffectedPaths...)
	result.PatchSummary = strings.TrimSpace(permission.Metadata.PatchSummary)
	rt.emitPatchPreview(result, selectedTool, input)

	decisionRequest := inferToolDecisionRequest(
		id,
		call.Function.Name,
		selectedTool,
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
	case tool.PermissionDeny, tool.PermissionNeedsAuth, tool.PermissionNeedsRuntime, tool.PermissionUnsupported, tool.PermissionNeedsMassMessageReview:
		execDecision = DefaultAutoToolPolicy{}.DecideExecution(toolCtx, intent, decisionRequest)
	default:
		execDecision = profile.DenyDecision(
			string(profile.DecisionSourcePolicyError),
			fmt.Sprintf("invalid permission decision %q", permission.Behavior),
			false,
		)
	}

	decision = executionDecisionToToolDecision(execDecision)
	// Preserve the normalized input for allow decisions that don't carry
	// an explicit UpdatedInput (e.g. the compatibility approval path).
	if decision.Action == ToolDecisionAllow && strings.TrimSpace(decision.UpdatedInput) == "" {
		decision.UpdatedInput = input
	}
	result.Decision = &execDecision

	decision = normalizeToolDecision(decision, firstNonEmpty(permission.Reason, "AUTO policy denied tool execution"))
	// Task 16: when the decision is a transform, record the original tool
	// name on the result BEFORE emitting the ToolPermissionDecided event so
	// the event carries TransformedFrom (e.g. "shell") alongside
	// TransformedTo (e.g. "apply_patch") and the UI can show the rewrite.
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
			result = synthesizeFailedToolResult(result, "ask_user_invalid_input", fmt.Sprintf("提问格式错误: %v", parseErr))
			result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
			rt.writeAudit(def, result, permission, string(decision.Source), "failed", parseErr.Error())
			return finalizeToolResult(result)
		}

		request := rt.userQuestionEvent(result, def, questions)
		awaiter, ok := rt.sink.(UserQuestionAwaiter)
		if !ok {
			rt.sink.Emit(request)
			result = synthesizeFailedToolResult(result, "ask_user_no_awaiter", "当前运行环境不支持用户提问")
			result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
			rt.writeAudit(def, result, permission, string(decision.Source), "failed", "no user question awaiter")
			return finalizeToolResult(result)
		}

		answer, err := awaiter.AwaitUserQuestion(ctx, request)
		if err != nil {
			result = synthesizeFailedToolResult(result, "ask_user_aborted", fmt.Sprintf("用户提问未完成: %v", err))
			result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
			rt.writeAudit(def, result, permission, string(decision.Source), "failed", err.Error())
			return finalizeToolResult(result)
		}

		result.Content = askuser.FormatAnswerTextResult(questions, answer)
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		rt.writeAudit(def, result, permission, string(decision.Source), "success", "")
		return finalizeToolResult(result)
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
		rt.emitRuntimePrepareStarted(result.ToolCallID, result.Name, "host runtime check before artifact tool")
		rt.emitRuntimeMissing(result, decision)
		result = synthesizeStructuredToolResult(result, decision, "needs_runtime", "缺少本地 runtime")
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		rt.writeAudit(def, result, permission, string(decision.Source), "needs_runtime", "")
		return finalizeToolResult(result)
	case ToolDecisionUnsupported:
		rt.emitToolProgress(result, "unsupported", firstNonEmpty(decision.Reason, "tool capability is unsupported"), 0)
		result = synthesizeStructuredToolResult(result, decision, "unsupported", "暂不支持该工具能力")
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		rt.writeAudit(def, result, permission, string(decision.Source), "unsupported", "")
		return finalizeToolResult(result)
	case ToolDecisionNeedsMassMessageReview:
		rt.emitToolProgress(result, "needs_mass_message_review", firstNonEmpty(decision.Reason, "mass message send requires human review"), 0)
		reviewRequest := rt.massMessageReviewEvent(result, def, permission)
		awaiter, ok := rt.sink.(MassMessageReviewAwaiter)
		if !ok {
			rt.sink.Emit(reviewRequest)
			result = synthesizeStructuredToolResult(result, decision, "needs_mass_message_review", "群发消息需要人工审核")
			result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
			rt.writeAudit(def, result, permission, string(decision.Source), "needs_mass_message_review", "")
			return finalizeToolResult(result)
		}
		reviewDecision, err := awaiter.AwaitMassMessageReview(ctx, reviewRequest)
		if err != nil {
			result = synthesizeStructuredToolResult(result, decision, "needs_mass_message_review", "群发消息审核未完成")
			result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
			rt.writeAudit(def, result, permission, string(decision.Source), "needs_mass_message_review", err.Error())
			return finalizeToolResult(result)
		}
		if reviewDecision != ReviewDecisionApproved {
			result = synthesizeStructuredToolResult(result, decision, "mass_message_review_rejected", "群发消息审核拒绝")
			result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
			rt.writeAudit(def, result, permission, string(decision.Source), "mass_message_review_rejected", "")
			return finalizeToolResult(result)
		}
		rt.emitToolProgress(result, "mass_message_review_approved", "mass message review approved", 0)
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
	if err != nil {
		rt.emitToolProgress(result, "failed", err.Error(), 0)
		failContent := output
		if strings.TrimSpace(failContent) == "" {
			failContent = fmt.Sprintf("工具执行失败: %v", err)
		}
		result = synthesizeFailedToolResult(result, "tool_execution_failed", failContent)
		applyExecutorOutcome(&result, execOut)
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		rt.writeAudit(def, result, permission, string(decision.Source), auditOutcomeLabel(result), err.Error())
		return finalizeToolResult(result)
	}

	result.Content = output
	applyExecutorOutcome(&result, execOut)
	result.ResultSummary = excerptString(strings.TrimSpace(output), 200)
	// TrashedPaths are sourced from the executor's ToolExecutionOutput when
	// the executor populates it, falling back to extractTrashedPaths(content)
	// for backwards compatibility (routeToExecutor handles the fallback).
	result.TrashedPaths = trashedPaths
	if result.IsError || (result.Outcome.Status != "" && result.Outcome.IsError()) {
		rt.emitToolProgress(result, "failed", firstNonEmpty(result.Outcome.ErrorMessage, result.ErrorCode, "tool failed"), 100)
	} else {
		rt.emitToolProgress(result, "completed", "", 100)
	}
	rt.writeAudit(def, result, permission, string(decision.Source), auditOutcomeLabel(result), "")
	rt.emitArtifacts(result, selectedTool, input, output)
	return finalizeToolResult(result)
}

func (rt *ToolRuntime) executeTool(
	ctx stdcontext.Context,
	result ToolExecutionResult,
	selectedTool tool.Tool,
	input string,
) (string, error) {
	if observableTool, ok := selectedTool.(tool.ObservableTool); ok {
		return observableTool.ExecuteWithObserver(ctx, input, toolObserverFunc(func(output tool.CommandOutput) {
			rt.emitCommandOutput(result, output)
		}))
	}
	return selectedTool.Execute(ctx, input)
}

// routeToExecutor dispatches a tool call through the unified executor layer
// (Task 11 of "零审批自动执行策略"). The Router selects the appropriate
// ToolExecutor based on the ExecutionIntent's ToolKind and the runtime's
// ExecutionProfile. The executor adds kind-specific enforcement (file-chain
// boundary, sandbox consultation, MCP risk annotations, guardian read-only,
// profile-driven sandbox/network/env/permission enforcement) before
// delegating to the tool's own Execute / ExecuteWithObserver.
//
// The observer is constructed here (mirroring executeTool) so streamed
// stdout/stderr deltas continue to flow to the event sink for ObservableTool
// implementations. The observer is then wrapped by visibilityFilterObserver
// so the profile's VisibilityPolicy is enforced:
//   - VisibilityHidden: deltas are dropped entirely.
//   - VisibilitySummary: real-time deltas are suppressed; after the tool
//     completes, a single truncated summary event is emitted in their place.
//   - VisibilityFull: the original observer is used (existing behavior).
//
// When the router is not configured (nil), the method falls back to the
// legacy executeTool path for backwards compatibility.
//
// Returns (content, trashedPaths, output, error) where content may be non-empty
// even when err is non-nil — this happens when the executor denies the call
// and surfaces a denial message. trashedPaths is sourced from the
// ToolExecutionOutput when the executor populates it, falling back to
// extractTrashedPaths(content) for backwards compatibility. Callers should
// use content when non-empty, otherwise fall back to formatting err.
func (rt *ToolRuntime) routeToExecutor(
	ctx stdcontext.Context,
	result ToolExecutionResult,
	intent profile.ExecutionIntent,
	selectedTool tool.Tool,
	input string,
) (string, []string, executors.ToolExecutionOutput, error) {
	if rt.router == nil {
		var observer tool.ToolObserver
		if _, ok := selectedTool.(tool.ObservableTool); ok {
			observer = toolObserverFunc(func(output tool.CommandOutput) {
				rt.emitCommandOutput(result, output)
			})
		}
		outcome, err := tool.ExecuteAsOutcome(ctx, selectedTool, input, observer)
		out := executors.ToolExecutionOutput{
			Outcome:   outcome,
			Content:   outcome.Content,
			IsError:   outcome.IsError(),
			ErrorCode: outcome.ErrorCode,
		}
		if err != nil {
			return out.Content, nil, out, err
		}
		return out.Content, extractTrashedPaths(out.Content), out, nil
	}
	executor, err := rt.router.Route(intent, rt.profile)
	if err != nil {
		return "", nil, executors.ToolExecutionOutput{}, err
	}
	var observer tool.ToolObserver
	if _, ok := selectedTool.(tool.ObservableTool); ok {
		observer = toolObserverFunc(func(output tool.CommandOutput) {
			rt.emitCommandOutput(result, output)
		})
	}
	observer = executors.VisibilityFilterObserver(observer, rt.profile.VisibilityPolicy)
	out, err := executor.Execute(ctx, intent, rt.profile, selectedTool, input, observer)
	if err != nil {
		return out.Content, out.TrashedPaths, out, err
	}
	// VisibilitySummary: when real-time deltas were suppressed, emit a
	// single truncated summary event so the user still sees that the tool
	// produced output. The summary is a short excerpt of the full content.
	if executors.SummaryHadOutput(observer) {
		summary := excerptString(out.Content, 500)
		if strings.TrimSpace(summary) != "" {
			rt.emitCommandOutput(result, tool.CommandOutput{
				Stream: "stdout",
				Delta:  summary,
			})
		}
	}
	trashedPaths := out.TrashedPaths
	if len(trashedPaths) == 0 {
		trashedPaths = extractTrashedPaths(out.Content)
	}
	return out.Content, trashedPaths, out, nil
}

// executeTransformedTool handles a ToolDecisionTransform by looking up the
// rewritten tool (e.g. apply_patch), normalizing its input, and executing it
// directly. The transform decision authorizes the rewritten call, so the
// auto policy is not re-evaluated for the transformed tool.
//
// One-level guard: if the current context already carries a transform depth
// (i.e. this call is itself the result of a transform), the nested transform
// is denied. This prevents transform-of-transform loops.
//
// Task 11: the transformed tool is routed through the executor layer
// (routeToExecutor) so the same executor-specific enforcement applies to
// transform targets as to direct calls.
func (rt *ToolRuntime) executeTransformedTool(
	ctx stdcontext.Context,
	result ToolExecutionResult,
	call convcontext.ToolCall,
	decision ToolDecision,
	permission tool.PermissionDecision,
	originalDef tool.ToolDefinition,
	originalIntent profile.ExecutionIntent,
) ToolExecutionResult {
	if transformDepthFromCtx(ctx) >= 1 {
		guarded := ToolDecision{
			Action: ToolDecisionDeny,
			Source: ToolDecisionSourcePolicyError,
			Reason: "transform chain depth exceeded; nested transform denied",
		}
		result = synthesizeDeniedToolResult(result, guarded)
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		rt.writeAudit(originalDef, result, permission, string(guarded.Source), "denied", "")
		return finalizeToolResult(result)
	}

	newToolName := strings.TrimSpace(decision.UpdatedToolName)
	if newToolName == "" {
		newToolName = call.Function.Name
	}

	transformedTool, err := tool.GetTool(newToolName)
	if err != nil {
		denied := ToolDecision{
			Action: ToolDecisionDeny,
			Source: ToolDecisionSourcePolicyError,
			Reason: fmt.Sprintf("transform target tool %q not found: %v", newToolName, err),
		}
		result = synthesizeDeniedToolResult(result, denied)
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		rt.writeAudit(originalDef, result, permission, string(denied.Source), "denied", "")
		return finalizeToolResult(result)
	}

	transformedInput, err := normalizeToolInputWithSchema(decision.UpdatedInput, transformedTool)
	if err != nil {
		denied := ToolDecision{
			Action: ToolDecisionDeny,
			Source: ToolDecisionSourcePolicyError,
			Reason: fmt.Sprintf("transform target input invalid: %v", err),
		}
		result = synthesizeDeniedToolResult(result, denied)
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		rt.writeAudit(originalDef, result, permission, string(denied.Source), "denied", "")
		return finalizeToolResult(result)
	}

	// Emit a transform progress event indicating the rewrite.
	rt.emitToolProgress(result, "transform",
		fmt.Sprintf("%s → %s: %s", call.Function.Name, newToolName, decision.Reason), 0)

	// Execute the transformed tool with a context flagged with transform depth
	// so any nested transform decision is denied by the guard above.
	transformedCtx := ctxWithTransformDepth(ctx, transformDepthFromCtx(ctx)+1)
	transformedDef := transformedTool.Definition()

	// Build an ExecutionIntent for the transformed tool so the Router can
	// select the appropriate executor. The transformed tool kind is inferred
	// from the transformed tool's name and annotations (e.g. apply_patch →
	// file_chain). Workspace roots and subagent origin are inherited from
	// the original intent.
	transformedCall := call
	transformedCall.Function.Name = newToolName
	transformedCall.Function.Arguments = decision.UpdatedInput
	transformedIntent := profile.BuildExecutionIntent(transformedCall, transformedTool, rt.permissionContext, originalIntent.SubagentOrigin)

	rt.emitHostActivity(result, transformedTool, transformedInput)
	output, trashedPaths, execOut, err := rt.routeToExecutor(transformedCtx, result, transformedIntent, transformedTool, transformedInput)
	if err != nil {
		rt.emitToolProgress(result, "failed", err.Error(), 0)
		failContent := output
		if strings.TrimSpace(failContent) == "" {
			failContent = fmt.Sprintf("工具执行失败: %v", err)
		}
		result = synthesizeFailedToolResult(result, "tool_execution_failed", failContent)
		applyExecutorOutcome(&result, execOut)
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		rt.writeAudit(transformedDef, result, permission, string(decision.Source), auditOutcomeLabel(result), err.Error())
		return finalizeToolResult(result)
	}

	result.Content = output
	applyExecutorOutcome(&result, execOut)
	result.ResultSummary = excerptString(strings.TrimSpace(output), 200)
	// TrashedPaths are sourced from the executor's ToolExecutionOutput when
	// the executor populates it, falling back to extractTrashedPaths(content)
	// for backwards compatibility (routeToExecutor handles the fallback).
	result.TrashedPaths = trashedPaths
	if result.IsError || (result.Outcome.Status != "" && result.Outcome.IsError()) {
		rt.emitToolProgress(result, "failed", firstNonEmpty(result.Outcome.ErrorMessage, result.ErrorCode, "tool failed"), 100)
	} else {
		rt.emitToolProgress(result, "completed", "", 100)
	}
	rt.writeAudit(transformedDef, result, permission, string(decision.Source), auditOutcomeLabel(result), "")
	rt.emitArtifacts(result, transformedTool, transformedInput, output)
	return finalizeToolResult(result)
}

func (rt *ToolRuntime) ExecuteToolCalls(ctx stdcontext.Context, calls []convcontext.ToolCall) []ToolExecutionResult {
	results := make([]ToolExecutionResult, 0, len(calls))
	for _, call := range calls {
		results = append(results, rt.Execute(ctx, call))
	}
	return results
}

func ExecuteToolCalls(ctx stdcontext.Context, calls []convcontext.ToolCall) []ToolExecutionResult {
	return NewToolRuntime(nil).ExecuteToolCalls(ctx, calls)
}

func (rt *ToolRuntime) decideToolPermission(
	ctx stdcontext.Context,
	selectedTool tool.Tool,
	input string,
) tool.PermissionDecision {
	if checker, ok := selectedTool.(tool.PermissionCheckingTool); ok {
		decision := checker.CheckPermission(ctx, input, rt.permissionContext)
		if decision.Behavior == "" {
			return tool.DenyPermission("tool returned an empty permission decision")
		}
		return decision
	}

	// Fall back to annotations-based decision
	def := selectedTool.Definition()
	return tool.DecideWithServerCheck(ctx, def, rt.serverConnected)
}

func (rt *ToolRuntime) emitArtifacts(
	result ToolExecutionResult,
	selectedTool tool.Tool,
	input string,
	output string,
) {
	producingTool, ok := selectedTool.(tool.ArtifactProducingTool)
	if !ok {
		return
	}

	for _, artifact := range producingTool.Artifacts(input, output, rt.permissionContext) {
		eventType := ArtifactUpdatedEventType
		if artifact.Action == tool.ArtifactActionCreated {
			eventType = ArtifactCreatedEventType
		}
		rt.sink.Emit(AwakenEvent{
			Type:                eventType,
			Timestamp:           timeNowUTC(),
			ToolCallID:          result.ToolCallID,
			ToolName:            result.Name,
			ArtifactID:          firstNonEmpty(strings.TrimSpace(artifact.ID), newAwakenID("artifact")),
			ArtifactTitle:       firstNonEmpty(strings.TrimSpace(artifact.Title), fallbackArtifactTitle(artifact.Path, result.Name)),
			ArtifactDescription: strings.TrimSpace(artifact.Description),
			ArtifactKind:        firstNonEmpty(strings.TrimSpace(artifact.Kind), "file"),
			ArtifactPath:        strings.TrimSpace(artifact.Path),
			ArtifactContent:     artifact.Content,
			ArtifactStatus:      normalizeArtifactStatus(artifact.Status),
		})
	}
}

func (rt *ToolRuntime) emitCommandOutput(result ToolExecutionResult, output tool.CommandOutput) {
	if output.Delta == "" {
		return
	}
	rt.sink.Emit(AwakenEvent{
		Type:          CommandOutputEventType,
		Timestamp:     timeNowUTC(),
		ToolCallID:    result.ToolCallID,
		ToolName:      result.Name,
		CommandStream: output.Stream,
		Delta:         output.Delta,
		Truncated:     output.Truncated,
	})
}

func (rt *ToolRuntime) emitToolProgress(result ToolExecutionResult, stage, message string, percentage int) {
	event := AwakenEvent{
		Type:                   ToolProgressEventType,
		Timestamp:              timeNowUTC(),
		ToolCallID:             result.ToolCallID,
		ToolName:               result.Name,
		ToolProgressStage:      stage,
		ToolProgressMessage:    message,
		ToolProgressPercentage: percentage,
		Progress:               message,
	}
	applyToolOutcomeFields(&event, result)
	rt.sink.Emit(event)
}

func (rt *ToolRuntime) emitHostActivity(result ToolExecutionResult, selectedTool tool.Tool, input string) {
	activity := tool.DefaultActivity(result.Name)
	if describing, ok := selectedTool.(tool.ActivityDescribingTool); ok {
		described := describing.DescribeActivity(input)
		if strings.TrimSpace(described.Title) != "" || strings.TrimSpace(described.Stage) != "" {
			activity = described
		}
	}
	stage := firstNonEmpty(strings.TrimSpace(activity.Stage), "started")
	message := firstNonEmpty(strings.TrimSpace(activity.Title), strings.TrimSpace(activity.Detail), "正在执行 "+result.Name)
	if detail := strings.TrimSpace(activity.Detail); detail != "" && detail != message {
		message = message + ": " + detail
	}
	rt.emitToolProgress(result, stage, message, 0)
}

func auditOutcomeLabel(result ToolExecutionResult) string {
	if result.Outcome.Status != "" {
		return string(result.Outcome.Status)
	}
	if result.IsError {
		return "failed"
	}
	if result.Status != "" {
		return string(result.Status)
	}
	return "succeeded"
}

func (rt *ToolRuntime) emitAuthRequired(result ToolExecutionResult, def tool.ToolDefinition, decision ToolDecision) {
	rt.sink.Emit(AwakenEvent{
		Type:               AuthRequiredEventType,
		Timestamp:          timeNowUTC(),
		ToolCallID:         result.ToolCallID,
		ToolName:           result.Name,
		Content:            decision.Reason,
		ToolDecisionAction: string(decision.Action),
		ToolDecisionSource: string(decision.Source),
		ToolDecisionReason: strings.TrimSpace(decision.Reason),
		ToolDefinition:     def,
		AuthProvider:       firstNonEmpty(def.Annotations.ServerName, def.Annotations.ToolName, string(def.Source)),
	})
}

func (rt *ToolRuntime) emitRuntimeMissing(result ToolExecutionResult, decision ToolDecision) {
	rt.sink.Emit(AwakenEvent{
		Type:               RuntimeMissingEventType,
		Timestamp:          timeNowUTC(),
		ToolCallID:         result.ToolCallID,
		ToolName:           result.Name,
		Content:            decision.Reason,
		ToolDecisionAction: string(decision.Action),
		ToolDecisionSource: string(decision.Source),
		ToolDecisionReason: strings.TrimSpace(decision.Reason),
		RuntimeName:        "primary-runtime",
	})
}

func (rt *ToolRuntime) emitRuntimePrepareStarted(toolCallID, toolName, message string) {
	rt.sink.Emit(AwakenEvent{
		Type:        RuntimePrepareStartedEventType,
		Timestamp:   timeNowUTC(),
		ToolCallID:  toolCallID,
		ToolName:    toolName,
		Content:     message,
		RuntimeName: "primary-runtime",
	})
}

func (rt *ToolRuntime) emitRuntimePrepareProgress(toolCallID, toolName, message, progress string) {
	rt.sink.Emit(AwakenEvent{
		Type:        RuntimePrepareProgressEventType,
		Timestamp:   timeNowUTC(),
		ToolCallID:  toolCallID,
		ToolName:    toolName,
		Content:     message,
		Progress:    progress,
		RuntimeName: "primary-runtime",
	})
}

func (rt *ToolRuntime) emitRuntimeReady(toolCallID, toolName, version, message string) {
	rt.sink.Emit(AwakenEvent{
		Type:           RuntimeReadyEventType,
		Timestamp:      timeNowUTC(),
		ToolCallID:     toolCallID,
		ToolName:       toolName,
		Content:        message,
		RuntimeName:    "primary-runtime",
		RuntimeVersion: version,
	})
}

func (rt *ToolRuntime) emitRuntimeRepairFailed(toolCallID, toolName, message string) {
	rt.sink.Emit(AwakenEvent{
		Type:        RuntimeRepairFailedEventType,
		Timestamp:   timeNowUTC(),
		ToolCallID:  toolCallID,
		ToolName:    toolName,
		Content:     message,
		Error:       message,
		RuntimeName: "primary-runtime",
	})
}

func (rt *ToolRuntime) massMessageReviewEvent(result ToolExecutionResult, def tool.ToolDefinition, permission tool.PermissionDecision) AwakenEvent {
	metadata := permission.Metadata.MassMessage
	if metadata == nil {
		metadata = def.Annotations.MassMessage
	}
	msg := tool.MassMessageMetadata{}
	if metadata != nil {
		msg = *metadata
	}
	if strings.TrimSpace(msg.ReviewID) == "" {
		msg.ReviewID = newAwakenID("mass_review")
	}
	if strings.TrimSpace(msg.SourceTool) == "" {
		msg.SourceTool = result.Name
	}
	return AwakenEvent{
		Type:                       MassMessageReviewRequestedType,
		Timestamp:                  timeNowUTC(),
		ToolCallID:                 result.ToolCallID,
		ToolName:                   result.Name,
		ToolDefinition:             def,
		ReviewID:                   msg.ReviewID,
		MassMessageChannel:         msg.Channel,
		MassMessageRecipientScope:  msg.RecipientScope,
		MassMessageRecipientCount:  msg.RecipientCount,
		MassMessageBodyPreview:     msg.BodyPreview,
		MassMessageTemplatePreview: msg.TemplatePreview,
		MassMessageAttachments:     append([]string(nil), msg.Attachments...),
		MassMessageSendingAccount:  msg.SendingAccount,
		MassMessageSourceTool:      msg.SourceTool,
	}
}

func (rt *ToolRuntime) userQuestionEvent(result ToolExecutionResult, def tool.ToolDefinition, questions []askuser.Question) AwakenEvent {
	questionID := newAwakenID("user_question")
	contentBytes, err := json.Marshal(map[string]any{
		"questions": questions,
	})
	if err != nil {
		contentBytes = []byte("{}")
	}
	return AwakenEvent{
		Type:           UserQuestionRequestedType,
		Timestamp:      timeNowUTC(),
		ToolCallID:     result.ToolCallID,
		ToolName:       result.Name,
		ToolDefinition: def,
		QuestionID:     questionID,
		Content:        string(contentBytes),
	}
}

func (rt *ToolRuntime) writeAudit(def tool.ToolDefinition, result ToolExecutionResult, permission tool.PermissionDecision, decision, outcome, errReason string) {
	started := result.StartedAt
	if started.IsZero() {
		started = timeNowUTC()
	}
	finished := result.FinishedAt
	if finished.IsZero() {
		finished = timeNowUTC()
	}
	origin := "system_policy"
	if permission.Behavior == tool.PermissionAllow {
		origin = "auto_allow"
	}
	rt.auditSink.Write(tool.ToolAuditRecord{
		ToolCallID:     result.ToolCallID,
		RunID:          rt.runID,
		ToolName:       result.Name,
		Source:         def.Source,
		Annotations:    def.Annotations,
		Decision:       decision,
		DecisionReason: permission.Reason,
		DecisionOrigin: origin,
		StartedAt:      started,
		CompletedAt:    finished,
		Outcome:        outcome,
		ResultSummary:  result.ResultSummary,
		ErrorReason:    errReason,
		// Task 16 unified observability: populate the structured fields from
		// the result so the audit log carries the full profile / intent /
		// decision / transform / trash / subagent / guardian picture. The
		// legacy Decision/DecisionReason fields above remain for
		// backwards-compatible consumers. Note: the `decision` parameter
		// passed in from call sites is the decision SOURCE string (e.g.
		// "tool_rule"), NOT the action; DecisionAction must be derived from
		// the unified ExecutionDecision on the result so it carries
		// "allow"/"deny"/"transform" as the spec requires.
		ProfileName:       result.ProfileName,
		Intent:            result.Intent,
		DecisionAction:    decisionAction(result),
		DecisionSource:    string(result.DecisionSource),
		TransformedFrom:   result.TransformedFrom,
		TransformedTo:     transformedToName(result),
		TrashedPaths:      append([]string(nil), result.TrashedPaths...),
		SubagentOrigin:    rt.subagentOrigin,
		GuardianViolation: result.GuardianViolation,
	})
}

// decisionAction returns the unified decision action ("allow"/"deny"/
// "transform") from the result's ExecutionDecision. It returns "" when the
// unified decision is absent (e.g. unknown-tool failures that bypass the
// decision layer).
func decisionAction(result ToolExecutionResult) string {
	if result.Decision == nil {
		return ""
	}
	return string(result.Decision.Action)
}

// transformedToName returns the rewritten tool name when the result is the
// target of a transform decision, or "" otherwise. It inspects the
// ExecutionDecision carried on the result: for transform decisions, the
// decision's UpdatedToolName is the rewritten tool (e.g. "apply_patch").
func transformedToName(result ToolExecutionResult) string {
	if result.Decision == nil {
		return ""
	}
	if result.Decision.Action != profile.DecisionTransform {
		return ""
	}
	return result.Decision.UpdatedToolName
}

// extractTrashedPaths parses the apply_patch tool's summary output and
// returns the trashed-path component of every deletion line. The apply_patch
// tool encodes deletions as either:
//
//	D path (已移到回收站: trashedPath)
//	D path (已移到回收站)
//
// In the first form the trashed-path component is returned verbatim. In the
// second form (no explicit trashed path was surfaced) the line contributes
// nothing — the file was trashed but the OS did not report a destination.
// Lines that do not match the deletion pattern are ignored.
//
// This is a best-effort parser keyed off the Chinese marker the apply_patch
// tool emits today. If the marker changes, this parser must be updated in
// lockstep.
func extractTrashedPaths(output string) []string {
	if strings.TrimSpace(output) == "" {
		return nil
	}
	const trashMarker = "已移到回收站"
	var paths []string
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "D ") {
			continue
		}
		idx := strings.Index(trimmed, trashMarker)
		if idx < 0 {
			continue
		}
		tail := trimmed[idx+len(trashMarker):]
		// Expect either ": trashedPath)" or ")".
		tail = strings.TrimPrefix(tail, ":")
		tail = strings.TrimSpace(tail)
		tail = strings.TrimSuffix(tail, ")")
		tail = strings.TrimSpace(tail)
		if tail == "" {
			continue
		}
		paths = append(paths, tail)
	}
	return paths
}

func (rt *ToolRuntime) emitPatchPreview(result ToolExecutionResult, selectedTool tool.Tool, input string) {
	previewingTool, ok := selectedTool.(tool.PatchPreviewingTool)
	if !ok {
		return
	}
	preview, ok := previewingTool.PatchPreview(input, rt.permissionContext)
	if !ok {
		return
	}
	limited := outputlimit.Limit(preview.Patch, maxPatchPreviewChars)
	rt.sink.Emit(AwakenEvent{
		Type:          PatchUpdatedEventType,
		Timestamp:     timeNowUTC(),
		ToolCallID:    result.ToolCallID,
		ToolName:      result.Name,
		Patch:         limited.Content,
		PatchSummary:  strings.TrimSpace(preview.Summary),
		AffectedPaths: preview.AffectedPaths,
		Truncated:     preview.Truncated || limited.Truncated,
	})
}

func (rt *ToolRuntime) emitToolPermissionDecided(
	result ToolExecutionResult,
	permission tool.PermissionDecision,
	decision ToolDecision,
) {
	rt.sink.Emit(AwakenEvent{
		Type:                     ToolPermissionDecidedEventType,
		Timestamp:                timeNowUTC(),
		ToolCallID:               result.ToolCallID,
		ToolName:                 result.Name,
		Content:                  decision.Reason,
		StaticPermissionBehavior: string(permission.Behavior),
		StaticPermissionReason:   strings.TrimSpace(permission.Reason),
		ToolDecisionAction:       string(decision.Action),
		ToolDecisionSource:       string(decision.Source),
		ToolDecisionReason:       strings.TrimSpace(decision.Reason),
		AffectedPaths:            result.AffectedPaths,
		PatchSummary:             result.PatchSummary,
		// Task 16 unified observability: surface the profile name and the
		// transform origin (original tool name + rewritten tool name) so the
		// Desktop UI can render "Profile: workspace_write" and
		// "shell → apply_patch" alongside the decision. TrashedPaths and
		// GuardianViolation are populated on the result by the executor /
		// guardian check and are mirrored here for the audit trail.
		ProfileName:       result.ProfileName,
		TransformedFrom:   result.TransformedFrom,
		TransformedTo:     decision.UpdatedToolName,
		TrashedPaths:      append([]string(nil), result.TrashedPaths...),
		GuardianViolation: result.GuardianViolation,
	})
}

type toolObserverFunc func(tool.CommandOutput)

func (fn toolObserverFunc) OnCommandOutput(output tool.CommandOutput) {
	fn(output)
}

func finalizeToolResult(result ToolExecutionResult) ToolExecutionResult {
	if result.FinishedAt.IsZero() {
		result.FinishedAt = timeNowUTC()
	}
	if !result.StartedAt.IsZero() {
		result.DurationMs = result.FinishedAt.Sub(result.StartedAt).Milliseconds()
	}
	if result.Outcome.Status != "" {
		result.IsError = result.Outcome.IsError()
		result.Status = ToolExecutionStatus(result.Outcome.Status)
		if result.ErrorCode == "" {
			result.ErrorCode = result.Outcome.ErrorCode
		}
		if result.Outcome.DurationMs <= 0 && result.DurationMs > 0 {
			result.Outcome.DurationMs = result.DurationMs
		}
		if result.Content == "" {
			result.Content = result.Outcome.Content
		}
	} else if result.Status == "" {
		if result.IsError {
			result.Status = ToolExecutionFailed
		} else {
			result.Status = ToolExecutionSucceeded
		}
	}
	limited := outputlimit.Limit(result.Content, outputlimit.DefaultToolResultMaxChars)
	result.Content = limited.Content
	result.Truncated = limited.Truncated
	return result
}

// applyExecutorOutcome copies structured Outcome fields from an executor
// output onto the ToolExecutionResult when present.
func applyExecutorOutcome(result *ToolExecutionResult, out executors.ToolExecutionOutput) {
	if out.Outcome.Status == "" {
		if out.IsError {
			result.IsError = true
		}
		if out.ErrorCode != "" && result.ErrorCode == "" {
			result.ErrorCode = out.ErrorCode
		}
		return
	}
	result.Outcome = out.Outcome
	result.IsError = out.Outcome.IsError()
	result.Status = ToolExecutionStatus(out.Outcome.Status)
	if out.Outcome.ErrorCode != "" {
		result.ErrorCode = out.Outcome.ErrorCode
	} else if out.ErrorCode != "" {
		result.ErrorCode = out.ErrorCode
	}
	if len(out.AffectedPaths) > 0 && len(result.AffectedPaths) == 0 {
		result.AffectedPaths = append([]string(nil), out.AffectedPaths...)
	}
	if out.PatchSummary != "" && result.PatchSummary == "" {
		result.PatchSummary = out.PatchSummary
	}
}

func synthesizeDeniedToolResult(result ToolExecutionResult, decision ToolDecision) ToolExecutionResult {
	result.Content = fmt.Sprintf("工具执行被 AUTO policy 拒绝: %s", firstNonEmpty(decision.Reason, "permission policy denied execution"))
	result.IsError = true
	result.Status = ToolExecutionDenied
	result.ErrorCode = "tool_denied"
	result.DecisionSource = decision.Source
	result.DecisionReason = decision.Reason
	result.Outcome = tool.DeniedOutcome(result.Content, "tool_denied")
	return result
}

// executionDecisionToToolDecision maps the unified ExecutionDecision back to
// the legacy ToolDecision so the existing runtime switch (allow/deny/abort/
// transform) continues to work. For transform decisions, the map-typed
// UpdatedInput is serialised back to a JSON string.
func executionDecisionToToolDecision(ed profile.ExecutionDecision) ToolDecision {
	td := ToolDecision{
		Source:    ToolDecisionSource(ed.Source),
		Reason:    ed.Reason,
		Retryable: ed.Retryable,
	}
	switch ed.Action {
	case profile.DecisionAllow:
		td.Action = ToolDecisionAllow
	case profile.DecisionDeny:
		td.Action = ToolDecisionDeny
	case profile.DecisionTransform:
		td.Action = ToolDecisionTransform
		td.UpdatedToolName = ed.UpdatedToolName
		if ed.UpdatedInput != nil {
			data, err := json.Marshal(ed.UpdatedInput)
			if err == nil {
				td.UpdatedInput = string(data)
			}
		}
	case profile.DecisionNeedsAuth:
		td.Action = ToolDecisionNeedsAuth
		td.Source = ToolDecisionSourceAuthRequired
	case profile.DecisionNeedsRuntime:
		td.Action = ToolDecisionNeedsRuntime
		td.Source = ToolDecisionSourceRuntimeRequired
	case profile.DecisionUnsupported:
		td.Action = ToolDecisionUnsupported
	case profile.DecisionNeedsMassMessageReview:
		td.Action = ToolDecisionNeedsMassMessageReview
		td.Source = ToolDecisionSourceMassMessageReview
	default:
		td.Action = ToolDecisionDeny
		td.Source = ToolDecisionSourcePolicyError
		td.Reason = firstNonEmpty(strings.TrimSpace(ed.Reason), "invalid execution decision action")
	}
	return td
}

func synthesizeAbortedToolResult(result ToolExecutionResult, decision ToolDecision) ToolExecutionResult {
	result.Content = firstNonEmpty(decision.Reason, "工具执行已被取消")
	if !strings.Contains(result.Content, "工具执行已被取消") {
		result.Content = "工具执行已被取消: " + result.Content
	}
	result.IsError = true
	result.Status = ToolExecutionCancelled
	result.ErrorCode = "tool_policy_abort"
	result.DecisionSource = decision.Source
	result.DecisionReason = decision.Reason
	now := timeNowUTC()
	result.Outcome = tool.ExecutionOutcome{
		Content:         result.Content,
		Status:          tool.OutcomeCancelled,
		TransportStatus: tool.TransportCancelled,
		SemanticStatus:  tool.SemanticUnknown,
		ProcessStatus:   tool.ProcessNotApplicable,
		ErrorCode:       "tool_policy_abort",
		ErrorCategory:   tool.ErrorCategoryCancelled,
		ErrorMessage:    result.Content,
		Retryable:       false,
		StartedAt:       now,
		FinishedAt:      now,
	}
	return result
}

func synthesizeValidationToolResult(result ToolExecutionResult, reason string) ToolExecutionResult {
	result.Content = fmt.Sprintf("工具参数错误: %s", firstNonEmpty(reason, "invalid tool arguments"))
	result.IsError = true
	result.Status = ToolExecutionDenied
	result.ErrorCode = "validation_error"
	outcome := tool.FailedOutcome(result.Content, tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false)
	outcome.Status = tool.OutcomeFailed
	outcome.TransportStatus = tool.TransportNotStarted
	result.Outcome = outcome
	return result
}

func synthesizeFailedToolResult(result ToolExecutionResult, code string, content string) ToolExecutionResult {
	result.Content = content
	result.IsError = true
	result.Status = ToolExecutionFailed
	result.ErrorCode = code
	if result.Outcome.Status == "" {
		result.Outcome = tool.FailedOutcome(content, code, tool.ErrorCategoryTool, true)
	}
	return result
}

func synthesizeStructuredToolResult(result ToolExecutionResult, decision ToolDecision, code string, prefix string) ToolExecutionResult {
	reason := firstNonEmpty(strings.TrimSpace(decision.Reason), "tool execution blocked")
	result.Content = fmt.Sprintf("%s: %s", prefix, reason)
	result.IsError = true
	result.Status = ToolExecutionDenied
	result.ErrorCode = code
	result.DecisionSource = decision.Source
	result.DecisionReason = reason
	now := timeNowUTC()
	status := tool.OutcomeDenied
	category := tool.ErrorCategoryPolicy
	transport := tool.TransportNotStarted
	switch code {
	case "needs_auth":
		status = tool.OutcomeNeedsAuth
		category = tool.ErrorCategoryAuthorization
	case "needs_runtime":
		status = tool.OutcomeNeedsRuntime
		category = tool.ErrorCategoryRuntime
	case "unsupported":
		status = tool.OutcomeUnsupported
		category = tool.ErrorCategoryUnsupported
	case "needs_mass_message_review", "mass_message_review_rejected":
		status = tool.OutcomeReviewRequired
		category = tool.ErrorCategoryPolicy
	}
	result.Outcome = tool.ExecutionOutcome{
		Content:         result.Content,
		Status:          status,
		TransportStatus: transport,
		SemanticStatus:  tool.SemanticUnknown,
		ProcessStatus:   tool.ProcessNotApplicable,
		ErrorCode:       code,
		ErrorCategory:   category,
		ErrorMessage:    result.Content,
		Retryable:       false,
		StartedAt:       now,
		FinishedAt:      now,
	}
	return result
}

func normalizeArtifactStatus(status string) ArtifactStatus {
	switch ArtifactStatus(strings.TrimSpace(status)) {
	case ArtifactStatusPending, ArtifactStatusError:
		return ArtifactStatus(strings.TrimSpace(status))
	default:
		return ArtifactStatusReady
	}
}

func fallbackArtifactTitle(path string, toolName string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return fmt.Sprintf("%s artifact", toolName)
	}
	return filepath.Base(trimmed)
}

func excerptString(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max-1] + "…"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func timeNowUTC() time.Time {
	return time.Now().UTC()
}

func normalizeToolInputWithSchema(arguments string, selectedTool tool.Tool) (string, error) {
	schema := tool.ResolveInputSchema(selectedTool)
	params := selectedTool.Parameters()
	if len(schema.Properties) > 0 || len(schema.Required) > 0 || len(schema.OneOf) > 0 {
		if err := tool.ValidateInputAgainstSchema(arguments, schema); err != nil {
			return "", err
		}
	}
	return normalizeToolInput(arguments, params)
}

func normalizeToolInput(arguments string, params []tool.ToolParameter) (string, error) {
	trimmed := strings.TrimSpace(arguments)
	if len(params) == 0 {
		if trimmed == "" {
			return "", nil
		}
		if !json.Valid([]byte(trimmed)) {
			return "", fmt.Errorf("arguments must be valid JSON")
		}
		return trimmed, nil
	}
	if trimmed == "" {
		trimmed = "{}"
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return "", fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	if decoded == nil {
		return "", fmt.Errorf("arguments must be a JSON object")
	}

	for _, param := range params {
		if param.Required {
			if _, ok := decoded[param.Name]; !ok {
				return "", fmt.Errorf("missing required parameter %q", param.Name)
			}
		}
	}

	if len(params) == 1 {
		value, ok := decoded[params[0].Name]
		if !ok {
			return "", nil
		}
		if str, ok := value.(string); ok {
			return str, nil
		}
		data, err := json.Marshal(value)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	data, err := json.Marshal(decoded)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func argumentsForXMLFallback(toolName, input string) string {
	selectedTool, err := tool.GetTool(toolName)
	if err != nil {
		return input
	}

	params := selectedTool.Parameters()
	paramName := singleRequiredStringParam(params)
	if paramName == "" {
		return input
	}

	data, err := json.Marshal(map[string]interface{}{
		paramName: input,
	})
	if err != nil {
		return input
	}
	return string(data)
}

func singleRequiredStringParam(params []tool.ToolParameter) string {
	requiredName := ""
	for _, param := range params {
		if !param.Required {
			continue
		}
		if param.Type != "string" {
			return ""
		}
		if requiredName != "" {
			return ""
		}
		requiredName = param.Name
	}
	return requiredName
}
