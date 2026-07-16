package websearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"awaken/src/tool"
)

type fakeSearchProvider struct {
	request  SearchRequest
	response SearchResponse
	err      error
}

func (p *fakeSearchProvider) Search(_ context.Context, request SearchRequest) (SearchResponse, error) {
	p.request = request
	return p.response, p.err
}

func TestFakeProviderDomainConstraintsAndPostFilter(t *testing.T) {
	provider := &fakeSearchProvider{response: SearchResponse{Provider: "fake", Results: []SearchResult{
		{Title: "Allowed", URL: "https://docs.example.com/a", Snippet: "ok"},
		{Title: "Blocked", URL: "https://other.test/b", Snippet: "no"},
	}}}
	outcome, err := (&WebSearchTool{provider: provider}).ExecuteOutcome(context.Background(), `{"query":"docs","allowed_domains":["EXAMPLE.com"]}`, nil)
	if err != nil || outcome.IsError() {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if len(provider.request.AllowedDomains) != 1 || provider.request.AllowedDomains[0] != "example.com" {
		t.Fatalf("request=%+v", provider.request)
	}
	if outcome.Diagnostic["result_count"] != 1 || !strings.Contains(outcome.Content, "docs.example.com") || strings.Contains(outcome.Content, "other.test") {
		t.Fatalf("outcome=%+v", outcome)
	}
}

func TestAllowedAndBlockedDomainsAreMutuallyExclusive(t *testing.T) {
	outcome, err := (&WebSearchTool{provider: &fakeSearchProvider{}}).ExecuteOutcome(context.Background(), `{"query":"docs","allowed_domains":["example.com"],"blocked_domains":["other.test"]}`, nil)
	if err != nil || outcome.ErrorCode != "web_search_invalid_input" {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
}

func TestExecuteOutcomeReturnsStructuredSearchResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "tool design" {
			t.Fatalf("unexpected query: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"web":{"results":[{"title":"One","url":"https://example.com/1","description":"first"},{"title":"Two","url":"https://example.com/2"}]}}`))
	}))
	defer server.Close()

	instance := &WebSearchTool{client: server.Client(), endpoint: server.URL}
	outcome, err := instance.ExecuteOutcome(context.Background(), "tool design", nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeSucceeded || outcome.Diagnostic["result_count"] != 2 {
		t.Fatalf("unexpected search outcome: %+v", outcome)
	}
	if len(outcome.AffectedPaths) != 0 {
		t.Fatalf("web search must not claim filesystem mutation: %+v", outcome.AffectedPaths)
	}
}

func TestExecuteOutcomeClassifiesAuthorizationFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	instance := &WebSearchTool{client: server.Client(), endpoint: server.URL}
	outcome, err := instance.ExecuteOutcome(context.Background(), "query", nil)
	if err != nil {
		t.Fatalf("authorization failure must be an outcome, got transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeNeedsAuth || outcome.ErrorCode != "web_search_authorization_failed" || outcome.ErrorCategory != tool.ErrorCategoryAuthorization || outcome.Retryable {
		t.Fatalf("unexpected authorization outcome: %+v", outcome)
	}
	if len(outcome.RecoveryOptions) == 0 {
		t.Fatalf("expected authorization recovery: %+v", outcome)
	}
}

func TestExecuteOutcomeClassifiesRateLimitAsRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	instance := &WebSearchTool{client: server.Client(), endpoint: server.URL}
	outcome, err := instance.ExecuteOutcome(context.Background(), "query", nil)
	if err != nil {
		t.Fatalf("rate limit must be an outcome, got transport error: %v", err)
	}
	if outcome.ErrorCode != "web_search_rate_limited" || !outcome.Retryable {
		t.Fatalf("unexpected rate-limit outcome: %+v", outcome)
	}
}
