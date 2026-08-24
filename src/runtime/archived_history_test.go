package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"awaken/src/agentcore"
	"awaken/src/tool"
)

func TestArchivedHistoryListAndReadStayInsideArchivedAuthority(t *testing.T) {
	setFileStoreTestRoots(t)
	store, ok := newThreadStore().(*FileDesktopStore)
	if !ok {
		t.Fatal("expected FileDesktopStore")
	}
	base := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	workspace := WorkspaceConfig{WorkspaceRoot: t.TempDir(), ProjectID: "project_1", ProjectName: "Pricing Project"}
	createArchivedHistoryFixture(t, store, ThreadSnapshot{
		ThreadID: "thread_pricing", ProjectID: "project_1", Label: "Pricing decisions", RunID: "run_pricing",
		Workspace: workspace, CreatedAt: base, UpdatedAt: base.Add(time.Minute),
	}, base.Add(4*time.Hour), true)
	createArchivedHistoryFixture(t, store, ThreadSnapshot{
		ThreadID: "thread_other", ProjectID: "project_2", Label: "Other notes", RunID: "run_other",
		Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir(), ProjectID: "project_2", ProjectName: "Other Project"},
		CreatedAt: base.Add(time.Minute), UpdatedAt: base.Add(2 * time.Minute),
	}, base.Add(5*time.Hour), true)
	createArchivedHistoryFixture(t, store, ThreadSnapshot{
		ThreadID: "thread_active", ProjectID: "project_1", Label: "Active pricing", RunID: "run_active",
		Workspace: workspace, CreatedAt: base.Add(2 * time.Minute), UpdatedAt: base.Add(3 * time.Minute),
	}, time.Time{}, false)

	service := NewRunService(nil, nil)
	first, err := service.ListArchivedHistory(context.Background(), tool.ArchivedHistoryListRequest{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Threads) != 1 || first.Threads[0].ThreadID != "thread_other" || first.NextCursor == "" {
		t.Fatalf("first page=%+v", first)
	}
	second, err := service.ListArchivedHistory(context.Background(), tool.ArchivedHistoryListRequest{Limit: 1, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Threads) != 1 || second.Threads[0].ThreadID != "thread_pricing" || second.NextCursor != "" {
		t.Fatalf("second page=%+v", second)
	}
	matched, err := service.ListArchivedHistory(context.Background(), tool.ArchivedHistoryListRequest{Query: "pricing project", ProjectID: "project_1"})
	if err != nil || len(matched.Threads) != 1 || matched.Threads[0].ThreadID != "thread_pricing" {
		t.Fatalf("matched=%+v err=%v", matched, err)
	}
	if _, err := service.ListArchivedHistory(context.Background(), tool.ArchivedHistoryListRequest{Query: "changed", Cursor: first.NextCursor}); archivedHistoryErrorCode(err) != tool.ArchivedHistoryErrorInvalidCursor {
		t.Fatalf("changed-filter cursor error=%v", err)
	}

	if _, err := service.ReadArchivedHistory(context.Background(), tool.ArchivedHistoryReadRequest{ThreadID: "thread_active"}); archivedHistoryErrorCode(err) != tool.ArchivedHistoryErrorNotFound {
		t.Fatalf("active thread must not be readable through archive tool: %v", err)
	}
	page, err := service.ReadArchivedHistory(context.Background(), tool.ArchivedHistoryReadRequest{ThreadID: "thread_pricing", Limit: 2, MaxCharsPerItem: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].Content != "second question" || page.Items[1].Content != "second answer" || page.NextBeforeSeq == 0 {
		t.Fatalf("latest page=%+v", page)
	}
	older, err := service.ReadArchivedHistory(context.Background(), tool.ArchivedHistoryReadRequest{
		ThreadID: "thread_pricing", BeforeSeq: page.NextBeforeSeq, Limit: 20, MaxCharsPerItem: 100, IncludeToolActivity: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(older.Items) != 4 {
		t.Fatalf("older items=%+v", older.Items)
	}
	if older.Items[1].Kind != string(agentcore.SemanticKindToolCall) || older.Items[1].Content != "" || older.Items[1].ToolName != "Read" {
		t.Fatalf("tool call must expose metadata without arguments: %+v", older.Items[1])
	}
	if older.Items[2].Kind != string(agentcore.SemanticKindToolResult) || !older.Items[2].ContentTruncated || len([]rune(older.Items[2].Content)) != 100 {
		t.Fatalf("tool result must be bounded: %+v", older.Items[2])
	}
	if strings.Contains(older.Items[1].Content, "secret") {
		t.Fatalf("tool arguments leaked: %+v", older.Items[1])
	}
}

func createArchivedHistoryFixture(t *testing.T, store *FileDesktopStore, snapshot ThreadSnapshot, archivedAt time.Time, archived bool) {
	t.Helper()
	if err := store.CreateThread(snapshot, "profile", snapshot.Label); err != nil {
		t.Fatal(err)
	}
	journal, err := NewExecutionJournal(store.ThreadTranscriptPath(snapshot.ThreadID), snapshot.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	longToolResult := strings.Repeat("x", 140)
	records := []agentcore.SemanticRecord{
		agentcore.NewSemanticRecord(agentcore.SemanticKindRunStarted, snapshot.ThreadID, snapshot.RunID, "turn_1", "", "session_1", map[string]any{}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindUserMessage, snapshot.ThreadID, snapshot.RunID, "turn_1", "", "session_1", map[string]any{"content": "first question"}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindToolCall, snapshot.ThreadID, snapshot.RunID, "turn_1", "", "session_1", map[string]any{"tool_call_id": "call_1", "tool_name": "Read", "arguments": `{"path":"secret"}`}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindToolResult, snapshot.ThreadID, snapshot.RunID, "turn_1", "call_1", "session_1", map[string]any{"tool_call_id": "call_1", "tool_name": "Read", "content": longToolResult}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindAssistantMessage, snapshot.ThreadID, snapshot.RunID, "turn_1", "", "session_1", map[string]any{"content": "first answer"}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindUserMessage, snapshot.ThreadID, snapshot.RunID, "turn_2", "", "session_1", map[string]any{"content": "second question"}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindAssistantMessage, snapshot.ThreadID, snapshot.RunID, "turn_2", "", "session_1", map[string]any{"content": "second answer"}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindRunTerminal, snapshot.ThreadID, snapshot.RunID, "turn_2", "", "session_1", map[string]any{"status": "finished", "result": "done"}),
	}
	if err := journal.AppendBatch(context.Background(), records); err != nil {
		t.Fatal(err)
	}
	if err := journal.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	if archived {
		if _, err := store.SetThreadArchived(snapshot.ThreadID, true, archivedAt); err != nil {
			t.Fatal(err)
		}
	}
}

func archivedHistoryErrorCode(err error) string {
	var historyErr *tool.ArchivedHistoryError
	if errors.As(err, &historyErr) {
		return historyErr.Code
	}
	return ""
}
