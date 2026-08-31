package threadhistoryread

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"awaken/src/tool"
)

type historyService struct{}

func (historyService) ListArchivedHistory(context.Context, tool.ArchivedHistoryListRequest) (tool.ArchivedHistoryListResult, error) {
	return tool.ArchivedHistoryListResult{}, nil
}
func (historyService) ReadArchivedHistory(context.Context, tool.ArchivedHistoryReadRequest) (tool.ArchivedHistoryReadResult, error) {
	return tool.ArchivedHistoryReadResult{}, nil
}
func (historyService) ReadCurrentThreadHistory(_ context.Context, request tool.ThreadHistoryReadRequest) (tool.ThreadHistoryReadResult, error) {
	return tool.ThreadHistoryReadResult{ThreadID: request.ThreadID, Items: []tool.ArchivedHistoryItem{{Seq: 7, Kind: "user_message", Role: "user", Content: "EXACT_OLD_FACT", Timestamp: time.Now().UTC()}}}, nil
}

func TestThreadHistoryReadUsesHostThreadIdentityAndSanitizesDurableReceipt(t *testing.T) {
	ctx := tool.ContextWithArchivedHistoryService(context.Background(), historyService{})
	ctx = tool.ContextWithToolThreadID(ctx, "thread_host_owned")
	outcome, err := (&Tool{}).ExecuteOutcome(ctx, `{"query":"EXACT_OLD_FACT"}`, nil)
	if err != nil || outcome.IsError() || !strings.Contains(outcome.Content, "EXACT_OLD_FACT") || !strings.Contains(outcome.Content, "thread_host_owned") {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	sanitized := (&Tool{}).SemanticJournalResult(outcome.Content)
	if strings.Contains(sanitized, "EXACT_OLD_FACT") || !strings.Contains(sanitized, `"item_count":1`) {
		t.Fatalf("durable receipt leaked body or lost count: %s", sanitized)
	}
	sanitizedOutcome := (&Tool{}).SemanticJournalOutcome(outcome, sanitized)
	encoded, _ := json.Marshal(sanitizedOutcome)
	if strings.Contains(string(encoded), "EXACT_OLD_FACT") {
		t.Fatalf("durable outcome leaked history: %s", encoded)
	}
}
