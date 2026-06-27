package redaction

import (
	"strings"
	"testing"
)

func TestRedactStringMasksCommonSecretPatterns(t *testing.T) {
	input := strings.Join([]string{
		"openai=sk-proj-abcdefghijklmnopqrstuvwxyz123456",
		"anthropic=sk-ant-api03-abcdefghijklmnopqrstuvwxyz123456",
		"github=ghp_abcdefghijklmnopqrstuvwxyz1234",
		"aws=AKIAIOSFODNN7EXAMPLE",
		"Authorization: Bearer abcdefghijklmnopqrstuvwxyz123456",
		"x-api-key: provider-key-value",
		`{"api_key":"json-secret-value","github_token":"json-token-value"}`,
		"https://example.test/callback?code=ok&access_token=query-secret&state=keep",
		"postgres://user:database-password@localhost/db",
		"LLM_API_KEY=env-secret-value",
	}, "\n")

	got := RedactString(input)
	for _, leaked := range []string{
		"sk-proj-abcdefghijklmnopqrstuvwxyz123456",
		"sk-ant-api03-abcdefghijklmnopqrstuvwxyz123456",
		"ghp_abcdefghijklmnopqrstuvwxyz1234",
		"AKIAIOSFODNN7EXAMPLE",
		"abcdefghijklmnopqrstuvwxyz123456",
		"provider-key-value",
		"json-secret-value",
		"json-token-value",
		"query-secret",
		"database-password",
		"env-secret-value",
	} {
		if strings.Contains(got, leaked) {
			t.Fatalf("redacted output still contains %q:\n%s", leaked, got)
		}
	}
	if !strings.Contains(got, "state=keep") {
		t.Fatalf("expected non-sensitive query param to remain, got:\n%s", got)
	}
}

func TestRedactStringAvoidsCommonNonSecretText(t *testing.T) {
	input := strings.Join([]string{
		"token_count=42",
		`{"token_count":42}`,
		"sk-short",
		"monkey=banana",
		"Authorization is a concept, not a header here",
	}, "\n")

	got := RedactString(input)
	if got != input {
		t.Fatalf("expected ordinary text to remain unchanged\nwant:\n%s\ngot:\n%s", input, got)
	}
}

func TestRedactorTypeUsesSameRules(t *testing.T) {
	redactor := NewRedactor()
	got := redactor.RedactString("Bearer abcdefghijklmnopqrstuvwxyz")
	if strings.Contains(got, "abcdefghijklmnopqrstuvwxyz") || !strings.Contains(got, replacement) {
		t.Fatalf("expected Redactor to redact bearer token, got %q", got)
	}
}
