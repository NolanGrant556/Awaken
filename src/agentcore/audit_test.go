package agentcore

import (
	"strings"
	"testing"
)

func TestRedactAuditTextRemovesCommonSecretsAndTruncates(t *testing.T) {
	input := "Authorization: Bearer token-secret-123456 OPENAI_API_KEY=sk-secret123456 password=hunter2 " +
		strings.Repeat("x", 260)

	got, truncated := RedactAuditText(input, 120)
	if !truncated {
		t.Fatalf("expected long audit text to be truncated")
	}
	for _, leaked := range []string{"token-secret-123456", "sk-secret123456", "hunter2"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("redacted text leaked %q: %s", leaked, got)
		}
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("expected redacted marker, got %s", got)
	}
}

func TestSanitizeSecurityAuditEventCleansFields(t *testing.T) {
	event := SanitizeSecurityAuditEvent(SecurityAuditEvent{
		Summary:       "x-api-key: secret-key-value",
		InputPreview:  `{"access_token":"abc1234567890"}`,
		AffectedPaths: []string{"secrets/sk-proj-abc123456789.txt"},
		Error:         "Authorization=Bearer abcdefghijklmnop",
	})

	combined := event.Summary + event.InputPreview + strings.Join(event.AffectedPaths, "") + event.Error
	for _, leaked := range []string{"secret-key-value", "abc1234567890", "sk-proj-abc123456789", "abcdefghijklmnop"} {
		if strings.Contains(combined, leaked) {
			t.Fatalf("sanitized audit event leaked %q: %+v", leaked, event)
		}
	}
}
