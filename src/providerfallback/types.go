package providerfallback

import "strings"

type Provider string

type ModelRef struct {
	Provider Provider
	Model    string
}

func (m ModelRef) Normalize() ModelRef {
	return ModelRef{
		Provider: Provider(strings.TrimSpace(string(m.Provider))),
		Model:    strings.TrimSpace(m.Model),
	}
}

func (m ModelRef) IsZero() bool {
	m = m.Normalize()
	return m.Provider == "" || m.Model == ""
}

func (m ModelRef) String() string {
	m = m.Normalize()
	switch {
	case m.Provider != "" && m.Model != "":
		return string(m.Provider) + "/" + m.Model
	case m.Model != "":
		return m.Model
	case m.Provider != "":
		return string(m.Provider)
	default:
		return "unknown"
	}
}

type Chain struct {
	Primary   ModelRef
	Fallbacks []ModelRef
}

func NewChain(primary ModelRef, fallbacks ...ModelRef) Chain {
	chain := Chain{
		Primary:   primary.Normalize(),
		Fallbacks: make([]ModelRef, 0, len(fallbacks)),
	}
	for _, fallback := range fallbacks {
		chain.Fallbacks = append(chain.Fallbacks, fallback.Normalize())
	}
	return chain
}

func (c Chain) Normalize() Chain {
	return NewChain(c.Primary, c.Fallbacks...)
}

func (c Chain) Fallback(index int) (ModelRef, bool) {
	if index < 0 || index >= len(c.Fallbacks) {
		return ModelRef{}, false
	}
	fallback := c.Fallbacks[index].Normalize()
	if fallback.IsZero() {
		return ModelRef{}, false
	}
	return fallback, true
}

func (c Chain) NextFallback(after int) (int, ModelRef, bool) {
	for index := after + 1; index < len(c.Fallbacks); index++ {
		if fallback, ok := c.Fallback(index); ok {
			return index, fallback, true
		}
	}
	return -1, ModelRef{}, false
}

type ActivationReason string

const (
	ReasonUnknown   ActivationReason = "unknown"
	ReasonAuthError ActivationReason = "auth_error"
	ReasonRateLimit ActivationReason = "rate_limit"
	ReasonBadModel  ActivationReason = "bad_model"
	ReasonNetwork   ActivationReason = "network"
	ReasonManual    ActivationReason = "manual"
)

func normalizeReason(reason ActivationReason) ActivationReason {
	if strings.TrimSpace(string(reason)) == "" {
		return ReasonUnknown
	}
	return ActivationReason(strings.TrimSpace(string(reason)))
}

type ActiveSource string

const (
	ActivePrimary  ActiveSource = "primary"
	ActiveFallback ActiveSource = "fallback"
	ActiveManual   ActiveSource = "manual"
)

type EventKind string

const (
	EventFallbackActivated EventKind = "fallback_activated"
	EventManualSwitch      EventKind = "manual_switch"
	EventRunSucceeded      EventKind = "run_succeeded"
	EventRunFailed         EventKind = "run_failed"
	EventPrimaryRetried    EventKind = "primary_retried"
)

type Event struct {
	Kind          EventKind
	From          ModelRef
	To            ModelRef
	Reason        ActivationReason
	FallbackIndex int
	Automatic     bool
}

type RunOutcome string

const (
	RunSucceeded RunOutcome = "succeeded"
	RunFailed    RunOutcome = "failed"
)

type DecisionKind string

const (
	DecisionStayOnActive                 DecisionKind = "stay_on_active"
	DecisionRetryPrimaryNextTurn         DecisionKind = "retry_primary_next_turn"
	DecisionDoNotEvictOnFailedRun        DecisionKind = "do_not_evict_on_failed_run"
	DecisionEvictAfterSuccessfulFallback DecisionKind = "evict_after_successful_fallback"
)

type Decision struct {
	Kind                 DecisionKind
	Active               ModelRef
	Reason               string
	ShouldEvict          bool
	RetryPrimaryNextTurn bool
}

type DisplayModel struct {
	ConfiguredDefault ModelRef
	Active            ModelRef
	Source            ActiveSource
	Reason            ActivationReason
	IsFallback        bool
	IsManualSwitch    bool
	Explanation       string
}
