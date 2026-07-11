package applypatch

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
	"awaken/src/tool/trash"
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

func TestExecuteOutcomeReportsEarlierOperationsWhenLaterPatchFails(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "existing.txt")
	if err := os.WriteFile(existing, []byte("current\n"), 0o644); err != nil {
		t.Fatalf("seed existing file: %v", err)
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	patch := "*** Begin Patch\n" +
		"*** Add File: created.txt\n" +
		"+created\n" +
		"*** Update File: existing.txt\n" +
		"@@\n" +
		"-stale\n" +
		"+changed\n" +
		"*** End Patch"

	outcome, err := (&ApplyPatchTool{}).ExecuteOutcome(ctx, patch, nil)
	if err != nil {
		t.Fatalf("business failure returned Go error: %v", err)
	}
	if !outcome.IsError() || outcome.ErrorCode != ErrorCodePatchContextNotFound {
		t.Fatalf("expected later context failure: %+v", outcome)
	}
	createdPath := filepath.Join(root, "created.txt")
	if content, readErr := os.ReadFile(createdPath); readErr != nil || string(content) != "created\n" {
		t.Fatalf("first operation did not actually apply: content=%q err=%v", content, readErr)
	}
	if len(outcome.Changes) != 1 || outcome.Changes[0].Action != "created" || outcome.Changes[0].Resource != "created.txt" {
		t.Fatalf("partial outcome lost the applied add: %+v", outcome.Changes)
	}
	normalized := outcome.Normalize(tool.OutcomeNormalizationContext{OperationType: "apply_patch"})
	if normalized.Operation == nil || normalized.Operation.SideEffectState != tool.OutcomeSideEffectPartial {
		t.Fatalf("partially applied patch must report partial: %+v", normalized.Operation)
	}
	if len(normalized.NextActions) == 0 || normalized.NextActions[0].ID != "verify_partial_patch_state" {
		t.Fatalf("partial patch lacks safe recovery guidance: %+v", normalized.NextActions)
	}
}

func TestExecuteOutcomeReportsWriteInsideFailedMove(t *testing.T) {
	useTestTrashAdapter(t)
	trash.SetAdapter(failingTrashAdapter{})
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("before\n"), 0o644); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	patch := "*** Begin Patch\n" +
		"*** Update File: source.txt\n" +
		"*** Move to: destination.txt\n" +
		"@@\n" +
		"-before\n" +
		"+after\n" +
		"*** End Patch"

	outcome, err := (&ApplyPatchTool{}).ExecuteOutcome(ctx, patch, nil)
	if err != nil {
		t.Fatalf("business failure returned Go error: %v", err)
	}
	if !outcome.IsError() || outcome.ErrorCode != ErrorCodeApplyPatchFailed {
		t.Fatalf("expected failed source cleanup: %+v", outcome)
	}
	destination := filepath.Join(root, "destination.txt")
	if content, readErr := os.ReadFile(destination); readErr != nil || string(content) != "after\n" {
		t.Fatalf("move destination write was not observed: content=%q err=%v", content, readErr)
	}
	if _, statErr := os.Stat(source); statErr != nil {
		t.Fatalf("failing trash adapter should leave source for this fixture: %v", statErr)
	}
	if len(outcome.Changes) != 1 || outcome.Changes[0].Resource != "destination.txt" || outcome.Changes[0].Action != "created" {
		t.Fatalf("failed move lost its observed destination write: %+v", outcome.Changes)
	}
	if outcome.Changes[0].Details["source_state"] != "unknown" {
		t.Fatalf("failed move guessed source final state: %+v", outcome.Changes[0])
	}
	normalized := outcome.Normalize(tool.OutcomeNormalizationContext{OperationType: "apply_patch"})
	if normalized.Operation == nil || normalized.Operation.SideEffectState != tool.OutcomeSideEffectPartial {
		t.Fatalf("failed move with written destination must be partial: %+v", normalized.Operation)
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
	if len(outcome.Changes) != 1 || outcome.Changes[0].Action != "created" || outcome.Changes[0].Certainty != tool.ChangeObserved {
		t.Fatalf("success outcome lost exact add operation: %+v", outcome.Changes)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "hello\n" {
		t.Fatalf("created file mismatch: %q err=%v", content, err)
	}
}

func TestPatchResourceChangesPreserveExactActions(t *testing.T) {
	root := t.TempDir()
	permissionContext := tool.NewPermissionContext(root, []string{root})
	patch := &patchDocument{operations: []patchOperation{
		{kind: patchKindAdd, path: filepath.Join(root, "created.txt")},
		{kind: patchKindUpdate, path: filepath.Join(root, "updated.txt")},
		{kind: patchKindUpdate, path: filepath.Join(root, "before.txt"), movePath: filepath.Join(root, "after.txt")},
		{kind: patchKindDelete, path: filepath.Join(root, "deleted.txt")},
	}}
	trashPath := filepath.Join(root, ".trash", "deleted.txt")
	summary := patchSummary{deleted: []deletedEntry{{path: filepath.Join(root, "deleted.txt"), trashedPath: trashPath}}}

	changes := patchResourceChanges(patch, summary, permissionContext)
	if len(changes) != 4 {
		t.Fatalf("expected four typed changes, got %+v", changes)
	}
	wantActions := []string{"created", "updated", "moved", "trashed"}
	for i, want := range wantActions {
		if changes[i].Action != want || changes[i].Certainty != tool.ChangeObserved {
			t.Fatalf("change %d lost exact action %q: %+v", i, want, changes[i])
		}
	}
	if changes[2].Resource != "before.txt" || changes[2].Details["destination_path"] != "after.txt" {
		t.Fatalf("move source/destination facts drifted: %+v", changes[2])
	}
	if changes[3].Resource != "deleted.txt" || changes[3].Details["trashed_path"] != trashPath {
		t.Fatalf("trash original/destination facts drifted: %+v", changes[3])
	}

	outcome := tool.SucceededOutcome("changed")
	outcome.AffectedPaths = []string{"created.txt", "updated.txt", "before.txt", "after.txt", "deleted.txt"}
	outcome.Changes = changes
	outcome = outcome.Normalize(tool.OutcomeNormalizationContext{OperationType: "apply_patch"})
	if len(outcome.Changes) != 4 {
		t.Fatalf("normalization added generic modified changes: %+v", outcome.Changes)
	}
}

func TestExecuteOutcomeLabelsCompatibilityAddOverwrite(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "existing.txt")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	permissionContext := tool.NewPermissionContext(root, []string{root})
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	patch := "*** Begin Patch\n*** Add File: existing.txt\n+new\n*** End Patch"
	outcome, err := (&ApplyPatchTool{}).ExecuteOutcome(ctx, patch, nil)
	if err != nil || outcome.IsError() {
		t.Fatalf("compatibility overwrite failed: outcome=%+v err=%v", outcome, err)
	}
	if len(outcome.Changes) != 1 || outcome.Changes[0].Action != "overwritten" || !strings.Contains(outcome.PatchSummary, "覆盖 1 个文件") {
		t.Fatalf("Add-existing was misreported as create: %+v", outcome)
	}
	if len(outcome.Warnings) == 0 || outcome.Warnings[0].Code != "patch_add_replaced_existing" {
		t.Fatalf("compatibility overwrite warning missing: %+v", outcome.Warnings)
	}
	artifacts := (&ApplyPatchTool{}).Artifacts(patch, outcome.Content, permissionContext)
	if len(artifacts) != 1 || artifacts[0].Action != tool.ArtifactActionUpdated {
		t.Fatalf("Add-existing artifact was misreported as created: %+v", artifacts)
	}
}

func TestExecuteOutcomeLabelsCompatibilityMoveDestinationReplacement(t *testing.T) {
	useTestTrashAdapter(t)
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	destination := filepath.Join(root, "destination.txt")
	if err := os.WriteFile(source, []byte("source\n"), 0o644); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	if err := os.WriteFile(destination, []byte("old destination\n"), 0o644); err != nil {
		t.Fatalf("seed destination: %v", err)
	}
	permissionContext := tool.NewPermissionContext(root, []string{root})
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	patch := "*** Begin Patch\n*** Update File: source.txt\n*** Move to: destination.txt\n@@\n-source\n+new\n*** End Patch"
	outcome, err := (&ApplyPatchTool{}).ExecuteOutcome(ctx, patch, nil)
	if err != nil || outcome.IsError() {
		t.Fatalf("compatibility move replacement failed: outcome=%+v err=%v", outcome, err)
	}
	if len(outcome.Changes) != 1 || outcome.Changes[0].Action != "moved" || outcome.Changes[0].Details["destination_replaced"] != true {
		t.Fatalf("move destination replacement was not explicit: %+v", outcome.Changes)
	}
	if len(outcome.Warnings) == 0 || outcome.Warnings[0].Code != "patch_move_replaced_destination" {
		t.Fatalf("move replacement warning missing: %+v", outcome.Warnings)
	}
	artifacts := (&ApplyPatchTool{}).Artifacts(patch, outcome.Content, permissionContext)
	if len(artifacts) != 1 || !strings.Contains(artifacts[0].Description, "replaced the existing destination") {
		t.Fatalf("move artifact lost destination replacement: %+v", artifacts)
	}
}
