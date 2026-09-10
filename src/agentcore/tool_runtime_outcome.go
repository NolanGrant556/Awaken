package agentcore

import (
	stdcontext "context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"awaken/src/agentcore/executors"
	"awaken/src/internal/outputlimit"
	"awaken/src/tool"
	"awaken/src/tool/profile"
)

func finalizeToolResult(result ToolExecutionResult) ToolExecutionResult {
	if result.FinishedAt.IsZero() {
		result.FinishedAt = timeNowUTC()
	}
	if !result.StartedAt.IsZero() {
		result.DurationMs = result.FinishedAt.Sub(result.StartedAt).Milliseconds()
	}
	if result.Outcome.Status == "" {
		result.Outcome = outcomeFromCompatibilityResult(result)
	}
	if result.Outcome.StartedAt.IsZero() {
		result.Outcome.StartedAt = result.StartedAt
	}
	if result.Outcome.FinishedAt.IsZero() {
		result.Outcome.FinishedAt = result.FinishedAt
	}
	if result.Outcome.DurationMs <= 0 && result.DurationMs > 0 {
		result.Outcome.DurationMs = result.DurationMs
	}
	if result.Outcome.Diagnostic == nil {
		result.Outcome.Diagnostic = map[string]any{}
	}
	if strings.TrimSpace(result.ToolSnapshotID) != "" {
		result.Outcome.Diagnostic["tool_snapshot_id"] = result.ToolSnapshotID
	}
	if strings.TrimSpace(result.EnvironmentSnapshotID) != "" {
		result.Outcome.Diagnostic["environment_snapshot_id"] = result.EnvironmentSnapshotID
	}
	if result.ProviderID != "" {
		result.Outcome.Diagnostic["provider_id"] = result.ProviderID
	}
	if result.ProviderSource != "" {
		result.Outcome.Diagnostic["provider_source"] = result.ProviderSource
	}
	if len(result.ProviderCapabilities) > 0 {
		result.Outcome.Diagnostic["capabilities"] = append([]tool.Capability(nil), result.ProviderCapabilities...)
	}
	if result.ProviderReadiness != "" {
		result.Outcome.Diagnostic["provider_readiness"] = result.ProviderReadiness
	}
	if len(result.ProviderDependencies) > 0 {
		result.Outcome.Diagnostic["dependency_requirements"] = append([]tool.DependencyRequirement(nil), result.ProviderDependencies...)
	}
	if len(result.DependencyObservations) > 0 {
		result.Outcome.Diagnostic["dependency_observations"] = append([]tool.DependencyObservation(nil), result.DependencyObservations...)
	}

	canonicalContent := result.Outcome.Content
	if canonicalContent == "" {
		canonicalContent = result.Content
	}
	if result.fullContent == "" && len(canonicalContent) > toolResultOffloadThresholdBytes {
		result.fullContent = canonicalContent
	}
	limited := outputlimit.Limit(canonicalContent, outputlimit.DefaultToolResultMaxChars)
	result.Content = limited.Content
	result.Truncated = result.Truncated || result.Outcome.Truncated || limited.Truncated
	result.Outcome.Content = limited.Content
	result.Outcome.Truncated = result.Truncated

	result.outcomeContext.ReferencedPaths = mergeToolResultPaths(
		result.outcomeContext.ReferencedPaths,
		result.AffectedPaths,
		result.Outcome.AffectedPaths,
	)
	result.outcomeContext.ActualAffectedPaths = mergeToolResultPaths(
		result.outcomeContext.ActualAffectedPaths,
		result.ActualAffectedPaths,
		observedChangeResources(result.Outcome.Changes),
	)
	if result.Outcome.Status == tool.OutcomeSucceeded {
		result.outcomeContext.ActualAffectedPaths = mergeToolResultPaths(
			result.outcomeContext.ActualAffectedPaths,
			result.Outcome.AffectedPaths,
		)
	}
	if len(result.Outcome.ObservedTrashedPaths()) == 0 {
		for index, path := range result.TrashedPaths {
			original := ""
			if result.Outcome.Operation != nil {
				original = strings.TrimSpace(result.Outcome.Operation.Target)
			}
			if original == "" && index < len(result.Outcome.AffectedPaths) {
				original = strings.TrimSpace(result.Outcome.AffectedPaths[index])
			}
			if original == "" && index < len(result.AffectedPaths) {
				original = strings.TrimSpace(result.AffectedPaths[index])
			}
			result.outcomeContext.Changes = append(result.outcomeContext.Changes, tool.ResourceChange{
				Kind: "file", Action: "trashed", Resource: original,
				Summary:   "Resource moved to the operating-system recycle bin.",
				Certainty: tool.ChangeInferred, Reversibility: tool.ChangeConditional,
				Details: map[string]any{"trashed_path": path},
			})
		}
	}
	result.Outcome = result.Outcome.Normalize(result.outcomeContext)
	result.ActualAffectedPaths = mergeToolResultPaths(result.ActualAffectedPaths, result.outcomeContext.ActualAffectedPaths)
	result.AffectedPaths = mergeToolResultPaths(result.AffectedPaths, result.ActualAffectedPaths)
	result.IsError = result.Outcome.IsError()
	result.Status = ToolExecutionStatus(result.Outcome.Status)
	result.ErrorCode = result.Outcome.ErrorCode
	result.DurationMs = result.Outcome.DurationMs
	if strings.TrimSpace(result.ResultSummary) == "" {
		result.ResultSummary = result.Outcome.Summary
	}
	return result
}

func (rt *ToolRuntime) providerReadinessResult(
	result ToolExecutionResult,
	provider tool.ProviderManifest,
	readiness tool.ProviderReadinessReport,
) ToolExecutionResult {
	status := tool.OutcomeNeedsRuntime
	errorCode := tool.ErrorCodeNeedsRuntime
	category := tool.ErrorCategoryRuntime
	retryStrategy := tool.RetryPrepareRuntime
	recovery := []string{"inspect_dependency_observations", "prepare_runtime_or_switch_provider"}
	nextActions := []tool.NextAction{
		{ID: "inspect_dependencies", Description: "Inspect the dependency observations and the user-machine environment snapshot."},
		{ID: "switch_provider", Description: "Select another ready provider for the same capability.", Preconditions: []string{"Do not replay an unavailable provider unchanged."}},
	}
	switch readiness.Readiness {
	case tool.ProviderNeedsAuth:
		status = tool.OutcomeNeedsAuth
		errorCode = tool.ErrorCodeNeedsAuth
		category = tool.ErrorCategoryAuthorization
		retryStrategy = tool.RetryRequestAuthorization
		recovery = []string{"authenticate_provider", "switch_provider"}
		nextActions[0] = tool.NextAction{ID: "authenticate_provider", Description: "Authenticate the selected provider, then explicitly refresh run availability."}
	case tool.ProviderUnavailable:
		status = tool.OutcomeUnsupported
		errorCode = tool.ErrorCodeUnsupported
		category = tool.ErrorCategoryUnsupported
		retryStrategy = tool.RetrySwitchTool
		recovery = []string{"switch_provider", "refresh_tool_catalog_after_provider_recovery"}
		nextActions = []tool.NextAction{
			{ID: "switch_provider", Description: "Select another ready provider for the same capability."},
			{ID: "refresh_catalog", Description: "After recovering the provider, explicitly start or refresh the run snapshot."},
		}
	}

	packageManagers := []string(nil)
	if rt != nil && rt.environment != nil {
		packageManagers = append(packageManagers, rt.environment.AvailablePackageManagers...)
	}
	if readiness.Readiness == tool.ProviderNeedsRuntime {
		seenPlans := map[string]bool{}
		installActions := make([]tool.NextAction, 0)
		for _, observation := range readiness.Dependencies {
			for _, plan := range observation.CandidateInstallPlans {
				if strings.TrimSpace(plan.PlanID) == "" || seenPlans[plan.PlanID] {
					continue
				}
				seenPlans[plan.PlanID] = true
				installActions = append(installActions, tool.NextAction{
					ID:          "install_dependency_" + plan.PlanID,
					Description: "Execute this exact host-generated dependency plan through the typed staged installer.",
					Tool:        "dependency_install",
					Preconditions: []string{
						"Use only the plan_id from this pinned run snapshot.",
						"Do not translate the plan into Shell, command, argv, package, URL, or install-directory input.",
					},
					InputHint: map[string]any{"plan_id": plan.PlanID},
				})
			}
		}
		if len(installActions) > 0 {
			nextActions = append(installActions, nextActions...)
			recovery = append([]string{"execute_typed_dependency_install_plan"}, recovery...)
		}
	}
	details := map[string]any{
		"provider_id":                provider.ProviderID,
		"provider_source":            provider.Source,
		"provider_readiness":         readiness.Readiness,
		"readiness_reason":           readiness.Reason,
		"dependency_requirements":    provider.Dependencies,
		"dependency_observations":    readiness.Dependencies,
		"available_package_managers": packageManagers,
		"tool_snapshot_id":           result.ToolSnapshotID,
		"environment_snapshot_id":    result.EnvironmentSnapshotID,
	}
	contentBytes, _ := json.Marshal(details)
	now := timeNowUTC()
	result.Content = string(contentBytes)
	result.Outcome = tool.ExecutionOutcome{
		Content: result.Content, Status: status, TransportStatus: tool.TransportNotStarted,
		SemanticStatus: tool.SemanticFailure, ProcessStatus: tool.ProcessNotApplicable,
		ErrorCode: errorCode, ErrorCategory: category,
		ErrorMessage:    firstNonEmpty(readiness.Reason, "provider is not ready in the pinned run snapshot"),
		Retryable:       readiness.RetryableAfterRemediation,
		RecoveryOptions: recovery,
		Diagnostic:      details,
		Summary:         "Selected provider is not ready in the pinned run snapshot.",
		Operation:       &tool.OperationDescriptor{Type: result.Name, SideEffectState: tool.OutcomeSideEffectNotStarted},
		Environment: &tool.EnvironmentObservation{
			Observation: tool.ObservationPartial, ObservedAt: now, Scope: "run_snapshot",
			Source: "execution_environment_snapshot",
		},
		Errors: []tool.OutcomeError{{
			Type: errorCode, Category: category, Stage: tool.ErrorStagePrecondition,
			Cause:         firstNonEmpty(readiness.Reason, "provider readiness precondition failed"),
			PossibleFixes: recovery, RetryStrategy: retryStrategy,
		}},
		NextActions: nextActions, Details: details,
	}
	if rt != nil && rt.environment != nil {
		result.Outcome.Environment.WorkspaceRoot = rt.environment.WorkspaceRoot
		result.Outcome.Environment.WorkingDirectory = rt.environment.WorkingDirectory
	}
	return result
}

func outcomeFromCompatibilityResult(result ToolExecutionResult) tool.ExecutionOutcome {
	content := result.Content
	switch result.Status {
	case ToolExecutionDenied:
		return tool.DeniedOutcome(content, firstNonEmpty(result.ErrorCode, tool.ErrorCodeToolDenied))
	case ToolExecutionCancelled:
		now := timeNowUTC()
		return tool.ExecutionOutcome{
			Content: content, Status: tool.OutcomeCancelled,
			TransportStatus: tool.TransportCancelled, SemanticStatus: tool.SemanticFailure,
			ProcessStatus: tool.ProcessNotApplicable, ErrorCode: firstNonEmpty(result.ErrorCode, tool.ErrorCodeToolCancelled),
			ErrorCategory: tool.ErrorCategoryCancelled, ErrorMessage: content,
			StartedAt: now, FinishedAt: now,
		}
	}
	if result.IsError || result.Status == ToolExecutionFailed {
		return tool.FailedOutcome(content, firstNonEmpty(result.ErrorCode, tool.ErrorCodeToolExecutionFailed), tool.ErrorCategoryTool, true)
	}
	return tool.SucceededOutcome(content)
}

func mergeToolResultPaths(groups ...[]string) []string {
	seen := map[string]bool{}
	var result []string
	for _, group := range groups {
		for _, path := range group {
			path = strings.TrimSpace(path)
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			result = append(result, path)
		}
	}
	return result
}

// applyExecutorOutcome copies structured Outcome fields from an executor
// output onto the ToolExecutionResult when present.
func applyExecutorOutcome(result *ToolExecutionResult, out executors.ToolExecutionOutput) {
	referencedPaths := mergeToolResultPaths(out.AffectedPaths, out.Outcome.AffectedPaths)
	result.outcomeContext.ReferencedPaths = mergeToolResultPaths(result.outcomeContext.ReferencedPaths, referencedPaths)
	actualPaths := observedChangeResources(out.Outcome.Changes)
	if out.Outcome.Status == tool.OutcomeSucceeded || (out.Outcome.Status == "" && !out.IsError) {
		actualPaths = mergeToolResultPaths(actualPaths, referencedPaths)
	}
	result.ActualAffectedPaths = mergeToolResultPaths(result.ActualAffectedPaths, actualPaths)
	result.AffectedPaths = mergeToolResultPaths(result.AffectedPaths, actualPaths)
	result.outcomeContext.ActualAffectedPaths = mergeToolResultPaths(result.outcomeContext.ActualAffectedPaths, actualPaths)
	if strings.TrimSpace(out.PatchSummary) != "" {
		result.outcomeContext.PatchSummary = strings.TrimSpace(out.PatchSummary)
		result.PatchSummary = strings.TrimSpace(out.PatchSummary)
	} else if strings.TrimSpace(out.Outcome.PatchSummary) != "" {
		result.outcomeContext.PatchSummary = strings.TrimSpace(out.Outcome.PatchSummary)
		result.PatchSummary = strings.TrimSpace(out.Outcome.PatchSummary)
	}
	if out.Outcome.Status == "" {
		if out.Artifact != nil {
			result.outcomeContext.Artifacts = append(result.outcomeContext.Artifacts, *out.Artifact)
		}
		if out.IsError {
			result.IsError = true
		}
		if out.ErrorCode != "" && result.ErrorCode == "" {
			result.ErrorCode = out.ErrorCode
		}
		return
	}
	result.Outcome = out.Outcome
	if out.Artifact != nil {
		result.Outcome.Artifacts = append(result.Outcome.Artifacts, *out.Artifact)
	}
	result.IsError = out.Outcome.IsError()
	result.Status = ToolExecutionStatus(out.Outcome.Status)
	if out.Outcome.ErrorCode != "" {
		result.ErrorCode = out.Outcome.ErrorCode
	} else if out.ErrorCode != "" {
		result.ErrorCode = out.ErrorCode
	}
}

func observedChangeResources(changes []tool.ResourceChange) []string {
	var paths []string
	for _, change := range changes {
		if change.Certainty == tool.ChangeObserved && strings.TrimSpace(change.Resource) != "" {
			paths = append(paths, change.Resource)
		}
	}
	return paths
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
		SemanticStatus:  tool.SemanticFailure,
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
	return synthesizeValidationFailure(
		result,
		tool.ErrorCodeValidationError,
		fmt.Sprintf("工具参数错误: %s", firstNonEmpty(reason, "invalid tool arguments")),
		nil,
	)
}

func synthesizeAskUserInputFailure(result ToolExecutionResult, reason string) ToolExecutionResult {
	const expected = `{"questions":[{"question":"选择主题","options":[{"label":"深色"},{"label":"浅色"}],"multi_select":false}]}`
	return synthesizeValidationFailure(
		result,
		"ask_user_invalid_input",
		fmt.Sprintf(
			"提问格式错误: %s。请传入包含 questions 数组的 JSON 对象，不要传空对象、questions_json 或裸数组。示例：%s",
			firstNonEmpty(reason, "invalid question arguments"),
			expected,
		),
		[]string{"send_questions_object", "provide_one_to_three_questions", "provide_two_to_four_options_per_question"},
	)
}

func synthesizeValidationFailure(result ToolExecutionResult, code, content string, recovery []string) ToolExecutionResult {
	result.Content = content
	result.IsError = true
	result.Status = ToolExecutionDenied
	result.ErrorCode = code
	outcome := tool.FailedOutcome(result.Content, code, tool.ErrorCategoryValidation, false)
	outcome.Status = tool.OutcomeFailed
	outcome.TransportStatus = tool.TransportNotStarted
	outcome.RecoveryOptions = append([]string(nil), recovery...)
	result.Outcome = outcome
	return result
}

func synthesizeUnsupportedToolResult(
	result ToolExecutionResult,
	code string,
	content string,
	recovery []string,
	nextActions []tool.NextAction,
) ToolExecutionResult {
	now := timeNowUTC()
	result.Content = content
	result.IsError = true
	result.Status = ToolExecutionStatus(tool.OutcomeUnsupported)
	result.ErrorCode = code
	result.Outcome = tool.ExecutionOutcome{
		Content:         content,
		Status:          tool.OutcomeUnsupported,
		TransportStatus: tool.TransportNotStarted,
		SemanticStatus:  tool.SemanticFailure,
		ProcessStatus:   tool.ProcessNotApplicable,
		ErrorCode:       code,
		ErrorCategory:   tool.ErrorCategoryUnsupported,
		ErrorMessage:    content,
		Retryable:       false,
		RecoveryOptions: append([]string(nil), recovery...),
		NextActions:     append([]tool.NextAction(nil), nextActions...),
		StartedAt:       now,
		FinishedAt:      now,
	}
	return result
}

// synthesizeExecutionErrorToolResult gives the Go error authoritative
// transport semantics. In particular, an implementation must never be able to
// return a succeeded outcome alongside a non-nil error and have success win.
func synthesizeExecutionErrorToolResult(result ToolExecutionResult, code, content, cause string) ToolExecutionResult {
	result.Content = content
	result.IsError = true
	result.Status = ToolExecutionFailed
	result.ErrorCode = code
	if result.Outcome.Status == "" {
		result.Outcome = tool.FailedOutcome(content, code, tool.ErrorCategoryTool, true)
		result.Outcome.TransportStatus = tool.TransportFailed
		return result
	}
	if result.Outcome.Status != tool.OutcomeSucceeded {
		// Preserve an already-typed business failure while retaining the Go
		// error as the audit-level transport cause.
		result.Content = firstNonEmpty(strings.TrimSpace(result.Outcome.Content), content)
		return result
	}

	result.Outcome.Content = content
	result.Outcome.Status = tool.OutcomeFailed
	result.Outcome.TransportStatus = tool.TransportFailed
	result.Outcome.SemanticStatus = tool.SemanticFailure
	result.Outcome.ErrorCode = code
	result.Outcome.ErrorCategory = tool.ErrorCategoryTool
	result.Outcome.ErrorMessage = firstNonEmpty(strings.TrimSpace(cause), content)
	result.Outcome.Summary = firstNonEmpty(strings.TrimSpace(cause), "Tool implementation failed after reporting success.")
	result.Outcome.Retryable = false
	result.Outcome.RecoveryOptions = []string{"inspect_current_state_before_retry", "report_tool_implementation_error"}
	return result
}

func synthesizeAskUserAwaitFailure(result ToolExecutionResult, ctx stdcontext.Context, err error) ToolExecutionResult {
	content := fmt.Sprintf("用户提问未完成: %v", err)
	now := timeNowUTC()
	status := tool.OutcomeInterrupted
	transport := tool.TransportFailed
	category := tool.ErrorCategoryRuntime
	compatStatus := ToolExecutionFailed
	if errors.Is(err, stdcontext.Canceled) || errors.Is(err, stdcontext.DeadlineExceeded) ||
		errors.Is(ctx.Err(), stdcontext.Canceled) || errors.Is(ctx.Err(), stdcontext.DeadlineExceeded) {
		status = tool.OutcomeCancelled
		transport = tool.TransportCancelled
		category = tool.ErrorCategoryCancelled
		compatStatus = ToolExecutionCancelled
	}
	result.Content = content
	result.IsError = true
	result.Status = compatStatus
	result.ErrorCode = "ask_user_aborted"
	result.Outcome = tool.ExecutionOutcome{
		Content:         content,
		Status:          status,
		TransportStatus: transport,
		SemanticStatus:  tool.SemanticFailure,
		ProcessStatus:   tool.ProcessNotApplicable,
		ErrorCode:       "ask_user_aborted",
		ErrorCategory:   category,
		ErrorMessage:    content,
		Retryable:       false,
		RecoveryOptions: []string{"respect_the_interruption", "request_clarification_again_only_if_still_required"},
		Operation:       &tool.OperationDescriptor{SideEffectState: tool.OutcomeSideEffectNone},
		NextActions: []tool.NextAction{{
			ID:            "respect_interruption",
			Description:   "Stop work that depends on the unanswered choice; ask again only if the user resumes that decision.",
			Preconditions: []string{"Do not infer or fabricate a user answer."},
		}},
		StartedAt:  now,
		FinishedAt: now,
	}
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
	semantic := tool.SemanticFailure
	switch decision.Action {
	case ToolDecisionNeedsAuth:
		status = tool.OutcomeNeedsAuth
		category = tool.ErrorCategoryAuthorization
	case ToolDecisionNeedsRuntime:
		status = tool.OutcomeNeedsRuntime
		category = tool.ErrorCategoryRuntime
	case ToolDecisionUnsupported:
		status = tool.OutcomeUnsupported
		category = tool.ErrorCategoryUnsupported
	}
	result.Outcome = tool.ExecutionOutcome{
		Content:         result.Content,
		Status:          status,
		TransportStatus: transport,
		SemanticStatus:  semantic,
		ProcessStatus:   tool.ProcessNotApplicable,
		ErrorCode:       code,
		ErrorCategory:   category,
		ErrorMessage:    result.Content,
		Retryable:       decision.Retryable,
		StartedAt:       now,
		FinishedAt:      now,
	}
	return result
}
