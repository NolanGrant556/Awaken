package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
	"awaken/src/tool/profile"
)

func TestToolRuntimeEmitsArtifactsForAllowedTool(t *testing.T) {
	toolName := "runtime_allowed_artifact_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:       toolName,
		permission: tool.AllowPermission(),
		artifacts: []tool.ArtifactDescriptor{{
			ContractVersion: tool.ArtifactContractVersion,
			Role:            tool.ArtifactRoleDeliverable,
			Producer:        toolName,
			Action:          tool.ArtifactActionCreated,
			ID:              "artifact-1",
			Title:           "README.md",
			Description:     "Generated readme",
			Kind:            "file",
			Path:            "/tmp/README.md",
			Content:         "# hello",
			Status:          "ready",
		}},
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	var got []AwakenEvent
	runtime := NewToolRuntime(EventSinkFunc(func(event AwakenEvent) {
		got = append(got, event)
	}))
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_allowed_artifact",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected allowed tool to succeed, got %+v", result)
	}
	if len(got) != 4 {
		t.Fatalf("expected decision, progress, and artifact events, got %+v", got)
	}
	if got[0].Type != ToolPermissionDecidedEventType ||
		got[1].Type != ToolProgressEventType ||
		got[1].ToolProgressStage != "started" ||
		got[2].Type != ToolProgressEventType ||
		got[2].ToolProgressStage != "completed" ||
		got[3].Type != ArtifactCreatedEventType {
		t.Fatalf("unexpected emitted events: %+v", got)
	}
	if got[1].ToolStatus != "" || got[2].ToolStatus != "" {
		t.Fatalf("progress events must not claim canonical tool outcomes: %+v", got)
	}
	if got[3].ArtifactContractVersion != tool.ArtifactContractVersion ||
		got[3].ArtifactRole != tool.ArtifactRoleDeliverable ||
		got[3].ArtifactProducer != toolName {
		t.Fatalf("artifact v2 metadata missing: %+v", got[3])
	}
	if len(result.Outcome.Artifacts) != 1 || result.Outcome.Artifacts[0].ID != "artifact-1" {
		t.Fatalf("canonical outcome must carry emitted artifacts: %+v", result.Outcome.Artifacts)
	}
	if result.Outcome.Operation == nil || result.Outcome.Operation.SideEffectState != tool.OutcomeSideEffectApplied {
		t.Fatalf("artifact production must be an applied side effect: %+v", result.Outcome.Operation)
	}
}

func TestToolRuntimeTransformedTypedFailureDoesNotEmitArtifact(t *testing.T) {
	sourceName := "runtime_transform_failure_source"
	targetName := "runtime_transform_failure_target"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:       sourceName,
		permission: tool.AllowPermission(),
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})
	mustRegisterRuntimeTool(t, runtimeTypedFailureArtifactTool{runtimeTestTool: runtimeTestTool{
		name:       targetName,
		permission: tool.AllowPermission(),
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
		artifacts: []tool.ArtifactDescriptor{{
			Action: tool.ArtifactActionUpdated,
			ID:     "must-not-surface",
			Title:  "unchanged.txt",
			Kind:   "file",
			Path:   "unchanged.txt",
			Status: "ready",
		}},
	}})

	var events []AwakenEvent
	runtime := NewToolRuntimeWithConfig(EventSinkFunc(func(event AwakenEvent) {
		events = append(events, event)
	}), ToolRuntimeConfig{
		AutoToolPolicy: staticAutoToolPolicy{decision: ToolDecision{
			Action:          ToolDecisionTransform,
			Source:          ToolDecisionSourceTransform,
			Reason:          "structured rewrite",
			UpdatedToolName: targetName,
			UpdatedInput:    `{"value":"rewritten"}`,
		}},
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_transform_failure",
		Function: convcontext.FunctionCall{
			Name:      sourceName,
			Arguments: `{"value":"original"}`,
		},
	})

	if !result.IsError || result.Outcome.Status != tool.OutcomeFailed || result.Outcome.ErrorCode != "typed_transform_failure" {
		t.Fatalf("expected typed transformed failure, got %+v", result)
	}
	for _, event := range events {
		if event.Type == ArtifactCreatedEventType || event.Type == ArtifactUpdatedEventType {
			t.Fatalf("transformed typed failure emitted false artifact: %+v", event)
		}
	}
}

func TestToolRuntimeStrictOutputVerificationPrecedesTerminalPublication(t *testing.T) {
	toolName := "runtime_strict_output_gate_direct"
	outcome := tool.SucceededOutcome("created report")
	outcome.Details = map[string]any{"unexpected": true}
	mustRegisterRuntimeTool(t, runtimeStrictOutputArtifactTool{runtimeOutcomeTestTool{
		runtimeTestTool: runtimeTestTool{
			name: toolName, permission: tool.AllowPermission(),
			artifacts: []tool.ArtifactDescriptor{{
				Action: tool.ArtifactActionCreated, ID: "must-not-publish", Kind: "file", Path: "report.md", Status: "ready",
			}},
		},
		outcome: outcome,
	}})
	registered, err := tool.GetRegisteredTool(toolName)
	if err != nil || registered.Quality != tool.DefinitionQualityStrict {
		t.Fatalf("test tool must exercise strict verification: registered=%+v err=%v", registered, err)
	}

	var events []AwakenEvent
	audit := &tool.InMemorySink{}
	result := NewToolRuntimeWithConfig(EventSinkFunc(func(event AwakenEvent) {
		events = append(events, event)
	}), ToolRuntimeConfig{AuditSink: audit}).Execute(context.Background(), convcontext.ToolCall{
		ID:       "call_strict_output_gate_direct",
		Function: convcontext.FunctionCall{Name: toolName, Arguments: `{"value":"report"}`},
	})

	assertStrictOutputFailureHasNoPublication(t, result, events, audit.Records())
}

func TestToolRuntimeTransformedStrictOutputVerificationPrecedesTerminalPublication(t *testing.T) {
	sourceName := "runtime_strict_output_gate_source"
	targetName := "runtime_strict_output_gate_target"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name: sourceName, permission: tool.AllowPermission(),
		params: []tool.ToolParameter{{Name: "value", Type: "string", Required: true}},
	})
	outcome := tool.SucceededOutcome("created transformed report")
	outcome.Details = map[string]any{"unexpected": true}
	mustRegisterRuntimeTool(t, runtimeStrictOutputArtifactTool{runtimeOutcomeTestTool{
		runtimeTestTool: runtimeTestTool{
			name: targetName, permission: tool.AllowPermission(),
			artifacts: []tool.ArtifactDescriptor{{
				Action: tool.ArtifactActionCreated, ID: "must-not-publish-transform", Kind: "file", Path: "transform.md", Status: "ready",
			}},
		},
		outcome: outcome,
	}})

	var events []AwakenEvent
	audit := &tool.InMemorySink{}
	runtime := NewToolRuntimeWithConfig(EventSinkFunc(func(event AwakenEvent) {
		events = append(events, event)
	}), ToolRuntimeConfig{
		AuditSink: audit,
		AutoToolPolicy: staticAutoToolPolicy{decision: ToolDecision{
			Action: ToolDecisionTransform, Source: ToolDecisionSourceTransform, Reason: "strict output gate",
			UpdatedToolName: targetName, UpdatedInput: `{"value":"report"}`,
		}},
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID:       "call_strict_output_gate_transform",
		Function: convcontext.FunctionCall{Name: sourceName, Arguments: `{"value":"original"}`},
	})

	assertStrictOutputFailureHasNoPublication(t, result, events, audit.Records())
}

func assertStrictOutputFailureHasNoPublication(
	t *testing.T,
	result ToolExecutionResult,
	events []AwakenEvent,
	records []tool.ToolAuditRecord,
) {
	t.Helper()
	if !result.IsError || result.Outcome.Status != tool.OutcomeFailed || result.Outcome.ErrorCode != "tool_output_schema_mismatch" {
		t.Fatalf("strict output mismatch must fail closed: %+v", result)
	}
	if len(result.Outcome.Errors) == 0 || result.Outcome.Errors[0].Stage != tool.ErrorStageVerification {
		t.Fatalf("strict output mismatch must identify verification stage: %+v", result.Outcome.Errors)
	}
	for _, event := range events {
		if event.Type == ArtifactCreatedEventType || event.Type == ArtifactUpdatedEventType {
			t.Fatalf("invalid output published an artifact: %+v", event)
		}
		if event.Type == ToolProgressEventType && event.ToolProgressStage == "completed" {
			t.Fatalf("invalid output emitted a completed event: %+v", event)
		}
	}
	foundFailedProgress := false
	for _, event := range events {
		if event.Type == ToolProgressEventType && event.ToolProgressStage == "failed" {
			foundFailedProgress = true
		}
	}
	if !foundFailedProgress {
		t.Fatalf("strict output mismatch did not emit failure progress: %+v", events)
	}
	if len(records) != 1 || records[0].Outcome != string(tool.OutcomeFailed) || records[0].ErrorReason == "" {
		t.Fatalf("strict output mismatch must produce only a failed audit record: %+v", records)
	}
}

func TestToolRuntimeGoErrorOverridesSucceededOutcome(t *testing.T) {
	toolName := "runtime_success_outcome_with_go_error_direct"
	outcome := tool.SucceededOutcome("reported success")
	outcome.Summary = "reported success"
	mustRegisterRuntimeTool(t, runtimeOutcomeTestTool{
		runtimeTestTool: runtimeTestTool{
			name: toolName, permission: tool.AllowPermission(),
			artifacts: []tool.ArtifactDescriptor{{Action: tool.ArtifactActionCreated, ID: "false-success", Kind: "file", Path: "false.txt", Status: "ready"}},
		},
		outcome: outcome, executeErr: errors.New("implementation transport broke"),
	})

	var events []AwakenEvent
	result := NewToolRuntime(EventSinkFunc(func(event AwakenEvent) {
		events = append(events, event)
	})).Execute(context.Background(), convcontext.ToolCall{
		ID:       "call_success_outcome_with_go_error_direct",
		Function: convcontext.FunctionCall{Name: toolName, Arguments: `{}`},
	})
	assertGoErrorOverridesSuccess(t, result, events)
}

func TestToolRuntimeTransformedGoErrorOverridesSucceededOutcome(t *testing.T) {
	sourceName := "runtime_success_outcome_with_go_error_source"
	targetName := "runtime_success_outcome_with_go_error_target"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name: sourceName, permission: tool.AllowPermission(),
		params: []tool.ToolParameter{{Name: "value", Type: "string", Required: true}},
	})
	mustRegisterRuntimeTool(t, runtimeOutcomeTestTool{
		runtimeTestTool: runtimeTestTool{
			name: targetName, permission: tool.AllowPermission(),
			params:    []tool.ToolParameter{{Name: "value", Type: "string", Required: true}},
			artifacts: []tool.ArtifactDescriptor{{Action: tool.ArtifactActionCreated, ID: "false-transform-success", Kind: "file", Path: "false-transform.txt", Status: "ready"}},
		},
		outcome: tool.SucceededOutcome("reported transformed success"), executeErr: errors.New("transformed implementation transport broke"),
	})

	var events []AwakenEvent
	runtime := NewToolRuntimeWithConfig(EventSinkFunc(func(event AwakenEvent) {
		events = append(events, event)
	}), ToolRuntimeConfig{AutoToolPolicy: staticAutoToolPolicy{decision: ToolDecision{
		Action: ToolDecisionTransform, Source: ToolDecisionSourceTransform, Reason: "go error gate",
		UpdatedToolName: targetName, UpdatedInput: `{"value":"rewritten"}`,
	}}})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID:       "call_success_outcome_with_go_error_transform",
		Function: convcontext.FunctionCall{Name: sourceName, Arguments: `{"value":"original"}`},
	})
	assertGoErrorOverridesSuccess(t, result, events)
}

func assertGoErrorOverridesSuccess(t *testing.T, result ToolExecutionResult, events []AwakenEvent) {
	t.Helper()
	if !result.IsError || result.Outcome.Status != tool.OutcomeFailed || result.Outcome.TransportStatus != tool.TransportFailed ||
		result.Outcome.ErrorCode != tool.ErrorCodeToolExecutionFailed || result.Outcome.Retryable {
		t.Fatalf("Go implementation error must override a succeeded outcome: %+v", result)
	}
	if !strings.Contains(result.Outcome.ErrorMessage, "transport broke") {
		t.Fatalf("Go error cause was not preserved: %+v", result.Outcome)
	}
	for _, event := range events {
		if event.Type == ArtifactCreatedEventType || event.Type == ArtifactUpdatedEventType ||
			(event.Type == ToolProgressEventType && event.ToolProgressStage == "completed") {
			t.Fatalf("Go-error conflict leaked a success terminal signal: %+v", event)
		}
	}
}

func TestToolRuntimeEmitsCommandOutputFromObservableTool(t *testing.T) {
	toolName := "runtime_observable_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name: toolName,
		commandOutputs: []tool.CommandOutput{
			{Stream: "stdout", Delta: "hello"},
			{Stream: "stderr", Delta: "warn", Truncated: true},
		},
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	var got []AwakenEvent
	result := NewToolRuntime(EventSinkFunc(func(event AwakenEvent) {
		got = append(got, event)
	})).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_observable",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected success, got %+v", result)
	}
	if len(got) != 5 {
		t.Fatalf("expected permission and two command output events, got %+v", got)
	}
	if got[0].Type != ToolPermissionDecidedEventType || got[0].ToolDecisionAction != string(ToolDecisionAllow) {
		t.Fatalf("unexpected permission event: %+v", got[0])
	}
	if got[1].Type != ToolProgressEventType || got[1].ToolProgressStage != "started" {
		t.Fatalf("unexpected start progress event: %+v", got[1])
	}
	if got[2].Type != CommandOutputEventType || got[2].ToolCallID != "call_observable" || got[2].CommandStream != "stdout" || got[2].Delta != "hello" {
		t.Fatalf("unexpected stdout event: %+v", got[2])
	}
	if got[3].Type != CommandOutputEventType || got[3].CommandStream != "stderr" || got[3].Delta != "warn" || !got[3].Truncated {
		t.Fatalf("unexpected stderr event: %+v", got[3])
	}
	if got[4].Type != ToolProgressEventType || got[4].ToolProgressStage != "completed" {
		t.Fatalf("unexpected completed progress event: %+v", got[4])
	}
}

func TestToolRuntimePublishesVerifiedArtifactsFromTypedPartialOutcome(t *testing.T) {
	toolName := "runtime_partial_artifact_tool"
	outcome := tool.FailedOutcome("one sibling failed", "media_generate_partial", tool.ErrorCategoryRuntime, false)
	outcome.TransportStatus = tool.TransportCompleted
	outcome.Artifacts = []tool.ArtifactDescriptor{{
		ContractVersion: tool.ArtifactContractVersion,
		Role:            tool.ArtifactRoleDeliverable, Producer: toolName, Action: tool.ArtifactActionCreated,
		ID: "partial-ready", Title: "Ready image", Kind: "image", Path: "output/ready.png", Status: "ready",
	}}
	outcome.Operation = &tool.OperationDescriptor{Type: toolName, SideEffectState: tool.OutcomeSideEffectPartial}
	mustRegisterRuntimeTool(t, runtimeOutcomeTestTool{
		runtimeTestTool: runtimeTestTool{name: toolName, permission: tool.AllowPermission()},
		outcome:         outcome,
	})

	var events []AwakenEvent
	result := NewToolRuntime(EventSinkFunc(func(event AwakenEvent) {
		events = append(events, event)
	})).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_partial_artifact", Function: convcontext.FunctionCall{Name: toolName, Arguments: `{}`},
	})
	if !result.IsError || len(result.Outcome.Artifacts) != 1 {
		t.Fatalf("typed partial outcome lost artifact: %+v", result)
	}
	created := 0
	for _, event := range events {
		if event.Type == ArtifactCreatedEventType && event.ArtifactID == "partial-ready" {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("expected exactly one durable-ready artifact event, got %d: %+v", created, events)
	}
}

func TestToolRuntimeExecutesWriteWithoutApproval(t *testing.T) {
	root := t.TempDir()

	var got []AwakenEvent
	runtime := NewToolRuntimeWithConfig(EventSinkFunc(func(event AwakenEvent) {
		got = append(got, event)
	}), ToolRuntimeConfig{
		PermissionContext: tool.NewPermissionContext(root, []string{root}),
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_write",
		Function: convcontext.FunctionCall{
			Name:      "Write",
			Arguments: `{"file_path":"note.md","content":"hello\n"}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected Write success, got %+v", result)
	}
	if content, err := os.ReadFile(filepath.Join(root, "note.md")); err != nil || string(content) != "hello\n" {
		t.Fatalf("Write result=%q err=%v", content, err)
	}
	for _, event := range got {
		if event.Type == AuthRequiredEventType {
			t.Fatalf("expected no auth event on Write, got %+v", event)
		}
	}
}

func TestToolRuntimeDeniesBeforeExecution(t *testing.T) {
	var observed string
	toolName := "runtime_denied_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:       toolName,
		observe:    &observed,
		permission: tool.DenyPermission("policy denied test tool"),
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	result := NewToolRuntime(nil).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_denied",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if !result.IsError || !strings.Contains(result.Content, "policy denied test tool") {
		t.Fatalf("expected policy denial, got %+v", result)
	}
	if observed != "" {
		t.Fatalf("expected denied tool not to execute, observed %q", observed)
	}
}

func TestToolRuntimePassesPermissionContextToTools(t *testing.T) {
	var observed tool.PermissionContext
	root := t.TempDir()
	writable := t.TempDir()
	toolName := "runtime_policy_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:          toolName,
		observePolicy: &observed,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	runtime := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		PermissionContext: tool.NewPermissionContext(root, []string{writable}),
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_policy",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected tool to execute, got %+v", result)
	}
	if observed.WorkspaceRoot != root || len(observed.WritableRoots) != 1 || observed.WritableRoots[0] != writable {
		t.Fatalf("unexpected permission context: %+v", observed)
	}
}

func TestToolRuntimeNeedsAuthEmitsAuthRequiredAndDoesNotExecute(t *testing.T) {
	var executeCount int
	toolName := "runtime_needs_auth_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:         toolName,
		permission:   tool.NeedsAuthPermission("outlook account is not connected"),
		executeCount: &executeCount,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	var got []AwakenEvent
	result := NewToolRuntime(EventSinkFunc(func(event AwakenEvent) {
		got = append(got, event)
	})).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_needs_auth",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if executeCount != 0 {
		t.Fatalf("expected needs_auth tool not to execute, got %d executions", executeCount)
	}
	if !result.IsError || result.ErrorCode != "needs_auth" {
		t.Fatalf("expected needs_auth result, got %+v", result)
	}
	if !result.Outcome.Retryable || len(result.Outcome.Errors) == 0 ||
		result.Outcome.Errors[0].RetryStrategy != tool.RetryRequestAuthorization ||
		len(result.Outcome.NextActions) == 0 {
		t.Fatalf("needs_auth recovery facts were lost: %+v", result.Outcome)
	}
	if len(got) != 3 ||
		got[0].Type != ToolPermissionDecidedEventType ||
		got[0].ToolDecisionAction != string(ToolDecisionNeedsAuth) ||
		got[1].Type != ToolProgressEventType ||
		got[1].ToolProgressStage != "needs_auth" ||
		got[2].Type != AuthRequiredEventType {
		t.Fatalf("unexpected needs_auth events: %+v", got)
	}
}

func TestToolRuntimeNeedsRuntimePreservesRecoveryDecision(t *testing.T) {
	var executeCount int
	toolName := "runtime_needs_runtime_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:         toolName,
		permission:   tool.NeedsRuntimePermission("primary runtime is missing"),
		executeCount: &executeCount,
	})

	result := NewToolRuntime(nil).Execute(context.Background(), convcontext.ToolCall{
		ID: "call_needs_runtime", Function: convcontext.FunctionCall{Name: toolName, Arguments: `{}`},
	})
	if executeCount != 0 || result.Outcome.Status != tool.OutcomeNeedsRuntime {
		t.Fatalf("needs_runtime must stop before execution: %+v", result)
	}
	if !result.Outcome.Retryable || len(result.Outcome.Errors) == 0 ||
		result.Outcome.Errors[0].RetryStrategy != tool.RetryPrepareRuntime ||
		len(result.Outcome.NextActions) == 0 {
		t.Fatalf("needs_runtime recovery facts were lost: %+v", result.Outcome)
	}
}

type staticAutoToolPolicy struct {
	decision ToolDecision
}

func (p staticAutoToolPolicy) Decide(context.Context, ToolDecisionRequest) ToolDecision {
	return p.decision
}

func (p staticAutoToolPolicy) DecideExecution(_ context.Context, intent profile.ExecutionIntent, _ ToolDecisionRequest) profile.ExecutionDecision {
	return toolDecisionToExecutionDecision(p.decision, intent)
}

func mustRegisterRuntimeTool(t *testing.T, candidate tool.Tool) {
	t.Helper()
	if err := tool.RegisterTool(candidate); err != nil {
		t.Fatalf("failed to register test tool %s: %v", candidate.Name(), err)
	}
}

func mustJSONQuote(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("failed to quote JSON string: %v", err)
	}
	return string(data)
}

type userQuestionTestSink struct {
	events []AwakenEvent
	answer string
	err    error
}

func (s *userQuestionTestSink) Emit(event AwakenEvent) {
	s.events = append(s.events, event)
}

func (s *userQuestionTestSink) AwaitUserQuestion(ctx context.Context, request AwakenEvent) (string, error) {
	s.Emit(request)
	return s.answer, s.err
}

func TestToolRuntimeInterceptsAskUser(t *testing.T) {
	sink := &userQuestionTestSink{answer: "Dark"}
	result := NewToolRuntime(sink).Execute(context.Background(), convcontext.ToolCall{
		ID:   "call_ask_1",
		Type: "function",
		Function: convcontext.FunctionCall{
			Name:      "ask_user",
			Arguments: `{"questions_json": "[{\"question\":\"Choose a theme\",\"options\":[{\"label\":\"Dark\"},{\"label\":\"Light\"}]}]"}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected success, got %+v", result)
	}
	if result.ToolCallID != "call_ask_1" || result.Name != "ask_user" {
		t.Fatalf("unexpected result: %+v", result)
	}
	expected := `User has answered your questions: "Choose a theme"="Dark"`
	if result.Content != expected {
		t.Fatalf("expected %q, got %q", expected, result.Content)
	}
	var found bool
	for _, event := range sink.events {
		if event.Type == UserQuestionRequestedType {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected UserQuestionRequested event, got %+v", sink.events)
	}
}

func TestToolRuntimeAskUserReturnsErrorWhenNoAwaiter(t *testing.T) {
	result := NewToolRuntime(EventSinkFunc(func(event AwakenEvent) {})).Execute(context.Background(), convcontext.ToolCall{
		ID:   "call_ask_2",
		Type: "function",
		Function: convcontext.FunctionCall{
			Name:      "ask_user",
			Arguments: `{"questions_json": "[{\"question\":\"Choose a theme\",\"options\":[{\"label\":\"Dark\"},{\"label\":\"Light\"}]}]"}`,
		},
	})

	if !result.IsError {
		t.Fatalf("expected error result")
	}
	if !strings.Contains(result.Content, "不支持用户提问") {
		t.Fatalf("expected unsupported message, got %q", result.Content)
	}
	if result.Outcome.Status != tool.OutcomeUnsupported || result.Outcome.TransportStatus != tool.TransportNotStarted ||
		result.Outcome.ErrorCategory != tool.ErrorCategoryUnsupported || result.Outcome.Retryable {
		t.Fatalf("missing awaiter must be an unsupported precondition, got %+v", result.Outcome)
	}
	if len(result.Outcome.Errors) == 0 || result.Outcome.Errors[0].RetryStrategy != tool.RetrySwitchTool {
		t.Fatalf("missing awaiter must route to an interactive runtime, got %+v", result.Outcome.Errors)
	}
}

func TestToolRuntimeAskUserPreservesMultiQuestionAnswer(t *testing.T) {
	answer := "Choose a theme: Dark\nPick output: PDF"
	sink := &userQuestionTestSink{answer: answer}
	result := NewToolRuntime(sink).Execute(context.Background(), convcontext.ToolCall{
		ID:   "call_ask_multi",
		Type: "function",
		Function: convcontext.FunctionCall{
			Name: "ask_user",
			Arguments: `{"questions_json": "{\"questions\":[` +
				`{\"question\":\"Choose a theme\",\"options\":[{\"label\":\"Dark\"},{\"label\":\"Light\"}]},` +
				`{\"question\":\"Pick output\",\"options\":[{\"label\":\"PDF\"},{\"label\":\"DOCX\"}]}` +
				`]}"}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected success, got %+v", result)
	}
	expected := "User has answered your questions:\n" + answer
	if result.Content != expected {
		t.Fatalf("expected %q, got %q", expected, result.Content)
	}
}

func TestToolRuntimeAskUserReturnsErrorForInvalidInput(t *testing.T) {
	sink := &userQuestionTestSink{answer: "Dark"}
	result := NewToolRuntime(sink).Execute(context.Background(), convcontext.ToolCall{
		ID:   "call_ask_3",
		Type: "function",
		Function: convcontext.FunctionCall{
			Name:      "ask_user",
			Arguments: `{"questions_json": "not-json"}`,
		},
	})

	if !result.IsError {
		t.Fatalf("expected error result")
	}
	if !strings.Contains(result.Content, "提问格式错误") {
		t.Fatalf("expected validation message, got %q", result.Content)
	}
	if result.Outcome.TransportStatus != tool.TransportNotStarted || result.Outcome.ErrorCategory != tool.ErrorCategoryValidation ||
		result.Outcome.Retryable || len(result.Outcome.Errors) == 0 || result.Outcome.Errors[0].RetryStrategy != tool.RetryChangeInput {
		t.Fatalf("invalid question structure must fail before execution and require changed input: %+v", result.Outcome)
	}
}

func TestToolRuntimeAskUserClassifiesAwaitInterruption(t *testing.T) {
	t.Run("interactive transport failure", func(t *testing.T) {
		sink := &userQuestionTestSink{err: errors.New("question channel closed")}
		result := NewToolRuntime(sink).Execute(context.Background(), convcontext.ToolCall{
			ID: "call_ask_interrupted", Function: convcontext.FunctionCall{
				Name:      "ask_user",
				Arguments: `{"questions_json": "[{\"question\":\"Choose a theme\",\"options\":[{\"label\":\"Dark\"},{\"label\":\"Light\"}]}]"}`,
			},
		})
		if result.Outcome.Status != tool.OutcomeInterrupted || result.Outcome.TransportStatus != tool.TransportFailed ||
			result.Outcome.ErrorCategory != tool.ErrorCategoryRuntime || result.Outcome.Operation.SideEffectState != tool.OutcomeSideEffectNone {
			t.Fatalf("awaiter failure must reflect an interrupted in-flight request without side effects: %+v", result.Outcome)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		sink := &userQuestionTestSink{err: context.Canceled}
		result := NewToolRuntime(sink).Execute(ctx, convcontext.ToolCall{
			ID: "call_ask_cancelled", Function: convcontext.FunctionCall{
				Name:      "ask_user",
				Arguments: `{"questions_json": "[{\"question\":\"Choose a theme\",\"options\":[{\"label\":\"Dark\"},{\"label\":\"Light\"}]}]"}`,
			},
		})
		if result.Outcome.Status != tool.OutcomeCancelled || result.Outcome.TransportStatus != tool.TransportCancelled ||
			result.Outcome.ErrorCategory != tool.ErrorCategoryCancelled || result.Status != ToolExecutionCancelled {
			t.Fatalf("context cancellation must remain cancelled: %+v", result)
		}
	})
}
