package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
	"awaken/src/tool/profile"
)

type runtimeTestTool struct {
	name            string
	params          []tool.ToolParameter
	fail            bool
	output          string
	readOnly        bool
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

	if !result.IsError || result.Status != ToolExecutionFailed || !strings.Contains(result.Content, "工具执行失败") {
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

func TestDefaultAutoToolPolicyDecisions(t *testing.T) {
	allow := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:       "read_file",
		StaticBehavior: tool.PermissionAsk,
		IsReadOnly:     true,
	})
	if allow.Action != ToolDecisionAllow || allow.Source != ToolDecisionSourceWorkspaceBoundary {
		t.Fatalf("expected read-only ask to be allowed, got %+v", allow)
	}

	// Other destructive tools (not apply_patch) are still hard-denied in
	// headless mode on the PermissionAsk path.
	deny := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:       "destructive_write_tool",
		StaticBehavior: tool.PermissionAsk,
		StaticReason:   "destructive tool requires approval",
		IsDestructive:  true,
	})
	if deny.Action != ToolDecisionDeny || deny.Source != ToolDecisionSourceHeadlessDefault {
		t.Fatalf("expected destructive ask to be denied, got %+v", deny)
	}

	// Other destructive tools (not apply_patch) are still hard-denied on
	// the PermissionAllow path via deniesStaticAllowInHeadless.
	denyStaticAllowWrite := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:       "destructive_write_tool",
		StaticBehavior: tool.PermissionAllow,
		StaticReason:   "destructive tool static allow",
		IsDestructive:  true,
	})
	if denyStaticAllowWrite.Action != ToolDecisionDeny || denyStaticAllowWrite.Source != ToolDecisionSourceHeadlessDefault {
		t.Fatalf("expected destructive static allow to be denied in headless AUTO, got %+v", denyStaticAllowWrite)
	}

	allowlistedStaticAllowWrite := DefaultAutoToolPolicy{
		Allowlist: map[string]bool{"apply_patch": true},
	}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:       "apply_patch",
		StaticBehavior: tool.PermissionAllow,
		StaticReason:   "invalid patch fell through to static allow",
	})
	if allowlistedStaticAllowWrite.Action != ToolDecisionAllow ||
		allowlistedStaticAllowWrite.Source != ToolDecisionSourceAutoAllowlist {
		t.Fatalf("expected allowlisted static allow write to be allowed, got %+v", allowlistedStaticAllowWrite)
	}

	denyStillWinsWithAllowlist := DefaultAutoToolPolicy{
		Allowlist: map[string]bool{"apply_patch": true},
	}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:       "apply_patch",
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
		ToolName:       "read_file",
		StaticBehavior: tool.PermissionAsk,
		IsReadOnly:     true,
	})
	if abort.Action != ToolDecisionAbort || abort.Source != ToolDecisionSourcePolicyError {
		t.Fatalf("expected cancelled context to abort, got %+v", abort)
	}
}

// TestApplyPatchFileChainRelaxation covers the Task 4 relaxation: apply_patch
// is no longer hard-denied in headless mode. With workspace boundary,
// sensitive-path, and trash constraints in place, in-workspace mutations
// auto-execute. Out-of-writable-roots and sensitive paths are still denied.
func TestApplyPatchFileChainRelaxation(t *testing.T) {
	workspace := t.TempDir()

	// Normal in-workspace write (Add/Update): warning risk tone → allow.
	allowWrite := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:        "apply_patch",
		StaticBehavior:  tool.PermissionAsk,
		StaticReason:    "apply_patch will modify files",
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
		RiskTone:        "warning",
		AffectedPaths:   []string{"note.md"},
		NormalizedInput: "*** Begin Patch\n*** Add File: note.md\n+hello\n*** End Patch",
	})
	if allowWrite.Action != ToolDecisionAllow || allowWrite.Source != ToolDecisionSourceWorkspaceBoundary {
		t.Fatalf("expected in-workspace write to be allowed, got %+v", allowWrite)
	}

	// apply_patch with PermissionAllow (parse error fall-through): allow.
	// The executor will surface the parse error.
	allowStatic := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:       "apply_patch",
		StaticBehavior: tool.PermissionAllow,
		StaticReason:   "invalid patch fell through to static allow",
	})
	if allowStatic.Action != ToolDecisionAllow {
		t.Fatalf("expected apply_patch PermissionAllow to be allowed (parse error surfaces at execution), got %+v", allowStatic)
	}

	// Delete within workspace: danger risk tone + path within writable roots → allow.
	// Trash makes the delete recoverable.
	allowDelete := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:        "apply_patch",
		StaticBehavior:  tool.PermissionAsk,
		StaticReason:    "apply_patch will delete, move, or touch files outside writable roots",
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
		RiskTone:        "danger",
		AffectedPaths:   []string{"note.md"},
		NormalizedInput: "*** Begin Patch\n*** Delete File: note.md\n*** End Patch",
		IsDestructive:   true,
	})
	if allowDelete.Action != ToolDecisionAllow || allowDelete.Source != ToolDecisionSourceWorkspaceBoundary {
		t.Fatalf("expected in-workspace delete to be allowed (trash-recoverable), got %+v", allowDelete)
	}

	// Out-of-writable-roots write: danger risk tone + path outside workspace → deny.
	outside := t.TempDir()
	denyOutOfBounds := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:        "apply_patch",
		StaticBehavior:  tool.PermissionAsk,
		StaticReason:    "apply_patch will delete, move, or touch files outside writable roots",
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
		RiskTone:        "danger",
		AffectedPaths:   []string{filepath.Join(outside, "note.md")},
		NormalizedInput: "*** Begin Patch\n*** Add File: " + filepath.Join(outside, "note.md") + "\n+hello\n*** End Patch",
		IsDestructive:   true,
	})
	if denyOutOfBounds.Action != ToolDecisionDeny || denyOutOfBounds.Source != ToolDecisionSourceHeadlessDefault {
		t.Fatalf("expected out-of-writable-roots write to be denied, got %+v", denyOutOfBounds)
	}
	if !strings.Contains(denyOutOfBounds.Reason, "outside writable roots") {
		t.Fatalf("expected deny reason to mention outside writable roots, got %q", denyOutOfBounds.Reason)
	}

	// Sensitive path (.git/config): danger risk tone + sensitive path → deny.
	// Isolate HOME so the sensitive-path matcher doesn't trip over the
	// developer's real ~/.awaken.
	t.Setenv("HOME", workspace)
	denySensitive := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:        "apply_patch",
		StaticBehavior:  tool.PermissionAsk,
		StaticReason:    "apply_patch will delete, move, or touch files outside writable roots",
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
		RiskTone:        "danger",
		AffectedPaths:   []string{".git/config"},
		NormalizedInput: "*** Begin Patch\n*** Add File: .git/config\n+[core]\n*** End Patch",
		IsDestructive:   true,
	})
	if denySensitive.Action != ToolDecisionDeny || denySensitive.Source != ToolDecisionSourceHeadlessDefault {
		t.Fatalf("expected sensitive path to be denied, got %+v", denySensitive)
	}
	if !strings.Contains(denySensitive.Reason, "sensitive path") {
		t.Fatalf("expected deny reason to mention sensitive path, got %q", denySensitive.Reason)
	}
}

// SubTask 4.3 — Desktop UI smoke verification (manual):
//
// The unit tests above cover the policy layer. To verify end-to-end that no
// approval UI appears for in-workspace file mutations, follow the root
// README's "入口契约" to launch Awaken Desktop, then:
//
//  1. Start a new run in a workspace that has at least one regular file
//     (e.g. a temp dir with note.md containing "hello").
//  2. Ask the agent: "请用 apply_patch 把 note.md 的内容从 hello 改成 world".
//  3. Confirm:
//     - No approval dialog / awaiting_approval status appears.
//     - The AWAKEN_TOOL_PERMISSION_DECIDED event has action=allow,
//       source=workspace_boundary.
//     - note.md is actually modified to "world".
//  4. Ask the agent to create a new file (e.g. "新建 src/new.go"):
//     confirm it is created without approval.
//  5. Ask the agent to delete a file (e.g. "删除 src/new.go"):
//     confirm it is moved to trash (recoverable) without approval.
//  6. Ask the agent to write outside the workspace (e.g. /etc/foo):
//     confirm the policy DENIES the call (no approval dialog, just a
//     denied tool_result).
//  7. Ask the agent to modify .git/config: confirm the policy DENIES.
//
// Do NOT run this from a sub-agent; launch the Desktop app per the root
// README and drive it from the UI.

func TestDefaultAutoToolPolicyTransformsShellEchoWrite(t *testing.T) {
	workspace := t.TempDir()
	normalizedInput := `{"command":"echo \"hello\" > ./file.txt"}`

	decision := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:        "shell",
		StaticBehavior:  tool.PermissionAsk,
		StaticReason:    "shell command contains operators, redirections, substitutions, or multiple operations",
		NormalizedInput: normalizedInput,
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
		IsOpenWorld:     true,
	})

	if decision.Action != ToolDecisionTransform {
		t.Fatalf("expected transform action, got %+v", decision)
	}
	if decision.Source != ToolDecisionSourceTransform {
		t.Fatalf("expected transform source, got %q", decision.Source)
	}
	if decision.UpdatedToolName != "apply_patch" {
		t.Fatalf("expected UpdatedToolName=apply_patch, got %q", decision.UpdatedToolName)
	}
	if !strings.Contains(decision.UpdatedInput, "*** Add File: file.txt") {
		t.Fatalf("expected UpdatedInput to contain Add File for file.txt, got %q", decision.UpdatedInput)
	}
	if !strings.Contains(decision.UpdatedInput, "+hello") {
		t.Fatalf("expected UpdatedInput to contain +hello, got %q", decision.UpdatedInput)
	}
}

func TestDefaultAutoToolPolicyDeniesOpenWorldShellCommand(t *testing.T) {
	workspace := t.TempDir()
	normalizedInput := `{"command":"curl http://example.com > file"}`

	decision := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:        "shell",
		StaticBehavior:  tool.PermissionAsk,
		StaticReason:    "shell command may mutate files, change repository state, or access external systems",
		NormalizedInput: normalizedInput,
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
		IsOpenWorld:     true,
		IsDestructive:   true,
	})

	if decision.Action != ToolDecisionDeny {
		t.Fatalf("expected deny action for open-world shell command, got %+v", decision)
	}
	if !strings.Contains(decision.Reason, "apply_patch") {
		t.Fatalf("expected reason to suggest apply_patch, got %q", decision.Reason)
	}
}

func TestDefaultAutoToolPolicyDeniesNonTransformableShellMutation(t *testing.T) {
	workspace := t.TempDir()
	normalizedInput := `{"command":"sed -i 's/a/b/' file.txt"}`

	decision := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:        "shell",
		StaticBehavior:  tool.PermissionAsk,
		StaticReason:    "shell command is not classified as read-only",
		NormalizedInput: normalizedInput,
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
		IsOpenWorld:     true,
	})

	if decision.Action != ToolDecisionDeny {
		t.Fatalf("expected deny action for non-transformable mutation, got %+v", decision)
	}
	if !strings.Contains(decision.Reason, "apply_patch") {
		t.Fatalf("expected reason to suggest apply_patch, got %q", decision.Reason)
	}
}

func TestDefaultAutoToolPolicyFallsThroughForReadOnlyShell(t *testing.T) {
	workspace := t.TempDir()
	normalizedInput := `{"command":"ls -la"}`

	// `ls -la` is inspect_read_only. The shell tool's CheckPermission
	// returns PermissionAllow for safe read-only commands, so this request
	// simulates the Ask path (e.g. if the classifier were conservative).
	// The transform logic should fall through (not handle it), and the
	// default deny should apply.
	decision := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:        "shell",
		StaticBehavior:  tool.PermissionAsk,
		StaticReason:    "shell command is not classified as read-only",
		NormalizedInput: normalizedInput,
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
		IsOpenWorld:     true,
	})

	if decision.Action != ToolDecisionDeny {
		t.Fatalf("expected deny (fallthrough) for read-only shell on Ask path, got %+v", decision)
	}
	if decision.Source != ToolDecisionSourceHeadlessDefault {
		t.Fatalf("expected headless_default source for read-only shell fallthrough, got %q", decision.Source)
	}
}

func TestDefaultAutoToolPolicyDoesNotTransformInsideTransformChain(t *testing.T) {
	workspace := t.TempDir()
	normalizedInput := `{"command":"echo \"hello\" > ./file.txt"}`

	// When the context already carries transform depth (i.e. this call is the
	// result of a transform), the policy must not transform again.
	transformedCtx := ctxWithTransformDepth(context.Background(), 1)
	decision := DefaultAutoToolPolicy{}.Decide(transformedCtx, ToolDecisionRequest{
		ToolName:        "shell",
		StaticBehavior:  tool.PermissionAsk,
		StaticReason:    "shell command contains operators",
		NormalizedInput: normalizedInput,
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
		IsOpenWorld:     true,
	})

	if decision.Action == ToolDecisionTransform {
		t.Fatalf("expected nested transform to be suppressed, got %+v", decision)
	}
}

// --- DecideExecution tests (Task 9) ---
//
// These verify that DefaultAutoToolPolicy.DecideExecution produces correct
// ExecutionDecision outputs for the four scenarios required by the spec:
// allow (apply_patch write), deny (sensitive path), transform (shell echo
// write), and deny (open-world shell).

func TestDecideExecution_AllowsApplyPatchWrite(t *testing.T) {
	workspace := t.TempDir()
	intent := profile.ExecutionIntent{
		ToolName:        "apply_patch",
		ToolKind:        profile.ToolKindFileChain,
		RequestedAction: profile.ActionWrite,
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
	}
	request := ToolDecisionRequest{
		ToolName:        "apply_patch",
		StaticBehavior:  tool.PermissionAsk,
		StaticReason:    "apply_patch will modify files",
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
		RiskTone:        "warning",
		AffectedPaths:   []string{"note.md"},
		NormalizedInput: "*** Begin Patch\n*** Add File: note.md\n+hello\n*** End Patch",
	}

	decision := DefaultAutoToolPolicy{}.DecideExecution(context.Background(), intent, request)

	if decision.Action != profile.DecisionAllow {
		t.Fatalf("expected allow, got %s: %+v", decision.Action, decision)
	}
	if decision.Source != profile.DecisionSourceWorkspaceBoundary {
		t.Fatalf("expected workspace_boundary source, got %s", decision.Source)
	}
	if decision.UpdatedIntent == nil {
		t.Fatal("UpdatedIntent should not be nil for allow")
	}
	if decision.UpdatedIntent.ToolName != "apply_patch" {
		t.Errorf("UpdatedIntent.ToolName = %q, want apply_patch", decision.UpdatedIntent.ToolName)
	}
}

func TestDecideExecution_DeniesSensitivePath(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("HOME", workspace)
	intent := profile.ExecutionIntent{
		ToolName:        "apply_patch",
		ToolKind:        profile.ToolKindFileChain,
		RequestedAction: profile.ActionWrite,
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
	}
	request := ToolDecisionRequest{
		ToolName:        "apply_patch",
		StaticBehavior:  tool.PermissionAsk,
		StaticReason:    "apply_patch touches sensitive path",
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
		RiskTone:        "danger",
		AffectedPaths:   []string{".git/config"},
		NormalizedInput: "*** Begin Patch\n*** Add File: .git/config\n+[core]\n*** End Patch",
		IsDestructive:   true,
	}

	decision := DefaultAutoToolPolicy{}.DecideExecution(context.Background(), intent, request)

	if decision.Action != profile.DecisionDeny {
		t.Fatalf("expected deny, got %s: %+v", decision.Action, decision)
	}
	if !strings.Contains(decision.Reason, "sensitive path") {
		t.Fatalf("expected reason to mention sensitive path, got %q", decision.Reason)
	}
}

func TestDecideExecution_TransformsShellEchoWrite(t *testing.T) {
	workspace := t.TempDir()
	intent := profile.ExecutionIntent{
		ToolName:        "shell",
		ToolKind:        profile.ToolKindShellCommand,
		RequestedAction: profile.ActionExecute,
		NetworkScope:    profile.NetworkScopeOpen,
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
	}
	request := ToolDecisionRequest{
		ToolName:        "shell",
		StaticBehavior:  tool.PermissionAsk,
		StaticReason:    "shell command contains operators",
		NormalizedInput: `{"command":"echo \"hello\" > ./file.txt"}`,
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
		IsOpenWorld:     true,
	}

	decision := DefaultAutoToolPolicy{}.DecideExecution(context.Background(), intent, request)

	if decision.Action != profile.DecisionTransform {
		t.Fatalf("expected transform, got %s: %+v", decision.Action, decision)
	}
	if decision.Source != profile.DecisionSourceTransform {
		t.Fatalf("expected transform source, got %s", decision.Source)
	}
	if decision.UpdatedToolName != "apply_patch" {
		t.Fatalf("expected UpdatedToolName=apply_patch, got %q", decision.UpdatedToolName)
	}
	if decision.UpdatedInput == nil {
		t.Fatal("UpdatedInput should not be nil for transform")
	}
	patch, _ := decision.UpdatedInput["patch"].(string)
	if !strings.Contains(patch, "*** Add File: file.txt") {
		t.Fatalf("expected patch to contain Add File for file.txt, got %q", patch)
	}
	if !strings.Contains(patch, "+hello") {
		t.Fatalf("expected patch to contain +hello, got %q", patch)
	}
	if decision.UpdatedIntent == nil {
		t.Fatal("UpdatedIntent should not be nil for transform")
	}
}

func TestDecideExecution_DeniesOpenWorldShell(t *testing.T) {
	workspace := t.TempDir()
	intent := profile.ExecutionIntent{
		ToolName:        "shell",
		ToolKind:        profile.ToolKindShellCommand,
		RequestedAction: profile.ActionExecute,
		NetworkScope:    profile.NetworkScopeOpen,
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
	}
	request := ToolDecisionRequest{
		ToolName:        "shell",
		StaticBehavior:  tool.PermissionAsk,
		StaticReason:    "shell command may access external systems",
		NormalizedInput: `{"command":"curl http://example.com > file"}`,
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
		IsOpenWorld:     true,
		IsDestructive:   true,
	}

	decision := DefaultAutoToolPolicy{}.DecideExecution(context.Background(), intent, request)

	if decision.Action != profile.DecisionDeny {
		t.Fatalf("expected deny, got %s: %+v", decision.Action, decision)
	}
	if !strings.Contains(decision.Reason, "apply_patch") {
		t.Fatalf("expected reason to suggest apply_patch, got %q", decision.Reason)
	}
}

func TestDecideExecution_DeniesOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	intent := profile.ExecutionIntent{
		ToolName: "read_file",
		ToolKind: profile.ToolKindFileChain,
	}
	request := ToolDecisionRequest{
		ToolName:       "read_file",
		StaticBehavior: tool.PermissionAsk,
		IsReadOnly:     true,
	}

	decision := DefaultAutoToolPolicy{}.DecideExecution(ctx, intent, request)

	// In the unified model, a cancelled context produces a deny with
	// policy_error source (not abort, which is a legacy-only action).
	if decision.Action != profile.DecisionDeny {
		t.Fatalf("expected deny for cancelled context, got %s", decision.Action)
	}
	if decision.Source != profile.DecisionSourcePolicyError {
		t.Fatalf("expected policy_error source, got %s", decision.Source)
	}
}

func TestToolRuntimeAutoDeniesAskWithoutApprovalAwaiter(t *testing.T) {
	var executeCount int
	toolName := "runtime_auto_deny_no_awaiter_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:         toolName,
		permission:   tool.AskPermission("test tool requires approval", tool.ApprovalRequest{}),
		executeCount: &executeCount,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	var got []AwakenEvent
	result := NewToolRuntime(EventSinkFunc(func(event AwakenEvent) {
		got = append(got, event)
	})).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_missing_awaiter",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if executeCount != 0 {
		t.Fatalf("expected AUTO-denied tool not to execute, got %d executions", executeCount)
	}
	if len(got) != 1 ||
		got[0].Type != ToolPermissionDecidedEventType ||
		got[0].ToolDecisionAction != string(ToolDecisionDeny) ||
		got[0].ToolDecisionSource != string(ToolDecisionSourceHeadlessDefault) {
		t.Fatalf("expected one AUTO deny permission event, got %+v", got)
	}
	if !result.IsError ||
		result.Status != ToolExecutionDenied ||
		result.DecisionSource != ToolDecisionSourceHeadlessDefault ||
		!strings.Contains(result.Content, "工具执行被 AUTO policy 拒绝") ||
		strings.Contains(result.Content, "no approval awaiter") {
		t.Fatalf("expected AUTO denial without approval awaiter error, got %+v", result)
	}
}

func TestToolRuntimeAutoPolicyAbortSynthesizesResult(t *testing.T) {
	var executeCount int
	toolName := "runtime_auto_abort_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:         toolName,
		permission:   tool.AskPermission("test tool requires approval", tool.ApprovalRequest{}),
		executeCount: &executeCount,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	runtime := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		AutoToolPolicy: staticAutoToolPolicy{decision: ToolDecision{
			Action: ToolDecisionAbort,
			Source: ToolDecisionSourcePolicyError,
			Reason: "policy unavailable",
		}},
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_abort",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if executeCount != 0 {
		t.Fatalf("expected denied tool not to execute, got %d executions", executeCount)
	}
	// In the unified ExecutionDecision model, abort maps to deny with
	// policy_error source (context cancellation is handled separately).
	if !result.IsError ||
		result.Status != ToolExecutionDenied ||
		result.ErrorCode != "tool_denied" ||
		result.DecisionSource != ToolDecisionSourcePolicyError ||
		!strings.Contains(result.Content, "工具执行被 AUTO policy 拒绝") {
		t.Fatalf("expected denied tool result (abort→deny), got %+v", result)
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
	runtime := NewToolRuntimeWithConfig(testApprovalAwaiter{
		decision: ApprovalDecisionApproved,
		events:   &got,
	}, ToolRuntimeConfig{UseApprovalForAsk: true})
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
	if len(got) != 5 {
		t.Fatalf("expected approval and artifact events, got %+v", got)
	}
	if got[0].Type != ApprovalRequestedEventType ||
		got[1].Type != ToolPermissionDecidedEventType ||
		got[2].Type != ToolProgressEventType ||
		got[2].ToolProgressStage != "started" ||
		got[3].Type != ToolProgressEventType ||
		got[3].ToolProgressStage != "completed" ||
		got[4].Type != ArtifactCreatedEventType {
		t.Fatalf("unexpected emitted events: %+v", got)
	}

	var rejectedEvents []AwakenEvent
	rejected := NewToolRuntimeWithConfig(
		testApprovalAwaiter{decision: ApprovalDecisionRejected, events: &rejectedEvents},
		ToolRuntimeConfig{UseApprovalForAsk: true},
	)
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
	if len(got) != 5 {
		t.Fatalf("expected permission and two command output events, got %+v", got)
	}
	if got[0].Type != ToolPermissionDecidedEventType || got[0].ToolDecisionAction != string(ToolDecisionAllow) {
		t.Fatalf("unexpected permission event: %+v", got[0])
	}
	if got[1].Type != ToolProgressEventType || got[1].ToolProgressStage != "started" {
		t.Fatalf("unexpected start progress event: %+v", got[1])
	}
	if got[2].Type != CommandOutputEventType || got[2].ToolCallID != "call_observable" || got[2].CommandStream != "stdout" || got[2].Delta != "hello" {
		t.Fatalf("unexpected stdout event: %+v", got[2])
	}
	if got[3].Type != CommandOutputEventType || got[3].CommandStream != "stderr" || got[3].Delta != "warn" || !got[3].Truncated {
		t.Fatalf("unexpected stderr event: %+v", got[3])
	}
	if got[4].Type != ToolProgressEventType || got[4].ToolProgressStage != "completed" {
		t.Fatalf("unexpected completed progress event: %+v", got[4])
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
		UseApprovalForAsk: true,
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

	runtime := NewToolRuntimeWithConfig(
		testApprovalAwaiter{decision: ApprovalDecisionApproved},
		ToolRuntimeConfig{UseApprovalForAsk: true},
	)
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

	runtime := NewToolRuntimeWithConfig(
		testApprovalAwaiter{decision: ApprovalDecisionRejected},
		ToolRuntimeConfig{UseApprovalForAsk: true},
	)
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
	runtime := NewToolRuntimeWithConfig(
		testApprovalAwaiter{decision: ApprovalDecisionRejected},
		ToolRuntimeConfig{UseApprovalForAsk: true},
	)
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

type staticAutoToolPolicy struct {
	decision ToolDecision
}

func (p staticAutoToolPolicy) Decide(context.Context, ToolDecisionRequest) ToolDecision {
	return p.decision
}

func (p staticAutoToolPolicy) DecideExecution(_ context.Context, intent profile.ExecutionIntent, _ ToolDecisionRequest) profile.ExecutionDecision {
	return toolDecisionToExecutionDecision(p.decision, intent)
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
