package acp

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ClientFactory creates a protocol-level ACP client for a runner snapshot.
type ClientFactory func(ctx context.Context, snapshot RunnerSnapshot, cwd string) (Client, error)

// Conversation represents one (thread, runner) ACP session.
type Conversation struct {
	Key               ConversationKey
	ACPSessionID      string
	WorkspaceRoot     string
	RunnerSnapshot    RunnerSnapshot
	Capabilities      AgentCapabilitiesSnapshot
	client            Client
	mu                sync.Mutex
	state             ConversationState
	activeTurn        *TurnState
	pendingPermission *PermissionRequest
	startedAt         time.Time
	updatedAt         time.Time
}

func newConversation(key ConversationKey, workspaceRoot string, snapshot RunnerSnapshot, client Client, caps AgentCapabilitiesSnapshot, sessionID string) *Conversation {
	now := time.Now().UTC()
	return &Conversation{
		Key:            key,
		ACPSessionID:   sessionID,
		WorkspaceRoot:  workspaceRoot,
		RunnerSnapshot: snapshot,
		Capabilities:   caps,
		client:         client,
		state:          ConversationStateReady,
		startedAt:      now,
		updatedAt:      now,
	}
}

// State returns the current conversation state.
func (c *Conversation) State() ConversationState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

func (c *Conversation) setState(s ConversationState) {
	c.state = s
	c.updatedAt = time.Now().UTC()
}

// Summary returns a safe, non-secret view.
func (c *Conversation) Summary() SessionSummary {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := SessionSummary{
		ThreadID:      c.Key.ThreadID,
		RunnerID:      c.Key.RunnerID,
		ACPSessionID:  c.ACPSessionID,
		State:         c.state,
		WorkspaceRoot: c.WorkspaceRoot,
		StartedAt:     c.startedAt,
		UpdatedAt:     c.updatedAt,
	}
	if c.activeTurn != nil {
		s.ActiveTurnID = c.activeTurn.TurnID
	}
	if c.pendingPermission != nil {
		p := *c.pendingPermission
		if len(p.Options) > 0 {
			p.Options = append([]PermissionOption(nil), p.Options...)
		}
		s.PendingPermission = &p
	}
	return s
}

// StartTurn starts a new prompt turn. It returns an error if a turn is already active.
func (c *Conversation) StartTurn(hostRunID, toolCallID string) (*TurnState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.activeTurn != nil {
		return nil, ErrTurnInProgress
	}
	if c.state != ConversationStateReady {
		return nil, fmt.Errorf("%s: conversation is %s", ErrCodeSessionConflict, c.state)
	}
	c.setState(ConversationStatePrompting)
	now := time.Now().UTC()
	turn := &TurnState{
		TurnID:       newTurnID(),
		HostRunID:    hostRunID,
		ToolCallID:   toolCallID,
		StartedAt:    now,
		ExpiresAt:    now.Add(time.Duration(c.RunnerSnapshot.TurnTimeoutSeconds) * time.Second),
		ToolSnapshot: make(map[string]ToolCallSnapshot),
	}
	c.activeTurn = turn
	return turn, nil
}

// FinishTurn releases the active turn and returns the accumulated state.
func (c *Conversation) FinishTurn(result TurnResult) {
	c.mu.Lock()
	c.activeTurn = nil
	c.pendingPermission = nil
	if c.state == ConversationStatePrompting || c.state == ConversationStateWaitingPermission || c.state == ConversationStateCancelling {
		c.setState(ConversationStateReady)
	}
	c.mu.Unlock()
}

// SetPendingPermission registers a permission request for this turn.
func (c *Conversation) SetPendingPermission(req *PermissionRequest) {
	c.mu.Lock()
	c.pendingPermission = req
	c.setState(ConversationStateWaitingPermission)
	c.mu.Unlock()
}

// ClearPendingPermission removes the pending request.
func (c *Conversation) ClearPendingPermission() {
	c.mu.Lock()
	c.pendingPermission = nil
	if c.state == ConversationStateWaitingPermission {
		c.setState(ConversationStatePrompting)
	}
	c.mu.Unlock()
}

// PendingPermission returns the current pending permission request.
func (c *Conversation) PendingPermission() *PermissionRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pendingPermission == nil {
		return nil
	}
	p := *c.pendingPermission
	if len(p.Options) > 0 {
		p.Options = append([]PermissionOption(nil), p.Options...)
	}
	return &p
}

// ActiveTurn returns the active turn.
func (c *Conversation) ActiveTurn() *TurnState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.activeTurn
}

// Cancel transitions the conversation to cancelling and asks the runner to cancel
// the active turn. It keeps the session alive so that subsequent turns can reuse
// it. Cancel is idempotent: calling it when the conversation is not prompting or
// waiting for permission is a no-op.
func (c *Conversation) Cancel(ctx context.Context) error {
	c.mu.Lock()
	if c.state != ConversationStatePrompting && c.state != ConversationStateWaitingPermission {
		c.mu.Unlock()
		return nil
	}
	c.setState(ConversationStateCancelling)
	client := c.client
	c.mu.Unlock()

	if client == nil {
		return nil
	}
	return client.Cancel(ctx, c.ACPSessionID)
}

// Close closes the conversation, its session, and the underlying process.
func (c *Conversation) Close(ctx context.Context) error {
	c.mu.Lock()
	if c.state == ConversationStateClosed || c.state == ConversationStateFailed {
		c.mu.Unlock()
		return nil
	}
	c.setState(ConversationStateClosing)
	client := c.client
	c.mu.Unlock()

	if client == nil {
		c.mu.Lock()
		c.setState(ConversationStateClosed)
		c.mu.Unlock()
		return nil
	}

	// Best-effort two-phase cleanup: cancel the active turn, close the ACP
	// session, then terminate the process tree and wait for it to exit.
	_ = client.Cancel(ctx, c.ACPSessionID)
	_ = client.CloseSession(ctx, c.ACPSessionID)
	err := client.Close()

	c.mu.Lock()
	if err != nil {
		c.setState(ConversationStateFailed)
	} else {
		c.setState(ConversationStateClosed)
	}
	c.mu.Unlock()
	return err
}

func newTurnID() string {
	return fmt.Sprintf("acp_turn_%d", time.Now().UnixNano())
}
