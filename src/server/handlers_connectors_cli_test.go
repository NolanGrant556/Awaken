package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"awaken/src/connectors"
	"awaken/src/mcpclient"
)

type cliConnectorTestRuntime struct{}

func (cliConnectorTestRuntime) InjectDynamic(string, mcpclient.ScopedMcpServerConfig)    {}
func (cliConnectorTestRuntime) RemoveDynamic(string, string)                             {}
func (cliConnectorTestRuntime) Start(context.Context, []mcpclient.ScopedMcpServerConfig) {}
func (cliConnectorTestRuntime) Remove(string) error                                      { return nil }
func (cliConnectorTestRuntime) Snapshot() []mcpclient.ServerSnapshot                     { return nil }
func (cliConnectorTestRuntime) SnapshotServer(string) (mcpclient.ServerSnapshot, error) {
	return mcpclient.ServerSnapshot{}, mcpclient.ErrServerNotFound
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
