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
}

func NewAgent(config AgentConfig) *Agent {
	conv := context.NewConversation(nil)
	reqBuilder := NewRequestBuilder(config)

	return &Agent{
		config:       config,
		conversation: conv,
		reqBuilder:   reqBuilder,
	}
}

func NewAgentWithConversation(config AgentConfig, conv *context.Conversation) *Agent {
	reqBuilder := NewRequestBuilder(config)

	return &Agent{
		config:       config,
		conversation: conv,
		reqBuilder:   reqBuilder,
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
	return runner.Run(ctx)
}

func (a *Agent) RunStream(ctx stdcontext.Context, userInput string, callback StreamCallback) (string, error) {
	a.conversation.UserInput(userInput)

	runner := NewRunner(a.config, a.conversation, a.reqBuilder)
	return runner.RunStream(ctx, callback)
}
