package agui

import (
	"testing"
	"time"

	"awaken/src/agentcore"
)

func TestMapEventReasoningLifecycle(t *testing.T) {
	ts := time.Unix(0, 0).UTC()
	events := MapEvent(agentcore.AwakenEvent{
		Type:      agentcore.ReasoningStartedEventType,
		Timestamp: ts,
		ThreadID:  "thread-1",
		RunID:     "run-1",
		MessageID: "reasoning-1",
	})

	if len(events) != 2 {
		t.Fatalf("expected 2 AG-UI events, got %d", len(events))
	}
	if events[0].Type != EventReasoningStart || events[1].Type != EventReasoningMessageStart {
		t.Fatalf("unexpected event types: %+v", events)
	}
	if events[1].Role != "reasoning" {
		t.Fatalf("expected reasoning role, got %q", events[1].Role)
	}
	if events[0].Timestamp != 0 {
		t.Fatalf("expected unix epoch millis, got %d", events[0].Timestamp)
	}
}

func TestMapEventToolResult(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:       agentcore.ToolResultEventType,
		ThreadID:   "thread-1",
		RunID:      "run-1",
		MessageID:  "toolmsg-1",
		ToolCallID: "tool-1",
		Content:    "echo:hello",
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventToolCallResult {
		t.Fatalf("expected TOOL_CALL_RESULT, got %s", events[0].Type)
	}
	if events[0].Role != "tool" {
		t.Fatalf("expected tool role, got %q", events[0].Role)
	}
	if events[0].ToolCallName != "" {
		t.Fatalf("expected empty tool name when not provided, got %q", events[0].ToolCallName)
	}
	if events[0].Content != "echo:hello" {
		t.Fatalf("unexpected content %q", events[0].Content)
	}
}

func TestMapEventToolCallArgsDelta(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:       agentcore.ToolCallArgsDeltaEventType,
		ThreadID:   "thread-1",
		RunID:      "run-1",
		ToolCallID: "tool-1",
		ToolName:   "read_file",
		Delta:      `{"path"`,
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventToolCallArgsDelta {
		t.Fatalf("expected TOOL_CALL_ARGS_DELTA, got %s", events[0].Type)
	}
	if events[0].ToolCallID != "tool-1" || events[0].ToolCallName != "read_file" || events[0].Delta != `{"path"` {
		t.Fatalf("unexpected args delta payload: %+v", events[0])
	}
}

func TestMapEventToolCallArgs(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:       agentcore.ToolCallArgsEventType,
		ThreadID:   "thread-1",
		RunID:      "run-1",
		ToolCallID: "tool-1",
		ToolName:   "read_file",
		Arguments:  `{"path":"README.md"}`,
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventToolCallArgs {
		t.Fatalf("expected TOOL_CALL_ARGS, got %s", events[0].Type)
	}
	if events[0].ToolCallID != "tool-1" || events[0].ToolCallName != "read_file" || events[0].Delta != `{"path":"README.md"}` {
		t.Fatalf("unexpected args payload: %+v", events[0])
	}
}

func TestMapEventToolProgress(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:       agentcore.ToolProgressEventType,
		ThreadID:   "thread-1",
		RunID:      "run-1",
		ToolCallID: "tool-1",
		ToolName:   "read_file",
		Progress:   "Executing tool",
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventToolProgress {
		t.Fatalf("expected TOOL_PROGRESS, got %s", events[0].Type)
	}
	if events[0].ToolCallID != "tool-1" || events[0].ToolCallName != "read_file" {
		t.Fatalf("unexpected tool identity: %+v", events[0])
	}
	if events[0].Progress != "Executing tool" || events[0].Message != "Executing tool" {
		t.Fatalf("unexpected progress payload: %+v", events[0])
	}
}

func TestMapEventCommandOutput(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:          agentcore.CommandOutputEventType,
		ThreadID:      "thread-1",
		RunID:         "run-1",
		ToolCallID:    "tool-1",
		ToolName:      "shell",
		CommandStream: "stdout",
		Delta:         "hello",
		Truncated:     true,
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventAwakenCommandOutput {
		t.Fatalf("expected AWAKEN_COMMAND_OUTPUT, got %s", events[0].Type)
	}
	if events[0].ToolCallID != "tool-1" || events[0].ToolCallName != "shell" || events[0].CommandStream != "stdout" || events[0].Delta != "hello" || !events[0].Truncated {
		t.Fatalf("unexpected command output payload: %+v", events[0])
	}
}

func TestMapEventPatchUpdated(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:                  agentcore.PatchUpdatedEventType,
		ThreadID:              "thread-1",
		RunID:                 "run-1",
		ToolCallID:            "tool-1",
		ToolName:              "apply_patch",
		Patch:                 "*** Begin Patch\n*** End Patch",
		PatchSummary:          "新增 1 个文件",
		ApprovalAffectedPaths: []string{"note.md"},
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventAwakenPatchUpdated {
		t.Fatalf("expected AWAKEN_PATCH_UPDATED, got %s", events[0].Type)
	}
	if events[0].ToolCallID != "tool-1" || events[0].ToolCallName != "apply_patch" || events[0].PatchSummary != "新增 1 个文件" {
		t.Fatalf("unexpected patch identity: %+v", events[0])
	}
	if events[0].Patch == "" || len(events[0].ApprovalAffectedPaths) != 1 || events[0].ApprovalAffectedPaths[0] != "note.md" {
		t.Fatalf("unexpected patch payload: %+v", events[0])
	}
}

func TestMapEventTurnDiff(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:      agentcore.TurnDiffEventType,
		ThreadID:  "thread-1",
		RunID:     "run-1",
		Diff:      "diff --git a/note.md b/note.md",
		Truncated: true,
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventAwakenTurnDiff {
		t.Fatalf("expected AWAKEN_TURN_DIFF, got %s", events[0].Type)
	}
	if events[0].Diff != "diff --git a/note.md b/note.md" || !events[0].Truncated {
		t.Fatalf("unexpected diff payload: %+v", events[0])
	}
}

func TestMapEventApprovalRequested(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:                agentcore.ApprovalRequestedEventType,
		ThreadID:            "thread-1",
		RunID:               "run-1",
		ToolCallID:          "tool-1",
		ToolName:            "write_file",
		ApprovalID:          "approval-1",
		ApprovalTitle:       "Approve write_file",
		ApprovalDescription: "Write README.md",
		ApprovalRiskTone:    agentcore.ApprovalRiskDanger,
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventAwakenApprovalRequested {
		t.Fatalf("expected AWAKEN_APPROVAL_REQUESTED, got %s", events[0].Type)
	}
	if events[0].ApprovalID != "approval-1" || events[0].ApprovalRiskTone != "danger" {
		t.Fatalf("unexpected approval payload: %+v", events[0])
	}
}

func TestMapEventArtifactUpdated(t *testing.T) {
	events := MapEvent(agentcore.AwakenEvent{
		Type:                agentcore.ArtifactUpdatedEventType,
		ThreadID:            "thread-1",
		RunID:               "run-1",
		ToolCallID:          "tool-1",
		ToolName:            "write_file",
		ArtifactID:          "artifact-1",
		ArtifactTitle:       "README.md",
		ArtifactDescription: "Updated project readme",
		ArtifactKind:        "file",
		ArtifactPath:        "/tmp/README.md",
		ArtifactContent:     "# hello",
		ArtifactStatus:      agentcore.ArtifactStatusReady,
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventAwakenArtifactUpdated {
		t.Fatalf("expected AWAKEN_ARTIFACT_UPDATED, got %s", events[0].Type)
	}
	if events[0].ArtifactPath != "/tmp/README.md" || events[0].ArtifactStatus != "ready" {
		t.Fatalf("unexpected artifact payload: %+v", events[0])
	}
}
