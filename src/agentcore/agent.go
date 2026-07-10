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

func (a *Agent) SemanticRecorder() SemanticRecorder {
	return a.recorder
}

func (a *Agent) RunStreamWithEvents(ctx stdcontext.Context, userInput string, sink EventSink) (string, error) {
	attachments := a.consumePendingUserAttachments()
	a.conversation.UserInputWithAttachments(userInput, attachments)

	runner := NewRunner(a.config, a.conversation, a.reqBuilder)
	runner.threadID = a.threadID
	runner.runID = a.runID
	runner.sessionID = a.sessionID
	runner.recorder = a.recorder

	if a.recorder != nil {
		payload := map[string]any{
			"content": userInput,
			"role":    "user",
		}
		if len(attachments) > 0 {
			payload["attachment_count"] = len(attachments)
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
			return "", err
		}
	}

	return runner.RunStreamWithEvents(ctx, sink)
}

func (a *Agent) SetPendingUserAttachments(attachments []context.MessageAttachment) {
	a.pendingUserAttachments = append([]context.MessageAttachment(nil), attachments...)
}

func (a *Agent) consumePendingUserAttachments() []context.MessageAttachment {
	if len(a.pendingUserAttachments) == 0 {
		return nil
	}
	attachments := append([]context.MessageAttachment(nil), a.pendingUserAttachments...)
	a.pendingUserAttachments = nil
	return attachments
}
