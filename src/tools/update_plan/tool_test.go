package updateplan

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"awaken/src/tool"
)

type fakeRunPlanService struct {
	result tool.RunPlanResult
	err    error
	calls  int
}

func (s *fakeRunPlanService) ApplyUpdate(_ context.Context, _ tool.ToolExecutionIdentity, update tool.RunPlanUpdate) (tool.RunPlanResult, error) {
	s.calls++
	if s.err != nil {
		return tool.RunPlanResult{}, s.err
	}
	return s.result, nil
}

func (s *fakeRunPlanService) Snapshot() (tool.RunPlanSnapshot, bool) {
	return s.result.Snapshot, s.result.Snapshot.PlanID != ""
}

func (s *fakeRunPlanService) ContextBlock() (string, error) { return "", nil }

func successService() *fakeRunPlanService {
	return &fakeRunPlanService{result: tool.RunPlanResult{
		Snapshot: tool.RunPlanSnapshot{
			PlanID:  "plan_tool",
			Version: 1,
			Items: []tool.RunPlanItem{
				{Step: "调查现状", ActiveForm: "正在调查现状", Status: tool.RunPlanInProgress},
				{Step: "实现功能", ActiveForm: "正在实现功能", Status: tool.RunPlanPending},
			},
			UpdatedAt: time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC),
		},
		CompletedCount: 0,
		TotalCount:     2,
		CurrentStep:    1,
		AllCompleted:   false,
	}}
}

func ctxWithService(service tool.RunPlanService) context.Context {
	ctx := tool.ContextWithRunPlanService(context.Background(), service)
	return tool.ContextWithToolExecutionIdentity(ctx, tool.ToolExecutionIdentity{
		RunID: "run_tool", TurnID: "turn_1", ToolCallID: "call_1", ToolName: ToolName,
	})
}

const validInput = `{"plan":[` +
	`{"step":"调查现状","active_form":"正在调查现状","status":"in_progress"},` +
	`{"step":"实现功能","active_form":"正在实现功能","status":"pending"}]}`

// RP-G01 (tool level): a successful update emits the §6.1 output schema.
func TestUpdatePlanSuccessOutputSchema(t *testing.T) {
	service := successService()
	outcome, err := (&Tool{}).ExecuteOutcome(ctxWithService(service), validInput, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome: %v", err)
	}
	if outcome.IsError() {
		t.Fatalf("outcome = %+v", outcome)
	}
	if service.calls != 1 {
		t.Fatalf("service calls = %d", service.calls)
	}
	var decoded struct {
		PlanID         string `json:"plan_id"`
		Version        uint64 `json:"version"`
		CompletedCount int    `json:"completed_count"`
		TotalCount     int    `json:"total_count"`
		CurrentStep    int    `json:"current_step"`
		AllCompleted   bool   `json:"all_completed"`
		Plan           []struct {
			Step       string `json:"step"`
			ActiveForm string `json:"active_form"`
			Status     string `json:"status"`
		} `json:"plan"`
	}
	if err := json.Unmarshal([]byte(outcome.Content), &decoded); err != nil {
		t.Fatalf("output not JSON: %v (%q)", err, outcome.Content)
	}
	if decoded.PlanID != "plan_tool" || decoded.Version != 1 || decoded.TotalCount != 2 ||
		decoded.CompletedCount != 0 || decoded.CurrentStep != 1 || decoded.AllCompleted {
		t.Fatalf("output = %+v", decoded)
	}
	if len(decoded.Plan) != 2 || decoded.Plan[0].Step != "调查现状" || decoded.Plan[0].Status != "in_progress" ||
		decoded.Plan[0].ActiveForm != "正在调查现状" {
		t.Fatalf("output plan = %+v", decoded.Plan)
	}
}

// RP-G04 (tool level): malformed JSON and validation failures surface the
// stable validation code and are not retryable.
func TestUpdatePlanInvalidInput(t *testing.T) {
	service := successService()
	outcome, err := (&Tool{}).ExecuteOutcome(ctxWithService(service), `{"plan":`, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome: %v", err)
	}
	if !outcome.IsError() || outcome.ErrorCode != "plan_invalid_input" ||
		outcome.ErrorCategory != tool.ErrorCategoryValidation || outcome.Retryable {
		t.Fatalf("outcome = %+v", outcome)
	}
	if service.calls != 0 {
		t.Fatal("malformed JSON must not reach the service")
	}

	service.err = &tool.RunPlanError{Code: tool.RunPlanErrorInvalidInput, Message: "duplicate step", Retryable: false}
	outcome, err = (&Tool{}).ExecuteOutcome(ctxWithService(service), validInput, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome: %v", err)
	}
	if !outcome.IsError() || outcome.ErrorCode != "plan_invalid_input" || outcome.Retryable {
		t.Fatalf("validation outcome = %+v", outcome)
	}
}

// RP-G06 (tool level): a version conflict carries the stable code, the latest
// version and a bounded snapshot for the model to retry against.
func TestUpdatePlanVersionConflictOutcome(t *testing.T) {
	latest := tool.RunPlanSnapshot{
		PlanID:  "plan_tool",
		Version: 3,
		Items: []tool.RunPlanItem{
			{Step: "调查现状", ActiveForm: "正在调查现状", Status: tool.RunPlanCompleted},
		},
	}
	service := &fakeRunPlanService{err: &tool.RunPlanError{
		Code:           tool.RunPlanErrorVersionConflict,
		Message:        "base_version 1 does not match current version 3",
		Retryable:      true,
		LatestVersion:  3,
		LatestSnapshot: &latest,
	}}
	outcome, err := (&Tool{}).ExecuteOutcome(ctxWithService(service), validInput, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome: %v", err)
	}
	if !outcome.IsError() || outcome.ErrorCode != "plan_version_conflict" || !outcome.Retryable {
		t.Fatalf("outcome = %+v", outcome)
	}
	if outcome.Diagnostic["latest_version"] != uint64(3) {
		t.Fatalf("diagnostic = %+v", outcome.Diagnostic)
	}
	snapshot, ok := outcome.Diagnostic["latest_snapshot"].(tool.RunPlanSnapshot)
	if !ok || snapshot.PlanID != "plan_tool" || len(snapshot.Items) != 1 {
		t.Fatalf("bounded snapshot = %+v", outcome.Diagnostic["latest_snapshot"])
	}
	// The model payload must carry the conflict facts too.
	payload := outcome.ModelPayload()
	if !strings.Contains(payload, "plan_version_conflict") || !strings.Contains(payload, "latest_version") {
		t.Fatalf("model payload = %q", payload)
	}
}

// RP-G07 (tool level): journal persistence failures are retryable.
func TestUpdatePlanPersistenceFailureOutcome(t *testing.T) {
	service := &fakeRunPlanService{err: &tool.RunPlanError{
		Code: tool.RunPlanErrorPersistenceFailed, Message: "disk full", Retryable: true,
	}}
	outcome, err := (&Tool{}).ExecuteOutcome(ctxWithService(service), validInput, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome: %v", err)
	}
	if !outcome.IsError() || outcome.ErrorCode != "plan_persistence_failed" ||
		outcome.ErrorCategory != tool.ErrorCategoryPersistence || !outcome.Retryable {
		t.Fatalf("outcome = %+v", outcome)
	}
	// Untyped service errors also degrade to a retryable persistence failure.
	service.err = errors.New("boom")
	outcome, err = (&Tool{}).ExecuteOutcome(ctxWithService(service), validInput, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome: %v", err)
	}
	if !outcome.IsError() || outcome.ErrorCode != "plan_persistence_failed" || !outcome.Retryable {
		t.Fatalf("untyped error outcome = %+v", outcome)
	}
}

// Child degradation path: without a bound RunPlanService the tool fails
// closed with plan_service_unavailable instead of panicking.
func TestUpdatePlanServiceUnavailable(t *testing.T) {
	outcome, err := (&Tool{}).ExecuteOutcome(context.Background(), validInput, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome: %v", err)
	}
	if !outcome.IsError() || outcome.ErrorCode != "plan_service_unavailable" ||
		outcome.ErrorCategory != tool.ErrorCategoryUnsupported || outcome.Retryable {
		t.Fatalf("outcome = %+v", outcome)
	}
}

// The definition must carry the §5.1 usage boundaries and the §6.3 model
// constraints, and the tool must be registered for the root tool pool.
func TestUpdatePlanDefinitionContract(t *testing.T) {
	registered, err := tool.GetRegisteredTool(ToolName)
	if err != nil {
		t.Fatalf("update_plan not registered: %v", err)
	}
	definition := registered.Runtime.Definition()
	if definition.Name != ToolName || definition.InputSchema.AdditionalProperties == nil ||
		*definition.InputSchema.AdditionalProperties {
		t.Fatalf("definition = %+v", definition)
	}
	if len(definition.InputSchema.Required) != 1 || definition.InputSchema.Required[0] != "plan" {
		t.Fatalf("required = %+v", definition.InputSchema.Required)
	}
	joined := definition.Description + " " + definition.Purpose + " " +
		strings.Join(definition.WhenToUse, " ") + " " + strings.Join(definition.WhenNotToUse, " ")
	for _, want := range []string{"in_progress", "explanation", "3 or more"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("definition missing %q: %q", want, joined)
		}
	}
	codes := map[string]bool{}
	for _, mode := range definition.FailureModes {
		codes[mode.Code] = true
	}
	for _, want := range []string{"plan_invalid_input", "plan_version_conflict", "plan_persistence_failed", "plan_service_unavailable"} {
		if !codes[want] {
			t.Fatalf("failure modes missing %q: %+v", want, definition.FailureModes)
		}
	}
}
