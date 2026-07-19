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
