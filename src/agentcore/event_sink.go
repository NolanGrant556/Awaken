package agentcore

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
