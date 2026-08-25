package runtime

import (
	"context"
	"testing"
	"time"

	"awaken/src/agentcore"
)

func TestSessionAwaitAndResolveUserQuestion(t *testing.T) {
	session := NewSession("run-1", "thread-1", "hello", nil, nil, func() {})

	request := agentcore.AwakenEvent{
		Type:       agentcore.UserQuestionRequestedType,
		QuestionID: "q-1",
		Content:    `{"questions":[{"question":"Choose a theme","options":[{"label":"Dark"},{"label":"Light"}]}]}`,
	}

	resolutionCh := make(chan agentcore.UserQuestionResolution, 1)
	errCh := make(chan error, 1)
	go func() {
		resolution, err := session.AwaitUserQuestion(context.Background(), request)
		if err != nil {
			errCh <- err
			return
		}
		resolutionCh <- resolution
	}()

	// Give AwaitUserQuestion time to register the pending question.
	time.Sleep(50 * time.Millisecond)

	if err := session.ResolveUserQuestion("q-1", "Dark"); err != nil {
		t.Fatalf("ResolveUserQuestion failed: %v", err)
	}

	select {
	case resolution := <-resolutionCh:
		if resolution.Status != agentcore.UserQuestionAnswered || resolution.Answer != "Dark" {
			t.Fatalf("expected answered/Dark, got %+v", resolution)
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
			if event.AgentEvent.QuestionResolution != string(agentcore.UserQuestionAnswered) {
				t.Fatalf("resolved event status mismatch: %q", event.AgentEvent.QuestionResolution)
			}
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

func TestSessionCancelUserQuestionResolvesWaitWithoutCancellingRun(t *testing.T) {
	session := NewSession("run-1", "thread-1", "hello", nil, nil, func() {})
	request := agentcore.AwakenEvent{
		Type:       agentcore.UserQuestionRequestedType,
		QuestionID: "q-cancel",
		Content:    `{"questions":[{"question":"Choose a theme","options":[{"label":"Dark"},{"label":"Light"}]}]}`,
	}

	resolutionCh := make(chan agentcore.UserQuestionResolution, 1)
	errCh := make(chan error, 1)
	go func() {
		resolution, err := session.AwaitUserQuestion(context.Background(), request)
		if err != nil {
			errCh <- err
			return
		}
		resolutionCh <- resolution
	}()

	time.Sleep(50 * time.Millisecond)
	if err := session.CancelUserQuestion("q-cancel"); err != nil {
		t.Fatalf("CancelUserQuestion failed: %v", err)
	}

	select {
	case resolution := <-resolutionCh:
		if resolution.Status != agentcore.UserQuestionCancelled || resolution.Answer != "" {
			t.Fatalf("expected cancelled resolution without answer, got %+v", resolution)
		}
	case err := <-errCh:
		t.Fatalf("AwaitUserQuestion returned error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatalf("AwaitUserQuestion did not resolve after user cancellation")
	}

	if run := session.Snapshot(); run.Status != RunStatusRunning || run.CancelRequested {
		t.Fatalf("cancelling a question must leave the run active: %+v", run)
	}

	history := session.History(0)
	for _, event := range history {
		if event.AgentEvent.Type != agentcore.UserQuestionResolvedType {
			continue
		}
		if event.AgentEvent.QuestionResolution != string(agentcore.UserQuestionCancelled) || event.AgentEvent.QuestionAnswer != "" {
			t.Fatalf("cancelled resolved event mismatch: %+v", event.AgentEvent)
		}
		return
	}
	t.Fatal("cancelled resolved event not found in history")
}

func TestSessionResolveUserQuestionMissing(t *testing.T) {
	session := NewSession("run-1", "thread-1", "hello", nil, nil, func() {})
	if err := session.ResolveUserQuestion("missing", "answer"); err != ErrUserQuestionNotFound {
		t.Fatalf("expected ErrUserQuestionNotFound, got %v", err)
	}
}

func TestSessionResolveUserQuestionEmptyAnswer(t *testing.T) {
	session := NewSession("run-1", "thread-1", "hello", nil, nil, func() {})
	if err := session.ResolveUserQuestion("q-1", "   "); err != ErrUserQuestionNotFound {
		t.Fatalf("expected ErrUserQuestionNotFound for empty answer, got %v", err)
	}
}
