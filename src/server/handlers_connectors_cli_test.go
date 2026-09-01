package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"awaken/src/connectorauth"
	"awaken/src/connectors"
	"awaken/src/mcpclient"
)

func TestConnectorAdminApprovalRouteProjectsPersistentState(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	registry := connectors.NewRegistry(cliConnectorTestRuntime{}, nil)
	if err := registry.LoadSources(context.Background(), []connectors.ConnectorSource{connectors.NewBuiltinSource(connectors.BuiltinOptions{MicrosoftClientID: "public-client"})}); err != nil {
		t.Fatal(err)
	}
	auth := connectorauth.NewService(registry, mcpclient.NewInMemoryCredentialStore())
	registry.SetAccountReadinessProvider(auth)
	handler := newRouterInternal(routerConfig{connectorRegistry: registry, connectorAuth: auth})

	started := httptest.NewRecorder()
	handler.ServeHTTP(started, httptest.NewRequest(http.MethodPost, "/v1/connectors/awaken.microsoft.outlook/connect", strings.NewReader(`{}`)))
	if started.Code != http.StatusOK {
		t.Fatalf("start status=%d body=%s", started.Code, started.Body.String())
	}
	var payload struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(started.Body.Bytes(), &payload); err != nil || payload.SessionID == "" {
		t.Fatalf("start payload=%s err=%v", started.Body.String(), err)
	}
	pending := httptest.NewRecorder()
	handler.ServeHTTP(pending, httptest.NewRequest(http.MethodPost, "/v1/connectors/auth/sessions/"+payload.SessionID+"/admin-approval", nil))
	if pending.Code != http.StatusOK || !strings.Contains(pending.Body.String(), `"status":"pending_admin_approval"`) {
		t.Fatalf("pending status=%d body=%s", pending.Code, pending.Body.String())
	}
	listed := httptest.NewRecorder()
	handler.ServeHTTP(listed, httptest.NewRequest(http.MethodGet, "/v1/connectors", nil))
	if listed.Code != http.StatusOK || strings.Contains(listed.Body.String(), `"id":"awaken.microsoft.outlook"`) {
		t.Fatalf("list status=%d body=%s", listed.Code, listed.Body.String())
	}
	status, err := registry.Status("awaken.microsoft.outlook")
	if err != nil || status.State != connectors.StateApprovalRequired {
		t.Fatalf("Outlook runtime status=%+v err=%v", status, err)
	}
}

type cliConnectorTestRuntime struct{}

func (cliConnectorTestRuntime) InjectDynamic(string, mcpclient.ScopedMcpServerConfig)    {}
func (cliConnectorTestRuntime) RemoveDynamic(string, string)                             {}
func (cliConnectorTestRuntime) Start(context.Context, []mcpclient.ScopedMcpServerConfig) {}
func (cliConnectorTestRuntime) Remove(string) error                                      { return nil }
func (cliConnectorTestRuntime) Snapshot() []mcpclient.ServerSnapshot                     { return nil }
func (cliConnectorTestRuntime) SnapshotServer(string) (mcpclient.ServerSnapshot, error) {
	return mcpclient.ServerSnapshot{}, mcpclient.ErrServerNotFound
}

func TestOutlookConnectorAuthFailsClosedWithoutHostClientID(t *testing.T) {
	registry := connectors.NewRegistry(cliConnectorTestRuntime{}, nil)
	if err := registry.LoadSources(context.Background(), []connectors.ConnectorSource{connectors.NewBuiltinSource()}); err != nil {
		t.Fatal(err)
	}
	auth := connectorauth.NewService(registry, mcpclient.NewInMemoryCredentialStore())
	registry.SetAccountReadinessProvider(auth)
	handler := newRouterInternal(routerConfig{connectorRegistry: registry, connectorAuth: auth})

	list := httptest.NewRequest(http.MethodGet, "/v1/connectors", nil)
	listRecorder := httptest.NewRecorder()
	handler.ServeHTTP(listRecorder, list)
	if listRecorder.Code != http.StatusOK || strings.Contains(listRecorder.Body.String(), `"id":"awaken.microsoft.outlook"`) {
		t.Fatalf("Outlook catalog state: status=%d body=%s", listRecorder.Code, listRecorder.Body.String())
	}
	status, statusErr := registry.Status("awaken.microsoft.outlook")
	if statusErr != nil || status.State != connectors.StateFailed {
		t.Fatalf("Outlook runtime state: status=%+v err=%v", status, statusErr)
	}

	connect := httptest.NewRequest(http.MethodPost, "/v1/connectors/awaken.microsoft.outlook/connect", strings.NewReader(`{}`))
	connect.Header.Set("Content-Type", "application/json")
	connectRecorder := httptest.NewRecorder()
	handler.ServeHTTP(connectRecorder, connect)
	if connectRecorder.Code != http.StatusServiceUnavailable || !strings.Contains(connectRecorder.Body.String(), "CONNECTOR_OAUTH_CLIENT_ID_REQUIRED") {
		t.Fatalf("Outlook connect did not fail closed: status=%d body=%s", connectRecorder.Code, connectRecorder.Body.String())
	}
}
func (cliConnectorTestRuntime) ClearCredentialsFor(mcpclient.ScopedMcpServerConfig) error { return nil }

func TestCLIReadinessRouteRequiresControlToken(t *testing.T) {
	registry := connectors.NewRegistry(cliConnectorTestRuntime{}, nil)
	if err := registry.LoadSources(context.Background(), []connectors.ConnectorSource{connectors.NewBuiltinSource()}); err != nil {
		t.Fatal(err)
	}
	registry.SetCLIControlToken("control-secret")
	handler := newRouterInternal(routerConfig{connectorRegistry: registry})
	body := `{"providerId":"lark-cli","activation":"enabled","install":"installed","executable":"available","configuration":"ready","authentication":"authorized","overall":"ready","updatedAt":"2026-07-17T00:00:00Z"}`

	unauthorized := httptest.NewRequest(http.MethodPut, "/v1/connectors/lark-cli/cli-readiness", strings.NewReader(body))
	unauthorized.Header.Set("Content-Type", "application/json")
	unauthorizedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedRecorder, unauthorized)
	if unauthorizedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized readiness update status=%d body=%s", unauthorizedRecorder.Code, unauthorizedRecorder.Body.String())
	}

	authorized := httptest.NewRequest(http.MethodPut, "/v1/connectors/lark-cli/cli-readiness", strings.NewReader(body))
	authorized.Header.Set("Content-Type", "application/json")
	authorized.Header.Set("X-Awaken-Control-Token", "control-secret")
	authorizedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(authorizedRecorder, authorized)
	if authorizedRecorder.Code != http.StatusOK {
		t.Fatalf("authorized readiness update status=%d body=%s", authorizedRecorder.Code, authorizedRecorder.Body.String())
	}

	connect := httptest.NewRequest(http.MethodPost, "/v1/connectors/lark-cli/connect", strings.NewReader(`{}`))
	connectRecorder := httptest.NewRecorder()
	handler.ServeHTTP(connectRecorder, connect)
	if connectRecorder.Code != http.StatusUnprocessableEntity || !strings.Contains(connectRecorder.Body.String(), "CONNECTOR_ACTION_UNSUPPORTED") {
		t.Fatalf("CLI connector entered MCP connect route: status=%d body=%s", connectRecorder.Code, connectRecorder.Body.String())
	}
}
