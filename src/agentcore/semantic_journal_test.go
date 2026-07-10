package agentcore

import (
	"context"
	"errors"
	"testing"

	convcontext "awaken/src/context"
)

type failingRecorder struct {
	failToolCall bool
}

func (r *failingRecorder) Append(ctx context.Context, record SemanticRecord) error {
	return r.AppendBatch(ctx, []SemanticRecord{record})
}

func (r *failingRecorder) AppendBatch(_ context.Context, records []SemanticRecord) error {
	for _, record := range records {
		if r.failToolCall && record.Kind == SemanticKindToolCall {
			return errors.New("disk full")
		}
	}
	return nil
}

func (r *failingRecorder) Flush(context.Context) error { return nil }

func TestEmitToolCallPersistFailureBlocksBeforeExecution(t *testing.T) {
	recorder := &failingRecorder{failToolCall: true}
	events := newRunEventEmitter("thread_test", EventSinkFunc(func(AwakenEvent) {}))
	events.recorder = recorder
	events.runID = "run_test"

	err := events.emitToolCall(context.Background(), convcontext.ToolCall{
		ID: "call_1",
		Function: convcontext.FunctionCall{
			Name:      "shell",
			Arguments: `{"command":"echo hi"}`,
		},
	})
	if err == nil {
		t.Fatalf("expected persist failure")
	}
}

func TestPersistDurableRequiresRecorder(t *testing.T) {
	err := PersistDurable(context.Background(), nil, EventSinkFunc(func(AwakenEvent) {}), NewSemanticRecord(
		SemanticKindRunStarted, "t", "r", "", "", "", map[string]any{},
	))
	if !errors.Is(err, ErrPersistenceUnavailable) {
		t.Fatalf("expected ErrPersistenceUnavailable, got %v", err)
	}
}

func TestExecuteToolCallsSkipsExecutionAfterPersistFailure(t *testing.T) {
	recorder := &failingRecorder{failToolCall: true}
	progress := 0
	runner := &Runner{
		recorder: recorder,
		threadID: "thread_test",
		runID:    "run_test",
		toolRuntime: NewToolRuntimeWithConfig(EventSinkFunc(func(event AwakenEvent) {
			progress++
		}), ToolRuntimeConfig{}),
	}
	events := newRunEventEmitter("thread_test", EventSinkFunc(func(AwakenEvent) {}))
	events.recorder = recorder
	events.runID = "run_test"

	results := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{{
		ID: "call_1",
		Function: convcontext.FunctionCall{
			Name:      "shell",
			Arguments: `{"command":"echo hi"}`,
		},
	}}, events)
	if len(results) != 1 {
		t.Fatalf("expected 1 synthetic result, got %d", len(results))
	}
	if results[0].Status != ToolExecutionCancelled {
		t.Fatalf("expected cancelled synthetic result, got %+v", results[0])
	}
	if progress != 0 {
		t.Fatalf("expected no tool runtime emissions after persist failure, got %d", progress)
	}
}
