package agentbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
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
	promptActive       bool
	promptCancel       context.CancelFunc
	modelProfiles      []ACPModelProfileSummary
	selectedProfileID  string
	textByMessageID    map[string]string
	pendingPermissions map[string][]agui.ACPPermissionOption
	terminalErr        error
	stopReason         acpsdk.StopReason
	terminalSeen       bool
}

// Prompt starts a run for this session and streams ACP updates until terminal.
func (s *bridgeSession) Prompt(ctx context.Context, text string) (acpsdk.StopReason, error) {
	s.mu.Lock()
	if s.promptActive {
		s.mu.Unlock()
		return "", errors.New("session already has an active prompt")
	}
	s.promptActive = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.promptActive = false
		s.mu.Unlock()
	}()

	runCtx, runCancel := context.WithCancel(ctx)
	stopSessionCancel := context.AfterFunc(s.ctx, runCancel)
	defer stopSessionCancel()
	if s.ctx.Err() != nil {
		runCancel()
	}
	defer runCancel()
	s.mu.Lock()
	s.promptCancel = runCancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.promptCancel = nil
		s.mu.Unlock()
	}()

	s.mu.Lock()
	selectedProfileID := s.selectedProfileID
	s.mu.Unlock()
	if runCtx.Err() != nil {
		return acpsdk.StopReasonCancelled, nil
	}
	run, err := s.coreClient.CreateRunForSession(runCtx, s.mapping.ACPSessionID, text, selectedProfileID)
	if err != nil {
		if runCtx.Err() != nil {
			if cleanupErr := s.cancelCreatingRun(); cleanupErr != nil {
				return "", fmt.Errorf("confirm cancelled run creation: %w", cleanupErr)
			}
			return acpsdk.StopReasonCancelled, nil
		}
		return "", fmt.Errorf("create run: %w", err)
	}
	s.mu.Lock()
	s.currentRunID = run.RunID
	s.pendingPermissions = make(map[string][]agui.ACPPermissionOption)
	s.textByMessageID = make(map[string]string)
	s.terminalErr = nil
	s.stopReason = ""
	s.terminalSeen = false
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.currentRunID = ""
		s.pendingPermissions = nil
		s.textByMessageID = nil
		s.mu.Unlock()
		_ = s.coreClient.ClearCurrentRun(context.Background(), s.mapping.ACPSessionID, run.RunID)
	}()

	var lastCursor uint64
	var streamErr error
	for attempt := 0; attempt < 4; attempt++ {
		frames, errs := s.coreClient.StreamRunEventsControl(runCtx, s.mapping.ACPSessionID, run.RunID, lastCursor)
		streamErr = nil
		for frames != nil || errs != nil {
			select {
			case <-ctx.Done():
				runCancel()
				s.cancelAndWait(run.RunID)
				return acpsdk.StopReasonCancelled, nil
			case <-s.ctx.Done():
				runCancel()
				s.cancelAndWait(run.RunID)
				return acpsdk.StopReasonCancelled, nil
			case <-runCtx.Done():
				s.cancelAndWait(run.RunID)
				return acpsdk.StopReasonCancelled, nil
			case err, ok := <-errs:
				if !ok {
					errs = nil
					continue
				}
				if err != nil {
					streamErr = err
				}
			case frame, ok := <-frames:
				if !ok {
					frames = nil
					continue
				}
				cursor := frame.StreamCursor
				if cursor == 0 {
					cursor = frame.Seq
				}
				if cursor != 0 && cursor <= lastCursor {
					continue
				}
				if err := s.handleFrame(runCtx, run.RunID, frame); err != nil {
					runCancel()
					s.cancelAndWait(run.RunID)
					return "", err
				}
				if cursor > lastCursor {
					lastCursor = cursor
				}
				if s.hasTerminal() {
					return s.terminalResult()
				}
			}
		}
		if attempt < 3 {
			timer := time.NewTimer(time.Duration(attempt+1) * 100 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				runCancel()
				s.cancelAndWait(run.RunID)
				return acpsdk.StopReasonCancelled, nil
			case <-s.ctx.Done():
				timer.Stop()
				runCancel()
				s.cancelAndWait(run.RunID)
				return acpsdk.StopReasonCancelled, nil
			case <-runCtx.Done():
				timer.Stop()
				s.cancelAndWait(run.RunID)
				return acpsdk.StopReasonCancelled, nil
			case <-timer.C:
			}
		}
	}
	if streamErr != nil {
		return "", fmt.Errorf("event stream ended before a terminal event: %w", streamErr)
	}
	return "", errors.New("event stream ended before a terminal event")
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

func (s *bridgeSession) hasTerminal() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.terminalSeen
}

// Keep prompt ownership until Core releases its creation reservation. If the
// response was lost after creation, recover the run ID before cancelling it.
func (s *bridgeSession) cancelCreatingRun() error {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		mapping, err := s.coreClient.GetACPSession(ctx, s.mapping.ACPSessionID)
		if err != nil {
			return err
		}
		if mapping == nil || (!mapping.RunStarting && mapping.CurrentRunID == "") {
			return nil
		}
		if mapping.CurrentRunID != "" {
			if err := s.cancelAndWaitContext(ctx, mapping.CurrentRunID); err != nil {
				latest, readErr := s.coreClient.GetACPSession(ctx, s.mapping.ACPSessionID)
				if readErr == nil && (latest == nil || (!latest.RunStarting && latest.CurrentRunID == "")) {
					return nil
				}
				return err
			}
			// Core also cleans up cancelled create requests. A lost successful
			// response leaves cleanup to this bridge after draining the run.
			if err := s.coreClient.ClearCurrentRun(ctx, s.mapping.ACPSessionID, mapping.CurrentRunID); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *bridgeSession) cancelAndWait(runID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.cancelAndWaitContext(ctx, runID)
}

func (s *bridgeSession) cancelAndWaitContext(ctx context.Context, runID string) error {
	if err := s.coreClient.CancelRunControl(ctx, s.mapping.ACPSessionID, runID); err != nil {
		return err
	}
	frames, errs := s.coreClient.StreamRunEventsControl(ctx, s.mapping.ACPSessionID, runID, 0)
	for frame := range frames {
		if frame.Type == agui.EventRunFinished || frame.Type == agui.EventRunError {
			return nil
		}
	}
	if err := <-errs; err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return errors.New("cancelled run stream ended before a terminal event")
}

func (s *bridgeSession) handleFrame(runCtx context.Context, runID string, frame EventFrame) error {
	switch frame.Type {
	case agui.EventRunStarted:
		return nil
	case agui.EventRunFinished:
		s.mu.Lock()
		s.stopReason = acpsdk.StopReasonEndTurn
		s.terminalErr = nil
		s.terminalSeen = true
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
		s.terminalSeen = true
		s.mu.Unlock()
		return nil
	case agui.EventTextMessageContent:
		text, err := s.dedupeTextDelta(frame)
		if err != nil {
			return err
		}
		return s.emitTextDelta(text)
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
		return s.handlePermissionRequest(runCtx, runID, frame)
	default:
		// Drop unknown frames silently.
		return nil
	}
}

func (s *bridgeSession) emitTextDelta(text string) error {
	if s.conn == nil || text == "" {
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

// dedupeTextDelta distinguishes transport deltas from durable full-message
// projections. Equal text can be a legitimate repeated token, not a replay.
func (s *bridgeSession) dedupeTextDelta(frame EventFrame) (string, error) {
	messageID, delta := frame.MessageID, frame.Delta
	if messageID == "" || delta == "" {
		return delta, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.textByMessageID == nil {
		s.textByMessageID = make(map[string]string)
	}
	previous := s.textByMessageID[messageID]
	if frame.RecordID != "" || frame.SemanticSeq != 0 {
		if !strings.HasPrefix(delta, previous) {
			return "", errors.New("ACP durable text disagrees with streamed message")
		}
		s.textByMessageID[messageID] = delta
		return strings.TrimPrefix(delta, previous), nil
	}
	s.textByMessageID[messageID] = previous + delta
	return delta, nil
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

func (s *bridgeSession) handlePermissionRequest(runCtx context.Context, runID string, frame EventFrame) error {
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
		resp, err := s.conn.RequestPermission(runCtx, acpsdk.RequestPermissionRequest{
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
	if err := s.coreClient.ResolveACPPermission(context.Background(), s.mapping.ACPSessionID, runID, interactionID, optionID, cancelled); err != nil {
		log.Printf("agentbridge: resolve permission %s error: %v", interactionID, err)
		return err
	}
	s.mu.Lock()
	delete(s.pendingPermissions, interactionID)
	s.mu.Unlock()
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

func (s *bridgeSession) modelConfigOptions() []acpsdk.SessionConfigOption {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.modelConfigOptionsLocked()
}

func (s *bridgeSession) modelConfigOptionsLocked() []acpsdk.SessionConfigOption {
	options := make(acpsdk.SessionConfigSelectOptionsUngrouped, 0, len(s.modelProfiles))
	for _, profile := range s.modelProfiles {
		label := profile.DisplayName
		if label == "" {
			label = profile.ModelID
		} else if profile.ModelID != "" && label != profile.ModelID {
			label += " (" + profile.ModelID + ")"
		}
		description := profile.ProviderID
		options = append(options, acpsdk.SessionConfigSelectOption{
			Name:        label,
			Value:       acpsdk.SessionConfigValueId(profile.ID),
			Description: &description,
		})
	}
	category := acpsdk.SessionConfigOptionCategoryModel
	description := "Select an enabled Awaken model profile. Credentials remain inside Awaken."
	return []acpsdk.SessionConfigOption{{
		Select: &acpsdk.SessionConfigOptionSelect{
			Type:         "select",
			Id:           acpsdk.SessionConfigId("model"),
			Name:         "Model",
			Description:  &description,
			Category:     &category,
			CurrentValue: acpsdk.SessionConfigValueId(s.selectedProfileID),
			Options: acpsdk.SessionConfigSelectOptions{
				Ungrouped: &options,
			},
		},
	}}
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
