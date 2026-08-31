package mcpclient

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

type StoredTokenSource struct {
	CredentialRef string        `json:"credentialRef,omitempty"`
	Token         *oauth2.Token `json:"token,omitempty"`
	ServerURL     string        `json:"serverUrl,omitempty"`
	AuthURL       string        `json:"authUrl,omitempty"`
	TokenURL      string        `json:"tokenUrl,omitempty"`
	RedirectURL   string        `json:"redirectUrl,omitempty"`
	ClientID      string        `json:"clientId,omitempty"`
	ClientSecret  string        `json:"clientSecret,omitempty"`
	Scopes        []string      `json:"scopes,omitempty"`
	NeedsReauth   bool          `json:"needsReauth,omitempty"`
}

var ErrCredentialNotFound = errors.New("credential not found")

// CredentialStore persists OAuth tokens and the metadata required to
// rebuild a token source after a Desktop restart.
type CredentialStore interface {
	LoadTokenSource(credentialIdentity string) (StoredTokenSource, error)
	SaveTokenSource(credentialIdentity string, source StoredTokenSource) error
	ClearTokenSource(credentialIdentity string) error
	HasTokenSource(credentialIdentity string) (bool, error)
}

// InMemoryCredentialStore is the default CredentialStore implementation.
// It is safe for concurrent access.
type InMemoryCredentialStore struct {
	mu      sync.RWMutex
	sources map[string]StoredTokenSource
}

// NewInMemoryCredentialStore returns an empty in-memory store.
func NewInMemoryCredentialStore() *InMemoryCredentialStore {
	return &InMemoryCredentialStore{sources: make(map[string]StoredTokenSource)}
}

func (s *InMemoryCredentialStore) LoadTokenSource(serverName string) (StoredTokenSource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	source, ok := s.sources[serverName]
	if !ok {
		return StoredTokenSource{}, ErrCredentialNotFound
	}
	return source, nil
}

func (s *InMemoryCredentialStore) SaveTokenSource(serverName string, source StoredTokenSource) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sources[serverName] = source
	return nil
}

func (s *InMemoryCredentialStore) ClearTokenSource(serverName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sources, serverName)
	return nil
}

func (s *InMemoryCredentialStore) HasTokenSource(serverName string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.sources[serverName]
	return ok, nil
}

// AuthFlowConfig captures the per-server OAuth parameters the desktop UI
// must supply when triggering the auth flow. The manager uses it to build
// an auth.OAuthHandler that the SDK transport consults on each request.
type AuthFlowConfig struct {
	// ServerURL is the MCP endpoint the auth flow protects.
	ServerURL string
	// RedirectURL is the loopback callback URL the desktop UI registered.
	RedirectURL string
	// ClientID is the OAuth client_id, when pre-registered.
	ClientID string
	// ClientMetadataURL is a public HTTPS Client ID Metadata Document used
	// when the authorization server advertises CIMD support.
	ClientMetadataURL string
	// ClientSecret is the OAuth client_secret, when pre-registered.
	ClientSecret string
	// Scopes requested from the authorization server.
	Scopes []string
	// AuthorizationCodeFetcher is invoked by the SDK handler to drive the
	// user's browser to the authorize URL and await the redirect. The
	// desktop UI supplies an implementation that opens the system browser
	// and serves the loopback callback.
	AuthorizationCodeFetcher auth.AuthorizationCodeFetcher
}

// AuthResult is the wire payload returned by POST /v1/mcp/servers/{name}/auth.
type AuthResult struct {
	AuthorizationURL string     `json:"authorizationUrl"`
	State            string     `json:"state,omitempty"`
	ServerName       string     `json:"serverName"`
	SessionID        string     `json:"sessionId"`
	ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
}

// AuthRequest is the wire payload accepted by POST /v1/mcp/servers/{name}/auth.
type AuthRequest struct {
	RedirectURL       string   `json:"redirectUrl,omitempty"`
	ClientID          string   `json:"clientId,omitempty"`
	ClientMetadataURL string   `json:"clientMetadataUrl,omitempty"`
	ClientSecret      string   `json:"clientSecret,omitempty"`
	Scopes            []string `json:"scopes,omitempty"`
}

// StartAuth initiates the OAuth flow for the named server. It returns the
// authorization URL the desktop UI should open in a browser. The resulting
// token source is persisted via the configured CredentialStore so Desktop
// restarts can rebuild the authenticated transport.
func (m *Manager) StartAuth(ctx context.Context, serverName string, req AuthRequest) (AuthResult, error) {
	_ = ctx
	return m.startCoreAuthSession(serverName, req)
}

// serverAuthURL extracts the URL the OAuth flow should protect from the
// server config. For stdio and in-proc transports there is none; the
// caller is expected to not invoke StartAuth on those.
func serverAuthURL(cfg McpServerConfig) string {
	if resource := strings.TrimSpace(cfg.OAuthResource); resource != "" {
		return resource
	}
	if cfg.HTTP != nil {
		return cfg.HTTP.URL
	}
	if cfg.SSE != nil {
		return cfg.SSE.URL
	}
	if cfg.WS != nil {
		return cfg.WS.URL
	}
	return ""
}

// CredentialIdentity binds an OAuth account to the effective config owner and
// endpoint. The full endpoint hash avoids leaking URLs into keyring account
// names while keeping the identity deterministic across restarts.
func CredentialIdentity(scope Scope, serverName string, cfg McpServerConfig) string {
	endpoint := canonicalServerURL(serverAuthURL(cfg))
	material := strings.Join([]string{
		endpoint,
		strings.TrimSpace(cfg.OAuthClientID),
		strings.TrimSpace(cfg.OAuthClientMetadataURL),
		strings.Join(cfg.OAuthScopes, " "),
	}, "\x00")
	sum := sha256.Sum256([]byte(material))
	return fmt.Sprintf("%s:%s:%x", scope, serverName, sum[:])
}

func canonicalServerURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return trimmed
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String()
}

// buildOAuthHandler constructs an SDK AuthorizationCodeHandler from the
// Awaken AuthFlowConfig. Preregistration is used when ClientID is set;
// otherwise dynamic client registration is attempted.
func buildOAuthHandler(cfg AuthFlowConfig) (*auth.AuthorizationCodeHandler, error) {
	if cfg.AuthorizationCodeFetcher == nil {
		return nil, fmt.Errorf("authorization code fetcher is required")
	}
	if cfg.RedirectURL == "" {
		return nil, fmt.Errorf("redirect url is required")
	}
	handlerCfg := &auth.AuthorizationCodeHandlerConfig{
		RedirectURL:              cfg.RedirectURL,
		AuthorizationCodeFetcher: cfg.AuthorizationCodeFetcher,
		Client:                   oauthSafeHTTPClient(),
	}
	if cfg.ClientMetadataURL != "" {
		handlerCfg.ClientIDMetadataDocumentConfig = &auth.ClientIDMetadataDocumentConfig{URL: cfg.ClientMetadataURL}
	} else if cfg.ClientID != "" {
		creds := &oauthex.ClientCredentials{ClientID: cfg.ClientID}
		if cfg.ClientSecret != "" {
			creds.ClientSecretAuth = &oauthex.ClientSecretAuth{ClientSecret: cfg.ClientSecret}
		}
		handlerCfg.PreregisteredClient = creds
	} else {
		handlerCfg.DynamicClientRegistrationConfig = &auth.DynamicClientRegistrationConfig{
			Metadata: &oauthex.ClientRegistrationMetadata{
				ClientName:              "Awaken",
				RedirectURIs:            []string{cfg.RedirectURL},
				TokenEndpointAuthMethod: "none",
				GrantTypes:              []string{"authorization_code", "refresh_token"},
				ResponseTypes:           []string{"code"},
				Scope:                   strings.Join(cfg.Scopes, " "),
			},
		}
	}
	return auth.NewAuthorizationCodeHandler(handlerCfg)
}

// serverAuthHandler returns the installed OAuthHandler for the server, or
// nil if none has been installed. Called by the transport factory when
// building an HTTP/SSE transport so the SDK can attach the Authorization
// header automatically.
func (m *Manager) serverAuthHandler(serverName string) auth.OAuthHandler {
	m.mu.RLock()
	defer m.mu.RUnlock()
	server, ok := m.servers[serverName]
	if !ok {
		return nil
	}
	return server.authHandler
}

// Ensure auth.OAuthHandler is referenced so the import stays used in the
// case the manager's transport.go is the only consumer.
var _ auth.OAuthHandler = (*auth.AuthorizationCodeHandler)(nil)
