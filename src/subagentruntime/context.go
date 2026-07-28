package subagentruntime

import "context"

type controllerContextKey struct{}

func WithController(ctx context.Context, controller Controller) context.Context {
	return context.WithValue(ctx, controllerContextKey{}, controller)
}

func ControllerFromContext(ctx context.Context) (Controller, bool) {
	controller, ok := ctx.Value(controllerContextKey{}).(Controller)
	return controller, ok && controller != nil
}
