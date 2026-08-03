package agentcore

import (
	stdcontext "context"
	"fmt"
	"strings"

	"awaken/src/agentcore/executors"
	convcontext "awaken/src/context"
	"awaken/src/tool"
	"awaken/src/tool/profile"
)

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
// ToolExecutionOutput when the executor populates it. Callers should use
// content when non-empty, otherwise fall back to formatting err.
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
		trashedPaths := out.Outcome.ObservedTrashedPaths()
		if err != nil {
			return out.Content, trashedPaths, out, err
		}
		return out.Content, trashedPaths, out, nil
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
	trashedPaths := out.TrashedPaths
	if len(trashedPaths) == 0 {
		trashedPaths = out.Outcome.ObservedTrashedPaths()
	}
	if err != nil {
		return out.Content, trashedPaths, out, err
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
	return out.Content, trashedPaths, out, nil
}

// executeTransformedTool handles a ToolDecisionTransform by looking up the
// rewritten tool, normalizing its input, and executing it
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
	result.outcomeContext.OperationType = newToolName

	var (
		transformedTool    tool.Tool
		transformedDef     tool.ToolDefinition
		transformedQuality tool.DefinitionQuality
		preserveInput      bool
		provider           tool.ProviderManifest
		readiness          tool.ProviderReadinessReport
		lookupErr          error
	)
	if rt.toolSnapshot != nil {
		entry, err := rt.toolSnapshot.Lookup(newToolName)
		lookupErr = err
		if err == nil {
			transformedTool = entry.Runtime
			transformedDef = entry.Definition
			transformedQuality = entry.Quality
			preserveInput = entry.PreserveInputObject
			provider = entry.Provider
			readiness = entry.Readiness
		}
	} else {
		registered, err := tool.GetRegisteredTool(newToolName)
		lookupErr = err
		if err == nil {
			transformedTool = registered.Runtime
			transformedDef = registered.Definition
			transformedQuality = registered.Quality
			preserveInput = registered.PreserveInputObject
			provider = registered.Provider
			readiness = tool.ProviderReadinessReport{Readiness: provider.Readiness}
		}
	}
	if lookupErr != nil || transformedTool == nil {
		denied := ToolDecision{
			Action: ToolDecisionDeny,
			Source: ToolDecisionSourcePolicyError,
			Reason: fmt.Sprintf("transform target tool %q not found in pinned run snapshot: %v", newToolName, lookupErr),
		}
		result = synthesizeDeniedToolResult(result, denied)
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		rt.writeAudit(originalDef, result, permission, string(denied.Source), "denied", "")
		return finalizeToolResult(result)
	}
	result.outcomeContext.DeclaredSideEffects = append([]tool.ToolSideEffect(nil), transformedDef.SideEffects...)
	result.outcomeContext.OutputSchema = transformedDef.OutputSchema
	result.outcomeContext.StrictOutputSchema = transformedQuality == tool.DefinitionQualityStrict
	result.ProviderID = provider.ProviderID
	result.ProviderSource = provider.Source
	result.ProviderCapabilities = append([]tool.Capability(nil), provider.Capabilities...)
	result.ProviderReadiness = readiness.Readiness
	result.ProviderDependencies = append([]tool.DependencyRequirement(nil), provider.Dependencies...)
	result.DependencyObservations = append([]tool.DependencyObservation(nil), readiness.Dependencies...)
	if readiness.Readiness == tool.ProviderNeedsAuth || readiness.Readiness == tool.ProviderNeedsRuntime || readiness.Readiness == tool.ProviderUnavailable {
		return finalizeToolResult(rt.providerReadinessResult(result, provider, readiness))
	}

	transformedInput, err := normalizeToolInputWithSchema(decision.UpdatedInput, transformedTool, transformedDef.InputSchema, preserveInput)
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
	// Build an ExecutionIntent for the transformed tool so the Router can
	// select the appropriate executor. The transformed tool kind is inferred
	// from the transformed tool's name and annotations. Workspace roots and
	// subagent origin are inherited from
	// the original intent.
	transformedCall := call
	transformedCall.Function.Name = newToolName
	transformedCall.Function.Arguments = decision.UpdatedInput
	transformedIntent := profile.BuildExecutionIntentWithDefinition(transformedCall, transformedTool, transformedDef, rt.permissionContext, originalIntent.SubagentOrigin)

	rt.emitHostActivity(result, transformedTool, transformedInput)
	output, trashedPaths, execOut, err := rt.routeToExecutor(transformedCtx, result, transformedIntent, transformedTool, transformedInput)
	result.TrashedPaths = append([]string(nil), trashedPaths...)
	if err != nil {
		failContent := output
		if strings.TrimSpace(failContent) == "" {
			failContent = fmt.Sprintf("工具执行失败: %v", err)
		}
		applyExecutorOutcome(&result, execOut)
		result = synthesizeExecutionErrorToolResult(result, "tool_execution_failed", failContent, err.Error())
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		return rt.finalizeExecutedToolResult(result, transformedDef, permission, string(decision.Source), transformedTool, transformedInput, output)
	}

	result.Content = output
	applyExecutorOutcome(&result, execOut)
	result.ResultSummary = excerptString(strings.TrimSpace(output), 200)
	// TrashedPaths are sourced from structured executor outcomes.
	result.TrashedPaths = trashedPaths
	return rt.finalizeExecutedToolResult(result, transformedDef, permission, string(decision.Source), transformedTool, transformedInput, output)
}

// finalizeExecutedToolResult is the single terminal gate for both direct and
// transformed execution. Canonical normalization (including strict output
// schema verification) must happen before any completed event, artifact
// publication, or success audit is externally observable.
func (rt *ToolRuntime) finalizeExecutedToolResult(
	result ToolExecutionResult,
	definition tool.ToolDefinition,
	permission tool.PermissionDecision,
	decisionSource string,
	selectedTool tool.Tool,
	input string,
	output string,
) ToolExecutionResult {
	if len(result.Outcome.ModelContent) > 0 && rt.modelContentStore != nil {
		identity := result.executionIdentity
		persisted, err := rt.modelContentStore.PersistToolResult(
			stdcontext.Background(),
			rt.threadID,
			identity,
			result.Outcome.ModelContent,
		)
		if err != nil {
			result = synthesizeExecutionErrorToolResult(
				result,
				"tool_media_persistence_failed",
				"Tool media could not be persisted safely.",
				err.Error(),
			)
		} else {
			result.Outcome.ModelContent = persisted
		}
	}
	result = finalizeToolResult(result)
	if result.IsError || result.Outcome.IsError() {
		// A typed partial outcome may contain files that were already verified
		// and durably written before a later sibling result failed. Publish only
		// those ready artifacts when the provider transport itself completed;
		// transport/implementation failures and schema failures remain fail-closed.
		if publishablePartialArtifacts(result.Outcome) {
			result.Outcome.Artifacts = rt.emitArtifactDescriptors(result, result.Outcome.Artifacts, nil)
			result = finalizeToolResult(result)
		}
		rt.emitToolProgress(result, "failed", firstNonEmpty(result.Outcome.ErrorMessage, result.ErrorCode, "tool failed"), 100)
		rt.writeAudit(definition, result, permission, decisionSource, auditOutcomeLabel(result), terminalAuditError(result))
		return result
	}

	// Output verification has succeeded. Only now may the runtime publish a
	// completion signal or ask the tool to publish user-facing artifacts.
	rt.emitToolProgress(result, "completed", "", 100)
	seenArtifacts := map[string]bool{}
	outcomeArtifacts := rt.emitArtifactDescriptors(result, result.Outcome.Artifacts, seenArtifacts)
	producedArtifacts := rt.emitArtifacts(result, selectedTool, input, output, seenArtifacts)
	outcomeArtifacts = append(outcomeArtifacts, producedArtifacts...)
	result.Outcome.Artifacts = outcomeArtifacts
	result = finalizeToolResult(result)
	rt.writeAudit(definition, result, permission, decisionSource, auditOutcomeLabel(result), "")
	return result
}

func publishablePartialArtifacts(outcome tool.ExecutionOutcome) bool {
	if outcome.TransportStatus != tool.TransportCompleted || outcome.Operation == nil ||
		outcome.Operation.SideEffectState != tool.OutcomeSideEffectPartial || len(outcome.Artifacts) == 0 {
		return false
	}
	for _, artifact := range outcome.Artifacts {
		if normalizeArtifactStatus(artifact.Status) != ArtifactStatusReady || strings.TrimSpace(artifact.Path) == "" {
			return false
		}
	}
	return true
}

func terminalAuditError(result ToolExecutionResult) string {
	if !result.IsError && !result.Outcome.IsError() {
		return ""
	}
	return firstNonEmpty(strings.TrimSpace(result.Outcome.ErrorMessage), strings.TrimSpace(result.Content), result.ErrorCode)
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

type toolObserverFunc func(tool.CommandOutput)

func (fn toolObserverFunc) OnCommandOutput(output tool.CommandOutput) {
	fn(output)
}
