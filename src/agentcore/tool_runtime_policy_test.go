package agentcore

import (
	"context"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
	"awaken/src/tool/profile"
)

func TestDefaultAutoToolPolicyAllowsCanonicalToolsWithoutApproval(t *testing.T) {
	workspace := t.TempDir()
	tests := []struct {
		name      string
		readOnly  bool
		openWorld bool
	}{
		{"Read", true, false}, {"Glob", true, false}, {"Grep", true, false}, {"WebSearch", true, true},
		{"Edit", false, false}, {"Write", false, false}, {"Bash", false, true},
	}
	for _, test := range tests {
		decision := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
			ToolName: test.name, StaticBehavior: tool.PermissionAllow, StaticReason: "tool allowed execution",
			WorkspaceRoot: workspace, ReadableRoots: []string{workspace}, WritableRoots: []string{workspace},
			IsReadOnly: test.readOnly, IsOpenWorld: test.openWorld,
		})
		if decision.Action != ToolDecisionAllow || decision.Source != ToolDecisionSourceToolRule {
			t.Fatalf("%s decision=%+v", test.name, decision)
		}
		if decision.UpdatedToolName != "" {
			t.Fatalf("%s unexpectedly transformed to %s", test.name, decision.UpdatedToolName)
		}
	}
}

func TestDefaultAutoToolPolicyPreservesBashInput(t *testing.T) {
	input := `{"command":"echo hello > output.txt"}`
	decision := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName: "Bash", StaticBehavior: tool.PermissionAllow, NormalizedInput: input, IsOpenWorld: true,
	})
	if decision.Action != ToolDecisionAllow || decision.UpdatedInput != input || decision.UpdatedToolName != "" {
		t.Fatalf("Bash must execute directly without a transform: %+v", decision)
	}
}

func TestDefaultAutoToolPolicyStaticDenyWins(t *testing.T) {
	decision := DefaultAutoToolPolicy{}.Decide(context.Background(), ToolDecisionRequest{
		ToolName: "Edit", StaticBehavior: tool.PermissionDeny, StaticReason: "sensitive path is protected",
	})
	if decision.Action != ToolDecisionDeny || !strings.Contains(decision.Reason, "sensitive") {
		t.Fatalf("static deny was not preserved: %+v", decision)
	}
}

func TestDecideExecutionAllowsBashOpenWorld(t *testing.T) {
	intent := profile.ExecutionIntent{ToolName: "Bash", ToolKind: profile.ToolKindShellCommand, RequestedAction: profile.ActionExecute, NetworkScope: profile.NetworkScopeOpen}
	decision := DefaultAutoToolPolicy{}.DecideExecution(context.Background(), intent, ToolDecisionRequest{ToolName: "Bash", StaticBehavior: tool.PermissionAllow, IsOpenWorld: true})
	if decision.Action != profile.DecisionAllow || decision.Source != profile.DecisionSourceToolRule || decision.UpdatedIntent == nil {
		t.Fatalf("Bash execution decision=%+v", decision)
	}
}

func TestDecideExecutionDeniesCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	decision := DefaultAutoToolPolicy{}.DecideExecution(ctx, profile.ExecutionIntent{ToolName: "Read"}, ToolDecisionRequest{ToolName: "Read", StaticBehavior: tool.PermissionAllow})
	if decision.Action != profile.DecisionDeny || decision.Source != profile.DecisionSourcePolicyError {
		t.Fatalf("cancelled decision=%+v", decision)
	}
}

func TestToolRuntimeDenyDoesNotRequireApprovalAwaiter(t *testing.T) {
	toolName := "policy_denied_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{name: toolName, permission: tool.DenyPermission("policy denied")})
	result := NewToolRuntime(nil).Execute(context.Background(), convcontext.ToolCall{ID: "deny", Function: convcontext.FunctionCall{Name: toolName, Arguments: `{}`}})
	if !result.IsError || result.Outcome.Status != tool.OutcomeDenied {
		t.Fatalf("denied result=%+v", result)
	}
}

func TestToolRuntimeAutoPolicyAbortSynthesizesResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := NewToolRuntime(nil).Execute(ctx, convcontext.ToolCall{ID: "cancel", Function: convcontext.FunctionCall{Name: "Read", Arguments: `{"file_path":"README.md"}`}})
	if !result.IsError || result.Outcome.Status != tool.OutcomeDenied || result.DecisionSource != ToolDecisionSourcePolicyError {
		t.Fatalf("cancelled result=%+v", result)
	}
}
