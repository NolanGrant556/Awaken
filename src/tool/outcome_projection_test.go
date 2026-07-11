package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSafeModelValueHandlesTypedContainersDepthAndCycles(t *testing.T) {
	typed := map[string]any{
		"typed_map": map[string]string{
			"api_key": "typed-map-secret",
			"label":   "visible",
		},
		"typed_slice": []map[string]string{{
			"access_token": "typed-slice-secret",
			"state":        "ready",
		}},
	}
	cycle := map[string]any{}
	cycle["self"] = cycle
	typed["cycle"] = cycle
	typed["function"] = func() {}
	typed["channel"] = make(chan int)

	deep := map[string]any{"leaf": "value"}
	for i := 0; i < modelProjectionMaxDepth+8; i++ {
		deep = map[string]any{"next": deep}
	}
	typed["deep"] = deep

	encoded, err := json.Marshal(safeModelValue(typed))
	if err != nil {
		t.Fatalf("safe projection must remain valid JSON: %v", err)
	}
	text := string(encoded)
	for _, secret := range []string{"typed-map-secret", "typed-slice-secret"} {
		if strings.Contains(text, secret) {
			t.Fatalf("typed container leaked %q: %s", secret, text)
		}
	}
	for _, marker := range []string{"[REDACTED]", "[CIRCULAR_REFERENCE]", "[MAX_DEPTH_EXCEEDED]", "[UNSUPPORTED_TYPE:func()]", "[UNSUPPORTED_TYPE:chan int]"} {
		if !strings.Contains(text, marker) {
			t.Fatalf("expected marker %q in %s", marker, text)
		}
	}
	if !strings.Contains(text, "visible") || !strings.Contains(text, "ready") {
		t.Fatalf("non-sensitive typed values were lost: %s", text)
	}
}

func TestModelPayloadBudgetsEveryStructuredResultChannel(t *testing.T) {
	huge := strings.Repeat("large-structured-value-密", modelStructuredFieldBudgetBytes)
	outcome := SucceededOutcome("ok")
	outcome.Diagnostic = map[string]any{"structured_content": map[string]string{"payload": huge}}
	outcome.Details = map[string]any{"provider_details": []string{huge}}
	outcome.Changes = []ResourceChange{{
		Kind: "remote_record", Action: "updated", Resource: "record-1",
		Certainty: ChangeObserved, Reversibility: ChangeReversibilityUnknown,
		Details: map[string]any{"response": huge},
	}}
	outcome.NextActions = []NextAction{{
		ID: "continue", Description: "Continue from structured output", Tool: "connector_next",
		InputHint: map[string]any{"cursor": huge},
	}}

	payload := outcome.ModelPayload()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("budgeted payload must remain valid JSON: %v\n%s", err, payload)
	}
	if truncated, _ := decoded["truncated"].(bool); !truncated {
		t.Fatalf("top-level payload must report projection truncation: %s", payload)
	}
	for _, key := range []string{"diagnostic", "details", "changes", "next_actions"} {
		value, ok := decoded[key].(map[string]any)
		if !ok {
			t.Fatalf("%s should be an explicit truncation envelope, got %#v", key, decoded[key])
		}
		if truncated, _ := value["truncated"].(bool); !truncated {
			t.Fatalf("%s missing truncated=true: %#v", key, value)
		}
		if originalBytes, _ := value["original_bytes"].(float64); originalBytes <= modelStructuredFieldBudgetBytes {
			t.Fatalf("%s original byte count should exceed budget: %#v", key, value)
		}
		if preview, _ := value["preview"].(string); preview == "" {
			t.Fatalf("%s should retain a bounded preview: %#v", key, value)
		}
		encodedField, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal %s: %v", key, err)
		}
		if len(encodedField) > modelStructuredFieldBudgetBytes {
			t.Fatalf("%s projection exceeded %d-byte budget: %d", key, modelStructuredFieldBudgetBytes, len(encodedField))
		}
	}
}

func TestModelPayloadBudgetsCanonicalTextFields(t *testing.T) {
	huge := strings.Repeat("large-canonical-error-密", 64*1024)
	outcome := FailedOutcome(huge, "huge_error", ErrorCategoryTool, false)
	outcome.ErrorMessage = huge
	outcome.PatchSummary = huge
	payload := outcome.ModelPayload()
	if len(payload) > 80*1024 {
		t.Fatalf("canonical text bypassed model payload budgets: %d bytes", len(payload))
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("budgeted canonical payload must remain valid JSON: %v", err)
	}
	if truncated, _ := decoded["truncated"].(bool); !truncated {
		t.Fatalf("canonical text truncation was not surfaced: %s", payload)
	}
	for _, key := range []string{"content", "error_message", "patch_summary"} {
		value, _ := decoded[key].(string)
		if value == "" || !strings.Contains(value, "omitted") {
			t.Fatalf("%s did not retain an explicit bounded preview: %q", key, value)
		}
	}
}

func TestCloneExecutionOutcomeCopiesEveryNestedMutableField(t *testing.T) {
	exitCode := 7
	processExitCode := 9
	original := ExecutionOutcome{
		ExitCode:        &exitCode,
		RecoveryOptions: []string{"retry"},
		Diagnostic: map[string]any{
			"nested": map[string]any{"value": "original"},
			"typed":  map[string]string{"state": "original"},
		},
		AffectedPaths: []string{"original.txt"},
		Artifacts:     []ArtifactDescriptor{{Path: "original.md"}},
		Operation:     &OperationDescriptor{Type: "original"},
		Changes: []ResourceChange{{
			Resource: "original.txt", Details: map[string]any{"nested": []string{"original"}},
		}},
		Environment: &EnvironmentObservation{
			Files: []string{"original.txt"},
			Git: &GitObservation{
				ModifiedFiles: []string{"original.txt"}, UntrackedFiles: []string{"new.txt"},
			},
			Processes:  []ProcessObservation{{ExitCode: &processExitCode}},
			Variables:  []EnvironmentVariableChange{{Name: "ORIGINAL"}},
			Unobserved: []string{"git"},
		},
		Warnings: []OutcomeNotice{{Code: "original"}},
		Errors:   []OutcomeError{{PossibleFixes: []string{"original"}}},
		NextActions: []NextAction{{
			Preconditions: []string{"original"},
			InputHint:     map[string]any{"nested": map[string]any{"value": "original"}},
		}},
		Details: map[string]any{"nested": []any{map[string]any{"value": "original"}}},
	}

	clone := cloneExecutionOutcome(original)
	*clone.ExitCode = 1
	clone.RecoveryOptions[0] = "changed"
	clone.Diagnostic["nested"].(map[string]any)["value"] = "changed"
	clone.Diagnostic["typed"].(map[string]any)["state"] = "changed"
	clone.AffectedPaths[0] = "changed.txt"
	clone.Artifacts[0].Path = "changed.md"
	clone.Operation.Type = "changed"
	clone.Changes[0].Details["nested"].([]any)[0] = "changed"
	clone.Environment.Files[0] = "changed.txt"
	clone.Environment.Git.ModifiedFiles[0] = "changed.txt"
	clone.Environment.Git.UntrackedFiles[0] = "changed-new.txt"
	*clone.Environment.Processes[0].ExitCode = 2
	clone.Environment.Variables[0].Name = "CHANGED"
	clone.Environment.Unobserved[0] = "processes"
	clone.Warnings[0].Code = "changed"
	clone.Errors[0].PossibleFixes[0] = "changed"
	clone.NextActions[0].Preconditions[0] = "changed"
	clone.NextActions[0].InputHint["nested"].(map[string]any)["value"] = "changed"
	clone.Details["nested"].([]any)[0].(map[string]any)["value"] = "changed"

	if exitCode != 7 || processExitCode != 9 || original.Operation.Type != "original" {
		t.Fatalf("pointer field leaked through clone: %+v", original)
	}
	if original.RecoveryOptions[0] != "retry" || original.AffectedPaths[0] != "original.txt" || original.Artifacts[0].Path != "original.md" {
		t.Fatalf("top-level slice leaked through clone: %+v", original)
	}
	if original.Diagnostic["nested"].(map[string]any)["value"] != "original" || original.Diagnostic["typed"].(map[string]string)["state"] != "original" {
		t.Fatalf("diagnostic nested value leaked through clone: %#v", original.Diagnostic)
	}
	if original.Changes[0].Details["nested"].([]string)[0] != "original" || original.NextActions[0].InputHint["nested"].(map[string]any)["value"] != "original" {
		t.Fatalf("change/action nested value leaked through clone: changes=%#v actions=%#v", original.Changes, original.NextActions)
	}
	if original.Environment.Files[0] != "original.txt" || original.Environment.Git.ModifiedFiles[0] != "original.txt" || original.Environment.Variables[0].Name != "ORIGINAL" {
		t.Fatalf("environment leaked through clone: %+v", original.Environment)
	}
	if original.Warnings[0].Code != "original" || original.Errors[0].PossibleFixes[0] != "original" || original.Details["nested"].([]any)[0].(map[string]any)["value"] != "original" {
		t.Fatalf("notice/error/details leaked through clone: %+v", original)
	}
}

func TestModelPayloadDoesNotMutateOriginalAndIsConcurrentReadSafe(t *testing.T) {
	observedAt := time.Date(2026, 7, 11, 8, 9, 10, 0, time.UTC)
	outcome := SucceededOutcome("ok")
	outcome.Operation = &OperationDescriptor{}
	outcome.Environment = &EnvironmentObservation{
		ObservedAt: observedAt, Files: []string{"original.txt"}, Git: &GitObservation{ModifiedFiles: []string{"original.txt"}},
	}
	outcome.Details = map[string]any{"nested": map[string]string{"state": "original"}}
	outcome.NextActions = []NextAction{{InputHint: map[string]any{"path": "original.txt"}}}

	before, err := json.Marshal(outcome)
	if err != nil {
		t.Fatalf("marshal before: %v", err)
	}
	expected := outcome.ModelPayload()

	const workers = 16
	var wait sync.WaitGroup
	errors := make(chan string, workers)
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if got := outcome.ModelPayload(); got != expected {
				errors <- got
			}
		}()
	}
	wait.Wait()
	close(errors)
	for got := range errors {
		t.Fatalf("concurrent projection drifted\nwant: %s\n got: %s", expected, got)
	}

	after, err := json.Marshal(outcome)
	if err != nil {
		t.Fatalf("marshal after: %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("ModelPayload mutated its input\nbefore: %s\n after: %s", before, after)
	}
	if outcome.SchemaVersion != 0 || outcome.Operation.Type != "" || outcome.Environment.Observation != "" {
		t.Fatalf("normalization facts leaked into original: %+v", outcome)
	}
}
