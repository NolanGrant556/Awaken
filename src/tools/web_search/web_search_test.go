package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["query"] != "tool design" || r.Header.Get("X-Tavily-Access-Mode") != "keyless" {
			t.Fatalf("unexpected request: %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"One","url":"https://example.com/1","content":"first"},{"title":"Two","url":"https://example.com/2"}]}`))
	}))
	defer server.Close()

	instance := New(NewTavilyProvider(server.Client(), server.URL))
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

	instance := New(NewTavilyProvider(server.Client(), server.URL))
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

	instance := New(NewTavilyProvider(server.Client(), server.URL))
	outcome, err := instance.ExecuteOutcome(context.Background(), "query", nil)
	if err != nil {
		t.Fatalf("rate limit must be an outcome, got transport error: %v", err)
	}
	if outcome.ErrorCode != "web_search_rate_limited" || !outcome.Retryable {
		t.Fatalf("unexpected rate-limit outcome: %+v", outcome)
	}
}

func TestExaProviderParsesSSEWithoutCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "" {
			t.Fatalf("Exa keyless requests must not send x-api-key")
		}
		var call struct {
			Params struct {
				Name      string `json:"name"`
				Arguments struct {
					Query      string `json:"query"`
					NumResults int    `json:"numResults"`
				} `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
			t.Fatal(err)
		}
		if call.Params.Name != "web_search_exa" || call.Params.Arguments.Query != "Go release notes (site:go.dev)" || call.Params.Arguments.NumResults != maximumResults {
			t.Fatalf("unexpected call: %+v", call)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message\n" +
			`data: {"result":{"content":[{"type":"text","text":"Title: Go Releases\nURL: https://go.dev/doc/devel/release\nPublished: N/A\nAuthor: N/A\nHighlights:\nCurrent release information.\n\n---\n\nTitle: Other\nURL: https://example.com/no\nHighlights:\nFiltered."}]}}` +
			"\n\n"))
	}))
	defer server.Close()

	outcome, err := New(NewExaProvider(server.Client(), server.URL)).ExecuteOutcome(
		context.Background(),
		`{"query":"Go release notes","allowed_domains":["go.dev"]}`,
		nil,
	)
	if err != nil || outcome.IsError() {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if outcome.Details["provider"] != "exa" || outcome.Details["result_count"] != 1 {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
}

func TestExaProviderParsesDirectJSONAndNoResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"No matching sources."}]}}`))
	}))
	defer server.Close()
	outcome, err := New(NewExaProvider(server.Client(), server.URL)).ExecuteOutcome(context.Background(), "unlikely query", nil)
	if err != nil || outcome.IsError() {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if outcome.Content != "No web results found." || outcome.Details["result_count"] != 0 {
		t.Fatalf("unexpected no-results outcome: %+v", outcome)
	}
}

func TestExaProviderClassifiesMalformedAndJSONRPCResponses(t *testing.T) {
	for _, responseBody := range []string{
		"not-json",
		`{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"invalid arguments"}}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(responseBody))
		}))
		outcome, err := New(NewExaProvider(server.Client(), server.URL)).ExecuteOutcome(context.Background(), "query", nil)
		server.Close()
		if err != nil || outcome.ErrorCode != "web_search_response_invalid" {
			t.Fatalf("body=%q outcome=%+v err=%v", responseBody, outcome, err)
		}
	}
}

func TestExaProviderRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maximumResponseBytes+1)))
	}))
	defer server.Close()
	outcome, err := New(NewExaProvider(server.Client(), server.URL)).ExecuteOutcome(context.Background(), "query", nil)
	if err != nil || outcome.ErrorCode != "web_search_response_invalid" {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
}

func TestProviderSelectionFromEnvironment(t *testing.T) {
	original := map[string]string{}
	for _, name := range []string{"AWAKEN_WEB_SEARCH_PROVIDERS"} {
		original[name] = os.Getenv(name)
		t.Cleanup(func() { _ = os.Setenv(name, original[name]) })
		_ = os.Unsetenv(name)
	}

	if _, ok := NewProviderFromEnvironment().(*ExaProvider); !ok {
		t.Fatalf("default provider must be Exa")
	}
	t.Setenv("AWAKEN_WEB_SEARCH_PROVIDERS", "tavily")
	if _, ok := NewProviderFromEnvironment().(*TavilyProvider); !ok {
		t.Fatalf("explicit Tavily selection must be used")
	}
	t.Setenv("AWAKEN_WEB_SEARCH_PROVIDERS", "exa,tavily")
	if _, ok := NewProviderFromEnvironment().(fallbackProvider); !ok {
		t.Fatalf("multiple enabled providers must use ordered fallback")
	}
	t.Setenv("AWAKEN_WEB_SEARCH_PROVIDERS", "")
	if _, ok := NewProviderFromEnvironment().(disabledProvider); !ok {
		t.Fatalf("an explicitly empty provider list must disable WebSearch")
	}
}

func TestTavilyProviderUsesKeylessDomainsAndParsesResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Tavily-Access-Mode") != "keyless" {
			t.Fatalf("missing Tavily keyless header")
		}
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("keyless Tavily requests must not send Authorization")
		}
		var request struct {
			Query          string   `json:"query"`
			MaxResults     int      `json:"max_results"`
			IncludeDomains []string `json:"include_domains"`
			ExcludeDomains []string `json:"exclude_domains"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Query != "Go release notes" || request.MaxResults != maximumResults ||
			len(request.IncludeDomains) != 1 || request.IncludeDomains[0] != "go.dev" ||
			len(request.ExcludeDomains) != 0 {
			t.Fatalf("unexpected Tavily request: %+v", request)
		}
		_, _ = w.Write([]byte(`{"results":[{"title":"Go Releases","url":"https://go.dev/doc/devel/release","content":"Current release information."}],"response_time":0.4}`))
	}))
	defer server.Close()
	outcome, err := New(NewTavilyProvider(server.Client(), server.URL)).ExecuteOutcome(
		context.Background(),
		`{"query":"Go release notes","allowed_domains":["go.dev"]}`,
		nil,
	)
	if err != nil || outcome.IsError() || outcome.Details["provider"] != "tavily" {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	metadata := outcome.Details["provider_metadata"].(map[string]any)
	if metadata["response_time"] != 0.4 {
		t.Fatalf("numeric response_time was not preserved: %+v", metadata)
	}
}

func TestTavilyProviderRejectsMissingResultsSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"response_time":0.4}`))
	}))
	defer server.Close()
	outcome, err := New(NewTavilyProvider(server.Client(), server.URL)).ExecuteOutcome(
		context.Background(),
		`{"query":"Go release notes"}`,
		nil,
	)
	if err != nil || outcome.ErrorCode != "web_search_response_invalid" {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
}

func TestFallbackProviderStopsAfterSuccessfulPrimary(t *testing.T) {
	first := &fakeSearchProvider{response: SearchResponse{Provider: "exa", Results: []SearchResult{
		{Title: "One", URL: "https://example.com/one"},
	}}}
	second := &fakeSearchProvider{response: SearchResponse{Provider: "tavily", Results: []SearchResult{
		{Title: "Two", URL: "https://example.com/two"},
	}}}
	outcome, err := New(fallbackProvider{providers: []SearchProvider{first, second}}).ExecuteOutcome(
		context.Background(), "query", nil,
	)
	if err != nil || outcome.IsError() || outcome.Details["result_count"] != 1 ||
		outcome.Details["provider"] != "exa" {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if second.request.Query != "" {
		t.Fatalf("fallback provider was called after primary success")
	}
}

func TestFallbackProviderUsesTavilyAfterExaFailure(t *testing.T) {
	first := &fakeSearchProvider{err: &ProviderError{
		Kind: ProviderErrorNetwork, Message: "exa unavailable", Retryable: true,
	}}
	second := &fakeSearchProvider{response: SearchResponse{
		Provider: "tavily",
		Results:  []SearchResult{{Title: "Fallback", URL: "https://example.com/fallback"}},
	}}
	outcome, err := New(fallbackProvider{providers: []SearchProvider{first, second}}).ExecuteOutcome(
		context.Background(), "query", nil,
	)
	if err != nil || outcome.IsError() || outcome.Details["provider"] != "tavily" {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	metadata := outcome.Details["provider_metadata"].(map[string]any)
	if metadata["fallback_from"] != "provider_0" || metadata["fallback_reason"] != "exa unavailable" {
		t.Fatalf("missing fallback evidence: %+v", metadata)
	}
}

func TestFallbackProviderDoesNotFallbackAfterCancellation(t *testing.T) {
	first := &fakeSearchProvider{err: &ProviderError{
		Kind: ProviderErrorCancelled, Message: "cancelled",
	}}
	second := &fakeSearchProvider{response: SearchResponse{Provider: "tavily"}}
	outcome, err := New(fallbackProvider{providers: []SearchProvider{first, second}}).ExecuteOutcome(
		context.Background(), "query", nil,
	)
	if err != nil || outcome.ErrorCode != "web_search_cancelled" {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if second.request.Query != "" {
		t.Fatalf("fallback provider was called after cancellation")
	}
}

func TestDisabledProviderReturnsStableUnsupportedOutcome(t *testing.T) {
	outcome, err := New(disabledProvider{}).ExecuteOutcome(context.Background(), "query", nil)
	if err != nil || outcome.Status != tool.OutcomeUnsupported || outcome.ErrorCode != "web_search_disabled" {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
}
