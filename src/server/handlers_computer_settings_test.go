package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"awaken/src/computer"
)

func TestComputerSettingsUpdateRefreshesRuntimeSettings(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	store, err := computer.NewSettingsStore()
	if err != nil {
		t.Fatal(err)
	}
	updated := make(chan computer.Settings, 1)
	handler := computerSettingsHandlers{
		store: store,
		onUpdated: func(settings computer.Settings) {
			updated <- settings
		},
	}
	request := httptest.NewRequest(
		http.MethodPatch,
		"/v1/computer/settings",
		strings.NewReader(`{"expected_revision":0,"enabled":true}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.handleUpdate(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	select {
	case settings := <-updated:
		if !settings.Enabled {
			t.Fatal("expected updated settings callback to receive enabled=true")
		}
	default:
		t.Fatal("expected runtime settings callback")
	}
}
