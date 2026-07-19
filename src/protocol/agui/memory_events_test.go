package agui

import (
	"testing"
	"time"

	"awaken/src/agentcore"
)

func TestMemoryLiveAndReplayProjectionMatch(t *testing.T) {
	receipt := agentcore.MemoryRecallReceipt{MemoryID: "mem_1", Scope: "project", Type: "project", Revision: 2, Bytes: 100}
	live := MapEvent(agentcore.AwakenEvent{Type: agentcore.MemoryRecalledEventType, ThreadID: "thread", RunID: "run", MemoryReceipts: []agentcore.MemoryRecallReceipt{receipt}})
	record := agentcore.NewSemanticRecord(agentcore.SemanticKindMemoryRecalled, "thread", "run", "turn", "", "session", map[string]any{"count": 1, "memories": []agentcore.MemoryRecallReceipt{receipt}})
	record.Timestamp = time.UnixMilli(live[0].Timestamp)
	replay := ProjectSemanticRecord(record)
	if len(live) != 1 || len(replay) != 1 || live[0].Type != EventAwakenMemoryRecalled || replay[0].Type != live[0].Type || len(replay[0].Memories) != 1 || replay[0].Memories[0].MemoryID != "mem_1" { t.Fatalf("live=%+v replay=%+v", live, replay) }
}

func TestMemoryMutationProjectionUsesActionSpecificEvent(t *testing.T) {
	receipt := agentcore.MemoryRecallReceipt{MemoryID: "mem_2", Scope: "user", Type: "feedback", Revision: 1, Action: "create", Status: "active"}
	live := MapEvent(agentcore.AwakenEvent{Type: agentcore.MemoryMutatedEventType, MemoryReceipts: []agentcore.MemoryRecallReceipt{receipt}})
	if len(live) != 1 || live[0].Type != EventAwakenMemoryCreated { t.Fatalf("unexpected live mutation: %+v", live) }
	record := agentcore.NewSemanticRecord(agentcore.SemanticKindMemoryMutated, "thread", "run", "turn", "call", "session", map[string]any{"memory": receipt})
	replay := ProjectSemanticRecord(record)
	if len(replay) != 1 || replay[0].Type != EventAwakenMemoryCreated || replay[0].Memories[0].MemoryID != "mem_2" { t.Fatalf("unexpected replay mutation: %+v", replay) }
}
