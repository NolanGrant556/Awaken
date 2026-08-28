package runtime

import (
	"encoding/json"
	"testing"

	"awaken/src/agentcore"
)

// TestActivatedDeferredToolsFromSemanticRecords proves restart restore
// rebuilds the root thread's on-demand loaded deferred tool set: JSON
// round-trip payload shape, order preservation, dedupe, child-agent record
// exclusion, and merging of legacy mcp_tools_activated records with the
// current deferred_tools_activated kind.
func TestActivatedDeferredToolsFromSemanticRecords(t *testing.T) {
	records := []agentcore.SemanticRecord{
		// Legacy MCP-only kind written before the disclosure generalization.
		agentcore.NewSemanticRecord(agentcore.SemanticKindMCPToolsActivated, "thread", "run_1", "turn_1", "call_1", "session", map[string]any{
			"tools":  []string{"mcp__github__create_issue", "mcp__github__list_issues"},
			"source": "mcp_find",
		}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindDeferredToolsActivated, "thread", "run_1", "turn_2", "call_2", "session", map[string]any{
			"tools":    []string{"mcp__notes__write"},
			"source":   "direct_call",
			"agent_id": "child_1",
		}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindDeferredToolsActivated, "thread", "run_2", "turn_3", "call_3", "session", map[string]any{
			"tools":  []string{"mcp__github__create_issue", "configure_hook"},
			"source": "tool_find",
		}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindSkillActivated, "thread", "run_2", "turn_4", "call_4", "session", map[string]any{
			"skill_id": "pdf", "content": "body",
		}),
	}
	// Round-trip through JSON so payload slices arrive as []any exactly like
	// records loaded back from the durable journal.
	encoded, err := json.Marshal(records)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var loaded []agentcore.SemanticRecord
	if err := json.Unmarshal(encoded, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	restored := activatedDeferredToolsFromSemanticRecords(loaded)
	want := []string{"mcp__github__create_issue", "mcp__github__list_issues", "configure_hook"}
	if len(restored) != len(want) {
		t.Fatalf("restored = %+v, want %+v", restored, want)
	}
	for index := range want {
		if restored[index] != want[index] {
			t.Fatalf("restored order wrong at %d: %+v, want %+v", index, restored, want)
		}
	}

	transcript := restoredTranscriptFromSemantic(loaded)
	if len(transcript.ActivatedDeferredTools) != len(want) {
		t.Fatalf("restoredTranscript must carry activations: %+v", transcript.ActivatedDeferredTools)
	}
}
