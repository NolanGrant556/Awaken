package threadhistoryread

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"awaken/src/tool"
)

type historyService struct{ agentID string }

func (historyService) ListArchivedHistory(context.Context, tool.ArchivedHistoryListRequest) (tool.ArchivedHistoryListResult, error) {
	return tool.ArchivedHistoryListResult{}, nil
}
func (historyService) ReadArchivedHistory(context.Context, tool.ArchivedHistoryReadRequest) (tool.ArchivedHistoryReadResult, error) {
	return tool.ArchivedHistoryReadResult{}, nil
}
func (s historyService) ReadCurrentThreadHistory(_ context.Context, request tool.ThreadHistoryReadRequest) (tool.ThreadHistoryReadResult, error) {
	return tool.ThreadHistoryReadResult{ThreadID: request.ThreadID, Items: []tool.ArchivedHistoryItem{{AgentID: s.agentID, Seq: 7, Kind: "user_message", Role: "user", Content: "EXACT_OLD_FACT", Timestamp: time.Now().UTC()}}}, nil
}

func TestHistoryOriginSurvivesStrictOutputContract(t *testing.T) {
	for _, origin := range []string{"", "root", "subagent_owned"} {
		t.Run("origin="+origin, func(t *testing.T) {
			ctx := tool.ContextWithArchivedHistoryService(context.Background(), historyService{agentID: origin})
			ctx = tool.ContextWithToolThreadID(ctx, "thread_host_owned")
			outcome, err := (&Tool{}).ExecuteOutcome(ctx, `{"query":"EXACT_OLD_FACT"}`, nil)
			if err != nil {
				t.Fatal(err)
			}
			normalization := tool.OutcomeNormalizationContext{OutputSchema: (&Tool{}).Definition().OutputSchema, StrictOutputSchema: true}
			outcome = outcome.Normalize(normalization)
			if outcome.IsError() {
				t.Fatalf("valid history rejected: %s", outcome.ErrorMessage)
			}
			var body struct {
				Items []tool.ArchivedHistoryItem `json:"items"`
			}
			if err := json.Unmarshal([]byte(outcome.Content), &body); err != nil {
				t.Fatal(err)
			}
			if len(body.Items) != 1 || body.Items[0].AgentID != origin || body.Items[0].Content != "EXACT_OLD_FACT" {
				t.Fatalf("origin or history was discarded: %+v", body)
			}
			var details map[string]any
			if err := json.Unmarshal([]byte(outcome.Content), &details); err != nil {
				t.Fatal(err)
			}
			details["items"].([]any)[0].(map[string]any)["unexpected_field"] = "must remain rejected"
			outcome.Details = details
			if invalid := outcome.Normalize(normalization); invalid.ErrorCode != "tool_output_schema_mismatch" {
				t.Fatalf("unknown history field bypassed strict schema: %+v", invalid)
			}
		})
	}
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
