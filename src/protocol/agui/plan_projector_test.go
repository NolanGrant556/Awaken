package agui

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"awaken/src/agentcore"
	"awaken/src/tool"
)

func planUpdatedRecord(t *testing.T, seq uint64, version uint64, explanation string) agentcore.SemanticRecord {
	t.Helper()
	snapshot := tool.RunPlanSnapshot{
		PlanID:      "plan_1",
		Version:     version,
		Explanation: explanation,
		Items: []tool.RunPlanItem{
			{Step: "调查现状", ActiveForm: "正在调查现状", Status: tool.RunPlanCompleted},
			{Step: "实现运行计划协议", ActiveForm: "正在实现运行计划协议", Status: tool.RunPlanInProgress},
			{Step: "补齐自动化验收", ActiveForm: "正在补齐自动化验收", Status: tool.RunPlanPending},
		},
	}
	record := agentcore.PlanUpdatedSemanticRecord(
		"thread-1", "run-1", "turn-1", "session-1", "call_plan_1",
		snapshot, agentcore.DeriveRunPlanCounts(snapshot.Items),
	)
	// The durable store assigns Seq/Timestamp on append; pin them here so the
	// projection assertions are deterministic.
	record.Seq = seq
	record.Timestamp = time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC)
	return record
}

func TestPlanUpdatedProjectionCarriesSnapshotAndIdentity(t *testing.T) {
	record := planUpdatedRecord(t, 11, 2, "需求变更，增加验收步骤")
	frames := ProjectSemanticRecord(record)
	if len(frames) != 1 {
		t.Fatalf("expected one frame, got %+v", frames)
	}
	frame := frames[0]
	if frame.Type != EventAwakenPlanUpdated {
		t.Fatalf("expected AWAKEN_PLAN_UPDATED, got %s", frame.Type)
	}
	if frame.PlanID != "plan_1" || frame.PlanVersion != 2 || frame.PlanExplanation != "需求变更，增加验收步骤" {
		t.Fatalf("projection lost plan identity: %+v", frame)
	}
	if frame.PlanCompletedCount != 1 || frame.PlanTotalCount != 3 || frame.PlanCurrentStep != 2 || frame.PlanAllCompleted {
		t.Fatalf("projection lost derived counters: %+v", frame)
	}
	wantItems := []RunPlanItemReceipt{
		{Step: "调查现状", ActiveForm: "正在调查现状", Status: "completed"},
		{Step: "实现运行计划协议", ActiveForm: "正在实现运行计划协议", Status: "in_progress"},
		{Step: "补齐自动化验收", ActiveForm: "正在补齐自动化验收", Status: "pending"},
	}
	if !reflect.DeepEqual(frame.PlanItems, wantItems) {
		t.Fatalf("projection lost plan items: got %+v want %+v", frame.PlanItems, wantItems)
	}
	if frame.ToolCallID != "call_plan_1" {
		t.Fatalf("projection lost tool_call_id: %+v", frame)
	}
	if frame.ThreadID != "thread-1" || frame.RunID != "run-1" || frame.TurnID != "turn-1" {
		t.Fatalf("projection lost thread/run/turn identity: %+v", frame)
	}
	if frame.Seq != FrameSeq(11, 0) || frame.SemanticSeq != frame.Seq || frame.RecordID != record.RecordID {
		t.Fatalf("projection lost durable identity: seq=%d semanticSeq=%d recordId=%q", frame.Seq, frame.SemanticSeq, frame.RecordID)
	}
	if frame.Role != "" || frame.MessageID != "" || frame.Delta != "" || frame.Content != "" {
		t.Fatalf("plan update must not pose as a chat message: %+v", frame)
	}

	raw, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	for _, key := range []string{
		`"type":"AWAKEN_PLAN_UPDATED"`,
		`"planId":"plan_1"`,
		`"planVersion":2`,
		`"planExplanation":"需求变更，增加验收步骤"`,
		`"planCompletedCount":1`,
		`"planTotalCount":3`,
		`"planCurrentStep":2`,
		`"toolCallId":"call_plan_1"`,
		`"activeForm":"正在实现运行计划协议"`,
	} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("wire payload missing %s: %s", key, raw)
		}
	}
}

// RP-G11: the same assigned records must project to identical frames whether
// they arrive as single live semantic projections or as a replay batch.
func TestPlanUpdatedLiveAndReplayProjectionParity(t *testing.T) {
	ts := time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC)
	first := planUpdatedRecord(t, 2, 1, "")
	second := planUpdatedRecord(t, 4, 2, "调查后增加一个必要步骤")
	records := []agentcore.SemanticRecord{
		{SchemaVersion: 2, RecordID: "rec_run_started_1", Seq: 1, ThreadID: "thread-1", RunID: "run-1", Kind: agentcore.SemanticKindRunStarted, Timestamp: ts, Payload: map[string]any{}},
		first,
		{SchemaVersion: 2, RecordID: "rec_turn_1", Seq: 3, ThreadID: "thread-1", RunID: "run-1", TurnID: "turn-1", Kind: agentcore.SemanticKindTurnBoundary, Timestamp: ts, Payload: map[string]any{"transition_reason": "tool_result_continuation"}},
		second,
		{SchemaVersion: 2, RecordID: "rec_run_terminal_1", Seq: 5, ThreadID: "thread-1", RunID: "run-1", Kind: agentcore.SemanticKindRunTerminal, Timestamp: ts, Payload: map[string]any{"status": "finished"}},
	}

	replay := ProjectSemanticRecords(records)
	var live []Event
	for _, record := range records {
		live = append(live, ProjectSemanticRecord(record)...)
	}
	if !reflect.DeepEqual(live, replay) {
		t.Fatalf("live/replay projection mismatch:\nlive=%+v\nreplay=%+v", live, replay)
	}

	var planFrames []Event
	for _, frame := range replay {
		if frame.Type == EventAwakenPlanUpdated {
			planFrames = append(planFrames, frame)
		}
	}
	if len(planFrames) != 2 {
		t.Fatalf("expected two plan frames in order, got %+v", planFrames)
	}
	if planFrames[0].PlanVersion != 1 || planFrames[1].PlanVersion != 2 {
		t.Fatalf("plan frames out of order: %+v", planFrames)
	}
	if planFrames[0].PlanExplanation != "" {
		t.Fatalf("first snapshot must omit empty explanation, got %q", planFrames[0].PlanExplanation)
	}
	if planFrames[1].PlanExplanation != "调查后增加一个必要步骤" {
		t.Fatalf("second snapshot lost explanation: %+v", planFrames[1])
	}
	for i, record := range []agentcore.SemanticRecord{first, second} {
		frame := planFrames[i]
		if frame.RunID != record.RunID || frame.TurnID != record.TurnID || frame.RecordID != record.RecordID {
			t.Fatalf("plan frame %d lost record identity: %+v vs record %+v", i, frame, record)
		}
		if frame.Seq != FrameSeq(record.Seq, 0) || frame.SemanticSeq != frame.Seq {
			t.Fatalf("plan frame %d lost seq inheritance: %+v", i, frame)
		}
	}
}

func TestPlanUpdatedProjectionToleratesMissingPayload(t *testing.T) {
	for name, payload := range map[string]map[string]any{
		"nil":     nil,
		"empty":   {},
		"partial": {"plan_id": "plan_1", "version": 3},
		"wrong_types": {
			"plan_id":         42,
			"version":         "three",
			"items":           "not-a-list",
			"completed_count": true,
			"all_completed":   "yes",
		},
	} {
		record := agentcore.SemanticRecord{
			SchemaVersion: 2,
			RecordID:      "rec_plan_updated_" + name,
			Seq:           1,
			ThreadID:      "thread-1",
			RunID:         "run-1",
			Kind:          agentcore.SemanticKindPlanUpdated,
			Timestamp:     time.Now().UTC(),
			Payload:       payload,
		}
		frames := ProjectSemanticRecord(record)
		if len(frames) != 1 || frames[0].Type != EventAwakenPlanUpdated {
			t.Fatalf("%s payload: expected one AWAKEN_PLAN_UPDATED frame, got %+v", name, frames)
		}
		if name == "partial" && (frames[0].PlanID != "plan_1" || frames[0].PlanVersion != 3) {
			t.Fatalf("partial payload lost fields: %+v", frames[0])
		}
		if name != "partial" && (frames[0].PlanID != "" || frames[0].PlanVersion != 0 || len(frames[0].PlanItems) != 0) {
			t.Fatalf("%s payload: expected zero-value fields, got %+v", name, frames[0])
		}
	}
}
