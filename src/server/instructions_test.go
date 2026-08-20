package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"awaken/src/instructions"
	awruntime "awaken/src/runtime"
)

func TestGlobalInstructionsAPIUsesRevisionCAS(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	router := NewRouter(awruntime.NewRunService(nil, nil), nil)

	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/v1/instructions/global", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", get.Code, get.Body.String())
	}
	var missing instructions.GlobalAgentsDocument
	if err := json.Unmarshal(get.Body.Bytes(), &missing); err != nil || missing.Exists {
		t.Fatalf("missing=%+v err=%v", missing, err)
	}

	putBody := []byte(`{"expected_revision":"","content":"global api rule"}`)
	put := httptest.NewRecorder()
	router.ServeHTTP(put, httptest.NewRequest(http.MethodPut, "/v1/instructions/global", bytes.NewReader(putBody)))
	if put.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", put.Code, put.Body.String())
	}
	var created instructions.GlobalAgentsDocument
	if err := json.Unmarshal(put.Body.Bytes(), &created); err != nil || !created.Exists || created.Revision == "" {
		t.Fatalf("created=%+v err=%v", created, err)
	}

	conflict := httptest.NewRecorder()
	router.ServeHTTP(conflict, httptest.NewRequest(http.MethodPut, "/v1/instructions/global", bytes.NewReader([]byte(`{"expected_revision":"stale","content":"lost"}`))))
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflict status=%d body=%s", conflict.Code, conflict.Body.String())
	}
}
