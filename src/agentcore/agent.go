package agentcore

import (
	"awaken/src/context"
	stdcontext "context"
)

type StreamCallback func(content string, isThinking bool)

type Agent struct {
	config       AgentConfig
	conversation *context.Conversation
	reqBuilder   *RequestBuilder
	threadID     string
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

func (a *Agent) Run(ctx stdcontext.Context, userInput string) (string, error) {
	a.conversation.UserInput(userInput)

	runner := NewRunner(a.config, a.conversation, a.reqBuilder)
	runner.threadID = a.threadID
	return runner.Run(ctx)
}

func (a *Agent) RunStream(ctx stdcontext.Context, userInput string, callback StreamCallback) (string, error) {
	a.conversation.UserInput(userInput)

	runner := NewRunner(a.config, a.conversation, a.reqBuilder)
	runner.threadID = a.threadID
	return runner.RunStream(ctx, callback)
}

func (a *Agent) RunStreamWithEvents(ctx stdcontext.Context, userInput string, callback StreamCallback, sink EventSink) (string, error) {
	a.conversation.UserInput(userInput)

	runner := NewRunner(a.config, a.conversation, a.reqBuilder)
	runner.threadID = a.threadID
	return runner.RunStreamWithEvents(ctx, callback, sink)
}
