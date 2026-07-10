package tool

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExecutionOutcomeJSONTagsAreSnakeCase(t *testing.T) {
	exit := 7
	started := time.Date(2026, 7, 10, 3, 0, 0, 0, time.UTC)
	finished := started.Add(15 * time.Millisecond)
	outcome := ExecutionOutcome{
		Content:         "failed",
		Status:          OutcomeFailed,
		TransportStatus: TransportCompleted,
		SemanticStatus:  SemanticFailure,
		ProcessStatus:   ProcessExited,
		ExitCode:        &exit,
		TimedOut:        false,
		ErrorCode:       ErrorCodeProcessExitNonzero,
		ErrorCategory:   ErrorCategoryProcess,
		ErrorMessage:    "exit 7",
		Retryable:       true,
		RecoveryOptions: []string{"inspect_stderr"},
		Diagnostic:      map[string]any{"reason": "nonzero"},
		AffectedPaths:   []string{"a.txt"},
		PatchSummary:    "M a.txt",
		StartedAt:       started,
		FinishedAt:      finished,
		DurationMs:      15,
		Truncated:       true,
	}
	raw, err := json.Marshal(outcome)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{
		"content", "status", "transport_status", "semantic_status", "process_status",
		"exit_code", "error_code", "error_category", "error_message", "retryable",
		"recovery_options", "diagnostic", "affected_paths", "patch_summary",
		"started_at", "finished_at", "duration_ms", "truncated",
	} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("missing snake_case key %q in %s", key, string(raw))
		}
	}
	startedAt, _ := decoded["started_at"].(string)
	if !strings.Contains(startedAt, "2026-07-10T03:00:00") {
		t.Fatalf("started_at should be RFC3339Nano UTC, got %q", startedAt)
	}
}

func TestIsErrorDerivedFromOutcome(t *testing.T) {
	if SucceededOutcome("ok").IsError() {
		t.Fatal("succeeded must not be error")
	}
	failed := FailedOutcome("x", ErrorCodeValidationError, ErrorCategoryValidation, false)
	if !failed.IsError() {
		t.Fatal("failed must be error")
	}
	neutral := SucceededOutcome("no match")
	neutral.SemanticStatus = SemanticNeutral
	if neutral.IsError() {
		t.Fatal("neutral success must not be error")
	}
	semanticFail := SucceededOutcome("x")
	semanticFail.SemanticStatus = SemanticFailure
	if !semanticFail.IsError() {
		t.Fatal("semantic failure must be error")
	}
}

func TestLegacyOutcomeAdapter(t *testing.T) {
	ok := LegacyOutcomeAdapter("hello", nil)
	if ok.Status != OutcomeSucceeded || ok.IsError() {
		t.Fatalf("unexpected success adapter: %+v", ok)
	}
	bad := LegacyOutcomeAdapter("", errors.New("boom"))
	if bad.Status != OutcomeFailed || !bad.IsError() || bad.ErrorCode != ErrorCodeToolExecutionFailed {
		t.Fatalf("unexpected failure adapter: %+v", bad)
	}
}

func TestModelPayloadStable(t *testing.T) {
	exit := 1
	payload := ExecutionOutcome{
		Content:         `{"exit_code":1}`,
		Status:          OutcomeFailed,
		TransportStatus: TransportCompleted,
		SemanticStatus:  SemanticFailure,
		ProcessStatus:   ProcessExited,
		ExitCode:        &exit,
		ErrorCode:       ErrorCodeProcessExitNonzero,
		ErrorCategory:   ErrorCategoryProcess,
		Retryable:       true,
		DurationMs:      8,
	}.ModelPayload()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("payload json: %v", err)
	}
	if decoded["status"] != string(OutcomeFailed) {
		t.Fatalf("status=%v", decoded["status"])
	}
	if decoded["error_code"] != ErrorCodeProcessExitNonzero {
		t.Fatalf("error_code=%v", decoded["error_code"])
	}
	if decoded["exit_code"].(float64) != 1 {
		t.Fatalf("exit_code=%v", decoded["exit_code"])
	}
}
