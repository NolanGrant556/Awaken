package hookprogress

import (
	"fmt"
	"testing"
)

func TestPendingEventsFlushAndDropsOldest(t *testing.T) {
	resetHookProgressState(t)

	for i := 0; i < 101; i++ {
		if ok := EmitHookStarted(fmt.Sprintf("hook-%03d", i), "setup", "SessionStart"); !ok {
			t.Fatalf("expected SessionStart hook %d to be emitted", i)
		}
	}

	var got []HookExecutionEvent
	RegisterHookEventHandler(func(event HookExecutionEvent) {
		got = append(got, event)
	})

	if len(got) != 100 {
		t.Fatalf("flushed %d pending events, want 100", len(got))
	}
	if got[0].HookID != "hook-001" {
		t.Fatalf("oldest pending event was not dropped: first hook ID %q", got[0].HookID)
	}
	if got[len(got)-1].HookID != "hook-100" {
		t.Fatalf("pending events lost newest item: last hook ID %q", got[len(got)-1].HookID)
	}

	var afterFlush []HookExecutionEvent
	RegisterHookEventHandler(func(event HookExecutionEvent) {
		afterFlush = append(afterFlush, event)
	})
	if len(afterFlush) != 0 {
		t.Fatalf("pending events flushed more than once: got %d", len(afterFlush))
	}
}

func TestAllowlistDefaultsToSessionStartAndSetup(t *testing.T) {
	resetHookProgressState(t)

	var got []HookExecutionEvent
	RegisterHookEventHandler(func(event HookExecutionEvent) {
		got = append(got, event)
	})

	if ok := EmitHookStarted("hook-1", "pre", "PreToolUse"); ok {
		t.Fatal("PreToolUse emitted before all hook events were enabled")
	}
	if ok := EmitHookStarted("hook-2", "session", "SessionStart"); !ok {
		t.Fatal("SessionStart did not emit by default")
	}
	if ok := EmitHookStarted("hook-3", "setup", "Setup"); !ok {
		t.Fatal("Setup did not emit by default")
	}

	if len(got) != 2 {
		t.Fatalf("got %d default events, want 2", len(got))
	}
	if got[0].HookEvent != "SessionStart" || got[1].HookEvent != "Setup" {
		t.Fatalf("unexpected default events: %#v", got)
	}
}

func TestAllHookEventsEnablesFullAllowlistOnly(t *testing.T) {
	resetHookProgressState(t)

	var got []HookExecutionEvent
	RegisterHookEventHandler(func(event HookExecutionEvent) {
		got = append(got, event)
	})
	SetAllHookEventsEnabled(true)

	if ok := EmitHookStarted("hook-1", "pre", "PreToolUse"); !ok {
		t.Fatal("PreToolUse did not emit after enabling all hook events")
	}
	if ok := EmitHookStarted("hook-2", "task", "TaskCreated"); !ok {
		t.Fatal("TaskCreated did not emit after enabling all hook events")
	}
	if ok := EmitHookStarted("hook-3", "unknown", "NotARealHookEvent"); ok {
		t.Fatal("unknown hook event emitted even though it is outside the allowlist")
	}

	if len(got) != 2 {
		t.Fatalf("got %d all-hook events, want 2", len(got))
	}
	if got[0].HookEvent != "PreToolUse" || got[1].HookEvent != "TaskCreated" {
		t.Fatalf("unexpected all-hook events: %#v", got)
	}
}

func TestHookProgressSnapshotsDeduplicateByOutput(t *testing.T) {
	resetHookProgressState(t)

	var got []HookExecutionEvent
	RegisterHookEventHandler(func(event HookExecutionEvent) {
		got = append(got, event)
	})
	SetAllHookEventsEnabled(true)

	first := HookProgressSnapshot{
		HookID:    "hook-1",
		HookName:  "lint",
		HookEvent: "PreToolUse",
		Stdout:    "line 1\n",
		Stderr:    "",
		Output:    "line 1\n",
	}
	if ok := EmitHookProgress(first); !ok {
		t.Fatal("first progress snapshot did not emit")
	}
	if ok := EmitHookProgress(HookProgressSnapshot{
		HookID:    "hook-1",
		HookName:  "lint",
		HookEvent: "PreToolUse",
		Stdout:    "line 1 changed\n",
		Stderr:    "ignored\n",
		Output:    "line 1\n",
	}); ok {
		t.Fatal("duplicate progress output emitted")
	}
	if ok := EmitHookProgress(HookProgressSnapshot{
		HookID:    "hook-1",
		HookName:  "lint",
		HookEvent: "PreToolUse",
		Stdout:    "line 1\nline 2\n",
		Stderr:    "",
		Output:    "line 1\nline 2\n",
	}); !ok {
		t.Fatal("changed progress output did not emit")
	}
	if ok := EmitHookProgress(HookProgressSnapshot{
		HookID:    "hook-2",
		HookName:  "lint",
		HookEvent: "PreToolUse",
		Stdout:    "line 1\nline 2\n",
		Stderr:    "",
		Output:    "line 1\nline 2\n",
	}); !ok {
		t.Fatal("same output for a different hook should emit")
	}

	if len(got) != 3 {
		t.Fatalf("got %d progress events, want 3", len(got))
	}
	if got[0].Type != HookProgress {
		t.Fatalf("first event type = %q, want %q", got[0].Type, HookProgress)
	}
	if got[0].Stdout != first.Stdout || got[0].Stderr != first.Stderr || got[0].Output != first.Output {
		t.Fatalf("first progress snapshot was not preserved: %#v", got[0])
	}
	if got[1].Output != "line 1\nline 2\n" {
		t.Fatalf("changed progress output not preserved: %#v", got[1])
	}
}

func TestHookResponsePreservesExitCodeAndOutcome(t *testing.T) {
	resetHookProgressState(t)

	var got []HookExecutionEvent
	RegisterHookEventHandler(func(event HookExecutionEvent) {
		got = append(got, event)
	})
	SetAllHookEventsEnabled(true)

	exitCode := 42
	if ok := EmitHookResponse(HookResponseData{
		HookID:    "hook-1",
		HookName:  "lint",
		HookEvent: "PostToolUse",
		Output:    "combined output",
		Stdout:    "stdout",
		Stderr:    "stderr",
		ExitCode:  &exitCode,
		Outcome:   HookOutcomeError,
	}); !ok {
		t.Fatal("hook response did not emit")
	}
	if ok := EmitHookResponse(HookResponseData{
		HookID:    "hook-2",
		HookName:  "cancelled",
		HookEvent: "PostToolUse",
		Outcome:   HookOutcomeCancelled,
	}); !ok {
		t.Fatal("cancelled hook response did not emit")
	}

	if len(got) != 2 {
		t.Fatalf("got %d response events, want 2", len(got))
	}

	failed := got[0]
	if failed.Type != HookResponse {
		t.Fatalf("response type = %q, want %q", failed.Type, HookResponse)
	}
	if failed.ExitCode == nil || *failed.ExitCode != 42 {
		t.Fatalf("exit code was not preserved: %#v", failed.ExitCode)
	}
	if failed.Outcome != HookOutcomeError {
		t.Fatalf("outcome = %q, want %q", failed.Outcome, HookOutcomeError)
	}
	if failed.Output != "combined output" || failed.Stdout != "stdout" || failed.Stderr != "stderr" {
		t.Fatalf("response output snapshot was not preserved: %#v", failed)
	}

	cancelled := got[1]
	if cancelled.ExitCode != nil {
		t.Fatalf("cancelled response unexpectedly had exit code: %#v", cancelled.ExitCode)
	}
	if cancelled.Outcome != HookOutcomeCancelled {
		t.Fatalf("cancelled outcome = %q, want %q", cancelled.Outcome, HookOutcomeCancelled)
	}
}

func resetHookProgressState(t *testing.T) {
	t.Helper()

	ClearHookEventState()
	t.Cleanup(ClearHookEventState)
}
