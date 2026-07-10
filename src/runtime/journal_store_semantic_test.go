package runtime

import (
	"encoding/json"
	"testing"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/tool"
)

func TestMessagesFromSemanticRecordsRestoresCanonicalOutcomeModelPayload(t *testing.T) {
	exitCode := 7
	now := time.Now().UTC()
	outcome := tool.ExecutionOutcome{
		Content:         "command exited with status 7",
		Status:          tool.OutcomeFailed,
		TransportStatus: tool.TransportCompleted,
		SemanticStatus:  tool.SemanticFailure,
		ProcessStatus:   tool.ProcessExited,
		ExitCode:        &exitCode,
		ErrorCode:       tool.ErrorCodeProcessExitNonzero,
		ErrorCategory:   tool.ErrorCategoryProcess,
		ErrorMessage:    "exit status 7",
		Retryable:       true,
		RecoveryOptions: []string{"inspect_exit_code_and_stderr", "change_command_or_stop"},
		Diagnostic:      map[string]any{"stderr": "boom"},
		StartedAt:       now,
		FinishedAt:      now,
	}
	record := agentcore.NewSemanticRecord(
		agentcore.SemanticKindToolResult,
		"thread_1",
		"run_1",
		"turn_1",
		"call_1",
		"",
		map[string]any{
			"tool_call_id": "call_1",
			"tool_name":    "shell",
			"content":      "UI display: command failed",
			"is_error":     false, // stale compatibility bit must not win
			"outcome":      outcomeToAnyMap(outcome),
		},
	)
	messages := messagesFromSemanticRecords([]agentcore.SemanticRecord{record})
	if len(messages) != 1 || messages[0].Role != convcontext.RoleTool {
		t.Fatalf("unexpected restored messages: %+v", messages)
	}
	msg := messages[0]
	if msg.Content == "UI display: command failed" {
		t.Fatal("model history received UI display text instead of Outcome.ModelPayload")
	}
	if !msg.IsError {
		t.Fatal("canonical outcome error state did not override stale compatibility bit")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(msg.Content), &payload); err != nil {
		t.Fatalf("restored model payload is not JSON: %v\n%s", err, msg.Content)
	}
	if payload["status"] != string(tool.OutcomeFailed) ||
		payload["error_code"] != tool.ErrorCodeProcessExitNonzero ||
		payload["exit_code"] != float64(7) ||
		payload["retryable"] != true {
		t.Fatalf("restored model payload lost outcome facts: %#v", payload)
	}
	diagnostic, _ := payload["diagnostic"].(map[string]any)
	if diagnostic["stderr"] != "boom" {
		t.Fatalf("restored model payload lost diagnostic: %#v", payload)
	}
}

func TestMessagesFromSemanticRecordsKeepsLegacyDisplayFallback(t *testing.T) {
	record := agentcore.NewSemanticRecord(
		agentcore.SemanticKindToolResult,
		"thread_1",
		"run_1",
		"turn_1",
		"call_legacy",
		"",
		map[string]any{
			"tool_call_id": "call_legacy",
			"tool_name":    "legacy",
			"content":      "legacy result",
			"is_error":     true,
		},
	)
	messages := messagesFromSemanticRecords([]agentcore.SemanticRecord{record})
	if len(messages) != 1 || messages[0].Content != "legacy result" || !messages[0].IsError {
		t.Fatalf("legacy fallback changed: %+v", messages)
	}
}
