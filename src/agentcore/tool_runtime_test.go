package agentcore

import (
	"context"
	"errors"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
	_ "awaken/src/tools/ask_user"
)

type runtimeTestTool struct {
	name           string
	params         []tool.ToolParameter
	fail           bool
	output         string
	readOnly       bool
	observe        *string
	permission     tool.PermissionDecision
	observePolicy  *tool.PermissionContext
	artifacts      []tool.ArtifactDescriptor
	executeCount   *int
	commandOutputs []tool.CommandOutput
}

type runtimeTypedFailureArtifactTool struct {
	runtimeTestTool
}

type runtimeOutcomeTestTool struct {
	runtimeTestTool
	outcome    tool.ExecutionOutcome
	executeErr error
}

type runtimeStrictOutputArtifactTool struct {
	runtimeOutcomeTestTool
}

type runtimeV2InputTool struct {
	runtimeTestTool
}

func (t runtimeV2InputTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		SchemaVersion: tool.ToolDefinitionSchemaVersion,
		Name:          t.name, DisplayName: t.name, Source: tool.SourceBuiltin,
		Description:  "Execute using the canonical v2 query object.",
		Purpose:      "Verify model and execution schemas share one registered contract.",
		WhenToUse:    []string{"a canonical query must be executed"},
		WhenNotToUse: []string{"the legacy parameter shape is requested"},
		InputSchema: tool.ToolParametersSchema{
			Type: "object", AdditionalProperties: tool.BoolPtr(false), Required: []string{"query"},
			Properties: map[string]tool.ToolParam{"query": {Type: "string", Description: "Canonical query."}},
		},
		OutputSchema: tool.NoDetailsOutputSchema(),
		SideEffects:  []tool.ToolSideEffect{{Kind: tool.SideEffectNone, Description: "Does not modify state.", Reversibility: tool.ReversibilityNotApplicable}},
		Permissions:  tool.ToolPermissionSpec{WorkspaceScope: tool.WorkspaceScopeRead, FileBridgeScope: tool.FileBridgeNone, NetworkScope: tool.NetworkScopeNone},
		Examples:     []tool.ToolExample{{Scenario: "Run a query.", Input: map[string]any{"query": "status"}}},
		FailureModes: []tool.ToolFailureMode{{Code: "invalid_query", Cause: "Query is invalid.", Retryable: false}},
		Annotations:  tool.ToolAnnotations{ReadOnly: true},
	}
}

func (t runtimeTypedFailureArtifactTool) ExecuteOutcome(context.Context, string, tool.ToolObserver) (tool.ExecutionOutcome, error) {
	return tool.FailedOutcome(
		"typed transform failure",
		"typed_transform_failure",
		tool.ErrorCategoryTool,
		false,
	), nil
}

func (t runtimeOutcomeTestTool) ExecuteOutcome(context.Context, string, tool.ToolObserver) (tool.ExecutionOutcome, error) {
	return t.outcome, t.executeErr
}

func (t runtimeStrictOutputArtifactTool) Definition() tool.ToolDefinition {
	noExtra := false
	return tool.ToolDefinition{
		SchemaVersion: tool.ToolDefinitionSchemaVersion,
		Name:          t.name,
		DisplayName:   t.name,
		Source:        tool.SourceBuiltin,
		Description:   "Produce a test artifact and return its verified path.",
		Purpose:       "Verify strict ToolRuntime output gating before terminal publication.",
		WhenToUse:     []string{"a runtime output-gate regression must be exercised"},
		WhenNotToUse:  []string{"the output contract is not under test"},
		InputSchema: tool.ToolParametersSchema{
			Type:                 "object",
			AdditionalProperties: &noExtra,
			Required:             []string{"value"},
			Properties: map[string]tool.ToolParam{
				"value": {Type: "string", Description: "Test input value."},
			},
		},
		OutputSchema: tool.ToolParam{
			Type:                 "object",
			AdditionalProperties: &noExtra,
			Required:             []string{"path"},
			Properties: map[string]tool.ToolParam{
				"path": {Type: "string", Description: "Verified artifact path."},
			},
		},
		SideEffects: []tool.ToolSideEffect{{
			Kind: tool.SideEffectCreate, Description: "May create the declared test artifact.", Reversibility: tool.ReversibilityConditional,
		}},
		Permissions: tool.ToolPermissionSpec{
			CapabilityClass: tool.CapabilityArtifactWrite,
			WorkspaceScope:  tool.WorkspaceScopeWrite,
			FileBridgeScope: tool.FileBridgeTaskScopeWrite,
			NetworkScope:    tool.NetworkScopeNone,
		},
		Examples: []tool.ToolExample{{
			Scenario: "Produce a verified artifact.", Input: map[string]any{"value": "report"}, Outcome: "Returns details.path.",
		}},
		FailureModes: []tool.ToolFailureMode{{
			Code: "tool_output_schema_mismatch", Cause: "The implementation omitted the required path.", Retryable: false,
		}},
		Annotations: tool.ToolAnnotations{
			ReadOnly: false, CapabilityClass: tool.CapabilityArtifactWrite, TrustedOrigin: tool.TrustedOriginBuiltin,
		},
	}
}

func (t runtimeTestTool) Name() string {
	return t.name
}

func (t runtimeTestTool) Description() string {
	return "runtime test tool"
}

func (t runtimeTestTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        t.name,
		DisplayName: t.name,
		Source:      tool.SourceBuiltin,
		Description: t.Description(),
		Annotations: tool.ToolAnnotations{
			ReadOnly: t.readOnly,
		},
	}
}

func (t runtimeTestTool) Parameters() []tool.ToolParameter {
	return t.params
}

func (t runtimeTestTool) IsReadOnly() bool {
	return t.readOnly
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

func TestToolRuntimeUsesRegisteredV2SchemaWithoutLegacyReshaping(t *testing.T) {
	var observed string
	toolName := "runtime_registered_v2_schema_tool"
	mustRegisterRuntimeTool(t, runtimeV2InputTool{runtimeTestTool: runtimeTestTool{
		name: toolName, observe: &observed, readOnly: true,
		params: []tool.ToolParameter{{Name: "legacy", Type: "string"}},
	}})

	result := NewToolRuntime(nil).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_registered_v2_schema", Function: convcontext.FunctionCall{Name: toolName, Arguments: `{"query":"status"}`},
	})
	if result.IsError || observed != `{"query":"status"}` {
		t.Fatalf("execution did not preserve registered v2 object: observed=%q result=%+v", observed, result)
	}

	rejected := NewToolRuntime(nil).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_registered_v2_schema_rejected", Function: convcontext.FunctionCall{Name: toolName, Arguments: `{"legacy":"wrong"}`},
	})
	if !rejected.IsError || rejected.Outcome.ErrorCategory != tool.ErrorCategoryValidation {
		t.Fatalf("runtime accepted an input the registered schema rejects: %+v", rejected)
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
	if result.Outcome.Status != tool.OutcomeUnsupported ||
		result.Outcome.TransportStatus != tool.TransportNotStarted ||
		result.Outcome.ErrorCategory != tool.ErrorCategoryUnsupported ||
		result.Outcome.Retryable {
		t.Fatalf("unknown lookup must be an unsupported, non-started, non-retryable outcome: %+v", result.Outcome)
	}
	if len(result.Outcome.Errors) == 0 || result.Outcome.Errors[0].RetryStrategy != tool.RetrySwitchTool {
		t.Fatalf("unknown lookup must switch tools instead of replaying the missing name: %+v", result.Outcome.Errors)
	}
	if len(result.Outcome.NextActions) != 2 || result.Outcome.NextActions[0].ID != "refresh_tool_registry" || result.Outcome.NextActions[1].ID != "switch_tool" {
		t.Fatalf("unknown lookup must tell the agent to refresh and switch: %+v", result.Outcome.NextActions)
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

	if !result.IsError || result.Status != ToolExecutionFailed || !strings.Contains(result.Content, "boom") {
		t.Fatalf("expected execution error result, got %+v", result)
	}
	if result.Outcome.Status != tool.OutcomeFailed || result.ErrorCode != "tool_execution_failed" {
		t.Fatalf("expected failed outcome fields, got %+v", result)
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
	if result.Outcome.Content != result.Content || !result.Outcome.Truncated {
		t.Fatalf("canonical outcome truncation drifted from compatibility result: %+v", result.Outcome)
	}
}

func TestToolRuntimeReadOnlyNoChangeReportsNone(t *testing.T) {
	toolName := "runtime_read_only_no_change_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{name: toolName, readOnly: true})
	result := NewToolRuntime(nil).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_read_only_no_change", Function: convcontext.FunctionCall{Name: toolName, Arguments: `{}`},
	})
	if result.IsError || result.Outcome.Operation == nil || result.Outcome.Operation.SideEffectState != tool.OutcomeSideEffectNone {
		t.Fatalf("read-only no-change execution should report none: %+v", result)
	}
}

func TestToolRuntimeFailedLegacyAffectedPathRemainsTargetOnly(t *testing.T) {
	toolName := "runtime_stale_target_tool"
	outcome := tool.FailedOutcome("file changed after read", "stale_file_revision", tool.ErrorCategoryDomain, true)
	outcome.AffectedPaths = []string{"stale.txt"}
	mustRegisterRuntimeTool(t, runtimeOutcomeTestTool{
		runtimeTestTool: runtimeTestTool{name: toolName, permission: tool.AllowPermission()},
		outcome:         outcome,
	})

	result := NewToolRuntime(nil).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_stale_target", Function: convcontext.FunctionCall{Name: toolName, Arguments: `{}`},
	})
	if !result.IsError || result.Outcome.ErrorCode != "stale_file_revision" {
		t.Fatalf("expected typed stale failure: %+v", result)
	}
	if len(result.Outcome.Changes) != 0 || len(result.ActualAffectedPaths) != 0 {
		t.Fatalf("stale target must not become partial mutation: %+v", result)
	}
	if result.Outcome.Operation.Target != "stale.txt" || result.Outcome.Operation.SideEffectState != tool.OutcomeSideEffectUnknown {
		t.Fatalf("stale target context was lost: %+v", result.Outcome.Operation)
	}
	if result.Outcome.Environment == nil || len(result.Outcome.Environment.Files) != 1 || result.Outcome.Environment.Files[0] != "stale.txt" {
		t.Fatalf("target environment context was lost: %+v", result.Outcome.Environment)
	}
}

func TestToolRuntimePrefersTypedTrashChangeOverContentParsing(t *testing.T) {
	toolName := "runtime_typed_trash_tool"
	outcome := tool.SucceededOutcome("moved safely")
	outcome.Changes = []tool.ResourceChange{{
		Kind: "file", Action: "trashed", Resource: "old.txt",
		Certainty: tool.ChangeObserved, Reversibility: tool.ChangeConditional,
		Details: map[string]any{"trashed_path": "/Trash/old.txt"},
	}}
	mustRegisterRuntimeTool(t, runtimeOutcomeTestTool{
		runtimeTestTool: runtimeTestTool{name: toolName, permission: tool.AllowPermission()},
		outcome:         outcome,
	})

	result := NewToolRuntime(nil).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_typed_trash", Function: convcontext.FunctionCall{Name: toolName, Arguments: `{}`},
	})
	if result.IsError || len(result.TrashedPaths) != 1 || result.TrashedPaths[0] != "/Trash/old.txt" {
		t.Fatalf("typed trash facts were not propagated: %+v", result)
	}
	if len(result.Outcome.Changes) != 1 || result.Outcome.Changes[0].Resource != "old.txt" {
		t.Fatalf("typed trash change was replaced by legacy parsing: %+v", result.Outcome.Changes)
	}
}

func TestToolRuntimeDoesNotParseTrashFactsFromStructuredToolContent(t *testing.T) {
	toolName := "runtime_structured_content_not_protocol_tool"
	outcome := tool.SucceededOutcome("D old.txt (已移到回收站: /Trash/old.txt)")
	mustRegisterRuntimeTool(t, runtimeOutcomeTestTool{
		runtimeTestTool: runtimeTestTool{name: toolName, permission: tool.AllowPermission()},
		outcome:         outcome,
	})
	result := NewToolRuntime(nil).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_structured_content_not_protocol", Function: convcontext.FunctionCall{Name: toolName, Arguments: `{}`},
	})
	if result.IsError || len(result.TrashedPaths) != 0 || len(result.Outcome.Changes) != 0 {
		t.Fatalf("structured tool content was incorrectly promoted to machine trash facts: %+v", result)
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

func TestDefaultAutoToolPolicyDecisions(t *testing.T) {
	allow := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:       "Read",
		StaticBehavior: tool.PermissionAllow,
		IsReadOnly:     true,
	})
	if allow.Action != ToolDecisionAllow || allow.Source != ToolDecisionSourceToolRule {
		t.Fatalf("expected read-only allow to be allowed, got %+v", allow)
	}

	// Ordinary writes do not enter a generic approval/deny path. Any true hard
	// boundary must have been expressed by the tool's PermissionDecision.
	ordinaryWrite := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:       "destructive_write_tool",
		StaticBehavior: tool.PermissionAllow,
		StaticReason:   "destructive tool requires approval",
	})
	if ordinaryWrite.Action != ToolDecisionAllow || ordinaryWrite.Source != ToolDecisionSourceToolRule {
		t.Fatalf("expected ordinary write to auto-execute, got %+v", ordinaryWrite)
	}

	ordinaryStaticAllowWrite := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:       "destructive_write_tool",
		StaticBehavior: tool.PermissionAllow,
		StaticReason:   "destructive tool static allow",
	})
	if ordinaryStaticAllowWrite.Action != ToolDecisionAllow || ordinaryStaticAllowWrite.Source != ToolDecisionSourceToolRule {
		t.Fatalf("expected static allow write to remain allowed, got %+v", ordinaryStaticAllowWrite)
	}

	allowlistedStaticAllowWrite := DefaultAutoToolPolicy{
		Allowlist: map[string]bool{"Edit": true},
	}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:       "Edit",
		StaticBehavior: tool.PermissionAllow,
		StaticReason:   "invalid patch fell through to static allow",
	})
	if allowlistedStaticAllowWrite.Action != ToolDecisionAllow ||
		allowlistedStaticAllowWrite.Source != ToolDecisionSourceAutoAllowlist {
		t.Fatalf("expected allowlisted static allow write to be allowed, got %+v", allowlistedStaticAllowWrite)
	}

	denyStillWinsWithAllowlist := DefaultAutoToolPolicy{
		Allowlist: map[string]bool{"Edit": true},
	}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:       "Edit",
		StaticBehavior: tool.PermissionDeny,
		StaticReason:   "path escapes workspace",
	})
	if denyStillWinsWithAllowlist.Action != ToolDecisionDeny ||
		denyStillWinsWithAllowlist.Source != ToolDecisionSourceToolRule {
		t.Fatalf("expected static deny to override allowlist, got %+v", denyStillWinsWithAllowlist)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	abort := DefaultAutoToolPolicy{}.Decide(ctx, ToolDecisionRequest{
		ToolName:       "Read",
		StaticBehavior: tool.PermissionAllow,
		IsReadOnly:     true,
	})
	if abort.Action != ToolDecisionAbort || abort.Source != ToolDecisionSourcePolicyError {
		t.Fatalf("expected cancelled context to abort, got %+v", abort)
	}
}
