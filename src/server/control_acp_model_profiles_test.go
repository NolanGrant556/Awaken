package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	awruntime "awaken/src/runtime"
)

func TestControlACPModelProfilesReturnsOnlyNonSensitiveCatalog(t *testing.T) {
	id, err := awruntime.GenerateRuntimeIdentity(t.TempDir(), "headless", "http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	control := &ControlConfig{
		Identity:          id,
		ACPSessionManager: awruntime.NewACPSessionManager(nil, nil),
		ModelProfileStore: fakeACPModelProfileStore{},
	}
	router := NewRouterWithControl(nil, NewLLMRuntimeInfoProvider("", "", "", "", ""), nil, nil, nil, nil, "", nil, control)
	req := httptest.NewRequest(http.MethodGet, "/v1/control/acp/model-profiles", nil)
	req.Header.Set("Authorization", "Bearer "+id.ControlToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["default_profile_id"] != "profile-default" {
		t.Fatalf("unexpected default: %v", body["default_profile_id"])
	}
	for _, forbidden := range []string{"api_key", "base_url", "auth_scheme"} {
		if strings.Contains(rec.Body.String(), forbidden) {
			t.Fatalf("response leaked forbidden field %q: %s", forbidden, rec.Body.String())
		}
	}
}
