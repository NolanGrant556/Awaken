package policylimits

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

func TestComputeChecksumStable(t *testing.T) {
	left := Restrictions{
		"allow_remote_sessions":  {Allowed: true},
		"allow_product_feedback": {Allowed: false},
	}
	right := Restrictions{
		"allow_product_feedback": {Allowed: false},
		"allow_remote_sessions":  {Allowed: true},
	}

	leftChecksum := ComputeChecksum(left)
	rightChecksum := ComputeChecksum(right)
	if leftChecksum != rightChecksum {
		t.Fatalf("expected stable checksum, got %q and %q", leftChecksum, rightChecksum)
	}

	sum := sha256.Sum256([]byte(`{"allow_product_feedback":{"allowed":false},"allow_remote_sessions":{"allowed":true}}`))
	expected := "sha256:" + hex.EncodeToString(sum[:])
	if leftChecksum != expected {
		t.Fatalf("expected checksum %q, got %q", expected, leftChecksum)
	}
}

func TestIsEligible(t *testing.T) {
	tests := []struct {
		name  string
		input EligibilityInput
		want  bool
	}{
		{
			name: "api key with first party provider and base url",
			input: EligibilityInput{
				FirstPartyProvider: true,
				FirstPartyBaseURL:  true,
				HasAPIKey:          true,
			},
			want: true,
		},
		{
			name: "custom base url is ineligible",
			input: EligibilityInput{
				FirstPartyProvider: true,
				FirstPartyBaseURL:  false,
				HasAPIKey:          true,
			},
			want: false,
		},
		{
			name: "third party provider is ineligible",
			input: EligibilityInput{
				FirstPartyProvider: false,
				FirstPartyBaseURL:  true,
				HasAPIKey:          true,
			},
			want: false,
		},
		{
			name: "oauth team with inference scope",
			input: EligibilityInput{
				FirstPartyProvider:  true,
				FirstPartyBaseURL:   true,
				HasOAuthAccessToken: true,
				OAuthScopes:         []string{ClaudeAIInferenceScope},
				SubscriptionType:    "team",
			},
			want: true,
		},
		{
			name: "oauth enterprise with inference scope",
			input: EligibilityInput{
				FirstPartyProvider:  true,
				FirstPartyBaseURL:   true,
				HasOAuthAccessToken: true,
				OAuthScopes:         []string{ClaudeAIInferenceScope},
				SubscriptionType:    "enterprise",
			},
			want: true,
		},
		{
			name: "oauth missing inference scope",
			input: EligibilityInput{
				FirstPartyProvider:  true,
				FirstPartyBaseURL:   true,
				HasOAuthAccessToken: true,
				OAuthScopes:         []string{"profile"},
				SubscriptionType:    "team",
			},
			want: false,
		},
		{
			name: "oauth individual subscription",
			input: EligibilityInput{
				FirstPartyProvider:  true,
				FirstPartyBaseURL:   true,
				HasOAuthAccessToken: true,
				OAuthScopes:         []string{ClaudeAIInferenceScope},
				SubscriptionType:    "pro",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsEligible(tt.input); got != tt.want {
				t.Fatalf("IsEligible() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadFromFetch304UsesCachedRestrictions(t *testing.T) {
	cached := &CacheEntry{
		Restrictions: Restrictions{
			"allow_remote_sessions": {Allowed: false},
		},
		Checksum: "sha256:cached",
		ETag:     "cached-etag",
	}

	decision := LoadFromFetch(cached, FetchResult{
		Success:     true,
		NotModified: true,
		StatusCode:  304,
		ETag:        "fresh-etag",
	}, false)

	if !decision.HasSessionCache {
		t.Fatalf("expected session cache")
	}
	if decision.WriteCache || decision.DeleteCache || decision.FailOpen || decision.UsedStaleCache {
		t.Fatalf("unexpected cache side effects: %+v", decision)
	}
	if decision.Reason != ReasonCacheValid304 {
		t.Fatalf("expected reason %q, got %q", ReasonCacheValid304, decision.Reason)
	}
	if decision.ETag != "fresh-etag" {
		t.Fatalf("expected refreshed etag, got %q", decision.ETag)
	}
	if decision.SessionCache["allow_remote_sessions"].Allowed {
		t.Fatalf("expected cached deny restriction to remain in session cache")
	}
}

func TestLoadFromFetch404DeletesCache(t *testing.T) {
	cached := &CacheEntry{
		Restrictions: Restrictions{
			"allow_remote_sessions": {Allowed: false},
		},
	}

	decision := LoadFromFetch(cached, FetchResult{
		Success:      true,
		StatusCode:   404,
		Restrictions: Restrictions{},
	}, false)

	if !decision.HasSessionCache {
		t.Fatalf("expected empty restrictions to still become session cache")
	}
	if decision.SessionCache == nil || len(decision.SessionCache) != 0 {
		t.Fatalf("expected empty non-nil session cache, got %#v", decision.SessionCache)
	}
	if !decision.DeleteCache || decision.WriteCache {
		t.Fatalf("expected cache delete without write: %+v", decision)
	}
	if decision.Reason != ReasonEmptyRestrictionsDeleteCache {
		t.Fatalf("expected reason %q, got %q", ReasonEmptyRestrictionsDeleteCache, decision.Reason)
	}
}

func TestLoadFromFetchFailureUsesStaleCache(t *testing.T) {
	cached := &CacheEntry{
		Restrictions: Restrictions{
			"allow_remote_sessions": {Allowed: false},
		},
		Checksum: "sha256:cached",
	}

	decision := LoadFromFetch(cached, FetchResult{
		Success: false,
		Error:   "network failure",
	}, false)

	if !decision.HasSessionCache || !decision.UsedStaleCache {
		t.Fatalf("expected stale cache, got %+v", decision)
	}
	if decision.FailOpen || decision.WriteCache || decision.DeleteCache {
		t.Fatalf("unexpected failure handling: %+v", decision)
	}
	if decision.Reason != ReasonFetchFailureStaleCache {
		t.Fatalf("expected reason %q, got %q", ReasonFetchFailureStaleCache, decision.Reason)
	}
}

func TestLoadFromFetchFailureNoCacheFailsOpen(t *testing.T) {
	decision := LoadFromFetch(nil, FetchResult{
		Success: false,
		Error:   "timeout",
	}, false)

	if !decision.FailOpen {
		t.Fatalf("expected fail-open decision, got %+v", decision)
	}
	if decision.HasSessionCache || decision.WriteCache || decision.DeleteCache {
		t.Fatalf("unexpected cache side effects: %+v", decision)
	}
	if decision.Reason != ReasonFetchFailureNoCacheFailOpen {
		t.Fatalf("expected reason %q, got %q", ReasonFetchFailureNoCacheFailOpen, decision.Reason)
	}
}

func TestIsPolicyAllowedEssentialTrafficFailClosedOnMiss(t *testing.T) {
	decision := IsPolicyAllowed(nil, PolicyAllowProductFeedback, true)
	if decision.Allowed || !decision.FailClosed {
		t.Fatalf("expected essential traffic product feedback miss to fail closed: %+v", decision)
	}
	if decision.Reason != ReasonEssentialTrafficCacheMissFailClosed {
		t.Fatalf("expected reason %q, got %q", ReasonEssentialTrafficCacheMissFailClosed, decision.Reason)
	}

	other := IsPolicyAllowed(nil, "allow_remote_sessions", true)
	if !other.Allowed || !other.FailOpen {
		t.Fatalf("expected other policy cache miss to fail open: %+v", other)
	}
}

func TestIsPolicyAllowedKnownDenyAllowAndUnknown(t *testing.T) {
	restrictions := Restrictions{
		"allow_remote_sessions":  {Allowed: false},
		"allow_product_feedback": {Allowed: true},
	}

	denied := IsPolicyAllowed(restrictions, "allow_remote_sessions", false)
	if denied.Allowed || denied.Reason != ReasonPolicyDenied {
		t.Fatalf("expected known deny, got %+v", denied)
	}

	allowed := IsPolicyAllowed(restrictions, "allow_product_feedback", false)
	if !allowed.Allowed || allowed.Reason != ReasonPolicyAllowed {
		t.Fatalf("expected known allow, got %+v", allowed)
	}

	unknown := IsPolicyAllowed(restrictions, "new_policy", false)
	if !unknown.Allowed || unknown.Reason != ReasonUnknownPolicyAllowed {
		t.Fatalf("expected unknown policy to allow, got %+v", unknown)
	}
}

func TestPermissionBridgeDecisionDenyBoundariesAndMapping(t *testing.T) {
	timeout := PermissionBridgeDecision(PermissionBridgeInput{TimedOut: true})
	if timeout.Result != PermissionResultDeny || !timeout.Denied || timeout.Reason != ReasonPermissionTimeoutDeny {
		t.Fatalf("expected timeout deny, got %+v", timeout)
	}

	failed := PermissionBridgeDecision(PermissionBridgeInput{Err: errors.New("request failed")})
	if failed.Result != PermissionResultDeny || !failed.Denied || failed.Reason != ReasonPermissionErrorDeny {
		t.Fatalf("expected error deny, got %+v", failed)
	}

	allowOnce := PermissionBridgeDecision(PermissionBridgeInput{
		Responded: true,
		OptionID:  "allow_once",
		Options: []PermissionOption{
			{ID: "allow_once", Kind: PermissionOptionAllowOnce},
			{ID: "allow_always", Kind: PermissionOptionAllowAlways},
			{ID: "deny", Kind: PermissionOptionRejectOnce},
		},
	})
	if allowOnce.Result != PermissionResultOnce || allowOnce.Denied {
		t.Fatalf("expected allow once mapping, got %+v", allowOnce)
	}

	unknown := PermissionBridgeDecision(PermissionBridgeInput{
		Responded: true,
		OptionID:  "unknown",
		Options: []PermissionOption{
			{ID: "allow_once", Kind: PermissionOptionAllowOnce},
		},
	})
	if unknown.Result != PermissionResultDeny || !unknown.Denied || unknown.Reason != ReasonPermissionUnknownOptionDeny {
		t.Fatalf("expected unknown option deny, got %+v", unknown)
	}
}
