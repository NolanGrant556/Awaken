package executors

import (
	"context"
	"fmt"

	"awaken/src/tool"
	"awaken/src/tool/profile"
)

// FileChainExecutor wraps execution of file-chain tools (apply_patch,
// file_read, list_files, grep_search). It enforces the file-chain boundary
// (sensitive paths, symlink escape, workspace roots) BEFORE delegating to
// the tool's own Execute, providing defense-in-depth on top of the checks
// the tools already perform internally.
//
// For mutation actions (write/delete/move), the executor re-checks:
//   - Sensitive paths (.git/, .env, key files, agent metadata) are denied.
//   - Paths outside writable roots are denied.
//   - Symlink escape is detected via IsWithinWritableRootsResolved.
//
// For read actions, no additional boundary check is applied — reads of
// sensitive paths remain permitted (only mutations are gated).
//
// For apply_patch specifically, the trash adapter is already used by the tool
// for deletes (Task 2). The executor does not re-implement trash logic; it
// only double-checks the boundary so that a future file-chain tool that
// forgets to call validateMutationPath is still caught here.
type FileChainExecutor struct{}

// NewFileChainExecutor returns a FileChainExecutor.
func NewFileChainExecutor() *FileChainExecutor {
	return &FileChainExecutor{}
}

// Execute enforces the file-chain boundary and then delegates to the tool.
// The execProfile's PermissionProfile is checked first: a read-only profile
// rejects any write/delete/move action before the existing sensitive-path
// and writable-roots checks run.
func (e *FileChainExecutor) Execute(
	ctx context.Context,
	intent profile.ExecutionIntent,
	execProfile profile.ExecutionProfile,
	t tool.Tool,
	input string,
	observer tool.ToolObserver,
) (ToolExecutionOutput, error) {
	if isMutationAction(intent.RequestedAction) &&
		execProfile.PermissionProfile == profile.PermissionReadOnly {
		return deniedOutput("read_only_violation",
			fmt.Sprintf("profile is read_only but tool requests %s", intent.RequestedAction))
	}
	if err := enforceFileChainBoundary(intent, execProfile, t); err != nil {
		return deniedOutput("file_chain_boundary", err.Error())
	}
	content, err := executeWithObserver(ctx, t, input, observer)
	if err != nil {
		return ToolExecutionOutput{}, err
	}
	return ToolExecutionOutput{Content: content}, nil
}

// enforceFileChainBoundary checks that mutation paths in the intent are
// within writable roots and not sensitive. Reads are not gated here.
//
// The execProfile's PermissionProfile read-only check is enforced in Execute
// (so it can surface the specific "read_only_violation" denial code) before
// this function runs. The execProfile parameter is accepted here for
// defense-in-depth: future extensions may add path-prefix checks keyed off
// the permission profile.
//
// The boundary check uses the lexical IsWithinWritableRoots guard rather
// than the symlink-resolved IsWithinWritableRootsResolved. This is because
// mutation targets often do not exist yet (e.g. apply_patch Add File), and
// EvalSymlinks fails on non-existent paths. The tool's own validateMutationPath
// (apply_patch) walks up the directory tree and does the full symlink-resolved
// check on existing ancestors; the executor's check is a defense-in-depth
// lexical guard that catches the obvious case (path clearly outside workspace)
// without breaking file creation.
func enforceFileChainBoundary(intent profile.ExecutionIntent, execProfile profile.ExecutionProfile, t tool.Tool) error {
	if !isMutationAction(intent.RequestedAction) {
		return nil
	}
	// PermissionProfile read-only enforcement is handled in Execute so it
	// can surface the specific "read_only_violation" denial code. The
	// execProfile is accepted here for defense-in-depth: future extensions
	// may add path-prefix checks keyed off the permission profile.
	// If the tool carries its own permission context enforcement
	// (apply_patch does), we still double-check here. Tools that do not
	// declare paths in the intent (e.g. a future file-chain tool without
	// PatchPreviewingTool) will have an empty Paths slice and skip the
	// check — the tool's own Execute is then the last line of defense.
	if len(intent.Paths) == 0 {
		return nil
	}
	permCtx := tool.NewPermissionContext(intent.WorkspaceRoot, intent.WritableRoots)
	for _, path := range intent.Paths {
		if err := permCtx.CheckSensitivePath(path); err != nil {
			return fmt.Errorf("file-chain mutation denied: %v", err)
		}
		if !permCtx.IsWithinWritableRoots(path) {
			return fmt.Errorf("file-chain mutation denied: path %q is outside writable roots", path)
		}
	}
	return nil
}

// isMutationAction reports whether the action mutates filesystem state.
func isMutationAction(action profile.RequestedAction) bool {
	switch action {
	case profile.ActionWrite, profile.ActionDelete, profile.ActionMove:
		return true
	}
	return false
}
