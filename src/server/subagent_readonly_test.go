package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	awruntime "awaken/src/runtime"
)

func TestSubagentConversationControlRoutesAreRemoved(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	router := NewRouter(awruntime.NewRunService(nil, nil), nil)
	for _, action := range []string{"messages", "followup", "close"} {
		record := httptest.NewRecorder()
		router.ServeHTTP(record, httptest.NewRequest(http.MethodPost, "/v1/runs/run/subagents/task/"+action, strings.NewReader(`{"prompt":"do work","content":"do work"}`)))
		if record.Code != http.StatusNotFound {
			t.Fatalf("%s still has a control handler: %d %s", action, record.Code, record.Body.String())
		}
	}
}
