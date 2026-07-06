package profile_test

import (
	"encoding/json"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
	"awaken/src/tool/profile"
	applypatch "awaken/src/tools/apply_patch"
	shelltool "awaken/src/tools/shell"
)

// --- InferToolKind tests ---

func TestInferToolKind(t *testing.T) {
	tests := []struct {
		label       string
		toolName    string
		annotations tool.ToolAnnotations
		want        profile.ToolKind
	}{
		// Builtin file-chain tools.
		{label: "apply_patch", toolName: "apply_patch", annotations: tool.ToolAnnotations{Destructive: true}, want: profile.ToolKindFileChain},
		{label: "file_read", toolName: "file_read", annotations: tool.ToolAnnotations{ReadOnly: true}, want: profile.ToolKindFileChain},
		{label: "read_file", toolName: "read_file", annotations: tool.ToolAnnotations{ReadOnly: true}, want: profile.ToolKindFileChain},
		{label: "list_files", toolName: "list_files", annotations: tool.ToolAnnotations{ReadOnly: true}, want: profile.ToolKindFileChain},
		{label: "grep_search", toolName: "grep_search", annotations: tool.ToolAnnotations{ReadOnly: true}, want: profile.ToolKindFileChain},

		// Shell.
		{label: "shell", toolName: "shell", annotations: tool.ToolAnnotations{OpenWorld: true, Destructive: true}, want: profile.ToolKindShellCommand},

		// MCP/connector.
		{label: "mcp by name prefix", toolName: "mcp__notion__search", annotations: tool.ToolAnnotations{ServerName: "notion"}, want: profile.ToolKindMcpApp},
		{label: "mcp by server name", toolName: "custom_tool", annotations: tool.ToolAnnotations{ServerName: "github"}, want: profile.ToolKindMcpApp},

		// Review/guardian.
		{label: "review tool", toolName: "review_code", annotations: tool.ToolAnnotations{ReadOnly: true}, want: profile.ToolKindReviewAgent},
		{label: "guardian tool", toolName: "guardian_check", annotations: tool.ToolAnnotations{}, want: profile.ToolKindReviewAgent},

		// Defaults.
		{label: "unknown read-only", toolName: "custom_read", annotations: tool.ToolAnnotations{ReadOnly: true}, want: profile.ToolKindFileChain},
		{label: "unknown write", toolName: "custom_write", annotations: tool.ToolAnnotations{}, want: profile.ToolKindShellCommand},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			got := profile.InferToolKind(tt.toolName, tt.annotations)
			if got != tt.want {
				t.Errorf("profile.InferToolKind(%q, %+v) = %q, want %q", tt.toolName, tt.annotations, got, tt.want)
			}
		})
	}
}

// --- InferNetworkScope tests ---

func TestInferNetworkScope(t *testing.T) {
	tests := []struct {
		label       string
		annotations tool.ToolAnnotations
		want        profile.NetworkScope
	}{
		{label: "open world", annotations: tool.ToolAnnotations{OpenWorld: true}, want: profile.NetworkScopeOpen},
		{label: "server name", annotations: tool.ToolAnnotations{ServerName: "notion"}, want: profile.NetworkScopeRestricted},
		{label: "requires auth", annotations: tool.ToolAnnotations{RequiresAuth: true}, want: profile.NetworkScopeRestricted},
		{label: "none", annotations: tool.ToolAnnotations{}, want: profile.NetworkScopeNone},
		{label: "read-only no network", annotations: tool.ToolAnnotations{ReadOnly: true}, want: profile.NetworkScopeNone},
		{label: "open world overrides server", annotations: tool.ToolAnnotations{OpenWorld: true, ServerName: "x"}, want: profile.NetworkScopeOpen},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			got := profile.InferNetworkScope(tt.annotations)
			if got != tt.want {
				t.Errorf("profile.InferNetworkScope(%+v) = %q, want %q", tt.annotations, got, tt.want)
			}
		})
	}
}

// --- BuildExecutionIntent tests ---

func mustMarshalArgs(t *testing.T, args map[string]interface{}) string {
	t.Helper()
	data, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("failed to marshal args: %v", err)
	}
	return string(data)
}

func TestBuildExecutionIntent_ApplyPatch(t *testing.T) {
	workspace := t.TempDir()
	permCtx := tool.NewPermissionContext(workspace, nil)

	patch := "*** Begin Patch\n" +
		"*** Add File: newfile.txt\n" +
		"+hello world\n" +
		"*** End Patch"
	call := convcontext.ToolCall{
		ID:   "call-apply-patch",
		Type: "function",
		Function: convcontext.FunctionCall{
			Name:      "apply_patch",
			Arguments: mustMarshalArgs(t, map[string]interface{}{"patch": patch}),
		},
	}

	intent := profile.BuildExecutionIntent(call, &applypatch.ApplyPatchTool{}, permCtx, nil)

	if intent.ToolName != "apply_patch" {
		t.Errorf("ToolName = %q, want %q", intent.ToolName, "apply_patch")
	}
	if intent.ToolKind != profile.ToolKindFileChain {
		t.Errorf("ToolKind = %q, want %q", intent.ToolKind, profile.ToolKindFileChain)
	}
	if intent.RequestedAction != profile.ActionWrite {
		t.Errorf("RequestedAction = %q, want %q", intent.RequestedAction, profile.ActionWrite)
	}
	if intent.NetworkScope != profile.NetworkScopeNone {
		t.Errorf("NetworkScope = %q, want %q", intent.NetworkScope, profile.NetworkScopeNone)
	}
	if intent.WorkspaceRoot == "" {
		t.Error("WorkspaceRoot should not be empty")
	}
	if len(intent.WritableRoots) == 0 {
		t.Error("WritableRoots should not be empty")
	}
	if intent.SubagentOrigin != nil {
		t.Errorf("SubagentOrigin should be nil for root agent, got %+v", intent.SubagentOrigin)
	}
	// apply_patch implements PatchPreviewingTool, so Paths should be populated.
	found := false
	for _, p := range intent.Paths {
		if p == "newfile.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Paths = %v, expected to contain %q", intent.Paths, "newfile.txt")
	}

	// String() should produce a non-empty log line.
	if s := intent.String(); s == "" {
		t.Error("String() should not be empty")
	}
}

func TestBuildExecutionIntent_ApplyPatchDelete(t *testing.T) {
	workspace := t.TempDir()
	permCtx := tool.NewPermissionContext(workspace, nil)

	patch := "*** Begin Patch\n" +
		"*** Delete File: oldfile.txt\n" +
		"*** End Patch"
	call := convcontext.ToolCall{
		ID:   "call-delete",
		Type: "function",
		Function: convcontext.FunctionCall{
			Name:      "apply_patch",
			Arguments: mustMarshalArgs(t, map[string]interface{}{"patch": patch}),
		},
	}

	intent := profile.BuildExecutionIntent(call, &applypatch.ApplyPatchTool{}, permCtx, nil)

	if intent.RequestedAction != profile.ActionDelete {
		t.Errorf("RequestedAction = %q, want %q", intent.RequestedAction, profile.ActionDelete)
	}
}

func TestBuildExecutionIntent_ApplyPatchMove(t *testing.T) {
	workspace := t.TempDir()
	permCtx := tool.NewPermissionContext(workspace, nil)

	patch := "*** Begin Patch\n" +
		"*** Update File: oldfile.txt\n" +
		"*** Move to: newfile.txt\n" +
		"@@\n" +
		"*** End Patch"
	call := convcontext.ToolCall{
		ID:   "call-move",
		Type: "function",
		Function: convcontext.FunctionCall{
			Name:      "apply_patch",
			Arguments: mustMarshalArgs(t, map[string]interface{}{"patch": patch}),
		},
	}

	intent := profile.BuildExecutionIntent(call, &applypatch.ApplyPatchTool{}, permCtx, nil)

	if intent.RequestedAction != profile.ActionMove {
		t.Errorf("RequestedAction = %q, want %q", intent.RequestedAction, profile.ActionMove)
	}
}

func TestBuildExecutionIntent_Shell(t *testing.T) {
	workspace := t.TempDir()
	permCtx := tool.NewPermissionContext(workspace, nil)

	call := convcontext.ToolCall{
		ID:   "call-shell",
		Type: "function",
		Function: convcontext.FunctionCall{
			Name:      "shell",
			Arguments: mustMarshalArgs(t, map[string]interface{}{"command": "echo hello > output.txt"}),
		},
	}

	intent := profile.BuildExecutionIntent(call, &shelltool.ShellTool{}, permCtx, nil)

	if intent.ToolName != "shell" {
		t.Errorf("ToolName = %q, want %q", intent.ToolName, "shell")
	}
	if intent.ToolKind != profile.ToolKindShellCommand {
		t.Errorf("ToolKind = %q, want %q", intent.ToolKind, profile.ToolKindShellCommand)
	}
	if intent.RequestedAction != profile.ActionExecute {
		t.Errorf("RequestedAction = %q, want %q", intent.RequestedAction, profile.ActionExecute)
	}
	if intent.NetworkScope != profile.NetworkScopeOpen {
		t.Errorf("NetworkScope = %q, want %q", intent.NetworkScope, profile.NetworkScopeOpen)
	}
	if intent.SubagentOrigin != nil {
		t.Errorf("SubagentOrigin should be nil for root agent, got %+v", intent.SubagentOrigin)
	}
	// shell path extraction is best-effort: redirect target "output.txt" should be extracted.
	found := false
	for _, p := range intent.Paths {
		if p == "output.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Paths = %v, expected to contain %q", intent.Paths, "output.txt")
	}
}

func TestBuildExecutionIntent_ShellNoRedirect(t *testing.T) {
	workspace := t.TempDir()
	permCtx := tool.NewPermissionContext(workspace, nil)

	call := convcontext.ToolCall{
		ID:   "call-shell-noredirect",
		Type: "function",
		Function: convcontext.FunctionCall{
			Name:      "shell",
			Arguments: mustMarshalArgs(t, map[string]interface{}{"command": "ls -la"}),
		},
	}

	intent := profile.BuildExecutionIntent(call, &shelltool.ShellTool{}, permCtx, nil)

	if intent.ToolKind != profile.ToolKindShellCommand {
		t.Errorf("ToolKind = %q, want %q", intent.ToolKind, profile.ToolKindShellCommand)
	}
	if len(intent.Paths) != 0 {
		t.Errorf("Paths = %v, expected empty for non-redirect shell command", intent.Paths)
	}
}

func TestBuildExecutionIntent_WithSubagentOrigin(t *testing.T) {
	workspace := t.TempDir()
	permCtx := tool.NewPermissionContext(workspace, nil)

	call := convcontext.ToolCall{
		ID:   "call-subagent",
		Type: "function",
		Function: convcontext.FunctionCall{
			Name:      "shell",
			Arguments: mustMarshalArgs(t, map[string]interface{}{"command": "echo hi"}),
		},
	}

	origin := &profile.SubagentOrigin{
		AgentID:       "agent-2",
		ParentAgentID: "agent-1",
		IsGuardian:    true,
		Depth:         1,
	}

	intent := profile.BuildExecutionIntent(call, &shelltool.ShellTool{}, permCtx, origin)

	if intent.SubagentOrigin == nil {
		t.Fatal("SubagentOrigin should not be nil")
	}
	if intent.SubagentOrigin.AgentID != "agent-2" {
		t.Errorf("SubagentOrigin.AgentID = %q, want %q", intent.SubagentOrigin.AgentID, "agent-2")
	}
	if intent.SubagentOrigin.ParentAgentID != "agent-1" {
		t.Errorf("SubagentOrigin.ParentAgentID = %q, want %q", intent.SubagentOrigin.ParentAgentID, "agent-1")
	}
	if !intent.SubagentOrigin.IsGuardian {
		t.Error("SubagentOrigin.IsGuardian should be true")
	}
	if intent.SubagentOrigin.Depth != 1 {
		t.Errorf("SubagentOrigin.Depth = %d, want 1", intent.SubagentOrigin.Depth)
	}

	// String() should include the origin info.
	s := intent.String()
	if s == "" {
		t.Error("String() should not be empty")
	}
}

func TestExecutionIntent_String(t *testing.T) {
	intent := profile.ExecutionIntent{
		ToolName:        "apply_patch",
		ToolKind:        profile.ToolKindFileChain,
		RequestedAction: profile.ActionWrite,
		Paths:           []string{"a.txt", "b.txt"},
		NetworkScope:    profile.NetworkScopeNone,
		WorkspaceRoot:   "/tmp/ws",
		WritableRoots:   []string{"/tmp/ws"},
	}
	s := intent.String()
	if s == "" {
		t.Fatal("String() should not be empty")
	}
	for _, want := range []string{"apply_patch", "file_chain", "write", "a.txt", "b.txt"} {
		if !strings.Contains(s, want) {
			t.Errorf("String() = %q, expected to contain %q", s, want)
		}
	}
}
