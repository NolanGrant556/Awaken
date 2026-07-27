package agentcore

import (
	"path/filepath"
	"testing"

	convcontext "awaken/src/context"
)

func TestLocalSkillRootObservationsIncludeEveryProjectAwakenFolder(t *testing.T) {
	primary := filepath.Join(t.TempDir(), "primary")
	secondary := filepath.Join(t.TempDir(), "secondary")
	observations := localSkillRootObservations(WorkspaceConfig{
		Root:          primary,
		ReadableRoots: []string{primary, secondary},
	})

	want := map[string]bool{
		filepath.Join(primary, ".awaken", "skills"):   false,
		filepath.Join(secondary, ".awaken", "skills"): false,
	}
	for _, observation := range observations {
		if observation.Source == "project" {
			if _, ok := want[observation.Path]; ok {
				want[observation.Path] = true
			}
		}
	}
	for path, seen := range want {
		if !seen {
			t.Fatalf("project Skill root %q missing from observations: %+v", path, observations)
		}
	}
}

func TestLocalSkillRootObservationsExcludeManagedSessionWorkspace(t *testing.T) {
	managed := filepath.Join(t.TempDir(), "sessions", "thread_123")
	observations := localSkillRootObservations(WorkspaceConfig{
		Root:             managed,
		ReadableRoots:    []string{managed},
		SessionWorkspace: true,
	})
	for _, observation := range observations {
		if observation.Source == "project" {
			t.Fatalf("managed session workspace leaked as a project Skill root: %+v", observations)
		}
	}
}

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
