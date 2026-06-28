package agentcore

import stdcontext "context"

import "awaken/src/tool"

type EventSink interface {
	Emit(AwakenEvent)
}

type EventSinkFunc func(AwakenEvent)

func (f EventSinkFunc) Emit(event AwakenEvent) {
	if f != nil {
		f(event)
	}
}

type noopEventSink struct{}

func (noopEventSink) Emit(AwakenEvent) {}

func normalizeEventSink(sink EventSink) EventSink {
	if sink == nil {
		return noopEventSink{}
	}
	return sink
}

type ApprovalRequest struct {
	ID            string
	ToolCallID    string
	ToolName      string
	Title         string
	Description   string
	RiskTone      ApprovalRiskTone
	AffectedPaths []string
	PatchSummary  string
	DiffReview    *tool.DiffReviewMetadata
}

type ApprovalAwaiter interface {
	AwaitApproval(ctx stdcontext.Context, request ApprovalRequest) (ApprovalDecision, error)
}
