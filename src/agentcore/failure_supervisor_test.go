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

func TestFailureSupervisorAllowsCorrectedInputAfterRepeatedPreflightValidationFailures(t *testing.T) {
	s := NewFailureSupervisor("/tmp/ws", 8, 32)
	invalidCalls := []convcontext.ToolCall{
		{ID: "invalid-1", Function: convcontext.FunctionCall{Name: "computer_act", Arguments: `{"observation_id":"obs-1","action":"click","target":{"x":0}}`}},
		{ID: "invalid-2", Function: convcontext.FunctionCall{Name: "computer_act", Arguments: `{"observation_id":"obs-1","action":"click","target":{"y":10}}`}},
		{ID: "invalid-3", Function: convcontext.FunctionCall{Name: "computer_act", Arguments: `{"observation_id":"obs-1","action":"click","target":{}}`}},
	}
	for _, call := range invalidCalls {
		before := s.BeforeCall(call)
		if before.Block || !before.Allow {
			t.Fatalf("preflight validation call was blocked too early: %+v", before)
		}
		s.NoteRealExecution(before.CallSignature)
		outcome := tool.FailedOutcome("incomplete coordinates", tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false)
		outcome.TransportStatus = tool.TransportNotStarted
		outcome.Operation = &tool.OperationDescriptor{Type: "computer_act", SideEffectState: tool.OutcomeSideEffectNotStarted}
		s.AfterOutcome(call, outcome)
	}

	corrected := convcontext.ToolCall{ID: "valid", Function: convcontext.FunctionCall{Name: "computer_act", Arguments: `{"observation_id":"obs-1","action":"click","target":{"x":0,"y":10}}`}}
	decision := s.BeforeCall(corrected)
	if decision.Block || !decision.Allow {
		t.Fatalf("corrected input inherited a resource failure barrier: %+v", decision)
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

func TestResourceRevisionAndIdentityRecognizeFilePath(t *testing.T) {
	first := ResourceRevision(`{"file_path":"reports/first.docx"}`, tool.ExecutionOutcome{})
	second := ResourceRevision(`{"file_path":"notes/fresh.txt"}`, tool.ExecutionOutcome{})
	if first == "unknown" || second == "unknown" || first == second {
		t.Fatalf("file_path revisions were not isolated: %q %q", first, second)
	}
	if got := ResourceRevision(`{"path":"reports/first.docx"}`, tool.ExecutionOutcome{}); got != first {
		t.Fatalf("file_path revision changed existing path semantics: path=%q file_path=%q", got, first)
	}
	if got := ResourceIdentity(`{"file_path":"notes/../notes/fresh.txt"}`, tool.ExecutionOutcome{}); got != "path:notes/fresh.txt" {
		t.Fatalf("file_path identity=%q", got)
	}
}

func TestFailureSupervisorDOCXFailuresDoNotBlockFreshTXTRead(t *testing.T) {
	s := NewFailureSupervisor("/tmp/ws", 8, 32)
	for index, path := range []string{"reports/first.docx", "reports/second.docx", "reports/third.docx"} {
		call := convcontext.ToolCall{
			ID: "docx-" + path,
			Function: convcontext.FunctionCall{
				Name:      "Read",
				Arguments: `{"file_path":"` + path + `"}`,
			},
		}
		before := s.BeforeCall(call)
		if before.Block || !before.Allow {
			t.Fatalf("DOCX read %d was blocked before execution: %+v", index+1, before)
		}
		s.NoteRealExecution(before.CallSignature)
		s.AfterOutcome(call, tool.FailedOutcome("cannot decode Word document", "file_read_failed", tool.ErrorCategoryDomain, false))
	}

	fresh := convcontext.ToolCall{
		ID: "txt-fresh",
		Function: convcontext.FunctionCall{
			Name:      "Read",
			Arguments: `{"file_path":"notes/fresh.txt"}`,
		},
	}
	decision := s.BeforeCall(fresh)
	if decision.Block || !decision.Allow {
		t.Fatalf("fresh TXT read inherited unrelated DOCX failures: %+v", decision)
	}
}

func TestSyntheticBlockedResultRequiresStrategyChange(t *testing.T) {
	call := convcontext.ToolCall{ID: "blocked", Function: convcontext.FunctionCall{Name: "Read", Arguments: `{"file_path":"missing"}`}}
	result := SyntheticBlockedResult(call, BeforeCallDecision{Block: true, ErrorCode: ErrorCodeRepeatedFailure, ErrorMessage: "repeated", RecoveryOptions: []string{"change_path"}})
	if !result.IsError || result.Outcome.ErrorCode != ErrorCodeRepeatedFailure || !strings.Contains(result.Content, StatusStrategyChangeRequired) {
		t.Fatalf("blocked result=%+v", result)
	}
	if result.Outcome.Retryable || result.Outcome.Diagnostic["retryable"] != false ||
		result.Outcome.Diagnostic["changed_call_allowed"] != true ||
		strings.Contains(result.Content, `"retryable":true`) {
		t.Fatalf("blocked result contains contradictory retry guidance: %+v", result)
	}
}

func TestFailureSupervisorTerminatesRepeatedInvalidAskUserCall(t *testing.T) {
	s := NewFailureSupervisor("/tmp/ws", 8, 32)
	call := convcontext.ToolCall{ID: "ask-empty", Function: convcontext.FunctionCall{Name: "ask_user", Arguments: `{}`}}
	first := s.BeforeCall(call)
	if !first.Allow {
		t.Fatalf("first ask_user call blocked: %+v", first)
	}
	s.NoteRealExecution(first.CallSignature)
	s.AfterOutcome(call, tool.FailedOutcome("questions required", "ask_user_invalid_input", tool.ErrorCategoryValidation, false))

	blocked := s.BeforeCall(call)
	if !blocked.Block || !blocked.TerminateRun || blocked.TerminalReason != TerminalReasonRepeatedToolFailure {
		t.Fatalf("repeated invalid ask_user call did not terminate: %+v", blocked)
	}
	result := SyntheticBlockedResult(call, blocked)
	if result.Outcome.Diagnostic["run_will_terminate"] != true ||
		result.Outcome.Diagnostic["changed_call_allowed"] != true {
		t.Fatalf("terminal ask_user guidance is incomplete: %+v", result)
	}
}

func TestFailureSupervisorBlocksChangedArgumentsAfterUnknownExternalCreation(t *testing.T) {
	s := NewFailureSupervisor("/tmp/ws", 8, 32)
	first := convcontext.ToolCall{ID: "video-1", Function: convcontext.FunctionCall{Name: "video_generate", Arguments: `{"prompt":"first","duration":5}`}}
	before := s.BeforeCall(first)
	if !before.Allow {
		t.Fatalf("first video call blocked: %+v", before)
	}
	s.NoteRealExecution(before.CallSignature)
	outcome := tool.FailedOutcome("create state unknown", "media_generate_create_unknown", tool.ErrorCategoryRuntime, false)
	outcome.TransportStatus = tool.TransportUnknown
	outcome.SemanticStatus = tool.SemanticUnknown
	outcome.Operation = &tool.OperationDescriptor{Type: "video_generate", SideEffectState: tool.OutcomeSideEffectUnknown}
	outcome.RecoveryOptions = []string{"check_provider_console", "do_not_resubmit"}
	outcome.Diagnostic = map[string]any{"requires_external_state_confirmation": true}
	after := s.AfterOutcome(first, outcome)
	if !after.RecoveryRequired || after.Directive == nil || !after.Directive.ExternalStateConfirmationRequired {
		t.Fatalf("unknown creation did not install external-state barrier: %+v", after)
	}

	changed := convcontext.ToolCall{ID: "video-2", Function: convcontext.FunctionCall{Name: "video_generate", Arguments: `{"prompt":"changed","duration":10}`}}
	blocked := s.BeforeCall(changed)
	if !blocked.Block || blocked.Allow || !blocked.ExternalStateConfirmationRequired {
		t.Fatalf("changed video submission was not blocked: %+v", blocked)
	}
	result := SyntheticBlockedResult(changed, blocked)
	if !strings.Contains(result.Content, StatusExternalStateConfirmationRequired) ||
		strings.Contains(result.Content, StatusStrategyChangeRequired) ||
		result.Outcome.Diagnostic["retryable"] != false ||
		result.Outcome.Diagnostic["changed_call_allowed"] != false {
		t.Fatalf("external-state block gave unsafe retry guidance: %+v", result)
	}

	unrelated := convcontext.ToolCall{ID: "read-1", Function: convcontext.FunctionCall{Name: "Read", Arguments: `{"file_path":"a"}`}}
	if decision := s.BeforeCall(unrelated); decision.Block {
		t.Fatalf("external-state barrier blocked unrelated tool: %+v", decision)
	}
}

func TestRecoveryOptionsUseCanonicalToolNames(t *testing.T) {
	options := RecoveryOptionsForError("file_changed_since_read")
	joined := strings.Join(options, " ")
	if !strings.Contains(joined, "Read") || strings.Contains(joined, "read_file") {
		t.Fatalf("recovery options=%v", options)
	}
}
