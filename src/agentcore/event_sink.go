package agentcore

import "context"

type EventSink interface {
	Emit(AwakenEvent)
}

type UserQuestionAwaiter interface {
	AwaitUserQuestion(ctx context.Context, request AwakenEvent) (string, error)
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
