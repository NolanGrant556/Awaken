package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestControlTokenMiddlewareAcceptsValidToken(t *testing.T) {
	auth := ControlTokenMiddleware("secret-token")
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := auth(inner)

	req := httptest.NewRequest(http.MethodGet, "/v1/control/runtime/identity", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestControlTokenMiddlewareRejectsMissingToken(t *testing.T) {
	auth := ControlTokenMiddleware("secret-token")
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner handler should not be called")
	})
	handler := auth(inner)

	req := httptest.NewRequest(http.MethodGet, "/v1/control/runtime/identity", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("expected WWW-Authenticate Bearer, got %q", rec.Header().Get("WWW-Authenticate"))
	}
}

func TestControlTokenMiddlewareRejectsWrongToken(t *testing.T) {
	auth := ControlTokenMiddleware("secret-token")
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner handler should not be called")
	})
	handler := auth(inner)

	req := httptest.NewRequest(http.MethodGet, "/v1/control/runtime/identity", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestControlTokenMiddlewareSkipsNonControlPaths(t *testing.T) {
	auth := ControlTokenMiddleware("secret-token")
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := auth(inner)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for non-control path, got %d", rec.Code)
	}
}
