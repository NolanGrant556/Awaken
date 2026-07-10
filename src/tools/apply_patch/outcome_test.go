package applypatch

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestExecuteOutcomeReturnsTypedGrammarFailure(t *testing.T) {
	outcome, err := (&ApplyPatchTool{}).ExecuteOutcome(
		context.Background(),
		"*** Update File: a.txt\n@@\n-old\n+new\n*** End Patch",
		nil,
	)
	if err != nil {
		t.Fatalf("business failure returned Go error: %v", err)
	}
	if outcome.Status != tool.OutcomeFailed || outcome.ErrorCode != ErrorCodePatchMissingBegin {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
	if outcome.Retryable {
		t.Fatal("malformed patch must not be retryable with identical input")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(outcome.ModelPayload()), &payload); err != nil {
		t.Fatalf("model payload is not JSON: %v", err)
	}
	if payload["error_code"] != ErrorCodePatchMissingBegin {
		t.Fatalf("model payload lost typed error: %#v", payload)
	}
}

func TestA12A13MalformedPatchHasTypedRecoveryAndZeroSideEffects(t *testing.T) {
	cases := []struct {
		name         string
		buildPatch   func(path string) string
		wantCode     string
		wantRecovery string
	}{
		{
			name: "A12_missing_begin",
			buildPatch: func(path string) string {
				return "*** Update File: " + path + "\n@@\n-current\n+changed\n*** End Patch"
			},
			wantCode:     ErrorCodePatchMissingBegin,
			wantRecovery: "use_text_edit_or_text_file_create",
		},
		{
			name: "A13_hunk_line_missing_prefix",
			buildPatch: func(path string) string {
				return "*** Begin Patch\n*** Update File: " + path + "\n@@\ncurrent\n+changed\n*** End Patch"
			},
			wantCode:     ErrorCodePatchInvalidLinePrefix,
			wantRecovery: "do_not_replay_same_patch",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "sample.txt")
			if err := os.WriteFile(path, []byte("current\n"), 0o640); err != nil {
				t.Fatalf("seed: %v", err)
			}
			before := snapshotPatchTestWorkspace(t, root)
			ctx := tool.ContextWithPermissionContext(
				context.Background(),
				tool.NewPermissionContext(root, []string{root}),
			)
			outcome, err := (&ApplyPatchTool{}).ExecuteOutcome(ctx, tc.buildPatch(path), nil)
			if err != nil {
				t.Fatalf("business failure returned Go error: %v", err)
			}
			if !outcome.IsError() || outcome.ErrorCode != tc.wantCode || outcome.ErrorCategory != tool.ErrorCategoryValidation {
				t.Fatalf("unexpected typed malformed-patch outcome: %+v", outcome)
			}
			if outcome.Retryable || !sliceContains(outcome.RecoveryOptions, tc.wantRecovery) {
				t.Fatalf("malformed patch recovery contract lost: %+v", outcome)
			}
			after := snapshotPatchTestWorkspace(t, root)
			if !equalStringMaps(before, after) {
				t.Fatalf("malformed patch caused filesystem side effects:\nbefore=%#v\nafter=%#v", before, after)
			}
		})
	}
}

func TestExecuteOutcomeReturnsTypedContextFailureWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("current\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	patch := "*** Begin Patch\n*** Update File: " + path + "\n@@\n-old\n+new\n*** End Patch"
	outcome, err := (&ApplyPatchTool{}).ExecuteOutcome(context.Background(), patch, nil)
	if err != nil {
		t.Fatalf("business failure returned Go error: %v", err)
	}
	if outcome.ErrorCode != ErrorCodePatchContextNotFound || !outcome.IsError() {
		t.Fatalf("unexpected context outcome: %+v", outcome)
	}
	if outcome.Retryable {
		t.Fatal("unchanged patch/context pair must not be directly retryable")
	}
	if outcome.ErrorCode == ErrorCodeInvalidPatchFormat ||
		!sliceContains(outcome.RecoveryOptions, "read_file") ||
		!sliceContains(outcome.RecoveryOptions, "use_text_edit") {
		t.Fatalf("context mismatch was not kept distinct from grammar failure: %+v", outcome)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if string(content) != "current\n" {
		t.Fatalf("failed patch mutated file: %q", content)
	}
}

func snapshotPatchTestWorkspace(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			snapshot[rel] = "dir"
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		snapshot[rel] = info.Mode().String() + "\x00" + string(content)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot workspace: %v", err)
	}
	return snapshot
}

func equalStringMaps(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func sliceContains(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func TestExecuteOutcomeSuccessCarriesDisplayAndStructuredFacts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "created.txt")
	patch := "*** Begin Patch\n*** Add File: " + path + "\n+hello\n*** End Patch"
	outcome, err := (&ApplyPatchTool{}).ExecuteOutcome(context.Background(), patch, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome: %v", err)
	}
	if outcome.Status != tool.OutcomeSucceeded || outcome.IsError() {
		t.Fatalf("unexpected success outcome: %+v", outcome)
	}
	if outcome.Content == "" || len(outcome.AffectedPaths) != 1 || outcome.PatchSummary == "" {
		t.Fatalf("success outcome missing display/structured facts: %+v", outcome)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "hello\n" {
		t.Fatalf("created file mismatch: %q err=%v", content, err)
	}
}
