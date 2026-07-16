package agentcore

import (
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
)

func TestEffectiveMaxDefaults(t *testing.T) {
	if got := EffectiveMaxAgentTurns(0); got != DefaultMaxAgentTurns {
		t.Fatalf("turns=%d", got)
	}
	if got := EffectiveMaxAgentTurns(12); got != 12 {
		t.Fatalf("turns=%d", got)
	}
	if got := EffectiveMaxToolExecutions(0); got != DefaultMaxToolExecutions {
		t.Fatalf("executions=%d", got)
	}
	if got := EffectiveMaxToolExecutions(9); got != 9 {
		t.Fatalf("executions=%d", got)
	}
}

func TestCallSignatureCanonicalizesJSONKeysAndNewlines(t *testing.T) {
	a := CallSignature("Edit", `{"old_string":"line\r\n","b":1,"a":2}`)
	b := CallSignature("Edit", `{"a":2,"b":1,"old_string":"line\n"}`)
	if a != b {
		t.Fatalf("canonical signatures differ: %s %s", a, b)
	}
	if a == CallSignature("Edit", `{"a":2,"b":1,"old_string":"other"}`) {
		t.Fatal("different inputs shared a signature")
	}
}

func TestFailureFingerprintStripsNoise(t *testing.T) {
	root := "/Users/demo/project"
	outcome := tool.ExecutionOutcome{ErrorMessage: root + "/src/a.ts line 12 failed at 2026-07-10T03:39:40Z in /tmp/xyz123"}
	first := FailureFingerprint("Edit", "file_changed_since_read", outcome, root)
	outcome.ErrorMessage = "<workspace>/src/a.ts line 99 failed at 2026-07-11T01:02:03Z in /tmp/other"
	second := FailureFingerprint("Edit", "file_changed_since_read", outcome, root)
	if first != second {
		t.Fatalf("fingerprints differ: %s %s", first, second)
	}
}

func TestFailureSupervisorBlocksNonRetryableDuplicate(t *testing.T) {
	s := NewFailureSupervisor("/tmp/ws", 8, 32)
	call := convcontext.ToolCall{ID: "c1", Function: convcontext.FunctionCall{Name: "Edit", Arguments: `{"file_path":"a","old_string":"x","new_string":"y"}`}}
	before := s.BeforeCall(call)
	if !before.Allow {
		t.Fatalf("first call blocked: %+v", before)
	}
	s.NoteRealExecution(before.CallSignature)
	s.AfterOutcome(call, tool.FailedOutcome("not found", "edit_match_not_found", tool.ErrorCategoryDomain, false))
	second := s.BeforeCall(call)
	if !second.Block || second.ErrorCode != ErrorCodeToolRetryBlocked || len(second.RecoveryOptions) == 0 {
		t.Fatalf("duplicate was not blocked: %+v", second)
	}
}

func TestFailureSupervisorAllowsRetryableTwiceThenBlocks(t *testing.T) {
	s := NewFailureSupervisor("/tmp/ws", 8, 32)
	call := convcontext.ToolCall{ID: "c1", Function: convcontext.FunctionCall{Name: "Bash", Arguments: `{"command":"false"}`}}
	for index := 0; index < 2; index++ {
		before := s.BeforeCall(call)
		if before.Block {
			t.Fatalf("attempt %d blocked", index+1)
		}
		s.NoteRealExecution(before.CallSignature)
		s.AfterOutcome(call, tool.FailedOutcome("exit 1", tool.ErrorCodeProcessExitNonzero, tool.ErrorCategoryProcess, true))
	}
	if third := s.BeforeCall(call); !third.Block {
		t.Fatalf("third call not blocked: %+v", third)
	}
}

func TestResourceRevisionSeparatesBashCommandsWithoutLeakingText(t *testing.T) {
	first := ResourceRevision(`{"command":"node scripts/validate.js"}`, tool.ExecutionOutcome{})
	second := ResourceRevision(`{"command":"npm install"}`, tool.ExecutionOutcome{})
	if first == "unknown" || second == "unknown" || first == second {
		t.Fatalf("bad revisions: %q %q", first, second)
	}
	if strings.Contains(first, "validate") || strings.Contains(second, "npm") {
		t.Fatalf("revision leaked input: %q %q", first, second)
	}
}

func TestSyntheticBlockedResultRequiresStrategyChange(t *testing.T) {
	call := convcontext.ToolCall{ID: "blocked", Function: convcontext.FunctionCall{Name: "Read", Arguments: `{"file_path":"missing"}`}}
	result := SyntheticBlockedResult(call, BeforeCallDecision{Block: true, ErrorCode: ErrorCodeRepeatedFailure, ErrorMessage: "repeated", RecoveryOptions: []string{"change_path"}})
	if !result.IsError || result.Outcome.ErrorCode != ErrorCodeRepeatedFailure || !strings.Contains(result.Content, StatusStrategyChangeRequired) {
		t.Fatalf("blocked result=%+v", result)
	}
}

func TestRecoveryOptionsUseCanonicalToolNames(t *testing.T) {
	options := RecoveryOptionsForError("file_changed_since_read")
	joined := strings.Join(options, " ")
	if !strings.Contains(joined, "Read") || strings.Contains(joined, "read_file") {
		t.Fatalf("recovery options=%v", options)
	}
}
