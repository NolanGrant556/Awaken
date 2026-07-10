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

func TestTextEditReplaceAllThreeOccurrences(t *testing.T) {
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

func TestTextEditStaleDigest(t *testing.T) {
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
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != string(original) {
		t.Fatalf("file corrupted on stale digest: %q", string(data))
	}
}

func TestTextEditMultiMatchWithoutReplaceAll(t *testing.T) {
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

func TestTextEditNotFoundAndValidation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
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

	outcome, err = (&TextEditTool{}).ExecuteOutcome(ctx, `{"path":"sample.txt","old_string":"hello","new_string":"hello"}`, nil)
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if outcome.ErrorCode != ErrorCodeValidationError {
		t.Fatalf("expected validation_error for old==new, got %+v", outcome)
	}
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
