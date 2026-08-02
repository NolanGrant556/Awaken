package executors

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
	"awaken/src/tool/profile"
)

// --- Mock tools for executor tests ---

// noopTool is a minimal tool that returns a fixed string. It is used by
// BashExecutor and FileOperationExecutor tests where the executor should
// delegate to the tool after passing enforcement checks.
type noopTool struct {
	name        string
	annotations tool.ToolAnnotations
}

func (t *noopTool) Name() string { return t.name }
func (t *noopTool) Description() string {
	return "noop test tool"
}
func (t *noopTool) Parameters() []tool.ToolParameter { return nil }
func (t *noopTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        t.name,
		DisplayName: t.name,
		Source:      tool.SourceBuiltin,
		Description: "noop test tool",
		Annotations: t.annotations,
	}
}
func (t *noopTool) Execute(ctx context.Context, input string) (string, error) {
	return "executed: " + input, nil
}

// openWorldReadOnlyTool is a tool that is both read-only (implements
// ReadOnlyTool) and declares OpenWorld. It is used by GuardianExecutor
// tests to verify that an OpenWorld read-only tool is allowed under a
// NetworkRestricted profile.
type openWorldReadOnlyTool struct {
	name string
}

func (t *openWorldReadOnlyTool) Name() string { return t.name }
func (t *openWorldReadOnlyTool) Description() string {
	return "open-world read-only test tool"
}
func (t *openWorldReadOnlyTool) Parameters() []tool.ToolParameter { return nil }
func (t *openWorldReadOnlyTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        t.name,
		DisplayName: t.name,
		Source:      tool.SourceBuiltin,
		Description: "open-world read-only test tool",
		Annotations: tool.ToolAnnotations{ReadOnly: true, OpenWorld: true},
	}
}
func (t *openWorldReadOnlyTool) IsReadOnly() bool { return true }
func (t *openWorldReadOnlyTool) Execute(ctx context.Context, input string) (string, error) {
	return "open-world read-only output: " + input, nil
}

// --- BashExecutor tests ---

// TestBashExecutorAllowsSandboxRequiredProfile verifies that Bash delegates
// to the current-user tool even when the profile carries a legacy sandbox flag.
func TestBashExecutorAllowsSandboxRequiredProfile(t *testing.T) {
	executor := NewBashExecutor()
	intent := profile.ExecutionIntent{
		ToolName: "Bash",
		ToolKind: profile.ToolKindShellCommand,
	}
	profileConfig := profile.GuardianReadOnly()
	noop := &noopTool{name: "Bash"}

	output, err := executor.Execute(context.Background(), intent, profileConfig, noop, "test", nil)
	if err != nil {
		t.Fatalf("unexpected error for SandboxRequired shell profile: %v", err)
	}
	if output.IsError {
		t.Fatalf("expected IsError=false, got true (ErrorCode=%q Content=%q)", output.ErrorCode, output.Content)
	}
	if output.Content != "executed: test" {
		t.Fatalf("expected content %q, got %q", "executed: test", output.Content)
	}
}

// TestBashExecutorAllowsBestEffortProfile verifies the normal profile path.
func TestBashExecutorAllowsBestEffortProfile(t *testing.T) {
	executor := NewBashExecutor()
	intent := profile.ExecutionIntent{
		ToolName: "Bash",
		ToolKind: profile.ToolKindShellCommand,
	}
	profileConfig := profile.WorkspaceWrite()
	noop := &noopTool{name: "Bash"}

	output, err := executor.Execute(context.Background(), intent, profileConfig, noop, "test", nil)
	if err != nil {
		t.Fatalf("unexpected error for SandboxBestEffort without sandbox: %v", err)
	}
	if output.IsError {
		t.Fatalf("expected IsError=false, got true (ErrorCode=%q Content=%q)", output.ErrorCode, output.Content)
	}
	if output.Content != "executed: test" {
		t.Fatalf("expected content %q, got %q", "executed: test", output.Content)
	}
}

// --- McpExecutor tests ---

// TestMcpExecutorDeniesWriteUnderReadOnlyProfile verifies that McpExecutor
// only blocks write-capable MCP tools when the execution profile itself is
// read-only.
func TestMcpExecutorDeniesOpenWorldWithNetworkNone(t *testing.T) {
	executor := NewMcpExecutor()
	intent := profile.ExecutionIntent{
		ToolName: "mcp__web__search",
		ToolKind: profile.ToolKindMcpApp,
	}
	mcpProfile := profile.WorkspaceReadOnly() // NetworkNone
	openWorldTool := &noopTool{
		name:        "mcp__web__search",
		annotations: tool.ToolAnnotations{OpenWorld: true},
	}

	output, err := executor.Execute(context.Background(), intent, mcpProfile, openWorldTool, "test", nil)
	if err == nil {
		t.Fatalf("expected error for write-capable MCP tool under read-only profile, got nil")
	}
	if !output.IsError {
		t.Fatalf("expected IsError=true, got false")
	}
	if output.ErrorCode != "read_only_violation" {
		t.Fatalf("expected ErrorCode=read_only_violation, got %q", output.ErrorCode)
	}
}

// TestMcpExecutorAllowsOpenWorldWithNetworkRestricted verifies that
// McpExecutor allows an OpenWorld tool when the profile's NetworkPolicy is
// NetworkRestricted.
func TestMcpExecutorAllowsOpenWorldWithNetworkRestricted(t *testing.T) {
	executor := NewMcpExecutor()
	intent := profile.ExecutionIntent{
		ToolName: "mcp__web__search",
		ToolKind: profile.ToolKindMcpApp,
	}
	mcpProfile := profile.OpenWorldRestricted() // NetworkRestricted
	openWorldTool := &noopTool{
		name:        "mcp__web__search",
		annotations: tool.ToolAnnotations{OpenWorld: true},
	}

	output, err := executor.Execute(context.Background(), intent, mcpProfile, openWorldTool, "test", nil)
	if err != nil {
		t.Fatalf("unexpected error for OpenWorld tool with NetworkRestricted: %v", err)
	}
	if output.IsError {
		t.Fatalf("expected IsError=false, got true (ErrorCode=%q Content=%q)", output.ErrorCode, output.Content)
	}
	if output.Content != "executed: test" {
		t.Fatalf("expected content %q, got %q", "executed: test", output.Content)
	}
}

// --- FileOperationExecutor tests ---

// TestFileOperationExecutorDeniesWriteUnderReadOnlyProfile verifies that
// FileOperationExecutor denies a write action when the profile is
// PermissionReadOnly.
func TestFileOperationExecutorDeniesWriteUnderReadOnlyProfile(t *testing.T) {
	executor := NewFileOperationExecutor()
	intent := profile.ExecutionIntent{
		ToolName:        "Edit",
		ToolKind:        profile.ToolKindFileOperation,
		RequestedAction: profile.ActionWrite,
	}
	readOnlyProfile := profile.WorkspaceReadOnly() // PermissionReadOnly
	noop := &noopTool{name: "Edit"}

	output, err := executor.Execute(context.Background(), intent, readOnlyProfile, noop, "test", nil)
	if err == nil {
		t.Fatalf("expected error for write under read-only profile, got nil")
	}
	if !output.IsError {
		t.Fatalf("expected IsError=true, got false")
	}
	if output.ErrorCode != "read_only_violation" {
		t.Fatalf("expected ErrorCode=read_only_violation, got %q", output.ErrorCode)
	}
}

// TestFileOperationExecutorAllowsWriteUnderWorkspaceWriteProfile verifies that
// FileOperationExecutor allows a write action when the profile is
// PermissionWorkspaceWrite.
func TestFileOperationExecutorAllowsWriteUnderWorkspaceWriteProfile(t *testing.T) {
	executor := NewFileOperationExecutor()
	intent := profile.ExecutionIntent{
		ToolName:        "Edit",
		ToolKind:        profile.ToolKindFileOperation,
		RequestedAction: profile.ActionWrite,
		// Paths intentionally empty: the boundary check skips when no
		// paths are declared, so the tool's own Execute is the last line
		// of defense (as documented in enforceFileOperationBoundary).
	}
	writeProfile := profile.WorkspaceWrite() // PermissionWorkspaceWrite
	noop := &noopTool{name: "Edit"}

	output, err := executor.Execute(context.Background(), intent, writeProfile, noop, "test", nil)
	if err != nil {
		t.Fatalf("unexpected error for write under workspace_write profile: %v", err)
	}
	if output.IsError {
		t.Fatalf("expected IsError=false, got true (ErrorCode=%q Content=%q)", output.ErrorCode, output.Content)
	}
	if output.Content != "executed: test" {
		t.Fatalf("expected content %q, got %q", "executed: test", output.Content)
	}
}

func TestFileOperationExecutorUsesFullPermissionContextForManagedWorkspace(t *testing.T) {
	home := t.TempDir()
	awakenHome := filepath.Join(home, ".awaken")
	sessionsRoot := filepath.Join(awakenHome, "sessions")
	workspace := filepath.Join(sessionsRoot, "thread_1")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	permissionContext := tool.NewTaskScopePermissionContext(
		"scope_thread_1",
		workspace,
		[]string{workspace},
		[]string{workspace},
		[]string{filepath.Join(workspace, "output")},
		nil,
		[]string{awakenHome},
		filepath.Join(t.TempDir(), "scope_thread_1"),
	)
	permissionContext.ProtectedRootExceptions = []string{sessionsRoot}
	permissionContext = permissionContext.Normalize()

	target := filepath.Join(workspace, "translation-content.md")
	intent := profile.ExecutionIntent{
		ToolName:        "Write",
		ToolKind:        profile.ToolKindFileOperation,
		RequestedAction: profile.ActionWrite,
		Paths:           []string{target},
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	output, err := NewFileOperationExecutor().Execute(ctx, intent, profile.WorkspaceWrite(), &noopTool{name: "Write"}, "test", nil)
	if err != nil || output.IsError {
		t.Fatalf("managed session workspace must keep its protected-root exception: output=%+v err=%v", output, err)
	}
}

func TestFileOperationExecutorAllowsExplicitPathOutsideWorkspaceRoots(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	permissionContext := tool.NewTaskScopePermissionContext(
		"scope_external_path",
		workspace,
		[]string{workspace},
		[]string{workspace},
		[]string{workspace},
		nil,
		nil,
		filepath.Join(workspace, ".tmp"),
	)
	target := filepath.Join(outside, "translation-content.md")
	intent := profile.ExecutionIntent{
		ToolName:        "Write",
		ToolKind:        profile.ToolKindFileOperation,
		RequestedAction: profile.ActionWrite,
		Paths:           []string{target},
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	output, err := NewFileOperationExecutor().Execute(ctx, intent, profile.WorkspaceWrite(), &noopTool{name: "Write"}, "test", nil)
	if err != nil || output.IsError {
		t.Fatalf("an explicit absolute path must not be fenced to workspace roots: output=%+v err=%v", output, err)
	}
}

func TestFileOperationExecutorStillDeniesProtectedAwakenMetadata(t *testing.T) {
	home := t.TempDir()
	awakenHome := filepath.Join(home, ".awaken")
	sessionsRoot := filepath.Join(awakenHome, "sessions")
	workspace := filepath.Join(sessionsRoot, "thread_1")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	permissionContext := tool.NewTaskScopePermissionContext(
		"scope_thread_1",
		workspace,
		[]string{workspace},
		[]string{workspace},
		[]string{workspace},
		nil,
		[]string{awakenHome},
		filepath.Join(t.TempDir(), "scope_thread_1"),
	)
	permissionContext.ProtectedRootExceptions = []string{sessionsRoot}
	permissionContext = permissionContext.Normalize()

	intent := profile.ExecutionIntent{
		ToolName:        "Write",
		ToolKind:        profile.ToolKindFileOperation,
		RequestedAction: profile.ActionWrite,
		Paths:           []string{filepath.Join(awakenHome, "config", "models.json")},
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	output, err := NewFileOperationExecutor().Execute(ctx, intent, profile.WorkspaceWrite(), &noopTool{name: "Write"}, "test", nil)
	if err == nil || !output.IsError || output.ErrorCode != "file_operation_boundary" {
		t.Fatalf("Awaken metadata outside the sessions exception must remain denied: output=%+v err=%v", output, err)
	}
}

// --- GuardianExecutor tests ---

// TestGuardianExecutorAllowsOpenWorldReadOnlyUnderNetworkRestricted
// verifies that GuardianExecutor allows a read-only OpenWorld tool when
// the profile's NetworkPolicy is NetworkRestricted.
func TestGuardianExecutorAllowsOpenWorldReadOnlyUnderNetworkRestricted(t *testing.T) {
	executor := NewGuardianExecutor()
	intent := profile.ExecutionIntent{
		ToolName: "review_external",
		ToolKind: profile.ToolKindReviewAgent,
	}
	guardianProfile := profile.GuardianReadOnly() // NetworkRestricted
	roTool := &openWorldReadOnlyTool{name: "review_external"}

	output, err := executor.Execute(context.Background(), intent, guardianProfile, roTool, "test", nil)
	if err != nil {
		t.Fatalf("unexpected error for OpenWorld read-only tool under NetworkRestricted: %v", err)
	}
	if output.IsError {
		t.Fatalf("expected IsError=false, got true (ErrorCode=%q Content=%q)", output.ErrorCode, output.Content)
	}
	if output.Content != "open-world read-only output: test" {
		t.Fatalf("expected content %q, got %q", "open-world read-only output: test", output.Content)
	}
}

// --- VisibilityPolicy tests ---

// recordingObserver captures every OnCommandOutput call so tests can
// verify whether deltas were forwarded or dropped by the visibility
// filter.
type recordingObserver struct {
	outputs []tool.CommandOutput
}

func (r *recordingObserver) OnCommandOutput(output tool.CommandOutput) {
	r.outputs = append(r.outputs, output)
}

// TestVisibilityHiddenDropsAllDeltas verifies that VisibilityFilterObserver
// with VisibilityHidden drops all OnCommandOutput calls so the inner
// observer never sees any delta.
func TestVisibilityHiddenDropsAllDeltas(t *testing.T) {
	recorder := &recordingObserver{}
	wrapped := VisibilityFilterObserver(recorder, profile.VisibilityHidden)

	wrapped.OnCommandOutput(tool.CommandOutput{Stream: "stdout", Delta: "line 1"})
	wrapped.OnCommandOutput(tool.CommandOutput{Stream: "stderr", Delta: "line 2"})

	if len(recorder.outputs) != 0 {
		t.Fatalf("expected 0 outputs to reach inner observer, got %d", len(recorder.outputs))
	}
}

// TestVisibilitySummaryDropsDeltasButRecordsOutput verifies that
// VisibilityFilterObserver with VisibilitySummary drops real-time deltas
// (the inner observer never sees them) but records that output was
// observed so routeToExecutor can emit a truncated summary.
func TestVisibilitySummaryDropsDeltasButRecordsOutput(t *testing.T) {
	recorder := &recordingObserver{}
	wrapped := VisibilityFilterObserver(recorder, profile.VisibilitySummary)

	wrapped.OnCommandOutput(tool.CommandOutput{Stream: "stdout", Delta: "line 1"})
	wrapped.OnCommandOutput(tool.CommandOutput{Stream: "stderr", Delta: "line 2"})

	if len(recorder.outputs) != 0 {
		t.Fatalf("expected 0 outputs to reach inner observer, got %d", len(recorder.outputs))
	}
	if !SummaryHadOutput(wrapped) {
		t.Fatalf("expected SummaryHadOutput=true after deltas, got false")
	}
}

// TestVisibilitySummaryNoOutputWhenDeltaEmpty verifies that
// SummaryHadOutput is false when only empty deltas were sent.
func TestVisibilitySummaryNoOutputWhenDeltaEmpty(t *testing.T) {
	recorder := &recordingObserver{}
	wrapped := VisibilityFilterObserver(recorder, profile.VisibilitySummary)

	wrapped.OnCommandOutput(tool.CommandOutput{Stream: "stdout", Delta: ""})

	if SummaryHadOutput(wrapped) {
		t.Fatalf("expected SummaryHadOutput=false after empty delta, got true")
	}
}

// TestVisibilityFullPassesThroughDeltas verifies that
// VisibilityFilterObserver with VisibilityFull returns the original
// observer unchanged so deltas flow through as before.
func TestVisibilityFullPassesThroughDeltas(t *testing.T) {
	recorder := &recordingObserver{}
	wrapped := VisibilityFilterObserver(recorder, profile.VisibilityFull)

	if wrapped != recorder {
		t.Fatalf("expected VisibilityFull to return the original observer unchanged")
	}

	wrapped.OnCommandOutput(tool.CommandOutput{Stream: "stdout", Delta: "line 1"})
	wrapped.OnCommandOutput(tool.CommandOutput{Stream: "stderr", Delta: "line 2"})

	if len(recorder.outputs) != 2 {
		t.Fatalf("expected 2 outputs to reach inner observer, got %d", len(recorder.outputs))
	}
}

// TestVisibilityFilterObserverNilReturnsNil verifies that a nil observer
// is returned unchanged so the executor layer can pass it through to
// tools that do not implement ObservableTool.
func TestVisibilityFilterObserverNilReturnsNil(t *testing.T) {
	for _, policy := range []profile.VisibilityPolicy{
		profile.VisibilityHidden,
		profile.VisibilitySummary,
		profile.VisibilityFull,
	} {
		if got := VisibilityFilterObserver(nil, policy); got != nil {
			t.Fatalf("expected nil observer for policy %q, got %T", policy, got)
		}
	}
}

// --- TrashedPaths field test ---

// TestToolExecutionOutputTrashedPathsField verifies that the TrashedPaths
// field on ToolExecutionOutput is populated and returned correctly. This
// is a structural test: it confirms the field exists and can be read by
// routeToExecutor.
func TestToolExecutionOutputTrashedPathsField(t *testing.T) {
	out := ToolExecutionOutput{
		Content:      "done",
		TrashedPaths: []string{"/tmp/deleted.txt"},
	}
	if len(out.TrashedPaths) != 1 || out.TrashedPaths[0] != "/tmp/deleted.txt" {
		t.Fatalf("expected TrashedPaths=[/tmp/deleted.txt], got %v", out.TrashedPaths)
	}
}

// Ensure the strings import is used (for future test extensions that
// inspect content). This is a compile-time guard.
var _ = strings.TrimSpace
