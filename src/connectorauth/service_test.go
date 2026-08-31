package connectorauth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"awaken/src/connectors"
	"awaken/src/mcpclient"
	"golang.org/x/oauth2"
)

type fakeRegistry struct {
	definition  connectors.ConnectorDefinition
	connects    atomic.Int32
	disconnects atomic.Int32
}

func (r *fakeRegistry) Definition(id string) (connectors.ConnectorDefinition, error) {
	if id != r.definition.ID {
		return connectors.ConnectorDefinition{}, connectors.ErrConnectorNotFound
	}
	return r.definition, nil
}

func (r *fakeRegistry) Status(id string) (connectors.ConnectorStatus, error) {
	if id != r.definition.ID {
		return connectors.ConnectorStatus{}, connectors.ErrConnectorNotFound
	}
	state := connectors.StateDisconnected
	if r.connects.Load() > 0 {
		state = connectors.StateConnected
	}
	return connectors.ConnectorStatus{ID: id, State: state}, nil
}

func (r *fakeRegistry) Connect(context.Context, string) error {
	r.connects.Add(1)
	return nil
}

func (r *fakeRegistry) Disconnect(context.Context, string) error {
	r.disconnects.Add(1)
	return nil
}

func testDefinition(endpoint string) connectors.ConnectorDefinition {
	return connectors.ConnectorDefinition{
		ID: "awaken.microsoft.outlook", AuthType: connectors.AuthOAuth2,
		OAuth: &connectors.OAuthConnectorSpec{
			AuthorizationEndpoint: endpoint + "/authorize", TokenEndpoint: endpoint + "/token",
			ClientID: "public-client", Scopes: []string{"offline_access", "Mail.ReadWrite"},
		},
	}
}

func waitSession(t *testing.T, service *Service, id string) connectors.ConnectorAuthSession {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		view, err := service.Session(id)
		if err != nil {
			t.Fatal(err)
		}
		if view.Status != "authorizing" && view.Status != "verifying" {
			return view
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("connector authorization did not finish")
	return connectors.ConnectorAuthSession{}
}

func TestPKCEConnectsRuntimeAndKeepsSecretsOutOfSession(t *testing.T) {
	var tokenRequests atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}
		tokenRequests.Add(1)
		if err := r.ParseForm(); err != nil || r.Form.Get("code") != "good" || r.Form.Get("code_verifier") == "" {
			http.Error(w, "bad exchange", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"OUTLOOK-TOP-SECRET","refresh_token":"OUTLOOK-REFRESH-SECRET","token_type":"Bearer","expires_in":3600}`)
	}))
	defer provider.Close()

	registry := &fakeRegistry{definition: testDefinition(provider.URL)}
	credentials := mcpclient.NewInMemoryCredentialStore()
	service := NewService(registry, credentials)
	started, err := service.Start(context.Background(), registry.definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	authorizationURL, err := url.Parse(started.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	callback := authorizationURL.Query().Get("redirect_uri")
	state := authorizationURL.Query().Get("state")
	if callback == "" || state == "" || authorizationURL.Query().Get("code_challenge") == "" {
		t.Fatalf("authorization URL is incomplete: %s", started.AuthorizationURL)
	}
	wrong, err := http.Get(callback + "?code=bad&state=wrong")
	if err != nil {
		t.Fatal(err)
	}
	_ = wrong.Body.Close()
	if wrong.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong state status=%d", wrong.StatusCode)
	}
	response, err := http.Get(callback + "?code=good&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("callback status=%d", response.StatusCode)
	}
	view := waitSession(t, service, started.SessionID)
	if view.Status != "ready" || registry.connects.Load() != 1 || tokenRequests.Load() != 1 {
		t.Fatalf("view=%+v connects=%d tokenRequests=%d", view, registry.connects.Load(), tokenRequests.Load())
	}
	encoded, _ := json.Marshal(view)
	if strings.Contains(string(encoded), "OUTLOOK-TOP-SECRET") || strings.Contains(string(encoded), "good") {
		t.Fatalf("session leaked credential material: %s", encoded)
	}
	token, err := service.AccessToken(context.Background(), registry.definition.ID)
	if err != nil || token != "OUTLOOK-TOP-SECRET" {
		t.Fatalf("access token resolution failed: token=%q err=%v", token, err)
	}
}

func TestMissingPublicClientIDFailsWithoutOpeningSession(t *testing.T) {
	registry := &fakeRegistry{definition: connectors.ConnectorDefinition{
		ID: "outlook", AuthType: connectors.AuthOAuth2,
		OAuth: &connectors.OAuthConnectorSpec{AuthorizationEndpoint: "https://login.example.test/authorize", TokenEndpoint: "https://login.example.test/token"},
	}}
	service := NewService(registry, mcpclient.NewInMemoryCredentialStore())
	if _, err := service.Start(context.Background(), "outlook"); err != ErrClientIDRequired {
		t.Fatalf("missing client ID error=%v", err)
	}
	readiness := service.AccountReadiness(registry.definition)
	if readiness.State != connectors.StateFailed || readiness.LastError == "" {
		t.Fatalf("missing client ID readiness=%+v", readiness)
	}
}

func TestCancelDuringCodeExchangeCannotPublishCredentialOrRuntime(t *testing.T) {
	exchangeStarted := make(chan struct{})
	releaseExchange := make(chan struct{})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}
		close(exchangeStarted)
		select {
		case <-releaseExchange:
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"MUST-NOT-PERSIST","token_type":"Bearer"}`)
		case <-r.Context().Done():
		}
	}))
	defer provider.Close()
	registry := &fakeRegistry{definition: testDefinition(provider.URL)}
	credentials := mcpclient.NewInMemoryCredentialStore()
	service := NewService(registry, credentials)
	started, err := service.Start(context.Background(), registry.definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	authorizationURL, _ := url.Parse(started.AuthorizationURL)
	callback := authorizationURL.Query().Get("redirect_uri")
	state := authorizationURL.Query().Get("state")
	callbackDone := make(chan struct{})
	go func() {
		defer close(callbackDone)
		response, _ := http.Get(callback + "?code=good&state=" + url.QueryEscape(state))
		if response != nil {
			_ = response.Body.Close()
		}
	}()
	select {
	case <-exchangeStarted:
	case <-time.After(time.Second):
		t.Fatal("token exchange did not start")
	}
	view, err := service.Cancel(started.SessionID)
	if err != nil || view.Status != "cancelled" {
		t.Fatalf("cancel view=%+v err=%v", view, err)
	}
	close(releaseExchange)
	select {
	case <-callbackDone:
	case <-time.After(time.Second):
		t.Fatal("callback did not stop after cancellation")
	}
	if present, _ := credentials.HasTokenSource(connectorCredentialIdentity(registry.definition.ID)); present {
		t.Fatal("cancelled exchange published a credential")
	}
	if registry.connects.Load() != 0 {
		t.Fatalf("cancelled exchange started runtime %d times", registry.connects.Load())
	}
}

func TestDisconnectClearsCredentialAndRuntime(t *testing.T) {
	registry := &fakeRegistry{definition: testDefinition("http://127.0.0.1:12345")}
	credentials := mcpclient.NewInMemoryCredentialStore()
	service := NewService(registry, credentials)
	identity := connectorCredentialIdentity(registry.definition.ID)
	if err := credentials.SaveTokenSource(identity, mcpclient.StoredTokenSource{
		Token: &oauth2.Token{AccessToken: "token"}, ServerURL: connectorContractURI(registry.definition),
		AuthURL: registry.definition.OAuth.AuthorizationEndpoint, TokenURL: registry.definition.OAuth.TokenEndpoint,
		ClientID: registry.definition.OAuth.ClientID, Scopes: registry.definition.OAuth.Scopes,
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.Disconnect(context.Background(), registry.definition.ID); err != nil {
		t.Fatal(err)
	}
	if registry.disconnects.Load() != 1 {
		t.Fatalf("disconnects=%d", registry.disconnects.Load())
	}
	if _, err := credentials.LoadTokenSource(identity); err != mcpclient.ErrCredentialNotFound {
		t.Fatalf("credential remained after disconnect: %v", err)
	}
}

func TestOAuthContractChangeDoesNotReuseHostAccount(t *testing.T) {
	registry := &fakeRegistry{definition: testDefinition("http://127.0.0.1:12345")}
	credentials := mcpclient.NewInMemoryCredentialStore()
	service := NewService(registry, credentials)
	if err := credentials.SaveTokenSource(connectorCredentialIdentity(registry.definition.ID), mcpclient.StoredTokenSource{
		Token:     &oauth2.Token{AccessToken: "token", RefreshToken: "refresh"},
		ServerURL: connectorContractURI(registry.definition), AuthURL: registry.definition.OAuth.AuthorizationEndpoint,
		TokenURL: registry.definition.OAuth.TokenEndpoint, ClientID: registry.definition.OAuth.ClientID,
		Scopes: append([]string(nil), registry.definition.OAuth.Scopes...),
	}); err != nil {
		t.Fatal(err)
	}
	changed := registry.definition
	changed.OAuth = &connectors.OAuthConnectorSpec{
		AuthorizationEndpoint: registry.definition.OAuth.AuthorizationEndpoint,
		TokenEndpoint:         registry.definition.OAuth.TokenEndpoint,
		ClientID:              registry.definition.OAuth.ClientID,
		Scopes:                []string{"offline_access", "Mail.ReadWrite", "Mail.Send"},
	}
	if readiness := service.AccountReadiness(changed); readiness.State != connectors.StateNeedsAuth {
		t.Fatalf("changed OAuth contract reused account: %+v", readiness)
	}
}

func TestConnectorAuthURLPolicyAllowsOnlyPublicHTTPSOrExplicitLoopback(t *testing.T) {
	for _, candidate := range []string{
		"http://example.com/authorize",
		"https://user:password@example.com/authorize",
		"https://192.168.1.5/authorize",
		"https://example.com/authorize?access_token=secret",
	} {
		if err := validateActionURL(candidate); err == nil {
			t.Fatalf("expected URL to be rejected: %s", candidate)
		}
	}
	for _, candidate := range []string{"https://login.microsoftonline.com/common/oauth2/v2.0/authorize", "http://localhost:43210/oauth/callback", "http://127.0.0.1:43210/oauth/callback"} {
		if err := validateActionURL(candidate); err != nil {
			t.Fatalf("expected URL to be allowed: %s: %v", candidate, err)
		}
	}
	if !isLoopbackHost("localhost") || !isLoopbackHost("127.0.0.1") || isLoopbackHost("login.example.com") {
		t.Fatal("loopback host classification is incorrect")
	}
}
