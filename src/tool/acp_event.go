package tool

import "context"

// ACPEventType is the stable set of ACP event kinds emitted by the
// delegate_external_agent tool and the ACP manager.
type ACPEventType string

const (
	ACPEventTypeSessionStarted   ACPEventType = "acp_session_started"
	ACPEventTypeSessionClosed    ACPEventType = "acp_session_closed"
	ACPEventTypeSessionInterrupted ACPEventType = "acp_session_interrupted"
	ACPEventTypeTurnStarted      ACPEventType = "acp_turn_started"
	ACPEventTypeTurnFinished     ACPEventType = "acp_turn_finished"
	ACPEventTypeToolSnapshot     ACPEventType = "acp_tool_snapshot"
	ACPEventTypePermissionRequested ACPEventType = "acp_permission_requested"
	ACPEventTypePermissionResolved  ACPEventType = "acp_permission_resolved"
)

// ACPEvent is a tool-package neutral representation of an ACP (Agent Client
// Protocol) lifecycle or progress event. It is intentionally free of
// agentcore.AwakenEvent so that the ACP subsystem and individual tools can emit
// events without creating an import cycle with agentcore.
type ACPEvent struct {
	Type          ACPEventType
	ThreadID      string
	RunID         string
	ToolCallID    string
	InteractionID string
	ToolName      string
	Content       string
	Status        string
	Diagnostic    map[string]any
}

// ACPEventEmitter receives ACP events produced during an external agent turn.
type ACPEventEmitter func(event ACPEvent)

type acpEventEmitterContextKey struct{}

// ContextWithACPEventEmitter attaches an ACP event emitter to the context.
// The runtime is expected to provide an implementation that maps ACPEvent to
// the canonical agentcore.AwakenEvent stream.
func ContextWithACPEventEmitter(ctx context.Context, emit ACPEventEmitter) context.Context {
	return context.WithValue(ctx, acpEventEmitterContextKey{}, emit)
}

// ACPEventEmitterFromContext retrieves the emitter stored by ContextWithACPEventEmitter.
// It returns nil when no emitter is present.
func ACPEventEmitterFromContext(ctx context.Context) ACPEventEmitter {
	if emit, ok := ctx.Value(acpEventEmitterContextKey{}).(ACPEventEmitter); ok {
		return emit
	}
	return nil
}
