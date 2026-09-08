package runtime

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"awaken/src/tool"
)

func TestReadCurrentThreadHistoryIsBoundedQueryableAndCurrentThreadOnly(t *testing.T) {
	setFileStoreTestRoots(t)
	store, ok := newThreadStore().(*FileDesktopStore)
	if !ok {
		t.Fatal("expected FileDesktopStore")
	}
	base := time.Date(2026, 8, 31, 1, 0, 0, 0, time.UTC)
	createArchivedHistoryFixture(t, store, ThreadSnapshot{
		ThreadID: "thread_current_history", ProjectID: "project_1", Label: "Current history", RunID: "run_current_history",
		Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir(), ProjectID: "project_1"}, CreatedAt: base, UpdatedAt: base,
	}, time.Time{}, false)
	service := NewRunService(nil, nil)
	matched, err := service.ReadCurrentThreadHistory(context.Background(), tool.ThreadHistoryReadRequest{
		ThreadID: "thread_current_history", Query: "second question", Limit: 20, MaxCharsPerItem: 100,
	})
	if err != nil || len(matched.Items) != 1 || matched.Items[0].Content != "second question" {
		t.Fatalf("query result=%+v err=%v", matched, err)
	}
	page, err := service.ReadCurrentThreadHistory(context.Background(), tool.ThreadHistoryReadRequest{
		ThreadID: "thread_current_history", Limit: 2, MaxCharsPerItem: 100,
	})
	if err != nil || len(page.Items) != 2 || page.NextBeforeSeq == 0 {
		t.Fatalf("latest page=%+v err=%v", page, err)
	}
	older, err := service.ReadCurrentThreadHistory(context.Background(), tool.ThreadHistoryReadRequest{
		ThreadID: "thread_current_history", BeforeSeq: page.NextBeforeSeq, Limit: 2, MaxCharsPerItem: 100,
	})
	if err != nil || len(older.Items) != 2 || older.Items[0].Content != "first question" || older.Items[1].Content != "first answer" || older.NextBeforeSeq != 0 {
		t.Fatalf("older page=%+v err=%v", older, err)
	}
	forward, err := service.ReadCurrentThreadHistory(context.Background(), tool.ThreadHistoryReadRequest{
		ThreadID: "thread_current_history", AfterSeq: older.Items[1].Seq, Limit: 1, MaxCharsPerItem: 100,
	})
	if err != nil || len(forward.Items) != 1 || forward.Items[0].Content != "second question" || forward.NextAfterSeq != forward.Items[0].Seq {
		t.Fatalf("forward page=%+v err=%v", forward, err)
	}
	if _, err := service.ReadCurrentThreadHistory(context.Background(), tool.ThreadHistoryReadRequest{
		ThreadID: "thread_current_history", BeforeSeq: 10, AfterSeq: 1,
	}); archivedHistoryErrorCode(err) != tool.ThreadHistoryErrorInvalidInput {
		t.Fatalf("mixed sequence range error=%v", err)
	}
}

func TestThreadHistoryBudgetPreservesOrderAndInput(t *testing.T) {
	items := []tool.ArchivedHistoryItem{
		{Seq: 1, Content: "甲乙丙"},
		{Seq: 2, Content: "🙂ab"},
		{Seq: 3, Content: "丁戊己"},
	}
	for _, test := range []struct {
		name       string
		budget     int
		preferTail bool
		want       []tool.ArchivedHistoryItem
		truncated  bool
	}{
		{"head_partial", 5, false, []tool.ArchivedHistoryItem{{Seq: 1, Content: "甲乙丙"}, {Seq: 2, Content: "🙂a", ContentTruncated: true}}, true},
		{"tail_partial", 5, true, []tool.ArchivedHistoryItem{{Seq: 2, Content: "🙂a", ContentTruncated: true}, {Seq: 3, Content: "丁戊己"}}, true},
		{"head_exact_item", 3, false, items[:1], true},
		{"tail_exact_item", 3, true, items[2:], true},
		{"head_full_page", 9, false, items, false},
		{"tail_full_page", 9, true, items, false},
		{"head_spare_budget", 12, false, items, false},
		{"tail_spare_budget", 12, true, items, false},
		{"zero_budget", 0, false, []tool.ArchivedHistoryItem{}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := slices.Clone(items)
			got, truncated := boundThreadHistoryItems(input, test.budget, test.preferTail)
			if !reflect.DeepEqual(got, test.want) || truncated != test.truncated {
				t.Fatalf("got %+v truncated=%v; want %+v truncated=%v", got, truncated, test.want, test.truncated)
			}
			if !reflect.DeepEqual(input, items) {
				t.Fatal("budgeting modified the source history")
			}
		})
	}
	if got, truncated := boundThreadHistoryItems(nil, 5, true); len(got) != 0 || truncated {
		t.Fatalf("empty history=%+v truncated=%v", got, truncated)
	}
}

func TestCurrentThreadHistorySanitizesSecretsAndEnforcesTotalBudget(t *testing.T) {
	setFileStoreTestRoots(t)
	storeRoot := resolveRuntimeStoreRoot()
	secret := sanitizeCurrentThreadHistoryContent(
		`Authorization: Bearer secret-token+/= sk-secret ghp_private api_key=plain-secret password:'hidden value' client_secret="client value" ` +
			`aws_secret_access_key=aws-private AIzaExample hf_private npm_private pypi-private passphrase='two words' ` + storeRoot + "/store-generations/private",
	)
	for _, leaked := range []string{"secret-token", "sk-secret", "ghp_private", "plain-secret", "hidden value", "client value", "aws-private", "AIzaExample", "hf_private", "npm_private", "pypi-private", "two words"} {
		if strings.Contains(secret, leaked) {
			t.Fatalf("history secret %q was not redacted: %q", leaked, secret)
		}
	}
	if strings.Count(secret, "[REDACTED]") < 10 {
		t.Fatalf("history secret was not redacted: %q", secret)
	}
	if storeRoot != "" && strings.Contains(secret, storeRoot) || !strings.Contains(secret, "<AWAKEN_THREAD_STORE>") {
		t.Fatalf("runtime store root was not redacted: %q", secret)
	}
	items := []tool.ArchivedHistoryItem{
		{Seq: 1, Content: strings.Repeat("a", 800)},
		{Seq: 2, Content: strings.Repeat("b", 800)},
		{Seq: 3, Content: strings.Repeat("c", 800)},
	}
	bounded, truncated := boundThreadHistoryItems(items, 1000, true)
	if !truncated || len(bounded) != 2 || bounded[len(bounded)-1].Seq != 3 {
		t.Fatalf("tail-prioritized total budget failed: %+v truncated=%v", bounded, truncated)
	}
	total := 0
	for _, item := range bounded {
		total += len([]rune(item.Content))
	}
	if total > 1000 {
		t.Fatalf("history exceeded total character budget: %d", total)
	}
}
