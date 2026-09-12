// Package runcontrol carries host cancellation provenance through existing
// contexts. It does not schedule work or define run states.
package runcontrol

import (
	"context"
	"errors"
	"sync"
)

type Source string

const (
	User     Source = "user"
	Parent   Source = "parent"
	Shutdown Source = "shutdown"
	Timeout  Source = "timeout"
	Unknown  Source = "unknown"
)

type stateKey struct{}
type requestKey struct{}
type stopState struct {
	mu     sync.Mutex
	source Source
	parent context.Context
}

func Normalize(source Source) Source {
	switch source {
	case User, Parent, Shutdown, Timeout, Unknown:
		return source
	}
	return Unknown
}

func WithCancel(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	return context.WithValue(ctx, stateKey{}, &stopState{parent: parent}), cancel
}

// SetSource fixes the first cancellation cause before its signal is sent.
func SetSource(ctx context.Context, source Source) {
	if ctx == nil {
		return
	}
	if state, ok := ctx.Value(stateKey{}).(*stopState); ok {
		state.mu.Lock()
		defer state.mu.Unlock()
		if state.source == "" {
			if state.parent.Err() != nil {
				state.source = FromContext(state.parent)
			} else {
				state.source = Normalize(source)
			}
		}
	}
}

func FromContext(ctx context.Context) Source {
	if ctx == nil {
		return Unknown
	}
	if state, ok := ctx.Value(stateKey{}).(*stopState); ok {
		state.mu.Lock()
		source, parent := state.source, state.parent
		state.mu.Unlock()
		if source != "" {
			return source
		}
		if parent.Err() != nil {
			return FromContext(parent)
		}
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return Timeout
	}
	return Unknown
}

// WithRequestSource is only used by host control entrypoints, never tool args.
func WithRequestSource(ctx context.Context, source Source) context.Context {
	return context.WithValue(ctx, requestKey{}, Normalize(source))
}

func RequestSource(ctx context.Context, fallback Source) Source {
	if source, ok := ctx.Value(requestKey{}).(Source); ok {
		return source
	}
	return fallback
}

func Message(source Source) string {
	switch source {
	case User:
		return "你停止了本次运行"
	case Parent:
		return "主任务已停止此子任务"
	case Shutdown:
		return "应用关闭，运行已中断"
	case Timeout:
		return "本次运行已超时"
	default:
		return "本次运行已中断"
	}
}
