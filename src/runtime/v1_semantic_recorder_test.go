package runtime

import (
	"context"
	"path/filepath"
	"testing"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
)

func TestV1SemanticRecorderPersistsLegacyCompactionAsActiveView(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thread_legacy_compact.jsonl")
	recorder := newV1SemanticRecorder(path, "thread_legacy_compact")

	before := []agentcore.SemanticRecord{
		agentcore.NewSemanticRecord(agentcore.SemanticKindUserMessage, "thread_legacy_compact", "run_before", "turn_before", "", "session", map[string]any{"content": "old request"}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindAssistantMessage, "thread_legacy_compact", "run_before", "turn_before", "", "session", map[string]any{"content": "old answer"}),
	}
	if err := recorder.AppendBatch(context.Background(), before); err != nil {
		t.Fatalf("append pre-compact records: %v", err)
	}

	boundaryID := "compact_legacy_v1"
	compact := agentcore.NewSemanticRecord(agentcore.SemanticKindContextCompacted, "thread_legacy_compact", "run_compact", "", "", "session", map[string]any{
		"compact_boundary_id": boundaryID,
		"messages": []agentcore.SemanticCompactedMessage{
			{Role: string(convcontext.RoleUser), Content: "durable summary", CompactBoundaryID: boundaryID},
			{Role: string(convcontext.RoleUser), Content: "latest request"},
		},
	})
	if err := recorder.Append(context.Background(), compact); err != nil {
		t.Fatalf("append compact record: %v", err)
	}

	restored, err := loadRestoredTranscriptAtPath(path)
	if err != nil {
		t.Fatalf("restore v1 transcript: %v", err)
	}
	conversation := convcontext.New()
	conversation.SetMessages(restored.Messages)
	active := conversation.GetMessagesForModel()
	if len(active) != 2 || active[0].Content != "durable summary" || active[1].Content != "latest request" {
		t.Fatalf("unexpected compacted active view: %+v", active)
	}
	if active[0].CompactBoundaryID != boundaryID {
		t.Fatalf("missing compact boundary: %+v", active[0])
	}
}
