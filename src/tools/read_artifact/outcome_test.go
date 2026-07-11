package readartifact

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"awaken/src/tool"
)

func TestExecuteOutcomeReportsArtifactPreviewFacts(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "report.csv")
	if err := os.WriteFile(path, []byte("name,amount\nAlice,10\nBob,20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))

	outcome, err := (&ReadArtifactTool{}).ExecuteOutcome(ctx, `{"path":"report.csv","max_rows":1}`, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeSucceeded || !outcome.Truncated {
		t.Fatalf("unexpected preview outcome: %+v", outcome)
	}
	if outcome.Diagnostic["kind"] != "csv" || outcome.Diagnostic["requested_path"] != "report.csv" {
		t.Fatalf("missing artifact diagnostics: %+v", outcome.Diagnostic)
	}
	if len(outcome.AffectedPaths) != 0 {
		t.Fatalf("read-only preview must not claim mutations: %+v", outcome.AffectedPaths)
	}
}

func TestExecuteOutcomeReturnsTypedUnsupportedType(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "image.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))

	outcome, err := (&ReadArtifactTool{}).ExecuteOutcome(ctx, `{"path":"image.png"}`, nil)
	if err != nil {
		t.Fatalf("domain failure must be an outcome, got transport error: %v", err)
	}
	if outcome.ErrorCode != "read_artifact_unsupported_type" || outcome.ErrorCategory != tool.ErrorCategoryDomain || outcome.Retryable {
		t.Fatalf("unexpected unsupported outcome: %+v", outcome)
	}
	if len(outcome.RecoveryOptions) < 2 {
		t.Fatalf("expected routing recovery: %+v", outcome.RecoveryOptions)
	}
}

func TestExecuteOutcomeReturnsTypedEmptyPDF(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "empty.pdf"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))

	outcome, err := (&ReadArtifactTool{}).ExecuteOutcome(ctx, `{"path":"empty.pdf"}`, nil)
	if err != nil {
		t.Fatalf("domain failure must be an outcome, got transport error: %v", err)
	}
	if outcome.ErrorCode != "read_artifact_empty_pdf" || outcome.ErrorCategory != tool.ErrorCategoryDomain {
		t.Fatalf("unexpected empty-PDF outcome: %+v", outcome)
	}
}
