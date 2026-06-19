package agentcore

import (
	"testing"

	convcontext "awaken/src/context"
)

func TestNewAgentWithConversationAndThreadIDSetsThreadID(t *testing.T) {
	cfg := testAgentConfig("http://unused", 1)
	conv := convcontext.NewConversation(nil)
	wantThreadID := "thread-external-123"

	agent := NewAgentWithConversationAndThreadID(cfg, conv, wantThreadID)

	if agent.threadID != wantThreadID {
		t.Fatalf("expected threadID %q, got %q", wantThreadID, agent.threadID)
	}
}

func TestNewAgentWithConversationAndThreadIDReusesConversation(t *testing.T) {
	cfg := testAgentConfig("http://unused", 1)
	conv := convcontext.NewConversation(nil)
	conv.UserInput("seed message")

	agent := NewAgentWithConversationAndThreadID(cfg, conv, "thread-reuse")

	if agent.GetConversation() != conv {
		t.Fatalf("expected GetConversation to return the same Conversation pointer passed in")
	}

	messages := agent.GetConversation().Context.GetMessages()
	if len(messages) != 1 || messages[0].Role != convcontext.RoleUser || messages[0].Content != "seed message" {
		t.Fatalf("expected provided conversation to retain seeded message, got %+v", messages)
	}
}

func TestNewAgentGeneratesNewThreadID(t *testing.T) {
	cfg := testAgentConfig("http://unused", 1)

	agent := NewAgent(cfg)

	if agent.threadID == "" {
		t.Fatalf("expected NewAgent to generate a non-empty threadID")
	}
	if agent.threadID == "thread-external-123" {
		t.Fatalf("expected NewAgent to generate a fresh threadID, got the external one")
	}
}

func TestNewAgentWithConversationGeneratesNewThreadID(t *testing.T) {
	cfg := testAgentConfig("http://unused", 1)
	conv := convcontext.NewConversation(nil)

	agent := NewAgentWithConversation(cfg, conv)

	if agent.threadID == "" {
		t.Fatalf("expected NewAgentWithConversation to generate a non-empty threadID")
	}
	if agent.threadID == "thread-external-123" {
		t.Fatalf("expected NewAgentWithConversation to generate a fresh threadID, got the external one")
	}
	if agent.GetConversation() != conv {
		t.Fatalf("expected NewAgentWithConversation to reuse the provided Conversation pointer")
	}
}
