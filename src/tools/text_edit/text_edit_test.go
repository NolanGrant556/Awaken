package textedit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestA07TextEditUniqueMatchCarriesDigestPreviewAndArtifactContract(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(path, []byte("old\nkeep\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	permissionContext := tool.NewPermissionContext(root, []string{root})
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	input := `{"path":"sample.txt","old_string":"old","new_string":"new"}`
	preview, ok := (&TextEditTool{}).PatchPreview(input, permissionContext)
	if !ok || preview.Patch == "" || preview.Summary == "" || len(preview.AffectedPaths) != 1 || preview.AffectedPaths[0] != "sample.txt" {
		t.Fatalf("A07 preview contract incomplete: %+v ok=%v", preview, ok)
	}
	outcome, err := (&TextEditTool{}).ExecuteOutcome(ctx, input, nil)
	if err != nil || outcome.IsError() {
		t.Fatalf("A07 unique edit failed: outcome=%+v err=%v", outcome, err)
	}
	beforeDigest, _ := outcome.Diagnostic["before_sha256"].(string)
	afterDigest, _ := outcome.Diagnostic["after_sha256"].(string)
	if beforeDigest == "" || afterDigest == "" || beforeDigest == afterDigest || outcome.Diagnostic["sha256"] != afterDigest {
		t.Fatalf("A07 digest contract incomplete: %+v", outcome.Diagnostic)
	}
	if len(outcome.Artifacts) != 1 || outcome.Artifacts[0].Path != "sample.txt" || outcome.Artifacts[0].Status != "ready" {
		t.Fatalf("A07 outcome artifact contract incomplete: %+v", outcome.Artifacts)
	}
	if outcome.Artifacts[0].Role != tool.ArtifactRoleSupport {
		t.Fatalf("plain-text source edit must stay support: %+v", outcome.Artifacts)
	}
	produced := (&TextEditTool{}).Artifacts(input, outcome.Content, permissionContext)
	if len(produced) != 1 || produced[0].ID != outcome.Artifacts[0].ID || produced[0].Path != outcome.Artifacts[0].Path {
		t.Fatalf("A07 artifact producer diverged from outcome: outcome=%+v produced=%+v", outcome.Artifacts, produced)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "new\nkeep\n" {
		t.Fatalf("A07 file result mismatch: %q err=%v", content, err)
	}
}

func TestTextEditPublishesModifiedMarkdownDocument(t *testing.T) {
	artifacts := buildTextEditArtifacts("report.md", 1)
	if len(artifacts) != 1 || artifacts[0].Role != tool.ArtifactRoleDeliverable {
		t.Fatalf("modified Markdown document must be deliverable: %+v", artifacts)
	}
}

func TestA11TextEditReplaceAllOnlyWhenExplicit(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	original := "foo\nbar\nfoo\nbaz\nfoo\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))

	outcome, err := (&TextEditTool{}).ExecuteOutcome(ctx, `{"path":"sample.txt","old_string":"foo","new_string":"qux","replace_all":true}`, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome transport error: %v", err)
	}
	if outcome.IsError() {
		t.Fatalf("expected success, got %+v", outcome)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := "qux\nbar\nqux\nbaz\nqux\n"
	if string(data) != want {
		t.Fatalf("content=%q want %q", string(data), want)
	}
	var result textEditResult
	if err := json.Unmarshal([]byte(outcome.Content), &result); err != nil {
		t.Fatalf("parse content: %v", err)
	}
	if result.ReplacementCount != 3 {
		t.Fatalf("replacement_count=%d want 3", result.ReplacementCount)
	}
	if result.BeforeSHA256 == "" || result.AfterSHA256 == "" || result.BeforeSHA256 == result.AfterSHA256 {
		t.Fatalf("expected distinct digests, got before=%s after=%s", result.BeforeSHA256, result.AfterSHA256)
	}
	if !strings.Contains(result.UnifiedDiff, "-foo") || !strings.Contains(result.UnifiedDiff, "+qux") {
		t.Fatalf("expected unified diff, got %q", result.UnifiedDiff)
	}
}

func TestA10TextEditStaleDigestLeavesFileUnchanged(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	original := []byte("alpha\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	input := `{"path":"sample.txt","old_string":"alpha","new_string":"beta","expected_sha256":"deadbeef"}`
	outcome, err := (&TextEditTool{}).ExecuteOutcome(ctx, input, nil)
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if !outcome.IsError() || outcome.ErrorCode != ErrorCodeStaleFileRevision {
		t.Fatalf("expected stale_file_revision, got %+v", outcome)
	}
	if !containsString(outcome.RecoveryOptions, "read_file") ||
		!containsString(outcome.RecoveryOptions, "retry_text_edit_with_fresh_digest") {
		t.Fatalf("stale digest recovery is not actionable: %+v", outcome.RecoveryOptions)
	}
	if outcome.Diagnostic["before_sha256"] != fileDigest(original) || outcome.Diagnostic["expected_sha256"] != "deadbeef" {
		t.Fatalf("stale digest diagnostic lost actual/expected revisions: %+v", outcome.Diagnostic)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != string(original) {
		t.Fatalf("file corrupted on stale digest: %q", string(data))
	}
}

func TestA09TextEditMultiMatchWithoutReplaceAllLeavesFileUnchanged(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	original := "x\nx\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	outcome, err := (&TextEditTool{}).ExecuteOutcome(ctx, `{"path":"sample.txt","old_string":"x","new_string":"y"}`, nil)
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if !outcome.IsError() || outcome.ErrorCode != ErrorCodeTextNotUnique {
		t.Fatalf("expected text_not_unique, got %+v", outcome)
	}
	if !containsString(outcome.RecoveryOptions, "set_replace_all_true") || outcome.Diagnostic["replace_all"] != false {
		t.Fatalf("multi-match failure must require an explicit replace_all decision: %+v", outcome)
	}
	data, _ := os.ReadFile(path)
	if string(data) != original {
		t.Fatalf("file changed on multi-match failure: %q", string(data))
	}
}

func TestTextEditAtomicWriteFailurePreservesOriginal(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	original := "keep-me\n"
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))

	prev := atomicWriteFile
	atomicWriteFile = func(p string, content []byte, mode os.FileMode) error {
		return fmt.Errorf("simulated atomic write failure")
	}
	t.Cleanup(func() { atomicWriteFile = prev })

	outcome, err := (&TextEditTool{}).ExecuteOutcome(ctx, `{"path":"sample.txt","old_string":"keep-me","new_string":"changed"}`, nil)
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if !outcome.IsError() {
		t.Fatalf("expected failure, got success: %+v", outcome)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != original {
		t.Fatalf("original corrupted after atomic failure: %q", string(data))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("permissions changed: %o", info.Mode().Perm())
	}
}

func TestA08TextEditNotFoundLeavesFileUnchangedAndValidation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	original := []byte("hello\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))

	outcome, err := (&TextEditTool{}).ExecuteOutcome(ctx, `{"path":"sample.txt","old_string":"missing","new_string":"x"}`, nil)
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if outcome.ErrorCode != ErrorCodeTextNotFound {
		t.Fatalf("expected text_not_found, got %+v", outcome)
	}
	if !containsString(outcome.RecoveryOptions, "read_file") ||
		!containsString(outcome.RecoveryOptions, "widen_or_correct_old_string") {
		t.Fatalf("text_not_found recovery is not actionable: %+v", outcome.RecoveryOptions)
	}
	afterNotFound, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after text_not_found: %v", err)
	}
	if string(afterNotFound) != string(original) {
		t.Fatalf("A08 text_not_found mutated the file: %q", afterNotFound)
	}

	outcome, err = (&TextEditTool{}).ExecuteOutcome(ctx, `{"path":"sample.txt","old_string":"hello","new_string":"hello"}`, nil)
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if outcome.ErrorCode != ErrorCodeValidationError {
		t.Fatalf("expected validation_error for old==new, got %+v", outcome)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestTextEditExpectedDigestSuccess(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	original := []byte("one\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	sum := sha256.Sum256(original)
	digest := hex.EncodeToString(sum[:])
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	input := fmt.Sprintf(`{"path":"sample.txt","old_string":"one","new_string":"two","expected_sha256":%q}`, digest)
	outcome, err := (&TextEditTool{}).ExecuteOutcome(ctx, input, nil)
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if outcome.IsError() {
		t.Fatalf("expected success, got %+v", outcome)
	}
}

func TestTextEditImplementsOptionalInterfaces(t *testing.T) {
	var _ tool.OutcomeTool = (*TextEditTool)(nil)
	var _ tool.ArtifactProducingTool = (*TextEditTool)(nil)
	var _ tool.PatchPreviewingTool = (*TextEditTool)(nil)
	var _ tool.ActivityDescribingTool = (*TextEditTool)(nil)

	preview, ok := (&TextEditTool{}).PatchPreview(`{"path":"a.txt","old_string":"a","new_string":"b"}`, tool.NewPermissionContext("/tmp", []string{"/tmp"}))
	if !ok || len(preview.AffectedPaths) != 1 {
		t.Fatalf("unexpected preview: %+v ok=%v", preview, ok)
	}
	activity := (&TextEditTool{}).DescribeActivity(`{"path":"a.txt","old_string":"a","new_string":"b"}`)
	if activity.Title == "" {
		t.Fatal("expected activity title")
	}
}

func TestTextEditDeniesOutsideWritableRoots(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	decision := (&TextEditTool{}).CheckPermission(
		context.Background(),
		fmt.Sprintf(`{"path":%q,"old_string":"a","new_string":"b"}`, filepath.Join(outside, "x.txt")),
		tool.NewPermissionContext(root, []string{root}),
	)
	if decision.Behavior != tool.PermissionDeny {
		t.Fatalf("expected deny, got %+v", decision)
	}
}
