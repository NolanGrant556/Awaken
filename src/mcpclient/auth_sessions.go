package mcpclient

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

const oauthSessionTTL = 10 * time.Minute

type oauthAuthSession struct {
	mu            sync.RWMutex
	id            string
	serverName    string
	status        string
	authorizeURL  string
	expectedState string
	errorMessage  string
	expiresAt     time.Time
	result        chan *sdkauth.AuthorizationResult
	providerError chan error
	ready         chan struct{}
	readyOnce     sync.Once
	cancel        context.CancelFunc
	listener      net.Listener
	httpServer    *http.Server
}

type AuthSessionView struct {
	SessionID    string     `json:"sessionId"`
	ServerName   string     `json:"serverName"`
	Status       string     `json:"status"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
}

func (s *oauthAuthSession) view() AuthSessionView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	expires := s.expiresAt
	return AuthSessionView{SessionID: s.id, ServerName: s.serverName, Status: s.status, ErrorMessage: s.errorMessage, ExpiresAt: &expires}
}

func (s *oauthAuthSession) publishReady() {
	s.readyOnce.Do(func() { close(s.ready) })
}

func (s *oauthAuthSession) setAuthorizationURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return errors.New("OAuth provider returned an invalid authorization URL")
	}
	validationCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := validateOAuthEndpoint(validationCtx, parsed); err != nil {
		return fmt.Errorf("OAuth provider returned an unsafe authorization URL: %w", err)
	}
	for key := range parsed.Query() {
		switch strings.ToLower(key) {
		case "access_token", "token", "client_secret", "password", "api_key":
			return errors.New("OAuth authorization URL contained a credential query parameter")
		}
	}
	state := strings.TrimSpace(parsed.Query().Get("state"))
	if state == "" {
		return errors.New("OAuth authorization URL did not include state")
	}
	s.mu.Lock()
	s.authorizeURL = parsed.String()
	s.expectedState = state
	s.status = "authorizing"
	s.mu.Unlock()
	s.publishReady()
	return nil
}

func (s *oauthAuthSession) finish(status string, err error) {
	s.mu.Lock()
	s.status = status
	if err != nil {
		s.errorMessage = err.Error()
	}
	s.mu.Unlock()
	s.publishReady()
}

func (m *Manager) startCoreAuthSession(serverName string, req AuthRequest) (AuthResult, error) {
	lifetime, finish, err := m.beginOperation(context.Background())
	if err != nil {
		return AuthResult{}, err
	}
	handedOff := false
	defer func() {
		if !handedOff {
			finish()
		}
	}()

	m.mu.RLock()
	server, ok := m.servers[serverName]
	if !ok {
		m.mu.RUnlock()
		return AuthResult{}, fmt.Errorf("mcp server %q not found", serverName)
	}
	scoped := ScopedMcpServerConfig{Name: serverName, Scope: server.scope, SourceRevision: server.sourceRevision, Config: server.config}
	serverURL := serverAuthURL(server.config)
	serverConfig := server.config
	m.mu.RUnlock()
	if strings.TrimSpace(serverURL) == "" {
		return AuthResult{}, fmt.Errorf("mcp server %q does not use a remote OAuth-capable transport", serverName)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return AuthResult{}, fmt.Errorf("start OAuth callback listener: %w", err)
	}
	ctx, cancel := context.WithTimeout(lifetime, oauthSessionTTL)
	session := &oauthAuthSession{
		id: uuid.NewString(), serverName: serverName, status: "authorizing",
		expiresAt: time.Now().UTC().Add(oauthSessionTTL), result: make(chan *sdkauth.AuthorizationResult, 1),
		ready: make(chan struct{}), cancel: cancel, listener: listener, providerError: make(chan error, 1),
	}
	callbackURL := "http://" + listener.Addr().String() + "/callback"
	serverMux := http.NewServeMux()
	serverMux.HandleFunc("GET /callback", session.handleCallback)
	session.httpServer = &http.Server{Handler: serverMux, ReadHeaderTimeout: 5 * time.Second}
	m.authSessionMu.Lock()
	for _, existing := range m.authSessions {
		if existing.serverName == serverName {
			existing.cancel()
		}
	}
	m.authSessions[session.id] = session
	m.authSessionMu.Unlock()
	go func() {
		_ = session.httpServer.Serve(listener)
	}()

	fetcher := func(fetchCtx context.Context, args *sdkauth.AuthorizationArgs) (*sdkauth.AuthorizationResult, error) {
		if err := session.setAuthorizationURL(args.URL); err != nil {
			session.finish("failed", err)
			return nil, err
		}
		select {
		case result := <-session.result:
			return result, nil
		case providerErr := <-session.providerError:
			return nil, providerErr
		case <-fetchCtx.Done():
			return nil, fetchCtx.Err()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	cfg := AuthFlowConfig{
		ServerURL: serverURL, RedirectURL: callbackURL,
		ClientID:          firstNonEmpty(strings.TrimSpace(req.ClientID), strings.TrimSpace(serverConfig.OAuthClientID)),
		ClientMetadataURL: firstNonEmpty(strings.TrimSpace(req.ClientMetadataURL), strings.TrimSpace(serverConfig.OAuthClientMetadataURL)),
		ClientSecret:      req.ClientSecret,
		Scopes:            append([]string(nil), req.Scopes...), AuthorizationCodeFetcher: fetcher,
	}
	if len(cfg.Scopes) == 0 && len(serverConfig.OAuthScopes) > 0 {
		cfg.Scopes = append([]string(nil), serverConfig.OAuthScopes...)
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = discoverMCPRegistrationScopes(ctx, serverURL)
	}
	handler := newPersistentOAuthHandler(CredentialIdentity(scoped.Scope, scoped.Name, scoped.Config), cfg, m.credentials)
	m.mu.Lock()
	if live := m.servers[serverName]; live == server && !m.closing {
		live.connectionGeneration++
		live.authHandler = handler
	}
	m.mu.Unlock()

	handedOff = true
	go func() { defer finish(); m.runCoreAuthSession(ctx, session, handler, cfg, scoped) }()
	select {
	case <-session.ready:
		view := session.view()
		session.mu.RLock()
		authorizationURL := session.authorizeURL
		session.mu.RUnlock()
		if authorizationURL == "" {
			cancel()
			_ = session.httpServer.Close()
			return AuthResult{}, fmt.Errorf("start OAuth session: %s", view.ErrorMessage)
		}
		return AuthResult{AuthorizationURL: authorizationURL, State: view.Status, ServerName: serverName, SessionID: session.id, ExpiresAt: view.ExpiresAt}, nil
	case <-ctx.Done():
		cancel()
		_ = session.httpServer.Close()
		return AuthResult{}, ctx.Err()
	case <-time.After(15 * time.Second):
		cancel()
		_ = session.httpServer.Close()
		return AuthResult{}, errors.New("OAuth discovery timed out before an authorization URL was available")
	}
}

func discoverMCPRegistrationScopes(ctx context.Context, serverURL string) []string {
	parsed, err := url.Parse(serverURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil
	}
	metadataURL := *parsed
	metadataURL.RawQuery = ""
	metadataURL.Fragment = ""
	metadataURL.Path = "/.well-known/oauth-protected-resource/" + strings.TrimLeft(parsed.Path, "/")
	discoveryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	metadata, err := oauthex.GetProtectedResourceMetadata(discoveryCtx, metadataURL.String(), serverURL, oauthSafeHTTPClient())
	if err != nil || metadata == nil {
		return nil
	}
	return append([]string(nil), metadata.ScopesSupported...)
}

func (m *Manager) runCoreAuthSession(ctx context.Context, session *oauthAuthSession, handler *persistentOAuthHandler, cfg AuthFlowConfig, scoped ScopedMcpServerConfig) {
	defer func() {
		_ = session.httpServer.Close()
		session.cancel()
		time.AfterFunc(15*time.Minute, func() {
			m.authSessionMu.Lock()
			delete(m.authSessions, session.id)
			m.authSessionMu.Unlock()
		})
	}()
	fakeReq, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.ServerURL, nil)
	if err != nil {
		session.finish("failed", err)
		return
	}
	fakeResp := &http.Response{StatusCode: http.StatusUnauthorized, Header: make(http.Header), Body: http.NoBody, Request: fakeReq}
	if err := handler.Authorize(ctx, fakeReq, fakeResp); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			session.finish("expired", errors.New("OAuth authorization expired"))
		} else if errors.Is(err, context.Canceled) {
			session.finish("cancelled", errors.New("OAuth authorization cancelled"))
		} else {
			session.finish("failed", err)
		}
		return
	}
	session.finish("verifying", nil)
	if err := m.RestartWithConfig(context.Background(), scoped); err != nil {
		session.finish("failed", fmt.Errorf("restart authenticated MCP server: %w", err))
		return
	}
	session.finish("ready", nil)
}

func (s *oauthAuthSession) handleCallback(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	expectedState := s.expectedState
	status := s.status
	s.mu.RUnlock()
	if status != "authorizing" || expectedState == "" {
		http.Error(w, "OAuth session is not waiting for a callback", http.StatusConflict)
		return
	}
	query := r.URL.Query()
	if providerError := strings.TrimSpace(query.Get("error")); providerError != "" {
		err := fmt.Errorf("OAuth provider returned %s", providerError)
		s.finish("failed", err)
		select {
		case s.providerError <- err:
		default:
		}
		http.Error(w, "Authorization failed. You can close this window.", http.StatusBadRequest)
		return
	}
	state := strings.TrimSpace(query.Get("state"))
	code := strings.TrimSpace(query.Get("code"))
	if state == "" || state != expectedState {
		http.Error(w, "OAuth state did not match", http.StatusBadRequest)
		return
	}
	if code == "" {
		http.Error(w, "OAuth callback did not include a code", http.StatusBadRequest)
		return
	}
	select {
	case s.result <- &sdkauth.AuthorizationResult{Code: code, State: state}:
		s.mu.Lock()
		s.status = "verifying"
		s.mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, "<!doctype html><meta charset=utf-8><title>Awaken</title><p>%s</p>", html.EscapeString("Authorization received. You can return to Awaken."))
	default:
		http.Error(w, "OAuth callback was already consumed", http.StatusConflict)
	}
}

func (m *Manager) AuthSession(sessionID string) (AuthSessionView, error) {
	m.authSessionMu.RLock()
	session := m.authSessions[strings.TrimSpace(sessionID)]
	m.authSessionMu.RUnlock()
	if session == nil {
		return AuthSessionView{}, errors.New("OAuth session not found")
	}
	return session.view(), nil
}

func (m *Manager) CancelAuthSession(sessionID string) (AuthSessionView, error) {
	m.authSessionMu.RLock()
	session := m.authSessions[strings.TrimSpace(sessionID)]
	m.authSessionMu.RUnlock()
	if session == nil {
		return AuthSessionView{}, errors.New("OAuth session not found")
	}
	session.cancel()
	_ = session.httpServer.Close()
	session.finish("cancelled", errors.New("OAuth authorization cancelled"))
	return session.view(), nil
}

func (m *Manager) ClearServerCredentials(serverName string) error {
	m.mu.Lock()
	server, ok := m.servers[serverName]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("mcp server %q not found", serverName)
	}
	identity := CredentialIdentity(server.scope, serverName, server.config)
	server.authHandler = nil
	m.mu.Unlock()
	return m.credentials.ClearTokenSource(identity)
}

func (m *Manager) HasServerCredentials(serverName string) (bool, error) {
	m.mu.RLock()
	server, ok := m.servers[serverName]
	if !ok {
		m.mu.RUnlock()
		return false, fmt.Errorf("mcp server %q not found", serverName)
	}
	identity := CredentialIdentity(server.scope, serverName, server.config)
	m.mu.RUnlock()
	return m.credentials.HasTokenSource(identity)
}

func (m *Manager) ServerCredentialIdentity(serverName string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	server, ok := m.servers[serverName]
	if !ok {
		return "", fmt.Errorf("mcp server %q not found", serverName)
	}
	return CredentialIdentity(server.scope, serverName, server.config), nil
}

func (m *Manager) ActiveAuthSessionForServer(serverName string) (AuthSessionView, bool) {
	m.authSessionMu.RLock()
	defer m.authSessionMu.RUnlock()
	for _, session := range m.authSessions {
		view := session.view()
		if view.ServerName == serverName && (view.Status == "authorizing" || view.Status == "verifying") {
			return view, true
		}
	}
	return AuthSessionView{}, false
}
