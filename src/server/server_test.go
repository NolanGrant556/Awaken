package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	NewRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "ok" {
		t.Fatalf("expected ok body, got %q", body)
	}
}

func TestAGUIDemoEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/agui/demo", nil)
	rec := httptest.NewRecorder()

	NewRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("expected event-stream content type, got %q", got)
	}

	body := rec.Body.String()
	for _, eventType := range []string{
		"RUN_STARTED",
		"TEXT_MESSAGE_CONTENT",
		"TOOL_CALL_START",
		"TOOL_CALL_RESULT",
		"RUN_FINISHED",
	} {
		if !strings.Contains(body, eventType) {
			t.Fatalf("expected body to contain %s, got %s", eventType, body)
		}
	}
}
