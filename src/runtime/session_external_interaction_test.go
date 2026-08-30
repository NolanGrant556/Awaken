package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"awaken/src/agentcore"
)

func TestSessionAwaitAndResolveExternalInteraction(t *testing.T) {
	session := NewSession("run-1", "thread-1", "input", nil, nil, func() {})
	request := agentcore.AwakenEvent{
		Type:                       agentcore.ExternalInteractionRequestedType,
		InteractionID:              "ix-1",
		InteractionTitle:           "钉钉授权",
		InteractionVerificationURL: "https://login.dingtalk.com/oauth2/auth",
	}

	done := make(chan struct{})
	var resolution agentcore.ExternalInteractionResolution
	var awaitErr error
	go func() {
		resolution, awaitErr = session.AwaitExternalInteraction(context.Background(), request)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	if err := session.ResolveExternalInteraction("ix-1", agentcore.ExternalInteractionCompleted); err != nil {
		t.Fatalf("ResolveExternalInteraction failed: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("AwaitExternalInteraction did not resolve")
	}
	if awaitErr != nil {
		t.Fatalf("await error: %v", awaitErr)
	}
	if resolution != agentcore.ExternalInteractionCompleted {
		t.Fatalf("resolution=%q", resolution)
	}

	events, _ := session.StreamSnapshot()
	var requested, resolved bool
	for _, event := range events {
		if event.AgentEvent.Type == agentcore.ExternalInteractionRequestedType {
			requested = true
		}
		if event.AgentEvent.Type == agentcore.ExternalInteractionResolvedType {
			resolved = true
			if event.AgentEvent.InteractionResolution != string(agentcore.ExternalInteractionCompleted) {
				t.Fatalf("resolved payload=%q", event.AgentEvent.InteractionResolution)
			}
		}
	}
	if !requested || !resolved {
		t.Fatalf("requested=%v resolved=%v", requested, resolved)
	}
}

func TestSessionResolveExternalInteractionMissing(t *testing.T) {
	session := NewSession("run-1", "thread-1", "input", nil, nil, func() {})
	if err := session.ResolveExternalInteraction("missing", agentcore.ExternalInteractionCompleted); err != ErrExternalInteractionNotFound {
		t.Fatalf("expected ErrExternalInteractionNotFound, got %v", err)
	}
}

func TestSessionMCPFormAnswerIsReturnedButNotJournaled(t *testing.T) {
	session := NewSession("run-1", "thread-1", "input", nil, nil, func() {})
	request := agentcore.AwakenEvent{
		Type: agentcore.ExternalInteractionRequestedType, InteractionID: "mcp-elicit-1",
		InteractionKind: "mcp-elicitation", InteractionSafeMessage: "Choose a destination",
		InteractionFormSchema: map[string]any{"type": "object", "properties": map[string]any{"targetPath": map[string]any{"type": "string"}}},
	}
	done := make(chan agentcore.ExternalInteractionAnswer, 1)
	go func() {
		answer, _ := session.AwaitExternalInteractionForm(context.Background(), request)
		done <- answer
	}()
	time.Sleep(20 * time.Millisecond)
	if err := session.ResolveExternalInteractionWithContent("mcp-elicit-1", agentcore.ExternalInteractionCompleted, map[string]any{"targetPath": ".env.local"}); err != nil {
		t.Fatal(err)
	}
	answer := <-done
	if answer.Resolution != agentcore.ExternalInteractionCompleted || answer.Content["targetPath"] != ".env.local" {
		t.Fatalf("answer=%#v", answer)
	}
	events, _ := session.StreamSnapshot()
	for _, event := range events {
		if event.AgentEvent.InteractionResolution == string(agentcore.ExternalInteractionCompleted) && strings.Contains(event.AgentEvent.Content, ".env.local") {
			t.Fatalf("form content leaked into event: %#v", event.AgentEvent)
		}
	}
}
