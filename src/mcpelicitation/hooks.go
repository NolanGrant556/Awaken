package mcpelicitation

import "context"

type HookResult struct {
	Handled  bool
	Response Response
}

type Hook interface {
	ResolveURLElicitation(context.Context, Request) (HookResult, error)
}

type HookFunc func(context.Context, Request) (HookResult, error)

func (f HookFunc) ResolveURLElicitation(ctx context.Context, req Request) (HookResult, error) {
	return f(ctx, req)
}

type HookResolver struct {
	hooks []Hook
}

func NewHookResolver(hooks ...Hook) *HookResolver {
	return &HookResolver{hooks: hooks}
}

func (r *HookResolver) Resolve(ctx context.Context, req Request) (Response, bool, error) {
	if r == nil {
		return Response{}, false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for _, hook := range r.hooks {
		if hook == nil {
			continue
		}
		result, err := hook.ResolveURLElicitation(ctx, req)
		if err != nil {
			return Response{}, false, err
		}
		if !result.Handled {
			continue
		}
		response := result.Response
		if response.Action == "" {
			response.Action = ActionHookResolved
		}
		if response.Source == "" {
			response.Source = SourceHook
		}
		return response, true, nil
	}
	return Response{}, false, nil
}
