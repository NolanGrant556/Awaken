package agentbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"awaken/src/protocol/agui"
)

func TestSessionPermissionRequestResolvesAllow(t *testing.T) {
	var resolvedPath string
	var resolvedBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resolvedPath = r.Method + " " + r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&resolvedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	s := &bridgeSession{
		mapping:    &ACPSessionMapping{ACPSessionID: "acp_1"},
		coreClient: NewCoreClient(server.URL, "token"),
		ctx:        context.Background(),
	}
	// Exercise the resolution helper directly; the full RequestPermission path is
	// covered by integration tests once a real ACP client is available.
	s.pendingPermissions = map[string][]agui.ACPPermissionOption{
		"ix_1": {{OptionID: "allow-once", Name: "Allow", Kind: "allow"}},
	}
	if err := s.resolvePermission("run_1", "ix_1", "allow-once"); err != nil {
		t.Fatalf("resolve permission: %v", err)
	}
	if resolvedPath != "POST /v1/runs/run_1/external-interactions/ix_1/resolve" {
		t.Fatalf("unexpected path: %q", resolvedPath)
	}
	if resolvedBody["resolution"] != "completed" {
		t.Fatalf("expected completed, got %q", resolvedBody["resolution"])
	}
}

func TestSessionPermissionRequestResolvesDeny(t *testing.T) {
	var resolvedBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&resolvedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	s := &bridgeSession{
		mapping:    &ACPSessionMapping{ACPSessionID: "acp_1"},
		coreClient: NewCoreClient(server.URL, "token"),
		ctx:        context.Background(),
	}
	s.pendingPermissions = map[string][]agui.ACPPermissionOption{
		"ix_1": {{OptionID: "deny", Name: "Deny", Kind: "deny"}},
	}
	if err := s.resolvePermission("run_1", "ix_1", "deny"); err != nil {
		t.Fatalf("resolve permission: %v", err)
	}
	if resolvedBody["resolution"] != "failed" {
		t.Fatalf("expected failed, got %q", resolvedBody["resolution"])
	}
}

func TestSessionPermissionRequestCancel(t *testing.T) {
	var resolvedBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&resolvedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	s := &bridgeSession{
		mapping:    &ACPSessionMapping{ACPSessionID: "acp_1"},
		coreClient: NewCoreClient(server.URL, "token"),
		ctx:        context.Background(),
	}
	if err := s.resolvePermission("run_1", "ix_1", ""); err != nil {
		t.Fatalf("resolve permission: %v", err)
	}
	if resolvedBody["resolution"] != "cancelled" {
		t.Fatalf("expected cancelled, got %q", resolvedBody["resolution"])
	}
}

func TestAgentCloseAllSessions(t *testing.T) {
	var closed bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method+" "+r.URL.Path == "POST /v1/control/acp/sessions/acp_1/close" {
			closed = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	agent := NewAgent(NewCoreClient(server.URL, "token"))
	agent.sessions["acp_1"] = &bridgeSession{
		mapping: &ACPSessionMapping{ACPSessionID: "acp_1"},
		ctx:     context.Background(),
	}
	agent.CloseAllSessions(context.Background())
	if !closed {
		t.Fatal("expected session close to be called")
	}
	if len(agent.sessions) != 0 {
		t.Fatalf("expected sessions cleared, got %d", len(agent.sessions))
	}
}

func TestSessionRunCancellation(t *testing.T) {
	var cancelled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/runs/run_1/cancel":
			cancelled = true
			w.WriteHeader(http.StatusOK)
		case "POST /v1/runs":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"run_id": "run_1", "thread_id": "thread_1", "status": "running"})
		case "GET /v1/runs/run_1/events":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_ = w.(http.Flusher)
			for {
				select {
				case <-r.Context().Done():
					return
				default:
				}
				time.Sleep(10 * time.Millisecond)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	s := &bridgeSession{
		mapping:    &ACPSessionMapping{ACPSessionID: "acp_1", ThreadID: "thread_1"},
		coreClient: NewCoreClient(server.URL, "token"),
		ctx:        context.Background(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	err := s.Prompt(ctx, "hello")
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !cancelled {
		t.Fatal("expected run cancel to be called")
	}
}
