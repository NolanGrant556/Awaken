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
