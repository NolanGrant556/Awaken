package ratelimitoptions

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const Max20xTier = "default_claude_max_20x"

type SubscriptionType string

const (
	SubscriptionMax        SubscriptionType = "max"
	SubscriptionTeam       SubscriptionType = "team"
	SubscriptionEnterprise SubscriptionType = "enterprise"
	SubscriptionFree       SubscriptionType = "free"
	SubscriptionPro        SubscriptionType = "pro"
)

type OverageStatus string

const (
	OverageRejected       OverageStatus = "rejected"
	OverageAllowedWarning OverageStatus = "allowed_warning"
	OverageAllowed        OverageStatus = "allowed"
	OverageUnknown        OverageStatus = "unknown"
)

type OverageDisabledReason string

const (
	ReasonOutOfCredits              OverageDisabledReason = "out_of_credits"
	ReasonOrgLevelDisabledUntil     OverageDisabledReason = "org_level_disabled_until"
	ReasonOrgServiceZeroCreditLimit OverageDisabledReason = "org_service_zero_credit_limit"
	ReasonOther                     OverageDisabledReason = "other"
)

type OptionValue string

const (
	OptionUpgrade    OptionValue = "upgrade"
	OptionExtraUsage OptionValue = "extra-usage"
	OptionCancel     OptionValue = "cancel"
)

const (
	EventSelectUpgrade    = "tengu_rate_limit_options_menu_select_upgrade"
	EventSelectExtraUsage = "tengu_rate_limit_options_menu_select_extra_usage"
	EventCancel           = "tengu_rate_limit_options_menu_cancel"
)

type MenuInput struct {
	SubscriptionType            SubscriptionType
	RateLimitTier               string
	ExtraUsageFeatureEnabled    bool
	AccountHasExtraUsageEnabled bool
	HasBillingAccess            bool
	OverageStatus               OverageStatus
	OverageDisabledReason       OverageDisabledReason
	UpgradeEnabled              bool
	BuyFirst                    bool
}

type Option struct {
	Label string
	Value OptionValue
}

func BuildOptions(input MenuInput) []Option {
	actionOptions := make([]Option, 0, 2)
	if shouldShowExtraUsage(input) {
		actionOptions = append(actionOptions, Option{
			Label: extraUsageLabel(input),
			Value: OptionExtraUsage,
		})
	}
	if shouldShowUpgrade(input) {
		actionOptions = append(actionOptions, Option{
			Label: "Upgrade your plan",
			Value: OptionUpgrade,
		})
	}

	cancelOption := Option{
		Label: "Stop and wait for limit to reset",
		Value: OptionCancel,
	}

	options := make([]Option, 0, len(actionOptions)+1)
	if input.BuyFirst {
		options = append(options, actionOptions...)
		options = append(options, cancelOption)
		return options
	}

	options = append(options, cancelOption)
	options = append(options, actionOptions...)
	return options
}

func shouldShowExtraUsage(input MenuInput) bool {
	if !input.ExtraUsageFeatureEnabled {
		return false
	}
	needsAdminRequest := isTeamOrEnterprise(input.SubscriptionType) && !input.HasBillingAccess
	return !(needsAdminRequest && isOrgSpendCapDepleted(input.OverageDisabledReason))
}

func shouldShowUpgrade(input MenuInput) bool {
	if !input.UpgradeEnabled {
		return false
	}
	if isTeamOrEnterprise(input.SubscriptionType) {
		return false
	}
	return !(input.SubscriptionType == SubscriptionMax && input.RateLimitTier == Max20xTier)
}

func extraUsageLabel(input MenuInput) string {
	needsAdminRequest := isTeamOrEnterprise(input.SubscriptionType) && !input.HasBillingAccess
	if needsAdminRequest {
		if input.OverageStatus == OverageRejected || input.OverageStatus == OverageAllowedWarning {
			return "Request more"
		}
		return "Request extra usage"
	}

	if input.AccountHasExtraUsageEnabled {
		return "Add funds to continue with extra usage"
	}
	return "Switch to extra usage"
}

func isTeamOrEnterprise(subscription SubscriptionType) bool {
	return subscription == SubscriptionTeam || subscription == SubscriptionEnterprise
}

func isOrgSpendCapDepleted(reason OverageDisabledReason) bool {
	switch reason {
	case ReasonOutOfCredits, ReasonOrgLevelDisabledUntil, ReasonOrgServiceZeroCreditLimit:
		return true
	default:
		return false
	}
}

type ActionKind string

const (
	ActionSubcommand ActionKind = "subcommand"
	ActionCancel     ActionKind = "cancel"
)

type ActionPlan struct {
	Selected   OptionValue
	Kind       ActionKind
	Subcommand string
	Event      string
	Display    string
}

func SelectOption(value OptionValue) (ActionPlan, error) {
	switch value {
	case OptionUpgrade:
		return ActionPlan{
			Selected:   OptionUpgrade,
			Kind:       ActionSubcommand,
			Subcommand: "upgrade",
			Event:      EventSelectUpgrade,
		}, nil
	case OptionExtraUsage:
		return ActionPlan{
			Selected:   OptionExtraUsage,
			Kind:       ActionSubcommand,
			Subcommand: "extra-usage",
			Event:      EventSelectExtraUsage,
		}, nil
	case OptionCancel:
		return ActionPlan{
			Selected: OptionCancel,
			Kind:     ActionCancel,
			Event:    EventCancel,
			Display:  "skip",
		}, nil
	default:
		return ActionPlan{}, fmt.Errorf("unknown rate limit option %q", value)
	}
}

type HeaderSnapshot struct {
	Buckets  []RateLimitBucket
	Warnings []RateLimitWarning
}

func (s HeaderSnapshot) HasData() bool {
	return len(s.Buckets) > 0
}

type RateLimitBucket struct {
	Name         string
	Limit        int
	Remaining    int
	Used         int
	UsagePct     float64
	ResetSeconds float64
	ResetText    string
	Hot          bool
}

type RateLimitWarning struct {
	Bucket   string
	UsagePct float64
	Reset    string
	Message  string
}

func ParseRateLimitHeaders(headers map[string]string) HeaderSnapshot {
	lowered := make(map[string]string, len(headers))
	hasRateLimitHeader := false
	for key, value := range headers {
		normalized := strings.ToLower(strings.TrimSpace(key))
		lowered[normalized] = value
		if strings.HasPrefix(normalized, "x-ratelimit-") {
			hasRateLimitHeader = true
		}
	}
	if !hasRateLimitHeader {
		return HeaderSnapshot{}
	}

	var snapshot HeaderSnapshot
	for _, name := range []string{"requests", "requests-1h", "tokens", "tokens-1h"} {
		bucket, ok := parseRateLimitBucket(lowered, name)
		if !ok {
			continue
		}
		if bucket.Limit > 0 && bucket.UsagePct >= 80 {
			bucket.Hot = true
			snapshot.Warnings = append(snapshot.Warnings, RateLimitWarning{
				Bucket:   bucket.Name,
				UsagePct: bucket.UsagePct,
				Reset:    bucket.ResetText,
				Message:  fmt.Sprintf("%s is %.0f%% used; resets in %s", bucket.Name, bucket.UsagePct, bucket.ResetText),
			})
		}
		snapshot.Buckets = append(snapshot.Buckets, bucket)
	}
	return snapshot
}

func parseRateLimitBucket(headers map[string]string, name string) (RateLimitBucket, bool) {
	limit, hasLimit := parseIntHeader(headers["x-ratelimit-limit-"+name])
	remaining, hasRemaining := parseIntHeader(headers["x-ratelimit-remaining-"+name])
	resetSeconds, hasReset := parseFloatHeader(headers["x-ratelimit-reset-"+name])
	if !hasLimit && !hasRemaining && !hasReset {
		return RateLimitBucket{}, false
	}

	bucket := RateLimitBucket{
		Name:         name,
		Limit:        limit,
		Remaining:    remaining,
		ResetSeconds: resetSeconds,
		ResetText:    "unknown",
	}
	if hasReset {
		bucket.ResetText = formatResetSeconds(resetSeconds)
	}
	if limit > 0 {
		bucket.Used = max(0, limit-remaining)
		bucket.UsagePct = (float64(bucket.Used) / float64(limit)) * 100
	}
	return bucket, true
}

func parseIntHeader(raw string) (int, bool) {
	value, ok := parseFloatHeader(raw)
	if !ok {
		return 0, false
	}
	return int(value), true
}

func parseFloatHeader(raw string) (float64, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func formatResetSeconds(seconds float64) string {
	total := max(0, int(math.Floor(seconds)))
	if total < 60 {
		return fmt.Sprintf("%ds", total)
	}
	if total < 3600 {
		minutes := total / 60
		seconds := total % 60
		if seconds == 0 {
			return fmt.Sprintf("%dm", minutes)
		}
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}

	hours := total / 3600
	minutes := (total % 3600) / 60
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}
