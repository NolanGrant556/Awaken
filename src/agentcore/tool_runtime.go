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
	"awaken/src/toolresults"
)

const maxPatchPreviewChars = 50000

type ToolRuntime struct {
	sink                   EventSink
	permissionContext      tool.PermissionContext
	permissionPolicy       tool.PermissionPolicy
	toolAccessPolicy       ToolAccessPolicy
	permissionMode         PermissionMode
	toolResultStore        *toolresults.Store
	toolResultMaxChars     int
	toolResultPreviewChars int
}

type ToolRuntimeConfig struct {
	PermissionContext tool.PermissionContext
	PermissionPolicy  tool.PermissionPolicy
	ToolResultStore   *toolresults.Store
	ToolAccessPolicy  ToolAccessPolicy
	PermissionMode    PermissionMode

	ToolResultMaxChars     int
	ToolResultPreviewChars int
}

type ToolExecutionResult struct {
	ToolCallID       string
	Name             string
	Content          string
	IsError          bool
	Truncated        bool
	ToolResultHandle *toolresults.Handle
}

func NewToolRuntime(sink EventSink) *ToolRuntime {
	return NewToolRuntimeWithConfig(sink, ToolRuntimeConfig{})
}

func NewToolRuntimeWithConfig(sink EventSink, config ToolRuntimeConfig) *ToolRuntime {
	permissionContext := config.PermissionContext.Normalize()
	maxChars := config.ToolResultMaxChars
	if maxChars <= 0 {
		maxChars = outputlimit.DefaultToolResultMaxChars
	}
	previewChars := config.ToolResultPreviewChars
	if previewChars <= 0 {
		previewChars = toolresults.DefaultPreviewChars
	}
	return &ToolRuntime{
		sink:                   normalizeEventSink(sink),
		permissionContext:      permissionContext,
		permissionPolicy:       config.PermissionPolicy.Normalize(),
		toolAccessPolicy:       config.ToolAccessPolicy,
		permissionMode:         config.PermissionMode,
		toolResultStore:        config.ToolResultStore,
		toolResultMaxChars:     maxChars,
		toolResultPreviewChars: previewChars,
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

	if !rt.toolAccessPolicy.Allows(call.Function.Name) {
		result.Content = fmt.Sprintf("工具权限拒绝: tool %q is not allowed for this run profile", call.Function.Name)
		result.IsError = true
		return rt.finalizeToolResult(result)
	}

	selectedTool, err := tool.GetTool(call.Function.Name)
	if err != nil {
		result.Content = fmt.Sprintf("工具执行失败: %v: %s", err, call.Function.Name)
		result.IsError = true
		return rt.finalizeToolResult(result)
	}

	input, err := normalizeToolInput(call.Function.Arguments, selectedTool.Parameters())
	if err != nil {
		result.Content = fmt.Sprintf("工具参数错误: %v", err)
		result.IsError = true
		return rt.finalizeToolResult(result)
	}

	toolCtx := tool.ContextWithPermissionContext(ctx, rt.permissionContext)
	permission := rt.decideToolPermission(toolCtx, selectedTool, input)
	rt.emitPatchPreview(result, selectedTool, input)
	switch permission.Behavior {
	case tool.PermissionAllow:
	case tool.PermissionDeny:
		result.Content = fmt.Sprintf("工具权限拒绝: %s", firstNonEmpty(permission.Reason, "permission policy denied execution"))
		result.IsError = true
		return rt.finalizeToolResult(result)
	case tool.PermissionAsk:
		decision, err := rt.awaitApproval(ctx, selectedTool, result, input, permission)
		if err != nil {
			result.Content = fmt.Sprintf("工具审批失败: %v", err)
			result.IsError = true
			return rt.finalizeToolResult(result)
		}
		if decision == ApprovalDecisionRejected {
			result.Content = "工具执行已被审批拒绝"
			result.IsError = true
			return rt.finalizeToolResult(result)
		}
	default:
		result.Content = fmt.Sprintf("工具权限拒绝: invalid permission decision %q", permission.Behavior)
		result.IsError = true
		return rt.finalizeToolResult(result)
	}

	output, err := rt.executeTool(toolCtx, result, selectedTool, input)
	if err != nil {
		result.Content = fmt.Sprintf("工具执行失败: %v", err)
		result.IsError = true
		return rt.finalizeToolResult(result)
	}

	result.Content = output
	rt.emitArtifacts(result, selectedTool, input, output)
	return rt.finalizeToolResult(result)
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
	readOnly := isReadOnlyTool(selectedTool)
	var (
		toolDecision tool.PermissionDecision
		hasDecision  bool
	)
	if checker, ok := selectedTool.(tool.PermissionCheckingTool); ok {
		toolDecision = checker.CheckPermission(ctx, input, rt.permissionContext)
		hasDecision = true
		if toolDecision.Behavior == "" {
			return tool.DenyPermission("tool returned an empty permission decision")
		}
	}

	decision := toolDecision
	if !hasDecision {
		if readOnly {
			decision = tool.AllowPermission()
		} else {
			decision = tool.AskPermission("tool does not declare permission behavior and is not marked read-only", tool.ApprovalRequest{})
		}
	}

	switch rt.permissionMode {
	case PermissionModeDeny:
		if hasDecision && toolDecision.Behavior == tool.PermissionDeny {
			return toolDecision
		}
		if readOnly && (!hasDecision || toolDecision.Behavior == tool.PermissionAllow) {
			decision = tool.AllowPermission()
			break
		}
		return tool.DenyPermission("run profile permission_mode=deny blocked tool execution")
	case PermissionModeAsk:
		if !hasDecision && !readOnly {
			decision = tool.AskPermission("run profile permission_mode=ask requires approval for non-read-only tool", tool.ApprovalRequest{})
		}
	case PermissionModeAllow:
		if hasDecision {
			if toolDecision.Behavior == tool.PermissionDeny {
				return toolDecision
			}
		}
		decision = tool.AllowPermission()
	}

	return tool.ApplyPermissionPolicy(decision, rt.permissionPolicy, selectedTool.Name(), readOnly)
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
	request = DisplaySafeApprovalRequest(request)

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
		rt.emitDisplayEvent(AwakenEvent{
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
	rt.emitDisplayEvent(AwakenEvent{
		Type:          CommandOutputEventType,
		Timestamp:     timeNowUTC(),
		ToolCallID:    result.ToolCallID,
		ToolName:      result.Name,
		CommandStream: output.Stream,
		Delta:         output.Delta,
		Truncated:     output.Truncated,
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
	rt.emitDisplayEvent(AwakenEvent{
		Type:                  PatchUpdatedEventType,
		Timestamp:             timeNowUTC(),
		ToolCallID:            result.ToolCallID,
		ToolName:              result.Name,
		Patch:                 limited.Content,
		PatchSummary:          strings.TrimSpace(preview.Summary),
		ApprovalAffectedPaths: preview.AffectedPaths,
		DiffReview:            preview.DiffReview,
		Truncated:             preview.Truncated || limited.Truncated,
	})
}

func (rt *ToolRuntime) emitDisplayEvent(event AwakenEvent) {
	rt.sink.Emit(DisplaySafeEvent(event))
}

type toolObserverFunc func(tool.CommandOutput)

func (fn toolObserverFunc) OnCommandOutput(output tool.CommandOutput) {
	fn(output)
}

func (rt *ToolRuntime) finalizeToolResult(result ToolExecutionResult) ToolExecutionResult {
	limited := outputlimit.Limit(result.Content, rt.toolResultMaxChars)
	if !limited.Truncated {
		result.Content = limited.Content
		result.Truncated = false
		return result
	}
	if rt.toolResultStore != nil {
		handle, err := rt.toolResultStore.Persist(result.Content, rt.toolResultPreviewChars)
		if err == nil {
			result.Content = toolresults.FormatModelContent(handle)
			result.Truncated = true
			result.ToolResultHandle = &handle
			return result
		}
	}
	result.Content = limited.Content
	result.Truncated = limited.Truncated
	return result
}

func isReadOnlyTool(selectedTool tool.Tool) bool {
	readOnlyTool, ok := selectedTool.(tool.ReadOnlyTool)
	return ok && readOnlyTool.IsReadOnly()
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
	request := ApprovalRequest{
		ID:          newAwakenID("approval"),
		ToolCallID:  result.ToolCallID,
		ToolName:    result.Name,
		Title:       fmt.Sprintf("Approve %s", result.Name),
		Description: approvalDescription(result.Name, input),
		RiskTone:    ApprovalRiskWarning,
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
	if metadata.DiffReview != nil {
		request.DiffReview = metadata.DiffReview
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
