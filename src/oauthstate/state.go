package oauthstate

import (
	"sort"
	"strings"
	"time"
)

const redactedToken = "[redacted]"

type Status string

const (
	StatusReady               Status = "ready"
	StatusMissingAccessToken  Status = "missing_access_token"
	StatusMissingRefreshToken Status = "missing_refresh_token"
	StatusMissingExpiresAt    Status = "missing_expires_at"
	StatusExpired             Status = "expired"
	StatusExpiringSoon        Status = "expiring_soon"
)

type Tokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Scopes       []string
}

type Profile struct {
	ID           string
	Email        string
	DisplayName  string
	Subscription string
}

type CacheState struct {
	Tokens        Tokens
	Profile       *Profile
	LastRefreshAt time.Time
}

type RefreshResponse struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Scopes       []string
	Profile      *Profile
}

type RefreshDecision struct {
	Needed                 bool
	CanRefresh             bool
	Status                 Status
	Reason                 string
	ReuseProfileCache      bool
	ReuseSubscriptionCache bool
}

type Decision = RefreshDecision

type SafeSnapshot struct {
	AccessToken            string
	RefreshToken           string
	HasAccessToken         bool
	HasRefreshToken        bool
	ExpiresAt              time.Time
	Scopes                 []string
	Profile                *Profile
	LastRefreshAt          time.Time
	ReuseProfileCache      bool
	ReuseSubscriptionCache bool
}

func IsExpired(tokens Tokens, now time.Time, buffer time.Duration) bool {
	status := tokenStatus(tokens, now, buffer)
	return status == StatusMissingAccessToken ||
		status == StatusMissingExpiresAt ||
		status == StatusExpired ||
		status == StatusExpiringSoon
}

func NeedsRefresh(state CacheState, now time.Time, buffer time.Duration) Decision {
	status := tokenStatus(state.Tokens, now, buffer)
	canRefresh := tokenPresent(state.Tokens.RefreshToken)
	needed := status != StatusReady

	if status == StatusReady && !canRefresh {
		status = StatusMissingRefreshToken
	}

	return Decision{
		Needed:                 needed,
		CanRefresh:             canRefresh,
		Status:                 status,
		Reason:                 reasonForStatus(status, needed, canRefresh),
		ReuseProfileCache:      state.Profile != nil,
		ReuseSubscriptionCache: hasSubscription(state.Profile),
	}
}

func ApplyRefresh(state CacheState, response RefreshResponse, now time.Time) CacheState {
	next := cloneState(state)
	next.Tokens.AccessToken = response.AccessToken
	if tokenPresent(response.RefreshToken) {
		next.Tokens.RefreshToken = response.RefreshToken
	}
	next.Tokens.ExpiresAt = response.ExpiresAt
	if response.Scopes != nil {
		next.Tokens.Scopes = normalizeScopes(response.Scopes)
	} else {
		next.Tokens.Scopes = normalizeScopes(next.Tokens.Scopes)
	}
	if response.Profile != nil {
		next.Profile = cloneProfile(response.Profile)
	}
	next.LastRefreshAt = now
	return next
}

func (state CacheState) SafeSnapshot() SafeSnapshot {
	return SafeSnapshot{
		AccessToken:            redact(state.Tokens.AccessToken),
		RefreshToken:           redact(state.Tokens.RefreshToken),
		HasAccessToken:         tokenPresent(state.Tokens.AccessToken),
		HasRefreshToken:        tokenPresent(state.Tokens.RefreshToken),
		ExpiresAt:              state.Tokens.ExpiresAt,
		Scopes:                 normalizeScopes(state.Tokens.Scopes),
		Profile:                cloneProfile(state.Profile),
		LastRefreshAt:          state.LastRefreshAt,
		ReuseProfileCache:      state.Profile != nil,
		ReuseSubscriptionCache: hasSubscription(state.Profile),
	}
}

func tokenStatus(tokens Tokens, now time.Time, buffer time.Duration) Status {
	if !tokenPresent(tokens.AccessToken) {
		return StatusMissingAccessToken
	}
	if tokens.ExpiresAt.IsZero() {
		return StatusMissingExpiresAt
	}
	if !tokens.ExpiresAt.After(now) {
		return StatusExpired
	}
	if !tokens.ExpiresAt.After(now.Add(normalizeBuffer(buffer))) {
		return StatusExpiringSoon
	}
	return StatusReady
}

func reasonForStatus(status Status, needed bool, canRefresh bool) string {
	var reason string
	switch status {
	case StatusReady:
		reason = "access token is valid"
	case StatusMissingAccessToken:
		reason = "access token is missing"
	case StatusMissingRefreshToken:
		reason = "refresh token is missing"
	case StatusMissingExpiresAt:
		reason = "token expiry is missing"
	case StatusExpired:
		reason = "access token is expired"
	case StatusExpiringSoon:
		reason = "access token expires within refresh buffer"
	default:
		reason = "token status is unknown"
	}
	if needed && !canRefresh {
		reason += "; refresh token is missing"
	}
	return reason
}

func normalizeScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		normalized = append(normalized, scope)
	}
	sort.Strings(normalized)
	return normalized
}

func normalizeBuffer(buffer time.Duration) time.Duration {
	if buffer < 0 {
		return 0
	}
	return buffer
}

func tokenPresent(token string) bool {
	return strings.TrimSpace(token) != ""
}

func hasSubscription(profile *Profile) bool {
	return profile != nil && strings.TrimSpace(profile.Subscription) != ""
}

func cloneState(state CacheState) CacheState {
	return CacheState{
		Tokens:        cloneTokens(state.Tokens),
		Profile:       cloneProfile(state.Profile),
		LastRefreshAt: state.LastRefreshAt,
	}
}

func cloneTokens(tokens Tokens) Tokens {
	tokens.Scopes = append([]string(nil), tokens.Scopes...)
	return tokens
}

func cloneProfile(profile *Profile) *Profile {
	if profile == nil {
		return nil
	}
	copied := *profile
	return &copied
}

func redact(token string) string {
	if !tokenPresent(token) {
		return ""
	}
	return redactedToken
}
