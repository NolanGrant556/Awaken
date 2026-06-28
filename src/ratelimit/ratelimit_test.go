package ratelimit

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestParseHeadersCaseInsensitiveAndFloatReset(t *testing.T) {
	capturedAt := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	headers := http.Header{
		"X-RateLimit-Limit-Requests":        {"100"},
		"x-ratelimit-remaining-requests":    {"25"},
		"X-RATELIMIT-RESET-REQUESTS":        {"1.5"},
		"x-ratelimit-limit-requests-1h":     {"1000"},
		"X-RateLimit-Remaining-Requests-1H": {"900"},
		"x-ratelimit-reset-requests-1h":     {"3600"},
		"X-RateLimit-Limit-Tokens":          {"20000"},
		"x-ratelimit-remaining-tokens":      {"12345"},
		"X-RateLimit-Reset-Tokens":          {"2.25"},
		"x-ratelimit-limit-tokens-1h":       {"1000000"},
		"x-ratelimit-remaining-tokens-1h":   {"750000"},
		"x-ratelimit-reset-tokens-1h":       {"3599.5"},
	}

	state, ok := ParseHeadersAt(headers, "openai", capturedAt)
	if !ok {
		t.Fatalf("expected headers to parse")
	}
	if !state.HasData() || state.Provider != "openai" || state.CapturedAt != capturedAt {
		t.Fatalf("unexpected state metadata: %+v", state)
	}
	if state.RequestsMinute.Limit != 100 || state.RequestsMinute.Remaining != 25 {
		t.Fatalf("unexpected request minute bucket: %+v", state.RequestsMinute)
	}
	if state.RequestsMinute.Reset != 1500*time.Millisecond {
		t.Fatalf("expected float reset to parse as 1.5s, got %s", state.RequestsMinute.Reset)
	}
	if state.RequestsHour.Limit != 1000 || state.RequestsHour.Remaining != 900 || state.RequestsHour.Reset != time.Hour {
		t.Fatalf("unexpected request hour bucket: %+v", state.RequestsHour)
	}
	if state.TokensMinute.Limit != 20000 || state.TokensMinute.Remaining != 12345 || state.TokensMinute.Reset != 2250*time.Millisecond {
		t.Fatalf("unexpected token minute bucket: %+v", state.TokensMinute)
	}
	if state.TokensHour.Limit != 1000000 || state.TokensHour.Remaining != 750000 || state.TokensHour.Reset != 3599500*time.Millisecond {
		t.Fatalf("unexpected token hour bucket: %+v", state.TokensHour)
	}
}

func TestParseHeadersMissingReturnsFalseAndNoData(t *testing.T) {
	state, ok := ParseHeadersAt(http.Header{"Content-Type": {"application/json"}}, "openai", time.Now())
	if ok {
		t.Fatalf("expected missing rate limit headers to return false")
	}
	if state.HasData() {
		t.Fatalf("expected zero state to have no data: %+v", state)
	}
}

func TestBucketRemainingDurationAndUsage(t *testing.T) {
	capturedAt := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	bucket := Bucket{
		Limit:      100,
		Remaining:  20,
		Reset:      10 * time.Second,
		CapturedAt: capturedAt,
	}

	if bucket.Used() != 80 {
		t.Fatalf("expected 80 used, got %d", bucket.Used())
	}
	if bucket.UsagePct() != 80 {
		t.Fatalf("expected 80%% usage, got %.1f", bucket.UsagePct())
	}
	if got := bucket.RemainingDuration(capturedAt.Add(4 * time.Second)); got != 6*time.Second {
		t.Fatalf("expected 6s remaining, got %s", got)
	}
	if got := bucket.RemainingDuration(capturedAt.Add(11 * time.Second)); got != 0 {
		t.Fatalf("expected elapsed reset to clamp to 0, got %s", got)
	}
}

func TestHotWarningsAndThrottle(t *testing.T) {
	capturedAt := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	state := State{
		RequestsMinute: Bucket{Limit: 10, Remaining: 2, Reset: 30 * time.Second, CapturedAt: capturedAt},
		TokensMinute:   Bucket{Limit: 1000, Remaining: 0, Reset: 45 * time.Second, CapturedAt: capturedAt},
		Provider:       "anthropic",
		CapturedAt:     capturedAt,
	}

	warnings := state.HotWarnings(capturedAt.Add(5 * time.Second))
	if len(warnings) != 2 {
		t.Fatalf("expected request and token warnings, got %+v", warnings)
	}
	if warnings[0].UsagePct != 80 {
		t.Fatalf("expected request warning at 80%%, got %+v", warnings[0])
	}
	if !state.ShouldThrottle(capturedAt.Add(5 * time.Second)) {
		t.Fatalf("expected exhausted token bucket to throttle before reset")
	}
	decision := state.ThrottleDecision(capturedAt.Add(5 * time.Second))
	if !decision.ShouldThrottle || decision.Resource != ResourceTokens || decision.Window != WindowMinute || decision.ResetIn != 40*time.Second {
		t.Fatalf("unexpected throttle decision: %+v", decision)
	}
	if state.ShouldThrottle(capturedAt.Add(46 * time.Second)) {
		t.Fatalf("expected elapsed reset to stop throttling")
	}
}

func TestTrackerIsolationPerProviderModelKey(t *testing.T) {
	capturedAt := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	tracker := NewTracker()

	tracker.UpdateState("openai", "gpt-4.1", "key-a", State{
		RequestsMinute: Bucket{Limit: 10, Remaining: 0, Reset: time.Minute, CapturedAt: capturedAt},
		CapturedAt:     capturedAt,
	})
	tracker.UpdateState("openai", "gpt-4.1", "key-b", State{
		RequestsMinute: Bucket{Limit: 10, Remaining: 8, Reset: time.Minute, CapturedAt: capturedAt},
		CapturedAt:     capturedAt,
	})
	tracker.UpdateState("anthropic", "claude-sonnet", "key-a", State{
		RequestsMinute: Bucket{Limit: 20, Remaining: 19, Reset: time.Minute, CapturedAt: capturedAt},
		CapturedAt:     capturedAt,
	})

	if !tracker.ShouldThrottle("openai", "gpt-4.1", "key-a", capturedAt.Add(10*time.Second)) {
		t.Fatalf("expected key-a to throttle")
	}
	if tracker.ShouldThrottle("openai", "gpt-4.1", "key-b", capturedAt.Add(10*time.Second)) {
		t.Fatalf("expected key-b not to throttle")
	}
	if tracker.ShouldThrottle("anthropic", "claude-sonnet", "key-a", capturedAt.Add(10*time.Second)) {
		t.Fatalf("expected anthropic key-a not to throttle")
	}

	state, ok := tracker.Snapshot("openai", "gpt-4.1", "key-b")
	if !ok {
		t.Fatalf("expected snapshot for key-b")
	}
	if state.Provider != "openai" || state.RequestsMinute.Remaining != 8 {
		t.Fatalf("unexpected isolated snapshot: %+v", state)
	}
	if _, ok := tracker.Snapshot("openai", "other-model", "key-b"); ok {
		t.Fatalf("expected different model to be isolated")
	}
}

func TestTrackerUpdateIgnoresMissingHeaders(t *testing.T) {
	tracker := NewTracker()
	state, ok := tracker.Update("openai", "gpt-4.1", "key-a", http.Header{"Content-Type": {"application/json"}})
	if ok {
		t.Fatalf("expected missing headers not to update")
	}
	if state.HasData() {
		t.Fatalf("expected no parsed state: %+v", state)
	}
	if _, ok := tracker.Snapshot("openai", "gpt-4.1", "key-a"); ok {
		t.Fatalf("expected no stored snapshot")
	}
}

func TestFormattingIsStableAndCompact(t *testing.T) {
	capturedAt := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	state := State{
		RequestsMinute: Bucket{Limit: 1000, Remaining: 200, Reset: 125 * time.Second, CapturedAt: capturedAt},
		TokensHour:     Bucket{Limit: 2000000, Remaining: 1500000, Reset: 3600 * time.Second, CapturedAt: capturedAt},
		Provider:       "openai",
		CapturedAt:     capturedAt,
	}

	if FormatCount(999) != "999" || FormatCount(1200) != "1.2K" || FormatCount(2000000) != "2.0M" {
		t.Fatalf("unexpected count formatting")
	}
	if FormatDuration(125*time.Second) != "2m 5s" || FormatDuration(3600*time.Second) != "1h" {
		t.Fatalf("unexpected duration formatting")
	}

	formatted := FormatCompact(state, capturedAt)
	for _, want := range []string{"openai", "req/min used 800/1.0K (80.0%)", "remaining 200", "tok/hour used 500.0K/2.0M"} {
		if !strings.Contains(formatted, want) {
			t.Fatalf("expected %q in formatted output %q", want, formatted)
		}
	}
	if strings.Contains(formatted, "\x1b[") {
		t.Fatalf("formatted output must not contain terminal color escapes: %q", formatted)
	}
}
