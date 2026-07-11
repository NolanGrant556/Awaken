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

func TestNormalizeOutcomeV2UsesOnlyObservedChanges(t *testing.T) {
	started := time.Date(2026, 7, 11, 1, 2, 3, 0, time.UTC)
	outcome := SucceededOutcome("updated")
	outcome.StartedAt = started
	outcome.FinishedAt = started.Add(12 * time.Millisecond)
	outcome = outcome.Normalize(OutcomeNormalizationContext{
		OperationType:       "text_edit",
		ActualAffectedPaths: []string{"src/tool/outcome.go"},
		PatchSummary:        "M src/tool/outcome.go",
		WorkspaceRoot:       "/workspace",
		WorkingDirectory:    "/workspace",
		ObservationSource:   "executor",
	})

	if outcome.SchemaVersion != OutcomeSchemaVersion || outcome.Operation == nil {
		t.Fatalf("missing v2 contract: %+v", outcome)
	}
	if outcome.Operation.Type != "text_edit" || outcome.Operation.Target != "src/tool/outcome.go" || outcome.Operation.SideEffectState != OutcomeSideEffectApplied {
		t.Fatalf("unexpected operation: %+v", outcome.Operation)
	}
	if len(outcome.Changes) != 1 || outcome.Changes[0].Certainty != ChangeObserved || outcome.Changes[0].Reversibility != ChangeConditional {
		t.Fatalf("expected one observed change: %+v", outcome.Changes)
	}
	if outcome.Environment == nil || outcome.Environment.Observation != ObservationPartial || outcome.Environment.Source != "executor" {
		t.Fatalf("missing observation provenance: %+v", outcome.Environment)
	}
	if len(outcome.Environment.Unobserved) == 0 {
		t.Fatalf("unobserved dimensions must be explicit: %+v", outcome.Environment)
	}
}

func TestNormalizeSuccessfulUnobservedMutationIsUnknown(t *testing.T) {
	outcome := SucceededOutcome("command completed").Normalize(OutcomeNormalizationContext{
		OperationType: "shell",
		DeclaredSideEffects: []ToolSideEffect{{
			Kind: SideEffectExecute, Description: "May execute a state-changing command.", Reversibility: ReversibilityUnknown,
		}},
	})
	if outcome.Operation == nil || outcome.Operation.SideEffectState != OutcomeSideEffectUnknown {
		t.Fatalf("unobserved mutation-capable success must remain unknown: %+v", outcome.Operation)
	}

	// Model projection must not re-normalize a canonical result with an empty
	// context and turn its conservative uncertainty into a false no-op claim.
	var payload map[string]any
	if err := json.Unmarshal([]byte(outcome.ModelPayload()), &payload); err != nil {
		t.Fatalf("decode model payload: %v", err)
	}
	operation, ok := payload["operation"].(map[string]any)
	if !ok || operation["side_effect_state"] != string(OutcomeSideEffectUnknown) {
		t.Fatalf("model payload lost canonical mutation uncertainty: %+v", payload["operation"])
	}
	if outcome.Operation.SideEffectState != OutcomeSideEffectUnknown {
		t.Fatalf("model projection mutated canonical outcome: %+v", outcome.Operation)
	}
}

func TestNormalizeStrictOutputContractFailsAtVerificationStage(t *testing.T) {
	outcome := SucceededOutcome("wrote file")
	outcome.AffectedPaths = []string{"report.md"}
	outcome.Details = map[string]any{"undeclared": true}
	outcome = outcome.Normalize(OutcomeNormalizationContext{
		OperationType: "text_file_create",
		DeclaredSideEffects: []ToolSideEffect{{
			Kind: SideEffectCreate, Description: "Creates a file.", Reversibility: ReversibilityConditional,
		}},
		OutputSchema: ToolParam{
			Type: "object", Properties: map[string]ToolParam{"path": {Type: "string"}}, AdditionalProperties: BoolPtr(false),
		},
		StrictOutputSchema: true,
	})
	if outcome.Status != OutcomeFailed || outcome.ErrorCode != "tool_output_schema_mismatch" {
		t.Fatalf("strict output mismatch did not fail closed: %+v", outcome)
	}
	if len(outcome.Errors) == 0 || outcome.Errors[0].Stage != ErrorStageVerification || outcome.Retryable {
		t.Fatalf("output mismatch lost verification/recovery facts: %+v", outcome.Errors)
	}
	if outcome.Operation == nil || outcome.Operation.SideEffectState != OutcomeSideEffectPartial {
		t.Fatalf("completed side effect with invalid feedback must be partial: %+v", outcome.Operation)
	}
}

func TestNormalizeInterruptedObservedMutationIsPartial(t *testing.T) {
	outcome := ExecutionOutcome{
		Content: "interrupted after first write", Status: OutcomeInterrupted,
		TransportStatus: TransportCancelled, SemanticStatus: SemanticFailure,
		ProcessStatus: ProcessSignaled,
		Changes: []ResourceChange{{
			Kind: "file", Action: "updated", Resource: "partial.txt",
			Certainty: ChangeObserved, Reversibility: ChangeConditional,
		}},
	}.Normalize(OutcomeNormalizationContext{OperationType: "text_edit"})
	if outcome.Operation == nil || outcome.Operation.SideEffectState != OutcomeSideEffectPartial {
		t.Fatalf("interrupted observed mutation must be partial: %+v", outcome.Operation)
	}
}

func TestNormalizeFailedAffectedPathIsTargetNotObservedMutation(t *testing.T) {
	outcome := FailedOutcome("file changed after read", "stale_file_revision", ErrorCategoryDomain, true)
	outcome.AffectedPaths = []string{"src/example.go"}
	outcome = outcome.Normalize(OutcomeNormalizationContext{OperationType: "text_edit"})

	if len(outcome.Changes) != 0 {
		t.Fatalf("failed legacy affected path must not become a mutation: %+v", outcome.Changes)
	}
	if outcome.Operation == nil || outcome.Operation.Target != "src/example.go" {
		t.Fatalf("failed target should remain available: %+v", outcome.Operation)
	}
	if outcome.Operation.SideEffectState != OutcomeSideEffectUnknown {
		t.Fatalf("unverified failed side effect must be unknown, got %q", outcome.Operation.SideEffectState)
	}
	if outcome.Environment == nil || len(outcome.Environment.Files) != 1 || outcome.Environment.Files[0] != "src/example.go" {
		t.Fatalf("failed target should remain environment context: %+v", outcome.Environment)
	}
	if len(outcome.Errors) == 0 || outcome.Errors[0].RetryStrategy != RetryRefreshState || len(outcome.NextActions) == 0 {
		t.Fatalf("retry guidance missing: errors=%+v next=%+v", outcome.Errors, outcome.NextActions)
	}
}

func TestValidationFailureChangesInputWithoutBlindReplay(t *testing.T) {
	outcome := FailedOutcome("path is required", ErrorCodeValidationError, ErrorCategoryValidation, false)
	outcome.RecoveryOptions = []string{"provide_path"}
	outcome = outcome.Normalize(OutcomeNormalizationContext{OperationType: "read_file"})

	if outcome.Retryable {
		t.Fatal("validation failure must not advertise replaying the same call")
	}
	if len(outcome.Errors) != 1 || outcome.Errors[0].RetryStrategy != RetryChangeInput {
		t.Fatalf("validation failure must recommend changed input: %+v", outcome.Errors)
	}
	if len(outcome.NextActions) != 1 || outcome.NextActions[0].ID != "correct_input" {
		t.Fatalf("validation failure must provide an actionable repair: %+v", outcome.NextActions)
	}
}

func TestModelPayloadV2RedactsDetailsAndOmitsArtifactContent(t *testing.T) {
	outcome := SucceededOutcome("created")
	outcome.Details = map[string]any{
		"api_key": "do-not-leak",
		"nested":  map[string]any{"access_token": "also-secret", "count": 2},
	}
	outcome.Artifacts = []ArtifactDescriptor{{
		ID: "a1", Path: "/workspace/report.md", Kind: "markdown", Content: "private preview", Status: "ready",
	}}
	payload := outcome.ModelPayload()
	if strings.Contains(payload, "do-not-leak") || strings.Contains(payload, "also-secret") || strings.Contains(payload, "private preview") {
		t.Fatalf("model payload leaked secret-bearing data: %s", payload)
	}
	if !strings.Contains(payload, `"api_key":"[REDACTED]"`) || !strings.Contains(payload, `"artifacts"`) {
		t.Fatalf("expected redacted v2 payload: %s", payload)
	}
}

func TestNormalizeNeedsAuthPreservesRetryGuidance(t *testing.T) {
	now := time.Now().UTC()
	outcome := ExecutionOutcome{
		Content: "authorization required", Status: OutcomeNeedsAuth,
		TransportStatus: TransportNotStarted, SemanticStatus: SemanticFailure,
		ProcessStatus: ProcessNotApplicable, ErrorCode: ErrorCodeNeedsAuth,
		ErrorCategory: ErrorCategoryAuthorization, ErrorMessage: "authorization required",
		Retryable: true, StartedAt: now, FinishedAt: now,
	}.Normalize(OutcomeNormalizationContext{OperationType: "mail_send"})
	if !outcome.Retryable || len(outcome.Errors) == 0 || outcome.Errors[0].RetryStrategy != RetryRequestAuthorization {
		t.Fatalf("needs_auth retry facts lost: %+v", outcome)
	}
	if len(outcome.NextActions) != 1 || outcome.Operation.SideEffectState != OutcomeSideEffectNotStarted {
		t.Fatalf("needs_auth recovery guidance missing: %+v", outcome)
	}
}

func TestObservedTrashedPathsUsesTypedChanges(t *testing.T) {
	outcome := ExecutionOutcome{Changes: []ResourceChange{{
		Kind: "file", Action: "trashed", Resource: "old.txt", Certainty: ChangeObserved,
		Reversibility: ChangeConditional, Details: map[string]any{"trashed_path": "/Trash/old.txt"},
	}}}
	paths := outcome.ObservedTrashedPaths()
	if len(paths) != 1 || paths[0] != "/Trash/old.txt" {
		t.Fatalf("typed trash destination not extracted: %v", paths)
	}
}

func TestModelPayloadV2Golden(t *testing.T) {
	observedAt := time.Date(2026, 7, 11, 4, 5, 6, 0, time.UTC)
	outcome := ExecutionOutcome{
		SchemaVersion:   OutcomeSchemaVersion,
		Content:         "updated",
		Summary:         "Updated config.json",
		Status:          OutcomeSucceeded,
		TransportStatus: TransportCompleted,
		SemanticStatus:  SemanticSuccess,
		ProcessStatus:   ProcessNotApplicable,
		Operation: &OperationDescriptor{
			Type: "text_edit", Target: "config.json", SideEffectState: OutcomeSideEffectApplied,
		},
		Changes: []ResourceChange{{
			Kind: "file", Action: "updated", Resource: "config.json",
			Certainty: ChangeObserved, Reversibility: ChangeConditional,
		}},
		Environment: &EnvironmentObservation{
			Observation: ObservationPartial, ObservedAt: observedAt,
			Scope: "/workspace", Source: "executor", WorkspaceRoot: "/workspace",
			Files: []string{"config.json"}, Unobserved: []string{"git", "processes"},
		},
		DurationMs: 9,
	}
	got := outcome.ModelPayload()
	want := `{"changes":[{"action":"updated","certainty":"observed","kind":"file","resource":"config.json","reversibility":"conditional"}],"content":"updated","duration_ms":9,"environment":{"files":["config.json"],"observation":"partial","observed_at":"2026-07-11T04:05:06Z","scope":"/workspace","source":"executor","unobserved":["git","processes","environment_variables"],"workspace_root":"/workspace"},"operation":{"type":"text_edit","target":"config.json","side_effect_state":"applied"},"process_status":"not_applicable","retryable":false,"schema_version":2,"semantic_status":"success","status":"succeeded","summary":"Updated config.json","transport_status":"completed","truncated":false}`
	if got != want {
		t.Fatalf("model payload contract drifted\nwant: %s\n got: %s", want, got)
	}
}

func TestNormalizeRejectsImpossibleSucceededTransportCombination(t *testing.T) {
	outcome := ExecutionOutcome{
		Content: "claimed success", Status: OutcomeSucceeded,
		TransportStatus: TransportFailed, SemanticStatus: SemanticSuccess,
		ProcessStatus: ProcessNotApplicable,
	}.Normalize(OutcomeNormalizationContext{OperationType: "shell"})
	if outcome.Status != OutcomeFailed || outcome.ErrorCode != "invalid_outcome_combination" {
		t.Fatalf("contradictory success must fail closed: %+v", outcome)
	}
	if outcome.Operation.SideEffectState != OutcomeSideEffectUnknown {
		t.Fatalf("contradictory success side effects must remain unknown: %+v", outcome.Operation)
	}
}
