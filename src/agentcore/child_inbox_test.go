package agentcore

import (
	"context"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/subagentruntime"
)

func TestInstructionArrivingDuringFinalModelCallIsProcessed(t *testing.T) {
	cfg := testAgentConfig("http://unused", 3)
	conversation := convcontext.NewConversation(nil)
	conversation.UserInput("original assignment")
	runner := NewRunner(cfg, conversation, NewRequestBuilder(cfg))
	var pending []subagentruntime.QueuedMessage
	runner.subagentMessages = func() []subagentruntime.QueuedMessage {
		messages := pending
		pending = nil
		return messages
	}
	calls := 0
	runner.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
		calls++
		if calls == 1 {
			pending = append(pending, subagentruntime.QueuedMessage{ID: "late-instruction", Content: "also handle the missing-file branch"})
			return &llm.Response{Content: "original work is complete"}, nil
		}
		messages := conversation.Context.GetMessages()
		answer, correction := -1, -1
		for i, message := range messages {
			if message.Role == convcontext.RoleAssistant && message.Content == "original work is complete" {
				answer = i
			}
			if strings.Contains(message.Content, "also handle the missing-file branch") {
				correction = i
			}
		}
		if answer < 0 || correction <= answer {
			t.Fatalf("the late instruction must follow the superseded answer: answer=%d correction=%d", answer, correction)
		}
		return &llm.Response{Content: "both branches are complete"}, nil
	}
	answer, err := runner.RunStreamWithEvents(context.Background(), EventSinkFunc(func(AwakenEvent) {}))
	if err != nil || answer != "both branches are complete" || calls != 2 {
		t.Fatalf("late instruction was left behind: answer=%q calls=%d err=%v", answer, calls, err)
	}
}

func TestUserStoppedAllSubagentsIsInjectedIntoRoot(t *testing.T) {
	started := make(chan struct{})
	manager := subagentruntime.NewManager(context.Background(), subagentruntime.Config{
		Starter: func(ctx context.Context, _ subagentruntime.StartRequest) (string, error) {
			close(started)
			<-ctx.Done()
			return "", ctx.Err()
		},
	})
	defer func() { _ = manager.Close(context.Background()) }()
	task, err := manager.Spawn(context.Background(), subagentruntime.SpawnRequest{
		Description: "child", Prompt: "work",
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if err := manager.CancelActiveAssignments(context.Background(), "user_stopped_all"); err != nil {
		t.Fatal(err)
	}
	conversation := convcontext.NewConversation(nil)
	conversation.UserInput("do the work")
	runner := NewRunner(AgentConfig{}, conversation, NewRequestBuilder(AgentConfig{}))
	runner.subagentController = manager
	if got := runner.injectSubagentTerminalNotifications(nil); got == 0 {
		t.Fatal("expected the user-stop notice to reach the root Agent")
	}
	found := false
	for _, message := range conversation.Context.GetMessages() {
		if strings.Contains(message.Content, "the user stopped all active Subagent assignments") &&
			strings.Contains(message.Content, task.TaskID) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("root context missing user-stop notice: %+v", conversation.Context.GetMessages())
	}
}
