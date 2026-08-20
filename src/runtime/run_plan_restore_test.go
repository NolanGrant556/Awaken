package runtime

import (
	"testing"
	"time"

	"awaken/src/agentcore"
	"awaken/src/tool"
)

func planUpdatedRecord(t *testing.T, runID string, version uint64, agentID string, items []tool.RunPlanItem) agentcore.SemanticRecord {
	t.Helper()
	snapshot := tool.RunPlanSnapshot{
		PlanID:  "plan_" + runID,
		Version: version,
		Items:   items,
	}
	record := agentcore.PlanUpdatedSemanticRecord(
		"thread_plan", runID, "turn_1", "session_plan", "call_1",
		snapshot, agentcore.DeriveRunPlanCounts(items),
	)
	if agentID != "" {
		record.Payload["agent_id"] = agentID
	}
	return record
}

// RP-G09 (replay half): restart replay keeps the latest run plan snapshot per
// run — lower versions never overwrite higher ones, child records never
// contribute, and unfinished items keep their true three-state.
func TestRunPlanSnapshotsFromSemanticRecords(t *testing.T) {
	inFlight := []tool.RunPlanItem{
		{Step: "调查现状", ActiveForm: "正在调查现状", Status: tool.RunPlanCompleted},
		{Step: "实现功能", ActiveForm: "正在实现功能", Status: tool.RunPlanInProgress},
		{Step: "补齐验证", ActiveForm: "正在补齐验证", Status: tool.RunPlanPending},
	}
	earlier := []tool.RunPlanItem{
		{Step: "调查现状", ActiveForm: "正在调查现状", Status: tool.RunPlanInProgress},
		{Step: "实现功能", ActiveForm: "正在实现功能", Status: tool.RunPlanPending},
	}
	records := []agentcore.SemanticRecord{
		// Out-of-order arrival: v2 first, then a stale v1 which must not win.
		planUpdatedRecord(t, "run_1", 2, "", inFlight),
		planUpdatedRecord(t, "run_1", 1, "", earlier),
		// A child-local record with a higher version must never rebuild the
		// root run plan.
		planUpdatedRecord(t, "run_1", 9, "child_agent", earlier),
		// A run whose only plan records are child-owned has no root snapshot.
		planUpdatedRecord(t, "run_child_only", 1, "child_agent", earlier),
	}
	snapshots := runPlanSnapshotsFromSemanticRecords(records)
	latest, ok := snapshots["run_1"]
	if !ok {
		t.Fatal("run_1 snapshot missing")
	}
	if latest.Version != 2 || latest.PlanID != "plan_run_1" {
		t.Fatalf("snapshot = %+v", latest)
	}
	if len(latest.Items) != 3 || latest.Items[1].Status != tool.RunPlanInProgress ||
		latest.Items[2].Status != tool.RunPlanPending {
		t.Fatalf("unfinished items must keep true state after replay: %+v", latest.Items)
	}
	if _, ok := snapshots["run_child_only"]; ok {
		t.Fatal("child-owned records must not create a root snapshot")
	}

	// restoredTranscript carries the same per-run snapshots.
	restored := restoredTranscriptFromSemantic(records)
	if restored.RunPlanSnapshots["run_1"].Version != 2 {
		t.Fatalf("restored snapshots = %+v", restored.RunPlanSnapshots)
	}
}

// RP-G09 (orphan half): a run interrupted without a terminal plan update
// replays its last durable snapshot unchanged — nothing fabricates completed
// items for cancelled/orphaned runs.
func TestRunPlanReplayPreservesCancelledRunSnapshot(t *testing.T) {
	items := []tool.RunPlanItem{
		{Step: "调查现状", ActiveForm: "正在调查现状", Status: tool.RunPlanCompleted},
		{Step: "实现功能", ActiveForm: "正在实现功能", Status: tool.RunPlanInProgress},
	}
	cancelled := agentcore.NewSemanticRecord(
		agentcore.SemanticKindRunTerminal, "thread_plan", "run_2", "", "", "session_plan",
		map[string]any{"status": "cancelled"},
	)
	records := []agentcore.SemanticRecord{
		planUpdatedRecord(t, "run_2", 1, "", items),
		cancelled,
	}
	restored := restoredTranscriptFromSemantic(records)
	snapshot, ok := restored.RunPlanSnapshots["run_2"]
	if !ok {
		t.Fatal("cancelled run snapshot missing")
	}
	if snapshot.Items[1].Status != tool.RunPlanInProgress {
		t.Fatalf("cancelled run must not pseudo-complete items: %+v", snapshot.Items)
	}
	if snapshot.UpdatedAt.IsZero() || snapshot.UpdatedAt.After(time.Now().UTC()) {
		t.Fatalf("snapshot UpdatedAt = %v", snapshot.UpdatedAt)
	}
}
