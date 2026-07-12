package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/tool"
	applypatch "awaken/src/tools/apply_patch"
)

func TestEffectiveMaxDefaults(t *testing.T) {
	if got := EffectiveMaxAgentTurns(0); got != DefaultMaxAgentTurns {
		t.Fatalf("MaxAgentTurns 0 => %d, want %d", got, DefaultMaxAgentTurns)
	}
	if got := EffectiveMaxAgentTurns(12); got != 12 {
		t.Fatalf("MaxAgentTurns 12 => %d", got)
	}
	if got := EffectiveMaxToolExecutions(0); got != DefaultMaxToolExecutions {
		t.Fatalf("MaxToolExecutions 0 => %d, want %d", got, DefaultMaxToolExecutions)
	}
	if got := EffectiveMaxToolExecutions(9); got != 9 {
		t.Fatalf("MaxToolExecutions 9 => %d", got)
	}
}

func TestCallSignatureCanonicalizesJSONKeysAndPatchWhitespace(t *testing.T) {
	a := CallSignature("apply_patch", `{"patch":"line\r\n","b":1,"a":2}`)
	b := CallSignature("apply_patch", `{"a":2,"b":1,"patch":"line\n"}`)
	if a != b {
		t.Fatalf("expected canonical signatures to match\n%s\n%s", a, b)
	}
	c := CallSignature("apply_patch", `{"a":2,"b":1,"patch":"line2\n"}`)
	if a == c {
		t.Fatalf("different patch content must differ")
	}
}

func TestFailureFingerprintStripsNoise(t *testing.T) {
	root := "/Users/demo/project"
	outcome := tool.ExecutionOutcome{
		ErrorMessage: root + "/src/a.ts line 12 failed at 2026-07-10T03:39:40Z in /tmp/xyz123",
	}
	fp1 := FailureFingerprint("apply_patch", "patch_context_mismatch", outcome, root)
	outcome.ErrorMessage = "<workspace>/src/a.ts line 99 failed at 2026-07-11T01:02:03Z in /tmp/other"
	fp2 := FailureFingerprint("apply_patch", "patch_context_mismatch", outcome, root)
	if fp1 != fp2 {
		t.Fatalf("normalized fingerprints should match\n%s\n%s", fp1, fp2)
	}
}

func TestFailureSupervisorBlocksNonRetryableDuplicate(t *testing.T) {
	s := NewFailureSupervisor("/tmp/ws", 8, 32)
	call := convcontext.ToolCall{
		ID: "c1",
		Function: convcontext.FunctionCall{
			Name:      "apply_patch",
			Arguments: `{"patch":"*** Begin Patch\nbad\n*** End Patch"}`,
		},
	}
	before := s.BeforeCall(call)
	if !before.Allow {
		t.Fatalf("first call should allow")
	}
	s.NoteRealExecution(before.CallSignature)
	failed := tool.ExecutionOutcome{
		Status:         tool.OutcomeFailed,
		SemanticStatus: tool.SemanticFailure,
		ErrorCode:      "invalid_patch_format",
		ErrorMessage:   "missing begin",
		Retryable:      false,
	}
	s.AfterOutcome(call, failed)

	second := s.BeforeCall(call)
	if !second.Block || second.ErrorCode != ErrorCodeToolRetryBlocked {
		t.Fatalf("second non-retryable duplicate must block, got %+v", second)
	}
	if len(second.RecoveryOptions) == 0 {
		t.Fatalf("expected recovery options")
	}
}

func TestFailureSupervisorAllowsRetryableTwiceThenBlocks(t *testing.T) {
	s := NewFailureSupervisor("/tmp/ws", 8, 32)
	call := convcontext.ToolCall{
		ID: "c1",
		Function: convcontext.FunctionCall{
			Name:      "shell",
			Arguments: `{"command":"false"}`,
		},
	}
	for i := 0; i < 2; i++ {
		before := s.BeforeCall(call)
		if before.Block {
			t.Fatalf("attempt %d should allow", i+1)
		}
		s.NoteRealExecution(before.CallSignature)
		s.AfterOutcome(call, tool.ExecutionOutcome{
			Status:         tool.OutcomeFailed,
			SemanticStatus: tool.SemanticFailure,
			ErrorCode:      tool.ErrorCodeProcessExitNonzero,
			ErrorMessage:   "exit 1",
			Retryable:      true,
		})
	}
	third := s.BeforeCall(call)
	if !third.Block {
		t.Fatalf("third retryable exact call must block")
	}
}

func TestShellResourceRevisionSeparatesCommandsWithoutLeakingTheirText(t *testing.T) {
	first := ResourceRevision(`{"command":"node scripts/validate.js output/slides.json","workdir":"/workspace"}`, tool.ExecutionOutcome{})
	second := ResourceRevision(`{"command":"npm install","workdir":"/workspace"}`, tool.ExecutionOutcome{})
	third := ResourceRevision(`{"command":"node scripts/validate.js output/slides.json","workdir":"/other"}`, tool.ExecutionOutcome{})
	if first == "unknown" || second == "unknown" || third == "unknown" {
		t.Fatalf("shell command revisions must be stable hashes: %q %q %q", first, second, third)
	}
	if first == second || first == third || second == third {
		t.Fatalf("different commands/cwds shared one revision: %q %q %q", first, second, third)
	}
	if strings.Contains(first, "validate") || strings.Contains(second, "npm") {
		t.Fatalf("revision leaked command text: %q %q", first, second)
	}
}

func TestFailureSupervisorAllowsShellValidationAfterObservedMutation(t *testing.T) {
	s := NewFailureSupervisor("/workspace", 8, 64)
	validate := convcontext.ToolCall{ID: "validate_1", Function: convcontext.FunctionCall{
		Name: "shell", Arguments: `{"command":"node scripts/validate.js output/slides.json","workdir":"/workspace"}`,
	}}
	before := s.BeforeCall(validate)
	if !before.Allow {
		t.Fatalf("initial validator call blocked: %+v", before)
	}
	s.NoteRealExecution(before.CallSignature)
	s.AfterOutcome(validate, tool.ExecutionOutcome{
		Status: tool.OutcomeFailed, SemanticStatus: tool.SemanticFailure,
		ErrorCode: tool.ErrorCodeProcessExitNonzero, ErrorMessage: "image path missing", Retryable: false,
	})
	if retry := s.BeforeCall(validate); !retry.Block {
		t.Fatal("unchanged non-retryable validator call should remain blocked")
	}

	edit := convcontext.ToolCall{ID: "edit_1", Function: convcontext.FunctionCall{
		Name: "text_edit", Arguments: `{"path":"output/slides.json"}`,
	}}
	s.AfterOutcome(edit, tool.ExecutionOutcome{
		Status: tool.OutcomeSucceeded, TransportStatus: tool.TransportCompleted,
		SemanticStatus: tool.SemanticSuccess, ProcessStatus: tool.ProcessNotApplicable,
		AffectedPaths: []string{"output/slides.json"},
		Operation:     &tool.OperationDescriptor{Type: "text_edit", Target: "output/slides.json", SideEffectState: tool.OutcomeSideEffectApplied},
	})
	if retry := s.BeforeCall(validate); !retry.Allow || retry.Block {
		t.Fatalf("validator stayed blocked after observed input repair: %+v", retry)
	}
}

func TestFailureSupervisorAllowsPipelineRetryAfterDirectShellInstall(t *testing.T) {
	s := NewFailureSupervisor("/workspace", 8, 64)
	pipeline := convcontext.ToolCall{ID: "pipeline_1", Function: convcontext.FunctionCall{
		Name: "shell", Arguments: `{"command":"node /Users/test/.awaken/skills/qoderwork-ppt/scripts/run-pipeline.js output/slides.json output/presentation.pptx","workdir":"/workspace"}`,
	}}
	before := s.BeforeCall(pipeline)
	if !before.Allow {
		t.Fatalf("initial pipeline call blocked: %+v", before)
	}
	s.NoteRealExecution(before.CallSignature)
	s.AfterOutcome(pipeline, tool.ExecutionOutcome{
		Status: tool.OutcomeFailed, SemanticStatus: tool.SemanticFailure,
		ErrorCode: tool.ErrorCodeProcessExitNonzero, ErrorMessage: "dom-to-pptx not found", Retryable: false,
		Operation: &tool.OperationDescriptor{Type: "shell", SideEffectState: tool.OutcomeSideEffectUnknown},
	})
	if retry := s.BeforeCall(pipeline); !retry.Block {
		t.Fatal("unchanged non-retryable pipeline should remain blocked before recovery")
	}

	install := convcontext.ToolCall{ID: "install_1", Function: convcontext.FunctionCall{
		Name: "shell", Arguments: `{"command":"npm install","workdir":"/Users/test/.awaken/skills/qoderwork-ppt"}`,
	}}
	installBefore := s.BeforeCall(install)
	if !installBefore.Allow || installBefore.Block {
		t.Fatalf("recovery install command was blocked: %+v", installBefore)
	}
	s.NoteRealExecution(installBefore.CallSignature)
	s.AfterOutcome(install, tool.ExecutionOutcome{
		Status: tool.OutcomeSucceeded, TransportStatus: tool.TransportCompleted,
		SemanticStatus: tool.SemanticSuccess, ProcessStatus: tool.ProcessExited,
		Operation: &tool.OperationDescriptor{Type: "shell", SideEffectState: tool.OutcomeSideEffectUnknown},
	})
	if retry := s.BeforeCall(pipeline); !retry.Allow || retry.Block {
		t.Fatalf("pipeline stayed blocked after successful direct Shell install: %+v", retry)
	}
}

func TestFailureSupervisorReadOnlyShellSuccessDoesNotUnlockFailedPipeline(t *testing.T) {
	s := NewFailureSupervisor("/workspace", 8, 64)
	pipeline := convcontext.ToolCall{ID: "pipeline_1", Function: convcontext.FunctionCall{
		Name: "shell", Arguments: `{"command":"node scripts/run-pipeline.js","workdir":"/workspace"}`,
	}}
	before := s.BeforeCall(pipeline)
	s.NoteRealExecution(before.CallSignature)
	s.AfterOutcome(pipeline, tool.ExecutionOutcome{
		Status: tool.OutcomeFailed, SemanticStatus: tool.SemanticFailure,
		ErrorCode: tool.ErrorCodeProcessExitNonzero, ErrorMessage: "missing dependency", Retryable: false,
	})

	inspect := convcontext.ToolCall{ID: "inspect_1", Function: convcontext.FunctionCall{
		Name: "shell", Arguments: `{"command":"pwd","workdir":"/workspace"}`,
	}}
	inspectBefore := s.BeforeCall(inspect)
	s.NoteRealExecution(inspectBefore.CallSignature)
	s.AfterOutcome(inspect, tool.ExecutionOutcome{
		Status: tool.OutcomeSucceeded, TransportStatus: tool.TransportCompleted,
		SemanticStatus: tool.SemanticSuccess, ProcessStatus: tool.ProcessExited,
		Operation: &tool.OperationDescriptor{Type: "shell", SideEffectState: tool.OutcomeSideEffectNone},
	})
	if retry := s.BeforeCall(pipeline); !retry.Block {
		t.Fatalf("read-only Shell success incorrectly unlocked failed pipeline: %+v", retry)
	}
}

func TestFailureSupervisorUnknownReadOnlyOrUnrelatedShellSuccessDoesNotUnlockFailedPipeline(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{name: "node version", command: "node --version"},
		{name: "plain curl", command: "curl https://example.invalid/health"},
		{name: "unrelated local cli", command: "awaken-doctor --version"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := NewFailureSupervisor("/workspace", 8, 64)
			pipeline := convcontext.ToolCall{ID: "pipeline_1", Function: convcontext.FunctionCall{
				Name: "shell", Arguments: `{"command":"node scripts/run-pipeline.js","workdir":"/workspace"}`,
			}}
			before := s.BeforeCall(pipeline)
			s.NoteRealExecution(before.CallSignature)
			s.AfterOutcome(pipeline, tool.ExecutionOutcome{
				Status: tool.OutcomeFailed, SemanticStatus: tool.SemanticFailure,
				ErrorCode: tool.ErrorCodeProcessExitNonzero, ErrorMessage: "missing dependency", Retryable: false,
			})
			s.pendingRecovery = &RecoveryDirective{
				ToolName:         "shell",
				ResourceRevision: ResourceRevision(pipeline.Function.Arguments, tool.ExecutionOutcome{}),
			}

			arguments, err := json.Marshal(map[string]string{"command": test.command, "workdir": "/workspace"})
			if err != nil {
				t.Fatalf("marshal arguments: %v", err)
			}
			inspect := convcontext.ToolCall{ID: "inspect_1", Function: convcontext.FunctionCall{
				Name: "shell", Arguments: string(arguments),
			}}
			inspectBefore := s.BeforeCall(inspect)
			if !inspectBefore.Allow || inspectBefore.Block {
				t.Fatalf("unrelated command was blocked: %+v", inspectBefore)
			}
			s.NoteRealExecution(inspectBefore.CallSignature)
			s.AfterOutcome(inspect, tool.ExecutionOutcome{
				Status: tool.OutcomeSucceeded, TransportStatus: tool.TransportCompleted,
				SemanticStatus: tool.SemanticSuccess, ProcessStatus: tool.ProcessExited,
				Operation: &tool.OperationDescriptor{Type: "shell", SideEffectState: tool.OutcomeSideEffectUnknown},
			})
			if retry := s.BeforeCall(pipeline); !retry.Block {
				t.Fatalf("unknown-side-effect %q incorrectly unlocked failed pipeline: %+v", test.command, retry)
			}
			if s.PendingRecovery() == nil {
				t.Fatalf("unknown-side-effect %q incorrectly cleared pending recovery", test.command)
			}
		})
	}
}

func TestShellCallHostClassifiesOnlyExplicitRecoveryMutations(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{name: "npm install", command: "npm install", want: true},
		{name: "python pip install", command: "python3 -m pip install -r requirements.txt", want: true},
		{name: "create directory", command: "mkdir -p .cache/runtime", want: true},
		{name: "explicit workspace file redirect", command: "printf ready > .awaken-ready", want: true},
		{name: "node version", command: "node --version", want: false},
		{name: "plain curl", command: "curl https://example.invalid/health", want: false},
		{name: "unrelated cli", command: "awaken-doctor --version", want: false},
		{name: "quoted install prose", command: `echo "npm install"`, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			arguments, err := json.Marshal(map[string]string{"command": test.command, "workdir": "/workspace"})
			if err != nil {
				t.Fatalf("marshal arguments: %v", err)
			}
			if got := shellCallHasHostClassifiedRecoveryMutation(string(arguments)); got != test.want {
				t.Fatalf("classification for %q = %v, want %v", test.command, got, test.want)
			}
		})
	}
}

func TestFailureSupervisorDoesNotMergeUnrelatedShellFailures(t *testing.T) {
	s := NewFailureSupervisor("/workspace", 8, 64)
	commands := []string{
		"node scripts/validate.js output/slides.json",
		"node scripts/run-pipeline.js output/slides.json output/presentation.pptx",
		"npm install",
		"cp -R ~/.awaken/skills/ppt output/ppt-runtime",
	}
	for index, command := range commands {
		call := convcontext.ToolCall{ID: fmt.Sprintf("shell_%d", index), Function: convcontext.FunctionCall{
			Name: "shell", Arguments: fmt.Sprintf(`{"command":%q,"workdir":"/workspace"}`, command),
		}}
		before := s.BeforeCall(call)
		if before.Block || !before.Allow {
			t.Fatalf("unrelated shell command %d was merged into an exhausted class: %+v", index, before)
		}
		s.NoteRealExecution(before.CallSignature)
		s.AfterOutcome(call, tool.ExecutionOutcome{
			Status: tool.OutcomeFailed, SemanticStatus: tool.SemanticFailure,
			ErrorCode: tool.ErrorCodeProcessExitNonzero, ErrorMessage: "exit 1", Retryable: false,
		})
	}
}

func TestFailureSupervisorSameErrorClassBudget(t *testing.T) {
	s := NewFailureSupervisor("/tmp/ws", 8, 64)
	for i := 0; i < maxSameClassFailures; i++ {
		call := convcontext.ToolCall{
			ID: fmt.Sprintf("c%d", i),
			Function: convcontext.FunctionCall{
				Name:      "text_edit",
				Arguments: fmt.Sprintf(`{"path":"a.txt","old_string":"x%d","new_string":"y","expected_sha256":"rev1"}`, i),
			},
		}
		before := s.BeforeCall(call)
		s.NoteRealExecution(before.CallSignature)
		after := s.AfterOutcome(call, tool.ExecutionOutcome{
			Status:         tool.OutcomeFailed,
			SemanticStatus: tool.SemanticFailure,
			ErrorCode:      "text_not_found",
			ErrorMessage:   "missing",
			Retryable:      true,
			Diagnostic:     map[string]any{"sha256": "rev1"},
		})
		if i == maxSameClassFailures-1 && !after.RecoveryRequired {
			t.Fatalf("expected recovery_required after %d same-class failures", maxSameClassFailures)
		}
	}
	fourth := convcontext.ToolCall{
		ID: "c4",
		Function: convcontext.FunctionCall{
			Name:      "text_edit",
			Arguments: `{"path":"a.txt","old_string":"fresh-shape","new_string":"y","expected_sha256":"rev1"}`,
		},
	}
	blocked := s.BeforeCall(fourth)
	if !blocked.Block || blocked.ErrorCode != ErrorCodeRepeatedFailure {
		t.Fatalf("fourth same-class call must be blocked before execution, got %+v", blocked)
	}
	if got := s.ToolExecutions(); got != maxSameClassFailures {
		t.Fatalf("blocked fourth call must not increment real executions: got %d", got)
	}
}

func TestApplyPatchTypedOutcomeReachesFailureSupervisor(t *testing.T) {
	s := NewFailureSupervisor(t.TempDir(), 8, 32)
	patchText := "*** Update File: a.txt\n@@\n-old\n+new\n*** End Patch"
	arguments, err := json.Marshal(map[string]string{"patch": patchText})
	if err != nil {
		t.Fatalf("marshal arguments: %v", err)
	}
	call := convcontext.ToolCall{
		ID: "patch_1",
		Function: convcontext.FunctionCall{
			Name:      "apply_patch",
			Arguments: string(arguments),
		},
	}
	before := s.BeforeCall(call)
	s.NoteRealExecution(before.CallSignature)
	runtime := NewToolRuntime(nil)
	result := runtime.Execute(context.Background(), call)
	outcome := result.Outcome
	if outcome.ErrorCode != applypatch.ErrorCodePatchMissingBegin || outcome.Retryable {
		t.Fatalf("ToolRuntime lost typed apply_patch outcome: result=%+v", result)
	}
	if outcome.Content == patchText {
		t.Fatalf("unexpected unchanged patch content: %+v", outcome)
	}
	s.AfterOutcome(call, outcome)
	blocked := s.BeforeCall(call)
	if !blocked.Block || blocked.ErrorCode != ErrorCodeToolRetryBlocked {
		t.Fatalf("supervisor did not consume typed apply_patch failure: %+v", blocked)
	}
}

func TestFailureSupervisorRecoveryInjectedThenClearedOnSuccess(t *testing.T) {
	cfg := AgentConfig{
		SystemPrompt: "base system",
		MaxThoughts:  8,
		ModelConfig:  ModelConfig{Model: "test-model"},
	}
	runner := NewRunner(cfg, convcontext.NewConversation(nil), NewRequestBuilder(cfg))
	failedCall := convcontext.ToolCall{
		ID: "fail_1",
		Function: convcontext.FunctionCall{
			Name:      "apply_patch",
			Arguments: `{"patch":"one","path":"a.txt"}`,
		},
	}
	failure := tool.ExecutionOutcome{
		Status:         tool.OutcomeFailed,
		SemanticStatus: tool.SemanticFailure,
		ErrorCode:      "patch_context_mismatch",
		ErrorMessage:   "context not found",
		Retryable:      true,
	}
	for i := 0; i < 2; i++ {
		before := runner.failureSupervisor.BeforeCall(failedCall)
		runner.failureSupervisor.NoteRealExecution(before.CallSignature)
		runner.failureSupervisor.AfterOutcome(failedCall, failure)
	}
	request, err := runner.prepareLLMRequest(context.Background(), nil)
	if err != nil {
		t.Fatalf("prepareLLMRequest: %v", err)
	}
	if !strings.Contains(request.SystemPrompt, `"recovery_required":true`) ||
		!strings.Contains(request.SystemPrompt, "patch_context_mismatch") {
		t.Fatalf("pending recovery was not injected: %s", request.SystemPrompt)
	}
	runner.failureSupervisor.AfterOutcome(convcontext.ToolCall{
		ID: "unrelated_1",
		Function: convcontext.FunctionCall{
			Name:      "calculator",
			Arguments: `{"expression":"1+1"}`,
		},
	}, tool.SucceededOutcome("2"))
	if runner.failureSupervisor.PendingRecovery() == nil {
		t.Fatal("unrelated successful tool cleared pending recovery")
	}

	successCall := convcontext.ToolCall{
		ID: "read_1",
		Function: convcontext.FunctionCall{
			Name:      "read_file",
			Arguments: `{"path":"a.txt"}`,
		},
	}
	runner.failureSupervisor.AfterOutcome(successCall, tool.SucceededOutcome("fresh revision"))
	if runner.failureSupervisor.PendingRecovery() != nil {
		t.Fatal("successful recovery must clear pending directive")
	}
	request, err = runner.prepareLLMRequest(context.Background(), nil)
	if err != nil {
		t.Fatalf("prepareLLMRequest after success: %v", err)
	}
	if strings.Contains(request.SystemPrompt, `"recovery_required":true`) {
		t.Fatalf("stale recovery directive remained after success: %s", request.SystemPrompt)
	}
}

func TestFailureSupervisorClassBudgetUsesStablePathWhenOutcomeHasCurrentDigest(t *testing.T) {
	s := NewFailureSupervisor(t.TempDir(), 8, 64)
	for i := 0; i < maxSameClassFailures; i++ {
		call := convcontext.ToolCall{
			ID: fmt.Sprintf("not_found_%d", i),
			Function: convcontext.FunctionCall{
				Name:      "text_edit",
				Arguments: fmt.Sprintf(`{"path":"a.txt","old_string":"missing-%d","new_string":"new"}`, i),
			},
		}
		before := s.BeforeCall(call)
		if before.Block {
			t.Fatalf("call %d blocked too early: %+v", i, before)
		}
		s.NoteRealExecution(before.CallSignature)
		s.AfterOutcome(call, tool.ExecutionOutcome{
			Status:         tool.OutcomeFailed,
			SemanticStatus: tool.SemanticFailure,
			ErrorCode:      "text_not_found",
			Retryable:      true,
			Diagnostic: map[string]any{
				"path":          "a.txt",
				"before_sha256": "actual-current-digest",
			},
		})
	}
	fourth := convcontext.ToolCall{
		ID: "not_found_4",
		Function: convcontext.FunctionCall{
			Name:      "text_edit",
			Arguments: `{"path":"a.txt","old_string":"another variant","new_string":"new"}`,
		},
	}
	decision := s.BeforeCall(fourth)
	if !decision.Block || decision.ErrorCode != ErrorCodeRepeatedFailure {
		t.Fatalf("outcome digest changed class revision key and allowed fourth failure: %+v", decision)
	}
}

func TestFailureSupervisorNewRevisionAllowsRetry(t *testing.T) {
	s := NewFailureSupervisor("/tmp/ws", 8, 64)
	for i := 0; i < maxSameClassFailures; i++ {
		call := convcontext.ToolCall{
			ID: fmt.Sprintf("old%d", i),
			Function: convcontext.FunctionCall{
				Name:      "text_edit",
				Arguments: fmt.Sprintf(`{"path":"a.txt","old_string":"x%d","new_string":"y","expected_sha256":"rev-old"}`, i),
			},
		}
		before := s.BeforeCall(call)
		s.NoteRealExecution(before.CallSignature)
		s.AfterOutcome(call, tool.ExecutionOutcome{
			Status:         tool.OutcomeFailed,
			SemanticStatus: tool.SemanticFailure,
			ErrorCode:      "stale_file_revision",
			Retryable:      true,
			Diagnostic:     map[string]any{"sha256": "rev-old"},
		})
	}
	fresh := convcontext.ToolCall{
		ID: "fresh",
		Function: convcontext.FunctionCall{
			Name:      "text_edit",
			Arguments: `{"path":"a.txt","old_string":"x","new_string":"y","expected_sha256":"rev-new"}`,
		},
	}
	before := s.BeforeCall(fresh)
	if before.Block {
		t.Fatalf("new revision must not be blocked by old class budget")
	}
	s.NoteRealExecution(before.CallSignature)
	after := s.AfterOutcome(fresh, tool.ExecutionOutcome{
		Status:         tool.OutcomeSucceeded,
		SemanticStatus: tool.SemanticSuccess,
		Diagnostic:     map[string]any{"sha256": "rev-new"},
	})
	if after.TerminateRun {
		t.Fatalf("successful recovery must not terminate")
	}
}

func TestFailureSupervisorTurnAndRunBudgets(t *testing.T) {
	s := NewFailureSupervisor("/tmp/ws", 8, 64)
	s.BeginTurn()
	for i := 0; i < maxTurnToolFailures; i++ {
		call := convcontext.ToolCall{
			ID: fmt.Sprintf("t%d", i),
			Function: convcontext.FunctionCall{
				Name:      "shell",
				Arguments: fmt.Sprintf(`{"command":"exit %d","resource_revision":"turn-%d"}`, i+2, i),
			},
		}
		before := s.BeforeCall(call)
		s.NoteRealExecution(before.CallSignature)
		after := s.AfterOutcome(call, tool.ExecutionOutcome{
			Status:         tool.OutcomeFailed,
			SemanticStatus: tool.SemanticFailure,
			ErrorCode:      tool.ErrorCodeProcessExitNonzero,
			ErrorMessage:   fmt.Sprintf("exit %d", i+2),
			Retryable:      true,
		})
		if i == maxTurnToolFailures-1 {
			if !after.TerminateTurn || after.TerminalReason != TerminalReasonRepeatedToolFailure {
				t.Fatalf("turn budget exceeded decision = %+v", after)
			}
		}
	}

	s2 := NewFailureSupervisor("/tmp/ws", 8, 64)
	for i := 0; i < maxRunToolFailures; i++ {
		if i%maxTurnToolFailures == 0 {
			s2.BeginTurn()
		}
		call := convcontext.ToolCall{
			ID: fmt.Sprintf("r%d", i),
			Function: convcontext.FunctionCall{
				Name:      "shell",
				Arguments: fmt.Sprintf(`{"command":"run-exit-%d","resource_revision":"run-%d"}`, i, i),
			},
		}
		before := s2.BeforeCall(call)
		s2.NoteRealExecution(before.CallSignature)
		after := s2.AfterOutcome(call, tool.ExecutionOutcome{
			Status:         tool.OutcomeFailed,
			SemanticStatus: tool.SemanticFailure,
			ErrorCode:      tool.ErrorCodeProcessExitNonzero,
			Retryable:      true,
		})
		if i == maxRunToolFailures-1 && (!after.TerminateRun || after.TerminalReason != TerminalReasonRepeatedToolFailure) {
			t.Fatalf("run budget exceeded decision = %+v", after)
		}
	}
}

func TestFailureSupervisorRecoveryRequiredAfterRepeatedFingerprint(t *testing.T) {
	s := NewFailureSupervisor("/tmp/ws", 8, 64)
	mk := func(id string) convcontext.ToolCall {
		return convcontext.ToolCall{
			ID: id,
			Function: convcontext.FunctionCall{
				Name:      "apply_patch",
				Arguments: `{"patch":"*** Begin Patch\n*** Update File: a.txt\n@@\n-old\n+new\n*** End Patch"}`,
			},
		}
	}
	outcome := tool.ExecutionOutcome{
		Status:         tool.OutcomeFailed,
		SemanticStatus: tool.SemanticFailure,
		ErrorCode:      "patch_context_mismatch",
		ErrorMessage:   "context not found",
		Retryable:      true,
	}
	c1 := mk("1")
	b1 := s.BeforeCall(c1)
	s.NoteRealExecution(b1.CallSignature)
	a1 := s.AfterOutcome(c1, outcome)
	if a1.RecoveryRequired {
		t.Fatalf("first failure should not require recovery yet")
	}
	// Change args slightly so call signature differs but fingerprint matches.
	c2 := convcontext.ToolCall{
		ID: "2",
		Function: convcontext.FunctionCall{
			Name:      "apply_patch",
			Arguments: `{"patch":"*** Begin Patch\n*** Update File: a.txt\n@@\n-old\n+newer\n*** End Patch"}`,
		},
	}
	b2 := s.BeforeCall(c2)
	s.NoteRealExecution(b2.CallSignature)
	a2 := s.AfterOutcome(c2, outcome)
	if !a2.RecoveryRequired || a2.Directive == nil {
		t.Fatalf("second same fingerprint must require recovery, got %+v", a2)
	}
	opts := RecoveryOptionsForError("patch_context_mismatch")
	if len(opts) == 0 || opts[0] != "read_file" {
		t.Fatalf("unexpected recovery options: %v", opts)
	}
}

type failingOutcomeTool struct {
	name      string
	errorCode string
	retryable bool
	calls     *int
}

func (t failingOutcomeTool) Name() string { return t.name }
func (t failingOutcomeTool) Description() string {
	return "test failing tool"
}
func (t failingOutcomeTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "value", Description: "value", Type: "string", Required: true},
		{Name: "resource_revision", Description: "test revision", Type: "string", Required: false},
	}
}
func (t failingOutcomeTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        t.name,
		DisplayName: t.name,
		Source:      tool.SourceBuiltin,
		Description: t.Description(),
		Annotations: tool.ToolAnnotations{ReadOnly: true},
	}
}
func (t failingOutcomeTool) Execute(context.Context, string) (string, error) {
	return "", errors.New("use ExecuteOutcome")
}
func (t failingOutcomeTool) ExecuteOutcome(ctx context.Context, input string, observer tool.ToolObserver) (tool.ExecutionOutcome, error) {
	if t.calls != nil {
		*t.calls++
	}
	now := time.Now().UTC()
	return tool.ExecutionOutcome{
		Content:         "failed",
		Status:          tool.OutcomeFailed,
		TransportStatus: tool.TransportCompleted,
		SemanticStatus:  tool.SemanticFailure,
		ProcessStatus:   tool.ProcessNotApplicable,
		ErrorCode:       t.errorCode,
		ErrorCategory:   tool.ErrorCategoryValidation,
		ErrorMessage:    "failed",
		Retryable:       t.retryable,
		StartedAt:       now,
		FinishedAt:      now,
	}, nil
}

func TestRunnerBlocksExactNonRetryableDuplicate(t *testing.T) {
	toolName := "fs_nonretry_dup_" + t.Name()
	var execCount int
	mustRegisterRunnerTool(t, failingOutcomeTool{
		name:      toolName,
		errorCode: "invalid_patch_format",
		retryable: false,
		calls:     &execCount,
	})

	args, _ := json.Marshal(map[string]string{"value": "same"})
	var calls int
	server := newRunnerStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls <= 2 {
			writeRunnerStreamToolCall(t, w, fmt.Sprintf("call_%d", calls), toolName, map[string]string{"value": "same"})
			return
		}
		writeRunnerStreamText(t, w, "should not reach")
	})
	defer server.Close()

	agent := NewAgent(testAgentConfig(server.URL, 5))
	var gotEvents []AwakenEvent
	_, err := agent.RunStreamWithEvents(context.Background(), "dup", EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))
	if err == nil {
		// May finish with text if model continues; ensure block happened.
	}
	if execCount != 1 {
		t.Fatalf("expected exactly 1 real execution, got %d", execCount)
	}
	blocked := false
	blockedIndex, blockedResultIndex := -1, -1
	for i, ev := range gotEvents {
		if ev.Type == ToolRetryBlockedEventType {
			blocked = true
			blockedIndex = i
		}
		if blockedIndex >= 0 && blockedResultIndex < 0 && ev.Type == ToolResultEventType {
			blockedResultIndex = i
		}
	}
	if !blocked {
		t.Fatalf("expected ToolRetryBlocked event, types=%v", eventTypes(gotEvents))
	}
	if blockedResultIndex <= blockedIndex {
		t.Fatalf("blocked recovery decision must precede synthetic denied result: blocked=%d result=%d types=%v", blockedIndex, blockedResultIndex, eventTypes(gotEvents))
	}
	_ = args
}

func TestA16RunnerRetryableExactCallBlockedOnThird(t *testing.T) {
	toolName := "fs_retryable_" + t.Name()
	var execCount int
	mustRegisterRunnerTool(t, failingOutcomeTool{
		name:      toolName,
		errorCode: tool.ErrorCodeProcessExitNonzero,
		retryable: true,
		calls:     &execCount,
	})

	var calls int
	server := newRunnerStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls <= 3 {
			writeRunnerStreamToolCall(t, w, fmt.Sprintf("call_%d", calls), toolName, map[string]string{"value": "same"})
			return
		}
		writeRunnerStreamText(t, w, "done")
	})
	defer server.Close()

	agent := NewAgent(testAgentConfig(server.URL, 6))
	var gotEvents []AwakenEvent
	_, _ = agent.RunStreamWithEvents(context.Background(), "retry", EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))
	if execCount != 2 {
		t.Fatalf("expected 2 real executions before block, got %d", execCount)
	}
	foundBlock := false
	resultByCall := map[string]int{}
	for i, ev := range gotEvents {
		if ev.Type == ToolResultEventType {
			resultByCall[ev.ToolCallID] = i
		}
		if ev.Type == ToolRecoveryRequiredEventType {
			if resultIndex, ok := resultByCall[ev.ToolCallID]; !ok || resultIndex >= i {
				t.Fatalf("real recovery directive appeared before its tool result: call=%s types=%v", ev.ToolCallID, eventTypes(gotEvents))
			}
		}
		if ev.Type == ToolRetryBlockedEventType {
			foundBlock = true
		}
	}
	if !foundBlock {
		t.Fatalf("expected ToolRetryBlocked, types=%v", eventTypes(gotEvents))
	}
}

func TestRunnerSwitchToTextEditSucceedsAfterPatchFailures(t *testing.T) {
	patchName := "fs_patch_" + t.Name()
	editName := "fs_edit_" + t.Name()
	var patchCalls, editCalls int
	mustRegisterRunnerTool(t, failingOutcomeTool{
		name:      patchName,
		errorCode: "patch_context_mismatch",
		retryable: true,
		calls:     &patchCalls,
	})
	mustRegisterRunnerTool(t, runnerTestTool{name: editName})

	var calls int
	server := newRunnerStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch calls {
		case 1, 2:
			writeRunnerStreamToolCall(t, w, fmt.Sprintf("patch_%d", calls), patchName, map[string]string{"value": fmt.Sprintf("p%d", calls)})
		case 3:
			writeRunnerStreamToolCall(t, w, "edit_1", editName, map[string]string{"value": "fixed"})
		default:
			writeRunnerStreamText(t, w, "recovered")
		}
	})
	defer server.Close()

	agent := NewAgent(testAgentConfig(server.URL, 8))
	answer, err := agent.RunStreamWithEvents(context.Background(), "recover", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if answer != "recovered" {
		t.Fatalf("expected recovered answer, got %q", answer)
	}
	if patchCalls != 2 || editCalls != 0 {
		// edit tool is runnerTestTool which increments via Execute; check messages instead.
	}
	if patchCalls != 2 {
		t.Fatalf("expected 2 patch failures, got %d", patchCalls)
	}
	var sawEdit bool
	for _, msg := range agent.GetConversation().Context.GetMessages() {
		if msg.Role == convcontext.RoleAssistant {
			for _, tc := range msg.ToolCalls {
				if tc.Function.Name == editName {
					sawEdit = true
				}
			}
		}
		if msg.Role == convcontext.RoleTool && strings.Contains(msg.Content, "echo:") {
			sawEdit = true
		}
	}
	if !sawEdit {
		t.Fatalf("expected text_edit-style recovery call in conversation")
	}
}

func TestRunnerTurnBudgetTerminatesRun(t *testing.T) {
	toolName := "fs_budget_" + t.Name()
	var execCount int
	mustRegisterRunnerTool(t, failingOutcomeTool{
		name:      toolName,
		errorCode: tool.ErrorCodeProcessExitNonzero,
		retryable: true,
		calls:     &execCount,
	})

	server := newRunnerStreamServer(t, func(w http.ResponseWriter, r *http.Request) {
		// One turn with enough distinct failing tool calls to hit the turn budget.
		calls := make([]map[string]interface{}, 0, maxTurnToolFailures)
		for i := 0; i < maxTurnToolFailures; i++ {
			args, _ := json.Marshal(map[string]string{
				"value":             fmt.Sprintf("v%d", i),
				"resource_revision": fmt.Sprintf("budget-%d", i),
			})
			calls = append(calls, map[string]interface{}{
				"index": i,
				"id":    fmt.Sprintf("b_%d", i),
				"type":  "function",
				"function": map[string]interface{}{
					"name":      toolName,
					"arguments": string(args),
				},
			})
		}
		writeRunnerStreamEvent(t, w, map[string]interface{}{
			"choices": []map[string]interface{}{
				{"delta": map[string]interface{}{"tool_calls": calls}},
			},
		})
		writeRunnerStreamLines(t, w, []string{"data: [DONE]"})
	})
	defer server.Close()

	agent := NewAgent(testAgentConfig(server.URL, 20))
	var gotEvents []AwakenEvent
	_, err := agent.RunStreamWithEvents(context.Background(), "budget", EventSinkFunc(func(event AwakenEvent) {
		gotEvents = append(gotEvents, event)
	}))
	if err == nil {
		t.Fatalf("expected repeated_tool_failure termination")
	}
	if !strings.Contains(err.Error(), "repeated_tool_failure") && !strings.Contains(err.Error(), "budget") {
		t.Fatalf("unexpected error: %v", err)
	}
	runErrorIndex := firstEventIndex(gotEvents, RunErrorEventType)
	if runErrorIndex < 0 || gotEvents[runErrorIndex].TerminalReason != TerminalReasonRepeatedToolFailure {
		t.Fatalf("expected repeated_tool_failure terminal, got %+v", gotEvents)
	}
	if execCount != maxTurnToolFailures {
		t.Fatalf("expected %d executions, got %d", maxTurnToolFailures, execCount)
	}
}

func TestRunnerZeroMaxThoughtsUsesProductDefault(t *testing.T) {
	if got := EffectiveMaxAgentTurns(0); got != DefaultMaxAgentTurns {
		t.Fatalf("0 must map to default %d, got %d", DefaultMaxAgentTurns, got)
	}
	cfg := testAgentConfig("http://example.invalid", 0)
	runner := NewRunner(cfg, convcontext.NewConversation(nil), NewRequestBuilder(cfg))
	if runner.effectiveMaxThoughts() != DefaultMaxAgentTurns {
		t.Fatalf("runner effective max thoughts = %d", runner.effectiveMaxThoughts())
	}
	if runner.failureSupervisor.MaxToolExecutions() != DefaultMaxToolExecutions {
		t.Fatalf("runner max tool executions = %d", runner.failureSupervisor.MaxToolExecutions())
	}
}
