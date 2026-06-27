package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
)

type runtimeTestTool struct {
	name            string
	params          []tool.ToolParameter
	fail            bool
	output          string
	observe         *string
	permission      tool.PermissionDecision
	observePolicy   *tool.PermissionContext
	approvalRequest tool.ApprovalRequest
	artifacts       []tool.ArtifactDescriptor
	executeCount    *int
	commandOutputs  []tool.CommandOutput
}

func (t runtimeTestTool) Name() string {
	return t.name
}

func (t runtimeTestTool) Description() string {
	return "runtime test tool"
}

func (t runtimeTestTool) Parameters() []tool.ToolParameter {
	return t.params
}

func (t runtimeTestTool) Execute(ctx context.Context, input string) (string, error) {
	if t.executeCount != nil {
		*t.executeCount++
	}
	if t.observe != nil {
		*t.observe = input
	}
	if t.fail {
		return "", errors.New("boom")
	}
	if t.output != "" {
		return t.output, nil
	}
	return "ran:" + input, nil
}

func (t runtimeTestTool) ExecuteWithObserver(ctx context.Context, input string, observer tool.ToolObserver) (string, error) {
	for _, output := range t.commandOutputs {
		observer.OnCommandOutput(output)
	}
	return t.Execute(ctx, input)
}

func (t runtimeTestTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	if t.observePolicy != nil {
		*t.observePolicy = permissionContext
	}
	if t.permission.Behavior == "" {
		return tool.AllowPermission()
	}
	return t.permission
}

func (t runtimeTestTool) ApprovalRequest(input string) tool.ApprovalRequest {
	return t.approvalRequest
}

func (t runtimeTestTool) Artifacts(input string, output string, permissionContext tool.PermissionContext) []tool.ArtifactDescriptor {
	return t.artifacts
}

func TestToolRuntimeExecutesKnownTool(t *testing.T) {
	var observed string
	toolName := "runtime_known_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:    toolName,
		observe: &observed,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	result := NewToolRuntime(nil).Execute(context.Background(), convcontext.ToolCall{
		ID:   "call_1",
		Type: "function",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected success, got %+v", result)
	}
	if result.ToolCallID != "call_1" || result.Name != toolName || result.Content != "ran:hello" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if observed != "hello" {
		t.Fatalf("expected normalized input hello, got %q", observed)
	}
}

func TestToolRuntimeUnknownToolReturnsErrorResult(t *testing.T) {
	result := NewToolRuntime(nil).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_unknown",
		Function: convcontext.FunctionCall{
			Name:      "runtime_missing_tool",
			Arguments: `{}`,
		},
	})

	if !result.IsError || result.ToolCallID != "call_unknown" || !strings.Contains(result.Content, "工具执行失败") {
		t.Fatalf("expected unknown tool error result, got %+v", result)
	}
}

func TestToolRuntimeInvalidJSONReturnsErrorResult(t *testing.T) {
	toolName := "runtime_json_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name: toolName,
		params: []tool.ToolParameter{
			{Name: "path", Type: "string", Required: true},
			{Name: "content", Type: "string", Required: true},
		},
	})

	result := NewToolRuntime(nil).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_bad_json",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `not-json`,
		},
	})

	if !result.IsError || !strings.Contains(result.Content, "工具参数错误") {
		t.Fatalf("expected invalid JSON error result, got %+v", result)
	}
}

func TestToolRuntimeToolFailureReturnsErrorResult(t *testing.T) {
	toolName := "runtime_failing_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name: toolName,
		fail: true,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	result := NewToolRuntime(nil).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_fail",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if !result.IsError || !strings.Contains(result.Content, "工具执行失败") {
		t.Fatalf("expected execution error result, got %+v", result)
	}
}

func TestToolRuntimeExecutesMultipleCalls(t *testing.T) {
	toolName := "runtime_multi_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name: toolName,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	results := NewToolRuntime(nil).ExecuteToolCalls(context.Background(), []convcontext.ToolCall{
		{ID: "call_1", Function: convcontext.FunctionCall{Name: toolName, Arguments: `{"value":"one"}`}},
		{ID: "call_2", Function: convcontext.FunctionCall{Name: toolName, Arguments: `{"value":"two"}`}},
	})

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ToolCallID != "call_1" || results[1].ToolCallID != "call_2" {
		t.Fatalf("expected tool call ids to be preserved, got %+v", results)
	}
}

func TestToolRuntimeTruncatesLargeToolResult(t *testing.T) {
	toolName := "runtime_large_output_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:   toolName,
		output: strings.Repeat("x", 25000),
	})

	result := NewToolRuntime(nil).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_large",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{}`,
		},
	})

	if result.IsError || !result.Truncated || !strings.Contains(result.Content, "truncated") {
		t.Fatalf("expected truncated success result, got %+v", result)
	}
	if len([]rune(result.Content)) >= 25000 {
		t.Fatalf("expected content to be shortened")
	}
}

func TestArgumentsForXMLFallbackUsesSingleRequiredStringParam(t *testing.T) {
	toolName := "runtime_xml_read_like_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name: toolName,
		params: []tool.ToolParameter{
			{Name: "path", Type: "string", Required: true},
			{Name: "offset", Type: "integer", Required: false},
			{Name: "limit", Type: "integer", Required: false},
		},
	})

	got := argumentsForXMLFallback(toolName, "README.md")
	if got != `{"path":"README.md"}` {
		t.Fatalf("unexpected fallback arguments: %q", got)
	}
}

func TestToolRuntimeFailsClosedWhenNoApprovalAwaiterIsWired(t *testing.T) {
	toolName := "runtime_no_awaiter_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:       toolName,
		permission: tool.AskPermission("test tool requires approval", tool.ApprovalRequest{}),
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	result := NewToolRuntime(nil).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_missing_awaiter",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if !result.IsError || !strings.Contains(result.Content, "no approval awaiter") {
		t.Fatalf("expected missing approval awaiter to fail closed, got %+v", result)
	}
}

func TestToolRuntimeEmitsArtifactsAndRejectsOnApprovalDecision(t *testing.T) {
	toolName := "runtime_approval_artifact_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name: toolName,
		permission: tool.AskPermission("test tool writes files", tool.ApprovalRequest{
			Title:       "Approve write",
			Description: "Write README.md",
			RiskTone:    "danger",
		}),
		artifacts: []tool.ArtifactDescriptor{{
			Action:      tool.ArtifactActionCreated,
			ID:          "artifact-1",
			Title:       "README.md",
			Description: "Generated readme",
			Kind:        "file",
			Path:        "/tmp/README.md",
			Content:     "# hello",
			Status:      "ready",
		}},
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	var got []AwakenEvent
	runtime := NewToolRuntime(testApprovalAwaiter{
		decision: ApprovalDecisionApproved,
		events:   &got,
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_with_approval",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected approved tool to succeed, got %+v", result)
	}
	if len(got) != 2 {
		t.Fatalf("expected approval and artifact events, got %+v", got)
	}
	if got[0].Type != ApprovalRequestedEventType || got[1].Type != ArtifactCreatedEventType {
		t.Fatalf("unexpected emitted events: %+v", got)
	}

	var rejectedEvents []AwakenEvent
	rejected := NewToolRuntime(testApprovalAwaiter{decision: ApprovalDecisionRejected, events: &rejectedEvents})
	rejectedResult := rejected.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_rejected",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if !rejectedResult.IsError || !strings.Contains(rejectedResult.Content, "审批拒绝") {
		t.Fatalf("expected rejected approval to stop tool execution, got %+v", rejectedResult)
	}
	for _, event := range rejectedEvents {
		if event.Type == ArtifactCreatedEventType || event.Type == ArtifactUpdatedEventType {
			t.Fatalf("expected no artifact events on rejected approval, got %+v", event)
		}
	}
}

func TestToolRuntimeEmitsCommandOutputFromObservableTool(t *testing.T) {
	toolName := "runtime_observable_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name: toolName,
		commandOutputs: []tool.CommandOutput{
			{Stream: "stdout", Delta: "hello"},
			{Stream: "stderr", Delta: "warn", Truncated: true},
		},
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	var got []AwakenEvent
	result := NewToolRuntime(EventSinkFunc(func(event AwakenEvent) {
		got = append(got, event)
	})).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_observable",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected success, got %+v", result)
	}
	if len(got) != 2 {
		t.Fatalf("expected two command output events, got %+v", got)
	}
	if got[0].Type != CommandOutputEventType || got[0].ToolCallID != "call_observable" || got[0].CommandStream != "stdout" || got[0].Delta != "hello" {
		t.Fatalf("unexpected stdout event: %+v", got[0])
	}
	if got[1].Type != CommandOutputEventType || got[1].CommandStream != "stderr" || got[1].Delta != "warn" || !got[1].Truncated {
		t.Fatalf("unexpected stderr event: %+v", got[1])
	}
}

func TestToolRuntimeEmitsPatchPreviewBeforeApplyPatchApproval(t *testing.T) {
	root := t.TempDir()
	patch := "*** Begin Patch\n" +
		"*** Add File: note.md\n" +
		"+hello\n" +
		"*** End Patch"

	var got []AwakenEvent
	runtime := NewToolRuntimeWithConfig(testApprovalAwaiter{
		decision: ApprovalDecisionRejected,
		events:   &got,
	}, ToolRuntimeConfig{
		PermissionContext: tool.NewPermissionContext(root, []string{root}),
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_patch_preview",
		Function: convcontext.FunctionCall{
			Name:      "apply_patch",
			Arguments: `{"patch":` + mustJSONQuote(t, patch) + `}`,
		},
	})

	if !result.IsError || !strings.Contains(result.Content, "审批拒绝") {
		t.Fatalf("expected rejected apply_patch result, got %+v", result)
	}
	if len(got) < 2 {
		t.Fatalf("expected patch preview and approval events, got %+v", got)
	}
	if got[0].Type != PatchUpdatedEventType {
		t.Fatalf("expected first event to be patch updated, got %+v", got)
	}
	if got[0].ToolCallID != "call_patch_preview" || got[0].Patch != patch || got[0].PatchSummary != "新增 1 个文件" {
		t.Fatalf("unexpected patch preview event: %+v", got[0])
	}
	if len(got[0].ApprovalAffectedPaths) != 1 || got[0].ApprovalAffectedPaths[0] != "note.md" {
		t.Fatalf("unexpected affected paths: %+v", got[0].ApprovalAffectedPaths)
	}
	for _, event := range got {
		if event.Type == ArtifactCreatedEventType || event.Type == ArtifactUpdatedEventType {
			t.Fatalf("expected rejected patch not to emit changed-file artifact, got %+v", event)
		}
	}
}

func TestToolRuntimeApplyPatchResultIncludesCheckpoint(t *testing.T) {
	root := t.TempDir()
	patch := "*** Begin Patch\n" +
		"*** Add File: note.md\n" +
		"+hello\n" +
		"*** End Patch"

	var got []AwakenEvent
	runtime := NewToolRuntimeWithConfig(testApprovalAwaiter{
		decision: ApprovalDecisionApproved,
		events:   &got,
	}, ToolRuntimeConfig{
		PermissionContext: tool.NewPermissionContext(root, []string{root}),
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_patch_checkpoint",
		Function: convcontext.FunctionCall{
			Name:      "apply_patch",
			Arguments: `{"patch":` + mustJSONQuote(t, patch) + `}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected apply_patch to succeed, got %+v", result)
	}
	if !strings.Contains(result.Content, "Checkpoint: checkpoint_") || !strings.Contains(result.Content, "(0 existing, 1 missing)") {
		t.Fatalf("expected checkpoint summary in tool result, got %q", result.Content)
	}

	var sawFileArtifact bool
	for _, event := range got {
		if event.Type == ArtifactCreatedEventType && event.ArtifactPath == "note.md" && event.ArtifactKind == "added" {
			sawFileArtifact = true
		}
	}
	if !sawFileArtifact {
		t.Fatalf("expected added file artifact, got %+v", got)
	}
}

func TestToolRuntimeDeniesBeforeExecution(t *testing.T) {
	var observed string
	toolName := "runtime_denied_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:       toolName,
		observe:    &observed,
		permission: tool.DenyPermission("policy denied test tool"),
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	result := NewToolRuntime(nil).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_denied",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if !result.IsError || !strings.Contains(result.Content, "policy denied test tool") {
		t.Fatalf("expected policy denial, got %+v", result)
	}
	if observed != "" {
		t.Fatalf("expected denied tool not to execute, observed %q", observed)
	}
}

func TestToolRuntimePassesPermissionContextToTools(t *testing.T) {
	var observed tool.PermissionContext
	root := t.TempDir()
	writable := t.TempDir()
	toolName := "runtime_policy_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:          toolName,
		observePolicy: &observed,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	runtime := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		PermissionContext: tool.NewPermissionContext(root, []string{writable}),
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_policy",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected tool to execute, got %+v", result)
	}
	if observed.WorkspaceRoot != root || len(observed.WritableRoots) != 1 || observed.WritableRoots[0] != writable {
		t.Fatalf("unexpected permission context: %+v", observed)
	}
}

func TestToolRuntimeApproveBeforeExecute(t *testing.T) {
	var executeCount int
	toolName := "runtime_approve_before_execute_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:         toolName,
		permission:   tool.AskPermission("test tool requires approval", tool.ApprovalRequest{}),
		executeCount: &executeCount,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	runtime := NewToolRuntime(testApprovalAwaiter{decision: ApprovalDecisionApproved})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_approved",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected approved tool to succeed, got %+v", result)
	}
	if executeCount != 1 {
		t.Fatalf("expected tool to execute exactly once, got %d", executeCount)
	}
}

func TestToolRuntimeRejectDoesNotExecute(t *testing.T) {
	var executeCount int
	toolName := "runtime_reject_no_execute_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:         toolName,
		permission:   tool.AskPermission("test tool requires approval", tool.ApprovalRequest{}),
		executeCount: &executeCount,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	runtime := NewToolRuntime(testApprovalAwaiter{decision: ApprovalDecisionRejected})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_rejected",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if executeCount != 0 {
		t.Fatalf("expected rejected tool not to execute, got %d executions", executeCount)
	}
	if !result.IsError || result.Content != "工具执行已被审批拒绝" {
		t.Fatalf("expected rejection error result, got %+v", result)
	}
}

func TestToolRuntimeRejectResultFedBack(t *testing.T) {
	toolName := "runtime_reject_fed_back_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:       toolName,
		permission: tool.AskPermission("test tool requires approval", tool.ApprovalRequest{}),
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	callID := "call_rejected_fed_back"
	runtime := NewToolRuntime(testApprovalAwaiter{decision: ApprovalDecisionRejected})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: callID,
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if !result.IsError || result.Content != "工具执行已被审批拒绝" {
		t.Fatalf("expected rejection error result, got %+v", result)
	}

	contextResults := toolExecutionResultsToContext([]ToolExecutionResult{result})
	if len(contextResults) != 1 {
		t.Fatalf("expected 1 context result, got %d", len(contextResults))
	}
	if contextResults[0].ToolCallID != callID || contextResults[0].Name != toolName || contextResults[0].Content != "工具执行已被审批拒绝" {
		t.Fatalf("expected result to be fed back to conversation, got %+v", contextResults[0])
	}
}

type testApprovalAwaiter struct {
	decision ApprovalDecision
	events   *[]AwakenEvent
}

func (t testApprovalAwaiter) Emit(event AwakenEvent) {
	if t.events != nil {
		*t.events = append(*t.events, event)
	}
}

func (t testApprovalAwaiter) AwaitApproval(context.Context, ApprovalRequest) (ApprovalDecision, error) {
	if t.events != nil {
		*t.events = append(*t.events, AwakenEvent{Type: ApprovalRequestedEventType})
	}
	return t.decision, nil
}

func mustRegisterRuntimeTool(t *testing.T, candidate tool.Tool) {
	t.Helper()
	if err := tool.RegisterTool(candidate); err != nil {
		t.Fatalf("failed to register test tool %s: %v", candidate.Name(), err)
	}
}

func mustJSONQuote(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("failed to quote JSON string: %v", err)
	}
	return string(data)
}
