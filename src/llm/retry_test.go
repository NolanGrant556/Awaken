package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInvokeRetriesTransientHTTPStatuses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		kind   ErrorKind
	}{
		{name: "rate limit", status: http.StatusTooManyRequests, kind: ErrorKindRateLimit},
		{name: "service unavailable", status: http.StatusServiceUnavailable, kind: ErrorKindServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			server := newChatServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 1 {
					w.WriteHeader(tt.status)
					_, _ = w.Write([]byte(`{"error":{"message":"retry later"}}`))
					return
				}
				writeJSON(t, w, map[string]interface{}{
					"choices": []map[string]interface{}{
						{"message": map[string]interface{}{"content": "ok"}},
					},
				})
			})
			defer server.Close()

			resp, err := Invoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"})
			if err != nil {
				t.Fatalf("Invoke returned error: %v", err)
			}
			if resp.Content != "ok" {
				t.Fatalf("expected ok, got %q", resp.Content)
			}
			if calls != 2 {
				t.Fatalf("expected one retry for %s, got %d calls", tt.kind, calls)
			}
		})
	}
}

func TestInvokeAuthErrorDoesNotRetry(t *testing.T) {
	calls := 0
	server := newChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls > 1 {
			t.Fatalf("auth error should not retry, got %d calls", calls)
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad api key"}}`))
	})
	defer server.Close()

	_, err := Invoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"})
	if !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("expected ErrRequestFailed, got %v", err)
	}
	var requestErr *RequestError
	if !errors.As(err, &requestErr) {
		t.Fatalf("expected RequestError, got %T", err)
	}
	if requestErr.Kind != ErrorKindAuthentication {
		t.Fatalf("expected auth classification, got %s", requestErr.Kind)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestContextOverflowClassificationDoesNotLeakBody(t *testing.T) {
	server := newChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"context_length_exceeded sk-test-secret internal path /var/secrets"}}`))
	})
	defer server.Close()

	_, err := Invoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"})
	if !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("expected ErrRequestFailed, got %v", err)
	}
	var requestErr *RequestError
	if !errors.As(err, &requestErr) {
		t.Fatalf("expected RequestError, got %T", err)
	}
	if requestErr.Kind != ErrorKindContextOverflow {
		t.Fatalf("expected context overflow classification, got %s", requestErr.Kind)
	}
	msg := err.Error()
	if strings.Contains(msg, "sk-test-secret") || strings.Contains(msg, "/var/secrets") {
		t.Fatalf("error message must not contain response body details: %q", msg)
	}
}

func TestStreamInvokeRetriesTransientStatus(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("expected /chat/completions, got %s", r.URL.Path)
		}
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"message":"retry stream"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"Hel"}}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"lo"}}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}))
	defer server.Close()

	var chunks []string
	resp, err := StreamInvoke(context.Background(), testConfig(server.URL), Request{UserPrompt: "hi"}, func(content string, isThinking bool) {
		if isThinking {
			t.Fatalf("unexpected thinking callback: %q", content)
		}
		chunks = append(chunks, content)
	})
	if err != nil {
		t.Fatalf("StreamInvoke returned error: %v", err)
	}
	if resp.Content != "Hello" {
		t.Fatalf("expected Hello, got %q", resp.Content)
	}
	if len(chunks) != 2 || chunks[0] != "Hel" || chunks[1] != "lo" {
		t.Fatalf("expected two streamed chunks, got %+v", chunks)
	}
	if calls != 2 {
		t.Fatalf("expected stream retry, got %d calls", calls)
	}
}

func TestRequestRetryPolicyCanDisableRetries(t *testing.T) {
	calls := 0
	server := newChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"do not retry"}}`))
	})
	defer server.Close()

	disabled := RetryPolicy{Disabled: true}
	_, err := Invoke(context.Background(), testConfig(server.URL), Request{
		UserPrompt: "hi",
		Retry:      &disabled,
	})
	if !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("expected ErrRequestFailed, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected retry disabled to make 1 call, got %d", calls)
	}
}
