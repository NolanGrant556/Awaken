package mcpelicitation

import (
	"context"
	"strings"
	"testing"
	"time"
)

func testRequest() Request {
	return Request{
		ServerName:    "github",
		ToolName:      "create_pr",
		ElicitationID: "elicit-1",
		Message:       "Open this URL to continue",
		URL:           "https://example.test/oauth?access_token=super-secret-token&page=1",
	}
}

func fixedClock() func() time.Time {
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return now }
}

func TestManagerEnqueueDeduplicatesByServerToolAndElicitationID(t *testing.T) {
	manager := NewManager(WithClock(fixedClock()))
	req := testRequest()

	first, duplicate, err := manager.Enqueue(req)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	if duplicate {
		t.Fatalf("first enqueue should not be duplicate")
	}

	second, duplicate, err := manager.Enqueue(req)
	if err != nil {
		t.Fatalf("second enqueue failed: %v", err)
	}
	if !duplicate {
		t.Fatalf("second enqueue should be duplicate")
	}
	if first.Key() != second.Key() {
		t.Fatalf("duplicate returned different key: %#v != %#v", first.Key(), second.Key())
	}

	snapshot := manager.Snapshot()
	if len(snapshot.Items) != 1 {
		t.Fatalf("expected one queued item, got %d", len(snapshot.Items))
	}
	if snapshot.PendingCount != 1 {
		t.Fatalf("expected one pending item, got %d", snapshot.PendingCount)
	}
}

func TestHookResolverAutomaticallyHandlesRequest(t *testing.T) {
	manager := NewManager(WithClock(fixedClock()))
	resolver := NewHookResolver(HookFunc(func(ctx context.Context, req Request) (HookResult, error) {
		if req.ElicitationID != "elicit-1" {
			t.Fatalf("unexpected elicitation id: %q", req.ElicitationID)
		}
		return HookResult{
			Handled: true,
			Response: Response{
				Action:  ActionHookResolved,
				Message: "handled by policy hook",
			},
		}, nil
	}))

	result, err := manager.ResolveOrEnqueue(context.Background(), testRequest(), resolver)
	if err != nil {
		t.Fatalf("resolve or enqueue failed: %v", err)
	}
	if !result.HookHandled {
		t.Fatalf("expected hook to handle request")
	}
	if result.Snapshot.State != StateHookResolved {
		t.Fatalf("expected hook resolved state, got %q", result.Snapshot.State)
	}
	if result.Snapshot.Response == nil || result.Snapshot.Response.Source != SourceHook {
		t.Fatalf("expected hook response source, got %#v", result.Snapshot.Response)
	}

	snapshot := manager.Snapshot()
	if snapshot.PendingCount != 0 {
		t.Fatalf("expected no pending UI work for hook-resolved request, got %d", snapshot.PendingCount)
	}
}

func TestUserDeclineCompletesWithoutRetryAndReturnsDisplayError(t *testing.T) {
	manager := NewManager(WithClock(fixedClock()))
	req := testRequest()
	queued, _, err := manager.Enqueue(req)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	completed, err := manager.Complete(queued.Key(), Response{Action: ActionDecline, Source: SourceUser})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if completed.State != StateDeclined {
		t.Fatalf("expected declined state, got %q", completed.State)
	}

	controller := NewRetryController(3)
	decision := controller.AfterElicitation(req, *completed.Response, 0)
	if decision.ShouldRetry {
		t.Fatalf("decline should not retry")
	}
	if !strings.Contains(decision.DisplayError, "declined") {
		t.Fatalf("expected declined display error, got %q", decision.DisplayError)
	}
	if strings.Contains(decision.DisplayError, "super-secret-token") {
		t.Fatalf("display error leaked query secret: %q", decision.DisplayError)
	}
}

func TestTimeoutCompletesWithoutRetry(t *testing.T) {
	manager := NewManager(WithClock(fixedClock()))
	req := testRequest()
	queued, _, err := manager.Enqueue(req)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	timedOut, err := manager.Timeout(queued.Key())
	if err != nil {
		t.Fatalf("timeout failed: %v", err)
	}
	if timedOut.State != StateTimedOut {
		t.Fatalf("expected timed out state, got %q", timedOut.State)
	}

	controller := NewRetryController(3)
	decision := controller.AfterElicitation(req, *timedOut.Response, 0)
	if decision.ShouldRetry {
		t.Fatalf("timeout should not retry")
	}
	if !strings.Contains(decision.DisplayError, "timed out") {
		t.Fatalf("expected timeout display error, got %q", decision.DisplayError)
	}
}

func TestRetryControllerStopsAtRetryLimit(t *testing.T) {
	controller := NewRetryController(1)
	req := testRequest()
	response := Response{Action: ActionApprove, Source: SourceUser}

	first := controller.AfterElicitation(req, response, 0)
	if !first.ShouldRetry {
		t.Fatalf("first approved elicitation should retry: %q", first.DisplayError)
	}
	if first.AttemptsRemaining != 0 {
		t.Fatalf("expected no attempts remaining after first retry, got %d", first.AttemptsRemaining)
	}

	second := controller.AfterElicitation(req, response, 1)
	if second.ShouldRetry {
		t.Fatalf("retry limit should stop another retry")
	}
	if !strings.Contains(second.DisplayError, "retry limit") {
		t.Fatalf("expected retry limit error, got %q", second.DisplayError)
	}
}

func TestURLValidationAndRedaction(t *testing.T) {
	info, err := ValidateURL("https://example.test/callback?access_token=abc123secret&page=1&api_key=sk-test-secret")
	if err != nil {
		t.Fatalf("expected valid URL: %v", err)
	}
	if strings.Contains(info.RedactedURL, "abc123secret") || strings.Contains(info.RedactedURL, "sk-test-secret") {
		t.Fatalf("redacted URL leaked secret values: %q", info.RedactedURL)
	}
	if !strings.Contains(info.RedactedURL, "page=1") {
		t.Fatalf("expected non-sensitive query value to remain, got %q", info.RedactedURL)
	}
	if _, err := ValidateURL("file:///tmp/token.txt"); err == nil {
		t.Fatalf("expected file URL to be rejected")
	}
	if _, err := ValidateURL("https://user:password@example.test/callback"); err == nil {
		t.Fatalf("expected userinfo URL to be rejected")
	}
}
