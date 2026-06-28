package mcpoauth

import (
	"net/url"
	"strings"
	"time"
)

type AuthorizationURLOptions struct {
	Metadata      OAuthServerMetadata
	Client        Client
	RedirectURI   string
	Scope         string
	State         string
	CodeChallenge string
	Resource      string
	Audience      string
	ExtraParams   url.Values
}

func BuildAuthorizationURL(opts AuthorizationURLOptions) (string, error) {
	if err := opts.Metadata.Validate(); err != nil {
		return "", err
	}
	client := opts.Client
	if opts.RedirectURI != "" {
		client.RedirectURI = opts.RedirectURI
	}
	if opts.Scope != "" {
		client.Scope = opts.Scope
	}
	if err := client.Validate(); err != nil {
		return "", err
	}
	if strings.TrimSpace(opts.State) == "" {
		return "", wrapInvalid(ErrInvalidAuthSession, "state is required")
	}
	if strings.TrimSpace(opts.CodeChallenge) == "" {
		return "", wrapInvalid(ErrInvalidAuthSession, "code_challenge is required")
	}

	authURL, err := url.Parse(opts.Metadata.AuthorizationEndpoint)
	if err != nil {
		return "", err
	}
	q := authURL.Query()
	q.Set("client_id", client.ClientID)
	q.Set("redirect_uri", client.RedirectURI)
	q.Set("response_type", "code")
	if client.Scope != "" {
		q.Set("scope", client.Scope)
	}
	q.Set("state", opts.State)
	q.Set("code_challenge", opts.CodeChallenge)
	q.Set("code_challenge_method", "S256")
	if opts.Resource != "" {
		q.Set("resource", opts.Resource)
	}
	if opts.Audience != "" {
		q.Set("audience", opts.Audience)
	}
	for key, values := range opts.ExtraParams {
		if isReservedAuthorizationParam(key) {
			continue
		}
		for _, value := range values {
			q.Add(key, value)
		}
	}
	authURL.RawQuery = q.Encode()
	return authURL.String(), nil
}

type AuthSessionOptions struct {
	Metadata    OAuthServerMetadata
	Client      Client
	RedirectURI string
	Scope       string
	Resource    string
	Audience    string
	TTL         time.Duration
	Clock       Clock
}

func NewAuthSession(opts AuthSessionOptions) (AuthSession, error) {
	if err := opts.Metadata.Validate(); err != nil {
		return AuthSession{}, err
	}
	client := opts.Client
	if opts.RedirectURI != "" {
		client.RedirectURI = opts.RedirectURI
	}
	if opts.Scope != "" {
		client.Scope = opts.Scope
	}
	if err := client.Validate(); err != nil {
		return AuthSession{}, err
	}
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		return AuthSession{}, err
	}
	state, err := GenerateState()
	if err != nil {
		return AuthSession{}, err
	}
	now := opts.Clock.Now()
	session := AuthSession{
		State:               state,
		CodeVerifier:        NewSecret(verifier),
		CodeChallenge:       CodeChallengeS256(verifier),
		CodeChallengeMethod: "S256",
		ClientID:            client.ClientID,
		RedirectURI:         client.RedirectURI,
		Scope:               client.Scope,
		Resource:            opts.Resource,
		Audience:            opts.Audience,
		CreatedAt:           now,
	}
	if opts.TTL > 0 {
		session.ExpiresAt = now.Add(opts.TTL)
	}
	authURL, err := BuildAuthorizationURL(AuthorizationURLOptions{
		Metadata:      opts.Metadata,
		Client:        client,
		State:         session.State,
		CodeChallenge: session.CodeChallenge,
		Resource:      session.Resource,
		Audience:      session.Audience,
	})
	if err != nil {
		return AuthSession{}, err
	}
	session.AuthorizationURL = authURL
	return session, nil
}

func isReservedAuthorizationParam(key string) bool {
	switch key {
	case "client_id", "redirect_uri", "response_type", "scope", "state", "code_challenge", "code_challenge_method", "resource", "audience":
		return true
	default:
		return false
	}
}
