package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"awaken/src/tool"
)

const defaultWebSearchEndpoint = "https://api.search.brave.com/res/v1/web/search"

type WebSearchTool struct {
	client   *http.Client
	endpoint string
}

var _ tool.LegacySingleValueInput = (*WebSearchTool)(nil)

func (t *WebSearchTool) UnwrapSingleParameterInput() bool { return true }

type webSearchResponse struct {
	Web struct {
		Results []webSearchResult `json:"results"`
	} `json:"web"`
}

type webSearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

func (t *WebSearchTool) Name() string {
	return "web_search"
}

func (t *WebSearchTool) Description() string {
	return "在互联网上搜索公开网页信息；不读取本地文件、不搜索工作区内容，本地文件请用 list_files/grep_search/read_file/read_artifact"
}

func (t *WebSearchTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{
			Name:        "query",
			Description: "互联网搜索关键词，不是本地文件路径或 glob",
			Type:        "string",
			Required:    true,
		},
	}
}

func (t *WebSearchTool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:        "web_search",
		DisplayName: "Web Search",
		Description: t.Description(),
		Purpose:     "查询当前互联网公开信息，并返回可继续打开或引用的搜索结果。",
		WhenToUse: []string{
			"用户要求搜索、查最新信息或事实可能随时间变化时",
			"工作区文件不能回答且公开网络检索适合时",
		},
		WhenNotToUse: []string{
			"搜索本机工作区文件内容使用 grep_search；枚举文件使用 list_files",
			"请求依赖登录态、私有数据或专用连接器时使用对应 connector/MCP，而不是公开搜索",
		},
		OutputSchema: tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
			"query":                 {Type: "string", Description: "实际搜索词"},
			"result_count":          {Type: "integer", Description: "返回结果数量"},
			"provider_result_count": {Type: "integer", Description: "上游返回结果总数"},
			"http_status":           {Type: "integer", Description: "搜索服务 HTTP 状态"},
		}},
		SideEffects: []tool.ToolSideEffect{{Kind: tool.SideEffectNetwork, Resources: []string{"public search service"}, Description: "向公开搜索服务发送查询；不修改工作区或外部业务数据。", Reversibility: tool.ReversibilityNotApplicable}},
		Examples:    []tool.ToolExample{{Scenario: "查询当前官方文档", Input: map[string]any{"query": "OpenAI Codex tool architecture official"}, Outcome: "返回有界搜索结果和来源链接"}},
		FailureModes: []tool.ToolFailureMode{
			{Code: "web_search_query_required", Cause: "query 缺失", Retryable: false, Recovery: []string{"提供具体搜索词"}},
			{Code: "web_search_authorization_failed", Cause: "搜索服务拒绝授权", Retryable: false, Recovery: []string{"修复服务凭据"}},
			{Code: "web_search_rate_limited", Cause: "搜索服务限流", Retryable: true, Recovery: []string{"按 backoff 等待后重试"}},
			{Code: "web_search_request_failed", Cause: "网络或请求执行失败", Retryable: true, Recovery: []string{"检查网络后有界重试"}},
			{Code: "web_search_upstream_failed", Cause: "上游搜索服务返回失败", Retryable: true, Recovery: []string{"等待后重试或改用官方来源"}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly:        true,
			Destructive:     false,
			OpenWorld:       true,
			CapabilityClass: tool.CapabilityBrowserRead,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
		},
	})
}

func (t *WebSearchTool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	if err != nil {
		return outcome.Content, err
	}
	if outcome.IsError() {
		return "", fmt.Errorf("%s", outcome.ErrorMessage)
	}
	return outcome.Content, nil
}

// ExecuteOutcome separates authorization, throttling, upstream, and response
// failures so the agent does not blindly replay an internet search request.
func (t *WebSearchTool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	query := strings.TrimSpace(input)
	if query == "" {
		outcome := tool.FailedOutcome("搜索关键词不能为空", "web_search_query_required", tool.ErrorCategoryValidation, false)
		outcome.RecoveryOptions = []string{"provide_search_query"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	endpoint := strings.TrimSpace(t.endpoint)
	if endpoint == "" {
		endpoint = defaultWebSearchEndpoint
	}
	separator := "?"
	if strings.Contains(endpoint, "?") {
		separator = "&"
	}
	requestURL := endpoint + separator + "q=" + url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
	if err != nil {
		message := fmt.Sprintf("创建请求失败: %v", err)
		outcome := tool.FailedOutcome(message, "web_search_request_invalid", tool.ErrorCategoryValidation, false)
		outcome.RecoveryOptions = []string{"change_search_query"}
		outcome.Diagnostic = map[string]any{"query": query}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	client := t.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		message := fmt.Sprintf("搜索请求失败: %v", err)
		outcome := webSearchRequestFailure(err, message, query)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		outcome := webSearchHTTPFailure(resp.StatusCode, query)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		message := fmt.Sprintf("读取响应失败: %v", err)
		outcome := tool.FailedOutcome(message, "web_search_response_read_failed", tool.ErrorCategoryRuntime, true)
		outcome.RecoveryOptions = []string{"retry_web_search"}
		outcome.Diagnostic = map[string]any{"query": query, "http_status": resp.StatusCode}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	var result webSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		message := fmt.Sprintf("解析响应失败: %v", err)
		outcome := tool.FailedOutcome(message, "web_search_response_invalid", tool.ErrorCategoryRuntime, true)
		outcome.RecoveryOptions = []string{"retry_web_search", "inspect_search_provider"}
		outcome.Diagnostic = map[string]any{"query": query, "http_status": resp.StatusCode}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	if len(result.Web.Results) == 0 {
		outcome := tool.SucceededOutcome("未找到相关结果")
		outcome.Diagnostic = map[string]any{"query": query, "result_count": 0, "http_status": resp.StatusCode}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	var summary strings.Builder
	summary.WriteString("搜索结果:\n")
	resultCount := 0
	for _, result := range result.Web.Results {
		if resultCount >= 5 {
			break
		}
		if strings.TrimSpace(result.Title) == "" || strings.TrimSpace(result.URL) == "" {
			continue
		}
		resultCount++
		summary.WriteString(fmt.Sprintf("%d. %s\n   URL: %s\n   摘要: %s\n\n", resultCount, result.Title, result.URL, result.Description))
	}

	if resultCount == 0 {
		outcome := tool.SucceededOutcome("未找到相关结果")
		outcome.Diagnostic = map[string]any{"query": query, "result_count": 0, "http_status": resp.StatusCode}
		outcome.Details = webSearchDetails(query, 0, 0, resp.StatusCode)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	outcome := tool.SucceededOutcome(summary.String())
	outcome.Truncated = len(result.Web.Results) > resultCount
	outcome.Diagnostic = map[string]any{
		"query": query, "result_count": resultCount, "provider_result_count": len(result.Web.Results),
		"http_status": resp.StatusCode, "truncated": outcome.Truncated,
	}
	outcome.Details = webSearchDetails(query, resultCount, len(result.Web.Results), resp.StatusCode)
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

func webSearchDetails(query string, resultCount, providerResultCount, statusCode int) map[string]any {
	return map[string]any{
		"query": query, "result_count": resultCount,
		"provider_result_count": providerResultCount, "http_status": statusCode,
	}
}

func webSearchRequestFailure(err error, message, query string) tool.ExecutionOutcome {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		outcome := tool.FailedOutcome(message, "web_search_cancelled", tool.ErrorCategoryCancelled, true)
		outcome.Status = tool.OutcomeCancelled
		outcome.TransportStatus = tool.TransportCancelled
		outcome.RecoveryOptions = []string{"retry_when_context_available"}
		outcome.Diagnostic = map[string]any{"query": query}
		return outcome
	}
	outcome := tool.FailedOutcome(message, "web_search_request_failed", tool.ErrorCategoryRuntime, true)
	outcome.RecoveryOptions = []string{"check_network_connection", "retry_web_search"}
	outcome.Diagnostic = map[string]any{"query": query}
	return outcome
}

func webSearchHTTPFailure(statusCode int, query string) tool.ExecutionOutcome {
	message := fmt.Sprintf("搜索失败，状态码: %d", statusCode)
	code := "web_search_upstream_rejected"
	category := tool.ErrorCategoryDomain
	retryable := false
	recovery := []string{"change_search_query_or_stop"}
	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		code = "web_search_authorization_failed"
		category = tool.ErrorCategoryAuthorization
		recovery = []string{"configure_web_search_credentials", "use_another_research_source"}
	case statusCode == http.StatusTooManyRequests:
		code = "web_search_rate_limited"
		category = tool.ErrorCategoryRuntime
		retryable = true
		recovery = []string{"retry_after_backoff", "narrow_search_query"}
	case statusCode >= http.StatusInternalServerError:
		code = "web_search_upstream_failed"
		category = tool.ErrorCategoryRuntime
		retryable = true
		recovery = []string{"retry_web_search", "use_another_research_source"}
	}
	outcome := tool.FailedOutcome(message, code, category, retryable)
	if code == "web_search_authorization_failed" {
		outcome.Status = tool.OutcomeNeedsAuth
	}
	outcome.RecoveryOptions = recovery
	outcome.Diagnostic = map[string]any{"query": query, "http_status": statusCode}
	return outcome
}

func init() {
	tool.MustRegisterToolWithMode(&WebSearchTool{}, tool.DefinitionValidationStrictBuiltin)
}
