package acp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"awaken/src/tool"
)

// PermissionAwaiter registers the request before calling ready, then waits.
// The callback may publish the request and synchronously resolve it.
type PermissionAwaiter interface {
	AwaitPermission(ctx context.Context, req PermissionRequest, ready func()) (PermissionResolution, error)
	Resolve(interactionID string, res PermissionResolution) error
	Pending(interactionID string) (PendingPermission, bool)
}

// PendingPermission captures the identity of a permission request so that a
// later resolve call can validate the run, thread, runner, session and option.
type PendingPermission struct {
	InteractionID    string
	ThreadID         string
	RunID            string
	RunnerID         string
	ACPSessionID     string
	AllowedOptionIDs []string
}

// Manager owns ACP runner registry and live conversations.
type Manager struct {
	registry      *Registry
	clientFactory ClientFactory
	awaiter       PermissionAwaiter

	mu            sync.RWMutex
	conversations map[ConversationKey]*Conversation
	starting      map[ConversationKey]chan struct{}
	closed        bool
}

// NewManager creates an ACP manager. clientFactory must be non-nil; callers
// that do not need live sessions (e.g. HTTP handler tests) should still pass a
// factory that returns an error rather than nil.
func NewManager(registry *Registry, clientFactory ClientFactory) *Manager {
	return &Manager{
		registry:      registry,
		clientFactory: clientFactory,
		conversations: make(map[ConversationKey]*Conversation),
		starting:      make(map[ConversationKey]chan struct{}),
	}
}

// SetPermissionAwaiter sets the awaiter used for permission requests.
func (m *Manager) SetPermissionAwaiter(a PermissionAwaiter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.awaiter = a
}

// Registry returns the underlying runner registry.
func (m *Manager) Registry() *Registry {
	return m.registry
}

// ProbeRunner performs a real, bounded ACP initialize probe against a runner.
// It distinguishes command missing, startup timeout, and protocol mismatch from
// a ready runner.
func (m *Manager) ProbeRunner(ctx context.Context, runnerID string) error {
	m.mu.RLock()
	factory := m.clientFactory
	m.mu.RUnlock()
	if factory == nil {
		return fmt.Errorf("%s: no client factory", ErrCodeRunnerNotReady)
	}

	snapshot, ok := m.registry.Snapshot(runnerID)
	if !ok {
		return fmt.Errorf("%s: %s", ErrCodeRunnerNotConfigured, runnerID)
	}

	probeCtx, cancel := context.WithTimeout(ctx, time.Duration(snapshot.StartupTimeoutSeconds)*time.Second)
	defer cancel()

	cwd := filepath.Join(os.TempDir(), fmt.Sprintf("awaken-acp-probe-%d", os.Getpid()))
	_ = os.MkdirAll(cwd, 0o700)
	defer os.RemoveAll(cwd)

	client, err := factory(probeCtx, snapshot, cwd)
	if err != nil {
		return err
	}
	defer client.Close()

	if _, err := client.Initialize(probeCtx); err != nil {
		return err
	}
	return nil
}

func emitACPEvent(ctx context.Context, event tool.ACPEvent) {
	if ctx == nil {
		return
	}
	if emit := tool.ACPEventEmitterFromContext(ctx); emit != nil {
		emit(event)
	}
}

func nowUTC() time.Time {
	return time.Now().UTC()
}

// StartSession creates a new ACP session for (threadID, runnerID).
func (m *Manager) StartSession(ctx context.Context, threadID, runnerID, workspaceRoot string) (*Conversation, error) {
	key := ConversationKey{ThreadID: threadID, RunnerID: runnerID}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, fmt.Errorf("%s: manager closed", ErrCodeTransportClosed)
	}
	if existing, ok := m.conversations[key]; ok && existing.State() != ConversationStateClosed && existing.State() != ConversationStateFailed {
		m.mu.Unlock()
		return nil, fmt.Errorf("%s: session already exists", ErrCodeSessionConflict)
	}
	if m.clientFactory == nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("%s: no client factory", ErrCodeRunnerNotReady)
	}
	if _, starting := m.starting[key]; starting {
		m.mu.Unlock()
		return nil, fmt.Errorf("%s: session is already starting", ErrCodeSessionConflict)
	}
	done := make(chan struct{})
	m.starting[key] = done
	m.mu.Unlock()

	cleanupStarting := func() {
		m.mu.Lock()
		delete(m.starting, key)
		m.mu.Unlock()
		close(done)
	}

	snapshot, ok := m.registry.Snapshot(runnerID)
	if !ok {
		cleanupStarting()
		return nil, fmt.Errorf("%s: %s", ErrCodeRunnerNotConfigured, runnerID)
	}
	if err := m.registry.Probe(runnerID); err != nil {
		cleanupStarting()
		return nil, err
	}

	client, err := m.clientFactory(ctx, snapshot, workspaceRoot)
	if err != nil {
		cleanupStarting()
		return nil, err
	}
	caps, err := client.Initialize(ctx)
	if err != nil {
		_ = client.Close()
		cleanupStarting()
		return nil, err
	}
	sessionID, err := client.NewSession(ctx, workspaceRoot)
	if err != nil {
		_ = client.Close()
		cleanupStarting()
		return nil, err
	}

	conv := newConversation(key, workspaceRoot, snapshot, client, caps, sessionID)
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		_ = conv.Close(ctx)
		cleanupStarting()
		return nil, fmt.Errorf("%s: manager closed", ErrCodeTransportClosed)
	}
	m.conversations[key] = conv
	delete(m.starting, key)
	m.mu.Unlock()
	close(done)
	m.watchConversation(conv)
	return conv, nil
}

// watchConversation invalidates a conversation as soon as a lifecycle-aware
// client reports that its process/transport exited. This covers idle exits,
// where no prompt write would otherwise discover the dead pipe.
func (m *Manager) watchConversation(conv *Conversation) {
	lifecycle, ok := conv.Lifecycle()
	if !ok || lifecycle.Done() == nil {
		return
	}
	go func() {
		<-lifecycle.Done()
		state := conv.State()
		if state == ConversationStateClosing || state == ConversationStateClosed || state == ConversationStateFailed {
			return
		}
		message := "ACP runner process exited"
		if err := lifecycle.ExitError(); err != nil {
			message = fmt.Sprintf("ACP runner process exited: %v", err)
		}
		m.failConversation(conv, ErrCodeTransportClosed, message)
	}()
}

func (m *Manager) failConversation(conv *Conversation, errorCode, errorMessage string) {
	if client := conv.Fail(errorCode, errorMessage); client != nil {
		_ = client.Close()
	}
}

// GetConversation returns an existing conversation.
func (m *Manager) GetConversation(threadID, runnerID string) (*Conversation, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	conv, ok := m.conversations[ConversationKey{ThreadID: threadID, RunnerID: runnerID}]
	return conv, ok
}

// ListSessions returns safe summaries of all conversations.
func (m *Manager) ListSessions() []SessionSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]SessionSummary, 0, len(m.conversations))
	for _, conv := range m.conversations {
		out = append(out, conv.Summary())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ThreadID == out[j].ThreadID {
			return out[i].RunnerID < out[j].RunnerID
		}
		return out[i].ThreadID < out[j].ThreadID
	})
	return out
}

// Prompt sends a message in an existing session and waits for the turn to complete.
func (m *Manager) Prompt(ctx context.Context, threadID, runnerID, hostRunID, toolCallID, message string, handler UpdateHandler, maxRuntimeSeconds int) (TurnResult, error) {
	conv, ok := m.GetConversation(threadID, runnerID)
	if !ok {
		return TurnResult{}, fmt.Errorf("%s: %s/%s", ErrCodeSessionMissing, threadID, runnerID)
	}
	turn, err := conv.StartTurn(hostRunID, toolCallID)
	if err != nil {
		return TurnResult{}, err
	}

	emitACPEvent(ctx, tool.ACPEvent{
		Type:       tool.ACPEventTypeTurnStarted,
		ThreadID:   threadID,
		ToolCallID: toolCallID,
		Content:    fmt.Sprintf("ACP turn started on runner %s", runnerID),
		Diagnostic: map[string]any{
			"runner_id":      runnerID,
			"acp_session_id": conv.ACPSessionID,
			"turn_id":        turn.TurnID,
		},
	})

	promptCtx, promptCancel := m.turnContext(ctx, turn, maxRuntimeSeconds)
	defer promptCancel()

	permissionHandler := &managerPermissionHandler{manager: m, conv: conv, turn: turn, ctx: ctx}
	client := conv.Client()
	var result TurnResult
	if client == nil {
		err = fmt.Errorf("%s: transport is not available", ErrCodeTransportClosed)
	} else {
		result, err = client.Prompt(promptCtx, conv.ACPSessionID, turn.TurnID, message, handler, permissionHandler)
	}
	result = m.normalizeTurnResult(result, err)
	if err != nil && result.ErrorCode != "" && !strings.Contains(err.Error(), result.ErrorCode) {
		err = fmt.Errorf("%s: %w", result.ErrorCode, err)
	}
	if result.ErrorCode == ErrCodeTransportClosed {
		m.failConversation(conv, result.ErrorCode, result.ErrorMessage)
	} else {
		conv.FinishTurn(result)
	}

	finishEvent := tool.ACPEvent{
		Type:       tool.ACPEventTypeTurnFinished,
		ThreadID:   threadID,
		ToolCallID: toolCallID,
		Content:    result.AssistantText,
		Diagnostic: map[string]any{
			"runner_id":      runnerID,
			"acp_session_id": conv.ACPSessionID,
			"turn_id":        turn.TurnID,
			"stop_reason":    result.StopReason,
			"truncated":      result.Truncated,
			"tool_summary": map[string]any{
				"completed": result.ToolSummary.Completed,
				"failed":    result.ToolSummary.Failed,
				"total":     result.ToolSummary.Total,
			},
		},
	}
	if result.ErrorCode != "" {
		finishEvent.Diagnostic["error_code"] = result.ErrorCode
		finishEvent.Diagnostic["error_message"] = result.ErrorMessage
	}
	if err != nil {
		finishEvent.Diagnostic["interrupted"] = true
	}
	emitACPEvent(ctx, finishEvent)

	return result, err
}

// Message is a convenience alias for Prompt.
func (m *Manager) Message(ctx context.Context, threadID, runnerID, hostRunID, toolCallID, message string, handler UpdateHandler, maxRuntimeSeconds int) (TurnResult, error) {
	return m.Prompt(ctx, threadID, runnerID, hostRunID, toolCallID, message, handler, maxRuntimeSeconds)
}

// turnContext returns a context bounded by the runner turn timeout, the tool
// parameter max_runtime_seconds, and the parent run deadline. The smallest
// positive deadline wins.
func (m *Manager) turnContext(ctx context.Context, turn *TurnState, maxRuntimeSeconds int) (context.Context, context.CancelFunc) {
	now := nowUTC()
	deadline := turn.ExpiresAt
	if maxRuntimeSeconds > 0 {
		candidate := now.Add(time.Duration(maxRuntimeSeconds) * time.Second)
		if candidate.Before(deadline) {
			deadline = candidate
		}
	}
	if parentDeadline, ok := ctx.Deadline(); ok && parentDeadline.Before(deadline) {
		deadline = parentDeadline
	}
	return context.WithDeadline(ctx, deadline)
}

func (m *Manager) normalizeTurnResult(result TurnResult, err error) TurnResult {
	if err == nil {
		return result
	}
	if result.ErrorCode == "" {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			result.ErrorCode = ErrCodeTurnTimeout
			result.ErrorMessage = "turn deadline exceeded"
			result.StopReason = string(ConversationStateCancelling)
		case errors.Is(err, context.Canceled):
			result.ErrorCode = ErrCodeTurnCancelled
			result.ErrorMessage = "turn cancelled"
			result.StopReason = string(ConversationStateCancelling)
		default:
			result.ErrorCode = errorCodeFromError(err)
			result.ErrorMessage = err.Error()
		}
	}
	if result.ErrorCode == ErrCodeTransportClosed && result.SideEffectState == "" {
		result.SideEffectState = "unknown"
	}
	return result
}

func errorCodeFromError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	for _, code := range []string{
		ErrCodeCapabilityUnsupported,
		ErrCodeOutputLimitExceeded,
		ErrCodePermissionDenied,
		ErrCodePermissionExpired,
		ErrCodeSessionConflict,
		ErrCodeSessionMissing,
		ErrCodeTransportClosed,
	} {
		if strings.Contains(message, code) {
			return code
		}
	}
	return ErrCodeTransportClosed
}

// Cancel cancels the active turn (but keeps the session).
func (m *Manager) Cancel(ctx context.Context, threadID, runnerID string) error {
	conv, ok := m.GetConversation(threadID, runnerID)
	if !ok {
		return fmt.Errorf("%s: %s/%s", ErrCodeSessionMissing, threadID, runnerID)
	}
	var turnID, toolCallID string
	if turn := conv.ActiveTurn(); turn != nil {
		turnID = turn.TurnID
		toolCallID = turn.ToolCallID
	}
	emitACPEvent(ctx, tool.ACPEvent{
		Type:       tool.ACPEventTypeSessionInterrupted,
		ThreadID:   threadID,
		ToolCallID: toolCallID,
		Content:    fmt.Sprintf("ACP turn cancelled on runner %s", runnerID),
		Diagnostic: map[string]any{
			"runner_id":      runnerID,
			"acp_session_id": conv.ACPSessionID,
			"turn_id":        turnID,
			"reason":         "cancel_requested",
		},
	})
	return conv.Cancel(ctx)
}

// CloseSession closes a specific session.
func (m *Manager) CloseSession(ctx context.Context, threadID, runnerID string) error {
	m.mu.Lock()
	key := ConversationKey{ThreadID: threadID, RunnerID: runnerID}
	conv, ok := m.conversations[key]
	if ok {
		delete(m.conversations, key)
	}
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("%s: %s/%s", ErrCodeSessionMissing, threadID, runnerID)
	}
	emitACPEvent(ctx, tool.ACPEvent{
		Type:     tool.ACPEventTypeSessionClosed,
		ThreadID: threadID,
		Content:  fmt.Sprintf("ACP session closed on runner %s", runnerID),
		Diagnostic: map[string]any{
			"runner_id":      runnerID,
			"acp_session_id": conv.ACPSessionID,
		},
	})
	return conv.Close(ctx)
}

// CancelAllForThread cancels every active turn in every conversation for the thread.
func (m *Manager) CancelAllForThread(ctx context.Context, threadID string) {
	m.mu.RLock()
	convs := make([]*Conversation, 0)
	for key, conv := range m.conversations {
		if key.ThreadID == threadID {
			convs = append(convs, conv)
		}
	}
	m.mu.RUnlock()
	for _, conv := range convs {
		_ = conv.Cancel(ctx)
	}
}

// CloseAll closes all conversations with a bounded total budget.
func (m *Manager) CloseAll(ctx context.Context) error {
	m.mu.Lock()
	m.closed = true
	convs := make([]*Conversation, 0, len(m.conversations))
	for _, conv := range m.conversations {
		convs = append(convs, conv)
	}
	m.conversations = make(map[ConversationKey]*Conversation)
	m.mu.Unlock()

	if len(convs) == 0 {
		return nil
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Second)
	}
	perConv := time.Until(deadline) / time.Duration(len(convs))
	if perConv < 500*time.Millisecond {
		perConv = 500 * time.Millisecond
	}

	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error
	for _, conv := range convs {
		wg.Add(1)
		go func(c *Conversation) {
			defer wg.Done()
			closeCtx, cancel := context.WithTimeout(context.Background(), perConv)
			defer cancel()
			if err := c.Close(closeCtx); err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
			}
		}(conv)
	}
	wg.Wait()
	return firstErr
}

// ResolvePermission resolves a pending permission request by interaction ID.
func (m *Manager) ResolvePermission(runID, interactionID, optionID string, cancelled bool) error {
	m.mu.RLock()
	awaiter := m.awaiter
	m.mu.RUnlock()
	if awaiter == nil {
		return fmt.Errorf("%s: no pending permission awaiter", ErrCodePermissionExpired)
	}
	pending, ok := awaiter.Pending(interactionID)
	if !ok {
		return fmt.Errorf("%s: %s", ErrCodePermissionExpired, interactionID)
	}
	if pending.RunID != runID {
		return fmt.Errorf("%s: run_id mismatch", ErrCodePermissionExpired)
	}
	if !cancelled {
		allowed := false
		for _, id := range pending.AllowedOptionIDs {
			if id == optionID {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("%s: option %q is not allowed", ErrCodePermissionDenied, optionID)
		}
	}
	return awaiter.Resolve(interactionID, PermissionResolution{InteractionID: interactionID, OptionID: optionID, Cancelled: cancelled})
}

type managerPermissionHandler struct {
	manager *Manager
	conv    *Conversation
	turn    *TurnState
	ctx     context.Context
}

func (h *managerPermissionHandler) RequestPermission(ctx context.Context, req PermissionRequest) (PermissionResolution, error) {
	if h.conv.RunnerSnapshot.PermissionMode == PermissionModeDeny {
		emitACPEvent(h.ctx, tool.ACPEvent{
			Type:          tool.ACPEventTypePermissionResolved,
			ThreadID:      h.conv.Key.ThreadID,
			ToolCallID:    h.turn.ToolCallID,
			InteractionID: req.InteractionID,
			Content:       "denied by permission mode",
			Diagnostic: map[string]any{
				"runner_id":      h.conv.Key.RunnerID,
				"acp_session_id": h.conv.ACPSessionID,
				"turn_id":        h.turn.TurnID,
				"cancelled":      true,
				"reason":         "permission_mode_deny",
			},
		})
		return PermissionResolution{InteractionID: req.InteractionID, Cancelled: true}, nil
	}
	if req.InteractionID == "" {
		req.InteractionID = newInteractionID()
	}
	req.ThreadID = h.conv.Key.ThreadID
	req.RunID = h.turn.HostRunID
	req.RunnerID = h.conv.Key.RunnerID
	req.ACPSessionID = h.conv.ACPSessionID
	req.ExpiresAt = time.Now().Add(time.Duration(h.conv.RunnerSnapshot.TurnTimeoutSeconds) * time.Second)

	h.manager.mu.RLock()
	awaiter := h.manager.awaiter
	h.manager.mu.RUnlock()
	if awaiter == nil {
		emitACPEvent(h.ctx, tool.ACPEvent{
			Type:          tool.ACPEventTypePermissionResolved,
			ThreadID:      h.conv.Key.ThreadID,
			ToolCallID:    h.turn.ToolCallID,
			InteractionID: req.InteractionID,
			Content:       "cancelled: no awaiter",
			Diagnostic: map[string]any{
				"runner_id":      h.conv.Key.RunnerID,
				"acp_session_id": h.conv.ACPSessionID,
				"turn_id":        h.turn.TurnID,
				"cancelled":      true,
				"reason":         "no_permission_awaiter",
			},
		})
		return PermissionResolution{InteractionID: req.InteractionID, Cancelled: true}, nil
	}
	defer h.conv.ClearPendingPermission()
	res, err := awaiter.AwaitPermission(ctx, req, func() {
		h.conv.SetPendingPermission(&req)

		options := make([]map[string]any, 0, len(req.Options))
		for _, opt := range req.Options {
			options = append(options, map[string]any{
				"option_id": opt.OptionID,
				"name":      opt.Name,
				"kind":      opt.Kind,
			})
		}
		emitACPEvent(h.ctx, tool.ACPEvent{
			Type:          tool.ACPEventTypePermissionRequested,
			ThreadID:      h.conv.Key.ThreadID,
			ToolCallID:    h.turn.ToolCallID,
			InteractionID: req.InteractionID,
			Content:       req.Title,
			Diagnostic: map[string]any{
				"runner_id":      h.conv.Key.RunnerID,
				"acp_session_id": h.conv.ACPSessionID,
				"turn_id":        h.turn.TurnID,
				"kind":           req.Kind,
				"title":          req.Title,
				"options":        options,
				"expires_at":     req.ExpiresAt.UTC().Format(time.RFC3339Nano),
			},
		})

	})
	resolutionContent := res.OptionID
	if resolutionContent == "" {
		resolutionContent = "cancelled"
	}
	emitACPEvent(h.ctx, tool.ACPEvent{
		Type:          tool.ACPEventTypePermissionResolved,
		ThreadID:      h.conv.Key.ThreadID,
		ToolCallID:    h.turn.ToolCallID,
		InteractionID: res.InteractionID,
		Content:       resolutionContent,
		Diagnostic: map[string]any{
			"runner_id":      h.conv.Key.RunnerID,
			"acp_session_id": h.conv.ACPSessionID,
			"turn_id":        h.turn.TurnID,
			"option_id":      res.OptionID,
			"cancelled":      res.Cancelled,
			"await_error":    err != nil,
		},
	})
	return res, err
}

// ChanPermissionAwaiter is a simple in-memory awaiter backed by channels.
type ChanPermissionAwaiter struct {
	mu      sync.Mutex
	pending map[string]pendingRecord
}

type pendingRecord struct {
	ctx context.Context
	ch  chan PermissionResolution
	req PendingPermission
}

// NewChanPermissionAwaiter creates a channel-based permission awaiter.
func NewChanPermissionAwaiter() *ChanPermissionAwaiter {
	return &ChanPermissionAwaiter{pending: make(map[string]pendingRecord)}
}

func (a *ChanPermissionAwaiter) AwaitPermission(ctx context.Context, req PermissionRequest, ready func()) (PermissionResolution, error) {
	if !req.ExpiresAt.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, req.ExpiresAt)
		defer cancel()
	}
	if err := ctx.Err(); err != nil {
		return PermissionResolution{InteractionID: req.InteractionID, Cancelled: true}, err
	}

	allowed := make([]string, 0, len(req.Options))
	for _, opt := range req.Options {
		allowed = append(allowed, opt.OptionID)
	}
	ch := make(chan PermissionResolution, 1)
	a.mu.Lock()
	if _, exists := a.pending[req.InteractionID]; exists {
		a.mu.Unlock()
		return PermissionResolution{}, fmt.Errorf("%s: duplicate interaction", ErrCodePermissionExpired)
	}
	a.pending[req.InteractionID] = pendingRecord{
		ctx: ctx,
		ch:  ch,
		req: PendingPermission{
			InteractionID:    req.InteractionID,
			ThreadID:         req.ThreadID,
			RunID:            req.RunID,
			RunnerID:         req.RunnerID,
			ACPSessionID:     req.ACPSessionID,
			AllowedOptionIDs: allowed,
		},
	}
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		if a.pending[req.InteractionID].ch == ch {
			delete(a.pending, req.InteractionID)
		}
		a.mu.Unlock()
	}()
	if ready != nil {
		ready()
	}
	select {
	case <-ctx.Done():
		return PermissionResolution{InteractionID: req.InteractionID, Cancelled: true}, ctx.Err()
	case res := <-ch:
		return res, nil
	}
}

func newInteractionID() string {
	return fmt.Sprintf("acp_perm_%d", time.Now().UnixNano())
}

func (a *ChanPermissionAwaiter) Resolve(interactionID string, res PermissionResolution) error {
	a.mu.Lock()
	rec, ok := a.pending[interactionID]
	if ok {
		delete(a.pending, interactionID)
	}
	a.mu.Unlock()
	if !ok || rec.ctx.Err() != nil {
		return fmt.Errorf("%s: %s", ErrCodePermissionExpired, interactionID)
	}
	select {
	case rec.ch <- res:
		return nil
	default:
		return fmt.Errorf("%s: %s already resolved", ErrCodePermissionExpired, interactionID)
	}
}

func (a *ChanPermissionAwaiter) Pending(interactionID string) (PendingPermission, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	rec, ok := a.pending[interactionID]
	return rec.req, ok
}
