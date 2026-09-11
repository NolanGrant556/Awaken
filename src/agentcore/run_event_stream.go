package agentcore

import (
	"awaken/src/context"
	"awaken/src/llm"
	stdcontext "context"
	"strings"
)

func (e *runEventEmitter) providerOpaqueState(ctx stdcontext.Context, state *context.ProviderOpaqueMessageState) error {
	if e == nil || e.recorder == nil || state == nil || strings.TrimSpace(state.Content) == "" {
		return nil
	}
	persistCtx, cancel := durablePersistenceContext(ctx)
	defer cancel()
	record := e.withAgentIdentity(NewSemanticRecord(
		SemanticKindProviderOpaqueState, e.threadID, e.runID, e.turnID(), "", e.sessionID,
		map[string]any{"message_id": e.ensureAssistantMessageID(), "state": state},
	))
	if err := e.recorder.Append(persistCtx, record); err != nil {
		return e.notePersistenceFailure("provider_opaque_state_append", err)
	}
	if err := e.recorder.Flush(persistCtx); err != nil {
		return e.notePersistenceFailure("provider_opaque_state_flush", err)
	}
	return nil
}

func (e *runEventEmitter) onLLMDelta(content string, isThinking bool) {
	if content == "" {
		return
	}
	if isThinking {
		e.startReasoningIfNeeded()
		e.reasoningText.WriteString(content)
		e.emit(AwakenEvent{
			Type:      ReasoningDeltaEventType,
			MessageID: e.reasoningMessageID,
			Delta:     content,
		})
		return
	}
	e.startTextIfNeeded()
	e.assistantText.WriteString(content)
	e.emit(AwakenEvent{
		Type:      TextDeltaEventType,
		MessageID: e.textMessageID,
		Role:      "assistant",
		Delta:     content,
	})
}

func (e *runEventEmitter) onLLMStreamEvent(event llm.StreamEvent) {
	if e == nil {
		return
	}
	if event.Type == llm.StreamEventToolCallStart {
		e.toolStreamStarted = true
	}
}

func (e *runEventEmitter) emitStandaloneText(content string) {
	if content == "" {
		return
	}
	e.startTextIfNeeded()
	e.emit(AwakenEvent{
		Type:      TextDeltaEventType,
		MessageID: e.textMessageID,
		Role:      "assistant",
		Delta:     content,
	})
	e.finishTextIfOpen()
}

func (e *runEventEmitter) startTextIfNeeded() {
	if e.textOpen {
		return
	}
	e.textMessageID = e.ensureAssistantMessageID()
	e.textStartedThisTurn = true
	e.textOpen = true
	e.emit(AwakenEvent{
		Type:      TextMessageStartedEventType,
		MessageID: e.textMessageID,
		Role:      "assistant",
	})
}

func (e *runEventEmitter) finishTextIfOpen() {
	if !e.textOpen {
		return
	}
	finished := AwakenEvent{
		Type:      TextMessageFinishedEventType,
		MessageID: e.textMessageID,
	}
	e.fillIDs(&finished)
	content := e.assistantText.String()
	if e.recorder != nil && strings.TrimSpace(content) != "" {
		if err := e.persistDurable(stdcontext.Background(), "assistant_message", NewSemanticRecord(
			SemanticKindAssistantMessage,
			e.threadID,
			e.runID,
			e.turnID(),
			"",
			e.sessionID,
			map[string]any{
				"content":    content,
				"role":       "assistant",
				"message_id": e.textMessageID,
			},
		), finished); err != nil {
			e.textOpen = false
			e.textMessageID = ""
			e.assistantText.Reset()
			return
		}
	} else {
		e.sink.Emit(finished)
	}
	e.textOpen = false
	e.textMessageID = ""
	e.assistantText.Reset()
}

func (e *runEventEmitter) startReasoningIfNeeded() {
	if e.reasoningOpen {
		return
	}
	e.reasoningMessageID = newAwakenID("reasoning")
	e.reasoningOpen = true
	e.emit(AwakenEvent{
		Type:      ReasoningStartedEventType,
		MessageID: e.reasoningMessageID,
	})
}

func (e *runEventEmitter) finishReasoningIfOpen() {
	if !e.reasoningOpen {
		return
	}
	finished := AwakenEvent{
		Type:      ReasoningFinishedEventType,
		MessageID: e.reasoningMessageID,
	}
	e.fillIDs(&finished)
	content := e.reasoningText.String()
	if e.recorder != nil && strings.TrimSpace(content) != "" {
		if err := e.persistDurable(stdcontext.Background(), "reasoning_summary", NewSemanticRecord(
			SemanticKindReasoningSummary,
			e.threadID,
			e.runID,
			e.turnID(),
			"",
			e.sessionID,
			map[string]any{
				"content":    content,
				"role":       "reasoning",
				"message_id": e.reasoningMessageID,
			},
		), finished); err != nil {
			e.reasoningOpen = false
			e.reasoningMessageID = ""
			e.reasoningText.Reset()
			return
		}
	} else {
		e.sink.Emit(finished)
	}
	e.reasoningOpen = false
	e.reasoningMessageID = ""
	e.reasoningText.Reset()
}

func (e *runEventEmitter) ensureAssistantMessageID() string {
	if e.currentAssistantID == "" {
		e.currentAssistantID = newAwakenID("msg")
	}
	return e.currentAssistantID
}

func (e *runEventEmitter) resetTurn() {
	e.logicalModelCallID, e.transportAttempt = "", 0
	e.currentAssistantID = ""
	e.textMessageID = ""
	e.reasoningMessageID = ""
	e.textOpen = false
	e.reasoningOpen = false
	e.textStartedThisTurn = false
	e.toolStreamStarted = false
	e.preferNonStreaming = false
	e.reasoningText.Reset()
	e.assistantText.Reset()
	e.toolArgumentProgress.reset()
}

func (e *runEventEmitter) hasEphemeralStreamOutput() bool {
	if e == nil {
		return false
	}
	return e.textOpen || e.reasoningOpen || e.assistantText.Len() > 0 || e.reasoningText.Len() > 0 || e.toolStreamStarted
}

func (e *runEventEmitter) discardEphemeralStreamOutput() {
	if e == nil {
		return
	}
	e.currentAssistantID = ""
	e.textMessageID = ""
	e.reasoningMessageID = ""
	e.textOpen = false
	e.reasoningOpen = false
	e.textStartedThisTurn = false
	e.toolStreamStarted = false
	e.assistantText.Reset()
	e.reasoningText.Reset()
	e.toolArgumentProgress.reset()
}

func (e *runEventEmitter) providerReconnect(phase string, attempt, maxAttempts int, err error, reset bool) {
	if e == nil {
		return
	}
	e.emit(AwakenEvent{
		Type: ProviderReconnectEventType,
		ProviderReconnect: &ProviderReconnectStatus{
			Phase:           phase,
			Attempt:         attempt,
			MaxAttempts:     maxAttempts,
			Detail:          llm.SafeRetryDetail(err),
			ResetInProgress: reset,
		},
	})
}

func (e *runEventEmitter) providerReconnectRecovered() {
	if e == nil {
		return
	}
	e.emit(AwakenEvent{
		Type: ProviderReconnectEventType,
		ProviderReconnect: &ProviderReconnectStatus{
			Phase: ProviderReconnectRecovered,
		},
	})
}

func (e *runEventEmitter) textWasStartedThisTurn() bool {
	return e.textStartedThisTurn
}
