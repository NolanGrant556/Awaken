package ratelimitoptions

import (
	"reflect"
	"strings"
	"testing"
)

func TestCancelOptionAlwaysPresent(t *testing.T) {
	options := BuildOptions(MenuInput{})
	if len(options) != 1 {
		t.Fatalf("expected only cancel option, got %+v", options)
	}
	if options[0].Value != OptionCancel {
		t.Fatalf("expected cancel option, got %+v", options[0])
	}
}

func TestBuyFirstControlsCancelOrder(t *testing.T) {
	input := MenuInput{
		SubscriptionType:         SubscriptionPro,
		ExtraUsageFeatureEnabled: true,
		UpgradeEnabled:           true,
	}

	got := optionValues(BuildOptions(input))
	want := []OptionValue{OptionCancel, OptionExtraUsage, OptionUpgrade}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("default order mismatch: got %v, want %v", got, want)
	}

	input.BuyFirst = true
	got = optionValues(BuildOptions(input))
	want = []OptionValue{OptionExtraUsage, OptionUpgrade, OptionCancel}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buy-first order mismatch: got %v, want %v", got, want)
	}
}

func TestMax20xHidesUpgrade(t *testing.T) {
	options := BuildOptions(MenuInput{
		SubscriptionType: SubscriptionMax,
		RateLimitTier:    Max20xTier,
		UpgradeEnabled:   true,
	})
	if hasOption(options, OptionUpgrade) {
		t.Fatalf("expected Max 20x tier to hide upgrade, got %+v", options)
	}
}

func TestTeamAdminRequestLabels(t *testing.T) {
	base := MenuInput{
		SubscriptionType:         SubscriptionTeam,
		ExtraUsageFeatureEnabled: true,
		HasBillingAccess:         false,
	}

	base.OverageStatus = OverageRejected
	option := findOption(t, BuildOptions(base), OptionExtraUsage)
	if option.Label != "Request more" {
		t.Fatalf("expected rejected overage label, got %q", option.Label)
	}

	base.OverageStatus = OverageAllowed
	option = findOption(t, BuildOptions(base), OptionExtraUsage)
	if option.Label != "Request extra usage" {
		t.Fatalf("expected non-overage request label, got %q", option.Label)
	}
}

func TestOrgSpendCapDepletedHidesExtraUsageForNonAdmin(t *testing.T) {
	options := BuildOptions(MenuInput{
		SubscriptionType:         SubscriptionEnterprise,
		ExtraUsageFeatureEnabled: true,
		HasBillingAccess:         false,
		OverageDisabledReason:    ReasonOutOfCredits,
	})
	if hasOption(options, OptionExtraUsage) {
		t.Fatalf("expected org spend cap depletion to hide extra usage, got %+v", options)
	}
}

func TestExtraUsageLabels(t *testing.T) {
	input := MenuInput{
		SubscriptionType:         SubscriptionPro,
		ExtraUsageFeatureEnabled: true,
	}

	option := findOption(t, BuildOptions(input), OptionExtraUsage)
	if option.Label != "Switch to extra usage" {
		t.Fatalf("expected switch label, got %q", option.Label)
	}

	input.AccountHasExtraUsageEnabled = true
	option = findOption(t, BuildOptions(input), OptionExtraUsage)
	if option.Label != "Add funds to continue with extra usage" {
		t.Fatalf("expected add-funds label, got %q", option.Label)
	}
}

func TestSelectionEventsAndActions(t *testing.T) {
	tests := []struct {
		name       string
		value      OptionValue
		kind       ActionKind
		subcommand string
		event      string
		display    string
	}{
		{
			name:       "upgrade",
			value:      OptionUpgrade,
			kind:       ActionSubcommand,
			subcommand: "upgrade",
			event:      EventSelectUpgrade,
		},
		{
			name:       "extra usage",
			value:      OptionExtraUsage,
			kind:       ActionSubcommand,
			subcommand: "extra-usage",
			event:      EventSelectExtraUsage,
		},
		{
			name:    "cancel",
			value:   OptionCancel,
			kind:    ActionCancel,
			event:   EventCancel,
			display: "skip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := SelectOption(tt.value)
			if err != nil {
				t.Fatalf("SelectOption returned error: %v", err)
			}
			if plan.Selected != tt.value || plan.Kind != tt.kind || plan.Subcommand != tt.subcommand || plan.Event != tt.event || plan.Display != tt.display {
				t.Fatalf("unexpected action plan: got %+v", plan)
			}
		})
	}
}

func TestParseRateLimitHeadersHotWarning(t *testing.T) {
	snapshot := ParseRateLimitHeaders(map[string]string{
		"X-RateLimit-Limit-Requests-1H":     "100",
		"x-ratelimit-remaining-requests-1h": "5",
		"x-ratelimit-reset-requests-1h":     "120",
	})

	if !snapshot.HasData() {
		t.Fatalf("expected parsed headers")
	}
	if len(snapshot.Buckets) != 1 {
		t.Fatalf("expected one bucket, got %+v", snapshot.Buckets)
	}
	bucket := snapshot.Buckets[0]
	if bucket.Name != "requests-1h" || bucket.Used != 95 || bucket.UsagePct != 95 || bucket.ResetText != "2m" || !bucket.Hot {
		t.Fatalf("unexpected bucket: %+v", bucket)
	}
	if len(snapshot.Warnings) != 1 {
		t.Fatalf("expected one warning, got %+v", snapshot.Warnings)
	}
	warning := snapshot.Warnings[0]
	if warning.Bucket != "requests-1h" || warning.UsagePct != 95 || warning.Reset != "2m" {
		t.Fatalf("unexpected warning: %+v", warning)
	}
	if !strings.Contains(warning.Message, "95%") || !strings.Contains(warning.Message, "2m") {
		t.Fatalf("warning message missing hot details: %q", warning.Message)
	}
}

func optionValues(options []Option) []OptionValue {
	values := make([]OptionValue, 0, len(options))
	for _, option := range options {
		values = append(values, option.Value)
	}
	return values
}

func hasOption(options []Option, value OptionValue) bool {
	for _, option := range options {
		if option.Value == value {
			return true
		}
	}
	return false
}

func findOption(t *testing.T, options []Option, value OptionValue) Option {
	t.Helper()
	for _, option := range options {
		if option.Value == value {
			return option
		}
	}
	t.Fatalf("missing option %q in %+v", value, options)
	return Option{}
}
