package pluginauth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"awaken/src/connectors"
	"awaken/src/mcpclient"
	"awaken/src/plugins"
	"awaken/src/plugins/ir"
	"golang.org/x/oauth2"
)

type fakeHostConnectorAuth struct {
	mu           sync.Mutex
	status       connectors.ConnectorStatus
	session      connectors.ConnectorAuthSession
	started      string
	reauthorized string
	cancelled    string
	disconnected string
}

func (f *fakeHostConnectorAuth) ConnectorStatus(string) (connectors.ConnectorStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status, nil
}
func (f *fakeHostConnectorAuth) Start(_ context.Context, connectorID string) (connectors.ConnectorAuthStartResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = connectorID
	expires := time.Now().Add(time.Minute)
	f.session = connectors.ConnectorAuthSession{SessionID: "host-session", ConnectorID: connectorID, Status: "authorizing", ExpiresAt: &expires}
	return connectors.ConnectorAuthStartResult{SessionID: f.session.SessionID, ConnectorID: connectorID, Status: f.session.Status, AuthorizationURL: "https://login.example.test/authorize", ExpiresAt: &expires}, nil
}
func (f *fakeHostConnectorAuth) Reauthorize(_ context.Context, connectorID string) (connectors.ConnectorAuthStartResult, error) {
	f.mu.Lock()
	f.reauthorized = connectorID
	f.mu.Unlock()
	return f.Start(context.Background(), connectorID)
}
func (f *fakeHostConnectorAuth) Session(string) (connectors.ConnectorAuthSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.session, nil
}
func (f *fakeHostConnectorAuth) Cancel(sessionID string) (connectors.ConnectorAuthSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelled = sessionID
	f.session.Status = "cancelled"
	return f.session, nil
}
func (f *fakeHostConnectorAuth) MarkAdminApprovalPending(sessionID string) (connectors.ConnectorAuthSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.session.Status = "pending_admin_approval"
	f.session.ErrorMessage = "等待组织管理员批准；批准后请重新连接。"
	f.status.State = connectors.StateApprovalRequired
	f.status.LastError = f.session.ErrorMessage
	return f.session, nil
}
func (f *fakeHostConnectorAuth) Disconnect(_ context.Context, connectorID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.disconnected = connectorID
	return nil
}

func waitPluginAuthSession(t *testing.T, service *Service, sessionID string) SessionView {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		view, err := service.Session(sessionID)
		if err != nil {
			t.Fatal(err)
		}
		if view.Status != "authorizing" && view.Status != "verifying" {
			return view
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("authorization session did not finish")
	return SessionView{}
}

func TestPKCESessionValidatesStateAndKeepsTokenOutOfView(t *testing.T) {
	var tokenRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}
		tokenRequests.Add(1)
		if err := r.ParseForm(); err != nil || r.Form.Get("code") != "good-code" || r.Form.Get("code_verifier") == "" {
			http.Error(w, "bad exchange", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"TOP-SECRET-TOKEN","token_type":"Bearer","expires_in":3600}`)
	}))
	defer server.Close()

	credentials := mcpclient.NewInMemoryCredentialStore()
	service := NewService(nil, nil, nil, credentials)
	requirement := ir.SetupRequirement{
		ID: "mail", Fingerprint: "fingerprint", Kind: ir.SetupOAuthPKCE,
		OAuth: &ir.OAuthSetupSpec{AuthorizationEndpoint: server.URL + "/authorize", TokenEndpoint: server.URL + "/token", ClientID: "public-client", Scopes: []string{"mail.read"}},
	}
	started, err := service.startPKCE("mail-plugin", requirement)
	if err != nil {
		t.Fatal(err)
	}
	authorizationURL, err := url.Parse(started.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	callbackURL := authorizationURL.Query().Get("redirect_uri")
	state := authorizationURL.Query().Get("state")
	if callbackURL == "" || state == "" || authorizationURL.Query().Get("code_challenge") == "" {
		t.Fatalf("PKCE authorization URL is incomplete: %s", started.AuthorizationURL)
	}
	response, err := http.Get(callbackURL + "?code=bad&state=wrong")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong state status=%d", response.StatusCode)
	}
	response, err = http.Get(callbackURL + "?code=good-code&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("callback status=%d", response.StatusCode)
	}
	view := waitPluginAuthSession(t, service, started.SessionID)
	if view.Status != "ready" || tokenRequests.Load() != 1 {
		t.Fatalf("view=%+v token requests=%d", view, tokenRequests.Load())
	}
	encoded, _ := json.Marshal(view)
	if strings.Contains(string(encoded), "TOP-SECRET") || strings.Contains(string(encoded), "good-code") {
		t.Fatalf("session view leaked credential material: %s", encoded)
	}
	stored, err := credentials.LoadTokenSource(pluginCredentialIdentity("mail-plugin", requirement))
	if err != nil || stored.Token == nil || stored.Token.AccessToken != "TOP-SECRET-TOKEN" {
		t.Fatalf("credential was not stored: stored=%+v err=%v", stored, err)
	}
}

func TestDeviceFlowUsesCompleteURLAndNeverReturnsCodeFromSession(t *testing.T) {
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/device":
			_, _ = io.WriteString(w, `{"device_code":"DEVICE-SECRET","user_code":"USER-SECRET","verification_uri":"https://example.test/device","verification_uri_complete":"https://example.test/device?user=bound","expires_in":60}`)
		case "/token":
			if polls.Add(1) == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"error":"authorization_pending"}`)
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"DEVICE-TOKEN","token_type":"Bearer","expires_in":3600}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	credentials := mcpclient.NewInMemoryCredentialStore()
	service := NewService(nil, nil, nil, credentials)
	service.deviceMinimumInterval = 10 * time.Millisecond
	requirement := ir.SetupRequirement{
		ID: "device", Fingerprint: "device-fingerprint", Kind: ir.SetupOAuthDevice,
		OAuth: &ir.OAuthSetupSpec{DeviceAuthorizationEndpoint: server.URL + "/device", TokenEndpoint: server.URL + "/token", ClientID: "public-client"},
	}
	started, err := service.startDevice("device-plugin", requirement)
	if err != nil {
		t.Fatal(err)
	}
	if started.VerificationURL != "https://example.test/device?user=bound" || started.DisplayUserCode || started.UserCode != "USER-SECRET" {
		t.Fatalf("unexpected device start response: %+v", started)
	}
	view := waitPluginAuthSession(t, service, started.SessionID)
	if view.Status != "ready" {
		t.Fatalf("device view=%+v", view)
	}
	encoded, _ := json.Marshal(view)
	if strings.Contains(string(encoded), "USER-SECRET") || strings.Contains(string(encoded), "DEVICE-SECRET") || strings.Contains(string(encoded), "DEVICE-TOKEN") {
		t.Fatalf("device session leaked credential material: %s", encoded)
	}
}

func TestValidateActionURLRejectsUnsafeTargets(t *testing.T) {
	for _, candidate := range []string{
		"http://example.com/auth",
		"https://user:pass@example.com/auth",
		"https://192.168.1.2/auth",
		"https://example.com/auth?access_token=secret",
	} {
		if err := ValidateActionURL(candidate); err == nil {
			t.Fatalf("expected URL to be rejected: %s", candidate)
		}
	}
	if err := ValidateActionURL("http://127.0.0.1:8080/callback"); err != nil {
		t.Fatalf("loopback callback should be accepted: %v", err)
	}
}

func TestPluginPKCEAdminApprovalObservationPersistsWithoutCredential(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()
	credentials := mcpclient.NewInMemoryCredentialStore()
	service := NewService(nil, nil, nil, credentials)
	requirement := ir.SetupRequirement{
		ID: "mail", Fingerprint: "approval-fingerprint", Kind: ir.SetupOAuthPKCE,
		OAuth: &ir.OAuthSetupSpec{AuthorizationEndpoint: server.URL + "/authorize", TokenEndpoint: server.URL + "/token", ClientID: "public-client"},
	}
	started, err := service.startPKCE("mail-plugin", requirement)
	if err != nil {
		t.Fatal(err)
	}
	view, err := service.MarkAdminApprovalPending(started.SessionID)
	if err != nil || view.Status != "pending_admin_approval" {
		t.Fatalf("pending view=%+v err=%v", view, err)
	}
	if status, _ := NewService(nil, nil, nil, credentials).SetupStatus("mail-plugin", requirement); status != plugins.SetupPendingAdminApproval {
		t.Fatalf("persisted setup status=%s", status)
	}
	if present, _ := credentials.HasTokenSource(pluginCredentialIdentity("mail-plugin", requirement)); present {
		t.Fatal("approval observation created a credential")
	}
}

func TestPluginPKCEExplicitProviderApprovalErrorMapsToPending(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	service := NewService(nil, nil, nil, mcpclient.NewInMemoryCredentialStore())
	requirement := ir.SetupRequirement{
		ID: "mail", Fingerprint: "approval-callback", Kind: ir.SetupOAuthPKCE,
		OAuth: &ir.OAuthSetupSpec{AuthorizationEndpoint: server.URL + "/authorize", TokenEndpoint: server.URL + "/token", ClientID: "public-client"},
	}
	started, err := service.startPKCE("mail-plugin", requirement)
	if err != nil {
		t.Fatal(err)
	}
	authorizationURL, _ := url.Parse(started.AuthorizationURL)
	callback := authorizationURL.Query().Get("redirect_uri")
	state := authorizationURL.Query().Get("state")
	response, err := http.Get(callback + "?error=admin_consent_required&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("callback status=%d", response.StatusCode)
	}
	view := waitPluginAuthSession(t, service, started.SessionID)
	if view.Status != "pending_admin_approval" {
		t.Fatalf("view=%+v", view)
	}
}

func TestCancelDeviceSessionIsTerminal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/device" {
			_, _ = io.WriteString(w, `{"device_code":"device","user_code":"user","verification_uri":"https://example.test/device","expires_in":60}`)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"authorization_pending"}`)
	}))
	defer server.Close()
	service := NewService(nil, nil, nil, mcpclient.NewInMemoryCredentialStore())
	service.deviceMinimumInterval = time.Second
	requirement := ir.SetupRequirement{ID: "device", Fingerprint: "f", Kind: ir.SetupOAuthDevice, OAuth: &ir.OAuthSetupSpec{DeviceAuthorizationEndpoint: server.URL + "/device", TokenEndpoint: server.URL + "/token", ClientID: "public"}}
	started, err := service.startDevice("plugin", requirement)
	if err != nil {
		t.Fatal(err)
	}
	view, err := service.Cancel(started.SessionID)
	if err != nil || view.Status != "cancelled" {
		t.Fatalf("cancel view=%+v err=%v", view, err)
	}
}

func TestPluginCredentialIdentityIsolationAndUpgradeFingerprint(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	credentials := mcpclient.NewInMemoryCredentialStore()
	service := NewService(nil, nil, nil, credentials)
	first := ir.SetupRequirement{ID: "account", Fingerprint: "v1", Kind: ir.SetupOAuthPKCE}
	second := ir.SetupRequirement{ID: "account", Fingerprint: "v2", Kind: ir.SetupOAuthPKCE}
	if pluginCredentialIdentity("plugin-a", first) == pluginCredentialIdentity("plugin-b", first) || pluginCredentialIdentity("plugin-a", first) == pluginCredentialIdentity("plugin-a", second) {
		t.Fatal("plugin/setup/fingerprint credential identities were not isolated")
	}
	stored := mcpclient.StoredTokenSource{Token: &oauth2.Token{AccessToken: "token"}}
	if err := service.saveCredential("plugin-a", first, stored); err != nil {
		t.Fatal(err)
	}
	if err := service.saveCredential("plugin-a", second, stored); err != nil {
		t.Fatal(err)
	}
	if present, _ := credentials.HasTokenSource(pluginCredentialIdentity("plugin-a", first)); present {
		t.Fatal("changed setup fingerprint retained the previous credential")
	}
	if present, _ := credentials.HasTokenSource(pluginCredentialIdentity("plugin-a", second)); !present {
		t.Fatal("new setup fingerprint credential is missing")
	}
}

func TestHostConnectorSetupReusesHostOwnedAccountSession(t *testing.T) {
	host := &fakeHostConnectorAuth{status: connectors.ConnectorStatus{State: connectors.StateNeedsAuth}}
	service := NewService(nil, nil, nil, mcpclient.NewInMemoryCredentialStore())
	service.SetHostConnectorAuth(host)
	requirement := ir.SetupRequirement{ID: "outlook", Fingerprint: "fp", Kind: ir.SetupHostConnector, ConnectorID: "awaken.microsoft.outlook", Required: true}
	if status, _ := service.SetupStatus("mail-plugin", requirement); status != "needs_auth" {
		t.Fatalf("status = %s", status)
	}
	started, err := service.startHostConnector(context.Background(), "mail-plugin", requirement, false)
	if err != nil {
		t.Fatal(err)
	}
	if started.SessionID == "" || started.AuthorizationURL == "" || host.started != requirement.ConnectorID {
		t.Fatalf("started = %+v host = %+v", started, host)
	}
	active, found, err := service.ActiveSession("mail-plugin", requirement.ID, requirement.Fingerprint)
	if err != nil || !found || active.SessionID != started.SessionID || active.Status != "authorizing" {
		t.Fatalf("active session = %+v found=%v err=%v", active, found, err)
	}
	if _, found, err := service.ActiveSession("mail-plugin", requirement.ID, "stale-fingerprint"); err != nil || found {
		t.Fatalf("stale fingerprint recovered a session: found=%v err=%v", found, err)
	}
	view, err := service.Cancel(started.SessionID)
	if err != nil || view.Status != "cancelled" || host.cancelled != "host-session" {
		t.Fatalf("view = %+v err = %v host = %+v", view, err, host)
	}
	if _, found, err := service.ActiveSession("mail-plugin", requirement.ID, requirement.Fingerprint); err != nil || found {
		t.Fatalf("cancelled session remained active: found=%v err=%v", found, err)
	}
}

func TestHostConnectorSetupProjectsAdminApprovalPending(t *testing.T) {
	host := &fakeHostConnectorAuth{status: connectors.ConnectorStatus{State: connectors.StateNeedsAuth}}
	service := NewService(nil, nil, nil, mcpclient.NewInMemoryCredentialStore())
	service.SetHostConnectorAuth(host)
	requirement := ir.SetupRequirement{ID: "outlook", Fingerprint: "fp", Kind: ir.SetupHostConnector, ConnectorID: "awaken.microsoft.outlook", Required: true}
	started, err := service.startHostConnector(context.Background(), "mail-plugin", requirement, false)
	if err != nil {
		t.Fatal(err)
	}
	view, err := service.MarkAdminApprovalPending(started.SessionID)
	if err != nil || view.Status != "pending_admin_approval" {
		t.Fatalf("view=%+v err=%v", view, err)
	}
	if status, _ := service.SetupStatus("mail-plugin", requirement); status != plugins.SetupPendingAdminApproval {
		t.Fatalf("setup status=%s", status)
	}
}

func TestHostConnectorSetupCanForceReauthorization(t *testing.T) {
	host := &fakeHostConnectorAuth{status: connectors.ConnectorStatus{State: connectors.StateConnected}}
	service := NewService(nil, nil, nil, mcpclient.NewInMemoryCredentialStore())
	service.SetHostConnectorAuth(host)
	requirement := ir.SetupRequirement{ID: "outlook", Fingerprint: "fp", Kind: ir.SetupHostConnector, ConnectorID: "awaken.microsoft.outlook", Required: true}
	started, err := service.startHostConnector(context.Background(), "mail-plugin", requirement, true)
	if err != nil {
		t.Fatal(err)
	}
	if started.SessionID == "" || host.reauthorized != requirement.ConnectorID {
		t.Fatalf("started=%+v host=%+v", started, host)
	}
}

func TestCleanupPluginDoesNotDisconnectSharedHostConnectorAccount(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	pluginStore := plugins.NewStoreAt(t.TempDir())
	root, err := filepath.Abs(filepath.Join("..", "..", "apps", "desktop", "resources", "builtin-plugins", "awaken-outlook"))
	if err != nil {
		t.Fatal(err)
	}
	source := plugins.Source{Kind: plugins.SourceLocalPath, Path: root, DialectHint: ir.DialectAgentPlugins}
	validated, err := pluginStore.Validate(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pluginStore.Install(context.Background(), plugins.InstallRequest{Source: source, ExpectedDigest: validated.Digest}); err != nil {
		t.Fatal(err)
	}
	host := &fakeHostConnectorAuth{status: connectors.ConnectorStatus{State: connectors.StateConnected}}
	service := NewService(pluginStore, nil, nil, mcpclient.NewInMemoryCredentialStore())
	service.SetHostConnectorAuth(host)
	if err := service.CleanupPlugin("awaken-outlook"); err != nil {
		t.Fatal(err)
	}
	if host.disconnected != "" {
		t.Fatalf("plugin cleanup disconnected shared host account %q", host.disconnected)
	}
}
