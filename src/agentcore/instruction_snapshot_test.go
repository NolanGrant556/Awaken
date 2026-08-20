package agentcore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/instructions"
)

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestInstructionSnapshotBuildsRequestOnlySyntheticPrefix(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "AGENTS.md"), []byte("global frozen rule"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot := instructions.NewLoader(home).Load("", true)
	conversation := convcontext.New()
	conversation.AddUserMessage("direct request")
	builder := NewRequestBuilder(AgentConfig{InstructionSnapshot: &snapshot})
	req := builder.BuildLLMRequestFromEnvelope(builder.BuildContextEnvelope(conversation, nil))
	if len(req.PrefixMessages) != 1 || req.PrefixMessages[0].Origin != convcontext.MessageOriginSynthetic {
		t.Fatalf("prefix=%+v", req.PrefixMessages)
	}
	if !strings.Contains(req.PrefixMessages[0].Content, "global frozen rule") {
		t.Fatalf("prefix does not contain instructions: %q", req.PrefixMessages[0].Content)
	}
	if len(req.Messages) != 1 || req.Messages[0].Content != "direct request" {
		t.Fatalf("conversation was mutated: %+v", req.Messages)
	}
	if got := conversation.GetMessages(); len(got) != 1 || strings.Contains(got[0].Content, "global frozen rule") {
		t.Fatalf("snapshot leaked into conversation: %+v", got)
	}
}

func TestInstructionSnapshotSemanticRecordContainsNoBody(t *testing.T) {
	home := t.TempDir()
	secretMarker := "BODY_MUST_NOT_ENTER_JOURNAL"
	if err := os.WriteFile(filepath.Join(home, "AGENTS.md"), []byte(secretMarker), 0o600); err != nil {
		t.Fatal(err)
	}
	summary := instructions.NewLoader(home).Load("", true).SafeSummary()
	record := InstructionSnapshotSemanticRecord("thread", "run", "session", summary)
	if record.Kind != SemanticKindInstructionSnapshot {
		t.Fatalf("kind=%s", record.Kind)
	}
	if strings.Contains(string(mustJSON(t, record.Payload)), secretMarker) {
		t.Fatal("instruction body leaked into semantic payload")
	}
}
