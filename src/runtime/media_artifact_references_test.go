package runtime

import (
	"strings"
	"testing"

	"awaken/src/agentcore"
	"awaken/src/tool"
)

func TestMediaArtifactReferencesPreserveGroupPositionAfterRetry(t *testing.T) {
	event := func(id string, seq uint64, groupIndex int) Event {
		return Event{
			Kind:  EventKindAgent,
			Seq:   seq,
			RunID: "run-images",
			AgentEvent: agentcore.AwakenEvent{
				Type:               agentcore.ArtifactCreatedEventType,
				ArtifactID:         id,
				ArtifactGroupID:    "group-5",
				ArtifactGroupIndex: groupIndex,
				ArtifactTitle:      id + ".png",
				ArtifactKind:       "image",
				ArtifactPath:       "output/" + id + ".png",
				ArtifactRole:       tool.ArtifactRoleDeliverable,
				ArtifactStatus:     agentcore.ArtifactStatusReady,
			},
		}
	}
	transcript := ThreadTranscript{Runs: []ThreadTranscriptRun{{
		Run: Run{RunID: "run-images"},
		Events: []Event{
			event("image-1", 1, 1),
			event("image-2", 2, 2),
			event("image-4", 3, 4),
			event("image-5", 4, 5),
			event("image-3-retry", 5, 3),
		},
	}}}

	references := mediaArtifactReferencesFromTranscript(transcript)
	wantIDs := []string{"image-1", "image-2", "image-3-retry", "image-4", "image-5"}
	if len(references) != len(wantIDs) {
		t.Fatalf("expected %d references, got %+v", len(wantIDs), references)
	}
	for index, wantID := range wantIDs {
		if references[index].ID != wantID || references[index].DisplayIndex != index+1 {
			t.Fatalf("reference %d = %+v, want id=%q display_index=%d", index, references[index], wantID, index+1)
		}
	}

	effective := effectiveInputForMediaArtifactReferences("把第3张做成视频", references)
	if !strings.Contains(effective, `"display_index":3,"artifact_id":"image-3-retry"`) {
		t.Fatalf("host manifest did not bind visible position 3 to the retried artifact: %s", effective)
	}
	if !strings.Contains(effective, "调用工具时使用对应artifact_id") || !strings.HasSuffix(effective, "把第3张做成视频") {
		t.Fatalf("host manifest lost the ordinal rule or visible user input: %s", effective)
	}
}

func TestMediaArtifactReferencesExcludePreviewAndUnsupportedArtifacts(t *testing.T) {
	transcript := ThreadTranscript{Runs: []ThreadTranscriptRun{{
		Run: Run{RunID: "run-media"},
		Events: []Event{
			{Kind: EventKindAgent, AgentEvent: agentcore.AwakenEvent{
				Type: agentcore.ArtifactCreatedEventType, ArtifactID: "preview", ArtifactKind: "image",
				ArtifactPath: "output/preview.png", ArtifactRole: tool.ArtifactRolePreview, ArtifactStatus: agentcore.ArtifactStatusReady,
			}},
			{Kind: EventKindAgent, AgentEvent: agentcore.AwakenEvent{
				Type: agentcore.ArtifactCreatedEventType, ArtifactID: "document", ArtifactKind: "document",
				ArtifactPath: "output/report.pdf", ArtifactRole: tool.ArtifactRoleDeliverable, ArtifactStatus: agentcore.ArtifactStatusReady,
			}},
		},
	}}}

	if references := mediaArtifactReferencesFromTranscript(transcript); len(references) != 0 {
		t.Fatalf("only ready deliverable media may enter the host reference manifest: %+v", references)
	}
	if got := effectiveInputForMediaArtifactReferences("继续", nil); got != "继续" {
		t.Fatalf("input without media references must remain unchanged: %q", got)
	}
}
