package agentbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"awaken/src/protocol/agui"
	acpsdk "github.com/coder/acp-go-sdk"
)

// bridgeSession tracks one inbound ACP session and its current run.
type bridgeSession struct {
	mapping            *ACPSessionMapping
	conn               *acpsdk.AgentSideConnection
	ctx                context.Context
	cancel             context.CancelFunc
	coreClient         *CoreClient
	mu                 sync.Mutex
	currentRunID       string
	pendingPermissions map[string][]agui.ACPPermissionOption
	terminalErr        error
	stopReason         acpsdk.StopReason
}

// Prompt starts a run for this session and streams ACP updates until terminal.
func (s *bridgeSession) Prompt(ctx context.Context, text string) (acpsdk.StopReason, error) {
	s.mu.Lock()
	if s.currentRunID != "" {
		s.mu.Unlock()
		return "", errors.New("session already has an active prompt")
	}
	s.mu.Unlock()

	runCtx, runCancel := context.WithCancel(s.ctx)
	defer runCancel()

	run, err := s.coreClient.CreateRunForSession(runCtx, s.mapping.ACPSessionID, s.mapping.ThreadID, text, s.mapping.CanonicalCWD)
	if err != nil {
		return "", fmt.Errorf("create run: %w", err)
	}
	s.mu.Lock()
	s.currentRunID = run.RunID
	s.pendingPermissions = make(map[string][]agui.ACPPermissionOption)
	s.terminalErr = nil
	s.stopReason = ""
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.currentRunID = ""
		s.pendingPermissions = nil
		s.mu.Unlock()
		_ = s.coreClient.ClearCurrentRun(context.Background(), s.mapping.ACPSessionID)
	}()

	if err := s.coreClient.SetCurrentRun(runCtx, s.mapping.ACPSessionID, run.RunID); err != nil {
		return "", fmt.Errorf("set current run: %w", err)
	}

	frames, errs := s.coreClient.StreamRunEventsControl(runCtx, s.mapping.ACPSessionID, run.RunID, 0)
	for {
		select {
		case <-ctx.Done():
			runCancel()
			s.cancelAndWait(run.RunID)
			return acpsdk.StopReasonCancelled, nil
		case <-s.ctx.Done():
			runCancel()
			s.cancelAndWait(run.RunID)
			return acpsdk.StopReasonCancelled, nil
		case err := <-errs:
			if err != nil {
				return "", fmt.Errorf("event stream: %w", err)
			}
			return s.terminalResult()
		case frame, ok := <-frames:
			if !ok {
				return s.terminalResult()
			}
			if err := s.handleFrame(run.RunID, frame); err != nil {
				return "", err
			}
		}
	}
}

func (s *bridgeSession) terminalResult() (acpsdk.StopReason, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.terminalErr != nil {
		return s.stopReason, s.terminalErr
	}
	if s.stopReason != "" {
		return s.stopReason, nil
	}
	return acpsdk.StopReasonEndTurn, nil
}

func (s *bridgeSession) cancelAndWait(runID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.coreClient.CancelRunControl(ctx, s.mapping.ACPSessionID, runID)
	frames, errs := s.coreClient.StreamRunEventsControl(ctx, s.mapping.ACPSessionID, runID, 0)
	for range frames {
	}
	select {
	case <-errs:
	case <-ctx.Done():
	}
}

func (s *bridgeSession) handleFrame(runID string, frame EventFrame) error {
	switch frame.Type {
	case agui.EventRunStarted:
		return nil
	case agui.EventRunFinished:
		s.mu.Lock()
		s.stopReason = acpsdk.StopReasonEndTurn
		s.terminalErr = nil
		s.mu.Unlock()
		return nil
	case agui.EventRunError:
		s.mu.Lock()
		s.terminalErr = fmt.Errorf("run error: %s: %s", frame.Code, frame.Message)
		s.stopReason = acpsdk.StopReasonEndTurn
		if frame.Code == "RUN_CANCELLED" {
			s.stopReason = acpsdk.StopReasonCancelled
			s.terminalErr = nil
		}
		s.mu.Unlock()
		return nil
	case agui.EventTextMessageContent:
		return s.emitTextDelta(frame.Delta)
	case agui.EventToolCallStart:
		return s.emitToolCall(frame.ToolCallID, frame.ToolCallName, string(acpsdk.ToolCallStatusPending))
	case agui.EventToolCallEnd:
		return s.emitToolCallUpdate(frame.ToolCallID, string(acpsdk.ToolCallStatusCompleted), "")
	case agui.EventToolCallResult:
		content := frame.Result
		if content == "" {
			content = frame.Content
		}
		status := string(acpsdk.ToolCallStatusCompleted)
		if frame.Code != "" || frame.Message != "" {
			status = string(acpsdk.ToolCallStatusFailed)
		}
		return s.emitToolCallUpdate(frame.ToolCallID, status, content)
	case agui.EventAwakenACPPermissionRequested:
		return s.handlePermissionRequest(runID, frame)
	default:
		// Drop unknown frames silently.
		return nil
	}
}

func (s *bridgeSession) emitTextDelta(text string) error {
	if s.conn == nil {
		return nil
	}
	return s.conn.SessionUpdate(s.ctx, acpsdk.SessionNotification{
		SessionId: acpsdk.SessionId(s.mapping.ACPSessionID),
		Update: acpsdk.SessionUpdate{
			AgentMessageChunk: &acpsdk.SessionUpdateAgentMessageChunk{
				SessionUpdate: "agent_message_chunk",
				Content:       acpsdk.ContentBlock{Text: &acpsdk.ContentBlockText{Text: text}},
			},
		},
	})
}

func (s *bridgeSession) emitToolCall(toolCallID, title, status string) error {
	if s.conn == nil || toolCallID == "" {
		return nil
	}
	return s.conn.SessionUpdate(s.ctx, acpsdk.SessionNotification{
		SessionId: acpsdk.SessionId(s.mapping.ACPSessionID),
		Update: acpsdk.SessionUpdate{
			ToolCall: &acpsdk.SessionUpdateToolCall{
				SessionUpdate: "tool_call",
				ToolCallId:    acpsdk.ToolCallId(toolCallID),
				Title:         title,
				Status:        acpsdk.ToolCallStatus(status),
			},
		},
	})
}

func (s *bridgeSession) emitToolCallUpdate(toolCallID, status, output string) error {
	if s.conn == nil || toolCallID == "" {
		return nil
	}
	st := acpsdk.ToolCallStatus(status)
	update := &acpsdk.SessionToolCallUpdate{
		SessionUpdate: "tool_call_update",
		ToolCallId:    acpsdk.ToolCallId(toolCallID),
		Status:        &st,
	}
	if output != "" {
		update.Content = []acpsdk.ToolCallContent{
			{
				Content: &acpsdk.ToolCallContentContent{
					Type:    "content",
					Content: acpsdk.ContentBlock{Text: &acpsdk.ContentBlockText{Text: output}},
				},
			},
		}
	}
	return s.conn.SessionUpdate(s.ctx, acpsdk.SessionNotification{
		SessionId: acpsdk.SessionId(s.mapping.ACPSessionID),
		Update:    acpsdk.SessionUpdate{ToolCallUpdate: update},
	})
}

func (s *bridgeSession) handlePermissionRequest(runID string, frame EventFrame) error {
	if s.conn == nil {
		return nil
	}
	options := make([]acpsdk.PermissionOption, 0, len(frame.ACPPermissionOptions))
	for _, opt := range frame.ACPPermissionOptions {
		options = append(options, acpsdk.PermissionOption{
			OptionId: acpsdk.PermissionOptionId(opt.OptionID),
			Name:     opt.Name,
			Kind:     acpsdk.PermissionOptionKind(opt.Kind),
		})
	}
	if len(options) == 0 {
		return nil
	}
	interactionID := frame.InteractionID
	toolCallID := frame.ToolCallID
	s.mu.Lock()
	s.pendingPermissions[interactionID] = frame.ACPPermissionOptions
	s.mu.Unlock()

	// Resolve permission asynchronously so the event stream keeps flowing.
	go func(requestID, tcID string, opts []acpsdk.PermissionOption) {
		resp, err := s.conn.RequestPermission(s.ctx, acpsdk.RequestPermissionRequest{
			SessionId: acpsdk.SessionId(s.mapping.ACPSessionID),
			ToolCall: acpsdk.ToolCallUpdate{
				ToolCallId: acpsdk.ToolCallId(tcID),
			},
			Options: opts,
		})
		if err != nil {
			_ = s.resolvePermission(runID, requestID, "")
			return
		}
		optionID := ""
		if resp.Outcome.Selected != nil {
			optionID = string(resp.Outcome.Selected.OptionId)
		}
		_ = s.resolvePermission(runID, requestID, optionID)
	}(interactionID, toolCallID, options)
	return nil
}

// resolvePermission forwards the selected option to Core via the control route.
// An empty optionID means cancelled.
func (s *bridgeSession) resolvePermission(runID, interactionID, optionID string) error {
	cancelled := optionID == ""
	if !cancelled {
		kind := s.permissionOptionKind(interactionID, optionID)
		switch kind {
		case string(acpsdk.PermissionOptionKindAllowOnce),
			string(acpsdk.PermissionOptionKindAllowAlways),
			"allow":
			// allow -> completed
		case string(acpsdk.PermissionOptionKindRejectOnce),
			string(acpsdk.PermissionOptionKindRejectAlways),
			"deny":
			// deny -> failed; still forwarded as a selected option
		default:
			// Unknown option kind: treat as cancellation for safety.
			cancelled = true
			optionID = ""
		}
	}
	s.mu.Lock()
	delete(s.pendingPermissions, interactionID)
	s.mu.Unlock()
	if err := s.coreClient.ResolveACPPermission(context.Background(), s.mapping.ACPSessionID, runID, interactionID, optionID, cancelled); err != nil {
		log.Printf("agentbridge: resolve permission %s error: %v", interactionID, err)
		return err
	}
	return nil
}

func (s *bridgeSession) permissionOptionKind(interactionID, optionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, opt := range s.pendingPermissions[interactionID] {
		if opt.OptionID == optionID {
			return opt.Kind
		}
	}
	return ""
}

func stringValue(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func newACPSessionID() string {
	return fmt.Sprintf("acp_%d", time.Now().UnixNano())
}

// setConnection wires the agent-side connection for outbound session updates.
func (s *bridgeSession) setConnection(conn *acpsdk.AgentSideConnection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conn = conn
}
