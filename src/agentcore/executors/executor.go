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

// ToolExecutionOutput carries the outcome of an executor invocation.
type ToolExecutionOutput struct {
	// Outcome is the canonical structured tool result.
	Outcome tool.ExecutionOutcome
	// Content is retained for compatibility; prefer Outcome.Content.
	Content string
	// Artifact is an optional artifact descriptor produced by the executor.
	Artifact *tool.ArtifactDescriptor
	// PatchSummary is a short summary of a patch operation, if applicable.
	PatchSummary string
	// AffectedPaths is the list of paths the tool touched, if known.
	AffectedPaths []string
	// TrashedPaths lists the absolute paths moved to the OS recycle bin.
	TrashedPaths []string
	// IsError is derived from Outcome when present; retained for one compatibility cycle.
	IsError bool
	// ErrorCode is derived from Outcome when present.
	ErrorCode string
}

// executeWithObserver delegates to OutcomeTool when available, otherwise
// ObservableTool / Execute, adapting legacy string results into Outcome.
func executeWithObserver(ctx context.Context, t tool.Tool, input string, observer tool.ToolObserver) (ToolExecutionOutput, error) {
	outcome, err := tool.ExecuteAsOutcome(ctx, t, input, observer)
	out := ToolExecutionOutput{
		Outcome:       outcome,
		Content:       outcome.Content,
		AffectedPaths: append([]string(nil), outcome.AffectedPaths...),
		PatchSummary:  outcome.PatchSummary,
		IsError:       outcome.IsError(),
		ErrorCode:     outcome.ErrorCode,
	}
	if len(outcome.Artifacts) == 1 {
		artifact := outcome.Artifacts[0]
		out.Artifact = &artifact
	}
	return out, err
}

// deniedOutput builds a ToolExecutionOutput and error pair for an executor
// denial. The denial message is surfaced both in the output content (so the
// runtime can display it to the user) and in the error (so the runtime
// treats it as a failed execution).
func deniedOutput(code, message string) (ToolExecutionOutput, error) {
	outcome := tool.DeniedOutcome(message, code)
	return ToolExecutionOutput{
		Outcome:   outcome,
		Content:   message,
		IsError:   true,
		ErrorCode: code,
	}, fmt.Errorf("%s", message)
}
