package fastmode

import (
	"testing"
	"time"
)

func TestUnavailableReasonPriority(t *testing.T) {
	settings := Settings{}
	env := Environment{
		FeatureDisabled:                true,
		StatsigReason:                  "statsig says no",
		NativeBinaryRequired:           true,
		NonInteractiveSession:          true,
		PreferThirdPartyAuthentication: true,
		Provider:                       ProviderBedrock,
		Org:                            DisabledOrg(OrgReasonPreference),
	}

	if got := UnavailableReason(env, settings); got != ReasonFeatureFlagDisabled {
		t.Fatalf("expected feature disabled to win, got %q", got)
	}

	env.FeatureDisabled = false
	if got := UnavailableReason(env, settings); got != "statsig says no" {
		t.Fatalf("expected statsig reason to win, got %q", got)
	}

	env.StatsigReason = ""
	if got := UnavailableReason(env, settings); got != ReasonNativeBinaryGate {
		t.Fatalf("expected native binary gate to win, got %q", got)
	}

	env.NativeBinaryRequired = false
	if got := UnavailableReason(env, settings); got != ReasonSDKGate {
		t.Fatalf("expected SDK gate to win, got %q", got)
	}
}

func TestUnavailableReasonFirstPartyGate(t *testing.T) {
	env := Environment{Provider: ProviderVertex}

	if got := UnavailableReason(env, Settings{FlagFastMode: true}); got != ReasonFirstPartyOnly {
		t.Fatalf("expected first-party reason, got %q", got)
	}
}

func TestUnavailableReasonSkipsNetworkErrors(t *testing.T) {
	env := Environment{
		Org:               DisabledOrg(OrgReasonNetworkError),
		SkipNetworkErrors: true,
	}

	if got := UnavailableReason(env, Settings{}); got != "" {
		t.Fatalf("expected network error skip to allow fast mode, got %q", got)
	}

	env.SkipNetworkErrors = false
	if got := UnavailableReason(env, Settings{}); got != "Fast mode unavailable due to network connectivity issues" {
		t.Fatalf("expected network error reason, got %q", got)
	}
}

func TestApplyTogglePlanSwitchesUnsupportedModel(t *testing.T) {
	plan := ApplyTogglePlan(true, "sonnet", Settings{})

	if !plan.FastMode {
		t.Fatal("expected fast mode enabled")
	}
	if !plan.ModelChanged || !plan.ClearSessionModel {
		t.Fatalf("expected model switch plan, got %+v", plan)
	}
	if plan.Model != FastModel() {
		t.Fatalf("expected fast model %q, got %q", FastModel(), plan.Model)
	}
	if plan.Settings.FastMode == nil || !*plan.Settings.FastMode {
		t.Fatalf("expected persisted fast mode true, got %+v", plan.Settings.FastMode)
	}

	off := ApplyTogglePlan(false, "sonnet", plan.Settings)
	if off.Model != "sonnet" || off.ModelChanged {
		t.Fatalf("expected disabling to keep model, got %+v", off)
	}
	if off.Settings.FastMode != nil {
		t.Fatalf("expected disabling to clear persisted setting, got %+v", off.Settings.FastMode)
	}
}

func TestInitialSettingHonorsPerSessionOptIn(t *testing.T) {
	settings := Settings{
		FastMode:                Bool(true),
		FastModePerSessionOptIn: true,
	}

	if InitialSetting("claude-opus-4-6-20260101", settings, Environment{}) {
		t.Fatal("expected per-session opt-in to start fast mode off")
	}

	settings.FastModePerSessionOptIn = false
	if !InitialSetting("claude-opus-4-6-20260101", settings, Environment{}) {
		t.Fatal("expected persisted fast mode to start on for supported model")
	}
}

func TestRuntimeCooldownExpires(t *testing.T) {
	runtime := NewRuntime()
	now := time.Unix(100, 0)
	resetAt := now.Add(2 * time.Minute)

	events := runtime.TriggerCooldown(now, resetAt, CooldownRateLimit)
	if len(events) != 1 || events[0].Type != EventCooldownTriggered {
		t.Fatalf("expected cooldown triggered event, got %+v", events)
	}

	state, events := runtime.GetRuntimeState(now.Add(time.Minute))
	if state.Status != RuntimeCooldown || len(events) != 0 {
		t.Fatalf("expected cooldown before reset, got state=%+v events=%+v", state, events)
	}

	state, events = runtime.GetRuntimeState(resetAt)
	if state.Status != RuntimeActive {
		t.Fatalf("expected active after reset, got %+v", state)
	}
	if len(events) != 1 || events[0].Type != EventCooldownExpired {
		t.Fatalf("expected cooldown expired event, got %+v", events)
	}

	state, events = runtime.GetRuntimeState(resetAt.Add(time.Second))
	if state.Status != RuntimeActive || len(events) != 0 {
		t.Fatalf("expected no duplicate expiry event, got state=%+v events=%+v", state, events)
	}
}

func TestApplyAPIRejectionDisablesPermanently(t *testing.T) {
	settings := Settings{FastMode: Bool(true)}

	plan := ApplyAPIRejection(settings, OrgStatus{State: OrgEnabled})

	if !plan.PermanentlyDisabled {
		t.Fatal("expected API rejection to permanently disable fast mode")
	}
	if plan.Settings.FastMode != nil {
		t.Fatalf("expected user fast mode setting cleared, got %+v", plan.Settings.FastMode)
	}
	if plan.Org.State != OrgDisabled || plan.Org.Reason != OrgReasonPreference {
		t.Fatalf("expected org preference disable, got %+v", plan.Org)
	}
	if len(plan.Events) != 1 || plan.Events[0].Type != EventOrgChanged || plan.Events[0].OrgEnabled {
		t.Fatalf("expected org disabled event, got %+v", plan.Events)
	}
}

func TestApplyExtraUsageRejection(t *testing.T) {
	settings := Settings{FastMode: Bool(true)}

	plan := ApplyExtraUsageRejection("member_level_disabled", settings)
	if !plan.PermanentlyDisabled {
		t.Fatal("expected member-level rejection to permanently disable")
	}
	if plan.Settings.FastMode != nil {
		t.Fatalf("expected fast mode setting cleared, got %+v", plan.Settings.FastMode)
	}
	if plan.Message != "Fast mode disabled · extra usage disabled for your account" {
		t.Fatalf("unexpected message %q", plan.Message)
	}

	credits := ApplyExtraUsageRejection("out_of_credits", settings)
	if credits.PermanentlyDisabled {
		t.Fatal("expected out_of_credits to notify without permanent disable")
	}
	if credits.Settings.FastMode == nil || !*credits.Settings.FastMode {
		t.Fatalf("expected out_of_credits to preserve setting, got %+v", credits.Settings.FastMode)
	}
}

func TestRateLimitBreaker(t *testing.T) {
	headers := map[string]string{
		"X-RateLimit-Remaining-Requests-1H": "0",
		"X-RateLimit-Reset-Requests-1H":     "120",
	}
	if !ShouldTripBreaker(headers, nil, time.Unix(100, 0)) {
		t.Fatal("expected exhausted hourly bucket with long reset to trip breaker")
	}

	shortReset := map[string]string{
		"x-ratelimit-remaining-requests": "0",
		"x-ratelimit-reset-requests":     "30",
	}
	if ShouldTripBreaker(shortReset, nil, time.Unix(100, 0)) {
		t.Fatal("expected short reset to be treated as transient")
	}

	now := time.Unix(100, 0)
	lastKnown, ok := ParseRateLimitHeaders(map[string]string{
		"x-ratelimit-limit-tokens-1h":     "1000",
		"x-ratelimit-remaining-tokens-1h": "0",
		"x-ratelimit-reset-tokens-1h":     "90",
	}, "nous", now)
	if !ok {
		t.Fatal("expected rate limit headers to parse")
	}
	if !ShouldTripBreaker(nil, &lastKnown, now.Add(10*time.Second)) {
		t.Fatal("expected last-known exhausted bucket to trip breaker")
	}
	if ShouldTripBreaker(nil, &lastKnown, now.Add(40*time.Second)) {
		t.Fatal("expected adjusted reset under threshold to stay transient")
	}
}
