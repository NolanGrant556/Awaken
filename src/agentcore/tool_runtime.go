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
	serverConnected   func(serverName string) bool
	auditSink         tool.AuditSink
	runID             string
}

type ToolRuntimeConfig struct {
	PermissionContext tool.PermissionContext
	ServerConnected   func(serverName string) bool
	AuditSink         tool.AuditSink
	RunID             string
}

type ToolExecutionResult struct {
	ToolCallID    string
	Name          string
	Content       string
	IsError       bool
	Truncated     bool
	ResultSummary string
}

func NewToolRuntime(sink EventSink) *ToolRuntime {
	return NewToolRuntimeWithConfig(sink, ToolRuntimeConfig{})
}

func NewToolRuntimeWithConfig(sink EventSink, config ToolRuntimeConfig) *ToolRuntime {
	auditSink := config.AuditSink
	if auditSink == nil {
		auditSink = &tool.NoopSink{}
	}
	return &ToolRuntime{
		sink:              normalizeEventSink(sink),
		permissionContext: config.PermissionContext.Normalize(),
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
	}

	selectedTool, err := tool.GetTool(call.Function.Name)
	if err != nil {
		result.Content = fmt.Sprintf("工具执行失败: %v: %s", err, call.Function.Name)
		result.IsError = true
		return finalizeToolResult(result)
	}

	input, err := normalizeToolInput(call.Function.Arguments, selectedTool.Parameters())
	if err != nil {
		result.Content = fmt.Sprintf("工具参数错误: %v", err)
		result.IsError = true
		return finalizeToolResult(result)
	}

	toolCtx := tool.ContextWithPermissionContext(ctx, rt.permissionContext)
	permission := rt.decideToolPermission(toolCtx, selectedTool, input)
	def := selectedTool.Definition()
	rt.emitPatchPreview(result, selectedTool, input)
	switch permission.Behavior {
	case tool.PermissionAllow:
	case tool.PermissionDeny:
		// needs_auth is a special deny that emits a progress event instead of
		// a regular error; the tool is never invoked.
		if permission.Reason == "needs_auth" {
			rt.emitToolProgress(result, "needs_auth", "tool requires authentication", 0)
			rt.writeAudit(def, result, permission, "deny", "needs_auth", "")
			result.Content = "工具需要认证: " + firstNonEmpty(permission.Reason, "needs_auth")
			result.IsError = true
			return finalizeToolResult(result)
		}
		rt.writeAudit(def, result, permission, "deny", "denied", "")
		result.Content = fmt.Sprintf("工具权限拒绝: %s", firstNonEmpty(permission.Reason, "permission policy denied execution"))
		result.IsError = true
		return finalizeToolResult(result)
	case tool.PermissionAsk:
		decision, err := rt.awaitApproval(ctx, selectedTool, result, input, permission)
		if err != nil {
			rt.writeAudit(def, result, permission, "ask", "failed", err.Error())
			result.Content = fmt.Sprintf("工具审批失败: %v", err)
			result.IsError = true
			return finalizeToolResult(result)
		}
		if decision == ApprovalDecisionRejected {
			rt.writeAudit(def, result, permission, "ask", "denied", "")
			result.Content = "工具执行已被审批拒绝"
			result.IsError = true
			return finalizeToolResult(result)
		}
	default:
		rt.writeAudit(def, result, permission, "deny", "denied", "")
		result.Content = fmt.Sprintf("工具权限拒绝: invalid permission decision %q", permission.Behavior)
		result.IsError = true
		return finalizeToolResult(result)
	}

	rt.emitToolProgress(result, "started", "", 0)

	output, err := rt.executeTool(toolCtx, result, selectedTool, input)
	if err != nil {
		rt.emitToolProgress(result, "failed", err.Error(), 0)
		rt.writeAudit(def, result, permission, string(permission.Behavior), "failed", err.Error())
		result.Content = fmt.Sprintf("工具执行失败: %v", err)
		result.IsError = true
		return finalizeToolResult(result)
	}

	rt.emitToolProgress(result, "completed", "", 100)
	rt.writeAudit(def, result, permission, string(permission.Behavior), "success", "")
	result.Content = output
	result.ResultSummary = excerptString(strings.TrimSpace(output), 200)
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
		Type:                  ToolProgressEventType,
		Timestamp:             timeNowUTC(),
		ToolCallID:            result.ToolCallID,
		ToolName:              result.Name,
		ToolProgressStage:     stage,
		ToolProgressMessage:   message,
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

type toolObserverFunc func(tool.CommandOutput)

func (fn toolObserverFunc) OnCommandOutput(output tool.CommandOutput) {
	fn(output)
}

func finalizeToolResult(result ToolExecutionResult) ToolExecutionResult {
	limited := outputlimit.Limit(result.Content, outputlimit.DefaultToolResultMaxChars)
	result.Content = limited.Content
	result.Truncated = limited.Truncated
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
