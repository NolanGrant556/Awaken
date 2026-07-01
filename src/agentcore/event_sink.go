package agentcore

import (
	stdcontext "context"

	"awaken/src/tool"
)

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
	// ToolDefinition carries the full metadata of the tool being approved.
	ToolDefinition tool.ToolDefinition
	// RiskHints carries the risk profile for the approval UI.
	RiskHints *tool.RiskHints
}

type ApprovalAwaiter interface {
	AwaitApproval(ctx stdcontext.Context, request ApprovalRequest) (ApprovalDecision, error)
}
