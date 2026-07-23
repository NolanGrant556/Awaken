package agentbridge

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	acpsdk "github.com/coder/acp-go-sdk"
)

// Agent implements the ACP agent interface for Awaken. It exposes Awaken as a
// stdio ACP agent by delegating session lifecycle and prompt execution to a
// managed awaken-core via its HTTP control routes.
type Agent struct {
	CoreClient *CoreClient
	AgentInfo  acpsdk.Implementation
	mu         sync.RWMutex
	sessions   map[string]*bridgeSession
	conn       *acpsdk.AgentSideConnection
}

// NewAgent creates an agent bound to a Core client.
func NewAgent(coreClient *CoreClient) *Agent {
	return &Agent{
		CoreClient: coreClient,
		AgentInfo: acpsdk.Implementation{
			Name:    "awaken",
			Version: "0.1.0",
		},
		sessions: make(map[string]*bridgeSession),
	}
}

// SetConnection wires the agent-side connection so sessions can send outbound
// session updates and permission requests.
func (a *Agent) SetConnection(conn *acpsdk.AgentSideConnection) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.conn = conn
	for _, s := range a.sessions {
		s.setConnection(conn)
	}
}

// CloseAllSessions cancels active runs and removes all local session mappings.
// It is called during bridge shutdown.
func (a *Agent) CloseAllSessions(ctx context.Context) {
	a.mu.Lock()
	sessions := make([]*bridgeSession, 0, len(a.sessions))
	for _, s := range a.sessions {
		sessions = append(sessions, s)
	}
	a.sessions = make(map[string]*bridgeSession)
	a.mu.Unlock()
	for _, s := range sessions {
		if s.cancel != nil {
			s.cancel()
		}
		if s.mapping != nil {
			_ = a.CoreClient.CloseACPSession(ctx, s.mapping.ACPSessionID)
		}
	}
}

// Authenticate rejects authentication requests; Awaken does not require client
// authentication beyond manifest control token.
func (a *Agent) Authenticate(ctx context.Context, params acpsdk.AuthenticateRequest) (acpsdk.AuthenticateResponse, error) {
	_ = ctx
	_ = params
	return acpsdk.AuthenticateResponse{}, acpsdk.NewMethodNotFound("authenticate")
}

// Initialize negotiates the ACP protocol version and advertises capabilities.
func (a *Agent) Initialize(ctx context.Context, params acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	// Accept only protocol v1 as documented.
	if params.ProtocolVersion != acpsdk.ProtocolVersionNumber {
		return acpsdk.InitializeResponse{}, fmt.Errorf("unsupported protocol version %d", params.ProtocolVersion)
	}
	return acpsdk.InitializeResponse{
		ProtocolVersion: acpsdk.ProtocolVersionNumber,
		AgentInfo:       &a.AgentInfo,
		AgentCapabilities: acpsdk.AgentCapabilities{
			PromptCapabilities: acpsdk.PromptCapabilities{},
			SessionCapabilities: acpsdk.SessionCapabilities{
				Close: &acpsdk.SessionCloseCapabilities{},
				List:  &acpsdk.SessionListCapabilities{},
			},
		},
		AuthMethods: []acpsdk.AuthMethod{},
	}, nil
}

// Logout rejects logout requests.
func (a *Agent) Logout(ctx context.Context, params acpsdk.LogoutRequest) (acpsdk.LogoutResponse, error) {
	_ = ctx
	_ = params
	return acpsdk.LogoutResponse{}, acpsdk.NewMethodNotFound("logout")
}

// Cancel forwards a cancel notification to the active run for a session.
func (a *Agent) Cancel(ctx context.Context, params acpsdk.CancelNotification) error {
	a.mu.RLock()
	session, ok := a.sessions[string(params.SessionId)]
	a.mu.RUnlock()
	if !ok {
		return nil
	}
	if session.currentRunID == "" {
		return nil
	}
	return a.CoreClient.CancelRun(ctx, session.currentRunID)
}

// CloseSession closes a session and cancels any active run.
func (a *Agent) CloseSession(ctx context.Context, params acpsdk.CloseSessionRequest) (acpsdk.CloseSessionResponse, error) {
	a.mu.Lock()
	session, ok := a.sessions[string(params.SessionId)]
	if ok {
		delete(a.sessions, string(params.SessionId))
	}
	a.mu.Unlock()
	if session != nil {
		session.cancel()
	}
	if err := a.CoreClient.CloseACPSession(ctx, string(params.SessionId)); err != nil {
		log.Printf("agentbridge: close acp session %s error: %v", params.SessionId, err)
	}
	return acpsdk.CloseSessionResponse{}, nil
}

// ListSessions returns the sessions this bridge knows about.
func (a *Agent) ListSessions(ctx context.Context, params acpsdk.ListSessionsRequest) (acpsdk.ListSessionsResponse, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	sessions := make([]acpsdk.SessionInfo, 0, len(a.sessions))
	for id, s := range a.sessions {
		sessions = append(sessions, acpsdk.SessionInfo{
			SessionId: acpsdk.SessionId(id),
			Cwd:       s.mapping.CanonicalCWD,
		})
	}
	return acpsdk.ListSessionsResponse{Sessions: sessions}, nil
}

// NewSession maps an ACP session to an Awaken project/thread.
func (a *Agent) NewSession(ctx context.Context, params acpsdk.NewSessionRequest) (acpsdk.NewSessionResponse, error) {
	if params.Cwd == "" {
		return acpsdk.NewSessionResponse{}, acpsdk.NewInvalidParams(map[string]any{"field": "cwd", "error": "cwd is required"})
	}
	visibility := "visible"
	if meta, ok := params.Meta["awaken_visibility"].(string); ok && meta != "" {
		if meta != "visible" && meta != "ephemeral" {
			return acpsdk.NewSessionResponse{}, acpsdk.NewInvalidParams(map[string]any{"field": "visibility", "error": "visibility must be visible or ephemeral"})
		}
		visibility = meta
	}
	clientName := ""
	clientVersion := ""
	if params.Meta != nil {
		if v, ok := params.Meta["client_name"].(string); ok {
			clientName = v
		}
		if v, ok := params.Meta["client_version"].(string); ok {
			clientVersion = v
		}
	}
	acpSessionID := newACPSessionID()
	mapping, err := a.CoreClient.CreateACPSession(ctx, acpSessionID, params.Cwd, visibility, clientName, clientVersion)
	if err != nil {
		return acpsdk.NewSessionResponse{}, fmt.Errorf("create core session: %w", err)
	}
	sessionCtx, cancel := context.WithCancel(context.Background())
	session := &bridgeSession{
		mapping:     mapping,
		conn:        nil, // set by Prompt
		ctx:         sessionCtx,
		cancel:      cancel,
		coreClient:  a.CoreClient,
	}
	a.mu.Lock()
	if a.conn != nil {
		session.setConnection(a.conn)
	}
	a.sessions[acpSessionID] = session
	a.mu.Unlock()
	return acpsdk.NewSessionResponse{
		SessionId: acpsdk.SessionId(acpSessionID),
	}, nil
}

// Prompt starts a run for the session and blocks until it reaches a terminal
// state, streaming ACP session updates back to the client.
func (a *Agent) Prompt(ctx context.Context, params acpsdk.PromptRequest) (acpsdk.PromptResponse, error) {
	a.mu.RLock()
	session, ok := a.sessions[string(params.SessionId)]
	a.mu.RUnlock()
	if !ok {
		return acpsdk.PromptResponse{}, acpsdk.NewInvalidParams(map[string]any{"error": "session not found"})
	}
	if len(params.Prompt) == 0 {
		return acpsdk.PromptResponse{}, acpsdk.NewInvalidParams(map[string]any{"error": "prompt is required"})
	}
	text, err := extractTextPrompt(params.Prompt)
	if err != nil {
		return acpsdk.PromptResponse{}, acpsdk.NewInvalidParams(map[string]any{"error": err.Error()})
	}
	if err := session.Prompt(ctx, text); err != nil {
		if errors.Is(err, context.Canceled) {
			return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonCancelled}, nil
		}
		return acpsdk.PromptResponse{}, err
	}
	return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
}

// ResumeSession is not supported.
func (a *Agent) ResumeSession(ctx context.Context, params acpsdk.ResumeSessionRequest) (acpsdk.ResumeSessionResponse, error) {
	_ = ctx
	_ = params
	return acpsdk.ResumeSessionResponse{}, acpsdk.NewMethodNotFound("session/resume")
}

// SetSessionConfigOption is not supported.
func (a *Agent) SetSessionConfigOption(ctx context.Context, params acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
	_ = ctx
	_ = params
	return acpsdk.SetSessionConfigOptionResponse{}, acpsdk.NewMethodNotFound("session/setConfigOption")
}

// SetSessionMode is not supported.
func (a *Agent) SetSessionMode(ctx context.Context, params acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
	_ = ctx
	_ = params
	return acpsdk.SetSessionModeResponse{}, acpsdk.NewMethodNotFound("session/setMode")
}

func extractTextPrompt(blocks []acpsdk.ContentBlock) (string, error) {
	var parts []string
	for _, block := range blocks {
		if block.Text != nil {
			parts = append(parts, block.Text.Text)
			continue
		}
		return "", errors.New("only text prompt blocks are supported")
	}
	if len(parts) == 0 {
		return "", errors.New("no text content found")
	}
	return parts[0], nil
}
