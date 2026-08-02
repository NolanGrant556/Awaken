package acp_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"awaken/src/acp"
)

func TestSessionSummaryUsesStableSnakeCaseJSONContract(t *testing.T) {
	data, err := json.Marshal(acp.SessionSummary{
		ThreadID:         "thread-1",
		RunnerID:         "claude_code",
		ACPSessionID:     "session-1",
		State:            acp.ConversationStateFailed,
		LastErrorCode:    acp.ErrCodeTransportClosed,
		LastErrorMessage: "broken pipe",
		StartedAt:        time.Unix(1, 0).UTC(),
		UpdatedAt:        time.Unix(2, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	encoded := string(data)
	for _, field := range []string{"thread_id", "runner_id", "acp_session_id", "session_state", "last_error_code", "last_error_message"} {
		if !strings.Contains(encoded, `"`+field+`"`) {
			t.Fatalf("missing %s in %s", field, encoded)
		}
	}
	if strings.Contains(encoded, "ThreadID") || strings.Contains(encoded, "ACPSessionID") {
		t.Fatalf("Go field names leaked into JSON: %s", encoded)
	}
}
