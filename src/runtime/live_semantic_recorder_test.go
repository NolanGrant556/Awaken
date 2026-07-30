package runtime

import (
	"context"
	"errors"
	"testing"

	"awaken/src/agentcore"
	"awaken/src/protocol/agui"
)

type flushRecoveryRecorder struct {
	nextSeq  uint64
	records  []agentcore.SemanticRecord
	failNext bool
}

func (r *flushRecoveryRecorder) Append(ctx context.Context, record agentcore.SemanticRecord) error {
	_, err := r.AppendAssigned(ctx, record)
	return err
}

func (r *flushRecoveryRecorder) AppendBatch(ctx context.Context, records []agentcore.SemanticRecord) error {
	_, err := r.AppendBatchAssigned(ctx, records)
	return err
}

func (r *flushRecoveryRecorder) AppendAssigned(_ context.Context, record agentcore.SemanticRecord) (agentcore.SemanticRecord, error) {
	assigned, err := r.AppendBatchAssigned(context.Background(), []agentcore.SemanticRecord{record})
	if err != nil {
		return agentcore.SemanticRecord{}, err
	}
	return assigned[0], nil
}

func (r *flushRecoveryRecorder) AppendBatchAssigned(_ context.Context, records []agentcore.SemanticRecord) ([]agentcore.SemanticRecord, error) {
	assigned := make([]agentcore.SemanticRecord, 0, len(records))
	for _, record := range records {
		r.nextSeq++
		record.Seq = r.nextSeq
		assigned = append(assigned, record)
		r.records = append(r.records, record)
	}
	return assigned, nil
}

func (r *flushRecoveryRecorder) Flush(context.Context) error {
	if r.failNext {
		r.failNext = false
		return errors.New("fsync failed")
	}
	return nil
}

func TestLiveSemanticRecorderFlushFailureReplacesUnconfirmedTerminal(t *testing.T) {
	session := NewSession("run_flush", "thread_flush", "input", nil, nil, nil)
	base := &flushRecoveryRecorder{failNext: true}
	recorder := newLiveSemanticRecorder(base, session)

	finished := agentcore.NewSemanticRecord(
		agentcore.SemanticKindRunTerminal,
		"thread_flush",
		"run_flush",
		"turn_1",
		"",
		"",
		map[string]any{"status": "finished", "result": "not durable"},
	)
	if err := recorder.Append(context.Background(), finished); err != nil {
		t.Fatalf("append finished terminal: %v", err)
	}
	if err := recorder.Flush(context.Background()); err == nil {
		t.Fatal("expected injected fsync failure")
	}
	if got := session.StreamCursor(); got != 0 {
		t.Fatalf("unconfirmed finished terminal became live at cursor %d", got)
	}

	persistenceError := agentcore.NewSemanticRecord(
		agentcore.SemanticKindRunTerminal,
		"thread_flush",
		"run_flush",
		"turn_1",
		"",
		"",
		map[string]any{
			"status":          "error",
			"error":           "persistence failed",
			"code":            "persistence_error",
			"terminal_reason": "persistence_error",
		},
	)
	if err := recorder.Append(context.Background(), persistenceError); err != nil {
		t.Fatalf("append recovery terminal: %v", err)
	}
	if err := recorder.Flush(context.Background()); err != nil {
		t.Fatalf("flush recovery terminal: %v", err)
	}

	events, _ := session.StreamSnapshot()
	if len(events) != 1 || events[0].SemanticRecord.Kind != agentcore.SemanticKindRunTerminal ||
		stringPayload(events[0].SemanticRecord.Payload, "status") != "error" {
		t.Fatalf("live stream must contain only persistence_error terminal: %+v", events)
	}
	if got := session.Snapshot(); got.Status != RunStatusError || got.Error != "persistence failed" {
		t.Fatalf("session exposed false success after fsync recovery: %+v", got)
	}

	projected := semanticRecordsWithTerminalLast(base.records)
	if len(projected) != 1 || stringPayload(projected[0].Payload, "status") != "error" {
		t.Fatalf("durable replay must keep only final recovery terminal: %+v", projected)
	}
	durableRun := durableRunsFromSemanticRecords("thread_flush", base.records)["run_flush"]
	if durableRun.Run.Status != RunStatusError || len(durableRun.SemanticRecords) != 1 ||
		stringPayload(durableRun.SemanticRecords[0].Payload, "terminal_reason") != "persistence_error" {
		t.Fatalf("authoritative durable run did not apply terminal last-wins: %+v", durableRun)
	}
	frames := agui.ProjectSemanticRecords(durableRun.SemanticRecords)
	var finishedFrames, errored int
	for _, frame := range frames {
		switch frame.Type {
		case agui.EventRunFinished:
			finishedFrames++
		case agui.EventRunError:
			errored++
			if frame.Code != "persistence_error" || frame.TerminalReason != "persistence_error" {
				t.Fatalf("replayed persistence terminal fields drifted: %+v", frame)
			}
		}
	}
	if finishedFrames != 0 || errored != 1 {
		t.Fatalf("restart replay exposed stale finished terminal: finished=%d errored=%d frames=%+v", finishedFrames, errored, frames)
	}
}

func TestLiveSemanticRecorderAppendFlushedPublishesOnlyAfterFlush(t *testing.T) {
	session := NewSession("run_computer", "thread_computer", "input", nil, nil, nil)
	base := &flushRecoveryRecorder{failNext: true}
	recorder := newLiveSemanticRecorder(base, session)
	record := agentcore.NewSemanticRecord(
		agentcore.SemanticKindComputerReviewRequested,
		"thread_computer",
		"run_computer",
		"",
		"",
		"",
		map[string]any{"review_id": "review-1"},
	)
	if _, err := recorder.AppendFlushed(context.Background(), record); err == nil {
		t.Fatal("expected injected flush failure")
	}
	if events, _ := session.StreamSnapshot(); len(events) != 0 {
		t.Fatalf("unflushed Computer review became visible: %+v", events)
	}
	if _, err := recorder.AppendFlushed(context.Background(), record); err != nil {
		t.Fatalf("append flushed review: %v", err)
	}
	events, _ := session.StreamSnapshot()
	if len(events) != 1 || events[0].SemanticRecord.Kind != agentcore.SemanticKindComputerReviewRequested {
		t.Fatalf("expected one durable Computer review event, got %+v", events)
	}
}

var _ agentcore.AssignedSemanticRecorder = (*flushRecoveryRecorder)(nil)
