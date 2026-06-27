package mcpoauth

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type OAuthServerMetadata struct {
	Issuer                        string   `json:"issuer,omitempty"`
	AuthorizationEndpoint         string   `json:"authorization_endpoint,omitempty"`
	TokenEndpoint                 string   `json:"token_endpoint,omitempty"`
	RegistrationEndpoint          string   `json:"registration_endpoint,omitempty"`
	ScopesSupported               []string `json:"scopes_supported,omitempty"`
	ResponseTypesSupported        []string `json:"response_types_supported,omitempty"`
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported,omitempty"`
	ResourceDocumentation         string   `json:"resource_documentation,omitempty"`
}

func (m OAuthServerMetadata) Validate() error {
	if err := validateHTTPURL(m.AuthorizationEndpoint, "authorization_endpoint", true, ErrInvalidMetadata); err != nil {
		return err
	}
	if err := validateHTTPURL(m.TokenEndpoint, "token_endpoint", true, ErrInvalidMetadata); err != nil {
		return err
	}
	if err := validateHTTPURL(m.Issuer, "issuer", false, ErrInvalidMetadata); err != nil {
		return err
	}
	if err := validateHTTPURL(m.RegistrationEndpoint, "registration_endpoint", false, ErrInvalidMetadata); err != nil {
		return err
	}
	if len(m.ResponseTypesSupported) > 0 && !containsString(m.ResponseTypesSupported, "code") {
		return wrapInvalid(ErrInvalidMetadata, "response_types_supported must include code")
	}
	if len(m.CodeChallengeMethodsSupported) > 0 && !containsString(m.CodeChallengeMethodsSupported, "S256") {
		return wrapInvalid(ErrInvalidMetadata, "code_challenge_methods_supported must include S256")
	}
	return nil
}

type ClientRegistration struct {
	ClientName              string   `json:"client_name,omitempty"`
	RedirectURIs            []string `json:"redirect_uris,omitempty"`
	GrantTypes              []string `json:"grant_types,omitempty"`
	ResponseTypes           []string `json:"response_types,omitempty"`
	Scope                   string   `json:"scope,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
}

type Client struct {
	ClientID                string             `json:"client_id,omitempty"`
	ClientSecret            Secret             `json:"client_secret,omitempty"`
	RedirectURI             string             `json:"redirect_uri,omitempty"`
	Scope                   string             `json:"scope,omitempty"`
	TokenEndpointAuthMethod string             `json:"token_endpoint_auth_method,omitempty"`
	Registration            ClientRegistration `json:"registration,omitempty"`
}

func (c Client) Validate() error {
	if strings.TrimSpace(c.ClientID) == "" {
		return wrapInvalid(ErrInvalidClient, "client_id is required")
	}
	if err := validateHTTPURL(c.RedirectURI, "redirect_uri", true, ErrInvalidClient); err != nil {
		return err
	}
	return nil
}

func (c Client) String() string {
	return fmt.Sprintf("Client{ClientID:%q ClientSecret:%s RedirectURI:%q Scope:%q TokenEndpointAuthMethod:%q}",
		c.ClientID, c.ClientSecret.Redacted(), c.RedirectURI, c.Scope, c.TokenEndpointAuthMethod)
}

func (c Client) GoString() string {
	return c.String()
}

type AuthSession struct {
	State               string    `json:"state,omitempty"`
	CodeVerifier        Secret    `json:"code_verifier,omitempty"`
	CodeChallenge       string    `json:"code_challenge,omitempty"`
	CodeChallengeMethod string    `json:"code_challenge_method,omitempty"`
	ClientID            string    `json:"client_id,omitempty"`
	RedirectURI         string    `json:"redirect_uri,omitempty"`
	Scope               string    `json:"scope,omitempty"`
	Resource            string    `json:"resource,omitempty"`
	Audience            string    `json:"audience,omitempty"`
	AuthorizationURL    string    `json:"authorization_url,omitempty"`
	CreatedAt           time.Time `json:"created_at,omitempty"`
	ExpiresAt           time.Time `json:"expires_at,omitempty"`
}

func (s AuthSession) Validate() error {
	if strings.TrimSpace(s.State) == "" {
		return wrapInvalid(ErrInvalidAuthSession, "state is required")
	}
	if s.CodeVerifier.IsZero() {
		return wrapInvalid(ErrInvalidAuthSession, "code_verifier is required")
	}
	if strings.TrimSpace(s.CodeChallenge) == "" {
		return wrapInvalid(ErrInvalidAuthSession, "code_challenge is required")
	}
	if s.CodeChallengeMethod != "" && s.CodeChallengeMethod != "S256" {
		return wrapInvalid(ErrInvalidAuthSession, "code_challenge_method must be S256")
	}
	if strings.TrimSpace(s.ClientID) == "" {
		return wrapInvalid(ErrInvalidAuthSession, "client_id is required")
	}
	if err := validateHTTPURL(s.RedirectURI, "redirect_uri", true, ErrInvalidAuthSession); err != nil {
		return err
	}
	return nil
}

func (s AuthSession) String() string {
	return fmt.Sprintf("AuthSession{State:%q CodeVerifier:%s CodeChallenge:%q CodeChallengeMethod:%q ClientID:%q RedirectURI:%q Scope:%q Resource:%q Audience:%q AuthorizationURL:%q CreatedAt:%s ExpiresAt:%s}",
		s.State,
		s.CodeVerifier.Redacted(),
		s.CodeChallenge,
		firstNonEmpty(s.CodeChallengeMethod, "S256"),
		s.ClientID,
		s.RedirectURI,
		s.Scope,
		s.Resource,
		s.Audience,
		s.AuthorizationURL,
		s.CreatedAt.Format(time.RFC3339),
		s.ExpiresAt.Format(time.RFC3339),
	)
}

func (s AuthSession) GoString() string {
	return s.String()
}

type TokenSet struct {
	AccessToken  Secret    `json:"access_token,omitempty"`
	RefreshToken Secret    `json:"refresh_token,omitempty"`
	IDToken      Secret    `json:"id_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	IssuedAt     time.Time `json:"issued_at,omitempty"`
}

func (t TokenSet) String() string {
	return fmt.Sprintf("TokenSet{AccessToken:%s RefreshToken:%s IDToken:%s TokenType:%q Scope:%q ExpiresAt:%s IssuedAt:%s}",
		t.AccessToken.Redacted(),
		t.RefreshToken.Redacted(),
		t.IDToken.Redacted(),
		t.TokenType,
		t.Scope,
		t.ExpiresAt.Format(time.RFC3339),
		t.IssuedAt.Format(time.RFC3339),
	)
}

func (t TokenSet) GoString() string {
	return t.String()
}

type TokenStore interface {
	LoadTokens(ctx context.Context, key string) (TokenSet, bool, error)
	SaveTokens(ctx context.Context, key string, tokens TokenSet) error
	DeleteTokens(ctx context.Context, key string) error
}

type Clock func() time.Time

func (c Clock) Now() time.Time {
	if c == nil {
		return time.Now().UTC()
	}
	return c().UTC()
}

func validateHTTPURL(raw, field string, required bool, base error) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if required {
			return wrapInvalid(base, field+" is required")
		}
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return wrapInvalid(base, field+" must be an absolute URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return wrapInvalid(base, field+" must use http or https")
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
