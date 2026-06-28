package fastmode

import (
	"sync"
	"time"
)

type Runtime struct {
	mu    sync.Mutex
	state RuntimeState
}

func NewRuntime() *Runtime {
	return &Runtime{state: RuntimeState{Status: RuntimeActive}}
}

func (r *Runtime) TriggerCooldown(now time.Time, resetAt time.Time, reason CooldownReason) []EventPlan {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.state = RuntimeState{Status: RuntimeCooldown, ResetAt: resetAt, Reason: reason}
	return []EventPlan{{
		Type:    EventCooldownTriggered,
		At:      now,
		ResetAt: resetAt,
		Reason:  reason,
	}}
}

func (r *Runtime) GetRuntimeState(now time.Time) (RuntimeState, []EventPlan) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state.Status == RuntimeCooldown && !now.Before(r.state.ResetAt) {
		expired := r.state
		r.state = RuntimeState{Status: RuntimeActive}
		return r.state, []EventPlan{{
			Type:    EventCooldownExpired,
			At:      now,
			ResetAt: expired.ResetAt,
			Reason:  expired.Reason,
		}}
	}
	if r.state.Status == "" {
		r.state.Status = RuntimeActive
	}
	return r.state, nil
}

func (r *Runtime) ClearCooldown() []EventPlan {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.state = RuntimeState{Status: RuntimeActive}
	return nil
}

var defaultRuntime = NewRuntime()

func TriggerCooldown(resetAt time.Time, reason CooldownReason) []EventPlan {
	return defaultRuntime.TriggerCooldown(time.Now(), resetAt, reason)
}

func GetRuntimeState() (RuntimeState, []EventPlan) {
	return defaultRuntime.GetRuntimeState(time.Now())
}

func ClearCooldown() []EventPlan {
	return defaultRuntime.ClearCooldown()
}
