package listfiles

import (
	"context"
	"testing"

	"awaken/src/tool"
)

func TestExecuteOutcomeReportsCountsAndTruncation(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "a.go", "package a")
	writeTestFile(t, root, "b.go", "package b")
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))

	outcome, err := (&ListFilesTool{}).ExecuteOutcome(ctx, `{"pattern":"*.go","max_results":1}`, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeSucceeded || !outcome.Truncated {
		t.Fatalf("unexpected outcome state: %+v", outcome)
	}
	if outcome.Diagnostic["result_count"] != 1 || outcome.Diagnostic["total_matches"] != 2 {
		t.Fatalf("missing count diagnostics: %+v", outcome.Diagnostic)
	}
	if len(outcome.AffectedPaths) != 0 {
		t.Fatalf("read-only list must not claim mutations: %+v", outcome.AffectedPaths)
	}
}

func TestExecuteOutcomeReturnsTypedInputFailure(t *testing.T) {
	outcome, err := (&ListFilesTool{}).ExecuteOutcome(context.Background(), `{`, nil)
	if err != nil {
		t.Fatalf("validation must be an outcome, got transport error: %v", err)
	}
	if outcome.ErrorCode != "list_files_invalid_input" || outcome.ErrorCategory != tool.ErrorCategoryValidation || outcome.Retryable {
		t.Fatalf("unexpected validation outcome: %+v", outcome)
	}
	if len(outcome.RecoveryOptions) == 0 {
		t.Fatalf("expected actionable recovery: %+v", outcome)
	}
}
