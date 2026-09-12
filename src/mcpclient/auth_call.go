package mcpclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"awaken/src/tool"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

type authCallKey struct{}
type authCall struct {
	server     string
	generation uint64
	readOnly   bool
	budget     *tool.AuthReplayBudget
	current    func() bool
	mu         sync.Mutex
	sends      int
	failure    *NeedsAuthError
}

func (c *authCall) fail() *NeedsAuthError {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failure = &NeedsAuthError{ServerName: c.server, Reason: "MCP OAuth authorization is required", CallStarted: true, ReadOnly: c.readOnly}
	return c.failure
}

func (m *Manager) callAuthContext(ctx context.Context, server *managedServer, name string, args map[string]any) context.Context {
	ctx = tool.WithAuthReplayBudget(ctx)
	m.mu.RLock()
	configBytes, _ := json.Marshal(server.config)
	binding := fmt.Sprintf("%s:%s:%x", CredentialIdentity(server.scope, server.name, server.config), server.sourceRevision, sha256.Sum256(configBytes))
	readOnly := false
	generation := server.connectionGeneration
	var selected *mcp.Tool
	for _, definition := range server.tools {
		if definition.Name == name {
			selected = definition
			break
		}
	}
	m.mu.RUnlock()
	if selected != nil {
		adapter := NewMcpToolAdapter(m, server.name, selected)
		readOnly = !adapter.artifactOutputEnabled && (adapter.declaredReadOperation(args) || (adapter.errorReadOnly && adapter.readOnly && !adapter.destructive))
	}
	budget := tool.AuthReplayBudgetFromContext(ctx)
	bound := budget.Bind(binding)
	call := &authCall{server: server.name, generation: generation, readOnly: readOnly, budget: budget, current: func() bool {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return bound && !m.closing && m.servers[server.name] == server && server.state == StateConnected && server.connectionGeneration == generation
	}}
	return context.WithValue(ctx, authCallKey{}, call)
}

type callOAuthHandler struct{ auth.OAuthHandler }

func (h callOAuthHandler) Authorize(ctx context.Context, req *http.Request, resp *http.Response) error {
	call, _ := ctx.Value(authCallKey{}).(*authCall)
	if call == nil {
		return h.OAuthHandler.Authorize(ctx, req, resp)
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !isToolCallHTTPRequest(req) {
		return fmt.Errorf("MCP protocol message requires authorization")
	}
	if !call.current() || !call.readOnly || resp.StatusCode != http.StatusUnauthorized {
		return call.fail()
	}
	challenges, err := oauthex.ParseWWWAuthenticate(resp.Header.Values("WWW-Authenticate"))
	if err != nil {
		return call.fail()
	}
	var scopes []string
	bearer := false
	for _, challenge := range challenges {
		if strings.EqualFold(challenge.Scheme, "bearer") && challenge.Params["error"] != "insufficient_scope" {
			bearer = true
			scopes = strings.Fields(challenge.Params["scope"])
			break
		}
	}
	refresher, ok := h.OAuthHandler.(*persistentOAuthHandler)
	if !bearer || !ok || !call.budget.Take() {
		return call.fail()
	}
	rejected := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	if err := refresher.refreshRejectedToken(ctx, rejected, scopes); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return call.fail()
	}
	if !call.current() {
		return call.fail()
	}
	return nil // The SDK performs exactly one resend after this success.
}

type authCallTransport struct {
	base         http.RoundTripper
	allowRefresh bool
}

func (t authCallTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	call, _ := req.Context().Value(authCallKey{}).(*authCall)
	if call == nil {
		return t.base.RoundTrip(req)
	}
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	// SDK cancellation notifications and elicitation responses carry the original
	// context values, but are not replays of tools/call. They must still be sent
	// during shutdown without consuming or poisoning that call's auth budget.
	if !isToolCallHTTPRequest(req) {
		return t.base.RoundTrip(req)
	}
	if !call.current() {
		return nil, call.fail()
	}
	call.mu.Lock()
	call.sends++
	sends := call.sends
	call.mu.Unlock()
	if sends > 2 {
		return nil, call.fail()
	}
	resp, err := t.base.RoundTrip(req)
	if err == nil && (resp.StatusCode == 401 || resp.StatusCode == 403) && (sends > 1 || resp.StatusCode == 403 || !t.allowRefresh) {
		_ = resp.Body.Close()
		return nil, call.fail()
	}
	return resp, err
}

func (h *persistentOAuthHandler) refreshRejectedToken(ctx context.Context, rejected string, scopes []string) error {
	release, err := h.lockAuthorization(ctx)
	if err != nil {
		return err
	}
	defer release()
	if h.store == nil {
		return fmt.Errorf("credential store unavailable")
	}
	stored, err := h.store.LoadTokenSource(h.credentialIdentity)
	if err != nil {
		return err
	}
	if stored.NeedsReauth || stored.Token == nil || canonicalServerURL(stored.ServerURL) != canonicalServerURL(h.cfg.ServerURL) {
		return fmt.Errorf("credential identity is unavailable")
	}
	allowed := map[string]bool{}
	for _, scope := range stored.Scopes {
		allowed[scope] = true
	}
	for _, scope := range scopes {
		if !allowed[scope] {
			return fmt.Errorf("additional scope requires user authorization")
		}
	}
	if stored.Token.AccessToken == rejected || !stored.Token.Valid() {
		if stored.Token.RefreshToken == "" || stored.TokenURL == "" || stored.ClientID == "" {
			return fmt.Errorf("silent refresh is unavailable")
		}
		token := *stored.Token
		token.AccessToken = "" // Force refresh even when the rejected token's expiry is still in the future.
		authStyle := oauth2.AuthStyleInParams
		if stored.ClientSecret != "" {
			authStyle = oauth2.AuthStyleInHeader
		}
		config := oauth2.Config{ClientID: stored.ClientID, ClientSecret: stored.ClientSecret, Scopes: stored.Scopes, Endpoint: oauth2.Endpoint{TokenURL: stored.TokenURL, AuthStyle: authStyle}}
		refreshCtx := context.WithValue(ctx, oauth2.HTTPClient, oauthSafeHTTPClient())
		next, refreshErr := config.TokenSource(refreshCtx, &token).Token()
		if refreshErr != nil {
			return refreshErr
		}
		latest, loadErr := h.store.LoadTokenSource(h.credentialIdentity)
		if loadErr != nil {
			return loadErr
		}
		if latest.Token == nil || latest.Token.AccessToken != stored.Token.AccessToken || latest.NeedsReauth {
			return fmt.Errorf("credential changed during refresh")
		}
		stored.Token = next
		if err := h.store.SaveTokenSource(h.credentialIdentity, stored); err != nil {
			return err
		}
	}
	wrapped, err := h.wrapStoredTokenSource(ctx, stored)
	if err != nil {
		return err
	}
	h.mu.Lock()
	h.tokenSource = wrapped
	h.generation++
	h.mu.Unlock()
	return nil
}

func (h *persistentOAuthHandler) lockAuthorization(ctx context.Context) (func(), error) {
	h.mu.Lock()
	if h.authorizeGate == nil {
		h.authorizeGate = make(chan struct{}, 1)
	}
	gate := h.authorizeGate
	h.mu.Unlock()
	select {
	case gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-gate
			return nil, err
		}
		return func() { <-gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Inspect the SDK JSON-RPC envelope without consuming the outgoing body.
func isToolCallHTTPRequest(req *http.Request) bool {
	if req.Method != http.MethodPost || req.Body == nil {
		return false
	}
	var body io.ReadCloser
	if req.GetBody != nil {
		var err error
		body, err = req.GetBody()
		if err != nil {
			return false
		}
	} else {
		data, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(data))
		if err != nil {
			return false
		}
		body = io.NopCloser(bytes.NewReader(data))
	}
	defer body.Close()
	var envelope struct {
		Method string `json:"method"`
	}
	return json.NewDecoder(body).Decode(&envelope) == nil && envelope.Method == "tools/call"
}
