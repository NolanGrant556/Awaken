package llm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPreparedRequestSendsExactLoweredBodyWithoutRebuild(t *testing.T) {
	captured := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		captured <- body
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	cfg := Config{APIKey: "test-key", BaseURL: server.URL, Model: "test-model", Protocol: ProtocolOpenAIChat}
	req := Request{RequestID: "req_exact", SystemPrompt: "stable system", UserPrompt: "stable user", Model: "test-model"}
	prepared, err := PrepareRequest(cfg, req, true)
	if err != nil {
		t.Fatalf("prepare request: %v", err)
	}
	wantBody := append([]byte(nil), prepared.Body...)
	digest := sha256.Sum256(wantBody)
	if prepared.RequestShapeDigest != hex.EncodeToString(digest[:]) {
		t.Fatalf("digest does not identify exact body")
	}

	// Mutating the source request after preparation must not affect the wire.
	req.SystemPrompt = "mutated"
	req.UserPrompt = "mutated"
	response, err := StreamPreparedWithObserver(context.Background(), cfg, prepared, StreamObserver{})
	if err != nil {
		t.Fatalf("send prepared request: %v", err)
	}
	if response.Content != "ok" {
		t.Fatalf("unexpected response: %+v", response)
	}
	gotBody := <-captured
	if !bytes.Equal(gotBody, wantBody) {
		t.Fatalf("transport rebuilt prepared body:\nwant %s\n got %s", wantBody, gotBody)
	}
}

func TestPreparedStreamPublishesProviderNeutralFailureEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "busy", http.StatusTooManyRequests)
	}))
	defer server.Close()

	cfg := Config{APIKey: "test-key", BaseURL: server.URL, Model: "test-model", Protocol: ProtocolOpenAIChat}
	prepared, err := PrepareRequest(cfg, Request{UserPrompt: "hello"}, true)
	if err != nil {
		t.Fatalf("prepare request: %v", err)
	}
	var events []StreamEvent
	_, err = StreamPreparedWithObserver(context.Background(), cfg, prepared, StreamObserver{
		OnEvent: func(event StreamEvent) { events = append(events, event) },
	})
	if err == nil {
		t.Fatal("expected provider error")
	}
	if len(events) != 1 || events[0].Type != StreamEventFailed || events[0].ErrorCode != "http_429" {
		t.Fatalf("unexpected terminal events: %+v", events)
	}
}

func TestProviderRateLimitObservationNormalizesOpenAIAndAnthropicHeaders(t *testing.T) {
	response := &http.Response{Header: http.Header{
		"X-Ratelimit-Remaining-Requests":       []string{"47"},
		"Anthropic-Ratelimit-Tokens-Remaining": []string{"12000"},
	}}
	observation := providerRateLimitObservation(response)
	if observation == nil || observation.RequestRemaining != "47" || observation.TokenRemaining != "12000" || observation.ObservedAt.IsZero() {
		t.Fatalf("unexpected rate limit observation: %+v", observation)
	}
}
