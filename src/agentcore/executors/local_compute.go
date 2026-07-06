package executors

import (
	"context"
	"fmt"

	"awaken/src/tool"
	"awaken/src/tool/profile"
)

// LocalComputeExecutor hosts restricted local transformations that do not use
// the broad shell surface.
type LocalComputeExecutor struct{}

func NewLocalComputeExecutor() *LocalComputeExecutor {
	return &LocalComputeExecutor{}
}

func (e *LocalComputeExecutor) Execute(
	ctx context.Context,
	intent profile.ExecutionIntent,
	execProfile profile.ExecutionProfile,
	t tool.Tool,
	input string,
	observer tool.ToolObserver,
) (ToolExecutionOutput, error) {
	if execProfile.NetworkPolicy != profile.NetworkNone {
		return deniedOutput("local_compute_network_denied", "local compute lane must run without network access")
	}
	if intent.RequestedAction != profile.ActionRead &&
		intent.RequestedAction != profile.ActionInspect &&
		intent.RequestedAction != profile.ActionWrite &&
		intent.RequestedAction != profile.ActionMove {
		return deniedOutput("local_compute_unsupported_action", fmt.Sprintf("local compute lane does not support %s", intent.RequestedAction))
	}
	content, err := executeWithObserver(ctx, t, input, observer)
	if err != nil {
		return ToolExecutionOutput{}, err
	}
	return ToolExecutionOutput{Content: content}, nil
}
