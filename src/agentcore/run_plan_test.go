package agentcore

import (
	stdcontext "context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/subagentruntime"
	"awaken/src/tool"
	_ "awaken/src/tools/update_plan"
)

type runPlanPersistSpy struct {
	calls   int
	records []SemanticRecord
	err     error
}

type flushedRunPlanRecorderSpy struct {
	appendCalls        int
	flushCalls         int
	appendFlushedCalls int
	record             SemanticRecord
}

func (s *flushedRunPlanRecorderSpy) Append(stdcontext.Context, SemanticRecord) error {
	s.appendCalls++
	return nil
}

func (s *flushedRunPlanRecorderSpy) AppendBatch(stdcontext.Context, []SemanticRecord) error {
	return nil
}

func (s *flushedRunPlanRecorderSpy) Flush(stdcontext.Context) error {
	s.flushCalls++
	return nil
}

func (s *flushedRunPlanRecorderSpy) AppendFlushed(_ stdcontext.Context, record SemanticRecord) (SemanticRecord, error) {
	s.appendFlushedCalls++
	s.record = record
	return record, nil
}

func (s *runPlanPersistSpy) persist(_ stdcontext.Context, record SemanticRecord) error {
	s.calls++
	if s.err != nil {
		return s.err
	}
	s.records = append(s.records, record)
	return nil
}

func newTestRunPlanService(spy *runPlanPersistSpy) tool.RunPlanService {
	return NewRunPlanService(RunPlanServiceConfig{
		ThreadID:  "thread_plan",
		RunID:     "run_plan",
		SessionID: "session_plan",
		Persist:   spy.persist,
		Now: func() time.Time {
			return time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
		},
		NewPlanID: func() string { return "plan_test" },
	})
}

func runPlanIdentity() tool.ToolExecutionIdentity {
	return tool.ToolExecutionIdentity{
		RunID:      "run_plan",
		TurnID:     "turn_1",
		ToolCallID: "call_plan_1",
		ToolName:   "update_plan",
	}
}

func runPlanItems(statuses ...tool.RunPlanStatus) []tool.RunPlanItem {
	steps := []string{"调查现状", "实现功能", "补齐验证", "更新文档"}
	items := make([]tool.RunPlanItem, 0, len(statuses))
	for index, status := range statuses {
		step := steps[index%len(steps)]
		if index >= len(steps) {
			step = step + string(rune('A'+index/len(steps)))
		}
		items = append(items, tool.RunPlanItem{
			Step:       step,
			ActiveForm: "正在" + step,
			Status:     status,
		})
	}
	return items
}

func planErrorCode(t *testing.T, err error) *tool.RunPlanError {
	t.Helper()
	var planErr *tool.RunPlanError
	if !errors.As(err, &planErr) {
		t.Fatalf("expected *tool.RunPlanError, got %T: %v", err, err)
	}
	return planErr
}

// RP-G01: first valid update establishes plan ID, version=1 and counters.
func TestRunPlanServiceFirstValidUpdate(t *testing.T) {
	spy := &runPlanPersistSpy{}
	service := newTestRunPlanService(spy)
	result, err := service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		Items: runPlanItems(tool.RunPlanInProgress, tool.RunPlanPending),
	})
	if err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}
	if result.Snapshot.PlanID != "plan_test" || !strings.HasPrefix(result.Snapshot.PlanID, "plan_") {
		t.Fatalf("plan id not established: %+v", result.Snapshot)
	}
	if result.Snapshot.Version != 1 {
		t.Fatalf("version = %d, want 1", result.Snapshot.Version)
	}
	if result.CompletedCount != 0 || result.TotalCount != 2 || result.CurrentStep != 1 || result.AllCompleted {
		t.Fatalf("counters = %+v", result)
	}
	if spy.calls != 1 || len(spy.records) != 1 {
		t.Fatalf("persist calls = %d records = %d", spy.calls, len(spy.records))
	}
	record := spy.records[0]
	if record.Kind != SemanticKindPlanUpdated {
		t.Fatalf("record kind = %q", record.Kind)
	}
	if record.RunID != "run_plan" || record.ThreadID != "thread_plan" || record.SessionID != "session_plan" || record.TurnID != "turn_1" {
		t.Fatalf("record identity = %+v", record)
	}
	payload := record.Payload
	if payload["plan_id"] != "plan_test" || payload["version"] != uint64(1) || payload["tool_call_id"] != "call_plan_1" {
		t.Fatalf("payload = %+v", payload)
	}
	if payload["completed_count"] != 0 || payload["total_count"] != 2 || payload["current_step"] != 1 || payload["all_completed"] != false {
		t.Fatalf("payload counters = %+v", payload)
	}
	if _, hasExplanation := payload["explanation"]; hasExplanation {
		t.Fatalf("empty explanation must be omitted: %+v", payload)
	}
	items, ok := payload["items"].([]map[string]any)
	if !ok || len(items) != 2 || items[0]["step"] != "调查现状" || items[0]["status"] != "in_progress" {
		t.Fatalf("payload items = %+v", payload["items"])
	}
	snapshot, ok := service.Snapshot()
	if !ok || snapshot.Version != 1 || snapshot.PlanID != "plan_test" {
		t.Fatalf("Snapshot = %+v ok=%v", snapshot, ok)
	}
}

func TestPersistRunPlanRecordUsesFlushBeforePublishRecorderPath(t *testing.T) {
	recorder := &flushedRunPlanRecorderSpy{}
	runner := &Runner{recorder: recorder}
	events := newRunEventEmitter("thread_plan", EventSinkFunc(func(AwakenEvent) {}))
	record := NewSemanticRecord(
		SemanticKindPlanUpdated,
		"thread_plan",
		"run_plan",
		"turn_plan",
		"call_plan",
		"session_plan",
		map[string]any{"version": uint64(1)},
	)

	if err := runner.persistRunPlanRecord(stdcontext.Background(), events, record); err != nil {
		t.Fatalf("persistRunPlanRecord: %v", err)
	}
	if recorder.appendFlushedCalls != 1 || recorder.appendCalls != 0 || recorder.flushCalls != 0 {
		t.Fatalf(
			"recorder calls appendFlushed=%d append=%d flush=%d",
			recorder.appendFlushedCalls,
			recorder.appendCalls,
			recorder.flushCalls,
		)
	}
	if recorder.record.RecordID != record.RecordID {
		t.Fatalf("persisted record = %+v, want %s", recorder.record, record.RecordID)
	}
}

// RP-G02: subsequent updates increment the version, keep the plan ID and
// enforce exactly one in_progress item.
func TestRunPlanServiceSubsequentUpdate(t *testing.T) {
	spy := &runPlanPersistSpy{}
	service := newTestRunPlanService(spy)
	first, err := service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		Items: runPlanItems(tool.RunPlanInProgress, tool.RunPlanPending),
	})
	if err != nil {
		t.Fatalf("first ApplyUpdate: %v", err)
	}
	second, err := service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		BaseVersion: first.Snapshot.Version,
		Items:       runPlanItems(tool.RunPlanCompleted, tool.RunPlanInProgress),
	})
	if err != nil {
		t.Fatalf("second ApplyUpdate: %v", err)
	}
	if second.Snapshot.Version != 2 || second.Snapshot.PlanID != first.Snapshot.PlanID {
		t.Fatalf("second snapshot = %+v, first plan id = %q", second.Snapshot, first.Snapshot.PlanID)
	}
	if second.CompletedCount != 1 || second.TotalCount != 2 || second.CurrentStep != 2 || second.AllCompleted {
		t.Fatalf("counters = %+v", second)
	}
	// Zero in_progress with remaining pending work is invalid.
	_, err = service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		BaseVersion: 2,
		Items:       runPlanItems(tool.RunPlanCompleted, tool.RunPlanPending),
	})
	if planErr := planErrorCode(t, err); planErr.Code != tool.RunPlanErrorInvalidInput {
		t.Fatalf("zero in_progress error = %+v", planErr)
	}
	// Two in_progress items are invalid.
	_, err = service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		BaseVersion: 2,
		Items: []tool.RunPlanItem{
			{Step: "甲", ActiveForm: "正在甲", Status: tool.RunPlanInProgress},
			{Step: "乙", ActiveForm: "正在乙", Status: tool.RunPlanInProgress},
		},
	})
	if planErr := planErrorCode(t, err); planErr.Code != tool.RunPlanErrorInvalidInput {
		t.Fatalf("two in_progress error = %+v", planErr)
	}
	snapshot, _ := service.Snapshot()
	if snapshot.Version != 2 {
		t.Fatalf("rejected updates must not move the snapshot, version = %d", snapshot.Version)
	}
}

// RP-G03: an all-completed snapshot reports M/M with no pending/in_progress.
func TestRunPlanServiceAllCompleted(t *testing.T) {
	spy := &runPlanPersistSpy{}
	service := newTestRunPlanService(spy)
	first, err := service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		Items: runPlanItems(tool.RunPlanInProgress, tool.RunPlanPending),
	})
	if err != nil {
		t.Fatalf("first ApplyUpdate: %v", err)
	}
	done, err := service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		BaseVersion: first.Snapshot.Version,
		Items:       runPlanItems(tool.RunPlanCompleted, tool.RunPlanCompleted),
	})
	if err != nil {
		t.Fatalf("all-completed ApplyUpdate: %v", err)
	}
	if !done.AllCompleted || done.CompletedCount != 2 || done.TotalCount != 2 || done.CurrentStep != 2 {
		t.Fatalf("all-completed counters = %+v", done)
	}
	// completed + in_progress (no pending) is still a legal in-flight
	// snapshot: it is not all-completed and has exactly one in_progress.
	// Moving an item out of completed requires an explanation.
	if _, err = service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		BaseVersion: 2,
		Explanation: "第二步需要返工",
		Items:       runPlanItems(tool.RunPlanCompleted, tool.RunPlanInProgress),
	}); err != nil {
		t.Fatalf("completed + in_progress should be valid: %v", err)
	}
	// A single in_progress item is a valid one-step in-flight plan.
	_, err = service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		BaseVersion: 3,
		Explanation: "合并为单步计划",
		Items: []tool.RunPlanItem{
			{Step: "调查现状", ActiveForm: "正在调查现状", Status: tool.RunPlanInProgress},
		},
	})
	if err != nil {
		t.Fatalf("single in_progress item should be valid (not all completed): %v", err)
	}
}

// RP-G04: invalid order, duplicate steps, over-limit item counts and text
// lengths are validation failures that keep the old snapshot and journal.
func TestRunPlanServiceValidationFailures(t *testing.T) {
	spy := &runPlanPersistSpy{}
	service := newTestRunPlanService(spy)
	first, err := service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		Items: runPlanItems(tool.RunPlanInProgress, tool.RunPlanPending),
	})
	if err != nil {
		t.Fatalf("first ApplyUpdate: %v", err)
	}
	longText := strings.Repeat("长", 161)
	cases := map[string][]tool.RunPlanItem{
		"completed_after_pending": {
			{Step: "甲", ActiveForm: "正在甲", Status: tool.RunPlanPending},
			{Step: "乙", ActiveForm: "正在乙", Status: tool.RunPlanCompleted},
		},
		"in_progress_after_pending": {
			{Step: "甲", ActiveForm: "正在甲", Status: tool.RunPlanPending},
			{Step: "乙", ActiveForm: "正在乙", Status: tool.RunPlanInProgress},
		},
		"completed_after_in_progress": {
			{Step: "甲", ActiveForm: "正在甲", Status: tool.RunPlanInProgress},
			{Step: "乙", ActiveForm: "正在乙", Status: tool.RunPlanCompleted},
		},
		"duplicate_step": {
			{Step: "甲", ActiveForm: "正在甲", Status: tool.RunPlanInProgress},
			{Step: "甲", ActiveForm: "正在甲（重复）", Status: tool.RunPlanPending},
		},
		"duplicate_step_after_trim": {
			{Step: "甲", ActiveForm: "正在甲", Status: tool.RunPlanInProgress},
			{Step: " 甲 ", ActiveForm: "正在甲", Status: tool.RunPlanPending},
		},
		"step_too_long": {
			{Step: longText, ActiveForm: "正在" + longText, Status: tool.RunPlanInProgress},
		},
		"step_empty": {
			{Step: "   ", ActiveForm: "正在甲", Status: tool.RunPlanInProgress},
		},
		"active_form_empty": {
			{Step: "甲", ActiveForm: "", Status: tool.RunPlanInProgress},
		},
		"bad_status": {
			{Step: "甲", ActiveForm: "正在甲", Status: tool.RunPlanStatus("doing")},
		},
	}
	for name, items := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
				BaseVersion: first.Snapshot.Version,
				Items:       items,
			})
			planErr := planErrorCode(t, err)
			if planErr.Code != tool.RunPlanErrorInvalidInput || planErr.Retryable {
				t.Fatalf("error = %+v", planErr)
			}
		})
	}
	// Item count bounds.
	_, err = service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		BaseVersion: first.Snapshot.Version,
		Items:       nil,
	})
	if planErr := planErrorCode(t, err); planErr.Code != tool.RunPlanErrorInvalidInput {
		t.Fatalf("empty plan error = %+v", planErr)
	}
	tooMany := make([]tool.RunPlanItem, 0, 13)
	for i := 0; i < 13; i++ {
		status := tool.RunPlanPending
		if i == 0 {
			status = tool.RunPlanInProgress
		}
		tooMany = append(tooMany, tool.RunPlanItem{Step: strings.Repeat("步", 1) + string(rune('a'+i)), ActiveForm: "正在", Status: status})
	}
	_, err = service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		BaseVersion: first.Snapshot.Version,
		Items:       tooMany,
	})
	if planErr := planErrorCode(t, err); planErr.Code != tool.RunPlanErrorInvalidInput {
		t.Fatalf("13-item plan error = %+v", planErr)
	}
	snapshot, _ := service.Snapshot()
	if snapshot.Version != first.Snapshot.Version {
		t.Fatalf("snapshot moved after validation failures: %+v", snapshot)
	}
	if spy.calls != 1 || len(spy.records) != 1 {
		t.Fatalf("journal moved after validation failures: calls=%d records=%d", spy.calls, len(spy.records))
	}
}

// RP-G05: reverting or deleting a completed step without an explanation is
// rejected; with an explanation it establishes a new version.
func TestRunPlanServiceCompletedRegressionRequiresExplanation(t *testing.T) {
	spy := &runPlanPersistSpy{}
	service := newTestRunPlanService(spy)
	if _, err := service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		Items: runPlanItems(tool.RunPlanInProgress, tool.RunPlanPending),
	}); err != nil {
		t.Fatalf("first ApplyUpdate: %v", err)
	}
	second, err := service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		BaseVersion: 1,
		Items:       runPlanItems(tool.RunPlanCompleted, tool.RunPlanInProgress),
	})
	if err != nil {
		t.Fatalf("second ApplyUpdate: %v", err)
	}
	// Revert the completed step without explanation.
	_, err = service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		BaseVersion: second.Snapshot.Version,
		Items:       runPlanItems(tool.RunPlanInProgress, tool.RunPlanPending),
	})
	if planErr := planErrorCode(t, err); planErr.Code != tool.RunPlanErrorInvalidInput {
		t.Fatalf("silent revert error = %+v", planErr)
	}
	// Delete the completed step without explanation.
	_, err = service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		BaseVersion: second.Snapshot.Version,
		Items: []tool.RunPlanItem{
			{Step: "实现功能", ActiveForm: "正在实现功能", Status: tool.RunPlanInProgress},
			{Step: "新步骤", ActiveForm: "正在新步骤", Status: tool.RunPlanPending},
		},
	})
	if planErr := planErrorCode(t, err); planErr.Code != tool.RunPlanErrorInvalidInput {
		t.Fatalf("silent delete error = %+v", planErr)
	}
	// With an explanation the revert establishes a new version.
	third, err := service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		BaseVersion: second.Snapshot.Version,
		Explanation: "需求变更：第一步需要返工",
		Items:       runPlanItems(tool.RunPlanInProgress, tool.RunPlanPending),
	})
	if err != nil {
		t.Fatalf("explained revert: %v", err)
	}
	if third.Snapshot.Version != 3 || third.Snapshot.Explanation != "需求变更：第一步需要返工" {
		t.Fatalf("third snapshot = %+v", third.Snapshot)
	}
	if spy.records[len(spy.records)-1].Payload["explanation"] != "需求变更：第一步需要返工" {
		t.Fatalf("journal explanation = %+v", spy.records[len(spy.records)-1].Payload)
	}
}

// RP-G06: a stale base_version returns the stable conflict code, the latest
// version and a bounded snapshot, and never overwrites the current plan.
func TestRunPlanServiceVersionConflict(t *testing.T) {
	spy := &runPlanPersistSpy{}
	service := newTestRunPlanService(spy)
	// First update must use base_version 0.
	_, err := service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		BaseVersion: 7,
		Items:       runPlanItems(tool.RunPlanInProgress, tool.RunPlanPending),
	})
	if planErr := planErrorCode(t, err); planErr.Code != tool.RunPlanErrorVersionConflict {
		t.Fatalf("first-update base_version error = %+v", planErr)
	}
	first, err := service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		Items: runPlanItems(tool.RunPlanInProgress, tool.RunPlanPending),
	})
	if err != nil {
		t.Fatalf("first ApplyUpdate: %v", err)
	}
	_, err = service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		BaseVersion: 99,
		Items:       runPlanItems(tool.RunPlanCompleted, tool.RunPlanInProgress),
	})
	planErr := planErrorCode(t, err)
	if planErr.Code != tool.RunPlanErrorVersionConflict || !planErr.Retryable {
		t.Fatalf("conflict error = %+v", planErr)
	}
	if planErr.LatestVersion != first.Snapshot.Version {
		t.Fatalf("latest version = %d, want %d", planErr.LatestVersion, first.Snapshot.Version)
	}
	if planErr.LatestSnapshot == nil || planErr.LatestSnapshot.PlanID != first.Snapshot.PlanID ||
		len(planErr.LatestSnapshot.Items) != len(first.Snapshot.Items) {
		t.Fatalf("bounded latest snapshot = %+v", planErr.LatestSnapshot)
	}
	snapshot, _ := service.Snapshot()
	if snapshot.Version != first.Snapshot.Version {
		t.Fatalf("conflict overwrote the snapshot: %+v", snapshot)
	}
	if spy.calls != 1 {
		t.Fatalf("conflict must not persist: calls=%d", spy.calls)
	}
}

// RP-G07: a journal append/flush failure fails the update without moving
// memory and surfaces a retryable error; a later retry succeeds.
func TestRunPlanServicePersistenceFailure(t *testing.T) {
	spy := &runPlanPersistSpy{err: errors.New("disk full")}
	service := newTestRunPlanService(spy)
	_, err := service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		Items: runPlanItems(tool.RunPlanInProgress, tool.RunPlanPending),
	})
	planErr := planErrorCode(t, err)
	if planErr.Code != tool.RunPlanErrorPersistenceFailed || !planErr.Retryable {
		t.Fatalf("persistence error = %+v", planErr)
	}
	if _, ok := service.Snapshot(); ok {
		t.Fatal("memory must not move when persistence fails")
	}
	if len(spy.records) != 0 {
		t.Fatalf("no record should be appended on failure: %d", len(spy.records))
	}
	spy.err = nil
	result, err := service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		Items: runPlanItems(tool.RunPlanInProgress, tool.RunPlanPending),
	})
	if err != nil {
		t.Fatalf("retry after persistence failure: %v", err)
	}
	if result.Snapshot.Version != 1 {
		t.Fatalf("retry version = %d, want 1", result.Snapshot.Version)
	}
}

// RP-G08: after compaction the CurrentRunPlan block still appears in later
// prepared requests as a host-owned system block, never as a user message.
func TestRunPlanContextBlockSurvivesCompact(t *testing.T) {
	spy := &runPlanPersistSpy{}
	service := newTestRunPlanService(spy)
	if _, err := service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		Items: runPlanItems(tool.RunPlanCompleted, tool.RunPlanInProgress, tool.RunPlanPending),
	}); err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}
	config := AgentConfig{ModelConfig: ModelConfig{Model: "test-model"}}
	builder := NewRequestBuilder(config)
	builder.UpdateRunPlanContextProvider(service.ContextBlock)
	conversation := convcontext.NewConversation(nil)
	conversation.UserInput("实现运行计划")

	assertPlanBlock := func(stage string) {
		envelope := builder.BuildContextEnvelope(conversation.Context, nil)
		var block *ContextBlock
		for index := range envelope.SystemBlocks {
			if envelope.SystemBlocks[index].ID == "current_run_plan" {
				block = &envelope.SystemBlocks[index]
			}
		}
		if block == nil {
			t.Fatalf("%s: CurrentRunPlan block missing from system blocks", stage)
		}
		if !strings.Contains(block.Content, "plan_test") || !strings.Contains(block.Content, "version: 1") ||
			!strings.Contains(block.Content, "[in_progress] 实现功能") {
			t.Fatalf("%s: block content = %q", stage, block.Content)
		}
		foundItem := false
		for _, item := range envelope.SystemContextItems {
			if item.ID == "current_run_plan" {
				foundItem = true
			}
		}
		if !foundItem {
			t.Fatalf("%s: CurrentRunPlan missing from system context items", stage)
		}
		foundEpoch := false
		for _, source := range envelope.EpochSources {
			if source.SourceKey == "current_run_plan" && source.State == ContextEpochSourceUpdated {
				foundEpoch = true
			}
		}
		if !foundEpoch {
			t.Fatalf("%s: CurrentRunPlan missing from epoch sources", stage)
		}
		for _, message := range envelope.Messages {
			if strings.Contains(message.Content, "awaken_run_plan") || strings.Contains(message.Content, "plan_test") {
				t.Fatalf("%s: plan leaked into a conversation message: %+v", stage, message)
			}
		}
	}
	assertPlanBlock("before compact")

	// Simulate compaction replacing the conversation with a compacted subset.
	conversation.Context.SetMessages([]convcontext.Message{{Role: convcontext.RoleUser, Content: "（压缩后）继续实现运行计划"}})
	assertPlanBlock("after compact")
}

// RP-G09 (Core half): nothing in the service auto-completes unfinished items;
// the snapshot keeps its true three-state until an explicit update.
func TestRunPlanServiceKeepsTrueStateWithoutAutoCompletion(t *testing.T) {
	spy := &runPlanPersistSpy{}
	service := newTestRunPlanService(spy)
	if _, err := service.ApplyUpdate(stdcontext.Background(), runPlanIdentity(), tool.RunPlanUpdate{
		Items: runPlanItems(tool.RunPlanCompleted, tool.RunPlanInProgress, tool.RunPlanPending),
	}); err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}
	snapshot, ok := service.Snapshot()
	if !ok {
		t.Fatal("snapshot missing")
	}
	if snapshot.Items[1].Status != tool.RunPlanInProgress || snapshot.Items[2].Status != tool.RunPlanPending {
		t.Fatalf("unfinished items must not be auto-completed: %+v", snapshot.Items)
	}
	block, err := service.ContextBlock()
	if err != nil || !strings.Contains(block, "[pending] 补齐验证") {
		t.Fatalf("context block = %q err=%v", block, err)
	}
}

// RP-G01/RP-G08 (runner wiring): a root run binds the RunPlanService into the
// tool context, persists plan_updated journal-first, and keeps the
// CurrentRunPlan block in later prepared requests.
func TestRootRunWiresRunPlanService(t *testing.T) {
	config := AgentConfig{
		MaxThoughts: 4,
		ModelConfig: ModelConfig{Model: "test-model"},
		Workspace:   WorkspaceConfig{Root: t.TempDir()},
	}
	conversation := convcontext.NewConversation(nil)
	conversation.UserInput("实现运行计划")
	runner := NewRunner(config, conversation, NewRequestBuilder(config))
	runner.threadID = "thread_root_plan"
	runner.runID = "run_root_plan"
	runner.sessionID = "session_root_plan"
	recorder := &forkCaptureRecorder{}
	runner.recorder = recorder
	calls := 0
	runner.streamLLMFunc = func(stdcontext.Context, *runEventEmitter) (*llm.Response, error) {
		calls++
		if calls == 1 {
			return &llm.Response{ToolCalls: []llm.ToolCall{{
				ID:   "call_update_plan",
				Name: "update_plan",
				Input: `{"plan":[` +
					`{"step":"调查现状","active_form":"正在调查现状","status":"completed"},` +
					`{"step":"实现功能","active_form":"正在实现功能","status":"in_progress"},` +
					`{"step":"补齐验证","active_form":"正在补齐验证","status":"pending"}]}`,
			}}}, nil
		}
		return &llm.Response{Content: "完成"}, nil
	}
	answer, err := runner.RunStreamWithEvents(stdcontext.Background(), EventSinkFunc(func(AwakenEvent) {}))
	if err != nil {
		t.Fatalf("RunStreamWithEvents: %v", err)
	}
	if answer != "完成" {
		t.Fatalf("answer = %q", answer)
	}
	if runner.runPlanService == nil {
		t.Fatal("root run must bind a RunPlanService")
	}
	snapshot, ok := runner.runPlanService.Snapshot()
	if !ok || snapshot.Version != 1 || !strings.HasPrefix(snapshot.PlanID, "plan_") {
		for _, record := range recorder.snapshot() {
			t.Logf("record kind=%s payload=%+v", record.Kind, record.Payload)
		}
		t.Fatalf("snapshot = %+v ok=%v", snapshot, ok)
	}
	var planRecord *SemanticRecord
	var toolResultRecord *SemanticRecord
	for index := range recorder.snapshot() {
		record := recorder.snapshot()[index]
		switch record.Kind {
		case SemanticKindPlanUpdated:
			copy := record
			planRecord = &copy
		case SemanticKindToolResult:
			if record.Payload["tool_name"] == "update_plan" {
				copy := record
				toolResultRecord = &copy
			}
		}
	}
	if planRecord == nil {
		t.Fatal("plan_updated record missing from journal")
	}
	if _, childOwned := planRecord.Payload["agent_id"]; childOwned {
		t.Fatalf("root plan record must not carry agent_id: %+v", planRecord.Payload)
	}
	if planRecord.Payload["tool_call_id"] != "call_update_plan" || planRecord.Payload["version"] != uint64(1) {
		t.Fatalf("plan payload = %+v", planRecord.Payload)
	}
	if toolResultRecord == nil || toolResultRecord.Payload["is_error"] == true {
		t.Fatalf("update_plan tool result = %+v", toolResultRecord)
	}
	// The next prepared request carries CurrentRunPlan as a system block, not
	// as a conversation message.
	llmReq, err := runner.buildLLMRequest()
	if err != nil {
		t.Fatalf("buildLLMRequest: %v", err)
	}
	if !strings.Contains(llmReq.SystemPrompt, "awaken_run_plan") || !strings.Contains(llmReq.SystemPrompt, snapshot.PlanID) {
		t.Fatalf("CurrentRunPlan missing from system prompt")
	}
	for _, message := range llmReq.Messages {
		if strings.Contains(message.Content, "awaken_run_plan") {
			t.Fatalf("plan block leaked into messages: %+v", message)
		}
	}
}

// RP-G10 (runtime half): a child run never receives a RunPlanService, so
// update_plan degrades to plan_service_unavailable instead of touching the
// parent's plan.
func TestChildRunHasNoRunPlanService(t *testing.T) {
	root := t.TempDir()
	config := AgentConfig{
		MaxThoughts: 4,
		ModelConfig: ModelConfig{Model: "test-model"},
		Workspace: WorkspaceConfig{
			Root: root, ReadableRoots: []string{root}, WritableRoots: []string{root},
		},
	}
	childConfig, err := CreateSubagent(config, SpawnConfig{AgentID: "child_plan", ParentAgentID: "root"})
	if err != nil {
		t.Fatalf("CreateSubagent: %v", err)
	}
	if childConfig.RunPlanContextProvider != nil {
		t.Fatal("child config must not inherit RunPlanContextProvider")
	}
	conversation := convcontext.NewConversation(nil)
	conversation.UserInput("子任务")
	runner := NewRunner(childConfig, conversation, NewRequestBuilder(childConfig))
	runner.threadID = "thread_child_plan"
	runner.runID = "run_child_plan"
	runner.sessionID = "session_child_plan"
	recorder := &forkCaptureRecorder{}
	runner.recorder = recorder
	calls := 0
	runner.streamLLMFunc = func(stdcontext.Context, *runEventEmitter) (*llm.Response, error) {
		calls++
		if calls == 1 {
			return &llm.Response{ToolCalls: []llm.ToolCall{{
				ID:   "call_child_plan",
				Name: "update_plan",
				Input: `{"plan":[` +
					`{"step":"子步骤","active_form":"正在子步骤","status":"in_progress"}]}`,
			}}}, nil
		}
		return &llm.Response{Content: "完成"}, nil
	}
	if _, err := runner.RunStreamWithEvents(stdcontext.Background(), EventSinkFunc(func(AwakenEvent) {})); err != nil {
		t.Fatalf("RunStreamWithEvents: %v", err)
	}
	if runner.runPlanService != nil {
		t.Fatal("child run must not bind a RunPlanService")
	}
	var toolResultRecord *SemanticRecord
	for index := range recorder.snapshot() {
		record := recorder.snapshot()[index]
		if record.Kind == SemanticKindPlanUpdated {
			t.Fatalf("child run must not persist plan_updated: %+v", record)
		}
		if record.Kind == SemanticKindToolResult && record.Payload["tool_name"] == "update_plan" {
			copy := record
			toolResultRecord = &copy
		}
	}
	if toolResultRecord == nil || toolResultRecord.Payload["is_error"] != true {
		t.Fatalf("child update_plan must fail closed: %+v", toolResultRecord)
	}
	content, _ := toolResultRecord.Payload["content"].(string)
	if !strings.Contains(content, "plan_service_unavailable") {
		t.Fatalf("child update_plan content = %q", content)
	}
}

// RP-G10 (exposure half): the root tool pool exposes update_plan while every
// native child role hides it.
func TestRunPlanToolExposureRootVsChild(t *testing.T) {
	names := map[string]bool{}
	for _, registered := range tool.FilterToolsForExposure(tool.ListTools(), tool.DefaultToolExposurePolicy()) {
		names[registered.Name()] = true
	}
	if !names["update_plan"] {
		t.Fatal("root tool snapshot must contain update_plan")
	}
	rootConfig := AgentConfig{}
	childConfig, err := CreateSubagent(rootConfig, SpawnConfig{AgentID: "child_exposure", ParentAgentID: "root"})
	if err != nil {
		t.Fatalf("CreateSubagent: %v", err)
	}
	for _, agentType := range []string{"explore", "plan", "review", "general"} {
		child := childConfig
		applyChildRole(&child, subagentruntime.TaskSnapshot{AgentType: agentType})
		policy := child.ToolExposure
		if tool.IsToolExposed("update_plan", policy) {
			t.Fatalf("child role %q must hide update_plan", agentType)
		}
	}
	if !tool.IsToolExposed("update_plan", rootConfig.ToolExposure) {
		t.Fatal("root exposure must keep update_plan visible")
	}
}

// Ensure journal payload for update_plan stays JSON-serializable end to end.
func TestPlanUpdatedSemanticRecordPayloadShape(t *testing.T) {
	snapshot := tool.RunPlanSnapshot{
		PlanID:  "plan_shape",
		Version: 2,
		Items: []tool.RunPlanItem{
			{Step: "调查现状", ActiveForm: "正在调查现状", Status: tool.RunPlanCompleted},
			{Step: "实现功能", ActiveForm: "正在实现功能", Status: tool.RunPlanInProgress},
		},
	}
	record := PlanUpdatedSemanticRecord("thread", "run", "turn", "session", "call_1", snapshot, DeriveRunPlanCounts(snapshot.Items))
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("record not serializable: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("record not decodable: %v", err)
	}
	if decoded["kind"] != "plan_updated" {
		t.Fatalf("kind = %v", decoded["kind"])
	}
}
