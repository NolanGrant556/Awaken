package busyinput

import (
	"fmt"
	"strings"
	"time"
)

const DefaultAckCooldown = 30 * time.Second

type Mode string

const (
	ModeInterrupt Mode = "interrupt"
	ModeQueue     Mode = "queue"
	ModeSteer     Mode = "steer"
)

type State string

const (
	StateBusy       State = "busy"
	StateDraining   State = "draining"
	StateRestarting State = "restarting"
	StateIdle       State = "idle"
)

type Action string

const (
	ActionQueuePending        Action = "queue_pending"
	ActionSteerSucceeded      Action = "steer_succeeded"
	ActionSteerFallbackQueued Action = "steer_fallback_queued"
	ActionInterruptRequested  Action = "interrupt_requested"
	ActionDrainingQueued      Action = "draining_queued"
	ActionDrainingRejected    Action = "draining_rejected"
	ActionRejected            Action = "rejected"
)

type Request struct {
	Mode           Mode
	State          State
	CanSteer       bool
	SteerSucceeded bool
	LastAck        time.Time
	Now            time.Time
	AckCooldown    time.Duration
}

type Decision struct {
	Mode                Mode   `json:"mode"`
	State               State  `json:"state"`
	Action              Action `json:"action"`
	Accept              bool   `json:"accept"`
	QueuePending        bool   `json:"queue_pending"`
	SteerAttempted      bool   `json:"steer_attempted"`
	SteerSucceeded      bool   `json:"steer_succeeded"`
	SteerFallbackQueued bool   `json:"steer_fallback_queued"`
	InterruptRequested  bool   `json:"interrupt_requested"`
	Rejected            bool   `json:"rejected"`
	AckSuppressed       bool   `json:"ack_suppressed"`
	Reason              string `json:"reason"`
	Status              string `json:"status"`
}

type ActivitySummary struct {
	RunningFor     time.Duration `json:"-"`
	RunningMinutes int           `json:"running_minutes,omitempty"`
	TurnIndex      int           `json:"turn_index,omitempty"`
	MaxTurns       int           `json:"max_turns,omitempty"`
	CurrentTool    string        `json:"current_tool,omitempty"`
}

type Ack struct {
	Suppressed bool            `json:"suppressed"`
	Mode       Mode            `json:"mode"`
	State      State           `json:"state"`
	Action     Action          `json:"action"`
	Reason     string          `json:"reason"`
	Status     string          `json:"status"`
	Activity   ActivitySummary `json:"activity"`
	Details    []string        `json:"details,omitempty"`
}

func NormalizeMode(raw string) Mode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(ModeQueue):
		return ModeQueue
	case string(ModeSteer):
		return ModeSteer
	case string(ModeInterrupt):
		return ModeInterrupt
	default:
		return ModeInterrupt
	}
}

func Decide(req Request) Decision {
	mode := NormalizeMode(string(req.Mode))
	state := normalizeState(req.State)
	decision := Decision{
		Mode:   mode,
		State:  state,
		Accept: true,
	}
	decision.AckSuppressed = !ShouldAck(req.LastAck, req.Now, req.AckCooldown)

	switch state {
	case StateDraining, StateRestarting:
		if mode == ModeQueue || mode == ModeSteer {
			decision.Action = ActionDrainingQueued
			decision.QueuePending = true
			decision.Reason = stateReason(state, "queued")
			decision.Status = "queued for the next turn after recovery"
			return decision
		}
		decision.Action = ActionDrainingRejected
		decision.Accept = false
		decision.Rejected = true
		decision.Reason = stateReason(state, "rejected")
		decision.Status = "not accepting new input while recovering"
		return decision
	case StateIdle:
		decision.Action = ActionRejected
		decision.Accept = false
		decision.Rejected = true
		decision.Reason = "not_busy"
		decision.Status = "no active busy run"
		return decision
	}

	switch mode {
	case ModeQueue:
		decision.Action = ActionQueuePending
		decision.QueuePending = true
		decision.Reason = "queue_pending"
		decision.Status = "queued for the next turn"
	case ModeSteer:
		if req.CanSteer {
			decision.SteerAttempted = true
			if req.SteerSucceeded {
				decision.Action = ActionSteerSucceeded
				decision.SteerSucceeded = true
				decision.Reason = "steer_succeeded"
				decision.Status = "injected into the current run"
				return decision
			}
		}
		decision.Action = ActionSteerFallbackQueued
		decision.QueuePending = true
		decision.SteerFallbackQueued = true
		decision.Reason = "steer_fallback_queued"
		if decision.SteerAttempted {
			decision.Status = "steer rejected; queued for the next turn"
		} else {
			decision.Status = "steer unavailable; queued for the next turn"
		}
	default:
		decision.Action = ActionInterruptRequested
		decision.QueuePending = true
		decision.InterruptRequested = true
		decision.Reason = "interrupt_requested"
		decision.Status = "interrupt requested for the current run"
	}

	return decision
}

func ShouldAck(lastAck, now time.Time, cooldown time.Duration) bool {
	if lastAck.IsZero() {
		return true
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if cooldown <= 0 {
		cooldown = DefaultAckCooldown
	}
	return !now.Before(lastAck.Add(cooldown))
}

func BuildAck(decision Decision, activity ActivitySummary) Ack {
	activity = activity.Normalize()
	details := activity.Details()
	reason := decision.Reason
	status := decision.Status
	if decision.AckSuppressed {
		reason = "ack_suppressed"
		status = "ack suppressed by cooldown"
	}
	if len(details) > 0 {
		status = fmt.Sprintf("%s (%s)", status, strings.Join(details, ", "))
	}

	return Ack{
		Suppressed: decision.AckSuppressed,
		Mode:       decision.Mode,
		State:      decision.State,
		Action:     decision.Action,
		Reason:     reason,
		Status:     status,
		Activity:   activity,
		Details:    details,
	}
}

func (a ActivitySummary) Normalize() ActivitySummary {
	if a.RunningMinutes < 0 {
		a.RunningMinutes = 0
	}
	if a.RunningMinutes == 0 && a.RunningFor > 0 {
		a.RunningMinutes = int(a.RunningFor / time.Minute)
		if a.RunningMinutes == 0 {
			a.RunningMinutes = 1
		}
	}
	if a.TurnIndex < 0 {
		a.TurnIndex = 0
	}
	if a.MaxTurns < 0 {
		a.MaxTurns = 0
	}
	a.CurrentTool = strings.TrimSpace(a.CurrentTool)
	return a
}

func (a ActivitySummary) Details() []string {
	a = a.Normalize()
	details := make([]string, 0, 3)
	if a.RunningMinutes > 0 {
		unit := "minutes"
		if a.RunningMinutes == 1 {
			unit = "minute"
		}
		details = append(details, fmt.Sprintf("running %d %s", a.RunningMinutes, unit))
	}
	if a.TurnIndex > 0 && a.MaxTurns > 0 {
		details = append(details, fmt.Sprintf("turn %d/%d", a.TurnIndex, a.MaxTurns))
	} else if a.TurnIndex > 0 {
		details = append(details, fmt.Sprintf("turn %d", a.TurnIndex))
	}
	if a.CurrentTool != "" {
		details = append(details, fmt.Sprintf("tool %s", a.CurrentTool))
	}
	return details
}

func normalizeState(state State) State {
	switch State(strings.ToLower(strings.TrimSpace(string(state)))) {
	case StateDraining:
		return StateDraining
	case StateRestarting:
		return StateRestarting
	case StateIdle:
		return StateIdle
	case StateBusy:
		return StateBusy
	default:
		return StateBusy
	}
}

func stateReason(state State, suffix string) string {
	if state == StateRestarting {
		return "restarting_" + suffix
	}
	return "draining_" + suffix
}
