package agentcore

import (
	"context"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
	"awaken/src/tool/profile"
)

func TestDefaultAutoToolPolicyAllowsShellEchoWrite(t *testing.T) {
	workspace := t.TempDir()
	normalizedInput := `{"command":"echo \"hello\" > ./file.txt"}`

	decision := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:        "shell",
		StaticBehavior:  tool.PermissionAllow,
		StaticReason:    "shell command contains operators, redirections, substitutions, or multiple operations",
		NormalizedInput: normalizedInput,
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
		IsOpenWorld:     true,
	})

	if decision.Action != ToolDecisionAllow {
		t.Fatalf("expected allow action, got %+v", decision)
	}
	if decision.Source != ToolDecisionSourceToolRule {
		t.Fatalf("expected tool_rule source, got %q", decision.Source)
	}
	if decision.UpdatedToolName != "" {
		t.Fatalf("expected no UpdatedToolName, got %q", decision.UpdatedToolName)
	}
	if decision.UpdatedInput != normalizedInput {
		t.Fatalf("expected original UpdatedInput, got %q", decision.UpdatedInput)
	}
}

func TestDefaultAutoToolPolicyAllowsOpenWorldShellCommand(t *testing.T) {
	workspace := t.TempDir()
	normalizedInput := `{"command":"curl http://example.com > file"}`

	decision := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:        "shell",
		StaticBehavior:  tool.PermissionAllow,
		StaticReason:    "shell command may mutate files, change repository state, or access external systems",
		NormalizedInput: normalizedInput,
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
		IsOpenWorld:     true,
		IsDestructive:   true,
	})

	if decision.Action != ToolDecisionAllow {
		t.Fatalf("expected allow action for open-world shell command, got %+v", decision)
	}
	if decision.Source != ToolDecisionSourceToolRule {
		t.Fatalf("expected tool_rule source, got %q", decision.Source)
	}
}

func TestDefaultAutoToolPolicyAllowsNonTransformableShellMutation(t *testing.T) {
	workspace := t.TempDir()
	normalizedInput := `{"command":"sed -i 's/a/b/' file.txt"}`

	decision := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:        "shell",
		StaticBehavior:  tool.PermissionAllow,
		StaticReason:    "shell command is not classified as read-only",
		NormalizedInput: normalizedInput,
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
		IsOpenWorld:     true,
	})

	if decision.Action != ToolDecisionAllow {
		t.Fatalf("expected allow action for non-transformable mutation, got %+v", decision)
	}
	if decision.Source != ToolDecisionSourceToolRule {
		t.Fatalf("expected tool_rule source, got %q", decision.Source)
	}
}

func TestDefaultAutoToolPolicyAllowsSupportedShellLanes(t *testing.T) {
	workspace := t.TempDir()
	cases := []struct {
		name    string
		command string
	}{
		{"mkdir", "mkdir -p reports"},
		{"touch", "touch reports.md"},
		{"go test", "go test ./..."},
		{"npm build", "npm run build"},
		{"dws search", `dws aitable base search --query "DVF" --format json`},
		{"gh issue list", "gh issue list --limit 10"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
				ToolName:        "shell",
				StaticBehavior:  tool.PermissionAllow,
				NormalizedInput: `{"command":` + mustJSONQuote(t, tc.command) + `}`,
				WorkspaceRoot:   workspace,
				ReadableRoots:   []string{workspace},
				WritableRoots:   []string{workspace},
				OutputRoots:     []string{workspace},
				IsOpenWorld:     true,
			})
			if decision.Action != ToolDecisionAllow {
				t.Fatalf("expected allow for %q, got %+v", tc.command, decision)
			}
			if decision.Reason != "local shell command allowed" {
				t.Fatalf("expected generic local shell reason, got %q", decision.Reason)
			}
		})
	}
}

func TestDefaultAutoToolPolicyAllowsShellNullDeviceRedirectProbe(t *testing.T) {
	workspace := t.TempDir()
	decision := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:        "shell",
		StaticBehavior:  tool.PermissionAllow,
		NormalizedInput: `{"command":"command -v pdftotext >/dev/null 2>&1"}`,
		WorkspaceRoot:   workspace,
		ReadableRoots:   []string{workspace},
		WritableRoots:   []string{workspace},
		OutputRoots:     []string{workspace},
		IsOpenWorld:     true,
	})

	if decision.Action != ToolDecisionAllow {
		t.Fatalf("expected allow for /dev/null redirect probe, got %+v", decision)
	}
}

func TestDefaultAutoToolPolicyAllowsShellSensitiveRead(t *testing.T) {
	workspace := t.TempDir()
	decision := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:        "shell",
		StaticBehavior:  tool.PermissionAllow,
		NormalizedInput: `{"command":"cat ~/.ssh/id_rsa"}`,
		WorkspaceRoot:   workspace,
		ReadableRoots:   []string{workspace},
		WritableRoots:   []string{workspace},
		OutputRoots:     []string{workspace},
		IsOpenWorld:     true,
	})

	if decision.Action != ToolDecisionAllow {
		t.Fatalf("expected allow for sensitive read through local shell, got %+v", decision)
	}
}

func TestDefaultAutoToolPolicyFallsThroughForReadOnlyShell(t *testing.T) {
	workspace := t.TempDir()
	normalizedInput := `{"command":"ls -la"}`

	// `ls -la` is inspect_read_only. The shell policy falls through from
	// transform handling and auto-allows it as a normal safe shell read.
	decision := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName:        "shell",
		StaticBehavior:  tool.PermissionAllow,
		StaticReason:    "shell command is not classified as read-only",
		NormalizedInput: normalizedInput,
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
		IsOpenWorld:     true,
	})

	if decision.Action != ToolDecisionAllow {
		t.Fatalf("expected allow for read-only shell fallthrough, got %+v", decision)
	}
	if decision.Source != ToolDecisionSourceToolRule {
		t.Fatalf("expected tool_rule source for read-only shell fallthrough, got %q", decision.Source)
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
		StaticBehavior:  tool.PermissionAllow,
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
// ExecutionDecision outputs for the key scenarios required by the spec:
// allow (apply_patch write), deny (sensitive path), allow (shell echo write),
// and allow (open-world shell).

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
		StaticBehavior:  tool.PermissionAllow,
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
		StaticBehavior:  tool.PermissionAllow,
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

func TestDecideExecution_AllowsShellEchoWrite(t *testing.T) {
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
		StaticBehavior:  tool.PermissionAllow,
		StaticReason:    "shell command contains operators",
		NormalizedInput: `{"command":"echo \"hello\" > ./file.txt"}`,
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
		IsOpenWorld:     true,
	}

	decision := DefaultAutoToolPolicy{}.DecideExecution(context.Background(), intent, request)

	if decision.Action != profile.DecisionAllow {
		t.Fatalf("expected allow, got %s: %+v", decision.Action, decision)
	}
	if decision.Source != profile.DecisionSourceToolRule {
		t.Fatalf("expected tool_rule source, got %s", decision.Source)
	}
	if decision.UpdatedToolName != "" {
		t.Fatalf("expected no UpdatedToolName, got %q", decision.UpdatedToolName)
	}
	if decision.UpdatedInput != nil {
		t.Fatalf("expected no UpdatedInput for shell allow, got %+v", decision.UpdatedInput)
	}
	if decision.UpdatedIntent == nil {
		t.Fatal("UpdatedIntent should not be nil for allow")
	}
	if decision.UpdatedIntent.ToolName != "shell" {
		t.Fatalf("expected UpdatedIntent.ToolName=shell, got %q", decision.UpdatedIntent.ToolName)
	}
}

func TestDecideExecution_AllowsOpenWorldShell(t *testing.T) {
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
		StaticBehavior:  tool.PermissionAllow,
		StaticReason:    "shell command may access external systems",
		NormalizedInput: `{"command":"curl http://example.com > file"}`,
		WorkspaceRoot:   workspace,
		WritableRoots:   []string{workspace},
		IsOpenWorld:     true,
		IsDestructive:   true,
	}

	decision := DefaultAutoToolPolicy{}.DecideExecution(context.Background(), intent, request)

	if decision.Action != profile.DecisionAllow {
		t.Fatalf("expected allow, got %s: %+v", decision.Action, decision)
	}
	if decision.Source != profile.DecisionSourceToolRule {
		t.Fatalf("expected tool_rule source, got %s", decision.Source)
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
		StaticBehavior: tool.PermissionAllow,
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

func TestToolRuntimeDenyDoesNotRequireApprovalAwaiter(t *testing.T) {
	var executeCount int
	toolName := "runtime_auto_deny_no_awaiter_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:         toolName,
		permission:   tool.DenyPermission("test tool is denied"),
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
		got[0].ToolDecisionSource != string(ToolDecisionSourceToolRule) {
		t.Fatalf("expected one AUTO deny permission event, got %+v", got)
	}
	if !result.IsError ||
		result.Status != ToolExecutionDenied ||
		result.DecisionSource != ToolDecisionSourceToolRule ||
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
		permission:   tool.AllowPermission(),
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
