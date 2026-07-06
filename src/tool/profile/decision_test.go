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
	input := map[string]any{"patch": "*** Begin Patch\n*** Add File: a.txt\n+hello\n*** End Patch"}
	d := TransformDecision("shell echo rewritten to apply_patch", "apply_patch", input)
	if d.Action != DecisionTransform {
		t.Errorf("Action = %q, want %q", d.Action, DecisionTransform)
	}
	if d.Source != DecisionSourceTransform {
		t.Errorf("Source = %q, want %q", d.Source, DecisionSourceTransform)
	}
	if d.Reason != "shell echo rewritten to apply_patch" {
		t.Errorf("Reason = %q, want %q", d.Reason, "shell echo rewritten to apply_patch")
	}
	if d.UpdatedToolName != "apply_patch" {
		t.Errorf("UpdatedToolName = %q, want %q", d.UpdatedToolName, "apply_patch")
	}
	if d.UpdatedInput == nil {
		t.Fatal("UpdatedInput should not be nil")
	}
	patch, ok := d.UpdatedInput["patch"].(string)
	if !ok {
		t.Fatalf("UpdatedInput[\"patch\"] is not a string: %T", d.UpdatedInput["patch"])
	}
	if !strings.Contains(patch, "*** Add File: a.txt") {
		t.Errorf("UpdatedInput patch = %q, expected to contain Add File for a.txt", patch)
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
	d := TransformDecision("rewritten", "apply_patch", map[string]any{"patch": "..."})
	s := d.String()
	for _, want := range []string{"transform", "apply_patch", "rewritten"} {
		if !strings.Contains(s, want) {
			t.Errorf("String() = %q, expected to contain %q", s, want)
		}
	}
	// Transform decisions should include the tool name in the output.
	if !strings.Contains(s, "tool=apply_patch") {
		t.Errorf("String() = %q, expected to contain tool=apply_patch", s)
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
