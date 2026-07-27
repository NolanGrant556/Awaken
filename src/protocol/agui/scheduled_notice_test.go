package agui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"awaken/src/agentcore"
)

func scheduledNoticeRecord(seq uint64) agentcore.SemanticRecord {
	return agentcore.SemanticRecord{
		SchemaVersion: 2,
		RecordID:      "rec_scheduled_notice_1",
		Seq:           seq,
		ThreadID:      "thread-1",
		RunID:         "run-1",
		Kind:          agentcore.SemanticKindScheduledNotice,
		Timestamp:     time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC),
		Payload: map[string]any{
			"task_id":      "task_1",
			"execution_id": "exec_1",
			"task_name":    "每日站会提醒",
			"content":      "记得填写日报",
		},
	}
}

func TestScheduledNoticeProjectionCarriesIdentityAndContent(t *testing.T) {
	frames := ProjectSemanticRecord(scheduledNoticeRecord(7))
	if len(frames) != 1 {
		t.Fatalf("expected one frame, got %+v", frames)
	}
	frame := frames[0]
	if frame.Type != EventAwakenScheduledNotice {
		t.Fatalf("expected AWAKEN_SCHEDULED_NOTICE, got %s", frame.Type)
	}
	if frame.TaskID != "task_1" || frame.ExecutionID != "exec_1" || frame.TaskName != "每日站会提醒" || frame.NoticeContent != "记得填写日报" {
		t.Fatalf("projection lost scheduled notice fields: %+v", frame)
	}
	if frame.ThreadID != "thread-1" || frame.RunID != "run-1" {
		t.Fatalf("projection lost thread/run identity: %+v", frame)
	}
	if frame.Seq != FrameSeq(7, 0) || frame.SemanticSeq != frame.Seq || frame.RecordID != "rec_scheduled_notice_1" {
		t.Fatalf("projection lost durable identity: seq=%d semanticSeq=%d recordId=%q", frame.Seq, frame.SemanticSeq, frame.RecordID)
	}
	if frame.Role != "" || frame.MessageID != "" || frame.Delta != "" || frame.Content != "" {
		t.Fatalf("scheduled notice must not pose as a chat message: %+v", frame)
	}

	raw, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	for _, key := range []string{`"type":"AWAKEN_SCHEDULED_NOTICE"`, `"taskId":"task_1"`, `"executionId":"exec_1"`, `"taskName":"每日站会提醒"`, `"noticeContent":"记得填写日报"`} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("wire payload missing %s: %s", key, raw)
		}
	}
}

func TestScheduledNoticeLiveAndReplayProjectionMatch(t *testing.T) {
	// Live SSE maps EventKindSemantic wrappers through the same
	// ProjectSemanticRecord as durable replay (server.mapRuntimeEvent), so the
	// wire frames must agree field-for-field on the same assigned record.
	record := scheduledNoticeRecord(3)
	live := ProjectSemanticRecord(record)
	replay := ProjectSemanticRecords([]agentcore.SemanticRecord{record})
	if len(live) != 1 || len(replay) != 1 {
		t.Fatalf("live=%+v replay=%+v", live, replay)
	}
	if live[0].Type != replay[0].Type ||
		live[0].TaskID != replay[0].TaskID ||
		live[0].ExecutionID != replay[0].ExecutionID ||
		live[0].TaskName != replay[0].TaskName ||
		live[0].NoticeContent != replay[0].NoticeContent ||
		live[0].Seq != replay[0].Seq ||
		live[0].RecordID != replay[0].RecordID {
		t.Fatalf("live/replay projection mismatch: live=%+v replay=%+v", live[0], replay[0])
	}
}

func TestScheduledNoticeRunFrameSequence(t *testing.T) {
	ts := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	records := []agentcore.SemanticRecord{
		{SchemaVersion: 2, RecordID: "rec_run_started_1", Seq: 1, ThreadID: "thread-1", RunID: "run-1", Kind: agentcore.SemanticKindRunStarted, Timestamp: ts, Payload: map[string]any{}},
		scheduledNoticeRecord(2),
		{SchemaVersion: 2, RecordID: "rec_run_terminal_1", Seq: 3, ThreadID: "thread-1", RunID: "run-1", Kind: agentcore.SemanticKindRunTerminal, Timestamp: ts, Payload: map[string]any{"status": "finished"}},
	}

	frames := ProjectSemanticRecords(records)
	if len(frames) != 3 {
		t.Fatalf("expected three frames, got %+v", frames)
	}
	wantTypes := []EventType{EventRunStarted, EventAwakenScheduledNotice, EventRunFinished}
	for i, want := range wantTypes {
		if frames[i].Type != want {
			t.Fatalf("frame[%d]=%s want %s (all=%v)", i, frames[i].Type, want, frames)
		}
		if frames[i].Seq != FrameSeq(records[i].Seq, 0) {
			t.Fatalf("frame[%d] seq=%d want %d", i, frames[i].Seq, FrameSeq(records[i].Seq, 0))
		}
		if frames[i].RunID != "run-1" || frames[i].ThreadID != "thread-1" {
			t.Fatalf("frame[%d] lost run/thread identity: %+v", i, frames[i])
		}
	}
	if !(frames[0].Seq < frames[1].Seq && frames[1].Seq < frames[2].Seq) {
		t.Fatalf("frame seqs not ordered: %d %d %d", frames[0].Seq, frames[1].Seq, frames[2].Seq)
	}
	terminal := frames[2]
	if terminal.Outcome == nil || terminal.Outcome.Type != "success" {
		t.Fatalf("expected success outcome on terminal frame, got %+v", terminal.Outcome)
	}
	if terminal.Result != "" || terminal.Message != "" {
		t.Fatalf("notice run terminal must not fabricate result/message: %+v", terminal)
	}
}

func TestScheduledNoticeProjectionToleratesMissingPayload(t *testing.T) {
	for name, payload := range map[string]map[string]any{
		"nil":     nil,
		"empty":   {},
		"partial": {"task_id": "task_1"},
		"wrong_types": {
			"task_id":      42,
			"execution_id": true,
			"task_name":    []string{"x"},
			"content":      map[string]any{"nested": 1},
		},
	} {
		record := agentcore.SemanticRecord{
			SchemaVersion: 2,
			RecordID:      "rec_scheduled_notice_" + name,
			Seq:           1,
			ThreadID:      "thread-1",
			RunID:         "run-1",
			Kind:          agentcore.SemanticKindScheduledNotice,
			Timestamp:     time.Now().UTC(),
			Payload:       payload,
		}
		frames := ProjectSemanticRecord(record)
		if len(frames) != 1 || frames[0].Type != EventAwakenScheduledNotice {
			t.Fatalf("%s payload: expected one AWAKEN_SCHEDULED_NOTICE frame, got %+v", name, frames)
		}
		if name == "partial" && frames[0].TaskID != "task_1" {
			t.Fatalf("partial payload lost task id: %+v", frames[0])
		}
		if name != "partial" && (frames[0].TaskID != "" || frames[0].ExecutionID != "" || frames[0].TaskName != "" || frames[0].NoticeContent != "") {
			t.Fatalf("%s payload: expected empty fields, got %+v", name, frames[0])
		}
	}
}
