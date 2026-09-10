package agentcore

import (
	convcontext "awaken/src/context"
	"awaken/src/tool"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type barrierReadTool struct {
	countingMediaReadTool
	started chan string
	release chan struct{}
}

func (t barrierReadTool) ExecuteOutcome(ctx context.Context, _ string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	id, _ := tool.ToolExecutionIdentityFromContext(ctx)
	t.started <- id.ToolCallID
	select {
	case <-ctx.Done():
		return tool.FailedOutcome("cancelled", "cancelled", tool.ErrorCategoryCancelled, false), nil
	case <-t.release:
	}
	return tool.SucceededOutcome(id.ToolCallID), nil
}

func TestReadBatchOverlapsAndCommitsInCallOrder(t *testing.T) {
	original, err := tool.GetTool("Read")
	if err != nil {
		t.Fatal(err)
	}
	fake := barrierReadTool{started: make(chan string, 2), release: make(chan struct{})}
	if err := tool.RegisterToolForce(fake); err != nil {
		t.Fatal(err)
	}
	defer tool.RegisterToolForce(original)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	config := testAgentConfig("http://unused", 3)
	runner := NewRunner(config, convcontext.NewConversation(nil), NewRequestBuilder(config))
	calls := []convcontext.ToolCall{{ID: "first", Function: convcontext.FunctionCall{Name: "Read", Arguments: `{"file_path":"one.md"}`}}, {ID: "second", Function: convcontext.FunctionCall{Name: "Read", Arguments: `{"file_path":"two.md"}`}}}
	completed := make(chan []ToolExecutionResult, 1)
	go func() { completed <- runner.executeToolCalls(ctx, calls, nil) }()
	for i := 0; i < 2; i++ {
		select {
		case <-fake.started:
		case <-ctx.Done():
			t.Fatal("reads did not overlap")
		}
	}
	close(fake.release)
	results := <-completed
	if len(results) != 2 || results[0].Content != "first" || results[1].Content != "second" {
		t.Fatalf("result ordering: %+v", results)
	}
}

func TestReadSchedulesSharePersistenceFailureBoundary(t *testing.T) {
	for _, count := range []int{1, 2, 5} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			original, err := tool.GetTool("Read")
			if err != nil {
				t.Fatal(err)
			}
			release := make(chan struct{})
			close(release)
			fake := barrierReadTool{started: make(chan string, 8), release: release}
			if err := tool.RegisterToolForce(fake); err != nil {
				t.Fatal(err)
			}
			defer tool.RegisterToolForce(original)
			for _, failCall := range []bool{true, false} {
				recorder := &failingRecorder{failToolCall: failCall, failToolResult: !failCall}
				events := newRunEventEmitter("thread", EventSinkFunc(func(AwakenEvent) {}))
				events.recorder, events.runID = recorder, "run"
				config := testAgentConfig("http://unused", 3)
				runner := NewRunner(config, convcontext.NewConversation(nil), NewRequestBuilder(config))
				var calls []convcontext.ToolCall
				for i := 0; i < count; i++ {
					calls = append(calls, convcontext.ToolCall{ID: fmt.Sprint(i), Function: convcontext.FunctionCall{Name: "Read", Arguments: fmt.Sprintf(`{"file_path":"file-%d.md"}`, i)}})
				}
				results := runner.executeToolCalls(context.Background(), calls, events)
				if len(results) != count {
					t.Fatal("lost result identity")
				}
				executed := len(fake.started)
				for len(fake.started) > 0 {
					<-fake.started
				}
				if failCall && executed != 0 {
					t.Fatalf("executed before durable call: %d", executed)
				}
				if !failCall && executed != min(count, maxParallelReadTools) {
					t.Fatalf("execution continued after result persistence failure: %d", executed)
				}
				if !errors.Is(events.persistenceFailure(), ErrPersistenceFailed) {
					t.Fatal("lost persistence failure")
				}
				if count > maxParallelReadTools && results[count-1].ErrorCode != string(TerminalReasonPersistenceError) {
					t.Fatal("later batch did not retain persistence failure")
				}
			}
		})
	}
}
