package agentcore

import stdcontext "context"

// transformCtxKey keeps the generic one-level transform guard available for
// provider-neutral future transforms. No built-in seven-tool call is rewritten.
type transformCtxKey struct{}

func transformDepthFromCtx(ctx stdcontext.Context) int {
	if value, ok := ctx.Value(transformCtxKey{}).(int); ok {
		return value
	}
	return 0
}

func ctxWithTransformDepth(ctx stdcontext.Context, depth int) stdcontext.Context {
	return stdcontext.WithValue(ctx, transformCtxKey{}, depth)
}
