package mcpclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
	LoadTokenSource(serverName string) (StoredTokenSource, error)
	SaveTokenSource(serverName string, source StoredTokenSource) error
	ClearTokenSource(serverName string) error
	HasTokenSource(serverName string) (bool, error)
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
	ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
}

// AuthRequest is the wire payload accepted by POST /v1/mcp/servers/{name}/auth.
type AuthRequest struct {
	RedirectURL  string   `json:"redirectUrl"`
	ClientID     string   `json:"clientId,omitempty"`
	ClientSecret string   `json:"clientSecret,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	// AuthorizationCodeFetcher cannot be serialized; it is supplied out of
	// band by the desktop UI via Manager.InstallAuthFetcher.
}

// StartAuth initiates the OAuth flow for the named server. It returns the
// authorization URL the desktop UI should open in a browser. The resulting
// token source is persisted via the configured CredentialStore so Desktop
// restarts can rebuild the authenticated transport.
//
// In v1, StartAuth requires that InstallAuthFetcher has been called for
// the server; otherwise it returns an error directing the caller to
// register a fetcher.
func (m *Manager) StartAuth(ctx context.Context, serverName string, req AuthRequest) (AuthResult, error) {
	m.mu.Lock()
	server, ok := m.servers[serverName]
	m.mu.Unlock()
	if !ok {
		return AuthResult{}, fmt.Errorf("mcp server %q not found", serverName)
	}
	fetcher := m.lookupAuthFetcher(serverName)
	if fetcher == nil {
		return AuthResult{}, fmt.Errorf("%w: server %q; install one via Manager.InstallAuthFetcher", ErrAuthFetcherNotInstalled, serverName)
	}
	cfg := AuthFlowConfig{
		ServerURL:                serverAuthURL(server.config),
		RedirectURL:              req.RedirectURL,
		ClientID:                 req.ClientID,
		ClientSecret:             req.ClientSecret,
		Scopes:                   req.Scopes,
		AuthorizationCodeFetcher: fetcher,
	}
	captureHandler, err := buildOAuthHandler(cfg)
	if err != nil {
		return AuthResult{}, fmt.Errorf("build oauth handler: %w", err)
	}
	// Trigger an Authorize round-trip so we can capture the authorize URL.
	// The SDK's Authorize is expected to invoke the fetcher, which itself
	// returns the URL plus the eventual code. We expose the URL via the
	// fetcher's first call.
	authURL, err := captureAuthorizeURL(ctx, captureHandler, cfg)
	if err != nil {
		return AuthResult{}, err
	}
	handler := newPersistentOAuthHandler(serverName, cfg, m.credentials)
	// Install the handler on the server so the next Connect/CallTool picks
	// it up. We do not block on the flow here; the desktop UI drives it.
	m.mu.Lock()
	server.authHandler = handler
	m.mu.Unlock()
	return AuthResult{
		AuthorizationURL: authURL,
		ServerName:       serverName,
	}, nil
}

// InstallAuthFetcher registers the AuthorizationCodeFetcher the desktop UI
// supplies for a server. The fetcher is responsible for opening the
// system browser and serving the loopback redirect.
func (m *Manager) InstallAuthFetcher(serverName string, fetcher auth.AuthorizationCodeFetcher) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.authFetchers == nil {
		m.authFetchers = make(map[string]auth.AuthorizationCodeFetcher)
	}
	m.authFetchers[serverName] = fetcher
}

func (m *Manager) lookupAuthFetcher(serverName string) auth.AuthorizationCodeFetcher {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.authFetchers[serverName]
}

// serverAuthURL extracts the URL the OAuth flow should protect from the
// server config. For stdio and in-proc transports there is none; the
// caller is expected to not invoke StartAuth on those.
func serverAuthURL(cfg McpServerConfig) string {
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
	}
	if cfg.ClientID != "" {
		creds := &oauthex.ClientCredentials{ClientID: cfg.ClientID}
		if cfg.ClientSecret != "" {
			creds.ClientSecretAuth = &oauthex.ClientSecretAuth{ClientSecret: cfg.ClientSecret}
		}
		handlerCfg.PreregisteredClient = creds
	}
	return auth.NewAuthorizationCodeHandler(handlerCfg)
}

// captureAuthorizeURL triggers the SDK handler's Authorize flow via a
// synthetic 401 response so we can capture the authorization URL the
// fetcher is about to open. The fetcher is expected to surface the URL
// through its args parameter.
func captureAuthorizeURL(ctx context.Context, handler *auth.AuthorizationCodeHandler, cfg AuthFlowConfig) (string, error) {
	// The SDK's Authorize expects the failed request and response. We
	// synthesize a 401 against the configured server URL so the handler
	// can introspect WWW-Authenticate challenges. Real challenges are
	// typically present in production; for v1 we accept that the flow may
	// fall back to the configured endpoints.
	fakeReq, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.ServerURL, nil)
	if err != nil {
		return "", fmt.Errorf("build synthetic request: %w", err)
	}
	fakeResp := &http.Response{
		StatusCode: http.StatusUnauthorized,
		Header:     make(http.Header),
		Body:       http.NoBody,
		Request:    fakeReq,
	}
	// Install a capture fetcher that records the URL before delegating to
	// the user-supplied fetcher.
	var capturedURL string
	capture := func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
		capturedURL = args.URL
		// We don't actually complete the flow here; the desktop UI will
		// re-invoke the real fetcher when it renders the auth URL. Return
		// a benign error so the SDK doesn't try to exchange a missing code.
		return nil, errors.New("auth url captured; awaiting desktop-driven completion")
	}
	original := cfg.AuthorizationCodeFetcher
	cfg.AuthorizationCodeFetcher = capture
	// Rebuild handler with the capture fetcher to grab the URL.
	captureCfg := &auth.AuthorizationCodeHandlerConfig{
		RedirectURL:              cfg.RedirectURL,
		AuthorizationCodeFetcher: capture,
	}
	if cfg.ClientID != "" {
		creds := &oauthex.ClientCredentials{ClientID: cfg.ClientID}
		if cfg.ClientSecret != "" {
			creds.ClientSecretAuth = &oauthex.ClientSecretAuth{ClientSecret: cfg.ClientSecret}
		}
		captureCfg.PreregisteredClient = creds
	}
	captureHandler, err := auth.NewAuthorizationCodeHandler(captureCfg)
	if err != nil {
		return "", fmt.Errorf("build capture handler: %w", err)
	}
	_ = captureHandler.Authorize(ctx, fakeReq, fakeResp)
	cfg.AuthorizationCodeFetcher = original
	if capturedURL == "" {
		return "", errors.New("oauth handler did not surface an authorization url")
	}
	return capturedURL, nil
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

// ErrAuthFetcherNotInstalled is returned by StartAuth when no
// AuthorizationCodeFetcher has been installed for the server.
var ErrAuthFetcherNotInstalled = errors.New("auth fetcher not installed")
