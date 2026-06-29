package agentcore

import (
	"awaken/src/context"
	stdcontext "context"
)

type userContentPartsContextKey struct{}

func ContextWithUserContentParts(ctx stdcontext.Context, parts []context.ContentPart) stdcontext.Context {
	if ctx == nil {
		ctx = stdcontext.Background()
	}
	if len(parts) == 0 {
		return ctx
	}
	cloned := append([]context.ContentPart(nil), parts...)
	return stdcontext.WithValue(ctx, userContentPartsContextKey{}, cloned)
}

func userContentPartsFromContext(ctx stdcontext.Context) []context.ContentPart {
	if ctx == nil {
		return nil
	}
	parts, ok := ctx.Value(userContentPartsContextKey{}).([]context.ContentPart)
	if !ok || len(parts) == 0 {
		return nil
	}
	return append([]context.ContentPart(nil), parts...)
}

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

func (a *Agent) SetConfig(config AgentConfig) {
	a.config = config
	a.reqBuilder = NewRequestBuilder(config)
}

func (a *Agent) WithConfig(config AgentConfig) *Agent {
	if a == nil {
		return nil
	}
	return &Agent{
		config:       config,
		conversation: a.conversation,
		reqBuilder:   NewRequestBuilder(config),
		threadID:     a.threadID,
	}
}

func (a *Agent) WithRunProfile(profile *RunProfile) (*Agent, error) {
	if a == nil || profile == nil {
		return a, nil
	}
	config, err := a.config.WithRunProfile(profile)
	if err != nil {
		return nil, err
	}
	return a.WithConfig(config), nil
}

func (a *Agent) RunStreamWithEvents(ctx stdcontext.Context, userInput string, sink EventSink) (string, error) {
	if parts := userContentPartsFromContext(ctx); len(parts) > 0 {
		a.conversation.Context.AddUserMessageParts(userInput, parts)
	} else {
		a.conversation.UserInput(userInput)
	}

	runner := NewRunner(a.config, a.conversation, a.reqBuilder)
	runner.threadID = a.threadID
	return runner.RunStreamWithEvents(ctx, sink)
}
