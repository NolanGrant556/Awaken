package agentcore

import (
	"awaken/src/context"
	stdcontext "context"
	"strings"
)

type Agent struct {
	config                 AgentConfig
	conversation           *context.Conversation
	reqBuilder             *RequestBuilder
	threadID               string
	runID                  string
	sessionID              string
	recorder               SemanticRecorder
	pendingUserAttachments []context.MessageAttachment
	pendingComposerTokens  []SemanticComposerToken
}

func NewAgent(config AgentConfig) *Agent {
	conv := context.NewConversation(nil)
	return NewAgentWithConversationAndThreadID(config, conv, newAwakenID("thread"))
}

func NewAgentWithThreadID(config AgentConfig, threadID string) *Agent {
	conv := context.NewConversation(nil)
	return NewAgentWithConversationAndThreadID(config, conv, threadID)
}

func NewAgentWithConversation(config AgentConfig, conv *context.Conversation) *Agent {
	return NewAgentWithConversationAndThreadID(config, conv, newAwakenID("thread"))
}

func NewAgentWithConversationAndThreadID(config AgentConfig, conv *context.Conversation, threadID string) *Agent {
	reqBuilder := NewRequestBuilder(config)

	return &Agent{
		config:       config,
		conversation: conv,
		reqBuilder:   reqBuilder,
		threadID:     threadID,
	}
}

func (a *Agent) Reset() {
	a.conversation.Reset()
}

func (a *Agent) GetConversation() *context.Conversation {
	return a.conversation
}

func (a *Agent) GetConfig() AgentConfig {
	return a.config
}

// ModelVisibleToolPool returns the exact tool names exposed by the current
// RequestBuilder after exposure policy and any resolved Skill gate. Skill
// activation must inherit this boundary rather than the wider process-global
// registry.
func (a *Agent) ModelVisibleToolPool() []string {
	if a == nil || a.reqBuilder == nil {
		return nil
	}
	schemas := a.reqBuilder.getTools()
	names := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		name := strings.TrimSpace(schema.Function.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func (a *Agent) Reconfigure(config AgentConfig) {
	a.config = config
	a.reqBuilder = NewRequestBuilder(config)
}

// ApplySkillActivationPlan narrows the dual-gate tool pool and appends the
// skill's typed context block. Used after explicit or implicit activation.
func (a *Agent) ApplySkillActivationPlan(skillID string, effectiveTools []string, mainBody string) {
	if a == nil {
		return
	}
	cfg := a.config
	if effectiveTools != nil {
		if cfg.ResolvedToolPool == nil {
			cfg.ResolvedToolPool = append([]string(nil), effectiveTools...)
		} else {
			set := make(map[string]bool, len(effectiveTools))
			for _, name := range effectiveTools {
				set[name] = true
			}
			next := make([]string, 0, len(cfg.ResolvedToolPool))
			for _, name := range cfg.ResolvedToolPool {
				if set[name] {
					next = append(next, name)
				}
			}
			cfg.ResolvedToolPool = next
		}
	}
	if strings.TrimSpace(mainBody) != "" {
		cfg.SkillContextBlocks = append(cfg.SkillContextBlocks, ContextBlock{
			ID:      "skill_activation_" + skillID,
			Content: mainBody,
		})
	}
	a.Reconfigure(cfg)
}

// SetSemanticRecorder injects the per-thread durable journal writer.
func (a *Agent) SetSemanticRecorder(recorder SemanticRecorder) {
	a.recorder = recorder
}

// SetRunIdentity binds the current run/session identifiers for journal records.
func (a *Agent) SetRunIdentity(runID, sessionID string) {
	a.runID = runID
	a.sessionID = sessionID
}

// ExecutionIdentity returns the IDs bound by RunService for the current run.
// Custom Executor implementations that emit SemanticRecord values must use
// these IDs so restart replay can associate records with the correct run.
func (a *Agent) ExecutionIdentity() (threadID, runID, sessionID string) {
	if a == nil {
		return "", "", ""
	}
	return a.threadID, a.runID, a.sessionID
}

func (a *Agent) SemanticRecorder() SemanticRecorder {
	return a.recorder
}

func (a *Agent) RunStreamWithEvents(ctx stdcontext.Context, userInput string, sink EventSink) (string, error) {
	attachments := a.consumePendingUserAttachments()
	composerTokens := a.consumePendingComposerTokens()
	if a.recorder != nil {
		payload := map[string]any{
			"content": userInput,
			"role":    "user",
		}
		if len(attachments) > 0 {
			payload["attachment_count"] = len(attachments)
			payload["attachments"] = semanticUserMessageAttachments(attachments)
		}
		if len(composerTokens) > 0 {
			payload["composer_tokens"] = composerTokens
		}
		record := NewSemanticRecord(
			SemanticKindUserMessage,
			a.threadID,
			a.runID,
			"",
			"",
			a.sessionID,
			payload,
		)
		if err := a.recorder.Append(ctx, record); err != nil {
			events := newRunEventEmitter(a.threadID, sink)
			events.runID = a.runID
			events.sessionID = a.sessionID
			events.recorder = a.recorder
			return "", events.notePersistenceFailure("user_message", err)
		}
	}

	// Conversation state may only advance after the user message is durable;
	// otherwise a failed journal write leaks an unpersisted prompt into the next
	// run on the same in-memory Agent.
	a.conversation.UserInputWithAttachments(userInput, attachments)

	runner := NewRunner(a.config, a.conversation, a.reqBuilder)
	runner.threadID = a.threadID
	runner.runID = a.runID
	runner.sessionID = a.sessionID
	runner.recorder = a.recorder

	return runner.RunStreamWithEvents(ctx, sink)
}

func (a *Agent) SetPendingUserAttachments(attachments []context.MessageAttachment) {
	a.pendingUserAttachments = append([]context.MessageAttachment(nil), attachments...)
}

// SetPendingComposerTokens binds the structured composer identity that must be
// written with the next durable user_message. Tokens affect runtime activation
// before Agent execution, but their display identity still belongs to the
// accepted user input and must survive Desktop restart.
func (a *Agent) SetPendingComposerTokens(tokens []SemanticComposerToken) {
	a.pendingComposerTokens = cloneSemanticComposerTokens(tokens)
}

func (a *Agent) consumePendingUserAttachments() []context.MessageAttachment {
	if len(a.pendingUserAttachments) == 0 {
		return nil
	}
	attachments := append([]context.MessageAttachment(nil), a.pendingUserAttachments...)
	a.pendingUserAttachments = nil
	return attachments
}

func (a *Agent) consumePendingComposerTokens() []SemanticComposerToken {
	if len(a.pendingComposerTokens) == 0 {
		return nil
	}
	tokens := cloneSemanticComposerTokens(a.pendingComposerTokens)
	a.pendingComposerTokens = nil
	return tokens
}

func semanticUserMessageAttachments(attachments []context.MessageAttachment) []SemanticUserMessageAttachment {
	if len(attachments) == 0 {
		return nil
	}
	result := make([]SemanticUserMessageAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		result = append(result, SemanticUserMessageAttachment{
			ID:          attachment.ID,
			Kind:        attachment.Kind,
			MimeType:    attachment.MimeType,
			Path:        attachment.Path,
			DisplayName: attachment.DisplayName,
			Width:       attachment.Width,
			Height:      attachment.Height,
			ByteSize:    attachment.ByteSize,
			StorageKey:  attachment.StorageKey,
		})
	}
	return result
}

func cloneSemanticComposerTokens(tokens []SemanticComposerToken) []SemanticComposerToken {
	if len(tokens) == 0 {
		return nil
	}
	cloned := make([]SemanticComposerToken, len(tokens))
	for i, token := range tokens {
		cloned[i] = token
		if token.Payload == nil {
			continue
		}
		cloned[i].Payload = make(map[string]any, len(token.Payload))
		for key, value := range token.Payload {
			cloned[i].Payload[key] = value
		}
	}
	return cloned
}
