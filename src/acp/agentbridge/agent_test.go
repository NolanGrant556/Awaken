package agentbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
)

func TestAgentInitializeRejectsUnsupportedVersion(t *testing.T) {
	agent := NewAgent(nil)
	_, err := agent.Initialize(context.Background(), acpsdk.InitializeRequest{
		ProtocolVersion: 999,
	})
	if err == nil {
		t.Fatal("expected unsupported version error")
	}
}

func TestAgentInitializeReturnsCapabilities(t *testing.T) {
	agent := NewAgent(nil)
	resp, err := agent.Initialize(context.Background(), acpsdk.InitializeRequest{
		ProtocolVersion: acpsdk.ProtocolVersionNumber,
	})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if resp.ProtocolVersion != acpsdk.ProtocolVersionNumber {
		t.Fatalf("unexpected protocol version: %d", resp.ProtocolVersion)
	}
	if resp.AgentCapabilities.SessionCapabilities.Close == nil {
		t.Fatal("expected close capability")
	}
	if resp.AgentCapabilities.SessionCapabilities.List == nil {
		t.Fatal("expected list capability")
	}
}

func TestAgentNewSessionCreatesMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method+" "+r.URL.Path != "POST /v1/control/acp/sessions" {
			http.NotFound(w, r)
			return
		}
		var req map[string]string
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"acp_session_id": req["acp_session_id"],
			"project_id":     "proj_1",
			"thread_id":      "thread_1",
			"canonical_cwd":  req["cwd"],
			"visibility":     req["visibility"],
		})
	}))
	defer server.Close()

	agent := NewAgent(NewCoreClient(server.URL, "token"))
	resp, err := agent.NewSession(context.Background(), acpsdk.NewSessionRequest{
		Cwd:        "/tmp/ws",
		McpServers: []acpsdk.McpServer{},
	})
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	if resp.SessionId == "" {
		t.Fatal("expected session id")
	}
	if len(agent.sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(agent.sessions))
	}
}

func TestAgentPromptRequiresExistingSession(t *testing.T) {
	agent := NewAgent(nil)
	_, err := agent.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "missing",
		Prompt:    []acpsdk.ContentBlock{{Text: &acpsdk.ContentBlockText{Text: "hi"}}},
	})
	if err == nil {
		t.Fatal("expected error for missing session")
	}
}

func TestAgentListSessions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"acp_session_id": "acp_1",
			"project_id":     "proj_1",
			"thread_id":      "thread_1",
			"canonical_cwd":  "/tmp/ws",
			"visibility":     "visible",
		})
	}))
	defer server.Close()

	agent := NewAgent(NewCoreClient(server.URL, "token"))
	_, _ = agent.NewSession(context.Background(), acpsdk.NewSessionRequest{
		Cwd:        "/tmp/ws",
		McpServers: []acpsdk.McpServer{},
	})
	resp, err := agent.ListSessions(context.Background(), acpsdk.ListSessionsRequest{})
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(resp.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(resp.Sessions))
	}
}
