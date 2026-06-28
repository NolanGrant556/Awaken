package providerfallback

import "fmt"

type RunState struct {
	chain               Chain
	source              ActiveSource
	fallbackIndex       int
	manualModel         ModelRef
	activationReason    ActivationReason
	automaticFallback   bool
	pendingPrimaryRetry bool
	events              []Event
	lastRunFailed       bool
}

func (s RunState) withAppendedEvent(event Event) RunState {
	s.events = append(append([]Event(nil), s.events...), event)
	return s
}

func NewRunState(chain Chain) RunState {
	return RunState{
		chain:         chain.Normalize(),
		source:        ActivePrimary,
		fallbackIndex: -1,
	}
}

func (s RunState) Chain() Chain {
	return s.chain.Normalize()
}

func (s RunState) ActiveModel() ModelRef {
	switch s.source {
	case ActiveFallback:
		if fallback, ok := s.chain.Fallback(s.fallbackIndex); ok {
			return fallback
		}
	case ActiveManual:
		if !s.manualModel.IsZero() {
			return s.manualModel.Normalize()
		}
	}
	return s.chain.Primary.Normalize()
}

func (s RunState) ActiveSource() ActiveSource {
	return s.source
}

func (s RunState) FallbackReason() ActivationReason {
	return s.activationReason
}

func (s RunState) FallbackIndex() int {
	return s.fallbackIndex
}

func (s RunState) IsAutomaticFallbackActive() bool {
	return s.source == ActiveFallback && s.automaticFallback
}

func (s RunState) PendingPrimaryRetry() bool {
	return s.pendingPrimaryRetry
}

func (s RunState) LastRunFailed() bool {
	return s.lastRunFailed
}

func (s RunState) Events() []Event {
	return append([]Event(nil), s.events...)
}

func (s RunState) ActivateNextFallback(reason ActivationReason) (RunState, Decision) {
	reason = normalizeReason(reason)
	from := s.ActiveModel()
	startAfter := -1
	if s.source == ActiveFallback {
		startAfter = s.fallbackIndex
	}

	nextIndex, next, ok := s.chain.NextFallback(startAfter)
	if !ok {
		return s, Decision{
			Kind:   DecisionStayOnActive,
			Active: from,
			Reason: fmt.Sprintf("fallback chain exhausted; staying on %s", from.String()),
		}
	}

	s.source = ActiveFallback
	s.fallbackIndex = nextIndex
	s.manualModel = ModelRef{}
	s.activationReason = reason
	s.automaticFallback = true
	s.pendingPrimaryRetry = false
	s.lastRunFailed = false
	s = s.withAppendedEvent(Event{
		Kind:          EventFallbackActivated,
		From:          from,
		To:            next,
		Reason:        reason,
		FallbackIndex: nextIndex,
		Automatic:     true,
	})

	return s, Decision{
		Kind:   DecisionStayOnActive,
		Active: next,
		Reason: fmt.Sprintf("automatic fallback activated because %s; active model is %s", reason, next.String()),
	}
}

func (s RunState) SwitchManually(target ModelRef) (RunState, Decision) {
	target = target.Normalize()
	from := s.ActiveModel()
	if target.IsZero() {
		return s, Decision{
			Kind:   DecisionStayOnActive,
			Active: from,
			Reason: fmt.Sprintf("manual switch target is empty; staying on %s", from.String()),
		}
	}

	s.source = ActiveManual
	s.fallbackIndex = -1
	s.manualModel = target
	s.activationReason = ReasonManual
	s.automaticFallback = false
	s.pendingPrimaryRetry = false
	s.lastRunFailed = false
	s = s.withAppendedEvent(Event{
		Kind:          EventManualSwitch,
		From:          from,
		To:            target,
		Reason:        ReasonManual,
		FallbackIndex: -1,
		Automatic:     false,
	})

	return s, Decision{
		Kind:   DecisionStayOnActive,
		Active: target,
		Reason: fmt.Sprintf("intentional model switch to %s", target.String()),
	}
}

func (s RunState) FinishRun(outcome RunOutcome) (RunState, Decision) {
	active := s.ActiveModel()
	switch outcome {
	case RunSucceeded:
		s.lastRunFailed = false
		s = s.withAppendedEvent(Event{
			Kind:          EventRunSucceeded,
			To:            active,
			Reason:        s.activationReason,
			FallbackIndex: s.fallbackIndex,
			Automatic:     s.IsAutomaticFallbackActive(),
		})
		if s.IsAutomaticFallbackActive() {
			s.pendingPrimaryRetry = true
			return s, Decision{
				Kind:                 DecisionEvictAfterSuccessfulFallback,
				Active:               active,
				Reason:               fmt.Sprintf("successful automatic fallback on %s; evict cached runtime so next turn retries %s", active.String(), s.chain.Primary.String()),
				ShouldEvict:          true,
				RetryPrimaryNextTurn: true,
			}
		}
		return s, Decision{
			Kind:   DecisionStayOnActive,
			Active: active,
			Reason: fmt.Sprintf("run succeeded on %s", active.String()),
		}
	case RunFailed:
		s.lastRunFailed = true
		s.pendingPrimaryRetry = false
		s = s.withAppendedEvent(Event{
			Kind:          EventRunFailed,
			To:            active,
			Reason:        s.activationReason,
			FallbackIndex: s.fallbackIndex,
			Automatic:     s.IsAutomaticFallbackActive(),
		})
		if s.IsAutomaticFallbackActive() {
			return s, Decision{
				Kind:   DecisionDoNotEvictOnFailedRun,
				Active: active,
				Reason: fmt.Sprintf("run failed while automatic fallback %s was active; do not evict or recreate the agent loop", active.String()),
			}
		}
		return s, Decision{
			Kind:   DecisionStayOnActive,
			Active: active,
			Reason: fmt.Sprintf("run failed on %s without automatic fallback", active.String()),
		}
	default:
		return s, Decision{
			Kind:   DecisionStayOnActive,
			Active: active,
			Reason: fmt.Sprintf("unknown run outcome %q; staying on active model", outcome),
		}
	}
}

func (s RunState) StartNextTurn() (RunState, Decision) {
	if !s.pendingPrimaryRetry {
		active := s.ActiveModel()
		return s, Decision{
			Kind:   DecisionStayOnActive,
			Active: active,
			Reason: fmt.Sprintf("no primary retry pending; staying on %s", active.String()),
		}
	}

	from := s.ActiveModel()
	primary := s.chain.Primary.Normalize()
	s.source = ActivePrimary
	s.fallbackIndex = -1
	s.manualModel = ModelRef{}
	s.activationReason = ""
	s.automaticFallback = false
	s.pendingPrimaryRetry = false
	s.lastRunFailed = false
	s = s.withAppendedEvent(Event{
		Kind:          EventPrimaryRetried,
		From:          from,
		To:            primary,
		FallbackIndex: -1,
		Automatic:     false,
	})

	return s, Decision{
		Kind:                 DecisionRetryPrimaryNextTurn,
		Active:               primary,
		Reason:               fmt.Sprintf("retrying configured primary model %s on the next turn", primary.String()),
		RetryPrimaryNextTurn: true,
	}
}

func (s RunState) DisplayModel() DisplayModel {
	active := s.ActiveModel()
	display := DisplayModel{
		ConfiguredDefault: s.chain.Primary.Normalize(),
		Active:            active,
		Source:            s.source,
		Reason:            s.activationReason,
		IsFallback:        s.source == ActiveFallback,
		IsManualSwitch:    s.source == ActiveManual,
	}

	switch s.source {
	case ActiveFallback:
		display.Explanation = fmt.Sprintf(
			"configured default is %s; currently active fallback is %s because %s",
			display.ConfiguredDefault.String(),
			active.String(),
			normalizeReason(s.activationReason),
		)
	case ActiveManual:
		display.Explanation = fmt.Sprintf(
			"configured default is %s; current active model is %s due to an intentional manual switch",
			display.ConfiguredDefault.String(),
			active.String(),
		)
	default:
		display.Explanation = fmt.Sprintf(
			"configured default is %s and it is currently active",
			display.ConfiguredDefault.String(),
		)
	}
	return display
}
