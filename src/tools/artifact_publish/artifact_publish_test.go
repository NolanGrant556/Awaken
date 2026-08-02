package artifactpublish

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestArtifactPublishProducesDurableDeliverableWithoutModifyingFile(t *testing.T) {
	root := t.TempDir()
	outputRoot := filepath.Join(root, "outputs")
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(outputRoot, "risk-report.pdf")
	wantContent := []byte("%PDF-1.4\nunchanged")
	if err := os.WriteFile(path, wantContent, 0o644); err != nil {
		t.Fatal(err)
	}
	permissionContext := publishPermissionContext(root, outputRoot)
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	input := `{"path":"outputs/risk-report.pdf","title":"Risk report"}`

	output, err := (&ArtifactPublishTool{}).Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(output, `"status":"published"`) {
		t.Fatalf("unexpected output: %s", output)
	}
	gotContent, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotContent) != string(wantContent) {
		t.Fatalf("artifact_publish modified file: got %q want %q", gotContent, wantContent)
	}

	artifacts := (&ArtifactPublishTool{}).Artifacts(input, output, permissionContext)
	if len(artifacts) != 1 {
		t.Fatalf("expected one artifact, got %+v", artifacts)
	}
	artifact := artifacts[0]
	if artifact.ContractVersion != tool.ArtifactContractVersion || artifact.Role != tool.ArtifactRoleDeliverable || artifact.Producer != "artifact_publish" {
		t.Fatalf("unexpected v2 contract: %+v", artifact)
	}
	if artifact.Action != tool.ArtifactActionPublished {
		t.Fatalf("publishing an existing file must not claim creation: %+v", artifact)
	}
	if artifact.Path != "outputs/risk-report.pdf" || artifact.Kind != "pdf" || artifact.Title != "Risk report" {
		t.Fatalf("unexpected artifact: %+v", artifact)
	}
}

// Output roots are where deliverables are expected to live, not a boundary on
// what may be published, so an artifact the user points at outside them is
// still publishable. The format and symlink checks below carry the real policy.
func TestArtifactPublishAcceptsAFileOutsideTheOutputRoot(t *testing.T) {
	root := t.TempDir()
	outputRoot := filepath.Join(root, "outputs")
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "private.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4\nprivate"), 0o644); err != nil {
		t.Fatal(err)
	}
	permissionContext := publishPermissionContext(root, outputRoot)
	decision := (&ArtifactPublishTool{}).CheckPermission(context.Background(), `{"path":"private.pdf"}`, permissionContext)
	if decision.Behavior == tool.PermissionDeny {
		t.Fatalf("expected the publish to be allowed, got %+v", decision)
	}
}

func TestArtifactPublishRejectsSymlinkEvenWhenTargetStaysInsideOutputRoot(t *testing.T) {
	root := t.TempDir()
	outputRoot := filepath.Join(root, "outputs")
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(outputRoot, "target.pdf")
	if err := os.WriteFile(target, []byte("target"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(outputRoot, "alias.pdf")); err != nil {
		t.Fatal(err)
	}
	permissionContext := publishPermissionContext(root, outputRoot)
	decision := (&ArtifactPublishTool{}).CheckPermission(context.Background(), `{"path":"outputs/alias.pdf"}`, permissionContext)
	if decision.Behavior != tool.PermissionDeny || !strings.Contains(decision.Reason, "not a regular file") {
		t.Fatalf("expected symlink denial, got %+v", decision)
	}
}

func TestArtifactPublishRejectsUnsupportedAndNonRegularPaths(t *testing.T) {
	root := t.TempDir()
	outputRoot := filepath.Join(root, "outputs")
	if err := os.MkdirAll(filepath.Join(outputRoot, "folder.pdf"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outputRoot, "notes.txt"), []byte("notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	permissionContext := publishPermissionContext(root, outputRoot)
	toolInstance := &ArtifactPublishTool{}

	if decision := toolInstance.CheckPermission(context.Background(), `{"path":"outputs/notes.txt"}`, permissionContext); decision.Behavior != tool.PermissionDeny {
		t.Fatalf("expected unsupported extension denial, got %+v", decision)
	}
	if decision := toolInstance.CheckPermission(context.Background(), `{"path":"outputs/folder.pdf"}`, permissionContext); decision.Behavior != tool.PermissionDeny || !strings.Contains(decision.Reason, "not a regular file") {
		t.Fatalf("expected non-regular-file denial, got %+v", decision)
	}
}

func TestArtifactPublishExplicitlyRoutesMediaToDedicatedTools(t *testing.T) {
	root := t.TempDir()
	outputRoot := filepath.Join(root, "outputs")
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	permissionContext := publishPermissionContext(root, outputRoot)
	instance := &ArtifactPublishTool{}
	for _, name := range []string{"image.png", "photo.jpg", "clip.mp4"} {
		path := filepath.Join(outputRoot, name)
		if err := os.WriteFile(path, []byte("media"), 0o644); err != nil {
			t.Fatal(err)
		}
		outcome, err := instance.ExecuteOutcome(
			tool.ContextWithPermissionContext(context.Background(), permissionContext),
			`{"path":"outputs/`+name+`"}`,
			nil,
		)
		if err != nil {
			t.Fatalf("%s: unsupported media must be a typed outcome: %v", name, err)
		}
		if outcome.ErrorCode != "artifact_publish_unsupported_type" ||
			!strings.Contains(outcome.ErrorMessage, "image_generate/video_generate") {
			t.Fatalf("%s: media boundary missing from outcome: %+v", name, outcome)
		}
	}

	modelDescription := tool.ModelDescription(instance.Definition())
	if !strings.Contains(modelDescription, "不支持图片或视频") ||
		!strings.Contains(modelDescription, "不得再次发布") {
		t.Fatalf("model-visible artifact boundary is incomplete: %s", modelDescription)
	}
}

func TestArtifactPublishRejectsDOCXDisguisedAsPDF(t *testing.T) {
	root := t.TempDir()
	outputRoot := filepath.Join(root, "outputs")
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outputRoot, "fake.pdf"), []byte("PK\x03\x04docx"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), publishPermissionContext(root, outputRoot))
	outcome, err := (&ArtifactPublishTool{}).ExecuteOutcome(ctx, `{"path":"outputs/fake.pdf"}`, nil)
	if err != nil {
		t.Fatalf("content mismatch must be a typed outcome: %v", err)
	}
	if outcome.ErrorCode != "artifact_publish_content_mismatch" || outcome.Status != tool.OutcomeFailed {
		t.Fatalf("unexpected mismatch outcome: %+v", outcome)
	}
}

func TestExecuteOutcomePublishesArtifactWithoutClaimingFileMutation(t *testing.T) {
	root := t.TempDir()
	outputRoot := filepath.Join(root, "outputs")
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(outputRoot, "report.pdf")
	want := []byte("%PDF-1.4\nunchanged")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}
	permissionContext := publishPermissionContext(root, outputRoot)
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)

	outcome, err := (&ArtifactPublishTool{}).ExecuteOutcome(ctx, `{"path":"outputs/report.pdf","title":"Report"}`, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeSucceeded || len(outcome.Artifacts) != 1 {
		t.Fatalf("unexpected publication outcome: %+v", outcome)
	}
	if outcome.Diagnostic["publication_status"] != "published" || outcome.Diagnostic["file_modified"] != false {
		t.Fatalf("missing publication facts: %+v", outcome.Diagnostic)
	}
	if len(outcome.AffectedPaths) != 0 {
		t.Fatalf("publish classification must not claim file mutation: %+v", outcome.AffectedPaths)
	}
	normalized := outcome.Normalize(tool.OutcomeNormalizationContext{OperationType: "artifact_publish"})
	if len(normalized.Changes) != 0 {
		t.Fatalf("published deliverable must not become a file/resource mutation: %+v", normalized.Changes)
	}
	if len(normalized.Artifacts) != 1 || normalized.Artifacts[0].Action != tool.ArtifactActionPublished {
		t.Fatalf("published deliverable fact was not preserved: %+v", normalized.Artifacts)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("artifact_publish modified the file: got %q want %q", got, want)
	}
}

func TestExecuteOutcomeReturnsTypedPublicationFailures(t *testing.T) {
	root := t.TempDir()
	outputRoot := filepath.Join(root, "outputs")
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	permissionContext := publishPermissionContext(root, outputRoot)
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	instance := &ArtifactPublishTool{}

	unsupported, err := instance.ExecuteOutcome(ctx, `{"path":"outputs/notes.txt"}`, nil)
	if err != nil {
		t.Fatalf("domain failure must be an outcome: %v", err)
	}
	if unsupported.ErrorCode != "artifact_publish_unsupported_type" || unsupported.ErrorCategory != tool.ErrorCategoryDomain {
		t.Fatalf("unexpected unsupported-type outcome: %+v", unsupported)
	}

	missing, err := instance.ExecuteOutcome(ctx, `{"path":"outputs/missing.pdf"}`, nil)
	if err != nil {
		t.Fatalf("missing-file failure must be an outcome: %v", err)
	}
	if missing.ErrorCode != "artifact_publish_file_unavailable" || !missing.Retryable {
		t.Fatalf("unexpected missing-file outcome: %+v", missing)
	}
	if len(missing.RecoveryOptions) == 0 {
		t.Fatalf("expected actionable recovery: %+v", missing)
	}

	mismatchedPath := filepath.Join(root, "outside.pdf")
	if err := os.WriteFile(mismatchedPath, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	mismatched, err := instance.ExecuteOutcome(ctx, `{"path":"outside.pdf"}`, nil)
	if err != nil {
		t.Fatalf("domain failure must be an outcome: %v", err)
	}
	if mismatched.ErrorCode != "artifact_publish_content_mismatch" || mismatched.ErrorCategory != tool.ErrorCategoryDomain {
		t.Fatalf("unexpected content-mismatch outcome: %+v", mismatched)
	}
}

func publishPermissionContext(root, outputRoot string) tool.PermissionContext {
	return tool.NewTaskScopePermissionContext(
		"scope",
		root,
		[]string{root},
		[]string{root},
		[]string{outputRoot},
		nil,
		nil,
		filepath.Join(root, ".tmp"),
	)
}
