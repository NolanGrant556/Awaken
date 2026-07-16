package profile

import (
	"strings"
	"testing"
)

// --- Constructor tests ---

func TestAllowDecision(t *testing.T) {
	d := AllowDecision(string(DecisionSourceToolRule), "tool allowed execution")
	if d.Action != DecisionAllow {
		t.Errorf("Action = %q, want %q", d.Action, DecisionAllow)
	}
	if d.Source != DecisionSourceToolRule {
		t.Errorf("Source = %q, want %q", d.Source, DecisionSourceToolRule)
	}
	if d.Reason != "tool allowed execution" {
		t.Errorf("Reason = %q, want %q", d.Reason, "tool allowed execution")
	}
	if d.Retryable {
		t.Error("Retryable should be false for allow decisions")
	}
	if d.UpdatedToolName != "" {
		t.Errorf("UpdatedToolName = %q, want empty", d.UpdatedToolName)
	}
	if d.UpdatedInput != nil {
		t.Errorf("UpdatedInput = %v, want nil", d.UpdatedInput)
	}
}

func TestDenyDecision(t *testing.T) {
	d := DenyDecision(string(DecisionSourceHeadlessDefault), "denied by default", true)
	if d.Action != DecisionDeny {
		t.Errorf("Action = %q, want %q", d.Action, DecisionDeny)
	}
	if d.Source != DecisionSourceHeadlessDefault {
		t.Errorf("Source = %q, want %q", d.Source, DecisionSourceHeadlessDefault)
	}
	if d.Reason != "denied by default" {
		t.Errorf("Reason = %q, want %q", d.Reason, "denied by default")
	}
	if !d.Retryable {
		t.Error("Retryable should be true")
	}
}

func TestDenyDecisionNotRetryable(t *testing.T) {
	d := DenyDecision(string(DecisionSourceSensitivePath), "touches .git/config", false)
	if d.Retryable {
		t.Error("Retryable should be false")
	}
	if d.Source != DecisionSourceSensitivePath {
		t.Errorf("Source = %q, want %q", d.Source, DecisionSourceSensitivePath)
	}
}

func TestTransformDecision(t *testing.T) {
	input := map[string]any{"file_path": "a.txt", "content": "hello"}
	d := TransformDecision("legacy source rewritten to Write", "Write", input)
	if d.Action != DecisionTransform {
		t.Errorf("Action = %q, want %q", d.Action, DecisionTransform)
	}
	if d.Source != DecisionSourceTransform {
		t.Errorf("Source = %q, want %q", d.Source, DecisionSourceTransform)
	}
	if d.Reason != "legacy source rewritten to Write" {
		t.Errorf("Reason = %q, want %q", d.Reason, "legacy source rewritten to Write")
	}
	if d.UpdatedToolName != "Write" {
		t.Errorf("UpdatedToolName = %q, want %q", d.UpdatedToolName, "Write")
	}
	if d.UpdatedInput == nil {
		t.Fatal("UpdatedInput should not be nil")
	}
	content, ok := d.UpdatedInput["content"].(string)
	if !ok {
		t.Fatalf("UpdatedInput[\"content\"] is not a string: %T", d.UpdatedInput["content"])
	}
	if content != "hello" {
		t.Errorf("UpdatedInput content = %q, want hello", content)
	}
}

// --- String() tests ---

func TestExecutionDecision_StringAllow(t *testing.T) {
	d := AllowDecision(string(DecisionSourceWorkspaceBoundary), "read-only tool allowed")
	s := d.String()
	if s == "" {
		t.Fatal("String() should not be empty")
	}
	for _, want := range []string{"allow", "workspace_boundary", "read-only tool allowed"} {
		if !strings.Contains(s, want) {
			t.Errorf("String() = %q, expected to contain %q", s, want)
		}
	}
}

func TestExecutionDecision_StringDeny(t *testing.T) {
	d := DenyDecision(string(DecisionSourceHeadlessDefault), "open-world denied", false)
	s := d.String()
	for _, want := range []string{"deny", "headless_default", "open-world denied"} {
		if !strings.Contains(s, want) {
			t.Errorf("String() = %q, expected to contain %q", s, want)
		}
	}
}

func TestExecutionDecision_StringTransform(t *testing.T) {
	d := TransformDecision("rewritten", "Write", map[string]any{"content": "..."})
	s := d.String()
	for _, want := range []string{"transform", "Write", "rewritten"} {
		if !strings.Contains(s, want) {
			t.Errorf("String() = %q, expected to contain %q", s, want)
		}
	}
	// Transform decisions should include the tool name in the output.
	if !strings.Contains(s, "tool=Write") {
		t.Errorf("String() = %q, expected to contain tool=Write", s)
	}
}

// --- ExecutionDecisionAction constant tests ---

func TestExecutionDecisionActionValues(t *testing.T) {
	if DecisionAllow != "allow" {
		t.Errorf("DecisionAllow = %q, want %q", DecisionAllow, "allow")
	}
	if DecisionDeny != "deny" {
		t.Errorf("DecisionDeny = %q, want %q", DecisionDeny, "deny")
	}
	if DecisionTransform != "transform" {
		t.Errorf("DecisionTransform = %q, want %q", DecisionTransform, "transform")
	}
}

func TestExecutionDecisionSourceValues(t *testing.T) {
	tests := []struct {
		value ExecutionDecisionSource
		want  string
	}{
		{DecisionSourceToolRule, "tool_rule"},
		{DecisionSourceAutoAllowlist, "auto_allowlist"},
		{DecisionSourceWorkspaceBoundary, "workspace_boundary"},
		{DecisionSourceHeadlessDefault, "headless_default"},
		{DecisionSourcePolicyError, "policy_error"},
		{DecisionSourceTransform, "transform"},
		{DecisionSourceSensitivePath, "sensitive_path"},
	}
	for _, tt := range tests {
		if string(tt.value) != tt.want {
			t.Errorf("source = %q, want %q", tt.value, tt.want)
		}
	}
}
