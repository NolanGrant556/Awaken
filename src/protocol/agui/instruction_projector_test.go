package agui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"awaken/src/agentcore"
	"awaken/src/instructions"
)

func TestInstructionSnapshotProjectionCarriesOnlySafeSummary(t *testing.T) {
	summary := instructions.Summary{
		SnapshotID: "instructions-1", CapturedAt: time.Unix(1, 0).UTC(), Partial: true,
		Sources:     []instructions.SourceSummary{{Scope: instructions.ScopeProject, DisplayPath: ".awaken/AGENTS.md", ScopeRoot: "/project", Digest: "digest", OriginalBytes: 12, AppliedBytes: 12, Status: instructions.SourceStatusApplied}},
		Diagnostics: []instructions.Diagnostic{{Code: "SOURCE_TRUNCATED", Message: "truncated", Scope: instructions.ScopeProject, DisplayPath: ".awaken/AGENTS.md"}},
	}
	record := agentcore.InstructionSnapshotSemanticRecord("thread", "run", "session", summary)
	frames := ProjectSemanticRecord(record)
	if len(frames) != 1 || frames[0].Type != EventAwakenInstructionSnapshot || frames[0].InstructionSnapshot == nil {
		t.Fatalf("frames=%+v", frames)
	}
	if frames[0].InstructionSnapshot.Sources[0].ScopeRoot != "/project" || !frames[0].InstructionSnapshot.Partial {
		t.Fatalf("snapshot=%+v", frames[0].InstructionSnapshot)
	}
}

func TestInstructionSnapshotProjectionKeepsEmptyCollectionsInWireContract(t *testing.T) {
	record := agentcore.InstructionSnapshotSemanticRecord("thread", "run", "session", instructions.Summary{
		SnapshotID: "instructions-empty", CapturedAt: time.Unix(2, 0).UTC(),
	})
	frames := ProjectSemanticRecord(record)
	if len(frames) != 1 {
		t.Fatalf("frames=%+v", frames)
	}
	encoded, err := json.Marshal(frames[0])
	if err != nil {
		t.Fatal(err)
	}
	wire := string(encoded)
	if !strings.Contains(wire, `"sources":[]`) || !strings.Contains(wire, `"diagnostics":[]`) {
		t.Fatalf("empty instruction collections must stay present: %s", wire)
	}
}
