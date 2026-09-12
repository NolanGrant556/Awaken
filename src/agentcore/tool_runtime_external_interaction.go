package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"awaken/src/tool"
	"awaken/src/tool/profile"
)

func (rt *ToolRuntime) maybeAwaitCLIAuthInteraction(
	ctx context.Context,
	result ToolExecutionResult,
	def tool.ToolDefinition,
	permission tool.PermissionDecision,
	decision ToolDecision,
	selectedTool tool.Tool,
	intent profile.ExecutionIntent,
	input string,
	alreadyReplayed bool,
) (ToolExecutionResult, bool) {
	outcome := result.Outcome
	if strings.TrimSpace(outcome.Content) == "" {
		outcome.Content = result.Content
	}
	if outcome.ExitCode == nil && result.Outcome.ExitCode != nil {
		outcome.ExitCode = result.Outcome.ExitCode
	}
	view, ok := DetectCLIAuthInteraction(result.Name, outcome)
	if !ok {
		view, ok = DetectPluginAuthInteraction(result.Name, outcome)
	}
	if !ok {
		view, ok = DetectConnectorAuthInteraction(result.Name, outcome)
	}
	if !ok {
		return result, false
	}
	isMCP := strings.HasPrefix(result.Name, "mcp__")
	if isMCP && (result.Outcome.TransportStatus != tool.TransportNotStarted ||
		(result.Outcome.Operation != nil && result.Outcome.Operation.SideEffectState != tool.OutcomeSideEffectNotStarted && result.Outcome.Operation.SideEffectState != tool.OutcomeSideEffectNone)) {
		return result, false
	}
	if alreadyReplayed {
		code := "cli_auth_replay_exhausted"
		message := "授权后原命令仍需要授权。请检查权限范围后重试，或前往连接器完成配置。"
		if strings.HasPrefix(view.Provider, "plugin:") {
			code = "plugin_auth_replay_exhausted"
			message = "授权后插件调用仍需要授权。请检查该插件的账号和权限范围后重试。"
		} else if strings.HasPrefix(view.Provider, "connector:") {
			code = "connector_auth_replay_exhausted"
			message = "授权后连接器调用仍需要授权。请检查 Outlook 账号和权限范围后重试。"
		}
		result = synthesizeFailedToolResult(
			result,
			code,
			message,
		)
		result.Outcome.Retryable = !isMCP
		result.Outcome.RecoveryOptions = []string{"complete_external_authorization", "retry_with_required_scopes"}
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		return rt.finalizeExecutedToolResult(result, def, permission, string(decision.Source), selectedTool, input, result.Content), true
	}

	return rt.awaitExternalInteractionAndContinue(ctx, result, def, permission, decision, selectedTool, intent, input, view), true
}

func (rt *ToolRuntime) awaitExternalInteractionAndContinue(
	ctx context.Context,
	result ToolExecutionResult,
	def tool.ToolDefinition,
	permission tool.PermissionDecision,
	decision ToolDecision,
	selectedTool tool.Tool,
	intent profile.ExecutionIntent,
	input string,
	view ExternalInteractionView,
) ToolExecutionResult {
	request := rt.externalInteractionEvent(result, def, view)
	awaiter, ok := rt.sink.(ExternalInteractionAwaiter)
	if !ok {
		rt.sink.Emit(request)
		result = synthesizeUnsupportedToolResult(
			result,
			"external_interaction_no_awaiter",
			"当前运行环境不支持等待外部授权操作",
			[]string{"use_an_interactive_runtime", "complete_authorization_in_connectors"},
			[]tool.NextAction{{
				ID:            "switch_to_interactive_runtime",
				Description:   "Continue in a runtime that can await an external user action.",
				Preconditions: []string{"Do not claim the external action completed."},
			}},
		)
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		rt.writeAudit(def, result, permission, string(decision.Source), auditOutcomeLabel(result), "no external interaction awaiter")
		return finalizeToolResult(result)
	}

	rt.emitToolProgress(result, "interaction_required", firstNonEmpty(view.SafeMessage, "等待用户完成外部操作"), 0)
	resolution, err := awaiter.AwaitExternalInteraction(ctx, request)
	if err != nil {
		result = synthesizeExternalInteractionAwaitFailure(result, ctx, err)
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		rt.writeAudit(def, result, permission, string(decision.Source), auditOutcomeLabel(result), err.Error())
		return finalizeToolResult(result)
	}

	switch resolution {
	case ExternalInteractionCompleted:
		if !view.RequiresReplay {
			result.Content = firstNonEmpty(view.SafeMessage, "用户已完成外部操作")
			result.Outcome = tool.SucceededOutcome(result.Content)
			result.Outcome.Summary = "External interaction completed."
			result.Outcome.Details = map[string]any{
				"interaction_id": request.InteractionID,
				"provider":       view.Provider,
				"resolution":     string(resolution),
			}
			result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
			return rt.finalizeExecutedToolResult(result, def, permission, string(decision.Source), selectedTool, input, result.Content)
		}

		if strings.HasPrefix(result.Name, "mcp__") && !tool.AuthReplayBudgetFromContext(ctx).Take() {
			result.Outcome.Retryable = false
			result.Outcome.RecoveryOptions = []string{"authenticate_mcp_server", "do_not_replay_original_call"}
			return finalizeToolResult(result)
		}
		output, trashedPaths, execOut, execErr := rt.routeToExecutor(ctx, result, intent, selectedTool, input)
		result.TrashedPaths = append([]string(nil), trashedPaths...)
		if execErr != nil {
			failContent := output
			if strings.TrimSpace(failContent) == "" {
				failContent = fmt.Sprintf("授权后重放失败: %v", execErr)
			}
			applyExecutorOutcome(&result, execOut)
			result = synthesizeExecutionErrorToolResult(result, "tool_execution_failed", failContent, execErr.Error())
			result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
			return rt.finalizeExecutedToolResult(result, def, permission, string(decision.Source), selectedTool, input, output)
		}
		result.Content = output
		applyExecutorOutcome(&result, execOut)
		result.ResultSummary = excerptString(strings.TrimSpace(output), 200)
		if handled, done := rt.maybeAwaitCLIAuthInteraction(ctx, result, def, permission, decision, selectedTool, intent, input, true); done {
			return handled
		}
		return rt.finalizeExecutedToolResult(result, def, permission, string(decision.Source), selectedTool, input, output)
	case ExternalInteractionCancelled:
		result = synthesizeFailedToolResult(result, "external_interaction_cancelled", "用户取消了外部授权操作")
	case ExternalInteractionExpired:
		result = synthesizeFailedToolResult(result, "external_interaction_expired", "外部授权已过期，请重新获取")
	default:
		result = synthesizeFailedToolResult(result, "external_interaction_failed", "外部授权失败，请重试或前往连接器检查状态")
	}
	result.Outcome.Retryable = true
	result.Outcome.RecoveryOptions = []string{"retry_external_authorization", "open_connectors"}
	result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
	rt.writeAudit(def, result, permission, string(decision.Source), auditOutcomeLabel(result), string(resolution))
	return finalizeToolResult(result)
}

func (rt *ToolRuntime) externalInteractionEvent(result ToolExecutionResult, def tool.ToolDefinition, view ExternalInteractionView) AwakenEvent {
	interactionID := newAwakenID("external_interaction")
	safeView := ExternalInteractionView{
		InteractionID:   interactionID,
		Title:           view.Title,
		SafeMessage:     view.SafeMessage,
		VerificationURL: view.VerificationURL,
		QRImageURL:      view.QRImageURL,
		Provider:        view.Provider,
		ToolCallID:      result.ToolCallID,
		ToolName:        result.Name,
		Recoveries:      append([]string(nil), view.Recoveries...),
		RequiresReplay:  view.RequiresReplay,
	}
	contentBytes, err := json.Marshal(safeView)
	if err != nil {
		contentBytes = []byte("{}")
	}
	return AwakenEvent{
		Type:                       ExternalInteractionRequestedType,
		Timestamp:                  timeNowUTC(),
		ToolCallID:                 result.ToolCallID,
		ToolName:                   result.Name,
		ToolDefinition:             def,
		InteractionID:              interactionID,
		InteractionTitle:           safeView.Title,
		InteractionSafeMessage:     safeView.SafeMessage,
		InteractionVerificationURL: safeView.VerificationURL,
		InteractionQRImageURL:      safeView.QRImageURL,
		InteractionProvider:        safeView.Provider,
		InteractionRecoveries:      append([]string(nil), safeView.Recoveries...),
		InteractionRequiresReplay:  safeView.RequiresReplay,
		AuthProvider:               safeView.Provider,
		AuthURL:                    safeView.VerificationURL,
		Content:                    string(contentBytes),
	}
}

func synthesizeExternalInteractionAwaitFailure(result ToolExecutionResult, ctx context.Context, err error) ToolExecutionResult {
	content := "外部授权等待被中断"
	now := timeNowUTC()
	status := tool.OutcomeInterrupted
	transport := tool.TransportFailed
	category := tool.ErrorCategoryRuntime
	compatStatus := ToolExecutionFailed
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		status = tool.OutcomeCancelled
		transport = tool.TransportCancelled
		category = tool.ErrorCategoryCancelled
		compatStatus = ToolExecutionCancelled
		content = "外部授权等待已取消"
	}
	result.Content = content
	result.IsError = true
	result.Status = compatStatus
	result.ErrorCode = "external_interaction_aborted"
	result.Outcome = tool.ExecutionOutcome{
		Content:         content,
		Status:          status,
		TransportStatus: transport,
		SemanticStatus:  tool.SemanticFailure,
		ProcessStatus:   tool.ProcessNotApplicable,
		ErrorCode:       "external_interaction_aborted",
		ErrorCategory:   category,
		ErrorMessage:    content,
		Retryable:       false,
		RecoveryOptions: []string{"respect_the_interruption", "retry_authorization_only_if_still_required"},
		Operation:       &tool.OperationDescriptor{SideEffectState: tool.OutcomeSideEffectNone},
		StartedAt:       now,
		FinishedAt:      now,
	}
	return result
}
