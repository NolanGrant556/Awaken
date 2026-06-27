package ratelimit

import (
	"net/http"
	"sync"
	"time"
)

type Tracker struct {
	mu     sync.RWMutex
	states map[trackerKey]State
}

type trackerKey struct {
	provider string
	model    string
	key      string
}

func NewTracker() *Tracker {
	return &Tracker{states: make(map[trackerKey]State)}
}

func (t *Tracker) Update(provider, model, key string, headers http.Header) (State, bool) {
	state, ok := ParseHeaders(headers, provider)
	if !ok {
		return State{}, false
	}
	t.UpdateState(provider, model, key, state)
	return state, true
}

func (t *Tracker) UpdateState(provider, model, key string, state State) bool {
	if !state.HasData() {
		return false
	}
	if state.Provider == "" {
		state.Provider = provider
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.states[trackerKey{provider: provider, model: model, key: key}] = state
	return true
}

func (t *Tracker) Snapshot(provider, model, key string) (State, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	state, ok := t.states[trackerKey{provider: provider, model: model, key: key}]
	return state, ok
}

func (t *Tracker) HotWarnings(provider, model, key string, now time.Time) []Warning {
	state, ok := t.Snapshot(provider, model, key)
	if !ok {
		return nil
	}
	return state.HotWarnings(now)
}

func (t *Tracker) ShouldThrottle(provider, model, key string, now time.Time) bool {
	decision, ok := t.ThrottleDecision(provider, model, key, now)
	return ok && decision.ShouldThrottle
}

func (t *Tracker) ThrottleDecision(provider, model, key string, now time.Time) (ThrottleDecision, bool) {
	state, ok := t.Snapshot(provider, model, key)
	if !ok {
		return ThrottleDecision{}, false
	}
	return state.ThrottleDecision(now), true
}
