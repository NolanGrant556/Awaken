package runtime

import (
	"context"
	"testing"
	"time"

	"awaken/src/agentcore"
)

func TestSessionAwaitAndResolveUserQuestion(t *testing.T) {
	session := NewSession("run-1", "thread-1", "hello", nil, func() {})

	request := agentcore.AwakenEvent{
		Type:       agentcore.UserQuestionRequestedType,
		QuestionID: "q-1",
		Content:    `{"questions":[{"question":"Choose a theme","options":[{"label":"Dark"},{"label":"Light"}]}]}`,
	}

	answerCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		answer, err := session.AwaitUserQuestion(context.Background(), request)
		if err != nil {
			errCh <- err
			return
		}
		answerCh <- answer
	}()

	// Give AwaitUserQuestion time to register the pending question.
	time.Sleep(50 * time.Millisecond)

	if err := session.ResolveUserQuestion("q-1", "Dark"); err != nil {
		t.Fatalf("ResolveUserQuestion failed: %v", err)
	}

	select {
	case answer := <-answerCh:
		if answer != "Dark" {
			t.Fatalf("expected answer Dark, got %q", answer)
		}
	case err := <-errCh:
		t.Fatalf("AwaitUserQuestion returned error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatalf("AwaitUserQuestion did not resolve")
	}

	// Resolved event should be in history.
	history := session.History(0)
	var foundResolved bool
	var foundRequested bool
	for _, event := range history {
		if event.AgentEvent.Type == agentcore.UserQuestionRequestedType {
			foundRequested = true
		}
		if event.AgentEvent.Type == agentcore.UserQuestionResolvedType {
			foundResolved = true
			if event.AgentEvent.QuestionAnswer != "Dark" {
				t.Fatalf("resolved event answer mismatch: %q", event.AgentEvent.QuestionAnswer)
			}
		}
	}
	if !foundRequested {
		t.Fatalf("requested event not found in history")
	}
	if !foundResolved {
		t.Fatalf("resolved event not found in history")
	}
}

func TestSessionResolveUserQuestionMissing(t *testing.T) {
	session := NewSession("run-1", "thread-1", "hello", nil, func() {})
	if err := session.ResolveUserQuestion("missing", "answer"); err != ErrUserQuestionNotFound {
		t.Fatalf("expected ErrUserQuestionNotFound, got %v", err)
	}
}

func TestSessionResolveUserQuestionEmptyAnswer(t *testing.T) {
	session := NewSession("run-1", "thread-1", "hello", nil, func() {})
	if err := session.ResolveUserQuestion("q-1", "   "); err != ErrUserQuestionNotFound {
		t.Fatalf("expected ErrUserQuestionNotFound for empty answer, got %v", err)
	}
}
