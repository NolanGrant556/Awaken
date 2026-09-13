package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"awaken/src/browserhost"
	awruntime "awaken/src/runtime"
)

func TestBrowserHostControlRoute(t *testing.T) {
	t.Cleanup(func() { _ = browserhost.Set(nil) })
	identity := &awruntime.RuntimeIdentity{
		RuntimeIdentity: "test-runtime",
		InstanceID:      "test-instance",
		ControlToken:    "test-token",
		OwnerMode:       "test",
		BaseURL:         "http://localhost",
		StartedAt:       time.Now().UTC(),
	}
	router := NewRouterWithControl(nil, nil, nil, nil, nil, nil, "", nil, &ControlConfig{
		Identity:     identity,
		LeaseManager: awruntime.NewLeaseManager(nil),
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/control/browser-host", bytes.NewReader([]byte(`{"base_url":"http://127.0.0.1:9","token":"secret"}`)))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/control/browser-host", bytes.NewReader([]byte(`{"base_url":"https://example.com","token":"secret"}`)))
	req.Header.Set("Authorization", "Bearer test-token")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/control/browser-host", bytes.NewReader([]byte(`{"base_url":"http://127.0.0.1:9","token":"secret"}`)))
	req.Header.Set("Authorization", "Bearer test-token")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var body struct {
		OK    bool `json:"ok"`
		Ready bool `json:"ready"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Ready {
		t.Fatal("expected ready host")
	}
}
