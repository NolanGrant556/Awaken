package executors

import (
	"context"
	"fmt"

	"awaken/src/tool"
	"awaken/src/tool/profile"
)

// FileOperationExecutor provides defense-in-depth for typed file operations.
// Read boundaries remain owned by each tool's PermissionContext checks;
// mutations are rechecked for read-only profiles and sensitive paths before
// the tool performs the same canonical/symlink validation again. Workspace
// roots remain resolution defaults, not a mutation fence.
type FileOperationExecutor struct{}

func NewFileOperationExecutor() *FileOperationExecutor { return &FileOperationExecutor{} }

func (e *FileOperationExecutor) Execute(
	ctx context.Context,
	intent profile.ExecutionIntent,
	execProfile profile.ExecutionProfile,
	t tool.Tool,
	input string,
	observer tool.ToolObserver,
) (ToolExecutionOutput, error) {
	if isMutationAction(intent.RequestedAction) && execProfile.PermissionProfile == profile.PermissionReadOnly {
		return deniedOutput("read_only_violation", fmt.Sprintf("profile is read_only but tool requests %s", intent.RequestedAction))
	}
	if err := enforceFileOperationBoundary(ctx, intent); err != nil {
		return deniedOutput("file_operation_boundary", err.Error())
	}
	return executeWithObserver(ctx, t, input, observer)
}

func enforceFileOperationBoundary(ctx context.Context, intent profile.ExecutionIntent) error {
	if !isMutationAction(intent.RequestedAction) || len(intent.Paths) == 0 {
		return nil
	}
	if !tool.HasPermissionContext(ctx) {
		return fmt.Errorf("file mutation denied: permission context unavailable")
	}
	permissionContext := tool.PermissionContextFromContext(ctx)
	for _, path := range intent.Paths {
		if _, err := permissionContext.ResolveMutationPathChecked(path); err != nil {
			return fmt.Errorf("file mutation denied: %v", err)
		}
	}
	return nil
}

func isMutationAction(action profile.RequestedAction) bool {
	switch action {
	case profile.ActionWrite, profile.ActionDelete, profile.ActionMove:
		return true
	default:
		return false
	}
}
