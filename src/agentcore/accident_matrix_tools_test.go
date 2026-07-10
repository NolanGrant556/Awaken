package agentcore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
	applypatch "awaken/src/tools/apply_patch"
	_ "awaken/src/tools/file_read"
	textedit "awaken/src/tools/text_edit"
)

func TestA07TextEditRealToolChainProducesDigestPreviewAndArtifact(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	arguments := mustAccidentArguments(t, map[string]any{
		"path":       "sample.txt",
		"old_string": "alpha",
		"new_string": "omega",
	})
	var events []AwakenEvent
	runtime := NewToolRuntimeWithConfig(EventSinkFunc(func(event AwakenEvent) {
		events = append(events, event)
	}), ToolRuntimeConfig{
		PermissionContext: tool.NewPermissionContext(root, []string{root}),
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "a07_text_edit",
		Function: convcontext.FunctionCall{
			Name:      "text_edit",
			Arguments: arguments,
		},
	})
	if result.IsError || result.Outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("real text_edit execution failed: %+v", result)
	}
	beforeDigest, _ := result.Outcome.Diagnostic["before_sha256"].(string)
	afterDigest, _ := result.Outcome.Diagnostic["after_sha256"].(string)
	if beforeDigest == "" || afterDigest == "" || beforeDigest == afterDigest || result.Outcome.Diagnostic["sha256"] != afterDigest {
		t.Fatalf("A07 digest contract incomplete: %+v", result.Outcome.Diagnostic)
	}
	if len(result.Outcome.Artifacts) != 1 || result.Outcome.Artifacts[0].Path != "sample.txt" {
		t.Fatalf("A07 outcome artifact missing: %+v", result.Outcome.Artifacts)
	}

	previewIndex, artifactIndex := -1, -1
	for index, event := range events {
		switch event.Type {
		case PatchUpdatedEventType:
			if event.ToolCallID == "a07_text_edit" {
				previewIndex = index
				if event.Patch == "" || event.PatchSummary == "" || len(event.AffectedPaths) != 1 || event.AffectedPaths[0] != "sample.txt" {
					t.Fatalf("A07 patch preview incomplete: %+v", event)
				}
			}
		case ArtifactUpdatedEventType:
			if event.ToolCallID == "a07_text_edit" {
				artifactIndex = index
				if event.ArtifactPath != "sample.txt" || event.ArtifactStatus != ArtifactStatusReady {
					t.Fatalf("A07 artifact event incomplete: %+v", event)
				}
			}
		}
	}
	if previewIndex < 0 || artifactIndex < 0 || previewIndex >= artifactIndex {
		t.Fatalf("A07 must emit preview before artifact in one real execution: %+v", events)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "omega\nbeta\n" {
		t.Fatalf("A07 file result mismatch: %q err=%v", content, err)
	}
}

func TestA08TextEditZeroMatchThroughRunnerHasNoArtifactOrMutation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	original := []byte("present\n")
	if err := os.WriteFile(path, original, 0o640); err != nil {
		t.Fatalf("seed: %v", err)
	}
	runner := newAccidentRunner(root)
	var eventsSeen []AwakenEvent
	events := newRunEventEmitter("a08_thread", EventSinkFunc(func(event AwakenEvent) {
		eventsSeen = append(eventsSeen, event)
	}))
	events.runID = "a08_run"
	events.recorder = NoopSemanticRecorder{}
	results := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{{
		ID: "a08_not_found",
		Function: convcontext.FunctionCall{
			Name: "text_edit",
			Arguments: mustAccidentArguments(t, map[string]any{
				"path":       "sample.txt",
				"old_string": "missing",
				"new_string": "changed",
			}),
		},
	}}, events)
	if len(results) != 1 || !results[0].IsError ||
		results[0].Outcome.ErrorCode != textedit.ErrorCodeTextNotFound ||
		results[0].Outcome.TransportStatus != tool.TransportCompleted {
		t.Fatalf("A08 Runner lost typed text_not_found outcome: %+v", results)
	}
	if runner.failureSupervisor.ToolExecutions() != 1 {
		t.Fatalf("A08 real execution count=%d want 1", runner.failureSupervisor.ToolExecutions())
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != string(original) {
		t.Fatalf("A08 zero match changed file: %q err=%v", content, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("A08 zero match changed file metadata: mode=%v err=%v", info, err)
	}
	for _, event := range eventsSeen {
		if event.Type == ArtifactCreatedEventType || event.Type == ArtifactUpdatedEventType {
			t.Fatalf("A08 failed edit emitted artifact: %+v", event)
		}
	}
}

func TestA09A11TextEditReplaceAllRequiresExplicitRuntimeInput(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	original := "x\nkeep\nx\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var events []AwakenEvent
	runtime := NewToolRuntimeWithConfig(EventSinkFunc(func(event AwakenEvent) {
		events = append(events, event)
	}), ToolRuntimeConfig{
		PermissionContext: tool.NewPermissionContext(root, []string{root}),
	})
	withoutReplaceAll := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "a09_no_replace_all",
		Function: convcontext.FunctionCall{
			Name: "text_edit",
			Arguments: mustAccidentArguments(t, map[string]any{
				"path":       "sample.txt",
				"old_string": "x",
				"new_string": "y",
			}),
		},
	})
	if withoutReplaceAll.Outcome.ErrorCode != textedit.ErrorCodeTextNotUnique || !withoutReplaceAll.IsError {
		t.Fatalf("A09 implicit multi-replace was not rejected: %+v", withoutReplaceAll)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != original {
		t.Fatalf("A09 multi-match failure changed file: %q err=%v", content, err)
	}
	for _, event := range events {
		if event.ToolCallID == "a09_no_replace_all" &&
			(event.Type == ArtifactCreatedEventType || event.Type == ArtifactUpdatedEventType) {
			t.Fatalf("A09 failed edit emitted artifact: %+v", event)
		}
	}

	events = nil
	withReplaceAll := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "a11_explicit_replace_all",
		Function: convcontext.FunctionCall{
			Name: "text_edit",
			Arguments: mustAccidentArguments(t, map[string]any{
				"path":        "sample.txt",
				"old_string":  "x",
				"new_string":  "y",
				"replace_all": true,
			}),
		},
	})
	if withReplaceAll.IsError || withReplaceAll.Outcome.Status != tool.OutcomeSucceeded ||
		withReplaceAll.Outcome.Diagnostic["replacement_count"] != float64(2) && withReplaceAll.Outcome.Diagnostic["replacement_count"] != 2 {
		t.Fatalf("A11 explicit replace_all did not succeed twice: %+v", withReplaceAll)
	}
	content, err = os.ReadFile(path)
	if err != nil || string(content) != "y\nkeep\ny\n" {
		t.Fatalf("A11 explicit replace_all result mismatch: %q err=%v", content, err)
	}
	foundArtifact := false
	for _, event := range events {
		if event.ToolCallID == "a11_explicit_replace_all" && event.Type == ArtifactUpdatedEventType {
			foundArtifact = true
		}
	}
	if !foundArtifact {
		t.Fatalf("A11 successful replace_all emitted no artifact: %+v", events)
	}
}

func TestA12MissingBeginThroughRunnerIsTypedRecoverableAndSideEffectFree(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	original := []byte("current\n")
	if err := os.WriteFile(path, original, 0o640); err != nil {
		t.Fatalf("seed: %v", err)
	}
	runner := newAccidentRunner(root)
	patch := "*** Update File: sample.txt\n@@\n-current\n+changed\n*** End Patch"
	var eventsSeen []AwakenEvent
	events := newRunEventEmitter("a12_thread", EventSinkFunc(func(event AwakenEvent) {
		eventsSeen = append(eventsSeen, event)
	}))
	events.runID = "a12_run"
	events.recorder = NoopSemanticRecorder{}
	results := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{{
		ID: "a12_missing_begin",
		Function: convcontext.FunctionCall{
			Name:      "apply_patch",
			Arguments: mustAccidentArguments(t, map[string]any{"patch": patch}),
		},
	}}, events)
	if len(results) != 1 || !results[0].IsError ||
		results[0].Outcome.ErrorCode != applypatch.ErrorCodePatchMissingBegin ||
		ErrorClass(results[0].Outcome.ErrorCode, results[0].Outcome) != "invalid_patch_format" {
		t.Fatalf("A12 Runner/Supervisor lost typed missing-Begin outcome: %+v", results)
	}
	if !accidentContains(results[0].Outcome.RecoveryOptions, "use_text_edit_or_text_file_create") ||
		!accidentContains(results[0].Outcome.RecoveryOptions, "do_not_replay_same_patch") {
		t.Fatalf("A12 recovery does not point to text_edit: %+v", results[0].Outcome)
	}
	if runner.failureSupervisor.ToolExecutions() != 1 {
		t.Fatalf("A12 real execution count=%d want 1", runner.failureSupervisor.ToolExecutions())
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != string(original) {
		t.Fatalf("A12 malformed patch changed file: %q err=%v", content, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("A12 malformed patch changed file metadata: mode=%v err=%v", info, err)
	}
	for _, event := range eventsSeen {
		if event.Type == PatchUpdatedEventType || event.Type == ArtifactCreatedEventType || event.Type == ArtifactUpdatedEventType {
			t.Fatalf("A12 malformed patch emitted mutation preview/artifact: %+v", event)
		}
	}
}

func TestA13MalformedHunkStaysTypedAndSideEffectFreeThroughRuntime(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(path, []byte("current\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	patch := "*** Begin Patch\n*** Update File: sample.txt\n@@\ncurrent\n+changed\n*** End Patch"
	var events []AwakenEvent
	runtime := NewToolRuntimeWithConfig(EventSinkFunc(func(event AwakenEvent) {
		events = append(events, event)
	}), ToolRuntimeConfig{
		PermissionContext: tool.NewPermissionContext(root, []string{root}),
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "a13_bad_hunk",
		Function: convcontext.FunctionCall{
			Name:      "apply_patch",
			Arguments: mustAccidentArguments(t, map[string]any{"patch": patch}),
		},
	})
	if !result.IsError || result.Outcome.ErrorCode != applypatch.ErrorCodePatchInvalidLinePrefix ||
		ErrorClass(result.Outcome.ErrorCode, result.Outcome) != "invalid_patch_format" {
		t.Fatalf("A13 malformed hunk lost typed classification: %+v", result)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "current\n" {
		t.Fatalf("A13 malformed hunk changed file: %q err=%v", content, err)
	}
	for _, event := range events {
		if event.Type == PatchUpdatedEventType || event.Type == ArtifactCreatedEventType || event.Type == ArtifactUpdatedEventType {
			t.Fatalf("A13 malformed hunk emitted mutation preview/artifact: %+v", event)
		}
	}
}

func TestA14ApplyPatchContextMismatchIsDistinctThroughToolRuntime(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(path, []byte("current\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	patch := "*** Begin Patch\n*** Update File: sample.txt\n@@\n-old\n+new\n*** End Patch"
	runtime := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		PermissionContext: tool.NewPermissionContext(root, []string{root}),
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "a14_context",
		Function: convcontext.FunctionCall{
			Name:      "apply_patch",
			Arguments: mustAccidentArguments(t, map[string]any{"patch": patch}),
		},
	})
	if !result.IsError || result.Outcome.ErrorCode != applypatch.ErrorCodePatchContextNotFound {
		t.Fatalf("A14 typed context failure lost in ToolRuntime: %+v", result)
	}
	if got := ErrorClass(result.Outcome.ErrorCode, result.Outcome); got != "patch_context_mismatch" {
		t.Fatalf("A14 context failure classified as %q", got)
	}
	if !accidentContains(result.Outcome.RecoveryOptions, "read_file") ||
		!accidentContains(result.Outcome.RecoveryOptions, "use_text_edit") {
		t.Fatalf("A14 recovery contract incomplete: %+v", result.Outcome)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "current\n" {
		t.Fatalf("A14 changed file despite context mismatch: %q err=%v", content, err)
	}
}

func TestA15ExactNonRetryableApplyPatchRunsOnceThenBlocks(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(path, []byte("current\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	runner := newAccidentRunner(root)
	patch := "*** Update File: sample.txt\n@@\n-current\n+changed\n*** End Patch"
	arguments := mustAccidentArguments(t, map[string]any{"patch": patch})
	var eventsSeen []AwakenEvent
	events := newRunEventEmitter("a15_thread", EventSinkFunc(func(event AwakenEvent) {
		eventsSeen = append(eventsSeen, event)
	}))
	events.runID = "a15_run"
	// Both patch attempts are parser failures and cannot mutate the file, so
	// a no-op recorder is safe for this isolated runner/event contract test.
	events.recorder = NoopSemanticRecorder{}
	results := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{
		{ID: "a15_first", Function: convcontext.FunctionCall{Name: "apply_patch", Arguments: arguments}},
		{ID: "a15_second", Function: convcontext.FunctionCall{Name: "apply_patch", Arguments: arguments}},
	}, events)
	if len(results) != 2 || results[0].Outcome.ErrorCode != applypatch.ErrorCodePatchMissingBegin {
		t.Fatalf("A15 first real failure is not typed: %+v", results)
	}
	if results[1].Outcome.ErrorCode != ErrorCodeToolRetryBlocked ||
		results[1].Outcome.TransportStatus != tool.TransportNotStarted {
		t.Fatalf("A15 duplicate was not blocked before execution: %+v", results[1])
	}
	if runner.failureSupervisor.ToolExecutions() != 1 {
		t.Fatalf("A15 exact duplicate executed more than once: %d", runner.failureSupervisor.ToolExecutions())
	}
	if !accidentContains(results[1].Outcome.RecoveryOptions, "use_text_edit_or_text_file_create") {
		t.Fatalf("A15 blocked result lost recovery guidance: %+v", results[1].Outcome)
	}
	blockedEvent, blockedResultEvent := -1, -1
	for index, event := range eventsSeen {
		if event.Type == ToolRetryBlockedEventType && event.ToolCallID == "a15_second" {
			blockedEvent = index
		}
		if event.Type == ToolResultEventType && event.ToolCallID == "a15_second" {
			blockedResultEvent = index
			if event.ToolStatus != string(tool.OutcomeDenied) || event.TransportStatus != string(tool.TransportNotStarted) {
				t.Fatalf("A15 blocked ToolResult event lost canonical outcome: %+v", event)
			}
		}
	}
	if blockedEvent < 0 || blockedResultEvent <= blockedEvent {
		t.Fatalf("A15 missing ordered ToolRetryBlocked -> ToolResult events: %+v", eventsSeen)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "current\n" {
		t.Fatalf("A15 malformed patch caused side effects: %q err=%v", content, err)
	}
}

func TestA16RealRetryableStaleEditRunsTwiceThenBlocksThird(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(path, []byte("alpha\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	runner := newAccidentRunner(root)
	arguments := mustAccidentArguments(t, map[string]any{
		"path":            "sample.txt",
		"old_string":      "alpha",
		"new_string":      "beta",
		"expected_sha256": "deadbeef",
	})
	var eventsSeen []AwakenEvent
	events := newRunEventEmitter("a16_thread", EventSinkFunc(func(event AwakenEvent) {
		eventsSeen = append(eventsSeen, event)
	}))
	events.runID = "a16_run"
	events.recorder = NoopSemanticRecorder{}
	results := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{
		{ID: "a16_first", Function: convcontext.FunctionCall{Name: "text_edit", Arguments: arguments}},
		{ID: "a16_second", Function: convcontext.FunctionCall{Name: "text_edit", Arguments: arguments}},
		{ID: "a16_third", Function: convcontext.FunctionCall{Name: "text_edit", Arguments: arguments}},
	}, events)
	if len(results) != 3 {
		t.Fatalf("A16 result count=%d", len(results))
	}
	for index := 0; index < 2; index++ {
		if results[index].Outcome.ErrorCode != textedit.ErrorCodeStaleFileRevision ||
			!results[index].Outcome.Retryable ||
			results[index].Outcome.TransportStatus != tool.TransportCompleted {
			t.Fatalf("A16 attempt %d was not a real retryable stale failure: %+v", index+1, results[index])
		}
	}
	if results[2].Outcome.ErrorCode != ErrorCodeToolRetryBlocked ||
		results[2].Outcome.TransportStatus != tool.TransportNotStarted {
		t.Fatalf("A16 third exact retry was not blocked: %+v", results[2])
	}
	if runner.failureSupervisor.ToolExecutions() != 2 {
		t.Fatalf("A16 exact retry executed %d times, want 2", runner.failureSupervisor.ToolExecutions())
	}
	blockedEvent := false
	for _, event := range eventsSeen {
		if event.Type == ToolRetryBlockedEventType && event.ToolCallID == "a16_third" {
			blockedEvent = true
		}
	}
	if !blockedEvent {
		t.Fatalf("A16 emitted no ToolRetryBlocked event: %+v", eventsSeen)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "alpha\n" {
		t.Fatalf("A16 stale retries changed file: %q err=%v", content, err)
	}
}

func TestA17RealTextEditClassBudgetBlocksFourthVariant(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(path, []byte("present\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	runner := newAccidentRunner(root)
	calls := make([]convcontext.ToolCall, 0, 4)
	for index := 0; index < 4; index++ {
		calls = append(calls, convcontext.ToolCall{
			ID: "a17_" + string(rune('1'+index)),
			Function: convcontext.FunctionCall{
				Name: "text_edit",
				Arguments: mustAccidentArguments(t, map[string]any{
					"path":       "sample.txt",
					"old_string": "missing-" + string(rune('a'+index)),
					"new_string": "changed",
				}),
			},
		})
	}
	results := runner.executeToolCalls(context.Background(), calls, nil)
	if len(results) != 4 {
		t.Fatalf("A17 result count=%d", len(results))
	}
	for index := 0; index < 3; index++ {
		if results[index].Outcome.ErrorCode != textedit.ErrorCodeTextNotFound ||
			results[index].Outcome.TransportStatus != tool.TransportCompleted {
			t.Fatalf("A17 call %d was not a real text_edit failure: %+v", index+1, results[index])
		}
	}
	if results[3].Outcome.ErrorCode != ErrorCodeRepeatedFailure ||
		results[3].Outcome.TransportStatus != tool.TransportNotStarted {
		t.Fatalf("A17 fourth same-class variant was not blocked: %+v", results[3])
	}
	if !accidentContains(results[3].Outcome.RecoveryOptions, "change_tool_or_refresh_resource_revision") ||
		!accidentContains(results[3].Outcome.RecoveryOptions, "stop_repeating_same_error_class") {
		t.Fatalf("A17 class-budget block did not require a strategy change: %+v", results[3].Outcome)
	}
	if runner.failureSupervisor.ToolExecutions() != 3 {
		t.Fatalf("A17 real execution count=%d want 3", runner.failureSupervisor.ToolExecutions())
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "present\n" {
		t.Fatalf("A17 failed edits changed file: %q err=%v", content, err)
	}
}

func TestA18RealReadRefreshesRevisionAndAllowsTextEdit(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(path, []byte("alpha\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	runner := newAccidentRunner(root)
	staleArguments := mustAccidentArguments(t, map[string]any{
		"path":            "sample.txt",
		"old_string":      "alpha",
		"new_string":      "beta",
		"expected_sha256": "deadbeef",
	})
	staleResults := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{
		{ID: "a18_stale_1", Function: convcontext.FunctionCall{Name: "text_edit", Arguments: staleArguments}},
		{ID: "a18_stale_2", Function: convcontext.FunctionCall{Name: "text_edit", Arguments: staleArguments}},
	}, nil)
	if len(staleResults) != 2 ||
		staleResults[0].Outcome.ErrorCode != textedit.ErrorCodeStaleFileRevision ||
		staleResults[1].Outcome.ErrorCode != textedit.ErrorCodeStaleFileRevision ||
		runner.failureSupervisor.PendingRecovery() == nil {
		t.Fatalf("A18 did not establish a real stale-revision recovery state: %+v", staleResults)
	}

	readResults := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{{
		ID: "a18_read",
		Function: convcontext.FunctionCall{
			Name:      "read_file",
			Arguments: mustAccidentArguments(t, map[string]any{"path": "sample.txt"}),
		},
	}}, nil)
	if len(readResults) != 1 || readResults[0].IsError {
		t.Fatalf("A18 real read_file failed: %+v", readResults)
	}
	freshDigest, _ := readResults[0].Outcome.Diagnostic["sha256"].(string)
	if freshDigest == "" {
		t.Fatalf("A18 read_file did not return digest: %+v", readResults[0].Outcome)
	}
	if runner.failureSupervisor.PendingRecovery() != nil {
		t.Fatalf("A18 same-resource read did not clear recovery: %+v", runner.failureSupervisor.PendingRecovery())
	}

	freshResults := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{{
		ID: "a18_fresh_edit",
		Function: convcontext.FunctionCall{
			Name: "text_edit",
			Arguments: mustAccidentArguments(t, map[string]any{
				"path":            "sample.txt",
				"old_string":      "alpha",
				"new_string":      "beta",
				"expected_sha256": freshDigest,
			}),
		},
	}}, nil)
	if len(freshResults) != 1 || freshResults[0].IsError || freshResults[0].Outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("A18 fresh revision was not allowed: %+v", freshResults)
	}
	if runner.failureSupervisor.ToolExecutions() != 4 {
		t.Fatalf("A18 unexpected real execution count: %d", runner.failureSupervisor.ToolExecutions())
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "beta\n" {
		t.Fatalf("A18 fresh edit result mismatch: %q err=%v", content, err)
	}
}

func newAccidentRunner(root string) *Runner {
	config := AgentConfig{
		SystemPrompt:      "test",
		MaxThoughts:       16,
		MaxToolExecutions: 64,
		ModelConfig:       ModelConfig{Model: "test-model"},
		Workspace: WorkspaceConfig{
			Root:          root,
			ReadableRoots: []string{root},
			WritableRoots: []string{root},
			OutputRoots:   []string{root},
		},
	}
	return NewRunner(config, convcontext.NewConversation(nil), NewRequestBuilder(config))
}

func mustAccidentArguments(t *testing.T, payload map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal tool arguments: %v", err)
	}
	return string(encoded)
}

func accidentContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
