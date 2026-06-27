package activitynotify

import (
	"strings"
	"testing"
	"time"
)

func TestActivityFreshness(t *testing.T) {
	base := testTime()
	tracker := NewTracker(InactivityTimeoutPolicy(5 * time.Minute))
	tracker.Record(Event{Type: EventAPICall, At: base})

	summary := tracker.Summary(base.Add(2 * time.Second))
	if summary.LastActivityType != EventAPICall {
		t.Fatalf("expected api_call as last activity, got %s", summary.LastActivityType)
	}
	if summary.APICallCount != 1 {
		t.Fatalf("expected 1 api call, got %d", summary.APICallCount)
	}
	if summary.IdleFor != 2*time.Second || summary.SecondsSinceActivity != 2 {
		t.Fatalf("expected 2s freshness, got idle=%s seconds=%.1f", summary.IdleFor, summary.SecondsSinceActivity)
	}

	decision := tracker.EvaluateTimeout(base.Add(2 * time.Second))
	if decision.TimedOut {
		t.Fatalf("expected fresh activity to avoid timeout: %+v", decision)
	}
}

func TestInactivityTimeout(t *testing.T) {
	base := testTime()
	tracker := NewTracker(InactivityTimeoutPolicy(5 * time.Minute))
	tracker.Record(Event{Type: EventAPICall, At: base})

	decision := tracker.EvaluateTimeout(base.Add(6 * time.Minute))
	if !decision.TimedOut {
		t.Fatalf("expected inactivity timeout: %+v", decision)
	}
	if decision.IdleFor != 6*time.Minute || decision.Limit != 5*time.Minute {
		t.Fatalf("unexpected timeout durations: %+v", decision)
	}
}

func TestNoTimeoutWithRecentToolOutput(t *testing.T) {
	base := testTime()
	tracker := NewTracker(InactivityTimeoutPolicy(5 * time.Minute))
	tracker.Record(Event{Type: EventToolStart, At: base, ToolName: "shell"})
	tracker.Record(Event{Type: EventToolOutput, At: base.Add(9 * time.Minute), ToolName: "shell", Message: "stdout chunk"})

	decision := tracker.EvaluateTimeout(base.Add(10 * time.Minute))
	if decision.TimedOut {
		t.Fatalf("expected recent tool output to keep task alive: %+v", decision)
	}
	if decision.Summary.CurrentTool != "shell" {
		t.Fatalf("expected current tool shell, got %q", decision.Summary.CurrentTool)
	}
	if decision.Summary.LastActivityType != EventToolOutput {
		t.Fatalf("expected tool_output as last activity, got %s", decision.Summary.LastActivityType)
	}
}

func TestDisabledTimeout(t *testing.T) {
	base := testTime()
	tracker := NewTracker(DisabledTimeoutPolicy())
	tracker.Record(Event{Type: EventHeartbeat, At: base})

	decision := tracker.EvaluateTimeout(base.Add(24 * time.Hour))
	if decision.TimedOut {
		t.Fatalf("expected disabled timeout to never fire: %+v", decision)
	}

	tracker.SetTimeoutPolicy(InactivityTimeoutPolicy(0))
	decision = tracker.EvaluateTimeout(base.Add(48 * time.Hour))
	if decision.TimedOut {
		t.Fatalf("expected zero inactivity limit to disable timeout: %+v", decision)
	}
}

func TestNotificationDedupe(t *testing.T) {
	base := testTime()
	manager := NewNotificationManager()
	manager.Record(Event{Type: EventBackgrounded, At: base, SessionID: "session-1", TaskID: "task-1"}, ActivitySummary{})

	summary := ActivitySummary{Backgrounded: true}
	first := manager.Record(Event{Type: EventCompleted, At: base.Add(time.Second), SessionID: "session-1", TaskID: "task-1"}, summary)
	if first.Decision != NotificationQueued {
		t.Fatalf("expected first completion notification to queue, got %+v", first)
	}

	second := manager.Record(Event{Type: EventCompleted, At: base.Add(2 * time.Second), SessionID: "session-1", TaskID: "task-1"}, summary)
	if second.Decision != NotificationDuplicate {
		t.Fatalf("expected duplicate notification to be skipped, got %+v", second)
	}
	if got := len(manager.Queue()); got != 1 {
		t.Fatalf("expected one queued notification, got %d", got)
	}
}

func TestForegroundSuppression(t *testing.T) {
	base := testTime()
	manager := NewNotificationManager()
	manager.Record(Event{Type: EventBackgrounded, At: base, SessionID: "session-1", TaskID: "task-1"}, ActivitySummary{})
	manager.Record(Event{Type: EventForegrounded, At: base.Add(time.Second), SessionID: "session-1", TaskID: "task-1"}, ActivitySummary{})
	result := manager.Record(Event{Type: EventCompleted, At: base.Add(2 * time.Second), SessionID: "session-1", TaskID: "task-1"}, ActivitySummary{Backgrounded: true})

	if result.Decision != NotificationSuppressedForeground {
		t.Fatalf("expected foreground completion to be suppressed, got %+v", result)
	}
	if got := len(manager.Queue()); got != 0 {
		t.Fatalf("expected no queued notifications, got %d", got)
	}
}

func TestBackgroundCompletionNotification(t *testing.T) {
	base := testTime()
	manager := NewNotificationManager()
	manager.Record(Event{Type: EventBackgrounded, At: base, SessionID: "session-1", TaskID: "task-1"}, ActivitySummary{})

	result := manager.Record(Event{
		Type:      EventCompleted,
		At:        base.Add(time.Second),
		SessionID: "session-1",
		TaskID:    "task-1",
		Message:   "finished indexing",
	}, ActivitySummary{Backgrounded: true, APICallCount: 3, LastActivityDescription: "completed"})

	if result.Decision != NotificationQueued || result.Notification == nil {
		t.Fatalf("expected queued background notification, got %+v", result)
	}
	if result.Notification.Event != EventCompleted || !strings.Contains(result.Notification.Body, "finished indexing") {
		t.Fatalf("unexpected notification: %+v", result.Notification)
	}
}

func TestAwaySummaryTriggerGating(t *testing.T) {
	base := testTime()
	manager := NewNotificationManager()
	if ShouldTriggerAwaySummary(AwaySummaryInput{UserWasAway: true, ProgressHappened: false}) {
		t.Fatalf("expected no away summary without progress")
	}

	manager.Record(Event{Type: EventBackgrounded, At: base, SessionID: "session-1", TaskID: "task-1"}, ActivitySummary{})
	if manager.ShouldTriggerAwaySummary("session-1") {
		t.Fatalf("expected no away summary before progress")
	}

	manager.Record(Event{Type: EventToolOutput, At: base.Add(time.Second), SessionID: "session-1", TaskID: "task-1", ToolName: "shell"}, ActivitySummary{})
	if !manager.ShouldTriggerAwaySummary("session-1") {
		t.Fatalf("expected away summary after away progress")
	}

	manager.MarkAwaySummarySent("session-1", base.Add(2*time.Second))
	if manager.ShouldTriggerAwaySummary("session-1") {
		t.Fatalf("expected existing summary since last user message to gate trigger")
	}

	manager.Record(Event{Type: EventUserInput, At: base.Add(3 * time.Second), SessionID: "session-1"}, ActivitySummary{})
	manager.Record(Event{Type: EventBackgrounded, At: base.Add(4 * time.Second), SessionID: "session-1", TaskID: "task-1"}, ActivitySummary{})
	if manager.ShouldTriggerAwaySummary("session-1") {
		t.Fatalf("expected new away period to wait for fresh progress")
	}
}

func TestNotificationTextRedactsSecrets(t *testing.T) {
	base := testTime()
	manager := NewNotificationManager()
	manager.Record(Event{Type: EventBackgrounded, At: base, SessionID: "session-1", TaskID: "task-1"}, ActivitySummary{})

	secretText := "OPENAI_API_KEY=sk-proj-abc123456789 token=xoxb-123456789-secret password=hunter2"
	result := manager.Record(Event{
		Type:      EventCompleted,
		At:        base.Add(time.Second),
		SessionID: "session-1",
		TaskID:    "task-1",
		Message:   secretText,
	}, ActivitySummary{Backgrounded: true, LastActivityDescription: secretText})
	if result.Notification == nil {
		t.Fatalf("expected notification")
	}

	text := result.Notification.Title + " " + result.Notification.Body
	for _, secret := range []string{"sk-proj-abc123456789", "xoxb-123456789-secret", "hunter2"} {
		if strings.Contains(text, secret) {
			t.Fatalf("expected %q to be redacted from %q", secret, text)
		}
	}
	if !strings.Contains(text, "[REDACTED]") {
		t.Fatalf("expected redaction marker in %q", text)
	}
}

func testTime() time.Time {
	return time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC)
}
