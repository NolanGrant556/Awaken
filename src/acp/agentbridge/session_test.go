package agentbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"awaken/src/protocol/agui"
	acpsdk "github.com/coder/acp-go-sdk"
)

func TestSessionPermissionRequestResolvesAllow(t *testing.T) {
	var resolvedPath string
	var resolvedBody map[string]any
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
		"ix_1": {{OptionID: "allow-once", Name: "Allow", Kind: string(acpsdk.PermissionOptionKindAllowOnce)}},
	}
	if err := s.resolvePermission("run_1", "ix_1", "allow-once"); err != nil {
		t.Fatalf("resolve permission: %v", err)
	}
	if resolvedPath != "POST /v1/control/acp/sessions/acp_1/runs/run_1/permissions/ix_1/resolve" {
		t.Fatalf("unexpected path: %q", resolvedPath)
	}
	if resolvedBody["option_id"] != "allow-once" {
		t.Fatalf("expected option_id allow-once, got %q", resolvedBody["option_id"])
	}
	if resolvedBody["cancelled"] != false {
		t.Fatalf("expected cancelled false, got %v", resolvedBody["cancelled"])
	}
}

func TestSessionPermissionRequestResolvesDeny(t *testing.T) {
	var resolvedBody map[string]any
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
		"ix_1": {{OptionID: "deny", Name: "Deny", Kind: string(acpsdk.PermissionOptionKindRejectOnce)}},
	}
	if err := s.resolvePermission("run_1", "ix_1", "deny"); err != nil {
		t.Fatalf("resolve permission: %v", err)
	}
	if resolvedBody["option_id"] != "deny" {
		t.Fatalf("expected option_id deny, got %q", resolvedBody["option_id"])
	}
	if resolvedBody["cancelled"] != false {
		t.Fatalf("expected cancelled false, got %v", resolvedBody["cancelled"])
	}
}

func TestSessionPermissionRequestCancel(t *testing.T) {
	var resolvedBody map[string]any
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
	if resolvedBody["option_id"] != "" {
		t.Fatalf("expected empty option_id, got %q", resolvedBody["option_id"])
	}
	if resolvedBody["cancelled"] != true {
		t.Fatalf("expected cancelled true, got %v", resolvedBody["cancelled"])
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
		case "POST /v1/control/acp/sessions/acp_1/runs/run_1/cancel":
			cancelled = true
			w.WriteHeader(http.StatusOK)
		case "POST /v1/control/acp/sessions/acp_1/runs":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"run_id": "run_1", "thread_id": "thread_1", "status": "running"})
		case "DELETE /v1/control/acp/sessions/acp_1/current-run/run_1":
			w.WriteHeader(http.StatusOK)
		case "GET /v1/control/acp/sessions/acp_1/runs/run_1/events":
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
	stopReason, err := s.Prompt(ctx, "hello")
	if err != nil {
		t.Fatalf("expected nil error on cancellation, got %v", err)
	}
	if stopReason != acpsdk.StopReasonCancelled {
		t.Fatalf("expected cancelled stop reason, got %q", stopReason)
	}
	if !cancelled {
		t.Fatal("expected run cancel to be called")
	}
}

func TestSessionRejectsEventStreamEOFBeforeTerminal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/control/acp/sessions/acp_1/runs":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"run_id": "run_1", "thread_id": "thread_1", "status": "running"})
		case "GET /v1/control/acp/sessions/acp_1/runs/run_1/events":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
		case "DELETE /v1/control/acp/sessions/acp_1/current-run/run_1":
			w.WriteHeader(http.StatusOK)
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
	if _, err := s.Prompt(context.Background(), "hello"); err == nil {
		t.Fatal("expected premature event stream EOF to fail")
	}
}

func TestSessionReconnectsFromLastEventCursor(t *testing.T) {
	streamCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/control/acp/sessions/acp_1/runs":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"run_id": "run_1", "thread_id": "thread_1", "status": "running"})
		case "GET /v1/control/acp/sessions/acp_1/runs/run_1/events":
			w.Header().Set("Content-Type", "text/event-stream")
			streamCount++
			if streamCount == 1 {
				_, _ = fmt.Fprintln(w, `data: {"type":"TEXT_MESSAGE_CONTENT","delta":"hello","streamCursor":10}`)
				_, _ = fmt.Fprintln(w)
				return
			}
			if got := r.URL.Query().Get("after_cursor"); got != "10" {
				t.Errorf("reconnect cursor: got %q, want 10", got)
			}
			_, _ = fmt.Fprintln(w, `data: {"type":"RUN_FINISHED","streamCursor":20}`)
			_, _ = fmt.Fprintln(w)
		case "DELETE /v1/control/acp/sessions/acp_1/current-run/run_1":
			w.WriteHeader(http.StatusOK)
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
	stopReason, err := s.Prompt(context.Background(), "hello")
	if err != nil {
		t.Fatalf("prompt after reconnect: %v", err)
	}
	if stopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("stop reason: got %q", stopReason)
	}
	if streamCount != 2 {
		t.Fatalf("expected one reconnect, got %d streams", streamCount)
	}
}

func TestSessionEventMapping(t *testing.T) {
	s := &bridgeSession{
		mapping:    &ACPSessionMapping{ACPSessionID: "acp_1"},
		ctx:        context.Background(),
		coreClient: NewCoreClient("http://localhost", "token"),
	}

	frames := []agui.Event{
		{Type: agui.EventRunStarted, RunID: "run_1"},
		{Type: agui.EventTextMessageContent, RunID: "run_1", Delta: "hello"},
		{Type: agui.EventToolCallStart, RunID: "run_1", ToolCallID: "tc_1", ToolCallName: "read"},
		{Type: agui.EventToolCallEnd, RunID: "run_1", ToolCallID: "tc_1"},
		{Type: agui.EventToolCallResult, RunID: "run_1", ToolCallID: "tc_1", Result: "result"},
		{Type: agui.EventRunFinished, RunID: "run_1"},
	}
	for _, frame := range frames {
		if err := s.handleFrame(context.Background(), "run_1", frame); err != nil {
			t.Fatalf("handle frame %s: %v", frame.Type, err)
		}
	}

	stopReason, err := s.terminalResult()
	if err != nil {
		t.Fatalf("terminal result error: %v", err)
	}
	if stopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("expected end_turn stop reason, got %q", stopReason)
	}
}

func TestSessionEventMappingRunError(t *testing.T) {
	s := &bridgeSession{
		mapping:    &ACPSessionMapping{ACPSessionID: "acp_1"},
		ctx:        context.Background(),
		coreClient: NewCoreClient("http://localhost", "token"),
	}

	frames := []agui.Event{
		{Type: agui.EventRunError, RunID: "run_1", Code: "RUN_CANCELLED", Message: "cancelled by user"},
	}
	for _, frame := range frames {
		if err := s.handleFrame(context.Background(), "run_1", frame); err != nil {
			t.Fatalf("handle frame %s: %v", frame.Type, err)
		}
	}

	stopReason, err := s.terminalResult()
	if err != nil {
		t.Fatalf("expected nil error for RUN_CANCELLED, got %v", err)
	}
	if stopReason != acpsdk.StopReasonCancelled {
		t.Fatalf("expected cancelled stop reason, got %q", stopReason)
	}
}

func TestSessionDedupesDurableAssistantProjectionAfterLiveChunks(t *testing.T) {
	s := &bridgeSession{}
	if got := s.dedupeTextDelta("msg_1", "ACP_"); got != "ACP_" {
		t.Fatalf("first chunk: got %q", got)
	}
	if got := s.dedupeTextDelta("msg_1", "OK"); got != "OK" {
		t.Fatalf("second chunk: got %q", got)
	}
	if got := s.dedupeTextDelta("msg_1", "ACP_OK"); got != "" {
		t.Fatalf("durable replay must be suppressed, got %q", got)
	}
	if got := s.dedupeTextDelta("msg_2", "other"); got != "other" {
		t.Fatalf("different message must remain independent, got %q", got)
	}
}

func TestSessionPromptSendsSelectedModelProfileID(t *testing.T) {
	var createBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/control/acp/sessions/acp_1/runs":
			_ = json.NewDecoder(r.Body).Decode(&createBody)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"run_id": "run_1", "thread_id": "thread_1", "status": "running"})
		case "GET /v1/control/acp/sessions/acp_1/runs/run_1/events":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: {\"type\":\"RUN_FINISHED\",\"runId\":\"run_1\",\"streamCursor\":1000}\n\n")
		case "DELETE /v1/control/acp/sessions/acp_1/current-run/run_1":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	s := &bridgeSession{
		mapping:           &ACPSessionMapping{ACPSessionID: "acp_1", ThreadID: "thread_1"},
		coreClient:        NewCoreClient(server.URL, "token"),
		ctx:               context.Background(),
		selectedProfileID: "profile-other",
	}
	if _, err := s.Prompt(context.Background(), "hello"); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if createBody["model_profile_id"] != "profile-other" {
		t.Fatalf("expected selected profile ID, got %q", createBody["model_profile_id"])
	}
	if _, leaked := createBody["api_key"]; leaked {
		t.Fatal("bridge must never submit an API key")
	}
}
