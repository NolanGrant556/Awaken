package skillruntime

import "context"

// ActivationObserver receives an immutable per-invocation plan before the
// Skill executor is allowed to run. Returning an error fails the tool call
// closed, which lets the runner make durable activation records a prerequisite
// for executing the Skill body.
type ActivationObserver func(context.Context, *SkillActivationPlan) error

type activationScope struct {
	parentToolPool []string
	observer       ActivationObserver
}

type activationScopeKey struct{}

// WithActivationScope binds one implicit Skill invocation to its current
// runner's parent tool boundary and durable observer. A non-nil empty parent
// pool remains an explicit deny-all boundary. Both input and delivered plans
// are copied so concurrent threads cannot share mutable activation state.
func WithActivationScope(ctx context.Context, parentToolPool []string, observer ActivationObserver) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	var parentCopy []string
	if parentToolPool != nil {
		parentCopy = append([]string(nil), parentToolPool...)
	}
	return context.WithValue(ctx, activationScopeKey{}, activationScope{
		parentToolPool: parentCopy,
		observer:       observer,
	})
}

func activationScopeFromContext(ctx context.Context) (activationScope, bool) {
	if ctx == nil {
		return activationScope{}, false
	}
	scope, ok := ctx.Value(activationScopeKey{}).(activationScope)
	if !ok {
		return activationScope{}, false
	}
	if scope.parentToolPool != nil {
		scope.parentToolPool = append([]string(nil), scope.parentToolPool...)
	}
	return scope, true
}

func notifyActivation(ctx context.Context, scope activationScope, plan *SkillActivationPlan) error {
	if scope.observer == nil || plan == nil {
		return nil
	}
	return scope.observer(ctx, plan.Clone())
}
