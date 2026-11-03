package agentcore

import (
	"context"

	conversation "awaken/src/context"
	"awaken/src/tool"
)

const TransitionSteeringContinuation TransitionReason = "steering_continuation"

// SteeringInput is a user message admitted at a model request boundary.
type SteeringInput struct {
	ID          string
	Text        string
	Attachments []conversation.MessageAttachment
}

// SteeringSource is implemented by the root runtime session. The runner
// consumes it only between complete model/tool exchanges.
type SteeringSource interface {
	TakeSteering(context.Context) ([]SteeringInput, error)
	SealSteering(allowContinuation bool) bool
}

func (r *Runner) admitSteering(ctx context.Context, source SteeringSource) error {
	if source == nil {
		return nil
	}
	inputs, err := source.TakeSteering(ctx)
	if err != nil {
		return err
	}
	for _, input := range inputs {
		for _, attachment := range input.Attachments {
			if attachment.Kind == "file" && r.toolRuntime != nil {
				r.toolRuntime.permissionContext.AttachedFiles = append(r.toolRuntime.permissionContext.AttachedFiles,
					tool.ScopedFile{Path: attachment.Path, DisplayName: attachment.DisplayName})
			}
		}
		r.conversation.UserInputWithAttachments(input.Text, input.Attachments)
		r.pendingSteering = append(r.pendingSteering, input)
	}
	return nil
}
