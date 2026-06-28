package policylimits

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	ReasonEligibleAPIKey                      = "eligible_api_key"
	ReasonEligibleOAuth                       = "eligible_oauth"
	ReasonNotFirstPartyProvider               = "not_first_party_provider"
	ReasonNotFirstPartyBaseURL                = "not_first_party_base_url"
	ReasonNoAuthentication                    = "no_authentication"
	ReasonOAuthMissingInferenceScope          = "oauth_missing_inference_scope"
	ReasonOAuthUnsupportedSubscription        = "oauth_unsupported_subscription"
	ReasonFetchedRestrictions                 = "fetched_restrictions"
	ReasonCacheValid304                       = "cache_valid_304"
	ReasonNoCacheFor304FailOpen               = "no_cache_for_304_fail_open"
	ReasonEmptyRestrictionsDeleteCache        = "empty_restrictions_delete_cache"
	ReasonFetchFailureStaleCache              = "fetch_failure_stale_cache"
	ReasonFetchFailureNoCacheFailOpen         = "fetch_failure_no_cache_fail_open"
	ReasonFetchFailureNoCacheEssentialTraffic = "fetch_failure_no_cache_essential_traffic"
	ReasonCacheMissFailOpen                   = "cache_miss_fail_open"
	ReasonEssentialTrafficCacheMissFailClosed = "essential_traffic_cache_miss_fail_closed"
	ReasonUnknownPolicyAllowed                = "unknown_policy_allowed"
	ReasonPolicyAllowed                       = "policy_allowed"
	ReasonPolicyDenied                        = "policy_denied"
	ReasonPermissionTimeoutDeny               = "permission_timeout_deny"
	ReasonPermissionErrorDeny                 = "permission_error_deny"
	ReasonPermissionNoResponseDeny            = "permission_no_response_deny"
	ReasonPermissionUnknownOptionDeny         = "permission_unknown_option_deny"
	ReasonPermissionAllowOnce                 = "permission_allow_once"
	ReasonPermissionAllowAlways               = "permission_allow_always"
	ReasonPermissionRejectDeny                = "permission_reject_deny"
	ReasonPermissionUnknownKindDeny           = "permission_unknown_kind_deny"
	ReasonPollingEligible                     = "polling_eligible"
	ReasonPollingNotEligible                  = "polling_not_eligible"
)

const (
	DefaultLoadingTimeout = 30 * time.Second
	DefaultPollInterval   = time.Hour
)

func ComputeChecksum(restrictions Restrictions) string {
	normalized := canonicalRestrictionsJSON(restrictions)
	sum := sha256.Sum256([]byte(normalized))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func IsEligible(input EligibilityInput) bool {
	decision := EligibilityDecision(input)
	return decision.Eligible
}

type EligibilityDecisionResult struct {
	Eligible bool
	Reason   string
}

func EligibilityDecision(input EligibilityInput) EligibilityDecisionResult {
	if !input.FirstPartyProvider {
		return EligibilityDecisionResult{Reason: ReasonNotFirstPartyProvider}
	}
	if !input.FirstPartyBaseURL {
		return EligibilityDecisionResult{Reason: ReasonNotFirstPartyBaseURL}
	}
	if input.HasAPIKey {
		return EligibilityDecisionResult{Eligible: true, Reason: ReasonEligibleAPIKey}
	}
	if !input.HasOAuthAccessToken {
		return EligibilityDecisionResult{Reason: ReasonNoAuthentication}
	}
	if !hasScope(input.OAuthScopes, ClaudeAIInferenceScope) {
		return EligibilityDecisionResult{Reason: ReasonOAuthMissingInferenceScope}
	}
	switch strings.ToLower(strings.TrimSpace(input.SubscriptionType)) {
	case "team", "enterprise":
		return EligibilityDecisionResult{Eligible: true, Reason: ReasonEligibleOAuth}
	default:
		return EligibilityDecisionResult{Reason: ReasonOAuthUnsupportedSubscription}
	}
}

func BuildAuthHeaders(input AuthInput) AuthHeaders {
	apiKey := strings.TrimSpace(input.APIKey)
	if apiKey != "" {
		return AuthHeaders{Headers: map[string]string{"x-api-key": apiKey}}
	}

	token := strings.TrimSpace(input.OAuthAccessToken)
	if token != "" {
		headers := map[string]string{"Authorization": "Bearer " + token}
		if beta := strings.TrimSpace(input.OAuthBetaHeader); beta != "" {
			headers["anthropic-beta"] = beta
		}
		return AuthHeaders{Headers: headers}
	}

	return AuthHeaders{
		Headers: map[string]string{},
		Error:   "No authentication available",
	}
}

func LoadFromFetch(cached *CacheEntry, result FetchResult, essentialTrafficOnly bool) LoadDecision {
	if !result.Success {
		if hasCache(cached) {
			return LoadDecision{
				SessionCache:    cloneRestrictions(cached.Restrictions),
				HasSessionCache: true,
				UsedStaleCache:  true,
				Reason:          ReasonFetchFailureStaleCache,
				Checksum:        cached.Checksum,
				ETag:            cached.ETag,
			}
		}

		reason := ReasonFetchFailureNoCacheFailOpen
		if essentialTrafficOnly {
			reason = ReasonFetchFailureNoCacheEssentialTraffic
		}
		return LoadDecision{
			FailOpen: true,
			Reason:   reason,
		}
	}

	if result.NotModified || result.StatusCode == 304 {
		if hasCache(cached) {
			return LoadDecision{
				SessionCache:    cloneRestrictions(cached.Restrictions),
				HasSessionCache: true,
				Reason:          ReasonCacheValid304,
				Checksum:        cached.Checksum,
				ETag:            firstNonEmpty(result.ETag, cached.ETag),
			}
		}
		return LoadDecision{
			FailOpen: true,
			Reason:   ReasonNoCacheFor304FailOpen,
			ETag:     result.ETag,
		}
	}

	restrictions := cloneRestrictions(result.Restrictions)
	if restrictions == nil {
		restrictions = Restrictions{}
	}
	if len(restrictions) == 0 || result.StatusCode == 404 {
		return LoadDecision{
			SessionCache:    Restrictions{},
			HasSessionCache: true,
			DeleteCache:     true,
			Reason:          ReasonEmptyRestrictionsDeleteCache,
			ETag:            result.ETag,
		}
	}

	checksum := ComputeChecksum(restrictions)
	return LoadDecision{
		SessionCache:    restrictions,
		HasSessionCache: true,
		WriteCache:      true,
		Reason:          ReasonFetchedRestrictions,
		Checksum:        checksum,
		ETag:            firstNonEmpty(result.ETag, checksum),
	}
}

func IsPolicyAllowed(restrictions Restrictions, policy string, essentialTrafficOnly bool) PolicyDecision {
	policy = strings.TrimSpace(policy)
	if restrictions == nil {
		if essentialTrafficOnly && policy == PolicyAllowProductFeedback {
			return PolicyDecision{
				Policy:     policy,
				Allowed:    false,
				FailClosed: true,
				Reason:     ReasonEssentialTrafficCacheMissFailClosed,
			}
		}
		return PolicyDecision{
			Policy:   policy,
			Allowed:  true,
			FailOpen: true,
			Reason:   ReasonCacheMissFailOpen,
		}
	}

	restriction, ok := restrictions[policy]
	if !ok {
		return PolicyDecision{
			Policy:  policy,
			Allowed: true,
			Reason:  ReasonUnknownPolicyAllowed,
		}
	}
	if restriction.Allowed {
		return PolicyDecision{
			Policy:  policy,
			Allowed: true,
			Reason:  ReasonPolicyAllowed,
		}
	}
	return PolicyDecision{
		Policy:  policy,
		Allowed: false,
		Reason:  ReasonPolicyDenied,
	}
}

func PermissionBridgeDecision(input PermissionBridgeInput) PermissionBridgeOutcome {
	if input.TimedOut {
		return denyPermission(ReasonPermissionTimeoutDeny)
	}
	if input.Err != nil {
		return denyPermission(ReasonPermissionErrorDeny)
	}
	if !input.Responded {
		return denyPermission(ReasonPermissionNoResponseDeny)
	}

	for _, option := range input.Options {
		if option.ID != input.OptionID {
			continue
		}
		switch option.Kind {
		case PermissionOptionAllowOnce:
			return PermissionBridgeOutcome{
				Result: PermissionResultOnce,
				Reason: ReasonPermissionAllowOnce,
			}
		case PermissionOptionAllowAlways:
			return PermissionBridgeOutcome{
				Result: PermissionResultAlways,
				Reason: ReasonPermissionAllowAlways,
			}
		case PermissionOptionRejectOnce, PermissionOptionRejectAlways:
			return denyPermission(ReasonPermissionRejectDeny)
		default:
			return denyPermission(ReasonPermissionUnknownKindDeny)
		}
	}

	return denyPermission(ReasonPermissionUnknownOptionDeny)
}

func PlanPolling(eligible bool) PollPlan {
	return PlanPollingWithDurations(eligible, DefaultLoadingTimeout, DefaultPollInterval)
}

func PlanPollingWithDurations(eligible bool, loadingTimeout time.Duration, pollInterval time.Duration) PollPlan {
	if loadingTimeout <= 0 {
		loadingTimeout = DefaultLoadingTimeout
	}
	if pollInterval <= 0 {
		pollInterval = DefaultPollInterval
	}
	if !eligible {
		return PollPlan{
			Eligible:       false,
			LoadingTimeout: loadingTimeout,
			PollInterval:   pollInterval,
			Reason:         ReasonPollingNotEligible,
		}
	}
	return PollPlan{
		Eligible:                     true,
		ShouldCreateLoadingPromise:   true,
		ShouldStartBackgroundPolling: true,
		LoadingTimeout:               loadingTimeout,
		PollInterval:                 pollInterval,
		Reason:                       ReasonPollingEligible,
	}
}

func canonicalRestrictionsJSON(restrictions Restrictions) string {
	if restrictions == nil {
		restrictions = Restrictions{}
	}

	keys := make([]string, 0, len(restrictions))
	for key := range restrictions {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	builder.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			builder.WriteByte(',')
		}
		encodedKey, _ := json.Marshal(key)
		builder.Write(encodedKey)
		builder.WriteString(`:{"allowed":`)
		builder.WriteString(strconv.FormatBool(restrictions[key].Allowed))
		builder.WriteByte('}')
	}
	builder.WriteByte('}')
	return builder.String()
}

func cloneRestrictions(restrictions Restrictions) Restrictions {
	if restrictions == nil {
		return nil
	}
	cloned := make(Restrictions, len(restrictions))
	for policy, restriction := range restrictions {
		cloned[policy] = restriction
	}
	return cloned
}

func hasCache(cached *CacheEntry) bool {
	return cached != nil && cached.Restrictions != nil
}

func hasScope(scopes []string, expected string) bool {
	for _, scope := range scopes {
		if strings.TrimSpace(scope) == expected {
			return true
		}
	}
	return false
}

func denyPermission(reason string) PermissionBridgeOutcome {
	return PermissionBridgeOutcome{
		Result: PermissionResultDeny,
		Denied: true,
		Reason: reason,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
