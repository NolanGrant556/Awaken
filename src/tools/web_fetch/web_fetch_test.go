package webfetch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestValidateTargetURLRejectsUnsupportedSchemes(t *testing.T) {
	cases := []string{
		"file:///etc/passwd",
		"data:text/plain,hello",
		"javascript:alert(1)",
		"mailto:test@example.com",
	}

	for _, rawURL := range cases {
		if _, err := validateTargetURL(rawURL, false); err == nil {
			t.Fatalf("expected %q to be rejected", rawURL)
		}
	}
}

func TestValidateTargetURLRejectsPrivateHosts(t *testing.T) {
	cases := []string{
		"http://localhost",
		"http://127.0.0.1",
		"http://127.1",
		"http://[::1]",
		"http://10.0.0.1",
		"http://172.16.0.1",
		"http://192.168.1.1",
		"http://169.254.169.254/latest/meta-data",
		"http://printer.local",
		"http://internal-service",
	}

	for _, rawURL := range cases {
		if _, err := validateTargetURL(rawURL, false); err == nil {
			t.Fatalf("expected %q to be rejected", rawURL)
		}
	}
}

func TestWebFetchHTTPTestSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("web_fetch should not send Authorization, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("hello from httptest"))
	}))
	defer server.Close()

	out, err := (&WebFetchTool{client: server.Client(), allowPrivateHosts: true}).Execute(context.Background(), `{"url":"`+server.URL+`/page"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	result := decodeResult(t, out)
	if result.Status != http.StatusCreated {
		t.Fatalf("expected status 201, got %+v", result)
	}
	if result.ContentType != "text/plain; charset=utf-8" {
		t.Fatalf("unexpected content type: %+v", result)
	}
	if result.FinalURL != server.URL+"/page" {
		t.Fatalf("unexpected final URL: %+v", result)
	}
	if result.BodyExcerpt != "hello from httptest" || result.Truncated {
		t.Fatalf("unexpected body result: %+v", result)
	}
}

func TestWebFetchTruncatesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("1234567890"))
	}))
	defer server.Close()

	out, err := (&WebFetchTool{client: server.Client(), allowPrivateHosts: true}).Execute(context.Background(), `{"url":"`+server.URL+`","max_bytes":4}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	result := decodeResult(t, out)
	if !result.Truncated || result.BodyExcerpt != "1234" {
		t.Fatalf("expected truncated four-byte excerpt, got %+v", result)
	}
}

func TestWebFetchRedactsSensitiveValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("web_fetch should not send Authorization, got %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte("Authorization: Bearer supersecret\naccess_token=supersecret"))
	}))
	defer server.Close()

	out, err := (&WebFetchTool{client: server.Client(), allowPrivateHosts: true}).Execute(context.Background(), `{"url":"`+server.URL+`/secret?access_token=supersecret&safe=ok"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(out, "supersecret") {
		t.Fatalf("expected sensitive values to be redacted, got %q", out)
	}

	result := decodeResult(t, out)
	if !strings.Contains(result.FinalURL, "safe=ok") || strings.Contains(result.FinalURL, "supersecret") {
		t.Fatalf("unexpected sanitized final URL: %+v", result)
	}
	if strings.Contains(result.BodyExcerpt, "supersecret") || !strings.Contains(result.BodyExcerpt, "[REDACTED]") {
		t.Fatalf("unexpected redacted body excerpt: %+v", result)
	}
}

func TestWebFetchReadOnlyContract(t *testing.T) {
	fetchTool := &WebFetchTool{}
	readOnly, ok := interface{}(fetchTool).(tool.ReadOnlyTool)
	if !ok || !readOnly.IsReadOnly() {
		t.Fatalf("expected web_fetch to be read-only")
	}
	if _, ok := interface{}(fetchTool).(tool.PermissionCheckingTool); ok {
		t.Fatalf("web_fetch should not request approval through PermissionCheckingTool")
	}
}

func decodeResult(t *testing.T, out string) webFetchResult {
	t.Helper()
	var result webFetchResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to decode result %q: %v", out, err)
	}
	return result
}
