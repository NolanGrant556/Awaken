package browsercontrol

import (
	"context"
	"encoding/json"
	"testing"

	"awaken/src/tool"
)

func TestBrowserOpenReturnsDisabledStateWithoutAttachedRuntime(t *testing.T) {
	restore := swapBrowserRuntime(NewDisabledRuntime("browser runtime not configured"))
	defer restore()

	out, err := (&BrowserOpenTool{}).Execute(context.Background(), `{"url":"https://example.com"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	result := decodeBrowserResponse(t, out)
	if result.Status != browserResultDisabled || result.Runtime.Available || result.Runtime.Mode != "disabled" {
		t.Fatalf("unexpected disabled response: %+v", result)
	}
}

func TestBrowserOpenAndNavigateAskPermissionWhenRuntimeAvailable(t *testing.T) {
	restore := swapBrowserRuntime(NewMockRuntime())
	defer restore()

	permissionContext := tool.NewPermissionContext(t.TempDir(), nil)

	openDecision := (&BrowserOpenTool{}).CheckPermission(context.Background(), `{"url":"https://example.com"}`, permissionContext)
	if openDecision.Behavior != tool.PermissionAsk || openDecision.ApprovalRequest.RiskTone != "warning" {
		t.Fatalf("expected browser_open to ask permission, got %+v", openDecision)
	}

	navigateDecision := (&BrowserNavigateTool{}).CheckPermission(context.Background(), `{"action":"reload"}`, permissionContext)
	if navigateDecision.Behavior != tool.PermissionAsk || navigateDecision.ApprovalRequest.RiskTone != "warning" {
		t.Fatalf("expected browser_navigate to ask permission, got %+v", navigateDecision)
	}
}

func TestBrowserPageSummaryIsReadOnlyAndAsksPermissionWhenRuntimeAvailable(t *testing.T) {
	restore := swapBrowserRuntime(NewMockRuntime())
	defer restore()

	summaryTool := &BrowserPageSummaryTool{}
	if !summaryTool.IsReadOnly() {
		t.Fatal("expected browser_page_summary to be read-only")
	}

	permissionContext := tool.NewPermissionContext(t.TempDir(), nil)
	decision := tool.AllowPermission()
	if checker, ok := interface{}(summaryTool).(tool.PermissionCheckingTool); ok {
		decision = checker.CheckPermission(context.Background(), "", permissionContext)
	}
	if decision.Behavior != tool.PermissionAsk || decision.ApprovalRequest.RiskTone != "warning" {
		t.Fatalf("expected browser_page_summary to ask permission, got %+v", decision)
	}
}

func TestBrowserToolsUseMockRuntimeContract(t *testing.T) {
	restore := swapBrowserRuntime(NewMockRuntime())
	defer restore()

	openOut, err := (&BrowserOpenTool{}).Execute(context.Background(), `{"url":"https://example.com/docs"}`)
	if err != nil {
		t.Fatalf("browser_open returned error: %v", err)
	}
	openResult := decodeBrowserResponse(t, openOut)
	if openResult.Status != browserResultOK || openResult.Runtime.Mode != "mock" || openResult.URL != "https://example.com/docs" {
		t.Fatalf("unexpected open result: %+v", openResult)
	}

	summaryOut, err := (&BrowserPageSummaryTool{}).Execute(context.Background(), "")
	if err != nil {
		t.Fatalf("browser_page_summary returned error: %v", err)
	}
	summaryResult := decodeBrowserResponse(t, summaryOut)
	if summaryResult.Status != browserResultOK || summaryResult.Title == "" || summaryResult.Summary == "" {
		t.Fatalf("unexpected summary result: %+v", summaryResult)
	}
	if len(summaryResult.AvailableActions) != 1 || summaryResult.AvailableActions[0] != "reload" {
		t.Fatalf("unexpected available actions: %+v", summaryResult.AvailableActions)
	}
}

func TestBrowserNavigateReturnsStructuredErrorWithoutPage(t *testing.T) {
	restore := swapBrowserRuntime(NewMockRuntime())
	defer restore()

	out, err := (&BrowserNavigateTool{}).Execute(context.Background(), `{"action":"back"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	result := decodeBrowserResponse(t, out)
	if result.Status != browserResultError || result.Message != ErrNoActiveBrowserPage.Error() {
		t.Fatalf("unexpected navigate error result: %+v", result)
	}
}

func TestBrowserNavigateTracksBackAndForward(t *testing.T) {
	restore := swapBrowserRuntime(NewMockRuntime())
	defer restore()

	openTool := &BrowserOpenTool{}
	navigateTool := &BrowserNavigateTool{}

	if _, err := openTool.Execute(context.Background(), `{"url":"https://example.com/one"}`); err != nil {
		t.Fatalf("first open failed: %v", err)
	}
	if _, err := openTool.Execute(context.Background(), `{"url":"https://example.com/two"}`); err != nil {
		t.Fatalf("second open failed: %v", err)
	}

	backOut, err := navigateTool.Execute(context.Background(), `{"action":"back"}`)
	if err != nil {
		t.Fatalf("back navigation failed: %v", err)
	}
	backResult := decodeBrowserResponse(t, backOut)
	if backResult.Status != browserResultOK || backResult.URL != "https://example.com/one" {
		t.Fatalf("unexpected back result: %+v", backResult)
	}

	forwardOut, err := navigateTool.Execute(context.Background(), `{"action":"forward"}`)
	if err != nil {
		t.Fatalf("forward navigation failed: %v", err)
	}
	forwardResult := decodeBrowserResponse(t, forwardOut)
	if forwardResult.Status != browserResultOK || forwardResult.URL != "https://example.com/two" {
		t.Fatalf("unexpected forward result: %+v", forwardResult)
	}
}

func TestBrowserOpenRejectsNonHTTPURL(t *testing.T) {
	restore := swapBrowserRuntime(NewMockRuntime())
	defer restore()

	_, err := (&BrowserOpenTool{}).Execute(context.Background(), `{"url":"file:///tmp/test.html"}`)
	if err == nil {
		t.Fatal("expected invalid URL error")
	}
}

func swapBrowserRuntime(runtime Runtime) func() {
	previous := CurrentRuntime()
	SetRuntime(runtime)
	return func() {
		SetRuntime(previous)
	}
}

func decodeBrowserResponse(t *testing.T, raw string) browserResponse {
	t.Helper()

	var result browserResponse
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("failed to decode browser response %q: %v", raw, err)
	}
	return result
}
