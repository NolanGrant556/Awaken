package runtime

import (
	"awaken/src/agentcore"
	"context"
	"errors"
	"testing"
	"time"
)

func TestChildQuestionsCannotRegisterOrReopenUserGate(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		s := NewSession("run", "thread", "", nil, nil, func() {})
		if terminal {
			s.CompleteFinished("done")
		}
		before := s.StreamCursor()
		_, err := s.AwaitUserQuestion(context.Background(), agentcore.AwakenEvent{QuestionID: "child-question", AgentID: "child", SubagentTaskID: "task"})
		if !errors.Is(err, ErrChildUserQuestionUnsupported) {
			t.Fatalf("child question: %v", err)
		}
		if len(s.userQuestions) != 0 || s.StreamCursor() != before {
			t.Fatal("child created a user gate")
		}
		if err := s.ResolveUserQuestion("child-question", "answer"); !errors.Is(err, ErrUserQuestionNotFound) {
			t.Fatal(err)
		}
	}
}

func TestChildAuthorizationStillSurvivesRootTerminalAndActivityOverflow(t *testing.T) {
	s := NewSession("run", "thread", "", nil, nil, func() {})
	s.SetActivityBus(NewActivityBus())
	s.CompleteFinished("done")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := s.AwaitExternalInteractionForm(ctx, agentcore.AwakenEvent{InteractionID: "auth", AgentID: "child", SubagentTaskID: "task"})
		done <- err
	}()
	for {
		s.mu.RLock()
		registered := s.externalInteractions["auth"] != nil
		s.mu.RUnlock()
		if registered {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("authorization did not register")
		}
		time.Sleep(time.Millisecond)
	}
	for i := 0; i < 5000; i++ {
		s.activity.Append(Event{Message: "progress"}, false)
	}
	_, events, _ := s.TranscriptSnapshot()
	found := false
	for _, event := range events {
		found = found || event.AgentEvent.InteractionID == "auth"
	}
	if !found {
		t.Fatal("authorization was lost on replay")
	}
	if err := s.ResolveExternalInteraction("auth", agentcore.ExternalInteractionCompleted); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := s.ResolveExternalInteraction("auth", agentcore.ExternalInteractionCompleted); !errors.Is(err, ErrExternalInteractionNotFound) {
		t.Fatalf("late authorization: %v", err)
	}
}
