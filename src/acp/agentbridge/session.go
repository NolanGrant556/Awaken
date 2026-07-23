package agentbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
)

// bridgeSession tracks one inbound ACP session and its current run.
type bridgeSession struct {
	mapping      *ACPSessionMapping
	conn         *acpsdk.AgentSideConnection
	ctx          context.Context
	cancel       context.CancelFunc
	coreClient   *CoreClient
	mu           sync.Mutex
	currentRunID string
}

// Prompt starts a run for this session and streams ACP updates until terminal.
func (s *bridgeSession) Prompt(ctx context.Context, text string) error {
	s.mu.Lock()
	if s.currentRunID != "" {
		s.mu.Unlock()
		return errors.New("session already has an active prompt")
	}
	s.mu.Unlock()

	runCtx, runCancel := context.WithCancel(s.ctx)
	defer runCancel()

	run, err := s.coreClient.CreateRun(runCtx, s.mapping.ThreadID, text, s.mapping.CanonicalCWD)
	if err != nil {
		return fmt.Errorf("create run: %w", err)
	}
	s.mu.Lock()
	s.currentRunID = run.RunID
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.currentRunID = ""
		s.mu.Unlock()
	}()

	frames, errs := s.coreClient.StreamRunEvents(runCtx, run.RunID, 0)
	for {
		select {
		case <-ctx.Done():
			_ = s.coreClient.CancelRun(context.Background(), run.RunID)
			return ctx.Err()
		case <-s.ctx.Done():
			return s.ctx.Err()
		case err := <-errs:
			if err != nil {
				return fmt.Errorf("event stream: %w", err)
			}
			return nil
		case frame, ok := <-frames:
			if !ok {
				return nil
			}
			if err := s.handleFrame(run.RunID, frame); err != nil {
				return err
			}
		}
	}
}

func (s *bridgeSession) handleFrame(runID string, frame EventFrame) error {
	switch frame.Type {
	case "RUN_STARTED", "RUN_FINISHED":
		return nil
	case "TEXT_DELTA":
		return s.emitTextDelta(stringValue(frame.Content))
	case "TOOL_CALL_STARTED":
		return s.emitToolCall(frame.ToolCallID, frame.ToolCallName, "running")
	case "TOOL_CALL_FINISHED":
		return s.emitToolCallUpdate(frame.ToolCallID, "completed", "")
	case "TOOL_RESULT":
		return s.emitToolCallUpdate(frame.ToolCallID, "completed", stringValue(frame.Content))
	case "AWAKEN_ACP_PERMISSION_REQUESTED":
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
	// Resolve permission asynchronously so the event stream keeps flowing.
	go func(requestID, tcID string) {
		resp, err := s.conn.RequestPermission(s.ctx, acpsdk.RequestPermissionRequest{
			SessionId: acpsdk.SessionId(s.mapping.ACPSessionID),
			ToolCall: acpsdk.ToolCallUpdate{
				ToolCallId: acpsdk.ToolCallId(tcID),
			},
			Options: options,
		})
		if err != nil {
			_ = s.coreClient.resolveACPPermission(context.Background(), runID, requestID, "")
			return
		}
		optionID := ""
		if resp.Outcome.Selected != nil {
			optionID = string(resp.Outcome.Selected.OptionId)
		}
		_ = s.coreClient.resolveACPPermission(context.Background(), runID, requestID, optionID)
	}(interactionID, toolCallID)
	return nil
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
