package browsercontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"awaken/src/tool"
)

const (
	browserResultOK       = "ok"
	browserResultDisabled = "disabled"
	browserResultError    = "error"
)

type BrowserOpenTool struct{}

type BrowserPageSummaryTool struct{}

type BrowserNavigateTool struct{}

type browserResponse struct {
	Status           string        `json:"status"`
	Runtime          RuntimeStatus `json:"runtime"`
	Action           string        `json:"action,omitempty"`
	URL              string        `json:"url,omitempty"`
	Title            string        `json:"title,omitempty"`
	Summary          string        `json:"summary,omitempty"`
	AvailableActions []string      `json:"available_actions,omitempty"`
	Message          string        `json:"message,omitempty"`
}

type browserOpenParams struct {
	URL string `json:"url"`
}

type browserNavigateParams struct {
	Action string `json:"action"`
}

func (t *BrowserOpenTool) Name() string {
	return "browser_open"
}

func (t *BrowserOpenTool) Description() string {
	return "打开指定 http/https URL 到附加的浏览器 runtime；如果未接入真实 browser runtime，会返回 disabled 状态。该操作会改变当前浏览器页面，默认需要审批。"
}

func (t *BrowserOpenTool) IsAvailable() bool {
	return CurrentRuntime().Status().Available
}

func (t *BrowserOpenTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "url", Description: "要打开的 http/https URL", Type: "string", Required: true},
	}
}

func (t *BrowserOpenTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	if !CurrentRuntime().Status().Available {
		return tool.AllowPermission()
	}

	rawURL, err := parseBrowserOpenInput(input)
	if err != nil || strings.TrimSpace(rawURL) == "" {
		return tool.AllowPermission()
	}

	return tool.AskPermission(
		"opening a browser page may load external content and change the active browser state",
		tool.ApprovalRequest{
			Title:       "Approve browser open",
			Description: fmt.Sprintf("Open browser URL:\n\n%s", strings.TrimSpace(rawURL)),
			RiskTone:    "warning",
		},
	)
}

func (t *BrowserOpenTool) Execute(ctx context.Context, input string) (string, error) {
	rawURL, err := parseBrowserOpenInput(input)
	if err != nil {
		return "", err
	}

	runtime := CurrentRuntime()
	status := runtime.Status()
	if !status.Available {
		return encodeBrowserResponse(browserResponse{
			Status:  browserResultDisabled,
			Runtime: status,
			Message: disabledMessage(status),
		})
	}

	page, err := runtime.OpenURL(ctx, rawURL)
	if err != nil {
		return browserResponseForError(status, "", err)
	}

	return encodeBrowserResponse(pageResponse(browserResultOK, status, "", page, ""))
}

func (t *BrowserPageSummaryTool) Name() string {
	return "browser_page_summary"
}

func (t *BrowserPageSummaryTool) Description() string {
	return "读取当前浏览器页面的 URL、标题和摘要；这是只读工具。如果未接入真实 browser runtime，会返回 disabled 状态。"
}

func (t *BrowserPageSummaryTool) IsAvailable() bool {
	return CurrentRuntime().Status().Available
}

func (t *BrowserPageSummaryTool) Parameters() []tool.ToolParameter {
	return nil
}

func (t *BrowserPageSummaryTool) IsReadOnly() bool {
	return true
}

func (t *BrowserPageSummaryTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	if !CurrentRuntime().Status().Available {
		return tool.AllowPermission()
	}

	return tool.AskPermission(
		"reading the active browser page may expose page content from the user's current browsing session",
		tool.ApprovalRequest{
			Title:       "Approve browser page summary",
			Description: "Read the active browser page URL, title, and summary.",
			RiskTone:    "warning",
		},
	)
}

func (t *BrowserPageSummaryTool) Execute(ctx context.Context, input string) (string, error) {
	runtime := CurrentRuntime()
	status := runtime.Status()
	if !status.Available {
		return encodeBrowserResponse(browserResponse{
			Status:  browserResultDisabled,
			Runtime: status,
			Message: disabledMessage(status),
		})
	}

	page, err := runtime.CurrentPage(ctx)
	if err != nil {
		return browserResponseForError(status, "", err)
	}

	return encodeBrowserResponse(pageResponse(browserResultOK, status, "", page, ""))
}

func (t *BrowserNavigateTool) Name() string {
	return "browser_navigate"
}

func (t *BrowserNavigateTool) Description() string {
	return "执行浏览器基础导航动作 back、forward、reload；如果未接入真实 browser runtime，会返回 disabled 状态。该操作会改变活动页面，默认需要审批。"
}

func (t *BrowserNavigateTool) IsAvailable() bool {
	return CurrentRuntime().Status().Available
}

func (t *BrowserNavigateTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "action", Description: "导航动作: back, forward, reload", Type: "string", Required: true},
	}
}

func (t *BrowserNavigateTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	if !CurrentRuntime().Status().Available {
		return tool.AllowPermission()
	}

	action, err := parseBrowserNavigateInput(input)
	if err != nil || strings.TrimSpace(action) == "" {
		return tool.AllowPermission()
	}

	return tool.AskPermission(
		"browser navigation changes the active page and may trigger additional network requests",
		tool.ApprovalRequest{
			Title:       "Approve browser navigation",
			Description: fmt.Sprintf("Run browser navigation action:\n\n%s", strings.TrimSpace(action)),
			RiskTone:    "warning",
		},
	)
}

func (t *BrowserNavigateTool) Execute(ctx context.Context, input string) (string, error) {
	rawAction, err := parseBrowserNavigateInput(input)
	if err != nil {
		return "", err
	}
	action, err := ParseNavigationAction(rawAction)
	if err != nil {
		return "", err
	}

	runtime := CurrentRuntime()
	status := runtime.Status()
	if !status.Available {
		return encodeBrowserResponse(browserResponse{
			Status:  browserResultDisabled,
			Runtime: status,
			Action:  string(action),
			Message: disabledMessage(status),
		})
	}

	page, err := runtime.Navigate(ctx, action)
	if err != nil {
		return browserResponseForError(status, string(action), err)
	}

	return encodeBrowserResponse(pageResponse(browserResultOK, status, string(action), page, ""))
}

func parseBrowserOpenInput(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", fmt.Errorf("缺少必需参数: url")
	}
	if !strings.HasPrefix(trimmed, "{") {
		_, err := NormalizeBrowserURL(trimmed)
		if err != nil {
			return "", err
		}
		return trimmed, nil
	}

	var params browserOpenParams
	if err := json.Unmarshal([]byte(trimmed), &params); err != nil {
		return "", fmt.Errorf("参数格式错误，需要 JSON 对象或 URL 字符串: %w", err)
	}
	normalized, err := NormalizeBrowserURL(params.URL)
	if err != nil {
		return "", err
	}
	return normalized, nil
}

func parseBrowserNavigateInput(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", fmt.Errorf("缺少必需参数: action")
	}
	if !strings.HasPrefix(trimmed, "{") {
		return trimmed, nil
	}

	var params browserNavigateParams
	if err := json.Unmarshal([]byte(trimmed), &params); err != nil {
		return "", fmt.Errorf("参数格式错误，需要 JSON 对象或 action 字符串: %w", err)
	}
	if strings.TrimSpace(params.Action) == "" {
		return "", fmt.Errorf("缺少必需参数: action")
	}
	return params.Action, nil
}

func pageResponse(status string, runtime RuntimeStatus, action string, page PageState, message string) browserResponse {
	return browserResponse{
		Status:           status,
		Runtime:          runtime,
		Action:           action,
		URL:              page.URL,
		Title:            page.Title,
		Summary:          page.Summary,
		AvailableActions: availableBrowserActions(page),
		Message:          message,
	}
}

func availableBrowserActions(page PageState) []string {
	actions := []string{"reload"}
	if page.CanGoBack {
		actions = append([]string{"back"}, actions...)
	}
	if page.CanGoForward {
		actions = append(actions, "forward")
	}
	return actions
}

func browserResponseForError(runtime RuntimeStatus, action string, err error) (string, error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "", err
	}

	status := browserResultError
	message := err.Error()
	if errors.Is(err, ErrBrowserRuntimeDisabled) {
		status = browserResultDisabled
		message = disabledMessage(runtime)
	}

	return encodeBrowserResponse(browserResponse{
		Status:  status,
		Runtime: runtime,
		Action:  action,
		Message: message,
	})
}

func disabledMessage(runtime RuntimeStatus) string {
	if strings.TrimSpace(runtime.Reason) != "" {
		return runtime.Reason
	}
	return "browser runtime is not available"
}

func encodeBrowserResponse(response browserResponse) (string, error) {
	data, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func init() {
	tool.MustRegisterTool(&BrowserOpenTool{})
	tool.MustRegisterTool(&BrowserPageSummaryTool{})
	tool.MustRegisterTool(&BrowserNavigateTool{})
}
