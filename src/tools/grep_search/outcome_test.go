package grepsearch

import (
	"context"
	"testing"

	"awaken/src/tool"
)

func TestExecuteOutcomeReportsSearchState(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "a.txt", "hit one\nhit two\n")
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))

	outcome, err := (&GrepSearchTool{}).ExecuteOutcome(ctx, `{"query":"hit","max_results":1}`, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeSucceeded || !outcome.Truncated {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
	if outcome.Diagnostic["result_count"] != 1 || outcome.Diagnostic["output_mode"] != "content" {
		t.Fatalf("missing search diagnostics: %+v", outcome.Diagnostic)
	}
	if len(outcome.AffectedPaths) != 0 {
		t.Fatalf("read-only search must not claim mutations: %+v", outcome.AffectedPaths)
	}
}

func TestExecuteOutcomeReturnsTypedRegexFailure(t *testing.T) {
	outcome, err := (&GrepSearchTool{}).ExecuteOutcome(context.Background(), `{"query":"["}`, nil)
	if err != nil {
		t.Fatalf("validation must be an outcome, got transport error: %v", err)
	}
	if outcome.ErrorCode != "grep_search_invalid_regex" || outcome.ErrorCategory != tool.ErrorCategoryValidation || outcome.Retryable {
		t.Fatalf("unexpected regex outcome: %+v", outcome)
	}
	if len(outcome.RecoveryOptions) < 2 {
		t.Fatalf("expected regex recovery options: %+v", outcome.RecoveryOptions)
	}
}
