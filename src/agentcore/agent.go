package agentcore

import (
	"awaken/src/context"
	stdcontext "context"
)

type Agent struct {
	config                 AgentConfig
	conversation           *context.Conversation
	reqBuilder             *RequestBuilder
	threadID               string
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

func (a *Agent) RunStreamWithEvents(ctx stdcontext.Context, userInput string, sink EventSink) (string, error) {
	a.conversation.UserInputWithAttachments(userInput, a.consumePendingUserAttachments())

	runner := NewRunner(a.config, a.conversation, a.reqBuilder)
	runner.threadID = a.threadID
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
