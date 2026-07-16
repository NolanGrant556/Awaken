package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"awaken/src/tool"
)

const (
	ToolName                 = "WebSearch"
	defaultWebSearchEndpoint = "https://api.search.brave.com/res/v1/web/search"
	maximumResults           = 10
)

type SearchRequest struct {
	Query          string
	AllowedDomains []string
	BlockedDomains []string
	Limit          int
}

type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

type SearchResponse struct {
	Results  []SearchResult
	Provider string
	Metadata map[string]any
}

type SearchProvider interface {
	Search(context.Context, SearchRequest) (SearchResponse, error)
}

type ProviderErrorKind string

const (
	ProviderErrorAuthentication ProviderErrorKind = "authentication"
	ProviderErrorRateLimit      ProviderErrorKind = "rate_limit"
	ProviderErrorNetwork        ProviderErrorKind = "network"
	ProviderErrorUpstream       ProviderErrorKind = "upstream"
	ProviderErrorFormat         ProviderErrorKind = "format"
	ProviderErrorCancelled      ProviderErrorKind = "cancelled"
)

type ProviderError struct {
	Kind       ProviderErrorKind
	Message    string
	Retryable  bool
	HTTPStatus int
	Cause      error
}

func (e *ProviderError) Error() string {
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return string(e.Kind)
}
func (e *ProviderError) Unwrap() error { return e.Cause }

type BraveProvider struct {
	client   *http.Client
	endpoint string
	apiKey   string
}

func NewBraveProvider(client *http.Client, endpoint, apiKey string) *BraveProvider {
	if client == nil {
		client = http.DefaultClient
	}
	return &BraveProvider{client: client, endpoint: strings.TrimSpace(endpoint), apiKey: strings.TrimSpace(apiKey)}
}

func NewBraveProviderFromEnvironment() *BraveProvider {
	return NewBraveProvider(nil, os.Getenv("AWAKEN_WEB_SEARCH_ENDPOINT"), os.Getenv("AWAKEN_WEB_SEARCH_API_KEY"))
}

func (p *BraveProvider) Search(ctx context.Context, request SearchRequest) (SearchResponse, error) {
	endpoint := strings.TrimSpace(p.endpoint)
	if endpoint == "" {
		endpoint = defaultWebSearchEndpoint
	}
	if endpoint == defaultWebSearchEndpoint && strings.TrimSpace(p.apiKey) == "" {
		return SearchResponse{}, &ProviderError{Kind: ProviderErrorAuthentication, Message: "WebSearch provider credentials are not configured.", Retryable: false}
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return SearchResponse{}, &ProviderError{Kind: ProviderErrorFormat, Message: "WebSearch endpoint is invalid.", Retryable: false, Cause: err}
	}
	query := constrainedQuery(request.Query, request.AllowedDomains, request.BlockedDomains)
	values := parsed.Query()
	values.Set("q", query)
	values.Set("count", fmt.Sprintf("%d", request.Limit))
	parsed.RawQuery = values.Encode()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return SearchResponse{}, &ProviderError{Kind: ProviderErrorFormat, Message: "WebSearch request could not be constructed.", Retryable: false, Cause: err}
	}
	if p.apiKey != "" {
		httpRequest.Header.Set("X-Subscription-Token", p.apiKey)
	}
	httpRequest.Header.Set("Accept", "application/json")
	response, err := p.client.Do(httpRequest)
	if err != nil {
		kind := ProviderErrorNetwork
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			kind = ProviderErrorCancelled
		}
		return SearchResponse{}, &ProviderError{Kind: kind, Message: "WebSearch provider request failed.", Retryable: true, Cause: err}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return SearchResponse{}, classifyHTTPError(response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	if err != nil {
		return SearchResponse{}, &ProviderError{Kind: ProviderErrorNetwork, Message: "WebSearch provider response could not be read.", Retryable: true, Cause: err}
	}
	var payload struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return SearchResponse{}, &ProviderError{Kind: ProviderErrorFormat, Message: "WebSearch provider returned an invalid response.", Retryable: true, Cause: err, HTTPStatus: response.StatusCode}
	}
	results := make([]SearchResult, 0, len(payload.Web.Results))
	for _, item := range payload.Web.Results {
		if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.URL) == "" {
			continue
		}
		results = append(results, SearchResult{Title: strings.TrimSpace(item.Title), URL: strings.TrimSpace(item.URL), Snippet: strings.TrimSpace(item.Description)})
	}
	return SearchResponse{Results: results, Provider: "brave", Metadata: map[string]any{"http_status": response.StatusCode, "provider_result_count": len(payload.Web.Results)}}, nil
}

func classifyHTTPError(status int) *ProviderError {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return &ProviderError{Kind: ProviderErrorAuthentication, Message: "WebSearch provider rejected authentication.", HTTPStatus: status}
	case status == http.StatusTooManyRequests:
		return &ProviderError{Kind: ProviderErrorRateLimit, Message: "WebSearch provider rate limit was reached.", HTTPStatus: status, Retryable: true}
	case status >= 500:
		return &ProviderError{Kind: ProviderErrorUpstream, Message: "WebSearch provider is temporarily unavailable.", HTTPStatus: status, Retryable: true}
	default:
		return &ProviderError{Kind: ProviderErrorUpstream, Message: fmt.Sprintf("WebSearch provider rejected the request with HTTP %d.", status), HTTPStatus: status}
	}
}

type WebSearchTool struct {
	provider SearchProvider
	// Compatibility fields keep direct package tests and embedders able to
	// inject an HTTP transport without exposing credentials to model input.
	client   *http.Client
	endpoint string
	apiKey   string
}

func New(provider SearchProvider) *WebSearchTool { return &WebSearchTool{provider: provider} }

var _ tool.LegacySingleValueInput = (*WebSearchTool)(nil)

func (t *WebSearchTool) UnwrapSingleParameterInput() bool { return true }
func (t *WebSearchTool) Name() string                     { return ToolName }
func (t *WebSearchTool) Description() string {
	return "Search current public web pages through a host-configured provider. Optional allowed_domains or blocked_domains are normalized and enforced both in the provider query and against returned URLs."
}
func (t *WebSearchTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "query", Description: "Public web search query.", Type: "string", Required: true},
		{Name: "allowed_domains", Description: "Only return these domains and their subdomains.", Type: "array"},
		{Name: "blocked_domains", Description: "Exclude these domains and their subdomains.", Type: "array"},
	}
}
func (t *WebSearchTool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name: ToolName, DisplayName: "Web Search", Description: t.Description(), Purpose: "Retrieve current public web sources with provider and domain-filter evidence.",
		InputSchema: tool.ToolParametersSchema{Type: "object", Properties: map[string]tool.ToolParam{
			"query":           {Type: "string", Description: "Public web search query."},
			"allowed_domains": {Type: "array", Description: "Only return these domains and subdomains.", Items: &tool.ToolParam{Type: "string"}},
			"blocked_domains": {Type: "array", Description: "Exclude these domains and subdomains.", Items: &tool.ToolParam{Type: "string"}},
		}, Required: []string{"query"}, AdditionalProperties: tool.BoolPtr(false)},
		WhenToUse:    []string{"The user requests web research or current information", "Repository files cannot answer a public time-sensitive question"},
		WhenNotToUse: []string{"Use Grep for local files", "Use an authenticated MCP or connector for private sources"},
		SideEffects:  []tool.ToolSideEffect{{Kind: tool.SideEffectNetwork, Resources: []string{"public search provider"}, Description: "Sends the query and domain constraints to the configured search provider.", Reversibility: tool.ReversibilityNotApplicable}},
		Examples:     []tool.ToolExample{{Scenario: "Search official sources", Input: map[string]any{"query": "Go release notes", "allowed_domains": []string{"go.dev"}}, Outcome: "Returns up to 10 post-filtered sources with provider metadata."}},
		FailureModes: []tool.ToolFailureMode{
			{Code: "web_search_authorization_failed", Cause: "Provider credentials are absent or rejected", Retryable: false},
			{Code: "web_search_rate_limited", Cause: "Provider rate limit", Retryable: true},
			{Code: "web_search_network_failed", Cause: "Network transport failed", Retryable: true},
			{Code: "web_search_response_invalid", Cause: "Provider response is malformed", Retryable: true},
		},
		OutputSchema: tool.ToolParam{Type: "object", Description: "Filtered sources, provider metadata, counts, and truncation details.", AdditionalProperties: tool.BoolPtr(true)},
		Annotations:  tool.ToolAnnotations{ReadOnly: true, OpenWorld: true, CapabilityClass: tool.CapabilityBrowserRead, TrustedOrigin: tool.TrustedOriginBuiltin},
	})
}

type searchParams struct {
	Query          string   `json:"query"`
	AllowedDomains []string `json:"allowed_domains,omitempty"`
	BlockedDomains []string `json:"blocked_domains,omitempty"`
}

func (t *WebSearchTool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	if err != nil {
		return outcome.Content, err
	}
	if outcome.IsError() {
		return outcome.Content, fmt.Errorf("%s", outcome.ErrorMessage)
	}
	return outcome.Content, nil
}

func (t *WebSearchTool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	params, err := parseSearchParams(input)
	if err != nil {
		return tool.FailedOutcome(err.Error(), "web_search_invalid_input", tool.ErrorCategoryValidation, false).WithTiming(started, time.Now().UTC()), nil
	}
	provider := t.provider
	if provider == nil {
		if t.client != nil || strings.TrimSpace(t.endpoint) != "" || strings.TrimSpace(t.apiKey) != "" {
			provider = NewBraveProvider(t.client, t.endpoint, t.apiKey)
		} else {
			provider = NewBraveProviderFromEnvironment()
		}
	}
	response, err := provider.Search(ctx, SearchRequest{Query: params.Query, AllowedDomains: params.AllowedDomains, BlockedDomains: params.BlockedDomains, Limit: maximumResults})
	if err != nil {
		return providerFailure(err).WithTiming(started, time.Now().UTC()), nil
	}
	filtered := filterResults(response.Results, params.AllowedDomains, params.BlockedDomains)
	providerCount := len(response.Results)
	if len(filtered) > maximumResults {
		filtered = filtered[:maximumResults]
	}
	var builder strings.Builder
	for index, item := range filtered {
		fmt.Fprintf(&builder, "%d. %s\n   URL: %s", index+1, item.Title, item.URL)
		if item.Snippet != "" {
			fmt.Fprintf(&builder, "\n   Summary: %s", item.Snippet)
		}
		builder.WriteString("\n")
	}
	content := strings.TrimSpace(builder.String())
	if content == "" {
		content = "No web results found."
	}
	outcome := tool.SucceededOutcome(content).WithTiming(started, time.Now().UTC())
	outcome.Truncated = providerCount > len(filtered)
	outcome.Summary = fmt.Sprintf("WebSearch returned %d source(s).", len(filtered))
	outcome.Details = map[string]any{
		"query": params.Query, "allowed_domains": params.AllowedDomains, "blocked_domains": params.BlockedDomains,
		"results": filtered, "result_count": len(filtered), "provider_result_count": providerCount,
		"provider": response.Provider, "provider_metadata": response.Metadata, "truncated": outcome.Truncated,
	}
	outcome.Diagnostic = map[string]any{"query": params.Query, "result_count": len(filtered), "provider_result_count": providerCount, "provider": response.Provider}
	return outcome, nil
}

func parseSearchParams(input string) (searchParams, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return searchParams{}, fmt.Errorf("query is required")
	}
	params := searchParams{}
	if strings.HasPrefix(input, "{") {
		decoder := json.NewDecoder(strings.NewReader(input))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&params); err != nil {
			return searchParams{}, fmt.Errorf("invalid WebSearch input: %w", err)
		}
	} else {
		params.Query = input
	}
	params.Query = strings.TrimSpace(params.Query)
	if params.Query == "" {
		return searchParams{}, fmt.Errorf("query is required")
	}
	if len(params.AllowedDomains) > 0 && len(params.BlockedDomains) > 0 {
		return searchParams{}, fmt.Errorf("allowed_domains and blocked_domains cannot both be non-empty")
	}
	var err error
	params.AllowedDomains, err = normalizeDomains(params.AllowedDomains)
	if err != nil {
		return searchParams{}, err
	}
	params.BlockedDomains, err = normalizeDomains(params.BlockedDomains)
	if err != nil {
		return searchParams{}, err
	}
	return params, nil
}

func normalizeDomains(domains []string) ([]string, error) {
	seen := make(map[string]bool)
	result := make([]string, 0, len(domains))
	for _, domain := range domains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if strings.Contains(domain, "://") {
			parsed, err := url.Parse(domain)
			if err != nil {
				return nil, fmt.Errorf("invalid domain %q", domain)
			}
			domain = parsed.Hostname()
		}
		domain = strings.TrimPrefix(domain, "www.")
		domain = strings.TrimSuffix(domain, ".")
		if domain == "" || net.ParseIP(domain) == nil && (!strings.Contains(domain, ".") || strings.ContainsAny(domain, " /?#")) {
			return nil, fmt.Errorf("invalid domain %q", domain)
		}
		if !seen[domain] {
			seen[domain] = true
			result = append(result, domain)
		}
	}
	sort.Strings(result)
	return result, nil
}

func constrainedQuery(query string, allowed, blocked []string) string {
	if len(allowed) > 0 {
		terms := make([]string, len(allowed))
		for i, domain := range allowed {
			terms[i] = "site:" + domain
		}
		return query + " (" + strings.Join(terms, " OR ") + ")"
	}
	if len(blocked) > 0 {
		for _, domain := range blocked {
			query += " -site:" + domain
		}
	}
	return query
}

func filterResults(results []SearchResult, allowed, blocked []string) []SearchResult {
	filtered := make([]SearchResult, 0, len(results))
	seen := make(map[string]bool)
	for _, result := range results {
		parsed, err := url.Parse(result.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
			continue
		}
		host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
		if len(allowed) > 0 && !hostMatchesAny(host, allowed) {
			continue
		}
		if len(blocked) > 0 && hostMatchesAny(host, blocked) {
			continue
		}
		canonical := parsed.String()
		if seen[canonical] {
			continue
		}
		seen[canonical] = true
		result.URL = canonical
		filtered = append(filtered, result)
	}
	return filtered
}

func hostMatchesAny(host string, domains []string) bool {
	for _, domain := range domains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func providerFailure(err error) tool.ExecutionOutcome {
	var providerError *ProviderError
	if !errors.As(err, &providerError) {
		providerError = &ProviderError{Kind: ProviderErrorNetwork, Message: "WebSearch provider failed.", Retryable: true, Cause: err}
	}
	code := "web_search_upstream_failed"
	category := tool.ErrorCategoryRuntime
	status := tool.OutcomeFailed
	recovery := []string{"retry_web_search", "use_another_research_source"}
	switch providerError.Kind {
	case ProviderErrorAuthentication:
		code, category, status = "web_search_authorization_failed", tool.ErrorCategoryAuthorization, tool.OutcomeNeedsAuth
		recovery = []string{"configure_web_search_credentials"}
	case ProviderErrorRateLimit:
		code = "web_search_rate_limited"
		recovery = []string{"retry_after_backoff", "narrow_search_query"}
	case ProviderErrorNetwork:
		code = "web_search_network_failed"
		recovery = []string{"check_network", "retry_web_search"}
	case ProviderErrorFormat:
		code = "web_search_response_invalid"
	case ProviderErrorCancelled:
		code, category, status = "web_search_cancelled", tool.ErrorCategoryCancelled, tool.OutcomeCancelled
		recovery = []string{"retry_when_context_available"}
	}
	outcome := tool.FailedOutcome(providerError.Message, code, category, providerError.Retryable)
	outcome.Status = status
	outcome.RecoveryOptions = recovery
	if providerError.HTTPStatus != 0 {
		outcome.Diagnostic = map[string]any{"http_status": providerError.HTTPStatus}
	}
	return outcome
}

func init() {
	tool.MustRegisterToolWithMode(New(NewBraveProviderFromEnvironment()), tool.DefinitionValidationStrictBuiltin)
}
