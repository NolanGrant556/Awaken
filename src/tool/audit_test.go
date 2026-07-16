package tool

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestToolAuditRecordSerializesNewObservabilityFields verifies that the
// Task 16 unified observability fields round-trip through JSON correctly.
// The FileSink writes JSONL, so every field on ToolAuditRecord must survive
// json.Marshal / json.Unmarshal.
func TestToolAuditRecordSerializesNewObservabilityFields(t *testing.T) {
	record := ToolAuditRecord{
		ToolCallID:        "call_16",
		RunID:             "run_16",
		ToolName:          "Edit",
		Decision:          "allow",
		DecisionReason:    "in-workspace write",
		DecisionOrigin:    "auto_allow",
		Outcome:           "success",
		ResultSummary:     "Updated note.md",
		ProfileName:       "workspace_write",
		Intent:            map[string]any{"ToolName": "Edit", "ToolKind": "file_operation"},
		DecisionAction:    "allow",
		DecisionSource:    "workspace_boundary",
		TransformedFrom:   "legacy_source",
		TransformedTo:     "Write",
		TrashedPaths:      []string{"/tmp/trash/note.md"},
		SubagentOrigin:    map[string]any{"AgentID": "agent_1", "Depth": 1},
		GuardianViolation: "guardian profile requires a read-only tool",
	}

	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("failed to marshal audit record: %v", err)
	}

	var decoded ToolAuditRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal audit record: %v", err)
	}

	if decoded.ProfileName != record.ProfileName {
		t.Errorf("ProfileName = %q, want %q", decoded.ProfileName, record.ProfileName)
	}
	if decoded.DecisionAction != record.DecisionAction {
		t.Errorf("DecisionAction = %q, want %q", decoded.DecisionAction, record.DecisionAction)
	}
	if decoded.DecisionSource != record.DecisionSource {
		t.Errorf("DecisionSource = %q, want %q", decoded.DecisionSource, record.DecisionSource)
	}
	if decoded.TransformedFrom != record.TransformedFrom {
		t.Errorf("TransformedFrom = %q, want %q", decoded.TransformedFrom, record.TransformedFrom)
	}
	if decoded.TransformedTo != record.TransformedTo {
		t.Errorf("TransformedTo = %q, want %q", decoded.TransformedTo, record.TransformedTo)
	}
	if len(decoded.TrashedPaths) != 1 || decoded.TrashedPaths[0] != record.TrashedPaths[0] {
		t.Errorf("TrashedPaths = %v, want %v", decoded.TrashedPaths, record.TrashedPaths)
	}
	if decoded.GuardianViolation != record.GuardianViolation {
		t.Errorf("GuardianViolation = %q, want %q", decoded.GuardianViolation, record.GuardianViolation)
	}
	if decoded.Intent == nil {
		t.Error("Intent should not be nil after round-trip")
	}
	if decoded.SubagentOrigin == nil {
		t.Error("SubagentOrigin should not be nil after round-trip")
	}
}

// TestToolAuditRecordOmitsEmptyObservabilityFields verifies that the
// omitempty tags work so a minimal allow record doesn't carry empty
// transform/trash/guardian noise.
func TestToolAuditRecordOmitsEmptyObservabilityFields(t *testing.T) {
	record := ToolAuditRecord{
		ToolCallID:     "call_minimal",
		ToolName:       "Read",
		Decision:       "allow",
		Outcome:        "success",
		ProfileName:    "workspace_write",
		DecisionAction: "allow",
	}

	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	jsonStr := string(data)

	// These fields should be absent when empty.
	for _, field := range []string{
		`"transformedFrom"`,
		`"transformedTo"`,
		`"trashedPaths"`,
		`"subagentOrigin"`,
		`"guardianViolation"`,
		`"intent"`,
	} {
		if strings.Contains(jsonStr, field) {
			t.Errorf("expected %s to be omitted from %s", field, jsonStr)
		}
	}
}

// TestInMemorySinkCapturesObservabilityFields verifies that the InMemorySink
// (used in tests and available for per-run audit wiring) preserves the new
// fields across Write/Records.
func TestInMemorySinkCapturesObservabilityFields(t *testing.T) {
	sink := &InMemorySink{}
	record := ToolAuditRecord{
		ToolCallID:        "call_sink",
		ProfileName:       "guardian_read_only",
		DecisionAction:    "deny",
		TrashedPaths:      []string{"/tmp/deleted.md"},
		GuardianViolation: "guardian forbids action write",
	}
	if err := sink.Write(record); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	records := sink.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	got := records[0]
	if got.ProfileName != record.ProfileName {
		t.Errorf("ProfileName = %q, want %q", got.ProfileName, record.ProfileName)
	}
	if got.DecisionAction != record.DecisionAction {
		t.Errorf("DecisionAction = %q, want %q", got.DecisionAction, record.DecisionAction)
	}
	if len(got.TrashedPaths) != 1 || got.TrashedPaths[0] != record.TrashedPaths[0] {
		t.Errorf("TrashedPaths = %v, want %v", got.TrashedPaths, record.TrashedPaths)
	}
	if got.GuardianViolation != record.GuardianViolation {
		t.Errorf("GuardianViolation = %q, want %q", got.GuardianViolation, record.GuardianViolation)
	}
}
