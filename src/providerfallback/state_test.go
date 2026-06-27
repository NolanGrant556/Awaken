package providerfallback

import (
	"strings"
	"testing"
)

func TestNoFallbackStaysOnPrimary(t *testing.T) {
	primary := ModelRef{Provider: "anthropic", Model: "claude-sonnet"}
	state := NewRunState(NewChain(primary))

	if got := state.ActiveModel(); got != primary {
		t.Fatalf("expected primary active, got %+v", got)
	}

	next, decision := state.ActivateNextFallback(ReasonRateLimit)
	if decision.Kind != DecisionStayOnActive {
		t.Fatalf("expected stay_on_active, got %s", decision.Kind)
	}
	if got := next.ActiveModel(); got != primary {
		t.Fatalf("expected primary after exhausted fallback, got %+v", got)
	}
	if next.IsAutomaticFallbackActive() {
		t.Fatalf("did not expect automatic fallback without configured fallbacks")
	}
}

func TestActivateFallbacksInOrder(t *testing.T) {
	primary := ModelRef{Provider: "anthropic", Model: "claude-sonnet"}
	first := ModelRef{Provider: "openrouter", Model: "anthropic/claude-sonnet"}
	second := ModelRef{Provider: "nous", Model: "nous-hermes"}
	state := NewRunState(NewChain(primary, first, second))

	state, decision := state.ActivateNextFallback(ReasonAuthError)
	if decision.Kind != DecisionStayOnActive {
		t.Fatalf("expected stay_on_active, got %s", decision.Kind)
	}
	if got := state.ActiveModel(); got != first {
		t.Fatalf("expected first fallback, got %+v", got)
	}
	if got := state.FallbackIndex(); got != 0 {
		t.Fatalf("expected fallback index 0, got %d", got)
	}
	if got := state.FallbackReason(); got != ReasonAuthError {
		t.Fatalf("expected auth_error reason, got %s", got)
	}

	state, decision = state.ActivateNextFallback(ReasonRateLimit)
	if decision.Kind != DecisionStayOnActive {
		t.Fatalf("expected stay_on_active for second fallback, got %s", decision.Kind)
	}
	if got := state.ActiveModel(); got != second {
		t.Fatalf("expected second fallback, got %+v", got)
	}
	if got := state.FallbackIndex(); got != 1 {
		t.Fatalf("expected fallback index 1, got %d", got)
	}
}

func TestManualSwitchIsNotAutomaticFallback(t *testing.T) {
	primary := ModelRef{Provider: "anthropic", Model: "claude-sonnet"}
	fallback := ModelRef{Provider: "openrouter", Model: "anthropic/claude-sonnet"}
	manual := ModelRef{Provider: "openai", Model: "gpt-5"}
	state := NewRunState(NewChain(primary, fallback))

	state, decision := state.SwitchManually(manual)
	if decision.Kind != DecisionStayOnActive {
		t.Fatalf("expected stay_on_active, got %s", decision.Kind)
	}
	if state.IsAutomaticFallbackActive() {
		t.Fatalf("manual switch must not be automatic fallback")
	}
	if got := state.ActiveSource(); got != ActiveManual {
		t.Fatalf("expected manual source, got %s", got)
	}
	if got := state.ActiveModel(); got != manual {
		t.Fatalf("expected manual model active, got %+v", got)
	}

	state, decision = state.FinishRun(RunSucceeded)
	if decision.Kind != DecisionStayOnActive {
		t.Fatalf("expected manual success to stay active, got %s", decision.Kind)
	}
	if decision.ShouldEvict || decision.RetryPrimaryNextTurn {
		t.Fatalf("manual switch should not request fallback eviction or primary retry: %+v", decision)
	}
	if !strings.Contains(state.DisplayModel().Explanation, "intentional manual switch") {
		t.Fatalf("expected manual display explanation, got %q", state.DisplayModel().Explanation)
	}
}

func TestSuccessfulFallbackRetriesPrimaryNextTurn(t *testing.T) {
	primary := ModelRef{Provider: "anthropic", Model: "claude-sonnet"}
	fallback := ModelRef{Provider: "openrouter", Model: "anthropic/claude-sonnet"}
	state := NewRunState(NewChain(primary, fallback))

	state, _ = state.ActivateNextFallback(ReasonRateLimit)
	state, decision := state.FinishRun(RunSucceeded)
	if decision.Kind != DecisionEvictAfterSuccessfulFallback {
		t.Fatalf("expected evict_after_successful_fallback, got %s", decision.Kind)
	}
	if !decision.ShouldEvict || !decision.RetryPrimaryNextTurn {
		t.Fatalf("expected eviction and next-turn primary retry, got %+v", decision)
	}
	if !state.PendingPrimaryRetry() {
		t.Fatalf("expected pending primary retry after successful fallback")
	}

	state, decision = state.StartNextTurn()
	if decision.Kind != DecisionRetryPrimaryNextTurn {
		t.Fatalf("expected retry_primary_next_turn, got %s", decision.Kind)
	}
	if got := state.ActiveModel(); got != primary {
		t.Fatalf("expected primary active on next turn, got %+v", got)
	}
	if state.IsAutomaticFallbackActive() {
		t.Fatalf("fallback should be cleared after next-turn primary retry")
	}
}

func TestFailedBadModelFallbackDoesNotEvictOrRecreateLoop(t *testing.T) {
	primary := ModelRef{Provider: "anthropic", Model: "bad-model-id"}
	fallback := ModelRef{Provider: "openrouter", Model: "anthropic/claude-sonnet"}
	state := NewRunState(NewChain(primary, fallback))

	state, _ = state.ActivateNextFallback(ReasonBadModel)
	state, decision := state.FinishRun(RunFailed)
	if decision.Kind != DecisionDoNotEvictOnFailedRun {
		t.Fatalf("expected do_not_evict_on_failed_run, got %s", decision.Kind)
	}
	if decision.ShouldEvict || decision.RetryPrimaryNextTurn {
		t.Fatalf("failed run must not evict or retry primary: %+v", decision)
	}
	if !state.LastRunFailed() {
		t.Fatalf("expected failed run state")
	}

	state, nextDecision := state.StartNextTurn()
	if nextDecision.Kind != DecisionStayOnActive {
		t.Fatalf("expected no automatic primary retry after failed fallback, got %s", nextDecision.Kind)
	}
	if got := state.ActiveModel(); got != fallback {
		t.Fatalf("expected failed fallback state to remain active, got %+v", got)
	}
}

func TestDisplayExplainsConfiguredDefaultAndActiveFallback(t *testing.T) {
	primary := ModelRef{Provider: "anthropic", Model: "claude-sonnet"}
	fallback := ModelRef{Provider: "openrouter", Model: "anthropic/claude-sonnet"}
	state := NewRunState(NewChain(primary, fallback))
	state, _ = state.ActivateNextFallback(ReasonRateLimit)

	display := state.DisplayModel()
	if display.ConfiguredDefault != primary {
		t.Fatalf("expected configured default %+v, got %+v", primary, display.ConfiguredDefault)
	}
	if display.Active != fallback {
		t.Fatalf("expected active fallback %+v, got %+v", fallback, display.Active)
	}
	if !display.IsFallback || display.IsManualSwitch {
		t.Fatalf("expected automatic fallback display flags, got %+v", display)
	}
	if display.Reason != ReasonRateLimit {
		t.Fatalf("expected rate_limit display reason, got %s", display.Reason)
	}
	for _, want := range []string{"configured default", "currently active fallback", "rate_limit"} {
		if !strings.Contains(display.Explanation, want) {
			t.Fatalf("expected display explanation to contain %q, got %q", want, display.Explanation)
		}
	}
}
