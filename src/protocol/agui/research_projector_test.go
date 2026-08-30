package agui

import (
	"testing"
	"time"

	"awaken/src/agentcore"
)

func TestProjectResearchAcceptedEmitsPhaseFrame(t *testing.T) {
	frames := ProjectSemanticRecord(agentcore.SemanticRecord{
		Kind:      agentcore.SemanticKindResearchAccepted,
		Timestamp: time.UnixMilli(1_700_000_000_000),
		ThreadID:  "thread",
		RunID:     "run",
		TurnID:    "turn",
		RecordID:  "rec_research_accepted",
		Payload: map[string]any{
			"query":    "what is WebSearch",
			"breadth":  4,
			"revision": 1,
			"phase":    "accepted",
		},
	})
	if len(frames) != 1 {
		t.Fatalf("expected one frame, got %+v", frames)
	}
	if frames[0].Type != EventAwakenResearchPhase {
		t.Fatalf("expected AWAKEN_RESEARCH_PHASE, got %s", frames[0].Type)
	}
	if frames[0].ResearchPhase != "accepted" || frames[0].ResearchRevision != 1 {
		t.Fatalf("phase payload: %+v", frames[0])
	}
}
