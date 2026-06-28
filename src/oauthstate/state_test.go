package oauthstate

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestIsExpiredUsesRefreshBuffer(t *testing.T) {
	now := time.Unix(1000, 0)
	tokens := Tokens{
		AccessToken: "access",
		ExpiresAt:   now.Add(5 * time.Minute),
	}

	if !IsExpired(tokens, now, 10*time.Minute) {
		t.Fatalf("expected token inside refresh buffer to be expired")
	}
	if IsExpired(tokens, now, 2*time.Minute) {
		t.Fatalf("expected token outside refresh buffer to remain valid")
	}
}

func TestNeedsRefreshHandlesMissingTokenState(t *testing.T) {
	now := time.Unix(1000, 0)

	decision := NeedsRefresh(CacheState{
		Tokens: Tokens{RefreshToken: "refresh"},
	}, now, time.Minute)
	if !decision.Needed || !decision.CanRefresh || decision.Status != StatusMissingAccessToken {
		t.Fatalf("unexpected missing access token decision: %+v", decision)
	}

	decision = NeedsRefresh(CacheState{
		Tokens: Tokens{
			AccessToken: "access",
			ExpiresAt:   now.Add(time.Hour),
		},
	}, now, time.Minute)
	if decision.Needed || decision.CanRefresh || decision.Status != StatusMissingRefreshToken {
		t.Fatalf("unexpected missing refresh token decision: %+v", decision)
	}

	decision = NeedsRefresh(CacheState{
		Tokens: Tokens{
			AccessToken:  "access",
			RefreshToken: "refresh",
		},
	}, now, time.Minute)
	if !decision.Needed || !decision.CanRefresh || decision.Status != StatusMissingExpiresAt {
		t.Fatalf("unexpected missing expiry decision: %+v", decision)
	}
}

func TestNeedsRefreshReportsExpiredTokenWithoutRefreshToken(t *testing.T) {
	now := time.Unix(1000, 0)
	decision := NeedsRefresh(CacheState{
		Tokens: Tokens{
			AccessToken: "access",
			ExpiresAt:   now.Add(-time.Minute),
		},
	}, now, time.Minute)

	if !decision.Needed || decision.CanRefresh || decision.Status != StatusExpired {
		t.Fatalf("unexpected expired token decision: %+v", decision)
	}
	if !strings.Contains(decision.Reason, "refresh token is missing") {
		t.Fatalf("expected missing refresh token in reason, got %q", decision.Reason)
	}
}

func TestApplyRefreshPreservesRefreshTokenWhenResponseOmitsIt(t *testing.T) {
	now := time.Unix(1000, 0)
	state := CacheState{
		Tokens: Tokens{
			AccessToken:  "old-access",
			RefreshToken: "old-refresh",
			ExpiresAt:    now,
		},
	}

	next := ApplyRefresh(state, RefreshResponse{
		AccessToken: "new-access",
		ExpiresAt:   now.Add(time.Hour),
	}, now)

	if next.Tokens.AccessToken != "new-access" {
		t.Fatalf("expected access token replacement, got %q", next.Tokens.AccessToken)
	}
	if next.Tokens.RefreshToken != "old-refresh" {
		t.Fatalf("expected old refresh token passthrough, got %q", next.Tokens.RefreshToken)
	}
}

func TestApplyRefreshReplacesRefreshTokenWhenResponseIncludesIt(t *testing.T) {
	now := time.Unix(1000, 0)
	state := CacheState{
		Tokens: Tokens{
			AccessToken:  "old-access",
			RefreshToken: "old-refresh",
			ExpiresAt:    now,
		},
	}

	next := ApplyRefresh(state, RefreshResponse{
		AccessToken:  "new-access",
		RefreshToken: "new-refresh",
		ExpiresAt:    now.Add(time.Hour),
	}, now)

	if next.Tokens.RefreshToken != "new-refresh" {
		t.Fatalf("expected new refresh token, got %q", next.Tokens.RefreshToken)
	}
}

func TestApplyRefreshPreservesProfileCache(t *testing.T) {
	now := time.Unix(1000, 0)
	state := CacheState{
		Tokens: Tokens{
			AccessToken:  "old-access",
			RefreshToken: "refresh",
			ExpiresAt:    now,
		},
		Profile: &Profile{
			ID:           "user-1",
			Email:        "user@example.com",
			DisplayName:  "User",
			Subscription: "pro",
		},
	}

	next := ApplyRefresh(state, RefreshResponse{
		AccessToken: "new-access",
		ExpiresAt:   now.Add(time.Hour),
	}, now)

	if next.Profile == nil || *next.Profile != *state.Profile {
		t.Fatalf("expected profile cache preservation, got %+v", next.Profile)
	}
	if next.Profile == state.Profile {
		t.Fatalf("expected preserved profile to be copied")
	}

	decision := NeedsRefresh(next, now.Add(2*time.Hour), time.Minute)
	if !decision.ReuseProfileCache || !decision.ReuseSubscriptionCache {
		t.Fatalf("expected profile and subscription cache reuse, got %+v", decision)
	}
}

func TestApplyRefreshNormalizesScopes(t *testing.T) {
	now := time.Unix(1000, 0)
	next := ApplyRefresh(CacheState{}, RefreshResponse{
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresAt:    now.Add(time.Hour),
		Scopes:       []string{" write ", "read", "", "read", "admin"},
	}, now)

	want := []string{"admin", "read", "write"}
	if !reflect.DeepEqual(next.Tokens.Scopes, want) {
		t.Fatalf("expected normalized scopes %v, got %v", want, next.Tokens.Scopes)
	}
}

func TestSafeSnapshotRedactsSecrets(t *testing.T) {
	now := time.Unix(1000, 0)
	state := CacheState{
		Tokens: Tokens{
			AccessToken:  "access-secret",
			RefreshToken: "refresh-secret",
			ExpiresAt:    now.Add(time.Hour),
			Scopes:       []string{"read"},
		},
	}

	snapshot := state.SafeSnapshot()
	if snapshot.AccessToken != redactedToken || snapshot.RefreshToken != redactedToken {
		t.Fatalf("expected redacted tokens, got %+v", snapshot)
	}
	if !snapshot.HasAccessToken || !snapshot.HasRefreshToken {
		t.Fatalf("expected token presence flags, got %+v", snapshot)
	}

	dump := fmt.Sprintf("%+v", snapshot)
	if strings.Contains(dump, "access-secret") || strings.Contains(dump, "refresh-secret") {
		t.Fatalf("safe snapshot leaked token material: %s", dump)
	}
}

func TestDefensiveCopies(t *testing.T) {
	now := time.Unix(1000, 0)
	responseScopes := []string{"read"}
	responseProfile := &Profile{ID: "user-1", Email: "user@example.com"}

	next := ApplyRefresh(CacheState{}, RefreshResponse{
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresAt:    now.Add(time.Hour),
		Scopes:       responseScopes,
		Profile:      responseProfile,
	}, now)

	responseScopes[0] = "write"
	responseProfile.Email = "changed@example.com"
	if !reflect.DeepEqual(next.Tokens.Scopes, []string{"read"}) {
		t.Fatalf("expected response scope copy, got %v", next.Tokens.Scopes)
	}
	if next.Profile.Email != "user@example.com" {
		t.Fatalf("expected response profile copy, got %+v", next.Profile)
	}

	snapshot := next.SafeSnapshot()
	snapshot.Scopes[0] = "mutated"
	snapshot.Profile.Email = "snapshot@example.com"
	if !reflect.DeepEqual(next.Tokens.Scopes, []string{"read"}) {
		t.Fatalf("expected snapshot scope mutation not to affect state, got %v", next.Tokens.Scopes)
	}
	if next.Profile.Email != "user@example.com" {
		t.Fatalf("expected snapshot profile mutation not to affect state, got %+v", next.Profile)
	}
}
