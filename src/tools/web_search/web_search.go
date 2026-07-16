package websearch

import (
	"bytes"
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
	ToolName              = "WebSearch"
	defaultExaEndpoint    = "https://mcp.exa.ai/mcp?tools=web_search_exa"
	defaultTavilyEndpoint = "https://api.tavily.com/search"
	maximumResults        = 10
	maximumResponseBytes  = 4 * 1024 * 1024
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
	ProviderErrorDisabled       ProviderErrorKind = "disabled"
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

type ExaProvider struct {
	client   *http.Client
	endpoint string
}

func NewExaProvider(client *http.Client, endpoint string) *ExaProvider {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &ExaProvider{client: client, endpoint: strings.TrimSpace(endpoint)}
}

func NewExaProviderFromEnvironment() *ExaProvider { return NewExaProvider(nil, "") }

func (p *ExaProvider) Search(ctx context.Context, request SearchRequest) (SearchResponse, error) {
	endpoint := strings.TrimSpace(p.endpoint)
	if endpoint == "" {
		endpoint = defaultExaEndpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" ||
		(parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname()))) {
		return SearchResponse{}, &ProviderError{Kind: ProviderErrorFormat, Message: "WebSearch endpoint is invalid.", Retryable: false, Cause: err}
	}
	query := constrainedQuery(request.Query, request.AllowedDomains, request.BlockedDomains)
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "web_search_exa",
			"arguments": map[string]any{
				"query":      query,
				"numResults": request.Limit,
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return SearchResponse{}, &ProviderError{Kind: ProviderErrorFormat, Message: "WebSearch request could not be constructed.", Retryable: false, Cause: err}
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), strings.NewReader(string(body)))
	if err != nil {
		return SearchResponse{}, &ProviderError{Kind: ProviderErrorFormat, Message: "WebSearch request could not be constructed.", Retryable: false, Cause: err}
	}
	httpRequest.Header.Set("Accept", "application/json, text/event-stream")
	httpRequest.Header.Set("Content-Type", "application/json")
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
	responseBody, err := readBoundedResponse(response.Body)
	if err != nil {
		var tooLarge *responseTooLargeError
		if errors.As(err, &tooLarge) {
			return SearchResponse{}, &ProviderError{Kind: ProviderErrorFormat, Message: "WebSearch provider response exceeded the size limit.", Retryable: true, Cause: err}
		}
		return SearchResponse{}, &ProviderError{Kind: ProviderErrorNetwork, Message: "WebSearch provider response could not be read.", Retryable: true, Cause: err}
	}
	text, err := parseExaMCPResponse(responseBody)
	if err != nil {
		return SearchResponse{}, &ProviderError{Kind: ProviderErrorFormat, Message: "WebSearch provider returned an invalid response.", Retryable: true, Cause: err, HTTPStatus: response.StatusCode}
	}
	results := parseExaResults(text)
	return SearchResponse{
		Results:  results,
		Provider: "exa",
		Metadata: map[string]any{"http_status": response.StatusCode, "provider_result_count": len(results)},
	}, nil
}

type TavilyProvider struct {
	client   *http.Client
	endpoint string
}

func NewTavilyProvider(client *http.Client, endpoint string) *TavilyProvider {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &TavilyProvider{client: client, endpoint: strings.TrimSpace(endpoint)}
}

func NewTavilyProviderFromEnvironment() *TavilyProvider {
	return NewTavilyProvider(nil, "")
}

func (p *TavilyProvider) Search(ctx context.Context, request SearchRequest) (SearchResponse, error) {
	endpoint := p.endpoint
	if endpoint == "" {
		endpoint = defaultTavilyEndpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" ||
		(parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname()))) {
		return SearchResponse{}, &ProviderError{Kind: ProviderErrorFormat, Message: "WebSearch endpoint is invalid.", Cause: err}
	}
	payload := map[string]any{
		"query":               request.Query,
		"search_depth":        "basic",
		"max_results":         request.Limit,
		"include_answer":      false,
		"include_raw_content": false,
		"include_images":      false,
		"include_domains":     request.AllowedDomains,
		"exclude_domains":     request.BlockedDomains,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return SearchResponse{}, &ProviderError{Kind: ProviderErrorFormat, Message: "WebSearch request could not be constructed.", Cause: err}
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return SearchResponse{}, &ProviderError{Kind: ProviderErrorFormat, Message: "WebSearch request could not be constructed.", Cause: err}
	}
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("X-Tavily-Access-Mode", "keyless")
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
	responseBody, err := readBoundedResponse(response.Body)
	if err != nil {
		var tooLarge *responseTooLargeError
		if errors.As(err, &tooLarge) {
			return SearchResponse{}, &ProviderError{Kind: ProviderErrorFormat, Message: "WebSearch provider response exceeded the size limit.", Retryable: true, Cause: err}
		}
		return SearchResponse{}, &ProviderError{Kind: ProviderErrorNetwork, Message: "WebSearch provider response could not be read.", Retryable: true, Cause: err}
	}
	var payloadResponse struct {
		Results      json.RawMessage `json:"results"`
		ResponseTime any             `json:"response_time"`
	}
	if err := json.Unmarshal(responseBody, &payloadResponse); err != nil {
		return SearchResponse{}, &ProviderError{Kind: ProviderErrorFormat, Message: "WebSearch provider returned an invalid response.", Retryable: true, Cause: err, HTTPStatus: response.StatusCode}
	}
	if len(payloadResponse.Results) == 0 || string(payloadResponse.Results) == "null" {
		return SearchResponse{}, &ProviderError{Kind: ProviderErrorFormat, Message: "WebSearch provider returned an invalid response.", Retryable: true, HTTPStatus: response.StatusCode}
	}
	var providerResults []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(payloadResponse.Results, &providerResults); err != nil {
		return SearchResponse{}, &ProviderError{Kind: ProviderErrorFormat, Message: "WebSearch provider returned an invalid response.", Retryable: true, Cause: err, HTTPStatus: response.StatusCode}
	}
	results := make([]SearchResult, 0, len(providerResults))
	for _, item := range providerResults {
		if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.URL) == "" {
			continue
		}
		results = append(results, SearchResult{
			Title: strings.TrimSpace(item.Title), URL: strings.TrimSpace(item.URL), Snippet: strings.TrimSpace(item.Content),
		})
	}
	return SearchResponse{
		Results: results, Provider: "tavily",
		Metadata: map[string]any{"http_status": response.StatusCode, "provider_result_count": len(providerResults), "response_time": payloadResponse.ResponseTime},
	}, nil
}

type fallbackProvider struct {
	providers []SearchProvider
}

func (p fallbackProvider) Search(ctx context.Context, request SearchRequest) (SearchResponse, error) {
	var previousProvider string
	var previousError error
	for index, provider := range p.providers {
		response, err := provider.Search(ctx, request)
		if err == nil {
			if index > 0 {
				if response.Metadata == nil {
					response.Metadata = map[string]any{}
				}
				response.Metadata["fallback_from"] = previousProvider
				response.Metadata["fallback_reason"] = previousError.Error()
			}
			return response, nil
		}
		var providerError *ProviderError
		if errors.As(err, &providerError) && providerError.Kind == ProviderErrorCancelled {
			return SearchResponse{}, err
		}
		previousError = err
		switch provider.(type) {
		case *ExaProvider:
			previousProvider = "exa"
		case *TavilyProvider:
			previousProvider = "tavily"
		default:
			previousProvider = fmt.Sprintf("provider_%d", index)
		}
	}
	return SearchResponse{}, previousError
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type disabledProvider struct{}

func (disabledProvider) Search(context.Context, SearchRequest) (SearchResponse, error) {
	return SearchResponse{}, &ProviderError{Kind: ProviderErrorDisabled, Message: "WebSearch is disabled in Settings.", Retryable: false}
}

func NewProviderFromEnvironment() SearchProvider {
	configured, configuredByHost := os.LookupEnv("AWAKEN_WEB_SEARCH_PROVIDERS")
	configured = strings.TrimSpace(configured)
	if !configuredByHost {
		configured = "exa"
	}
	var providers []SearchProvider
	seen := make(map[string]bool)
	for _, raw := range strings.Split(configured, ",") {
		name := strings.ToLower(strings.TrimSpace(raw))
		if seen[name] {
			continue
		}
		seen[name] = true
		switch name {
		case "exa":
			providers = append(providers, NewExaProviderFromEnvironment())
		case "tavily":
			providers = append(providers, NewTavilyProviderFromEnvironment())
		}
	}
	switch len(providers) {
	case 0:
		return disabledProvider{}
	case 1:
		return providers[0]
	default:
		return fallbackProvider{providers: providers}
	}
}

type responseTooLargeError struct{}

func (*responseTooLargeError) Error() string { return "response exceeds size limit" }

func readBoundedResponse(reader io.Reader) ([]byte, error) {
	limited := io.LimitReader(reader, maximumResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(body) > maximumResponseBytes {
		return nil, &responseTooLargeError{}
	}
	return body, nil
}

func parseExaMCPResponse(body []byte) (string, error) {
	type mcpPayload struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	parse := func(raw []byte) (string, bool, error) {
		var payload mcpPayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			return "", false, err
		}
		if payload.Error != nil {
			return "", false, fmt.Errorf("mcp error %d: %s", payload.Error.Code, payload.Error.Message)
		}
		for _, content := range payload.Result.Content {
			if content.Type == "text" && strings.TrimSpace(content.Text) != "" {
				return strings.TrimSpace(content.Text), true, nil
			}
		}
		return "", false, nil
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "", fmt.Errorf("empty response")
	}
	if trimmed[0] == '{' {
		if text, ok, err := parse(trimmed); err != nil {
			return "", err
		} else if ok {
			return text, nil
		}
		return "", fmt.Errorf("response did not contain text content")
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		raw := bytes.TrimSpace([]byte(strings.TrimPrefix(line, "data:")))
		if text, ok, err := parse(raw); err != nil {
			return "", err
		} else if ok {
			return text, nil
		}
	}
	return "", fmt.Errorf("response did not contain an MCP data payload")
}

func parseExaResults(text string) []SearchResult {
	sections := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n---\n")
	results := make([]SearchResult, 0, len(sections))
	for _, section := range sections {
		var result SearchResult
		var snippet []string
		inHighlights := false
		for _, line := range strings.Split(section, "\n") {
			trimmed := strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(trimmed, "Title:"):
				result.Title = strings.TrimSpace(strings.TrimPrefix(trimmed, "Title:"))
			case strings.HasPrefix(trimmed, "URL:"):
				result.URL = strings.TrimSpace(strings.TrimPrefix(trimmed, "URL:"))
			case strings.HasPrefix(trimmed, "Highlights:"):
				inHighlights = true
				if value := strings.TrimSpace(strings.TrimPrefix(trimmed, "Highlights:")); value != "" {
					snippet = append(snippet, value)
				}
			case inHighlights && trimmed != "":
				snippet = append(snippet, trimmed)
			}
		}
		result.Snippet = strings.TrimSpace(strings.Join(snippet, "\n"))
		if result.Title != "" && result.URL != "" {
			results = append(results, result)
		}
	}
	return results
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
		provider = NewProviderFromEnvironment()
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
		recovery = []string{"retry_web_search", "enable_another_search_provider"}
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
	case ProviderErrorDisabled:
		code, category, status = "web_search_disabled", tool.ErrorCategoryPolicy, tool.OutcomeUnsupported
		recovery = []string{"enable_web_search_in_settings"}
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
	tool.MustRegisterToolWithMode(New(NewProviderFromEnvironment()), tool.DefinitionValidationStrictBuiltin)
}
