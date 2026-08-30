package tool

import "context"

// ElicitationRequest is a protocol-neutral, run-scoped request for user input.
// Form schemas are already validated by the owning protocol client.
type ElicitationRequest struct {
	Provider        string
	Mode            string
	Message         string
	RequestedSchema map[string]any
	URL             string
	RequestID       string
}

type ElicitationAnswer struct {
	Action  string
	Content map[string]any
}

type ElicitationHandler func(context.Context, ElicitationRequest) (ElicitationAnswer, error)

type elicitationHandlerContextKey struct{}

func ContextWithElicitationHandler(ctx context.Context, handler ElicitationHandler) context.Context {
	if handler == nil {
		return ctx
	}
	return context.WithValue(ctx, elicitationHandlerContextKey{}, handler)
}

func ElicitationHandlerFromContext(ctx context.Context) (ElicitationHandler, bool) {
	if ctx == nil {
		return nil, false
	}
	handler, ok := ctx.Value(elicitationHandlerContextKey{}).(ElicitationHandler)
	return handler, ok && handler != nil
}
