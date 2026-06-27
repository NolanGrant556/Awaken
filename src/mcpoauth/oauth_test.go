package mcpoauth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCodeChallengeS256Deterministic(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	if got := CodeChallengeS256(verifier); got != want {
		t.Fatalf("CodeChallengeS256() = %q, want %q", got, want)
	}
}

func TestGeneratedVerifierAndStateUseBase64URLNoPadding(t *testing.T) {
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier() error = %v", err)
	}
	state, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() error = %v", err)
	}

	for name, value := range map[string]string{"verifier": verifier, "state": state} {
		if len(value) != 43 {
			t.Fatalf("%s length = %d, want 43", name, len(value))
		}
		if strings.ContainsAny(value, "+/=") {
			t.Fatalf("%s = %q, want base64url without padding", name, value)
		}
	}
}

func TestBuildAuthorizationURLParams(t *testing.T) {
	got, err := BuildAuthorizationURL(AuthorizationURLOptions{
		Metadata: OAuthServerMetadata{
			AuthorizationEndpoint: "https://mcp.example.com/oauth/authorize",
			TokenEndpoint:         "https://mcp.example.com/oauth/token",
		},
		Client: Client{
			ClientID:    "client-123",
			RedirectURI: "http://localhost:49152/callback",
			Scope:       "mcp:read mcp:write",
		},
		State:         "state-123",
		CodeChallenge: "challenge-123",
		Resource:      "https://mcp.example.com/mcp",
		Audience:      "mcp.example.com",
		ExtraParams: url.Values{
			"login_hint": {"dev@example.com"},
			"state":      {"ignored"},
		},
	})
	if err != nil {
		t.Fatalf("BuildAuthorizationURL() error = %v", err)
	}

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	q := parsed.Query()
	assertQueryParam(t, q, "client_id", "client-123")
	assertQueryParam(t, q, "redirect_uri", "http://localhost:49152/callback")
	assertQueryParam(t, q, "response_type", "code")
	assertQueryParam(t, q, "scope", "mcp:read mcp:write")
	assertQueryParam(t, q, "state", "state-123")
	assertQueryParam(t, q, "code_challenge", "challenge-123")
	assertQueryParam(t, q, "code_challenge_method", "S256")
	assertQueryParam(t, q, "resource", "https://mcp.example.com/mcp")
	assertQueryParam(t, q, "audience", "mcp.example.com")
	assertQueryParam(t, q, "login_hint", "dev@example.com")
}

func TestValidateCallbackRejectsStateMismatch(t *testing.T) {
	session := validSession("state-good")
	_, err := ValidateCallback(session, CallbackParams{
		Code:  "auth-code",
		State: "state-bad",
	})
	if !errors.Is(err, ErrStateMismatch) {
		t.Fatalf("ValidateCallback() error = %v, want ErrStateMismatch", err)
	}
}

func TestInMemoryPendingSessionStoreConsumeIsOneTime(t *testing.T) {
	store := NewInMemoryPendingSessionStore(fixedClock(time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)))
	session := validSession("state-once")

	if err := store.Save(context.Background(), session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := store.Consume(context.Background(), "wrong-state"); !errors.Is(err, ErrStateMismatch) {
		t.Fatalf("wrong-state Consume() error = %v, want ErrStateMismatch", err)
	}
	got, err := store.Consume(context.Background(), session.State)
	if err != nil {
		t.Fatalf("first Consume() error = %v", err)
	}
	if got.State != session.State {
		t.Fatalf("first Consume() state = %q, want %q", got.State, session.State)
	}
	if _, err := store.Consume(context.Background(), session.State); !errors.Is(err, ErrStateMismatch) {
		t.Fatalf("second Consume() error = %v, want ErrStateMismatch", err)
	}
}

func TestRefreshNeededThreshold(t *testing.T) {
	now := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	tokens := TokenSet{
		AccessToken:  NewSecret("access-token"),
		RefreshToken: NewSecret("refresh-token"),
		ExpiresAt:    now.Add(5 * time.Minute),
	}

	if !tokens.RefreshNeeded(fixedClock(now), 10*time.Minute) {
		t.Fatal("RefreshNeeded() = false, want true inside threshold")
	}
	if tokens.RefreshNeeded(fixedClock(now), time.Minute) {
		t.Fatal("RefreshNeeded() = true, want false outside threshold")
	}
	if !tokens.AccessTokenExpired(fixedClock(now.Add(6 * time.Minute))) {
		t.Fatal("AccessTokenExpired() = false, want true after expiry")
	}
}

func TestRedactedDisplayHidesTokensAndClientSecret(t *testing.T) {
	tokens := TokenSet{
		AccessToken:  NewSecret("raw-access-token"),
		RefreshToken: NewSecret("raw-refresh-token"),
		IDToken:      NewSecret("raw-id-token"),
		TokenType:    "Bearer",
	}
	client := Client{
		ClientID:     "client-id",
		ClientSecret: NewSecret("raw-client-secret"),
		RedirectURI:  "http://localhost:49152/callback",
	}
	session := validSession("state-redacted")

	display := strings.Join([]string{
		fmt.Sprintf("%+v", tokens),
		fmt.Sprintf("%#v", tokens),
		fmt.Sprintf("%+v", client),
		fmt.Sprintf("%#v", client),
		fmt.Sprintf("%+v", session),
		fmt.Sprintf("%#v", session),
	}, "\n")

	for _, raw := range []string{
		"raw-access-token",
		"raw-refresh-token",
		"raw-id-token",
		"raw-client-secret",
		validSessionVerifier,
	} {
		if strings.Contains(display, raw) {
			t.Fatalf("redacted display leaked %q in:\n%s", raw, display)
		}
	}
	if !strings.Contains(display, redactedSecret) {
		t.Fatalf("redacted display = %q, want %q", display, redactedSecret)
	}
}

func TestOAuthServerMetadataValidation(t *testing.T) {
	valid := OAuthServerMetadata{
		Issuer:                        "https://mcp.example.com",
		AuthorizationEndpoint:         "https://mcp.example.com/oauth/authorize",
		TokenEndpoint:                 "https://mcp.example.com/oauth/token",
		RegistrationEndpoint:          "https://mcp.example.com/oauth/register",
		ResponseTypesSupported:        []string{"code"},
		CodeChallengeMethodsSupported: []string{"S256"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid metadata Validate() error = %v", err)
	}

	missingEndpoint := valid
	missingEndpoint.AuthorizationEndpoint = ""
	if err := missingEndpoint.Validate(); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("missing auth endpoint Validate() error = %v, want ErrInvalidMetadata", err)
	}

	unsupportedPKCE := valid
	unsupportedPKCE.CodeChallengeMethodsSupported = []string{"plain"}
	if err := unsupportedPKCE.Validate(); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("unsupported PKCE Validate() error = %v, want ErrInvalidMetadata", err)
	}
}

const validSessionVerifier = "verifier-secret"

func validSession(state string) AuthSession {
	return AuthSession{
		State:               state,
		CodeVerifier:        NewSecret(validSessionVerifier),
		CodeChallenge:       CodeChallengeS256(validSessionVerifier),
		CodeChallengeMethod: "S256",
		ClientID:            "client-id",
		RedirectURI:         "http://localhost:49152/callback",
		CreatedAt:           time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC),
		ExpiresAt:           time.Date(2026, 6, 28, 10, 10, 0, 0, time.UTC),
	}
}

func fixedClock(t time.Time) Clock {
	return func() time.Time {
		return t
	}
}

func assertQueryParam(t *testing.T, q url.Values, key, want string) {
	t.Helper()
	if got := q.Get(key); got != want {
		t.Fatalf("query %s = %q, want %q", key, got, want)
	}
}
