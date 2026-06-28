package fastmode

import "strings"

const (
	FastModelDisplay = "Opus 4.6"

	ReasonFeatureFlagDisabled = "Fast mode is not available"
	ReasonNativeBinaryGate    = "Fast mode requires the native binary · Install from: https://claude.com/product/claude-code"
	ReasonSDKGate             = "Fast mode is not available in the Agent SDK"
	ReasonFirstPartyOnly      = "Fast mode is not available on Bedrock, Vertex, or Foundry"
)

func Decide(env Environment, settings Settings) Decision {
	reason := UnavailableReason(env, settings)
	return Decision{
		Available:         reason == "",
		UnavailableReason: reason,
	}
}

func UnavailableReason(env Environment, settings Settings) string {
	if env.FeatureDisabled {
		return ReasonFeatureFlagDisabled
	}
	if env.StatsigReason != "" {
		return env.StatsigReason
	}
	if env.NativeBinaryRequired && !env.BundledMode {
		return ReasonNativeBinaryGate
	}
	if env.NonInteractiveSession && env.PreferThirdPartyAuthentication && !env.KairosActive && !settings.FlagFastMode {
		return ReasonSDKGate
	}
	if normalizeProvider(env.Provider) != ProviderFirstParty {
		return ReasonFirstPartyOnly
	}
	if env.Org.State == OrgDisabled {
		reason := normalizeOrgReason(env.Org.Reason)
		if env.SkipNetworkErrors && (reason == OrgReasonNetworkError || reason == OrgReasonUnknown) {
			return ""
		}
		return orgDisabledMessage(reason, normalizeAuth(env.Auth))
	}
	return ""
}

func IsSupportedModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return false
	}
	if model == "opus" || strings.HasPrefix(model, "opus[") {
		return true
	}
	return strings.Contains(model, "opus-4-6")
}

func FastModel(settings ...Settings) string {
	if len(settings) > 0 && settings[0].Opus1MMergeEnabled {
		return "opus[1m]"
	}
	return "opus"
}

func InitialSetting(model string, settings Settings, env Environment) bool {
	if UnavailableReason(env, settings) != "" {
		return false
	}
	if !IsSupportedModel(model) {
		return false
	}
	if settings.FastModePerSessionOptIn {
		return false
	}
	return settings.FastMode != nil && *settings.FastMode
}

func ApplyTogglePlan(enable bool, model string, settings Settings) RoutePlan {
	settings.FastMode = Bool(enable)
	if !enable {
		settings.FastMode = nil
		return RoutePlan{
			FastMode:      false,
			Model:         model,
			ClearCooldown: true,
			Settings:      settings,
		}
	}

	routeModel := model
	modelChanged := false
	if !IsSupportedModel(model) {
		routeModel = FastModel(settings)
		modelChanged = true
	}

	return RoutePlan{
		FastMode:          true,
		Model:             routeModel,
		ModelChanged:      modelChanged,
		ClearSessionModel: modelChanged,
		ClearCooldown:     true,
		Settings:          settings,
	}
}

func ApplyAPIRejection(settings Settings, org OrgStatus) RoutePlan {
	if org.State == OrgDisabled {
		return RoutePlan{Settings: settings, Org: org}
	}
	settings.FastMode = nil
	org = DisabledOrg(OrgReasonPreference)
	return RoutePlan{
		FastMode:            false,
		Settings:            settings,
		Org:                 org,
		PermanentlyDisabled: true,
		Events: []EventPlan{{
			Type:       EventOrgChanged,
			OrgEnabled: false,
		}},
	}
}

func ApplyExtraUsageRejection(reason string, settings Settings) RoutePlan {
	message := extraUsageMessage(reason)
	permanent := !isOutOfCredits(reason)
	if permanent {
		settings.FastMode = nil
	}
	return RoutePlan{
		FastMode:            false,
		Settings:            settings,
		PermanentlyDisabled: permanent,
		Message:             message,
		Events: []EventPlan{{
			Type:    EventOverageRejected,
			Message: message,
		}},
	}
}

func orgDisabledMessage(reason OrgDisabledReason, auth AuthType) string {
	switch reason {
	case OrgReasonFree:
		if auth == AuthOAuth {
			return "Fast mode requires a paid subscription"
		}
		return "Fast mode unavailable during evaluation. Please purchase credits."
	case OrgReasonPreference:
		return "Fast mode has been disabled by your organization"
	case OrgReasonExtraUsageDisabled:
		return "Fast mode requires extra usage billing · /extra-usage to enable"
	case OrgReasonNetworkError:
		return "Fast mode unavailable due to network connectivity issues"
	case OrgReasonUnknown:
		return "Fast mode is currently unavailable"
	default:
		return "Fast mode is currently unavailable"
	}
}

func extraUsageMessage(reason string) string {
	switch reason {
	case "out_of_credits":
		return "Fast mode disabled · extra usage credits exhausted"
	case "org_level_disabled", "org_service_level_disabled":
		return "Fast mode disabled · extra usage disabled by your organization"
	case "org_level_disabled_until":
		return "Fast mode disabled · extra usage spending cap reached"
	case "member_level_disabled":
		return "Fast mode disabled · extra usage disabled for your account"
	case "seat_tier_level_disabled", "seat_tier_zero_credit_limit", "member_zero_credit_limit":
		return "Fast mode disabled · extra usage not available for your plan"
	case "overage_not_provisioned", "no_limits_configured":
		return "Fast mode requires extra usage billing · /extra-usage to enable"
	default:
		return "Fast mode disabled · extra usage not available"
	}
}

func isOutOfCredits(reason string) bool {
	return reason == "org_level_disabled_until" || reason == "out_of_credits"
}

func normalizeProvider(provider ProviderKind) ProviderKind {
	if provider == "" {
		return ProviderFirstParty
	}
	return provider
}

func normalizeAuth(auth AuthType) AuthType {
	if auth == "" {
		return AuthAPIKey
	}
	return auth
}

func normalizeOrgReason(reason OrgDisabledReason) OrgDisabledReason {
	if reason == "" {
		return OrgReasonPreference
	}
	return reason
}
