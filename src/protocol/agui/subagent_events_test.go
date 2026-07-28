package agui

import (
	"testing"
	"time"

	"awaken/src/agentcore"
)

func TestProjectSubagentLifecycleAndSuppressChildNarrative(t *testing.T) {
	now := time.Now().UTC()
	spawned := agentcore.SemanticRecord{
		SchemaVersion: 2, RecordID: "spawned", Seq: 10,
		ThreadID: "thread", RunID: "run", TurnID: "turn",
		Kind: agentcore.SemanticKindSubagentSpawned, Timestamp: now,
		Payload: map[string]any{
			"task_id": "task-1", "agent_id": "agent-1", "parent_agent_id": "root",
			"description": "inspect auth", "agent_type": "explore",
			"workspace_mode": "shared_readonly", "status": "queued",
			"created_at": now.Format(time.RFC3339Nano),
		},
	}
	frames := ProjectSemanticRecord(spawned)
	if len(frames) != 1 || frames[0].Type != EventAwakenSubagentChanged {
		t.Fatalf("spawn frames = %+v", frames)
	}
	if frames[0].SubagentTaskID != "task-1" || frames[0].SubagentStatus != "queued" {
		t.Fatalf("spawn frame identity = %+v", frames[0])
	}

	childMessage := agentcore.SemanticRecord{
		SchemaVersion: 2, RecordID: "message", Seq: 11,
		ThreadID: "thread", RunID: "run", TurnID: "turn",
		Kind: agentcore.SemanticKindAssistantMessage, Timestamp: now,
		Payload: map[string]any{
			"subagent_task_id": "task-1", "agent_id": "agent-1",
			"content": "internal child answer",
		},
	}
	if frames := ProjectSemanticRecord(childMessage); len(frames) != 0 {
		t.Fatalf("child assistant message leaked into root narrative: %+v", frames)
	}

	childTool := childMessage
	childTool.RecordID = "tool"
	childTool.Seq = 12
	childTool.Kind = agentcore.SemanticKindToolCall
	childTool.Payload = map[string]any{
		"subagent_task_id": "task-1", "agent_id": "agent-1",
		"tool_call_id": "call-1", "tool_name": "Read",
	}
	frames = ProjectSemanticRecord(childTool)
	if len(frames) != 1 || frames[0].Type != EventAwakenSubagentActivity ||
		frames[0].SubagentTaskID != "task-1" || frames[0].ToolCallName != "Read" {
		t.Fatalf("child tool activity = %+v", frames)
	}
}
