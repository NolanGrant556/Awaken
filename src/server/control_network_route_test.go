package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"awaken/src/networkroute"
	awruntime "awaken/src/runtime"
)

func TestNetworkRouteControlRequiresTokenAndHotUpdatesManager(t *testing.T) {
	manager := networkroute.NewFromEnvironment(nil, networkroute.WithEndpointProbe(func(context.Context, string) bool { return true }))
	router := newRouterInternal(routerConfig{control: &ControlConfig{
		Identity:     &awruntime.RuntimeIdentity{ControlToken: "network-route-token"},
		NetworkRoute: manager,
	}})
	body := []byte(`{
		"schema_version":1,
		"generation":2,
		"policy":"auto",
		"http":{"proxies":["http://127.0.0.1:7890"],"direct":true},
		"https":{"proxies":["http://127.0.0.1:7890"],"direct":true},
		"observed_at":"2026-08-06T10:00:00Z"
	}`)

	unauthorized := httptest.NewRequest(http.MethodPut, "/v1/control/network-route", bytes.NewReader(body))
	unauthorizedResult := httptest.NewRecorder()
	router.ServeHTTP(unauthorizedResult, unauthorized)
	if unauthorizedResult.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorizedResult.Code)
	}

	authorized := httptest.NewRequest(http.MethodPut, "/v1/control/network-route", bytes.NewReader(body))
	authorized.Header.Set("Authorization", "Bearer network-route-token")
	authorizedResult := httptest.NewRecorder()
	router.ServeHTTP(authorizedResult, authorized)
	if authorizedResult.Code != http.StatusOK {
		t.Fatalf("authorized status = %d body=%s", authorizedResult.Code, authorizedResult.Body.String())
	}
	if got := manager.Snapshot(); got.Generation != 2 || len(got.HTTPS.Proxies) != 1 {
		t.Fatalf("manager snapshot not updated: %+v", got)
	}
}

func TestAgentPlanMediaToolReloadControlRequiresTokenAndCallsHotReloader(t *testing.T) {
	var reloadedCapability string
	router := newRouterInternal(routerConfig{control: &ControlConfig{
		Identity: &awruntime.RuntimeIdentity{ControlToken: "media-reload-token"},
		ReloadAgentPlanMediaTool: func(capability string) error {
			reloadedCapability = capability
			return nil
		},
	}})
	body := []byte(`{"capability":"media.image.generate"}`)

	unauthorized := httptest.NewRequest(http.MethodPost, "/v1/control/agent-plan/media-tools/reload", bytes.NewReader(body))
	unauthorizedResult := httptest.NewRecorder()
	router.ServeHTTP(unauthorizedResult, unauthorized)
	if unauthorizedResult.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorizedResult.Code)
	}

	authorized := httptest.NewRequest(http.MethodPost, "/v1/control/agent-plan/media-tools/reload", bytes.NewReader(body))
	authorized.Header.Set("Authorization", "Bearer media-reload-token")
	authorizedResult := httptest.NewRecorder()
	router.ServeHTTP(authorizedResult, authorized)
	if authorizedResult.Code != http.StatusOK {
		t.Fatalf("authorized status = %d body=%s", authorizedResult.Code, authorizedResult.Body.String())
	}
	if reloadedCapability != "media.image.generate" {
		t.Fatalf("reloaded capability = %q", reloadedCapability)
	}
}

func TestNetworkRouteControlRejectsUnknownFieldsAndCredentials(t *testing.T) {
	manager := networkroute.NewFromEnvironment(nil)
	handler := controlHandlers{networkRoute: manager}

	for name, body := range map[string]string{
		"unknown field":     `{"schema_version":1,"generation":1,"policy":"auto","http":{"direct":true},"https":{"direct":true},"secret":"no"}`,
		"proxy credentials": `{"schema_version":1,"generation":1,"policy":"auto","http":{"proxies":["http://user:pass@127.0.0.1:7890"],"direct":true},"https":{"direct":true}}`,
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPut, "/v1/control/network-route", strings.NewReader(body))
			result := httptest.NewRecorder()
			handler.handleUpdateNetworkRoute(result, request)
			if result.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s", result.Code, result.Body.String())
			}
		})
	}
}
