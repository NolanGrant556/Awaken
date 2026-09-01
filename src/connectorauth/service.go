// Package connectorauth owns accounts for host-provided Connectors. Unlike
// plugin auth, these accounts are keyed by Connector identity and can be
// referenced by multiple plugins without exposing credentials to any plugin.
package connectorauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"awaken/src/authapproval"
	"awaken/src/connectors"
	"awaken/src/mcpclient"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

const sessionTTL = 10 * time.Minute
const accountReadinessCacheTTL = 30 * time.Second

var (
	ErrUnsupported        = errors.New("connector authorization is unsupported")
	ErrClientIDRequired   = errors.New("connector public client ID is not configured")
	ErrSessionNotFound    = errors.New("connector authorization session not found")
	ErrNeedsAuth          = errors.New("connector account requires authorization")
	ErrCredentialChanged  = errors.New("connector authorization contract changed")
	ErrRefreshUnavailable = errors.New("connector account refresh is temporarily unavailable")
)

type registry interface {
	Definition(connectorID string) (connectors.ConnectorDefinition, error)
	Status(connectorID string) (connectors.ConnectorStatus, error)
	RuntimeStatus(connectorID string) (connectors.ConnectorStatus, error)
	Connect(ctx context.Context, connectorID string) error
	Disconnect(ctx context.Context, connectorID string) error
}

type session struct {
	mu               sync.RWMutex
	id               string
	connectorID      string
	fingerprint      string
	status           string
	errorText        string
	expiresAt        time.Time
	state            string
	verifier         string
	callbackConsumed bool
	cancel           context.CancelFunc
	listener         net.Listener
	server           *http.Server
}

func (s *session) view() connectors.ConnectorAuthSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	expires := s.expiresAt
	return connectors.ConnectorAuthSession{
		SessionID: s.id, ConnectorID: s.connectorID, Status: s.status,
		ErrorMessage: s.errorText, ExpiresAt: &expires,
	}
}

func (s *session) finish(status, safeError string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status != "authorizing" && s.status != "verifying" {
		return false
	}
	s.status = status
	s.errorText = strings.TrimSpace(safeError)
	return true
}

func (s *session) markAdminApprovalPending() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status != "authorizing" && s.status != "verifying" {
		return false
	}
	s.status = "pending_admin_approval"
	s.errorText = "等待组织管理员批准；批准后请重新连接。"
	return true
}

func (s *session) cancelSession() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status == "ready" || s.status == "failed" || s.status == "expired" || s.status == "cancelled" {
		return false
	}
	s.status = "cancelled"
	s.errorText = "授权已取消。"
	return true
}

type Service struct {
	registry    registry
	credentials mcpclient.CredentialStore
	httpClient  *http.Client
	approvals   *authapproval.Store

	mu             sync.RWMutex
	sessions       map[string]*session
	readinessCache map[string]cachedAccountReadiness
}

type cachedAccountReadiness struct {
	fingerprint string
	value       connectors.AccountReadiness
	expiresAt   time.Time
}

func NewService(registry registry, credentials mcpclient.CredentialStore) *Service {
	if credentials == nil {
		credentials = mcpclient.NewPersistentCredentialStore()
	}
	return &Service{
		registry: registry, credentials: credentials, httpClient: newSafeHTTPClient(),
		approvals: authapproval.NewStore("connector-approvals.json"),
		sessions:  make(map[string]*session), readinessCache: make(map[string]cachedAccountReadiness),
	}
}

func (s *Service) SetHTTPClientForTest(client *http.Client) {
	if client != nil {
		s.httpClient = client
	}
}

func (s *Service) AccountReadiness(definition connectors.ConnectorDefinition) connectors.AccountReadiness {
	if definition.OAuth == nil || definition.AuthType != connectors.AuthOAuth2 {
		return connectors.AccountReadiness{State: connectors.StateDisconnected}
	}
	if strings.TrimSpace(definition.OAuth.ClientID) == "" {
		return connectors.AccountReadiness{
			State:     connectors.StateFailed,
			LastError: "Awaken 的连接器登录配置尚未发布，当前不能发起账号授权。",
		}
	}
	if active := s.activeSession(definition.ID, connectorFingerprint(definition)); active != nil {
		view := active.view()
		switch view.Status {
		case "authorizing", "verifying":
			return connectors.AccountReadiness{State: connectors.StatePending}
		case "failed", "expired":
			return connectors.AccountReadiness{State: connectors.StateFailed, LastError: view.ErrorMessage}
		}
	}
	approvalPending, approvalErr := s.approvals.Matches(definition.ID, connectorFingerprint(definition))
	if approvalErr != nil {
		return connectors.AccountReadiness{State: connectors.StateFailed, LastError: "无法读取管理员审批状态。"}
	}
	if approvalPending {
		return connectors.AccountReadiness{
			State: connectors.StateApprovalRequired, LastError: "等待组织管理员批准；批准后请重新连接。",
		}
	}
	fingerprint := connectorFingerprint(definition)
	if cached, ok := s.cachedAccountReadiness(definition.ID, fingerprint); ok {
		return cached
	}
	readiness := connectors.AccountReadiness{State: connectors.StateNeedsAuth}
	stored, err := s.credentials.LoadTokenSource(connectorCredentialIdentity(definition.ID))
	if err != nil {
		if errors.Is(err, mcpclient.ErrCredentialNotFound) {
			s.cacheAccountReadiness(definition.ID, fingerprint, readiness)
			return readiness
		}
		readiness = connectors.AccountReadiness{State: connectors.StateFailed, LastError: "无法读取本机账号凭据状态。"}
		s.cacheAccountReadiness(definition.ID, fingerprint, readiness)
		return readiness
	}
	if !storedMatchesDefinition(stored, definition) || stored.Token == nil {
		s.cacheAccountReadiness(definition.ID, fingerprint, readiness)
		return readiness
	}
	if stored.NeedsReauth {
		if stored.ReauthReason == "provider_rejected" || strings.TrimSpace(stored.Token.RefreshToken) == "" {
			s.cacheAccountReadiness(definition.ID, fingerprint, readiness)
			return readiness
		}
		// access_token_rejected and legacy empty-reason records still hold a
		// refresh token. Keep the component usable so AccessToken can force a
		// safe refresh instead of requiring another interactive login.
		readiness = connectors.AccountReadiness{State: connectors.StateConnected}
		s.cacheAccountReadiness(definition.ID, fingerprint, readiness)
		return readiness
	}
	if !stored.Token.Expiry.IsZero() && time.Now().After(stored.Token.Expiry) && stored.Token.RefreshToken == "" {
		s.cacheAccountReadiness(definition.ID, fingerprint, readiness)
		return readiness
	}
	readiness = connectors.AccountReadiness{State: connectors.StateConnected}
	s.cacheAccountReadiness(definition.ID, fingerprint, readiness)
	return readiness
}

func (s *Service) cachedAccountReadiness(connectorID, fingerprint string) (connectors.AccountReadiness, bool) {
	s.mu.RLock()
	cached, ok := s.readinessCache[connectorID]
	s.mu.RUnlock()
	if !ok || cached.fingerprint != fingerprint || time.Now().After(cached.expiresAt) {
		return connectors.AccountReadiness{}, false
	}
	return cached.value, true
}

func (s *Service) cacheAccountReadiness(connectorID, fingerprint string, value connectors.AccountReadiness) {
	s.mu.Lock()
	s.readinessCache[connectorID] = cachedAccountReadiness{
		fingerprint: fingerprint,
		value:       value,
		expiresAt:   time.Now().Add(accountReadinessCacheTTL),
	}
	s.mu.Unlock()
}

func (s *Service) invalidateAccountReadiness(connectorID string) {
	s.mu.Lock()
	delete(s.readinessCache, strings.TrimSpace(connectorID))
	s.mu.Unlock()
}

// ConnectorReadiness resolves a host Connector by stable ID and reports its
// shared account state to plugin setup without exposing credential material.
func (s *Service) ConnectorReadiness(connectorID string) (connectors.AccountReadiness, error) {
	if s.registry == nil {
		return connectors.AccountReadiness{}, ErrUnsupported
	}
	definition, err := s.registry.Definition(strings.TrimSpace(connectorID))
	if err != nil {
		return connectors.AccountReadiness{}, err
	}
	if definition.AuthType != connectors.AuthOAuth2 || definition.OAuth == nil {
		return connectors.AccountReadiness{}, ErrUnsupported
	}
	return s.AccountReadiness(definition), nil
}

// ConnectorStatus returns the combined shared-account and tool-runtime state.
func (s *Service) ConnectorStatus(connectorID string) (connectors.ConnectorStatus, error) {
	if s.registry == nil {
		return connectors.ConnectorStatus{}, ErrUnsupported
	}
	return s.registry.Status(strings.TrimSpace(connectorID))
}

func (s *Service) Start(ctx context.Context, connectorID string) (connectors.ConnectorAuthStartResult, error) {
	return s.start(ctx, connectorID, false)
}

// Reauthorize starts a fresh interactive login even when a valid shared
// account already exists. The existing credential remains usable until a new
// token exchange succeeds, so closing the browser does not silently sign the
// user out.
func (s *Service) Reauthorize(ctx context.Context, connectorID string) (connectors.ConnectorAuthStartResult, error) {
	return s.start(ctx, connectorID, true)
}

func (s *Service) start(ctx context.Context, connectorID string, force bool) (connectors.ConnectorAuthStartResult, error) {
	if s.registry == nil {
		return connectors.ConnectorAuthStartResult{}, ErrUnsupported
	}
	definition, err := s.registry.Definition(strings.TrimSpace(connectorID))
	if err != nil {
		return connectors.ConnectorAuthStartResult{}, err
	}
	if err := validateDefinition(definition); err != nil {
		return connectors.ConnectorAuthStartResult{}, err
	}
	fingerprint := connectorFingerprint(definition)
	if active := s.activeSession(definition.ID, fingerprint); active != nil {
		view := active.view()
		return connectors.ConnectorAuthStartResult{
			SessionID: view.SessionID, ConnectorID: view.ConnectorID, Status: view.Status, ExpiresAt: view.ExpiresAt,
		}, nil
	}
	if readiness := s.AccountReadiness(definition); !force && readiness.State == connectors.StateConnected {
		if connectErr := s.registry.Connect(context.WithoutCancel(ctx), definition.ID); connectErr != nil && !errors.Is(connectErr, connectors.ErrConnectorAlreadyConnected) {
			return connectors.ConnectorAuthStartResult{}, connectErr
		}
		if runtimeErr := s.waitForRuntime(definition.ID); runtimeErr != nil {
			return connectors.ConnectorAuthStartResult{}, runtimeErr
		}
		return connectors.ConnectorAuthStartResult{ConnectorID: definition.ID, Status: "ready"}, nil
	}
	if err := s.approvals.Clear(definition.ID); err != nil {
		return connectors.ConnectorAuthStartResult{}, fmt.Errorf("clear connector approval state: %w", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return connectors.ConnectorAuthStartResult{}, fmt.Errorf("start connector OAuth callback: %w", err)
	}
	state, err := randomURLToken(32)
	if err != nil {
		_ = listener.Close()
		return connectors.ConnectorAuthStartResult{}, err
	}
	verifier, err := randomURLToken(64)
	if err != nil {
		_ = listener.Close()
		return connectors.ConnectorAuthStartResult{}, err
	}
	// Microsoft desktop app registrations use the localhost loopback redirect;
	// the listener itself remains pinned to 127.0.0.1 on a random port.
	callbackURL := fmt.Sprintf("http://localhost:%d/oauth/callback", listener.Addr().(*net.TCPAddr).Port)
	expiresAt := time.Now().UTC().Add(sessionTTL)
	sessionCtx, cancel := context.WithCancel(context.Background())
	entry := &session{
		id: uuid.NewString(), connectorID: definition.ID, fingerprint: fingerprint,
		status: "authorizing", expiresAt: expiresAt, state: state, verifier: verifier,
		cancel: cancel, listener: listener,
	}

	config := oauth2.Config{
		ClientID: definition.OAuth.ClientID, RedirectURL: callbackURL,
		Scopes:   append([]string(nil), definition.OAuth.Scopes...),
		Endpoint: oauth2.Endpoint{AuthURL: definition.OAuth.AuthorizationEndpoint, TokenURL: definition.OAuth.TokenEndpoint},
	}
	options := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("response_mode", "query"),
	}
	keys := make([]string, 0, len(definition.OAuth.AuthorizationParams))
	for key := range definition.OAuth.AuthorizationParams {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		options = append(options, oauth2.SetAuthURLParam(key, definition.OAuth.AuthorizationParams[key]))
	}
	authorizationURL := config.AuthCodeURL(state, options...)
	if err := validateActionURL(authorizationURL); err != nil {
		cancel()
		_ = listener.Close()
		return connectors.ConnectorAuthStartResult{}, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/callback", func(w http.ResponseWriter, request *http.Request) {
		entry.mu.Lock()
		if entry.status != "authorizing" || entry.callbackConsumed {
			entry.mu.Unlock()
			http.Error(w, "authorization callback is no longer active", http.StatusConflict)
			return
		}
		if request.URL.Query().Get("state") != entry.state {
			entry.mu.Unlock()
			http.Error(w, "authorization state did not match", http.StatusBadRequest)
			return
		}
		if providerError := strings.TrimSpace(request.URL.Query().Get("error")); providerError != "" {
			providerDescription := strings.TrimSpace(request.URL.Query().Get("error_description"))
			entry.callbackConsumed = true
			entry.mu.Unlock()
			if authapproval.RequiredByProvider(providerError, providerDescription) {
				if _, markErr := s.markAdminApprovalPending(entry); markErr != nil {
					entry.finish("failed", "无法保存管理员审批状态，请重试。")
					go entry.closeServer()
					http.Error(w, "authorization approval state could not be saved", http.StatusInternalServerError)
				} else {
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					_, _ = w.Write([]byte("<!doctype html><meta charset=\"utf-8\"><title>Awaken</title><p>已记录管理员审批请求。批准后请返回 Awaken 重新连接。</p>"))
				}
				return
			}
			entry.finish("failed", "账号提供方未完成授权，请重试。")
			http.Error(w, "authorization was not completed", http.StatusBadRequest)
			go entry.closeServer()
			return
		}
		code := strings.TrimSpace(request.URL.Query().Get("code"))
		if code == "" {
			entry.mu.Unlock()
			http.Error(w, "authorization callback did not include a code", http.StatusBadRequest)
			return
		}
		entry.callbackConsumed = true
		entry.status = "verifying"
		entry.mu.Unlock()

		exchangeCtx, exchangeCancel := context.WithTimeout(context.WithValue(sessionCtx, oauth2.HTTPClient, s.httpClient), 30*time.Second)
		token, exchangeErr := config.Exchange(exchangeCtx, code, oauth2.VerifierOption(verifier))
		exchangeCancel()
		if exchangeErr != nil {
			entry.finish("failed", "账号授权未能完成，请重试。")
			http.Error(w, "authorization failed", http.StatusBadGateway)
			return
		}
		if view := entry.view(); view.Status != "verifying" {
			http.Error(w, "authorization session was cancelled", http.StatusConflict)
			return
		}
		stored := mcpclient.StoredTokenSource{
			Token: token, ServerURL: connectorContractURI(definition),
			AuthURL: definition.OAuth.AuthorizationEndpoint, TokenURL: definition.OAuth.TokenEndpoint,
			RedirectURL: callbackURL, ClientID: definition.OAuth.ClientID,
			Scopes: append([]string(nil), definition.OAuth.Scopes...),
		}
		if saveErr := s.credentials.SaveTokenSource(connectorCredentialIdentity(definition.ID), stored); saveErr != nil {
			entry.finish("failed", "无法安全保存账号凭据。")
			http.Error(w, "credential persistence failed", http.StatusInternalServerError)
			return
		}
		s.cacheAccountReadiness(definition.ID, connectorFingerprint(definition), connectors.AccountReadiness{State: connectors.StateConnected})
		if view := entry.view(); view.Status != "verifying" {
			_ = s.credentials.ClearTokenSource(connectorCredentialIdentity(definition.ID))
			s.invalidateAccountReadiness(definition.ID)
			http.Error(w, "authorization session was cancelled", http.StatusConflict)
			return
		}
		if connectErr := s.registry.Connect(context.Background(), definition.ID); connectErr != nil && !errors.Is(connectErr, connectors.ErrConnectorAlreadyConnected) {
			entry.finish("failed", "账号已授权，但 Outlook 工具运行时启动失败。")
			http.Error(w, "connector runtime failed", http.StatusBadGateway)
			return
		}
		if runtimeErr := s.waitForRuntime(definition.ID); runtimeErr != nil {
			entry.finish("failed", "账号已授权，但 Outlook 工具运行时未就绪。")
			http.Error(w, "connector runtime was not ready", http.StatusBadGateway)
			return
		}
		if !entry.finish("ready", "") {
			_ = s.registry.Disconnect(context.Background(), definition.ID)
			_ = s.credentials.ClearTokenSource(connectorCredentialIdentity(definition.ID))
			s.invalidateAccountReadiness(definition.ID)
			http.Error(w, "authorization session was cancelled", http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><meta charset=\"utf-8\"><title>Awaken Outlook</title><p>Outlook 已连接，可以返回 Awaken。</p>"))
		go entry.closeServer()
	})
	entry.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	s.putSession(entry)
	go func() {
		if serveErr := entry.server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			entry.finish("failed", "本机授权回调服务异常，请重试。")
		}
	}()
	go func() {
		timer := time.NewTimer(time.Until(expiresAt))
		defer timer.Stop()
		select {
		case <-sessionCtx.Done():
			return
		case <-timer.C:
			view := entry.view()
			if view.Status == "authorizing" || view.Status == "verifying" {
				entry.finish("expired", "授权已过期，请重新连接。")
				entry.closeServer()
			}
		}
	}()

	return connectors.ConnectorAuthStartResult{
		SessionID: entry.id, ConnectorID: definition.ID, Status: "authorizing",
		AuthorizationURL: authorizationURL, ExpiresAt: &expiresAt,
	}, nil
}

func (s *Service) waitForRuntime(connectorID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		status, err := s.registry.RuntimeStatus(connectorID)
		if err != nil {
			return err
		}
		switch status.State {
		case connectors.StateConnected:
			return nil
		case connectors.StateFailed, connectors.StateNeedsAuth:
			return fmt.Errorf("connector runtime is %s", status.State)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("connector runtime readiness timed out: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (s *Service) Session(sessionID string) (connectors.ConnectorAuthSession, error) {
	s.mu.RLock()
	entry := s.sessions[strings.TrimSpace(sessionID)]
	s.mu.RUnlock()
	if entry == nil {
		return connectors.ConnectorAuthSession{}, ErrSessionNotFound
	}
	return entry.view(), nil
}

func (s *Service) Cancel(sessionID string) (connectors.ConnectorAuthSession, error) {
	s.mu.RLock()
	entry := s.sessions[strings.TrimSpace(sessionID)]
	s.mu.RUnlock()
	if entry == nil {
		return connectors.ConnectorAuthSession{}, ErrSessionNotFound
	}
	if entry.view().Status == "pending_admin_approval" {
		if err := s.approvals.Clear(entry.connectorID); err != nil {
			return connectors.ConnectorAuthSession{}, err
		}
	}
	entry.cancelSession()
	entry.closeServer()
	return entry.view(), nil
}

// MarkAdminApprovalPending records a user-observed external browser result.
// OAuth providers do not consistently redirect approval-request pages back to
// loopback clients, so this explicit transition is the only portable signal.
func (s *Service) MarkAdminApprovalPending(sessionID string) (connectors.ConnectorAuthSession, error) {
	s.mu.RLock()
	entry := s.sessions[strings.TrimSpace(sessionID)]
	s.mu.RUnlock()
	if entry == nil {
		return connectors.ConnectorAuthSession{}, ErrSessionNotFound
	}
	return s.markAdminApprovalPending(entry)
}

func (s *Service) markAdminApprovalPending(entry *session) (connectors.ConnectorAuthSession, error) {
	if entry == nil {
		return connectors.ConnectorAuthSession{}, ErrSessionNotFound
	}
	view := entry.view()
	if view.Status != "authorizing" && view.Status != "verifying" {
		return view, nil
	}
	if err := s.approvals.Mark(entry.connectorID, entry.fingerprint); err != nil {
		return connectors.ConnectorAuthSession{}, err
	}
	entry.markAdminApprovalPending()
	go entry.closeServer()
	return entry.view(), nil
}

func (s *Service) Disconnect(ctx context.Context, connectorID string) error {
	connectorID = strings.TrimSpace(connectorID)
	if s.registry == nil {
		return ErrUnsupported
	}
	if _, err := s.registry.Definition(connectorID); err != nil {
		return err
	}
	s.mu.RLock()
	var active []*session
	for _, entry := range s.sessions {
		if entry.connectorID == connectorID {
			active = append(active, entry)
		}
	}
	s.mu.RUnlock()
	for _, entry := range active {
		entry.cancelSession()
		entry.closeServer()
	}
	if err := s.registry.Disconnect(ctx, connectorID); err != nil && !errors.Is(err, connectors.ErrConnectorNotConnected) {
		return err
	}
	err := errors.Join(
		s.credentials.ClearTokenSource(connectorCredentialIdentity(connectorID)),
		s.approvals.Clear(connectorID),
	)
	s.invalidateAccountReadiness(connectorID)
	return err
}

func (s *Service) AccessToken(ctx context.Context, connectorID string) (string, error) {
	if s.registry == nil {
		return "", ErrUnsupported
	}
	definition, err := s.registry.Definition(strings.TrimSpace(connectorID))
	if err != nil {
		return "", err
	}
	if err := validateDefinition(definition); err != nil {
		return "", err
	}
	identity := connectorCredentialIdentity(definition.ID)
	stored, err := s.credentials.LoadTokenSource(identity)
	if err != nil {
		s.invalidateAccountReadiness(definition.ID)
		if errors.Is(err, mcpclient.ErrCredentialNotFound) {
			return "", ErrNeedsAuth
		}
		return "", err
	}
	if !storedMatchesDefinition(stored, definition) {
		_ = s.credentials.ClearTokenSource(identity)
		s.invalidateAccountReadiness(definition.ID)
		return "", ErrCredentialChanged
	}
	if stored.Token == nil || (stored.NeedsReauth && strings.TrimSpace(stored.Token.RefreshToken) == "") {
		return "", ErrNeedsAuth
	}
	clientCtx := context.WithValue(ctx, oauth2.HTTPClient, s.httpClient)
	config := oauth2.Config{
		ClientID: definition.OAuth.ClientID, RedirectURL: stored.RedirectURL,
		Scopes:   append([]string(nil), definition.OAuth.Scopes...),
		Endpoint: oauth2.Endpoint{AuthURL: definition.OAuth.AuthorizationEndpoint, TokenURL: definition.OAuth.TokenEndpoint},
	}
	sourceToken := cloneOAuthToken(stored.Token)
	if stored.NeedsReauth {
		// Older builds marked every refresh transport failure as needs_reauth.
		// A retained refresh token is still recoverable: force one provider
		// refresh so a transient proxy outage does not require another login.
		sourceToken.Expiry = time.Now().Add(-time.Hour)
	}
	token, err := config.TokenSource(clientCtx, sourceToken).Token()
	if err != nil {
		if !tokenErrorRequiresReauth(err) {
			return "", fmt.Errorf("%w: %v", ErrRefreshUnavailable, err)
		}
		stored.NeedsReauth = true
		stored.ReauthReason = "provider_rejected"
		_ = s.credentials.SaveTokenSource(identity, stored)
		s.invalidateAccountReadiness(definition.ID)
		return "", ErrNeedsAuth
	}
	if token == nil || strings.TrimSpace(token.AccessToken) == "" {
		stored.NeedsReauth = true
		stored.ReauthReason = "provider_rejected"
		_ = s.credentials.SaveTokenSource(identity, stored)
		s.invalidateAccountReadiness(definition.ID)
		return "", ErrNeedsAuth
	}
	if stored.NeedsReauth || tokenSignature(token) != tokenSignature(stored.Token) {
		stored.Token = token
		stored.NeedsReauth = false
		stored.ReauthReason = ""
		if err := s.credentials.SaveTokenSource(identity, stored); err != nil {
			return "", err
		}
	}
	s.cacheAccountReadiness(definition.ID, connectorFingerprint(definition), connectors.AccountReadiness{State: connectors.StateConnected})
	return token.AccessToken, nil
}

func (s *Service) MarkNeedsReauth(connectorID string) error {
	identity := connectorCredentialIdentity(strings.TrimSpace(connectorID))
	stored, err := s.credentials.LoadTokenSource(identity)
	if err != nil {
		return err
	}
	stored.NeedsReauth = true
	stored.ReauthReason = "access_token_rejected"
	if err := s.credentials.SaveTokenSource(identity, stored); err != nil {
		return err
	}
	s.invalidateAccountReadiness(connectorID)
	return nil
}

func (s *Service) activeSession(connectorID, fingerprint string) *session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, entry := range s.sessions {
		view := entry.view()
		if entry.connectorID == connectorID && entry.fingerprint == fingerprint && (view.Status == "authorizing" || view.Status == "verifying") {
			return entry
		}
	}
	return nil
}

func (s *Service) putSession(entry *session) {
	s.mu.Lock()
	for _, existing := range s.sessions {
		if existing.connectorID == entry.connectorID {
			existing.cancelSession()
			existing.closeServer()
		}
	}
	s.sessions[entry.id] = entry
	s.mu.Unlock()
	time.AfterFunc(15*time.Minute, func() {
		s.mu.Lock()
		if s.sessions[entry.id] == entry {
			delete(s.sessions, entry.id)
		}
		s.mu.Unlock()
	})
}

func (s *session) closeServer() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.server != nil {
		_ = s.server.Close()
	} else if s.listener != nil {
		_ = s.listener.Close()
	}
}

func validateDefinition(definition connectors.ConnectorDefinition) error {
	if definition.OAuth == nil || definition.AuthType != connectors.AuthOAuth2 {
		return ErrUnsupported
	}
	if strings.TrimSpace(definition.OAuth.ClientID) == "" {
		return ErrClientIDRequired
	}
	if err := validateActionURL(definition.OAuth.AuthorizationEndpoint); err != nil {
		return fmt.Errorf("invalid connector authorization endpoint: %w", err)
	}
	if err := validateActionURL(definition.OAuth.TokenEndpoint); err != nil {
		return fmt.Errorf("invalid connector token endpoint: %w", err)
	}
	return nil
}

func connectorCredentialIdentity(connectorID string) string {
	sum := sha256.Sum256([]byte("connector-account\x00" + strings.TrimSpace(connectorID) + "\x00default"))
	return "connector:" + hex.EncodeToString(sum[:])
}

func connectorFingerprint(definition connectors.ConnectorDefinition) string {
	if definition.OAuth == nil {
		return ""
	}
	keys := make([]string, 0, len(definition.OAuth.AuthorizationParams))
	for key := range definition.OAuth.AuthorizationParams {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := []string{
		definition.ID, definition.OAuth.AuthorizationEndpoint, definition.OAuth.TokenEndpoint,
		definition.OAuth.ClientID, definition.OAuth.Resource, strings.Join(definition.OAuth.Scopes, " "),
	}
	for _, key := range keys {
		parts = append(parts, key+"="+definition.OAuth.AuthorizationParams[key])
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func connectorContractURI(definition connectors.ConnectorDefinition) string {
	return "awaken://connector/" + url.PathEscape(definition.ID) + "?fingerprint=" + connectorFingerprint(definition)
}

func storedMatchesDefinition(stored mcpclient.StoredTokenSource, definition connectors.ConnectorDefinition) bool {
	if definition.OAuth == nil {
		return false
	}
	return stored.ServerURL == connectorContractURI(definition) &&
		stored.AuthURL == definition.OAuth.AuthorizationEndpoint &&
		stored.TokenURL == definition.OAuth.TokenEndpoint &&
		stored.ClientID == definition.OAuth.ClientID &&
		strings.Join(stored.Scopes, "\x00") == strings.Join(definition.OAuth.Scopes, "\x00")
}

func randomURLToken(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate authorization nonce: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func tokenSignature(token *oauth2.Token) string {
	if token == nil {
		return ""
	}
	return strings.Join([]string{token.AccessToken, token.TokenType, token.RefreshToken, token.Expiry.UTC().Format(time.RFC3339Nano)}, "\x00")
}

func cloneOAuthToken(token *oauth2.Token) *oauth2.Token {
	if token == nil {
		return nil
	}
	clone := *token
	return &clone
}

func tokenErrorRequiresReauth(err error) bool {
	var retrieveErr *oauth2.RetrieveError
	if !errors.As(err, &retrieveErr) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(retrieveErr.ErrorCode)) {
	case "invalid_grant", "invalid_client", "unauthorized_client", "interaction_required", "consent_required", "admin_consent_required":
		return true
	default:
		return false
	}
}
