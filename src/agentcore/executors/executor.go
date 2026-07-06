// Package executors defines the unified executor layer that sits between the
// auto tool policy decision and the raw tool execution. Each ToolExecutor
// wraps tool execution with kind-specific enforcement:
//
//   - FileChainExecutor: enforces the file-chain boundary (sensitive paths,
//     symlink escape, workspace roots) before delegating to the tool.
//   - ShellExecutor: a thin wrapper that delegates to the existing
//     tools/shell.ShellExecutor (via the shell tool's own Execute), ensuring
//     the sandbox adapter is consulted.
//   - McpExecutor: checks MCP risk annotations; denies fail-closed when
//     annotations are missing.
//   - GuardianExecutor: enforces read-only for guardian/review agents.
//
// A Router maps an ExecutionIntent + ExecutionProfile to the appropriate
// ToolExecutor. ToolRuntime uses the Router after the auto policy decision
// is "allow" (or after a transform produces the final intent).
package executors

import (
	"context"
	"fmt"

	"awaken/src/tool"
	"awaken/src/tool/profile"
)

// ToolExecutor is the common interface all executors implement. It wraps
// tool execution with kind-specific enforcement (boundary checks, sandbox
// consultation, risk annotation checks, read-only enforcement).
type ToolExecutor interface {
	// Execute runs the tool with the given input and observer. The intent
	// provides the tool kind, requested action, and paths for enforcement.
	// The execProfile carries the unified ExecutionProfile (approval,
	// permission, sandbox, network, env, visibility policies) that the
	// executor enforces before delegating to the tool. The observer, if
	// non-nil, receives streamed stdout/stderr deltas for ObservableTool
	// implementations so the existing streaming pattern is preserved across
	// the executor layer.
	Execute(ctx context.Context, intent profile.ExecutionIntent, execProfile profile.ExecutionProfile, t tool.Tool, input string, observer tool.ToolObserver) (ToolExecutionOutput, error)
}

// ToolExecutionOutput carries the outcome of an executor invocation. It
// mirrors what ToolRuntime.executeTool currently produces (a content string)
// plus fields for executor-level artifact and path tracking.
type ToolExecutionOutput struct {
	// Content is the tool's output string.
	Content string
	// Artifact is an optional artifact descriptor produced by the executor.
	// Currently unused; reserved for future executor-level artifact surfacing.
	Artifact *tool.ArtifactDescriptor
	// PatchSummary is a short summary of a patch operation, if applicable.
	PatchSummary string
	// AffectedPaths is the list of paths the tool touched, if known.
	AffectedPaths []string
	// TrashedPaths lists the absolute paths moved to the OS recycle bin
	// during this call. Executors that perform deletions via the trash
	// adapter populate this field directly so routeToExecutor can surface
	// "Deleted file moved to recycle bin" without re-parsing the tool's
	// output. When empty, routeToExecutor falls back to extractTrashedPaths
	// for backwards compatibility.
	TrashedPaths []string
	// IsError indicates the executor denied or failed the call (vs. the
	// tool returning an error, which is surfaced via the error return).
	IsError bool
	// ErrorCode is a machine-readable error code when IsError is true.
	ErrorCode string
}

// executeWithObserver delegates to the tool's ExecuteWithObserver if the
// tool implements ObservableTool and an observer is provided; otherwise it
// falls back to Execute. This mirrors ToolRuntime.executeTool's dispatch
// logic so the observer pattern is preserved across the executor layer.
func executeWithObserver(ctx context.Context, t tool.Tool, input string, observer tool.ToolObserver) (string, error) {
	if observableTool, ok := t.(tool.ObservableTool); ok && observer != nil {
		return observableTool.ExecuteWithObserver(ctx, input, observer)
	}
	return t.Execute(ctx, input)
}

// deniedOutput builds a ToolExecutionOutput and error pair for an executor
// denial. The denial message is surfaced both in the output content (so the
// runtime can display it to the user) and in the error (so the runtime
// treats it as a failed execution).
func deniedOutput(code, message string) (ToolExecutionOutput, error) {
	return ToolExecutionOutput{
		Content:   message,
		IsError:   true,
		ErrorCode: code,
	}, fmt.Errorf("%s", message)
}
