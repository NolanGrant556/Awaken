package llm

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRequestFailedErrorPreservesRetryFactsAndRedactsSecrets(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Retry-After": []string{"7"}},
		Body:       ioNopCloser(strings.NewReader(`{"error":{"message":"Authorization: Bearer sk-secret-value"}}`)),
	}
	err := requestFailedError(response)
	if !errors.Is(err, ErrRequestFailed) || !IsTransientRequestError(err) {
		t.Fatalf("expected typed transient request failure: %v", err)
	}
	if status, ok := RequestStatus(err); !ok || status != http.StatusTooManyRequests {
		t.Fatalf("unexpected request status: %d %v", status, ok)
	}
	if delay, ok := RequestRetryAfter(err); !ok || delay != 7*time.Second {
		t.Fatalf("unexpected retry delay: %s %v", delay, ok)
	}
	if strings.Contains(err.Error(), "sk-secret-value") {
		t.Fatalf("request error leaked provider secret: %v", err)
	}
}

func TestIsTransientRequestErrorRetriesPreResponseTransport429And5xx(t *testing.T) {
	if !IsTransientRequestError(&TransportError{}) {
		t.Fatal("pre-response transport errors should be transient")
	}
	for _, status := range []int{429, 500, 503} {
		if !IsTransientRequestError(&RequestError{StatusCode: status}) {
			t.Fatalf("status %d should be transient", status)
		}
	}
	for _, status := range []int{400, 401, 403, 404} {
		if IsTransientRequestError(&RequestError{StatusCode: status}) {
			t.Fatalf("status %d must not be retried", status)
		}
	}
	if IsTransientRequestError(errors.New("LLM request failed: stream idle timeout")) {
		t.Fatal("stream errors after a response begins must not be retried")
	}
	if !IsTransientRequestError(&StreamDisconnectedError{Reason: "tls handshake eof"}) {
		t.Fatal("stream disconnect before completion should be transient")
	}
	if !IsConnectionError(&TransportError{Reason: "error sending request"}) {
		t.Fatal("pre-response transport errors are connection failures")
	}
	if IsConnectionError(&StreamDisconnectedError{Reason: "stream eof"}) {
		t.Fatal("post-response stream disconnect is not a connection wait")
	}
	if !IsStreamDisconnectedError(&StreamDisconnectedError{Reason: "stream eof"}) {
		t.Fatal("stream eof must classify as stream disconnect")
	}
}

func TestSafeRetryDetailNeverLeaksURL(t *testing.T) {
	detail := SafeRetryDetail(transportError(errors.New("Post \"https://secret.example/v1/chat\": tls: handshake eof")))
	if detail != "tls handshake eof" {
		t.Fatalf("unexpected transport detail %q", detail)
	}
	if strings.Contains(detail, "secret.example") || strings.Contains(detail, "https://") {
		t.Fatalf("retry detail leaked endpoint: %q", detail)
	}
	streamDetail := SafeRetryDetail(&StreamDisconnectedError{Reason: "stream eof"})
	if streamDetail != "stream eof" {
		t.Fatalf("unexpected stream detail %q", streamDetail)
	}
}

// Keep this file independent from the larger invoke test helpers.
type stringReadCloser struct{ *strings.Reader }

func (stringReadCloser) Close() error { return nil }

func ioNopCloser(reader *strings.Reader) stringReadCloser { return stringReadCloser{reader} }
