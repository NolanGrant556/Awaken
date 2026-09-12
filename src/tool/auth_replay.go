package tool

import (
	"context"
	"sync"
)

type authReplayKey struct{}

// AuthReplayBudget belongs to one original tool call, including any SDK retry
// and explicit external-auth continuation. It is never model input.
type AuthReplayBudget struct {
	mu      sync.Mutex
	used    bool
	binding string
}

func WithAuthReplayBudget(ctx context.Context) context.Context {
	if _, ok := ctx.Value(authReplayKey{}).(*AuthReplayBudget); ok {
		return ctx
	}
	return context.WithValue(ctx, authReplayKey{}, &AuthReplayBudget{})
}

func AuthReplayBudgetFromContext(ctx context.Context) *AuthReplayBudget {
	value, _ := ctx.Value(authReplayKey{}).(*AuthReplayBudget)
	return value
}

func (b *AuthReplayBudget) Bind(binding string) bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.binding == "" {
		b.binding = binding
	}
	return b.binding == binding
}

func (b *AuthReplayBudget) Take() bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.used {
		return false
	}
	b.used = true
	return true
}

func (b *AuthReplayBudget) Used() bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used
}
