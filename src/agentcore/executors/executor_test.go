package executors

import (
	"context"
	"strings"
	"testing"

	"awaken/src/tool"
	"awaken/src/tool/profile"
)

// --- Mock tools for executor tests ---

// noopTool is a minimal tool that returns a fixed string. It is used by
// ShellExecutor and FileChainExecutor tests where the executor should
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

// --- ShellExecutor tests ---

// TestShellExecutorAllowsSandboxRequiredProfile verifies that ShellExecutor
// no longer treats SandboxRequired as a hard gate. Shell is a local-computer
// execution surface; command behavior follows the selected shell tool policy.
func TestShellExecutorAllowsSandboxRequiredProfile(t *testing.T) {
	executor := NewShellExecutor()
	intent := profile.ExecutionIntent{
		ToolName: "shell",
		ToolKind: profile.ToolKindShellCommand,
	}
	shellProfile := profile.GuardianReadOnly()
	noop := &noopTool{name: "shell"}

	output, err := executor.Execute(context.Background(), intent, shellProfile, noop, "test", nil)
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

// TestShellExecutorAllowsBestEffortProfile verifies that the shell wrapper
// delegates to the shell tool under the normal WorkspaceWrite profile.
func TestShellExecutorAllowsBestEffortProfile(t *testing.T) {
	executor := NewShellExecutor()
	intent := profile.ExecutionIntent{
		ToolName: "shell",
		ToolKind: profile.ToolKindShellCommand,
	}
	shellProfile := profile.WorkspaceWrite() // SandboxBestEffort
	noop := &noopTool{name: "shell"}

	output, err := executor.Execute(context.Background(), intent, shellProfile, noop, "test", nil)
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

// --- FileChainExecutor tests ---

// TestFileChainExecutorDeniesWriteUnderReadOnlyProfile verifies that
// FileChainExecutor denies a write action when the profile is
// PermissionReadOnly.
func TestFileChainExecutorDeniesWriteUnderReadOnlyProfile(t *testing.T) {
	executor := NewFileChainExecutor()
	intent := profile.ExecutionIntent{
		ToolName:        "apply_patch",
		ToolKind:        profile.ToolKindFileChain,
		RequestedAction: profile.ActionWrite,
	}
	readOnlyProfile := profile.WorkspaceReadOnly() // PermissionReadOnly
	noop := &noopTool{name: "apply_patch"}

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

// TestFileChainExecutorAllowsWriteUnderWorkspaceWriteProfile verifies that
// FileChainExecutor allows a write action when the profile is
// PermissionWorkspaceWrite.
func TestFileChainExecutorAllowsWriteUnderWorkspaceWriteProfile(t *testing.T) {
	executor := NewFileChainExecutor()
	intent := profile.ExecutionIntent{
		ToolName:        "apply_patch",
		ToolKind:        profile.ToolKindFileChain,
		RequestedAction: profile.ActionWrite,
		// Paths intentionally empty: the boundary check skips when no
		// paths are declared, so the tool's own Execute is the last line
		// of defense (as documented in enforceFileChainBoundary).
	}
	writeProfile := profile.WorkspaceWrite() // PermissionWorkspaceWrite
	noop := &noopTool{name: "apply_patch"}

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
