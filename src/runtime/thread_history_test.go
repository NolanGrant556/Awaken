package runtime

import (
	"context"
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
	if _, err := service.ReadCurrentThreadHistory(context.Background(), tool.ThreadHistoryReadRequest{
		ThreadID: "thread_current_history", BeforeSeq: 10, AfterSeq: 1,
	}); archivedHistoryErrorCode(err) != tool.ThreadHistoryErrorInvalidInput {
		t.Fatalf("mixed sequence range error=%v", err)
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
