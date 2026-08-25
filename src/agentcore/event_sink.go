package agentcore

import "context"

type EventSink interface {
	Emit(AwakenEvent)
}

type UserQuestionResolutionStatus string

const (
	UserQuestionAnswered  UserQuestionResolutionStatus = "answered"
	UserQuestionCancelled UserQuestionResolutionStatus = "cancelled"
)

type UserQuestionResolution struct {
	Status UserQuestionResolutionStatus
	Answer string
}

type UserQuestionAwaiter interface {
	AwaitUserQuestion(ctx context.Context, request AwakenEvent) (UserQuestionResolution, error)
}

type ExternalInteractionAwaiter interface {
	AwaitExternalInteraction(ctx context.Context, request AwakenEvent) (ExternalInteractionResolution, error)
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

type eventSinkContextKey struct{}

// ContextWithEventSink attaches an EventSink to the context so tools can emit
// live events during execution. The returned context is safe to pass through
// tool boundaries.
func ContextWithEventSink(ctx context.Context, sink EventSink) context.Context {
	return context.WithValue(ctx, eventSinkContextKey{}, sink)
}

// EventSinkFromContext retrieves the EventSink stored by ContextWithEventSink.
// It returns a noop sink when none is present so callers can Emit without nil checks.
func EventSinkFromContext(ctx context.Context) EventSink {
	if sink, ok := ctx.Value(eventSinkContextKey{}).(EventSink); ok {
		return sink
	}
	return noopEventSink{}
}
