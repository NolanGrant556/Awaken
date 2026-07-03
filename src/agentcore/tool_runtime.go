package agentcore

import (
	stdcontext "context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/internal/outputlimit"
	"awaken/src/tool"
)

const maxPatchPreviewChars = 50000

type ToolRuntime struct {
	sink              EventSink
	permissionContext tool.PermissionContext
	autoPolicy        AutoToolPolicy
	useApprovalForAsk bool
	serverConnected   func(serverName string) bool
	auditSink         tool.AuditSink
	runID             string
}

type ToolRuntimeConfig struct {
	PermissionContext tool.PermissionContext
	AutoToolPolicy    AutoToolPolicy
	UseApprovalForAsk bool
	ServerConnected   func(serverName string) bool
	AuditSink         tool.AuditSink
	RunID             string
}

type ToolExecutionResult struct {
	ToolCallID     string
	Name           string
	Content        string
	IsError        bool
	Truncated      bool
	Status         ToolExecutionStatus
	ErrorCode      string
	DecisionSource ToolDecisionSource
	DecisionReason string
	StartedAt      time.Time
	FinishedAt     time.Time
	DurationMs     int64
	AffectedPaths  []string
	PatchSummary   string
	ResultSummary  string
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
	return &ToolRuntime{
		sink:              normalizeEventSink(sink),
		permissionContext: config.PermissionContext.Normalize(),
		autoPolicy:        policy,
		useApprovalForAsk: config.UseApprovalForAsk,
		serverConnected:   config.ServerConnected,
		auditSink:         auditSink,
		runID:             config.RunID,
	}
}

func (rt *ToolRuntime) Execute(ctx stdcontext.Context, call convcontext.ToolCall) ToolExecutionResult {
	id := call.ID
	if id == "" {
		id = convcontext.GenerateToolCallID()
	}

	result := ToolExecutionResult{
		ToolCallID: id,
		Name:       call.Function.Name,
		StartedAt:  timeNowUTC(),
	}

	selectedTool, err := tool.GetTool(call.Function.Name)
	if err != nil {
		return finalizeToolResult(synthesizeFailedToolResult(result, "unknown_tool", fmt.Sprintf("工具执行失败: %v: %s", err, call.Function.Name)))
	}

	input, err := normalizeToolInput(call.Function.Arguments, selectedTool.Parameters())
	if err != nil {
		return finalizeToolResult(synthesizeValidationToolResult(result, err.Error()))
	}

	toolCtx := tool.ContextWithPermissionContext(ctx, rt.permissionContext)
	permission := rt.decideToolPermission(toolCtx, selectedTool, input)
	def := selectedTool.Definition()
	result.AffectedPaths = append([]string(nil), permission.ApprovalRequest.AffectedPaths...)
	result.PatchSummary = strings.TrimSpace(permission.ApprovalRequest.PatchSummary)
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
	switch permission.Behavior {
	case tool.PermissionAllow:
		decision = rt.autoPolicy.Decide(toolCtx, decisionRequest)
	case tool.PermissionDeny:
		decision = DefaultAutoToolPolicy{}.Decide(toolCtx, decisionRequest)
	case tool.PermissionAsk:
		if rt.useApprovalForAsk {
			decision = ToolDecision{
				Action:       ToolDecisionAllow,
				Source:       ToolDecisionSourceToolRule,
				Reason:       "compatibility approval path",
				UpdatedInput: input,
			}
			approvalDecision, err := rt.awaitApproval(ctx, selectedTool, result, input, permission)
			if err != nil {
				return finalizeToolResult(synthesizeFailedToolResult(result, "approval_failed", fmt.Sprintf("工具审批失败: %v", err)))
			}
			if approvalDecision == ApprovalDecisionRejected {
				return finalizeToolResult(synthesizeApprovalRejectedToolResult(result))
			}
			break
		}
		decision = rt.autoPolicy.Decide(toolCtx, decisionRequest)
	default:
		decision = ToolDecision{
			Action: ToolDecisionDeny,
			Source: ToolDecisionSourcePolicyError,
			Reason: fmt.Sprintf("invalid permission decision %q", permission.Behavior),
		}
	}

	decision = normalizeToolDecision(decision, firstNonEmpty(permission.Reason, "AUTO policy denied tool execution"))
	rt.emitToolPermissionDecided(result, permission, decision)
	result.DecisionSource = decision.Source
	result.DecisionReason = decision.Reason
	if strings.TrimSpace(decision.UpdatedInput) != "" {
		input = decision.UpdatedInput
	}
	switch decision.Action {
	case ToolDecisionAllow:
	case ToolDecisionDeny:
		if permission.Reason == "needs_auth" {
			rt.emitToolProgress(result, "needs_auth", "tool requires authentication", 0)
		}
		result = synthesizeDeniedToolResult(result, decision)
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		rt.writeAudit(def, result, permission, string(decision.Source), "denied", "")
		return finalizeToolResult(result)
	case ToolDecisionAbort:
		result = synthesizeAbortedToolResult(result, decision)
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		rt.writeAudit(def, result, permission, string(decision.Source), "aborted", result.DecisionReason)
		return finalizeToolResult(result)
	}

	rt.emitToolProgress(result, "started", "", 0)

	output, err := rt.executeTool(toolCtx, result, selectedTool, input)
	if err != nil {
		rt.emitToolProgress(result, "failed", err.Error(), 0)
		result = synthesizeFailedToolResult(result, "tool_execution_failed", fmt.Sprintf("工具执行失败: %v", err))
		result.ResultSummary = excerptString(strings.TrimSpace(result.Content), 200)
		rt.writeAudit(def, result, permission, string(decision.Source), "failed", err.Error())
		return finalizeToolResult(result)
	}

	rt.emitToolProgress(result, "completed", "", 100)
	result.Content = output
	result.ResultSummary = excerptString(strings.TrimSpace(output), 200)
	rt.writeAudit(def, result, permission, string(decision.Source), "success", "")
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

func (rt *ToolRuntime) awaitApproval(
	ctx stdcontext.Context,
	selectedTool tool.Tool,
	result ToolExecutionResult,
	input string,
	permission tool.PermissionDecision,
) (ApprovalDecision, error) {
	awaiter, ok := rt.sink.(ApprovalAwaiter)
	if !ok {
		return "", fmt.Errorf("approval required for %s but no approval awaiter is configured", result.Name)
	}

	request := approvalRequestForPermission(selectedTool, result, input, permission)

	decision, err := awaiter.AwaitApproval(ctx, request)
	if err != nil {
		return "", err
	}
	return decision, nil
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
	rt.sink.Emit(AwakenEvent{
		Type:                   ToolProgressEventType,
		Timestamp:              timeNowUTC(),
		ToolCallID:             result.ToolCallID,
		ToolName:               result.Name,
		ToolProgressStage:      stage,
		ToolProgressMessage:    message,
		ToolProgressPercentage: percentage,
	})
}

func (rt *ToolRuntime) writeAudit(def tool.ToolDefinition, result ToolExecutionResult, permission tool.PermissionDecision, decision, outcome, errReason string) {
	now := timeNowUTC()
	origin := "system_policy"
	if permission.Behavior == tool.PermissionAsk {
		origin = "local_ui"
	} else if permission.Behavior == tool.PermissionAllow {
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
		ApprovalOrigin: origin,
		StartedAt:      now,
		CompletedAt:    now,
		Outcome:        outcome,
		ResultSummary:  result.ResultSummary,
		ErrorReason:    errReason,
	})
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
		Type:                  PatchUpdatedEventType,
		Timestamp:             timeNowUTC(),
		ToolCallID:            result.ToolCallID,
		ToolName:              result.Name,
		Patch:                 limited.Content,
		PatchSummary:          strings.TrimSpace(preview.Summary),
		ApprovalAffectedPaths: preview.AffectedPaths,
		Truncated:             preview.Truncated || limited.Truncated,
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
		ApprovalAffectedPaths:    result.AffectedPaths,
		ApprovalPatchSummary:     result.PatchSummary,
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
	if result.Status == "" {
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

func synthesizeDeniedToolResult(result ToolExecutionResult, decision ToolDecision) ToolExecutionResult {
	result.Content = fmt.Sprintf("工具执行被 AUTO policy 拒绝: %s", firstNonEmpty(decision.Reason, "permission policy denied execution"))
	result.IsError = true
	result.Status = ToolExecutionDenied
	result.ErrorCode = "tool_denied"
	result.DecisionSource = decision.Source
	result.DecisionReason = decision.Reason
	return result
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
	return result
}

func synthesizeValidationToolResult(result ToolExecutionResult, reason string) ToolExecutionResult {
	result.Content = fmt.Sprintf("工具参数错误: %s", firstNonEmpty(reason, "invalid tool arguments"))
	result.IsError = true
	result.Status = ToolExecutionDenied
	result.ErrorCode = "validation_error"
	return result
}

func synthesizeFailedToolResult(result ToolExecutionResult, code string, content string) ToolExecutionResult {
	result.Content = content
	result.IsError = true
	result.Status = ToolExecutionFailed
	result.ErrorCode = code
	return result
}

func synthesizeApprovalRejectedToolResult(result ToolExecutionResult) ToolExecutionResult {
	result.Content = "工具执行已被审批拒绝"
	result.IsError = true
	result.Status = ToolExecutionDenied
	result.ErrorCode = "approval_rejected"
	result.DecisionSource = ToolDecisionSourceToolRule
	result.DecisionReason = "审批拒绝"
	return result
}

func approvalDescription(toolName string, input string) string {
	trimmed := excerptString(strings.TrimSpace(input), 240)
	if trimmed == "" {
		return fmt.Sprintf("%s requires approval before execution.", toolName)
	}
	return fmt.Sprintf("%s requires approval before execution.\n\n%s", toolName, trimmed)
}

func approvalRequestForPermission(
	selectedTool tool.Tool,
	result ToolExecutionResult,
	input string,
	permission tool.PermissionDecision,
) ApprovalRequest {
	def := selectedTool.Definition()
	affectedSystems := []string{}
	if def.Annotations.ServerName != "" {
		affectedSystems = append(affectedSystems, def.Annotations.ServerName)
	}
	request := ApprovalRequest{
		ID:             newAwakenID("approval"),
		ToolCallID:     result.ToolCallID,
		ToolName:       result.Name,
		Title:          fmt.Sprintf("Approve %s", result.Name),
		Description:    approvalDescription(result.Name, input),
		RiskTone:       ApprovalRiskWarning,
		ToolDefinition: def,
		RiskHints: &tool.RiskHints{
			Destructive:     def.Annotations.Destructive,
			OpenWorld:       def.Annotations.OpenWorld,
			RequiresAuth:    def.Annotations.RequiresAuth,
			AffectedSystems: affectedSystems,
		},
		AllowedActions: []string{"approve", "reject", "edit_plan", "request_more_info"},
	}

	if describingTool, ok := selectedTool.(tool.ApprovalDescribingTool); ok {
		mergeApprovalRequest(&request, describingTool.ApprovalRequest(input))
	}
	if strings.TrimSpace(permission.Reason) != "" {
		request.Description = permissionDescription(permission.Reason, input)
	}
	mergeApprovalRequest(&request, permission.ApprovalRequest)

	return request
}

func mergeApprovalRequest(request *ApprovalRequest, metadata tool.ApprovalRequest) {
	if strings.TrimSpace(metadata.Title) != "" {
		request.Title = strings.TrimSpace(metadata.Title)
	}
	if strings.TrimSpace(metadata.Description) != "" {
		request.Description = strings.TrimSpace(metadata.Description)
	}
	if riskTone, ok := approvalRiskToneFromString(metadata.RiskTone); ok {
		request.RiskTone = riskTone
	}
	if len(metadata.AffectedPaths) > 0 {
		request.AffectedPaths = metadata.AffectedPaths
	}
	if strings.TrimSpace(metadata.PatchSummary) != "" {
		request.PatchSummary = strings.TrimSpace(metadata.PatchSummary)
	}
}

func approvalRiskToneFromString(value string) (ApprovalRiskTone, bool) {
	switch ApprovalRiskTone(strings.TrimSpace(value)) {
	case ApprovalRiskSafe, ApprovalRiskWarning, ApprovalRiskDanger:
		return ApprovalRiskTone(strings.TrimSpace(value)), true
	default:
		return "", false
	}
}

func permissionDescription(reason string, input string) string {
	trimmedReason := strings.TrimSpace(reason)
	trimmedInput := excerptString(strings.TrimSpace(input), 240)
	if trimmedInput == "" {
		return trimmedReason
	}
	return fmt.Sprintf("%s\n\n%s", trimmedReason, trimmedInput)
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
