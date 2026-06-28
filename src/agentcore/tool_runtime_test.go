package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
	"awaken/src/toolresults"
	_ "awaken/src/tools/apply_patch"
	_ "awaken/src/tools/shell"
	_ "awaken/src/tools/todo_write"
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

type runtimePlainTool struct {
	name         string
	params       []tool.ToolParameter
	output       string
	readOnly     bool
	executeCount *int
}

type runtimeBasicTool struct {
	name         string
	params       []tool.ToolParameter
	readOnly     bool
	executeCount *int
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

func (t runtimePlainTool) Name() string {
	return t.name
}

func (t runtimePlainTool) Description() string {
	return "runtime plain tool"
}

func (t runtimePlainTool) Parameters() []tool.ToolParameter {
	return t.params
}

func (t runtimePlainTool) Execute(context.Context, string) (string, error) {
	if t.executeCount != nil {
		*t.executeCount++
	}
	if t.output != "" {
		return t.output, nil
	}
	return "plain", nil
}

func (t runtimePlainTool) IsReadOnly() bool {
	return t.readOnly
}

func (t runtimeBasicTool) Name() string {
	return t.name
}

func (t runtimeBasicTool) Description() string {
	return "runtime basic tool"
}

func (t runtimeBasicTool) Parameters() []tool.ToolParameter {
	return t.params
}

func (t runtimeBasicTool) Execute(ctx context.Context, input string) (string, error) {
	if t.executeCount != nil {
		*t.executeCount++
	}
	return "ran:" + input, nil
}

func (t runtimeBasicTool) IsReadOnly() bool {
	return t.readOnly
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

func TestToolRuntimeRejectsToolBlockedByRunProfile(t *testing.T) {
	toolName := "runtime_blocked_tool"
	executions := 0
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:         toolName,
		executeCount: &executions,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	result := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		ToolAccessPolicy: RunProfile{
			DisallowedTools: []string{toolName},
		}.ToolAccessPolicy(),
	}).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_blocked",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if !result.IsError || !strings.Contains(result.Content, "not allowed for this run profile") {
		t.Fatalf("expected blocked tool error, got %+v", result)
	}
	if executions != 0 {
		t.Fatalf("expected blocked tool not to execute, got %d executions", executions)
	}
}

func TestToolRuntimeAskModeRequiresApprovalForNonReadOnlyToolWithoutChecker(t *testing.T) {
	toolName := "runtime_ask_mode_tool"
	executeCount := 0
	mustRegisterRuntimeTool(t, runtimeBasicTool{
		name:         toolName,
		executeCount: &executeCount,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	result := NewToolRuntimeWithConfig(testApprovalAwaiter{decision: ApprovalDecisionApproved}, ToolRuntimeConfig{
		PermissionMode: PermissionModeAsk,
	}).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_ask_mode",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected approved ask-mode tool to succeed, got %+v", result)
	}
	if executeCount != 1 {
		t.Fatalf("expected ask-mode tool to execute after approval, got %d executions", executeCount)
	}
}

func TestToolRuntimeAllowModeBypassesApprovalButStillExecutes(t *testing.T) {
	toolName := "runtime_allow_mode_tool"
	executeCount := 0
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:         toolName,
		permission:   tool.AskPermission("tool would normally ask", tool.ApprovalRequest{}),
		executeCount: &executeCount,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	result := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		PermissionMode: PermissionModeAllow,
	}).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_allow_mode",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected allow-mode tool to succeed without approval, got %+v", result)
	}
	if executeCount != 1 {
		t.Fatalf("expected allow-mode tool to execute exactly once, got %d", executeCount)
	}
}

func TestToolRuntimeDenyModeBlocksNonReadOnlyToolWithoutChecker(t *testing.T) {
	toolName := "runtime_deny_mode_tool"
	executeCount := 0
	mustRegisterRuntimeTool(t, runtimeBasicTool{
		name:         toolName,
		executeCount: &executeCount,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	result := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		PermissionMode: PermissionModeDeny,
	}).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_deny_mode",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if !result.IsError || !strings.Contains(result.Content, "permission_mode=deny") {
		t.Fatalf("expected deny-mode tool to be blocked, got %+v", result)
	}
	if executeCount != 0 {
		t.Fatalf("expected deny-mode tool not to execute, got %d executions", executeCount)
	}
}

func TestToolRuntimeAllowModeDoesNotOverrideExplicitToolDeny(t *testing.T) {
	toolName := "runtime_allow_mode_respects_deny_tool"
	executeCount := 0
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:         toolName,
		permission:   tool.DenyPermission("tool-specific deny"),
		executeCount: &executeCount,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	result := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		PermissionMode: PermissionModeAllow,
	}).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_allow_mode_respects_deny",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if !result.IsError || !strings.Contains(result.Content, "tool-specific deny") {
		t.Fatalf("expected allow-mode to preserve tool deny, got %+v", result)
	}
	if executeCount != 0 {
		t.Fatalf("expected denied tool not to execute, got %d executions", executeCount)
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

func TestToolRuntimeLeavesSmallToolResultUnstored(t *testing.T) {
	toolName := "runtime_small_output_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:   toolName,
		output: "small output",
	})
	storeDir := filepath.Join(t.TempDir(), "tool-results")
	runtime := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		ToolResultStore:    toolresults.NewStore(toolresults.Config{Dir: storeDir}),
		ToolResultMaxChars: 20,
	})

	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_small",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{}`,
		},
	})

	if result.IsError || result.Truncated || result.Content != "small output" || result.ToolResultHandle != nil {
		t.Fatalf("expected unchanged small result, got %+v", result)
	}
	if _, err := os.Stat(storeDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected small result not to create store dir, stat err=%v", err)
	}
}

func TestToolRuntimePersistsLargeToolResult(t *testing.T) {
	toolName := "runtime_large_output_tool"
	fullOutput := strings.Repeat("x", 25000) + "COMPLETE_TAIL_SHOULD_ONLY_BE_IN_STORE"
	workspace := t.TempDir()
	store := toolresults.NewStore(toolresults.Config{
		Dir:           filepath.Join(t.TempDir(), "tool-results"),
		WorkspaceRoot: workspace,
	})
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:   toolName,
		output: fullOutput,
	})

	result := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		PermissionContext:      tool.NewPermissionContext(workspace, []string{workspace}),
		ToolResultStore:        store,
		ToolResultMaxChars:     20000,
		ToolResultPreviewChars: 128,
	}).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_large",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{}`,
		},
	})

	if result.IsError || !result.Truncated || result.ToolResultHandle == nil {
		t.Fatalf("expected persisted success result, got %+v", result)
	}
	if !strings.Contains(result.Content, "<persisted-tool-result>") || !strings.Contains(result.Content, "handle_id: "+result.ToolResultHandle.ID) {
		t.Fatalf("expected handle metadata in result content, got %q", result.Content)
	}
	if strings.Contains(result.Content, "COMPLETE_TAIL_SHOULD_ONLY_BE_IN_STORE") {
		t.Fatalf("expected inline result content not to contain full tail")
	}
	fullStored, err := store.Read(result.ToolResultHandle.ID)
	if err != nil {
		t.Fatalf("read persisted result: %v", err)
	}
	if fullStored != fullOutput {
		t.Fatalf("expected full persisted output")
	}
	if isPathWithin(result.ToolResultHandle.Path, workspace) {
		t.Fatalf("expected persisted path outside workspace, got %q in %q", result.ToolResultHandle.Path, workspace)
	}
}

func TestToolRuntimeDefaultRuntimeStillTruncatesLargeToolResult(t *testing.T) {
	toolName := "runtime_large_output_default_truncation_tool"
	fullOutput := strings.Repeat("x", 25000) + "COMPLETE_TAIL_SHOULD_STAY_INLINE"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:   toolName,
		output: fullOutput,
	})

	result := NewToolRuntime(nil).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_large_default",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{}`,
		},
	})

	if result.IsError || !result.Truncated || result.ToolResultHandle != nil {
		t.Fatalf("expected default runtime to truncate inline, got %+v", result)
	}
	if !strings.Contains(result.Content, "truncated") || !strings.Contains(result.Content, "COMPLETE_TAIL_SHOULD_STAY_INLINE") {
		t.Fatalf("expected inline truncated content with preserved tail, got %q", result.Content)
	}
	if strings.Contains(result.Content, "<persisted-tool-result>") {
		t.Fatalf("expected default runtime not to emit persisted handle content, got %q", result.Content)
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

func TestToolRuntimeUndeclaredNonReadOnlyToolDefaultsToApproval(t *testing.T) {
	toolName := "runtime_plain_mutating_tool"
	mustRegisterRuntimeTool(t, runtimePlainTool{
		name: toolName,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	result := NewToolRuntime(nil).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_plain_missing_awaiter",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if !result.IsError || !strings.Contains(result.Content, "no approval awaiter") {
		t.Fatalf("expected undeclared non-read-only tool to require approval, got %+v", result)
	}
}

func TestToolRuntimeUndeclaredReadOnlyToolRemainsAllowed(t *testing.T) {
	var executeCount int
	toolName := "runtime_plain_read_only_tool"
	mustRegisterRuntimeTool(t, runtimePlainTool{
		name:         toolName,
		readOnly:     true,
		executeCount: &executeCount,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	result := NewToolRuntime(nil).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_plain_read_only",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected undeclared read-only tool to succeed, got %+v", result)
	}
	if executeCount != 1 {
		t.Fatalf("expected read-only tool to execute once, got %d", executeCount)
	}
}

func TestToolRuntimePermissionPolicyAllowListBypassesApproval(t *testing.T) {
	var executeCount int
	toolName := "runtime_allowlisted_plain_tool"
	mustRegisterRuntimeTool(t, runtimePlainTool{
		name:         toolName,
		executeCount: &executeCount,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	runtime := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		PermissionPolicy: tool.PermissionPolicy{
			AllowTools: []string{toolName},
		},
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_allowlisted",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected allowlisted tool to succeed, got %+v", result)
	}
	if executeCount != 1 {
		t.Fatalf("expected allowlisted tool to execute once, got %d", executeCount)
	}
}

func TestToolRuntimePermissionPolicyDenyListOverridesReadOnly(t *testing.T) {
	toolName := "runtime_denylisted_read_only_tool"
	mustRegisterRuntimeTool(t, runtimePlainTool{
		name:     toolName,
		readOnly: true,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	runtime := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		PermissionPolicy: tool.PermissionPolicy{
			DenyTools: []string{toolName},
		},
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_denylisted",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if !result.IsError || !strings.Contains(result.Content, "权限拒绝") {
		t.Fatalf("expected denylisted tool to be rejected, got %+v", result)
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

func TestToolRuntimePersistsOnlyFinalResultNotCommandOutputEvents(t *testing.T) {
	toolName := "runtime_observable_large_result_tool"
	fullOutput := strings.Repeat("result-line\n", 1000) + "FINAL_RESULT_TAIL"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:   toolName,
		output: fullOutput,
		commandOutputs: []tool.CommandOutput{
			{Stream: "stdout", Delta: "streamed output tail"},
		},
	})

	var got []AwakenEvent
	runtime := NewToolRuntimeWithConfig(EventSinkFunc(func(event AwakenEvent) {
		got = append(got, event)
	}), ToolRuntimeConfig{
		ToolResultStore:        toolresults.NewStore(toolresults.Config{Dir: filepath.Join(t.TempDir(), "tool-results")}),
		ToolResultMaxChars:     100,
		ToolResultPreviewChars: 24,
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_observable_large",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{}`,
		},
	})

	if result.IsError || !result.Truncated || result.ToolResultHandle == nil {
		t.Fatalf("expected persisted final result, got %+v", result)
	}
	if len(got) != 1 || got[0].Type != CommandOutputEventType || got[0].Delta != "streamed output tail" {
		t.Fatalf("expected command output event to remain unchanged, got %+v", got)
	}
	if strings.Contains(result.Content, "FINAL_RESULT_TAIL") {
		t.Fatalf("expected final result content to be preview plus handle, got %q", result.Content)
	}
}

func TestToolRuntimeRedactsShellDisplayEventsWithoutChangingRawResult(t *testing.T) {
	stdoutSecret := "sk-proj-abcdefghijklmnopqrstuvwxyz123456"
	stderrSecret := "abcdefghijklmnopqrstuvwxyz987654"
	command := "printf 'stdout " + stdoutSecret + "'; printf 'stderr Authorization: Bearer " + stderrSecret + "' >&2"
	input, err := json.Marshal(map[string]interface{}{
		"command": command,
	})
	if err != nil {
		t.Fatalf("failed to encode shell input: %v", err)
	}

	var got []AwakenEvent
	var requests []ApprovalRequest
	runtime := NewToolRuntime(testApprovalAwaiter{
		decision: ApprovalDecisionApproved,
		events:   &got,
		requests: &requests,
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_shell_redaction",
		Function: convcontext.FunctionCall{
			Name:      "shell",
			Arguments: string(input),
		},
	})

	if result.IsError {
		t.Fatalf("expected shell command to succeed, got %+v", result)
	}
	if !strings.Contains(result.Content, stdoutSecret) || !strings.Contains(result.Content, stderrSecret) {
		t.Fatalf("expected raw tool result to remain available for agent feedback, got %q", result.Content)
	}
	if len(requests) != 1 {
		t.Fatalf("expected one approval request, got %+v", requests)
	}
	assertNoDisplaySecret(t, requests[0].Description, stdoutSecret, stderrSecret)

	var stdoutDelta, stderrDelta string
	for _, event := range got {
		if event.Type != CommandOutputEventType {
			continue
		}
		switch event.CommandStream {
		case "stdout":
			stdoutDelta += event.Delta
		case "stderr":
			stderrDelta += event.Delta
		}
	}
	if stdoutDelta == "" || stderrDelta == "" {
		t.Fatalf("expected stdout and stderr command output events, got %+v", got)
	}
	assertNoDisplaySecret(t, stdoutDelta, stdoutSecret, stderrSecret)
	assertNoDisplaySecret(t, stderrDelta, stdoutSecret, stderrSecret)

	var toolResultEvents []AwakenEvent
	emitter := newRunEventEmitter("thread-redaction", EventSinkFunc(func(event AwakenEvent) {
		toolResultEvents = append(toolResultEvents, event)
	}))
	emitter.emitToolResult(result)
	if len(toolResultEvents) != 1 || toolResultEvents[0].Type != ToolResultEventType {
		t.Fatalf("expected one tool result event, got %+v", toolResultEvents)
	}
	assertNoDisplaySecret(t, toolResultEvents[0].Content, stdoutSecret, stderrSecret)
}

func TestToolRuntimeRedactsPatchPreviewDisplayEvent(t *testing.T) {
	root := t.TempDir()
	patchSecret := "sk-proj-abcdefghijklmnopqrstuvwxyz654321"
	patch := "*** Begin Patch\n" +
		"*** Add File: config.txt\n" +
		"+LLM_API_KEY=" + patchSecret + "\n" +
		"*** End Patch"

	var got []AwakenEvent
	runtime := NewToolRuntimeWithConfig(testApprovalAwaiter{
		decision: ApprovalDecisionRejected,
		events:   &got,
	}, ToolRuntimeConfig{
		PermissionContext: tool.NewPermissionContext(root, []string{root}),
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_patch_redaction",
		Function: convcontext.FunctionCall{
			Name:      "apply_patch",
			Arguments: `{"patch":` + mustJSONQuote(t, patch) + `}`,
		},
	})

	if !result.IsError || !strings.Contains(result.Content, "审批拒绝") {
		t.Fatalf("expected rejected apply_patch result, got %+v", result)
	}
	if len(got) == 0 || got[0].Type != PatchUpdatedEventType {
		t.Fatalf("expected first event to be patch preview, got %+v", got)
	}
	assertNoDisplaySecret(t, got[0].Patch, patchSecret)
	if !strings.Contains(got[0].Patch, "[REDACTED]") {
		t.Fatalf("expected patch preview to include redaction marker, got %q", got[0].Patch)
	}
}

func TestToolRuntimeEmitsPatchPreviewBeforeApplyPatchApproval(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "note.md")
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
	if got[0].DiffReview == nil || got[0].DiffReview.FileCounts.Added != 1 || got[0].DiffReview.LineCounts.Added != 1 {
		t.Fatalf("unexpected diff review metadata: %+v", got[0].DiffReview)
	}
	for _, event := range got {
		if event.Type == ArtifactCreatedEventType || event.Type == ArtifactUpdatedEventType {
			t.Fatalf("expected rejected patch not to emit changed-file artifact, got %+v", event)
		}
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected rejected approval not to write %s, stat err=%v", target, err)
	}
}

func TestToolRuntimeRecordsSecurityAuditForTools(t *testing.T) {
	var got []SecurityAuditEvent
	toolName := "runtime_audit_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name: toolName,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	runtime := NewToolRuntime(testApprovalAwaiter{auditEvents: &got})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_audit",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"Authorization: Bearer token-secret-123456 OPENAI_API_KEY=sk-secret123456"}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected audited tool to succeed, got %+v", result)
	}
	if len(got) != 2 {
		t.Fatalf("expected tool start and finish audit events, got %+v", got)
	}
	if got[0].Type != SecurityAuditToolExecutionStarted || got[1].Type != SecurityAuditToolExecutionFinished {
		t.Fatalf("unexpected audit event types: %+v", got)
	}
	if got[0].Action != SecurityAuditActionTool || got[1].Outcome != SecurityAuditOutcomeFinished {
		t.Fatalf("unexpected audit action/outcome: %+v", got)
	}
	for _, event := range got {
		if strings.Contains(event.InputPreview, "token-secret-123456") || strings.Contains(event.InputPreview, "sk-secret123456") {
			t.Fatalf("audit input preview leaked secret: %+v", event)
		}
	}
}

func TestToolRuntimeAuditDistinguishesShellAndFileMutation(t *testing.T) {
	root := t.TempDir()
	patch := "*** Begin Patch\n" +
		"*** Add File: note.md\n" +
		"+OPENAI_API_KEY=sk-secret123456\n" +
		"*** End Patch"

	var patchAudit []SecurityAuditEvent
	patchRuntime := NewToolRuntimeWithConfig(testApprovalAwaiter{
		decision:    ApprovalDecisionRejected,
		auditEvents: &patchAudit,
	}, ToolRuntimeConfig{
		PermissionContext: tool.NewPermissionContext(root, []string{root}),
	})
	patchResult := patchRuntime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_patch_audit",
		Function: convcontext.FunctionCall{
			Name:      "apply_patch",
			Arguments: `{"patch":` + mustJSONQuote(t, patch) + `}`,
		},
	})

	if !patchResult.IsError {
		t.Fatalf("expected rejected patch result, got %+v", patchResult)
	}
	if len(patchAudit) != 2 {
		t.Fatalf("expected patch start and finish audit events, got %+v", patchAudit)
	}
	if patchAudit[0].Action != SecurityAuditActionFileMutation || patchAudit[0].Summary != "新增 1 个文件" {
		t.Fatalf("unexpected patch audit event: %+v", patchAudit[0])
	}
	if len(patchAudit[0].AffectedPaths) != 1 || patchAudit[0].AffectedPaths[0] != "note.md" {
		t.Fatalf("unexpected patch affected paths: %+v", patchAudit[0].AffectedPaths)
	}
	if strings.Contains(patchAudit[0].InputPreview, "sk-secret123456") || strings.Contains(patchAudit[1].InputPreview, "sk-secret123456") {
		t.Fatalf("patch audit should not include patch content: %+v", patchAudit)
	}

	var shellAudit []SecurityAuditEvent
	shellRuntime := NewToolRuntimeWithConfig(testApprovalAwaiter{auditEvents: &shellAudit}, ToolRuntimeConfig{
		PermissionContext: tool.NewPermissionContext(root, []string{root}),
	})
	shellResult := shellRuntime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_shell_audit",
		Function: convcontext.FunctionCall{
			Name:      "shell",
			Arguments: `{"command":"echo Authorization: Bearer token-secret-123456"}`,
		},
	})

	if shellResult.IsError {
		t.Fatalf("expected shell audit command to succeed, got %+v", shellResult)
	}
	if len(shellAudit) != 2 {
		t.Fatalf("expected shell start and finish audit events, got %+v", shellAudit)
	}
	if shellAudit[0].Action != SecurityAuditActionShellCommand {
		t.Fatalf("expected shell command audit action, got %+v", shellAudit[0])
	}
	if !strings.Contains(shellAudit[0].InputPreview, "echo") {
		t.Fatalf("expected shell input preview to include command shape, got %+v", shellAudit[0])
	}
	if strings.Contains(shellAudit[0].InputPreview, "token-secret-123456") {
		t.Fatalf("shell audit preview leaked token: %+v", shellAudit[0])
	}
}

func TestToolRuntimeTodoWriteIsReadOnlyAndEmitsArtifact(t *testing.T) {
	var got []AwakenEvent
	runtime := NewToolRuntime(testApprovalAwaiter{
		decision: ApprovalDecisionApproved,
		events:   &got,
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_todo_write",
		Function: convcontext.FunctionCall{
			Name:      "todo_write",
			Arguments: `{"items":[{"id":"1","content":"Implement todo tool","status":"in_progress","priority":"high"},{"id":"2","content":"Run tests","status":"pending"}]}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected todo_write to execute without approval, got %+v", result)
	}
	if !strings.Contains(result.Content, `"artifact_id":"todo:current"`) {
		t.Fatalf("expected structured todo summary, got %q", result.Content)
	}

	var artifactEvent *AwakenEvent
	for i := range got {
		if got[i].Type == ApprovalRequestedEventType {
			t.Fatalf("todo_write should be read-only and not request approval, got events %+v", got)
		}
		if got[i].Type == ArtifactUpdatedEventType {
			artifactEvent = &got[i]
		}
	}
	if artifactEvent == nil {
		t.Fatalf("expected todo_write artifact event, got %+v", got)
	}
	if artifactEvent.ArtifactID != "todo:current" || artifactEvent.ArtifactKind != "todo" || artifactEvent.ArtifactStatus != ArtifactStatusReady {
		t.Fatalf("unexpected todo artifact event: %+v", artifactEvent)
	}
	if !strings.Contains(artifactEvent.ArtifactContent, `"kind": "todo"`) || !strings.Contains(artifactEvent.ArtifactContent, `"status": "in_progress"`) {
		t.Fatalf("unexpected todo artifact content: %q", artifactEvent.ArtifactContent)
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
	decision    ApprovalDecision
	events      *[]AwakenEvent
	requests    *[]ApprovalRequest
	auditEvents *[]SecurityAuditEvent
}

func (t testApprovalAwaiter) Emit(event AwakenEvent) {
	if t.events != nil {
		*t.events = append(*t.events, event)
	}
}

func (t testApprovalAwaiter) AwaitApproval(_ context.Context, request ApprovalRequest) (ApprovalDecision, error) {
	if t.requests != nil {
		*t.requests = append(*t.requests, request)
	}
	if t.events != nil {
		*t.events = append(*t.events, AwakenEvent{Type: ApprovalRequestedEventType})
	}
	return t.decision, nil
}

func (t testApprovalAwaiter) RecordSecurityAuditEvent(event SecurityAuditEvent) {
	if t.auditEvents != nil {
		*t.auditEvents = append(*t.auditEvents, SanitizeSecurityAuditEvent(event))
	}
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

func isPathWithin(path string, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func assertNoDisplaySecret(t *testing.T, value string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		if strings.Contains(value, secret) {
			t.Fatalf("display output leaked secret %q in %q", secret, value)
		}
	}
}
