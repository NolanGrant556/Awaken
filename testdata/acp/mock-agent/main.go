// mock-agent is a minimal ACP v1 agent for Awaken integration tests.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
)

type mockSession struct {
	cancel context.CancelFunc
}

type mockAgent struct {
	mu       sync.Mutex
	conn     *acpsdk.AgentSideConnection
	sessions map[string]*mockSession
}

func newMockAgent() *mockAgent {
	return &mockAgent{sessions: make(map[string]*mockSession)}
}

func (a *mockAgent) SetAgentConnection(conn *acpsdk.AgentSideConnection) {
	a.conn = conn
}

func (a *mockAgent) Initialize(ctx context.Context, params acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	return acpsdk.InitializeResponse{
		ProtocolVersion: params.ProtocolVersion,
		AgentInfo: &acpsdk.Implementation{
			Name:    "mock-agent",
			Version: "0.1.0",
		},
		AgentCapabilities: acpsdk.AgentCapabilities{
			PromptCapabilities: acpsdk.PromptCapabilities{
				Image:           false,
				Audio:           false,
				EmbeddedContext: false,
			},
		},
	}, nil
}

func (a *mockAgent) NewSession(ctx context.Context, params acpsdk.NewSessionRequest) (acpsdk.NewSessionResponse, error) {
	id := fmt.Sprintf("mock-session-%d", time.Now().UnixNano())
	a.mu.Lock()
	a.sessions[id] = &mockSession{}
	a.mu.Unlock()
	return acpsdk.NewSessionResponse{SessionId: acpsdk.SessionId(id)}, nil
}

func (a *mockAgent) LoadSession(ctx context.Context, params acpsdk.LoadSessionRequest) (acpsdk.LoadSessionResponse, error) {
	return acpsdk.LoadSessionResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodSessionLoad)
}

func (a *mockAgent) Prompt(ctx context.Context, params acpsdk.PromptRequest) (acpsdk.PromptResponse, error) {
	text := extractTextPrompt(params.Prompt)
	lower := strings.ToLower(text)

	sid := string(params.SessionId)
	a.mu.Lock()
	s, ok := a.sessions[sid]
	if !ok {
		a.mu.Unlock()
		return acpsdk.PromptResponse{}, fmt.Errorf("session not found")
	}
	if s.cancel != nil {
		s.cancel()
	}
	turnCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	a.mu.Unlock()

	a.sendText(turnCtx, params.SessionId, "Mock agent received: ")
	a.sendText(turnCtx, params.SessionId, text)

	if strings.Contains(lower, "permission") {
		toolCallID := acpsdk.ToolCallId("mock-tool-1")
		a.sendToolCall(turnCtx, params.SessionId, toolCallID, "Mock risky operation", acpsdk.ToolKindExecute, acpsdk.ToolCallStatusPending)
		res, err := a.conn.RequestPermission(turnCtx, acpsdk.RequestPermissionRequest{
			SessionId: params.SessionId,
			ToolCall: acpsdk.ToolCallUpdate{
				ToolCallId: toolCallID,
				Title:      ptr("Mock risky operation"),
				Kind:       ptr(acpsdk.ToolKindExecute),
			},
			Options: []acpsdk.PermissionOption{
				{OptionId: "allow-once", Name: "Allow once", Kind: acpsdk.PermissionOptionKindAllowOnce},
				{OptionId: "reject-once", Name: "Reject once", Kind: acpsdk.PermissionOptionKindRejectOnce},
			},
		})
		if err != nil {
			return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonCancelled}, err
		}
		if res.Outcome.Selected != nil && res.Outcome.Selected.OptionId == "allow-once" {
			a.sendToolCallUpdate(turnCtx, params.SessionId, toolCallID, acpsdk.ToolCallStatusCompleted)
		} else {
			a.sendToolCallUpdate(turnCtx, params.SessionId, toolCallID, acpsdk.ToolCallStatusFailed)
		}
	}

	if strings.Contains(lower, "tool") {
		a.sendToolCall(turnCtx, params.SessionId, acpsdk.ToolCallId("mock-tool-2"), "Mock read", acpsdk.ToolKindRead, acpsdk.ToolCallStatusCompleted)
	}

	select {
	case <-turnCtx.Done():
		return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonCancelled}, turnCtx.Err()
	default:
		return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
	}
}

func (a *mockAgent) sendText(ctx context.Context, sessionID acpsdk.SessionId, text string) {
	if a.conn == nil {
		return
	}
	_ = a.conn.SessionUpdate(ctx, acpsdk.SessionNotification{
		SessionId: sessionID,
		Update: acpsdk.SessionUpdate{
			AgentMessageChunk: &acpsdk.SessionUpdateAgentMessageChunk{
				SessionUpdate: "agent_message_chunk",
				Content:       acpsdk.TextBlock(text),
			},
		},
	})
}

func (a *mockAgent) sendToolCall(ctx context.Context, sessionID acpsdk.SessionId, id acpsdk.ToolCallId, title string, kind acpsdk.ToolKind, status acpsdk.ToolCallStatus) {
	if a.conn == nil {
		return
	}
	_ = a.conn.SessionUpdate(ctx, acpsdk.SessionNotification{
		SessionId: sessionID,
		Update: acpsdk.SessionUpdate{
			ToolCall: &acpsdk.SessionUpdateToolCall{
				SessionUpdate: "tool_call",
				ToolCallId:    id,
				Title:         title,
				Kind:          kind,
				Status:        status,
			},
		},
	})
}

func (a *mockAgent) sendToolCallUpdate(ctx context.Context, sessionID acpsdk.SessionId, id acpsdk.ToolCallId, status acpsdk.ToolCallStatus) {
	if a.conn == nil {
		return
	}
	_ = a.conn.SessionUpdate(ctx, acpsdk.SessionNotification{
		SessionId: sessionID,
		Update: acpsdk.SessionUpdate{
			ToolCallUpdate: &acpsdk.SessionToolCallUpdate{
				SessionUpdate: "tool_call_update",
				ToolCallId:    id,
				Status:        ptr(status),
			},
		},
	})
}

func (a *mockAgent) Cancel(ctx context.Context, params acpsdk.CancelNotification) error {
	a.mu.Lock()
	s, ok := a.sessions[string(params.SessionId)]
	a.mu.Unlock()
	if ok && s != nil && s.cancel != nil {
		s.cancel()
	}
	return nil
}

func (a *mockAgent) CloseSession(ctx context.Context, params acpsdk.CloseSessionRequest) (acpsdk.CloseSessionResponse, error) {
	a.mu.Lock()
	delete(a.sessions, string(params.SessionId))
	a.mu.Unlock()
	return acpsdk.CloseSessionResponse{}, nil
}

func (a *mockAgent) ListSessions(ctx context.Context, params acpsdk.ListSessionsRequest) (acpsdk.ListSessionsResponse, error) {
	return acpsdk.ListSessionsResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodSessionList)
}

func (a *mockAgent) ResumeSession(ctx context.Context, params acpsdk.ResumeSessionRequest) (acpsdk.ResumeSessionResponse, error) {
	return acpsdk.ResumeSessionResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodSessionResume)
}

func (a *mockAgent) SetSessionConfigOption(ctx context.Context, params acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
	return acpsdk.SetSessionConfigOptionResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodSessionSetConfigOption)
}

func (a *mockAgent) SetSessionMode(ctx context.Context, params acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
	return acpsdk.SetSessionModeResponse{}, nil
}

func (a *mockAgent) Authenticate(ctx context.Context, params acpsdk.AuthenticateRequest) (acpsdk.AuthenticateResponse, error) {
	return acpsdk.AuthenticateResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodAuthenticate)
}

func (a *mockAgent) Logout(ctx context.Context, params acpsdk.LogoutRequest) (acpsdk.LogoutResponse, error) {
	return acpsdk.LogoutResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodLogout)
}

func (a *mockAgent) UnstableDidChangeDocument(ctx context.Context, params acpsdk.UnstableDidChangeDocumentNotification) error {
	return nil
}
func (a *mockAgent) UnstableDidCloseDocument(ctx context.Context, params acpsdk.UnstableDidCloseDocumentNotification) error {
	return nil
}
func (a *mockAgent) UnstableDidFocusDocument(ctx context.Context, params acpsdk.UnstableDidFocusDocumentNotification) error {
	return nil
}
func (a *mockAgent) UnstableDidOpenDocument(ctx context.Context, params acpsdk.UnstableDidOpenDocumentNotification) error {
	return nil
}
func (a *mockAgent) UnstableDidSaveDocument(ctx context.Context, params acpsdk.UnstableDidSaveDocumentNotification) error {
	return nil
}
func (a *mockAgent) UnstableAcceptNes(ctx context.Context, params acpsdk.UnstableAcceptNesNotification) error {
	return nil
}
func (a *mockAgent) UnstableCloseNes(ctx context.Context, params acpsdk.UnstableCloseNesRequest) (acpsdk.UnstableCloseNesResponse, error) {
	return acpsdk.UnstableCloseNesResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodNesClose)
}
func (a *mockAgent) UnstableRejectNes(ctx context.Context, params acpsdk.UnstableRejectNesNotification) error {
	return nil
}
func (a *mockAgent) UnstableStartNes(ctx context.Context, params acpsdk.UnstableStartNesRequest) (acpsdk.UnstableStartNesResponse, error) {
	return acpsdk.UnstableStartNesResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodNesStart)
}
func (a *mockAgent) UnstableSuggestNes(ctx context.Context, params acpsdk.UnstableSuggestNesRequest) (acpsdk.UnstableSuggestNesResponse, error) {
	return acpsdk.UnstableSuggestNesResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodNesSuggest)
}
func (a *mockAgent) UnstableDisableProvider(ctx context.Context, params acpsdk.UnstableDisableProviderRequest) (acpsdk.UnstableDisableProviderResponse, error) {
	return acpsdk.UnstableDisableProviderResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodProvidersDisable)
}
func (a *mockAgent) UnstableListProviders(ctx context.Context, params acpsdk.UnstableListProvidersRequest) (acpsdk.UnstableListProvidersResponse, error) {
	return acpsdk.UnstableListProvidersResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodProvidersList)
}
func (a *mockAgent) UnstableSetProvider(ctx context.Context, params acpsdk.UnstableSetProviderRequest) (acpsdk.UnstableSetProviderResponse, error) {
	return acpsdk.UnstableSetProviderResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodProvidersSet)
}
func (a *mockAgent) UnstableDeleteSession(ctx context.Context, params acpsdk.UnstableDeleteSessionRequest) (acpsdk.UnstableDeleteSessionResponse, error) {
	return acpsdk.UnstableDeleteSessionResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodSessionDelete)
}
func (a *mockAgent) UnstableForkSession(ctx context.Context, params acpsdk.UnstableForkSessionRequest) (acpsdk.UnstableForkSessionResponse, error) {
	return acpsdk.UnstableForkSessionResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodSessionFork)
}

func extractTextPrompt(blocks []acpsdk.ContentBlock) string {
	for _, b := range blocks {
		if b.Text != nil {
			return b.Text.Text
		}
	}
	return ""
}

func ptr[T any](v T) *T { return &v }

func main() {
	agent := newMockAgent()
	conn := acpsdk.NewAgentSideConnection(agent, os.Stdout, os.Stdin)
	agent.SetAgentConnection(conn)
	<-conn.Done()
}
