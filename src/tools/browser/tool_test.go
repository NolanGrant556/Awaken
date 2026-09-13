package browser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"awaken/src/browserhost"
	"awaken/src/tool"
)

func TestBrowserToolsOmitWithoutHostAndFailClosed(t *testing.T) {
	t.Cleanup(func() { _ = browserhost.Set(nil) })
	_ = browserhost.Set(nil)
	registered, err := tool.GetRegisteredTool(ToolNavigate)
	if err != nil {
		t.Fatal(err)
	}
	filter, ok := registered.Runtime.(tool.RunSnapshotFilter)
	if !ok {
		t.Fatal("expected IncludeInRunSnapshot")
	}
	if filter.IncludeInRunSnapshot(tool.ExecutionEnvironmentSnapshot{}) {
		t.Fatal("navigate should be omitted without a Desktop host")
	}
	outcome, err := registered.Runtime.(tool.OutcomeTool).ExecuteOutcome(context.Background(), `{"url":"example.com"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.IsError() || outcome.ErrorCode != "browser_host_unavailable" {
		t.Fatalf("got %+v", outcome)
	}
}

func TestBrowserNavigateRequiresThreadAndCallsHost(t *testing.T) {
	t.Cleanup(func() { _ = browserhost.Set(nil) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request browserhost.ActionRequest
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request.Action != "navigate" || request.ThreadID != "thread-1" || request.URL != "example.com" {
			t.Errorf("unexpected %+v", request)
		}
		_ = json.NewEncoder(w).Encode(browserhost.Result{OK: true, URL: "https://example.com/", Title: "Example"})
	}))
	t.Cleanup(server.Close)
	if err := browserhost.Set(&browserhost.Registration{BaseURL: server.URL, Token: "token"}); err != nil {
		t.Fatal(err)
	}
	registered, err := tool.GetRegisteredTool(ToolNavigate)
	if err != nil {
		t.Fatal(err)
	}
	runtime := registered.Runtime.(tool.OutcomeTool)
	missingThread, err := runtime.ExecuteOutcome(context.Background(), `{"url":"example.com"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if missingThread.ErrorCode != "thread_id_required" {
		t.Fatalf("got %+v", missingThread)
	}
	ctx := tool.ContextWithToolThreadID(context.Background(), "thread-1")
	outcome, err := runtime.ExecuteOutcome(ctx, `{"url":"example.com"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.IsError() || outcome.Content == "" {
		t.Fatalf("got %+v", outcome)
	}
}

func TestBrowserClickMapsOriginLeaseFailure(t *testing.T) {
	t.Cleanup(func() { _ = browserhost.Set(nil) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(browserhost.Result{OK: false, Error: "origin_lease_expired"})
	}))
	t.Cleanup(server.Close)
	if err := browserhost.Set(&browserhost.Registration{BaseURL: server.URL, Token: "token"}); err != nil {
		t.Fatal(err)
	}
	registered, err := tool.GetRegisteredTool(ToolClick)
	if err != nil {
		t.Fatal(err)
	}
	ctx := tool.ContextWithToolThreadID(context.Background(), "thread-1")
	outcome, err := registered.Runtime.(tool.OutcomeTool).ExecuteOutcome(ctx, `{"ref":"[1]"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.IsError() || outcome.ErrorCode != "origin_lease_expired" {
		t.Fatalf("got %+v", outcome)
	}
}
