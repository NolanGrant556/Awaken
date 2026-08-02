package delegateexternalagent

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"awaken/src/acp"
	"awaken/src/tool"
)

type outputContractClient struct {
	promptCalls atomic.Int32
	promptErr   error
}

func (c *outputContractClient) Initialize(context.Context) (acp.AgentCapabilitiesSnapshot, error) {
	return acp.AgentCapabilitiesSnapshot{ProtocolVersion: 1, SupportsTextPrompt: true}, nil
}

func (c *outputContractClient) NewSession(context.Context, string) (string, error) {
	return "acp-session-contract", nil
}

func (c *outputContractClient) Prompt(context.Context, string, string, string, acp.UpdateHandler, acp.PermissionHandler) (acp.TurnResult, error) {
	c.promptCalls.Add(1)
	if c.promptErr != nil {
		return acp.TurnResult{SideEffectState: "unknown"}, c.promptErr
	}
	return acp.TurnResult{
		StopReason:    "end_turn",
		AssistantText: "contract response",
		ToolSummary:   acp.ToolSummary{Completed: 1, Total: 1},
	}, nil
}

func (c *outputContractClient) Cancel(context.Context, string) error       { return nil }
func (c *outputContractClient) CloseSession(context.Context, string) error { return nil }
func (c *outputContractClient) Close() error                               { return nil }

func TestDelegateExternalAgentImplementsOutcomeTool(t *testing.T) {
	var _ tool.OutcomeTool = (*DelegateExternalAgentTool)(nil)
}

func TestExecuteOutcomeClassifiesInvalidInput(t *testing.T) {
	task := &DelegateExternalAgentTool{}
	outcome, err := task.ExecuteOutcome(context.Background(), `{`, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome returned transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeFailed || outcome.ErrorCode != "acp_delegate_invalid_input" {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
	if outcome.Retryable {
		t.Fatal("invalid input must not be retryable")
	}
}

func TestExecuteOutcomeClassifiesMissingRuntime(t *testing.T) {
	task := &DelegateExternalAgentTool{}
	outcome, err := task.ExecuteOutcome(context.Background(), `{"action":"list"}`, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome returned transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeFailed || outcome.ErrorCode != acp.ErrCodeRunnerNotReady {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
	if !outcome.Retryable {
		t.Fatal("missing ACP runtime must be retryable")
	}
	if !strings.Contains(outcome.ErrorMessage, "manager not available") {
		t.Fatalf("unexpected error message: %q", outcome.ErrorMessage)
	}
}

func TestACPUpdateHandlerEmitsToolSnapshot(t *testing.T) {
	var emitted []tool.ACPEvent
	ctx := tool.ContextWithACPEventEmitter(context.Background(), func(e tool.ACPEvent) {
		emitted = append(emitted, e)
	})
	ctx = tool.ContextWithToolThreadID(ctx, "thread-1")

	handler := acpUpdateHandler(ctx, "codex", "host-tool-1")
	handler.OnUpdate(acp.SessionUpdate{
		SessionID: "sess-1",
		Kind:      acp.SessionUpdateKindToolCall,
		ToolCall: &acp.ToolCallSnapshot{
			ToolCallID: "acp-tool-1",
			Title:      "Edit file",
			Kind:       "edit",
			Status:     "completed",
			Locations:  []string{"/tmp/note.md"},
			Summary:    "Updated note.md",
		},
	})

	if len(emitted) != 1 {
		t.Fatalf("expected 1 event, got %d", len(emitted))
	}
	got := emitted[0]
	if got.Type != tool.ACPEventTypeToolSnapshot {
		t.Fatalf("expected tool snapshot event, got %s", got.Type)
	}
	if got.ToolCallID != "host-tool-1" || got.ToolName != "edit" {
		t.Fatalf("unexpected tool snapshot payload: %+v", got)
	}
	if got.Diagnostic["runner_id"] != "codex" || got.Diagnostic["acp_session_id"] != "sess-1" {
		t.Fatalf("unexpected diagnostic: %+v", got.Diagnostic)
	}
}

func TestACPUpdateHandlerIgnoresNonToolUpdates(t *testing.T) {
	var emitted []tool.ACPEvent
	ctx := tool.ContextWithACPEventEmitter(context.Background(), func(e tool.ACPEvent) {
		emitted = append(emitted, e)
	})
	ctx = tool.ContextWithToolThreadID(ctx, "thread-1")

	handler := acpUpdateHandler(ctx, "codex", "host-tool-1")
	handler.OnUpdate(acp.SessionUpdate{Kind: acp.SessionUpdateKindAgentText, TextDelta: "hello"})
	handler.OnUpdate(acp.SessionUpdate{Kind: acp.SessionUpdateKindAgentThought, TextDelta: "thinking"})

	if len(emitted) != 0 {
		t.Fatalf("expected no events for non-tool updates, got %+v", emitted)
	}
}

func TestAllActionsSatisfyStrictOutputContract(t *testing.T) {
	workspace := t.TempDir()
	registry := acp.NewRegistry(filepath.Join(t.TempDir(), "acp-agents.json"))
	if err := registry.UpsertUser(acp.RunnerConfig{
		ID: "contract", Enabled: true, Command: "go", PermissionMode: acp.PermissionModeDeny,
	}); err != nil {
		t.Fatalf("upsert runner: %v", err)
	}
	client := &outputContractClient{}
	manager := acp.NewManager(registry, func(context.Context, acp.RunnerSnapshot, string) (acp.Client, error) {
		return client, nil
	})
	task := &DelegateExternalAgentTool{Manager: manager}

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(workspace, []string{workspace}))
	ctx = tool.ContextWithToolThreadID(ctx, "thread-contract")
	ctx = tool.ContextWithToolExecutionIdentity(ctx, tool.ToolExecutionIdentity{
		RunID: "run-contract", ToolCallID: "call-contract", ToolName: ToolName,
	})

	assertStrictSuccess := func(name, input string) map[string]any {
		t.Helper()
		outcome, err := task.ExecuteOutcome(ctx, input, nil)
		if err != nil {
			t.Fatalf("%s ExecuteOutcome: %v", name, err)
		}
		outcome = outcome.Normalize(tool.OutcomeNormalizationContext{
			OperationType: ToolName, OutputSchema: task.Definition().OutputSchema, StrictOutputSchema: true,
		})
		if outcome.Status != tool.OutcomeSucceeded {
			t.Fatalf("%s did not satisfy strict output contract: %+v", name, outcome)
		}
		if outcome.Details == nil {
			t.Fatalf("%s returned no details", name)
		}
		return outcome.Details
	}

	if action := assertStrictSuccess("list", `{"action":"list"}`)["action"]; action != delegateActionList {
		t.Fatalf("list action = %v", action)
	}
	if action := assertStrictSuccess("status sessions", `{"action":"status"}`)["action"]; action != delegateActionStatus {
		t.Fatalf("status sessions action = %v", action)
	}
	if action := assertStrictSuccess("start", `{"action":"start","runner":"contract","message":"start"}`)["action"]; action != delegateActionStart {
		t.Fatalf("start action = %v", action)
	}
	if action := assertStrictSuccess("status runner", `{"action":"status","runner":"contract"}`)["action"]; action != delegateActionStatus {
		t.Fatalf("status runner action = %v", action)
	}
	if action := assertStrictSuccess("message", `{"action":"message","runner":"contract","message":"follow up"}`)["action"]; action != delegateActionMessage {
		t.Fatalf("message action = %v", action)
	}
	if action := assertStrictSuccess("cancel", `{"action":"cancel","runner":"contract"}`)["action"]; action != delegateActionCancel {
		t.Fatalf("cancel action = %v", action)
	}
	if action := assertStrictSuccess("close", `{"action":"close","runner":"contract"}`)["action"]; action != delegateActionClose {
		t.Fatalf("close action = %v", action)
	}
	if calls := client.promptCalls.Load(); calls != 2 {
		t.Fatalf("expected start and message prompts, got %d", calls)
	}
}

func TestDelegateOutputSchemaRejectsUnknownFields(t *testing.T) {
	task := &DelegateExternalAgentTool{}
	outcome := tool.SucceededOutcome(`{"action":"list","runners":[],"unexpected":true}`)
	outcome.Details = map[string]any{"action": delegateActionList, "runners": []any{}, "unexpected": true}
	outcome = outcome.Normalize(tool.OutcomeNormalizationContext{
		OperationType: ToolName, OutputSchema: task.Definition().OutputSchema, StrictOutputSchema: true,
	})
	if outcome.ErrorCode != "tool_output_schema_mismatch" {
		t.Fatalf("expected strict mismatch, got %+v", outcome)
	}
}

func TestStatusReportsFailedTransportWithStrictContract(t *testing.T) {
	workspace := t.TempDir()
	registry := acp.NewRegistry(filepath.Join(t.TempDir(), "acp-agents.json"))
	if err := registry.UpsertUser(acp.RunnerConfig{ID: "broken", Enabled: true, Command: "go"}); err != nil {
		t.Fatalf("upsert runner: %v", err)
	}
	client := &outputContractClient{promptErr: &transportTestError{message: "write |1: broken pipe"}}
	manager := acp.NewManager(registry, func(context.Context, acp.RunnerSnapshot, string) (acp.Client, error) {
		return client, nil
	})
	task := &DelegateExternalAgentTool{Manager: manager}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(workspace, []string{workspace}))
	ctx = tool.ContextWithToolThreadID(ctx, "thread-failed-status")

	startOutcome, err := task.ExecuteOutcome(ctx, `{"action":"start","runner":"broken","message":"fail"}`, nil)
	if err != nil || startOutcome.ErrorCode != acp.ErrCodeTransportClosed {
		t.Fatalf("expected start transport failure, outcome=%+v err=%v", startOutcome, err)
	}
	statusOutcome, err := task.ExecuteOutcome(ctx, `{"action":"status","runner":"broken"}`, nil)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	statusOutcome = statusOutcome.Normalize(tool.OutcomeNormalizationContext{
		OperationType: ToolName, OutputSchema: task.Definition().OutputSchema, StrictOutputSchema: true,
	})
	if statusOutcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("failed-session status violated output contract: %+v", statusOutcome)
	}
	if statusOutcome.Details["session_state"] != string(acp.ConversationStateFailed) || statusOutcome.Details["last_error_code"] != acp.ErrCodeTransportClosed {
		t.Fatalf("failed-session status lost transport facts: %+v", statusOutcome.Details)
	}
}

func TestTransportFailureRequiresNewSessionInsteadOfBlindRetry(t *testing.T) {
	outcome := delegateFailureOutcome(&transportTestError{message: acp.ErrCodeTransportClosed + ": broken pipe"})
	if outcome.Retryable {
		t.Fatalf("transport-closed delegation must not be blindly retryable: %+v", outcome)
	}
	if outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectUnknown {
		t.Fatalf("transport-closed side effect must remain unknown: %+v", outcome)
	}
	if len(outcome.RecoveryOptions) == 0 {
		t.Fatalf("transport-closed outcome must explain recovery: %+v", outcome)
	}
}

type transportTestError struct{ message string }

func (e *transportTestError) Error() string { return e.message }
